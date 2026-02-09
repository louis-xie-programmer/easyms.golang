package plugins

import (
	"io"
	"net/http"
	"strings"
	"testing"
)

type roundTripperFunc func(*http.Request) (*http.Response, error)

func (f roundTripperFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return f(req)
}

func TestRetryTransportIdempotentRetry(t *testing.T) {
	calls := 0
	rt := &retryTransport{
		base: roundTripperFunc(func(req *http.Request) (*http.Response, error) {
			calls++
			if calls == 1 {
				return &http.Response{StatusCode: http.StatusBadGateway, Body: io.NopCloser(strings.NewReader(""))}, nil
			}
			return &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(strings.NewReader("ok"))}, nil
		}),
		maxRetries: 1,
		backoff:    0,
	}
	req, _ := http.NewRequest(http.MethodGet, "http://example", nil)
	resp, err := rt.RoundTrip(req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}
	if calls != 2 {
		t.Fatalf("expected 2 calls, got %d", calls)
	}
}

func TestRetryTransportNonIdempotentNoRetry(t *testing.T) {
	calls := 0
	rt := &retryTransport{
		base: roundTripperFunc(func(req *http.Request) (*http.Response, error) {
			calls++
			return &http.Response{StatusCode: http.StatusBadGateway, Body: io.NopCloser(strings.NewReader(""))}, nil
		}),
		maxRetries: 3,
		backoff:    0,
	}
	req, _ := http.NewRequest(http.MethodPost, "http://example", nil)
	resp, err := rt.RoundTrip(req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.StatusCode != http.StatusBadGateway {
		t.Fatalf("expected 502, got %d", resp.StatusCode)
	}
	if calls != 1 {
		t.Fatalf("expected 1 call, got %d", calls)
	}
}

func TestShouldRetryStatus(t *testing.T) {
	if !shouldRetryStatus(http.StatusBadGateway) {
		t.Fatalf("expected retry for 502")
	}
	if !shouldRetryStatus(http.StatusServiceUnavailable) {
		t.Fatalf("expected retry for 503")
	}
	if !shouldRetryStatus(http.StatusGatewayTimeout) {
		t.Fatalf("expected retry for 504")
	}
	if shouldRetryStatus(http.StatusBadRequest) {
		t.Fatalf("did not expect retry for 400")
	}
}
