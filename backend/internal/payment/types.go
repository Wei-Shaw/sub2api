// Package payment provides the core payment provider abstraction,
// registry, load balancing, and shared utilities for the payment subsystem.
package payment

import "context"

// PaymentType represents a supported payment method.
type PaymentType = string

// Supported payment type constants.
//
// Each provider backs exactly one user-facing method, so a payment type and a
// provider key are the same string.
const (
	TypeSePay       PaymentType = "sepay"
	TypeNowPayments PaymentType = "nowpayments"
)

// Currencies each provider settles in. SePay collects Vietnamese bank
// transfers; NOWPayments prices in USD and lets the buyer pay any coin.
const (
	CurrencySePay       = "VND"
	CurrencyNowPayments = "USD"
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

// Order types distinguish balance recharges from subscription purchases.
const (
	OrderTypeBalance      = "balance"
	OrderTypeSubscription = "subscription"
)

// Entity statuses shared across users, groups, etc.
const (
	EntityStatusActive = "active"
)

// Payment notification status values.
const (
	NotificationStatusSuccess = "success"
	NotificationStatusPaid    = "paid"
)

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

// GetBasePaymentType normalises a payment type to its canonical form.
// Kept as a function because persisted orders may carry historical values that
// no longer map to a live provider.
func GetBasePaymentType(t string) string {
	return t
}

// CreatePaymentRequest holds the parameters for creating a new payment.
type CreatePaymentRequest struct {
	OrderID     string // Internal order ID, also the bank transfer reference
	Amount      string // 支付金额，按服务商实例配置的币种解释
	PaymentType string // e.g. "sepay", "nowpayments"
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
)

// BankTransferInfo carries the human-readable transfer instructions that sit
// beside a QR code, for payers who would rather type the details into their
// banking app than scan.
type BankTransferInfo struct {
	BankCode      string `json:"bank_code,omitempty"`
	BankBIN       string `json:"bank_bin,omitempty"`
	AccountNumber string `json:"account_number,omitempty"`
	AccountName   string `json:"account_name,omitempty"`
	Content       string `json:"content,omitempty"`
	Amount        string `json:"amount,omitempty"`
}

// CreatePaymentResponse is returned after successfully initiating a payment.
type CreatePaymentResponse struct {
	TradeNo    string                  // Third-party transaction ID
	PayURL     string                  // Hosted checkout URL
	QRCode     string                  // QR code content for scanning
	IntentID   string                  // 前端 SDK 需要的服务商支付意图 ID
	Currency   string                  // 服务商支付币种
	ResultType CreatePaymentResultType // Typed result contract for frontend flows
	Transfer   *BankTransferInfo       // Bank transfer instructions when applicable
}

// QueryOrderResponse describes the payment status from the upstream provider.
type QueryOrderResponse struct {
	TradeNo  string
	Status   string  // "pending", "paid", "failed", "refunded"
	Amount   float64 // 按服务商返回币种解释的金额
	PaidAt   string  // RFC3339 timestamp or empty
	Metadata map[string]string
}

// PaymentNotification is the parsed result of a webhook/notify callback.
type PaymentNotification struct {
	TradeNo  string
	OrderID  string
	Amount   float64
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
	PaymentMode    string // Payment display mode: "qrcode", "redirect"
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

// MerchantIdentityProvider exposes the current non-sensitive merchant identity
// derived from provider configuration for snapshot consistency checks.
type MerchantIdentityProvider interface {
	MerchantIdentityMetadata() map[string]string
}
