package integration_test

import (
	"net/http"
	"testing"

	"github.com/Djunichi/APIGateway/internal/config"
	"github.com/Djunichi/APIGateway/test/integration/internal/testenv"
)

func TestHeaderRolloutSelectsCanaryOverTCP(t *testing.T) {
	env := testenv.New(t, testenv.WithUsers(2), testenv.WithRouteBalancing("round_robin", nil, config.Rollout{Strategy: "header", Header: "X-Canary"}))
	env.Do(http.MethodGet, "/api/users", http.Header{"X-Canary": []string{"true"}}).RequireStatus(http.StatusOK).RequireService("users-2")
}
