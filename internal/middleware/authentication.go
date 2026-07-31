package middleware

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"

	"github.com/Djunichi/APIGateway/internal/auth"
)

type Authenticator interface {
	Verify(context.Context, string) (auth.Principal, error)
}

type principalKey struct{}

func Principal(r *http.Request) (auth.Principal, bool) {
	principal, ok := r.Context().Value(principalKey{}).(auth.Principal)
	return principal, ok
}

func Authentication(verifier Authenticator, requiredScopes []string) Middleware {
	scopes := append([]string(nil), requiredScopes...)
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			raw, ok := bearerToken(r.Header.Get("Authorization"))
			if !ok {
				writeAuthError(w, http.StatusUnauthorized, "invalid_token", "Bearer")
				return
			}
			principal, err := verifier.Verify(r.Context(), raw)
			if err != nil {
				writeAuthError(w, http.StatusUnauthorized, "invalid_token", `Bearer error="invalid_token"`)
				return
			}
			for _, scope := range scopes {
				if !principal.HasScope(scope) {
					writeAuthError(w, http.StatusForbidden, "insufficient_scope", `Bearer error="insufficient_scope"`)
					return
				}
			}
			r.Header.Del("X-Authenticated-Subject")
			r.Header.Set("X-Authenticated-Subject", principal.Subject)
			next.ServeHTTP(w, r.WithContext(context.WithValue(r.Context(), principalKey{}, principal)))
		})
	}
}

func bearerToken(header string) (string, bool) {
	parts := strings.Fields(header)
	returnValue := ""
	if len(parts) == 2 && strings.EqualFold(parts[0], "Bearer") {
		returnValue = parts[1]
	}
	return returnValue, returnValue != ""
}

func writeAuthError(w http.ResponseWriter, status int, code, challenge string) {
	w.Header().Set("WWW-Authenticate", challenge)
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(map[string]string{"error": code})
}
