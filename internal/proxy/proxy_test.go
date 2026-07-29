package proxy

import (
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"

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

	handler, err := Handler([]config.Route{{PathPrefix: "/api/", Upstream: upstream.URL, StripPrefix: true}}, slog.New(slog.NewTextHandler(io.Discard, nil)))
	if err != nil {
		t.Fatal(err)
	}
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/api/hello", nil))
	if response.Code != http.StatusOK || response.Body.String() != "hello" {
		t.Fatalf("response = %d %q", response.Code, response.Body.String())
	}
}
