package healthcheck

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
	"time"

	"github.com/Djunichi/APIGateway/internal/circuitbreaker"
	"github.com/Djunichi/APIGateway/internal/config"
	"github.com/Djunichi/APIGateway/internal/upstream"
)

func TestCheckerRecordsHealthAfterThresholds(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusOK) }))
	defer server.Close()
	u, _ := url.Parse(server.URL)
	b, _ := circuitbreaker.New(circuitbreaker.Config{FailureThreshold: 1, OpenTimeout: time.Minute})
	target, _ := upstream.NewTarget(u, b)
	c := New([]*upstream.Target{target}, config.ActiveHealthCheck{Enabled: true, Path: "/healthz", HealthyThreshold: 1, UnhealthyThreshold: 1}, server.Client())
	c.checkAll(testContext())
	if target.Snapshot().State != circuitbreaker.StateClosed {
		t.Fatal("healthy probe did not close target")
	}
}

func testContext() context.Context { return context.Background() }
