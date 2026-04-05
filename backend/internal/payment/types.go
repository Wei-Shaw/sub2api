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
)

// GetBasePaymentType extracts the base payment method from a composite key.
// For example, "alipay_direct" -> "alipay".
func GetBasePaymentType(t string) string {
	switch {
	case len(t) >= 6 && t[:6] == "alipay":
		return "alipay"
	case len(t) >= 5 && t[:5] == "wxpay":
		return "wxpay"
	case len(t) >= 6 && t[:6] == "stripe":
		return "stripe"
	default:
		return t
	}
}

// CreatePaymentRequest holds the parameters for creating a new payment.
type CreatePaymentRequest struct {
	OrderID     string // Internal order ID
	Amount      string // Pay amount in CNY (formatted to 2 decimal places)
	PaymentType string // e.g. "alipay", "wxpay"
	Subject     string // Product description
	NotifyURL   string // Webhook callback URL
	ReturnURL   string // Browser redirect URL after payment
	ClientIP    string // Payer's IP address
	IsMobile    bool   // Whether the request comes from a mobile device
}

// CreatePaymentResponse is returned after successfully initiating a payment.
type CreatePaymentResponse struct {
	TradeNo      string // Third-party transaction ID
	PayURL       string // H5 payment URL (alipay/wxpay)
	QRCode       string // QR code content for scanning
	ClientSecret string // Stripe PaymentIntent client secret
}

// QueryOrderResponse describes the payment status from the upstream provider.
type QueryOrderResponse struct {
	TradeNo string
	Status  string  // "pending", "paid", "failed", "refunded"
	Amount  float64 // Amount in CNY
	PaidAt  string  // RFC3339 timestamp or empty
}

// PaymentNotification is the parsed result of a webhook/notify callback.
type PaymentNotification struct {
	TradeNo string
	OrderID string
	Amount  float64
	Status  string // "success" or "failed"
	RawData string // Raw notification body for audit
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
	InstanceID string
	Config     map[string]string
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
