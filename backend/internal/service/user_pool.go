package service

import (
	"context"
	"net/http"
	"time"

	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
)

// ── Pool business errors ─────────────────────────────────────────────────────

var (
	ErrPoolNotFound     = infraerrors.NotFound("POOL_NOT_FOUND", "pool not found")
	ErrPoolNameConflict = infraerrors.Conflict("POOL_NAME_CONFLICT", "pool name already exists")

	ErrGroupGrantNotFound  = infraerrors.NotFound("POOL_GROUP_GRANT_NOT_FOUND", "pool group grant not found")
	ErrDuplicateGrantGroup = infraerrors.New(http.StatusUnprocessableEntity, "POOL_GROUP_GRANT_DUPLICATE_GROUP", "duplicate group id in pool grants")

	ErrPoolGrantRateInvalid = infraerrors.New(http.StatusUnprocessableEntity, "POOL_GRANT_RATE_INVALID", "pool grant rate multiplier must be null or greater than zero")
	ErrPoolGrantRPMInvalid  = infraerrors.New(http.StatusUnprocessableEntity, "POOL_GRANT_RPM_INVALID", "pool grant rpm override must be null or greater than or equal to zero")

	ErrPoolGrantPublicGroupNotAllowed = infraerrors.New(http.StatusUnprocessableEntity, "POOL_GRANT_PUBLIC_GROUP_NOT_ALLOWED", "pool grants cannot target public groups")
	ErrPoolGrantGroupDisabled         = infraerrors.New(http.StatusUnprocessableEntity, "POOL_GRANT_GROUP_DISABLED", "pool grants can only target active groups")

	ErrPoolMatchedTooManyUsers = infraerrors.BadRequest("POOL_MATCHED_TOO_MANY_USERS", "user_pool: matched too many users, refine your filter (max 100000)")
)

// ── Pool domain types ─────────────────────────────────────────────────────────

// Pool 代表一个用户池，包含一批用户，并向这批用户授予分组访问权限。
type Pool struct {
	ID          int64
	Name        string
	Description string
	Status      string // active | disabled
	CreatedAt   time.Time
	UpdatedAt   time.Time
	DeletedAt   *time.Time
}

// PoolMember 代表用户池中的一名成员。
type PoolMember struct {
	PoolID    int64
	UserID    int64
	Email     string
	Username  string
	CreatedAt time.Time
}

// PoolGroupGrant 代表用户池对某个分组的授权，可选附带 rate_multiplier 和 rpm_override 覆盖。
type PoolGroupGrant struct {
	PoolID         int64
	GroupID        int64
	RateMultiplier *float64 // nil=继承分组默认值
	RPMOverride    *int     // nil=继承分组 rpm_limit; 0=不限制; >0=覆盖值
	CreatedAt      time.Time
	UpdatedAt      time.Time
}

// ── Pool repository options ──────────────────────────────────────────────────

// ListPoolsOptions 分页与过滤参数。
type ListPoolsOptions struct {
	Page   int
	Limit  int
	Status string // "" = 全部; "active" | "disabled"
}

// ListMembersOptions 成员分页参数。
type ListMembersOptions struct {
	Page  int
	Limit int
}

// ── UserPoolRepository interface ─────────────────────────────────────────────

// UserPoolRepository 提供 user_pools / user_pool_members / user_pool_group_grants 表的读写接口。
// 所有写操作应在调用方事务内执行（ctx 携带活跃 Ent 事务时通过 txAwareSQLExecutor 自动参与）。
type UserPoolRepository interface {
	Create(ctx context.Context, pool Pool) (Pool, error)
	List(ctx context.Context, opts ListPoolsOptions) ([]Pool, int, error)
	GetByID(ctx context.Context, id int64) (Pool, error)
	Update(ctx context.Context, id int64, pool Pool) (Pool, error)
	SoftDelete(ctx context.Context, id int64) error

	AddMembers(ctx context.Context, poolID int64, userIDs []int64) (added []int64, skipped []int64, err error)
	RemoveMembers(ctx context.Context, poolID int64, userIDs []int64) (removed []int64, err error)
	ListMembers(ctx context.Context, poolID int64, opts ListMembersOptions) ([]PoolMember, int, error)
	// ListAllMemberIDs returns all user IDs for a pool, bypassing the normPage 200-row cap.
	// Uses cursor-style pagination internally (batch size 1000) to handle arbitrarily large pools.
	ListAllMemberIDs(ctx context.Context, poolID int64) ([]int64, error)
	MemberCount(ctx context.Context, poolID int64) (int, error)

	ReplaceGroupGrants(ctx context.Context, poolID int64, grants []PoolGroupGrant) error
	DeleteGroupGrant(ctx context.Context, poolID, groupID int64) error
	ListGroupGrants(ctx context.Context, poolID int64) ([]PoolGroupGrant, error)

	GetUserPools(ctx context.Context, userID int64) ([]Pool, error)
	// GetUserPoolsBatch returns each user's pool list in one query.
	GetUserPoolsBatch(ctx context.Context, userIDs []int64) (map[int64][]Pool, error)
	// ListGroupGrantsBatch returns grants of multiple pools in one query.
	ListGroupGrantsBatch(ctx context.Context, poolIDs []int64) (map[int64][]PoolGroupGrant, error)
}

// ── EffectiveGroupProfile ────────────────────────────────────────────────────

// EffectiveGroupProfile 描述用户在某个分组的有效权限、费率和 RPM 来源。
// 用于管理员展示 grant_effective_groups / allowed_groups_detail 以及 Pool 写路径的 diff 判断。
//
// 计算规则：
//  1. permission_source: active standard group 先看 user_allowed_groups（direct），
//     仅在 flag=true 时看 active Pool grant（pool）；
//     只有 is_exclusive=false 且无显式 grant 的 active public standard 分组才标 public。
//  2. rate_source: user_group_rate_multipliers.rate_multiplier 非 NULL 才是 direct；
//     否则仅当 active standard + flag=true 时选最小 active Pool grant rate；
//     若 rate_multiplier 非 NULL 则标 pool，否则 group_default。
//  3. rpm_source: 同 rate_source 规则，对应 rpm_override。
//  4. direct + Pool 同 Group 时，permission_source 可以是 direct，
//     但 rate_source/rpm_source 仍可能是 pool 或 group_default。
type EffectiveGroupProfile struct {
	GroupID                 int64
	PermissionSource        string // direct | pool | public | subscription
	PermissionPoolID        *int64
	PermissionPoolName      *string
	RateSource              string // direct | pool | group_default
	RatePoolID              *int64
	RatePoolName            *string
	EffectiveRateMultiplier float64
	RPMSource               string // direct | pool | group_default
	RPMPoolID               *int64
	RPMPoolName             *string
	EffectiveRPMOverride    *int // nil=继承 groups.rpm_limit; 0=不限制; >0=覆盖值
}
