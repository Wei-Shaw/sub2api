package repository

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"
)

type ProxyProbeServiceSuite struct {
	suite.Suite
	ctx      context.Context
	proxySrv *httptest.Server
	prober   *proxyProbeService
}

func (s *ProxyProbeServiceSuite) SetupTest() {
	s.ctx = context.Background()
	s.prober = &proxyProbeService{ipInfoURL: "http://ipinfo.test/json"}
}

func (s *ProxyProbeServiceSuite) TearDownTest() {
	if s.proxySrv != nil {
		s.proxySrv.Close()
		s.proxySrv = nil
	}
}

func (s *ProxyProbeServiceSuite) setupProxyServer(handler http.HandlerFunc) {
	s.proxySrv = httptest.NewServer(handler)
}

func (s *ProxyProbeServiceSuite) TestCreateProxyTransport_InvalidURL() {
	_, err := createProxyTransport("://bad", false)
	require.Error(s.T(), err)
	require.ErrorContains(s.T(), err, "invalid proxy URL")
}

func (s *ProxyProbeServiceSuite) TestCreateProxyTransport_UnsupportedScheme() {
	_, err := createProxyTransport("ftp://127.0.0.1:1", false)
	require.Error(s.T(), err)
	require.ErrorContains(s.T(), err, "unsupported proxy protocol")
}

func (s *ProxyProbeServiceSuite) TestCreateProxyTransport_Socks5SetsDialer() {
	tr, err := createProxyTransport("socks5://127.0.0.1:1080", false)
	require.NoError(s.T(), err, "createProxyTransport")
	require.NotNil(s.T(), tr.DialContext, "expected DialContext to be set for socks5")
}

func (s *ProxyProbeServiceSuite) TestProbeProxy_Success() {
	seen := make(chan string, 1)
	s.setupProxyServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		seen <- r.RequestURI
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"ip":"1.2.3.4","city":"c","region":"r","country":"cc"}`)
	}))

	info, latencyMs, err := s.prober.ProbeProxy(s.ctx, s.proxySrv.URL)
	require.NoError(s.T(), err, "ProbeProxy")
	require.GreaterOrEqual(s.T(), latencyMs, int64(0), "unexpected latency")
	require.Equal(s.T(), "1.2.3.4", info.IP)
	require.Equal(s.T(), "c", info.City)
	require.Equal(s.T(), "r", info.Region)
	require.Equal(s.T(), "cc", info.Country)

	// Verify proxy received the request
	select {
	case uri := <-seen:
		require.Contains(s.T(), uri, "ipinfo.test", "expected request to go through proxy")
	default:
		require.Fail(s.T(), "expected proxy to receive request")
	}
}

func (s *ProxyProbeServiceSuite) TestProbeProxy_NonOKStatus() {
	s.setupProxyServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusServiceUnavailable)
	}))

	_, _, err := s.prober.ProbeProxy(s.ctx, s.proxySrv.URL)
	require.Error(s.T(), err)
	require.ErrorContains(s.T(), err, "status: 503")
}

func (s *ProxyProbeServiceSuite) TestProbeProxy_InvalidJSON() {
	s.setupProxyServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, "not-json")
	}))

	_, _, err := s.prober.ProbeProxy(s.ctx, s.proxySrv.URL)
	require.Error(s.T(), err)
	require.ErrorContains(s.T(), err, "failed to parse response")
}

func (s *ProxyProbeServiceSuite) TestProbeProxy_InvalidIPInfoURL() {
	s.prober.ipInfoURL = "://invalid-url"
	s.setupProxyServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	_, _, err := s.prober.ProbeProxy(s.ctx, s.proxySrv.URL)
	require.Error(s.T(), err, "expected error for invalid ipInfoURL")
}

func (s *ProxyProbeServiceSuite) TestProbeProxy_ProxyServerClosed() {
	s.setupProxyServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	s.proxySrv.Close()

	_, _, err := s.prober.ProbeProxy(s.ctx, s.proxySrv.URL)
	require.Error(s.T(), err, "expected error when proxy server is closed")
}

func TestProxyProbeServiceSuite(t *testing.T) {
	suite.Run(t, new(ProxyProbeServiceSuite))
}
