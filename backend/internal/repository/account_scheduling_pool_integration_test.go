//go:build integration

package repository

import (
	"context"
	"fmt"
	"strconv"
	"sync"
	"testing"
	"time"

	dbent "github.com/Wei-Shaw/sub2api/ent"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/lib/pq"
	"github.com/stretchr/testify/require"
)

type schedulingPoolFixture struct {
	t       *testing.T
	ctx     context.Context
	client  *dbent.Client
	repo    service.AccountSchedulingPoolRepository
	proxy   *service.Proxy
	scope   string
	ids     []int64
	poolIDs []int64
}

func newSchedulingPoolFixture(t *testing.T, limit int) *schedulingPoolFixture {
	t.Helper()
	f := &schedulingPoolFixture{t: t, ctx: context.Background(), client: testEntClient(t), scope: fmt.Sprintf("pool-test-%d", time.Now().UnixNano())}
	f.repo = NewAccountSchedulingPoolRepository(f.client, integrationDB)
	f.proxy = mustCreateProxy(t, f.client, &service.Proxy{Name: f.scope, Host: f.scope + ".invalid", Port: 3128})
	settings := NewSettingRepository(f.client)
	previous, previousErr := settings.GetValue(f.ctx, service.AccountSchedulingPolicyKey)
	require.True(t, previousErr == nil || previousErr == service.ErrSettingNotFound)
	t.Cleanup(func() {
		if previousErr == nil {
			_ = settings.Set(f.ctx, service.AccountSchedulingPolicyKey, previous)
		} else {
			_, _ = integrationDB.ExecContext(f.ctx, "DELETE FROM settings WHERE key=$1", service.AccountSchedulingPolicyKey)
		}
		_, _ = integrationDB.ExecContext(f.ctx, "DELETE FROM account_scheduling_slots WHERE exit_group=$1 OR account_id=ANY($2::bigint[])", f.scope, pq.Array(f.ids))
		_, _ = integrationDB.ExecContext(f.ctx, "DELETE FROM scheduler_outbox WHERE account_id=ANY($1::bigint[])", pq.Array(f.ids))
		_, _ = integrationDB.ExecContext(f.ctx, "DELETE FROM accounts WHERE id=ANY($1::bigint[])", pq.Array(f.ids))
		_, _ = integrationDB.ExecContext(f.ctx, "DELETE FROM account_scheduling_pools WHERE id=ANY($1::bigint[])", pq.Array(f.poolIDs))
		_, _ = integrationDB.ExecContext(f.ctx, "DELETE FROM proxies WHERE id=$1", f.proxy.ID)
	})
	require.NoError(t, f.repo.UpdatePolicy(f.ctx, service.AccountSchedulingPolicy{Enabled: true, MaxActiveAccounts: limit, ProxyExitGroups: map[string]string{strconv.FormatInt(f.proxy.ID, 10): f.scope}}))
	return f
}

func (f *schedulingPoolFixture) account() *service.Account {
	f.t.Helper()
	a := mustCreateAccount(f.t, f.client, &service.Account{Name: fmt.Sprintf("%s-%d", f.scope, len(f.ids)), Platform: service.PlatformOpenAI, ProxyID: &f.proxy.ID, Credentials: map[string]any{"plan_type": "free", "chatgpt_account_id": fmt.Sprintf("%s-%d", f.scope, len(f.ids))}})
	f.ids = append(f.ids, a.ID)
	return a
}

func (f *schedulingPoolFixture) pool(enabled bool) *service.AccountSchedulingPool {
	f.t.Helper()
	p := &service.AccountSchedulingPool{Name: fmt.Sprintf("%s-pool-%d", f.scope, len(f.poolIDs)), Enabled: enabled}
	require.NoError(f.t, f.repo.Create(f.ctx, p))
	f.poolIDs = append(f.poolIDs, p.ID)
	return p
}

func (f *schedulingPoolFixture) reconcile() *service.AccountSchedulingDecision {
	f.t.Helper()
	d, err := f.repo.Reconcile(f.ctx, service.AccountSchedulingAdmission{})
	require.NoError(f.t, err)
	return d
}

func (f *schedulingPoolFixture) acquire(id int64, lease string) *service.AccountSchedulingDecision {
	f.t.Helper()
	d, err := f.repo.Reconcile(f.ctx, service.AccountSchedulingAdmission{AccountID: id, ActualProxyURL: f.proxy.URL(), LeaseID: lease, LeaseTTL: time.Minute})
	require.NoError(f.t, err)
	return d
}

func TestAccountSchedulingPoolConcurrentSharedExitAndRestart(t *testing.T) {
	f := newSchedulingPoolFixture(t, 3)
	a, b, c, d := f.account(), f.account(), f.account(), f.account()
	p1, p2 := f.pool(true), f.pool(true)
	require.NoError(t, f.repo.SetMembers(f.ctx, &p1.ID, []int64{a.ID, b.ID}))
	require.NoError(t, f.repo.SetMembers(f.ctx, &p2.ID, []int64{c.ID}))
	// The fourth account is outside both pools but shares their exit budget.
	ids := []int64{a.ID, b.ID, c.ID, d.ID}
	start := make(chan struct{})
	var wg sync.WaitGroup
	var mu sync.Mutex
	acquired := map[int64]int{}
	var failures []error
	for n := 0; n < 24; n++ {
		wg.Add(1)
		go func(n int) {
			defer wg.Done()
			<-start
			id := ids[n%len(ids)]
			// Separate repository objects model admission by different processes.
			repo := NewAccountSchedulingPoolRepository(f.client, integrationDB)
			decision, err := repo.Reconcile(f.ctx, service.AccountSchedulingAdmission{AccountID: id, ActualProxyURL: f.proxy.URL(), LeaseID: fmt.Sprintf("%s-concurrent-%d", f.scope, n), LeaseTTL: time.Minute})
			mu.Lock()
			defer mu.Unlock()
			if err != nil {
				failures = append(failures, err)
			} else if decision.Acquired {
				acquired[id]++
			}
		}(n)
	}
	close(start)
	wg.Wait()
	require.Empty(t, failures)
	require.Len(t, acquired, 3, "different pools and unpooled accounts must share one identity budget")
	var identities int
	require.NoError(t, integrationDB.QueryRowContext(f.ctx, `SELECT COUNT(DISTINCT l.identity) FROM account_scheduling_leases l JOIN account_scheduling_slots s USING(identity) WHERE s.exit_group=$1`, f.scope).Scan(&identities))
	require.Equal(t, 3, identities)
	before := f.reconcile()
	f.repo = NewAccountSchedulingPoolRepository(f.client, integrationDB)
	after := f.reconcile()
	for _, id := range ids {
		require.Equal(t, before.Allowed[id], after.Allowed[id], "recreating the repository must retain the active set")
	}
}

func TestAccountSchedulingPoolDrainsLeaseAndRecoversAtTail(t *testing.T) {
	f := newSchedulingPoolFixture(t, 1)
	a, b, c := f.account(), f.account(), f.account()
	leaseID := f.scope + "-stream"
	require.True(t, f.acquire(a.ID, leaseID).Acquired)
	now := time.Now()
	extra := map[string]any{"codex_7d_used_percent": 100, "codex_7d_window_minutes": 43200, "codex_7d_reset_at": now.Add(20 * 24 * time.Hour).Format(time.RFC3339), "codex_usage_updated_at": now.Format(time.RFC3339)}
	require.NoError(t, f.client.Account.UpdateOneID(a.ID).SetExtra(extra).Exec(f.ctx))
	d := f.reconcile()
	require.False(t, d.Allowed[a.ID])
	require.False(t, d.Allowed[b.ID], "quota exhaustion cannot replace an identity whose stream is still open")
	require.NoError(t, f.repo.ReleaseLease(f.ctx, leaseID))
	d = f.reconcile()
	require.True(t, d.Allowed[b.ID])
	extra["codex_7d_reset_at"] = now.Add(-time.Minute).Format(time.RFC3339)
	require.NoError(t, f.client.Account.UpdateOneID(a.ID).SetExtra(extra).Exec(f.ctx))
	d = f.reconcile()
	require.False(t, d.Allowed[a.ID], "recovered accounts do not preempt an active identity")
	require.NoError(t, f.client.Account.UpdateOneID(b.ID).SetSchedulable(false).Exec(f.ctx))
	d = f.reconcile()
	require.True(t, d.Allowed[c.ID], "an existing reserve precedes a recovered identity")
	require.False(t, d.Allowed[a.ID])
}

func TestAccountSchedulingPoolFreshAdmissionAndExpiry(t *testing.T) {
	f := newSchedulingPoolFixture(t, 1)
	a, b := f.account(), f.account()
	leaseID := f.scope + "-expires"
	require.True(t, f.acquire(a.ID, leaseID).Acquired)
	ok, err := f.repo.RefreshLease(f.ctx, leaseID, 2*time.Minute)
	require.NoError(t, err)
	require.True(t, ok)
	require.NoError(t, f.client.Account.UpdateOneID(a.ID).SetSchedulable(false).Exec(f.ctx))
	require.False(t, f.acquire(a.ID, f.scope+"-disabled").Acquired)
	require.False(t, f.reconcile().Allowed[b.ID])
	_, err = integrationDB.ExecContext(f.ctx, "UPDATE account_scheduling_leases SET expires_at=clock_timestamp()-INTERVAL '1 second' WHERE lease_id=$1", leaseID)
	require.NoError(t, err)
	ok, err = f.repo.RefreshLease(f.ctx, leaseID, time.Minute)
	require.NoError(t, err)
	require.False(t, ok, "an expired lease must not be revived")
	require.True(t, f.reconcile().Allowed[b.ID])
	_, err = f.repo.Reconcile(f.ctx, service.AccountSchedulingAdmission{AccountID: b.ID, ActualProxyURL: "", LeaseID: f.scope + "-stale-route", LeaseTTL: time.Minute})
	require.ErrorIs(t, err, service.ErrAccountSchedulingUnavailable)
	_, err = f.repo.Reconcile(f.ctx, service.AccountSchedulingAdmission{AccountID: 9223372036854775807, LeaseID: f.scope + "-missing", LeaseTTL: time.Minute})
	require.ErrorIs(t, err, service.ErrAccountSchedulingUnavailable)
	require.NoError(t, f.client.Account.DeleteOneID(b.ID).Exec(f.ctx))
	_, err = f.repo.Reconcile(f.ctx, service.AccountSchedulingAdmission{AccountID: b.ID, ActualProxyURL: f.proxy.URL(), LeaseID: f.scope + "-deleted", LeaseTTL: time.Minute})
	require.ErrorIs(t, err, service.ErrAccountSchedulingUnavailable)
}

func TestAccountSchedulingPoolImportAtomicityAndFilter(t *testing.T) {
	f := newSchedulingPoolFixture(t, 1)
	p := f.pool(false)
	accounts := newAccountRepositoryWithSQL(f.client, integrationDB, nil)
	a := &service.Account{Name: f.scope + "-import", Platform: service.PlatformOpenAI, Type: service.AccountTypeOAuth, Status: service.StatusActive, Schedulable: true, ProxyID: &f.proxy.ID, SchedulingPoolID: &p.ID, Credentials: map[string]any{"plan_type": "free", "email": f.scope + "@example.invalid"}, Extra: map[string]any{"crs_account_id": f.scope}}
	require.NoError(t, accounts.Create(f.ctx, a))
	f.ids = append(f.ids, a.ID)
	var poolID int64
	require.NoError(t, integrationDB.QueryRowContext(f.ctx, "SELECT pool_id FROM account_scheduling_members WHERE account_id=$1", a.ID).Scan(&poolID))
	require.Equal(t, p.ID, poolID)
	fromCRS, err := accounts.GetByCRSAccountID(f.ctx, f.scope)
	require.NoError(t, err)
	require.NotNil(t, fromCRS.SchedulingPoolID, "CRS lookup must retain local pool management")
	require.Equal(t, p.ID, *fromCRS.SchedulingPoolID)
	require.False(t, f.acquire(a.ID, f.scope+"-disabled-pool").Acquired)
	rows, err := accounts.ListAllWithFilters(service.WithAccountSchedulingPoolFilter(f.ctx, &p.ID), "", "", "", "", 0, "")
	require.NoError(t, err)
	require.Len(t, rows, 1)
	require.Equal(t, a.ID, rows[0].ID)
	ungrouped := int64(0)
	rows, err = accounts.ListAllWithFilters(service.WithAccountSchedulingPoolFilter(f.ctx, &ungrouped), "", "", "", f.scope, 0, "")
	require.NoError(t, err)
	require.Empty(t, rows)
	missingPool := int64(9223372036854775807)
	failed := *a
	failed.ID, failed.Name, failed.SchedulingPoolID = 0, f.scope+"-failed-import", &missingPool
	require.ErrorIs(t, accounts.Create(f.ctx, &failed), service.ErrAccountSchedulingPoolNotFound)
	var count int
	require.NoError(t, integrationDB.QueryRowContext(f.ctx, "SELECT COUNT(*) FROM accounts WHERE name=$1", failed.Name).Scan(&count))
	require.Zero(t, count, "failed membership must roll back the account insert")
	require.NoError(t, integrationDB.QueryRowContext(f.ctx, "SELECT COUNT(*) FROM scheduler_outbox WHERE account_id=$1", failed.ID).Scan(&count))
	require.Zero(t, count)
	require.ErrorIs(t, f.repo.Delete(f.ctx, p.ID), service.ErrAccountSchedulingPoolNotEmpty)
	require.NoError(t, f.client.Account.DeleteOneID(a.ID).Exec(f.ctx))
	require.NoError(t, f.repo.Delete(f.ctx, p.ID), "a soft-deleted member must not prevent deleting a visibly empty pool")
}

func TestAccountSchedulingPoolExplicitReorderAndPaidVisibility(t *testing.T) {
	f := newSchedulingPoolFixture(t, 1)
	a, b, c := f.account(), f.account(), f.account()
	p := f.pool(true)
	require.NoError(t, f.repo.SetMembers(f.ctx, &p.ID, []int64{a.ID, b.ID, c.ID}))
	require.True(t, f.reconcile().Allowed[a.ID])
	// Simulate an older reserve and a later recovered reserve before manual reorder.
	_, err := integrationDB.ExecContext(f.ctx, "UPDATE account_scheduling_slots SET queued_at=queued_at-INTERVAL '1 day' WHERE account_id=$1", b.ID)
	require.NoError(t, err)
	require.NoError(t, f.repo.SetMembers(f.ctx, &p.ID, []int64{a.ID, c.ID, b.ID}))
	require.NoError(t, f.client.Account.UpdateOneID(a.ID).SetSchedulable(false).Exec(f.ctx))
	d := f.reconcile()
	require.True(t, d.Allowed[c.ID], "manual reorder must override old queue timestamps")
	require.False(t, d.Allowed[b.ID])
	require.NoError(t, f.client.Account.UpdateOneID(b.ID).SetCredentials(map[string]any{"plan_type": "plus", "email": f.scope + "-paid@example.invalid"}).Exec(f.ctx))
	d = f.reconcile()
	var paid *service.AccountSchedulingMember
	for i := range d.Members {
		if d.Members[i].AccountID == b.ID {
			paid = &d.Members[i]
		}
	}
	require.NotNil(t, paid)
	require.Equal(t, "unmanaged", paid.State)
	require.Equal(t, "not_free_tier", paid.Reason)
}

func TestAccountSchedulingPoolExitRelabelRequiresDrainedRequests(t *testing.T) {
	f := newSchedulingPoolFixture(t, 3)
	accounts := []*service.Account{f.account(), f.account(), f.account(), f.account()}
	leases := []string{f.scope + "-live-1", f.scope + "-live-2", f.scope + "-live-3"}
	for i, id := range leases {
		require.True(t, f.acquire(accounts[i].ID, id).Acquired)
	}
	key := strconv.FormatInt(f.proxy.ID, 10)
	relabel := service.AccountSchedulingPolicy{Enabled: true, MaxActiveAccounts: 3, ProxyExitGroups: map[string]string{key: f.scope + "-renamed"}}
	require.ErrorIs(t, f.repo.UpdatePolicy(f.ctx, relabel), service.ErrAccountSchedulingExitInUse)
	require.False(t, f.acquire(accounts[3].ID, f.scope+"-fourth").Acquired, "same proxy URL must not acquire a second budget after relabeling")
	var raw string
	require.NoError(t, integrationDB.QueryRowContext(f.ctx, "SELECT value FROM settings WHERE key=$1", service.AccountSchedulingPolicyKey).Scan(&raw))
	require.Contains(t, raw, `"`+key+`":"`+f.scope+`"`, "rejected relabel must leave the old mapping intact")
	// Existing requests do not block emergency disabling or lowering the limit.
	unchangedScope := service.AccountSchedulingPolicy{Enabled: false, MaxActiveAccounts: 1, ProxyExitGroups: map[string]string{key: f.scope}}
	require.NoError(t, f.repo.UpdatePolicy(f.ctx, unchangedScope))
	unchangedScope.Enabled = true
	require.NoError(t, f.repo.UpdatePolicy(f.ctx, unchangedScope))
	for _, id := range leases {
		require.NoError(t, f.repo.ReleaseLease(f.ctx, id))
	}
	relabel.MaxActiveAccounts = 1
	require.NoError(t, f.repo.UpdatePolicy(f.ctx, relabel))
	decision := f.reconcile()
	active := 0
	for _, a := range accounts {
		if decision.Allowed[a.ID] {
			active++
		}
	}
	require.Equal(t, 1, active, "after draining a relabeled exit has exactly one shared budget")
}
