package service

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func newPaymentFXSyncTestService(t *testing.T, repo *paymentConfigSettingRepoStub) *PaymentFXSyncService {
	t.Helper()
	svc := NewPaymentFXSyncService(repo, &PaymentConfigService{settingRepo: repo}, nil, nil)
	svc.now = func() time.Time { return time.Date(2026, 11, 10, 12, 0, 0, 0, time.UTC) }
	return svc
}

func newFXRatesServer(t *testing.T, body string, status int) *httptest.Server {
	t.Helper()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(status)
		_, _ = w.Write([]byte(body))
	}))
	t.Cleanup(server.Close)
	return server
}

func TestPaymentFXSyncFetchRateFormats(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		body string
		want float64
	}{
		{
			name: "open er api with USD and base_code",
			body: `{"base_code":"USD","rates":{"USD":1,"CNY":7.1}}`,
			want: 7.1,
		},
		{
			name: "frankfurter base USD without USD entry",
			body: `{"base":"USD","rates":{"CNY":7.2}}`,
			want: 7.2,
		},
		{
			name: "conversion_rates fallback",
			body: `{"base_code":"USD","conversion_rates":{"USD":1,"CNY":7.3}}`,
			want: 7.3,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			server := newFXRatesServer(t, tt.body, http.StatusOK)
			repo := &paymentConfigSettingRepoStub{}
			svc := newPaymentFXSyncTestService(t, repo)
			svc.httpClient = server.Client()

			got, err := svc.fetchRateFromURL(context.Background(), server.URL)
			require.NoError(t, err)
			require.InDelta(t, tt.want, got, 1e-9)
		})
	}
}

func TestPaymentFXSyncFetchRateFallsBackToNextURL(t *testing.T) {
	t.Parallel()

	bad := newFXRatesServer(t, `{"rates":{"CNY":7.1}}`, http.StatusInternalServerError)
	good := newFXRatesServer(t, `{"base":"USD","rates":{"CNY":7.15}}`, http.StatusOK)
	repo := &paymentConfigSettingRepoStub{}
	svc := newPaymentFXSyncTestService(t, repo)
	svc.httpClient = good.Client()

	got, source, err := svc.fetchRate(context.Background(), []string{bad.URL, good.URL})
	require.NoError(t, err)
	require.InDelta(t, 7.15, got, 1e-9)
	require.Equal(t, good.URL, source)
}

func TestPaymentFXSyncRejectsInvalidRatesAndResponses(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		body string
		code int
	}{
		{"missing CNY", `{"base":"USD","rates":{"USD":1}}`, http.StatusOK},
		{"non USD base without USD", `{"base":"EUR","rates":{"CNY":7.1}}`, http.StatusOK},
		{"rate out of range", `{"base":"USD","rates":{"CNY":100}}`, http.StatusOK},
		{"http error", `{"base":"USD","rates":{"CNY":7.1}}`, http.StatusBadGateway},
		{"malformed JSON", `not json`, http.StatusOK},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			server := newFXRatesServer(t, tt.body, tt.code)
			repo := &paymentConfigSettingRepoStub{}
			svc := newPaymentFXSyncTestService(t, repo)
			svc.httpClient = server.Client()

			rate, _, err := svc.fetchRate(context.Background(), []string{server.URL})
			if rate >= paymentFXSyncMinRate && rate <= paymentFXSyncMaxRate {
				t.Fatalf("expected invalid rate, got %v", rate)
			}
			require.Error(t, err)
		})
	}
}

func TestPaymentFXSyncUpdatesMultiplierAndStatus(t *testing.T) {
	t.Parallel()

	server := newFXRatesServer(t, `{"base":"USD","rates":{"CNY":6.686583}}`, http.StatusOK)
	repo := &paymentConfigSettingRepoStub{}
	svc := newPaymentFXSyncTestService(t, repo)
	svc.httpClient = server.Client()

	cfg := &PaymentFXSyncConfig{
		Enabled:                true,
		URLs:                   []string{server.URL},
		IntervalHours:          24,
		UpdateSubscriptionRate: true,
	}
	require.NoError(t, svc.syncOnce(context.Background(), cfg))

	require.Equal(t, "0.149553", repo.values[SettingBalanceRechargeMult])
	require.Equal(t, "6.686583", repo.values[SettingSubscriptionUSDToCNYRate])
	require.Equal(t, "6.686583", repo.values[SettingPaymentFXSyncLastRate])
	require.Equal(t, "0.149553", repo.values[SettingPaymentFXSyncLastMultiplier])
	require.Empty(t, repo.values[SettingPaymentFXSyncLastError])
	require.Equal(t, server.URL, repo.values[SettingPaymentFXSyncLastSource])
	require.NotEmpty(t, repo.values[SettingPaymentFXSyncLastSuccessAt])
}

func TestPaymentFXSyncAppliesMarkupAndOptionalSubscriptionRate(t *testing.T) {
	t.Parallel()

	server := newFXRatesServer(t, `{"base":"USD","rates":{"CNY":7}}`, http.StatusOK)
	repo := &paymentConfigSettingRepoStub{}
	svc := newPaymentFXSyncTestService(t, repo)
	svc.httpClient = server.Client()

	cfg := &PaymentFXSyncConfig{
		Enabled:                true,
		URLs:                   []string{server.URL},
		IntervalHours:          24,
		MarkupPercent:          2,
		UpdateSubscriptionRate: true,
	}
	require.NoError(t, svc.syncOnce(context.Background(), cfg))

	require.Equal(t, "0.140056", repo.values[SettingBalanceRechargeMult])
	require.Equal(t, "7.14", repo.values[SettingSubscriptionUSDToCNYRate])
	require.Equal(t, "7.14", repo.values[SettingPaymentFXSyncLastSubscriptionRate])
}

func TestPaymentFXSyncRecordsFailureWithoutChangingMultiplier(t *testing.T) {
	t.Parallel()

	server := newFXRatesServer(t, `{"base":"USD","rates":{"CNY":7.1}}`, http.StatusInternalServerError)
	repo := &paymentConfigSettingRepoStub{values: map[string]string{
		SettingBalanceRechargeMult: "0.15",
	}}
	svc := newPaymentFXSyncTestService(t, repo)
	svc.httpClient = server.Client()

	cfg := &PaymentFXSyncConfig{
		Enabled:       true,
		URLs:          []string{server.URL},
		IntervalHours: 24,
	}
	require.Error(t, svc.syncOnce(context.Background(), cfg))

	require.Equal(t, "0.15", repo.values[SettingBalanceRechargeMult])
	require.NotEmpty(t, repo.values[SettingPaymentFXSyncLastError])
	require.NotEmpty(t, repo.values[SettingPaymentFXSyncLastAttemptAt])
	require.Empty(t, repo.values[SettingPaymentFXSyncLastSuccessAt])
}

func TestPaymentFXSyncDueLogic(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, 11, 10, 12, 0, 0, 0, time.UTC)
	cfg := &PaymentFXSyncConfig{Enabled: true, IntervalHours: 24}
	require.True(t, paymentFXSyncDue(cfg, now))

	cfg.LastSuccessAt = now.Add(-23 * time.Hour).Format(time.RFC3339)
	require.False(t, paymentFXSyncDue(cfg, now))

	cfg.LastSuccessAt = now.Add(-25 * time.Hour).Format(time.RFC3339)
	require.True(t, paymentFXSyncDue(cfg, now))

	cfg.LastError = "boom"
	cfg.LastAttemptAt = now.Add(-30 * time.Minute).Format(time.RFC3339)
	require.False(t, paymentFXSyncDue(cfg, now))

	cfg.LastAttemptAt = now.Add(-2 * time.Hour).Format(time.RFC3339)
	require.True(t, paymentFXSyncDue(cfg, now))

	cfg.Enabled = false
	require.False(t, paymentFXSyncDue(cfg, now))
}

func TestPaymentFXSyncUpdateConfigValidation(t *testing.T) {
	t.Parallel()

	repo := &paymentConfigSettingRepoStub{}
	svc := newPaymentFXSyncTestService(t, repo)

	_, err := svc.UpdateConfig(context.Background(), UpdatePaymentFXSyncRequest{
		Enabled:       true,
		IntervalHours: 24,
	})
	require.Error(t, err)

	_, err = svc.UpdateConfig(context.Background(), UpdatePaymentFXSyncRequest{
		Enabled:       true,
		URLs:          []string{"https://example.com/rates"},
		IntervalHours: 0,
	})
	require.Error(t, err)

	_, err = svc.UpdateConfig(context.Background(), UpdatePaymentFXSyncRequest{
		Enabled:       true,
		URLs:          []string{"https://example.com/rates"},
		IntervalHours: 24,
		MarkupPercent: 101,
	})
	require.Error(t, err)

	_, err = svc.UpdateConfig(context.Background(), UpdatePaymentFXSyncRequest{
		Enabled:       true,
		URLs:          []string{"ftp://example.com/rates"},
		IntervalHours: 24,
	})
	require.Error(t, err)

	cfg, err := svc.UpdateConfig(context.Background(), UpdatePaymentFXSyncRequest{
		Enabled:       true,
		URLs:          []string{"https://example.com/rates"},
		IntervalHours: 24,
		MarkupPercent: 1.25,
	})
	require.NoError(t, err)
	require.True(t, cfg.Enabled)
	require.Equal(t, []string{"https://example.com/rates"}, cfg.URLs)
	require.Equal(t, 24, cfg.IntervalHours)
	require.InDelta(t, 1.25, cfg.MarkupPercent, 1e-9)
	require.Equal(t, "true", repo.values[SettingPaymentFXSyncEnabled])
	require.Equal(t, "https://example.com/rates", repo.values[SettingPaymentFXSyncURLs])
}

func TestPaymentFXSyncSyncNowReturnsConflictWhenLockIsHeld(t *testing.T) {
	t.Parallel()

	repo := &paymentConfigSettingRepoStub{values: map[string]string{
		SettingPaymentFXSyncEnabled: "true",
		SettingPaymentFXSyncURLs:    "https://example.com/rates",
	}}
	svc := NewPaymentFXSyncService(repo, &PaymentConfigService{settingRepo: repo}, &fakeFXLeaderLock{acquired: false}, nil)
	_, err := svc.SyncNow(context.Background())
	require.Error(t, err)
	require.ErrorIs(t, err, ErrPaymentFXSyncInProgress)
}

type fakeFXLeaderLock struct {
	acquired bool
}

func (f *fakeFXLeaderLock) TryAcquireLeaderLock(context.Context, string, string, time.Duration) (bool, error) {
	return f.acquired, nil
}

func (f *fakeFXLeaderLock) ReleaseLeaderLock(context.Context, string, string) error { return nil }

// Ensure the JSON shape remains stable for the admin UI.
func TestPaymentFXSyncConfigJSONShape(t *testing.T) {
	t.Parallel()

	cfg := PaymentFXSyncConfig{Enabled: true, URLs: []string{"https://example.com"}, IntervalHours: 24}
	data, err := json.Marshal(cfg)
	require.NoError(t, err)
	require.Contains(t, string(data), `"enabled":true`)
	require.Contains(t, string(data), `"interval_hours":24`)
	require.NotContains(t, string(data), `"last_error"`)
}
