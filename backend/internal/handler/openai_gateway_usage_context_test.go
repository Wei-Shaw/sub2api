package handler

import (
	"context"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/pkg/ctxkey"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/stretchr/testify/require"
)

func TestSubmitUsageRecordTaskCopiesRequestContext(t *testing.T) {
	parent := context.WithValue(context.Background(), ctxkey.ClientRequestID, "client-request-123")
	parent = context.WithValue(parent, ctxkey.RequestID, "request-456")

	var gotClientRequestID string
	var gotRequestID string
	h := &GatewayHandler{}
	h.submitUsageRecordTask(parent, func(ctx context.Context) {
		gotClientRequestID, _ = ctx.Value(ctxkey.ClientRequestID).(string)
		gotRequestID, _ = ctx.Value(ctxkey.RequestID).(string)
	})

	require.Equal(t, "client-request-123", gotClientRequestID)
	require.Equal(t, "request-456", gotRequestID)
}

func TestOpenAISubmitUsageRecordTaskCopiesRequestContext(t *testing.T) {
	parent := context.WithValue(context.Background(), ctxkey.ClientRequestID, "openai-client-request-123")
	parent = context.WithValue(parent, ctxkey.RequestID, "openai-request-456")

	var gotClientRequestID string
	var gotRequestID string
	h := &OpenAIGatewayHandler{}
	h.submitUsageRecordTask(parent, func(ctx context.Context) {
		gotClientRequestID, _ = ctx.Value(ctxkey.ClientRequestID).(string)
		gotRequestID, _ = ctx.Value(ctxkey.RequestID).(string)
	})

	require.Equal(t, "openai-client-request-123", gotClientRequestID)
	require.Equal(t, "openai-request-456", gotRequestID)
}

// composite 分组的后扣运行在 worker 池的 background ctx 上。
// 目标平台必须随任务带过去，否则账号统计规则按 "composite" 匹配不到任何条目。
func TestOpenAISubmitUsageRecordTaskCarriesResolvedTargetPlatform(t *testing.T) {
	parent := service.WithResolvedTargetPlatform(context.Background(), service.PlatformAnthropic)

	var gotPlatform string
	var gotOK bool
	h := &OpenAIGatewayHandler{}
	h.submitUsageRecordTask(parent, func(ctx context.Context) {
		gotPlatform, gotOK = service.ResolvedTargetPlatformFromContext(ctx)
	})

	require.True(t, gotOK)
	require.Equal(t, service.PlatformAnthropic, gotPlatform)
}

// 强制平台路由（/antigravity）的目标平台只存在于 ForcePlatform。
// 它同样必须随后扣任务带走，否则计费会退回分组平台。
func TestOpenAISubmitUsageRecordTaskCarriesForcePlatform(t *testing.T) {
	parent := context.WithValue(context.Background(), ctxkey.ForcePlatform, service.PlatformAntigravity)

	var gotPlatform string
	h := &OpenAIGatewayHandler{}
	h.submitUsageRecordTask(parent, func(ctx context.Context) {
		gotPlatform, _ = ctx.Value(ctxkey.ForcePlatform).(string)
	})

	require.Equal(t, service.PlatformAntigravity, gotPlatform)
}
