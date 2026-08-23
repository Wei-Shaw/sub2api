//go:build unit

package tlsfingerprint

import (
	"context"
	"crypto/tls"
	"errors"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"

	utls "github.com/refraction-networking/utls"
	"github.com/stretchr/testify/require"
)

// alpnReportingConn wraps a *tls.Conn and reports the negotiated protocol in
// the utls ConnectionState shape that TLSRoundTripper inspects, mimicking a
// *utls.UConn without requiring a real uTLS handshake against a loopback
// server (performTLSHandshake verifies certificates against system roots).
type alpnReportingConn struct {
	*tls.Conn
	proto string
}

func (c *alpnReportingConn) ConnectionState() utls.ConnectionState {
	return utls.ConnectionState{NegotiatedProtocol: c.proto}
}

// sniffTestDialer dials addr with crypto/tls (skipping verification for the
// loopback test server) and reports the actually negotiated ALPN protocol.
func sniffTestDialer(dials *atomic.Int64) UtlsDialFunc {
	return func(ctx context.Context, network, addr string) (net.Conn, error) {
		raw, err := (&net.Dialer{}).DialContext(ctx, network, addr)
		if err != nil {
			return nil, err
		}
		tlsConn := tls.Client(raw, &tls.Config{
			InsecureSkipVerify: true, //nolint:gosec // loopback test server only
			NextProtos:         []string{"h2", "http/1.1"},
		})
		if err := tlsConn.HandshakeContext(ctx); err != nil {
			_ = raw.Close()
			return nil, err
		}
		if dials != nil {
			dials.Add(1)
		}
		return &alpnReportingConn{Conn: tlsConn, proto: tlsConn.ConnectionState().NegotiatedProtocol}, nil
	}
}

func TestTLSRoundTripperNegotiatesHTTP2(t *testing.T) {
	server := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Saw-Proto", r.Proto)
		w.WriteHeader(http.StatusOK)
	}))
	server.EnableHTTP2 = true
	server.StartTLS()
	t.Cleanup(server.Close)

	var dials atomic.Int64
	client := &http.Client{Transport: NewTLSRoundTripper(sniffTestDialer(&dials), nil, nil)}

	for i := 0; i < 2; i++ {
		resp, err := client.Get(server.URL)
		require.NoError(t, err)
		body, err := io.ReadAll(resp.Body)
		require.NoError(t, err)
		require.NoError(t, resp.Body.Close())
		require.Equal(t, http.StatusOK, resp.StatusCode)
		require.Equal(t, 2, resp.ProtoMajor, "response must speak HTTP/2: %s", string(body))
		require.Equal(t, "HTTP/2.0", resp.Header.Get("X-Saw-Proto"))
	}

	// The bootstrap connection is handed to the h2 transport and multiplexes
	// every request; no second handshake may happen.
	require.Equal(t, int64(1), dials.Load())
}

func TestTLSRoundTripperFallsBackToHTTP1(t *testing.T) {
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Saw-Proto", r.Proto)
		w.WriteHeader(http.StatusOK)
	}))
	t.Cleanup(server.Close)

	var dials atomic.Int64
	client := &http.Client{Transport: NewTLSRoundTripper(sniffTestDialer(&dials), nil, nil)}

	for i := 0; i < 2; i++ {
		resp, err := client.Get(server.URL)
		require.NoError(t, err)
		_, err = io.ReadAll(resp.Body)
		require.NoError(t, err)
		require.NoError(t, resp.Body.Close())
		require.Equal(t, http.StatusOK, resp.StatusCode)
		require.Equal(t, 1, resp.ProtoMajor, "response must speak HTTP/1.1")
		require.Equal(t, "HTTP/1.1", resp.Header.Get("X-Saw-Proto"))
	}

	// h1 keep-alive reuses the bootstrap connection.
	require.Equal(t, int64(1), dials.Load())
}

func TestTLSRoundTripperPreservesConfiguredTransports(t *testing.T) {
	h1 := &http.Transport{MaxIdleConnsPerHost: 7}
	rt := NewTLSRoundTripper(sniffTestDialer(nil), h1, nil)

	require.Equal(t, 7, rt.h1.MaxIdleConnsPerHost, "caller pool settings must survive")
	require.False(t, rt.h1.ForceAttemptHTTP2, "h1 transport must never attempt HTTP/2 on uTLS conns")
	require.NotNil(t, rt.h1.DialTLSContext)
	require.NotNil(t, rt.h1.TLSNextProto, "net/http's built-in H2 hook must be disabled")
	require.NotNil(t, rt.h2, "nil h2 transport must be replaced with a default")
	require.NotNil(t, rt.h2.DialTLSContext)
	require.False(t, rt.h2.AllowHTTP)

	rt.CloseIdleConnections() // must not panic before any request
}

func TestTLSRoundTripperSurfacesBootstrapDialError(t *testing.T) {
	dialErr := errors.New("dial refused")
	rt := NewTLSRoundTripper(func(context.Context, string, string) (net.Conn, error) {
		return nil, dialErr
	}, nil, nil)

	req, err := http.NewRequest(http.MethodGet, "https://upstream.example/", nil)
	require.NoError(t, err)
	_, err = rt.RoundTrip(req)
	require.ErrorIs(t, err, dialErr)
}

func TestCustomProfileAdvertisesConfiguredALPN(t *testing.T) {
	profile := &Profile{Name: "h2 profile", ALPNProtocols: []string{"h2", "http/1.1"}}
	spec := buildClientHelloSpecFromProfile(profile)

	var alpn []string
	for _, extension := range spec.Extensions {
		if typed, ok := extension.(*utls.ALPNExtension); ok {
			alpn = typed.AlpnProtocols
		}
	}
	require.Equal(t, []string{"h2", "http/1.1"}, alpn)
}
