//go:build integration

package repository

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/stretchr/testify/require"
)

func TestAccountUpdatePreservesConcurrentProbeSnapshot(t *testing.T) {
	ctx := context.Background()
	tx := testEntTx(t)
	repo := newAccountRepositoryWithSQL(tx.Client(), tx, nil)
	account := mustCreateAccount(t, tx.Client(), &service.Account{
		Name:        "probe-update-preserve",
		Platform:    service.PlatformOpenAI,
		Type:        service.AccountTypeAPIKey,
		Credentials: map[string]any{"api_key": "sk-old"},
		Extra:       map[string]any{service.UpstreamBillingProbeEnabledExtraKey: true},
	})

	stale, err := repo.GetByID(ctx, account.ID)
	require.NoError(t, err)
	require.NotContains(t, stale.Extra, service.UpstreamBillingProbeExtraKey)
	require.NoError(t, repo.UpdateUpstreamBillingProbeSnapshot(ctx, stale, &service.UpstreamBillingProbeSnapshot{
		Status:        service.UpstreamBillingProbeStatusOK,
		LastAttemptAt: time.Now().UTC(),
	}))

	stale.Name = "ordinary-edit"
	require.NoError(t, repo.Update(ctx, stale))
	got, err := repo.GetByID(ctx, account.ID)
	require.NoError(t, err)
	snapshot, ok := got.Extra[service.UpstreamBillingProbeExtraKey].(map[string]any)
	require.True(t, ok)
	require.Equal(t, service.UpstreamBillingProbeStatusOK, snapshot["status"])

	require.NoError(t, repo.UpdateExtra(ctx, got.ID, map[string]any{service.UpstreamBillingProbeEnabledExtraKey: false}))
	disabled, err := repo.GetByID(ctx, account.ID)
	require.NoError(t, err)
	require.NotContains(t, disabled.Extra, service.UpstreamBillingProbeExtraKey)
}

func TestAccountUpdatePreservesConcurrentProbeEnableFlag(t *testing.T) {
	ctx := context.Background()
	tx := testEntTx(t)
	repo := newAccountRepositoryWithSQL(tx.Client(), tx, nil)
	account := mustCreateAccount(t, tx.Client(), &service.Account{
		Name:        "probe-update-enable",
		Platform:    service.PlatformOpenAI,
		Type:        service.AccountTypeAPIKey,
		Credentials: map[string]any{"api_key": "sk-test"},
		Extra: map[string]any{
			service.UpstreamBillingProbeEnabledExtraKey: true,
			service.UpstreamBillingProbeExtraKey:        map[string]any{"status": service.UpstreamBillingProbeStatusOK},
		},
	})

	stale, err := repo.GetByID(ctx, account.ID)
	require.NoError(t, err)
	require.NoError(t, repo.UpdateExtra(ctx, account.ID, map[string]any{service.UpstreamBillingProbeEnabledExtraKey: false}))
	stale.Name = "ordinary-edit"
	require.NoError(t, repo.Update(ctx, stale))

	got, err := repo.GetByID(ctx, account.ID)
	require.NoError(t, err)
	require.Equal(t, false, got.Extra[service.UpstreamBillingProbeEnabledExtraKey])
	require.NotContains(t, got.Extra, service.UpstreamBillingProbeExtraKey)
}

func TestAccountUpdateClearsProbeSnapshotWhenIdentityChanges(t *testing.T) {
	ctx := context.Background()
	tx := testEntTx(t)
	repo := newAccountRepositoryWithSQL(tx.Client(), tx, nil)
	account := mustCreateAccount(t, tx.Client(), &service.Account{
		Name:        "probe-update-identity",
		Platform:    service.PlatformOpenAI,
		Type:        service.AccountTypeAPIKey,
		Credentials: map[string]any{"api_key": "sk-old"},
		Extra: map[string]any{
			service.UpstreamBillingProbeEnabledExtraKey: true,
			service.UpstreamBillingProbeExtraKey:        map[string]any{"status": service.UpstreamBillingProbeStatusOK},
		},
	})

	loaded, err := repo.GetByID(ctx, account.ID)
	require.NoError(t, err)
	loaded.Credentials["api_key"] = "sk-new"
	require.NoError(t, repo.Update(ctx, loaded))

	got, err := repo.GetByID(ctx, account.ID)
	require.NoError(t, err)
	require.NotContains(t, got.Extra, service.UpstreamBillingProbeExtraKey)
}

func TestBulkUpdateAndCredentialUpdateDeleteProbeKey(t *testing.T) {
	ctx := context.Background()
	tx := testEntTx(t)
	repo := newAccountRepositoryWithSQL(tx.Client(), tx, nil)
	newAccount := func(name string) *service.Account {
		return mustCreateAccount(t, tx.Client(), &service.Account{
			Name:        name,
			Platform:    service.PlatformOpenAI,
			Type:        service.AccountTypeAPIKey,
			Credentials: map[string]any{"api_key": "sk-old"},
			Extra: map[string]any{
				service.UpstreamBillingProbeEnabledExtraKey: true,
				service.UpstreamBillingProbeExtraKey:        map[string]any{"status": service.UpstreamBillingProbeStatusOK},
			},
		})
	}

	bulkAccount := newAccount("probe-bulk-clear")
	_, err := repo.BulkUpdate(ctx, []int64{bulkAccount.ID}, service.AccountBulkUpdate{
		Extra: map[string]any{service.UpstreamBillingProbeExtraKey: nil},
	})
	require.NoError(t, err)
	got, err := repo.GetByID(ctx, bulkAccount.ID)
	require.NoError(t, err)
	require.NotContains(t, got.Extra, service.UpstreamBillingProbeExtraKey)

	credentialAccount := newAccount("probe-credentials-clear")
	require.NoError(t, repo.UpdateCredentials(ctx, credentialAccount.ID, map[string]any{"api_key": "sk-new"}))
	got, err = repo.GetByID(ctx, credentialAccount.ID)
	require.NoError(t, err)
	require.NotContains(t, got.Extra, service.UpstreamBillingProbeExtraKey)
}

func TestProbeSnapshotCASIncludesLoadedEnabledState(t *testing.T) {
	tests := []struct {
		name           string
		loadedEnabled  bool
		concurrentFlip *bool
		wantConflict   bool
	}{
		{name: "manual_false_stays_false", loadedEnabled: false},
		{name: "periodic_true_disabled_in_flight", loadedEnabled: true, concurrentFlip: boolPtr(false), wantConflict: true},
		{name: "manual_false_enabled_in_flight", loadedEnabled: false, concurrentFlip: boolPtr(true), wantConflict: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := context.Background()
			tx := testEntTx(t)
			repo := newAccountRepositoryWithSQL(tx.Client(), tx, nil)
			account := mustCreateAccount(t, tx.Client(), &service.Account{
				Name:        "probe-enabled-cas-" + tt.name,
				Platform:    service.PlatformOpenAI,
				Type:        service.AccountTypeAPIKey,
				Credentials: map[string]any{"api_key": "sk-test"},
				Extra:       map[string]any{service.UpstreamBillingProbeEnabledExtraKey: tt.loadedEnabled},
			})
			inFlight, err := repo.GetByID(ctx, account.ID)
			require.NoError(t, err)
			if tt.concurrentFlip != nil {
				require.NoError(t, repo.UpdateExtra(ctx, account.ID, map[string]any{
					service.UpstreamBillingProbeEnabledExtraKey: *tt.concurrentFlip,
				}))
			}

			err = repo.UpdateUpstreamBillingProbeSnapshot(ctx, inFlight, &service.UpstreamBillingProbeSnapshot{
				Status:        service.UpstreamBillingProbeStatusOK,
				LastAttemptAt: time.Now().UTC(),
			})
			if tt.wantConflict {
				require.ErrorIs(t, err, service.ErrUpstreamBillingProbeIdentityChanged)
			} else {
				require.NoError(t, err)
			}
			got, err := repo.GetByID(ctx, account.ID)
			require.NoError(t, err)
			if tt.wantConflict {
				require.NotContains(t, got.Extra, service.UpstreamBillingProbeExtraKey)
			} else {
				require.Contains(t, got.Extra, service.UpstreamBillingProbeExtraKey)
			}
		})
	}
}

func boolPtr(value bool) *bool {
	return &value
}

func TestProxyIdentityUpdateInvalidatesProbeAndRejectsInFlightSnapshot(t *testing.T) {
	tests := []struct {
		name             string
		includeProbeKey  bool
		probeValue       any
		wantInvalidation bool
	}{
		{name: "missing_snapshot"},
		{name: "json_null_snapshot", includeProbeKey: true},
		{name: "existing_snapshot", includeProbeKey: true, probeValue: map[string]any{"status": service.UpstreamBillingProbeStatusOK}, wantInvalidation: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := context.Background()
			tx := testEntTx(t)
			accountRepo := newAccountRepositoryWithSQL(tx.Client(), tx, nil)
			proxyRepo := newProxyRepositoryWithSQL(tx.Client(), tx)
			proxy := mustCreateProxy(t, tx.Client(), &service.Proxy{
				Name:     "probe-proxy",
				Protocol: "http",
				Host:     "old.example",
				Port:     8080,
				Username: "old-user",
				Password: "old-pass",
				Status:   service.StatusActive,
			})
			extra := map[string]any{service.UpstreamBillingProbeEnabledExtraKey: true}
			if tt.includeProbeKey {
				extra[service.UpstreamBillingProbeExtraKey] = tt.probeValue
			}
			account := mustCreateAccount(t, tx.Client(), &service.Account{
				Name:        "proxy-probe-account",
				Platform:    service.PlatformOpenAI,
				Type:        service.AccountTypeAPIKey,
				Credentials: map[string]any{"api_key": "sk-test"},
				Extra:       extra,
				ProxyID:     &proxy.ID,
			})
			inFlight, err := accountRepo.GetByID(ctx, account.ID)
			require.NoError(t, err)
			require.NotNil(t, inFlight.Proxy)
			require.Equal(t, "old.example", inFlight.Proxy.Host)

			proxyToUpdate, err := proxyRepo.GetByID(ctx, proxy.ID)
			require.NoError(t, err)
			proxyToUpdate.Host = "new.example"
			require.NoError(t, proxyRepo.Update(ctx, proxyToUpdate))

			got, err := accountRepo.GetByID(ctx, account.ID)
			require.NoError(t, err)
			if tt.wantInvalidation || !tt.includeProbeKey {
				require.NotContains(t, got.Extra, service.UpstreamBillingProbeExtraKey)
			} else {
				require.Contains(t, got.Extra, service.UpstreamBillingProbeExtraKey)
				require.Nil(t, got.Extra[service.UpstreamBillingProbeExtraKey])
			}
			if !tt.wantInvalidation {
				require.Equal(t, inFlight.UpdatedAt, got.UpdatedAt, "missing/null snapshots must not cause an account row write")
			}
			err = accountRepo.UpdateUpstreamBillingProbeSnapshot(ctx, inFlight, &service.UpstreamBillingProbeSnapshot{
				Status:        service.UpstreamBillingProbeStatusOK,
				LastAttemptAt: time.Now().UTC(),
			})
			require.ErrorIs(t, err, service.ErrUpstreamBillingProbeIdentityChanged)

			rows, err := tx.QueryContext(ctx, `
				SELECT COUNT(*), COALESCE(MAX(payload::text), '')
				FROM scheduler_outbox
				WHERE event_type = $1
			`, service.SchedulerOutboxEventAccountBulkChanged)
			require.NoError(t, err)
			require.True(t, rows.Next())
			var (
				outboxCount int
				payloadJSON string
			)
			require.NoError(t, rows.Scan(&outboxCount, &payloadJSON))
			require.NoError(t, rows.Close())
			if tt.wantInvalidation {
				require.Equal(t, 1, outboxCount)
				var payload struct {
					AccountIDs []int64 `json:"account_ids"`
				}
				require.NoError(t, json.Unmarshal([]byte(payloadJSON), &payload))
				require.Equal(t, []int64{account.ID}, payload.AccountIDs)
			} else {
				require.Zero(t, outboxCount, "no snapshot change means no PR2 cache invalidation event")
			}
		})
	}
}

func TestSweepExpiredProxyWithoutFallbackInvalidatesOnlyExistingProbeSnapshot(t *testing.T) {
	ctx := context.Background()
	tx := testEntTx(t)
	proxyRepo := newProxyRepositoryWithSQL(tx.Client(), tx)
	accountRepo := newAccountRepositoryWithSQL(tx.Client(), tx, nil)
	past := time.Now().Add(-time.Hour)
	proxy := &service.Proxy{
		Name:           "expired-probe-proxy-none",
		Protocol:       "http",
		Host:           "127.0.0.1",
		Port:           8080,
		Status:         service.StatusActive,
		ExpiresAt:      &past,
		FallbackMode:   service.FallbackModeNone,
		ExpiryWarnDays: 7,
	}
	require.NoError(t, proxyRepo.Create(ctx, proxy))
	newAccount := func(name string, probe any, includeProbe bool) *service.Account {
		extra := map[string]any{service.UpstreamBillingProbeEnabledExtraKey: true}
		if includeProbe {
			extra[service.UpstreamBillingProbeExtraKey] = probe
		}
		return mustCreateAccount(t, tx.Client(), &service.Account{
			Name:        name,
			Platform:    service.PlatformOpenAI,
			Type:        service.AccountTypeAPIKey,
			Credentials: map[string]any{"api_key": "sk-test"},
			Extra:       extra,
			ProxyID:     &proxy.ID,
		})
	}
	withSnapshot := newAccount("expired-proxy-with-snapshot", map[string]any{"status": service.UpstreamBillingProbeStatusOK}, true)
	withoutSnapshot := newAccount("expired-proxy-without-snapshot", nil, false)
	withJSONNull := newAccount("expired-proxy-null-snapshot", nil, true)
	untouchedUpdatedAt := make(map[int64]time.Time, 2)
	for _, untouched := range []*service.Account{withoutSnapshot, withJSONNull} {
		loaded, err := accountRepo.GetByID(ctx, untouched.ID)
		require.NoError(t, err)
		untouchedUpdatedAt[untouched.ID] = loaded.UpdatedAt
	}

	changed, err := proxyRepo.SweepExpiredProxies(ctx, time.Now())
	require.NoError(t, err)
	require.Zero(t, changed, "probe invalidation must not inflate the rerouted account count")

	got, err := accountRepo.GetByID(ctx, withSnapshot.ID)
	require.NoError(t, err)
	require.NotContains(t, got.Extra, service.UpstreamBillingProbeExtraKey)
	for _, untouched := range []*service.Account{withoutSnapshot, withJSONNull} {
		got, err = accountRepo.GetByID(ctx, untouched.ID)
		require.NoError(t, err)
		require.Equal(t, untouchedUpdatedAt[untouched.ID], got.UpdatedAt)
	}

	payload := latestBulkAccountOutboxPayload(t, ctx, tx)
	require.Equal(t, []int64{withSnapshot.ID}, payload)
}

func TestSweepExpiredProxyFallbackRerouteDeletesProbeSnapshot(t *testing.T) {
	ctx := context.Background()
	tx := testEntTx(t)
	proxyRepo := newProxyRepositoryWithSQL(tx.Client(), tx)
	accountRepo := newAccountRepositoryWithSQL(tx.Client(), tx, nil)
	past := time.Now().Add(-time.Hour)
	proxy := &service.Proxy{
		Name:           "expired-probe-proxy-direct",
		Protocol:       "http",
		Host:           "127.0.0.1",
		Port:           8080,
		Status:         service.StatusActive,
		ExpiresAt:      &past,
		FallbackMode:   service.FallbackModeDirect,
		ExpiryWarnDays: 7,
	}
	require.NoError(t, proxyRepo.Create(ctx, proxy))
	account := mustCreateAccount(t, tx.Client(), &service.Account{
		Name:        "expired-proxy-rerouted-snapshot",
		Platform:    service.PlatformOpenAI,
		Type:        service.AccountTypeAPIKey,
		Credentials: map[string]any{"api_key": "sk-test"},
		Extra: map[string]any{
			service.UpstreamBillingProbeEnabledExtraKey: true,
			service.UpstreamBillingProbeExtraKey:        map[string]any{"status": service.UpstreamBillingProbeStatusOK},
		},
		ProxyID: &proxy.ID,
	})

	changed, err := proxyRepo.SweepExpiredProxies(ctx, time.Now())
	require.NoError(t, err)
	require.EqualValues(t, 1, changed)

	got, err := accountRepo.GetByID(ctx, account.ID)
	require.NoError(t, err)
	require.Nil(t, got.ProxyID)
	require.NotContains(t, got.Extra, service.UpstreamBillingProbeExtraKey)
	require.Equal(t, []int64{account.ID}, latestBulkAccountOutboxPayload(t, ctx, tx))
}

func latestBulkAccountOutboxPayload(t *testing.T, ctx context.Context, tx sqlQueryer) []int64 {
	t.Helper()
	var payloadJSON []byte
	require.NoError(t, scanSingleRow(ctx, tx, `
		SELECT payload
		FROM scheduler_outbox
		WHERE event_type = $1
		ORDER BY id DESC
		LIMIT 1
	`, []any{service.SchedulerOutboxEventAccountBulkChanged}, &payloadJSON))
	var payload struct {
		AccountIDs []int64 `json:"account_ids"`
	}
	require.NoError(t, json.Unmarshal(payloadJSON, &payload))
	return payload.AccountIDs
}
