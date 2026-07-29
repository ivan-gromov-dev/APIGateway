package middleware

import (
	"net/http"
	"time"
)

// Timeout limits how long a handler may process a request.
func Timeout(duration time.Duration) Middleware {
	return func(next http.Handler) http.Handler {
		if duration <= 0 {
			return next
		}
		return http.TimeoutHandler(next, duration, `{"error":"gateway timeout"}`)
	}
}
