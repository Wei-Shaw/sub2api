package service

import (
	"context"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/openai"
	"github.com/stretchr/testify/require"
)

// --- mock: 只记录临时不可调度写入，其余方法不应被调用 ---

type capacityShedAccountRepoStub struct {
	AccountRepository // 嵌入接口，未实现的方法会 panic（不应被调用）

	tempUnschedCalls int
}

func (r *capacityShedAccountRepoStub) SetTempUnschedulable(_ context.Context, _ int64, _ time.Time, _ string) error {
	r.tempUnschedCalls++
	return nil
}

// 上游容量降载是请求级信号：故障因素（客户端身份、模型容量）与账号无关，
// 同账号重试用尽后不得把账号临时摘掉——否则一个被降载的请求会顺着 failover
// 把整池账号逐个封禁，而每个账号都会以同一个错误失败。
func TestTempUnscheduleRetryableErrorSkipsRequestScopedTransient(t *testing.T) {
	t.Run("请求级瞬时故障不写账号状态", func(t *testing.T) {
		repo := &capacityShedAccountRepoStub{}
		svc := &GatewayService{accountRepo: repo}

		svc.TempUnscheduleRetryableError(context.Background(), 1, &UpstreamFailoverError{
			StatusCode:             http.StatusBadGateway,
			RetryableOnSameAccount: true,
			RequestScopedTransient: true,
		})

		require.Zero(t, repo.tempUnschedCalls)
	})

	// 对照组：同样的 502 在未标记请求级瞬时故障时仍按原有语义临时摘号，
	// 确认上面的断言来自新增守卫而非其他前置条件。
	t.Run("未标记时保持原有临时摘号语义", func(t *testing.T) {
		repo := &capacityShedAccountRepoStub{}
		svc := &GatewayService{accountRepo: repo}

		svc.TempUnscheduleRetryableError(context.Background(), 1, &UpstreamFailoverError{
			StatusCode:             http.StatusBadGateway,
			RetryableOnSameAccount: true,
		})

		require.Equal(t, 1, repo.tempUnschedCalls)
	})
}

// 非池模式账号同样要先在同账号重试：换号不改变降载因素。
func TestStreamFailedEventCapacityShedRetriesOnSameAccount(t *testing.T) {
	nonPool := &Account{ID: 1, Platform: PlatformOpenAI, Type: AccountTypeOAuth}

	for _, code := range []string{"server_is_overloaded", "slow_down"} {
		payload := []byte(`{"type":"response.failed","response":{"error":{"code":"` + code + `"}}}`)
		require.True(t, isOpenAIUpstreamCapacityShedEvent(payload), code)
		require.True(t, openAIStreamFailedEventRetryableOnSameAccount(nonPool, payload, "overloaded"), code)
	}

	// 非降载的 failed 事件在非池模式下仍不做同账号重试，避免放大改动面。
	other := []byte(`{"type":"response.failed","response":{"error":{"code":"server_error"}}}`)
	require.False(t, isOpenAIUpstreamCapacityShedEvent(other))
	require.False(t, openAIStreamFailedEventRetryableOnSameAccount(nonPool, other, "boom"))
}

// 出站身份的版本声明只能有一个来源：UA 的版本段、version 头、探针版本三处必须同源，
// 各自硬编码会漂移成互相矛盾的身份，而自相矛盾或陈旧的身份会被上游优先降载。
func TestCodexOutboundVersionHasSingleSource(t *testing.T) {
	require.True(t,
		strings.HasPrefix(codexCLIUserAgent, openai.CodexDefaultOriginator+"/"+codexCLIVersion+" "),
		"codexCLIUserAgent=%q 必须以 codexCLIVersion=%q 作为版本段", codexCLIUserAgent, codexCLIVersion,
	)
	require.Equal(t, codexCLIVersion, openAICodexProbeVersion)
	require.GreaterOrEqual(t, CompareVersions(codexCLIVersion, codexUpstreamMinVersion), 0,
		"codexCLIVersion=%q 不得低于上游最低门槛 %q", codexCLIVersion, codexUpstreamMinVersion,
	)
}
