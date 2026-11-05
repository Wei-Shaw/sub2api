package repository

import (
	"context"
	"database/sql/driver"
	"encoding/json"
	"io"
	"regexp"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/Wei-Shaw/sub2api/internal/pkg/traffic"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/stretchr/testify/require"
)

func trafficTestUsage() *traffic.Usage {
	zero, original, actual, large := int64(0), int64(100), int64(125), int64(1<<54+1)
	return &traffic.Usage{
		Upstream: &traffic.Link{
			Request:  &traffic.Measurement{OriginalBytes: &original, ActualBytes: &actual, Compressed: true},
			Response: &traffic.Measurement{OriginalBytes: &large, ActualBytes: &zero, Compressed: true},
		},
		Downstream: &traffic.Link{
			Request:  &traffic.Measurement{OriginalBytes: &zero, ActualBytes: &zero},
			Response: &traffic.Measurement{ActualBytes: &actual, Compressed: true},
		},
	}
}

func trafficConsumedResponseTestUsage(t *testing.T) *traffic.Usage {
	t.Helper()
	const prefix = "consumed-encoded-prefix"
	collector := traffic.NewCollector()
	counter := collector.Begin(traffic.UpstreamResponse, true)
	body := traffic.ObserveResponseBody(io.NopCloser(strings.NewReader(prefix+"-unread-tail")), counter, false, -1)
	_, err := io.ReadFull(body, make([]byte, len(prefix)))
	require.NoError(t, err)
	require.NoError(t, body.Close())
	collector.Finalize()
	value := collector.Snapshot()
	require.NotNil(t, value.Upstream.Response.ActualBytes)
	require.EqualValues(t, len(prefix), *value.Upstream.Response.ActualBytes)
	require.Nil(t, value.Upstream.Response.OriginalBytes)
	return value
}

func trafficTestLog(observation *traffic.Usage) *service.UsageLog {
	return &service.UsageLog{
		UserID: 1, APIKeyID: 2, AccountID: 3, RequestID: "traffic-test", Model: "test-model",
		Traffic: observation, NativeCompactionV2: true,
		CreatedAt: time.Date(2026, 10, 15, 1, 2, 3, 0, time.UTC),
	}
}

func usageInsertColumns() []string {
	return strings.Split(strings.TrimPrefix(usageLogSelectColumns, "id, "), ", ")
}

func TestUsageTrafficPrepareAndSQLScanRoundTrip(t *testing.T) {
	for _, tc := range []struct {
		name  string
		value *traffic.Usage
	}{
		{"historical-null", nil},
		{"all-directions-zero-unknown-and-expansion", trafficTestUsage()},
		{"consumed-upstream-response-prefix", trafficConsumedResponseTestUsage(t)},
		{"missing-directions", &traffic.Usage{Downstream: &traffic.Link{}}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			log := trafficTestLog(tc.value)
			prepared := prepareUsageLogInsert(log)
			columns := usageInsertColumns()
			require.Len(t, prepared.args, len(columns))
			trafficIndex := len(columns) - 2
			require.Equal(t, "traffic", columns[trafficIndex])
			require.Equal(t, "jsonb", usageLogInsertArgTypes[trafficIndex])
			if tc.value == nil {
				require.Nil(t, prepared.args[trafficIndex], "historical traffic must be SQL NULL")
			} else {
				var decoded traffic.Usage
				payload, ok := prepared.args[trafficIndex].(string)
				require.True(t, ok)
				require.NoError(t, json.Unmarshal([]byte(payload), &decoded))
				require.Equal(t, tc.value, &decoded)
			}

			db, mock, err := sqlmock.New()
			require.NoError(t, err)
			defer func() { _ = db.Close() }()
			values := append([]driver.Value{int64(71)}, anySliceToDriverValues(prepared.args)...)
			query := "SELECT " + usageLogSelectColumns + " FROM usage_logs WHERE id = $1"
			mock.ExpectQuery(regexp.QuoteMeta(query)).WithArgs(int64(71)).
				WillReturnRows(sqlmock.NewRows(strings.Split(usageLogSelectColumns, ", ")).AddRow(values...))
			got, err := scanUsageLog(db.QueryRowContext(context.Background(), query, int64(71)))
			require.NoError(t, err)
			require.Equal(t, tc.value, got.Traffic)
			require.Equal(t, log.CreatedAt, got.CreatedAt, "traffic must not shift the created_at scanner position")
			require.True(t, got.NativeCompactionV2)
			require.NoError(t, mock.ExpectationsWereMet())
		})
	}
}

func TestUsageTrafficPrepareFreezesBatchSnapshot(t *testing.T) {
	value := trafficTestUsage()
	prepared := prepareUsageLogInsert(trafficTestLog(value))
	payload, ok := prepared.args[len(prepared.args)-2].(string)
	require.True(t, ok)
	*value.Downstream.Request.ActualBytes = 999
	value.Upstream = nil
	var got traffic.Usage
	require.NoError(t, json.Unmarshal([]byte(payload), &got))
	require.NotNil(t, got.Upstream)
	require.Equal(t, int64(0), *got.Downstream.Request.ActualBytes)
}

func TestUsageTrafficSQLScanMissingAndInvalidJSON(t *testing.T) {
	for _, tc := range []struct {
		name, payload string
		invalid       bool
	}{
		{"JSON-null", "null", false},
		{"empty-object", "{}", false},
		{"partial-observation", `{"downstream":{"request":{"actual_bytes":0}}}`, false},
		{"malformed", `{`, true},
		{"wrong-byte-type", `{"upstream":{"response":{"actual_bytes":"12"}}}`, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			prepared := prepareUsageLogInsert(trafficTestLog(nil))
			prepared.args[len(prepared.args)-2] = tc.payload
			db, mock, err := sqlmock.New()
			require.NoError(t, err)
			defer func() { _ = db.Close() }()
			values := append([]driver.Value{int64(1)}, anySliceToDriverValues(prepared.args)...)
			mock.ExpectQuery("SELECT").WillReturnRows(sqlmock.NewRows(strings.Split(usageLogSelectColumns, ", ")).AddRow(values...))
			got, err := scanUsageLog(db.QueryRow("SELECT"))
			if tc.invalid {
				require.ErrorContains(t, err, "decode usage log traffic")
			} else {
				require.NoError(t, err)
				var want *traffic.Usage
				require.NoError(t, json.Unmarshal([]byte(tc.payload), &want))
				require.Equal(t, want, got.Traffic)
			}
			require.NoError(t, mock.ExpectationsWereMet())
		})
	}
}

func splitUsageTrafficColumns(value string) []string {
	columns := strings.Split(value, ",")
	for i := range columns {
		columns[i] = strings.TrimSpace(columns[i])
	}
	return columns
}

func TestUsageTrafficAllInsertColumnContracts(t *testing.T) {
	log := trafficTestLog(trafficTestUsage())
	prepared := prepareUsageLogInsert(log)
	key := usageLogBatchKey(log.RequestID, log.APIKeyID)
	wantColumns := usageInsertColumns()
	for _, tc := range []struct {
		name  string
		batch bool
	}{{"create-batch", true}, {"best-effort-batch", false}} {
		t.Run(tc.name, func(t *testing.T) {
			var query string
			var args []any
			wantInput := wantColumns
			offset := 0
			if tc.batch {
				query, args = buildUsageLogBatchInsertQuery([]string{key}, map[string]usageLogInsertPrepared{key: prepared})
				wantInput = append([]string{"input_idx"}, wantColumns...)
				offset = 1
			} else {
				query, args = buildUsageLogBestEffortInsertQuery([]usageLogInsertPrepared{prepared})
			}
			require.Equal(t, prepared.args, args[offset:])
			for _, shape := range []struct {
				expr string
				want []string
			}{
				{`(?s)WITH input \((.*?)\) AS`, wantInput},
				{`(?s)INSERT INTO usage_logs \((.*?)\)`, wantColumns},
				{`(?s)\)\s+SELECT\s+(.*?)\s+FROM input`, wantColumns},
			} {
				match := regexp.MustCompile(shape.expr).FindStringSubmatch(query)
				require.Len(t, match, 2, query)
				require.Equal(t, shape.want, splitUsageTrafficColumns(match[1]))
			}
			for i, kind := range usageLogInsertArgTypes {
				require.Contains(t, query, "$"+strconv.Itoa(i+offset+1)+"::"+kind)
			}
		})
	}

	for _, kind := range []string{"create", "best-effort", "no-result"} {
		t.Run(kind, func(t *testing.T) {
			var actual string
			db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherFunc(func(_, query string) error { actual = query; return nil })))
			require.NoError(t, err)
			defer func() { _ = db.Close() }()
			if kind == "no-result" {
				mock.ExpectExec("").WithArgs(anySliceToDriverValues(prepared.args)...).WillReturnResult(sqlmock.NewResult(0, 1))
				require.NoError(t, execUsageLogInsertNoResult(context.Background(), db, prepared))
			} else {
				mock.ExpectQuery("").WithArgs(anySliceToDriverValues(prepared.args)...).WillReturnRows(sqlmock.NewRows([]string{"id", "created_at"}).AddRow(int64(1), log.CreatedAt))
				repo := &usageLogRepository{sql: db}
				if kind == "create" {
					_, err = repo.Create(context.Background(), log)
				} else {
					err = repo.CreateBestEffort(context.Background(), log)
				}
				require.NoError(t, err)
			}
			match := regexp.MustCompile(`(?s)INSERT INTO usage_logs \((.*?)\) VALUES \((.*?)\)`).FindStringSubmatch(actual)
			require.Len(t, match, 3)
			require.Equal(t, wantColumns, splitUsageTrafficColumns(match[1]))
			placeholders := splitUsageTrafficColumns(match[2])
			require.Len(t, placeholders, len(prepared.args))
			for i := range placeholders {
				require.Equal(t, "$"+strconv.Itoa(i+1), placeholders[i])
			}
			require.NoError(t, mock.ExpectationsWereMet())
		})
	}
}
