package repository

import (
	"io"
	"net/http"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"
)

// HTTPUpstreamSuite HTTP 上游服务测试套件
// 使用 testify/suite 组织测试，支持 SetupTest 初始化
type HTTPUpstreamSuite struct {
	suite.Suite
	cfg *config.Config // 测试用配置
}

// SetupTest 每个测试用例执行前的初始化
// 创建空配置，各测试用例可按需覆盖
func (s *HTTPUpstreamSuite) SetupTest() {
	s.cfg = &config.Config{
		Security: config.SecurityConfig{
			URLAllowlist: config.URLAllowlistConfig{
				AllowPrivateHosts: true,
			},
		},
	}
}

// newService 创建测试用的 httpUpstreamService 实例
// 返回具体类型以便访问内部状态进行断言
func (s *HTTPUpstreamSuite) newService() *httpUpstreamService {
	up := NewHTTPUpstream(s.cfg)
	svc, ok := up.(*httpUpstreamService)
	require.True(s.T(), ok, "expected *httpUpstreamService")
	return svc
}

// TestDefaultResponseHeaderTimeout 测试默认响应头超时配置
// 验证未配置时使用 300 秒默认值
func (s *HTTPUpstreamSuite) TestDefaultResponseHeaderTimeout() {
	svc := s.newService()
	entry := mustGetOrCreateClient(s.T(), svc, "", 0, 0)
	transport, ok := entry.client.Transport.(*http.Transport)
	require.True(s.T(), ok, "expected *http.Transport")
	require.Equal(s.T(), 300*time.Second, transport.ResponseHeaderTimeout, "ResponseHeaderTimeout mismatch")
}

// TestCustomResponseHeaderTimeout 测试自定义响应头超时配置
// 验证配置值能正确应用到 Transport
func (s *HTTPUpstreamSuite) TestCustomResponseHeaderTimeout() {
	s.cfg.Gateway = config.GatewayConfig{ResponseHeaderTimeout: 7}
	svc := s.newService()
	entry := mustGetOrCreateClient(s.T(), svc, "", 0, 0)
	transport, ok := entry.client.Transport.(*http.Transport)
	require.True(s.T(), ok, "expected *http.Transport")
	require.Equal(s.T(), 7*time.Second, transport.ResponseHeaderTimeout, "ResponseHeaderTimeout mismatch")
}

// TestGetOrCreateClient_InvalidURLReturnsError 测试无效代理 URL 返回错误
// 验证解析失败时拒绝回退到直连模式
func (s *HTTPUpstreamSuite) TestGetOrCreateClient_InvalidURLReturnsError() {
	svc := s.newService()
	_, err := svc.getClientEntry("://bad-proxy-url", 1, 1, false, false)
	require.Error(s.T(), err, "expected error for invalid proxy URL")
}

// TestNormalizeProxyURL_Canonicalizes 测试代理 URL 规范化
// 验证等价地址能够映射到同一缓存键
func (s *HTTPUpstreamSuite) TestNormalizeProxyURL_Canonicalizes() {
	key1, _, err1 := normalizeProxyURL("http://proxy.local:8080")
	require.NoError(s.T(), err1)
	key2, _, err2 := normalizeProxyURL("http://proxy.local:8080/")
	require.NoError(s.T(), err2)
	require.Equal(s.T(), key1, key2, "expected normalized proxy keys to match")
}

// TestAcquireClient_OverLimitReturnsError 测试连接池缓存上限保护
// 验证超限且无可淘汰条目时返回错误
func (s *HTTPUpstreamSuite) TestAcquireClient_OverLimitReturnsError() {
	s.cfg.Gateway = config.GatewayConfig{
		ConnectionPoolIsolation: config.ConnectionPoolIsolationAccountProxy,
		MaxUpstreamClients:      1,
	}
	svc := s.newService()
	entry1, err := svc.acquireClient("http://proxy-a:8080", 1, 1)
	require.NoError(s.T(), err, "expected first acquire to succeed")
	require.NotNil(s.T(), entry1, "expected entry")

	entry2, err := svc.acquireClient("http://proxy-b:8080", 2, 1)
	require.Error(s.T(), err, "expected error when cache limit reached")
	require.Nil(s.T(), entry2, "expected nil entry when cache limit reached")
}

// TestDo_WithoutProxy_GoesDirect 测试无代理时直连
// 验证空代理 URL 时请求直接发送到目标服务器
func (s *HTTPUpstreamSuite) TestDo_WithoutProxy_GoesDirect() {
	// 创建模拟上游服务器
	upstream := newLocalTestServer(s.T(), http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.WriteString(w, "direct")
	}))
	s.T().Cleanup(upstream.Close)

	up := NewHTTPUpstream(s.cfg)

	req, err := http.NewRequest(http.MethodGet, upstream.URL+"/x", nil)
	require.NoError(s.T(), err, "NewRequest")
	resp, err := up.Do(req, "", 1, 1)
	require.NoError(s.T(), err, "Do")
	defer func() { _ = resp.Body.Close() }()
	b, _ := io.ReadAll(resp.Body)
	require.Equal(s.T(), "direct", string(b), "unexpected body")
}

// TestDo_WithHTTPProxy_UsesProxy 测试 HTTP 代理功能
// 验证请求通过代理服务器转发，使用绝对 URI 格式
func (s *HTTPUpstreamSuite) TestDo_WithHTTPProxy_UsesProxy() {
	// 用于接收代理请求的通道
	seen := make(chan string, 1)
	// 创建模拟代理服务器
	proxySrv := newLocalTestServer(s.T(), http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		seen <- r.RequestURI // 记录请求 URI
		_, _ = io.WriteString(w, "proxied")
	}))
	s.T().Cleanup(proxySrv.Close)

	s.cfg.Gateway = config.GatewayConfig{ResponseHeaderTimeout: 1}
	up := NewHTTPUpstream(s.cfg)

	// 发送请求到外部地址，应通过代理
	req, err := http.NewRequest(http.MethodGet, "http://example.com/test", nil)
	require.NoError(s.T(), err, "NewRequest")
	resp, err := up.Do(req, proxySrv.URL, 1, 1)
	require.NoError(s.T(), err, "Do")
	defer func() { _ = resp.Body.Close() }()
	b, _ := io.ReadAll(resp.Body)
	require.Equal(s.T(), "proxied", string(b), "unexpected body")

	// 验证代理收到的是绝对 URI 格式（HTTP 代理规范要求）
	select {
	case uri := <-seen:
		require.Equal(s.T(), "http://example.com/test", uri, "expected absolute-form request URI")
	default:
		require.Fail(s.T(), "expected proxy to receive request")
	}
}

// TestDo_EmptyProxy_UsesDirect 测试空代理字符串
// 验证空字符串代理等同于直连
func (s *HTTPUpstreamSuite) TestDo_EmptyProxy_UsesDirect() {
	upstream := newLocalTestServer(s.T(), http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.WriteString(w, "direct-empty")
	}))
	s.T().Cleanup(upstream.Close)

	up := NewHTTPUpstream(s.cfg)
	req, err := http.NewRequest(http.MethodGet, upstream.URL+"/y", nil)
	require.NoError(s.T(), err, "NewRequest")
	resp, err := up.Do(req, "", 1, 1)
	require.NoError(s.T(), err, "Do with empty proxy")
	defer func() { _ = resp.Body.Close() }()
	b, _ := io.ReadAll(resp.Body)
	require.Equal(s.T(), "direct-empty", string(b))
}

// TestAccountIsolation_DifferentAccounts 测试账户隔离模式
// 验证不同账户使用独立的连接池
func (s *HTTPUpstreamSuite) TestAccountIsolation_DifferentAccounts() {
	s.cfg.Gateway = config.GatewayConfig{ConnectionPoolIsolation: config.ConnectionPoolIsolationAccount}
	svc := s.newService()
	// 同一代理，不同账户
	entry1 := mustGetOrCreateClient(s.T(), svc, "http://proxy.local:8080", 1, 3)
	entry2 := mustGetOrCreateClient(s.T(), svc, "http://proxy.local:8080", 2, 3)
	require.NotSame(s.T(), entry1, entry2, "不同账号不应共享连接池")
	require.Equal(s.T(), 2, len(svc.clients), "账号隔离应缓存两个客户端")
}

// TestAccountProxyIsolation_DifferentProxy 测试账户+代理组合隔离模式
// 验证同一账户使用不同代理时创建独立连接池
func (s *HTTPUpstreamSuite) TestAccountProxyIsolation_DifferentProxy() {
	s.cfg.Gateway = config.GatewayConfig{ConnectionPoolIsolation: config.ConnectionPoolIsolationAccountProxy}
	svc := s.newService()
	// 同一账户，不同代理
	entry1 := mustGetOrCreateClient(s.T(), svc, "http://proxy-a:8080", 1, 3)
	entry2 := mustGetOrCreateClient(s.T(), svc, "http://proxy-b:8080", 1, 3)
	require.NotSame(s.T(), entry1, entry2, "账号+代理隔离应区分不同代理")
	require.Equal(s.T(), 2, len(svc.clients), "账号+代理隔离应缓存两个客户端")
}

// TestAccountModeProxyChangeClearsPool 测试账户模式下代理变更
// 验证账户切换代理时清理旧连接池，避免复用错误代理
func (s *HTTPUpstreamSuite) TestAccountModeProxyChangeClearsPool() {
	s.cfg.Gateway = config.GatewayConfig{ConnectionPoolIsolation: config.ConnectionPoolIsolationAccount}
	svc := s.newService()
	// 同一账户，先后使用不同代理
	entry1 := mustGetOrCreateClient(s.T(), svc, "http://proxy-a:8080", 1, 3)
	entry2 := mustGetOrCreateClient(s.T(), svc, "http://proxy-b:8080", 1, 3)
	require.NotSame(s.T(), entry1, entry2, "账号切换代理应创建新连接池")
	require.Equal(s.T(), 1, len(svc.clients), "账号模式下应仅保留一个连接池")
	require.False(s.T(), hasEntry(svc, entry1), "旧连接池应被清理")
}

// TestAccountConcurrencyOverridesPoolSettings 测试账户并发数覆盖连接池配置
// 验证账户隔离模式下，连接池大小与账户并发数对应
func (s *HTTPUpstreamSuite) TestAccountConcurrencyOverridesPoolSettings() {
	s.cfg.Gateway = config.GatewayConfig{ConnectionPoolIsolation: config.ConnectionPoolIsolationAccount}
	svc := s.newService()
	// 账户并发数为 12
	entry := mustGetOrCreateClient(s.T(), svc, "", 1, 12)
	transport, ok := entry.client.Transport.(*http.Transport)
	require.True(s.T(), ok, "expected *http.Transport")
	// 连接池参数应与并发数一致
	require.Equal(s.T(), 12, transport.MaxConnsPerHost, "MaxConnsPerHost mismatch")
	require.Equal(s.T(), 12, transport.MaxIdleConns, "MaxIdleConns mismatch")
	require.Equal(s.T(), 12, transport.MaxIdleConnsPerHost, "MaxIdleConnsPerHost mismatch")
}

// TestAccountConcurrencyFallbackToDefault 测试账户并发数为 0 时回退到默认配置
// 验证未指定并发数时使用全局配置值
func (s *HTTPUpstreamSuite) TestAccountConcurrencyFallbackToDefault() {
	s.cfg.Gateway = config.GatewayConfig{
		ConnectionPoolIsolation: config.ConnectionPoolIsolationAccount,
		MaxIdleConns:            77,
		MaxIdleConnsPerHost:     55,
		MaxConnsPerHost:         66,
	}
	svc := s.newService()
	// 账户并发数为 0，应使用全局配置
	entry := mustGetOrCreateClient(s.T(), svc, "", 1, 0)
	transport, ok := entry.client.Transport.(*http.Transport)
	require.True(s.T(), ok, "expected *http.Transport")
	require.Equal(s.T(), 66, transport.MaxConnsPerHost, "MaxConnsPerHost fallback mismatch")
	require.Equal(s.T(), 77, transport.MaxIdleConns, "MaxIdleConns fallback mismatch")
	require.Equal(s.T(), 55, transport.MaxIdleConnsPerHost, "MaxIdleConnsPerHost fallback mismatch")
}

// TestEvictOverLimitRemovesOldestIdle 测试超出数量限制时的 LRU 淘汰
// 验证优先淘汰最久未使用的空闲客户端
func (s *HTTPUpstreamSuite) TestEvictOverLimitRemovesOldestIdle() {
	s.cfg.Gateway = config.GatewayConfig{
		ConnectionPoolIsolation: config.ConnectionPoolIsolationAccountProxy,
		MaxUpstreamClients:      2, // 最多缓存 2 个客户端
	}
	svc := s.newService()
	// 创建两个客户端，设置不同的最后使用时间
	entry1 := mustGetOrCreateClient(s.T(), svc, "http://proxy-a:8080", 1, 1)
	entry2 := mustGetOrCreateClient(s.T(), svc, "http://proxy-b:8080", 2, 1)
	atomic.StoreInt64(&entry1.lastUsed, time.Now().Add(-2*time.Hour).UnixNano()) // 最久
	atomic.StoreInt64(&entry2.lastUsed, time.Now().Add(-time.Hour).UnixNano())
	// 创建第三个客户端，触发淘汰
	_ = mustGetOrCreateClient(s.T(), svc, "http://proxy-c:8080", 3, 1)

	require.LessOrEqual(s.T(), len(svc.clients), 2, "应保持在缓存上限内")
	require.False(s.T(), hasEntry(svc, entry1), "最久未使用的连接池应被清理")
}

// TestIdleTTLDoesNotEvictActive 测试活跃请求保护
// 验证有进行中请求的客户端不会被空闲超时淘汰
func (s *HTTPUpstreamSuite) TestIdleTTLDoesNotEvictActive() {
	s.cfg.Gateway = config.GatewayConfig{
		ConnectionPoolIsolation: config.ConnectionPoolIsolationAccount,
		ClientIdleTTLSeconds:    1, // 1 秒空闲超时
	}
	svc := s.newService()
	entry1 := mustGetOrCreateClient(s.T(), svc, "", 1, 1)
	// 设置为很久之前使用，但有活跃请求
	atomic.StoreInt64(&entry1.lastUsed, time.Now().Add(-2*time.Minute).UnixNano())
	atomic.StoreInt64(&entry1.inFlight, 1) // 模拟有活跃请求
	// 创建新客户端，触发淘汰检查
	_, _ = svc.getOrCreateClient("", 2, 1)

	require.True(s.T(), hasEntry(svc, entry1), "有活跃请求时不应回收")
}

// TestHTTPUpstreamSuite 运行测试套件
func TestHTTPUpstreamSuite(t *testing.T) {
	suite.Run(t, new(HTTPUpstreamSuite))
}

// mustGetOrCreateClient 测试辅助函数，调用 getOrCreateClient 并断言无错误
func mustGetOrCreateClient(t *testing.T, svc *httpUpstreamService, proxyURL string, accountID int64, concurrency int) *upstreamClientEntry {
	t.Helper()
	entry, err := svc.getOrCreateClient(proxyURL, accountID, concurrency)
	require.NoError(t, err, "getOrCreateClient(%q, %d, %d)", proxyURL, accountID, concurrency)
	return entry
}

// hasEntry 检查客户端是否存在于缓存中
// 辅助函数，用于验证淘汰逻辑
func hasEntry(svc *httpUpstreamService, target *upstreamClientEntry) bool {
	for _, entry := range svc.clients {
		if entry == target {
			return true
		}
	}
	return false
}
