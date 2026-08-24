package integration_test

import (
	"net/http"
	"testing"

	"github.com/ivan-gromov-dev/APIGateway/test/integration/internal/testenv"
)

func TestMetricsDescribeGatewayAndUpstreamTraffic(t *testing.T) {
	env := testenv.New(t, testenv.WithUsers(1))
	env.GET("/api/users").RequireStatus(http.StatusOK)

	env.AdminGET("/metrics").RequireStatus(http.StatusOK).RequireBodyContains(
		`gateway_http_requests_total{method="GET",status_code="200"} 1`,
		`gateway_route_requests_total{route="/api/",status_code="200"} 1`,
		`gateway_proxy_attempts_total{outcome="success",route="/api/"`,
		"gateway_http_request_duration_seconds_bucket",
		"go_goroutines",
		"process_resident_memory_bytes",
	)
}
