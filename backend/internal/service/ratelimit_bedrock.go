package service

import (
	"context"
	"log/slog"
	"net/http"
	"strings"
	"time"
)

// Bedrock 错误文案（小写，与 extractUpstreamErrorMessage 的结果比对）。
// 400 ValidationException 会把请求里的非法字段名回显进消息，账号级判定只认整句或固定开头，
// 不按子串匹配，避免请求内容触发停号。
const (
	// 400 ValidationException：AWS 封禁账号后所有调用返回的完整消息。
	bedrockAccountBlockedMessage = "operation not allowed"
	// 404 ResourceNotFoundException：账号未提交 Anthropic 模型用例表单时消息的开头。
	bedrockUseCaseMissingPrefix = "model use case details have not been submitted"
	// 5xx：Bedrock 内容过滤拦截了请求或输出。
	bedrockContentFilterMarker = "blocked by content filtering policy"

	bedrockThrottlingReason = "bedrock_throttling"
)

type gatewaySchedulingModelContextKey struct{}

// WithGatewaySchedulingModel 记录选号时使用的客户端模型。请求经渠道模型映射改写模型后转发时，
// Bedrock 账号的模型级状态按该模型推导调度器读取的键。
func WithGatewaySchedulingModel(ctx context.Context, model string) context.Context {
	if ctx == nil {
		ctx = context.Background()
	}
	return context.WithValue(ctx, gatewaySchedulingModelContextKey{}, strings.TrimSpace(model))
}

// bedrockSchedulingModelKey 返回调度器按模型隔离账号状态时读取的键：账号映射后的选号模型；
// 上下文未记录选号模型时用本次转发的模型。键不是发往上游的 Bedrock 模型 ID。
func bedrockSchedulingModelKey(ctx context.Context, account *Account, forwardModel string) string {
	model := forwardModel
	if ctx != nil {
		if scheduling, _ := ctx.Value(gatewaySchedulingModelContextKey{}).(string); scheduling != "" {
			model = scheduling
		}
	}
	return account.GetMappedModel(model)
}

// handleBedrockUpstreamError 处理 Bedrock 账号特有的错误语义，handled=false 时交回通用逻辑。
//
// Bedrock 的限流额度按（AWS 账号、源区域、推理配置、模型）计算，而一个 Bedrock 账号固定了
// AWS 凭证、区域与推理配置，所以 429 只冷却本次请求的模型；模型未知时交回通用的账号级兜底。
func (s *RateLimitService) handleBedrockUpstreamError(ctx context.Context, account *Account, statusCode int, upstreamMsg string, modelKey string) (handled bool, shouldDisable bool) {
	switch statusCode {
	case http.StatusBadRequest:
		if isBedrockAccountBlockedMessage(upstreamMsg) {
			s.handleAuthError(ctx, account, "Bedrock account blocked (400): "+upstreamMsg)
			return true, true
		}
		// 其余 400 都是请求错误，不影响账号状态。
		return true, false
	case http.StatusNotFound:
		if strings.HasPrefix(normalizeBedrockErrorMessage(upstreamMsg), bedrockUseCaseMissingPrefix) {
			s.handleAuthError(ctx, account, "Bedrock model use case not submitted (404): "+upstreamMsg)
			return true, true
		}
	case http.StatusTooManyRequests:
		modelKey = strings.TrimSpace(modelKey)
		if modelKey == "" {
			return false, false
		}
		s.applyBedrockModelThrottle(ctx, account, modelKey)
		return true, false
	}
	return false, false
}

// applyBedrockModelThrottle 按 429 默认回避配置冷却 (账号, 模型)；回避关闭时不写任何状态。
func (s *RateLimitService) applyBedrockModelThrottle(ctx context.Context, account *Account, modelKey string) {
	cooldown, enabled := s.get429FallbackCooldown(ctx, account)
	if !enabled {
		slog.Info("bedrock_throttling_cooldown_ignored", "account_id", account.ID, "model", modelKey)
		return
	}
	resetAt := time.Now().Add(cooldown)
	if err := s.accountRepo.SetModelRateLimit(ctx, account.ID, modelKey, resetAt, bedrockThrottlingReason); err != nil {
		slog.Warn("bedrock_throttling_set_model_rate_limit_failed", "account_id", account.ID, "model", modelKey, "error", err)
		return
	}
	slog.Info("bedrock_throttling_model_rate_limited", "account_id", account.ID, "model", modelKey, "reset_at", resetAt)
}

func normalizeBedrockErrorMessage(msg string) string {
	return strings.ToLower(strings.TrimSpace(msg))
}

// isBedrockAccountBlockedMessage 按整句比对封号消息（忽略大小写与句末句点）。
func isBedrockAccountBlockedMessage(msg string) bool {
	return strings.TrimSuffix(normalizeBedrockErrorMessage(msg), ".") == bedrockAccountBlockedMessage
}

// isBedrockContentFilterError 判断上游错误是否为 Bedrock 内容过滤拦截；状态码集合与
// handleErrorResponse 把 5xx 改写为客户端响应的 case 一致（500/502/503/504）。
func isBedrockContentFilterError(statusCode int, body []byte) bool {
	switch statusCode {
	case http.StatusInternalServerError, http.StatusBadGateway, http.StatusServiceUnavailable, http.StatusGatewayTimeout:
	default:
		return false
	}
	return strings.Contains(normalizeBedrockErrorMessage(extractUpstreamErrorMessage(body)), bedrockContentFilterMarker)
}
