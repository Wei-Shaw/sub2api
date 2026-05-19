package kiro

import (
	"bytes"
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
)

// roundTripperFunc lets tests intercept every HTTP request. Multiple
// fake endpoints share one transport so we can simulate the fallback
// sequence without spinning up 3 servers.
type roundTripperFunc func(*http.Request) (*http.Response, error)

func (f roundTripperFunc) RoundTrip(r *http.Request) (*http.Response, error) {
	return f(r)
}

func buildBufferedEventStream(events []Event) []byte {
	var buf bytes.Buffer
	_ = EncodeEventStream(&buf, events)
	return buf.Bytes()
}

func TestCall_FirstEndpointSucceeds(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer tok" {
			t.Fatalf("bearer = %q", r.Header.Get("Authorization"))
		}
		if r.Header.Get("Content-Type") != "application/json" {
			t.Fatalf("content-type = %q", r.Header.Get("Content-Type"))
		}
		_, _ = w.Write(buildBufferedEventStream([]Event{
			{Type: "assistantResponseEvent", Payload: map[string]any{"content": "ok"}},
		}))
	}))
	defer srv.Close()

	// Redirect the first endpoint to our test server via a custom transport.
	first := endpoints[0]
	overrideEndpoint(t, 0, Endpoint{URL: srv.URL, Origin: first.Origin, AmzTarget: first.AmzTarget, Name: first.Name})

	client := &http.Client{}
	result, err := Call(context.Background(), CallOptions{
		AccessToken:      "tok",
		Payload:          &Payload{},
		HTTPClient:       client,
		EndpointFallback: false,
	})
	if err != nil {
		t.Fatal(err)
	}
	defer result.Response.Body.Close()
	if result.Endpoint.Name != "Kiro IDE" {
		t.Fatalf("endpoint = %q", result.Endpoint.Name)
	}
}

func TestCall_FallsThroughOn429(t *testing.T) {
	attemptCount := int32(0)
	transport := roundTripperFunc(func(r *http.Request) (*http.Response, error) {
		count := atomic.AddInt32(&attemptCount, 1)
		host := r.URL.Host
		// First endpoint (Kiro IDE) → 429; second (CodeWhisperer) → success.
		if count == 1 && strings.Contains(host, "q.us-east-1.amazonaws.com") {
			return &http.Response{
				StatusCode: 429,
				Body:       io_NopCloser(bytes.NewReader([]byte("rate limited"))),
				Header:     http.Header{},
			}, nil
		}
		if count == 2 && strings.Contains(host, "codewhisperer.us-east-1.amazonaws.com") {
			return &http.Response{
				StatusCode: 200,
				Body:       io_NopCloser(bytes.NewReader(buildBufferedEventStream([]Event{
					{Type: "assistantResponseEvent", Payload: map[string]any{"content": "ok"}},
				}))),
				Header: http.Header{},
			}, nil
		}
		t.Fatalf("unexpected attempt %d to %s", count, host)
		return nil, nil
	})

	client := &http.Client{Transport: transport}
	result, err := Call(context.Background(), CallOptions{
		AccessToken:      "tok",
		Payload:          &Payload{},
		HTTPClient:       client,
		EndpointFallback: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	defer result.Response.Body.Close()
	if result.Endpoint.Name != "CodeWhisperer" {
		t.Fatalf("expected fallback to CodeWhisperer, got %q", result.Endpoint.Name)
	}
	if atomic.LoadInt32(&attemptCount) != 2 {
		t.Fatalf("attempt count = %d", attemptCount)
	}
}

func TestCall_401StopsImmediately(t *testing.T) {
	attemptCount := int32(0)
	transport := roundTripperFunc(func(*http.Request) (*http.Response, error) {
		atomic.AddInt32(&attemptCount, 1)
		return &http.Response{
			StatusCode: 401,
			Body:       io_NopCloser(bytes.NewReader([]byte("unauth"))),
			Header:     http.Header{},
		}, nil
	})

	client := &http.Client{Transport: transport}
	_, err := Call(context.Background(), CallOptions{
		AccessToken:      "tok",
		Payload:          &Payload{},
		HTTPClient:       client,
		EndpointFallback: true,
	})
	if err == nil {
		t.Fatal("expected 401 error")
	}
	var herr *HTTPError
	if !errors.As(err, &herr) || herr.StatusCode != 401 {
		t.Fatalf("error = %v", err)
	}
	if atomic.LoadInt32(&attemptCount) != 1 {
		t.Fatalf("401 should NOT trigger fallback; attempts = %d", attemptCount)
	}
}

func TestCall_AllEndpointsExhausted(t *testing.T) {
	transport := roundTripperFunc(func(*http.Request) (*http.Response, error) {
		return &http.Response{
			StatusCode: 429,
			Body:       io_NopCloser(bytes.NewReader([]byte("limited"))),
			Header:     http.Header{},
		}, nil
	})
	client := &http.Client{Transport: transport}
	_, err := Call(context.Background(), CallOptions{
		AccessToken:      "tok",
		Payload:          &Payload{},
		HTTPClient:       client,
		EndpointFallback: true,
	})
	if err == nil {
		t.Fatal("expected exhaustion error")
	}
	if !strings.Contains(err.Error(), "quota") {
		t.Fatalf("expected quota error mention, got %v", err)
	}
}

func TestCall_OnAttemptInvokedPerEndpoint(t *testing.T) {
	transport := roundTripperFunc(func(*http.Request) (*http.Response, error) {
		return &http.Response{
			StatusCode: 200,
			Body:       io_NopCloser(bytes.NewReader([]byte{})),
			Header:     http.Header{},
		}, nil
	})
	client := &http.Client{Transport: transport}
	var attempts []string
	_, err := Call(context.Background(), CallOptions{
		AccessToken:      "tok",
		Payload:          &Payload{},
		HTTPClient:       client,
		EndpointFallback: true,
		OnAttempt:        func(name string) { attempts = append(attempts, name) },
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(attempts) != 1 || attempts[0] != "Kiro IDE" {
		t.Fatalf("attempts = %v", attempts)
	}
}

func TestCall_MissingTokenErrors(t *testing.T) {
	_, err := Call(context.Background(), CallOptions{
		Payload:    &Payload{},
		HTTPClient: &http.Client{},
	})
	if err == nil || !strings.Contains(err.Error(), "AccessToken required") {
		t.Fatalf("expected access-token error, got %v", err)
	}
}

// overrideEndpoint replaces endpoints[i] for the duration of the test
// and restores it afterwards.
func overrideEndpoint(t *testing.T, i int, ep Endpoint) {
	t.Helper()
	prev := endpoints[i]
	endpoints[i] = ep
	t.Cleanup(func() { endpoints[i] = prev })
}

// io_NopCloser is just io.NopCloser, kept here so the test file doesn't
// import io directly (less noise).
func io_NopCloser(r *bytes.Reader) interface {
	Read([]byte) (int, error)
	Close() error
} {
	return ioNopCloserShim{r}
}

type ioNopCloserShim struct{ r *bytes.Reader }

func (s ioNopCloserShim) Read(p []byte) (int, error) { return s.r.Read(p) }
func (s ioNopCloserShim) Close() error               { return nil }
