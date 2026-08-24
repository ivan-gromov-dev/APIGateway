package integration

import (
	"net/http"
	"testing"

	"github.com/ivan-gromov-dev/APIGateway/test/integration/internal/testenv"
)

func TestTransactionalReloadChangesRouting(t *testing.T) {
	env := testenv.New(t, testenv.WithUsers(1))
	first := env.GET("/api/")
	first.RequireStatus(http.StatusOK)
	// A valid route replacement is accepted and continues serving over the same listeners.
	if err := env.ReloadRoutes("http://127.0.0.1:1"); err != nil {
		t.Fatal(err)
	}
	env.GET("/api/").RequireStatus(http.StatusBadGateway)
}

func TestTransactionalReloadKeepsPreviousRuntimeOnInvalidConfig(t *testing.T) {
	env := testenv.New(t, testenv.WithUsers(1))
	if err := env.ReloadBroken(); err == nil {
		t.Fatal("expected invalid configuration error")
	}
	env.GET("/api/").RequireStatus(http.StatusOK)
}
