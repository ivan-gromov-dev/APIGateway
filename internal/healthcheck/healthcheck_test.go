package healthcheck

import (
	"context"
	"fmt"
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

func TestCheckerMarksUnhealthyAndResetsStreak(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusServiceUnavailable) }))
	defer server.Close()
	target := testTarget(t, server.URL)
	c := New([]*upstream.Target{target}, config.ActiveHealthCheck{Path: "/healthz", HealthyThreshold: 2, UnhealthyThreshold: 2}, server.Client())
	c.record(target, false)
	if c.streak[target][1] != 1 {
		t.Fatal("first unhealthy result was not recorded")
	}
	c.record(target, false)
	if c.streak[target][1] != 2 {
		t.Fatal("target unhealthy streak was not recorded")
	}
}

func TestCheckerHandlesTransportErrorAndRunExit(t *testing.T) {
	target := testTarget(t, "http://127.0.0.1:1")
	c := New([]*upstream.Target{target}, config.ActiveHealthCheck{Enabled: true, Path: "/healthz", Interval: time.Millisecond, Timeout: time.Millisecond, HealthyThreshold: 1, UnhealthyThreshold: 1}, &http.Client{})
	c.client.Transport = errorTransport{}
	ctx, cancel := context.WithCancel(context.Background())
	go func() { time.Sleep(5 * time.Millisecond); cancel() }()
	c.record(target, false)
	if c.streak[target][1] == 0 {
		t.Fatal("transport error was not recorded")
	}
	c.Run(ctx)
	New(nil, config.ActiveHealthCheck{}, nil).Run(context.Background())
}

func TestCheckerNilClientAndStatusClassification(t *testing.T) {
	if New(nil, config.ActiveHealthCheck{}, nil).client == nil {
		t.Fatal("client was not initialized")
	}
	if statusErr(http.StatusOK) != nil || statusErr(http.StatusFound) != nil {
		t.Fatal("2xx/3xx should be healthy")
	}
	if statusErr(http.StatusBadRequest) == nil || statusErr(http.StatusInternalServerError) == nil {
		t.Fatal("non-2xx/3xx should be unhealthy")
	}
	if statusErr(http.StatusNotFound) == nil {
		t.Fatal("status error should be non-nil")
	}
}

func testTarget(t *testing.T, raw string) *upstream.Target {
	t.Helper()
	u, err := url.Parse(raw)
	if err != nil {
		t.Fatal(err)
	}
	b, err := circuitbreaker.New(circuitbreaker.Config{FailureThreshold: 1, OpenTimeout: time.Minute})
	if err != nil {
		t.Fatal(err)
	}
	target, err := upstream.NewTarget(u, b)
	if err != nil {
		t.Fatal(err)
	}
	return target
}

type errorTransport struct{}

func (errorTransport) RoundTrip(*http.Request) (*http.Response, error) {
	return nil, fmt.Errorf("transport failed")
}

func testContext() context.Context { return context.Background() }
