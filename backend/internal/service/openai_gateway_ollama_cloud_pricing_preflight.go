package service

// 请求期 ollama_cloud 定价预检：在上游 I/O 之前拒绝无价可循的出站模型（客户端
// 得到 400），取代曾在 RecordUsage 成本计算里执行的异步阶段拒绝。异步阶段的拒绝
// 位置是错的——那时上游已被调用、响应已写给客户端（各 handler 先 Forward，再把
// RecordUsage 投递给后台 worker，其错误只在闭包里打日志），拒绝只会让 usage 整条
// 不落账，形成「请求成功但不计费」的静默漏损。
//
// 挂点（合计覆盖 ollama_cloud 的全部 HTTP 出站）：
//   - buildUpstreamRequest               原生 Responses 出站；三条入站路径的转换分支都经过它
//   - sendCCUpstreamRequest              raw CC 出站；三条入站路径的回退分支都经过它
//   - buildNativeAnthropicUpstreamRequest api_protocol=anthropic 的独立构造器
//
// 判定与计费同源：复用 RecordUsage → CalculateCostUnified → ModelPricingResolver
// 的同一条解析链（Group → Channel → LiteLLM → Fallback）。无价判定取
// calculateTokenCost 的失败条件（token 模式下 GetIntervalPricing 无价可取）；按次 /
// 图片 / 视频模式走 calculatePerRequestCost，从不因缺价报错，因此不视为未定价——
// 预检绝不会比计费更严格，「只有渠道价 / 分组价」的合法流量不会被误拒。
//
// 预检是主门；未定价请求被放行（理论上仅剩 body 无模型名等无法判定的形态）时，
// 计费侧回退到既有的零成本落账 + `openai_usage.pricing_missing_record_zero_cost`
// 告警路径——那条告警是放量观察断言的观测依据，不能被本门禁遮蔽。

import (
	"context"
	"fmt"
	"net/http"
	"strings"

	"github.com/Wei-Shaw/sub2api/internal/pkg/ctxkey"
	"github.com/gin-gonic/gin"
	"github.com/tidwall/gjson"
)

// openAIOllamaCloudUnpricedModelError 标记 ollama_cloud 出站模型名在计费同一条
// 解析链上无价可循。网关必须把它应答为 400 invalid_request_error，不得向上游转发，
// 也不得触发账号 failover。
type openAIOllamaCloudUnpricedModelError struct {
	model string
}

func (e *openAIOllamaCloudUnpricedModelError) Error() string {
	return fmt.Sprintf(
		"model %q has no pricing configured and cannot be billed: configure model pricing (base, group or channel) or adjust the account model_mapping/allowed_models",
		e.model,
	)
}

// respondOpenAIOllamaCloudUnpricedModelError 按 respondOpenAIStatelessFieldError 的
// 先例由服务层直接写出 400 invalid_request_error。写前 MarkResponseCommitted 与
// cyber_policy / grok content 拒绝同款：handler 侧 ensureForwardErrorResponse /
// ensureAnthropicErrorResponse 检测到响应已提交便不再追加 fallback 错误；返回的
// 错误非 UpstreamFailoverError——不换号、不写账号处置。
func respondOpenAIOllamaCloudUnpricedModelError(c *gin.Context, err *openAIOllamaCloudUnpricedModelError) {
	setOpsUpstreamError(c, http.StatusBadRequest, err.Error(), "")
	MarkResponseCommitted(c)
	c.JSON(http.StatusBadRequest, gin.H{"error": gin.H{
		"type":    "invalid_request_error",
		"message": err.Error(),
	}})
}

// ollamaCloudBillingGroupFromContext 取认证中间件放入 ctx 的计费分组（与利润门 D
// 同一口径：计费永远按 apiKey 自身分组），用于分组/渠道定价查找。ctx 无有效分组
// （内部调用、直接构造 service 的单测）时返回 nil——等价于计费在无分组输入下的
// 解析行为，不会让预检更严格。
func ollamaCloudBillingGroupFromContext(ctx context.Context) *Group {
	if ctx == nil {
		return nil
	}
	group, _ := ctx.Value(ctxkey.Group).(*Group)
	if IsGroupContextValid(group) {
		return group
	}
	return nil
}

// ollamaCloudModelUnpricedForBilling 报告模型在计费的解析链上是否无价可循。
// s.resolver 为 nil 时恒返回 false（放行）：生产 wire 必然注入 resolver，直接构造
// struct 的单测与内部调用不应被门禁拦截（与 resolveOpenAIChannelPricing 的 nil 守卫
// 同一取舍）。
func (s *OpenAIGatewayService) ollamaCloudModelUnpricedForBilling(ctx context.Context, model string) bool {
	if s == nil || s.resolver == nil {
		return false
	}
	group := ollamaCloudBillingGroupFromContext(ctx)
	var groupID *int64
	if group != nil {
		gid := group.ID
		groupID = &gid
	}
	resolved := s.resolver.Resolve(ctx, PricingInput{Model: model, GroupID: groupID, Group: group})
	if resolved == nil {
		return true
	}
	// 与 CalculateCostUnified 的模式分发一致：按次/图片/视频模式由
	// calculatePerRequestCost 计价，缺价只会得到 0 而不会报错；只有 token 模式
	// 会在 GetIntervalPricing 无价可取时以 ErrModelPricingUnavailable 失败。
	if resolved.Mode != BillingModeToken {
		return false
	}
	// 定价上下文取 1 token（最低档）：GetIntervalPricing 返回 nil 与否只取决于
	// BasePricing 是否存在，与上下文档位无关，与 calculateTokenCost 的失败条件
	// 完全同一条链。
	return s.resolver.GetIntervalPricing(resolved, 1) == nil
}

// enforceOllamaCloudRequestPricingPreflight 在上游 I/O 之前对 ollama_cloud 出站做
// 定价预检。未定价时写出 400 并返回类型化错误，调用方原样 `return nil, err` 即可
// 终止本次出站（构造的请求不会发出，上游零调用）。非 ollama_cloud 账号、出站
// body 缺模型名（无法判定，交由计费侧零成本告警观测）一律放行。
func (s *OpenAIGatewayService) enforceOllamaCloudRequestPricingPreflight(ctx context.Context, c *gin.Context, account *Account, body []byte) error {
	if account == nil || !account.IsOllamaCloud() {
		return nil
	}
	model := strings.TrimSpace(gjson.GetBytes(body, "model").String())
	if model == "" {
		return nil
	}
	if !s.ollamaCloudModelUnpricedForBilling(ctx, model) {
		return nil
	}
	err := &openAIOllamaCloudUnpricedModelError{model: model}
	respondOpenAIOllamaCloudUnpricedModelError(c, err)
	return err
}
