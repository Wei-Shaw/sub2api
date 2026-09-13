//go:build integration

package repository

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
)

func windowIntegrationRepo(t *testing.T) (*accountWindowUsageRepository, *service.Account) {
	t.Helper()
	client := testEntClient(t)
	account := mustCreateAccount(t, client, &service.Account{Name: "window-" + uuid.NewString(), Platform: service.PlatformOpenAI, Type: service.AccountTypeOAuth})
	t.Cleanup(func() {
		// Hard delete after per-log cleanup; journal/history foreign keys cascade.
		_, err := integrationDB.Exec("DELETE FROM accounts WHERE id=$1", account.ID)
		require.NoError(t, err)
	})
	return &accountWindowUsageRepository{client: client, db: integrationDB}, account
}
func insertWindowObservation(t *testing.T, accountID int64, at, reset time.Time, used float64) int64 {
	t.Helper()
	payload := fmt.Sprintf(`{"codex_5h_used_percent":%g,"codex_5h_reset_at":%q,"codex_5h_window_minutes":300}`, used, reset.Format(time.RFC3339Nano))
	var id int64
	require.NoError(t, integrationDB.QueryRow(`INSERT INTO account_quota_observations(account_id,observed_at,payload) VALUES($1,$2,$3::jsonb) RETURNING id`, accountID, at, payload).Scan(&id))
	return id
}
func windowProcessedCount(t *testing.T, accountID int64) int {
	t.Helper()
	var n int
	require.NoError(t, integrationDB.QueryRow(`SELECT COUNT(*) FROM account_quota_observations WHERE account_id=$1 AND processed_at IS NOT NULL`, accountID).Scan(&n))
	return n
}

func TestQuotaHistoryJournalRollbackRetriesAtomically(t *testing.T) {
	r, a := windowIntegrationRepo(t)
	ctx := context.Background()
	at := time.Now().UTC().Add(-time.Hour).Truncate(time.Microsecond)
	id := insertWindowObservation(t, a.ID, at, at.Add(5*time.Hour), 10)
	failed := errors.New("transient aggregation failure")
	ok, err := r.ConsumeNextObservation(ctx, time.Now(), func(ctx context.Context, obs *service.AccountQuotaObservation) error {
		require.Equal(t, id, obs.ID)
		row := &service.AccountWindowUsageRecord{AccountID: a.ID, WindowType: "5h", ResetAt: at.Add(5 * time.Hour), DurationMinutes: 300, LastObservationID: obs.ID, AccountWindowUsageEntry: service.AccountWindowUsageEntry{WindowStart: at, WindowEnd: at.Add(5 * time.Hour), FirstObservedAt: at, QualityFlags: []string{}}}
		require.NoError(t, r.SaveWindow(ctx, row))
		return failed
	})
	require.ErrorIs(t, err, failed)
	require.False(t, ok)
	require.Zero(t, windowProcessedCount(t, a.ID))
	row, err := r.GetOpenWindow(ctx, a.ID, "5h")
	require.NoError(t, err)
	require.Nil(t, row)
	g := service.NewAccountWindowUsageIngester(r)
	ok, err = r.ConsumeNextObservation(ctx, time.Now(), g.ApplyObservation)
	require.NoError(t, err)
	require.True(t, ok)
	require.Equal(t, 1, windowProcessedCount(t, a.ID))
	row, err = r.GetOpenWindow(ctx, a.ID, "5h")
	require.NoError(t, err)
	require.Equal(t, 1, row.SampleCount)
}
func TestQuotaHistoryJournalSameTimestampIsOrderedAcrossReplicas(t *testing.T) {
	r, a := windowIntegrationRepo(t)
	at := time.Now().UTC().Add(-time.Hour).Truncate(time.Second)
	reset := at.Add(5 * time.Hour)
	const count = 505
	for i := 0; i < count; i++ {
		insertWindowObservation(t, a.ID, at, reset, float64(i)/10)
	}
	g := service.NewAccountWindowUsageIngester(r)
	var wg sync.WaitGroup
	errs := make(chan error, 4)
	for n := 0; n < 4; n++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := 0; i < count; i++ {
				ok, err := r.ConsumeNextObservation(context.Background(), time.Now(), g.ApplyObservation)
				if err != nil {
					errs <- err
					return
				}
				if !ok {
					return
				}
			}
		}()
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		require.NoError(t, err)
	}
	// A replica may find the account locked and finish; drain any remainder.
	for {
		ok, err := r.ConsumeNextObservation(context.Background(), time.Now(), g.ApplyObservation)
		require.NoError(t, err)
		if !ok {
			break
		}
	}
	require.Equal(t, count, windowProcessedCount(t, a.ID))
	row, err := r.GetOpenWindow(context.Background(), a.ID, "5h")
	require.NoError(t, err)
	require.Equal(t, count, row.SampleCount)
	require.InDelta(t, 50.4, row.LastUsedPercent, 1e-9)
}
func TestQuotaHistoryWindowRolloverAndResetMarker(t *testing.T) {
	r, a := windowIntegrationRepo(t)
	ctx := context.Background()
	at := time.Now().UTC().Add(-7 * time.Hour).Truncate(time.Microsecond)
	reset := at.Add(5 * time.Hour)
	g := service.NewAccountWindowUsageIngester(r)
	insertWindowObservation(t, a.ID, at, reset, 10)
	insertWindowObservation(t, a.ID, reset.Add(-time.Second), reset, 100)
	insertWindowObservation(t, a.ID, reset.Add(time.Second), reset.Add(5*time.Hour), 0)
	for {
		ok, err := r.ConsumeNextObservation(ctx, time.Now(), g.ApplyObservation)
		require.NoError(t, err)
		if !ok {
			break
		}
	}
	rows, err := r.ListHistorySince(ctx, a.ID, at.Add(-time.Hour))
	require.NoError(t, err)
	require.Len(t, rows, 2)
	require.Equal(t, 100.0, rows[0].LastUsedPercent)
	require.NotNil(t, rows[0].FinalizedAt)
	require.Equal(t, "expired", *rows[0].EndReason)
	marker := reset.Add(time.Minute)
	_, err = integrationDB.Exec(`INSERT INTO account_quota_observations(account_id,observed_at,payload) VALUES($1,$2,$3::jsonb)`, a.ID, marker, fmt.Sprintf(`{"codex_history_reset_at":%q,"codex_history_reset_windows":1}`, marker.Format(time.RFC3339Nano)))
	require.NoError(t, err)
	insertWindowObservation(t, a.ID, marker.Add(time.Second), marker.Add(5*time.Hour), 0)
	for {
		ok, err := r.ConsumeNextObservation(ctx, time.Now(), g.ApplyObservation)
		require.NoError(t, err)
		if !ok {
			break
		}
	}
	rows, err = r.ListHistorySince(ctx, a.ID, at.Add(-time.Hour))
	require.NoError(t, err)
	require.Len(t, rows, 3)
	require.Equal(t, "early_reset", *rows[1].EndReason)
	require.True(t, rows[1].WindowEnd.Equal(marker))
	require.True(t, rows[2].WindowStart.Equal(marker))
}
func seedReferenceWindowLog(t *testing.T, accountID int64, createdAt time.Time, pricingAt *time.Time, cost *float64) {
	t.Helper()
	client := testEntClient(t)
	user := mustCreateUser(t, client, &service.User{Email: "win-" + uuid.NewString() + "@example.com"})
	key := mustCreateApiKey(t, client, &service.APIKey{UserID: user.ID, Key: "sk-" + uuid.NewString(), Name: "window"})
	requestID := uuid.NewString()
	t.Cleanup(func() {
		_, err := integrationDB.Exec("DELETE FROM usage_logs WHERE request_id=$1", requestID)
		require.NoError(t, err)
		_, err = integrationDB.Exec("DELETE FROM api_keys WHERE id=$1", key.ID)
		require.NoError(t, err)
		_, err = integrationDB.Exec("DELETE FROM users WHERE id=$1", user.ID)
		require.NoError(t, err)
	})
	_, err := newUsageLogRepositoryWithSQL(client, integrationDB).Create(context.Background(), &service.UsageLog{UserID: user.ID, APIKeyID: key.ID, AccountID: accountID, RequestID: requestID, Model: "gpt-5", InputTokens: 10, OutputTokens: 20, CreatedAt: createdAt})
	require.NoError(t, err)
	var pricing any
	if pricingAt != nil {
		raw, err := json.Marshal(map[string]any{"pricing_at": pricingAt.UTC()})
		require.NoError(t, err)
		pricing = string(raw)
	}
	_, err = integrationDB.Exec(`UPDATE usage_logs SET api_reference_cost=$1,api_reference_pricing=$2::jsonb WHERE request_id=$3`, cost, pricing, requestID)
	require.NoError(t, err)
}
func TestQuotaHistoryReferenceAggregationUsesImmutableHalfOpenPricingTime(t *testing.T) {
	r, a := windowIntegrationRepo(t)
	start := time.Now().UTC().Add(-24 * time.Hour).Truncate(time.Second)
	end := start.Add(time.Hour)
	before := start.Add(-time.Second)
	atEnd := end
	one, two := 1.0, 2.0
	// created_at deliberately differs: the stored pricing timestamp owns the range.
	seedReferenceWindowLog(t, a.ID, start.Add(2*time.Hour), &start, &one)
	seedReferenceWindowLog(t, a.ID, start, &before, &two)
	seedReferenceWindowLog(t, a.ID, start, &atEnd, &two)
	seedReferenceWindowLog(t, a.ID, start.Add(time.Minute), nil, nil)
	stats, err := r.AggregateReferenceUsage(context.Background(), a.ID, start, end)
	require.NoError(t, err)
	require.Equal(t, int64(2), stats.Requests)
	require.Equal(t, int64(1), stats.PricedRequests)
	require.Equal(t, int64(1), stats.MissingPricingRequests)
	require.NotNil(t, stats.ReferenceCost)
	require.InDelta(t, 1, *stats.ReferenceCost, 1e-10)
}

func TestQuotaHistoryReconcileDoesNotWaitForFutureWindowTraffic(t *testing.T) {
	r, a := windowIntegrationRepo(t)
	ctx := context.Background()
	at := time.Now().UTC().Add(-7 * time.Hour).Truncate(time.Microsecond)
	end := at.Add(5 * time.Hour)
	insertWindowObservation(t, a.ID, at, end, 10)
	g := service.NewAccountWindowUsageIngester(r)
	ok, err := r.ConsumeNextObservation(ctx, time.Now(), g.ApplyObservation)
	require.NoError(t, err)
	require.True(t, ok)
	// A new window has pending traffic, but its observations are past the old
	// window's grace interval and must not prevent the old row from reconciling.
	insertWindowObservation(t, a.ID, time.Now(), time.Now().Add(5*time.Hour), 5)
	now := time.Now()
	ok, err = r.ReconcileNextWindow(ctx, now.Add(-5*time.Minute), func(ctx context.Context, row *service.AccountWindowUsageRecord) error {
		require.True(t, row.WindowEnd.Equal(end))
		row.FinalizedAt = &now
		row.StatsFinalizedAt = &now
		reason := "expired"
		row.EndReason = &reason
		return r.SaveWindow(ctx, row)
	})
	require.NoError(t, err)
	require.True(t, ok)
	rows, err := r.ListHistorySince(ctx, a.ID, at)
	require.NoError(t, err)
	require.Len(t, rows, 1)
	require.NotNil(t, rows[0].StatsFinalizedAt)
}
func TestQuotaHistoryRetentionNeverDropsPendingEvidence(t *testing.T) {
	r, a := windowIntegrationRepo(t)
	now := time.Now().UTC()
	old := now.AddDate(0, 0, -20)
	pendingID := insertWindowObservation(t, a.ID, old, old.Add(5*time.Hour), 50)
	processedID := insertWindowObservation(t, a.ID, old, old.Add(5*time.Hour), 60)
	_, err := integrationDB.Exec(`UPDATE account_quota_observations SET processed_at=NOW() WHERE id=$1`, processedID)
	require.NoError(t, err)
	var markerID int64
	require.NoError(t, integrationDB.QueryRow(`INSERT INTO account_quota_observations(account_id,observed_at,payload,processed_at) VALUES($1,$2,'{"codex_history_reset_at":"2026-01-01T00:00:00Z"}'::jsonb,NOW()) RETURNING id`, a.ID, old).Scan(&markerID))
	require.NoError(t, r.PruneHistory(context.Background(), now.AddDate(0, 0, -90), now.AddDate(0, 0, -14)))
	var ids []int64
	rows, err := integrationDB.Query(`SELECT id FROM account_quota_observations WHERE account_id=$1 ORDER BY id`, a.ID)
	require.NoError(t, err)
	defer func() { _ = rows.Close() }()
	for rows.Next() {
		var id int64
		require.NoError(t, rows.Scan(&id))
		ids = append(ids, id)
	}
	require.NoError(t, rows.Err())
	require.Equal(t, []int64{pendingID, markerID}, ids)
}

func TestQuotaHistoryLateTerminalIsRoutedToClosedPeriod(t *testing.T) {
	r, a := windowIntegrationRepo(t)
	ctx := context.Background()
	at := time.Now().UTC().Add(-7 * time.Hour).Truncate(time.Microsecond)
	reset := at.Add(5 * time.Hour)
	g := service.NewAccountWindowUsageIngester(r)
	insertWindowObservation(t, a.ID, at, reset, 50)
	insertWindowObservation(t, a.ID, reset, reset.Add(5*time.Hour), 0)
	// The earlier terminal sample commits later on another replica.
	insertWindowObservation(t, a.ID, reset.Add(-time.Second), reset, 100)
	for {
		ok, err := r.ConsumeNextObservation(ctx, time.Now(), g.ApplyObservation)
		require.NoError(t, err)
		if !ok {
			break
		}
	}
	rows, err := r.ListHistorySince(ctx, a.ID, at)
	require.NoError(t, err)
	require.Len(t, rows, 2)
	require.Equal(t, 100.0, rows[0].LastUsedPercent)
	require.Equal(t, 100.0, rows[0].PeakUsedPercent)
	require.Equal(t, 0.0, rows[1].LastUsedPercent)
	require.Equal(t, 2, rows[0].SampleCount)
	require.Equal(t, 1, rows[1].SampleCount)
}

func TestQuotaHistoryProducerRetryIsIdempotent(t *testing.T) {
	r, a := windowIntegrationRepo(t)
	ctx := context.Background()
	at := time.Now().UTC().Add(-7 * time.Hour).Truncate(time.Microsecond)
	reset := at.Add(5 * time.Hour)
	g := service.NewAccountWindowUsageIngester(r)
	for i := 0; i < 2; i++ {
		insertWindowObservation(t, a.ID, at, reset, 50)
	}
	for i := 0; i < 2; i++ {
		insertWindowObservation(t, a.ID, reset, reset.Add(5*time.Hour), 0)
	}
	for i := 0; i < 2; i++ {
		insertWindowObservation(t, a.ID, reset.Add(-time.Second), reset, 100)
	}
	for {
		ok, err := r.ConsumeNextObservation(ctx, time.Now(), g.ApplyObservation)
		require.NoError(t, err)
		if !ok {
			break
		}
	}
	rows, err := r.ListHistorySince(ctx, a.ID, at)
	require.NoError(t, err)
	require.Len(t, rows, 2)
	require.Equal(t, 2, rows[0].SampleCount)
	require.Equal(t, 1, rows[1].SampleCount)
	require.Equal(t, 100.0, rows[0].LastUsedPercent)
	require.Equal(t, 6, windowProcessedCount(t, a.ID))
}
