package integration

import (
	"net/http"
	"testing"

	"github.com/Djunichi/APIGateway/internal/config"
	"github.com/Djunichi/APIGateway/test/integration/internal/testenv"
)

func TestDNSDiscoveryBuildsWeightedRouteOverTCP(t *testing.T) {
	env := testenv.New(t, testenv.WithUsers(1), testenv.WithDNSDiscovery(), testenv.WithRouteBalancing("weighted_round_robin", []int{1}, config.Rollout{}))
	env.GET("/api/").RequireStatus(http.StatusOK).RequireService("users-1")
}

func TestDNSDiscoveryFailureKeepsLastSnapshot(t *testing.T) {
	env := testenv.New(t, testenv.WithUsers(1), testenv.WithDNSDiscovery())
	if err := env.ReloadDiscoveryBroken(); err == nil {
		t.Fatal("expected discovery resolution error")
	}
	env.GET("/api/").RequireStatus(http.StatusOK).RequireService("users-1")
}
