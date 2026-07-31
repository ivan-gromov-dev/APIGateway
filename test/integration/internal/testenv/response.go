package testenv

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"
)

type upstreamPayload struct {
	Service     string `json:"service"`
	Path        string `json:"path"`
	RequestID   string `json:"request_id"`
	Traceparent string `json:"traceparent"`
}

// RequireTraceID fails unless the upstream received the expected W3C trace ID.
func (r *Response) RequireTraceID(expected string) *Response {
	r.t.Helper()
	parts := strings.Split(r.payload.Traceparent, "-")
	if len(parts) != 4 || parts[1] != expected {
		r.t.Fatalf("upstream traceparent = %q, want trace ID %q", r.payload.Traceparent, expected)
	}
	return r
}

// Response wraps an HTTP response with scenario-focused assertions.
type Response struct {
	t       testing.TB
	Status  int
	Header  http.Header
	Body    []byte
	payload upstreamPayload
}

func newResponse(t testing.TB, status int, header http.Header, body []byte) *Response {
	response := &Response{t: t, Status: status, Header: header, Body: body}
	_ = json.Unmarshal(body, &response.payload)
	return response
}

// RequireStatus fails the test unless the response has the expected status.
func (r *Response) RequireStatus(expected int) *Response {
	r.t.Helper()
	if r.Status != expected {
		r.t.Fatalf("status = %d, want %d; body=%s", r.Status, expected, r.Body)
	}
	return r
}

// RequireService fails the test unless the response came from the expected upstream.
func (r *Response) RequireService(expected string) *Response {
	r.t.Helper()
	if r.payload.Service != expected {
		r.t.Fatalf("service = %q, want %q; body=%s", r.payload.Service, expected, r.Body)
	}
	return r
}

// RequireUpstreamPath fails unless the upstream observed the expected path.
func (r *Response) RequireUpstreamPath(expected string) *Response {
	r.t.Helper()
	if r.payload.Path != expected {
		r.t.Fatalf("upstream path = %q, want %q", r.payload.Path, expected)
	}
	return r
}

// RequireRequestID fails unless the upstream observed the expected request ID.
func (r *Response) RequireRequestID(expected string) *Response {
	r.t.Helper()
	if r.payload.RequestID != expected {
		r.t.Fatalf("request ID = %q, want %q", r.payload.RequestID, expected)
	}
	return r
}

// RequireResponseHeader fails unless a response header equals the expected value.
func (r *Response) RequireResponseHeader(name, expected string) *Response {
	r.t.Helper()
	if actual := r.Header.Get(name); actual != expected {
		r.t.Fatalf("response header %s = %q, want %q", name, actual, expected)
	}
	return r
}
