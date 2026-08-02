// Package integration_test verifies the gateway through real network listeners.
package integration_test

import (
	"bytes"
	"net/http"
	"testing"

	"github.com/Djunichi/APIGateway/internal/config"
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

func TestWeightedRoundRobinDistributionOverTCP(t *testing.T) {
	env := testenv.New(t, testenv.WithUsers(2), testenv.WithRouteBalancing("weighted_round_robin", []int{1, 3}, config.Rollout{}))
	counts := map[string]int{}
	for i := 0; i < 40; i++ {
		response := env.GET("/api/users").RequireStatus(http.StatusOK)
		for _, service := range []string{"users-1", "users-2"} {
			if bytes.Contains(response.Body, []byte(`"service":"`+service+`"`)) {
				counts[service]++
				break
			}
		}
	}
	if counts["users-1"] != 10 || counts["users-2"] != 30 {
		t.Fatalf("counts = %#v", counts)
	}
}
