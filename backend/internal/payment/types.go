// Package payment provides the core payment provider abstraction,
// registry, load balancing, and shared utilities for the payment subsystem.
package payment

import (
	"context"

	"github.com/shopspring/decimal"
)

// PaymentType represents a supported payment method.
type PaymentType = string

// Supported payment type constants.
//
// TypeSePay is the provider key; the remaining constants are the user-facing
// payment methods a SePay instance can offer, and map one-to-one onto the
// gateway's payment_method values.
const (
	TypeSePay             PaymentType = "sepay"
	TypeSePayBankTransfer PaymentType = "sepay_bank_transfer"
	TypeSePayNapas        PaymentType = "sepay_napas"
	TypeSePayCard         PaymentType = "sepay_card"
)

// Order status constants shared across payment and service layers.
const (
	OrderStatusPending    = "PENDING"
	OrderStatusPaid       = "PAID"
	OrderStatusRecharging = "RECHARGING"
	OrderStatusCompleted  = "COMPLETED"
	OrderStatusExpired    = "EXPIRED"
	OrderStatusCancelled  = "CANCELLED"
	OrderStatusFailed     = "FAILED"
)

// Legacy refund order statuses.
//
// The refund feature has been removed together with the providers that could
// service it, so nothing writes these any more. They stay declared because
// historical rows in payment_orders still carry them, and admin order listings
// must keep rendering those orders instead of showing a blank status.
const (
	OrderStatusRefundRequested   = "REFUND_REQUESTED"
	OrderStatusRefunding         = "REFUNDING"
	OrderStatusRefundPending     = "REFUND_PENDING"
	OrderStatusPartiallyRefunded = "PARTIALLY_REFUNDED"
	OrderStatusRefunded          = "REFUNDED"
	OrderStatusRefundFailed      = "REFUND_FAILED"
)

// Order types distinguish balance recharges from subscription purchases.
const (
	OrderTypeBalance      = "balance"
	OrderTypeSubscription = "subscription"
)

// Entity statuses shared across users, groups, etc.
const (
	EntityStatusActive = "active"
)

// NotificationStatusSuccess is the PaymentNotification.Status value that marks
// a callback as a completed payment.
const NotificationStatusSuccess = "success"

// Provider-level status constants returned by provider implementations
// to the service layer (lowercase, distinct from OrderStatus uppercase constants).
const (
	ProviderStatusPending  = "pending"
	ProviderStatusPaid     = "paid"
	ProviderStatusSuccess  = "success"
	ProviderStatusFailed   = "failed"
	ProviderStatusRefunded = "refunded"
)

// DefaultLoadBalanceStrategy is the default load-balancing strategy
// used when no strategy is configured.
const DefaultLoadBalanceStrategy = "round-robin"

// GetBasePaymentType extracts the base payment method from a composite key.
// For example, "sepay_card" maps back to "sepay".
func GetBasePaymentType(t string) string {
	if len(t) >= len(TypeSePay) && t[:len(TypeSePay)] == TypeSePay {
		return TypeSePay
	}
	return t
}

// MinorUnitToDecimalAmount 把最小货币单位（分 / cent）换算成精确金额。
//
// 与 MinorUnitToAmount 的区别是不经过 float64：金额从服务商解析、跨接口传递、
// 一直到与订单应付金额比对，全程保持十进制精确值。
func MinorUnitToDecimalAmount(value int64, currency string) decimal.Decimal {
	return decimal.NewFromInt(value).Div(decimal.New(1, int32(CurrencyMinorUnit(currency))))
}

// CreatePaymentRequest holds the parameters for creating a new payment.
type CreatePaymentRequest struct {
	OrderID     string // Internal order ID
	Amount      string // 支付金额，按服务商实例配置的币种解释
	PaymentType string // e.g. "sepay_bank_transfer"
	Subject     string // Product description
	NotifyURL   string // Webhook callback URL
	ReturnURL   string // Browser redirect URL after payment
	ClientIP    string // Payer's IP address
	IsMobile    bool   // Whether the request comes from a mobile device
}

// CreatePaymentResultType describes the shape of the create-payment result.
type CreatePaymentResultType = string

const (
	CreatePaymentResultOrderCreated CreatePaymentResultType = "order_created"
	// CreatePaymentResultFormPost means the payer must reach the gateway through
	// an HTTP POST form rather than a plain redirect. FormAction and FormFields
	// carry everything the auto-submitting checkout page needs.
	CreatePaymentResultFormPost CreatePaymentResultType = "form_post"
)

// CreatePaymentResponse is returned after successfully initiating a payment.
type CreatePaymentResponse struct {
	TradeNo    string                  // Third-party transaction ID
	PayURL     string                  // Browser payment URL when the gateway offers a plain redirect
	QRCode     string                  // QR code content for scanning
	Currency   string                  // 服务商支付币种
	PaymentEnv string                  // 服务商前端环境标识
	ResultType CreatePaymentResultType // Typed result contract for frontend flows
	FormAction string                  // POST target for CreatePaymentResultFormPost
	FormFields map[string]string       // Signed form fields for CreatePaymentResultFormPost
}

// QueryOrderResponse describes the payment status from the upstream provider.
type QueryOrderResponse struct {
	TradeNo string
	Status  string // "pending", "paid", "failed", "refunded"
	// Amount 是按服务商返回币种解释的金额。用 decimal 而不是 float64：
	// 服务商给的是十进制字符串或最小货币单位整数，转成 float64 再比对会引入
	// 表示误差，历来只能靠一个「一分钱」的容差掩盖过去——而那个容差正好让
	// 100.00 的订单收到 99.99 也照样足额履约。
	Amount   decimal.Decimal
	PaidAt   string // RFC3339 timestamp or empty
	Metadata map[string]string
}

// PaymentNotification is the parsed result of a webhook/notify callback.
type PaymentNotification struct {
	TradeNo string
	OrderID string
	// Amount 同 QueryOrderResponse.Amount：保持十进制精确值。
	Amount   decimal.Decimal
	Status   string // "success" or "failed"
	RawData  string // Raw notification body for audit
	Metadata map[string]string
}

// InstanceSelection holds the selected provider instance and its decrypted config.
type InstanceSelection struct {
	InstanceID     string
	ProviderKey    string // Provider key of the selected instance (e.g. "sepay")
	Config         map[string]string
	SupportedTypes string // Comma-separated list of supported payment types from the instance
	PaymentMode    string // Payment display mode: "qrcode", "redirect", "popup"
}

// Provider defines the interface that all payment providers must implement.
type Provider interface {
	// Name returns a human-readable name for this provider.
	Name() string
	// ProviderKey returns the unique key identifying this provider type (e.g. "sepay").
	ProviderKey() string
	// SupportedTypes returns the list of payment types this provider handles.
	SupportedTypes() []PaymentType
	// CreatePayment initiates a payment and returns the upstream response.
	CreatePayment(ctx context.Context, req CreatePaymentRequest) (*CreatePaymentResponse, error)
	// QueryOrder queries the payment status of the given trade number.
	QueryOrder(ctx context.Context, tradeNo string) (*QueryOrderResponse, error)
	// VerifyNotification parses and verifies a webhook callback.
	// Returns nil for unrecognized or irrelevant events (caller should return 200).
	VerifyNotification(ctx context.Context, rawBody string, headers map[string]string) (*PaymentNotification, error)
}

// CancelableProvider extends Provider with the ability to cancel pending payments.
type CancelableProvider interface {
	Provider
	// CancelPayment cancels/expires a pending payment on the upstream platform.
	CancelPayment(ctx context.Context, tradeNo string) error
}

// MerchantIdentityProvider exposes the current non-sensitive merchant identity
// derived from provider configuration for snapshot consistency checks.
type MerchantIdentityProvider interface {
	MerchantIdentityMetadata() map[string]string
}
