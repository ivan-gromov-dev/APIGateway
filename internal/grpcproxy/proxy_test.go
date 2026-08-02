package grpcproxy

import (
	"context"
	"crypto/tls"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Djunichi/APIGateway/internal/config"
	"github.com/Djunichi/APIGateway/internal/metrics"
	"golang.org/x/net/http2"
	"golang.org/x/net/http2/h2c"
)

func TestProxyPreservesHTTP2StreamingMetadataAndTrailers(t *testing.T) {
	var calls atomic.Int32
	upstream := newH2CServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		if r.ProtoMajor != 2 || r.Header.Get("Grpc-Metadata-Token") != "opaque" {
			t.Errorf("request protocol=%s metadata=%q", r.Proto, r.Header.Get("Grpc-Metadata-Token"))
		}
		body, _ := io.ReadAll(r.Body)
		w.Header().Set("Content-Type", "application/grpc")
		w.Header().Set("Trailer", "Grpc-Status, Grpc-Message")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(body[:len(body)/2])
		if flusher, ok := w.(http.Flusher); ok {
			flusher.Flush()
		}
		_, _ = w.Write(body[len(body)/2:])
		w.Header().Set("Grpc-Status", "0")
		w.Header().Set("Grpc-Message", "done")
	}))
	collector := &metrics.Collector{}
	handler, err := New(config.GRPC{Enabled: true, Routes: []config.GRPCRoute{{PathPrefix: "/echo.Echo/", Upstream: upstream}}}, collector)
	if err != nil {
		t.Fatal(err)
	}
	gateway := newH2CServer(t, handler)

	request, _ := http.NewRequest(http.MethodPost, gateway+"/echo.Echo/Chat", strings.NewReader("abcdefgh"))
	request.Header.Set("Content-Type", "application/grpc+proto")
	request.Header.Set("Grpc-Metadata-Token", "opaque")
	response, err := h2Client().Do(request)
	if err != nil {
		t.Fatal(err)
	}
	body, err := io.ReadAll(response.Body)
	_ = response.Body.Close()
	if err != nil || string(body) != "abcdefgh" || response.Trailer.Get("Grpc-Status") != "0" || response.Trailer.Get("Grpc-Message") != "done" {
		t.Fatalf("body=%q trailers=%v err=%v", body, response.Trailer, err)
	}
	if calls.Load() != 1 {
		t.Fatalf("upstream calls=%d, want exactly one", calls.Load())
	}

	metricsResponse := httptest.NewRecorder()
	collector.ServeHTTP(metricsResponse, httptest.NewRequest(http.MethodGet, "/metrics", nil))
	if !strings.Contains(metricsResponse.Body.String(), `gateway_grpc_requests_total{grpc_status="0",route="/echo.Echo/"} 1`) {
		t.Fatalf("missing gRPC metric:\n%s", metricsResponse.Body.String())
	}
}

func TestProxyTimeoutCancelsSingleAttempt(t *testing.T) {
	started := make(chan struct{})
	canceled := make(chan struct{})
	upstream := newH2CServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		close(started)
		<-r.Context().Done()
		close(canceled)
	}))
	handler, err := New(config.GRPC{Enabled: true, Routes: []config.GRPCRoute{{PathPrefix: "/slow.Service/", Upstream: upstream, Timeout: 25 * time.Millisecond}}}, nil)
	if err != nil {
		t.Fatal(err)
	}
	gateway := newH2CServer(t, handler)
	request, _ := http.NewRequestWithContext(context.Background(), http.MethodPost, gateway+"/slow.Service/Call", nil)
	request.Header.Set("Content-Type", "application/grpc")
	response, err := h2Client().Do(request)
	if err == nil {
		_, _ = io.Copy(io.Discard, response.Body)
		_ = response.Body.Close()
	}
	<-started
	select {
	case <-canceled:
	case <-time.After(time.Second):
		t.Fatal("upstream context was not canceled")
	}
}

func TestProxyRejectsNonGRPCAndUnknownRoute(t *testing.T) {
	handler, err := New(config.GRPC{Routes: []config.GRPCRoute{{PathPrefix: "/x/", Upstream: "http://127.0.0.1:1"}}}, nil)
	if err != nil {
		t.Fatal(err)
	}
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, httptest.NewRequest(http.MethodPost, "/x/Call", nil))
	if response.Code != http.StatusUnsupportedMediaType {
		t.Fatalf("status=%d", response.Code)
	}
	request := httptest.NewRequest(http.MethodPost, "/missing/Call", nil)
	request.ProtoMajor = 2
	request.Header.Set("Content-Type", "application/grpc")
	response = httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	if response.Code != http.StatusNotFound {
		t.Fatalf("status=%d", response.Code)
	}
}

func newH2CServer(t *testing.T, handler http.Handler) string {
	t.Helper()
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	server := &http.Server{Handler: h2c.NewHandler(handler, &http2.Server{})}
	go func() { _ = server.Serve(listener) }()
	t.Cleanup(func() { _ = server.Shutdown(context.Background()) })
	return "http://" + listener.Addr().String()
}

func h2Client() *http.Client {
	return &http.Client{Transport: &http2.Transport{AllowHTTP: true, DialTLSContext: func(ctx context.Context, network, address string, _ *tls.Config) (net.Conn, error) {
		var dialer net.Dialer
		return dialer.DialContext(ctx, network, address)
	}}}
}
