package service

import (
	"context"
	"sort"
	"strings"

	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
)

// 内部 API RPC 账本相关错误。
var (
	// ErrBalanceInsufficient 余额不足（不透支）。
	ErrBalanceInsufficient = infraerrors.Conflict("INSUFFICIENT_BALANCE", "insufficient balance")
	// ErrOverRefund 累计退款超过原扣金额。
	ErrOverRefund = infraerrors.Conflict("OVER_REFUND", "refund exceeds original deduction")
	// ErrOriginalDeductNotFound 凭原流水冲销时找不到对应的原扣流水（或不属于本 app）。
	ErrOriginalDeductNotFound = infraerrors.NotFound("ORIGINAL_DEDUCT_NOT_FOUND", "original deduction not found")
	// ErrLedgerRequestConflict 相同 request_id 但参数与首次不一致。
	ErrLedgerRequestConflict = infraerrors.Conflict("LEDGER_REQUEST_CONFLICT", "request_id reused with different parameters")
	// ErrInnerAPIAppNotFound 接入方不存在。
	ErrInnerAPIAppNotFound = infraerrors.NotFound("INNER_API_APP_NOT_FOUND", "inner api app not found")
	// ErrInnerAPIAppUnauthenticated 接入方鉴权失败（不存在 / 停用 / token 不匹配，统一错误）。
	ErrInnerAPIAppUnauthenticated = infraerrors.Unauthorized("INNER_API_APP_UNAUTHENTICATED", "invalid inner api app credentials")
	// ErrInnerAPIAppPermissionDenied 接入方没有调用目标方法的权限。
	ErrInnerAPIAppPermissionDenied  = infraerrors.Forbidden("INNER_API_APP_PERMISSION_DENIED", "inner api app permission denied")
	ErrInnerAPIAppInvalidPermission = infraerrors.BadRequest("INNER_API_APP_INVALID_PERMISSION", "invalid inner api app permission")
)

// 账本流水类型。
const (
	BalanceLedgerKindDeduct int8 = 1
	BalanceLedgerKindRefund int8 = 2
)

// InnerAPIApp 接入方身份领域模型。
type InnerAPIApp struct {
	ID           int64
	AppID        string
	AppName      string
	Enabled      bool
	TokenVersion int
	Permissions  []string
}

const (
	InnerAPIPermissionBalanceWrite   = "balance:write"
	InnerAPIPermissionBalanceRead    = "balance:read"
	InnerAPIPermissionMaterialsRead  = "materials:read"
	InnerAPIPermissionMaterialsWrite = "materials:write"
)

var validInnerAPIPermissions = map[string]struct{}{
	InnerAPIPermissionBalanceWrite:   {},
	InnerAPIPermissionBalanceRead:    {},
	InnerAPIPermissionMaterialsRead:  {},
	InnerAPIPermissionMaterialsWrite: {},
}

// ValidateInnerAPIPermissions 校验并规范化 app 权限，拒绝未知权限和重复项。
func ValidateInnerAPIPermissions(permissions []string) ([]string, error) {
	seen := make(map[string]struct{}, len(permissions))
	out := make([]string, 0, len(permissions))
	for _, permission := range permissions {
		permission = strings.TrimSpace(permission)
		if _, ok := validInnerAPIPermissions[permission]; !ok {
			return nil, ErrInnerAPIAppInvalidPermission
		}
		if _, ok := seen[permission]; ok {
			continue
		}
		seen[permission] = struct{}{}
		out = append(out, permission)
	}
	sort.Strings(out)
	return out, nil
}

func (app *InnerAPIApp) HasPermission(permission string) bool {
	if app == nil {
		return false
	}
	for _, granted := range app.Permissions {
		if granted == permission {
			return true
		}
	}
	return false
}

// InnerAPIAppRepository 接入方身份仓储。
type InnerAPIAppRepository interface {
	GetByAppID(ctx context.Context, appID string) (*InnerAPIApp, error)
	Create(ctx context.Context, app *InnerAPIApp) (*InnerAPIApp, error)
	SetEnabled(ctx context.Context, appID string, enabled bool) error
	SetPermissions(ctx context.Context, appID string, permissions []string) error
	// BumpTokenVersion 自增 token_version 并返回新值，用于刷新 token（旧 token 失效）。
	BumpTokenVersion(ctx context.Context, appID string) (int, error)
	Delete(ctx context.Context, appID string) error
	List(ctx context.Context) ([]*InnerAPIApp, error)
}

// AppLedgerStats 某接入方的累计扣费统计。
type AppLedgerStats struct {
	AppID         string  `json:"app_id"`
	TotalDeducted float64 `json:"total_deducted"` // 累计扣费（kind=deduct 之和）
	TotalRefunded float64 `json:"total_refunded"` // 累计退费（kind=refund 之和）
	NetDeducted   float64 `json:"net_deducted"`   // 净扣费 = 扣 - 退
	DeductCount   int64   `json:"deduct_count"`
	RefundCount   int64   `json:"refund_count"`
}

// LedgerDeductCommand 一次扣费请求。
type LedgerDeductCommand struct {
	AppID           string
	RequestID       string // 幂等键
	UserID          int64
	Amount          float64
	Description     string // 必填，扣费原因
	Extra           string // jsonb 文本，接入方自存
	OrganizationID  *int64
	PayerUserID     int64
	BalanceSource   string
	AuthzGeneration int64
}

// LedgerDeductResult 扣费结果。
type LedgerDeductResult struct {
	Applied         bool    // false = 幂等重放
	BalanceAfter    float64 // 扣后余额
	OrganizationID  *int64
	PayerUserID     int64
	BalanceSource   string
	AuthzGeneration int64
}

// LedgerRefundCommand 一次退费请求（部分退）。
type LedgerRefundCommand struct {
	AppID             string
	RefundRequestID   string // 本笔退款幂等键
	OriginalRequestID string // 被冲销的原扣 request_id
	Amount            float64
	Description       string // 必填，退费原因
	Extra             string
}

// LedgerRefundResult 退费结果。
type LedgerRefundResult struct {
	Applied         bool    // false = 幂等重放
	UserID          int64   // 被退费用户（由原扣流水推导，供缓存失效用）
	BalanceAfter    float64 // 退后余额
	RefundedTotal   float64 // 原扣累计已退
	OrganizationID  *int64
	PayerUserID     int64
	BalanceSource   string
	AuthzGeneration int64
}

type BillingBalanceResult struct {
	Balance         float64
	OrganizationID  *int64
	PayerUserID     int64
	BalanceSource   string
	AuthzGeneration int64
}

// BalanceLedgerRepository 余额账本仓储：扣/退在单事务内原子完成。
type BalanceLedgerRepository interface {
	// FindDeduct returns a committed result before current authorization is
	// resolved, so an idempotent replay preserves the original payer snapshot.
	FindDeduct(ctx context.Context, cmd *LedgerDeductCommand) (*LedgerDeductResult, error)
	// Deduct 不透支扣费 + (app_id, request_id) 幂等。
	Deduct(ctx context.Context, cmd *LedgerDeductCommand) (*LedgerDeductResult, error)
	// Refund 部分退 + 凭原流水冲销 + (app_id, refund_request_id) 幂等。
	Refund(ctx context.Context, cmd *LedgerRefundCommand) (*LedgerRefundResult, error)
	// AppStats 返回某接入方的累计扣/退统计。
	AppStats(ctx context.Context, appID string) (*AppLedgerStats, error)
}
