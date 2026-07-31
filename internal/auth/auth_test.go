package auth

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"encoding/base64"
	"encoding/json"
	"math/big"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

func TestVerifierValidatesJWTAndScopes(t *testing.T) {
	key, _ := rsa.GenerateKey(rand.Reader, 2048)
	jwks := jwksServer(t, &key.PublicKey)
	verifier, err := New(context.Background(), Config{
		JWKSURL: jwks.URL, Issuer: "issuer", Audience: "gateway",
		Algorithms: []string{"RS256"}, HTTPTimeout: time.Second,
	})
	if err != nil {
		t.Fatal(err)
	}
	token := signedToken(t, key, jwt.MapClaims{
		"sub": "user-1", "iss": "issuer", "aud": "gateway",
		"exp": time.Now().Add(time.Minute).Unix(), "scope": "users.read billing.read",
	})
	principal, err := verifier.Verify(context.Background(), token)
	if err != nil {
		t.Fatal(err)
	}
	if principal.Subject != "user-1" || !principal.HasScope("billing.read") {
		t.Fatalf("principal = %+v", principal)
	}
}

func TestVerifierRejectsInvalidTokens(t *testing.T) {
	key, _ := rsa.GenerateKey(rand.Reader, 2048)
	jwks := jwksServer(t, &key.PublicKey)
	verifier, err := New(context.Background(), Config{
		JWKSURL: jwks.URL, Issuer: "issuer", Audience: "gateway",
		Algorithms: []string{"RS256"}, HTTPTimeout: time.Second,
	})
	if err != nil {
		t.Fatal(err)
	}
	tests := []string{
		"",
		signedToken(t, key, jwt.MapClaims{"sub": "x", "iss": "wrong", "aud": "gateway", "exp": time.Now().Add(time.Minute).Unix()}),
		signedToken(t, key, jwt.MapClaims{"sub": "x", "iss": "issuer", "aud": "gateway", "exp": time.Now().Add(-time.Minute).Unix()}),
		signedToken(t, key, jwt.MapClaims{"sub": "", "iss": "issuer", "aud": "gateway", "exp": time.Now().Add(time.Minute).Unix()}),
	}
	for _, token := range tests {
		if _, err := verifier.Verify(context.Background(), token); err == nil {
			t.Fatalf("expected token %q to fail", token)
		}
	}
}

func TestNewRejectsUnusableJWKS(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { _, _ = w.Write([]byte(`{"keys":[]}`)) }))
	defer server.Close()
	if _, err := New(context.Background(), Config{JWKSURL: server.URL, HTTPTimeout: time.Second}); err == nil {
		t.Fatal("expected error")
	}
}

func TestNewReportsHTTPAndDocumentErrors(t *testing.T) {
	for _, handler := range []http.HandlerFunc{
		func(w http.ResponseWriter, _ *http.Request) { http.Error(w, "no", http.StatusBadGateway) },
		func(w http.ResponseWriter, _ *http.Request) { _, _ = w.Write([]byte("{")) },
	} {
		server := httptest.NewServer(handler)
		if _, err := New(context.Background(), Config{JWKSURL: server.URL, HTTPTimeout: time.Second}); err == nil {
			t.Fatal("expected error")
		}
		server.Close()
	}
}

func TestMissingKeyRefreshIsRateLimited(t *testing.T) {
	key, _ := rsa.GenerateKey(rand.Reader, 2048)
	var requests atomic.Int32
	n := base64.RawURLEncoding.EncodeToString(key.N.Bytes())
	e := base64.RawURLEncoding.EncodeToString(big.NewInt(int64(key.E)).Bytes())
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		requests.Add(1)
		_ = json.NewEncoder(w).Encode(map[string]any{"keys": []map[string]string{{
			"kty": "RSA", "kid": "known", "n": n, "e": e,
		}}})
	}))
	defer server.Close()
	verifier, err := New(context.Background(), Config{
		JWKSURL: server.URL, Issuer: "issuer", Audience: "gateway",
		Algorithms: []string{"RS256"}, HTTPTimeout: time.Second,
	})
	if err != nil {
		t.Fatal(err)
	}
	token := jwt.NewWithClaims(jwt.SigningMethodRS256, jwt.MapClaims{
		"sub": "x", "iss": "issuer", "aud": "gateway", "exp": time.Now().Add(time.Minute).Unix(),
	})
	token.Header["kid"] = "unknown"
	raw, _ := token.SignedString(key)
	_, _ = verifier.Verify(context.Background(), raw)
	_, _ = verifier.Verify(context.Background(), raw)
	if requests.Load() != 2 {
		t.Fatalf("JWKS requests = %d, want startup plus one refresh", requests.Load())
	}
}

func jwksServer(t *testing.T, key *rsa.PublicKey) *httptest.Server {
	t.Helper()
	n := base64.RawURLEncoding.EncodeToString(key.N.Bytes())
	e := base64.RawURLEncoding.EncodeToString(big.NewInt(int64(key.E)).Bytes())
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{"keys": []map[string]string{{"kty": "RSA", "kid": "key-1", "use": "sig", "alg": "RS256", "n": n, "e": e}}})
	}))
}

func signedToken(t *testing.T, key *rsa.PrivateKey, claims jwt.MapClaims) string {
	t.Helper()
	token := jwt.NewWithClaims(jwt.SigningMethodRS256, claims)
	token.Header["kid"] = "key-1"
	value, err := token.SignedString(key)
	if err != nil {
		t.Fatal(err)
	}
	return value
}
