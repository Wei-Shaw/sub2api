package service

import (
	"context"

	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
)

// 余额 RPC 账本相关错误。
var (
	// ErrBalanceInsufficient 余额不足（不透支）。
	ErrBalanceInsufficient = infraerrors.Conflict("INSUFFICIENT_BALANCE", "insufficient balance")
	// ErrOverRefund 累计退款超过原扣金额。
	ErrOverRefund = infraerrors.Conflict("OVER_REFUND", "refund exceeds original deduction")
	// ErrOriginalDeductNotFound 凭原流水冲销时找不到对应的原扣流水（或不属于本 app）。
	ErrOriginalDeductNotFound = infraerrors.NotFound("ORIGINAL_DEDUCT_NOT_FOUND", "original deduction not found")
	// ErrLedgerRequestConflict 相同 request_id 但参数与首次不一致。
	ErrLedgerRequestConflict = infraerrors.Conflict("LEDGER_REQUEST_CONFLICT", "request_id reused with different parameters")
	// ErrBillingAppNotFound 接入方不存在。
	ErrBillingAppNotFound = infraerrors.NotFound("BILLING_APP_NOT_FOUND", "billing app not found")
	// ErrBillingAppUnauthenticated 接入方鉴权失败（不存在 / 停用 / secret 不匹配，统一错误）。
	ErrBillingAppUnauthenticated = infraerrors.Unauthorized("BILLING_APP_UNAUTHENTICATED", "invalid billing app credentials")
)

// 账本流水类型。
const (
	BalanceLedgerKindDeduct int8 = 1
	BalanceLedgerKindRefund int8 = 2
)

// BillingApp 接入方身份领域模型。
type BillingApp struct {
	ID           int64
	AppID        string
	AppName      string
	Enabled      bool
	TokenVersion int
}

// BillingAppRepository 接入方身份仓储。
type BillingAppRepository interface {
	GetByAppID(ctx context.Context, appID string) (*BillingApp, error)
	Create(ctx context.Context, app *BillingApp) (*BillingApp, error)
	SetEnabled(ctx context.Context, appID string, enabled bool) error
	// BumpTokenVersion 自增 token_version 并返回新值，用于刷新 token（旧 token 失效）。
	BumpTokenVersion(ctx context.Context, appID string) (int, error)
	Delete(ctx context.Context, appID string) error
	List(ctx context.Context) ([]*BillingApp, error)
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
	AppID       string
	RequestID   string // 幂等键
	UserID      int64
	Amount      float64
	Description string // 必填，扣费原因
	Extra       string // jsonb 文本，接入方自存
}

// LedgerDeductResult 扣费结果。
type LedgerDeductResult struct {
	Applied      bool    // false = 幂等重放
	BalanceAfter float64 // 扣后余额
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
	Applied       bool    // false = 幂等重放
	UserID        int64   // 被退费用户（由原扣流水推导，供缓存失效用）
	BalanceAfter  float64 // 退后余额
	RefundedTotal float64 // 原扣累计已退
}

// BalanceLedgerRepository 余额账本仓储：扣/退在单事务内原子完成。
type BalanceLedgerRepository interface {
	// Deduct 不透支扣费 + (app_id, request_id) 幂等。
	Deduct(ctx context.Context, cmd *LedgerDeductCommand) (*LedgerDeductResult, error)
	// Refund 部分退 + 凭原流水冲销 + (app_id, refund_request_id) 幂等。
	Refund(ctx context.Context, cmd *LedgerRefundCommand) (*LedgerRefundResult, error)
	// AppStats 返回某接入方的累计扣/退统计。
	AppStats(ctx context.Context, appID string) (*AppLedgerStats, error)
}
