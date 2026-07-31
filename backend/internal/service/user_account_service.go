package service

import (
	"context"
	"fmt"
	"log/slog"
	"strings"

	dbent "github.com/Wei-Shaw/sub2api/ent"
	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
	"github.com/Wei-Shaw/sub2api/internal/pkg/pagination"
)

// User-owned accounts feature errors (PR3).
var (
	ErrUserOwnedAccountsDisabled = infraerrors.Forbidden(
		"USER_OWNED_ACCOUNTS_DISABLED",
		"user-owned accounts feature is disabled",
	)
	ErrUserAccountLimitExceeded = infraerrors.BadRequest(
		"USER_ACCOUNT_LIMIT_EXCEEDED",
		"user-owned account limit exceeded",
	)
	ErrUserAccountNotFound = infraerrors.NotFound(
		"ACCOUNT_NOT_FOUND",
		"account not found",
	)
	ErrUserAccountInvalidPlatform = infraerrors.BadRequest(
		"INVALID_PLATFORM",
		"platform is not allowed for user-owned accounts",
	)
	ErrUserAccountInvalidType = infraerrors.BadRequest(
		"INVALID_ACCOUNT_TYPE",
		"account type is not allowed for this platform",
	)
	ErrUserAccountInvalidVisibility = infraerrors.BadRequest(
		"INVALID_VISIBILITY",
		"visibility must be private or public",
	)
	ErrUserAccountInvalidStatus = infraerrors.BadRequest(
		"INVALID_STATUS",
		"status must be active, inactive, or disabled",
	)
)

// Visibility reason codes returned on Create/SetVisibility (ephemeral; not a DB column).
const (
	VisibilityReasonPlanProbeFailed      = "plan_probe_failed"
	VisibilityReasonPlanProbeUnsupported = "plan_probe_unsupported"
	VisibilityReasonPlanEmpty            = "plan_empty"
	// VisibilityReasonNoSharePoolMatch 请求 public 但无匹配共享池组（含空档位无空档位共享池）。
	VisibilityReasonNoSharePoolMatch = "no_share_pool_match"
)

// UserAccountService 用户自建上游账号 CRUD + visibility（与 Admin API 分离）。
type UserAccountService interface {
	List(ctx context.Context, userID int64, params pagination.PaginationParams) ([]Account, int64, error)
	Get(ctx context.Context, userID, accountID int64) (*Account, error)
	Create(ctx context.Context, userID int64, input *CreateUserAccountInput) (*Account, error)
	Update(ctx context.Context, userID, accountID int64, input *UpdateUserAccountInput) (*Account, error)
	Delete(ctx context.Context, userID, accountID int64) error
	SetVisibility(ctx context.Context, userID, accountID int64, visibility string) (*Account, error)
	// SetSchedulable 仅允许操作本人账号。
	SetSchedulable(ctx context.Context, userID, accountID int64, schedulable bool) (*Account, error)
	// BatchDeleteOwned 仅删除本人账号；非本人 ID 记入 FailedIDs（404 语义，不暴露存在性）。
	BatchDeleteOwned(ctx context.Context, userID int64, ids []int64) (*UserAccountBatchDeleteResult, error)
	// BatchSetSchedulableOwned 批量设置本人账号可调度状态。
	BatchSetSchedulableOwned(ctx context.Context, userID int64, ids []int64, schedulable bool) (*UserAccountBatchSchedulableResult, error)
}

// CreateUserAccountInput 用户创建账号输入（K17）。
type CreateUserAccountInput struct {
	Name        string
	Platform    string
	Type        string
	Credentials map[string]any
	// Extra 非敏感扩展字段（如 privacy_mode、load_code_assist 摘要）；与 credentials 分离
	Extra map[string]any
	Visibility string // private|public；最终可能因探测失败强制 private
	// Concurrency 账号并发上限；<=0 时 normalizeAccountConcurrency 按平台默认处理
	Concurrency int
}

// UpdateUserAccountInput 用户更新白名单（K15 扩展）。
// 不接受 group_ids / proxy_id（handler 不绑定；服务层亦不处理）。
type UpdateUserAccountInput struct {
	Name           *string
	Notes          *string
	Credentials    map[string]any
	Status         *string  // active|inactive|disabled（inactive/disabled 统一存 inactive）
	Concurrency    *int     // 账号并发；nil 不改
	Schedulable    *bool    // nil 不改
	RateMultiplier *float64 // nil 不改；>=0
	// Extra 浅合并到现有 extra（禁止 owner 相关键）
	Extra map[string]any
}

// UserAccountBatchDeleteResult 批量删除结果。
type UserAccountBatchDeleteResult struct {
	DeletedIDs []int64 `json:"deleted_ids"`
	FailedIDs  []int64 `json:"failed_ids"`
	Deleted    int     `json:"deleted"`
	Failed     int     `json:"failed"`
}

// UserAccountBatchSchedulableResult 批量设置 schedulable 结果。
type UserAccountBatchSchedulableResult struct {
	SuccessIDs []int64 `json:"success_ids"`
	FailedIDs  []int64 `json:"failed_ids"`
	Success    int     `json:"success"`
	Failed     int     `json:"failed"`
}

// UserAccountListFilters 预留扩展（v1 List 仅 owner=me）。
type UserAccountListFilters struct {
	Platform   string
	Status     string
	Visibility string
}

type userAccountService struct {
	accountRepo          AccountRepository
	groupRepo            GroupRepository
	privateGroups        PrivateGroupProvisioner
	recomputer           *AccountGroupRecomputer
	settingSvc           *SettingService
	entClient            *dbent.Client
	privacyClientFactory PrivacyClientFactory // OpenAI 隐私 API（可空；空则跳过 Ensure）
}

// NewUserAccountService 构造用户自建账号服务。
// privacyFactory 可为 nil（测试）；生产应注入 ImpersonateChrome 工厂以便 OpenAI 隐私兜底。
func NewUserAccountService(
	accountRepo AccountRepository,
	groupRepo GroupRepository,
	privateGroups PrivateGroupProvisioner,
	settingSvc *SettingService,
	entClient *dbent.Client,
	privacyFactory PrivacyClientFactory,
) UserAccountService {
	var recomputer *AccountGroupRecomputer
	if accountRepo != nil && groupRepo != nil {
		recomputer = NewAccountGroupRecomputer(accountRepo, groupRepo)
	}
	return &userAccountService{
		accountRepo:          accountRepo,
		groupRepo:            groupRepo,
		privateGroups:        privateGroups,
		recomputer:           recomputer,
		settingSvc:           settingSvc,
		entClient:            entClient,
		privacyClientFactory: privacyFactory,
	}
}

func (s *userAccountService) List(ctx context.Context, userID int64, params pagination.PaginationParams) ([]Account, int64, error) {
	if err := s.requireFeatureEnabled(ctx); err != nil {
		return nil, 0, err
	}
	if userID <= 0 {
		return nil, 0, ErrUserAccountNotFound
	}
	if params.Page <= 0 {
		params.Page = 1
	}
	if params.PageSize <= 0 {
		params.PageSize = 20
	}
	accounts, result, err := s.accountRepo.ListByOwnerUserID(ctx, userID, params)
	if err != nil {
		return nil, 0, err
	}
	total := int64(0)
	if result != nil {
		total = result.Total
	}
	return accounts, total, nil
}

func (s *userAccountService) Get(ctx context.Context, userID, accountID int64) (*Account, error) {
	if err := s.requireFeatureEnabled(ctx); err != nil {
		return nil, err
	}
	return s.getOwned(ctx, userID, accountID)
}

// Create 按 K17：Tx1 Ensure+insert private+bind → AfterCommit → 事务外 probe → 可选 Tx2 升 public。
func (s *userAccountService) Create(ctx context.Context, userID int64, input *CreateUserAccountInput) (*Account, error) {
	if err := s.requireFeatureEnabled(ctx); err != nil {
		return nil, err
	}
	if input == nil {
		return nil, ErrAccountNilInput
	}
	if userID <= 0 {
		return nil, ErrUserAccountNotFound
	}

	platform := strings.ToLower(strings.TrimSpace(input.Platform))
	accountType := strings.TrimSpace(input.Type)
	name := strings.TrimSpace(input.Name)
	if name == "" {
		return nil, infraerrors.BadRequest("INVALID_NAME", "name is required")
	}
	if !IsAllowedQuotaPlatform(platform) {
		return nil, ErrUserAccountInvalidPlatform
	}
	if !isUserAllowedAccountType(platform, accountType) {
		return nil, ErrUserAccountInvalidType
	}

	requestedVisibility := strings.ToLower(strings.TrimSpace(input.Visibility))
	if requestedVisibility == "" {
		requestedVisibility = VisibilityPrivate
	}
	if requestedVisibility != VisibilityPrivate && requestedVisibility != VisibilityPublic {
		return nil, ErrUserAccountInvalidVisibility
	}

	// openai apikey 禁止 public（设计 type 表）
	forcePrivateReason := ""
	if requestedVisibility == VisibilityPublic && !userAccountTypeSupportsPublicProbe(platform, accountType) {
		requestedVisibility = VisibilityPrivate
		forcePrivateReason = VisibilityReasonPlanProbeUnsupported
	}

	if err := NormalizeHeaderOverrideCredentials(input.Credentials); err != nil {
		return nil, err
	}

	// 事务外预检软上限（Tx1 内再检）
	if err := s.requireUnderLimit(ctx, userID); err != nil {
		return nil, err
	}

	ownerID := userID
	extra := cloneAnyMap(input.Extra)
	if extra == nil {
		extra = map[string]any{}
	}
	// credentials 中若带 privacy_mode（兼容），提升到 extra
	if pm, ok := input.Credentials["privacy_mode"].(string); ok {
		if strings.TrimSpace(pm) != "" {
			if _, exists := extra["privacy_mode"]; !exists {
				extra["privacy_mode"] = strings.TrimSpace(pm)
			}
		}
	}
	// tier_id 可写入 load_code_assist 摘要供列表 Pro 角标
	if tid, ok := input.Credentials["tier_id"].(string); ok {
		tid = strings.TrimSpace(tid)
		if tid != "" {
			if _, exists := extra["load_code_assist"]; !exists {
				extra["load_code_assist"] = map[string]any{
					"currentTier": map[string]any{"id": tid},
					"paidTier":    map[string]any{"id": tid},
				}
			}
		}
	}

	account := &Account{
		Name:        name,
		Platform:    platform,
		Type:        accountType,
		Credentials: cloneAnyMap(input.Credentials),
		Extra:       extra,
		Concurrency: normalizeAccountConcurrency(platform, accountType, input.Concurrency),
		Priority:    0,
		Status:      StatusActive,
		Schedulable: true,
		OwnerUserID: &ownerID,
		Visibility:  VisibilityPrivate, // K17：一律先 private
	}
	// 勿把 privacy_mode 留在 credentials
	if account.Credentials != nil {
		delete(account.Credentials, "privacy_mode")
	}
	account.AutoPauseOnExpired = true

	var provisionResult *ProvisionResult
	var created *Account

	runTx1 := func(opCtx context.Context) error {
		if err := s.requireUnderLimit(opCtx, userID); err != nil {
			return err
		}
		if s.privateGroups == nil {
			return fmt.Errorf("private group provisioner not configured")
		}
		group, result, err := s.privateGroups.EnsurePrivateGroupForPlatform(opCtx, userID, platform)
		if err != nil {
			return err
		}
		provisionResult = result
		if group == nil || group.ID <= 0 {
			return fmt.Errorf("ensure private group returned empty group for platform=%s", platform)
		}

		if err := s.accountRepo.Create(opCtx, account); err != nil {
			return err
		}
		// 首次 private 绑：AddGroups（与 Recompute 等价且不依赖 recompute 读回）
		if err := s.accountRepo.AddGroups(opCtx, account.ID, []int64{group.ID}); err != nil {
			return err
		}
		account.GroupIDs = []int64{group.ID}
		created = account
		return nil
	}

	if s.entClient != nil {
		tx, err := s.entClient.Tx(ctx)
		if err != nil {
			return nil, err
		}
		defer func() { _ = tx.Rollback() }()
		opCtx := dbent.NewTxContext(ctx, tx)
		if err := runTx1(opCtx); err != nil {
			return nil, err
		}
		if err := tx.Commit(); err != nil {
			return nil, err
		}
	} else {
		// 测试 / 无 ent：无事务路径
		if err := runTx1(ctx); err != nil {
			return nil, err
		}
	}

	if s.privateGroups != nil && provisionResult != nil && provisionResult.NeedsAfterCommit {
		s.privateGroups.AfterCommit(ctx, provisionResult)
	}

	// OAuth 隐私兜底：创建时未带 privacy_mode 则异步补写（与管理端 CreateAccount 对齐）
	if created != nil && created.Type == AccountTypeOAuth {
		switch created.Platform {
		case PlatformAntigravity:
			go func(accID int64) {
				defer func() {
					if r := recover(); r != nil {
						slog.Error("user_account_antigravity_privacy_panic", "account_id", accID, "recover", r)
					}
				}()
				acc, err := s.accountRepo.GetByID(context.Background(), accID)
				if err != nil || acc == nil {
					return
				}
				ensureUserOwnedAntigravityPrivacy(context.Background(), s.accountRepo, acc)
			}(created.ID)
		case PlatformOpenAI:
			go func(accID int64) {
				defer func() {
					if r := recover(); r != nil {
						slog.Error("user_account_openai_privacy_panic", "account_id", accID, "recover", r)
					}
				}()
				acc, err := s.accountRepo.GetByID(context.Background(), accID)
				if err != nil || acc == nil {
					return
				}
				ensureUserOwnedOpenAIPrivacy(context.Background(), s.accountRepo, s.privacyClientFactory, acc)
			}(created.ID)
		}
	}

	// 事务外 best-effort：从 credentials 提取 plan 并 ApplyProbedPlan
	visibilityReason := forcePrivateReason
	if err := ApplyProbedPlanFromCredentials(ctx, s.accountRepo, s.recomputer, created); err != nil {
		slog.Warn("user_account_create_probe_apply_failed",
			"account_id", created.ID,
			"user_id", userID,
			"error", err.Error(),
		)
	}
	// 重新加载以拿到 ApplyProbedPlan 写回的 UpstreamPlan / GroupIDs
	if reloaded, err := s.accountRepo.GetByID(ctx, created.ID); err == nil && reloaded != nil {
		created = reloaded
	}

	// 请求 public：尝试升 public；无匹配共享池则强制 private（含空档位空对空）
	wantPublic := strings.EqualFold(strings.TrimSpace(input.Visibility), VisibilityPublic) && forcePrivateReason == ""
	if wantPublic {
		if err := s.promoteToPublic(ctx, created); err != nil {
			slog.Error("user_account_create_promote_public_failed",
				"account_id", created.ID,
				"user_id", userID,
				"error", err.Error(),
			)
			visibilityReason = VisibilityReasonPlanProbeFailed
		} else if reloaded, err := s.accountRepo.GetByID(ctx, created.ID); err == nil && reloaded != nil {
			created = reloaded
		}
		if reason, demoteErr := s.demotePublicIfNoSharePoolMatch(ctx, created); demoteErr != nil {
			slog.Error("user_account_create_demote_no_pool_failed",
				"account_id", created.ID,
				"error", demoteErr.Error(),
			)
		} else if reason != "" {
			visibilityReason = reason
			if reloaded, err := s.accountRepo.GetByID(ctx, created.ID); err == nil && reloaded != nil {
				created = reloaded
			}
		}
	}

	if visibilityReason != "" {
		created.VisibilityReason = visibilityReason
	}

	slog.Info("user_account_create",
		"account_id", created.ID,
		"user_id", userID,
		"platform", platform,
		"type", accountType,
		"visibility", created.Visibility,
		"upstream_plan", created.UpstreamPlan,
		"visibility_reason", visibilityReason,
	)
	return created, nil
}

func (s *userAccountService) promoteToPublic(ctx context.Context, account *Account) error {
	if account == nil {
		return nil
	}
	account.Visibility = VisibilityPublic
	if err := s.accountRepo.Update(ctx, account); err != nil {
		return err
	}
	if s.recomputer != nil {
		if err := s.recomputer.RecomputeManagedLinks(ctx, account); err != nil {
			// 回滚 visibility 到 private，避免 public 无池
			account.Visibility = VisibilityPrivate
			_ = s.accountRepo.Update(ctx, account)
			return err
		}
	}
	return nil
}

func (s *userAccountService) Update(ctx context.Context, userID, accountID int64, input *UpdateUserAccountInput) (*Account, error) {
	if err := s.requireFeatureEnabled(ctx); err != nil {
		return nil, err
	}
	if input == nil {
		return nil, ErrAccountNilInput
	}
	account, err := s.getOwned(ctx, userID, accountID)
	if err != nil {
		return nil, err
	}

	if input.Name != nil {
		name := strings.TrimSpace(*input.Name)
		if name == "" {
			return nil, infraerrors.BadRequest("INVALID_NAME", "name cannot be empty")
		}
		account.Name = name
	}
	if input.Notes != nil {
		account.Notes = normalizeAccountNotes(input.Notes)
	}
	if input.Status != nil {
		st := strings.ToLower(strings.TrimSpace(*input.Status))
		// 兼容前端 admin 表单 inactive 与早期 user API disabled
		switch st {
		case StatusActive:
			account.Status = StatusActive
		case StatusDisabled, "inactive":
			account.Status = "inactive"
		default:
			return nil, ErrUserAccountInvalidStatus
		}
	}
	if len(input.Credentials) > 0 {
		account.Credentials = MergePreservingSensitiveCreds(account.Credentials, input.Credentials)
		if err := NormalizeHeaderOverrideCredentials(account.Credentials); err != nil {
			return nil, err
		}
	}
	if input.Concurrency != nil {
		account.Concurrency = normalizeAccountConcurrency(account.Platform, account.Type, *input.Concurrency)
		if account.Concurrency < 1 {
			account.Concurrency = 1
		}
	}
	if input.Schedulable != nil {
		account.Schedulable = *input.Schedulable
	}
	if input.RateMultiplier != nil {
		if *input.RateMultiplier < 0 {
			return nil, infraerrors.BadRequest("INVALID_RATE_MULTIPLIER", "rate_multiplier must be >= 0")
		}
		rm := *input.RateMultiplier
		account.RateMultiplier = &rm
	}
	if len(input.Extra) > 0 {
		if account.Extra == nil {
			account.Extra = map[string]any{}
		}
		for k, v := range input.Extra {
			key := strings.TrimSpace(k)
			if key == "" {
				continue
			}
			// 禁止经 extra 注入归属相关键
			lk := strings.ToLower(key)
			if lk == "owner_user_id" || lk == "owner" || lk == "owner_id" {
				continue
			}
			account.Extra[key] = v
		}
	}

	if err := s.accountRepo.Update(ctx, account); err != nil {
		return nil, err
	}

	// credentials 变更后 best-effort 同步 plan（K16）
	if len(input.Credentials) > 0 {
		if err := ApplyProbedPlanFromCredentials(ctx, s.accountRepo, s.recomputer, account); err != nil {
			slog.Warn("user_account_update_probe_apply_failed",
				"account_id", account.ID,
				"error", err.Error(),
			)
		}
	}

	return s.accountRepo.GetByID(ctx, account.ID)
}

func (s *userAccountService) Delete(ctx context.Context, userID, accountID int64) error {
	if err := s.requireFeatureEnabled(ctx); err != nil {
		return err
	}
	if _, err := s.getOwned(ctx, userID, accountID); err != nil {
		return err
	}
	// 与 Admin DeleteAccount 同仓库路径（硬删 account_groups + 软删账号）
	return s.accountRepo.Delete(ctx, accountID)
}

func (s *userAccountService) SetSchedulable(ctx context.Context, userID, accountID int64, schedulable bool) (*Account, error) {
	if err := s.requireFeatureEnabled(ctx); err != nil {
		return nil, err
	}
	if _, err := s.getOwned(ctx, userID, accountID); err != nil {
		return nil, err
	}
	if err := s.accountRepo.SetSchedulable(ctx, accountID, schedulable); err != nil {
		return nil, err
	}
	return s.accountRepo.GetByID(ctx, accountID)
}

func (s *userAccountService) BatchDeleteOwned(ctx context.Context, userID int64, ids []int64) (*UserAccountBatchDeleteResult, error) {
	if err := s.requireFeatureEnabled(ctx); err != nil {
		return nil, err
	}
	if userID <= 0 {
		return nil, ErrUserAccountNotFound
	}
	ids = dedupePositiveIDs(ids)
	if len(ids) == 0 {
		return nil, infraerrors.BadRequest("INVALID_IDS", "ids is required")
	}
	if len(ids) > 100 {
		return nil, infraerrors.BadRequest("INVALID_IDS", "ids must contain at most 100 items")
	}
	out := &UserAccountBatchDeleteResult{
		DeletedIDs: make([]int64, 0, len(ids)),
		FailedIDs:  make([]int64, 0),
	}
	for _, id := range ids {
		if err := s.Delete(ctx, userID, id); err != nil {
			out.FailedIDs = append(out.FailedIDs, id)
			continue
		}
		out.DeletedIDs = append(out.DeletedIDs, id)
	}
	out.Deleted = len(out.DeletedIDs)
	out.Failed = len(out.FailedIDs)
	return out, nil
}

func (s *userAccountService) BatchSetSchedulableOwned(ctx context.Context, userID int64, ids []int64, schedulable bool) (*UserAccountBatchSchedulableResult, error) {
	if err := s.requireFeatureEnabled(ctx); err != nil {
		return nil, err
	}
	if userID <= 0 {
		return nil, ErrUserAccountNotFound
	}
	ids = dedupePositiveIDs(ids)
	if len(ids) == 0 {
		return nil, infraerrors.BadRequest("INVALID_IDS", "ids is required")
	}
	if len(ids) > 100 {
		return nil, infraerrors.BadRequest("INVALID_IDS", "ids must contain at most 100 items")
	}
	out := &UserAccountBatchSchedulableResult{
		SuccessIDs: make([]int64, 0, len(ids)),
		FailedIDs:  make([]int64, 0),
	}
	for _, id := range ids {
		if _, err := s.SetSchedulable(ctx, userID, id, schedulable); err != nil {
			out.FailedIDs = append(out.FailedIDs, id)
			continue
		}
		out.SuccessIDs = append(out.SuccessIDs, id)
	}
	out.Success = len(out.SuccessIDs)
	out.Failed = len(out.FailedIDs)
	return out, nil
}

func dedupePositiveIDs(ids []int64) []int64 {
	if len(ids) == 0 {
		return nil
	}
	seen := make(map[int64]struct{}, len(ids))
	out := make([]int64, 0, len(ids))
	for _, id := range ids {
		if id <= 0 {
			continue
		}
		if _, ok := seen[id]; ok {
			continue
		}
		seen[id] = struct{}{}
		out = append(out, id)
	}
	return out
}

// SetVisibility private ↔ public；升 public 时 Ensure 私有组；无匹配共享池则强制 private。
func (s *userAccountService) SetVisibility(ctx context.Context, userID, accountID int64, visibility string) (*Account, error) {
	if err := s.requireFeatureEnabled(ctx); err != nil {
		return nil, err
	}
	visibility = strings.ToLower(strings.TrimSpace(visibility))
	if visibility != VisibilityPrivate && visibility != VisibilityPublic {
		return nil, ErrUserAccountInvalidVisibility
	}

	account, err := s.getOwned(ctx, userID, accountID)
	if err != nil {
		return nil, err
	}

	reason := ""
	if visibility == VisibilityPublic {
		// 类型不支持 public
		if !userAccountTypeSupportsPublicProbe(account.Platform, account.Type) {
			visibility = VisibilityPrivate
			reason = VisibilityReasonPlanProbeUnsupported
		} else {
			// Ensure 私有组（K18：SetVisibility→public 显式 Ensure）
			if s.privateGroups != nil {
				var prov *ProvisionResult
				group, result, ensErr := s.privateGroups.EnsurePrivateGroupForPlatform(ctx, userID, account.Platform)
				if ensErr != nil {
					return nil, ensErr
				}
				prov = result
				_ = group
				if prov != nil && prov.NeedsAfterCommit {
					s.privateGroups.AfterCommit(ctx, prov)
				}
			}
			// 若 plan 空：尝试从 credentials 提取（成功则按有档位匹配）
			if strings.TrimSpace(account.UpstreamPlan) == "" {
				_ = ApplyProbedPlanFromCredentials(ctx, s.accountRepo, s.recomputer, account)
				if reloaded, gerr := s.accountRepo.GetByID(ctx, account.ID); gerr == nil && reloaded != nil {
					account = reloaded
				}
			}
			// 空档位仍允许尝试 public，靠空==空匹配共享池；无匹配则下方 demote
		}
	}

	if visibility == VisibilityPublic && reason == "" {
		// 先校验是否存在可匹配共享池，避免假 public
		n, cerr := s.countSharePoolMatches(ctx, account)
		if cerr != nil {
			return nil, cerr
		}
		if n == 0 {
			visibility = VisibilityPrivate
			if strings.TrimSpace(account.UpstreamPlan) == "" {
				reason = VisibilityReasonPlanEmpty
			} else {
				reason = VisibilityReasonNoSharePoolMatch
			}
		}
	}

	account.Visibility = visibility
	if err := s.accountRepo.Update(ctx, account); err != nil {
		return nil, err
	}
	if s.recomputer != nil {
		if err := s.recomputer.RecomputeManagedLinks(ctx, account); err != nil {
			return nil, fmt.Errorf("recompute after set visibility: %w", err)
		}
	}

	out, err := s.accountRepo.GetByID(ctx, account.ID)
	if err != nil {
		return nil, err
	}
	if reason != "" {
		out.VisibilityReason = reason
	}
	slog.Info("user_account_visibility",
		"account_id", out.ID,
		"user_id", userID,
		"visibility", out.Visibility,
		"visibility_reason", reason,
	)
	return out, nil
}

// demotePublicIfNoSharePoolMatch 若当前为 public 且无可匹配共享池，强制 private 并 recompute。
func (s *userAccountService) demotePublicIfNoSharePoolMatch(ctx context.Context, account *Account) (string, error) {
	if account == nil || account.Visibility != VisibilityPublic {
		return "", nil
	}
	n, err := s.countSharePoolMatches(ctx, account)
	if err != nil {
		return "", err
	}
	if n > 0 {
		return "", nil
	}
	account.Visibility = VisibilityPrivate
	if err := s.accountRepo.Update(ctx, account); err != nil {
		return "", err
	}
	if s.recomputer != nil {
		if err := s.recomputer.RecomputeManagedLinks(ctx, account); err != nil {
			return "", err
		}
	}
	if strings.TrimSpace(account.UpstreamPlan) == "" {
		return VisibilityReasonPlanEmpty, nil
	}
	return VisibilityReasonNoSharePoolMatch, nil
}

func (s *userAccountService) countSharePoolMatches(ctx context.Context, account *Account) (int, error) {
	if s == nil || account == nil {
		return 0, nil
	}
	if s.recomputer != nil {
		return s.recomputer.CountSharePoolMatches(ctx, account)
	}
	if s.groupRepo == nil {
		return 0, nil
	}
	matches, err := s.groupRepo.ListSharePoolMatches(ctx, account.Platform, account.UpstreamPlan)
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

func (s *userAccountService) getOwned(ctx context.Context, userID, accountID int64) (*Account, error) {
	if accountID <= 0 || userID <= 0 {
		return nil, ErrUserAccountNotFound
	}
	account, err := s.accountRepo.GetByID(ctx, accountID)
	if err != nil {
		// 统一 404，避免枚举
		if infraerrors.IsNotFound(err) {
			return nil, ErrUserAccountNotFound
		}
		return nil, err
	}
	if account == nil || account.OwnerUserID == nil || *account.OwnerUserID != userID {
		return nil, ErrUserAccountNotFound
	}
	return account, nil
}

func (s *userAccountService) requireFeatureEnabled(ctx context.Context) error {
	if s.settingSvc == nil {
		// 测试场景：无 setting 视为关闭，强制显式 mock
		return ErrUserOwnedAccountsDisabled
	}
	if !s.settingSvc.IsUserOwnedAccountsEnabled(ctx) {
		return ErrUserOwnedAccountsDisabled
	}
	return nil
}

func (s *userAccountService) requireUnderLimit(ctx context.Context, userID int64) error {
	max := DefaultMaxUserOwnedAccounts
	if s.settingSvc != nil {
		max = s.settingSvc.GetMaxUserOwnedAccounts(ctx)
	}
	// <=0：禁止创建（设置解析层通常已 clamp，此处双保险）
	if max <= 0 {
		return ErrUserAccountLimitExceeded
	}
	n, err := s.accountRepo.CountActiveOwned(ctx, userID)
	if err != nil {
		return err
	}
	if n >= max {
		return ErrUserAccountLimitExceeded
	}
	return nil
}

// isUserAllowedAccountType 用户创建平台/type 白名单。
func isUserAllowedAccountType(platform, accountType string) bool {
	platform = strings.ToLower(strings.TrimSpace(platform))
	accountType = strings.TrimSpace(accountType)
	switch platform {
	case PlatformOpenAI:
		return accountType == AccountTypeOAuth || accountType == AccountTypeAPIKey
	case PlatformGrok, PlatformAntigravity:
		return accountType == AccountTypeOAuth
	case PlatformAnthropic:
		return accountType == AccountTypeOAuth || accountType == AccountTypeSetupToken || accountType == AccountTypeAPIKey
	case PlatformGemini:
		return accountType == AccountTypeOAuth || accountType == AccountTypeAPIKey
	default:
		return false
	}
}

// userAccountTypeSupportsPublicProbe 是否允许请求 public（探测路径）。
// openai apikey 禁止 public。
func userAccountTypeSupportsPublicProbe(platform, accountType string) bool {
	platform = strings.ToLower(strings.TrimSpace(platform))
	accountType = strings.TrimSpace(accountType)
	if platform == PlatformOpenAI && accountType == AccountTypeAPIKey {
		return false
	}
	// 允许的 type 均可尝试；anthropic/gemini probe 可能失败并降 private
	return isUserAllowedAccountType(platform, accountType)
}

// ensureUserOwnedAntigravityPrivacy 用户自建号补写 privacy_mode（与 admin EnsureAntigravityPrivacy 同探测逻辑，无 proxy）。
func ensureUserOwnedAntigravityPrivacy(ctx context.Context, repo AccountRepository, account *Account) {
	if account == nil || repo == nil {
		return
	}
	if account.Platform != PlatformAntigravity || account.Type != AccountTypeOAuth {
		return
	}
	if account.Extra != nil {
		if existing, ok := account.Extra["privacy_mode"].(string); ok && existing == AntigravityPrivacySet {
			return
		}
	}
	token, _ := account.Credentials["access_token"].(string)
	if strings.TrimSpace(token) == "" {
		return
	}
	projectID, _ := account.Credentials["project_id"].(string)
	mode := setAntigravityPrivacy(ctx, token, projectID, "")
	if mode == "" {
		return
	}
	if err := repo.UpdateExtra(ctx, account.ID, map[string]any{"privacy_mode": mode}); err != nil {
		slog.Warn("user_account_antigravity_privacy_update_failed",
			"account_id", account.ID,
			"error", err.Error(),
		)
		return
	}
	applyAntigravityPrivacyMode(account, mode)
}

// ensureUserOwnedOpenAIPrivacy 用户自建 OpenAI OAuth 号补写 privacy_mode（与 admin EnsureOpenAIPrivacy 对齐，无 proxy）。
func ensureUserOwnedOpenAIPrivacy(ctx context.Context, repo AccountRepository, factory PrivacyClientFactory, account *Account) {
	if account == nil || repo == nil {
		return
	}
	if account.IsCredentialShadow() {
		return
	}
	if account.Platform != PlatformOpenAI || account.Type != AccountTypeOAuth {
		return
	}
	if factory == nil {
		return
	}
	if shouldSkipOpenAIPrivacyEnsure(account.Extra) {
		return
	}
	token, _ := account.Credentials["access_token"].(string)
	if strings.TrimSpace(token) == "" {
		return
	}
	mode := disableOpenAITraining(ctx, factory, token, "")
	if mode == "" {
		return
	}
	if err := repo.UpdateExtra(ctx, account.ID, map[string]any{"privacy_mode": mode}); err != nil {
		slog.Warn("user_account_openai_privacy_update_failed",
			"account_id", account.ID,
			"error", err.Error(),
		)
		return
	}
	if account.Extra == nil {
		account.Extra = make(map[string]any)
	}
	account.Extra["privacy_mode"] = mode
}

func cloneAnyMap(in map[string]any) map[string]any {
	if in == nil {
		return map[string]any{}
	}
	out := make(map[string]any, len(in))
	for k, v := range in {
		out[k] = v
	}
	return out
}
