package handler

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

// composite 组 inbound=/v1/chat/completions、实际出站 /v1/messages：显式记录的
// actual endpoint 必须优先于 group/platform 推导；无记录时 composite 缺
// DeriveUpstreamEndpoint case，才回退入站（仅兜底，不代表真实出站）。
func TestGetUpstreamEndpointPrefersActualOutboundRecordOverCompositeFallback(t *testing.T) {
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodPost, EndpointChatCompletions, nil)
	c.Set(ctxKeyInboundEndpoint, EndpointChatCompletions)

	// 无任何实际出站记录：回退入站推导。
	require.Equal(t, EndpointChatCompletions, GetUpstreamEndpoint(c, service.PlatformComposite))

	// generic forwarder 发送前记录真实出站 /v1/messages 后必须原样返回，
	// 且不因传入平台不同而被推导值覆盖。
	service.SetOpsUpstreamEndpoint(c, EndpointMessages)
	require.Equal(t, EndpointMessages, GetUpstreamEndpoint(c, service.PlatformComposite))
	require.Equal(t, EndpointMessages, GetUpstreamEndpoint(c, service.PlatformAnthropic))
	require.Equal(t, EndpointMessages, GetUpstreamEndpoint(c, service.PlatformOpenAI))

	// 实际出站就是 CC 时保持 CC，不得被平台推导改写。
	service.SetOpsUpstreamEndpoint(c, EndpointChatCompletions)
	require.Equal(t, EndpointChatCompletions, GetUpstreamEndpoint(c, service.PlatformComposite))
}

// composite 组经 OpenAI handler 转发兼容池账号：actualOpenAI 运行时记录对
// composite 平台同样要生效，不能漏读后回退到入站推导。
func TestGetUpstreamEndpointCompositeReadsOpenAIRuntimeOverride(t *testing.T) {
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodPost, EndpointChatCompletions, nil)
	c.Set(ctxKeyInboundEndpoint, EndpointChatCompletions)

	service.SetActualOpenAIUpstreamEndpoint(c, EndpointChatCompletions)
	require.Equal(t, EndpointChatCompletions, GetUpstreamEndpoint(c, service.PlatformComposite))

	service.SetActualOpenAIUpstreamEndpoint(c, EndpointResponses)
	require.Equal(t, EndpointResponses, GetUpstreamEndpoint(c, service.PlatformComposite))

	// generic 记录优先级高于 OpenAI 运行时记录（同为最近一次出站尝试时，
	// generic 记录来自更底层发送点）。
	service.SetOpsUpstreamEndpoint(c, EndpointMessages)
	require.Equal(t, EndpointMessages, GetUpstreamEndpoint(c, service.PlatformComposite))
}

// 每次账号重选（含 failover 重试）后，上一账号的尝试级元数据不得残留，
// 否则失败日志会把旧账号的端点/模型安到新账号头上。
func TestSetOpsSelectedAccountResetsAttemptScopedMetadata(t *testing.T) {
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodPost, EndpointMessages, nil)

	service.SetOpsUpstreamEndpoint(c, EndpointMessages)
	service.SetActualOpenAIUpstreamEndpoint(c, EndpointResponses)
	service.SetOpsUpstreamModel(c, "gpt-5.2")
	service.SetOpsUpstreamEndpoint(c, "/v1/v1/messages")

	setOpsSelectedAccount(c, 42, service.PlatformAnthropic)

	require.Empty(t, service.GetOpsUpstreamEndpoint(c))
	require.Empty(t, service.GetActualOpenAIUpstreamEndpoint(c))
	require.Empty(t, c.GetString(opsUpstreamModelKey))
	require.Equal(t, int64(42), c.GetInt64(opsAccountIDKey))
}

// GetUpstreamEndpoint 旧行为保持：无记录时 anthropic 推导 /v1/messages、
// antigravity handler 记录优先于推导。
func TestGetUpstreamEndpointFallbackBehaviorUnchanged(t *testing.T) {
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodPost, EndpointChatCompletions, nil)
	c.Set(ctxKeyInboundEndpoint, EndpointChatCompletions)

	require.Equal(t, EndpointMessages, GetUpstreamEndpoint(c, service.PlatformAnthropic))

	setActualUpstreamEndpoint(c, EndpointAntigravityGenerateContent)
	require.Equal(t, EndpointAntigravityGenerateContent, GetUpstreamEndpoint(c, service.PlatformAntigravity))
}
