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

func (s *ProxyProbeServiceSuite) TestProbeProxy_Success_IPAPI() {
	// 限制为 HTTP 探测目标，避免 mock HTTP 代理无法处理 HTTPS CONNECT 导致超时。
	s.prober.probeURLs = []probeTarget{
		{"http://ip-api.com/json/?lang=zh-CN", "ip-api"},
		{"http://api64.ipify.org?format=json", "ipify"},
	}
	s.setupProxyServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.Contains(r.RequestURI, "ip-api.com") {
			w.Header().Set("Content-Type", "application/json")
			_, _ = io.WriteString(w, `{"status":"success","query":"1.2.3.4","city":"c","regionName":"r","country":"cc","countryCode":"CC"}`)
			return
		}
		w.WriteHeader(http.StatusServiceUnavailable)
	}))

	info, latencyMs, err := s.prober.ProbeProxy(s.ctx, s.proxySrv.URL)
	require.NoError(s.T(), err, "ProbeProxy")
	require.GreaterOrEqual(s.T(), latencyMs, int64(0), "unexpected latency")
	require.Equal(s.T(), "1.2.3.4", info.IP)
	require.Equal(s.T(), "c", info.City)
	require.Equal(s.T(), "r", info.Region)
	require.Equal(s.T(), "cc", info.Country)
	require.Equal(s.T(), "CC", info.CountryCode)
}

func (s *ProxyProbeServiceSuite) TestProbeProxy_Success_IPifyFallback() {
	s.prober.probeURLs = []probeTarget{
		{"http://ip-api.com/json/?lang=zh-CN", "ip-api"},
		{"http://api64.ipify.org?format=json", "ipify"},
	}
	s.setupProxyServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.Contains(r.RequestURI, "ip-api.com") {
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

func (s *ProxyProbeServiceSuite) TestProbeProxy_SkipsHTTPWebblockAndFallsBack() {
	// 模拟：明文 HTTP 探测被 302 劫持；后续目标返回真实出口 IP JSON。
	s.prober.probeURLs = []probeTarget{
		{"http://ip-api.com/json/?lang=zh-CN", "ip-api"},
		{"http://api64.ipify.org?format=json", "ipify"},
	}
	s.setupProxyServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.Contains(r.RequestURI, "ip-api.com") {
			w.Header().Set("Location", "https://dnspod.qcloud.com/static/webblock.html?d=ip-api.com")
			w.WriteHeader(http.StatusFound)
			return
		}
		if strings.Contains(r.RequestURI, "api64.ipify.org") {
			w.Header().Set("Content-Type", "application/json")
			_, _ = io.WriteString(w, `{"ip":"9.9.9.9"}`)
			return
		}
		w.WriteHeader(http.StatusServiceUnavailable)
	}))

	info, _, err := s.prober.ProbeProxy(s.ctx, s.proxySrv.URL)
	require.NoError(s.T(), err)
	require.Equal(s.T(), "9.9.9.9", info.IP)
}

func (s *ProxyProbeServiceSuite) TestProbeProxy_RejectsHTMLBody() {
	s.prober.probeURLs = []probeTarget{
		{"http://api64.ipify.org?format=json", "ipify"},
	}
	s.setupProxyServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		w.WriteHeader(http.StatusOK)
		_, _ = io.WriteString(w, `<!DOCTYPE html><html><body class="beian-block">blocked</body></html>`)
	}))

	_, _, err := s.prober.ProbeProxy(s.ctx, s.proxySrv.URL)
	require.Error(s.T(), err)
	require.ErrorContains(s.T(), err, "HTML")
}

func (s *ProxyProbeServiceSuite) TestProbeProxy_DoesNotFollowRedirect() {
	s.prober.probeURLs = []probeTarget{
		{"http://api64.ipify.org?format=json", "ipify"},
	}
	s.setupProxyServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.Contains(r.RequestURI, "api64.ipify.org") {
			w.Header().Set("Location", "http://example.invalid/webblock")
			w.WriteHeader(http.StatusFound)
			return
		}
		// 若错误地跟随了重定向，这里会返回“假成功”
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"ip":"should-not-use"}`)
	}))

	_, _, err := s.prober.ProbeProxy(s.ctx, s.proxySrv.URL)
	require.Error(s.T(), err)
	require.ErrorContains(s.T(), err, "status: 302")
}

func (s *ProxyProbeServiceSuite) TestProbeProxy_AllFailed() {
	s.prober.probeURLs = []probeTarget{
		{"http://ip-api.com/json/?lang=zh-CN", "ip-api"},
		{"http://api64.ipify.org?format=json", "ipify"},
	}
	s.setupProxyServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusServiceUnavailable)
	}))

	_, _, err := s.prober.ProbeProxy(s.ctx, s.proxySrv.URL)
	require.Error(s.T(), err)
	require.ErrorContains(s.T(), err, "all probe URLs failed")
}

func (s *ProxyProbeServiceSuite) TestProbeProxy_InvalidJSON() {
	s.prober.probeURLs = []probeTarget{
		{"http://ip-api.com/json/?lang=zh-CN", "ip-api"},
		{"http://api64.ipify.org?format=json", "ipify"},
	}
	s.setupProxyServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.Contains(r.RequestURI, "ip-api.com") {
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
	s.prober.probeURLs = []probeTarget{
		{"http://api64.ipify.org?format=json", "ipify"},
	}
	s.setupProxyServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	s.proxySrv.Close()

	_, _, err := s.prober.ProbeProxy(s.ctx, s.proxySrv.URL)
	require.Error(s.T(), err, "expected error when proxy server is closed")
}

func (s *ProxyProbeServiceSuite) TestParseIPAPI_Success() {
	body := []byte(`{"status":"success","query":"1.2.3.4","city":"Beijing","regionName":"Beijing","country":"China","countryCode":"CN"}`)
	info, latencyMs, err := s.prober.parseIPAPI(body, 100)
	require.NoError(s.T(), err)
	require.Equal(s.T(), int64(100), latencyMs)
	require.Equal(s.T(), "1.2.3.4", info.IP)
	require.Equal(s.T(), "Beijing", info.City)
	require.Equal(s.T(), "Beijing", info.Region)
	require.Equal(s.T(), "China", info.Country)
	require.Equal(s.T(), "CN", info.CountryCode)
}

func (s *ProxyProbeServiceSuite) TestParseIPAPI_Failure() {
	body := []byte(`{"status":"fail","message":"rate limited"}`)
	_, _, err := s.prober.parseIPAPI(body, 100)
	require.Error(s.T(), err)
	require.ErrorContains(s.T(), err, "rate limited")
}

func (s *ProxyProbeServiceSuite) TestParseIPify_Success() {
	body := []byte(`{"ip": "2001:db8::1"}`)
	info, latencyMs, err := s.prober.parseIPify(body, 50)
	require.NoError(s.T(), err)
	require.Equal(s.T(), int64(50), latencyMs)
	require.Equal(s.T(), "2001:db8::1", info.IP)
}

func (s *ProxyProbeServiceSuite) TestParseIPify_NoIP() {
	body := []byte(`{"ip": ""}`)
	_, _, err := s.prober.parseIPify(body, 50)
	require.Error(s.T(), err)
	require.ErrorContains(s.T(), err, "no IP found")
}

func (s *ProxyProbeServiceSuite) TestLooksLikeHTMLProbeBody() {
	require.True(s.T(), looksLikeHTMLProbeBody([]byte("<!DOCTYPE html><html></html>")))
	require.True(s.T(), looksLikeHTMLProbeBody([]byte(` <div class="beian-block">x</div>`)))
	require.True(s.T(), looksLikeHTMLProbeBody([]byte(`webblock intercept`)))
	require.False(s.T(), looksLikeHTMLProbeBody([]byte(`{"ip":"1.2.3.4"}`)))
	require.False(s.T(), looksLikeHTMLProbeBody(nil))
}

func (s *ProxyProbeServiceSuite) TestDefaultProbeURLsPreferHTTPS() {
	require.GreaterOrEqual(s.T(), len(defaultProbeURLs), 2)
	require.True(s.T(), strings.HasPrefix(defaultProbeURLs[0].url, "https://"))
	require.True(s.T(), strings.HasPrefix(defaultProbeURLs[1].url, "https://"))
}

func TestProxyProbeServiceSuite(t *testing.T) {
	suite.Run(t, new(ProxyProbeServiceSuite))
}
