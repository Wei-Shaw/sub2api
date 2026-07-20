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
