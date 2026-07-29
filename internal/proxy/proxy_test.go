package proxy

import (
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"

	"github.com/Djunichi/APIGateway/internal/balancer"
	"github.com/Djunichi/APIGateway/internal/config"
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
	}}, slog.New(slog.NewTextHandler(io.Discard, nil)))
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
	}}, slog.New(slog.NewTextHandler(io.Discard, nil)))
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

func TestHandlerRejectsRouteWithoutUpstreams(t *testing.T) {
	_, err := Handler(
		[]config.Route{{PathPrefix: "/api/"}},
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
	factory := func(upstreams []*url.URL) (balancer.Balancer, error) {
		factoryCalls++
		if len(upstreams) != 2 {
			t.Fatalf("factory received %d upstreams, want 2", len(upstreams))
		}
		return &fixedBalancer{target: target}, nil
	}

	handler, err := HandlerWithBalancer(
		[]config.Route{{
			PathPrefix: "/api/",
			Upstreams:  []string{"http://unused-1.example", "http://unused-2.example"},
		}},
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

	if _, err := HandlerWithBalancer(route, logger, nil); err == nil {
		t.Fatal("expected an error for a nil factory")
	}
	failingFactory := func([]*url.URL) (balancer.Balancer, error) {
		return nil, errors.New("factory failed")
	}
	if _, err := HandlerWithBalancer(route, logger, failingFactory); err == nil {
		t.Fatal("expected the factory error")
	}
	nilFactory := func([]*url.URL) (balancer.Balancer, error) {
		return nil, nil
	}
	if _, err := HandlerWithBalancer(route, logger, nilFactory); err == nil {
		t.Fatal("expected an error for a nil balancer")
	}
}

type fixedBalancer struct {
	target *url.URL
}

func (b *fixedBalancer) Next() *url.URL {
	return b.target
}
