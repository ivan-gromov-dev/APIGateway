package proxy

import (
	"context"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/Djunichi/APIGateway/internal/balancer"
	"github.com/Djunichi/APIGateway/internal/config"
)

type retryTransport struct {
	base     http.RoundTripper
	balancer balancer.Balancer
	policy   config.Retry
	statuses map[int]struct{}
}

func newRetryTransport(base http.RoundTripper, upstreams balancer.Balancer, policy config.Retry) http.RoundTripper {
	if policy.MaxAttempts < 1 {
		policy.MaxAttempts = 1
	}
	if policy.PerAttemptTimeout <= 0 {
		policy.PerAttemptTimeout = 30 * time.Second
	}
	statuses := make(map[int]struct{}, len(policy.Statuses))
	for _, status := range policy.Statuses {
		statuses[status] = struct{}{}
	}
	return &retryTransport{base: base, balancer: upstreams, policy: policy, statuses: statuses}
}

func (t *retryTransport) RoundTrip(request *http.Request) (*http.Response, error) {
	attempts := 1
	if retryableRequest(request) {
		attempts = t.policy.MaxAttempts
	}
	for attempt := 0; attempt < attempts; attempt++ {
		if attempt > 0 {
			if err := waitForRetry(request.Context(), t.policy.Backoff); err != nil {
				return nil, err
			}
		}
		attemptRequest, cancel, err := t.attemptRequest(request, attempt)
		if err != nil {
			return nil, err
		}
		response, err := t.base.RoundTrip(attemptRequest)
		if err != nil {
			cancel()
			if attempt+1 == attempts || request.Context().Err() != nil {
				return nil, err
			}
			continue
		}
		if _, retry := t.statuses[response.StatusCode]; !retry || attempt+1 == attempts {
			response.Body = &cancelOnClose{ReadCloser: response.Body, cancel: cancel}
			return response, nil
		}
		_, _ = io.CopyN(io.Discard, response.Body, 32<<10)
		_ = response.Body.Close()
		cancel()
	}
	panic("unreachable")
}

func (t *retryTransport) attemptRequest(original *http.Request, attempt int) (*http.Request, context.CancelFunc, error) {
	ctx, cancel := context.WithTimeout(original.Context(), t.policy.PerAttemptTimeout)
	request := original.Clone(ctx)
	request.URL = targetURL(t.balancer.Next(), original.URL)
	if attempt > 0 && original.Body != nil && original.Body != http.NoBody {
		body, err := original.GetBody()
		if err != nil {
			cancel()
			return nil, func() {}, err
		}
		request.Body = body
	}
	return request, cancel, nil
}

func retryableRequest(request *http.Request) bool {
	switch request.Method {
	case http.MethodGet, http.MethodHead, http.MethodOptions:
	default:
		return false
	}
	return request.Body == nil || request.Body == http.NoBody || request.GetBody != nil
}

func waitForRetry(ctx context.Context, delay time.Duration) error {
	if delay == 0 {
		return nil
	}
	timer := time.NewTimer(delay)
	defer timer.Stop()
	select {
	case <-timer.C:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

func targetURL(target, incoming *url.URL) *url.URL {
	result := *incoming
	result.Scheme = target.Scheme
	result.Host = target.Host
	result.Path, result.RawPath = joinURLPath(target, incoming)
	if target.RawQuery == "" || incoming.RawQuery == "" {
		result.RawQuery = target.RawQuery + incoming.RawQuery
	} else {
		result.RawQuery = target.RawQuery + "&" + incoming.RawQuery
	}
	return &result
}

func joinURLPath(target, incoming *url.URL) (string, string) {
	if target.RawPath == "" && incoming.RawPath == "" {
		return singleJoiningSlash(target.Path, incoming.Path), ""
	}
	return singleJoiningSlash(target.Path, incoming.Path),
		singleJoiningSlash(target.EscapedPath(), incoming.EscapedPath())
}

func singleJoiningSlash(left, right string) string {
	switch {
	case strings.HasSuffix(left, "/") && strings.HasPrefix(right, "/"):
		return left + right[1:]
	case !strings.HasSuffix(left, "/") && !strings.HasPrefix(right, "/"):
		return left + "/" + right
	default:
		return left + right
	}
}

type cancelOnClose struct {
	io.ReadCloser
	cancel context.CancelFunc
}

func (c *cancelOnClose) Close() error {
	err := c.ReadCloser.Close()
	c.cancel()
	return err
}
