package provider

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"sync"

	"github.com/Wei-Shaw/sub2api/internal/payment"
	stripe "github.com/stripe/stripe-go/v85"
	"github.com/stripe/stripe-go/v85/webhook"
)

// Stripe constants.
const (
	stripeEventPaymentSuccess = "payment_intent.succeeded"
	stripeEventPaymentFailed  = "payment_intent.payment_failed"
)

// Stripe implements the payment.CancelableProvider interface for Stripe payments.
type Stripe struct {
	instanceID string
	config     map[string]string

	mu          sync.Mutex
	initialized bool
	sc          *stripe.Client
REDACTED

// NewStripe creates a new Stripe provider instance.
func NewStripe(instanceID string, config map[string]string) (*Stripe, error) {
	if config["secretKey"] == "" {
		return nil, fmt.Errorf("stripe config missing required key: secretKey")
REDACTED
	cfg := cloneStringMap(config)
	currency, err := payment.NormalizePaymentCurrency(cfg["currency"])
	if err != nil {
		return nil, fmt.Errorf("stripe config currency: %w", err)
REDACTED
	cfg["currency"] = currency
	return &Stripe{
		instanceID: instanceID,
		config:     cfg,
REDACTED, nil
REDACTED

func (s *Stripe) ensureInit() {
	s.mu.Lock()
	defer s.mu.Unlock()
	if !s.initialized {
		s.sc = stripe.NewClient(s.config["secretKey"])
		s.initialized = true
REDACTED
REDACTED

// GetPublishableKey returns the publishable key for frontend use.
func (s *Stripe) GetPublishableKey() string {
	return s.config["publishableKey"]
REDACTED

func (s *Stripe) Name() string        { return "Stripe" REDACTED
func (s *Stripe) ProviderKey() string { return payment.TypeStripe REDACTED
func (s *Stripe) SupportedTypes() []payment.PaymentType {
	return []payment.PaymentType{payment.TypeStripeREDACTED
REDACTED

func (s *Stripe) MerchantIdentityMetadata() map[string]string {
	if s == nil {
		return nil
REDACTED
	return map[string]string{"currency": s.currency()REDACTED
REDACTED

func (s *Stripe) currency() string {
	if s == nil {
		return payment.DefaultPaymentCurrency
REDACTED
	currency, err := payment.NormalizePaymentCurrency(s.config["currency"])
	if err != nil {
		return payment.DefaultPaymentCurrency
REDACTED
	return currency
REDACTED

// stripePaymentMethodTypes maps our PaymentType to Stripe payment_method_types.
var stripePaymentMethodTypes = map[string][]string{
	payment.TypeCard:   {"card"REDACTED,
	payment.TypeAlipay: {"alipay"REDACTED,
	payment.TypeWxpay:  {"wechat_pay"REDACTED,
	payment.TypeLink:   {"link"REDACTED,
REDACTED

// CreatePayment creates a Stripe PaymentIntent.
func (s *Stripe) CreatePayment(ctx context.Context, req payment.CreatePaymentRequest) (*payment.CreatePaymentResponse, error) {
	s.ensureInit()

	currency := s.currency()
	amountInMinorUnit, err := payment.AmountToMinorUnit(req.Amount, currency)
	if err != nil {
		return nil, fmt.Errorf("stripe create payment: %w", err)
REDACTED

	// Collect all Stripe payment_method_types from the instance's configured sub-methods
	methods := resolveStripeMethodTypes(req.InstanceSubMethods)

	pmTypes := make([]*string, len(methods))
	for i, m := range methods {
		pmTypes[i] = stripe.String(m)
REDACTED

	params := &stripe.PaymentIntentCreateParams{
		Amount:             stripe.Int64(amountInMinorUnit),
		Currency:           stripe.String(strings.ToLower(currency)),
		PaymentMethodTypes: pmTypes,
		Description:        stripe.String(req.Subject),
		Metadata:           map[string]string{"orderId": req.OrderIDREDACTED,
REDACTED

	// WeChat Pay requires payment_method_options with client type
	if hasStripeMethod(methods, "wechat_pay") {
		params.PaymentMethodOptions = &stripe.PaymentIntentCreatePaymentMethodOptionsParams{
			WeChatPay: &stripe.PaymentIntentCreatePaymentMethodOptionsWeChatPayParams{
				Client: stripe.String("web"),
		REDACTED,
	REDACTED
REDACTED

	params.SetIdempotencyKey(fmt.Sprintf("pi-%s", req.OrderID))
	params.Context = ctx

	pi, err := s.sc.V1PaymentIntents.Create(ctx, params)
	if err != nil {
		return nil, fmt.Errorf("stripe create payment: %w", err)
REDACTED

	return &payment.CreatePaymentResponse{
		TradeNo:      pi.ID,
		ClientSecret: pi.ClientSecret,
		Currency:     currency,
REDACTED, nil
REDACTED

// QueryOrder retrieves a PaymentIntent by ID.
func (s *Stripe) QueryOrder(ctx context.Context, tradeNo string) (*payment.QueryOrderResponse, error) {
	s.ensureInit()

	pi, err := s.sc.V1PaymentIntents.Retrieve(ctx, tradeNo, nil)
	if err != nil {
		return nil, fmt.Errorf("stripe query order: %w", err)
REDACTED

	status := payment.ProviderStatusPending
	switch pi.Status {
	case stripe.PaymentIntentStatusSucceeded:
		status = payment.ProviderStatusPaid
	case stripe.PaymentIntentStatusCanceled:
		status = payment.ProviderStatusFailed
REDACTED

	currency := stripeIntentCurrency(pi.Currency, s.currency())
	return &payment.QueryOrderResponse{
		TradeNo: pi.ID,
		Status:  status,
		Amount:  payment.MinorUnitToAmount(pi.Amount, currency),
		Metadata: map[string]string{
			"currency": currency,
	REDACTED,
REDACTED, nil
REDACTED

// VerifyNotification verifies a Stripe webhook event.
func (s *Stripe) VerifyNotification(_ context.Context, rawBody string, headers map[string]string) (*payment.PaymentNotification, error) {
	s.ensureInit()

	webhookSecret := s.config["webhookSecret"]
	if webhookSecret == "" {
		return nil, fmt.Errorf("stripe webhookSecret not configured")
REDACTED

	sig := headers["stripe-signature"]
	if sig == "" {
		return nil, fmt.Errorf("stripe notification missing stripe-signature header")
REDACTED

	event, err := webhook.ConstructEvent([]byte(rawBody), sig, webhookSecret)
	if err != nil {
		return nil, fmt.Errorf("stripe verify notification: %w", err)
REDACTED

	switch event.Type {
	case stripeEventPaymentSuccess:
		return parseStripePaymentIntent(&event, payment.ProviderStatusSuccess, rawBody)
	case stripeEventPaymentFailed:
		return parseStripePaymentIntent(&event, payment.ProviderStatusFailed, rawBody)
REDACTED

	return nil, nil
REDACTED

func parseStripePaymentIntent(event *stripe.Event, status string, rawBody string) (*payment.PaymentNotification, error) {
	var pi stripe.PaymentIntent
	if err := json.Unmarshal(event.Data.Raw, &pi); err != nil {
		return nil, fmt.Errorf("stripe parse payment_intent: %w", err)
REDACTED
	currency := stripeIntentCurrency(pi.Currency, payment.DefaultPaymentCurrency)
	return &payment.PaymentNotification{
		TradeNo: pi.ID,
		OrderID: pi.Metadata["orderId"],
		Amount:  payment.MinorUnitToAmount(pi.Amount, currency),
		Status:  status,
		RawData: rawBody,
		Metadata: map[string]string{
			"currency": currency,
	REDACTED,
REDACTED, nil
REDACTED

// Refund creates a Stripe refund.
func (s *Stripe) Refund(ctx context.Context, req payment.RefundRequest) (*payment.RefundResponse, error) {
	s.ensureInit()

	amountInMinorUnit, err := payment.AmountToMinorUnit(req.Amount, s.currency())
	if err != nil {
		return nil, fmt.Errorf("stripe refund: %w", err)
REDACTED

	params := &stripe.RefundCreateParams{
		PaymentIntent: stripe.String(req.TradeNo),
		Amount:        stripe.Int64(amountInMinorUnit),
		Reason:        stripe.String(string(stripe.RefundReasonRequestedByCustomer)),
REDACTED
	params.Context = ctx

	r, err := s.sc.V1Refunds.Create(ctx, params)
	if err != nil {
		return nil, fmt.Errorf("stripe refund: %w", err)
REDACTED

	refundStatus := payment.ProviderStatusPending
	if r.Status == stripe.RefundStatusSucceeded {
		refundStatus = payment.ProviderStatusSuccess
REDACTED

	return &payment.RefundResponse{
		RefundID: r.ID,
		Status:   refundStatus,
REDACTED, nil
REDACTED

// QueryRefund retrieves a Stripe refund by refund ID when available, otherwise
// falls back to the latest refund for the PaymentIntent.
func (s *Stripe) QueryRefund(ctx context.Context, req payment.RefundQueryRequest) (*payment.RefundResponse, error) {
	s.ensureInit()

	var r *stripe.Refund
	var err error
	if refundID := strings.TrimSpace(req.RefundID); refundID != "" {
		r, err = s.sc.V1Refunds.Retrieve(ctx, refundID, nil)
		if err != nil {
			return nil, fmt.Errorf("stripe query refund: %w", err)
	REDACTED
REDACTED else {
		tradeNo := strings.TrimSpace(req.TradeNo)
		if tradeNo == "" {
			return nil, fmt.Errorf("stripe query refund: missing payment intent id")
	REDACTED
		params := &stripe.RefundListParams{PaymentIntent: stripe.String(tradeNo)REDACTED
		params.Limit = stripe.Int64(1)
		list := s.sc.V1Refunds.List(ctx, params)
		if list.Err() != nil {
			return nil, fmt.Errorf("stripe query refund: %w", list.Err())
	REDACTED
		refunds := list.Data()
		if len(refunds) == 0 {
			return nil, fmt.Errorf("stripe query refund: no refund found")
	REDACTED
		r = refunds[0]
REDACTED

	return &payment.RefundResponse{RefundID: r.ID, Status: stripeRefundProviderStatus(r.Status)REDACTED, nil
REDACTED

func stripeRefundProviderStatus(status stripe.RefundStatus) string {
	switch status {
	case stripe.RefundStatusSucceeded:
		return payment.ProviderStatusSuccess
	case stripe.RefundStatusFailed, stripe.RefundStatusCanceled:
		return payment.ProviderStatusFailed
	default:
		return payment.ProviderStatusPending
REDACTED
REDACTED

func stripeIntentCurrency(raw stripe.Currency, fallback string) string {
	currency, err := payment.NormalizePaymentCurrency(string(raw))
	if err != nil || currency == payment.DefaultPaymentCurrency && strings.TrimSpace(string(raw)) == "" {
		normalizedFallback, fallbackErr := payment.NormalizePaymentCurrency(fallback)
		if fallbackErr == nil {
			return normalizedFallback
	REDACTED
		return payment.DefaultPaymentCurrency
REDACTED
	return currency
REDACTED

// resolveStripeMethodTypes converts instance supported_types (comma-separated)
// into Stripe API payment_method_types. Falls back to ["card"] if empty.
func resolveStripeMethodTypes(instanceSubMethods string) []string {
	if instanceSubMethods == "" {
		return []string{"card"REDACTED
REDACTED
	var methods []string
	for _, t := range strings.Split(instanceSubMethods, ",") {
		t = strings.TrimSpace(t)
		if mapped, ok := stripePaymentMethodTypes[t]; ok {
			methods = append(methods, mapped...)
	REDACTED
REDACTED
	if len(methods) == 0 {
		return []string{"card"REDACTED
REDACTED
	return methods
REDACTED

// hasStripeMethod checks if the given Stripe method list contains the target method.
func hasStripeMethod(methods []string, target string) bool {
	for _, m := range methods {
		if m == target {
			return true
	REDACTED
REDACTED
	return false
REDACTED

// CancelPayment cancels a pending PaymentIntent.
func (s *Stripe) CancelPayment(ctx context.Context, tradeNo string) error {
	s.ensureInit()

	_, err := s.sc.V1PaymentIntents.Cancel(ctx, tradeNo, nil)
	if err != nil {
		return fmt.Errorf("stripe cancel payment: %w", err)
REDACTED
	return nil
REDACTED

// Ensure interface compliance.
var (
	_ payment.Provider                 = (*Stripe)(nil)
	_ payment.CancelableProvider       = (*Stripe)(nil)
	_ payment.MerchantIdentityProvider = (*Stripe)(nil)
)
