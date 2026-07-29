package integration_test

import (
	"net/http"
	"testing"

	"github.com/Djunichi/APIGateway/test/integration/internal/testenv"
)

func TestRequestIDIsGeneratedAndPropagated(t *testing.T) {
	env := testenv.New(t, testenv.WithUsers(1))

	response := env.GET("/api/users").RequireStatus(http.StatusOK)
	requestID := response.Header.Get("X-Request-ID")
	if requestID == "" {
		t.Fatal("gateway did not generate X-Request-ID")
	}
	response.
		RequireResponseHeader("X-Request-ID", requestID).
		RequireRequestID(requestID)
}

func TestExistingRequestIDIsPreserved(t *testing.T) {
	env := testenv.New(t, testenv.WithUsers(1))
	headers := make(http.Header)
	headers.Set("X-Request-ID", "integration-request-id")

	env.Do(http.MethodGet, "/api/users", headers).
		RequireStatus(http.StatusOK).
		RequireResponseHeader("X-Request-ID", "integration-request-id").
		RequireRequestID("integration-request-id")
}
