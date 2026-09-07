//go:build unit

package service

import (
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/pkg/kiro"
	"github.com/stretchr/testify/require"
)

func TestDecideKiroActionMatrix(t *testing.T) {
	cases := []struct {
		name             string
		sig              kiro.Signal
		sawContent       bool
		alreadyRefreshed bool
		hasMoreEndpoints bool
		want             kiroAction
	}{
		{"ok", kiro.SignalOK, false, false, true, kiroActionProceed},

		{"auth first time", kiro.SignalAuthExpired, false, false, true, kiroActionRefreshAndRetry},
		{"auth after refresh", kiro.SignalAuthExpired, false, true, true, kiroActionFailoverAccount},

		{"429 with endpoints left", kiro.SignalRateLimited, false, false, true, kiroActionNextEndpoint},
		{"429 endpoints exhausted", kiro.SignalRateLimited, false, false, false, kiroActionFailoverAccount},

		{"credits exhausted", kiro.SignalCreditsExhausted, false, false, true, kiroActionFailoverAccount},

		// C3：Suspended/Overage 是账号订阅/配置问题，换账号大概率能正常
		// 服务同一个请求（kiro.Signal.Failoverable() 对这两者恒为
		// true——见其注释里的 Ruling I5），之前这里错误地返回 Abort，
		// 导致有问题的账号永远留在池子里、从不自愈。
		{"overage", kiro.SignalOverage, false, false, true, kiroActionFailoverAccount},
		{"suspended", kiro.SignalSuspended, false, false, true, kiroActionFailoverAccount},

		{"unknown with endpoints", kiro.SignalUnknown, false, false, true, kiroActionNextEndpoint},
		{"unknown exhausted", kiro.SignalUnknown, false, false, false, kiroActionFailoverAccount},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			got := decideKiroAction(tc.sig, tc.sawContent, tc.alreadyRefreshed, tc.hasMoreEndpoints)
			require.Equal(t, tc.want, got, "signal=%s", tc.sig)
		})
	}
}

// TestDecideKiroActionInvalidModelIDNeverFailsOver 是红线一：
// INVALID_MODEL_ID 是网络/区域问题（大陆直连必现），不是账号的错。
// 若触发账号转移，首个请求就会把整个账号池轮一遍并全部标记失败。
func TestDecideKiroActionInvalidModelIDNeverFailsOver(t *testing.T) {
	// 还有端点时可以换端点试试。
	require.Equal(t, kiroActionNextEndpoint,
		decideKiroAction(kiro.SignalNetworkRegion, false, false, true))

	// 端点耗尽后必须中止，绝不能转移账号。
	got := decideKiroAction(kiro.SignalNetworkRegion, false, false, false)
	require.Equal(t, kiroActionAbort, got)
	require.NotEqual(t, kiroActionFailoverAccount, got,
		"网络问题换账号解决不了，只会把整池账号标记失败")
}

// TestDecideKiroActionBadRequestNeverRetriesOrFailsOver 是红线二：
// 400 说明我们自己的 schema 清洗或角色规整有误，换账号一样失败。
func TestDecideKiroActionBadRequestNeverRetriesOrFailsOver(t *testing.T) {
	for _, hasMore := range []bool{true, false} {
		for _, refreshed := range []bool{true, false} {
			got := decideKiroAction(kiro.SignalBadRequest, false, refreshed, hasMore)
			require.Equal(t, kiroActionAbort, got,
				"400 在任何组合下都必须中止（hasMore=%v refreshed=%v）", hasMore, refreshed)
		}
	}
}

// TestDecideKiroActionSawContentAlwaysAborts 覆盖「已出字节不可重试」：
// 客户端已经收到部分内容，任何重试都会产生重复输出。Suspended/Overage 在
// C3 之后改成默认 Failoverable，这里显式覆盖它们——函数最上面的
// sawContent 强制 Abort 检查必须仍然对这两个信号生效（不因为 C3 而失守）。
func TestDecideKiroActionSawContentAlwaysAborts(t *testing.T) {
	signals := []kiro.Signal{
		kiro.SignalAuthExpired, kiro.SignalRateLimited, kiro.SignalUnknown,
		kiro.SignalCreditsExhausted, kiro.SignalNetworkRegion,
		kiro.SignalSuspended, kiro.SignalOverage,
	}
	for _, sig := range signals {
		require.Equal(t, kiroActionAbort,
			decideKiroAction(sig, true, false, true),
			"signal=%s 在已出内容后必须中止", sig)
	}
}

func TestKiroActionStringIsStable(t *testing.T) {
	// 这些字符串进日志与告警，改动会破坏既有检索。
	require.Equal(t, "proceed", kiroActionProceed.String())
	require.Equal(t, "refresh_and_retry", kiroActionRefreshAndRetry.String())
	require.Equal(t, "next_endpoint", kiroActionNextEndpoint.String())
	require.Equal(t, "failover_account", kiroActionFailoverAccount.String())
	require.Equal(t, "abort", kiroActionAbort.String())
}
