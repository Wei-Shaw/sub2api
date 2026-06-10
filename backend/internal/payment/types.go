// Package payment provides the core payment provider abstraction,
// registry, load balancing, and shared utilities for the payment subsystem.
package payment

import "context"

// PaymentType represents a supported payment method.
type PaymentType = string

// Supported payment type constants.
const (
	TypeAlipay       PaymentType = "alipay"
	TypeWxpay        PaymentType = "wxpay"
	TypeAlipayDirect PaymentType = "alipay_direct"
	TypeWxpayDirect  PaymentType = "wxpay_direct"
	TypeStripe       PaymentType = "stripe"
	TypeCard         PaymentType = "card"
	TypeLink         PaymentType = "link"
	TypeEasyPay      PaymentType = "easypay"
	TypeAirwallex    PaymentType = "airwallex"
)

// Order status constants shared across payment and service layers.
const (
	OrderStatusPending           = "PENDING"
	OrderStatusPaid              = "PAID"
	OrderStatusRecharging        = "RECHARGING"
	OrderStatusCompleted         = "COMPLETED"
	OrderStatusExpired           = "EXPIRED"
	OrderStatusCancelled         = "CANCELLED"
	OrderStatusFailed            = "FAILED"
	OrderStatusRefundRequested   = "REFUND_REQUESTED"
	OrderStatusRefunding         = "REFUNDING"
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

// Deduction types for refund flow.
const (
	DeductionTypeBalance      = "balance"
	DeductionTypeSubscription = "subscription"
	DeductionTypeNone         = "none"
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

// ConfigKeyPublishableKey is the config map key for Stripe's publishable key.
const ConfigKeyPublishableKey = "publishableKey"

// GetBasePaymentType extracts the base payment method from a composite key.
// For example, "alipay_direct" -> "alipay".
func GetBasePaymentType(t string) string {
	switch {
	case t == TypeEasyPay:
		return TypeEasyPay
	case t == TypeAirwallex:
		return TypeAirwallex
	case t == TypeStripe || t == TypeCard || t == TypeLink:
		return TypeStripe
	case len(t) >= len(TypeAlipay) && t[:len(TypeAlipay)] == TypeAlipay:
		return TypeAlipay
	case len(t) >= len(TypeWxpay) && t[:len(TypeWxpay)] == TypeWxpay:
		return TypeWxpay
	default:
		return t
	}
}

// CreatePaymentRequest holds the parameters for creating a new payment.
type CreatePaymentRequest struct {
	OrderID            string // Internal order ID
	Amount             string // 支付金额，按服务商实例配置的币种解释
	PaymentType        string // e.g. "alipay", "wxpay", "stripe"
	Subject            string // Product description
	NotifyURL          string // Webhook callback URL
	ReturnURL          string // Browser redirect URL after payment
	OpenID             string // WeChat JSAPI payer OpenID when available
	ClientIP           string // Payer's IP address
	IsMobile           bool   // Whether the request comes from a mobile device
	InstanceSubMethods string // Comma-separated sub-methods from instance supported_types (for Stripe)
}

// CreatePaymentResultType describes the shape of the create-payment result.
type CreatePaymentResultType = string

const (
	CreatePaymentResultOrderCreated  CreatePaymentResultType = "order_created"
	CreatePaymentResultOAuthRequired CreatePaymentResultType = "oauth_required"
	CreatePaymentResultJSAPIReady    CreatePaymentResultType = "jsapi_ready"
)

// WechatOAuthInfo describes the next step when WeChat OAuth is required before payment.
type WechatOAuthInfo struct {
	AuthorizeURL string `json:"authorize_url,omitempty"`
	AppID        string `json:"appid,omitempty"`
	OpenID       string `json:"openid,omitempty"`
	Scope        string `json:"scope,omitempty"`
	State        string `json:"state,omitempty"`
	RedirectURL  string `json:"redirect_url,omitempty"`
}

// WechatJSAPIPayload contains the fields the frontend needs to invoke WeChat JSAPI payment.
type WechatJSAPIPayload struct {
	AppID     string `json:"appId,omitempty"`
	TimeStamp string `json:"timeStamp,omitempty"`
	NonceStr  string `json:"nonceStr,omitempty"`
	Package   string `json:"package,omitempty"`
	SignType  string `json:"signType,omitempty"`
	PaySign   string `json:"paySign,omitempty"`
}

// CreatePaymentResponse is returned after successfully initiating a payment.
type CreatePaymentResponse struct {
	TradeNo      string                  // Third-party transaction ID
	PayURL       string                  // H5 payment URL (alipay/wxpay)
	QRCode       string                  // QR code content for scanning
	ClientSecret string                  // Stripe PaymentIntent 客户端密钥
	IntentID     string                  // 前端 SDK 需要的服务商支付意图 ID
	Currency     string                  // 服务商支付币种
	CountryCode  string                  // 服务商收银台国家/地区代码
	PaymentEnv   string                  // 服务商前端环境标识
	ResultType   CreatePaymentResultType // Typed result contract for frontend flows
	OAuth        *WechatOAuthInfo        // WeChat OAuth bootstrap payload when required
	JSAPI        *WechatJSAPIPayload     // WeChat JSAPI invocation payload when ready
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

// RefundRequest contains the parameters for requesting a refund.
type RefundRequest struct {
	TradeNo string
	OrderID string
	Amount  string // Refund amount formatted to 2 decimal places
	Reason  string
}

// RefundResponse is returned after a refund request.
type RefundResponse struct {
	RefundID string
	Status   string // "success", "pending", "failed"
}

// InstanceSelection holds the selected provider instance and its decrypted config.
type InstanceSelection struct {
	InstanceID     string
	ProviderKey    string // Provider key of the selected instance (e.g. "alipay", "easypay")
	Config         map[string]string
	SupportedTypes string // Comma-separated list of supported payment types from the instance
	PaymentMode    string // Payment display mode: "qrcode", "redirect", "popup"
}

// Provider defines the interface that all payment providers must implement.
type Provider interface {
	// Name returns a human-readable name for this provider.
	Name() string
	// ProviderKey returns the unique key identifying this provider type (e.g. "easypay").
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
	// Refund requests a refund from the upstream provider.
	Refund(ctx context.Context, req RefundRequest) (*RefundResponse, error)
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
