package service

import (
	"context"
	"fmt"
	"strings"

	"github.com/Wei-Shaw/sub2api/internal/pkg/logger"

	"go.uber.org/zap"
)

// AccountGroupRecomputer 维护用户自建号的 managed 分组链接（私有组 + 共享池）。
// K18：Recompute 仅 GetByName 取私有组，禁止隐式 Ensure。
// K13：组字段变更必须先显式 RemoveGroups（卸链），再 Absorb + Recompute。
type AccountGroupRecomputer struct {
	accountRepo AccountRepository
	groupRepo   GroupRepository
}

// NewAccountGroupRecomputer 构造 recompute 服务。
func NewAccountGroupRecomputer(accountRepo AccountRepository, groupRepo GroupRepository) *AccountGroupRecomputer {
	return &AccountGroupRecomputer{
		accountRepo: accountRepo,
		groupRepo:   groupRepo,
	}
}

// RecomputeManagedLinks 差分维护 managed 链接：仅私有组 + 匹配共享池；
// 不碰 admin 绑在非 share_pool 组上的链接。系统号（owner 空）no-op。
// K18：私有组缺失时不 Ensure，仅 log 并卸 managed share_pool。
func (r *AccountGroupRecomputer) RecomputeManagedLinks(ctx context.Context, account *Account) error {
	if r == nil || r.accountRepo == nil || r.groupRepo == nil || account == nil {
		return nil
	}
	if account.OwnerUserID == nil || *account.OwnerUserID <= 0 {
		return nil
	}

	// 保证有最新 GroupIDs / Groups 用于差分
	acc, err := r.loadAccountForRecompute(ctx, account)
	if err != nil {
		return err
	}

	desired, err := r.desiredManaged(ctx, acc)
	if err != nil {
		return err
	}
	currentManaged, err := r.currentManagedIDs(ctx, acc)
	if err != nil {
		return err
	}

	currentSet := make(map[int64]struct{}, len(acc.GroupIDs))
	for _, id := range acc.GroupIDs {
		currentSet[id] = struct{}{}
	}

	toAdd := make([]int64, 0)
	for id := range desired {
		if _, ok := currentSet[id]; !ok {
			toAdd = append(toAdd, id)
		}
	}
	toRemove := make([]int64, 0)
	for id := range currentManaged {
		if _, ok := desired[id]; !ok {
			toRemove = append(toRemove, id)
		}
	}

	if len(toRemove) > 0 {
		if err := r.accountRepo.RemoveGroups(ctx, acc.ID, toRemove); err != nil {
			return fmt.Errorf("recompute remove groups account=%d: %w", acc.ID, err)
		}
	}
	if len(toAdd) > 0 {
		if err := r.accountRepo.AddGroups(ctx, acc.ID, toAdd); err != nil {
			return fmt.Errorf("recompute add groups account=%d: %w", acc.ID, err)
		}
	}
	return nil
}

// OnSharePoolGroupChange 组字段变更统一入口（K13 Unlink+Absorb）。
// after=nil 表示删除。Step A 必须先于 Step C 调用 RemoveGroups。
func (r *AccountGroupRecomputer) OnSharePoolGroupChange(ctx context.Context, before, after *Group) error {
	if r == nil || r.accountRepo == nil || r.groupRepo == nil {
		return nil
	}
	if before == nil && after == nil {
		return nil
	}

	groupID := int64(0)
	if after != nil {
		groupID = after.ID
	} else if before != nil {
		groupID = before.ID
	}
	if groupID <= 0 {
		return nil
	}

	// —— Step A: Unlink path（破坏性，MUST 显式 RemoveGroups）——
	needsForcedUnlink := after == nil ||
		!isSharePoolCandidate(after) ||
		(before != nil && after != nil && after.Platform != before.Platform)

	boundOwnerAccounts := make([]*Account, 0)
	if needsForcedUnlink || (before != nil && before.IsSharePool) {
		bound, err := r.accountRepo.ListOwnerAccountsBoundToGroup(ctx, groupID)
		if err != nil {
			return fmt.Errorf("list owner accounts bound to group %d: %w", groupID, err)
		}
		boundOwnerAccounts = bound
		// MUST：不依赖 recompute 谓词；幂等
		for _, acc := range boundOwnerAccounts {
			if acc == nil {
				continue
			}
			if err := r.accountRepo.RemoveGroups(ctx, acc.ID, []int64{groupID}); err != nil {
				return fmt.Errorf("forced remove group %d from account %d: %w", groupID, acc.ID, err)
			}
		}
	}

	// —— Step B: Absorb path（建设性）——
	absorbCandidates := make([]*Account, 0)
	if after != nil && isSharePoolCandidate(after) {
		list, err := r.accountRepo.ListPublicOwnerAccountsByPlatformPlan(ctx, after.Platform, after.UpstreamPlan)
		if err != nil {
			return fmt.Errorf("list public owner accounts for absorb group %d: %w", groupID, err)
		}
		absorbCandidates = list
	}

	// —— Step C: 合并受影响集合并 recompute ——
	affected := uniqueAccountsByID(boundOwnerAccounts, absorbCandidates)

	if before != nil && after != nil {
		// 平台变更：旧平台+旧档位（含空档位）上的 public 号需 recompute
		if before.Platform != after.Platform {
			extra, err := r.accountRepo.ListPublicOwnerAccountsByPlatformPlan(ctx, before.Platform, before.UpstreamPlan)
			if err != nil {
				return fmt.Errorf("list public owner accounts for platform change: %w", err)
			}
			affected = uniqueAccountsByID(affected, extra)
		}
		// 档位变更（含变为空/从空变有）：旧 plan 上的 public 号需 recompute
		if normalizeUpstreamPlanForMatch(before.UpstreamPlan) != normalizeUpstreamPlanForMatch(after.UpstreamPlan) && before.IsSharePool {
			extra, err := r.accountRepo.ListPublicOwnerAccountsByPlatformPlan(ctx, before.Platform, before.UpstreamPlan)
			if err != nil {
				return fmt.Errorf("list public owner accounts for plan change: %w", err)
			}
			affected = uniqueAccountsByID(affected, extra)
		}
	}

	for _, acc := range affected {
		if acc == nil {
			continue
		}
		// 卸链后 GroupIDs 可能陈旧：recompute 内会 reload
		if err := r.RecomputeManagedLinks(ctx, acc); err != nil {
			return err
		}
	}
	return nil
}

// isSharePoolCandidate 与共享池匹配谓词一致（不含 platform 上下文，由调用方保证）。
// 空 upstream_plan 仍可为候选：与账号空档位按「空==空」匹配（见 desiredManaged）。
func isSharePoolCandidate(g *Group) bool {
	if g == nil {
		return false
	}
	if !g.IsSharePool || g.IsExclusive {
		return false
	}
	if g.Status != StatusActive {
		return false
	}
	if IsPrivateGroupName(g.Name) {
		return false
	}
	return true
}

// normalizeUpstreamPlanForMatch 档位匹配用规范化（trim；空串表示无档位）。
func normalizeUpstreamPlanForMatch(plan string) string {
	return strings.TrimSpace(plan)
}

// plansMatchForSharePool 平台档位严格相等，含「双方都空」。
func plansMatchForSharePool(accountPlan, groupPlan string) bool {
	return normalizeUpstreamPlanForMatch(accountPlan) == normalizeUpstreamPlanForMatch(groupPlan)
}

// desiredManaged 返回应存在的 managed group ID 集合（K18：Get only，无 Ensure）。
func (r *AccountGroupRecomputer) desiredManaged(ctx context.Context, account *Account) (map[int64]struct{}, error) {
	out := make(map[int64]struct{})
	if account == nil || account.OwnerUserID == nil {
		return out, nil
	}
	name := PrivateGroupName(*account.OwnerUserID, account.Platform)
	private, err := r.groupRepo.GetByName(ctx, name)
	if err != nil {
		if err != ErrGroupNotFound {
			return nil, fmt.Errorf("get private group %q: %w", name, err)
		}
		logger.L().Info("private_group_missing",
			zap.String("component", "service.account_group_recompute"),
			zap.String("event", "private_group_missing"),
			zap.Int64("account_id", account.ID),
			zap.Int64("owner_user_id", *account.OwnerUserID),
			zap.String("platform", account.Platform),
			zap.String("name", name),
		)
	} else if private != nil {
		out[private.ID] = struct{}{}
	}

	// public：按 platform + plan 严格匹配共享池（空档位可匹配空档位组）
	if account.Visibility != VisibilityPublic {
		return out, nil
	}
	matches, err := r.groupRepo.ListSharePoolMatches(ctx, account.Platform, account.UpstreamPlan)
	if err != nil {
		return nil, fmt.Errorf("list share pool matches: %w", err)
	}
	for i := range matches {
		g := &matches[i]
		if isSharePoolCandidate(g) && plansMatchForSharePool(account.UpstreamPlan, g.UpstreamPlan) {
			out[g.ID] = struct{}{}
		}
	}
	return out, nil
}

// CountSharePoolMatches 统计当前账号在 public 语义下可匹配的共享池组数量（不含私有组）。
func (r *AccountGroupRecomputer) CountSharePoolMatches(ctx context.Context, account *Account) (int, error) {
	if r == nil || r.groupRepo == nil || account == nil {
		return 0, nil
	}
	matches, err := r.groupRepo.ListSharePoolMatches(ctx, account.Platform, account.UpstreamPlan)
	if err != nil {
		return 0, err
	}
	n := 0
	for i := range matches {
		g := &matches[i]
		if isSharePoolCandidate(g) && plansMatchForSharePool(account.UpstreamPlan, g.UpstreamPlan) {
			n++
		}
	}
	return n, nil
}

// currentManagedIDs 过滤当前绑定中仍属 managed 谓词的链接。
// 谓词：该 owner 的 private 组，或 当前 is_share_pool==true 且同 platform。
func (r *AccountGroupRecomputer) currentManagedIDs(ctx context.Context, account *Account) (map[int64]struct{}, error) {
	out := make(map[int64]struct{})
	if account == nil || len(account.GroupIDs) == 0 {
		return out, nil
	}
	groupsByID := make(map[int64]*Group, len(account.Groups))
	for _, g := range account.Groups {
		if g != nil {
			groupsByID[g.ID] = g
		}
	}
	missing := make([]int64, 0)
	for _, id := range account.GroupIDs {
		if _, ok := groupsByID[id]; !ok {
			missing = append(missing, id)
		}
	}
	if len(missing) > 0 {
		loaded, err := r.groupRepo.ListByIDs(ctx, missing)
		if err != nil {
			return nil, err
		}
		for i := range loaded {
			g := loaded[i]
			groupsByID[g.ID] = &g
		}
	}

	for _, id := range account.GroupIDs {
		g := groupsByID[id]
		if g == nil {
			continue
		}
		if isCurrentlyManagedLink(account, g) {
			out[id] = struct{}{}
		}
	}
	return out, nil
}

func isCurrentlyManagedLink(account *Account, g *Group) bool {
	if account == nil || g == nil {
		return false
	}
	if account.OwnerUserID != nil && IsPrivateGroupNameForUser(g.Name, *account.OwnerUserID) {
		return true
	}
	if g.IsSharePool && g.Platform == account.Platform {
		return true
	}
	return false
}

func (r *AccountGroupRecomputer) loadAccountForRecompute(ctx context.Context, account *Account) (*Account, error) {
	// 若 GroupIDs 已齐全可直接用；卸链后或列表扫描常缺 groups 边，统一 GetByID 最稳。
	if account.ID <= 0 {
		return account, nil
	}
	// 若调用方刚 RemoveGroups 后传入的 GroupIDs 可能陈旧，始终 reload。
	fresh, err := r.accountRepo.GetByID(ctx, account.ID)
	if err != nil {
		return nil, err
	}
	if fresh == nil {
		return account, nil
	}
	return fresh, nil
}

func uniqueAccountsByID(sets ...[]*Account) []*Account {
	seen := make(map[int64]struct{})
	out := make([]*Account, 0)
	for _, set := range sets {
		for _, acc := range set {
			if acc == nil || acc.ID <= 0 {
				continue
			}
			if _, ok := seen[acc.ID]; ok {
				continue
			}
			seen[acc.ID] = struct{}{}
			out = append(out, acc)
		}
	}
	return out
}

// sharePoolRelevantFieldsChanged 判断组变更是否需要触发 OnSharePoolGroupChange。
func sharePoolRelevantFieldsChanged(before, after *Group) bool {
	if before == nil && after == nil {
		return false
	}
	if before == nil || after == nil {
		return true
	}
	return before.IsSharePool != after.IsSharePool ||
		before.UpstreamPlan != after.UpstreamPlan ||
		before.Platform != after.Platform ||
		before.Status != after.Status ||
		before.IsExclusive != after.IsExclusive ||
		before.Name != after.Name
}

// cloneGroupShallow 浅拷贝组身份/共享池相关字段，避免 Update 原地改写污染 before 快照。
func cloneGroupShallow(g *Group) *Group {
	if g == nil {
		return nil
	}
	cp := *g
	return &cp
}
