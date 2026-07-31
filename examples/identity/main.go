// Command identity starts a deliberately small OAuth-compatible identity service for local demos.
package main

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"math/big"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

type service struct {
	issuer, clientID, clientSecret string
	key                            *rsa.PrivateKey
	keyID                          string
	now                            func() time.Time
}

func main() {
	address := flag.String("address", env("IDENTITY_ADDRESS", ":8084"), "HTTP listen address")
	healthcheck := flag.String("healthcheck", "", "check an identity health URL and exit")
	flag.Parse()
	if *healthcheck != "" {
		if err := checkHealth(*healthcheck); err != nil {
			log.Fatal(err)
		}
		return
	}
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		log.Fatalf("generate signing key: %v", err)
	}
	identity := &service{
		issuer:       env("IDENTITY_ISSUER", "http://localhost:8084"),
		clientID:     env("IDENTITY_CLIENT_ID", "gateway-demo"),
		clientSecret: env("IDENTITY_CLIENT_SECRET", "gateway-demo-secret"),
		key:          key, keyID: signingKeyID(&key.PublicKey), now: time.Now,
	}
	server := &http.Server{Addr: *address, Handler: identity.handler(), ReadHeaderTimeout: 5 * time.Second}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	go func() {
		<-ctx.Done()
		shutdown, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = server.Shutdown(shutdown)
	}()
	log.Printf("demo identity listening on %s", *address)
	if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		log.Fatal(err)
	}
}

func (s *service) handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, _ *http.Request) { writeJSON(w, 200, map[string]string{"status": "ok"}) })
	mux.HandleFunc("GET /.well-known/jwks.json", s.jwks)
	mux.HandleFunc("POST /token", s.token)
	return mux
}

func (s *service) jwks(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, 200, map[string]any{"keys": []map[string]string{{
		"kty": "RSA", "use": "sig", "alg": "RS256", "kid": s.keyID,
		"n": base64.RawURLEncoding.EncodeToString(s.key.N.Bytes()),
		"e": base64.RawURLEncoding.EncodeToString(big.NewInt(int64(s.key.E)).Bytes()),
	}}})
}

func (s *service) token(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("Pragma", "no-cache")
	r.Body = http.MaxBytesReader(w, r.Body, 4<<10)
	if err := r.ParseForm(); err != nil {
		oauthError(w, 400, "invalid_request")
		return
	}
	clientID, clientSecret, ok := r.BasicAuth()
	if !ok {
		clientID, clientSecret = r.Form.Get("client_id"), r.Form.Get("client_secret")
	}
	if !equal(clientID, s.clientID) || !equal(clientSecret, s.clientSecret) {
		w.Header().Set("WWW-Authenticate", `Basic realm="demo-identity"`)
		oauthError(w, 401, "invalid_client")
		return
	}
	if r.Form.Get("grant_type") != "client_credentials" {
		oauthError(w, 400, "unsupported_grant_type")
		return
	}
	scope := strings.TrimSpace(r.Form.Get("scope"))
	if scope == "" {
		scope = "users.read"
	}
	if scope != "users.read" {
		oauthError(w, 400, "invalid_scope")
		return
	}
	now := s.now()
	claims := jwt.MapClaims{"sub": clientID, "iss": s.issuer, "aud": "api-gateway", "iat": now.Unix(), "nbf": now.Unix(), "exp": now.Add(5 * time.Minute).Unix(), "scope": scope}
	token := jwt.NewWithClaims(jwt.SigningMethodRS256, claims)
	token.Header["kid"] = s.keyID
	raw, err := token.SignedString(s.key)
	if err != nil {
		oauthError(w, 500, "server_error")
		return
	}
	writeJSON(w, 200, map[string]any{"access_token": raw, "token_type": "Bearer", "expires_in": 300, "scope": scope})
}

func oauthError(w http.ResponseWriter, status int, code string) {
	writeJSON(w, status, map[string]string{"error": code})
}
func writeJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}
func env(name, fallback string) string {
	if value := os.Getenv(name); value != "" {
		return value
	}
	return fallback
}
func checkHealth(url string) error {
	client := http.Client{Timeout: time.Second}
	response, err := client.Get(url)
	if err != nil {
		return err
	}
	defer response.Body.Close()
	if response.StatusCode != 200 {
		return fmt.Errorf("health returned %s", response.Status)
	}
	return nil
}
func equal(left, right string) bool {
	return subtle.ConstantTimeCompare([]byte(left), []byte(right)) == 1
}
func signingKeyID(key *rsa.PublicKey) string {
	sum := sha256.Sum256(key.N.Bytes())
	return base64.RawURLEncoding.EncodeToString(sum[:12])
}
