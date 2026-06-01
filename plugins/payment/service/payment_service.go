package service

import (
	"context"
	"fmt"
	"log/slog"
	"math/rand/v2"
	"sync"
	"time"

	"github.com/shopspring/decimal"

	pluginsdk "github.com/Wei-Shaw/sub2api/plugin-sdk"
	pluginent "github.com/Wei-Shaw/sub2api/plugins/payment/ent"
	"github.com/Wei-Shaw/sub2api/plugins/payment/ent/paymentproviderinstance"
	paymentpkg "github.com/Wei-Shaw/sub2api/plugins/payment/internal/payment"
	"github.com/Wei-Shaw/sub2api/plugins/payment/internal/payment/provider"
)

// --- Order status constants (re-exported from internal/payment) ---

const (
	OrderStatusPending           = paymentpkg.OrderStatusPending
	OrderStatusPaid              = paymentpkg.OrderStatusPaid
	OrderStatusRecharging        = paymentpkg.OrderStatusRecharging
	OrderStatusCompleted         = paymentpkg.OrderStatusCompleted
	OrderStatusExpired           = paymentpkg.OrderStatusExpired
	OrderStatusCancelled         = paymentpkg.OrderStatusCancelled
	OrderStatusFailed            = paymentpkg.OrderStatusFailed
	OrderStatusRefundRequested   = paymentpkg.OrderStatusRefundRequested
	OrderStatusRefunding         = paymentpkg.OrderStatusRefunding
	OrderStatusPartiallyRefunded = paymentpkg.OrderStatusPartiallyRefunded
	OrderStatusRefunded          = paymentpkg.OrderStatusRefunded
	OrderStatusRefundFailed      = paymentpkg.OrderStatusRefundFailed
)

// Page-size and order-id formatting constants.
const (
	defaultPageSize = 20
	maxPageSize     = 100
	topUsersLimit   = 10
	orderIDPrefix   = "sub2_"

	// paymentGraceMinutes pads the visible window (PENDING -> EXPIRED)
	// so a slow webhook does not lose a payment that completed within
	// seconds of timeout.
	paymentGraceMinutes = 5
)

// PaymentService is the central service the payment plugin's handlers
// depend on. Most business logic lives in payment_*.go files next to
// this one and is currently behind the payment_services_wip build tag
// pending full SDK adaptation.
//
// Construction is intentionally permissive: every dependency is optional
// so the plugin can boot in degraded mode (returning 503 from the
// affected endpoints) when the host has not granted the matching
// capability.
type PaymentService struct {
	providerMu      sync.Mutex
	providersLoaded bool

	entClient    *pluginent.Client
	registry     *paymentpkg.Registry
	loadBalancer paymentpkg.LoadBalancer
	config       *PaymentConfigService
	host         pluginsdk.HostPaymentClient
	events       pluginsdk.EventsClient
	secrets      pluginsdk.SecretEncryptor
	settings     pluginsdk.SettingsClient
	logger       *slog.Logger
	resume       *PaymentResumeService
}

// PaymentServiceDeps bundles the dependencies NewPaymentService needs.
// Using a struct rather than a positional argument list keeps additions
// backwards-compatible — adding a new dependency means a new field, not
// a churning constructor signature shared with mocks across many tests.
type PaymentServiceDeps struct {
	EntClient    *pluginent.Client
	Registry     *paymentpkg.Registry
	LoadBalancer paymentpkg.LoadBalancer
	Config       *PaymentConfigService
	Host         pluginsdk.HostPaymentClient
	Events       pluginsdk.EventsClient
	Secrets      pluginsdk.SecretEncryptor
	Settings     pluginsdk.SettingsClient
	Logger       *slog.Logger
}

// NewPaymentService constructs a PaymentService from the SDK-provided
// dependencies. The returned service shares the supplied logger; if nil
// the slog default is used so log lines still appear.
func NewPaymentService(deps PaymentServiceDeps) *PaymentService {
	logger := deps.Logger
	if logger == nil {
		logger = slog.Default()
	}
	svc := &PaymentService{
		entClient:    deps.EntClient,
		registry:     deps.Registry,
		loadBalancer: deps.LoadBalancer,
		config:       deps.Config,
		host:         deps.Host,
		events:       deps.Events,
		secrets:      deps.Secrets,
		settings:     deps.Settings,
		logger:       logger,
	}
	svc.resume = NewPaymentResumeService(deps.Settings)
	return svc
}

// Resume returns the embedded PaymentResumeService so handlers can
// mint or validate resume tokens directly.
func (s *PaymentService) Resume() *PaymentResumeService {
	if s == nil {
		return nil
	}
	return s.resume
}

// Registry returns the live provider registry. The webhook handler
// consumes it directly to fall back onto registry-level lookups when the
// order's pinned provider is missing. May be nil before EnsureProviders
// has been called.
func (s *PaymentService) Registry() *paymentpkg.Registry {
	if s == nil {
		return nil
	}
	return s.registry
}

// EnsureProviders lazily initialises the provider registry on first
// call. Subsequent invocations are cheap.
func (s *PaymentService) EnsureProviders(ctx context.Context) {
	if s == nil {
		return
	}
	s.providerMu.Lock()
	defer s.providerMu.Unlock()
	if s.providersLoaded {
		return
	}
	s.loadProviders(ctx)
	s.providersLoaded = true
}

// RefreshProviders re-reads the configured provider instances from the
// database and rebuilds the registry. Called from admin write paths
// (provider create/update/delete) so a config change takes effect
// without restarting the plugin.
func (s *PaymentService) RefreshProviders(ctx context.Context) {
	if s == nil {
		return
	}
	s.providerMu.Lock()
	defer s.providerMu.Unlock()
	if s.registry != nil {
		s.registry.Clear()
	}
	s.loadProviders(ctx)
	s.providersLoaded = true
}

// loadProviders enumerates all enabled payment_provider_instances rows
// and registers a Provider in the registry for each one. Decryption
// failures and unknown provider keys are logged and skipped so a single
// bad row cannot stop the rest from coming online.
func (s *PaymentService) loadProviders(ctx context.Context) {
	if s.entClient == nil || s.loadBalancer == nil || s.registry == nil {
		return
	}
	instances, err := s.entClient.PaymentProviderInstance.Query().
		Where(paymentproviderinstance.EnabledEQ(true)).
		All(ctx)
	if err != nil {
		s.logger.Error("payment: failed to query provider instances", "error", err)
		return
	}
	for _, inst := range instances {
		cfg, err := s.loadBalancer.GetInstanceConfig(ctx, int64(inst.ID))
		if err != nil {
			s.logger.Warn("payment: failed to decrypt config for instance",
				"instance_id", inst.ID, "error", err)
			continue
		}
		if cfg == nil {
			cfg = map[string]string{}
		}
		if inst.PaymentMode != "" {
			cfg["paymentMode"] = inst.PaymentMode
		}
		instID := fmt.Sprintf("%d", inst.ID)
		p, err := provider.CreateProvider(inst.ProviderKey, instID, cfg)
		if err != nil {
			s.logger.Warn("payment: failed to create provider for instance",
				"instance_id", inst.ID, "key", inst.ProviderKey, "error", err)
			continue
		}
		s.registry.Register(p)
	}
}

// generateOutTradeNo creates the user-visible external order id used
// by payment providers. Format: sub2_YYYYMMDD<8 alphanumeric chars>.
func generateOutTradeNo() string {
	date := time.Now().Format("20060102")
	return orderIDPrefix + date + generateRandomString(8)
}

func generateRandomString(n int) string {
	const charset = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789"
	b := make([]byte, n)
	for i := range b {
		b[i] = charset[rand.IntN(len(charset))]
	}
	return string(b)
}

// applyPagination clamps page-size and page values to the service-wide
// bounds. Both arguments are zero-tolerant; size <= 0 falls back to
// defaultPageSize and page < 1 falls back to 1.
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

// DashboardStats is the aggregate snapshot returned by GetDashboardStats.
// Currency-typed fields are shopspring/decimal so the JSON encoder emits
// exact strings rather than IEEE-754 drift. The frontend already accepts
// decimal-encoded numbers without changes.
type DashboardStats struct {
	TodayAmount   decimal.Decimal `json:"today_amount"`
	TotalAmount   decimal.Decimal `json:"total_amount"`
	TodayCount    int             `json:"today_count"`
	TotalCount    int             `json:"total_count"`
	AvgAmount     decimal.Decimal `json:"avg_amount"`
	PendingOrders int             `json:"pending_orders"`

	DailySeries    []DailyStats        `json:"daily_series"`
	PaymentMethods []PaymentMethodStat `json:"payment_methods"`
	TopUsers       []TopUserStat       `json:"top_users"`
}

// DailyStats is one row of the dashboard's date-series chart.
type DailyStats struct {
	Date   string          `json:"date"`
	Amount decimal.Decimal `json:"amount"`
	Count  int             `json:"count"`
}

// PaymentMethodStat is one row of the dashboard's payment-method
// distribution chart.
type PaymentMethodStat struct {
	Type   string          `json:"type"`
	Amount decimal.Decimal `json:"amount"`
	Count  int             `json:"count"`
}

// TopUserStat is one row of the dashboard's top-users leaderboard.
type TopUserStat struct {
	UserID int64           `json:"user_id"`
	Email  string          `json:"email"`
	Amount decimal.Decimal `json:"amount"`
}

// helpers used by the unported files (kept here so the build tag block
// can be removed file-by-file without dragging the helpers along too).

// psStringValue dereferences a *string that the ent generator emits for
// nullable text columns. nil pointer collapses to empty string.
func psStringValue(value *string) string {
	if value == nil {
		return ""
	}
	return *value
}

// psNilIfEmpty inverts psStringValue: an empty string becomes nil so the
// caller can clear an ent column on update without writing a literal "".
func psNilIfEmpty(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}

// psErrMsg returns err.Error() or "" — convenient when surfacing the
// failure reason via an audit log column that takes a plain string.
func psErrMsg(err error) string {
	if err == nil {
		return ""
	}
	return err.Error()
}
