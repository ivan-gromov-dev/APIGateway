package middleware

import (
	"crypto/rand"
	"encoding/hex"
	"net/http"
)

const requestIDHeader = "X-Request-ID"

// RequestID propagates an existing request ID or generates a new one.
func RequestID(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		id := r.Header.Get(requestIDHeader)
		if id == "" {
			var value [16]byte
			if _, err := rand.Read(value[:]); err == nil {
				id = hex.EncodeToString(value[:])
			}
		}
		w.Header().Set(requestIDHeader, id)
		r.Header.Set(requestIDHeader, id)
		next.ServeHTTP(w, r)
	})
}
