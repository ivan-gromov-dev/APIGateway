package integration_test

import (
	"net/http"
	"testing"
	"time"

	"github.com/ivan-gromov-dev/APIGateway/internal/config"
	"github.com/ivan-gromov-dev/APIGateway/test/integration/internal/testenv"
)

func TestPassiveTrackingSkipsOpenUpstream(t *testing.T) {
	env := testenv.New(
		t,
		testenv.WithUsers(2),
		testenv.WithUserStatuses(1, http.StatusServiceUnavailable),
		testenv.WithCircuitBreaker(testenvCircuitBreaker()),
	)

	env.GET("/api/users/1").RequireStatus(http.StatusServiceUnavailable)
	env.GET("/api/users/2").RequireStatus(http.StatusOK).RequireService("users-2")
	env.GET("/api/users/3").RequireStatus(http.StatusOK).RequireService("users-2")
}

func TestPassiveTrackingReturnsServiceUnavailableWhenAllUpstreamsOpen(t *testing.T) {
	env := testenv.New(
		t,
		testenv.WithUsers(2),
		testenv.WithUserStatuses(1, http.StatusServiceUnavailable),
		testenv.WithUserStatuses(2, http.StatusServiceUnavailable),
		testenv.WithCircuitBreaker(testenvCircuitBreaker()),
	)

	env.GET("/api/users/1").RequireStatus(http.StatusServiceUnavailable)
	env.GET("/api/users/2").RequireStatus(http.StatusServiceUnavailable)
	env.GET("/api/users/3").RequireStatus(http.StatusServiceUnavailable)
}

func testenvCircuitBreaker() config.CircuitBreaker {
	return config.CircuitBreaker{
		FailureThreshold: 1,
		OpenTimeout:      time.Hour,
		FailureStatuses:  []int{http.StatusServiceUnavailable},
	}
}
