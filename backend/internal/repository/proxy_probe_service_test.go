package repository

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
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
	s.prober = &proxyProbeService{
		allowPrivateHosts: true,
	}
}

func (s *ProxyProbeServiceSuite) TearDownTest() {
	if s.proxySrv != nil {
		s.proxySrv.Close()
		s.proxySrv = nil
	}
}

func (s *ProxyProbeServiceSuite) setupProxyServer(handler http.HandlerFunc) {
	s.proxySrv = newLocalTestServer(s.T(), handler)
}

func (s *ProxyProbeServiceSuite) TestProbeProxy_InvalidProxyURL() {
	_, _, err := s.prober.ProbeProxy(s.ctx, "://bad")
	require.Error(s.T(), err)
	require.ErrorContains(s.T(), err, "failed to create proxy client")
}

func (s *ProxyProbeServiceSuite) TestProbeProxy_UnsupportedProxyScheme() {
	_, _, err := s.prober.ProbeProxy(s.ctx, "ftp://127.0.0.1:1")
	require.Error(s.T(), err)
	require.ErrorContains(s.T(), err, "failed to create proxy client")
}

func (s *ProxyProbeServiceSuite) TestProbeProxy_Success_Ifconfig() {
	s.setupProxyServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.Contains(r.RequestURI, "ifconfig.co") {
			w.Header().Set("Content-Type", "application/json")
			_, _ = io.WriteString(w, `{"ip":"1.2.3.4","city":"c","region_name":"r","country":"cc","country_iso":"CC"}`)
			return
		}
		w.WriteHeader(http.StatusServiceUnavailable)
	}))

	info, latencyMs, err := s.prober.ProbeProxy(s.ctx, s.proxySrv.URL)
	require.NoError(s.T(), err)
	require.GreaterOrEqual(s.T(), latencyMs, int64(0), "unexpected latency")
	require.Equal(s.T(), "1.2.3.4", info.IP)
	require.Equal(s.T(), "c", info.City)
	require.Equal(s.T(), "r", info.Region)
	require.Equal(s.T(), "cc", info.Country)
	require.Equal(s.T(), "CC", info.CountryCode)
}

func (s *ProxyProbeServiceSuite) TestProbeProxy_Success_IPifyFallback() {
	s.setupProxyServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.Contains(r.RequestURI, "ifconfig.co") {
			w.WriteHeader(http.StatusServiceUnavailable)
			return
		}
		if strings.Contains(r.RequestURI, "api64.ipify.org") {
			w.Header().Set("Content-Type", "application/json")
			_, _ = io.WriteString(w, `{"ip": "5.6.7.8"}`)
			return
		}
		w.WriteHeader(http.StatusServiceUnavailable)
	}))

	info, latencyMs, err := s.prober.ProbeProxy(s.ctx, s.proxySrv.URL)
	require.NoError(s.T(), err, "ProbeProxy should fallback to ipify")
	require.GreaterOrEqual(s.T(), latencyMs, int64(0), "unexpected latency")
	require.Equal(s.T(), "5.6.7.8", info.IP)
}

func (s *ProxyProbeServiceSuite) TestProbeProxy_AllFailed() {
	s.setupProxyServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusServiceUnavailable)
	}))

	_, _, err := s.prober.ProbeProxy(s.ctx, s.proxySrv.URL)
	require.Error(s.T(), err)
	require.ErrorContains(s.T(), err, "all probe URLs failed")
}

func (s *ProxyProbeServiceSuite) TestProbeProxy_InvalidJSON() {
	s.setupProxyServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.Contains(r.RequestURI, "ifconfig.co") {
			w.Header().Set("Content-Type", "application/json")
			_, _ = io.WriteString(w, "not-json")
			return
		}
		if strings.Contains(r.RequestURI, "api64.ipify.org") {
			w.Header().Set("Content-Type", "application/json")
			_, _ = io.WriteString(w, "not-json")
			return
		}
		w.WriteHeader(http.StatusServiceUnavailable)
	}))

	_, _, err := s.prober.ProbeProxy(s.ctx, s.proxySrv.URL)
	require.Error(s.T(), err)
	require.ErrorContains(s.T(), err, "all probe URLs failed")
}

func (s *ProxyProbeServiceSuite) TestProbeProxy_ProxyServerClosed() {
	s.setupProxyServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	s.proxySrv.Close()

	_, _, err := s.prober.ProbeProxy(s.ctx, s.proxySrv.URL)
	require.Error(s.T(), err, "expected error when proxy server is closed")
}

func (s *ProxyProbeServiceSuite) TestParseIfconfig_Success() {
	body := []byte(`{"ip":"1.2.3.4","city":"Beijing","region_name":"Beijing","country":"China","country_iso":"CN"}`)
	info, latencyMs, err := s.prober.parseIfconfig(body, 100)
	require.NoError(s.T(), err)
	require.Equal(s.T(), int64(100), latencyMs)
	require.Equal(s.T(), "1.2.3.4", info.IP)
	require.Equal(s.T(), "Beijing", info.City)
	require.Equal(s.T(), "Beijing", info.Region)
	require.Equal(s.T(), "China", info.Country)
	require.Equal(s.T(), "CN", info.CountryCode)
}

func (s *ProxyProbeServiceSuite) TestParseIfconfig_NoIP() {
	body := []byte(`{"ip":""}`)
	_, _, err := s.prober.parseIfconfig(body, 100)
	require.Error(s.T(), err)
	require.ErrorContains(s.T(), err, "no IP found")
}

func (s *ProxyProbeServiceSuite) TestParseIPify_Success() {
	body := []byte(`{"ip":"9.8.7.6"}`)
	info, latencyMs, err := s.prober.parseIPify(body, 50)
	require.NoError(s.T(), err)
	require.Equal(s.T(), int64(50), latencyMs)
	require.Equal(s.T(), "9.8.7.6", info.IP)
}

func (s *ProxyProbeServiceSuite) TestParseIPify_NoIP() {
	body := []byte(`{"ip":""}`)
	_, _, err := s.prober.parseIPify(body, 50)
	require.Error(s.T(), err)
	require.ErrorContains(s.T(), err, "no IP found")
}

func TestProxyProbeServiceSuite(t *testing.T) {
	suite.Run(t, new(ProxyProbeServiceSuite))
}
