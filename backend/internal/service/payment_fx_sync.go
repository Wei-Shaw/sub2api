package service

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"math"
	"sort"
	"strings"
	"sync"
	"time"
)

type PaymentFXStatus struct {
	Source            string     `json:"source"`
	UpdatedAt         *time.Time `json:"updated_at,omitempty"`
	StaleAfterSeconds int        `json:"stale_after_seconds"`
	Stale             bool       `json:"stale"`
	MissingCurrencies []string   `json:"missing_currencies"`
}

type PaymentFXRateRequest struct {
	LedgerCurrency      string
	PaymentCurrencies   []string
	ExistingManualRates map[string]float64
}

type PaymentFXRateSnapshot struct {
	Rates     map[string]float64
	Source    string
	Timestamp time.Time
}

type PaymentFXRateProvider interface {
	FetchPaymentFXRates(ctx context.Context, req PaymentFXRateRequest) (PaymentFXRateSnapshot, error)
}

type PaymentFXSyncResult struct {
	Rates             map[string]float64 `json:"rates"`
	Source            string             `json:"source"`
	UpdatedAt         time.Time          `json:"updated_at"`
	LedgerCurrency    string             `json:"ledger_currency"`
	PaymentCurrencies []string           `json:"payment_currencies"`
	MissingCurrencies []string           `json:"missing_currencies"`
}

type PaymentFXSyncRetryPolicy struct {
	MaxAttempts    int
	InitialBackoff time.Duration
	MaxBackoff     time.Duration
}

type PaymentFXSyncService struct {
	configService *PaymentConfigService
	settingRepo   SettingRepository
	provider      PaymentFXRateProvider
	providers     map[string]PaymentFXRateProvider
	retryPolicy   PaymentFXSyncRetryPolicy
	now           func() time.Time
	sleep         func(context.Context, time.Duration) error

	mu     sync.Mutex
	cancel context.CancelFunc
	done   chan struct{}
}

func NewPaymentFXSyncService(configService *PaymentConfigService, settingRepo SettingRepository, provider PaymentFXRateProvider) *PaymentFXSyncService {
	if settingRepo == nil && configService != nil {
		settingRepo = configService.settingRepo
	}
	return &PaymentFXSyncService{
		configService: configService,
		settingRepo:   settingRepo,
		provider:      provider,
		now:           time.Now,
		sleep:         paymentFXSleep,
		retryPolicy: PaymentFXSyncRetryPolicy{
			MaxAttempts:    3,
			InitialBackoff: 2 * time.Second,
			MaxBackoff:     30 * time.Second,
		},
	}
}

func (s *PaymentFXSyncService) SetProviders(providers map[string]PaymentFXRateProvider) {
	if s == nil {
		return
	}
	s.providers = map[string]PaymentFXRateProvider{}
	for name, provider := range providers {
		name = normalizeFXProviderName(name)
		if name != "" && provider != nil {
			s.providers[name] = provider
		}
	}
}

func (s *PaymentFXSyncService) SetRetryPolicy(policy PaymentFXSyncRetryPolicy) {
	if s == nil {
		return
	}
	if policy.MaxAttempts <= 0 {
		policy.MaxAttempts = 3
	}
	if policy.InitialBackoff <= 0 {
		policy.InitialBackoff = 2 * time.Second
	}
	if policy.MaxBackoff <= 0 {
		policy.MaxBackoff = 30 * time.Second
	}
	if policy.MaxBackoff < policy.InitialBackoff {
		policy.MaxBackoff = policy.InitialBackoff
	}
	s.retryPolicy = policy
}

func (s *PaymentFXSyncService) Start() {
	if s == nil {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.cancel != nil {
		return
	}
	ctx, cancel := context.WithCancel(context.Background())
	s.cancel = cancel
	s.done = make(chan struct{})
	go func() {
		defer close(s.done)
		slog.Info("payment fx sync scheduler started")
		s.Run(ctx, 0, func(err error) {
			slog.Error("payment fx sync failed", "error", err)
		})
		slog.Info("payment fx sync scheduler stopped")
	}()
}

func (s *PaymentFXSyncService) Stop() {
	if s == nil {
		return
	}
	s.mu.Lock()
	cancel := s.cancel
	done := s.done
	s.cancel = nil
	s.done = nil
	s.mu.Unlock()
	if cancel != nil {
		cancel()
	}
	if done != nil {
		<-done
	}
}

func (s *PaymentFXSyncService) SyncOnce(ctx context.Context) (*PaymentFXSyncResult, error) {
	if s == nil || s.configService == nil || s.settingRepo == nil {
		return nil, fmt.Errorf("payment fx sync service is not configured")
	}
	cfg, err := s.configService.GetPaymentConfig(ctx)
	if err != nil {
		return nil, err
	}
	ledgerCurrency := normalizeCurrencyCode(cfg.LedgerCurrency, defaultLedgerCurrency)
	paymentCurrencies := paymentFXSyncCurrencies(ledgerCurrency, cfg.AllowedPaymentCurrencies)
	providerName := normalizeFXProviderName(cfg.FXAutoSyncProvider)

	snapshot := PaymentFXRateSnapshot{
		Rates:     copyFXRates(cfg.ManualFXRates),
		Source:    fxSourceManual,
		Timestamp: s.now().UTC(),
	}
	provider := s.provider
	if provider == nil && providerName != "" && providerName != fxSourceManual {
		provider = s.providers[providerName]
		if provider == nil {
			return nil, fmt.Errorf("payment fx provider %q is not configured", providerName)
		}
	}
	if provider != nil {
		snapshot, err = provider.FetchPaymentFXRates(ctx, PaymentFXRateRequest{
			LedgerCurrency:      ledgerCurrency,
			PaymentCurrencies:   paymentCurrencies,
			ExistingManualRates: copyFXRates(cfg.ManualFXRates),
		})
		if err != nil {
			return nil, err
		}
	}

	rates, missing := validatePaymentFXRates(ledgerCurrency, paymentCurrencies, snapshot.Rates)
	if len(missing) > 0 {
		return nil, fmt.Errorf("fx rates missing for currencies: %s", strings.Join(missing, ","))
	}
	updatedAt := snapshot.Timestamp.UTC()
	if updatedAt.IsZero() {
		updatedAt = s.now().UTC()
	}
	source := normalizeFXSource(snapshot.Source)
	ratesJSON, err := marshalFXRatesJSON(rates)
	if err != nil {
		return nil, err
	}
	if err := s.settingRepo.SetMultiple(ctx, map[string]string{
		SettingManualFXRates:    ratesJSON,
		SettingFXRatesSource:    source,
		SettingFXRatesUpdatedAt: updatedAt.Format(time.RFC3339),
	}); err != nil {
		return nil, err
	}
	return &PaymentFXSyncResult{
		Rates:             rates,
		Source:            source,
		UpdatedAt:         updatedAt,
		LedgerCurrency:    ledgerCurrency,
		PaymentCurrencies: paymentCurrencies,
	}, nil
}

func (s *PaymentFXSyncService) SyncOnceWithRetry(ctx context.Context) (*PaymentFXSyncResult, error) {
	if s == nil {
		return nil, fmt.Errorf("payment fx sync service is not configured")
	}
	policy := s.retryPolicy
	if policy.MaxAttempts <= 0 {
		policy.MaxAttempts = 1
	}
	backoff := policy.InitialBackoff
	if backoff <= 0 {
		backoff = time.Second
	}
	if policy.MaxBackoff <= 0 {
		policy.MaxBackoff = backoff
	}
	var lastErr error
	for attempt := 1; attempt <= policy.MaxAttempts; attempt++ {
		started := s.now()
		result, err := s.SyncOnce(ctx)
		if err == nil {
			slog.Info("payment fx sync succeeded",
				"source", result.Source,
				"ledger_currency", result.LedgerCurrency,
				"payment_currencies", result.PaymentCurrencies,
				"updated_at", result.UpdatedAt.Format(time.RFC3339),
				"duration_ms", s.now().Sub(started).Milliseconds(),
			)
			return result, nil
		}
		lastErr = err
		if attempt == policy.MaxAttempts {
			break
		}
		slog.Warn("payment fx sync attempt failed; retrying",
			"attempt", attempt,
			"max_attempts", policy.MaxAttempts,
			"backoff", backoff.String(),
			"error", err,
		)
		if err := s.sleep(ctx, backoff); err != nil {
			return nil, err
		}
		backoff *= 2
		if backoff > policy.MaxBackoff {
			backoff = policy.MaxBackoff
		}
	}
	return nil, fmt.Errorf("payment fx sync failed after %d attempt(s): %w", policy.MaxAttempts, lastErr)
}

func (s *PaymentFXSyncService) Run(ctx context.Context, interval time.Duration, onError func(error)) {
	if s == nil {
		return
	}
	for {
		cfg, cfgErr := s.configService.GetPaymentConfig(ctx)
		nextInterval := interval
		if nextInterval <= 0 {
			nextInterval = time.Duration(defaultFXAutoSyncIntervalSec) * time.Second
			if cfgErr == nil && cfg != nil && cfg.FXAutoSyncIntervalSec > 0 {
				nextInterval = time.Duration(cfg.FXAutoSyncIntervalSec) * time.Second
			}
		}
		if cfgErr != nil {
			if onError != nil {
				onError(cfgErr)
			}
		} else if cfg != nil && cfg.FXAutoSyncEnabled {
			if _, err := s.SyncOnceWithRetry(ctx); err != nil && onError != nil {
				onError(err)
			}
		} else {
			slog.Debug("payment fx auto sync disabled; skipping cycle")
		}

		if err := s.sleep(ctx, nextInterval); err != nil {
			return
		}
	}
}

type StaticPaymentFXRateProvider struct {
	Rates     map[string]float64
	Source    string
	Timestamp time.Time
}

func (p StaticPaymentFXRateProvider) FetchPaymentFXRates(context.Context, PaymentFXRateRequest) (PaymentFXRateSnapshot, error) {
	return PaymentFXRateSnapshot{Rates: copyFXRates(p.Rates), Source: p.Source, Timestamp: p.Timestamp}, nil
}

func buildPaymentFXStatus(cfg *PaymentConfig, vals map[string]string, now time.Time) PaymentFXStatus {
	if cfg == nil {
		return PaymentFXStatus{Source: defaultFXRatesSource, StaleAfterSeconds: defaultFXRatesStaleAfterSeconds, Stale: true}
	}
	staleAfterSeconds := pcParseInt(vals[SettingFXRatesStaleAfterSeconds], defaultFXRatesStaleAfterSeconds)
	if staleAfterSeconds <= 0 {
		staleAfterSeconds = defaultFXRatesStaleAfterSeconds
	}
	updatedAt := parseFXUpdatedAt(vals[SettingFXRatesUpdatedAt])
	status := PaymentFXStatus{
		Source:            normalizeFXSource(vals[SettingFXRatesSource]),
		UpdatedAt:         updatedAt,
		StaleAfterSeconds: staleAfterSeconds,
		MissingCurrencies: missingPaymentFXCurrencies(cfg.LedgerCurrency, cfg.AllowedPaymentCurrencies, cfg.ManualFXRates),
	}
	if status.Source == "" {
		status.Source = defaultFXRatesSource
	}
	if updatedAt == nil {
		status.Stale = true
	} else if now.UTC().After(updatedAt.UTC().Add(time.Duration(staleAfterSeconds) * time.Second)) {
		status.Stale = true
	}
	if len(status.MissingCurrencies) > 0 {
		status.Stale = true
	}
	return status
}

func parseFXUpdatedAt(raw string) *time.Time {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil
	}
	parsed, err := time.Parse(time.RFC3339, raw)
	if err != nil {
		return nil
	}
	parsed = parsed.UTC()
	return &parsed
}

func normalizeFXSource(source string) string {
	source = strings.TrimSpace(strings.ToLower(source))
	if source == "" {
		return defaultFXRatesSource
	}
	return source
}

func normalizeFXProviderName(provider string) string {
	provider = strings.TrimSpace(strings.ToLower(provider))
	provider = strings.ReplaceAll(provider, "_", "")
	provider = strings.ReplaceAll(provider, "-", "")
	switch provider {
	case "openexchangerates", "openexchange", "oer":
		return PaymentFXProviderOpenExchangeRates
	case "", fxSourceManual:
		return fxSourceManual
	default:
		return provider
	}
}

func paymentFXSyncCurrencies(ledgerCurrency string, paymentCurrencies []string) []string {
	currencies := append([]string{}, paymentCurrencies...)
	currencies = append(currencies, ledgerCurrency)
	return normalizeCurrencyList(currencies)
}

func validatePaymentFXRates(ledgerCurrency string, currencies []string, rates map[string]float64) (map[string]float64, []string) {
	ledgerCurrency = normalizeCurrencyCode(ledgerCurrency, defaultLedgerCurrency)
	out := map[string]float64{ledgerCurrency: 1}
	missing := make([]string, 0)
	for _, currency := range paymentFXSyncCurrencies(ledgerCurrency, currencies) {
		if currency == ledgerCurrency {
			out[currency] = 1
			continue
		}
		rate := rates[normalizeCurrencyCode(currency, "")]
		if math.IsNaN(rate) || math.IsInf(rate, 0) || rate <= 0 {
			missing = append(missing, currency)
			continue
		}
		out[currency] = rate
	}
	sort.Strings(missing)
	return out, missing
}

func missingPaymentFXCurrencies(ledgerCurrency string, paymentCurrencies []string, rates map[string]float64) []string {
	_, missing := validatePaymentFXRates(ledgerCurrency, paymentCurrencies, rates)
	return missing
}

func copyFXRates(in map[string]float64) map[string]float64 {
	out := make(map[string]float64, len(in))
	for currency, rate := range in {
		out[normalizeCurrencyCode(currency, "")] = rate
	}
	return out
}

func marshalFXRatesJSON(rates map[string]float64) (string, error) {
	ordered := make(map[string]float64, len(rates))
	keys := make([]string, 0, len(rates))
	for currency := range rates {
		keys = append(keys, currency)
	}
	sort.Strings(keys)
	for _, currency := range keys {
		ordered[currency] = rates[currency]
	}
	blob, err := json.Marshal(ordered)
	if err != nil {
		return "", err
	}
	return string(blob), nil
}

func paymentFXSleep(ctx context.Context, d time.Duration) error {
	if d <= 0 {
		return nil
	}
	timer := time.NewTimer(d)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}
