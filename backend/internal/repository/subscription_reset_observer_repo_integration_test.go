//go:build integration

package repository

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
)

type resetObserverFixture struct {
	r     service.SubscriptionResetObserverRepository
	p     *service.SubscriptionResetPolicy
	ids   []int64
	reset time.Time
}

func newResetObserverFixture(t *testing.T, count int) *resetObserverFixture {
	t.Helper()
	ctx, client := context.Background(), testEntClient(t)
	g := mustCreateGroup(t, client, &service.Group{Name: "reset-observer-" + uuid.NewString(), Platform: service.PlatformOpenAI, SubscriptionType: service.SubscriptionTypeSubscription})
	f := &resetObserverFixture{r: NewSubscriptionResetObserverRepository(integrationDB), p: service.DefaultSubscriptionResetPolicy(g.ID), reset: time.Now().UTC().Add(-time.Minute).Truncate(time.Microsecond)}
	t.Cleanup(func() {
		for _, id := range f.ids {
			_, err := integrationDB.Exec(`DELETE FROM accounts WHERE id=$1`, id)
			require.NoError(t, err)
		}
		_, err := integrationDB.Exec(`DELETE FROM groups WHERE id=$1`, g.ID)
		require.NoError(t, err)
	})
	for i := 0; i < count; i++ {
		a := mustCreateAccount(t, client, &service.Account{Name: "reset-observer-" + uuid.NewString(), Platform: service.PlatformOpenAI})
		f.ids = append(f.ids, a.ID)
		mustBindAccountToGroup(t, client, a.ID, g.ID, 50)
	}
	f.p.Mode, f.p.AccountIDs = "observe", append([]int64{}, f.ids...)
	var err error
	f.p, err = f.r.SavePolicy(ctx, f.p)
	require.NoError(t, err)
	// Keep samples in the recent past without sleeps. Production saves use the
	// real DB clock; this fixture only backdates the empty policy baseline.
	f.p.UpdatedAt = f.reset.Add(-2 * time.Minute)
	raw, err := json.Marshal(f.p)
	require.NoError(t, err)
	_, err = integrationDB.Exec(`UPDATE subscription_reset_policies SET policy=$2::jsonb,updated_at=$3 WHERE group_id=$1`, g.ID, string(raw), f.p.UpdatedAt)
	require.NoError(t, err)
	return f
}

func (f *resetObserverFixture) sample(index, subject int, at, reset time.Time, used float64) *service.SubscriptionResetSample {
	minutes := 10080
	return &service.SubscriptionResetSample{AccountID: f.ids[index], ObservedAt: at, Subject: &service.SubscriptionResetQuotaSubject{UserID: fmt.Sprintf("user-%d", subject), AccountID: "workspace", Scope: "codex:global"}, UsedPercent: &used, ResetAt: &reset, WindowMinutes: &minutes, PlanType: "plus"}
}

func (f *resetObserverFixture) record(t *testing.T, s *service.SubscriptionResetSample) {
	t.Helper()
	require.NoError(t, f.r.RecordSample(context.Background(), f.p.GroupID, f.p.Version, s))
}

func (f *resetObserverFixture) status(t *testing.T) *service.SubscriptionResetStatus {
	t.Helper()
	s, err := f.r.GetStatus(context.Background(), f.p.GroupID, 20)
	require.NoError(t, err)
	return s
}

func TestSubscriptionResetObserverRepositoryVersionAndReconfiguration(t *testing.T) {
	f := newResetObserverFixture(t, 2)
	ctx := context.Background()
	got, err := f.r.GetPolicy(ctx, f.p.GroupID)
	require.NoError(t, err)
	require.Equal(t, f.p, got)
	enabled, err := f.r.ListEnabled(ctx)
	require.NoError(t, err)
	require.Contains(t, func() []int64 {
		ids := []int64{}
		for _, p := range enabled {
			ids = append(ids, p.GroupID)
		}
		return ids
	}(), f.p.GroupID)
	for i := range f.ids {
		f.record(t, f.sample(i, i, f.reset.Add(-time.Minute), f.reset, 99))
	}
	f.record(t, f.sample(0, 0, f.reset.Add(time.Second), f.reset.Add(7*24*time.Hour), 0))
	require.Equal(t, "pending", f.status(t).Events[0].Status)
	old := *f.p
	f.p, err = f.r.SavePolicy(ctx, f.p)
	require.NoError(t, err)
	require.Equal(t, old.Version+1, f.p.Version)
	s := f.status(t)
	require.Equal(t, "config_changed", s.Events[0].Status)
	require.Equal(t, "waiting_baseline", s.Accounts[0].State)
	require.Nil(t, s.LastObservedAt)
	_, err = f.r.SavePolicy(ctx, &old)
	require.ErrorIs(t, err, service.ErrSubscriptionResetPolicyConflict)
	err = f.r.RecordSample(ctx, f.p.GroupID, old.Version, f.sample(1, 1, time.Now(), f.reset.Add(7*24*time.Hour), 0))
	require.ErrorIs(t, err, service.ErrSubscriptionResetPolicyConflict)
	// Even a newly labelled response cannot resurrect a sample begun before save.
	f.record(t, f.sample(1, 1, f.reset.Add(2*time.Second), f.reset.Add(7*24*time.Hour), 0))
	require.Nil(t, f.status(t).LastObservedAt)
}

func TestSubscriptionResetObserverRepositoryRejectsChangedPlatform(t *testing.T) {
	f := newResetObserverFixture(t, 1)
	_, err := integrationDB.Exec(`UPDATE groups SET platform='anthropic' WHERE id=$1`, f.p.GroupID)
	require.NoError(t, err)
	_, err = f.r.SavePolicy(context.Background(), f.p)
	require.Error(t, err)
	err = f.r.RecordSample(context.Background(), f.p.GroupID, f.p.Version, f.sample(0, 0, time.Now(), time.Now().Add(7*24*time.Hour), 0))
	require.ErrorIs(t, err, service.ErrGroupNotFound)
	enabled, err := f.r.ListEnabled(context.Background())
	require.NoError(t, err)
	for _, p := range enabled {
		require.NotEqual(t, f.p.GroupID, p.GroupID)
	}
	require.Nil(t, f.status(t).LastObservedAt)
}

func TestSubscriptionResetObserverRepositoryConcurrentReplayAndFrozenSubjects(t *testing.T) {
	f := newResetObserverFixture(t, 3)
	for i, subject := range []int{0, 1, 0} {
		f.record(t, f.sample(i, subject, f.reset.Add(-time.Minute), f.reset, 99))
	}
	sample := f.sample(0, 0, f.reset.Add(time.Second), f.reset.Add(7*24*time.Hour), 0)
	r2 := NewSubscriptionResetObserverRepository(integrationDB)
	var wg sync.WaitGroup
	errs := make(chan error, 12)
	for i := 0; i < 12; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			r := f.r
			if i%2 == 0 {
				r = r2
			}
			errs <- r.RecordSample(context.Background(), f.p.GroupID, f.p.Version, sample)
		}(i)
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		require.NoError(t, err)
	}
	s := f.status(t)
	require.Len(t, s.Events, 1)
	require.Equal(t, 2, s.Events[0].Denominator)
	require.Equal(t, 1, s.Events[0].ConfirmedCount)
	require.Equal(t, f.ids[0], *s.Accounts[2].DuplicateOf)
	f.record(t, f.sample(2, 0, f.reset.Add(2*time.Second), f.reset.Add(7*24*time.Hour), 0))
	require.Equal(t, 1, f.status(t).Events[0].ConfirmedCount)
	f.record(t, f.sample(1, 1, f.reset.Add(3*time.Second), f.reset.Add(7*24*time.Hour), 0))
	s = f.status(t)
	require.Equal(t, "confirmed", s.Events[0].Status)
	confirmed := *s.Events[0].ConfirmedAt
	// New repository instance reloads state and cannot create a replay event.
	f.r = NewSubscriptionResetObserverRepository(integrationDB)
	f.record(t, sample)
	s = f.status(t)
	require.Len(t, s.Events, 1)
	require.True(t, confirmed.Equal(*s.Events[0].ConfirmedAt))
}

func TestSubscriptionResetObserverRepositoryEventAndStateRollbackTogether(t *testing.T) {
	f := newResetObserverFixture(t, 2)
	for i := range f.ids {
		f.record(t, f.sample(i, i, f.reset.Add(-time.Minute), f.reset, 100))
	}
	var before string
	require.NoError(t, integrationDB.QueryRow(`SELECT state::text FROM subscription_reset_policies WHERE group_id=$1`, f.p.GroupID).Scan(&before))
	// A group-specific CHECK forces an actual event write failure after the
	// policy state UPDATE; both must roll back in the repository transaction.
	constraint := fmt.Sprintf("observer_test_reject_%d", f.p.GroupID)
	_, err := integrationDB.Exec(fmt.Sprintf(`ALTER TABLE subscription_reset_events ADD CONSTRAINT %s CHECK (group_id <> %d)`, constraint, f.p.GroupID))
	require.NoError(t, err)
	t.Cleanup(func() {
		_, _ = integrationDB.Exec(`ALTER TABLE subscription_reset_events DROP CONSTRAINT IF EXISTS ` + constraint)
	})
	sample := f.sample(0, 0, f.reset.Add(time.Second), f.reset.Add(7*24*time.Hour), 0)
	require.Error(t, f.r.RecordSample(context.Background(), f.p.GroupID, f.p.Version, sample))
	var after string
	require.NoError(t, integrationDB.QueryRow(`SELECT state::text FROM subscription_reset_policies WHERE group_id=$1`, f.p.GroupID).Scan(&after))
	require.Equal(t, before, after)
	require.Empty(t, f.status(t).Events)
	_, err = integrationDB.Exec(`ALTER TABLE subscription_reset_events DROP CONSTRAINT ` + constraint)
	require.NoError(t, err)
	f.record(t, sample)
	require.Len(t, f.status(t).Events, 1)
}

func TestSubscriptionResetObserverRepositoryRechecksMarkerMembershipAndEligibility(t *testing.T) {
	f := newResetObserverFixture(t, 4)
	for i := range f.ids {
		f.record(t, f.sample(i, i, f.reset.Add(-time.Minute), f.reset, 100))
	}
	f.record(t, f.sample(0, 0, f.reset.Add(time.Second), f.reset.Add(7*24*time.Hour), 0))
	marker := f.reset.Add(-time.Second)
	_, err := integrationDB.Exec(`UPDATE accounts SET extra=extra || jsonb_build_object('codex_history_reset_at',$2::text) WHERE id=$1`, f.ids[1], marker.Format(time.RFC3339Nano))
	require.NoError(t, err)
	_, err = integrationDB.Exec(`DELETE FROM account_groups WHERE account_id=$1 AND group_id=$2`, f.ids[2], f.p.GroupID)
	require.NoError(t, err)
	_, err = integrationDB.Exec(`UPDATE accounts SET credentials=credentials || '{"auth_mode":"agent_identity"}'::jsonb WHERE id=$1`, f.ids[3])
	require.NoError(t, err)
	for i := 1; i < 4; i++ {
		f.record(t, f.sample(i, i, f.reset.Add(2*time.Second), f.reset.Add(7*24*time.Hour), 0))
	}
	s := f.status(t)
	require.Equal(t, 4, s.Events[0].Denominator)
	require.Equal(t, 1, s.Events[0].ConfirmedCount)
	require.Equal(t, "local_reset", s.Accounts[1].State)
	require.Equal(t, "account_removed", s.Accounts[2].Reason)
	require.Equal(t, "account_ineligible", s.Accounts[3].Reason)
	// A marker committed after the upstream GET prevents even a new baseline.
	marker = f.reset.Add(20 * time.Second)
	_, err = integrationDB.Exec(`UPDATE accounts SET extra=extra || jsonb_build_object('codex_history_reset_at',$2::text) WHERE id=$1`, f.ids[1], marker.Format(time.RFC3339Nano))
	require.NoError(t, err)
	f.record(t, f.sample(1, 1, f.reset.Add(3*time.Second), f.reset.Add(7*24*time.Hour), 0))
	var baselineNil bool
	require.NoError(t, integrationDB.QueryRow(`SELECT state->'accounts'->$2->'baseline' = 'null'::jsonb FROM subscription_reset_policies WHERE group_id=$1`, f.p.GroupID, fmt.Sprint(f.ids[1])).Scan(&baselineNil))
	require.True(t, baselineNil)
}

func TestSubscriptionResetObserverRepositoryUnknownDenominatorAndNoSubscriptionWrites(t *testing.T) {
	f := newResetObserverFixture(t, 3)
	client := testEntClient(t)
	u := mustCreateUser(t, client, &service.User{Email: "reset-observer-" + uuid.NewString() + "@example.com"})
	sub := mustCreateSubscription(t, client, &service.UserSubscription{UserID: u.ID, GroupID: f.p.GroupID, DailyUsageUSD: 11, WeeklyUsageUSD: 22, MonthlyUsageUSD: 33})
	t.Cleanup(func() {
		_, err := integrationDB.Exec(`DELETE FROM user_subscriptions WHERE id=$1`, sub.ID)
		require.NoError(t, err)
		_, err = integrationDB.Exec(`DELETE FROM users WHERE id=$1`, u.ID)
		require.NoError(t, err)
	})
	var before string
	require.NoError(t, integrationDB.QueryRow(`SELECT row_to_json(s)::text FROM user_subscriptions s WHERE id=$1`, sub.ID).Scan(&before))
	observationID := insertWindowObservation(t, f.ids[0], f.reset, f.reset.Add(7*24*time.Hour), 100)
	for i := 0; i < 2; i++ {
		f.record(t, f.sample(i, i, f.reset.Add(-time.Minute), f.reset, 100))
	}
	f.record(t, f.sample(0, 0, f.reset.Add(time.Second), f.reset.Add(7*24*time.Hour), 0))
	f.record(t, f.sample(1, 1, f.reset.Add(2*time.Second), f.reset.Add(7*24*time.Hour), 0))
	// Discovering the third subject later does not silently remove/replace its
	// unknown slot in the already frozen denominator.
	f.record(t, f.sample(2, 2, f.reset.Add(3*time.Second), f.reset.Add(7*24*time.Hour), 0))
	s := f.status(t)
	require.Equal(t, 3, s.Events[0].Denominator)
	require.Equal(t, 3, s.Events[0].RequiredCount)
	require.Equal(t, 2, s.Events[0].ConfirmedCount)
	require.Equal(t, "pending", s.Events[0].Status)
	var after string
	require.NoError(t, integrationDB.QueryRow(`SELECT row_to_json(s)::text FROM user_subscriptions s WHERE id=$1`, sub.ID).Scan(&after))
	require.Equal(t, before, after, "observation must not alter expiry, counters, windows or status")
	var unprocessed bool
	require.NoError(t, integrationDB.QueryRow(`SELECT processed_at IS NULL FROM account_quota_observations WHERE id=$1`, observationID).Scan(&unprocessed))
	require.True(t, unprocessed, "observer must not consume the history worker's journal")
}

func TestSubscriptionResetObserverRepositoryStatusBoundsAndOffMode(t *testing.T) {
	f := newResetObserverFixture(t, 1)
	for i := 0; i < 110; i++ {
		at := f.reset.Add(time.Duration(i) * time.Millisecond)
		e := service.SubscriptionResetEvent{ID: uuid.NewString(), GroupID: f.p.GroupID, PolicyVersion: f.p.Version, Source: "7d", Kind: "early_drop", Status: "needs_review", OpenedAt: at, UpdatedAt: at, Members: []service.SubscriptionResetEventMember{}}
		raw, err := json.Marshal(e)
		require.NoError(t, err)
		_, err = integrationDB.Exec(`INSERT INTO subscription_reset_events(id,group_id,policy_version,opened_at,updated_at,payload) VALUES($1,$2,$3,$4,$4,$5::jsonb)`, e.ID, e.GroupID, e.PolicyVersion, at, string(raw))
		require.NoError(t, err)
	}
	s, err := f.r.GetStatus(context.Background(), f.p.GroupID, 1000)
	require.NoError(t, err)
	require.Len(t, s.Events, 100)
	require.True(t, s.Events[0].OpenedAt.After(s.Events[99].OpenedAt))
	f.p.Mode = "off"
	f.p, err = f.r.SavePolicy(context.Background(), f.p)
	require.NoError(t, err)
	f.record(t, f.sample(0, 0, time.Now(), time.Now().Add(7*24*time.Hour), 0))
	s = f.status(t)
	require.True(t, s.ObservationOnly)
	require.False(t, s.Ready)
	require.Nil(t, s.LastObservedAt)
	require.Nil(t, s.ActiveEvent)
}
