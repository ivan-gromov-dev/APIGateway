package proxy

import (
	"bytes"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
	"time"

	"github.com/Djunichi/APIGateway/internal/balancer"
	"github.com/Djunichi/APIGateway/internal/circuitbreaker"
	"github.com/Djunichi/APIGateway/internal/config"
	upstreammodel "github.com/Djunichi/APIGateway/internal/upstream"
)

func TestHandlerProxiesAndStripsPrefix(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/hello" {
			t.Errorf("path = %q, want /hello", r.URL.Path)
		}
		_, _ = w.Write([]byte("hello"))
	}))
	defer upstream.Close()

	handler, err := Handler([]config.Route{{
		PathPrefix:  "/api/",
		Upstreams:   []string{upstream.URL},
		StripPrefix: true,
	}}, testRetry(), testCircuit(), slog.New(slog.NewTextHandler(io.Discard, nil)))
	if err != nil {
		t.Fatal(err)
	}
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/api/hello", nil))
	if response.Code != http.StatusOK || response.Body.String() != "hello" {
		t.Fatalf("response = %d %q", response.Code, response.Body.String())
	}
}

func TestHandlerBalancesRequestsAcrossUpstreams(t *testing.T) {
	newUpstream := func(name string) *httptest.Server {
		return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			_, _ = w.Write([]byte(name))
		}))
	}
	first := newUpstream("first")
	defer first.Close()
	second := newUpstream("second")
	defer second.Close()
	third := newUpstream("third")
	defer third.Close()

	handler, err := Handler([]config.Route{{
		PathPrefix: "/api/",
		Upstreams:  []string{first.URL, second.URL, third.URL},
	}}, testRetry(), testCircuit(), slog.New(slog.NewTextHandler(io.Discard, nil)))
	if err != nil {
		t.Fatal(err)
	}

	want := []string{"first", "second", "third", "first", "second", "third"}
	for i, expected := range want {
		response := httptest.NewRecorder()
		handler.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/api/", nil))
		if response.Code != http.StatusOK || response.Body.String() != expected {
			t.Fatalf("response %d = %d %q, want 200 %q", i, response.Code, response.Body.String(), expected)
		}
	}
}

func TestHandlerRetriesSafeReplayableRequestOnConfiguredStatus(t *testing.T) {
	firstCalls, secondCalls := 0, 0
	first := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		firstCalls++
		http.Error(w, "unavailable", http.StatusServiceUnavailable)
	}))
	defer first.Close()
	second := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		secondCalls++
		_, _ = w.Write([]byte("recovered"))
	}))
	defer second.Close()

	retry := testRetry()
	retry.MaxAttempts = 2
	handler, err := Handler([]config.Route{{
		PathPrefix: "/api/", Upstreams: []string{first.URL, second.URL},
	}}, retry, testCircuit(), slog.New(slog.NewTextHandler(io.Discard, nil)))
	if err != nil {
		t.Fatal(err)
	}
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/api/resource", nil))

	if response.Code != http.StatusOK || response.Body.String() != "recovered" {
		t.Fatalf("response = %d %q", response.Code, response.Body.String())
	}
	if firstCalls != 1 || secondCalls != 1 {
		t.Fatalf("calls = first:%d second:%d, want 1 each", firstCalls, secondCalls)
	}
}

func TestHandlerDoesNotRetryUnsafeOrNonReplayableRequest(t *testing.T) {
	tests := []struct {
		name   string
		method string
		body   io.Reader
	}{
		{name: "unsafe method", method: http.MethodPost},
		{name: "non-replayable body", method: http.MethodGet, body: bytes.NewBufferString("payload")},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			firstCalls, secondCalls := 0, 0
			first := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				firstCalls++
				http.Error(w, "unavailable", http.StatusServiceUnavailable)
			}))
			defer first.Close()
			second := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				secondCalls++
				w.WriteHeader(http.StatusOK)
			}))
			defer second.Close()

			retry := testRetry()
			retry.MaxAttempts = 2
			handler, err := Handler([]config.Route{{
				PathPrefix: "/api/", Upstreams: []string{first.URL, second.URL},
			}}, retry, testCircuit(), slog.New(slog.NewTextHandler(io.Discard, nil)))
			if err != nil {
				t.Fatal(err)
			}
			request := httptest.NewRequest(test.method, "/api/resource", test.body)
			request.GetBody = nil
			response := httptest.NewRecorder()
			handler.ServeHTTP(response, request)

			if response.Code != http.StatusServiceUnavailable {
				t.Fatalf("status = %d, want 503", response.Code)
			}
			if firstCalls != 1 || secondCalls != 0 {
				t.Fatalf("calls = first:%d second:%d, want 1 and 0", firstCalls, secondCalls)
			}
		})
	}
}

func TestHandlerPassivelyOpensAndSkipsFailingUpstream(t *testing.T) {
	firstCalls, secondCalls := 0, 0
	first := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		firstCalls++
		http.Error(w, "upstream unavailable", http.StatusServiceUnavailable)
	}))
	defer first.Close()
	second := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		secondCalls++
		_, _ = w.Write([]byte("healthy"))
	}))
	defer second.Close()

	circuit := testCircuit()
	circuit.FailureThreshold = 1
	handler, err := Handler([]config.Route{{
		PathPrefix: "/api/", Upstreams: []string{first.URL, second.URL},
	}}, testRetry(), circuit, slog.New(slog.NewTextHandler(io.Discard, nil)))
	if err != nil {
		t.Fatal(err)
	}

	for i, wantStatus := range []int{http.StatusServiceUnavailable, http.StatusOK, http.StatusOK} {
		response := httptest.NewRecorder()
		handler.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/api/resource", nil))
		if response.Code != wantStatus {
			t.Fatalf("response %d status = %d, want %d", i, response.Code, wantStatus)
		}
	}
	if firstCalls != 1 || secondCalls != 2 {
		t.Fatalf("calls = first:%d second:%d, want 1 and 2", firstCalls, secondCalls)
	}
}

func TestHandlerReturnsServiceUnavailableWhenAllUpstreamsAreOpen(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "upstream response", http.StatusServiceUnavailable)
	}))
	defer upstream.Close()

	circuit := testCircuit()
	circuit.FailureThreshold = 1
	handler, err := Handler([]config.Route{{
		PathPrefix: "/api/", Upstreams: []string{upstream.URL},
	}}, testRetry(), circuit, slog.New(slog.NewTextHandler(io.Discard, nil)))
	if err != nil {
		t.Fatal(err)
	}
	first := httptest.NewRecorder()
	handler.ServeHTTP(first, httptest.NewRequest(http.MethodGet, "/api/resource", nil))
	second := httptest.NewRecorder()
	handler.ServeHTTP(second, httptest.NewRequest(http.MethodGet, "/api/resource", nil))

	if first.Body.String() != "upstream response\n" {
		t.Fatalf("first body = %q", first.Body.String())
	}
	if second.Code != http.StatusServiceUnavailable || second.Body.String() != "Service Unavailable\n" {
		t.Fatalf("second response = %d %q", second.Code, second.Body.String())
	}
}

func TestHandlerRejectsRouteWithoutUpstreams(t *testing.T) {
	_, err := Handler(
		[]config.Route{{PathPrefix: "/api/"}},
		testRetry(),
		testCircuit(),
		slog.New(slog.NewTextHandler(io.Discard, nil)),
	)
	if err == nil {
		t.Fatal("expected an error")
	}
}

func TestHandlerWithBalancerUsesInjectedImplementation(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte("injected"))
	}))
	defer upstream.Close()
	target, err := url.Parse(upstream.URL)
	if err != nil {
		t.Fatal(err)
	}
	factoryCalls := 0
	factory := func(targets []*upstreammodel.Target) (balancer.Balancer, error) {
		factoryCalls++
		if len(targets) != 2 {
			t.Fatalf("factory received %d upstreams, want 2", len(targets))
		}
		return newFixedBalancer(t, target), nil
	}

	handler, err := HandlerWithBalancer(
		[]config.Route{{
			PathPrefix: "/api/",
			Upstreams:  []string{"http://unused-1.example", "http://unused-2.example"},
		}},
		testRetry(),
		testCircuit(),
		slog.New(slog.NewTextHandler(io.Discard, nil)),
		factory,
	)
	if err != nil {
		t.Fatal(err)
	}
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/api/", nil))

	if factoryCalls != 1 {
		t.Fatalf("factory calls = %d, want 1", factoryCalls)
	}
	if response.Code != http.StatusOK || response.Body.String() != "injected" {
		t.Fatalf("response = %d %q", response.Code, response.Body.String())
	}
}

func TestHandlerWithBalancerRejectsInvalidFactory(t *testing.T) {
	route := []config.Route{{PathPrefix: "/api/", Upstreams: []string{"http://upstream.example"}}}
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))

	if _, err := HandlerWithBalancer(route, testRetry(), testCircuit(), logger, nil); err == nil {
		t.Fatal("expected an error for a nil factory")
	}
	failingFactory := func([]*upstreammodel.Target) (balancer.Balancer, error) {
		return nil, errors.New("factory failed")
	}
	if _, err := HandlerWithBalancer(route, testRetry(), testCircuit(), logger, failingFactory); err == nil {
		t.Fatal("expected the factory error")
	}
	nilFactory := func([]*upstreammodel.Target) (balancer.Balancer, error) {
		return nil, nil
	}
	if _, err := HandlerWithBalancer(route, testRetry(), testCircuit(), logger, nilFactory); err == nil {
		t.Fatal("expected an error for a nil balancer")
	}
}

func testCircuit() config.CircuitBreaker {
	return config.CircuitBreaker{
		FailureThreshold: 5, OpenTimeout: time.Minute,
		FailureStatuses: []int{502, 503, 504},
	}
}

func testRetry() config.Retry {
	return config.Retry{
		MaxAttempts: 1, PerAttemptTimeout: time.Second,
		Statuses: []int{http.StatusBadGateway, http.StatusServiceUnavailable, http.StatusGatewayTimeout},
	}
}

type fixedBalancer struct {
	target *upstreammodel.Target
}

func newFixedBalancer(t *testing.T, targetURL *url.URL) *fixedBalancer {
	t.Helper()
	breaker, err := circuitbreaker.New(circuitbreaker.Config{
		FailureThreshold: 100, OpenTimeout: time.Minute,
	})
	if err != nil {
		t.Fatal(err)
	}
	target, err := upstreammodel.NewTarget(targetURL, breaker)
	if err != nil {
		t.Fatal(err)
	}
	return &fixedBalancer{target: target}
}

func (b *fixedBalancer) Next(now time.Time) (balancer.Selection, bool) {
	done, allowed := b.target.Acquire(now)
	return balancer.Selection{Target: b.target, Done: done}, allowed
}
