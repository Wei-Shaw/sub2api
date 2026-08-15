package service

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"log/slog"
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
	OrderStatusPending    = payment.OrderStatusPending
	OrderStatusPaid       = payment.OrderStatusPaid
	OrderStatusRecharging = payment.OrderStatusRecharging
	OrderStatusCompleted  = payment.OrderStatusCompleted
	OrderStatusExpired    = payment.OrderStatusExpired
	OrderStatusCancelled  = payment.OrderStatusCancelled
	OrderStatusFailed     = payment.OrderStatusFailed
)

const (
	// defaultMaxPendingOrders and defaultOrderTimeoutMin are defined in
	// payment_config_service.go alongside other payment configuration defaults.
	paymentGraceMinutes = 5

	defaultPageSize = 20
	maxPageSize     = 100
	topUsersLimit   = 10
	// amountTolerance absorbs float rounding when comparing a provider-reported
	// amount to the order's own pay_amount.
	amountTolerance = 0.01

	orderIDPrefix = payment.OrderCodePrefix
)

const paymentResumeSigningKeyEnv = "PAYMENT_RESUME_SIGNING_KEY"

// --- Types ---

// generateOutTradeNo creates a unique external order ID for payment providers.
// Format: SUB220250409AB3KX9MQ (prefix + date + 8-char random)
//
// The alphabet is uppercase-only because SePay orders are matched by finding
// this code inside a bank transfer description, and banks routinely uppercase
// that text — a mixed-case code could not be mapped back to one exact order.
func generateOutTradeNo() string {
	date := time.Now().Format("20060102")
	rnd := generateRandomString(payment.OrderCodeRandomLength)
	return orderIDPrefix + date + rnd
}

// generateRandomString draws n characters uniformly from
// payment.OrderCodeAlphabet using crypto/rand.
//
// The order code is the only identifier tying a bank transfer — and the
// unauthenticated public order lookup — back to one order, so it must not come
// from a predictable generator. Bytes at or above limit are rejected instead of
// folded, which would otherwise bias the first 256%len(alphabet) letters.
func generateRandomString(n int) string {
	size := len(payment.OrderCodeAlphabet)
	limit := 256 - (256 % size)

	out := make([]byte, 0, n)
	buf := make([]byte, n)
	for len(out) < n {
		// crypto/rand.Read never returns an error and always fills buf.
		_, _ = rand.Read(buf)
		for _, v := range buf {
			if int(v) >= limit {
				continue
			}
			out = append(out, payment.OrderCodeAlphabet[int(v)%size])
			if len(out) == n {
				break
			}
		}
	}
	return string(out)
}

type CreateOrderRequest struct {
	UserID        int64
	Amount        float64
	PaymentType   string
	ClientIP      string
	IsMobile      bool
	SrcHost       string
	SrcURL        string
	ReturnURL     string
	PaymentSource string
	OrderType     string
	PlanID        int64
	Locale        string
}

type CreateOrderResponse struct {
	OrderID     int64                           `json:"order_id"`
	Amount      float64                         `json:"amount"`
	PayAmount   float64                         `json:"pay_amount"`
	FeeRate     float64                         `json:"fee_rate"`
	Status      string                          `json:"status"`
	ResultType  payment.CreatePaymentResultType `json:"result_type,omitempty"`
	PaymentType string                          `json:"payment_type"`
	OutTradeNo  string                          `json:"out_trade_no,omitempty"`
	PayURL      string                          `json:"pay_url,omitempty"`
	QRCode      string                          `json:"qr_code,omitempty"`
	IntentID    string                          `json:"intent_id,omitempty"`
	Currency    string                          `json:"currency,omitempty"`
	Transfer    *payment.BankTransferInfo       `json:"transfer,omitempty"`
	ExpiresAt   time.Time                       `json:"expires_at"`
	PaymentMode string                          `json:"payment_mode,omitempty"`
	ResumeToken string                          `json:"resume_token,omitempty"`
}

type OrderListParams struct {
	Page        int
	PageSize    int
	Status      string
	OrderType   string
	PaymentType string
	Keyword     string
}

type DashboardStats struct {
	TodayAmount   CurrencyAmounts `json:"today_amount"`
	TotalAmount   CurrencyAmounts `json:"total_amount"`
	TodayCount    int             `json:"today_count"`
	TotalCount    int             `json:"total_count"`
	AvgAmount     CurrencyAmounts `json:"avg_amount"`
	PendingOrders int             `json:"pending_orders"`

	DailySeries    []DailyStats        `json:"daily_series"`
	PaymentMethods []PaymentMethodStat `json:"payment_methods"`
	TopUsers       TopUsersByCurrency  `json:"top_users"`
}

// CurrencyAmounts holds payment amounts keyed by their ISO 4217 currency.
// Amounts in different currencies must never be added together.
type CurrencyAmounts map[string]float64

type DailyStats struct {
	Date   string          `json:"date"`
	Amount CurrencyAmounts `json:"amount"`
	Count  int             `json:"count"`
}

type PaymentMethodStat struct {
	Type   string          `json:"type"`
	Amount CurrencyAmounts `json:"amount"`
	Count  int             `json:"count"`
}

type TopUserStat struct {
	UserID int64   `json:"user_id"`
	Email  string  `json:"email"`
	Amount float64 `json:"amount"`
}

// TopUsersByCurrency contains an independent ranked user list for each
// currency. A single cross-currency leaderboard would be misleading.
type TopUsersByCurrency map[string][]TopUserStat

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
}

func NewPaymentService(entClient *dbent.Client, registry *payment.Registry, loadBalancer payment.LoadBalancer, redeemService *RedeemService, subscriptionSvc *SubscriptionService, configService *PaymentConfigService, userRepo UserRepository, groupRepo GroupRepository, affiliateService *AffiliateService) *PaymentService {
	svc := &PaymentService{entClient: entClient, registry: registry, loadBalancer: loadBalancer, redeemService: redeemService, subscriptionSvc: subscriptionSvc, configService: configService, userRepo: userRepo, groupRepo: groupRepo, affiliateService: affiliateService}
	svc.resumeService = psNewPaymentResumeService(configService)
	return svc
}

func (s *PaymentService) SetNotificationEmailService(notificationEmailService *NotificationEmailService) {
	s.notificationEmailService = notificationEmailService
}

// --- Provider Registry ---

// EnsureProviders lazily initializes the provider registry on first call.
func (s *PaymentService) EnsureProviders(ctx context.Context) {
	s.providerMu.Lock()
	defer s.providerMu.Unlock()
	if !s.providersLoaded {
		s.loadProviders(ctx)
		s.providersLoaded = true
	}
}

// RefreshProviders clears and re-registers all providers from the database.
func (s *PaymentService) RefreshProviders(ctx context.Context) {
	s.providerMu.Lock()
	defer s.providerMu.Unlock()
	s.registry.Clear()
	s.loadProviders(ctx)
	s.providersLoaded = true
}

func (s *PaymentService) loadProviders(ctx context.Context) {
	instances, err := s.entClient.PaymentProviderInstance.Query().
		Where(paymentproviderinstance.EnabledEQ(true)).
		All(ctx)
	if err != nil {
		slog.Error("[PaymentService] failed to query provider instances", "error", err)
		return
	}
	for _, inst := range instances {
		cfg, err := s.loadBalancer.GetInstanceConfig(ctx, int64(inst.ID))
		if err != nil {
			slog.Warn("[PaymentService] failed to decrypt config for instance", "instanceID", inst.ID, "error", err)
			continue
		}
		if inst.PaymentMode != "" {
			cfg["paymentMode"] = inst.PaymentMode
		}
		instID := fmt.Sprintf("%d", inst.ID)
		p, err := provider.CreateProvider(inst.ProviderKey, instID, cfg)
		if err != nil {
			slog.Warn("[PaymentService] failed to create provider for instance", "instanceID", inst.ID, "key", inst.ProviderKey, "error", err)
			continue
		}
		s.registry.Register(p)
	}
}

// --- Helpers ---

func psErrMsg(err error) string {
	if err == nil {
		return ""
	}
	return err.Error()
}

func psNilIfEmpty(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}

func (s *PaymentService) paymentResume() *PaymentResumeService {
	if s.resumeService != nil {
		return s.resumeService
	}
	return psNewPaymentResumeService(s.configService)
}

func NewLegacyAwarePaymentResumeService(legacyKey []byte) *PaymentResumeService {
	return newLegacyAwarePaymentResumeService(legacyKey)
}

func psNewPaymentResumeService(configService *PaymentConfigService) *PaymentResumeService {
	return newLegacyAwarePaymentResumeService(psResumeLegacyVerificationKey(configService))
}

func newLegacyAwarePaymentResumeService(legacyKey []byte) *PaymentResumeService {
	signingKey, verifyFallbacks := resolvePaymentResumeSigningKeys(legacyKey)
	return NewPaymentResumeService(signingKey, verifyFallbacks...)
}

func psResumeLegacyVerificationKey(configService *PaymentConfigService) []byte {
	if configService == nil {
		return nil
	}
	return configService.encryptionKey
}

func resolvePaymentResumeSigningKeys(legacyKey []byte) ([]byte, [][]byte) {
	signingKey := parsePaymentResumeSigningKey(os.Getenv(paymentResumeSigningKeyEnv))
	if len(signingKey) == 0 {
		if len(legacyKey) == 0 {
			return nil, nil
		}
		return legacyKey, nil
	}
	if len(legacyKey) == 0 || bytes.Equal(legacyKey, signingKey) {
		return signingKey, nil
	}
	return signingKey, [][]byte{legacyKey}
}

func parsePaymentResumeSigningKey(raw string) []byte {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil
	}
	if len(raw) >= 64 && len(raw)%2 == 0 {
		if decoded, err := hex.DecodeString(raw); err == nil && len(decoded) > 0 {
			return decoded
		}
	}
	return []byte(raw)
}

func psSliceContains(sl []string, s string) bool {
	for _, v := range sl {
		if v == s {
			return true
		}
	}
	return false
}

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
	}
}

func psStartOfDayUTC(t time.Time) time.Time {
	y, m, d := t.UTC().Date()
	return time.Date(y, m, d, 0, 0, 0, 0, time.UTC)
}

func applyPagination(pageSize, page int) (size, pg int) {
	size = pageSize
	if size <= 0 {
		size = defaultPageSize
	}
	if size > maxPageSize {
		size = maxPageSize
	}
	pg = page
	if pg < 1 {
		pg = 1
	}
	return size, pg
}
