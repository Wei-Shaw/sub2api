//go:build integration

package repository

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/minimax"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
)

func mediaIntegrationFixture(t *testing.T) (*miniMaxMediaRepository, *service.MiniMaxMediaTask) {
	t.Helper()
	client := testEntClient(t)
	user := mustCreateUser(t, client, &service.User{Email: "media-" + uuid.NewString() + "@example.com", Balance: 10})
	group := mustCreateGroup(t, client, &service.Group{Name: "media-" + uuid.NewString(), Platform: service.PlatformMiniMax})
	key := mustCreateApiKey(t, client, &service.APIKey{UserID: user.ID, GroupID: &group.ID, Key: "sk-media-" + uuid.NewString(), Quota: 100})
	account := mustCreateAccount(t, client, &service.Account{Name: "media-" + uuid.NewString(), Type: service.AccountTypeAPIKey, Platform: service.PlatformMiniMax})
	one, half, zero := 1.0, .5, 0.0
	task := &service.MiniMaxMediaTask{ID: "mm_" + uuid.NewString(), Kind: "video", Model: minimax.H3, Resolution: "2K", Duration: 6, UserID: user.ID, APIKeyID: key.ID, GroupID: group.ID, AccountID: account.ID, AccountType: account.Type, Origin: "https://api.minimax.cn", IdempotencyKey: uuid.NewString(), RequestHash: "hash", ReservedUSD: 5, Pricing: service.MiniMaxMediaPricing{Resolution: "2K", CostCurrency: "USD", Multiplier: &one, Retail: service.MiniMaxMediaRates{OutputSecond: &half, InputSecond: &half, ExtraImage: &zero}, Cost: service.MiniMaxMediaRates{OutputSecond: &half, InputSecond: &half, ExtraImage: &zero}}}
	t.Cleanup(func() {
		cleanup := func(query string, args ...any) {
			if _, err := integrationDB.ExecContext(context.Background(), query, args...); err != nil {
				t.Errorf("cleanup MiniMax media fixture: %v", err)
			}
		}
		// The integration database is shared, so remove ledger rows before their owners.
		cleanup("DELETE FROM minimax_media_tasks WHERE user_id=$1", user.ID)
		cleanup("DELETE FROM usage_logs WHERE user_id=$1", user.ID)
		cleanup("DELETE FROM api_keys WHERE user_id=$1", user.ID)
		cleanup("DELETE FROM users WHERE id=$1", user.ID)
		cleanup("DELETE FROM accounts WHERE id=$1", account.ID)
		cleanup("DELETE FROM groups WHERE id=$1", group.ID)
	})
	return &miniMaxMediaRepository{db: integrationDB}, task
}
func mediaBalances(t *testing.T, task *service.MiniMaxMediaTask, wantBalance, wantFrozen, wantQuota float64) {
	t.Helper()
	var balance, frozen, quota float64
	require.NoError(t, integrationDB.QueryRow(`SELECT balance,frozen_balance FROM users WHERE id=$1`, task.UserID).Scan(&balance, &frozen))
	require.NoError(t, integrationDB.QueryRow(`SELECT quota_used FROM api_keys WHERE id=$1`, task.APIKeyID).Scan(&quota))
	require.InDelta(t, wantBalance, balance, 1e-8)
	require.InDelta(t, wantFrozen, frozen, 1e-8)
	require.InDelta(t, wantQuota, quota, 1e-8)
}

func TestMiniMaxMediaIntegrationStandardSynchronousUsage(t *testing.T) {
	for _, tc := range []struct {
		kind, model, mode, endpoint, source string
		usage                               minimax.Usage
		price, charged                      float64
	}{
		{"image", minimax.Image, "image", "/v1/image_generation", "", minimax.Usage{OutputImages: 2}, .0035, .007},
		{"voice_design", "voice_design", "per_request", "/v1/voice_design", "upstream_usage", minimax.Usage{Characters: 100}, 60, .006},
		{"voice_design", "voice_design", "per_request", "/v1/voice_design", "request_characters", minimax.Usage{Characters: 100}, 60, .006},
	} {
		t.Run(tc.kind+"/"+tc.source, func(t *testing.T) {
			ctx := context.Background()
			repo, task := mediaIntegrationFixture(t)
			task.Kind, task.Model, task.Resolution, task.Duration = tc.kind, tc.model, "", 0
			task.ReservedUSD = 1
			task.RequestInfo = service.MiniMaxMediaRequestInfo{StartedAt: time.Now().Add(-2 * time.Second), UserAgent: "MiniMax integration test", IPAddress: "192.0.2.1"}
			one := 1.0
			rates := service.MiniMaxMediaRates{CharactersPerMillion: &tc.price}
			if tc.kind == "image" {
				rates = service.MiniMaxMediaRates{Image: &tc.price}
			}
			task.Pricing = service.MiniMaxMediaPricing{Version: 2, CostCurrency: "USD", Multiplier: &one, Retail: rates, Cost: rates}
			_, created, err := repo.Reserve(ctx, task)
			require.NoError(t, err)
			require.True(t, created)
			require.NoError(t, repo.Record(ctx, task.ID, "succeeded", &service.MiniMaxMediaBill{Usage: tc.usage, MeterSource: tc.source}, nil))
			applied, err := repo.Settle(ctx, task.ID)
			require.NoError(t, err)
			require.True(t, applied)
			mediaBalances(t, task, 10-tc.charged, 0, tc.charged)
			var mode, inbound, upstream, agent, ip string
			var images, elapsed int64
			var size, source sql.NullString
			require.NoError(t, integrationDB.QueryRow(`SELECT billing_mode,inbound_endpoint,upstream_endpoint,image_count,duration_ms,user_agent,ip_address,image_size,image_size_source FROM usage_logs WHERE request_id=$1`, task.ID).Scan(&mode, &inbound, &upstream, &images, &elapsed, &agent, &ip, &size, &source))
			require.Equal(t, tc.mode, mode)
			require.Equal(t, tc.endpoint, inbound)
			require.Equal(t, tc.endpoint, upstream)
			require.EqualValues(t, tc.usage.OutputImages, images)
			if tc.kind == "image" {
				require.Equal(t, service.ImageBillingSizePerImage, size.String)
				require.Equal(t, service.ImageSizeSourceDefault, source.String)
				_, err := integrationDB.Exec(`UPDATE usage_logs SET image_size=NULL WHERE request_id=$1`, task.ID)
				require.Error(t, err, "positive image counts still require an explicit billing tier")
			} else {
				require.False(t, size.Valid)
			}
			require.GreaterOrEqual(t, elapsed, int64(2000))
			require.Equal(t, task.RequestInfo.UserAgent, agent)
			require.Equal(t, task.RequestInfo.IPAddress, ip)
			fresh, err := repo.Get(ctx, task.ID, task.UserID, task.APIKeyID)
			require.NoError(t, err)
			require.Equal(t, tc.source, fresh.Bill.MeterSource)
		})
	}
}

func TestMiniMaxMediaIntegrationConcurrentVoiceActivation(t *testing.T) {
	ctx := context.Background()
	repo, base := mediaIntegrationFixture(t)
	t.Cleanup(func() {
		_, err := integrationDB.Exec(`DELETE FROM minimax_media_assets WHERE user_id=$1`, base.UserID)
		require.NoError(t, err)
	})
	require.NoError(t, repo.SaveAsset(ctx, &service.MiniMaxMediaAsset{Kind: "voice", ID: "privateVoice", Category: "voice_clone", UserID: base.UserID, APIKeyID: base.APIKeyID, AccountID: base.AccountID}))
	_, err := repo.FindAsset(ctx, "voice", "privateVoice", base.UserID, base.APIKeyID+1000000)
	require.ErrorIs(t, err, service.ErrMiniMaxMediaNotFound)
	one, chars := 1.0, 60.0
	tasks := []*service.MiniMaxMediaTask{}
	for i := 0; i < 2; i++ {
		task := *base
		task.ID = "mm_" + uuid.NewString()
		task.IdempotencyKey = uuid.NewString()
		task.Kind = "speech"
		task.Model = minimax.SpeechTurbo
		task.Resolution = ""
		task.Duration = 0
		task.ReservedUSD = 1.56
		task.Pricing = service.MiniMaxMediaPricing{Version: 2, CostCurrency: "USD", Multiplier: &one, Retail: service.MiniMaxMediaRates{CharactersPerMillion: &chars}, Cost: service.MiniMaxMediaRates{CharactersPerMillion: &chars}, CustomVoices: []service.MiniMaxVoiceCharge{{ID: "privateVoice", Kind: "voice_clone", Retail: 1.5, Cost: 1.5}}}
		_, created, e := repo.Reserve(ctx, &task)
		require.NoError(t, e)
		require.True(t, created)
		require.NoError(t, repo.Record(ctx, task.ID, "succeeded", &service.MiniMaxMediaBill{Usage: minimax.Usage{Characters: 1000, VoiceCount: 1}}, nil))
		tasks = append(tasks, &task)
	}
	var wg sync.WaitGroup
	errs := make(chan error, 2)
	for _, task := range tasks {
		wg.Add(1)
		go func(id string) { defer wg.Done(); _, e := repo.Settle(ctx, id); errs <- e }(task.ID)
	}
	wg.Wait()
	close(errs)
	for e := range errs {
		require.NoError(t, e)
	}
	mediaBalances(t, base, 8.38, 0, 1.62)
	var charged float64
	require.NoError(t, integrationDB.QueryRow(`SELECT sum(actual_cost) FROM usage_logs WHERE user_id=$1`, base.UserID).Scan(&charged))
	require.InDelta(t, 1.62, charged, 1e-8)
	asset, e := repo.FindAsset(ctx, "voice", "privateVoice", base.UserID, base.APIKeyID)
	require.NoError(t, e)
	require.True(t, asset.Activated)
	for _, task := range tasks {
		_, e = repo.Settle(ctx, task.ID)
		require.NoError(t, e)
	}
	mediaBalances(t, base, 8.38, 0, 1.62)
}
func TestMiniMaxMediaIntegrationReserveSettleConcurrentReplay(t *testing.T) {
	ctx := context.Background()
	repo, task := mediaIntegrationFixture(t)
	_, created, err := repo.Reserve(ctx, task)
	require.NoError(t, err)
	require.True(t, created)
	mediaBalances(t, task, 5, 5, 5)
	old, created, err := repo.Reserve(ctx, task)
	require.NoError(t, err)
	require.False(t, created)
	require.Equal(t, task.ID, old.ID)
	other := *task
	other.RequestHash = "other"
	_, _, err = repo.Reserve(ctx, &other)
	require.ErrorIs(t, err, service.ErrMiniMaxMediaConflict)
	upstreamID := "upstream-" + uuid.NewString()
	require.NoError(t, repo.Submitted(ctx, task.ID, upstreamID))
	require.NoError(t, repo.Submitted(ctx, task.ID, upstreamID))
	require.Error(t, repo.Submitted(ctx, task.ID, "different-provider-task"))
	require.NoError(t, repo.Record(ctx, task.ID, "succeeded", &service.MiniMaxMediaBill{Usage: minimax.Usage{OutputSeconds: 6}}, nil))
	var wg sync.WaitGroup
	errs := make(chan error, 8)
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() { defer wg.Done(); _, err := repo.Settle(ctx, task.ID); errs <- err }()
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		require.NoError(t, err)
	}
	mediaBalances(t, task, 7, 0, 3)
	var count int
	require.NoError(t, integrationDB.QueryRow(`SELECT count(*) FROM usage_logs WHERE request_id=$1`, task.ID).Scan(&count))
	require.Equal(t, 1, count)
	require.NoError(t, repo.RefreshResult(ctx, task.ID, json.RawMessage(`{"content":{"url":"https://example.com/refreshed.mp4"}}`)))
	mediaBalances(t, task, 7, 0, 3)
	fresh, err := repo.Get(ctx, task.ID, task.UserID, task.APIKeyID)
	require.NoError(t, err)
	require.Contains(t, string(fresh.Result), "refreshed.mp4")
	var cost float64
	require.NoError(t, integrationDB.QueryRow(`SELECT account_stats_cost FROM usage_logs WHERE request_id=$1`, task.ID).Scan(&cost))
	require.Equal(t, 3.0, cost)
	_, err = repo.Get(ctx, task.ID, task.UserID+1, task.APIKeyID)
	require.ErrorIs(t, err, service.ErrMiniMaxMediaNotFound)
	_, err = repo.Get(ctx, task.ID, task.UserID, task.APIKeyID+1)
	require.ErrorIs(t, err, service.ErrMiniMaxMediaNotFound)
}
func TestMiniMaxMediaIntegrationNoOverspendAndUnknownHeld(t *testing.T) {
	ctx := context.Background()
	repo, task := mediaIntegrationFixture(t)
	task.ReservedUSD = 6
	var wg sync.WaitGroup
	errs := make(chan error, 2)
	for i := 0; i < 2; i++ {
		next := *task
		next.ID = "mm_" + uuid.NewString()
		next.IdempotencyKey = uuid.NewString()
		wg.Add(1)
		go func() { defer wg.Done(); _, _, err := repo.Reserve(ctx, &next); errs <- err }()
	}
	wg.Wait()
	close(errs)
	successes := 0
	for err := range errs {
		if err == nil {
			successes++
		} else {
			require.ErrorIs(t, err, service.ErrMiniMaxMediaFunds)
		}
	}
	require.Equal(t, 1, successes)
	mediaBalances(t, task, 4, 6, 6)
	tasks, err := repo.List(ctx, task.UserID, task.APIKeyID, 10)
	require.NoError(t, err)
	require.Len(t, tasks, 1)
	require.NoError(t, repo.Record(ctx, tasks[0].ID, "unknown", nil, nil))
	_, err = repo.Settle(ctx, tasks[0].ID)
	require.Error(t, err)
	mediaBalances(t, task, 4, 6, 6)
}
func TestMiniMaxMediaIntegrationFailureReleaseAndReceiptRecovery(t *testing.T) {
	ctx := context.Background()
	repo, task := mediaIntegrationFixture(t)
	_, _, err := repo.Reserve(ctx, task)
	require.NoError(t, err)
	require.NoError(t, repo.Record(ctx, task.ID, "failed", &service.MiniMaxMediaBill{}, nil))
	_, err = repo.Settle(ctx, task.ID)
	require.NoError(t, err)
	mediaBalances(t, task, 10, 0, 0)
	// A receipt is immutable even if a later poll reports a different status.
	require.NoError(t, repo.Record(ctx, task.ID, "succeeded", &service.MiniMaxMediaBill{Usage: minimax.Usage{OutputSeconds: 6}}, nil))
	_, err = repo.Settle(ctx, task.ID)
	require.NoError(t, err)
	mediaBalances(t, task, 10, 0, 0)
	task.ID = "mm_" + uuid.NewString()
	task.IdempotencyKey = uuid.NewString()
	_, _, err = repo.Reserve(ctx, task)
	require.NoError(t, err)
	require.NoError(t, repo.Record(ctx, task.ID, "succeeded", &service.MiniMaxMediaBill{Usage: minimax.Usage{OutputSeconds: 6}}, nil))
	_, err = integrationDB.Exec(`UPDATE minimax_media_tasks SET next_poll_at=NOW()-INTERVAL '1 second' WHERE id=$1`, task.ID)
	require.NoError(t, err)
	claimed, err := repo.Claim(ctx, 100)
	require.NoError(t, err)
	found := false
	for _, v := range claimed {
		if v.ID == task.ID {
			found = true
			require.NotNil(t, v.Bill)
		}
	}
	require.True(t, found)
	_, err = repo.Settle(ctx, task.ID)
	require.NoError(t, err)
	mediaBalances(t, task, 7, 0, 3)
}
func TestMiniMaxMediaIntegrationSubscriptionWindowRefund(t *testing.T) {
	ctx := context.Background()
	repo, task := mediaIntegrationFixture(t)
	now := time.Now().UTC().Truncate(time.Second)
	sub := mustCreateSubscription(t, testEntClient(t), &service.UserSubscription{UserID: task.UserID, GroupID: task.GroupID, DailyWindowStart: &now, WeeklyWindowStart: &now, MonthlyWindowStart: &now})
	task.SubscriptionID = &sub.ID
	_, err := integrationDB.Exec(`UPDATE groups SET subscription_type='subscription',daily_limit_usd=6,weekly_limit_usd=20,monthly_limit_usd=30 WHERE id=$1`, task.GroupID)
	require.NoError(t, err)
	_, _, err = repo.Reserve(ctx, task)
	require.NoError(t, err)
	mediaBalances(t, task, 10, 0, 5)
	next := *task
	next.ID = "mm_" + uuid.NewString()
	next.IdempotencyKey = uuid.NewString()
	_, _, err = repo.Reserve(ctx, &next)
	require.ErrorIs(t, err, service.ErrMiniMaxMediaFunds)
	// Reset to a new day while work is running. Refund the old week/month
	// reservation, but do not subtract it from the new day's unrelated usage.
	_, err = integrationDB.Exec(`UPDATE user_subscriptions SET daily_window_start=$1,daily_usage_usd=1 WHERE id=$2`, now.Add(time.Hour), sub.ID)
	require.NoError(t, err)
	require.NoError(t, repo.Record(ctx, task.ID, "succeeded", &service.MiniMaxMediaBill{Usage: minimax.Usage{OutputSeconds: 6}}, nil))
	_, err = repo.Settle(ctx, task.ID)
	require.NoError(t, err)
	var day, week, month float64
	require.NoError(t, integrationDB.QueryRow(`SELECT daily_usage_usd,weekly_usage_usd,monthly_usage_usd FROM user_subscriptions WHERE id=$1`, sub.ID).Scan(&day, &week, &month))
	require.Equal(t, 4.0, day)
	require.Equal(t, 3.0, week)
	require.Equal(t, 3.0, month)
	mediaBalances(t, task, 10, 0, 3)
	t.Log(fmt.Sprintf("subscription %d settled without altering wallet", sub.ID))
}

func TestMiniMaxMediaIntegrationKeyWindowAndQuotaStatusAfterRelease(t *testing.T) {
	ctx := context.Background()
	repo, task := mediaIntegrationFixture(t)
	_, _, err := repo.Reserve(ctx, task)
	require.NoError(t, err)
	// Another request exhausted the key while this task was in flight. Its
	// five-hour window has since reset and contains unrelated new usage.
	_, err = integrationDB.Exec(`UPDATE api_keys SET quota=6,quota_used=6,status='quota_exhausted',window_5h_start=NOW()+INTERVAL '1 hour',usage_5h=1 WHERE id=$1`, task.APIKeyID)
	require.NoError(t, err)
	require.NoError(t, repo.Record(ctx, task.ID, "failed", &service.MiniMaxMediaBill{}, nil))
	applied, err := repo.Settle(ctx, task.ID)
	require.NoError(t, err)
	require.True(t, applied)
	applied, err = repo.Settle(ctx, task.ID)
	require.NoError(t, err)
	require.False(t, applied)
	mediaBalances(t, task, 10, 0, 1)
	var status string
	var five, day, week float64
	require.NoError(t, integrationDB.QueryRow(`SELECT status,usage_5h,usage_1d,usage_7d FROM api_keys WHERE id=$1`, task.APIKeyID).Scan(&status, &five, &day, &week))
	require.Equal(t, "active", status)
	require.Equal(t, 1.0, five)
	require.Zero(t, day)
	require.Zero(t, week)
}

func TestMiniMaxMediaIntegrationSimpleModeNeverDebitsWallet(t *testing.T) {
	for _, policy := range []string{service.MiniMaxBillingSimple, service.MiniMaxBillingKeyWindows} {
		t.Run(policy, func(t *testing.T) {
			ctx := context.Background()
			repo, task := mediaIntegrationFixture(t)
			task.Kind, task.Model, task.Resolution, task.Duration = "image", minimax.Image, "", 0
			task.BillingPolicy = policy
			one, price := 1.0, .0035
			task.Pricing = service.MiniMaxMediaPricing{Version: 2, CostCurrency: "USD", Multiplier: &one, Retail: service.MiniMaxMediaRates{Image: &price}, Cost: service.MiniMaxMediaRates{Image: &price}}
			task.ReservedUSD = 0
			if task.TracksKeyWindows() {
				task.ReservedUSD = .007
			}
			_, err := integrationDB.Exec(`UPDATE users SET balance=0 WHERE id=$1`, task.UserID)
			require.NoError(t, err)
			_, err = integrationDB.Exec(`UPDATE api_keys SET quota=1,quota_used=7,status='quota_exhausted',expires_at=NOW()-INTERVAL '1 day',rate_limit_5h=.01 WHERE id=$1`, task.APIKeyID)
			require.NoError(t, err)
			_, err = integrationDB.Exec(`UPDATE accounts SET extra='{"quota_limit":100,"quota_used":2}'::jsonb WHERE id=$1`, task.AccountID)
			require.NoError(t, err)
			_, created, err := repo.Reserve(ctx, task)
			require.NoError(t, err)
			require.True(t, created)
			mediaBalances(t, task, 0, 0, 7)
			fresh, err := repo.Get(ctx, task.ID, task.UserID, task.APIKeyID)
			require.NoError(t, err)
			require.Equal(t, policy, fresh.BillingPolicy)
			require.NoError(t, repo.Record(ctx, task.ID, "succeeded", &service.MiniMaxMediaBill{Usage: minimax.Usage{OutputImages: 1}}, nil))
			applied, err := repo.Settle(ctx, task.ID)
			require.NoError(t, err)
			require.True(t, applied)
			mediaBalances(t, task, 0, 0, 7)
			var five, accountUsage, charged float64
			require.NoError(t, integrationDB.QueryRow(`SELECT usage_5h FROM api_keys WHERE id=$1`, task.APIKeyID).Scan(&five))
			if task.TracksKeyWindows() {
				require.InDelta(t, .0035, five, 1e-8)
			} else {
				require.Zero(t, five)
			}
			require.NoError(t, integrationDB.QueryRow(`SELECT (extra->>'quota_used')::numeric FROM accounts WHERE id=$1`, task.AccountID).Scan(&accountUsage))
			require.Equal(t, 2.0, accountUsage)
			require.NoError(t, integrationDB.QueryRow(`SELECT actual_cost FROM usage_logs WHERE request_id=$1`, task.ID).Scan(&charged))
			require.Zero(t, charged)
			fresh, err = repo.Get(ctx, task.ID, task.UserID, task.APIKeyID)
			require.NoError(t, err)
			require.Zero(t, fresh.Bill.ChargedUSD)
			applied, err = repo.Settle(ctx, task.ID)
			require.NoError(t, err)
			require.False(t, applied)
		})
	}
}

func TestMiniMaxMediaIntegrationPartialVoiceReceiptRecovers(t *testing.T) {
	ctx := context.Background()
	repo, task := mediaIntegrationFixture(t)
	t.Cleanup(func() {
		_, err := integrationDB.Exec(`DELETE FROM minimax_media_assets WHERE user_id=$1`, task.UserID)
		require.NoError(t, err)
	})
	for _, id := range []string{"voiceA", "voiceB"} {
		require.NoError(t, repo.SaveAsset(ctx, &service.MiniMaxMediaAsset{Kind: "voice", ID: id, Category: "voice_clone", AccountID: task.AccountID, UserID: task.UserID, APIKeyID: task.APIKeyID}))
	}
	one, chars := 1.0, 60.0
	task.Kind, task.Model, task.Resolution, task.Duration = "speech", minimax.SpeechTurbo, "", 0
	task.Pricing = service.MiniMaxMediaPricing{Version: 2, CostCurrency: "USD", Multiplier: &one, Retail: service.MiniMaxMediaRates{CharactersPerMillion: &chars}, Cost: service.MiniMaxMediaRates{CharactersPerMillion: &chars}, CustomVoices: []service.MiniMaxVoiceCharge{{ID: "voiceA", Kind: "voice_clone", Retail: 1.5, Cost: 1.5}, {ID: "voiceB", Kind: "voice_design", Retail: 3, Cost: 3}}}
	_, created, err := repo.Reserve(ctx, task)
	require.NoError(t, err)
	require.True(t, created)
	u := minimax.Usage{Characters: 1000, VoiceCount: 1}
	completedAt := time.Now().UTC().Truncate(time.Second)
	require.NoError(t, repo.Record(ctx, task.ID, "succeeded", &service.MiniMaxMediaBill{Usage: u, ReconciliationRequired: true, MeterSource: "upstream_usage", CompletedAt: completedAt}, nil))
	applied, err := repo.Settle(ctx, task.ID)
	require.Error(t, err, "without an activation record either differently priced voice could be billed")
	require.False(t, applied)
	mediaBalances(t, task, 5, 5, 5)
	fresh, err := repo.Get(ctx, task.ID, task.UserID, task.APIKeyID)
	require.NoError(t, err)
	require.Equal(t, u, fresh.Bill.Usage, "receipt remains durable while awaiting attribution")
	// Another request's first-use settlement becomes visible after this one.
	_, err = integrationDB.Exec(`UPDATE minimax_media_assets SET activated_task_id='mm_prior' WHERE account_id=$1 AND asset_id='voiceA'`, task.AccountID)
	require.NoError(t, err)
	applied, err = repo.Settle(ctx, task.ID)
	require.NoError(t, err)
	require.True(t, applied)
	mediaBalances(t, task, 6.94, 0, 3.06)
	fresh, err = repo.Get(ctx, task.ID, task.UserID, task.APIKeyID)
	require.NoError(t, err)
	require.Equal(t, []string{"voiceB"}, fresh.Bill.BilledVoices)
	require.Equal(t, 3.06, fresh.Bill.UpstreamUSD)
	require.Equal(t, "upstream_usage", fresh.Bill.MeterSource)
	require.True(t, completedAt.Equal(fresh.Bill.CompletedAt))
	require.False(t, fresh.Bill.ReconciliationRequired)
}
