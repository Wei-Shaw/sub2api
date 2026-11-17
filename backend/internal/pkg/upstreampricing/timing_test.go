package upstreampricing

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func timingMap(t *testing.T, text string) map[string]any {
	t.Helper()
	var value map[string]any
	d := json.NewDecoder(strings.NewReader(text))
	d.UseNumber()
	require.NoError(t, d.Decode(&value))
	return value
}

func TestStationTimingNormalization(t *testing.T) {
	row, ok := normalizeTiming(timingMap(t, `{"id":4,"request_id":"r","model_name":"gpt-model","created_at":1700000000,"use_time":3,"other":"{\"frt\":420}"}`), "newapi")
	require.True(t, ok)
	require.EqualValues(t, 3000, *row.DurationMs)
	require.EqualValues(t, 420, *row.FirstTokenMs)
	require.EqualValues(t, 1000, row.PrecisionMs)
	row, ok = normalizeTiming(timingMap(t, `{"id":5,"model":"gpt-model","created_at":"2026-11-12T08:00:00+08:00","duration_ms":1234,"first_token_ms":null}`), "sub2api")
	require.True(t, ok)
	require.EqualValues(t, 1234, *row.DurationMs)
	require.Nil(t, row.FirstTokenMs)
	require.Equal(t, 0, row.CreatedAt.UTC().Hour())
	_, ok = normalizeTiming(timingMap(t, `{"id":5,"model":"gpt-model","created_at":"2026-11-12T08:00:00"}`), "sub2api")
	require.False(t, ok, "timestamps without timezone cannot be compared")
	require.Nil(t, timingInteger(json.Number("-1")))
	require.Nil(t, timingInteger(json.Number("1.5")))
	require.Nil(t, timingInteger(true))
	row, ok = normalizeTiming(timingMap(t, `{"id":5,"model_name":"gpt-model","created_at":1700000000,"use_time":0,"other":{"frt":0}}`), "newapi")
	require.True(t, ok)
	require.EqualValues(t, 0, *row.DurationMs)
	require.EqualValues(t, 0, *row.FirstTokenMs)
}

func TestStationTimingExactAndUncertainMatches(t *testing.T) {
	localDuration, remoteDuration, first := int64(3000), int64(4000), int64(100)
	local := TimingRecord{ID: "1", RequestID: "client", Model: "m", CreatedAt: time.Now(), DurationMs: &localDuration, FirstTokenMs: &first}
	remote := TimingRecord{ID: "2", RequestID: "upstream", Model: "m", CreatedAt: local.CreatedAt, DurationMs: &remoteDuration}
	result := compareTiming(local, "upstream", []TimingRecord{remote})
	require.Equal(t, "exact", result.Status)
	require.EqualValues(t, -1000, *result.DurationDiff, "negative differences are valid, not clamped to zero")
	require.Nil(t, result.FirstDiff, "missing station TTFT is not zero")
	result = compareTiming(local, "upstream", []TimingRecord{remote, remote})
	require.Equal(t, "ambiguous", result.Status)
	require.Nil(t, result.Station)
	require.Nil(t, result.DurationDiff)
	remote.RequestID = "another"
	result = compareTiming(local, "upstream", []TimingRecord{remote})
	require.Equal(t, "candidate", result.Status)
	require.Nil(t, result.Station)
	require.Nil(t, result.DurationDiff)
	remote.CreatedAt = remote.CreatedAt.Add(181 * time.Second)
	result = compareTiming(local, "", []TimingRecord{remote})
	require.Equal(t, "unmatched", result.Status)
	remote.RequestID = "client"
	require.Equal(t, "exact", compareTiming(local, "", []TimingRecord{remote}).Status)
}

func TestStationTimingRemoteScopesAndBounds(t *testing.T) {
	start := time.Unix(1700000000, 0)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		require.Equal(t, "/api/log/self", req.URL.Path)
		require.Equal(t, "Bearer synthetic-panel-token", req.Header.Get("Authorization"))
		require.Equal(t, "42", req.Header.Get("New-Api-User"))
		require.Equal(t, "private", req.URL.Query().Get("group"))
		require.Equal(t, "2", req.URL.Query().Get("type"))
		require.Equal(t, "1700000000", req.URL.Query().Get("start_timestamp"))
		_, _ = w.Write([]byte(`{"success":true,"data":{"total":2,"items":[{"id":1,"type":2,"group":"private","model_name":"m","created_at":1700000001,"use_time":2},{"id":2,"type":2,"group":"other","model_name":"m","created_at":1700000001,"use_time":5}]}}`))
	}))
	defer server.Close()
	r := NewRemote("")
	c := Config{Provider: "newapi", BaseURL: server.URL, Token: "synthetic-panel-token", UserID: "42", Group: "private"}
	rows, err := r.timingRecords(context.Background(), c, start, start.Add(time.Minute))
	require.NoError(t, err)
	require.Len(t, rows, 1)
	require.EqualValues(t, 2000, *rows[0].DurationMs)
	for _, payload := range []string{
		`{"data":{"total":1001,"items":[]}}`,
		`{"data":{"total":1}}`,
		`{"data":{"total":10,"items":[]}}`,
		`{"data":{"total":1,"items":[{"id":1,"model_name":"m","created_at":1700000001}]}}`,
		`{"data":{"total":2,"items":[{"id":1,"type":2,"group":"private","model_name":"m","created_at":1700000001},{"id":1,"type":2,"group":"private","model_name":"m","created_at":1700000001}]}}`,
	} {
		t.Run(payload[:20], func(t *testing.T) {
			bad := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) { _, _ = w.Write([]byte(payload)) }))
			defer bad.Close()
			c.BaseURL = bad.URL
			_, err := r.timingRecords(context.Background(), c, start, start.Add(time.Minute))
			require.Error(t, err)
			require.NotContains(t, err.Error(), c.Token)
		})
	}
}

func TestStationTimingSub2APIShanghaiQuery(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		require.Equal(t, "/api/v1/usage", req.URL.Path)
		require.Equal(t, "Asia/Shanghai", req.URL.Query().Get("timezone"))
		require.Equal(t, "desc", req.URL.Query().Get("sort_order"))
		require.Equal(t, "2026-11-12", req.URL.Query().Get("start_date"))
		require.Equal(t, "2026-11-12", req.URL.Query().Get("end_date"))
		_, _ = w.Write([]byte(`{"code":0,"data":{"total":1,"items":[{"id":6,"request_id":"exact","model":"m","created_at":"2026-11-12T07:59:00+08:00","duration_ms":456}]}}`))
	}))
	defer server.Close()
	start, err := time.Parse(time.RFC3339, "2026-11-11T23:58:00Z")
	require.NoError(t, err)
	rows, err := NewRemote("").timingRecords(context.Background(), Config{Provider: "sub2api", BaseURL: server.URL, Token: "synthetic"}, start, start.Add(5*time.Minute))
	require.NoError(t, err)
	require.Len(t, rows, 1)
	require.EqualValues(t, 456, *rows[0].DurationMs)
}

func TestStationTimingPaginationAndMovingSnapshot(t *testing.T) {
	start := time.Unix(1700000000, 0)
	for _, moving := range []bool{false, true} {
		calls := 0
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
			calls++
			page, err := strconv.Atoi(req.URL.Query().Get("p"))
			require.NoError(t, err)
			total := 101
			if moving && page > 1 {
				total++
			}
			items := []map[string]any{}
			for id := (page-1)*100 + 1; id <= min(page*100, total); id++ {
				items = append(items, map[string]any{"id": id, "type": 2, "group": "g", "model_name": "m", "created_at": 1700000001, "use_time": 1})
			}
			_ = json.NewEncoder(w).Encode(map[string]any{"data": map[string]any{"total": total, "items": items}})
		}))
		rows, err := NewRemote("").timingRecords(context.Background(), Config{Provider: "newapi", BaseURL: server.URL, Token: "synthetic", Group: "g"}, start, start.Add(time.Minute))
		server.Close()
		require.Equal(t, 2, calls)
		if moving {
			require.Error(t, err, "changed total must not publish partial comparisons")
		} else {
			require.NoError(t, err)
			require.Len(t, rows, 101)
		}
	}
}
