//go:build unit

package service

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

// newTestResolver 构造一个用于测试的 resolver：gin engine + 静态 CIDR。
// Start 之后 rootCtx 为传入 ctx（测试要能 cancel）。
func newTestResolver(t *testing.T, staticCIDRs []string) *TrustedProxyResolver {
	t.Helper()
	gin.SetMode(gin.TestMode)
	engine := gin.New()
	return NewTrustedProxyResolver(engine, staticCIDRs)
}

// TestMergedCIDRs_Union 验证 static + extra + source cache 会被合并去重。
func TestMergedCIDRs_Union(t *testing.T) {
	r := newTestResolver(t, []string{"10.0.0.0/8", "192.168.1.0/24"})
	// 手动写 sourceCache（模拟一次成功拉取）
	r.sourceCache.Store("s1", []string{"172.16.0.0/12", "10.0.0.0/8"}) // 与 static 重复
	// enabled + sources
	r.enabled.Store(true)
	srcs := []TrustedProxyDynamicSource{{ID: "s1", Enabled: true}}
	r.sources.Store(&srcs)
	extra := []string{"192.168.1.0/24", "203.0.113.0/24"} // 与 static 重复 + 新增
	r.extraCIDRs.Store(&extra)

	merged := r.mergedCIDRs()
	// 应包含 4 条唯一：10.0.0.0/8、192.168.1.0/24、172.16.0.0/12、203.0.113.0/24
	require.Contains(t, merged, "10.0.0.0/8")
	require.Contains(t, merged, "192.168.1.0/24")
	require.Contains(t, merged, "172.16.0.0/12")
	require.Contains(t, merged, "203.0.113.0/24")
	require.Len(t, merged, 4, "去重后应有 4 条")
}

// TestMergedCIDRs_DisabledSkipSourceCache 验证总开关关闭时不合并 source cache。
func TestMergedCIDRs_DisabledSkipSourceCache(t *testing.T) {
	r := newTestResolver(t, []string{"10.0.0.0/8"})
	r.sourceCache.Store("s1", []string{"172.16.0.0/12"})
	r.enabled.Store(false)
	srcs := []TrustedProxyDynamicSource{{ID: "s1", Enabled: true}}
	r.sources.Store(&srcs)
	extra := []string{"203.0.113.0/24"}
	r.extraCIDRs.Store(&extra)

	merged := r.mergedCIDRs()
	require.Contains(t, merged, "10.0.0.0/8")
	require.Contains(t, merged, "203.0.113.0/24") // extra 依然生效
	require.NotContains(t, merged, "172.16.0.0/12", "总开关关闭时不应合并动态拉取缓存")
}

// TestMergedCIDRs_DisabledSourceSkipsCache 验证 source 单独 disabled 时也不合并。
func TestMergedCIDRs_DisabledSourceSkipsCache(t *testing.T) {
	r := newTestResolver(t, nil)
	r.sourceCache.Store("s1", []string{"172.16.0.0/12"})
	r.enabled.Store(true)
	srcs := []TrustedProxyDynamicSource{{ID: "s1", Enabled: false}}
	r.sources.Store(&srcs)

	merged := r.mergedCIDRs()
	require.Empty(t, merged)
}

// TestFetchAndUpdate_Success 通过 httptest 服务器验证一次成功拉取的完整链路。
func TestFetchAndUpdate_Success(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		_, _ = w.Write([]byte("173.245.48.0/20\n# comment\n\n2400:cb00::/32\n"))
	}))
	defer ts.Close()

	r := newTestResolver(t, nil)
	src := TrustedProxyDynamicSource{
		ID: "test-src", Name: "test", URL: ts.URL,
		Enabled: true, IntervalSeconds: 60, TimeoutSeconds: 5,
	}
	r.enabled.Store(true)
	srcs := []TrustedProxyDynamicSource{src}
	r.sources.Store(&srcs)

	r.fetchAndUpdate(context.Background(), src)

	// sourceCache 应被写入
	v, ok := r.sourceCache.Load("test-src")
	require.True(t, ok)
	list, ok := v.([]string)
	require.True(t, ok)
	require.Contains(t, list, "173.245.48.0/20")
	require.Contains(t, list, "2400:cb00::/32")

	// status 应记录成功 + count
	statuses := r.GetSourceStatuses()
	require.Len(t, statuses, 1)
	require.Equal(t, "test-src", statuses[0].ID)
	require.Empty(t, statuses[0].LastError)
	require.Equal(t, 2, statuses[0].CIDRCount)
	require.False(t, statuses[0].LastSuccessAt.IsZero())
}

// TestFetchAndUpdate_FailurePreservesCache 验证失败不清空上一次成功缓存。
func TestFetchAndUpdate_FailurePreservesCache(t *testing.T) {
	successCallCount := int32(0)
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		// 首次成功返回 CIDR，后续返回 500
		if atomic.AddInt32(&successCallCount, 1) == 1 {
			_, _ = w.Write([]byte("173.245.48.0/20\n"))
			return
		}
		http.Error(w, "boom", http.StatusInternalServerError)
	}))
	defer ts.Close()

	r := newTestResolver(t, nil)
	src := TrustedProxyDynamicSource{
		ID: "s1", URL: ts.URL, Enabled: true,
		IntervalSeconds: 60, TimeoutSeconds: 5,
	}
	r.enabled.Store(true)
	srcs := []TrustedProxyDynamicSource{src}
	r.sources.Store(&srcs)

	// 首次成功
	r.fetchAndUpdate(context.Background(), src)
	v1, _ := r.sourceCache.Load("s1")
	list1 := v1.([]string)
	require.Equal(t, []string{"173.245.48.0/20"}, list1)

	// 再次调用（此时上游返 500）
	r.fetchAndUpdate(context.Background(), src)
	v2, _ := r.sourceCache.Load("s1")
	list2 := v2.([]string)
	require.Equal(t, []string{"173.245.48.0/20"}, list2, "失败时应保留上一次成功缓存")

	statuses := r.GetSourceStatuses()
	require.NotEmpty(t, statuses[0].LastError, "失败应记录 last_error")
	require.Equal(t, 1, statuses[0].CIDRCount, "count 保留上次成功值")
}

// TestFetchSource_InvalidCIDRLines 验证畸形/注释行被跳过；纯 IP 自动补掩码。
func TestFetchSource_InvalidCIDRLines(t *testing.T) {
	body := `# 注释行
173.245.48.0/20
not-a-cidr
1.2.3.4
2606:4700::/32
`
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		_, _ = w.Write([]byte(body))
	}))
	defer ts.Close()

	r := newTestResolver(t, nil)
	src := TrustedProxyDynamicSource{ID: "s", URL: ts.URL, TimeoutSeconds: 5}
	list, err := r.fetchSource(context.Background(), src)
	require.NoError(t, err)
	require.Contains(t, list, "173.245.48.0/20")
	require.Contains(t, list, "1.2.3.4/32")
	require.Contains(t, list, "2606:4700::/32")
	require.NotContains(t, list, "not-a-cidr")
}

// TestFetchSource_TooLarge 验证超大响应会被拒绝。
func TestFetchSource_TooLarge(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		// 写 2MiB 数据
		big := strings.Repeat("1.2.3.4/32\n", (TrustedProxySourceMaxResponseBytes/11)+100)
		_, _ = w.Write([]byte(big))
	}))
	defer ts.Close()

	r := newTestResolver(t, nil)
	src := TrustedProxyDynamicSource{ID: "s", URL: ts.URL, TimeoutSeconds: 5}
	_, err := r.fetchSource(context.Background(), src)
	require.Error(t, err)
	require.Contains(t, err.Error(), "exceeds")
}

// TestReconfigure_StopOldStartNew 验证 Reconfigure 停旧 workers、按新配置起新 workers。
func TestReconfigure_StopOldStartNew(t *testing.T) {
	hitS1 := int32(0)
	hitS2 := int32(0)
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		switch req.URL.Path {
		case "/s1":
			atomic.AddInt32(&hitS1, 1)
			_, _ = w.Write([]byte("1.1.1.0/24\n"))
		case "/s2":
			atomic.AddInt32(&hitS2, 1)
			_, _ = w.Write([]byte("2.2.2.0/24\n"))
		default:
			http.NotFound(w, req)
		}
	}))
	defer ts.Close()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	r := newTestResolver(t, nil)
	r.Configure(true,
		[]TrustedProxyDynamicSource{{ID: "s1", URL: ts.URL + "/s1", Enabled: true, IntervalSeconds: 60, TimeoutSeconds: 5}},
		nil,
	)
	r.Start(ctx)

	// 首次拉取立即发生（异步 goroutine），等一小段时间
	require.Eventually(t, func() bool { return atomic.LoadInt32(&hitS1) >= 1 }, 2*time.Second, 20*time.Millisecond)

	// Reconfigure 切换到 s2
	r.Reconfigure(true,
		[]TrustedProxyDynamicSource{{ID: "s2", URL: ts.URL + "/s2", Enabled: true, IntervalSeconds: 60, TimeoutSeconds: 5}},
		nil,
	)
	require.Eventually(t, func() bool { return atomic.LoadInt32(&hitS2) >= 1 }, 2*time.Second, 20*time.Millisecond)

	// s1 cache 已被丢弃（sources 里不再有 s1 → mergedCIDRs 不再包含）
	merged := r.mergedCIDRs()
	require.Contains(t, merged, "2.2.2.0/24")
	require.NotContains(t, merged, "1.1.1.0/24", "Reconfigure 后旧 source 的 CIDR 不应再合并")
}

// TestReconfigure_DisableStopsWorkers 验证关闭总开关会停 workers 且不合并动态。
func TestReconfigure_DisableStopsWorkers(t *testing.T) {
	hit := int32(0)
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		atomic.AddInt32(&hit, 1)
		_, _ = w.Write([]byte("1.1.1.0/24\n"))
	}))
	defer ts.Close()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	r := newTestResolver(t, []string{"10.0.0.0/8"})
	r.Configure(true,
		[]TrustedProxyDynamicSource{{ID: "s1", URL: ts.URL, Enabled: true, IntervalSeconds: 60, TimeoutSeconds: 5}},
		nil,
	)
	r.Start(ctx)
	require.Eventually(t, func() bool { return atomic.LoadInt32(&hit) >= 1 }, 2*time.Second, 20*time.Millisecond)

	r.Reconfigure(false, r.getSources(), nil)
	// 关闭后不再合并 s1 cache
	merged := r.mergedCIDRs()
	require.Contains(t, merged, "10.0.0.0/8")
	require.NotContains(t, merged, "1.1.1.0/24")
}

func (r *TrustedProxyResolver) getSources() []TrustedProxyDynamicSource {
	return *r.sources.Load()
}

// TestParseTrustedProxyDynamicSources_Defaults 空/损坏值回退到内置默认。
func TestParseTrustedProxyDynamicSources_Defaults(t *testing.T) {
	def := DefaultTrustedProxyDynamicSources()
	require.Equal(t, def, ParseTrustedProxyDynamicSources(""))
	require.Equal(t, def, ParseTrustedProxyDynamicSources("{invalid json"))
	require.Equal(t, def, ParseTrustedProxyDynamicSources("[]"), "空数组也回退到默认")
}

// TestParseTrustedProxyDynamicSources_LenientBounds 越界数值走默认。
func TestParseTrustedProxyDynamicSources_LenientBounds(t *testing.T) {
	raw := `[{"id":"a","name":"A","url":"http://x","enabled":true,"interval_seconds":10,"timeout_seconds":9999}]`
	got := ParseTrustedProxyDynamicSources(raw)
	require.Len(t, got, 1)
	require.Equal(t, TrustedProxyIntervalSecondsDefault, got[0].IntervalSeconds)
	require.Equal(t, TrustedProxyTimeoutSecondsDefault, got[0].TimeoutSeconds)
}

// TestNormalizeTrustedProxyDynamicSources_Reject 校验各种非法输入。
func TestNormalizeTrustedProxyDynamicSources_Reject(t *testing.T) {
	cases := []struct {
		name    string
		input   []TrustedProxyDynamicSource
		wantSub string
	}{
		{"empty id", []TrustedProxyDynamicSource{{URL: "http://x", IntervalSeconds: 60, TimeoutSeconds: 30}}, "id must not be empty"},
		{"empty url", []TrustedProxyDynamicSource{{ID: "a", IntervalSeconds: 60, TimeoutSeconds: 30}}, "url must not be empty"},
		{"bad scheme", []TrustedProxyDynamicSource{{ID: "a", URL: "ftp://x", IntervalSeconds: 60, TimeoutSeconds: 30}}, "http or https"},
		{"interval too small", []TrustedProxyDynamicSource{{ID: "a", URL: "http://x", IntervalSeconds: 10, TimeoutSeconds: 30}}, "interval_seconds"},
		{"timeout too big", []TrustedProxyDynamicSource{{ID: "a", URL: "http://x", IntervalSeconds: 60, TimeoutSeconds: 9999}}, "timeout_seconds"},
		{"duplicate id", []TrustedProxyDynamicSource{
			{ID: "a", URL: "http://x1", IntervalSeconds: 60, TimeoutSeconds: 30},
			{ID: "a", URL: "http://x2", IntervalSeconds: 60, TimeoutSeconds: 30},
		}, "duplicate"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := NormalizeTrustedProxyDynamicSources(tc.input)
			require.Error(t, err)
			require.Contains(t, err.Error(), tc.wantSub)
		})
	}
}

// TestNormalizeTrustedProxyDynamicExtraCIDRs_Reject 校验 extra_cidrs 非法。
func TestNormalizeTrustedProxyDynamicExtraCIDRs_Reject(t *testing.T) {
	_, err := NormalizeTrustedProxyDynamicExtraCIDRs([]string{"not-a-cidr"})
	require.Error(t, err)
	require.Contains(t, err.Error(), "not-a-cidr")
}

// TestNormalizeTrustedProxyDynamicExtraCIDRs_NormalizeAndDedupe 合法输入去重规范化。
func TestNormalizeTrustedProxyDynamicExtraCIDRs_NormalizeAndDedupe(t *testing.T) {
	got, err := NormalizeTrustedProxyDynamicExtraCIDRs([]string{"1.2.3.4", "1.2.3.4/32", "10.0.0.0/8", "10.0.0.0/8"})
	require.NoError(t, err)
	require.Equal(t, []string{"1.2.3.4/32", "10.0.0.0/8"}, got)
}

// TestGetStaticTrustedProxies_ProviderNotSet 未注入 provider 时返回空切片。
func TestGetStaticTrustedProxies_ProviderNotSet(t *testing.T) {
	prev := trustedProxySnapshotCB.Load()
	SetTrustedProxySnapshotProvider(nil)
	t.Cleanup(func() {
		if prev != nil && *prev != nil {
			SetTrustedProxySnapshotProvider(*prev)
		}
	})
	require.Empty(t, GetStaticTrustedProxies())
	require.Empty(t, GetTrustedProxySourceStatuses())
}

// TestResolverImplementsProvider 保证 resolver 满足 TrustedProxySnapshotProvider。
func TestResolverImplementsProvider(t *testing.T) {
	r := newTestResolver(t, []string{"10.0.0.0/8"})
	var p TrustedProxySnapshotProvider = r
	SetTrustedProxySnapshotProvider(p)
	t.Cleanup(func() { SetTrustedProxySnapshotProvider(nil) })
	require.Equal(t, []string{"10.0.0.0/8"}, GetStaticTrustedProxies())
	require.Empty(t, GetTrustedProxySourceStatuses())
	// 断言接口不会 panic
	require.NotPanics(t, func() { _ = fmt.Sprint(GetStaticTrustedProxies()) })
}
