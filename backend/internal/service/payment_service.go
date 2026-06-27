package service

import (
	"bytes"
	"context"
	"encoding/hex"
	"fmt"
	"log/slog"
	"math/rand/v2"
	"os"
	"strings"
	"sync"
	"time"

	dbent "github.com/Wei-Shaw/sub2api/ent"
	"github.com/Wei-Shaw/sub2api/ent/paymentproviderinstance"
	"github.com/Wei-Shaw/sub2api/internal/payment"
	"github.com/Wei-Shaw/sub2api/internal/payment/provider"
)

// --- Order Status Constants ---

const (
	OrderStatusPending           = payment.OrderStatusPending
	OrderStatusPaid              = payment.OrderStatusPaid
	OrderStatusRecharging        = payment.OrderStatusRecharging
	OrderStatusCompleted         = payment.OrderStatusCompleted
	OrderStatusExpired           = payment.OrderStatusExpired
	OrderStatusCancelled         = payment.OrderStatusCancelled
	OrderStatusFailed            = payment.OrderStatusFailed
	OrderStatusRefundRequested   = payment.OrderStatusRefundRequested
	OrderStatusRefunding         = payment.OrderStatusRefunding
	OrderStatusRefundPending     = payment.OrderStatusRefundPending
	OrderStatusPartiallyRefunded = payment.OrderStatusPartiallyRefunded
	OrderStatusRefunded          = payment.OrderStatusRefunded
	OrderStatusRefundFailed      = payment.OrderStatusRefundFailed
)

const (
	// defaultMaxPendingOrders and defaultOrderTimeoutMin are defined in
	// payment_config_service.go alongside other payment configuration defaults.
	paymentGraceMinutes = 5

	defaultPageSize    = 20
	maxPageSize        = 100
	topUsersLimit      = 10
	amountToleranceCNY = 0.01

	orderIDPrefix = "sub2_"
)

const paymentResumeSigningKeyEnv = "PAYMENT_RESUME_SIGNING_KEY"

// --- Types ---

// generateOutTradeNo creates a unique external order ID for payment providers.
// Format: sub2_20250409aB3kX9mQ (prefix + date + 8-char random)
func generateOutTradeNo() string {
	date := time.Now().Format("20060102")
	rnd := generateRandomString(8)
	return orderIDPrefix + date + rnd
REDACTED

func generateRandomString(n int) string {
	const charset = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789"
	b := make([]byte, n)
	for i := range b {
		b[i] = charset[rand.IntN(len(charset))]
REDACTED
	return string(b)
REDACTED

type CreateOrderRequest struct {
	UserID          int64
	Amount          float64
	PaymentType     string
	OpenID          string
	ClientIP        string
	IsMobile        bool
	IsWeChatBrowser bool
	SrcHost         string
	SrcURL          string
	ReturnURL       string
	PaymentSource   string
	OrderType       string
	PlanID          int64
	Locale          string
REDACTED

type CreateOrderResponse struct {
	OrderID      int64                           `json:"order_id"`
	Amount       float64                         `json:"amount"`
	PayAmount    float64                         `json:"pay_amount"`
	FeeRate      float64                         `json:"fee_rate"`
	Status       string                          `json:"status"`
	ResultType   payment.CreatePaymentResultType `json:"result_type,omitempty"`
	PaymentType  string                          `json:"payment_type"`
	OutTradeNo   string                          `json:"out_trade_no,omitempty"`
	PayURL       string                          `json:"pay_url,omitempty"`
	QRCode       string                          `json:"qr_code,omitempty"`
	ClientSecret string                          `json:"client_secret,omitempty"`
	IntentID     string                          `json:"intent_id,omitempty"`
	Currency     string                          `json:"currency,omitempty"`
	CountryCode  string                          `json:"country_code,omitempty"`
	PaymentEnv   string                          `json:"payment_env,omitempty"`
	OAuth        *payment.WechatOAuthInfo        `json:"oauth,omitempty"`
	JSAPI        *payment.WechatJSAPIPayload     `json:"jsapi,omitempty"`
	JSAPIPayload *payment.WechatJSAPIPayload     `json:"jsapi_payload,omitempty"`
	ExpiresAt    time.Time                       `json:"expires_at"`
	PaymentMode  string                          `json:"payment_mode,omitempty"`
	ResumeToken  string                          `json:"resume_token,omitempty"`
REDACTED

type OrderListParams struct {
	Page        int
	PageSize    int
	Status      string
	OrderType   string
	PaymentType string
	Keyword     string
REDACTED

type RefundPlan struct {
	OrderID         int64
	Order           *dbent.PaymentOrder
	RefundAmount    float64
	GatewayAmount   float64
	Reason          string
	Force           bool
	DeductBalance   bool
	DeductionType   string
	BalanceToDeduct float64
	SubDaysToDeduct int
	SubscriptionID  int64
REDACTED

type RefundResult struct {
	Success         bool    `json:"success"`
	Warning         string  `json:"warning,omitempty"`
	RequireForce    bool    `json:"require_force,omitempty"`
	BalanceDeducted float64 `json:"balance_deducted,omitempty"`
	SubDaysDeducted int     `json:"subscription_days_deducted,omitempty"`
REDACTED

type DashboardStats struct {
	TodayAmount   float64 `json:"today_amount"`
	TotalAmount   float64 `json:"total_amount"`
	TodayCount    int     `json:"today_count"`
	TotalCount    int     `json:"total_count"`
	AvgAmount     float64 `json:"avg_amount"`
	PendingOrders int     `json:"pending_orders"`

	DailySeries    []DailyStats        `json:"daily_series"`
	PaymentMethods []PaymentMethodStat `json:"payment_methods"`
	TopUsers       []TopUserStat       `json:"top_users"`
REDACTED

type DailyStats struct {
	Date   string  `json:"date"`
	Amount float64 `json:"amount"`
	Count  int     `json:"count"`
REDACTED

type PaymentMethodStat struct {
	Type   string  `json:"type"`
	Amount float64 `json:"amount"`
	Count  int     `json:"count"`
REDACTED

type TopUserStat struct {
	UserID int64   `json:"user_id"`
	Email  string  `json:"email"`
	Amount float64 `json:"amount"`
REDACTED

// --- Service ---

type PaymentService struct {
	providerMu               sync.Mutex
	providersLoaded          bool
	entClient                *dbent.Client
	registry                 *payment.Registry
	loadBalancer             payment.LoadBalancer
	redeemService            *RedeemService
	subscriptionSvc          *SubscriptionService
	configService            *PaymentConfigService
	userRepo                 UserRepository
	groupRepo                GroupRepository
	resumeService            *PaymentResumeService
	affiliateService         *AffiliateService
	notificationEmailService *NotificationEmailService
REDACTED

func NewPaymentService(entClient *dbent.Client, registry *payment.Registry, loadBalancer payment.LoadBalancer, redeemService *RedeemService, subscriptionSvc *SubscriptionService, configService *PaymentConfigService, userRepo UserRepository, groupRepo GroupRepository, affiliateService *AffiliateService) *PaymentService {
	svc := &PaymentService{entClient: entClient, registry: registry, loadBalancer: newVisibleMethodLoadBalancer(loadBalancer, configService), redeemService: redeemService, subscriptionSvc: subscriptionSvc, configService: configService, userRepo: userRepo, groupRepo: groupRepo, affiliateService: affiliateServiceREDACTED
	svc.resumeService = psNewPaymentResumeService(configService)
	return svc
REDACTED

func (s *PaymentService) SetNotificationEmailService(notificationEmailService *NotificationEmailService) {
	s.notificationEmailService = notificationEmailService
REDACTED

// --- Provider Registry ---

// EnsureProviders lazily initializes the provider registry on first call.
func (s *PaymentService) EnsureProviders(ctx context.Context) {
	s.providerMu.Lock()
	defer s.providerMu.Unlock()
	if !s.providersLoaded {
		s.loadProviders(ctx)
		s.providersLoaded = true
REDACTED
REDACTED

// RefreshProviders clears and re-registers all providers from the database.
func (s *PaymentService) RefreshProviders(ctx context.Context) {
	s.providerMu.Lock()
	defer s.providerMu.Unlock()
	s.registry.Clear()
	s.loadProviders(ctx)
	s.providersLoaded = true
REDACTED

func (s *PaymentService) loadProviders(ctx context.Context) {
	instances, err := s.entClient.PaymentProviderInstance.Query().
		Where(paymentproviderinstance.EnabledEQ(true)).
		All(ctx)
	if err != nil {
		slog.Error("[PaymentService] failed to query provider instances", "error", err)
		return
REDACTED
	for _, inst := range instances {
		cfg, err := s.loadBalancer.GetInstanceConfig(ctx, int64(inst.ID))
		if err != nil {
			slog.Warn("[PaymentService] failed to decrypt config for instance", "instanceID", inst.ID, "error", err)
			continue
	REDACTED
		if inst.PaymentMode != "" {
			cfg["paymentMode"] = inst.PaymentMode
	REDACTED
		instID := fmt.Sprintf("%d", inst.ID)
		p, err := provider.CreateProvider(inst.ProviderKey, instID, cfg)
		if err != nil {
			slog.Warn("[PaymentService] failed to create provider for instance", "instanceID", inst.ID, "key", inst.ProviderKey, "error", err)
			continue
	REDACTED
		s.registry.Register(p)
REDACTED
REDACTED

// --- Helpers ---

func psIsRefundStatus(s string) bool {
	switch s {
	case OrderStatusRefundRequested, OrderStatusRefunding, OrderStatusRefundPending, OrderStatusPartiallyRefunded, OrderStatusRefunded, OrderStatusRefundFailed:
		return true
REDACTED
	return false
REDACTED

func psErrMsg(err error) string {
	if err == nil {
		return ""
REDACTED
	return err.Error()
REDACTED

func psNilIfEmpty(s string) *string {
	if s == "" {
		return nil
REDACTED
	return &s
REDACTED

func (s *PaymentService) paymentResume() *PaymentResumeService {
	if s.resumeService != nil {
		return s.resumeService
REDACTED
	return psNewPaymentResumeService(s.configService)
REDACTED

func NewLegacyAwarePaymentResumeService(legacyKey []byte) *PaymentResumeService {
	return newLegacyAwarePaymentResumeService(legacyKey)
REDACTED

func psNewPaymentResumeService(configService *PaymentConfigService) *PaymentResumeService {
	return newLegacyAwarePaymentResumeService(psResumeLegacyVerificationKey(configService))
REDACTED

func newLegacyAwarePaymentResumeService(legacyKey []byte) *PaymentResumeService {
	signingKey, verifyFallbacks := resolvePaymentResumeSigningKeys(legacyKey)
	return NewPaymentResumeService(signingKey, verifyFallbacks...)
REDACTED

func psResumeLegacyVerificationKey(configService *PaymentConfigService) []byte {
	if configService == nil {
		return nil
REDACTED
	return configService.encryptionKey
REDACTED

func resolvePaymentResumeSigningKeys(legacyKey []byte) ([]byte, [][]byte) {
	signingKey := parsePaymentResumeSigningKey(os.Getenv(paymentResumeSigningKeyEnv))
	if len(signingKey) == 0 {
		if len(legacyKey) == 0 {
			return nil, nil
	REDACTED
		return legacyKey, nil
REDACTED
	if len(legacyKey) == 0 || bytes.Equal(legacyKey, signingKey) {
		return signingKey, nil
REDACTED
	return signingKey, [][]byte{legacyKeyREDACTED
REDACTED

func parsePaymentResumeSigningKey(raw string) []byte {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil
REDACTED
	if len(raw) >= 64 && len(raw)%2 == 0 {
		if decoded, err := hex.DecodeString(raw); err == nil && len(decoded) > 0 {
			return decoded
	REDACTED
REDACTED
	return []byte(raw)
REDACTED

func psSliceContains(sl []string, s string) bool {
	for _, v := range sl {
		if v == s {
			return true
	REDACTED
REDACTED
	return false
REDACTED

// Subscription validity period unit constants.
const (
	validityUnitWeek   = "week"
	validityUnitWeeks  = "weeks"
	validityUnitMonth  = "month"
	validityUnitMonths = "months"
)

func psComputeValidityDays(days int, unit string) int {
	switch unit {
	case validityUnitWeek, validityUnitWeeks:
		return days * 7
	case validityUnitMonth, validityUnitMonths:
		return days * 30
	default:
		return days
REDACTED
REDACTED

func psStartOfDayUTC(t time.Time) time.Time {
	y, m, d := t.UTC().Date()
	return time.Date(y, m, d, 0, 0, 0, 0, time.UTC)
REDACTED

func applyPagination(pageSize, page int) (size, pg int) {
	size = pageSize
	if size <= 0 {
		size = defaultPageSize
REDACTED
	if size > maxPageSize {
		size = maxPageSize
REDACTED
	pg = page
	if pg < 1 {
		pg = 1
REDACTED
	return size, pg
REDACTED
