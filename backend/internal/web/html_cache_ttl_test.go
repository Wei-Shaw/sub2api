//go:build embed

package web

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// 本文件覆盖「注入进 index.html 的公开设置缓存不会自然过期」这条链路。
//
// 缓存失效原本只有一条路径：进程内设置写入 → SetOnUpdateCallback → Invalidate()。
// 任何**触达不到本进程**的写入（直接改库、运维脚本、另一个副本）都无法让它失效，
// 于是注入值永久滞留旧值，而 /api/v1/settings/public 已是新值 —— 前端据此选错数据源。
// 加上 TTL 后，这类带外写入必须在有界时间内自动收敛。

// expireHTMLCache 把缓存条目的到期时间推到过去，等价于「TTL 已到」。
// 直接改包内字段，避免让测试真的睡满一个 TTL。
func expireHTMLCache(t *testing.T, s *FrontendServer) {
	t.Helper()
	s.cache.mu.Lock()
	defer s.cache.mu.Unlock()
	require.False(t, s.cache.expiresAt.IsZero(), "前置条件：缓存必须是已 Set 过的状态")
	s.cache.expiresAt = time.Now().Add(-time.Second)
}

func newIndexRouter(t *testing.T, s *FrontendServer) *gin.Engine {
	t.Helper()
	router := gin.New()
	router.Use(s.Middleware())
	return router
}

func getIndex(router *gin.Engine, ifNoneMatch string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	if ifNoneMatch != "" {
		req.Header.Set("If-None-Match", ifNoneMatch)
	}
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	return w
}

func TestHTMLCache_TTLExpiresWithoutInvalidate(t *testing.T) {
	cache := NewHTMLCache()
	cache.SetBaseHTML([]byte("<html></html>"))
	cache.Set([]byte("<html>v1</html>"), []byte(`{"channel_monitor_mode":"v1"}`))

	require.NotNil(t, cache.Get(), "刚写入必须命中")

	// 不调用 Invalidate，仅让时间走过 TTL。
	cache.mu.Lock()
	cache.expiresAt = time.Now().Add(-time.Second)
	cache.mu.Unlock()

	assert.Nil(t, cache.Get(), "TTL 到点后必须视为未命中，否则带外改设置永远收敛不了")
}

func TestHTMLCache_InvalidateThenSetIsFresh(t *testing.T) {
	cache := NewHTMLCache()
	cache.SetBaseHTML([]byte("<html></html>"))
	cache.Set([]byte("<html>v1</html>"), []byte(`{"v":1}`))

	cache.Invalidate()
	assert.Nil(t, cache.Get())
	assert.True(t, cache.expiresAt.IsZero(), "Invalidate 必须清掉到期时间")

	cache.Set([]byte("<html>v2</html>"), []byte(`{"v":2}`))
	assert.NotNil(t, cache.Get(), "重新 Set 后必须重新可命中")
}

func TestHTMLCache_TTLIsBounded(t *testing.T) {
	// 防止有人把 TTL 调成 0（等于永不缓存）或调到小时级（等于没有 TTL）。
	assert.Greater(t, htmlCacheTTL, time.Duration(0))
	assert.LessOrEqual(t, htmlCacheTTL, 5*time.Minute)
}

func TestHTMLCache_SetExtendsExpiryFromNow(t *testing.T) {
	cache := NewHTMLCache()
	cache.SetBaseHTML([]byte("<html></html>"))
	cache.Set([]byte("<html>v1</html>"), []byte(`{"v":1}`))

	first := cache.expiresAt
	time.Sleep(2 * time.Millisecond)
	cache.Set([]byte("<html>v2</html>"), []byte(`{"v":2}`))

	assert.True(t, cache.expiresAt.After(first), "每次 Set 都应从当下重新计算到期时间")
}

// 这是本修复的核心回归用例：模拟一次**绕过 settingService 的带外改设置**。
func TestFrontendServer_OutOfBandSettingsChangeConvergesAfterTTL(t *testing.T) {
	provider := &mockSettingsProvider{
		settings: map[string]string{"channel_monitor_mode": "v1"},
	}
	server, err := NewFrontendServer(provider)
	require.NoError(t, err)
	router := newIndexRouter(t, server)

	// 第一次请求：把 v1 渲染进 HTML。
	first := getIndex(router, "")
	require.Equal(t, http.StatusOK, first.Code)
	require.Contains(t, first.Body.String(), `"channel_monitor_mode":"v1"`,
		"前置条件：首次请求必须注入当时的设置")
	callsAfterFirst := provider.called

	// 带外把设置改成 v2：只改数据源，绝不触碰本进程的缓存失效回调。
	provider.settings = map[string]string{"channel_monitor_mode": "v2"}

	// TTL 未到：仍然吃缓存，这是「进程内写入靠 Invalidate 立即生效」之外的高效路径。
	stillCached := getIndex(router, "")
	assert.Contains(t, stillCached.Body.String(), `"channel_monitor_mode":"v1"`,
		"TTL 未到时应当继续吃缓存")
	assert.Equal(t, callsAfterFirst, provider.called, "TTL 未到不应重新读设置")

	// TTL 到点：必须自动重新渲染，注入值收敛到新设置。
	expireHTMLCache(t, server)
	converged := getIndex(router, "")
	assert.Equal(t, http.StatusOK, converged.Code)
	assert.Contains(t, converged.Body.String(), `"channel_monitor_mode":"v2"`,
		"TTL 到点后必须自动收敛到新设置，否则前端会永远按旧开关选数据源")
	assert.NotContains(t, converged.Body.String(), `"channel_monitor_mode":"v1"`)
	assert.Greater(t, provider.called, callsAfterFirst, "收敛必须来自重新读取设置")
}

// TTL 到期重建时，若设置其实没变，仍然应该回 304 —— 否则每次到期都要白传一份完整 HTML。
func TestFrontendServer_UnchangedSettingsAfterTTLStillReturn304(t *testing.T) {
	provider := &mockSettingsProvider{
		settings: map[string]string{"channel_monitor_mode": "v1"},
	}
	server, err := NewFrontendServer(provider)
	require.NoError(t, err)
	router := newIndexRouter(t, server)

	first := getIndex(router, "")
	require.Equal(t, http.StatusOK, first.Code)
	etag := first.Header().Get("ETag")
	require.NotEmpty(t, etag)

	expireHTMLCache(t, server)

	revalidated := getIndex(router, etag)
	assert.Equal(t, http.StatusNotModified, revalidated.Code,
		"设置未变时，TTL 到期重建后仍应回 304")
	assert.Empty(t, revalidated.Body.String(), "304 不能带正文")
}

// 设置真的变了，就必须给出新的 ETag 和完整正文，让浏览器丢掉旧配置。
func TestFrontendServer_ChangedSettingsAfterTTLReturnsNewETagAndBody(t *testing.T) {
	provider := &mockSettingsProvider{
		settings: map[string]string{"channel_monitor_mode": "v1"},
	}
	server, err := NewFrontendServer(provider)
	require.NoError(t, err)
	router := newIndexRouter(t, server)

	first := getIndex(router, "")
	oldETag := first.Header().Get("ETag")
	require.NotEmpty(t, oldETag)

	provider.settings = map[string]string{"channel_monitor_mode": "v2"}
	expireHTMLCache(t, server)

	changed := getIndex(router, oldETag)
	require.Equal(t, http.StatusOK, changed.Code, "设置变了就不能回 304")
	assert.Contains(t, changed.Body.String(), `"channel_monitor_mode":"v2"`)
	assert.NotEqual(t, oldETag, changed.Header().Get("ETag"), "内容变了 ETag 必须跟着变")
	assert.True(t, strings.Contains(changed.Header().Get("Cache-Control"), "no-cache"),
		"index.html 必须继续要求每次重新校验")
}
