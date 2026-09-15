package service

import (
	"context"
	"errors"
	"net/http"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/adobe"
	"github.com/Wei-Shaw/sub2api/internal/pkg/logger"
	"go.uber.org/zap"
)

// adobeQuotaCooldown 是账号 credits 耗尽后摘掉的时长。
//
// Adobe 的额度按它自己的周期回补，上游不告诉我们确切时刻，所以取一个不长的固定值：
// 太短会反复撞同一个空账号，太长会在额度已恢复后仍白白闲置账号。
const adobeQuotaCooldown = 30 * time.Minute

// Adobe 失败原因，进入 UpstreamFailoverError.Reason 供 ops 归因。
const (
	adobeFailureQuotaExhausted  = GatewayFailureReason("adobe_quota_exhausted")
	adobeFailureNotEntitled     = GatewayFailureReason("adobe_not_entitled")
	adobeFailureAuth            = GatewayFailureReason("adobe_auth")
	adobeFailureUpstream        = GatewayFailureReason("adobe_upstream")
	adobeFailureRequest         = GatewayFailureReason("adobe_request")
	adobeFailureContentRejected = GatewayFailureReason("adobe_content_rejected")
)

// adobeFailure 是 adobe 包的错误翻译成网关语义后的结果。
type adobeFailure struct {
	// Failover 描述该错误对本次请求的影响（是否换号、给客户端什么状态码）。
	Failover *UpstreamFailoverError
	// Cooldown 大于 0 时，handler 应把该账号临时摘出调度。
	Cooldown time.Duration
	// CooldownReason 写进账号的不可调度原因，便于管理端排查。
	CooldownReason string
	// InvalidateToken 为 true 时，本次使用的 access_token 已被上游明确拒绝，应从账号里清掉，
	// 让下次取 token 时走带锁刷新，而不是按 JWT exp 继续复用它。
	InvalidateToken bool
}

// classifyAdobeError 把 internal/pkg/adobe 的类型化错误翻译成网关语义。
//
// adobe.IsRotatable 已经把「换个账号可能成功」的类别归好，这里补的是 sub2api
// 侧的动作：给客户端什么状态码、要不要冷却账号。
func classifyAdobeError(err error) adobeFailure {
	if err == nil {
		return adobeFailure{}
	}

	// 配额耗尽、权益不足与鉴权失效共用 401/403，必须按这个顺序拆开：
	// 冷却账号、换更高套餐号、刷新凭据，三者不能混。
	var quotaErr *adobe.QuotaExhaustedError
	if errors.As(err, &quotaErr) {
		return adobeFailure{
			Failover: &UpstreamFailoverError{
				StatusCode:        statusOrDefault(quotaErr.StatusCode, http.StatusTooManyRequests),
				Stage:             GatewayFailureStageInference,
				Scope:             GatewayFailureScopeAccount,
				Reason:            adobeFailureQuotaExhausted,
				NextAccountAction: NextAccountRetry,
			},
			Cooldown:       adobeQuotaCooldown,
			CooldownReason: "adobe credits exhausted",
		}
	}

	var entitledErr *adobe.NotEntitledError
	if errors.As(err, &entitledErr) {
		return adobeFailure{
			Failover: &UpstreamFailoverError{
				StatusCode:        statusOrDefault(entitledErr.StatusCode, http.StatusForbidden),
				ClientStatusCode:  http.StatusForbidden,
				ClientMessage:     entitledErr.User(),
				Stage:             GatewayFailureStageInference,
				Scope:             GatewayFailureScopeAccount,
				Reason:            adobeFailureNotEntitled,
				NextAccountAction: NextAccountRetry,
			},
		}
	}

	var authErr *adobe.AuthError
	if errors.As(err, &authErr) {
		// 不冷却账号：冷却只会让恢复变慢。401 是上游对这个 token 的明确拒绝（被吊销但 JWT 未到期），
		// 清掉 token 后下次取 token 会走带锁的 cookie 刷新；cookie 真的失效时，刷新路径会把账号置为 error。
		// 403 可能是 WAF/风控，不是 token 本身的问题，只让当次请求换号。
		return adobeFailure{
			Failover: &UpstreamFailoverError{
				StatusCode:        statusOrDefault(authErr.StatusCode, http.StatusUnauthorized),
				Stage:             GatewayFailureStageAccountAuth,
				Scope:             GatewayFailureScopeAccount,
				Reason:            adobeFailureAuth,
				NextAccountAction: NextAccountRetry,
			},
			InvalidateToken: authErr.StatusCode == http.StatusUnauthorized,
		}
	}

	var contentErr *adobe.ContentRejectedError
	if errors.As(err, &contentErr) {
		return adobeFailure{
			Failover: &UpstreamFailoverError{
				StatusCode:        statusOrDefault(contentErr.StatusCode, http.StatusUnavailableForLegalReasons),
				ClientStatusCode:  http.StatusBadRequest,
				ClientMessage:     contentErr.User(),
				Stage:             GatewayFailureStageInference,
				Scope:             GatewayFailureScopeRequest,
				Reason:            adobeFailureContentRejected,
				NextAccountAction: NextAccountRetry,
			},
		}
	}

	var temporaryErr *adobe.UpstreamTemporaryError
	if errors.As(err, &temporaryErr) {
		return adobeFailure{
			Failover: &UpstreamFailoverError{
				StatusCode:        statusOrDefault(temporaryErr.StatusCode, http.StatusBadGateway),
				Stage:             GatewayFailureStageInference,
				Scope:             GatewayFailureScopeProvider,
				Reason:            adobeFailureUpstream,
				NextAccountAction: NextAccountRetry,
			},
		}
	}

	// 终态：请求本身有问题（比例不支持、内容被拒、模型未知等），换号也救不了。
	var requestErr *adobe.RequestError
	if errors.As(err, &requestErr) {
		return adobeFailure{
			Failover: &UpstreamFailoverError{
				StatusCode:        statusOrDefault(requestErr.StatusCode, http.StatusBadRequest),
				ClientStatusCode:  http.StatusBadRequest,
				ClientMessage:     requestErr.User(),
				Stage:             GatewayFailureStageInference,
				Scope:             GatewayFailureScopeRequest,
				Reason:            adobeFailureRequest,
				NextAccountAction: NextAccountStop,
			},
		}
	}

	// 非本包错误（ctx 取消、编解码失败等）：不换号，交由调用方原样上抛。
	return adobeFailure{}
}

func statusOrDefault(status, fallback int) int {
	if status > 0 {
		return status
	}
	return fallback
}

// applyAdobeCooldown 按分类结果把账号临时摘出调度。失败只记日志——
// 冷却是优化项，摘不掉最多是下次请求再撞一次，不该让本次请求失败。
func applyAdobeCooldown(ctx context.Context, repo AccountRepository, accountID int64, failure adobeFailure) {
	if repo == nil || accountID <= 0 || failure.Cooldown <= 0 {
		return
	}
	until := time.Now().Add(failure.Cooldown)
	if err := repo.SetTempUnschedulable(ctx, accountID, until, failure.CooldownReason); err != nil {
		logger.L().With(zap.String("component", "service.adobe")).Warn(
			"adobe.cooldown_failed",
			zap.Int64("account_id", accountID),
			zap.String("reason", failure.CooldownReason),
			zap.Error(err),
		)
	}
}

// AdobeFailover 把 internal/pkg/adobe 的错误翻译成网关的 failover 决策，
// 并按需把账号临时摘出调度。
//
// 这是 handler 层的唯一入口：分类结果（adobeFailure）刻意不导出，避免把
// 「怎么处置」的判断散到 handler 里各写一遍。
// 返回 nil 表示该错误不属于 Adobe 上游语义（如 ctx 取消），调用方应原样上抛。
// token 是本次请求发给上游的 access_token，用于在上游明确拒绝时按值清掉它。
func (s *GatewayService) AdobeFailover(ctx context.Context, accountID int64, token string, err error) *UpstreamFailoverError {
	failure := classifyAdobeError(err)
	if failure.Failover == nil {
		return nil
	}
	if s != nil {
		applyAdobeCooldown(ctx, s.accountRepo, accountID, failure)
		applyAdobeTokenInvalidation(ctx, s.accountRepo, accountID, token, failure)
	}
	return failure.Failover
}

// applyAdobeTokenInvalidation 清掉被上游明确拒绝的 access_token。失败只记日志——
// 清不掉最多是下个请求再撞一次 401 再换号，不该让本次请求失败。
func applyAdobeTokenInvalidation(ctx context.Context, repo AccountRepository, accountID int64, token string, failure adobeFailure) {
	if !failure.InvalidateToken || accountID <= 0 || token == "" {
		return
	}
	invalidator, ok := repo.(AdobeTokenInvalidationRepository)
	if !ok {
		return
	}
	if _, err := invalidator.InvalidateAdobeAccessTokenIfUnchanged(ctx, accountID, token); err != nil {
		logger.L().With(zap.String("component", "service.adobe")).Warn(
			"adobe.token_invalidation_failed",
			zap.Int64("account_id", accountID),
			zap.Error(err),
		)
	}
}

// IsAdobeContentRejected 报告这次 failover 是否来自 Firefly 内容安全拒绝。
// Handler 用它决定「跳过其余 Cookie 号，只试组内 OpenAI 形中转号」。
func IsAdobeContentRejected(err *UpstreamFailoverError) bool {
	return err != nil && err.Reason == adobeFailureContentRejected
}
