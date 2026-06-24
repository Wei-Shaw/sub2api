//go:build unit

package repository

import (
	"context"
	"database/sql"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/stretchr/testify/require"
	_ "modernc.org/sqlite"
)

func newTrafficPackTestDB(t *testing.T) *sql.DB {
	t.Helper()
	db, err := sql.Open("sqlite", ":memory:")
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })

	for _, stmt := range []string{
		`CREATE TABLE traffic_packs (
			id INTEGER PRIMARY KEY,
			code TEXT NOT NULL UNIQUE,
			name TEXT NOT NULL,
			description TEXT NOT NULL DEFAULT '',
			price REAL NOT NULL,
			credit_usd REAL NOT NULL,
			validity_days INTEGER NOT NULL,
			platform TEXT NOT NULL,
			for_sale BOOLEAN NOT NULL,
			sort_order INTEGER NOT NULL DEFAULT 0,
			created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
			updated_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
		)`,
		`CREATE TABLE user_traffic_credits (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			user_id INTEGER NOT NULL,
			order_id INTEGER NOT NULL UNIQUE,
			pack_id INTEGER,
			platform TEXT NOT NULL,
			initial_usd REAL NOT NULL,
			remaining_usd REAL NOT NULL,
			credited_at TIMESTAMP NOT NULL,
			expires_at TIMESTAMP NOT NULL,
			created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
			updated_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
		)`,
		`CREATE TABLE traffic_credit_ledger (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			user_id INTEGER NOT NULL,
			credit_id INTEGER,
			order_id INTEGER,
			request_id TEXT NOT NULL DEFAULT '',
			entry_type TEXT NOT NULL,
			amount_usd REAL NOT NULL,
			balance_after_usd REAL NOT NULL,
			created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
		)`,
	} {
		_, err := db.Exec(stmt)
		require.NoError(t, err)
	}
	return db
}

func TestTrafficPackRepository_CreditPurchaseIsIdempotentAndSummarizes(t *testing.T) {
	ctx := context.Background()
	db := newTrafficPackTestDB(t)
	repo := NewTrafficPackRepository(db)
	now := time.Date(2026, 6, 22, 12, 0, 0, 0, time.UTC)

	input := service.CreditTrafficPackInput{
		UserID:       7,
		OrderID:      1001,
		PackID:       3,
		CreditUSD:    20,
		ValidityDays: 365,
		CreditedAt:   now,
	}

	require.NoError(t, repo.CreditPurchase(ctx, input))
	require.NoError(t, repo.CreditPurchase(ctx, input))

	var creditCount int
	require.NoError(t, db.QueryRow(`SELECT COUNT(*) FROM user_traffic_credits WHERE user_id = 7`).Scan(&creditCount))
	require.Equal(t, 1, creditCount)

	summary, err := repo.GetSummary(ctx, 7, now)
	require.NoError(t, err)
	require.InDelta(t, 20, summary.TotalRemainingUSD, 0.000001)
	require.InDelta(t, 20, summary.NextExpiringUSD, 0.000001)
	require.NotNil(t, summary.NextExpiresAt)
	require.Equal(t, now.AddDate(0, 0, 365), *summary.NextExpiresAt)
}

func TestTrafficPackRepository_DeductConsumesEarliestExpiringCredits(t *testing.T) {
	ctx := context.Background()
	db := newTrafficPackTestDB(t)
	repo := NewTrafficPackRepository(db)
	now := time.Date(2026, 6, 22, 12, 0, 0, 0, time.UTC)

	for _, input := range []service.CreditTrafficPackInput{
		{UserID: 9, OrderID: 2002, PackID: 2, CreditUSD: 10, ValidityDays: 365, CreditedAt: now.Add(24 * time.Hour)},
		{UserID: 9, OrderID: 2001, PackID: 1, CreditUSD: 5, ValidityDays: 365, CreditedAt: now},
	} {
		require.NoError(t, repo.CreditPurchase(ctx, input))
	}

	covered, deductions, err := repo.Deduct(ctx, 9, 7, "req-traffic-1", now.Add(48*time.Hour))

	require.NoError(t, err)
	require.True(t, covered)
	require.Len(t, deductions, 2)
	require.InDelta(t, 5, deductions[0].AmountUSD, 0.000001)
	require.InDelta(t, 2, deductions[1].AmountUSD, 0.000001)

	summary, err := repo.GetSummary(ctx, 9, now.Add(48*time.Hour))
	require.NoError(t, err)
	require.InDelta(t, 8, summary.TotalRemainingUSD, 0.000001)
}
