package middleware

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/pkg/ctxkey"
	"github.com/Wei-Shaw/sub2api/internal/pkg/ip"
	"github.com/Wei-Shaw/sub2api/internal/pkg/logger"
	"github.com/Wei-Shaw/sub2api/internal/service"

	"github.com/gin-gonic/gin"
)

const maxAPIKeyAuthorizationHeaderBytes = service.MaxAPIKeyCredentialBytes + 128

// NewAPIKeyAuthMiddleware 创建 API Key 认证中间件
func NewAPIKeyAuthMiddleware(apiKeyService *service.APIKeyService, subscriptionService *service.SubscriptionService, cfg *config.Config) APIKeyAuthMiddleware {
	return APIKeyAuthMiddleware(apiKeyAuthWithSubscription(apiKeyService, subscriptionService, cfg))
}

// apiKeyAuthWithSubscription API Key认证中间件（支持订阅验证）
//
// 中间件职责分为两层：
//   - 鉴权（Authentication）：验证 Key 有效性、用户状态、IP 限制 —— 始终执行
//   - 计费执行（Billing Enforcement）：过期/配额/订阅/余额检查 —— skipBilling 时整块跳过
//
// /v1/usage、/v1/sub2api/billing 端点与异步生图任务查询只需鉴权，不需要计费执行。
// usage 允许过期/配额耗尽的 Key 查询自身用量，billing 用于读取当前 Key 的倍率配置，
// 异步生图查询允许已耗尽额度的 Key 拉取自身任务结果。
func apiKeyAuthWithSubscription(apiKeyService *service.APIKeyService, subscriptionService *service.SubscriptionService, cfg *config.Config) gin.HandlerFunc {
	return func(c *gin.Context) {
		// ── 1. 提取 API Key ──────────────────────────────────────────
		if rejectInvalidAuthAbuse(c, apiKeyService) {
			AbortWithError(c, http.StatusTooManyRequests, "INVALID_AUTH_RATE_LIMITED", "Too many invalid authentication attempts; retry later")
			return
		}

		if apiKeyHeadersTooLarge(c) {
			recordInvalidAuthFailure(c, apiKeyService)
			MarkIngressRejected(c, IngressRejectInvalidAPIKey)
			AbortWithError(c, http.StatusUnauthorized, "INVALID_API_KEY", "Invalid API key")
			return
		}

		queryKey := strings.TrimSpace(c.Query("key"))
		queryApiKey := strings.TrimSpace(c.Query("api_key"))
		if queryKey != "" || queryApiKey != "" {
			recordInvalidAuthFailure(c, apiKeyService)
			MarkIngressRejected(c, IngressRejectQueryAPIKeyDeprecated)
			AbortWithError(c, 400, "api_key_in_query_deprecated", "API key in query parameter is deprecated. Please use Authorization header instead.")
			return
		}

		// 尝试从Authorization header中提取API key (Bearer scheme)
		authHeader := c.GetHeader("Authorization")
		var apiKeyString string

		if authHeader != "" {
			// 验证Bearer scheme
			parts := strings.SplitN(authHeader, " ", 2)
			if len(parts) == 2 && strings.EqualFold(parts[0], "Bearer") {
				apiKeyString = strings.TrimSpace(parts[1])
			}
		}

		// 如果Authorization header中没有，尝试从x-api-key header中提取
		if apiKeyString == "" {
			apiKeyString = c.GetHeader("x-api-key")
		}
		if len(apiKeyString) > service.MaxAPIKeyCredentialBytes {
			recordInvalidAuthFailure(c, apiKeyService)
			MarkIngressRejected(c, IngressRejectInvalidAPIKey)
			AbortWithError(c, http.StatusUnauthorized, "INVALID_API_KEY", "Invalid API key")
			return
		}

		// 如果x-api-key header中没有，尝试从x-goog-api-key header中提取（Gemini CLI兼容）
		if apiKeyString == "" {
			apiKeyString = c.GetHeader("x-goog-api-key")
		}

		// 如果所有header都没有API key
		if apiKeyString == "" {
			recordInvalidAuthFailure(c, apiKeyService)
			if hasAPIKeyCredentialInput(c) {
				MarkIngressRejected(c, IngressRejectInvalidAPIKey)
			} else {
				MarkIngressRejected(c, IngressRejectAPIKeyRequired)
			}
			AbortWithError(c, 401, "API_KEY_REQUIRED", "API key is required in Authorization header (Bearer scheme), x-api-key header, or x-goog-api-key header")
			return
		}

		// ── 2. 验证 Key 存在 ─────────────────────────────────────────

		apiKey, err := apiKeyService.GetByKey(c.Request.Context(), apiKeyString)
		if err != nil {
			if errors.Is(err, service.ErrAPIKeyNotFound) {
				recordInvalidAuthFailure(c, apiKeyService)
				MarkIngressRejected(c, IngressRejectInvalidAPIKey)
				AbortWithError(c, 401, "INVALID_API_KEY", "Invalid API key")
				return
			}
			if errors.Is(err, service.ErrAPIKeyAuthOverloaded) {
				MarkIngressRejected(c, IngressRejectAPIKeyAuthOverloaded)
				AbortWithError(c, http.StatusServiceUnavailable, "API_KEY_AUTH_OVERLOADED", "API key authentication is temporarily unavailable")
				return
			}
			AbortWithError(c, 500, "INTERNAL_ERROR", "Failed to validate API key")
			return
		}

		// apiKey 已加载（含 User/Group）。即便后续因分组停用/Key 停用/用户停用/
		// IP 限制等早退中断，也让 Ops 错误日志能回退取到 user/group/platform。
		SetOpsFallbackAPIKey(c, apiKey)

		// ── 3. 基础鉴权（始终执行） ─────────────────────────────────

		// disabled / 未知状态 → 无条件拦截（expired 和 quota_exhausted 留给计费阶段）
		if !apiKey.IsActive() &&
			apiKey.Status != service.StatusAPIKeyExpired &&
			apiKey.Status != service.StatusAPIKeyQuotaExhausted {
			MarkIngressRejected(c, IngressRejectAPIKeyDisabled)
			AbortWithError(c, 401, "API_KEY_DISABLED", "API key is disabled")
			return
		}

		// 检查 IP 限制（白名单/黑名单）
		// 注意：错误信息故意模糊，避免暴露具体的 IP 限制机制
		if len(apiKey.IPWhitelist) > 0 || len(apiKey.IPBlacklist) > 0 {
			clientIP := ip.GetSecurityClientIP(c, cfg.TrustForwardedIPForAPIKeyACL())
			allowed, _ := ip.CheckIPRestrictionWithCompiledRules(clientIP, apiKey.CompiledIPWhitelist, apiKey.CompiledIPBlacklist)
			if !allowed {
				if clientIP == "" {
					clientIP = "unknown"
				}
				service.MarkOpsClientBusinessLimited(c, service.OpsClientBusinessLimitedReasonIPRestriction)
				MarkIngressRejected(c, IngressRejectIPRestricted)
				AbortWithError(c, 403, "ACCESS_DENIED", fmt.Sprintf("Access denied. Your IP is %s", clientIP))
				return
			}
		}

		// 检查关联的用户
		if apiKey.User == nil {
			AbortWithError(c, 401, "USER_NOT_FOUND", "User associated with API key not found")
			return
		}

		// 检查用户状态
		if !apiKey.User.IsActive() {
			MarkIngressRejected(c, IngressRejectUserInactive)
			AbortWithError(c, 401, "USER_INACTIVE", "User account is not active")
			return
		}
		if err := apiKeyService.ValidateOrganizationAccess(c.Request.Context(), apiKey.User); err != nil {
			MarkIngressRejected(c, IngressRejectUserInactive)
			AbortWithError(c, 403, "ORGANIZATION_ACCESS_DENIED", "Organization access is unavailable")
			return
		}
		billingInfoRequest := c.Request.URL.Path == "/v1/sub2api/billing"
		// Async image task polling only reads data that already belongs to the
		// authenticated key and must remain available after the completed
		// generation consumes the key's remaining balance.
		skipBilling := c.Request.URL.Path == "/v1/usage" || billingInfoRequest || isAsyncImageTaskRead(c.Request.Method, c.Request.URL.Path)
		routingState, routingErr := prepareAPIKeyRoutingState(
			c.Request.Context(),
			apiKeyService,
			subscriptionService,
			apiKey,
			skipBilling || cfg.RunMode == config.RunModeSimple,
		)
		if routingErr != nil {
			AbortWithError(c, http.StatusForbidden, "NO_AVAILABLE_GROUP", "No available API key group")
			return
		}
		if routingState != nil {
			c.Request = c.Request.WithContext(service.WithAPIKeyRoutingState(c.Request.Context(), routingState))
		}
		if abortIfAPIKeyGroupUnavailable(c, apiKey) {
			return
		}
		if abortIfAPIKeyGroupNotAllowed(c, apiKey) {
			return
		}
		ctx := context.WithValue(c.Request.Context(), ctxkey.UserID, apiKey.User.ID)
		c.Request = c.Request.WithContext(ctx)

		// ── 4. SimpleMode → early return ─────────────────────────────

		if cfg.RunMode == config.RunModeSimple {
			c.Set(string(ContextKeyAPIKey), apiKey)
			c.Set(string(ContextKeyUser), AuthSubject{
				UserID:      apiKey.User.ID,
				Concurrency: apiKey.User.Concurrency,
			})
			c.Set(string(ContextKeyUserRole), apiKey.User.Role)
			setGroupContext(c, apiKey.Group)
			if !billingInfoRequest {
				_ = apiKeyService.TouchLastUsed(c.Request.Context(), apiKey.ID)
			}
			c.Next()
			return
		}

		// ── 5. 按端点需要加载订阅 ───────────────────────────────────

		var subscription *service.UserSubscription
		if routingState != nil {
			subscription = routingState.EffectiveSubscription()
		}
		isSubscriptionType := apiKey.Group != nil && apiKey.Group.IsSubscriptionType()
		// 企业 API Key 绑定公司订阅：消费走组织订阅计数器，不加载个人订阅。
		isEnterpriseKey := apiKey.OrganizationSubscriptionID != nil

		// 倍率自省不需要订阅数据；/v1/usage 仍保留原有订阅读取行为。
		if routingState == nil && isSubscriptionType && subscriptionService != nil && !billingInfoRequest && !isEnterpriseKey {
			sub, subErr := subscriptionService.GetActiveSubscription(
				c.Request.Context(),
				apiKey.User.ID,
				apiKey.Group.ID,
			)
			if subErr != nil {
				if !skipBilling {
					AbortWithError(c, 403, "SUBSCRIPTION_NOT_FOUND", "No active subscription found for this group")
					return
				}
				// skipBilling: 订阅不存在也放行，handler 会返回可用的数据
			} else {
				subscription = sub
			}
		}

		// ── 6. 计费执行（skipBilling 时整块跳过） ────────────────────

		if !skipBilling {
			// Key 状态检查
			switch apiKey.Status {
			case service.StatusAPIKeyQuotaExhausted:
				abortWithAPIKeyQuotaError(c)
				return
			case service.StatusAPIKeyExpired:
				AbortWithError(c, 403, "API_KEY_EXPIRED", "API key 已过期")
				return
			}

			// 运行时过期/配额检查（即使状态是 active，也要检查时间和用量）
			if apiKey.IsExpired() {
				AbortWithError(c, 403, "API_KEY_EXPIRED", "API key 已过期")
				return
			}
			if apiKey.IsQuotaExhausted() {
				abortWithAPIKeyQuotaError(c)
				return
			}

			// 企业 Key：校验所绑定公司订阅的活跃性与限额（不检查个人余额）
			if isEnterpriseKey {
				if validateErr := apiKeyService.ValidateEnterpriseSubscription(c.Request.Context(), apiKey); validateErr != nil {
					// DIAG_USAGE_LIMIT: 记录企业订阅命中日/周/月限额的原因，便于排查 IAM 侧误判
					orgSubIDForLog := int64(0)
					if apiKey.OrganizationSubscriptionID != nil {
						orgSubIDForLog = *apiKey.OrganizationSubscriptionID
					}
					logger.LegacyPrintf(
						"middleware.api_key_auth",
						"DIAG_USAGE_LIMIT branch=enterprise user_id=%d api_key_id=%d org_sub_id=%d group=%s validate_err=%v",
						apiKey.User.ID, apiKey.ID, orgSubIDForLog,
						func() string {
							if apiKey.Group != nil {
								return apiKey.Group.Name
							}
							return ""
						}(),
						validateErr,
					)
					code := "SUBSCRIPTION_INVALID"
					status := 403
					if errors.Is(validateErr, service.ErrDailyLimitExceeded) ||
						errors.Is(validateErr, service.ErrWeeklyLimitExceeded) ||
						errors.Is(validateErr, service.ErrMonthlyLimitExceeded) {
						code = "USAGE_LIMIT_EXCEEDED"
						status = 429
					} else if errors.Is(validateErr, service.ErrOrgSubscriptionNotFound) {
						code = "SUBSCRIPTION_NOT_FOUND"
					}
					AbortWithError(c, status, code, validateErr.Error())
					return
				}
			} else if routingState != nil {
				// Candidate-specific billing is checked by BillingCacheService, which
				// can advance from metered to subscription groups (and vice versa).
			} else if subscription != nil {
				// 订阅模式：验证订阅限额
				needsMaintenance, validateErr := subscriptionService.ValidateAndCheckLimits(subscription, apiKey.Group)
				if needsMaintenance {
					refreshed, maintenanceErr := subscriptionService.EnsureWindowMaintenance(c.Request.Context(), subscription)
					if maintenanceErr != nil {
						AbortWithError(c, 500, "SUBSCRIPTION_MAINTENANCE_FAILED", "Failed to maintain subscription usage windows")
						return
					}
					subscription = refreshed
					_, validateErr = subscriptionService.ValidateAndCheckLimits(subscription, apiKey.Group)
				}
				if validateErr != nil {
					// DIAG_USAGE_LIMIT: 记录个人订阅命中限额的具体上下文，用于确认 IAM 是否误挂订阅
					var (
						dailyUsage, weeklyUsage, monthlyUsage float64
						dailyLimit, weeklyLimit, monthlyLimit float64
						hasDaily, hasWeekly, hasMonthly       bool
						subID                                 int64
					)
					if subscription != nil {
						dailyUsage = subscription.DailyUsageUSD
						weeklyUsage = subscription.WeeklyUsageUSD
						monthlyUsage = subscription.MonthlyUsageUSD
						subID = subscription.ID
					}
					if apiKey.Group != nil {
						if apiKey.Group.DailyLimitUSD != nil {
							dailyLimit = *apiKey.Group.DailyLimitUSD
							hasDaily = true
						}
						if apiKey.Group.WeeklyLimitUSD != nil {
							weeklyLimit = *apiKey.Group.WeeklyLimitUSD
							hasWeekly = true
						}
						if apiKey.Group.MonthlyLimitUSD != nil {
							monthlyLimit = *apiKey.Group.MonthlyLimitUSD
							hasMonthly = true
						}
					}
					logger.LegacyPrintf(
						"middleware.api_key_auth",
						"DIAG_USAGE_LIMIT branch=personal user_id=%d api_key_id=%d sub_id=%d group=%s daily_usage=%f daily_limit=%f has_daily=%v weekly_usage=%f weekly_limit=%f has_weekly=%v monthly_usage=%f monthly_limit=%f has_monthly=%v validate_err=%v",
						apiKey.User.ID, apiKey.ID, subID,
						func() string {
							if apiKey.Group != nil {
								return apiKey.Group.Name
							}
							return ""
						}(),
						dailyUsage, dailyLimit, hasDaily,
						weeklyUsage, weeklyLimit, hasWeekly,
						monthlyUsage, monthlyLimit, hasMonthly,
						validateErr,
					)
					code := "SUBSCRIPTION_INVALID"
					status := 403
					if errors.Is(validateErr, service.ErrDailyLimitExceeded) ||
						errors.Is(validateErr, service.ErrWeeklyLimitExceeded) ||
						errors.Is(validateErr, service.ErrMonthlyLimitExceeded) {
						code = "USAGE_LIMIT_EXCEEDED"
						status = 429
					}
					AbortWithError(c, status, code, validateErr.Error())
					return
				}
			} else {
				// 非订阅模式 或 订阅模式但 subscriptionService 未注入：回退到余额检查
				// DIAG_BILLING_BYPASS: 记录鉴权层看到的实际余额，用于排查 IAM 子账户绕过余额检查的问题
				orgSubIDForLog := int64(0)
				if apiKey.OrganizationSubscriptionID != nil {
					orgSubIDForLog = *apiKey.OrganizationSubscriptionID
				}
				logger.LegacyPrintf(
					"middleware.api_key_auth",
					"DIAG_BILLING_BYPASS auth_balance_gate user_id=%d api_key_id=%d identity_type=%s user_balance=%f is_enterprise_key=%v org_sub_id=%d group=%s is_subscription_type=%v has_personal_subscription=%v",
					apiKey.User.ID, apiKey.ID, apiKey.User.IdentityType, apiKey.User.Balance,
					isEnterpriseKey, orgSubIDForLog,
					func() string {
						if apiKey.Group != nil {
							return apiKey.Group.Name
						}
						return ""
					}(),
					isSubscriptionType, subscription != nil,
				)
				if apiKeyBalanceBelowAuthThreshold(apiKey.User.Balance, cfg) {
					// IAM 用户即使自身余额已 <=0，若归属企业组织且具备 SharedBalanceUse
					// 权限，仍可由企业钱包代付。此处调用 resolver 询问支付源：
					//   - 若返回 BalanceSourceCompany：放行到 BillingCacheService 做企业
					//     余额预检，让企业钱包承担后续消费；
					//   - 若返回 Self/Allocated：说明没有企业代付能力，维持 403。
					// 非 IAM 用户 resolver 恒返回 Self，等价于原逻辑。
					if billingCtx := apiKeyService.ResolveBillingContextForUser(c.Request.Context(), apiKey.User.ID); billingCtx != nil && billingCtx.BalanceSource == service.BalanceSourceCompany {
						logger.LegacyPrintf(
							"middleware.api_key_auth",
							"DIAG_BILLING_BYPASS auth_balance_gate_allow_company user_id=%d api_key_id=%d user_balance=%f payer_user_id=%d balance_source=%s",
							apiKey.User.ID, apiKey.ID, apiKey.User.Balance, billingCtx.PayerUserID, billingCtx.BalanceSource,
						)
					} else {
						AbortWithError(c, 403, "INSUFFICIENT_BALANCE", "Insufficient account balance")
						return
					}
				}
			}
		}

		// ── 7. 设置上下文 → Next ─────────────────────────────────────

		if subscription != nil {
			c.Set(string(ContextKeySubscription), subscription)
		}
		c.Set(string(ContextKeyAPIKey), apiKey)
		c.Set(string(ContextKeyUser), AuthSubject{
			UserID:      apiKey.User.ID,
			Concurrency: apiKey.User.Concurrency,
		})
		c.Set(string(ContextKeyUserRole), apiKey.User.Role)
		setGroupContext(c, apiKey.Group)
		if !billingInfoRequest {
			_ = apiKeyService.TouchLastUsed(c.Request.Context(), apiKey.ID)
		}

		c.Next()
	}
}

func apiKeyHeadersTooLarge(c *gin.Context) bool {
	if c == nil {
		return false
	}
	return len(c.GetHeader("Authorization")) > maxAPIKeyAuthorizationHeaderBytes ||
		len(c.GetHeader("x-api-key")) > service.MaxAPIKeyCredentialBytes ||
		len(c.GetHeader("x-goog-api-key")) > service.MaxAPIKeyCredentialBytes
}

func hasAPIKeyCredentialInput(c *gin.Context) bool {
	if c == nil {
		return false
	}
	return c.GetHeader("Authorization") != "" ||
		c.GetHeader("x-api-key") != "" ||
		c.GetHeader("x-goog-api-key") != ""
}

func abortWithAPIKeyQuotaError(c *gin.Context) {
	const message = "API key 额度已用完"
	if isOpenAICompatibleAPIKeyRequest(c) {
		abortWithOpenAIQuotaError(c, http.StatusTooManyRequests, message)
		return
	}
	AbortWithError(c, http.StatusTooManyRequests, "API_KEY_QUOTA_EXHAUSTED", message)
}

func isOpenAICompatibleAPIKeyRequest(c *gin.Context) bool {
	if c == nil || c.Request == nil || c.Request.URL == nil {
		return false
	}

	path := strings.TrimRight(c.Request.URL.Path, "/")
	for _, root := range []string{
		"/v1/responses",
		"/openai/v1/responses",
		"/responses",
		"/backend-api/codex/responses",
	} {
		if path == root || strings.HasPrefix(path, root+"/") {
			return true
		}
	}
	return false
}

func isAsyncImageTaskRead(method, path string) bool {
	if method != http.MethodGet {
		return false
	}
	return strings.HasPrefix(path, "/v1/images/tasks/") || strings.HasPrefix(path, "/images/tasks/")
}

// GetAPIKeyFromContext 从上下文中获取API key
func GetAPIKeyFromContext(c *gin.Context) (*service.APIKey, bool) {
	value, exists := c.Get(string(ContextKeyAPIKey))
	if !exists {
		return nil, false
	}
	apiKey, ok := value.(*service.APIKey)
	return apiKey, ok
}

// SetOpsFallbackAPIKey 记录已加载的 API Key，供 Ops 错误日志在鉴权早退时回退使用。
// 与 ContextKeyAPIKey 区分：写入它不代表请求已通过鉴权，因此不影响 handler、
// 审计日志等对“已鉴权”的判断。
func SetOpsFallbackAPIKey(c *gin.Context, apiKey *service.APIKey) {
	if c == nil || apiKey == nil {
		return
	}
	c.Set(string(ContextKeyOpsFallbackAPIKey), apiKey)
}

// GetOpsFallbackAPIKey 读取 Ops 错误日志专用的回退 API Key。
func GetOpsFallbackAPIKey(c *gin.Context) (*service.APIKey, bool) {
	value, exists := c.Get(string(ContextKeyOpsFallbackAPIKey))
	if !exists {
		return nil, false
	}
	apiKey, ok := value.(*service.APIKey)
	return apiKey, ok
}

// GetSubscriptionFromContext 从上下文中获取订阅信息
func GetSubscriptionFromContext(c *gin.Context) (*service.UserSubscription, bool) {
	if c != nil && c.Request != nil {
		if state := service.APIKeyRoutingStateFromContext(c.Request.Context()); state != nil {
			return state.SubscriptionRef(), true
		}
	}
	value, exists := c.Get(string(ContextKeySubscription))
	if !exists {
		return nil, false
	}
	subscription, ok := value.(*service.UserSubscription)
	return subscription, ok
}

func setGroupContext(c *gin.Context, group *service.Group) {
	if !service.IsGroupContextValid(group) {
		return
	}
	if existing, ok := c.Request.Context().Value(ctxkey.Group).(*service.Group); ok && existing != nil && existing.ID == group.ID && service.IsGroupContextValid(existing) {
		return
	}
	ctx := context.WithValue(c.Request.Context(), ctxkey.Group, group)
	c.Request = c.Request.WithContext(ctx)
}

// apiKeyBalanceBelowAuthThreshold 保持鉴权层的历史语义：仅在余额耗尽（<=0）时拒绝。
// MinimumBalanceReserve 只作为 billing-cache 预检的保守下限，不得复用为鉴权硬门槛，
// 否则已配置该值的存量部署升级后，0 < balance < reserve 的用户会在所有端点被静默 403。
func apiKeyBalanceBelowAuthThreshold(balance float64, _ *config.Config) bool {
	return balance <= 0
}

func abortIfAPIKeyGroupUnavailable(c *gin.Context, apiKey *service.APIKey) bool {
	code, message, ok := validateAPIKeyGroupAvailable(apiKey)
	if ok {
		return false
	}
	service.MarkOpsClientBusinessLimited(c, service.OpsClientBusinessLimitedReasonAPIKeyGroupUnavailable)
	if code == "GROUP_DELETED" {
		MarkIngressRejected(c, IngressRejectGroupDeleted)
	} else {
		MarkIngressRejected(c, IngressRejectGroupDisabled)
	}
	AbortWithError(c, 403, code, message)
	return true
}

func abortIfAPIKeyGroupNotAllowed(c *gin.Context, apiKey *service.APIKey) bool {
	if validateAPIKeyGroupAllowed(apiKey) {
		return false
	}
	service.MarkOpsClientBusinessLimited(c, service.OpsClientBusinessLimitedReasonAPIKeyGroupUnavailable)
	MarkIngressRejected(c, IngressRejectGroupNotAllowed)
	AbortWithError(c, 403, "GROUP_NOT_ALLOWED", "API Key 所属专属分组不再允许当前用户使用")
	return true
}

func validateAPIKeyGroupAllowed(apiKey *service.APIKey) bool {
	if apiKey == nil || apiKey.GroupID == nil || apiKey.User == nil || apiKey.Group == nil {
		return true
	}
	group := apiKey.Group
	if group.IsSubscriptionType() {
		return true
	}
	return apiKey.User.CanBindGroup(group.ID, group.IsExclusive)
}

func validateAPIKeyGroupAvailable(apiKey *service.APIKey) (string, string, bool) {
	if apiKey == nil || apiKey.GroupID == nil {
		return "", "", true
	}
	group := apiKey.Group
	if group == nil || strings.EqualFold(group.Status, "deleted") {
		return "GROUP_DELETED", "API Key 所属分组已删除", false
	}
	if !group.IsActive() {
		return "GROUP_DISABLED", "API Key 所属分组已停用", false
	}
	return "", "", true
}
