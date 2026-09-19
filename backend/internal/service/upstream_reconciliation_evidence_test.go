//go:build unit

package service

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/require"
)

func billingTestTokenPointer(n int64) *int64 { return &n }

func TestUpstreamReconciliationOfficialOutputPreservesOutsideUsage(t *testing.T) {
	usage := []billingUsageWeight{
		{AccountID: 1, APIKeyID: 11, UserID: 8, Requests: 1, Tokens: 120, LocalCost: "0.2", MatchedTokens: billingTestTokenPointer(100)},
		{AccountID: 2, APIKeyID: 12, UserID: 9, Requests: 2, Tokens: 490, LocalCost: "0.7", MatchedTokens: billingTestTokenPointer(400)},
	}
	for _, amount := range []string{"10", "-10"} {
		rows, local, err := allocateOfficialBillingAmount(amount, 1000, usage)
		require.NoError(t, err)
		require.Equal(t, int64(500), local)
		require.Len(t, rows, 3)
		require.Equal(t, "official_token_weighted", rows[0].Method)
		require.Equal(t, "0.2", rows[0].LocalCost, "token weights must not replace original list-price cost")
		require.Equal(t, "unmatched", rows[2].Method)
		require.Zero(t, rows[2].UserID, "outside-site usage is not attributed to any user")
		sign := ""
		if amount == "-10" {
			sign = "-"
		}
		require.Equal(t, sign+"1.00000000", rows[0].AllocatedCost)
		require.Equal(t, sign+"4.00000000", rows[1].AllocatedCost)
		require.Equal(t, sign+"5.00000000", rows[2].AllocatedCost)
	}
	rows, local, err := allocateOfficialBillingAmount("10", 200, usage)
	require.NoError(t, err)
	require.Nil(t, rows, "contradictory evidence must not manufacture an official proportional allocation")
	require.Equal(t, int64(500), local)
	rows, _, err = allocateOfficialBillingAmount("0.00000001", 1000, usage)
	require.NoError(t, err)
	sum := decimal.Zero
	for _, r := range rows {
		n, _ := decimal.NewFromString(r.AllocatedCost)
		sum = sum.Add(n)
	}
	require.Equal(t, "0.00000001", sum.StringFixed(8))
}

func TestUpstreamReconciliationTokenEvidenceIsConservative(t *testing.T) {
	base := ProviderBillUsage{Model: "claude-sonnet-4-6", TokenType: "output", Tokens: 1000, ContextWindow: "0-200k", ServiceTier: "standard", InferenceGeo: "not_available", Complete: true}
	for _, tc := range []struct {
		name, status string
		mutate       func(*ProviderBillUsage)
	}{
		{"supported-output", "verified", func(*ProviderBillUsage) {}},
		{"mutated-input", "local_category_unverified", func(u *ProviderBillUsage) { u.TokenType = "input" }},
		{"mutated-cache", "local_category_unverified", func(u *ProviderBillUsage) { u.TokenType = "cache_read" }},
		{"mutated-cache-ttl", "local_category_unverified", func(u *ProviderBillUsage) { u.TokenType = "cache_creation_1h" }},
		{"unknown-geo", "dimensions_unverified", func(u *ProviderBillUsage) { u.InferenceGeo = "global" }},
		{"us-geo", "dimensions_unverified", func(u *ProviderBillUsage) { u.InferenceGeo = "us" }},
		{"batch-tier", "dimensions_unverified", func(u *ProviderBillUsage) { u.ServiceTier = "batch" }},
		{"unknown-context", "dimensions_unverified", func(u *ProviderBillUsage) { u.ContextWindow = "" }},
		{"missing-model", "dimensions_unverified", func(u *ProviderBillUsage) { u.Model = "" }},
		{"partial-report", "incomplete", func(u *ProviderBillUsage) { u.Complete = false }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			u := base
			tc.mutate(&u)
			e, ok := billingEvidenceFor(ProviderBill{Usage: &u, UsageStatus: "available"})
			require.Equal(t, tc.status, e.TokenStatus)
			require.Equal(t, tc.status == "verified", ok)
		})
	}
	e, ok := billingEvidenceFor(ProviderBill{UsageStatus: "unavailable"})
	require.False(t, ok)
	require.Equal(t, "unavailable", e.TokenStatus)
}

func TestUpstreamReconciliationRawSnapshotsAreBoundedAndFingerprinted(t *testing.T) {
	b := ProviderBill{RawSource: json.RawMessage(` { "amount": "123.0000000001", "quantity": 12 } `), UsageStatus: "unsupported"}
	require.NoError(t, normalizeBillingEvidence(&b))
	require.JSONEq(t, `{"amount":"123.0000000001","quantity":12}`, string(b.RawSource))
	snapshot := []billingBillSnapshot{{Bill: b}}
	first, err := billingSnapshotFingerprint(nil, snapshot)
	require.NoError(t, err)
	snapshot[0].Bill.RawSource = json.RawMessage(`{"amount":"124"}`)
	second, err := billingSnapshotFingerprint(nil, snapshot)
	require.NoError(t, err)
	require.NotEqual(t, first, second, "raw source changes must remain auditable")
	snapshot[0].Evidence = billingUsageEvidence{TokenStatus: "verified", LocalMatchedTokens: billingTestTokenPointer(12)}
	third, err := billingSnapshotFingerprint(nil, snapshot)
	require.NoError(t, err)
	require.NotEqual(t, second, third, "comparison evidence is part of immutable revision identity")
	b.RawSource = json.RawMessage(`{"value":"` + strings.Repeat("x", 65536) + `"}`)
	require.Error(t, normalizeBillingEvidence(&b))
	b.RawSource = json.RawMessage(`[]`)
	require.Error(t, normalizeBillingEvidence(&b))
}

func TestUpstreamReconciliationRawSourceSurvivesJSONBWhitespace(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()
	compact := `{"quantities":[` + strings.Repeat("0,", 26000) + `0]}`
	expanded := strings.ReplaceAll(compact, ",", ", ")
	require.Less(t, len(compact), 65536)
	require.Greater(t, len(expanded), 65536)
	start := time.Date(2026, 9, 1, 0, 0, 0, 0, time.UTC)
	mock.ExpectQuery("SELECT b.id,b.provider_bill_id").WithArgs(int64(9)).WillReturnRows(sqlmock.NewRows([]string{"id", "provider_bill_id", "provider", "description", "source_amount", "currency", "period_start", "period_end", "raw_source"}).AddRow(9, "line", "tencent", "Original cost row", "1.234567891", "CNY", start, start.AddDate(0, 1, 0), expanded))
	s := &UpstreamReconciliationService{db: db}
	result, err := s.RawBillSource(context.Background(), 9)
	require.NoError(t, err)
	require.JSONEq(t, compact, string(result.RawSource))
	require.Equal(t, "1.234567891", result.SourceAmount)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestUpstreamReconciliationRejectsOverlappingImports(t *testing.T) {
	for _, overlaps := range []bool{false, true} {
		db, mock, err := sqlmock.New()
		require.NoError(t, err)
		mock.ExpectBegin()
		tx, err := db.Begin()
		require.NoError(t, err)
		mock.ExpectQuery("SELECT pg_try_advisory_xact_lock").WithArgs("sub2api:upstream-billing:import:anthropic").WillReturnRows(sqlmock.NewRows([]string{"locked"}).AddRow(true))
		mock.ExpectQuery("(?s)SELECT EXISTS.*b.period_start<incoming.period_end AND b.period_end>incoming.period_start.*m.connection_id<>\\$2").WithArgs("anthropic", int64(7), sqlmock.AnyArg()).WillReturnRows(sqlmock.NewRows([]string{"exists"}).AddRow(overlaps))
		mock.ExpectRollback()
		start := time.Date(2026, 9, 1, 0, 0, 0, 0, time.UTC)
		err = checkBillingImportOverlap(context.Background(), tx, &BillingConnection{ID: 7, Provider: "anthropic"}, []ProviderBill{{ResourceID: "__organization__", PeriodStart: start, PeriodEnd: start.AddDate(0, 1, 0)}})
		if overlaps {
			require.Error(t, err)
			require.Contains(t, err.Error(), "OVERLAPPING_SCOPE")
		} else {
			require.NoError(t, err)
		}
		require.NoError(t, tx.Rollback())
		require.NoError(t, mock.ExpectationsWereMet())
		db.Close()
	}
}

func TestUpstreamReconciliationComparableUsageMatchesExactDimensions(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()
	mock.ExpectBegin()
	tx, err := db.Begin()
	require.NoError(t, err)
	start := time.Date(2026, 9, 1, 0, 0, 0, 0, time.UTC)
	b := ProviderBill{PeriodStart: start, PeriodEnd: start.AddDate(0, 0, 1), Usage: &ProviderBillUsage{Model: "claude-sonnet-exact", ContextWindow: "0-200k"}}
	mock.ExpectQuery("(?s)SELECT .*u.upstream_response_model.*u.service_tier.*CASE WHEN").WithArgs("{1}", start, b.PeriodEnd, "claude-sonnet-exact", "0-200k", billingMaxAllocations+1).WillReturnRows(sqlmock.NewRows([]string{"account_id", "account_name", "api_key_id", "api_key_name", "user_id", "requests", "tokens", "local_cost", "matched_tokens"}).AddRow(1, "upstream", 2, "key", 3, 4, 99, "0.1", 30))
	rows, err := loadComparableBillingUsage(context.Background(), tx, []int64{1}, b)
	require.NoError(t, err)
	require.Len(t, rows, 1)
	require.Equal(t, int64(30), *rows[0].MatchedTokens)
	mock.ExpectRollback()
	require.NoError(t, tx.Rollback())
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestUpstreamReconciliationUsageOutageCannotIncreaseSiteShare(t *testing.T) {
	for _, previouslyReconciled := range []bool{false, true} {
		db, mock, err := sqlmock.New()
		require.NoError(t, err)
		mock.ExpectBegin()
		tx, err := db.Begin()
		require.NoError(t, err)
		mock.ExpectQuery("(?s)SELECT EXISTS.*a.official_tokens IS NOT NULL").WithArgs(int64(5), "2026-09", `{"same-invoice"}`).WillReturnRows(sqlmock.NewRows([]string{"exists"}).AddRow(previouslyReconciled))
		err = checkBillingEvidenceRegression(context.Background(), tx, 5, "2026-09", []ProviderBill{{ID: "same-invoice", UsageStatus: "unavailable"}})
		if previouslyReconciled {
			require.Error(t, err)
			require.Contains(t, err.Error(), "previous reconciled snapshot")
		} else {
			require.NoError(t, err, "first-time bills remain available even without optional token evidence")
		}
		mock.ExpectRollback()
		require.NoError(t, tx.Rollback())
		require.NoError(t, mock.ExpectationsWereMet())
		db.Close()
	}
}
