//go:build unit

package service

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/stretchr/testify/require"
)

func TestUpstreamReconciliationPolicyDefaultsPreservationAndValidation(t *testing.T) {
	lookback, mode, err := resolveBillingPolicy(SaveBillingConnectionInput{}, nil)
	require.NoError(t, err)
	require.Equal(t, 2, lookback)
	require.Equal(t, "local_weighted", mode)
	previous := &BillingConnection{SyncLookbackMonths: 5, AllocationMode: "official_only"}
	lookback, mode, err = resolveBillingPolicy(SaveBillingConnectionInput{}, previous)
	require.NoError(t, err)
	require.Equal(t, 5, lookback)
	require.Equal(t, "official_only", mode)
	count := 1
	lookback, mode, err = resolveBillingPolicy(SaveBillingConnectionInput{SyncLookbackMonths: &count, AllocationMode: "disabled"}, previous)
	require.NoError(t, err)
	require.Equal(t, 1, lookback)
	require.Equal(t, "disabled", mode)
	for _, count := range []int{-1, 0, 7, 100} {
		_, _, err = resolveBillingPolicy(SaveBillingConnectionInput{SyncLookbackMonths: &count}, nil)
		require.Error(t, err)
	}
	_, _, err = resolveBillingPolicy(SaveBillingConnectionInput{AllocationMode: "provider_said_so"}, nil)
	require.Error(t, err)
}

func TestUpstreamReconciliationScheduledMonthsUseProviderCalendar(t *testing.T) {
	now := time.Date(2026, 3, 31, 23, 30, 0, 0, time.UTC)
	require.Equal(t, []string{"2026-04", "2026-03", "2026-02", "2026-01", "2025-12", "2025-11"}, billingScheduledMonths(now, billingLocation("tencent"), 6))
	require.Equal(t, []string{"2026-03", "2026-02"}, billingScheduledMonths(now, time.UTC, 2))
	require.Equal(t, []string{"2026-03"}, billingScheduledMonths(now, time.UTC, 1))
	for _, count := range []int{0, -1, 7} {
		require.Len(t, billingScheduledMonths(now, time.UTC, count), 2)
	}
}

func TestUpstreamReconciliationAllocationPolicyChangesSnapshot(t *testing.T) {
	legacy, err := billingSnapshotFingerprint(nil, nil)
	require.NoError(t, err)
	standard, err := billingSnapshotFingerprint(nil, nil, "local_weighted")
	require.NoError(t, err)
	require.Equal(t, legacy, standard)
	strict, err := billingSnapshotFingerprint(nil, nil, "official_only")
	require.NoError(t, err)
	disabled, err := billingSnapshotFingerprint(nil, nil, "disabled")
	require.NoError(t, err)
	require.NotEqual(t, standard, strict)
	require.NotEqual(t, strict, disabled)
}

func TestUpstreamReconciliationDomesticScopeSurvivesCredentialRotation(t *testing.T) {
	for _, provider := range []string{"aliyun", "volcengine"} {
		a := BillingProviderConfig{Provider: provider, Settings: map[string]string{"account_id": "12345678", "product_code": "verified-product"}, Secrets: map[string]string{"access_key_id": "old", "access_key_secret": "old"}}
		b := BillingProviderConfig{Provider: provider, Settings: map[string]string{"account_id": "12345678", "product_code": "verified-product"}, Secrets: map[string]string{"access_key_id": "new", "access_key_secret": "new"}}
		require.Equal(t, billingScopeKey(a), billingScopeKey(b))
		b.Settings["account_id"] = "87654321"
		require.NotEqual(t, billingScopeKey(a), billingScopeKey(b))
		now := time.Date(2026, 9, 18, 0, 0, 0, 0, time.UTC)
		start, end, err := billingMonth("2026-09", now, billingLocation(provider))
		require.NoError(t, err)
		require.Equal(t, "2026-08-31T16:00:00Z", start.UTC().Format(time.RFC3339))
		require.Equal(t, "2026-09-30T16:00:00Z", end.UTC().Format(time.RFC3339))
	}
}

func TestUpstreamReconciliationStrictPoliciesDoNotDistributeUnverifiedCosts(t *testing.T) {
	for _, mode := range []string{"official_only", "disabled"} {
		t.Run(mode, func(t *testing.T) {
			db, mock, err := sqlmock.New()
			require.NoError(t, err)
			defer db.Close()
			conn, err := db.Conn(context.Background())
			require.NoError(t, err)
			defer conn.Close()
			c := &BillingConnection{ID: 9, Provider: "tencent", AllocationMode: mode, Bindings: []BillingBinding{{ResourceID: "resource", AccountID: 1}}}
			start := time.Date(2026, 9, 1, 0, 0, 0, 0, time.UTC)
			end := start.AddDate(0, 1, 0)
			bill := ProviderBill{ID: "line", ResourceID: "resource", Amount: "-2.500000001", Currency: "CNY", PeriodStart: start, PeriodEnd: end}
			mock.ExpectBegin()
			mock.ExpectQuery("SELECT pg_try_advisory_xact_lock").WillReturnRows(sqlmock.NewRows([]string{"locked"}).AddRow(true))
			mock.ExpectQuery("SELECT EXISTS").WillReturnRows(sqlmock.NewRows([]string{"exists"}).AddRow(false))
			mock.ExpectQuery("SELECT DISTINCT ON").WillReturnRows(sqlmock.NewRows([]string{"resource", "start", "end", "account_id", "account_name", "api_key_id", "api_key_name", "user_id", "requests", "tokens", "local_cost"}))
			// No usage_logs read is expected, despite an existing resource binding.
			mock.ExpectQuery("SELECT r.fingerprint").WillReturnRows(sqlmock.NewRows([]string{"fingerprint"}))
			mock.ExpectQuery("INSERT INTO upstream_billing_runs").WithArgs(int64(9), "2026-09", sqlmock.AnyArg(), sqlmock.AnyArg(), mode).WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow(2))
			mock.ExpectQuery("INSERT INTO upstream_billing_bills").WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow(3))
			status := "unsupported"
			if mode == "disabled" {
				status = "not_checked"
			}
			mock.ExpectExec("INSERT INTO upstream_billing_allocations").WithArgs(int64(3), int64(0), "", int64(0), "", int64(0), int64(0), int64(0), "0", "-2.50000000", "unmatched", nil, nil, status).WillReturnResult(sqlmock.NewResult(1, 1))
			mock.ExpectExec("INSERT INTO upstream_billing_months").WillReturnResult(sqlmock.NewResult(1, 1))
			mock.ExpectCommit()
			s := &UpstreamReconciliationService{db: db}
			require.NoError(t, s.persistBills(context.Background(), conn, c, "2026-09", start, end, []ProviderBill{bill}))
			require.NoError(t, mock.ExpectationsWereMet())
		})
	}
}

func TestUpstreamReconciliationOldClientPreservesSavedStrictPolicy(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()
	enc := &billingTestEncryptor{plaintext: `{"secret_id":"id","secret_key":"secret"}`}
	s := &UpstreamReconciliationService{db: db, encryptor: enc, persistentKey: true}
	input := SaveBillingConnectionInput{Name: "Updated", Provider: "tencent", Settings: map[string]string{"business_code": "verified"}, Enabled: true, SyncIntervalHours: 24}
	mock.ExpectBegin()
	mock.ExpectQuery("SELECT pg_try_advisory_xact_lock").WillReturnRows(sqlmock.NewRows([]string{"locked"}).AddRow(true))
	mock.ExpectQuery("SELECT pg_try_advisory_xact_lock").WillReturnRows(sqlmock.NewRows([]string{"locked"}).AddRow(true))
	mock.ExpectQuery("SELECT provider,encrypted_secrets,sync_lookback_months,allocation_mode").WithArgs(int64(7)).WillReturnRows(sqlmock.NewRows([]string{"provider", "encrypted_secrets", "sync_lookback_months", "allocation_mode", "settings"}).AddRow("tencent", "sealed", 5, "official_only", `{"business_code":"verified"}`))
	mock.ExpectQuery("SELECT EXISTS").WillReturnRows(sqlmock.NewRows([]string{"exists"}).AddRow(false))
	mock.ExpectExec("UPDATE upstream_billing_connections").WithArgs(int64(7), "Updated", `{"business_code":"verified"}`, "sealed-secret", true, 24, sqlmock.AnyArg(), 5, "official_only").WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec("DELETE FROM upstream_billing_bindings").WithArgs(int64(7)).WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectQuery("SELECT id,name,provider").WillReturnRows(sqlmock.NewRows([]string{"id", "name", "provider", "settings", "enabled", "sync_interval_hours", "last_synced_at", "next_sync_at", "last_error", "has_credentials", "sync_lookback_months", "allocation_mode"}).AddRow(7, "Updated", "tencent", `{"business_code":"verified"}`, true, 24, nil, time.Now(), "", true, 5, "official_only"))
	mock.ExpectQuery("SELECT resource_id,account_id").WillReturnRows(sqlmock.NewRows([]string{"resource_id", "account_id"}))
	mock.ExpectCommit()
	saved, err := s.SaveConnection(context.Background(), 7, 1, input)
	require.NoError(t, err)
	require.Equal(t, 5, saved.SyncLookbackMonths)
	require.Equal(t, "official_only", saved.AllocationMode)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestUpstreamReconciliationAdminBillAmountsRemainLineSpecific(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()
	start := time.Date(2026, 9, 1, 0, 0, 0, 0, time.UTC)
	end := start.AddDate(0, 1, 0)
	s := &UpstreamReconciliationService{db: db, now: func() time.Time { return end }}
	mock.ExpectBegin()
	mock.ExpectQuery("SELECT c.id").WillReturnRows(sqlmock.NewRows([]string{"connection_id", "connection_name", "provider", "resource_id", "account_id", "account_name", "api_key_id", "api_key_name", "user_id", "requests", "tokens", "local_cost", "allocated_cost", "currency", "method", "period_start", "period_end", "official_tokens", "local_matched_tokens", "token_status"}))
	mock.ExpectQuery("SELECT b.provider_bill_id.*LEFT JOIN LATERAL").WithArgs("2026-09", billingMaxBills+1).WillReturnRows(sqlmock.NewRows([]string{"id", "connection_id", "resource_id", "description", "amount", "source_amount", "currency", "period_start", "period_end", "source_id", "has_raw_source", "usage_evidence", "allocated_cost", "unmatched_cost"}).AddRow("refund", 1, "resource", "refund", "-0.00000002", "-0.000000021", "CNY", start, end, 9, true, `{"usage_status":"unsupported","token_status":"unsupported"}`, "-0.00000001", "-0.00000001"))
	mock.ExpectCommit()
	report, err := s.Report(context.Background(), "2026-09", 1, true)
	require.NoError(t, err)
	require.Len(t, report.Bills, 1)
	require.Equal(t, "-0.00000001", report.Bills[0].AllocatedCost)
	require.Equal(t, "-0.00000001", report.Bills[0].UnmatchedCost)
	require.Equal(t, "-0.000000021", report.Bills[0].SourceAmount)
	encoded, err := json.Marshal(report)
	require.NoError(t, err)
	require.NotContains(t, string(encoded), `"raw_source":`)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestUpstreamReconciliationOfficialConflictNeverFallsBackToFullLocalEstimate(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()
	conn, err := db.Conn(context.Background())
	require.NoError(t, err)
	defer conn.Close()
	c := &BillingConnection{ID: 9, Provider: "anthropic", AllocationMode: "local_weighted", Bindings: []BillingBinding{{ResourceID: "workspace", AccountID: 1}}}
	start := time.Date(2026, 9, 1, 0, 0, 0, 0, time.UTC)
	end := start.AddDate(0, 0, 1)
	bill := ProviderBill{ID: "line", ResourceID: "workspace", Amount: "5", Currency: "USD", PeriodStart: start, PeriodEnd: end, UsageStatus: "available", Usage: &ProviderBillUsage{Model: "claude-exact", TokenType: "output", Tokens: 100, ContextWindow: "0-200k", ServiceTier: "standard", InferenceGeo: "not_available", Complete: true}}
	mock.ExpectBegin()
	mock.ExpectQuery("SELECT pg_try_advisory_xact_lock").WillReturnRows(sqlmock.NewRows([]string{"locked"}).AddRow(true))
	mock.ExpectQuery("SELECT EXISTS").WillReturnRows(sqlmock.NewRows([]string{"exists"}).AddRow(false))
	priorColumns := []string{"resource", "start", "end", "account_id", "account_name", "api_key_id", "api_key_name", "user_id", "requests", "tokens", "local_cost"}
	mock.ExpectQuery("SELECT DISTINCT ON").WillReturnRows(sqlmock.NewRows(priorColumns))
	usageColumns := []string{"account_id", "account_name", "api_key_id", "api_key_name", "user_id", "requests", "tokens", "local_cost", "matched_tokens"}
	mock.ExpectQuery("SELECT u.account_id,COALESCE").WillReturnRows(sqlmock.NewRows(usageColumns).AddRow(1, "upstream", 2, "key", 3, 1, 101, "4", 101))
	mock.ExpectQuery("SELECT DISTINCT ON").WillReturnRows(sqlmock.NewRows(usageColumns))
	// A second/general usage_logs query would mean an unsafe estimation fallback.
	mock.ExpectQuery("SELECT r.fingerprint").WillReturnRows(sqlmock.NewRows([]string{"fingerprint"}))
	mock.ExpectQuery("INSERT INTO upstream_billing_runs").WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow(2))
	mock.ExpectQuery("INSERT INTO upstream_billing_bills").WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow(3))
	mock.ExpectExec("INSERT INTO upstream_billing_allocations").WithArgs(int64(3), int64(0), "", int64(0), "", int64(0), int64(0), int64(0), "0", "5.00000000", "unmatched", int64(100), nil, "local_exceeds_official").WillReturnResult(sqlmock.NewResult(1, 1))
	mock.ExpectExec("INSERT INTO upstream_billing_months").WillReturnResult(sqlmock.NewResult(1, 1))
	mock.ExpectCommit()
	s := &UpstreamReconciliationService{db: db}
	require.NoError(t, s.persistBills(context.Background(), conn, c, "2026-09", start, start.AddDate(0, 1, 0), []ProviderBill{bill}))
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestUpstreamReconciliationDomesticHistoricalScopeCannotBeReassigned(t *testing.T) {
	previous := map[string]string{"account_id": "12345678", "product_code": "product"}
	for _, provider := range []string{"aliyun", "volcengine"} {
		require.NoError(t, validateBillingScopeEdit(provider, previous, previous))
		require.Error(t, validateBillingScopeEdit(provider, previous, map[string]string{"account_id": "87654321", "product_code": "product"}))
		require.Error(t, validateBillingScopeEdit(provider, previous, map[string]string{"account_id": "12345678", "product_code": "other"}))
	}
}
