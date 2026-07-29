// Package middleware provides composable HTTP handlers for gateway cross-cutting concerns.
package middleware

import "net/http"

// Middleware decorates an HTTP handler.
type Middleware func(http.Handler) http.Handler

// Chain applies middleware in declaration order.
func Chain(handler http.Handler, middleware ...Middleware) http.Handler {
	for i := len(middleware) - 1; i >= 0; i-- {
		handler = middleware[i](handler)
	}
	return handler
}
