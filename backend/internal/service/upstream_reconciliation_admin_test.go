//go:build unit

package service

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/stretchr/testify/require"
)

var adminBillingTestColumns = []string{"id", "run_id", "provider_bill_id", "connection_id", "connection_name", "provider", "resource_id", "description", "amount", "source_amount", "currency", "period_start", "period_end", "synced_at", "allocation_mode", "raw_source", "usage_evidence"}

func adminBillingFixtureRows(ids []int64, raw string) *sqlmock.Rows {
	rows := sqlmock.NewRows(adminBillingTestColumns)
	start := time.Date(2026, 9, 1, 0, 0, 0, 0, time.UTC)
	for _, id := range ids {
		run := int64(7)
		if id > 2 {
			run = 9
		}
		rows.AddRow(id, run, "provider-line", 1, "Cloud account", "anthropic", "workspace", "Official charge", "1.00000000", "1.000000001", "USD", start, start.AddDate(0, 0, 1), start, "local_weighted", raw, `{"usage_status":"unsupported","token_status":"unsupported"}`)
	}
	return rows
}

func TestUpstreamReconciliationAdminPaginationFreezesSuccessfulRuns(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()
	now := time.Date(2026, 9, 18, 0, 0, 0, 0, time.UTC)
	s := &UpstreamReconciliationService{db: db, now: func() time.Time { return now }}
	input := AdminBillingListInput{AdminBillingFilter: AdminBillingFilter{Month: "2026-09"}, Limit: 2}
	mock.ExpectBegin()
	mock.ExpectQuery("SELECT m.active_run_id").WithArgs("2026-09", int64(0), "", "", billingMaxConnections+1).WillReturnRows(sqlmock.NewRows([]string{"active_run_id"}).AddRow(7).AddRow(9))
	mock.ExpectQuery("(?s)SELECT b.id,r.id.*WHERE r.id=ANY").WithArgs("{7,9}", "2026-09", int64(0), int64(0), "", "", 3).WillReturnRows(adminBillingFixtureRows([]int64{1, 2, 3}, `{"official":"source"}`))
	mock.ExpectCommit()
	first, err := s.AdminBills(context.Background(), input)
	require.NoError(t, err)
	require.Len(t, first.Items, 2)
	require.True(t, first.HasMore)
	require.NotEmpty(t, first.NextCursor)
	cursor, err := decodeAdminBillingCursor(first.NextCursor, input.AdminBillingFilter, now)
	require.NoError(t, err)
	require.Equal(t, []int64{7, 9}, cursor.RunIDs)
	require.EqualValues(t, 2, cursor.AfterID)
	input.Cursor = first.NextCursor
	// The second request validates the original revisions and never rereads the
	// active pointers, even if a sync has created newer monthly revisions.
	mock.ExpectBegin()
	mock.ExpectQuery("SELECT COUNT\\(\\*\\),COUNT\\(DISTINCT r.connection_id\\)").WithArgs("{7,9}", "2026-09", int64(0), "", "").WillReturnRows(sqlmock.NewRows([]string{"count", "connections"}).AddRow(2, 2))
	mock.ExpectQuery("(?s)SELECT b.id,r.id.*WHERE r.id=ANY").WithArgs("{7,9}", "2026-09", int64(2), int64(0), "", "", 3).WillReturnRows(adminBillingFixtureRows([]int64{3}, `{"official":"source"}`))
	mock.ExpectCommit()
	second, err := s.AdminBills(context.Background(), input)
	require.NoError(t, err)
	require.Len(t, second.Items, 1)
	require.False(t, second.HasMore)
	require.Equal(t, first.SnapshotAt, second.SnapshotAt)
	require.EqualValues(t, 3, second.Items[0].ID)
	serialized, err := json.Marshal(second)
	require.NoError(t, err)
	for _, field := range []string{"encrypted_secrets", "client_secret", "secret_key", "settings", "credentials"} {
		require.NotContains(t, string(serialized), field)
	}
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestUpstreamReconciliationAdminCursorRejectsInvalidScope(t *testing.T) {
	now := time.Date(2026, 9, 18, 0, 0, 0, 0, time.UTC)
	filter := AdminBillingFilter{Month: "2026-09", Provider: "azure", ConnectionID: 4}
	valid := adminBillingCursor{Version: 1, FilterHash: adminBillingFilterHash(filter), SnapshotAt: now, RunIDs: []int64{7, 9}, AfterID: 3}
	encoded, err := encodeAdminBillingCursor(valid)
	require.NoError(t, err)
	_, err = decodeAdminBillingCursor(encoded, filter, now)
	require.NoError(t, err)
	wrong := filter
	wrong.Provider = "anthropic"
	_, err = decodeAdminBillingCursor(encoded, wrong, now)
	require.Error(t, err)
	for _, mutate := range []func(*adminBillingCursor){func(c *adminBillingCursor) { c.RunIDs = []int64{9, 7} }, func(c *adminBillingCursor) { c.RunIDs = []int64{7, 7} }, func(c *adminBillingCursor) { c.AfterID = -1 }, func(c *adminBillingCursor) { c.Version = 2 }, func(c *adminBillingCursor) { c.SnapshotAt = now.Add(time.Hour) }} {
		bad := valid
		mutate(&bad)
		text, err := encodeAdminBillingCursor(bad)
		require.NoError(t, err)
		_, err = decodeAdminBillingCursor(text, filter, now)
		require.Error(t, err)
	}
	for _, value := range []string{"not-base64", strings.Repeat("x", 8193), base64.RawURLEncoding.EncodeToString([]byte(`{} {}`))} {
		_, err = decodeAdminBillingCursor(value, filter, now)
		require.Error(t, err)
	}
}

func TestUpstreamReconciliationAdminPageBoundsSerializedRawData(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()
	now := time.Date(2026, 9, 18, 0, 0, 0, 0, time.UTC)
	s := &UpstreamReconciliationService{db: db, now: func() time.Time { return now }}
	// HTML escaping can expand one compact 60KiB source row to >350KiB.
	// The response budget must account for the actual serialized record size.
	raw := `{"value":"` + strings.Repeat("<", 60000) + `"}`
	mock.ExpectBegin()
	mock.ExpectQuery("SELECT m.active_run_id").WillReturnRows(sqlmock.NewRows([]string{"active_run_id"}).AddRow(7).AddRow(9))
	mock.ExpectQuery("(?s)SELECT b.id,r.id.*WHERE r.id=ANY").WillReturnRows(adminBillingFixtureRows([]int64{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11}, raw))
	mock.ExpectCommit()
	result, err := s.AdminBills(context.Background(), AdminBillingListInput{AdminBillingFilter: AdminBillingFilter{Month: "2026-09"}})
	require.NoError(t, err)
	require.True(t, result.HasMore)
	require.Greater(t, len(result.Items), 0)
	require.Less(t, len(result.Items), AdminBillingDefaultPageSize)
	encoded, err := json.Marshal(result)
	require.NoError(t, err)
	require.Less(t, len(encoded), AdminBillingMaxPageBytes)
	cursor, err := decodeAdminBillingCursor(result.NextCursor, AdminBillingFilter{Month: "2026-09"}, now)
	require.NoError(t, err)
	require.Equal(t, result.Items[len(result.Items)-1].ID, cursor.AfterID, "unreturned raw rows must not be skipped")
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestUpstreamReconciliationAdminSingleRequiresMonthAndFilters(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()
	s := &UpstreamReconciliationService{db: db, now: func() time.Time { return time.Date(2026, 9, 18, 0, 0, 0, 0, time.UTC) }}
	_, err = s.AdminBill(context.Background(), 1, AdminBillingFilter{})
	require.Error(t, err)
	filter := AdminBillingFilter{Month: "2026-09", ConnectionID: 1, Provider: "anthropic", ResourceID: "workspace"}
	mock.ExpectQuery("(?s)SELECT b.id,r.id.*WHERE b.id=\\$1 AND r.month=\\$2").WithArgs(int64(2), "2026-09", int64(1), "anthropic", "workspace").WillReturnRows(adminBillingFixtureRows([]int64{2}, `{"original_amount":"1.000000001"}`))
	record, err := s.AdminBill(context.Background(), 2, filter)
	require.NoError(t, err)
	require.Equal(t, "1.000000001", record.SourceAmount)
	mock.ExpectQuery("(?s)SELECT b.id,r.id.*WHERE b.id=\\$1").WithArgs(int64(2), "2026-08", int64(1), "anthropic", "workspace").WillReturnRows(sqlmock.NewRows(adminBillingTestColumns))
	filter.Month = "2026-08"
	_, err = s.AdminBill(context.Background(), 2, filter)
	require.ErrorIs(t, err, ErrBillingNotFound)
	require.NoError(t, mock.ExpectationsWereMet())
}
