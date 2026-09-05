package httpclient

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"errors"
	"io"
	"math/big"
	"net"
	"net/http"
	"strings"
	"sync"
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

func TestGetClientKeysCacheByConvergedHTTP1Profile(t *testing.T) {
	sharedClients = sync.Map{}
	h2Profile := &tlsfingerprint.Profile{
		CipherSuites:  []uint16{0x1301},
		ALPNProtocols: []string{"h2", "http/1.1"},
	}
	h2Client, err := GetClient(Options{Timeout: time.Second, TLSProfile: h2Profile})
	require.NoError(t, err)
	require.Equal(t, []string{"h2", "http/1.1"}, h2Profile.ALPNProtocols,
		"cache normalization must not mutate the caller's profile")

	h1Client, err := GetClient(Options{Timeout: time.Second, TLSProfile: &tlsfingerprint.Profile{
		CipherSuites:  []uint16{0x1301},
		ALPNProtocols: []string{"http/1.1"},
	}})
	require.NoError(t, err)
	require.Same(t, h2Client, h1Client,
		"profiles with identical effective HTTP/1.1 wire behavior must share one cache entry")
}

func TestGetClientSeparatesExtensionRandomizationProfiles(t *testing.T) {
	sharedClients = sync.Map{}
	fixed, err := GetClient(Options{TLSProfile: &tlsfingerprint.Profile{
		Extensions: []uint16{0, 10, 16, 43},
	}})
	require.NoError(t, err)
	randomized, err := GetClient(Options{TLSProfile: &tlsfingerprint.Profile{
		Extensions:              []uint16{0, 10, 16, 43},
		RandomizeExtensionOrder: true,
	}})
	require.NoError(t, err)

	require.NotSame(t, fixed, randomized,
		"fixed-order and per-connection-randomized profiles must not share a cached transport")
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

func TestGetClientConvergesH2ProfileALPNForUTLSDialerPaths(t *testing.T) {
	// A local TLS server that records the ALPN protocols the client offers in
	// its ClientHello. The request is expected to fail certificate verification
	// (self-signed); ALPN capture happens during ClientHello processing, before
	// the certificate is verified.
	cert := newSelfSignedTestCertificate(t)
	offeredCh := make(chan []string, 1)
	ln, err := tls.Listen("tcp", "127.0.0.1:0", &tls.Config{
		Certificates: []tls.Certificate{cert},
		NextProtos:   []string{"h2", "http/1.1"},
		GetConfigForClient: func(hello *tls.ClientHelloInfo) (*tls.Config, error) {
			offeredCh <- append([]string(nil), hello.SupportedProtos...)
			return nil, nil
		},
	})
	require.NoError(t, err)
	defer func() { _ = ln.Close() }()
	go func() {
		for {
			conn, acceptErr := ln.Accept()
			if acceptErr != nil {
				return
			}
			go func() {
				defer func() { _ = conn.Close() }()
				tlsConn, ok := conn.(*tls.Conn)
				if !ok {
					return
				}
				if handshakeErr := tlsConn.Handshake(); handshakeErr != nil {
					return
				}
			}()
		}
	}()

	profile := &tlsfingerprint.Profile{ALPNProtocols: []string{"h2", "http/1.1"}}
	client, err := GetClient(Options{Timeout: 2 * time.Second, TLSProfile: profile})
	require.NoError(t, err)
	_, _ = client.Get("https://" + ln.Addr().String())

	select {
	case offered := <-offeredCh:
		require.Equal(t, []string{"http/1.1"}, offered,
			"the h1-only uTLS dialer transport must never offer h2 to the server")
	case <-time.After(3 * time.Second):
		t.Fatal("timed out waiting for the client TLS handshake")
	}
	// Convergence happens on a clone: the caller's shared profile is untouched.
	require.Equal(t, []string{"h2", "http/1.1"}, profile.ALPNProtocols)
}

func newSelfSignedTestCertificate(t *testing.T) tls.Certificate {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	require.NoError(t, err)
	tmpl := x509.Certificate{
		SerialNumber: big.NewInt(1),
		Subject:      pkix.Name{CommonName: "127.0.0.1"},
		NotBefore:    time.Now().Add(-time.Hour),
		NotAfter:     time.Now().Add(time.Hour),
		KeyUsage:     x509.KeyUsageDigitalSignature,
		ExtKeyUsage:  []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		IPAddresses:  []net.IP{net.ParseIP("127.0.0.1")},
	}
	der, err := x509.CreateCertificate(rand.Reader, &tmpl, &tmpl, &key.PublicKey, key)
	require.NoError(t, err)
	return tls.Certificate{Certificate: [][]byte{der}, PrivateKey: key}
}
