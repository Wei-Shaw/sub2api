package handler

import (
	"context"
	"errors"
	"fmt"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/service"
	coderws "github.com/coder/websocket"
	"github.com/stretchr/testify/require"
)

// issue #6105：入站 Responses WebSocket 的正常结束会被记成账号故障。
//
// 归因发生在 openai_gateway_handler.go 的 ingress 收尾处：只有
// *service.OpenAIWSClientCloseError 且状态码为 1000 被认作正常关闭，其余一律落到
// shouldReportOpenAIWSProxyAccountFailure —— 而它只排除 model-switch 与
// session-preempted 两种。于是客户端干净关闭（底层直接回裸 coderws.CloseError{1000}）
// 与客户端中途断开（context.Canceled，收尾用 1001 关闭）都会喂给
// ObserveOpenAIAPIKeyHealthFailure 与 scheduler.ReportResult(success=false)，
// 累积到阈值即把上游账号熔断出调度池。
//
// 这些用例钉住判定本身，与既有的 TestShouldReportOpenAIWSProxyAccountFailure 同一层级：
// 调用点位于一个需要真实上游 WS 才能进入的巨型 handler 循环内，仓库既有约定就是直接测判定函数。

// 缺陷主复现之一：客户端干净关闭。底层 conn.Read 的错误被 ReadOpenAIWSClientMessage
// 原样返回，没有任何地方把它包成 *OpenAIWSClientCloseError，所以旧断言看不见它。
func TestOpenAIWSIngressEndedByClient_BareNormalClosureIsNotAccountFailure(t *testing.T) {
	err := coderws.CloseError{Code: coderws.StatusNormalClosure, Reason: "client done"}

	// 前提：这正是旧判据漏掉它的原因——类型不匹配，不是状态码不匹配。
	var closeErr *service.OpenAIWSClientCloseError
	require.False(t, errors.As(err, &closeErr),
		"裸 coderws.CloseError 不是 *OpenAIWSClientCloseError，旧的 errors.As 必然为假")
	// 而按关闭码读，它确实是 1000。
	require.Equal(t, coderws.StatusNormalClosure, coderws.CloseStatus(err))

	require.True(t, openAIWSIngressEndedByClient(err))
}

// 同一形状被包一层（例如 ingress 把 read 错误裹进上下文）时也必须认得。
func TestOpenAIWSIngressEndedByClient_WrappedBareNormalClosureIsNotAccountFailure(t *testing.T) {
	err := fmt.Errorf("ingress turn 3: %w",
		coderws.CloseError{Code: coderws.StatusNormalClosure, Reason: "client done"})

	require.True(t, openAIWSIngressEndedByClient(err))
}

// 缺陷主复现之二：客户端中途断开。ReadOpenAIWSClientMessage 在 controlCtx.Done()
// 分支用 StatusGoingAway 收尾并把 context.Canceled 作为 cause，所以「只认 1000」
// 这一条判据根本匹配不到它。
func TestOpenAIWSIngressEndedByClient_ClientCancelDuringTurnIsNotAccountFailure(t *testing.T) {
	err := service.NewOpenAIWSClientCloseError(
		coderws.StatusGoingAway, "websocket request canceled", context.Canceled)

	// 前提：状态码是 1001 不是 1000，旧判据必然放行到账号归因。
	var closeErr *service.OpenAIWSClientCloseError
	require.ErrorAs(t, err, &closeErr)
	require.Equal(t, coderws.StatusGoingAway, closeErr.StatusCode())
	require.NotEqual(t, coderws.StatusNormalClosure, closeErr.StatusCode())

	require.True(t, openAIWSIngressEndedByClient(err))
}

// 既有行为不得回退：网关自己用 1000 收尾（inter-turn idle timeout 就是这条）
// 原本就被认作正常关闭。
func TestOpenAIWSIngressEndedByClient_GatewayNormalClosureStillRecognised(t *testing.T) {
	err := service.NewOpenAIWSClientCloseError(
		coderws.StatusNormalClosure, "websocket idle timeout", context.DeadlineExceeded)

	require.True(t, openAIWSIngressEndedByClient(err))
}

// 收窄证明：1001 本身不足以豁免。网关也会因自身原因用 GoingAway 收场，
// 客户端取消那一支已由 context.Canceled 覆盖，无需整类放行。
func TestOpenAIWSIngressEndedByClient_GoingAwayWithoutCancellationStillReported(t *testing.T) {
	err := service.NewOpenAIWSClientCloseError(
		coderws.StatusGoingAway, "upstream going away", errors.New("upstream closed session"))

	require.False(t, openAIWSIngressEndedByClient(err))
	require.True(t, shouldReportOpenAIWSProxyAccountFailure(err), "真实上游故障仍须归因账号")
}

// 契约没有丢：真正的故障仍然惩罚账号。判定组合与调用点一致——
// openAIWSIngressEndedByClient 为假才会走到 shouldReportOpenAIWSProxyAccountFailure。
func TestOpenAIWSIngressEndedByClient_AbnormalClosuresStillReportAccountFailure(t *testing.T) {
	cases := []struct {
		name string
		err  error
	}{
		{
			name: "upstream_policy_violation",
			err: service.NewOpenAIWSClientCloseError(
				coderws.StatusPolicyViolation, "upstream websocket authentication failed",
				errors.New("upstream rejected credentials")),
		},
		{
			name: "upstream_internal_error",
			err: service.NewOpenAIWSClientCloseError(
				coderws.StatusInternalError, "upstream websocket proxy failed", nil),
		},
		{
			name: "bare_abnormal_closure",
			err:  coderws.CloseError{Code: coderws.StatusAbnormalClosure, Reason: "connection reset"},
		},
		{
			name: "generic_read_failure",
			err:  errors.New("upstream websocket read failed"),
		},
		{
			// 空闲超时之外的 deadline 是真实停滞：不豁免。
			name: "deadline_without_normal_close",
			err:  fmt.Errorf("upstream stalled: %w", context.DeadlineExceeded),
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			require.False(t, openAIWSIngressEndedByClient(tc.err))
			require.True(t, shouldReportOpenAIWSProxyAccountFailure(tc.err))
		})
	}
}

// 不变式：同一条错误，日志侧与归因侧必须给出一致的结论。
// summarizeWSCloseErrorForLog 一直用 coderws.CloseStatus 读关闭码，这正是缺陷时期
// WARN 打印 close_status=1000(StatusNormalClosure) 却同时把账号记为故障的原因。
// 以后任何一侧改了读法，这条会红。
func TestOpenAIWSIngressEndedByClient_MatchesCloseCodeReportedInLog(t *testing.T) {
	errs := []error{
		coderws.CloseError{Code: coderws.StatusNormalClosure, Reason: "client done"},
		fmt.Errorf("ingress turn 3: %w", coderws.CloseError{Code: coderws.StatusNormalClosure}),
		service.NewOpenAIWSClientCloseError(coderws.StatusNormalClosure, "websocket idle timeout", context.DeadlineExceeded),
		service.NewOpenAIWSClientCloseError(coderws.StatusGoingAway, "websocket request canceled", context.Canceled),
		coderws.CloseError{Code: coderws.StatusAbnormalClosure, Reason: "connection reset"},
		errors.New("upstream websocket read failed"),
	}

	for _, err := range errs {
		t.Run(err.Error(), func(t *testing.T) {
			closeStatus, _ := summarizeWSCloseErrorForLog(err)
			if closeStatus == "1000(StatusNormalClosure)" {
				require.True(t, openAIWSIngressEndedByClient(err),
					"日志按 1000 归类为正常关闭，归因侧不得同时判为账号故障")
			}
		})
	}
}
