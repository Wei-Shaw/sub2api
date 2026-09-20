package admin

import (
	"context"

	"github.com/Wei-Shaw/sub2api/internal/handler/dto"
	"github.com/Wei-Shaw/sub2api/internal/service"
)

var linkedAccountUsageExtraKeys = [...]string{
	"codex_usage_updated_at",
	"codex_5h_used_percent",
	"codex_5h_reset_at",
	"codex_5h_reset_after_seconds",
	"codex_5h_window_minutes",
	"codex_7d_used_percent",
	"codex_7d_reset_at",
	"codex_7d_reset_after_seconds",
	"codex_7d_window_minutes",
	"session_window_utilization",
	"passive_usage_7d_utilization",
	"passive_usage_7d_reset",
	"passive_usage_7d_oi_utilization",
	"passive_usage_7d_oi_reset",
	"passive_usage_sampled_at",
}

// inheritLinkedAccountUsageSnapshot makes every normal linked account-list row
// reflect the parent's core usage-window state, regardless of platform. Spark
// shadows keep their own quota snapshot and must not inherit this state.
func inheritLinkedAccountUsageSnapshot(account *dto.Account, parent *service.Account) {
	if account == nil || parent == nil || account.QuotaDimension != service.QuotaDimensionLinked {
		return
	}
	if account.Extra == nil {
		account.Extra = make(map[string]any, len(linkedAccountUsageExtraKeys))
	}
	for _, key := range linkedAccountUsageExtraKeys {
		value, exists := parent.Extra[key]
		if !exists {
			delete(account.Extra, key)
			continue
		}
		account.Extra[key] = value
	}
	account.SessionWindowStart = parent.SessionWindowStart
	account.SessionWindowEnd = parent.SessionWindowEnd
	account.SessionWindowStatus = parent.SessionWindowStatus
}

// enrichShadowParentInfo 把母账号的展示信息回填到影子行的 parent_* 字段。
// 纯函数：仅依赖传入的母账号 map，便于单测；非影子或母账号缺失时优雅留空。
func enrichShadowParentInfo(items []AccountWithConcurrency, parents map[int64]*service.Account) {
	for i := range items {
		a := items[i].Account
		if a == nil || a.ParentAccountID == nil {
			continue
		}
		p := parents[*a.ParentAccountID]
		if p == nil {
			continue
		}
		a.ParentEmail = p.GetCredential("email")
		a.ParentPlanType = p.GetCredential("plan_type")
		a.ParentSubscriptionExpiresAt = p.GetCredential("subscription_expires_at")
		a.ParentChatGPTAccountID = p.GetCredential("chatgpt_account_id")
		a.ParentPrivacyMode = p.GetExtraString("privacy_mode")
		inheritLinkedAccountUsageSnapshot(a, p)
	}
}

// enrichShadowParents 收集本批影子行的母账号 ID、一次批量解析（避免 N+1），再回填。
// 解析失败时不报错（parent_* 留空，降级）。
func (h *AccountHandler) enrichShadowParents(ctx context.Context, items []AccountWithConcurrency) {
	seen := make(map[int64]struct{})
	for i := range items {
		a := items[i].Account
		if a == nil || a.ParentAccountID == nil {
			continue
		}
		seen[*a.ParentAccountID] = struct{}{}
	}
	if len(seen) == 0 {
		return
	}
	parentIDs := make([]int64, 0, len(seen))
	for pid := range seen {
		parentIDs = append(parentIDs, pid)
	}
	parents, err := h.adminService.GetAccountsByIDs(ctx, parentIDs)
	if err != nil {
		return
	}
	pmap := make(map[int64]*service.Account, len(parents))
	for _, p := range parents {
		pmap[p.ID] = p
	}
	enrichShadowParentInfo(items, pmap)
}
