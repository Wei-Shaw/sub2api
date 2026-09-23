//go:build unit

package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/require"
)

func TestUpstreamReconciliationAllocationConservation(t *testing.T) {
	for _, amount := range []string{"1", "-1", "0.00000001", "-0.00000001", "12345678.123456789", "0"} {
		t.Run(amount, func(t *testing.T) {
			usage := make([]billingUsageWeight, 997)
			for i := range usage {
				usage[i] = billingUsageWeight{AccountID: 1, APIKeyID: int64(i + 1), UserID: 2, Requests: 1, Tokens: 10, LocalCost: "1"}
			}
			allocations, err := allocateBillingAmount(amount, usage)
			require.NoError(t, err)
			require.Len(t, allocations, len(usage))
			sum := decimal.Zero
			for _, a := range allocations {
				n, e := decimal.NewFromString(a.AllocatedCost)
				require.NoError(t, e)
				require.Equal(t, "local_cost_weighted", a.Method)
				if strings.HasPrefix(amount, "-") {
					require.False(t, n.IsPositive())
				} else {
					require.False(t, n.IsNegative())
				}
				sum = sum.Add(n)
			}
			expected, _ := decimal.NewFromString(amount)
			require.True(t, sum.Equal(expected.Round(8)), "allocated %s versus bill %s", sum, expected)
		})
	}
}

func TestUpstreamReconciliationWeightsAndUnmatched(t *testing.T) {
	weighted, err := allocateBillingAmount("10", []billingUsageWeight{{LocalCost: "3", Tokens: 1}, {LocalCost: "1", Tokens: 100}})
	require.NoError(t, err)
	require.Equal(t, "7.50000000", weighted[0].AllocatedCost)
	require.Equal(t, "2.50000000", weighted[1].AllocatedCost)
	fallback, err := allocateBillingAmount("10", []billingUsageWeight{{LocalCost: "0", Tokens: 3}, {LocalCost: "0", Tokens: 1}})
	require.NoError(t, err)
	require.Equal(t, "token_weighted", fallback[0].Method)
	require.Equal(t, "7.50000000", fallback[0].AllocatedCost)
	unmatched, err := allocateBillingAmount("-2.5", nil)
	require.NoError(t, err)
	require.Len(t, unmatched, 1)
	require.Equal(t, "unmatched", unmatched[0].Method)
	require.Equal(t, "-2.50000000", unmatched[0].AllocatedCost)
	require.Zero(t, unmatched[0].UserID)
}

func TestUpstreamReconciliationTotalsNeverMixCurrencies(t *testing.T) {
	items := []BillingReportItem{{Currency: "USD", AllocatedCost: "2", Method: "local_cost_weighted"}, {Currency: "CNY", AllocatedCost: "7", Method: "token_weighted"}, {Currency: "USD", AllocatedCost: "-0.1", Method: "unmatched"}}
	bills := []BillingReportBill{{Currency: "USD", Amount: "1.9"}, {Currency: "CNY", Amount: "7"}}
	totals, err := billingReportTotals(items, bills, true)
	require.NoError(t, err)
	require.Equal(t, []BillingReportTotal{{Currency: "CNY", OfficialCost: "7.00000000", AllocatedCost: "7.00000000", UnmatchedCost: "0.00000000"}, {Currency: "USD", OfficialCost: "1.90000000", AllocatedCost: "2.00000000", UnmatchedCost: "-0.10000000"}}, totals)
	userTotals, err := billingReportTotals(items[:1], bills, false)
	require.NoError(t, err)
	require.Equal(t, []BillingReportTotal{{Currency: "USD", AllocatedCost: "2.00000000"}}, userTotals)
}

func TestUpstreamReconciliationPeriodsAndBillValidation(t *testing.T) {
	now := time.Date(2026, 9, 18, 12, 0, 0, 0, time.UTC)
	start, end, err := billingMonth("2026-09", now, billingLocation("tencent"))
	require.NoError(t, err)
	require.Equal(t, "2026-08-31T16:00:00Z", start.UTC().Format(time.RFC3339))
	require.Equal(t, "2026-09-30T16:00:00Z", end.UTC().Format(time.RFC3339))
	for _, month := range []string{"2026-9", "2026-13", "2026-10", "1999-01", "2026-09junk"} {
		_, _, err = billingMonth(month, now, time.UTC)
		require.Error(t, err)
	}
	valid := ProviderBill{ID: "line1", ResourceID: "resource1", Amount: "1.123456789", Currency: "cny", PeriodStart: start, PeriodEnd: end}
	bills, err := normalizeBillingBills([]ProviderBill{valid}, start, end)
	require.NoError(t, err)
	require.Equal(t, "CNY", bills[0].Currency)
	require.Equal(t, "1.123456789", bills[0].Amount, "source precision must survive normalization")
	_, err = normalizeBillingBills([]ProviderBill{valid, valid}, start, end)
	require.Error(t, err)
	bad := valid
	bad.PeriodEnd = end.Add(time.Nanosecond)
	_, err = normalizeBillingBills([]ProviderBill{bad}, start, end)
	require.Error(t, err)
	bad = valid
	bad.Amount = "NaN"
	_, err = normalizeBillingBills([]ProviderBill{bad}, start, end)
	require.Error(t, err)
	bad = valid
	bad.Amount = "1e2000000000"
	_, err = normalizeBillingBills([]ProviderBill{bad}, start, end)
	require.Error(t, err)
}

func TestUpstreamReconciliationUsageRetentionAndBindingChanges(t *testing.T) {
	previous := []billingUsageWeight{{AccountID: 1, APIKeyID: 2, UserID: 3, Requests: 10, Tokens: 100, LocalCost: "2"}, {AccountID: 4, APIKeyID: 5, UserID: 3, Requests: 3, Tokens: 30, LocalCost: "1"}}
	current := []billingUsageWeight{{AccountID: 1, APIKeyID: 2, UserID: 3, Requests: 4, Tokens: 40, LocalCost: "0.8"}}
	kept := preserveBillingUsage(current, previous, []int64{1})
	require.Len(t, kept, 1, "removed account bindings must not retain their allocations")
	require.Equal(t, int64(10), kept[0].Requests, "pruned logs must not erase saved allocation weights")
	current[0].Requests = 11
	kept = preserveBillingUsage(current, previous, []int64{1})
	require.Equal(t, int64(11), kept[0].Requests)
}

type billingTestEncryptor struct {
	plaintext    string
	decryptCalls int
	decryptError bool
}

func (e *billingTestEncryptor) Encrypt(plaintext string) (string, error) {
	e.plaintext = plaintext
	return "sealed-secret", nil
}
func (e *billingTestEncryptor) Decrypt(string) (string, error) {
	e.decryptCalls++
	if e.decryptError {
		return "", errors.New("sensitive ciphertext detail")
	}
	return e.plaintext, nil
}

func TestUpstreamReconciliationCredentialRecoveryAndPreservation(t *testing.T) {
	enc := &billingTestEncryptor{plaintext: `{"secret_id":"old_id","secret_key":"old_key"}`}
	s := &UpstreamReconciliationService{encryptor: enc, persistentKey: true}
	input := SaveBillingConnectionInput{Provider: "tencent", Settings: map[string]string{"business_code": "hunyuan"}, Secrets: map[string]string{"secret_id": "", "secret_key": "new_key"}}
	secrets, err := s.resolveBillingSecrets("ciphertext", input)
	require.NoError(t, err)
	require.Equal(t, map[string]string{"secret_id": "old_id", "secret_key": "new_key"}, secrets)
	require.Equal(t, 1, enc.decryptCalls)
	enc.decryptError = true
	input.Secrets = map[string]string{"secret_id": "replacement_id", "secret_key": "replacement_key"}
	secrets, err = s.resolveBillingSecrets("unreadable", input)
	require.NoError(t, err)
	require.Equal(t, input.Secrets, secrets)
	require.Equal(t, 1, enc.decryptCalls, "full credential replacement must not require the old encryption key")
	input.Secrets = map[string]string{"secret_id": ""}
	_, err = s.resolveBillingSecrets("unreadable", input)
	require.ErrorIs(t, err, ErrBillingCredentials)
	require.NotContains(t, err.Error(), "sensitive")
	s.persistentKey = false
	_, err = s.SaveConnection(context.Background(), 0, 1, input)
	require.ErrorIs(t, err, ErrBillingEncryption)
}

func TestUpstreamReconciliationScopeFingerprint(t *testing.T) {
	a := BillingProviderConfig{Provider: "azure", Settings: map[string]string{"resource_id": "/SUBSCRIPTIONS/S/RESOURCEGROUPS/R/PROVIDERS/MICROSOFT.COGNITIVESERVICES/ACCOUNTS/A"}, Secrets: map[string]string{"client_secret": "one"}}
	b := BillingProviderConfig{Provider: "azure", Settings: map[string]string{"resource_id": strings.ToLower(a.Settings["resource_id"])}, Secrets: map[string]string{"client_secret": "rotated"}}
	require.Equal(t, billingScopeKey(a), billingScopeKey(b))
	a = BillingProviderConfig{Provider: "tencent", Settings: map[string]string{"business_code": "hunyuan"}, Secrets: map[string]string{"secret_id": "id", "secret_key": "one"}}
	b = BillingProviderConfig{Provider: "tencent", Settings: a.Settings, Secrets: map[string]string{"secret_id": "id", "secret_key": "two"}}
	require.Equal(t, billingScopeKey(a), billingScopeKey(b), "secret rotation must not bypass duplicate scope checks")
	require.NotContains(t, billingScopeKey(a), "hunyuan")
}

func TestUpstreamReconciliationSaveEncryptsAndRedacts(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()
	enc := &billingTestEncryptor{}
	s := &UpstreamReconciliationService{db: db, encryptor: enc, persistentKey: true}
	input := SaveBillingConnectionInput{Name: "My upstream", Provider: "tencent", Settings: map[string]string{"business_code": "hunyuan"}, Secrets: map[string]string{"secret_id": "private_id", "secret_key": "private_key"}, Enabled: true, SyncIntervalHours: 24}
	mock.ExpectBegin()
	mock.ExpectQuery("SELECT pg_try_advisory_xact_lock").WithArgs("sub2api:upstream-billing:configuration").WillReturnRows(sqlmock.NewRows([]string{"locked"}).AddRow(true))
	mock.ExpectQuery("SELECT pg_try_advisory_xact_lock").WithArgs(billingLockName(0)).WillReturnRows(sqlmock.NewRows([]string{"locked"}).AddRow(true))
	mock.ExpectQuery("SELECT COUNT").WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(0))
	mock.ExpectQuery("SELECT EXISTS.*scope_key").WithArgs(billingScopeKey(BillingProviderConfig{Provider: input.Provider, Settings: input.Settings, Secrets: input.Secrets}), int64(0)).WillReturnRows(sqlmock.NewRows([]string{"exists"}).AddRow(false))
	mock.ExpectQuery("INSERT INTO upstream_billing_connections").WithArgs(int64(7), input.Name, input.Provider, `{"business_code":"hunyuan"}`, "sealed-secret", true, 24, sqlmock.AnyArg(), 2, "local_weighted").WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow(1))
	mock.ExpectExec("DELETE FROM upstream_billing_bindings").WithArgs(int64(1)).WillReturnResult(sqlmock.NewResult(0, 0))
	now := time.Now()
	mock.ExpectQuery(regexp.QuoteMeta("SELECT " + billingConnectionColumns + " FROM upstream_billing_connections WHERE id=$1")).WithArgs(int64(1)).WillReturnRows(sqlmock.NewRows([]string{"id", "name", "provider", "settings", "enabled", "sync_interval_hours", "last_synced_at", "next_sync_at", "last_error", "has_credentials", "sync_lookback_months", "allocation_mode"}).AddRow(1, input.Name, input.Provider, `{"business_code":"hunyuan"}`, true, 24, nil, now, "", true, 2, "local_weighted"))
	mock.ExpectQuery("SELECT resource_id,account_id").WithArgs(int64(1)).WillReturnRows(sqlmock.NewRows([]string{"resource_id", "account_id"}))
	mock.ExpectCommit()
	c, err := s.SaveConnection(context.Background(), 0, 7, input)
	require.NoError(t, err)
	require.Contains(t, enc.plaintext, "private_key")
	serialized, err := json.Marshal(c)
	require.NoError(t, err)
	require.NotContains(t, string(serialized), "private_key")
	require.NotContains(t, string(serialized), "private_id")
	require.NotContains(t, string(serialized), "sealed-secret")
	require.True(t, c.HasCredentials)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestUpstreamReconciliationUserReportIsolation(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()
	s := &UpstreamReconciliationService{db: db, now: func() time.Time { return time.Date(2026, 9, 18, 0, 0, 0, 0, time.UTC) }}
	mock.ExpectBegin()
	columns := []string{"connection_id", "connection_name", "provider", "resource_id", "account_id", "account_name", "api_key_id", "api_key_name", "user_id", "requests", "tokens", "local_cost", "allocated_cost", "currency", "method", "period_start", "period_end", "official_tokens", "local_matched_tokens", "token_status"}
	start := time.Date(2026, 9, 1, 0, 0, 0, 0, time.UTC)
	end := start.AddDate(0, 1, 0)
	mock.ExpectQuery(`(?s)SELECT c.id.*WHERE m.month=\$1 AND a.user_id=\$2 AND a.method<>'unmatched'`).WithArgs("2026-09", int64(42)).WillReturnRows(sqlmock.NewRows(columns).AddRow(99, "private connection", "tencent", "private resource", 88, "private account", 11, "My key", 42, 3, 100, "0.01", "0.02", "CNY", "official_token_weighted", start, end, 999, 100, "verified"))
	mock.ExpectCommit()
	report, err := s.Report(context.Background(), "2026-09", 42, false)
	require.NoError(t, err)
	require.Len(t, report.Items, 1)
	require.Equal(t, int64(11), report.Items[0].APIKeyID)
	require.Empty(t, report.Bills)
	require.Empty(t, report.Totals[0].OfficialCost)
	serialized, err := json.Marshal(report)
	require.NoError(t, err)
	for _, sensitive := range []string{"private", "connection_id", "account_id", "resource_id", "user_id", "official_cost", "unmatched_cost", "official_tokens"} {
		require.NotContains(t, string(serialized), sensitive)
	}
	require.NoError(t, mock.ExpectationsWereMet())
	_, err = s.Report(context.Background(), "2026-09", 0, false)
	require.Error(t, err)
}

func TestUpstreamReconciliationIdenticalSnapshotDoesNotDuplicate(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()
	conn, err := db.Conn(context.Background())
	require.NoError(t, err)
	defer conn.Close()
	s := &UpstreamReconciliationService{db: db}
	c := &BillingConnection{ID: 1, Bindings: []BillingBinding{}}
	start := time.Date(2026, 9, 1, 0, 0, 0, 0, time.UTC)
	fingerprint, err := billingSnapshotFingerprint(c.Bindings, []billingBillSnapshot{})
	require.NoError(t, err)
	for run := 0; run < 2; run++ {
		mock.ExpectBegin()
		mock.ExpectQuery("SELECT pg_try_advisory_xact_lock").WithArgs("sub2api:upstream-billing:import:").WillReturnRows(sqlmock.NewRows([]string{"locked"}).AddRow(true))
		mock.ExpectQuery("SELECT DISTINCT ON").WithArgs(int64(1), "2026-09", billingMaxAllocations+1).WillReturnRows(sqlmock.NewRows([]string{"resource", "start", "end", "account_id", "account_name", "api_key_id", "api_key_name", "user_id", "requests", "tokens", "local_cost"}))
		mock.ExpectQuery("SELECT r.fingerprint").WithArgs(int64(1), "2026-09").WillReturnRows(sqlmock.NewRows([]string{"fingerprint"}).AddRow(fingerprint))
		mock.ExpectCommit()
		require.NoError(t, s.persistBills(context.Background(), conn, c, "2026-09", start, start.AddDate(0, 1, 0), nil), fmt.Sprintf("run %d", run))
	}
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestUpstreamReconciliationRevisionFailureKeepsActiveMonth(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()
	conn, err := db.Conn(context.Background())
	require.NoError(t, err)
	defer conn.Close()
	s := &UpstreamReconciliationService{db: db}
	c := &BillingConnection{ID: 1, Bindings: []BillingBinding{}}
	start := time.Date(2026, 9, 1, 0, 0, 0, 0, time.UTC)
	end := start.AddDate(0, 1, 0)
	bills := []ProviderBill{{ID: "corrected-line", ResourceID: "unbound-resource", Amount: "-1.25", Currency: "USD", PeriodStart: start, PeriodEnd: end}}
	mock.ExpectBegin()
	mock.ExpectQuery("SELECT pg_try_advisory_xact_lock").WithArgs("sub2api:upstream-billing:import:").WillReturnRows(sqlmock.NewRows([]string{"locked"}).AddRow(true))
	mock.ExpectQuery("SELECT EXISTS.*").WithArgs("", int64(1), sqlmock.AnyArg()).WillReturnRows(sqlmock.NewRows([]string{"exists"}).AddRow(false))
	mock.ExpectQuery("SELECT DISTINCT ON").WithArgs(int64(1), "2026-09", billingMaxAllocations+1).WillReturnRows(sqlmock.NewRows([]string{"resource", "start", "end", "account_id", "account_name", "api_key_id", "api_key_name", "user_id", "requests", "tokens", "local_cost"}))
	mock.ExpectQuery("SELECT r.fingerprint").WithArgs(int64(1), "2026-09").WillReturnRows(sqlmock.NewRows([]string{"fingerprint"}).AddRow("previous-successful-revision"))
	mock.ExpectQuery("INSERT INTO upstream_billing_runs").WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow(2))
	mock.ExpectQuery("INSERT INTO upstream_billing_bills").WillReturnError(errors.New("database failed with secret detail"))
	mock.ExpectRollback()
	err = s.persistBills(context.Background(), conn, c, "2026-09", start, end, bills)
	require.ErrorIs(t, err, ErrBillingStorage)
	require.NotContains(t, err.Error(), "secret detail")
	// No active-month UPDATE is expected: the complete revision is transactional.
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestUpstreamReconciliationSyncRespectsCrossProcessLock(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()
	s := &UpstreamReconciliationService{db: db, encryptor: &billingTestEncryptor{}, persistentKey: true, ctx: context.Background()}
	mock.ExpectQuery("SELECT pg_try_advisory_lock").WithArgs(billingLockName(8)).WillReturnRows(sqlmock.NewRows([]string{"locked"}).AddRow(false))
	err = s.Sync(context.Background(), 8, "2026-09")
	require.ErrorIs(t, err, ErrBillingBusy)
	require.NoError(t, mock.ExpectationsWereMet())
}
