package integration

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/rsa"
	"encoding/base64"
	"encoding/json"
	"math/big"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"

	"github.com/ivan-gromov-dev/APIGateway/internal/cache"
	"github.com/ivan-gromov-dev/APIGateway/internal/config"
	"github.com/ivan-gromov-dev/APIGateway/test/integration/internal/testenv"
)

func TestJWTAuthenticationProtectsRoute(t *testing.T) {
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	jwks := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{"keys": []map[string]string{{
			"kty": "RSA", "kid": "one", "use": "sig", "alg": "RS256",
			"n": base64.RawURLEncoding.EncodeToString(key.N.Bytes()),
			"e": base64.RawURLEncoding.EncodeToString(big.NewInt(int64(key.E)).Bytes()),
		}}})
	}))
	defer jwks.Close()
	env := testenv.New(t, testenv.WithUsers(1), testenv.WithAuth(
		config.Auth{Providers: map[string]config.JWTProvider{"main": {
			JWKSURL: jwks.URL, Issuer: "issuer", Audience: "gateway",
			Algorithms: []string{"RS256"}, HTTPTimeout: time.Second,
		}}},
		config.RouteAuth{Required: true, Provider: "main", RequiredScopes: []string{"users.read"}},
	))
	if response := env.GET("/api/users"); response.Status != http.StatusUnauthorized {
		t.Fatalf("unauthenticated status = %d", response.Status)
	}
	token := jwt.NewWithClaims(jwt.SigningMethodRS256, jwt.MapClaims{
		"sub": "user-1", "iss": "issuer", "aud": "gateway",
		"exp": time.Now().Add(time.Minute).Unix(), "scope": "users.read",
	})
	token.Header["kid"] = "one"
	raw, err := token.SignedString(key)
	if err != nil {
		t.Fatal(err)
	}
	response := env.Do(http.MethodGet, "/api/users", http.Header{"Authorization": {"Bearer " + raw}})
	if response.Status != http.StatusOK {
		t.Fatalf("authenticated status = %d body=%s", response.Status, response.Body)
	}
}

type integrationCache struct {
	mu     sync.Mutex
	values map[string]cache.Entry
}

func (s *integrationCache) Get(_ context.Context, key string) (cache.Entry, bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	value, ok := s.values[key]
	return value, ok, nil
}
func (s *integrationCache) Set(_ context.Context, key string, value cache.Entry, _ time.Duration) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.values == nil {
		s.values = map[string]cache.Entry{}
	}
	s.values[key] = value
	return nil
}

func TestResponseCacheServesRepeatedRequest(t *testing.T) {
	env := testenv.New(t, testenv.WithUsers(2), testenv.WithCache(
		config.Cache{Enabled: true, OperationTimeout: time.Second, MaxBodyBytes: 1 << 20, OnBackendError: "allow"},
		config.RouteCache{Enabled: true, TTL: time.Minute},
		&integrationCache{},
	))
	first := env.GET("/api/users")
	second := env.GET("/api/users")
	if first.Header.Get("X-Cache") != "MISS" || second.Header.Get("X-Cache") != "HIT" {
		t.Fatalf("cache headers = %q, %q", first.Header.Get("X-Cache"), second.Header.Get("X-Cache"))
	}
	if !bytes.Equal(first.Body, second.Body) {
		t.Fatalf("cached body differs: %q != %q", first.Body, second.Body)
	}
}
