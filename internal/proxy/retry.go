package proxy

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/Djunichi/APIGateway/internal/balancer"
	"github.com/Djunichi/APIGateway/internal/circuitbreaker"
	"github.com/Djunichi/APIGateway/internal/config"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/trace"
)

type retryTransport struct {
	base            http.RoundTripper
	balancer        balancer.Balancer
	policy          config.Retry
	statuses        map[int]struct{}
	failureStatuses map[int]struct{}
	tracer          trace.Tracer
}

func newRetryTransport(base http.RoundTripper, upstreams balancer.Balancer, policy config.Retry, failureStatuses []int) http.RoundTripper {
	return newRetryTransportWithTracing(base, upstreams, policy, failureStatuses, nil)
}

func newRetryTransportWithTracing(base http.RoundTripper, upstreams balancer.Balancer, policy config.Retry, failureStatuses []int, tracer trace.Tracer) http.RoundTripper {
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
	passiveStatuses := make(map[int]struct{}, len(failureStatuses))
	for _, status := range failureStatuses {
		passiveStatuses[status] = struct{}{}
	}
	return &retryTransport{
		base: base, balancer: upstreams, policy: policy,
		statuses: statuses, failureStatuses: passiveStatuses, tracer: tracer,
	}
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
		attemptRequest, cancel, done, err := t.attemptRequest(request, attempt)
		if err != nil {
			return nil, err
		}
		var span trace.Span
		if t.tracer != nil {
			ctx, started := t.tracer.Start(attemptRequest.Context(), "proxy.attempt", trace.WithSpanKind(trace.SpanKindInternal),
				trace.WithAttributes(attribute.Int("gateway.retry.attempt", attempt+1), attribute.Int("gateway.retry.max_attempts", attempts)))
			attemptRequest = attemptRequest.WithContext(ctx)
			span = started
		}
		response, err := t.base.RoundTrip(attemptRequest)
		completedAt := time.Now()
		if err != nil {
			if span != nil {
				span.RecordError(err)
				span.SetStatus(codes.Error, "transport failure")
				span.End()
			}
			if request.Context().Err() != nil {
				done(circuitbreaker.OutcomeNeutral, completedAt)
			} else {
				done(circuitbreaker.OutcomeFailure, completedAt)
			}
			cancel()
			if attempt+1 == attempts || request.Context().Err() != nil {
				return nil, err
			}
			continue
		}
		if _, failed := t.failureStatuses[response.StatusCode]; failed {
			done(circuitbreaker.OutcomeFailure, completedAt)
			if span != nil {
				span.SetStatus(codes.Error, http.StatusText(response.StatusCode))
			}
		}
		if span != nil {
			span.SetAttributes(attribute.Int("http.response.status_code", response.StatusCode))
		}
		response.Body = &trackedBody{
			ReadCloser: response.Body,
			requestCtx: request.Context(),
			done:       done,
			cancel:     cancel,
			span:       span,
		}
		if _, retry := t.statuses[response.StatusCode]; !retry || attempt+1 == attempts {
			return response, nil
		}
		_, _ = io.CopyN(io.Discard, response.Body, 32<<10)
		_ = response.Body.Close()
	}
	panic("unreachable")
}

func (t *retryTransport) attemptRequest(
	original *http.Request,
	attempt int,
) (*http.Request, context.CancelFunc, circuitbreaker.DoneFunc, error) {
	ctx, cancel := context.WithTimeout(original.Context(), t.policy.PerAttemptTimeout)
	request := original.Clone(ctx)
	if attempt > 0 && original.Body != nil && original.Body != http.NoBody {
		body, err := original.GetBody()
		if err != nil {
			cancel()
			return nil, func() {}, nil, err
		}
		request.Body = body
	}
	selection, available := t.balancer.Next(time.Now())
	if !available {
		cancel()
		return nil, func() {}, nil, errNoAvailableUpstream
	}
	target := selection.Target.URL()
	request.URL = targetURL(&target, original.URL)
	return request, cancel, selection.Done, nil
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

type trackedBody struct {
	io.ReadCloser
	requestCtx context.Context
	done       circuitbreaker.DoneFunc
	cancel     context.CancelFunc
	once       sync.Once
	span       trace.Span
}

func (b *trackedBody) Read(buffer []byte) (int, error) {
	n, err := b.ReadCloser.Read(buffer)
	if err != nil {
		outcome := circuitbreaker.OutcomeFailure
		if errors.Is(err, io.EOF) {
			outcome = circuitbreaker.OutcomeSuccess
		} else if b.requestCtx.Err() != nil {
			outcome = circuitbreaker.OutcomeNeutral
		}
		b.complete(outcome)
	}
	return n, err
}

func (b *trackedBody) Close() error {
	b.complete(circuitbreaker.OutcomeNeutral)
	err := b.ReadCloser.Close()
	b.cancel()
	return err
}

func (b *trackedBody) complete(outcome circuitbreaker.Outcome) {
	b.once.Do(func() {
		b.done(outcome, time.Now())
		if b.span != nil {
			b.span.End()
		}
	})
}
