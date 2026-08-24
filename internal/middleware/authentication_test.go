package middleware

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/ivan-gromov-dev/APIGateway/internal/auth"
)

type fakeAuthenticator struct {
	principal auth.Principal
	err       error
}

func (f fakeAuthenticator) Verify(context.Context, string) (auth.Principal, error) {
	return f.principal, f.err
}

func TestAuthentication(t *testing.T) {
	verifier := fakeAuthenticator{principal: auth.Principal{Subject: "user-1", Scopes: map[string]struct{}{"read": {}}}}
	handler := Authentication(verifier, []string{"read"})(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if principal, ok := Principal(r); !ok || principal.Subject != "user-1" {
			t.Fatal("principal missing")
		}
		if r.Header.Get("X-Authenticated-Subject") != "user-1" {
			t.Fatal("trusted header missing")
		}
		w.WriteHeader(http.StatusNoContent)
	}))
	request := httptest.NewRequest(http.MethodGet, "/", nil)
	request.Header.Set("Authorization", "Bearer token")
	request.Header.Set("X-Authenticated-Subject", "spoofed")
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	if response.Code != http.StatusNoContent {
		t.Fatalf("status = %d", response.Code)
	}
}

func TestAuthenticationRejectsMissingInvalidAndInsufficient(t *testing.T) {
	tests := []struct {
		name, header string
		verifier     fakeAuthenticator
		status       int
	}{
		{"missing", "", fakeAuthenticator{}, 401},
		{"invalid", "Bearer bad", fakeAuthenticator{err: errors.New("bad")}, 401},
		{"scope", "Bearer token", fakeAuthenticator{principal: auth.Principal{Subject: "x", Scopes: map[string]struct{}{}}}, 403},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			handler := Authentication(test.verifier, []string{"read"})(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { t.Fatal("called") }))
			request := httptest.NewRequest(http.MethodGet, "/", nil)
			request.Header.Set("Authorization", test.header)
			response := httptest.NewRecorder()
			handler.ServeHTTP(response, request)
			if response.Code != test.status || response.Header().Get("WWW-Authenticate") == "" {
				t.Fatalf("response = %d %v", response.Code, response.Header())
			}
		})
	}
}
