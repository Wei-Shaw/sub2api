// Package account contains the port interfaces (repository abstractions) for
// the account bounded context. The Account aggregate, its value types, and its
// domain errors live in internal/domain; this package only owns the
// persistence/read port contracts.
//
// TODO(account-isp): Repository is intentionally a single fat interface
// mirroring the legacy service.AccountRepository method set verbatim so the 67
// existing test stubs keep compiling. A future PR should split it along read /
// write / schedulability / quota / OAuth-refresh-pager axes.
package account

import (
	"context"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/domain"
	"github.com/Wei-Shaw/sub2api/internal/pkg/pagination"
)

// OAuthRefreshPageOptions describes one bounded, cursor-stable scan of OAuth
// accounts. Candidate platforms are supplied by TokenRefreshService's refresher
// registry so repository eligibility cannot drift from registered providers.
type OAuthRefreshPageOptions struct {
	Platforms            []string
	AfterID              int64
	Limit                int
	ActiveOnly           bool
	IncludeSetupToken    bool
	RequireRefreshToken  bool
	ExcludeRetryCooldown bool
}

// OAuthRefreshCandidatePage keeps cursor metadata from the raw SQL ID page.
// Hydration may legitimately lose a concurrently deleted row, but callers can
// still advance past the raw page without truncating or duplicating the scan.
type OAuthRefreshCandidatePage struct {
	Accounts    []domain.Account
	NextAfterID int64
	HasMore     bool
}

// OAuthRefreshCandidatePager is intentionally narrower than Repository.
// Production refresh cycles fail closed when the repository does not implement
// this bounded contract instead of silently falling back to an unpaged scan.
type OAuthRefreshCandidatePager interface {
	ListOAuthRefreshCandidatePage(ctx context.Context, options OAuthRefreshPageOptions) (*OAuthRefreshCandidatePage, error)
}

// Repository persists Account aggregates. Method set is kept verbatim from the
// legacy service.AccountRepository so existing repository stubs in tests stay
// satisfied; signatures use domain.* types.
type Repository interface {
	Create(ctx context.Context, account *domain.Account) error
	GetByID(ctx context.Context, id int64) (*domain.Account, error)
	// GetByIDs fetches accounts by IDs in a single query.
	// It should return all accounts found (missing IDs are ignored).
	GetByIDs(ctx context.Context, ids []int64) ([]*domain.Account, error)
	// ExistsByID 检查账号是否存在，仅返回布尔值，用于删除前的轻量级存在性检查
	ExistsByID(ctx context.Context, id int64) (bool, error)
	// GetByCRSAccountID finds an account previously synced from CRS.
	// Returns (nil, nil) if not found.
	GetByCRSAccountID(ctx context.Context, crsAccountID string) (*domain.Account, error)
	// FindByExtraField 根据 extra 字段中的键值对查找账号
	FindByExtraField(ctx context.Context, key string, value any) ([]domain.Account, error)
	// ListCRSAccountIDs returns a map of crs_account_id -> local account ID
	// for all accounts that have been synced from CRS.
	ListCRSAccountIDs(ctx context.Context) (map[string]int64, error)
	Update(ctx context.Context, account *domain.Account) error
	Delete(ctx context.Context, id int64) error

	List(ctx context.Context, params pagination.PaginationParams) ([]domain.Account, *pagination.PaginationResult, error)
	ListWithFilters(ctx context.Context, params pagination.PaginationParams, platform, accountType, status, search string, groupID int64, privacyMode string) ([]domain.Account, *pagination.PaginationResult, error)
	// ListAllWithFilters 返回符合过滤条件的全部账号（不分页），用于账号列表页
	// 计算 OpenAI 调度分数的过滤范围池。
	ListAllWithFilters(ctx context.Context, platform, accountType, status, search string, groupID int64, privacyMode string) ([]domain.Account, error)
	ListByGroup(ctx context.Context, groupID int64) ([]domain.Account, error)
	ListActive(ctx context.Context) ([]domain.Account, error)
	ListByPlatform(ctx context.Context, platform string) ([]domain.Account, error)

	UpdateLastUsed(ctx context.Context, id int64) error
	BatchUpdateLastUsed(ctx context.Context, updates map[int64]time.Time) error
	SetError(ctx context.Context, id int64, errorMsg string) error
	ClearError(ctx context.Context, id int64) error
	SetSchedulable(ctx context.Context, id int64, schedulable bool) error
	AutoPauseExpiredAccounts(ctx context.Context, now time.Time) (int64, error)
	BindGroups(ctx context.Context, accountID int64, groupIDs []int64) error

	ListSchedulable(ctx context.Context) ([]domain.Account, error)
	ListSchedulableByGroupID(ctx context.Context, groupID int64) ([]domain.Account, error)
	ListSchedulableByPlatform(ctx context.Context, platform string) ([]domain.Account, error)
	ListSchedulableByGroupIDAndPlatform(ctx context.Context, groupID int64, platform string) ([]domain.Account, error)
	ListSchedulableByPlatforms(ctx context.Context, platforms []string) ([]domain.Account, error)
	ListSchedulableByGroupIDAndPlatforms(ctx context.Context, groupID int64, platforms []string) ([]domain.Account, error)
	ListSchedulableUngroupedByPlatform(ctx context.Context, platform string) ([]domain.Account, error)
	ListSchedulableUngroupedByPlatforms(ctx context.Context, platforms []string) ([]domain.Account, error)
	// ListModelAvailabilityCandidates returns accounts that are enabled by
	// persistent configuration (active + schedulable) for model-support
	// diagnosis. It deliberately does not filter transient runtime state such
	// as rate-limit, overload, temporary-unschedulable, or expiry windows.
	// When groupID is nil, includeGrouped controls whether the query scans all
	// matching accounts or only accounts without a group binding.
	ListModelAvailabilityCandidates(ctx context.Context, groupID *int64, platforms []string, includeGrouped bool) ([]domain.Account, error)

	SetRateLimited(ctx context.Context, id int64, resetAt time.Time) error
	SetModelRateLimit(ctx context.Context, id int64, scope string, resetAt time.Time, reason ...string) error
	SetOverloaded(ctx context.Context, id int64, until time.Time) error
	SetTempUnschedulable(ctx context.Context, id int64, until time.Time, reason string) error
	ClearTempUnschedulable(ctx context.Context, id int64) error
	ClearRateLimit(ctx context.Context, id int64) error
	ClearAntigravityQuotaScopes(ctx context.Context, id int64) error
	ClearModelRateLimits(ctx context.Context, id int64) error
	UpdateSessionWindow(ctx context.Context, id int64, start, end *time.Time, status string) error
	// UpdateSessionWindowEnd 仅更新 5h 窗口的结束时间，不动 start / status。
	// 用于 active poll 拿到新 ResetsAt 后回写，避免覆盖请求路径上记录的 status。
	UpdateSessionWindowEnd(ctx context.Context, id int64, end time.Time) error
	UpdateExtra(ctx context.Context, id int64, updates map[string]any) error
	BulkUpdate(ctx context.Context, ids []int64, updates AccountBulkUpdate) (int64, error)
	// IncrementQuotaUsed 原子递增 API Key 账号的配额用量（总/日/周）
	IncrementQuotaUsed(ctx context.Context, id int64, amount float64) error
	// ResetQuotaUsed 重置 API Key 账号所有维度的配额用量为 0
	ResetQuotaUsed(ctx context.Context, id int64) error
	// RevertProxyFallback 将账号的 proxy_id 切回 proxy_fallback_origin_id，并清空 origin 字段。
	// 仅当 proxy_fallback_origin_id IS NOT NULL 时更新，否则视为账号不存在（返回 ErrAccountNotFound）。
	RevertProxyFallback(ctx context.Context, accountID int64) error
	// ListShadowsByParent 返回指定父账号的影子账号；当前实现仅查 quota_dimension='spark'（唯一预设）。
	// ⚠️ 新增影子维度时：须更新此函数（或新增维度专用列举），并检查所有调用点（级联删除/一母一影校验/type 守卫），否则会静默漏掉新维度。
	ListShadowsByParent(ctx context.Context, parentID int64) ([]*domain.Account, error)
}

// DuplicateRepository captures the account-duplication write capability.
type DuplicateRepository interface {
	// CreateWithAccountGroups atomically persists an account, its exact group priorities,
	// and the scheduler outbox event for the new routing snapshot.
	CreateWithAccountGroups(ctx context.Context, account *domain.Account, groups []domain.AccountGroup) error
}

// AdminRepository makes the account-duplication write capability an explicit
// construction dependency without forcing read-only gateway test doubles to
// implement it.
type AdminRepository interface {
	Repository
	DuplicateRepository
}

// AccountBulkUpdate describes the fields that can be updated in a bulk operation.
// Nil pointers mean "do not change".
type AccountBulkUpdate struct {
	Name           *string
	ProxyID        *int64
	Concurrency    *int
	Priority       *int
	RateMultiplier *float64
	LoadFactor     *int
	Status         *string
	Schedulable    *bool
	Credentials    map[string]any
	Extra          map[string]any
	ProbeEnabled   *bool
}
