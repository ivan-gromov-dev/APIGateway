package integration_test

import (
	"net/http"
	"testing"
	"time"

	"github.com/ivan-gromov-dev/APIGateway/internal/config"
	"github.com/ivan-gromov-dev/APIGateway/test/integration/internal/testenv"
)

func TestActiveHealthCheckSkipsUnhealthyUpstream(t *testing.T) {
	env := testenv.New(t,
		testenv.WithUsers(2),
		testenv.WithActiveHealthCheck(config.ActiveHealthCheck{
			Enabled: true, Path: "/healthz", Interval: 10 * time.Millisecond,
			Timeout: time.Second, HealthyThreshold: 1, UnhealthyThreshold: 1,
		}),
	)
	env.SetUserHealthStatus(1, http.StatusServiceUnavailable)

	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		response := env.GET("/api/")
		if response.Status == http.StatusOK && string(response.Body) != "" {
			// The checker may need one polling cycle; once users-1 is unhealthy,
			// traffic must be served by the healthy peer.
			if responseBodyService(response.Body) == "users-2" {
				return
			}
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatal("active health checker did not remove unhealthy upstream")
}

func TestActiveHealthCheckRecoversUpstream(t *testing.T) {
	env := testenv.New(t, testenv.WithUsers(2), testenv.WithActiveHealthCheck(config.ActiveHealthCheck{
		Enabled: true, Path: "/healthz", Interval: 10 * time.Millisecond, Timeout: time.Second,
		HealthyThreshold: 1, UnhealthyThreshold: 1,
	}))
	env.SetUserHealthStatus(1, http.StatusServiceUnavailable)
	waitForService(t, env, "users-2")
	env.SetUserHealthStatus(1, http.StatusOK)
	waitForService(t, env, "users-1")
}

func waitForService(t *testing.T, env *testenv.Environment, expected string) {
	t.Helper()
	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		if responseBodyService(env.GET("/api/").Body) == expected {
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatalf("upstream %q was not observed", expected)
}

func responseBodyService(body []byte) string {
	const key = `"service":"`
	start := indexOf(body, []byte(key))
	if start < 0 {
		return ""
	}
	start += len(key)
	end := start
	for end < len(body) && body[end] != '"' {
		end++
	}
	return string(body[start:end])
}

func indexOf(body, needle []byte) int {
	for i := 0; i+len(needle) <= len(body); i++ {
		match := true
		for j := range needle {
			if body[i+j] != needle[j] {
				match = false
				break
			}
		}
		if match {
			return i
		}
	}
	return -1
}
