package admin

import (
	"context"

	"github.com/Wei-Shaw/sub2api/internal/service"
)

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
