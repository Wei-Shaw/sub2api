package admin

import (
	"context"
	"slices"
	"strconv"
	"strings"

	"github.com/Wei-Shaw/sub2api/internal/pkg/adminaccess"
	"github.com/Wei-Shaw/sub2api/internal/pkg/response"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
)

type scopedAccountReader interface {
	GetAccount(context.Context, int64) (*service.Account, error)
}

func scopedAccountResource(a *service.Account) adminaccess.Account {
	return adminaccess.Account{ID: a.ID, GroupIDs: a.GroupIDs, Platform: a.Platform, Type: a.Type}
}

// 仅校验实际读取对象；上游凭证来自母账户时，母账户必须同时获授权。
func scopedAuthorizedAccount(c *gin.Context, reader scopedAccountReader, id int64, action string, parent bool) (*service.Account, bool) {
	p := adminaccess.RestrictedFromContext(c.Request.Context())
	if p == nil {
		return nil, true
	}
	if reader == nil || id <= 0 {
		response.Error(c, 404, "账户不存在")
		return nil, false
	}
	a, err := reader.GetAccount(c.Request.Context(), id)
	if err != nil || a == nil || !p.Policy.AllowsAccount(action, scopedAccountResource(a)) {
		response.Error(c, 404, "账户不存在")
		return nil, false
	}
	if parent && a.IsShadow() {
		if a.ParentAccountID == nil || *a.ParentAccountID == id {
			response.Error(c, 404, "账户不存在")
			return nil, false
		}
		source, err := reader.GetAccount(c.Request.Context(), *a.ParentAccountID)
		if err != nil || source == nil || source.IsShadow() || !p.Policy.AllowsAccount(action, scopedAccountResource(source)) {
			response.Error(c, 404, "账户不存在")
			return nil, false
		}
	}
	return a, true
}

func (h *AccountHandler) scopedAdminStatistics(c *gin.Context) bool {
	path := strings.TrimPrefix(c.FullPath(), "/api/v1/admin/accounts")
	action := ""
	switch {
	case c.Request.Method == "GET" && (path == "/:id/today-stats" || path == "/:id/stats"):
		action = "accounts.stats.read"
	case c.Request.Method == "GET" && path == "/:id/usage":
		action = "accounts.usage.read"
	case c.Request.Method == "POST" && path == "/today-stats/batch":
		action = "accounts.stats.read"
	case c.Request.Method == "POST" && path == "/usage/batch":
		action = "accounts.usage.read"
	default:
		return false
	}
	if strings.HasSuffix(path, "/batch") {
		var req BatchUsageRequest
		if c.ShouldBindJSON(&req) != nil || len(req.AccountIDs) == 0 || len(req.AccountIDs) > 100 {
			response.BadRequest(c, "请明确指定 1 至 100 个账户 ID")
			return true
		}
		ids := make([]int64, 0, len(req.AccountIDs))
		accounts := map[int64]*service.Account{}
		for _, id := range req.AccountIDs {
			if id <= 0 {
				response.BadRequest(c, "账户 ID 无效")
				return true
			}
			if slices.Contains(ids, id) {
				continue
			}
			a, ok := scopedAuthorizedAccount(c, h.adminService, id, action, action == "accounts.usage.read")
			if !ok {
				return true
			}
			ids = append(ids, id)
			accounts[id] = a
		}
		if h.accountUsageService == nil {
			response.Error(c, 503, "账户额度服务不可用")
			return true
		}
		// 不读取全权批量缓存，避免不同权限共享 ETag 或未裁剪结果。
		if action == "accounts.stats.read" {
			stats, err := h.accountUsageService.GetTodayStatsBatch(c.Request.Context(), ids)
			if err != nil {
				response.Error(c, 502, "账户统计查询失败")
				return true
			}
			response.Success(c, gin.H{"stats": stats})
			return true
		}
		usage, errs, err := h.accountUsageService.GetUsageBatch(c.Request.Context(), ids, req.Force)
		if err != nil {
			response.Error(c, 502, "账户额度查询失败")
			return true
		}
		safe := map[int64]any{}
		for id, u := range usage {
			if a := accounts[id]; a != nil && u != nil {
				safe[id] = scopedUsageDTO(u, adminaccess.RestrictedFromContext(c.Request.Context()).Policy, a)
			}
		}
		safeErrors := map[int64]string{}
		for id := range errs {
			if accounts[id] != nil {
				safeErrors[id] = "账户额度查询失败"
			}
		}
		response.Success(c, gin.H{"usage": safe, "errors": safeErrors})
		return true
	}
	id, _ := strconv.ParseInt(c.Param("id"), 10, 64)
	a, ok := scopedAuthorizedAccount(c, h.adminService, id, action, action == "accounts.usage.read")
	if !ok {
		return true
	}
	if h.accountUsageService == nil {
		response.Error(c, 503, "账户额度服务不可用")
		return true
	}
	switch path {
	case "/:id/today-stats":
		stats, err := h.accountUsageService.GetTodayStats(c.Request.Context(), id)
		if err != nil {
			response.Error(c, 502, "账户统计查询失败")
		} else {
			response.Success(c, stats)
		}
	case "/:id/stats":
		h.GetStats(c)
	case "/:id/usage":
		var u *service.UsageInfo
		var err error
		if c.Query("source") == "passive" {
			u, err = h.accountUsageService.GetPassiveUsage(c.Request.Context(), id)
		} else {
			u, err = h.accountUsageService.GetUsage(c.Request.Context(), id, c.Query("force") == "true")
		}
		if err != nil || u == nil {
			response.Error(c, 502, "账户额度查询失败")
		} else {
			response.Success(c, scopedUsageDTO(u, adminaccess.RestrictedFromContext(c.Request.Context()).Policy, a))
		}
	}
	return true
}

func scopedUsageDTO(u *service.UsageInfo, p adminaccess.Policy, a *service.Account) gin.H {
	stats := p.AllowsAccount("accounts.stats.read", scopedAccountResource(a))
	window := func(w *service.UsageProgress) *service.UsageProgress {
		if w == nil {
			return nil
		}
		safe := *w
		if !stats {
			safe.WindowStats = nil
		}
		return &safe
	}
	out := gin.H{
		"source":                    u.Source,
		"updated_at":                u.UpdatedAt,
		"five_hour":                 window(u.FiveHour),
		"seven_day":                 window(u.SevenDay),
		"seven_day_sonnet":          window(u.SevenDaySonnet),
		"seven_day_fable":           window(u.SevenDayFable),
		"gemini_shared_daily":       window(u.GeminiSharedDaily),
		"gemini_pro_daily":          window(u.GeminiProDaily),
		"gemini_flash_daily":        window(u.GeminiFlashDaily),
		"gemini_shared_minute":      window(u.GeminiSharedMinute),
		"gemini_pro_minute":         window(u.GeminiProMinute),
		"gemini_flash_minute":       window(u.GeminiFlashMinute),
		"thirty_day":                window(u.ThirtyDay),
		"antigravity_quota":         u.AntigravityQuota,
		"antigravity_quota_details": u.AntigravityQuotaDetails,
		"ai_credits":                u.AICredits,
		"grok_request_quota":        u.GrokRequestQuota,
		"grok_token_quota":          u.GrokTokenQuota,
		"grok_retry_after_seconds":  u.GrokRetryAfterSeconds,
		"grok_quota_snapshot_state": u.GrokQuotaSnapshotState,
		"grok_free_token_limit":     u.GrokFreeTokenLimit,
		"needs_reauth":              u.NeedsReauth,
		"needs_verify":              u.NeedsVerify,
		"is_banned":                 u.IsBanned,
		"is_forbidden":              u.IsForbidden,
	}
	if p.AllowsAccount("accounts.identity.read", scopedAccountResource(a)) {
		out["subscription_tier"] = u.SubscriptionTier
		out["subscription_tier_raw"] = u.SubscriptionTierRaw
	}

	if u.GrokBilling != nil {
		billing := *u.GrokBilling
		if !p.AllowsAccount("accounts.identity.read", scopedAccountResource(a)) {
			billing.Plan = ""
			billing.TopUpMethod = ""
			billing.IsUnifiedBillingUser = false
		}
		if !stats {
			billing.UsedCents = nil
			billing.IncludedUsedCents = nil
			billing.MonthlyUsed = nil
			billing.OnDemandUsed = nil
		}
		out["grok_billing"] = &billing
	}
	if stats {
		out["grok_local_usage"] = u.GrokLocalUsage
		out["grok_local_usage_24h"] = u.GrokLocalUsage24h
		out["grok_local_usage_7d"] = u.GrokLocalUsage7d
		out["grok_local_usage_monthly"] = u.GrokLocalUsageMonthly
	}
	if u.Error != "" {
		out["error"] = "上游额度查询失败"
	}
	switch u.ErrorCode {
	case "forbidden", "unauthenticated", "rate_limited", "network_error":
		out["error_code"] = u.ErrorCode
	}
	return out
}
