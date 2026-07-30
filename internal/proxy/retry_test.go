package proxy

import (
	"bytes"
	"context"
	"errors"
	"io"
	"net/http"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/Djunichi/APIGateway/internal/circuitbreaker"
	"github.com/Djunichi/APIGateway/internal/config"
)

func TestRetryTransportRetriesNetworkFailureAndReplaysBody(t *testing.T) {
	target, err := url.Parse("http://upstream.example/base?source=gateway")
	if err != nil {
		t.Fatal(err)
	}
	calls := 0
	transport := newRetryTransport(
		roundTripFunc(func(request *http.Request) (*http.Response, error) {
			calls++
			if calls == 1 {
				return nil, errors.New("connection reset")
			}
			body, readErr := io.ReadAll(request.Body)
			if readErr != nil {
				t.Fatal(readErr)
			}
			if string(body) != "payload" {
				t.Fatalf("replayed body = %q", body)
			}
			if request.URL.String() != "http://upstream.example/base/resource?source=gateway&id=42" {
				t.Fatalf("target URL = %q", request.URL)
			}
			return &http.Response{
				StatusCode: http.StatusOK,
				Body:       io.NopCloser(strings.NewReader("ok")),
				Header:     make(http.Header),
				Request:    request,
			}, nil
		}),
		newFixedBalancer(t, target),
		config.Retry{
			MaxAttempts:       2,
			PerAttemptTimeout: time.Second,
			Statuses:          []int{http.StatusServiceUnavailable},
		},
		[]int{http.StatusServiceUnavailable},
	)
	request, err := http.NewRequest(http.MethodGet, "http://gateway/resource?id=42", bytes.NewBufferString("payload"))
	if err != nil {
		t.Fatal(err)
	}

	response, err := transport.RoundTrip(request)
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	if calls != 2 {
		t.Fatalf("calls = %d, want 2", calls)
	}
}

func TestRetryTransportReturnsBodyReplayError(t *testing.T) {
	target, _ := url.Parse("http://upstream.example")
	replayErr := errors.New("cannot replay")
	calls := 0
	transport := newRetryTransport(
		roundTripFunc(func(*http.Request) (*http.Response, error) {
			calls++
			return nil, errors.New("network failure")
		}),
		newFixedBalancer(t, target),
		config.Retry{MaxAttempts: 2, PerAttemptTimeout: time.Second},
		[]int{http.StatusServiceUnavailable},
	)
	request, err := http.NewRequest(http.MethodGet, "http://gateway/resource", strings.NewReader("payload"))
	if err != nil {
		t.Fatal(err)
	}
	request.GetBody = func() (io.ReadCloser, error) { return nil, replayErr }

	_, err = transport.RoundTrip(request)
	if !errors.Is(err, replayErr) {
		t.Fatalf("error = %v, want replay error", err)
	}
	if calls != 1 {
		t.Fatalf("calls = %d, want 1", calls)
	}
}

func TestWaitForRetryHonorsDelayAndCancellation(t *testing.T) {
	if err := waitForRetry(context.Background(), 0); err != nil {
		t.Fatal(err)
	}
	if err := waitForRetry(context.Background(), time.Millisecond); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := waitForRetry(ctx, time.Hour); !errors.Is(err, context.Canceled) {
		t.Fatalf("error = %v, want context canceled", err)
	}
}

func TestNewRetryTransportAppliesDefensiveDefaults(t *testing.T) {
	target, _ := url.Parse("http://upstream.example")
	transport := newRetryTransport(
		roundTripFunc(func(request *http.Request) (*http.Response, error) {
			return &http.Response{
				StatusCode: http.StatusOK,
				Body:       io.NopCloser(strings.NewReader("ok")),
				Header:     make(http.Header),
				Request:    request,
			}, nil
		}),
		newFixedBalancer(t, target),
		config.Retry{},
		nil,
	).(*retryTransport)

	if transport.policy.MaxAttempts != 1 || transport.policy.PerAttemptTimeout != 30*time.Second {
		t.Fatalf("policy = %+v", transport.policy)
	}
}

func TestRetryTransportRecordsClientCancellationAsNeutral(t *testing.T) {
	targetURL, _ := url.Parse("http://upstream.example")
	targetBalancer := newFixedBalancer(t, targetURL)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	transport := newRetryTransport(
		roundTripFunc(func(request *http.Request) (*http.Response, error) {
			return nil, request.Context().Err()
		}),
		targetBalancer,
		config.Retry{MaxAttempts: 1, PerAttemptTimeout: time.Second},
		[]int{http.StatusServiceUnavailable},
	)
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, "http://gateway/resource", nil)
	if err != nil {
		t.Fatal(err)
	}

	_, _ = transport.RoundTrip(request)
	snapshot := targetBalancer.target.Snapshot()
	if snapshot.State != circuitbreaker.StateClosed || snapshot.ConsecutiveFailures != 0 {
		t.Fatalf("snapshot = %+v, want unchanged closed circuit", snapshot)
	}
}

func TestRetryTransportRecordsResponseBodyFailure(t *testing.T) {
	targetURL, _ := url.Parse("http://upstream.example")
	targetBalancer := newFixedBalancer(t, targetURL)
	bodyErr := errors.New("upstream stream reset")
	transport := newRetryTransport(
		roundTripFunc(func(request *http.Request) (*http.Response, error) {
			return &http.Response{
				StatusCode: http.StatusOK,
				Body:       &failingBody{err: bodyErr},
				Header:     make(http.Header),
				Request:    request,
			}, nil
		}),
		targetBalancer,
		config.Retry{MaxAttempts: 1, PerAttemptTimeout: time.Second},
		[]int{http.StatusServiceUnavailable},
	)
	request, err := http.NewRequest(http.MethodGet, "http://gateway/resource", nil)
	if err != nil {
		t.Fatal(err)
	}
	response, err := transport.RoundTrip(request)
	if err != nil {
		t.Fatal(err)
	}
	_, err = io.ReadAll(response.Body)
	if !errors.Is(err, bodyErr) {
		t.Fatalf("body error = %v, want %v", err, bodyErr)
	}
	_ = response.Body.Close()

	if got := targetBalancer.target.Snapshot().ConsecutiveFailures; got != 1 {
		t.Fatalf("failures = %d, want 1", got)
	}
}

func TestURLJoiningVariants(t *testing.T) {
	tests := []struct {
		left  string
		right string
		want  string
	}{
		{left: "/base/", right: "/resource", want: "/base/resource"},
		{left: "/base", right: "resource", want: "/base/resource"},
		{left: "/base/", right: "resource", want: "/base/resource"},
	}
	for _, test := range tests {
		if got := singleJoiningSlash(test.left, test.right); got != test.want {
			t.Fatalf("singleJoiningSlash(%q, %q) = %q, want %q", test.left, test.right, got, test.want)
		}
	}

	target := &url.URL{Path: "/base path", RawPath: "/base%20path"}
	incoming := &url.URL{Path: "/a path", RawPath: "/a%20path"}
	path, rawPath := joinURLPath(target, incoming)
	if path != "/base path/a path" || rawPath != "/base%20path/a%20path" {
		t.Fatalf("joined path = %q raw path = %q", path, rawPath)
	}
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(request *http.Request) (*http.Response, error) {
	return f(request)
}

type failingBody struct {
	err error
}

func (b *failingBody) Read([]byte) (int, error) {
	return 0, b.err
}

func (b *failingBody) Close() error {
	return nil
}
