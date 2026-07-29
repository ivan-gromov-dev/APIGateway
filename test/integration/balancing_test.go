// Package integration_test verifies the gateway through real network listeners.
package integration_test

import (
	"net/http"
	"testing"

	"github.com/Djunichi/APIGateway/test/integration/internal/testenv"
)

func TestRoundRobinAcrossUsersInstances(t *testing.T) {
	env := testenv.New(t, testenv.WithUsers(3))

	for _, expected := range []string{"users-1", "users-2", "users-3", "users-1"} {
		env.GET("/api/users").
			RequireStatus(http.StatusOK).
			RequireService(expected)
	}
}
