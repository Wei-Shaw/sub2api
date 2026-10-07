//go:build integration

package muse_test

import (
	"context"
	"database/sql"
	"encoding/json"
	"strings"
	"sync"
	"testing"
	"time"

	"entgo.io/ent/dialect"
	entsql "entgo.io/ent/dialect/sql"
	dbent "github.com/Wei-Shaw/sub2api/ent"
	_ "github.com/Wei-Shaw/sub2api/ent/runtime"
	"github.com/Wei-Shaw/sub2api/internal/pkg/apicompat"
	"github.com/Wei-Shaw/sub2api/internal/pkg/muse"
	"github.com/Wei-Shaw/sub2api/internal/pkg/timezone"
	"github.com/Wei-Shaw/sub2api/internal/repository"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/Wei-Shaw/sub2api/migrations"
	"github.com/stretchr/testify/require"
)

// Synthetic provider contracts exercise the production schema and native billing
// transaction. They are deliberately not represented as Meta protocol fixtures.
func TestMuseProviderPostgres(t *testing.T) {
	ctx := context.Background()
	require.NoError(t, timezone.Init("UTC"))
	db := museIsolatedTestDatabase(t)
	client := dbent.NewClient(dbent.Driver(entsql.OpenDB(dialect.Postgres, db)))
	require.NoError(t, client.Schema.Create(ctx))
	for _, name := range []string{
		"027_usage_billing_consistency.sql",
		"036_scheduler_outbox.sql",
		"046_add_usage_log_reasoning_effort.sql",
		"060_add_usage_log_openai_ws_mode.sql",
		"061_add_usage_log_request_type.sql",
		"070_add_usage_log_service_tier.sql",
		"071_add_usage_billing_dedup.sql",
		"073_add_usage_billing_dedup_archive.sql",
		"074_add_usage_log_endpoints.sql",
		"089_usage_log_image_output_tokens.sql",
		"101_add_account_stats_pricing.sql",
		"179_usage_log_image_input_tokens.sql",
		"187_add_usage_log_session_id.sql",
		"231_add_usage_log_requested_reasoning_effort.sql",
		"231_add_usage_log_native_compaction_v2.sql",
		"232_add_usage_log_upstream_request_id.sql",
		"241_add_typesafe_platform.sql",
		"242_muse_runtime.sql",
		"243_muse_provider.sql",
		"244_muse_submission_snapshot.sql",
	} {
		ddl, e := migrations.FS.ReadFile(name)
		require.NoError(t, e)
		if name == "101_add_account_stats_pricing.sql" {
			// Channels use raw repositories rather than Ent; only the physical usage
			// column is relevant to the billing serializer tested here.
			ddl = []byte(string(ddl)[strings.Index(string(ddl), "ALTER TABLE usage_logs"):])
		}
		_, e = db.ExecContext(ctx, string(ddl))
		require.NoError(t, e, name)
	}
	ddl, err := migrations.FS.ReadFile("243_muse_provider.sql")
	require.NoError(t, err)
	_, err = db.ExecContext(ctx, string(ddl))
	require.NoError(t, err, "provider migration is rerunnable")
	owner, e := client.User.Create().SetEmail("muse-fixture@example.invalid").SetPasswordHash("synthetic").SetBalance(10).Save(ctx)
	require.NoError(t, e)
	require.EqualValues(t, 1, owner.ID)
	t.Run("ProviderMigrationPreservesTypeSafeQuotas", func(t *testing.T) {
		quota, err := client.UserPlatformQuota.Create().SetUserID(owner.ID).SetPlatform(service.PlatformTypeSafe).SetDailyLimitUsd(1).Save(ctx)
		require.NoError(t, err)
		_, err = db.ExecContext(ctx, string(ddl))
		require.NoError(t, err, "Muse registration must preserve existing TypeSafe quota rows")
		err = client.UserPlatformQuota.DeleteOneID(quota.ID).Exec(ctx)
		require.NoError(t, err)
	})
	_, e = client.User.Create().SetEmail("foreign-fixture@example.invalid").SetPasswordHash("synthetic").SetBalance(10).Save(ctx)
	require.NoError(t, e)
	group, e := client.Group.Create().SetName("muse-fixture").SetPlatform(service.PlatformMuse).Save(ctx)
	require.NoError(t, e)
	require.EqualValues(t, 1, group.ID)
	key, e := client.APIKey.Create().SetUserID(owner.ID).SetGroupID(group.ID).SetKey("synthetic-key").SetName("fixture").SetQuota(1).SetRateLimit5h(1).Save(ctx)
	require.NoError(t, e)
	require.EqualValues(t, 1, key.ID)
	_, e = client.UserPlatformQuota.Create().SetUserID(owner.ID).SetPlatform(service.PlatformMuse).SetDailyLimitUsd(1).Save(ctx)
	require.NoError(t, e)
	runtime := service.NewMuseRuntimeService(repository.NewMuseRuntimeRepository(db))
	store := repository.NewMuseProviderRepository(db)
	newAccount := func(t *testing.T) *service.Account {
		credentials := map[string]any{"muse_session": map[string]any{"fixture": "synthetic"}}
		saved, e := client.Account.Create().SetName("fixture").SetPlatform(service.PlatformMuse).SetType(service.AccountTypeSession).SetCredentials(credentials).SetExtra(map[string]any{service.MuseOwnerUserIDKey: int64(1)}).SetSchedulable(false).SetConcurrency(1).Save(ctx)
		require.NoError(t, e)
		id := saved.ID
		a := &service.Account{ID: id, Platform: service.PlatformMuse, Type: service.AccountTypeSession, Status: service.StatusActive, Credentials: credentials, Extra: map[string]any{service.MuseOwnerUserIDKey: int64(1)}}
		require.NoError(t, db.QueryRowContext(ctx, `SELECT updated_at FROM accounts WHERE id=$1`, id).Scan(&a.UpdatedAt))
		return a
	}
	bind := func(t *testing.T, a *service.Account) *muse.Workspace {
		identity := muse.Identity{PrincipalID: t.Name(), WorkspaceID: "fixture-workspace", OwnerUserID: 1, AccountID: a.ID, AccountUpdatedAt: a.UpdatedAt}
		w, e := runtime.BindVerifiedWorkspace(ctx, identity)
		require.NoError(t, e)
		require.NoError(t, store.SaveProfile(ctx, a, &muse.Observation{InferenceAllowed: true, Identity: identity, Capabilities: muse.Capabilities{Models: []string{"muse/assistant"}}}))
		require.NoError(t, store.EnableVerified(ctx, a))
		return w
	}
	reserve := func(t *testing.T, w *muse.Workspace) (*muse.Turn, muse.Lease) {
		turn, lease, e := runtime.Reserve(ctx, muse.Reservation{WorkspaceID: w.ID, Generation: w.Generation, Actor: muse.Actor{UserID: 1, APIKeyID: 1}, AccountID: w.Identity.AccountID, AccountUpdatedAt: w.Identity.AccountUpdatedAt, LeaseOwner: "fixture", LeaseDuration: time.Minute, Pricing: muse.Pricing{Mode: "flat_request", UnitPrice: "0.03", Multiplier: "1.25"}})
		require.NoError(t, e)
		return turn, lease
	}
	freeze := func(t *testing.T, turn *muse.Turn) {
		log := &service.UsageLog{UserID: 1, APIKeyID: 1, AccountID: turn.AccountID, RequestID: turn.ID, Model: "muse/assistant", TotalCost: 0.03, ActualCost: 0.0375, RateMultiplier: 1.25}
		group := int64(1)
		log.GroupID = &group
		mode := "per_request"
		log.BillingMode = &mode
		cmd := &service.UsageBillingCommand{RequestID: turn.ID, UserID: 1, APIKeyID: 1, AccountID: turn.AccountID, AccountType: service.AccountTypeSession, Model: log.Model, BalanceCost: 0.0375, APIKeyQuotaCost: 0.0375, APIKeyRateLimitCost: 0.0375}
		require.NoError(t, store.FreezeCharge(ctx, turn.ID, cmd, log))
	}
	complete := func(t *testing.T, turn *muse.Turn, lease muse.Lease, outcome muse.State) {
		require.NoError(t, runtime.BeginSubmission(ctx, lease))
		require.NoError(t, runtime.Advance(ctx, lease, muse.Submitting, muse.Accepted, ("muse_noise_v1:11111111-2222-4333-8444-555555555555:"+strings.Repeat("a", 64)+":aaaaaaaa-bbbb-4ccc-8ddd-eeeeeeeeeeee")))
		require.NoError(t, store.RecordResult(ctx, turn.ID, &muse.Result{ProviderTurnID: ("muse_noise_v1:11111111-2222-4333-8444-555555555555:" + strings.Repeat("a", 64) + ":aaaaaaaa-bbbb-4ccc-8ddd-eeeeeeeeeeee"), Response: &apicompat.ResponsesResponse{Status: "completed", Model: "observed-fixture-model"}}))
		require.NoError(t, runtime.Advance(ctx, lease, muse.Accepted, outcome, ("muse_noise_v1:11111111-2222-4333-8444-555555555555:"+strings.Repeat("a", 64)+":aaaaaaaa-bbbb-4ccc-8ddd-eeeeeeeeeeee")))
	}
	balance := func() string {
		var s string
		require.NoError(t, db.QueryRowContext(ctx, `SELECT balance::text FROM users WHERE id=1`).Scan(&s))
		return s
	}
	t.Run("ConcurrentSettlementAppliesOnce", func(t *testing.T) {
		a := newAccount(t)
		w := bind(t, a)
		turn, lease := reserve(t, w)
		freeze(t, turn)
		complete(t, turn, lease, muse.Completed)
		var wg sync.WaitGroup
		results := make(chan *service.MuseSettlement, 12)
		errors := make(chan error, 12)
		for range 12 {
			wg.Add(1)
			go func() { defer wg.Done(); r, e := store.Settle(ctx, turn.ID); results <- r; errors <- e }()
		}
		wg.Wait()
		close(results)
		close(errors)
		for e := range errors {
			require.NoError(t, e)
		}
		applied := 0
		for r := range results {
			if r != nil && r.Applied {
				applied++
			}
		}
		require.Equal(t, 1, applied)
		require.Equal(t, "9.96250000", balance())
		var count int
		require.NoError(t, db.QueryRowContext(ctx, `SELECT COUNT(*) FROM usage_logs WHERE request_id=$1`, turn.ID).Scan(&count))
		require.Equal(t, 1, count)
		var loggedRef, preservedRef string
		require.NoError(t, db.QueryRowContext(ctx, `SELECT upstream_request_id FROM usage_logs WHERE request_id=$1`, turn.ID).Scan(&loggedRef))
		require.NoError(t, db.QueryRowContext(ctx, `SELECT provider_turn_id FROM muse_turns WHERE id=$1`, turn.ID).Scan(&preservedRef))
		require.LessOrEqual(t, len(loggedRef), 128)
		require.True(t, strings.HasPrefix(loggedRef, "muse_ref_"))
		require.Greater(t, len(preservedRef), 128, "recovery must retain its complete canonical reference")
		require.NoError(t, db.QueryRowContext(ctx, `SELECT COUNT(*) FROM usage_billing_dedup WHERE request_id=$1`, turn.ID).Scan(&count))
		require.Equal(t, 1, count)
		var quota, daily string
		require.NoError(t, db.QueryRowContext(ctx, `SELECT quota_used::text FROM api_keys WHERE id=1`).Scan(&quota))
		require.Equal(t, "0.03750000", quota)
		require.NoError(t, db.QueryRowContext(ctx, `SELECT daily_usage_usd::text FROM user_platform_quotas WHERE user_id=1 AND platform='muse'`).Scan(&daily))
		require.InDelta(t, 0.0375, mustFixtureNumber(t, daily), 1e-8)
		var observed string
		var usage sql.NullString
		var link sql.NullInt64
		require.NoError(t, db.QueryRowContext(ctx, `SELECT observed_model,reported_usage::text,usage_log_id FROM muse_turns WHERE id=$1`, turn.ID).Scan(&observed, &usage, &link))
		require.Equal(t, "observed-fixture-model", observed)
		require.Equal(t, "null", usage.String)
		require.True(t, link.Valid)
		var updatedAt time.Time
		var lastUsedAt sql.NullTime
		require.NoError(t, db.QueryRowContext(ctx, `SELECT updated_at,last_used_at FROM accounts WHERE id=$1`, a.ID).Scan(&updatedAt, &lastUsedAt))
		require.True(t, updatedAt.Equal(a.UpdatedAt), "activity must retain the verified credential snapshot")
		require.True(t, lastUsedAt.Valid)
		_, e = store.Profile(ctx, a)
		require.NoError(t, e, "settlement must leave the verified session usable")
		var events int
		require.NoError(t, db.QueryRowContext(ctx, `SELECT COUNT(*) FROM scheduler_outbox WHERE account_id=$1 AND event_type=$2`, a.ID, service.SchedulerOutboxEventAccountLastUsed).Scan(&events))
		require.Equal(t, 1, events, "retrying settlement must not repeat its activity event")
		_, _, e = runtime.Reserve(ctx, muse.Reservation{WorkspaceID: w.ID, Generation: w.Generation, Actor: turn.Actor, AccountID: a.ID, AccountUpdatedAt: a.UpdatedAt, LeaseOwner: "next-turn", LeaseDuration: time.Minute, Pricing: turn.Pricing})
		require.NoError(t, e, "another request must be admissible after settlement")
	})
	t.Run("LogFailureRollsBackLedgerAndBalance", func(t *testing.T) {
		a := newAccount(t)
		w := bind(t, a)
		turn, lease := reserve(t, w)
		freeze(t, turn)
		complete(t, turn, lease, muse.Completed)
		before := balance()
		_, _, blocked := runtime.Reserve(ctx, muse.Reservation{WorkspaceID: w.ID, Generation: w.Generation, Actor: turn.Actor, AccountID: a.ID, AccountUpdatedAt: a.UpdatedAt, LeaseOwner: "blocked", LeaseDuration: time.Minute, Pricing: turn.Pricing})
		require.ErrorIs(t, blocked, muse.ErrBusy, "pending settlement blocks new work")
		_, e := db.ExecContext(ctx, `CREATE FUNCTION muse_fail_log() RETURNS trigger LANGUAGE plpgsql AS $$BEGIN RAISE EXCEPTION 'synthetic log failure'; END$$;CREATE TRIGGER muse_fail_log BEFORE INSERT ON usage_logs FOR EACH ROW EXECUTE FUNCTION muse_fail_log()`)
		require.NoError(t, e)
		_, e = store.Settle(ctx, turn.ID)
		require.Error(t, e)
		require.Equal(t, before, balance())
		var count int
		require.NoError(t, db.QueryRowContext(ctx, `SELECT COUNT(*) FROM usage_billing_dedup WHERE request_id=$1`, turn.ID).Scan(&count))
		require.Zero(t, count)
		_, e = db.ExecContext(ctx, `DROP TRIGGER muse_fail_log ON usage_logs;DROP FUNCTION muse_fail_log()`)
		require.NoError(t, e)
		pending, e := store.PendingSettlement(ctx, 25)
		require.NoError(t, e)
		require.Contains(t, pending, turn.ID)
		r, e := store.Settle(ctx, turn.ID)
		require.NoError(t, e)
		require.True(t, r.Applied)
	})
	t.Run("RemoteFailureSettlesZero", func(t *testing.T) {
		a := newAccount(t)
		w := bind(t, a)
		turn, lease := reserve(t, w)
		freeze(t, turn)
		complete(t, turn, lease, muse.Failed)
		before := balance()
		r, e := store.Settle(ctx, turn.ID)
		require.NoError(t, e)
		require.True(t, r.Applied)
		require.Equal(t, before, balance())
		require.Zero(t, r.Log.ActualCost)
	})
	t.Run("RenewalIsBlockedBeforeProviderCall", func(t *testing.T) {
		a := newAccount(t)
		w := bind(t, a)
		_, _ = reserve(t, w)
		calls := 0
		e := store.RenewSession(ctx, a, func(context.Context) (map[string]any, error) {
			calls++
			return map[string]any{"renewed": "synthetic"}, nil
		})
		require.ErrorIs(t, e, muse.ErrBusy)
		require.Zero(t, calls)
	})
	t.Run("RenewalInvalidatesProfileAndPreservesModelMapping", func(t *testing.T) {
		a := newAccount(t)
		_ = bind(t, a)
		e := store.RenewSession(ctx, a, func(context.Context) (map[string]any, error) { return map[string]any{"renewed": "synthetic"}, nil })
		require.NoError(t, e)
		_, e = store.Profile(ctx, a)
		require.ErrorIs(t, e, muse.ErrTransportUnqualified)
		e = store.RenewSession(ctx, a, func(context.Context) (map[string]any, error) {
			t.Fatal("stale account must not invoke provider")
			return nil, nil
		})
		require.ErrorIs(t, e, muse.ErrGeneration)
	})
	t.Run("ProxyEditsInvalidateVerificationBeforeRenewal", func(t *testing.T) {
		a := newAccount(t)
		proxy, e := client.Proxy.Create().SetName("synthetic").SetProtocol("http").SetHost("127.0.0.1").SetPort(12345).Save(ctx)
		require.NoError(t, e)
		_, e = db.ExecContext(ctx, `UPDATE accounts SET proxy_id=$2 WHERE id=$1`, a.ID, proxy.ID)
		require.NoError(t, e)
		a.ProxyID = &proxy.ID
		var at time.Time
		require.NoError(t, db.QueryRowContext(ctx, `SELECT updated_at FROM proxies WHERE id=$1`, proxy.ID).Scan(&at))
		a.Proxy = &service.Proxy{ID: proxy.ID, UpdatedAt: at}
		w := bind(t, a)
		observation := &muse.Observation{InferenceAllowed: true, ProxyUpdatedAt: &at, Identity: w.Identity, Capabilities: muse.Capabilities{Models: []string{"muse/assistant"}}}
		require.NoError(t, store.SaveProfile(ctx, a, observation))
		_, e = store.Profile(ctx, a)
		require.NoError(t, e)
		_, e = db.ExecContext(ctx, `UPDATE proxies SET port=12346,updated_at=clock_timestamp() WHERE id=$1`, proxy.ID)
		require.NoError(t, e)
		_, e = store.Profile(ctx, a)
		require.ErrorIs(t, e, muse.ErrTransportUnqualified)
		e = store.RenewSession(ctx, a, func(context.Context) (map[string]any, error) { t.Fatal("stale proxy must not renew"); return nil, nil })
		require.ErrorIs(t, e, muse.ErrGeneration)
	})
	t.Run("ProfileRejectsStaleLoadedAccountAfterReverification", func(t *testing.T) {
		a := newAccount(t)
		w := bind(t, a)
		old := *a
		_, e := db.ExecContext(ctx, `UPDATE accounts SET updated_at=clock_timestamp()+INTERVAL '1 second' WHERE id=$1`, a.ID)
		require.NoError(t, e)
		require.NoError(t, db.QueryRowContext(ctx, `SELECT updated_at FROM accounts WHERE id=$1`, a.ID).Scan(&a.UpdatedAt))
		fresh := w.Identity
		fresh.AccountUpdatedAt = a.UpdatedAt
		_, e = runtime.BindVerifiedWorkspace(ctx, fresh)
		require.NoError(t, e)
		require.NoError(t, store.SaveProfile(ctx, a, &muse.Observation{InferenceAllowed: true, Identity: fresh, Capabilities: muse.Capabilities{Models: []string{"muse/assistant"}}}))
		_, e = store.Profile(ctx, &old)
		require.ErrorIs(t, e, muse.ErrTransportUnqualified)
		_, e = store.Profile(ctx, a)
		require.NoError(t, e)
	})
	t.Run("PriceMismatchRejectedBeforeSubmission", func(t *testing.T) {
		a := newAccount(t)
		w := bind(t, a)
		turn, _ := reserve(t, w)
		e := store.FreezeCharge(ctx, turn.ID, &service.UsageBillingCommand{RequestID: turn.ID, UserID: 1, APIKeyID: 1, AccountID: a.ID, AccountType: service.AccountTypeSession, Model: "muse/assistant", BalanceCost: 99}, &service.UsageLog{RequestID: turn.ID, UserID: 1, APIKeyID: 1, AccountID: a.ID, Model: "muse/assistant", TotalCost: 99, ActualCost: 99, RateMultiplier: 1.25})
		require.ErrorIs(t, e, muse.ErrInvalid)
	})
	t.Run("OwnerReviewMustResolveSameTurn", func(t *testing.T) {
		a := newAccount(t)
		w := bind(t, a)
		turn, lease := reserve(t, w)
		freeze(t, turn)
		require.NoError(t, runtime.BeginSubmission(ctx, lease))
		require.NoError(t, runtime.Advance(ctx, lease, muse.Submitting, muse.Ambiguous, ""))
		require.NoError(t, runtime.Advance(ctx, lease, muse.Ambiguous, muse.OwnerReview, ""))
		pending, e := store.PendingTurns(ctx, a.ID)
		require.NoError(t, e)
		require.Len(t, pending, 1)
		e = store.ResolveOwnerReview(ctx, turn.ID, muse.Actor{UserID: 2, APIKeyID: 1}, muse.Cancelled)
		require.ErrorIs(t, e, muse.ErrNotFound)
		require.NoError(t, store.ResolveOwnerReview(ctx, turn.ID, turn.Actor, muse.Cancelled))
		_, e = store.Settle(ctx, turn.ID)
		require.NoError(t, e)
		_, _ = reserve(t, w)
	})
}

func mustFixtureNumber(t *testing.T, s string) float64 {
	t.Helper()
	var value float64
	require.NoError(t, json.Unmarshal([]byte(s), &value))
	return value
}
