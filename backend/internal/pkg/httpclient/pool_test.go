package httpclient

import (
	"errors"
	"io"
	"net/http"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/tlsfingerprint"
	"github.com/stretchr/testify/require"
)

func TestBuildTransportWithTLSProfileUsesContentKeyAndUTLSDialer(t *testing.T) {
	firstProfile := &tlsfingerprint.Profile{CipherSuites: []uint16{0x1301}}
	secondProfile := &tlsfingerprint.Profile{CipherSuites: []uint16{0x1302}}
	base := Options{Timeout: time.Second, TLSProfile: firstProfile}

	transport, err := buildTransport(base)
	require.NoError(t, err)
	require.NotNil(t, transport.DialTLSContext)
	require.False(t, transport.ForceAttemptHTTP2)

	changed := base
	changed.TLSProfile = secondProfile
	require.NotEqual(t, buildClientKey(base), buildClientKey(changed))
}

func TestBuildTransportWithTLSProfileProxiesPlainHTTPWithoutDoubleProxyingHTTPS(t *testing.T) {
	transport, err := buildTransport(Options{
		ProxyURL:   "http://proxy.example:8080",
		TLSProfile: &tlsfingerprint.Profile{CipherSuites: []uint16{0x1301}},
	})
	require.NoError(t, err)
	require.NotNil(t, transport.DialTLSContext)
	require.NotNil(t, transport.Proxy)

	httpReq, err := http.NewRequest(http.MethodGet, "http://upstream.example/v1", nil)
	require.NoError(t, err)
	proxyURL, err := transport.Proxy(httpReq)
	require.NoError(t, err)
	require.Equal(t, "http://proxy.example:8080", proxyURL.String())

	httpsReq, err := http.NewRequest(http.MethodGet, "https://upstream.example/v1", nil)
	require.NoError(t, err)
	proxyURL, err = transport.Proxy(httpsReq)
	require.NoError(t, err)
	require.Nil(t, proxyURL, "the fingerprint dialer already owns HTTPS proxy tunneling")
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return f(req)
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
