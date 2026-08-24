package integration_test

import (
	"net/http"
	"testing"

	"github.com/ivan-gromov-dev/APIGateway/test/integration/internal/testenv"
)

func TestRoutesToMostSpecificServiceAndStripsPrefix(t *testing.T) {
	env := testenv.New(t, testenv.WithUsers(1), testenv.WithBilling())

	env.GET("/api/users/42").
		RequireStatus(http.StatusOK).
		RequireService("users-1").
		RequireUpstreamPath("/users/42")

	env.GET("/api/billing/invoices/inv-1002").
		RequireStatus(http.StatusOK).
		RequireService("billing").
		RequireUpstreamPath("/invoices/inv-1002")

	env.POST("/api/billing/payments").
		RequireStatus(http.StatusOK).
		RequireService("billing").
		RequireUpstreamPath("/payments")
}

func TestUnknownRouteReturnsNotFound(t *testing.T) {
	env := testenv.New(t, testenv.WithUsers(1))

	env.GET("/not-configured").RequireStatus(http.StatusNotFound)
}
