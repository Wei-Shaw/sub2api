package httpclient

import (
	"errors"
	"io"
	"net/http"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return f(req)
}

type closeTrackingRoundTripper struct {
	closed atomic.Int32
}

func (t *closeTrackingRoundTripper) RoundTrip(*http.Request) (*http.Response, error) {
	return nil, errors.New("not implemented")
}

func (t *closeTrackingRoundTripper) CloseIdleConnections() {
	t.closed.Add(1)
}

func TestSharedClientPoolBoundsEntries(t *testing.T) {
	pool := newSharedClientPool()
	for i := 0; i < sharedClientMaxEntries+50; i++ {
		pool.store(strconv.Itoa(i), &http.Client{})
	}

	pool.mu.Lock()
	count := len(pool.entries)
	pool.mu.Unlock()
	require.Equal(t, sharedClientMaxEntries, count)
}

func TestSharedClientPoolEvictsIdleAndClosesConnections(t *testing.T) {
	now := time.Unix(1730000000, 0)
	pool := newSharedClientPool()
	pool.now = func() time.Time { return now }
	transport := &closeTrackingRoundTripper{}
	pool.store("old", &http.Client{Transport: transport})

	now = now.Add(sharedClientIdleTTL)
	require.Nil(t, pool.get("missing"))
	require.Equal(t, int32(1), transport.closed.Load())

	pool.mu.Lock()
	count := len(pool.entries)
	pool.mu.Unlock()
	require.Zero(t, count)
}

func TestValidatedTransportBoundsHostCache(t *testing.T) {
	now := time.Unix(1730000000, 0)
	transport := newValidatedTransport(http.DefaultTransport)
	for i := 0; i < validatedHostMaxEntries+50; i++ {
		transport.markValidatedHost("host-"+strconv.Itoa(i)+".example", now)
	}

	transport.validatedHostsMu.Lock()
	count := len(transport.validatedHosts)
	transport.validatedHostsMu.Unlock()
	require.Equal(t, validatedHostMaxEntries, count)
}

func TestValidatedTransport_CacheHostValidation(t *testing.T) {
	originalValidate := validateResolvedIP
	defer func() { validateResolvedIP = originalValidate }()

	var validateCalls int32
	validateResolvedIP = func(host string) error {
		atomic.AddInt32(&validateCalls, 1)
		require.Equal(t, "api.openai.com", host)
		return nil
	}

	var baseCalls int32
	base := roundTripFunc(func(_ *http.Request) (*http.Response, error) {
		atomic.AddInt32(&baseCalls, 1)
		return &http.Response{
			StatusCode: http.StatusOK,
			Body:       io.NopCloser(strings.NewReader(`{}`)),
			Header:     make(http.Header),
		}, nil
	})

	now := time.Unix(1730000000, 0)
	transport := newValidatedTransport(base)
	transport.now = func() time.Time { return now }

	req, err := http.NewRequest(http.MethodGet, "https://api.openai.com/v1/responses", nil)
	require.NoError(t, err)

	_, err = transport.RoundTrip(req)
	require.NoError(t, err)
	_, err = transport.RoundTrip(req)
	require.NoError(t, err)

	require.Equal(t, int32(1), atomic.LoadInt32(&validateCalls))
	require.Equal(t, int32(2), atomic.LoadInt32(&baseCalls))
}

func TestValidatedTransport_ExpiredCacheTriggersRevalidation(t *testing.T) {
	originalValidate := validateResolvedIP
	defer func() { validateResolvedIP = originalValidate }()

	var validateCalls int32
	validateResolvedIP = func(_ string) error {
		atomic.AddInt32(&validateCalls, 1)
		return nil
	}

	base := roundTripFunc(func(_ *http.Request) (*http.Response, error) {
		return &http.Response{
			StatusCode: http.StatusOK,
			Body:       io.NopCloser(strings.NewReader(`{}`)),
			Header:     make(http.Header),
		}, nil
	})

	now := time.Unix(1730001000, 0)
	transport := newValidatedTransport(base)
	transport.now = func() time.Time { return now }

	req, err := http.NewRequest(http.MethodGet, "https://api.openai.com/v1/responses", nil)
	require.NoError(t, err)

	_, err = transport.RoundTrip(req)
	require.NoError(t, err)

	now = now.Add(validatedHostTTL + time.Second)
	_, err = transport.RoundTrip(req)
	require.NoError(t, err)

	require.Equal(t, int32(2), atomic.LoadInt32(&validateCalls))
}

func TestValidatedTransport_ValidationErrorStopsRoundTrip(t *testing.T) {
	originalValidate := validateResolvedIP
	defer func() { validateResolvedIP = originalValidate }()

	expectedErr := errors.New("dns rebinding rejected")
	validateResolvedIP = func(_ string) error {
		return expectedErr
	}

	var baseCalls int32
	base := roundTripFunc(func(_ *http.Request) (*http.Response, error) {
		atomic.AddInt32(&baseCalls, 1)
		return &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(strings.NewReader(`{}`))}, nil
	})

	transport := newValidatedTransport(base)
	req, err := http.NewRequest(http.MethodGet, "https://api.openai.com/v1/responses", nil)
	require.NoError(t, err)

	_, err = transport.RoundTrip(req)
	require.ErrorIs(t, err, expectedErr)
	require.Equal(t, int32(0), atomic.LoadInt32(&baseCalls))
}
