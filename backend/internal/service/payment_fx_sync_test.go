//go:build unit

package service

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestPaymentFXStatusDetectsStaleAndMissingCurrencies(t *testing.T) {
	updatedAt := time.Now().UTC().Add(-2 * time.Hour)
	svc := &PaymentConfigService{}
	cfg := svc.parsePaymentConfig(map[string]string{
		SettingLedgerCurrency:           "USD",
		SettingAllowedPaymentCurrencies: "USD,VND,KRW",
		SettingManualFXRates:            `{"USD":1,"VND":0.000039215686}`,
		SettingFXRatesSource:            "auto-test",
		SettingFXRatesUpdatedAt:         updatedAt.Format(time.RFC3339),
		SettingFXRatesStaleAfterSeconds: "60",
	})

	require.Equal(t, "auto-test", cfg.FXStatus.Source)
	require.True(t, cfg.FXStatus.Stale)
	require.Equal(t, []string{"KRW"}, cfg.FXStatus.MissingCurrencies)
	require.NotNil(t, cfg.FXStatus.UpdatedAt)
}

func TestPaymentFXSyncServiceSyncOnceUsesManualRatesByDefault(t *testing.T) {
	repo := &paymentConfigSettingRepoStub{values: map[string]string{
		SettingLedgerCurrency:           "USD",
		SettingAllowedPaymentCurrencies: "USD,VND,CNY",
		SettingManualFXRates:            `{"USD":1,"VND":0.000039215686,"CNY":0.139}`,
	}}
	configSvc := NewPaymentConfigService(nil, repo, nil)
	syncSvc := NewPaymentFXSyncService(configSvc, repo, nil)
	fixed := time.Date(2026, 4, 29, 10, 30, 0, 0, time.UTC)
	syncSvc.now = func() time.Time { return fixed }

	result, err := syncSvc.SyncOnce(context.Background())
	require.NoError(t, err)
	require.Equal(t, "manual", result.Source)
	require.Equal(t, fixed, result.UpdatedAt)
	require.Equal(t, "USD", result.LedgerCurrency)
	require.ElementsMatch(t, []string{"USD", "VND", "CNY"}, result.PaymentCurrencies)
	require.Equal(t, fixed.Format(time.RFC3339), repo.values[SettingFXRatesUpdatedAt])
	require.Equal(t, "manual", repo.values[SettingFXRatesSource])

	var stored map[string]float64
	require.NoError(t, json.Unmarshal([]byte(repo.values[SettingManualFXRates]), &stored))
	require.Equal(t, 1.0, stored["USD"])
	require.InDelta(t, 0.000039215686, stored["VND"], 0.000000000001)
	require.InDelta(t, 0.139, stored["CNY"], 0.000000000001)
}

func TestPaymentFXSyncServiceSyncOnceUsesInjectedProvider(t *testing.T) {
	repo := &paymentConfigSettingRepoStub{values: map[string]string{
		SettingLedgerCurrency:           "USD",
		SettingAllowedPaymentCurrencies: "USD,VND,CNY",
		SettingManualFXRates:            `{"USD":1,"VND":0.000039215686,"CNY":0.139}`,
	}}
	configSvc := NewPaymentConfigService(nil, repo, nil)
	fixed := time.Date(2026, 4, 29, 11, 0, 0, 0, time.UTC)
	syncSvc := NewPaymentFXSyncService(configSvc, repo, StaticPaymentFXRateProvider{
		Rates:     map[string]float64{"USD": 1, "VND": 0.00004, "CNY": 0.14},
		Source:    "unit-provider",
		Timestamp: fixed,
	})

	result, err := syncSvc.SyncOnce(context.Background())
	require.NoError(t, err)
	require.Equal(t, "unit-provider", result.Source)
	require.Equal(t, fixed, result.UpdatedAt)
	require.InDelta(t, 0.00004, result.Rates["VND"], 0.000000000001)
	require.Equal(t, "unit-provider", repo.values[SettingFXRatesSource])
	require.Equal(t, fixed.Format(time.RFC3339), repo.values[SettingFXRatesUpdatedAt])

	var stored map[string]float64
	require.NoError(t, json.Unmarshal([]byte(repo.values[SettingManualFXRates]), &stored))
	require.InDelta(t, 0.00004, stored["VND"], 0.000000000001)
	require.InDelta(t, 0.14, stored["CNY"], 0.000000000001)
}

func TestPaymentFXSyncServiceSelectsConfiguredProductionProvider(t *testing.T) {
	repo := &paymentConfigSettingRepoStub{values: map[string]string{
		SettingLedgerCurrency:           "USD",
		SettingAllowedPaymentCurrencies: "USD,VND,CNY",
		SettingManualFXRates:            `{"USD":1,"VND":0.000039215686,"CNY":0.139}`,
		SettingFXAutoSyncProvider:       "open_exchange_rates",
	}}
	configSvc := NewPaymentConfigService(nil, repo, nil)
	fixed := time.Date(2026, 4, 29, 12, 0, 0, 0, time.UTC)
	syncSvc := NewPaymentFXSyncService(configSvc, repo, nil)
	syncSvc.SetProviders(map[string]PaymentFXRateProvider{
		PaymentFXProviderOpenExchangeRates: StaticPaymentFXRateProvider{
			Rates:     map[string]float64{"USD": 1, "VND": 0.00004, "CNY": 0.14},
			Source:    PaymentFXProviderOpenExchangeRates,
			Timestamp: fixed,
		},
	})

	result, err := syncSvc.SyncOnce(context.Background())
	require.NoError(t, err)
	require.Equal(t, PaymentFXProviderOpenExchangeRates, result.Source)
	require.Equal(t, fixed, result.UpdatedAt)
	require.InDelta(t, 0.00004, result.Rates["VND"], 0.000000000001)
	require.Equal(t, PaymentFXProviderOpenExchangeRates, repo.values[SettingFXRatesSource])
}

func TestOpenExchangeRatesProviderConvertsUSDBaseRatesToLedgerRates(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, "secret-app-id", r.URL.Query().Get("app_id"))
		require.Contains(t, r.URL.Query().Get("symbols"), "VND")
		require.Contains(t, r.URL.Query().Get("symbols"), "CNY")
		_, _ = w.Write([]byte(`{"base":"USD","timestamp":1777450752,"rates":{"USD":1,"VND":25500,"CNY":7.2,"KRW":1350}}`))
	}))
	defer server.Close()

	provider := NewOpenExchangeRatesProvider(OpenExchangeRatesProviderConfig{
		AppID:   "secret-app-id",
		BaseURL: server.URL,
	})

	snapshot, err := provider.FetchPaymentFXRates(context.Background(), PaymentFXRateRequest{
		LedgerCurrency:    "USD",
		PaymentCurrencies: []string{"USD", "VND", "CNY"},
	})
	require.NoError(t, err)
	require.Equal(t, PaymentFXProviderOpenExchangeRates, snapshot.Source)
	require.Equal(t, time.Unix(1777450752, 0).UTC(), snapshot.Timestamp)
	require.Equal(t, 1.0, snapshot.Rates["USD"])
	require.InDelta(t, 1.0/25500.0, snapshot.Rates["VND"], 0.000000000001)
	require.InDelta(t, 1.0/7.2, snapshot.Rates["CNY"], 0.000000000001)
}

func TestPaymentFXSyncRetriesWithBackoff(t *testing.T) {
	repo := &paymentConfigSettingRepoStub{values: map[string]string{
		SettingLedgerCurrency:           "USD",
		SettingAllowedPaymentCurrencies: "USD,VND",
		SettingManualFXRates:            `{"USD":1,"VND":0.000039215686}`,
	}}
	provider := &flakyPaymentFXProvider{
		failures: 2,
		snapshot: PaymentFXRateSnapshot{
			Rates:     map[string]float64{"USD": 1, "VND": 0.00004},
			Source:    "retry-provider",
			Timestamp: time.Date(2026, 4, 29, 13, 0, 0, 0, time.UTC),
		},
	}
	configSvc := NewPaymentConfigService(nil, repo, nil)
	syncSvc := NewPaymentFXSyncService(configSvc, repo, provider)
	syncSvc.SetRetryPolicy(PaymentFXSyncRetryPolicy{MaxAttempts: 3, InitialBackoff: time.Second, MaxBackoff: 3 * time.Second})
	var sleeps []time.Duration
	syncSvc.sleep = func(_ context.Context, d time.Duration) error {
		sleeps = append(sleeps, d)
		return nil
	}

	result, err := syncSvc.SyncOnceWithRetry(context.Background())
	require.NoError(t, err)
	require.Equal(t, "retry-provider", result.Source)
	require.Equal(t, 3, provider.calls)
	require.Equal(t, []time.Duration{time.Second, 2 * time.Second}, sleeps)
}

func TestPaymentFXSyncRetryExhaustionReturnsSanitizedError(t *testing.T) {
	repo := &paymentConfigSettingRepoStub{values: map[string]string{
		SettingLedgerCurrency:           "USD",
		SettingAllowedPaymentCurrencies: "USD,VND",
		SettingManualFXRates:            `{"USD":1,"VND":0.000039215686}`,
	}}
	configSvc := NewPaymentConfigService(nil, repo, nil)
	syncSvc := NewPaymentFXSyncService(configSvc, repo, alwaysFailPaymentFXProvider{})
	syncSvc.SetRetryPolicy(PaymentFXSyncRetryPolicy{MaxAttempts: 2, InitialBackoff: time.Second, MaxBackoff: time.Second})
	syncSvc.sleep = func(context.Context, time.Duration) error { return nil }

	_, err := syncSvc.SyncOnceWithRetry(context.Background())
	require.Error(t, err)
	require.Contains(t, err.Error(), "failed after 2 attempt")
	require.NotContains(t, err.Error(), "secret")
}

type flakyPaymentFXProvider struct {
	failures int
	calls    int
	snapshot PaymentFXRateSnapshot
}

func (p *flakyPaymentFXProvider) FetchPaymentFXRates(context.Context, PaymentFXRateRequest) (PaymentFXRateSnapshot, error) {
	p.calls++
	if p.calls <= p.failures {
		return PaymentFXRateSnapshot{}, errors.New("temporary provider error")
	}
	return p.snapshot, nil
}

type alwaysFailPaymentFXProvider struct{}

func (alwaysFailPaymentFXProvider) FetchPaymentFXRates(context.Context, PaymentFXRateRequest) (PaymentFXRateSnapshot, error) {
	return PaymentFXRateSnapshot{}, errors.New("provider rejected request")
}

func TestPaymentFXSyncServiceRejectsProviderMissingCurrency(t *testing.T) {
	repo := &paymentConfigSettingRepoStub{values: map[string]string{
		SettingLedgerCurrency:           "USD",
		SettingAllowedPaymentCurrencies: "USD,VND,CNY",
		SettingManualFXRates:            `{"USD":1,"VND":0.000039215686,"CNY":0.139}`,
	}}
	configSvc := NewPaymentConfigService(nil, repo, nil)
	syncSvc := NewPaymentFXSyncService(configSvc, repo, StaticPaymentFXRateProvider{
		Rates:  map[string]float64{"USD": 1, "VND": 0.00004},
		Source: "unit-provider",
	})

	_, err := syncSvc.SyncOnce(context.Background())
	require.Error(t, err)
	require.Contains(t, err.Error(), "CNY")
}

func TestUpdatePaymentConfigManualFXRatesStampsMetadata(t *testing.T) {
	repo := &paymentConfigSettingRepoStub{values: map[string]string{}}
	svc := &PaymentConfigService{settingRepo: repo}

	err := svc.UpdatePaymentConfig(context.Background(), UpdatePaymentConfigRequest{
		ManualFXRates: paymentConfigStrPtr(`{"USD":1,"VND":0.00004}`),
	})
	require.NoError(t, err)
	require.Equal(t, "manual", repo.values[SettingFXRatesSource])
	require.NotEmpty(t, repo.values[SettingFXRatesUpdatedAt])
}
