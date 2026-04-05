package provider

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"strconv"
	"sync"

	"github.com/Wei-Shaw/sub2api/internal/payment"
	stripe "github.com/stripe/stripe-go/v82"
	"github.com/stripe/stripe-go/v82/paymentintent"
	"github.com/stripe/stripe-go/v82/refund"
	"github.com/stripe/stripe-go/v82/webhook"
)

// Stripe implements the payment.CancelableProvider interface for Stripe payments.
type Stripe struct {
	instanceID string
	config     map[string]string

	mu          sync.Mutex
	initialized bool
}

// NewStripe creates a new Stripe provider instance.
func NewStripe(instanceID string, config map[string]string) (*Stripe, error) {
	if config["secretKey"] == "" {
		return nil, fmt.Errorf("stripe config missing required key: secretKey")
	}
	return &Stripe{
		instanceID: instanceID,
		config:     config,
	}, nil
}

func (s *Stripe) ensureInit() {
	s.mu.Lock()
	defer s.mu.Unlock()
	if !s.initialized {
		stripe.Key = s.config["secretKey"]
		s.initialized = true
	}
}

// GetPublishableKey returns the publishable key for frontend use.
func (s *Stripe) GetPublishableKey() string {
	return s.config["publishableKey"]
}

func (s *Stripe) Name() string       { return "Stripe" }
func (s *Stripe) ProviderKey() string { return "stripe" }
func (s *Stripe) SupportedTypes() []payment.PaymentType {
	return []payment.PaymentType{payment.TypeStripe}
}

// yuanToCents converts a CNY yuan string to cents (int64).
func yuanToCents(amountStr string) (int64, error) {
	amount, err := strconv.ParseFloat(amountStr, 64)
	if err != nil {
		return 0, fmt.Errorf("invalid amount: %s", amountStr)
	}
	return int64(math.Round(amount * 100)), nil
}

// CreatePayment creates a Stripe PaymentIntent.
func (s *Stripe) CreatePayment(ctx context.Context, req payment.CreatePaymentRequest) (*payment.CreatePaymentResponse, error) {
	s.ensureInit()

	amountInCents, err := yuanToCents(req.Amount)
	if err != nil {
		return nil, fmt.Errorf("stripe create payment: %w", err)
	}

	params := &stripe.PaymentIntentParams{
		Amount:   stripe.Int64(amountInCents),
		Currency: stripe.String("cny"),
		AutomaticPaymentMethods: &stripe.PaymentIntentAutomaticPaymentMethodsParams{
			Enabled: stripe.Bool(true),
		},
		Description: stripe.String(req.Subject),
		Metadata:    map[string]string{"orderId": req.OrderID},
	}
	params.SetIdempotencyKey(fmt.Sprintf("pi-%s", req.OrderID))
	params.Context = ctx

	pi, err := paymentintent.New(params)
	if err != nil {
		return nil, fmt.Errorf("stripe create payment: %w", err)
	}

	return &payment.CreatePaymentResponse{
		TradeNo:      pi.ID,
		ClientSecret: pi.ClientSecret,
	}, nil
}

// QueryOrder retrieves a PaymentIntent by ID.
func (s *Stripe) QueryOrder(ctx context.Context, tradeNo string) (*payment.QueryOrderResponse, error) {
	s.ensureInit()

	params := &stripe.PaymentIntentParams{}
	params.Context = ctx

	pi, err := paymentintent.Get(tradeNo, params)
	if err != nil {
		return nil, fmt.Errorf("stripe query order: %w", err)
	}

	status := "pending"
	switch pi.Status {
	case stripe.PaymentIntentStatusSucceeded:
		status = "paid"
	case stripe.PaymentIntentStatusCanceled:
		status = "failed"
	}

	return &payment.QueryOrderResponse{
		TradeNo: pi.ID,
		Status:  status,
		Amount:  float64(pi.Amount) / 100,
	}, nil
}

// VerifyNotification verifies a Stripe webhook event.
func (s *Stripe) VerifyNotification(_ context.Context, rawBody string, headers map[string]string) (*payment.PaymentNotification, error) {
	s.ensureInit()

	webhookSecret := s.config["webhookSecret"]
	if webhookSecret == "" {
		return nil, fmt.Errorf("stripe webhookSecret not configured")
	}

	sig := headers["stripe-signature"]
	if sig == "" {
		return nil, fmt.Errorf("stripe notification missing stripe-signature header")
	}

	event, err := webhook.ConstructEvent([]byte(rawBody), sig, webhookSecret)
	if err != nil {
		return nil, fmt.Errorf("stripe verify notification: %w", err)
	}

	switch event.Type {
	case "payment_intent.succeeded":
		return parseStripePaymentIntent(&event, "success", rawBody)
	case "payment_intent.payment_failed":
		return parseStripePaymentIntent(&event, "failed", rawBody)
	}

	return nil, nil
}

func parseStripePaymentIntent(event *stripe.Event, status string, rawBody string) (*payment.PaymentNotification, error) {
	var pi stripe.PaymentIntent
	if err := json.Unmarshal(event.Data.Raw, &pi); err != nil {
		return nil, fmt.Errorf("stripe parse payment_intent: %w", err)
	}
	return &payment.PaymentNotification{
		TradeNo: pi.ID,
		OrderID: pi.Metadata["orderId"],
		Amount:  float64(pi.Amount) / 100,
		Status:  status,
		RawData: rawBody,
	}, nil
}

// Refund creates a Stripe refund.
func (s *Stripe) Refund(ctx context.Context, req payment.RefundRequest) (*payment.RefundResponse, error) {
	s.ensureInit()

	amountInCents, err := yuanToCents(req.Amount)
	if err != nil {
		return nil, fmt.Errorf("stripe refund: %w", err)
	}

	params := &stripe.RefundParams{
		PaymentIntent: stripe.String(req.TradeNo),
		Amount:        stripe.Int64(amountInCents),
		Reason:        stripe.String(string(stripe.RefundReasonRequestedByCustomer)),
	}
	params.Context = ctx

	r, err := refund.New(params)
	if err != nil {
		return nil, fmt.Errorf("stripe refund: %w", err)
	}

	refundStatus := "pending"
	if r.Status == stripe.RefundStatusSucceeded {
		refundStatus = "success"
	}

	return &payment.RefundResponse{
		RefundID: r.ID,
		Status:   refundStatus,
	}, nil
}

// CancelPayment cancels a pending PaymentIntent.
func (s *Stripe) CancelPayment(ctx context.Context, tradeNo string) error {
	s.ensureInit()

	params := &stripe.PaymentIntentCancelParams{}
	params.Context = ctx

	_, err := paymentintent.Cancel(tradeNo, params)
	if err != nil {
		return fmt.Errorf("stripe cancel payment: %w", err)
	}
	return nil
}

// Ensure interface compliance.
var (
	_ payment.Provider           = (*Stripe)(nil)
	_ payment.CancelableProvider = (*Stripe)(nil)
)
