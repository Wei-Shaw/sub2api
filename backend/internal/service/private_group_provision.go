package service

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"time"

	dbent "github.com/Wei-Shaw/sub2api/ent"
	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
	"github.com/Wei-Shaw/sub2api/internal/pkg/logger"
)

// PrivateGroupProvisioner 在用户创建/删除路径上供给与撤销私有专属平台分组。
// 不依赖 AdminService / AuthService，避免循环依赖。
type PrivateGroupProvisioner interface {
	// ProvisionPrivatePlatformGroups 为 role=user 幂等补齐私有组 + allowed + 订阅。
	// 错误向上返回（fail-closed）。admin 角色 no-op。
	ProvisionPrivatePlatformGroups(ctx context.Context, userID int64) (*ProvisionResult, error)
	// RevokePrivatePlatformGroups 软删该用户全部 private-{userId}-* 组（DeleteCascade）。
	RevokePrivatePlatformGroups(ctx context.Context, userID int64) (*RevokeResult, error)
	// AfterCommit 在外层事务成功提交后补发 outbox 与缓存失效。
	AfterCommit(ctx context.Context, result *ProvisionResult)
	// AfterRevokeCommit 在外层事务成功提交后补发删除 outbox。
	AfterRevokeCommit(ctx context.Context, result *RevokeResult)
}

// ProvisionResult 描述一次供给结果，供调用方 post-commit 处理。
type ProvisionResult struct {
	UserID           int64
	CreatedGroupIDs  []int64
	EnsuredGroupIDs  []int64
	NeedsAfterCommit bool
}

// RevokeResult 描述一次撤销结果。
type RevokeResult struct {
	UserID           int64
	DeletedGroupIDs  []int64
	NeedsAfterCommit bool
}

// subscriptionEnsure 抽象 EnsureSubscriptionWithExpiresAt，便于单测 stub。
type subscriptionEnsure interface {
	EnsureSubscriptionWithExpiresAt(
		ctx context.Context,
		userID, groupID int64,
		expiresAt time.Time,
		notes string,
		deferCacheInvalidation bool,
	) (*UserSubscription, error)
	InvalidateSubCache(userID, groupID int64)
}

type privateGroupProvisioner struct {
	groupRepo      GroupRepository
	userRepo       UserRepository
	subEnsure      subscriptionEnsure
	settingService *SettingService
	billingCache   *BillingCacheService
}

// NewPrivateGroupProvisioner 构造私有组供给器。
func NewPrivateGroupProvisioner(
	groupRepo GroupRepository,
	userRepo UserRepository,
	subEnsure *SubscriptionService,
	settingService *SettingService,
	billingCache *BillingCacheService,
) PrivateGroupProvisioner {
	return &privateGroupProvisioner{
		groupRepo:      groupRepo,
		userRepo:       userRepo,
		subEnsure:      subEnsure,
		settingService: settingService,
		billingCache:   billingCache,
	}
}

// privateGroupPlatforms 返回私有组覆盖平台（AllowedQuotaPlatforms 唯一权威源）。
func privateGroupPlatforms() []string {
	return append([]string(nil), AllowedQuotaPlatforms...)
}

// PrivateGroupName 生成标准私有组名：private-{userId}-{platform}。
func PrivateGroupName(userID int64, platform string) string {
	return fmt.Sprintf("private-%d-%s", userID, platform)
}

// privateGroupNameRegexp 匹配严格命名；platform 白名单来自 AllowedQuotaPlatforms。
var privateGroupNameRegexp = func() *regexp.Regexp {
	parts := make([]string, 0, len(AllowedQuotaPlatforms))
	for _, p := range AllowedQuotaPlatforms {
		parts = append(parts, regexp.QuoteMeta(p))
	}
	// 例：^private-(\d+)-(anthropic|openai|gemini|antigravity|grok)$
	return regexp.MustCompile(`^private-(\d+)-(` + strings.Join(parts, "|") + `)$`)
}()

// IsPrivateGroupName 判断 name 是否为严格格式的私有专属组名。
func IsPrivateGroupName(name string) bool {
	return privateGroupNameRegexp.MatchString(strings.TrimSpace(name))
}

// ParsePrivateGroupName 解析 private-{userId}-{platform}；非法返回 ok=false。
func ParsePrivateGroupName(name string) (userID int64, platform string, ok bool) {
	m := privateGroupNameRegexp.FindStringSubmatch(strings.TrimSpace(name))
	if len(m) != 3 {
		return 0, "", false
	}
	id, err := strconv.ParseInt(m[1], 10, 64)
	if err != nil || id <= 0 {
		return 0, "", false
	}
	return id, m[2], true
}

// IsPrivateGroupNameForUser 判断 name 是否为指定用户的私有组。
func IsPrivateGroupNameForUser(name string, userID int64) bool {
	uid, _, ok := ParsePrivateGroupName(name)
	return ok && uid == userID
}

func (p *privateGroupProvisioner) ProvisionPrivatePlatformGroups(ctx context.Context, userID int64) (*ProvisionResult, error) {
	result := &ProvisionResult{UserID: userID}
	if p == nil || userID <= 0 {
		return result, nil
	}
	if p.groupRepo == nil || p.userRepo == nil || p.subEnsure == nil {
		return nil, errors.New("private group provisioner not fully configured")
	}

	user, err := p.userRepo.GetByID(ctx, userID)
	if err != nil {
		return nil, fmt.Errorf("get user for private provision: %w", err)
	}
	if user.Role != RoleUser {
		logger.LegacyPrintf("service.private_group", "private_group_provision skip non-user: user_id=%d role=%s", userID, user.Role)
		return result, nil
	}

	inOuterTx := dbent.TxFromContext(ctx) != nil
	result.NeedsAfterCommit = inOuterTx

	expiresAt := time.Now()
	if p.settingService != nil {
		if at, ok := p.settingService.ResolvePrivateGroupExpiresAt(ctx); ok {
			expiresAt = at
		}
	}

	for _, platform := range privateGroupPlatforms() {
		group, created, err := p.ensurePrivateGroup(ctx, userID, platform)
		if err != nil {
			logger.LegacyPrintf("service.private_group", "private_group_provision failed: user_id=%d platform=%s err=%v", userID, platform, err)
			return nil, err
		}
		if created {
			result.CreatedGroupIDs = append(result.CreatedGroupIDs, group.ID)
		}
		result.EnsuredGroupIDs = append(result.EnsuredGroupIDs, group.ID)

		if err := p.userRepo.AddGroupToAllowedGroups(ctx, userID, group.ID); err != nil {
			return nil, fmt.Errorf("add private group to allowed: user_id=%d group_id=%d: %w", userID, group.ID, err)
		}

		notes := fmt.Sprintf("private platform group provision user_id=%d platform=%s", userID, platform)
		if _, err := p.subEnsure.EnsureSubscriptionWithExpiresAt(ctx, userID, group.ID, expiresAt, notes, inOuterTx); err != nil {
			return nil, fmt.Errorf("ensure private subscription: user_id=%d group_id=%d: %w", userID, group.ID, err)
		}
	}

	logger.LegacyPrintf("service.private_group", "private_group_provision ok: user_id=%d created=%d ensured=%d expires_at=%s",
		userID, len(result.CreatedGroupIDs), len(result.EnsuredGroupIDs), expiresAt.Format(time.RFC3339))
	return result, nil
}

func (p *privateGroupProvisioner) ensurePrivateGroup(ctx context.Context, userID int64, platform string) (*Group, bool, error) {
	name := PrivateGroupName(userID, platform)
	existing, err := p.groupRepo.GetByName(ctx, name)
	if err == nil && existing != nil {
		return existing, false, nil
	}
	if err != nil && !errors.Is(err, ErrGroupNotFound) {
		return nil, false, err
	}

	group := buildPrivateGroup(userID, platform)
	if err := p.groupRepo.Create(ctx, group); err != nil {
		// 并发唯一冲突：重读
		if existing, getErr := p.groupRepo.GetByName(ctx, name); getErr == nil && existing != nil {
			return existing, false, nil
		}
		return nil, false, err
	}
	return group, true, nil
}

// buildPrivateGroup 构造私有组字段；倍率必须显式赋值，避免 createGroupRecord 把 Go 零值写成 0。
func buildPrivateGroup(userID int64, platform string) *Group {
	return &Group{
		Name:                         PrivateGroupName(userID, platform),
		Description:                  fmt.Sprintf("private platform group for user_id=%d platform=%s", userID, platform),
		Platform:                     platform,
		SubscriptionType:             SubscriptionTypeSubscription,
		IsExclusive:                  true,
		RateMultiplier:               1.0,
		ImageRateMultiplier:          1.0,
		VideoRateMultiplier:          1.0,
		PeakRateMultiplier:           1.0,
		PeakRateEnabled:              false,
		BatchImageDiscountMultiplier: defaultBatchImageDiscountMultiplier, // 0.5
		BatchImageHoldMultiplier:     defaultBatchImageHoldMultiplier,     // 0.6
		DefaultValidityDays:          365,
		Status:                       StatusActive,
		SortOrder:                    0,
		AllowImageGeneration:         defaultAllowImageGenerationForPlatform(platform),
		MCPXMLInject:                 true,
	}
}

func (p *privateGroupProvisioner) RevokePrivatePlatformGroups(ctx context.Context, userID int64) (*RevokeResult, error) {
	result := &RevokeResult{UserID: userID}
	if p == nil || userID <= 0 || p.groupRepo == nil {
		return result, nil
	}

	inOuterTx := dbent.TxFromContext(ctx) != nil
	result.NeedsAfterCommit = inOuterTx

	// 尽量撤销全部平台，避免中途失败留下 private 孤儿组污染调度 ListActive。
	// 单平台失败 log 后 continue，循环结束后返回 firstErr。
	var firstErr error
	for _, platform := range privateGroupPlatforms() {
		name := PrivateGroupName(userID, platform)
		g, err := p.groupRepo.GetByName(ctx, name)
		if err != nil {
			if errors.Is(err, ErrGroupNotFound) {
				continue
			}
			if firstErr == nil {
				firstErr = fmt.Errorf("get private group for revoke: %s: %w", name, err)
			}
			logger.LegacyPrintf("service.private_group", "private_group_revoke get failed: user_id=%d name=%s err=%v", userID, name, err)
			continue
		}
		if _, err := p.groupRepo.DeleteCascade(ctx, g.ID); err != nil {
			if firstErr == nil {
				firstErr = fmt.Errorf("delete cascade private group %d: %w", g.ID, err)
			}
			logger.LegacyPrintf("service.private_group", "private_group_revoke delete failed: user_id=%d group_id=%d err=%v", userID, g.ID, err)
			continue
		}
		result.DeletedGroupIDs = append(result.DeletedGroupIDs, g.ID)

		// 非外层 tx 时 DeleteCascade 已自管 commit/outbox；同步失效缓存
		if !inOuterTx {
			p.invalidateGroupCaches(userID, g.ID)
		}
	}

	if firstErr != nil {
		logger.LegacyPrintf("service.private_group", "private_group_revoke partial: user_id=%d deleted=%d err=%v",
			userID, len(result.DeletedGroupIDs), firstErr)
		return result, firstErr
	}
	logger.LegacyPrintf("service.private_group", "private_group_revoke ok: user_id=%d deleted=%d",
		userID, len(result.DeletedGroupIDs))
	return result, nil
}

func (p *privateGroupProvisioner) AfterCommit(ctx context.Context, result *ProvisionResult) {
	if p == nil || result == nil || !result.NeedsAfterCommit {
		return
	}
	for _, gid := range result.CreatedGroupIDs {
		if err := p.groupRepo.EnqueueGroupChanged(ctx, gid); err != nil {
			logger.LegacyPrintf("service.private_group", "enqueue group changed after provision failed: group_id=%d err=%v", gid, err)
		}
	}
	for _, gid := range result.EnsuredGroupIDs {
		p.invalidateGroupCaches(result.UserID, gid)
	}
}

func (p *privateGroupProvisioner) AfterRevokeCommit(ctx context.Context, result *RevokeResult) {
	if p == nil || result == nil || !result.NeedsAfterCommit {
		return
	}
	for _, gid := range result.DeletedGroupIDs {
		if err := p.groupRepo.EnqueueGroupChanged(ctx, gid); err != nil {
			logger.LegacyPrintf("service.private_group", "enqueue group changed after revoke failed: group_id=%d err=%v", gid, err)
		}
		p.invalidateGroupCaches(result.UserID, gid)
	}
}

func (p *privateGroupProvisioner) invalidateGroupCaches(userID, groupID int64) {
	if p.subEnsure != nil {
		p.subEnsure.InvalidateSubCache(userID, groupID)
	}
	if p.billingCache != nil {
		go func() {
			cacheCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			_ = p.billingCache.InvalidateSubscription(cacheCtx, userID, groupID)
		}()
	}
}

// mergePrivateAllowedGroupIDs 将用户现有 private 组 ID 并入 allowed 列表（UpdateUser 防 Modal 抹除）。
func mergePrivateAllowedGroupIDs(requested []int64, existingUserGroups []Group, userID int64) []int64 {
	seen := make(map[int64]struct{}, len(requested)+len(AllowedQuotaPlatforms))
	out := make([]int64, 0, len(requested)+len(AllowedQuotaPlatforms))
	for _, id := range requested {
		if id <= 0 {
			continue
		}
		if _, ok := seen[id]; ok {
			continue
		}
		seen[id] = struct{}{}
		out = append(out, id)
	}
	for i := range existingUserGroups {
		g := existingUserGroups[i]
		if !IsPrivateGroupNameForUser(g.Name, userID) {
			continue
		}
		if _, ok := seen[g.ID]; ok {
			continue
		}
		seen[g.ID] = struct{}{}
		out = append(out, g.ID)
	}
	return out
}

// filterOutPrivateGroups 从列表中移除 private 组（service 层兜底）。
func filterOutPrivateGroups(groups []Group) []Group {
	if len(groups) == 0 {
		return groups
	}
	out := make([]Group, 0, len(groups))
	for i := range groups {
		if IsPrivateGroupName(groups[i].Name) {
			continue
		}
		out = append(out, groups[i])
	}
	return out
}

// validatePrivateGroupIdentityUpdate 私有组禁止改名 / platform / subscription_type / is_exclusive 降级。
// 允许同值提交（no-op）；允许倍率、限额、status、绑账号等运营字段（由调用方继续处理）。
func validatePrivateGroupIdentityUpdate(group *Group, input *UpdateGroupInput) error {
	if group == nil || input == nil || !IsPrivateGroupName(group.Name) {
		return nil
	}
	if input.Name != "" && input.Name != group.Name {
		return infraerrors.BadRequest("PRIVATE_GROUP_IDENTITY_LOCKED", "private group name cannot be changed")
	}
	if input.Platform != "" && input.Platform != group.Platform {
		return infraerrors.BadRequest("PRIVATE_GROUP_IDENTITY_LOCKED", "private group platform cannot be changed")
	}
	if input.SubscriptionType != "" && input.SubscriptionType != group.SubscriptionType {
		return infraerrors.BadRequest("PRIVATE_GROUP_IDENTITY_LOCKED", "private group subscription_type cannot be changed")
	}
	// 仅禁止 true→false 降级；保持 true 或从 false 升 true 允许
	if input.IsExclusive != nil && !*input.IsExclusive && group.IsExclusive {
		return infraerrors.BadRequest("PRIVATE_GROUP_IDENTITY_LOCKED", "private group cannot demote is_exclusive")
	}
	return nil
}

// noopPrivateGroupProvisioner 测试用 no-op。
type noopPrivateGroupProvisioner struct{}

func (noopPrivateGroupProvisioner) ProvisionPrivatePlatformGroups(context.Context, int64) (*ProvisionResult, error) {
	return &ProvisionResult{}, nil
}
func (noopPrivateGroupProvisioner) RevokePrivatePlatformGroups(context.Context, int64) (*RevokeResult, error) {
	return &RevokeResult{}, nil
}
func (noopPrivateGroupProvisioner) AfterCommit(context.Context, *ProvisionResult)   {}
func (noopPrivateGroupProvisioner) AfterRevokeCommit(context.Context, *RevokeResult) {}
