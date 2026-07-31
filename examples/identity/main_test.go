package main

import (
	"crypto/rand"
	"crypto/rsa"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

func TestTokenAndJWKS(t *testing.T) {
	key, _ := rsa.GenerateKey(rand.Reader, 2048)
	svc := &service{issuer: "issuer", clientID: "client", clientSecret: "secret", key: key, keyID: signingKeyID(&key.PublicKey), now: func() time.Time { return time.Unix(1000, 0) }}
	server := httptest.NewServer(svc.handler())
	defer server.Close()
	form := url.Values{"grant_type": {"client_credentials"}, "scope": {"users.read"}}
	request, _ := http.NewRequest(http.MethodPost, server.URL+"/token", strings.NewReader(form.Encode()))
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	request.SetBasicAuth("client", "secret")
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	var payload struct {
		AccessToken string `json:"access_token"`
	}
	if err := json.NewDecoder(response.Body).Decode(&payload); err != nil {
		t.Fatal(err)
	}
	parsed, err := jwt.Parse(payload.AccessToken, func(token *jwt.Token) (any, error) { return &key.PublicKey, nil }, jwt.WithAudience("api-gateway"), jwt.WithIssuer("issuer"), jwt.WithTimeFunc(func() time.Time { return time.Unix(1001, 0) }))
	if err != nil || !parsed.Valid {
		t.Fatalf("token invalid: %v", err)
	}
	if response, err := http.Get(server.URL + "/.well-known/jwks.json"); err != nil || response.StatusCode != 200 {
		t.Fatalf("JWKS response=%v err=%v", response, err)
	} else {
		_ = response.Body.Close()
	}
}

func TestTokenRejectsInvalidClientAndGrant(t *testing.T) {
	key, _ := rsa.GenerateKey(rand.Reader, 2048)
	svc := &service{clientID: "client", clientSecret: "secret", key: key, keyID: signingKeyID(&key.PublicKey), now: time.Now}
	for _, values := range []url.Values{{"grant_type": {"client_credentials"}}, {"grant_type": {"password"}, "client_id": {"client"}, "client_secret": {"secret"}}, {"grant_type": {"client_credentials"}, "client_id": {"client"}, "client_secret": {"secret"}, "scope": {"admin"}}} {
		request := httptest.NewRequest(http.MethodPost, "/token", strings.NewReader(values.Encode()))
		request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		response := httptest.NewRecorder()
		svc.token(response, request)
		if response.Code == http.StatusOK {
			t.Fatal("expected rejection")
		}
	}
}
