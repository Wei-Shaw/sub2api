//go:build integration

package repository

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/stretchr/testify/require"
)

func TestListDueUpstreamBillingProbeAccountsHandlesInvalidCalendarDate(t *testing.T) {
	ctx := context.Background()
	tx := testEntTx(t)
	repo := newAccountRepositoryWithSQL(tx.Client(), tx, nil)
	now := time.Date(2026, time.July, 14, 12, 0, 0, 0, time.UTC)
	_, err := tx.ExecContext(ctx, `
		UPDATE accounts
		SET extra = extra - 'upstream_billing_probe_enabled' - 'upstream_billing_probe'
	`)
	require.NoError(t, err)

	insert := func(name, nextProbeAt string) int64 {
		t.Helper()
		var id int64
		extra := fmt.Sprintf(`{
			"upstream_billing_probe_enabled": true,
			"upstream_billing_probe": {"status": "ok", "next_probe_at": %q}
		}`, nextProbeAt)
		err := scanSingleRow(ctx, tx, `
			INSERT INTO accounts (name, platform, type, status, extra)
			VALUES ($1, 'openai', $2, 'active', $3::jsonb)
			RETURNING id
		`, []any{name, service.AccountTypeAPIKey, extra}, &id)
		require.NoError(t, err)
		return id
	}

	invalidID := insert("probe-invalid-calendar-date", "2026-99-99T12:00:00Z")
	dueID := insert("probe-due", "2026-07-14T11:59:59Z")
	_ = insert("probe-not-due", "2026-07-14T12:00:01Z")

	accounts, err := repo.ListDueUpstreamBillingProbeAccounts(ctx, now, 20)
	require.NoError(t, err)
	require.Len(t, accounts, 2)
	require.Equal(t, invalidID, accounts[0].ID)
	require.Equal(t, dueID, accounts[1].ID)
}

func insertUpstreamBillingProbeAccount(ctx context.Context, t *testing.T, tx sqlQueryer, name, nextProbeAt string) int64 {
	t.Helper()
	var id int64
	extra := fmt.Sprintf(`{
		"upstream_billing_probe_enabled": true,
		"upstream_billing_probe": {"status": "ok", "next_probe_at": %q}
	}`, nextProbeAt)
	err := scanSingleRow(ctx, tx, `
		INSERT INTO accounts (name, platform, type, status, extra)
		VALUES ($1, 'openai', $2, 'active', $3::jsonb)
		RETURNING id
	`, []any{name, service.AccountTypeAPIKey, extra}, &id)
	require.NoError(t, err)
	return id
}

// rfc3339WithFraction renders t in UTC with an explicit fractional-second
// suffix, e.g. "2026-07-25T17:29:00.123456789Z". The probes' Go writer uses
// RFC3339Nano, so stored fractions carry up to 9 digits.
func rfc3339WithFraction(t time.Time, fraction string) string {
	return fmt.Sprintf("%s.%sZ", t.UTC().Format("2006-01-02T15:04:05"), fraction)
}

// Regression for the scheduled-probe starvation bug: Go persists next_probe_at
// via RFC3339Nano (7-9 fractional digits), which jsonpath datetime() cannot
// parse. Before the trim fix every such row was treated as malformed and
// fail-open "due", so the cycle always returned the lowest account IDs and
// higher IDs were never probed.
func TestListDueUpstreamBillingProbeAccountsParsesNanosecondTimestamps(t *testing.T) {
	ctx := context.Background()
	tx := testEntTx(t)
	repo := newAccountRepositoryWithSQL(tx.Client(), tx, nil)
	now := time.Date(2026, time.July, 25, 17, 30, 0, 0, time.UTC)
	_, err := tx.ExecContext(ctx, `
		UPDATE accounts
		SET extra = extra - 'upstream_billing_probe_enabled' - 'upstream_billing_probe'
	`)
	require.NoError(t, err)

	// 22 low-ID accounts that are NOT due yet, stored with 9 fractional digits
	// exactly as the legacy writer produced them (no migration).
	notDue := now.Add(time.Hour)
	for i := 0; i < 22; i++ {
		insertUpstreamBillingProbeAccount(ctx, t, tx,
			fmt.Sprintf("probe-nano-not-due-%02d", i),
			rfc3339WithFraction(notDue.Add(time.Duration(i)*time.Second), "123456789"))
	}
	// 3 high-ID accounts that ARE due, with 9/8/7 fractional digits. Their
	// parsed order must follow due time, not insertion/ID order.
	dueThird := insertUpstreamBillingProbeAccount(ctx, t, tx,
		"probe-nano-due-9digits", rfc3339WithFraction(now.Add(-time.Minute), "123456789"))
	dueFirst := insertUpstreamBillingProbeAccount(ctx, t, tx,
		"probe-nano-due-8digits", rfc3339WithFraction(now.Add(-3*time.Minute), "12345678"))
	dueSecond := insertUpstreamBillingProbeAccount(ctx, t, tx,
		"probe-nano-due-7digits", rfc3339WithFraction(now.Add(-2*time.Minute), "1234567"))

	accounts, err := repo.ListDueUpstreamBillingProbeAccounts(ctx, now, 20)
	require.NoError(t, err)
	require.Len(t, accounts, 3)
	require.Equal(t, dueFirst, accounts[0].ID)
	require.Equal(t, dueSecond, accounts[1].ID)
	require.Equal(t, dueThird, accounts[2].ID)
}

// With more due accounts than the cycle limit, selection must follow the
// parsed due time so accounts beyond the first <limit> IDs still get their
// turn; before the fix the same lowest IDs monopolized every cycle.
func TestListDueUpstreamBillingProbeAccountsSelectsEarliestDueAcrossIDs(t *testing.T) {
	ctx := context.Background()
	tx := testEntTx(t)
	repo := newAccountRepositoryWithSQL(tx.Client(), tx, nil)
	now := time.Date(2026, time.July, 25, 17, 30, 0, 0, time.UTC)
	_, err := tx.ExecContext(ctx, `
		UPDATE accounts
		SET extra = extra - 'upstream_billing_probe_enabled' - 'upstream_billing_probe'
	`)
	require.NoError(t, err)

	// 25 due accounts; the LOWEST IDs carry the LATEST due times, so a
	// correct query must pick the 20 highest-ID rows here.
	ids := make([]int64, 0, 25)
	for i := 0; i < 25; i++ {
		due := now.Add(-time.Duration(i+1) * time.Minute)
		ids = append(ids, insertUpstreamBillingProbeAccount(ctx, t, tx,
			fmt.Sprintf("probe-nano-order-%02d", i),
			rfc3339WithFraction(due, "123456789")))
	}

	accounts, err := repo.ListDueUpstreamBillingProbeAccounts(ctx, now, 20)
	require.NoError(t, err)
	require.Len(t, accounts, 20)
	got := make([]int64, 0, len(accounts))
	for _, account := range accounts {
		got = append(got, account.ID)
	}
	// Earliest due first == highest index first; the five most recently due
	// (lowest index / lowest IDs) fall outside the limit this cycle.
	want := make([]int64, 0, 20)
	for i := 24; i >= 5; i-- {
		want = append(want, ids[i])
	}
	require.Equal(t, want, got)
}
