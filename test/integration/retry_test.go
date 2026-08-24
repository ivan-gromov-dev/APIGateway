package integration_test

import (
	"net/http"
	"testing"
	"time"

	"github.com/ivan-gromov-dev/APIGateway/internal/config"
	"github.com/ivan-gromov-dev/APIGateway/test/integration/internal/testenv"
)

func TestRetryUsesNextUpstreamForSafeRequest(t *testing.T) {
	env := testenv.New(
		t,
		testenv.WithUsers(2),
		testenv.WithUserStatuses(1, http.StatusServiceUnavailable),
		testenv.WithRetry(config.Retry{
			MaxAttempts: 2, PerAttemptTimeout: time.Second,
			Statuses: []int{http.StatusServiceUnavailable},
		}),
	)

	env.GET("/api/users/42").
		RequireStatus(http.StatusOK).
		RequireService("users-2")
}

func TestRetryDoesNotRepeatUnsafeRequest(t *testing.T) {
	env := testenv.New(
		t,
		testenv.WithUsers(2),
		testenv.WithUserStatuses(1, http.StatusServiceUnavailable),
		testenv.WithRetry(config.Retry{
			MaxAttempts: 2, PerAttemptTimeout: time.Second,
			Statuses: []int{http.StatusServiceUnavailable},
		}),
	)

	env.POST("/api/users").RequireStatus(http.StatusServiceUnavailable)
}
