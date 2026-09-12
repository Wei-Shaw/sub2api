//go:build unit

package service

// 请求期 ollama_cloud 定价预检的回归测试（上游 I/O 之前 400）。
//
// 最重要的验收断言是「上游零调用 + 不 failover / 不写账号处置」：未定价请求必须在
// 构造上游请求之前被终止，客户端拿到 400 invalid_request_error，而不是像旧的异步
// 阶段拒绝那样——上游已被调用、响应已写给客户端之后才在记账阶段丢弃 usage。

import (
	"bytes"
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/pkg/ctxkey"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

// newPricingPreflightTestService 构造带真实定价解析链（fallbackPrices 价卡 +
// 可选渠道定价）的网关服务，供预检测试复用。
func newPricingPreflightTestService(upstream *httpUpstreamRecorder, groupID int64, channelPricings []ChannelModelPricing) *OpenAIGatewayService {
	bs := NewBillingService(nil, nil)
	svc := &OpenAIGatewayService{
		cfg:            rawChatCompletionsTestConfig(),
		httpUpstream:   upstream,
		billingService: bs,
		resolver:       NewModelPricingResolver(nil, bs),
	}
	if len(channelPricings) > 0 {
		svc.channelService = newChannelServiceWithPricings(groupID, channelPricings)
		svc.resolver = NewModelPricingResolver(svc.channelService, bs)
	}
	return svc
}

// pricingPreflightGroup 构造通过 IsGroupContextValid 的认证分组（ctxkey.Group 载体）。
func pricingPreflightGroup(id int64, modelPricing ...ChannelModelPricing) *Group {
	return &Group{
		ID:           id,
		Platform:     PlatformOllamaCloud,
		Status:       StatusActive,
		Hydrated:     true,
		ModelPricing: modelPricing,
	}
}

func pricingPreflightContext(path string, body []byte, group *Group) (*gin.Context, *httptest.ResponseRecorder, context.Context) {
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPost, path, bytes.NewReader(body))
	c.Request.Header.Set("Content-Type", "application/json")
	ctx := context.Background()
	if group != nil {
		ctx = context.WithValue(ctx, ctxkey.Group, group)
	}
	return c, rec, ctx
}

// assertPricingPreflightRejected 汇总预检拒绝路径的共同断言：上游零调用（最重要）、
// 客户端可见 400 invalid_request_error、响应已提交标记（handler 不追加 fallback）、
// 返回类型化错误且非 UpstreamFailoverError（不换号、不写账号处置）。
func assertPricingPreflightRejected(t *testing.T, c *gin.Context, upstream *httpUpstreamRecorder, rec *httptest.ResponseRecorder, err error, wantModel string) {
	t.Helper()

	require.Nil(t, upstream.lastReq, "未定价请求不得发起任何上游调用（核心验收断言）")
	require.Nil(t, upstream.lastBody)
	require.Equal(t, http.StatusBadRequest, rec.Code, "client-visible 状态码必须是 400")
	require.Equal(t, "invalid_request_error", gjson.Get(rec.Body.String(), "error.type").String())
	require.Contains(t, rec.Body.String(), wantModel, "错误体必须点名未定价模型")
	require.True(t, IsResponseCommitted(c), "响应已提交标记必须置位（handler 不再追加 fallback 错误）")

	var unpricedErr *openAIOllamaCloudUnpricedModelError
	require.ErrorAs(t, err, &unpricedErr, "必须是预检类型化错误")
	var failoverErr *UpstreamFailoverError
	require.False(t, errors.As(err, &failoverErr), "预检错误不得是 failover 错误")
}

// TestOllamaCloudPricingPreflight_RejectsUnpricedBeforeUpstream 主验收：原生
// Responses 出站（buildUpstreamRequest 挂点）上，未定价模型在发起上游请求之前被
// 400 终止。
func TestOllamaCloudPricingPreflight_RejectsUnpricedBeforeUpstream(t *testing.T) {
	gin.SetMode(gin.TestMode)

	upstream := &httpUpstreamRecorder{err: errors.New("must not reach upstream")}
	svc := newPricingPreflightTestService(upstream, 0, nil)
	account := adaptiveProtocolTestAccount(PlatformOllamaCloud, nil)

	body := []byte(`{"model":"totally-unpriced-model","input":"hi","stream":false}`)
	c, rec, ctx := pricingPreflightContext("/v1/responses", body, nil)
	_, err := svc.Forward(ctx, c, account, body)
	assertPricingPreflightRejected(t, c, upstream, rec, err, "totally-unpriced-model")
}

// TestOllamaCloudPricingPreflight_BasePricedModelPasses 基础价（fallbackPrices 命中）
// 的模型必须正常出站——预检只拒无价可循的名字。
func TestOllamaCloudPricingPreflight_BasePricedModelPasses(t *testing.T) {
	gin.SetMode(gin.TestMode)

	upstream := &httpUpstreamRecorder{err: errors.New("stop after capture")}
	svc := newPricingPreflightTestService(upstream, 0, nil)
	account := adaptiveProtocolTestAccount(PlatformOllamaCloud, nil)

	body := []byte(`{"model":"gpt-oss:120b-cloud","input":"hi","stream":false}`)
	c, rec, ctx := pricingPreflightContext("/v1/responses", body, nil)
	_, err := svc.Forward(ctx, c, account, body)
	require.NotNil(t, upstream.lastReq, "已定价模型不得被预检拦截")
	require.Equal(t, "https://ollama.com/v1/responses", upstream.lastReq.URL.String())
	require.NotEqual(t, http.StatusBadRequest, rec.Code)
	require.Error(t, err) // upstream 被Recorder 短路，错误来自传输层，与预检无关
}

// TestOllamaCloudPricingPreflight_ChannelOnlyPricedModelPasses 「判定与计费同源」的
// 关键用例：模型只在渠道定价里配置、不在基础价里 → 计费能算出成本，预检不得拒绝。
func TestOllamaCloudPricingPreflight_ChannelOnlyPricedModelPasses(t *testing.T) {
	gin.SetMode(gin.TestMode)

	const groupID = int64(777)
	upstream := &httpUpstreamRecorder{err: errors.New("stop after capture")}
	svc := newPricingPreflightTestService(upstream, groupID, []ChannelModelPricing{
		tokenPricingForModels([]string{"channel-only-model"}, 3.0),
	})
	account := adaptiveProtocolTestAccount(PlatformOllamaCloud, nil)

	body := []byte(`{"model":"channel-only-model","input":"hi","stream":false}`)
	c, rec, ctx := pricingPreflightContext("/v1/responses", body, pricingPreflightGroup(groupID))
	_, err := svc.Forward(ctx, c, account, body)
	require.NotNil(t, upstream.lastReq, "只有渠道价的模型不得被预检误拒（预检不得比计费更严格）")
	require.Equal(t, "channel-only-model", gjson.GetBytes(upstream.lastBody, "model").String())
	require.NotEqual(t, http.StatusBadRequest, rec.Code)
	require.Error(t, err) // upstream 短路错误，与预检无关
}

// TestOllamaCloudModelUnpricedForBilling_MatchesBillingChain 钉住判定函数与计费
// 解析链（Group → Channel → LiteLLM → Fallback）的同源矩阵。
func TestOllamaCloudModelUnpricedForBilling_MatchesBillingChain(t *testing.T) {
	const groupID = int64(778)
	bs := NewBillingService(nil, nil)
	channelSvc := newChannelServiceWithPricings(groupID, []ChannelModelPricing{
		tokenPricingForModels([]string{"channel-only-model"}, 3.0),
		{
			Platform:        PlatformOpenAI,
			Models:          []string{"per-request-only-model"},
			BillingMode:     BillingModePerRequest,
			PerRequestPrice: f64p(0.5),
		},
	})
	svc := &OpenAIGatewayService{
		billingService: bs,
		channelService: channelSvc,
		resolver:       NewModelPricingResolver(channelSvc, bs),
	}
	// resolver 缺失（直接构造 service 的场景）恒放行。
	require.False(t, (&OpenAIGatewayService{}).ollamaCloudModelUnpricedForBilling(context.Background(), "totally-unpriced-model"))

	withGroup := context.WithValue(context.Background(), ctxkey.Group, pricingPreflightGroup(groupID))

	// 基础价命中（fallbackPrices ollama 价卡，含 :tag 两级 fallback）。
	require.False(t, svc.ollamaCloudModelUnpricedForBilling(context.Background(), "gpt-oss:120b-cloud"))
	// 全链未命中。
	require.True(t, svc.ollamaCloudModelUnpricedForBilling(context.Background(), "totally-unpriced-model"))
	// 只有渠道价（token 模式）：带认证分组 → 有价；不带分组 → 计费同样看不到渠道价。
	require.False(t, svc.ollamaCloudModelUnpricedForBilling(withGroup, "channel-only-model"))
	require.True(t, svc.ollamaCloudModelUnpricedForBilling(context.Background(), "channel-only-model"))
	// 渠道按次模式：calculatePerRequestCost 从不因缺价报错 → 永不判无价。
	require.False(t, svc.ollamaCloudModelUnpricedForBilling(withGroup, "per-request-only-model"))
	// 分组价卡命中。
	groupWithPricing := pricingPreflightGroup(groupID, tokenPricingForModels([]string{"group-priced-model"}, 2.0))
	groupCtx := context.WithValue(context.Background(), ctxkey.Group, groupWithPricing)
	require.False(t, svc.ollamaCloudModelUnpricedForBilling(groupCtx, "group-priced-model"))
}

// TestOllamaCloudPricingPreflight_RawCCPathRejectsUnpriced raw CC 出站挂点
// （sendCCUpstreamRequest）：显式 api_protocol=chat_completions 的 ollama 账号，
// 未定价模型在发送前 400、上游零调用。
func TestOllamaCloudPricingPreflight_RawCCPathRejectsUnpriced(t *testing.T) {
	gin.SetMode(gin.TestMode)

	upstream := &httpUpstreamRecorder{err: errors.New("must not reach upstream")}
	svc := newPricingPreflightTestService(upstream, 0, nil)
	account := &Account{
		ID:          907,
		Name:        "ollama-cloud-cc",
		Platform:    PlatformOllamaCloud,
		Type:        AccountTypeAPIKey,
		Concurrency: 1,
		Credentials: map[string]any{
			"api_key":      "sk-test",
			"api_protocol": APIProtocolChatCompletions,
			"base_url":     "https://ollama.com",
		},
	}

	body := []byte(`{"model":"totally-unpriced-model","messages":[{"role":"user","content":"hi"}],"stream":false}`)
	c, rec, ctx := pricingPreflightContext("/v1/chat/completions", body, nil)
	_, err := svc.forwardAsRawChatCompletions(ctx, c, account, body, "")
	assertPricingPreflightRejected(t, c, upstream, rec, err, "totally-unpriced-model")
}

// TestOllamaCloudPricingPreflight_AnthropicNativePathRejectsUnpriced 第三挂点
// （buildNativeAnthropicUpstreamRequest）：api_protocol=anthropic 的 ollama 账号走
// 独立构造器，未定价模型同样必须在任何上游 I/O 之前 400。
func TestOllamaCloudPricingPreflight_AnthropicNativePathRejectsUnpriced(t *testing.T) {
	gin.SetMode(gin.TestMode)

	upstream := &httpUpstreamRecorder{err: errors.New("must not reach upstream")}
	svc := newPricingPreflightTestService(upstream, 0, nil)
	account := &Account{
		ID:          908,
		Name:        "ollama-cloud-anthropic",
		Platform:    PlatformOllamaCloud,
		Type:        AccountTypeAPIKey,
		Concurrency: 1,
		Credentials: map[string]any{
			"api_key":      "sk-test",
			"api_protocol": APIProtocolAnthropic,
			"base_url":     "https://ollama.com",
		},
	}

	body := []byte(`{"model":"totally-unpriced-model","max_tokens":64,"messages":[{"role":"user","content":"hi"}],"stream":false}`)
	c, rec, ctx := pricingPreflightContext("/v1/messages", body, nil)
	_, err := svc.ForwardAsAnthropic(ctx, c, account, body, "", "")
	assertPricingPreflightRejected(t, c, upstream, rec, err, "totally-unpriced-model")
}
