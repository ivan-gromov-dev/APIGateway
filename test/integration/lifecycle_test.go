package integration_test

import (
	"net/http"
	"testing"

	"github.com/ivan-gromov-dev/APIGateway/test/integration/internal/testenv"
)

func TestOperationalEndpointsAndGracefulStop(t *testing.T) {
	env := testenv.New(t, testenv.WithUsers(1))

	env.AdminGET("/healthz").RequireStatus(http.StatusOK)
	env.AdminGET("/readyz").RequireStatus(http.StatusOK)

	if err := env.Stop(); err != nil {
		t.Fatalf("graceful stop: %v", err)
	}
}
