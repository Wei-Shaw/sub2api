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
		"status must be active or disabled",
	)
)

// Visibility reason codes returned on Create/SetVisibility (ephemeral; not a DB column).
const (
	VisibilityReasonPlanProbeFailed      = "plan_probe_failed"
	VisibilityReasonPlanProbeUnsupported = "plan_probe_unsupported"
	VisibilityReasonPlanEmpty            = "plan_empty"
)

// UserAccountService 用户自建上游账号 CRUD + visibility（与 Admin API 分离）。
type UserAccountService interface {
	List(ctx context.Context, userID int64, params pagination.PaginationParams) ([]Account, int64, error)
	Get(ctx context.Context, userID, accountID int64) (*Account, error)
	Create(ctx context.Context, userID int64, input *CreateUserAccountInput) (*Account, error)
	Update(ctx context.Context, userID, accountID int64, input *UpdateUserAccountInput) (*Account, error)
	Delete(ctx context.Context, userID, accountID int64) error
	SetVisibility(ctx context.Context, userID, accountID int64, visibility string) (*Account, error)
}

// CreateUserAccountInput 用户创建账号输入（K17）。
type CreateUserAccountInput struct {
	Name        string
	Platform    string
	Type        string
	Credentials map[string]any
	Visibility  string // private|public；最终可能因探测失败强制 private
}

// UpdateUserAccountInput 用户更新白名单（K15）。
type UpdateUserAccountInput struct {
	Name        *string
	Notes       *string
	Credentials map[string]any
	Status      *string // active|disabled only
}

// UserAccountListFilters 预留扩展（v1 List 仅 owner=me）。
type UserAccountListFilters struct {
	Platform   string
	Status     string
	Visibility string
}

type userAccountService struct {
	accountRepo   AccountRepository
	groupRepo     GroupRepository
	privateGroups PrivateGroupProvisioner
	recomputer    *AccountGroupRecomputer
	settingSvc    *SettingService
	entClient     *dbent.Client
}

// NewUserAccountService 构造用户自建账号服务。
func NewUserAccountService(
	accountRepo AccountRepository,
	groupRepo GroupRepository,
	privateGroups PrivateGroupProvisioner,
	settingSvc *SettingService,
	entClient *dbent.Client,
) UserAccountService {
	var recomputer *AccountGroupRecomputer
	if accountRepo != nil && groupRepo != nil {
		recomputer = NewAccountGroupRecomputer(accountRepo, groupRepo)
	}
	return &userAccountService{
		accountRepo:   accountRepo,
		groupRepo:     groupRepo,
		privateGroups: privateGroups,
		recomputer:    recomputer,
		settingSvc:    settingSvc,
		entClient:     entClient,
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
	account := &Account{
		Name:        name,
		Platform:    platform,
		Type:        accountType,
		Credentials: cloneAnyMap(input.Credentials),
		Extra:       map[string]any{},
		Concurrency: normalizeAccountConcurrency(platform, accountType, 0),
		Priority:    0,
		Status:      StatusActive,
		Schedulable: true,
		OwnerUserID: &ownerID,
		Visibility:  VisibilityPrivate, // K17：一律先 private
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

	// 若用户请求 public 且 plan 非空 → Tx2 升 public + recompute
	wantPublic := strings.EqualFold(strings.TrimSpace(input.Visibility), VisibilityPublic) && forcePrivateReason == ""
	plan := strings.TrimSpace(created.UpstreamPlan)
	if wantPublic && plan != "" {
		if err := s.promoteToPublic(ctx, created); err != nil {
			slog.Error("user_account_create_promote_public_failed",
				"account_id", created.ID,
				"user_id", userID,
				"error", err.Error(),
			)
			// 账号已存在且保持 private；返回当前态 + reason（禁止假 public）
			visibilityReason = VisibilityReasonPlanProbeFailed
		} else if reloaded, err := s.accountRepo.GetByID(ctx, created.ID); err == nil && reloaded != nil {
			created = reloaded
		}
	} else if wantPublic && plan == "" {
		visibilityReason = VisibilityReasonPlanProbeFailed
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
		if st != StatusActive && st != StatusDisabled {
			return nil, ErrUserAccountInvalidStatus
		}
		account.Status = st
	}
	if len(input.Credentials) > 0 {
		account.Credentials = MergePreservingSensitiveCreds(account.Credentials, input.Credentials)
		if err := NormalizeHeaderOverrideCredentials(account.Credentials); err != nil {
			return nil, err
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

// SetVisibility private ↔ public；升 public 时 Ensure 私有组；无 plan 强制 private。
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
			// 若 plan 空：尝试从 credentials 提取
			if strings.TrimSpace(account.UpstreamPlan) == "" {
				_ = ApplyProbedPlanFromCredentials(ctx, s.accountRepo, s.recomputer, account)
				if reloaded, gerr := s.accountRepo.GetByID(ctx, account.ID); gerr == nil && reloaded != nil {
					account = reloaded
				}
			}
			if strings.TrimSpace(account.UpstreamPlan) == "" {
				visibility = VisibilityPrivate
				reason = VisibilityReasonPlanEmpty
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
