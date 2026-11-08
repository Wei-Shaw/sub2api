package main

import (
	"bytes"
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/shopspring/decimal"
)

func testApp(t *testing.T) *app {
	t.Helper()
	dir := t.TempDir()
	db, err := sql.Open("sqlite", filepath.Join(dir, "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	db.SetMaxOpenConns(1)
	a := &app{cfg: config{DataDir: dir, AdminUser: "admin", AdminPassword: "secret", Sub2APIUnitToCNY: decimal.NewFromInt(1)}, db: db}
	if err := a.migrate(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	return a
}

func configureTestRule(t *testing.T, a *app, accountID int64, provider, externalKey, multiplier string) {
	t.Helper()
	now := time.Now().UTC().Format(time.RFC3339Nano)
	if _, err := a.db.Exec(`INSERT OR REPLACE INTO upstream_account_rules(account_id,provider,external_key,multiplier,enabled,created_at,updated_at) VALUES(?,?,?,?,1,?,?)`, accountID, provider, externalKey, multiplier, now, now); err != nil {
		t.Fatal(err)
	}
	if err := a.reloadAccountRules(); err != nil {
		t.Fatal(err)
	}
}

func TestCollectCaddyMapsClientToUpstream(t *testing.T) {
	a := testApp(t)
	logPath := filepath.Join(t.TempDir(), "access.log")
	line := `{"ts":1785300000.25,"request":{"uri":"/responses"},"resp_headers":{"X-Client-Request-Id":["client-1"],"X-Request-Id":["sub2api-local-1","upstream-1"]}}` + "\n"
	if err := os.WriteFile(logPath, []byte(line), 0o600); err != nil {
		t.Fatal(err)
	}
	a.cfg.CaddyLogPath = logPath
	if err := a.collectCaddy(); err != nil {
		t.Fatal(err)
	}
	var upstream, path string
	if err := a.db.QueryRow(`SELECT upstream_request_id,path FROM request_maps WHERE client_request_id=?`, "client-1").Scan(&upstream, &path); err != nil {
		t.Fatal(err)
	}
	if upstream != "upstream-1" || path != "/responses" {
		t.Fatalf("unexpected mapping %q %q", upstream, path)
	}
	if got := a.stateInt64("caddy_offset"); got != int64(len(line)) {
		t.Fatalf("offset=%d want=%d", got, len(line))
	}
}

func TestImportRequiresFXAndStoresCNYCost(t *testing.T) {
	a := testApp(t)
	bad := []byte(`[{"upstream_request_id":"up-1","cost":"2","currency":"USD"}]`)
	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodPost, "/ops/api/upstream/import", bytes.NewReader(bad))
	r.Header.Set("Content-Type", "application/json")
	a.importUpstream(w, r)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("bad import status=%d", w.Code)
	}

	good := []byte(`[{"upstream_request_id":"up-1","cost":"2","currency":"USD","fx_rate_to_cny":"7.2","occurred_at":"2026-07-29T00:00:00Z"}]`)
	w = httptest.NewRecorder()
	r = httptest.NewRequest(http.MethodPost, "/ops/api/upstream/import", bytes.NewReader(good))
	r.Header.Set("Content-Type", "application/json")
	a.importUpstream(w, r)
	if w.Code != http.StatusOK {
		t.Fatalf("good import status=%d body=%s", w.Code, w.Body.String())
	}
	var original, fx, cny, currency string
	if err := a.db.QueryRow(`SELECT cost,fx_rate_to_cny,cost_cny,currency FROM upstream_usage WHERE upstream_request_id='up-1'`).Scan(&original, &fx, &cny, &currency); err != nil {
		t.Fatal(err)
	}
	if original != "2" || fx != "7.2" || cny != "14.4" || currency != "USD" {
		t.Fatalf("stored=%q %q %q %q", original, fx, cny, currency)
	}

	duplicate := []byte(`[{"upstream_request_id":"up-1","cost":"99","currency":"USD","fx_rate_to_cny":"6.8","occurred_at":"2026-07-29T01:00:00Z","model":"gpt-5.6-sol"}]`)
	w = httptest.NewRecorder()
	r = httptest.NewRequest(http.MethodPost, "/ops/api/upstream/import", bytes.NewReader(duplicate))
	r.Header.Set("Content-Type", "application/json")
	a.importUpstream(w, r)
	if w.Code != http.StatusOK {
		t.Fatalf("duplicate import status=%d body=%s", w.Code, w.Body.String())
	}
	var model string
	if err := a.db.QueryRow(`SELECT cost,fx_rate_to_cny,cost_cny,currency,model FROM upstream_usage WHERE upstream_request_id='up-1'`).Scan(&original, &fx, &cny, &currency, &model); err != nil {
		t.Fatal(err)
	}
	if original != "2" || fx != "7.2" || cny != "14.4" || currency != "USD" || model != "gpt-5.6-sol" {
		t.Fatalf("immutable stored=%q %q %q %q model=%q", original, fx, cny, currency, model)
	}
}

func TestSummaryUsesActualRevenueAndConvertedCost(t *testing.T) {
	a := testApp(t)
	now := time.Now().UTC().Format(time.RFC3339Nano)
	_, err := a.db.Exec(`INSERT INTO usage_snapshots(source_id,request_id,user_id,api_key_id,account_id,model,actual_cost,input_tokens,output_tokens,cache_read_tokens,actual_cost_cny,fx_rate_to_cny,created_at) VALUES(1,'client:c1',2,3,4,'gpt-5.5','20',550,5,3840,'20','1',?)`, now)
	if err != nil {
		t.Fatal(err)
	}
	_, err = a.db.Exec(`INSERT INTO request_maps(client_request_id,upstream_request_id,observed_at,path) VALUES('c1','u1',?,'/responses')`, now)
	if err != nil {
		t.Fatal(err)
	}
	_, err = a.db.Exec(`INSERT INTO upstream_usage(provider,upstream_request_id,cost,currency,fx_rate_to_cny,cost_cny,occurred_at,source,imported_at) VALUES('a6','u1','2','USD','7.2','14.4',?,'test',?)`, now, now)
	if err != nil {
		t.Fatal(err)
	}
	_, err = a.db.Exec(`INSERT INTO upstream_usage(provider,upstream_request_id,cost,currency,fx_rate_to_cny,cost_cny,occurred_at,source,imported_at) VALUES('a6','orphan','1','CNY','1','1',?,'test',?)`, now, now)
	if err != nil {
		t.Fatal(err)
	}
	if err := a.reconcileUpstream(); err != nil {
		t.Fatal(err)
	}
	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodGet, "/ops/api/summary?from=2026-01-01T00:00:00Z", nil)
	a.summary(w, r)
	if w.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", w.Code, w.Body.String())
	}
	var got map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if got["revenue"] != "20.00000000" || got["matched_revenue"] != "20.00000000" || got["upstream_cost"] != "14.40000000" || got["billed_upstream_cost"] != "14.40000000" || got["gross_profit"] != "5.60000000" || got["upstream_unmatched"] != float64(1) || got["cost_policy"] != "billed_or_subarx_api_or_rule" {
		t.Fatalf("summary=%v", got)
	}
	if _, exists := got["estimated_upstream_cost"]; exists {
		t.Fatalf("summary must not expose estimated cost: %v", got)
	}
}

func TestSummaryLeavesUnmatchedCostAndProfitUnconfirmed(t *testing.T) {
	a := testApp(t)
	configureTestRule(t, a, 4, "a6", "test-token", "")
	now := time.Now().UTC().Format(time.RFC3339Nano)
	_, err := a.db.Exec(`INSERT INTO usage_snapshots(source_id,request_id,user_id,api_key_id,account_id,model,actual_cost,input_tokens,output_tokens,cache_creation_tokens,actual_cost_cny,fx_rate_to_cny,created_at) VALUES(1,'client:c1',2,3,4,'gpt-5.5','20',12,4,3,'20','1',?)`, now)
	if err != nil {
		t.Fatal(err)
	}
	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodGet, "/ops/api/summary?from=2026-01-01T00:00:00Z", nil)
	a.summary(w, r)
	var got map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if got["revenue"] != "20.00000000" || got["matched_revenue"] != "0.00000000" || got["upstream_cost"] != "0.00000000" || got["gross_profit"] != "0.00000000" || got["downstream_unmatched"] != float64(1) {
		t.Fatalf("summary=%v", got)
	}
	rows, err := a.profitRows("2026-01-01T00:00:00Z", 10)
	if err != nil || len(rows) != 1 {
		t.Fatalf("rows=%v err=%v", rows, err)
	}
	if rows[0].CostSource != "pending" || rows[0].CostText != "" || rows[0].BilledCostText != "" || rows[0].ProfitText != "" || rows[0].InputTokens != 12 || rows[0].OutputTokens != 4 || rows[0].CacheTokens != 3 {
		t.Fatalf("row=%+v", rows[0])
	}
}

func TestA6ConfiguredUnmatchedRowIsNotMarkedRuleUnconfigured(t *testing.T) {
	a := testApp(t)
	configureTestRule(t, a, 27, "a6", "anthropic-0.15-95%", "")
	created := "2026-08-20T13:08:42.684953Z"
	_, err := a.db.Exec(`INSERT INTO usage_snapshots(source_id,request_id,user_id,api_key_id,account_id,model,actual_cost,input_tokens,output_tokens,cache_read_tokens,cache_creation_tokens,actual_cost_cny,created_at,rule_provider,rule_external_key,rule_version)
VALUES(1,'client:claude-pending',1,1,27,'claude-sonnet-5','0.01',56,1,22,36,'0.01',?,'a6','anthropic-0.15-95%',1)`, created)
	if err != nil {
		t.Fatal(err)
	}
	rows, err := a.profitRows("2026-01-01T00:00:00Z", 10)
	if err != nil || len(rows) != 1 {
		t.Fatalf("rows=%v err=%v", rows, err)
	}
	if rows[0].Matched || rows[0].CostSource != "a6_pending" || rows[0].CostSourceLabel != "上游账单待匹配" {
		t.Fatalf("row=%+v", rows[0])
	}
}

func TestA6MappedUnmatchedRowWaitsForBill(t *testing.T) {
	a := testApp(t)
	configureTestRule(t, a, 15, "a6", "openai-0.12-95%", "")
	created := "2026-08-20T16:38:57.408836Z"
	_, err := a.db.Exec(`INSERT INTO usage_snapshots(source_id,request_id,user_id,api_key_id,account_id,model,actual_cost,actual_cost_cny,created_at,rule_provider,rule_external_key,rule_version)
VALUES(1,'client:codex-waiting',1,1,15,'gpt-5.6-sol','0.01','0.01',?,'a6','openai-0.12-95%',1)`, created)
	if err != nil {
		t.Fatal(err)
	}
	_, err = a.db.Exec(`INSERT INTO request_maps(client_request_id,upstream_request_id,observed_at,path)
VALUES('codex-waiting','gateway-upstream-id',?,'/v1/responses')`, created)
	if err != nil {
		t.Fatal(err)
	}

	rows, err := a.profitRows("2026-01-01T00:00:00Z", 10)
	if err != nil || len(rows) != 1 {
		t.Fatalf("rows=%v err=%v", rows, err)
	}
	if rows[0].Matched || rows[0].CostSource != "a6_waiting" || rows[0].CostSourceLabel != "等待上游账单" {
		t.Fatalf("row=%+v", rows[0])
	}
}

func TestSummaryIncludesMoreThanOneHundredThousandRows(t *testing.T) {
	a := testApp(t)
	_, err := a.db.Exec(`WITH RECURSIVE seq(id) AS (
  VALUES(1) UNION ALL SELECT id+1 FROM seq WHERE id<100001
)
INSERT INTO usage_snapshots(source_id,request_id,user_id,api_key_id,account_id,model,actual_cost,actual_cost_cny,created_at)
SELECT id,'client:bulk-' || id,1,1,99,'gpt-test','1','1','2026-07-29T00:00:00Z' FROM seq`)
	if err != nil {
		t.Fatal(err)
	}
	w := httptest.NewRecorder()
	a.summary(w, httptest.NewRequest(http.MethodGet, "/ops/api/summary?from=2026-01-01T00:00:00Z", nil))
	if w.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", w.Code, w.Body.String())
	}
	var got map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if got["revenue"] != "100001.00000000" || got["downstream_unmatched"] != float64(100001) {
		t.Fatalf("summary truncated: %v", got)
	}
}

func TestRequestsPaginatesWithoutDuplicates(t *testing.T) {
	a := testApp(t)
	for id := 1; id <= 5; id++ {
		_, err := a.db.Exec(`INSERT INTO usage_snapshots(source_id,request_id,user_id,api_key_id,account_id,model,actual_cost,actual_cost_cny,created_at) VALUES(?,?,?,?,?,?,?,?,?)`, id, fmt.Sprintf("client:p-%d", id), 1, 1, 99, "gpt-test", "1", "1", "2026-07-29T00:00:00Z")
		if err != nil {
			t.Fatal(err)
		}
	}
	requestPage := func(page int) map[string]any {
		w := httptest.NewRecorder()
		r := httptest.NewRequest(http.MethodGet, fmt.Sprintf("/ops/api/requests?from=2026-01-01T00:00:00Z&page=%d&page_size=2", page), nil)
		a.requests(w, r)
		if w.Code != http.StatusOK {
			t.Fatalf("page=%d status=%d body=%s", page, w.Code, w.Body.String())
		}
		var got map[string]any
		if err := json.Unmarshal(w.Body.Bytes(), &got); err != nil {
			t.Fatal(err)
		}
		return got
	}
	first, second, third := requestPage(1), requestPage(2), requestPage(3)
	if first["total"] != float64(5) || first["total_pages"] != float64(3) || len(first["items"].([]any)) != 2 || len(second["items"].([]any)) != 2 || len(third["items"].([]any)) != 1 {
		t.Fatalf("pages=%v %v %v", first, second, third)
	}
	firstID := first["items"].([]any)[0].(map[string]any)["source_id"]
	secondID := second["items"].([]any)[0].(map[string]any)["source_id"]
	if firstID == secondID || firstID != float64(5) || secondID != float64(3) {
		t.Fatalf("unexpected page boundaries first=%v second=%v", firstID, secondID)
	}
	beyond := requestPage(99)
	if beyond["page"] != float64(3) || len(beyond["items"].([]any)) != 1 {
		t.Fatalf("page beyond end was not clamped: %v", beyond)
	}
}

func TestTimeWindowDefaultsToLastTwentyFourHours(t *testing.T) {
	now := time.Date(2026, 7, 29, 12, 30, 0, 0, time.FixedZone("CST", 8*60*60))
	r := httptest.NewRequest(http.MethodGet, "/ops/api/summary", nil)
	window, err := queryTimeWindow(r, now)
	if err != nil {
		t.Fatal(err)
	}
	if window.From != "2026-07-28T04:30:00Z" || window.To != "2026-07-29T04:30:00Z" {
		t.Fatalf("unexpected default window: %+v", window)
	}
}

func TestSummaryAndRequestsUseSameExclusiveTimeWindow(t *testing.T) {
	a := testApp(t)
	for id, created := range []string{
		"2026-07-28T23:59:59Z",
		"2026-07-29T00:00:00Z",
		"2026-07-29T11:59:59Z",
		"2026-07-29T12:00:00Z",
	} {
		_, err := a.db.Exec(`INSERT INTO usage_snapshots(source_id,request_id,user_id,api_key_id,account_id,model,actual_cost,actual_cost_cny,created_at) VALUES(?,?,?,?,?,?,?,?,?)`, id+1, fmt.Sprintf("client:window-%d", id), 1, 1, 99, "gpt-test", "1", "1", created)
		if err != nil {
			t.Fatal(err)
		}
	}
	query := "from=2026-07-29T00:00:00Z&to=2026-07-29T12:00:00Z"
	summaryRecorder := httptest.NewRecorder()
	a.summary(summaryRecorder, httptest.NewRequest(http.MethodGet, "/ops/api/summary?"+query, nil))
	if summaryRecorder.Code != http.StatusOK {
		t.Fatalf("summary status=%d body=%s", summaryRecorder.Code, summaryRecorder.Body.String())
	}
	var summary map[string]any
	if err := json.Unmarshal(summaryRecorder.Body.Bytes(), &summary); err != nil {
		t.Fatal(err)
	}
	requestsRecorder := httptest.NewRecorder()
	a.requests(requestsRecorder, httptest.NewRequest(http.MethodGet, "/ops/api/requests?"+query, nil))
	if requestsRecorder.Code != http.StatusOK {
		t.Fatalf("requests status=%d body=%s", requestsRecorder.Code, requestsRecorder.Body.String())
	}
	var requests map[string]any
	if err := json.Unmarshal(requestsRecorder.Body.Bytes(), &requests); err != nil {
		t.Fatal(err)
	}
	if summary["revenue"] != "2.00000000" || summary["downstream_unmatched"] != float64(2) || requests["total"] != float64(2) || len(requests["items"].([]any)) != 2 {
		t.Fatalf("window mismatch summary=%v requests=%v", summary, requests)
	}
}

func TestTimeWindowRejectsInvalidOrReversedBounds(t *testing.T) {
	for _, rawURL := range []string{
		"/ops/api/summary?from=not-a-time&to=2026-07-29T12:00:00Z",
		"/ops/api/summary?from=2026-07-29T12:00:00Z&to=2026-07-29T12:00:00Z",
	} {
		if _, err := queryTimeWindow(httptest.NewRequest(http.MethodGet, rawURL, nil), time.Now()); err == nil {
			t.Fatalf("expected invalid window for %s", rawURL)
		}
	}
}

func TestRecordCardsAndFiltersShareCategoryCounts(t *testing.T) {
	a := testApp(t)
	configureTestRule(t, a, 13, "subarx", "", "0.1")
	created := "2026-07-29T10:00:00Z"
	for _, values := range []struct {
		id        int
		accountID int
		requestID string
	}{
		{1, 15, "client:billed"},
		{2, 13, "client:subarx"},
		{3, 99, "client:pending"},
	} {
		_, err := a.db.Exec(`INSERT INTO usage_snapshots(source_id,request_id,user_id,api_key_id,account_id,model,actual_cost,input_cost,output_cost,actual_cost_cny,created_at) VALUES(?,?,?,?,?,?,?,?,?,?,?)`, values.id, values.requestID, 1, 1, values.accountID, "gpt-test", "1", "0.01", "0.02", "1", created)
		if err != nil {
			t.Fatal(err)
		}
	}
	_, err := a.db.Exec(`INSERT INTO upstream_usage(provider,upstream_request_id,cost,currency,fx_rate_to_cny,cost_cny,occurred_at,model,input_tokens,output_tokens,cache_tokens,usage_source_id,source,imported_at) VALUES
('a6','up-billed','0.1','USD','7.2','0.72',?,'gpt-test',10,2,0,1,'test',?),
('a6','up-orphan','0.2','USD','7.2','1.44','2026-07-29T10:01:00Z','gpt-test',20,4,3,NULL,'test',?)`, created, created, created)
	if err != nil {
		t.Fatal(err)
	}
	query := "from=2026-07-29T00:00:00Z&to=2026-07-30T00:00:00Z"
	summaryRecorder := httptest.NewRecorder()
	a.summary(summaryRecorder, httptest.NewRequest(http.MethodGet, "/ops/api/summary?"+query, nil))
	var summary map[string]any
	if err := json.Unmarshal(summaryRecorder.Body.Bytes(), &summary); err != nil {
		t.Fatal(err)
	}
	if summary["downstream_matched"] != float64(2) || summary["downstream_unmatched"] != float64(1) || summary["upstream_unmatched"] != float64(1) || summary["record_total"] != float64(4) {
		t.Fatalf("unexpected card counts: %v", summary)
	}
	wants := map[string]struct {
		total      float64
		recordType string
	}{
		"all":                {4, "upstream_unmatched"},
		"matched":            {2, "downstream"},
		"unmatched":          {1, "downstream"},
		"upstream_unmatched": {1, "upstream_unmatched"},
	}
	for status, want := range wants {
		w := httptest.NewRecorder()
		a.requests(w, httptest.NewRequest(http.MethodGet, "/ops/api/requests?"+query+"&status="+status, nil))
		if w.Code != http.StatusOK {
			t.Fatalf("status=%s response=%d body=%s", status, w.Code, w.Body.String())
		}
		var payload map[string]any
		if err := json.Unmarshal(w.Body.Bytes(), &payload); err != nil {
			t.Fatal(err)
		}
		items := payload["items"].([]any)
		if payload["total"] != want.total || len(items) == 0 || items[0].(map[string]any)["record_type"] != want.recordType {
			t.Fatalf("status=%s payload=%v", status, payload)
		}
	}
}

func TestTimeSeriesUsesCardAccountingAndFillsEmptyBuckets(t *testing.T) {
	a := testApp(t)
	configureTestRule(t, a, 13, "subarx", "", "0.1")
	for _, values := range []struct {
		id        int
		accountID int
		created   string
		revenue   string
		inputCost string
	}{
		{1, 15, "2026-07-29T00:10:00Z", "10", "0"},
		{2, 99, "2026-07-29T01:10:00Z", "5", "0"},
		{3, 13, "2026-07-29T02:10:00Z", "3", "0.001"},
	} {
		_, err := a.db.Exec(`INSERT INTO usage_snapshots(source_id,request_id,user_id,api_key_id,account_id,model,actual_cost,input_cost,actual_cost_cny,created_at) VALUES(?,?,?,?,?,?,?,?,?,?)`, values.id, fmt.Sprintf("client:series-%d", values.id), 1, 1, values.accountID, "gpt-test", values.revenue, values.inputCost, values.revenue, values.created)
		if err != nil {
			t.Fatal(err)
		}
	}
	_, err := a.db.Exec(`INSERT INTO upstream_usage(provider,upstream_request_id,cost,currency,fx_rate_to_cny,cost_cny,occurred_at,model,usage_source_id,source,imported_at) VALUES
('a6','series-billed','2','CNY','1','2','2026-07-29T00:11:00Z','gpt-test',1,'test','2026-07-29T00:11:00Z'),
('a6','series-orphan','1','CNY','1','1','2026-07-29T01:20:00Z','gpt-test',NULL,'test','2026-07-29T01:20:00Z')`)
	if err != nil {
		t.Fatal(err)
	}
	points, bucket, err := a.buildTimeSeries("2026-07-29T00:00:00Z", "2026-07-29T04:00:00Z")
	if err != nil {
		t.Fatal(err)
	}
	if bucket != "1小时" || len(points) != 4 {
		t.Fatalf("bucket=%s points=%d", bucket, len(points))
	}
	if points[0].Revenue != "10.00000000" || points[0].UpstreamCost != "2.00000000" || points[0].GrossProfit != "8.00000000" || points[0].Matched != 1 || points[0].RecordTotal != 1 {
		t.Fatalf("first bucket=%+v", points[0])
	}
	if points[1].Revenue != "5.00000000" || points[1].GrossProfit != "0.00000000" || points[1].Unmatched != 1 || points[1].UpstreamUnmatched != 1 || points[1].RecordTotal != 2 {
		t.Fatalf("second bucket=%+v", points[1])
	}
	if points[2].Revenue != "3.00000000" || points[2].UpstreamCost != "0.00010000" || points[2].GrossProfit != "2.99990000" || points[2].Matched != 1 {
		t.Fatalf("third bucket=%+v", points[2])
	}
	if points[3].RecordTotal != 0 || points[3].Revenue != "0.00000000" {
		t.Fatalf("empty bucket=%+v", points[3])
	}
}

func TestTimeSeriesBucketSizeScalesWithRange(t *testing.T) {
	wants := []struct {
		window time.Duration
		label  string
	}{
		{24 * time.Hour, "1小时"},
		{7 * 24 * time.Hour, "6小时"},
		{90 * 24 * time.Hour, "1天"},
		{365 * 24 * time.Hour, "1周"},
	}
	for _, want := range wants {
		_, label := timeSeriesBucketSize(want.window)
		if label != want.label {
			t.Fatalf("window=%s label=%s want=%s", want.window, label, want.label)
		}
	}
}

func TestTimeSeriesReconcilesPerBucketRoundingToSummaryPrecision(t *testing.T) {
	a := testApp(t)
	for id, created := range []string{"2026-07-29T00:10:00Z", "2026-07-29T01:10:00Z"} {
		_, err := a.db.Exec(`INSERT INTO usage_snapshots(source_id,request_id,user_id,api_key_id,account_id,model,actual_cost,actual_cost_cny,created_at) VALUES(?,?,?,?,?,?,?,?,?)`, id+1, fmt.Sprintf("client:round-%d", id), 1, 1, 99, "gpt-test", "0.000000006", "0.000000006", created)
		if err != nil {
			t.Fatal(err)
		}
	}
	points, _, err := a.buildTimeSeries("2026-07-29T00:00:00Z", "2026-07-29T02:00:00Z")
	if err != nil {
		t.Fatal(err)
	}
	total := decimal.Zero
	for _, point := range points {
		total = total.Add(decimal.RequireFromString(point.Revenue))
	}
	if total.StringFixed(8) != "0.00000001" {
		t.Fatalf("displayed bucket total=%s want=0.00000001 points=%+v", total, points)
	}
}

func TestSubarxRuleCalculatesOfficialCostByUpstreamMultiplier(t *testing.T) {
	a := testApp(t)
	configureTestRule(t, a, 13, "subarx", "", "0.1")
	now := time.Now().UTC().Format(time.RFC3339Nano)
	_, err := a.db.Exec(`INSERT INTO usage_snapshots(source_id,request_id,user_id,api_key_id,account_id,group_id,group_name,model,actual_cost,input_tokens,output_tokens,cache_read_tokens,input_cost,output_cost,cache_read_cost,cache_creation_cost,actual_cost_cny,created_at) VALUES(9,'client:subarx',2,3,13,5,'codex-plus稳定-0.2x','gpt-5.6-sol','0.000055',25,5,0,'0.000125','0.000150','0','0','0.000055',?)`, now)
	if err != nil {
		t.Fatal(err)
	}
	rows, err := a.profitRows("2026-01-01T00:00:00Z", 10)
	if err != nil || len(rows) != 1 {
		t.Fatalf("rows=%v err=%v", rows, err)
	}
	row := rows[0]
	if !row.Matched || row.CostSource != "subarx_rule" || row.CostSourceLabel != "规则实扣" || row.UpstreamCostText != "0.00003000" || row.CostText != "0.00003000" || row.ProfitText != "0.00002500" || row.UpstreamCurrency != "USD" {
		t.Fatalf("row=%+v", row)
	}
}

func TestSubarxAPIAccountWaitsForRealBillInsideCoverage(t *testing.T) {
	a := testApp(t)
	a.cfg.SubarxAccountAPIKeys = map[int64]string{13: "test-key"}
	a.cfg.SubarxLookbackDays = 90
	a.cfg.SubarxTimezone = "Asia/Shanghai"
	configureTestRule(t, a, 13, "subarx", "", "0.1")
	now := time.Now().UTC().Format(time.RFC3339Nano)
	_, err := a.db.Exec(`INSERT INTO usage_snapshots(source_id,request_id,user_id,api_key_id,account_id,group_id,group_name,model,actual_cost,input_tokens,output_tokens,input_cost,output_cost,cache_read_cost,cache_creation_cost,actual_cost_cny,created_at) VALUES(19,'client:subarx-api-wait',2,3,13,5,'codex-plus稳定-0.2x','gpt-5.6-sol','0.000055',25,5,'0.000125','0.000150','0','0','0.000055',?)`, now)
	if err != nil {
		t.Fatal(err)
	}
	rows, err := a.profitRows("2026-01-01T00:00:00Z", 10)
	if err != nil || len(rows) != 1 {
		t.Fatalf("rows=%v err=%v", rows, err)
	}
	if rows[0].Matched || rows[0].CostSource != "subarx_waiting" || rows[0].CostText != "" || rows[0].ProfitText != "" {
		t.Fatalf("recent Subarx API usage used rule fallback: %+v", rows[0])
	}
	totals, err := a.profitSummary("2026-01-01T00:00:00Z", "9999-12-31T00:00:00Z")
	if err != nil || totals.Matched != 0 || totals.Unmatched != 1 || !totals.MatchedCost.IsZero() {
		t.Fatalf("totals=%+v err=%v", totals, err)
	}
}

func TestSubarxAPIAccountKeepsRuleFallbackOutsideCoverage(t *testing.T) {
	a := testApp(t)
	a.cfg.SubarxAccountAPIKeys = map[int64]string{13: "test-key"}
	a.cfg.SubarxLookbackDays = 30
	a.cfg.SubarxTimezone = "Asia/Shanghai"
	configureTestRule(t, a, 13, "subarx", "", "0.1")
	created := time.Now().UTC().AddDate(0, 0, -45).Format(time.RFC3339Nano)
	_, err := a.db.Exec(`INSERT INTO usage_snapshots(source_id,request_id,user_id,api_key_id,account_id,model,actual_cost,input_cost,output_cost,cache_read_cost,cache_creation_cost,actual_cost_cny,created_at) VALUES(20,'client:subarx-api-history',2,3,13,'gpt-5.6-sol','0.000055','0.000125','0.000150','0','0','0.000055',?)`, created)
	if err != nil {
		t.Fatal(err)
	}
	rows, err := a.profitRows("2000-01-01T00:00:00Z", 10)
	if err != nil || len(rows) != 1 || !rows[0].Matched || rows[0].CostSource != "subarx_rule" {
		t.Fatalf("historical rule fallback was lost: rows=%+v err=%v", rows, err)
	}
}

func TestA6StagingRowNeverCreatesTemporaryDuplicate(t *testing.T) {
	a := testApp(t)
	created := "2026-08-15T02:55:14.065993Z"
	_, err := a.db.Exec(`INSERT INTO usage_snapshots(source_id,request_id,user_id,api_key_id,account_id,group_id,group_name,model,actual_cost,input_tokens,output_tokens,actual_cost_cny,created_at) VALUES(268014,'client:a6-staging',2,3,27,19,'claude-kiro高缓存-0.15x','claude-sonnet-5','0.00002',1,3,'0.00002',?)`, created)
	if err != nil {
		t.Fatal(err)
	}
	_, err = a.db.Exec(`INSERT INTO upstream_usage(provider,upstream_request_id,cost,currency,fx_rate_to_cny,cost_cny,occurred_at,model,input_tokens,output_tokens,cache_tokens,account_id_hint,match_method,source,imported_at) VALUES('a6','a6-staging-upstream','0.000002','USD','7','0.000014','2026-08-15T02:55:14Z','claude-sonnet-5',1,3,0,27,?,'a6_api','2026-08-15T02:58:40Z')`, a6StagingMatchMethod)
	if err != nil {
		t.Fatal(err)
	}

	before, err := a.profitRowsPage("2026-08-15T02:55:00Z", "2026-08-15T02:56:00Z", "all", 10, 0)
	if err != nil || len(before) != 1 || before[0].RecordType != "downstream" {
		t.Fatalf("staging A6 row leaked into dashboard: rows=%+v err=%v", before, err)
	}
	var count int
	if err := a.recordCount("2026-08-15T02:55:00Z", "2026-08-15T02:56:00Z", "all").Scan(&count); err != nil || count != 1 {
		t.Fatalf("record count exposed staging row: count=%d err=%v", count, err)
	}
	if err := a.reconcileUpstream(); err != nil {
		t.Fatal(err)
	}
	after, err := a.profitRowsPage("2026-08-15T02:55:00Z", "2026-08-15T02:56:00Z", "all", 10, 0)
	if err != nil || len(after) != 1 || !after[0].Matched || after[0].UpstreamRequestID != "a6-staging-upstream" {
		t.Fatalf("A6 row was not atomically presented after reconcile: rows=%+v err=%v", after, err)
	}
}

func TestSubarxChargeAlwaysRoundsUpToFiveDecimalPlaces(t *testing.T) {
	tests := map[string]string{
		"0":          "0",
		"0.000028":   "0.00003",
		"0.00026175": "0.00027",
		"0.00102350": "0.00103",
		"0.00202350": "0.00203",
		"0.00103":    "0.00103",
	}
	for input, want := range tests {
		got := subarxCharge(decimal.RequireFromString(input))
		if !got.Equal(decimal.RequireFromString(want)) {
			t.Fatalf("subarxCharge(%s)=%s want=%s", input, got, want)
		}
	}
}

func TestParseSubarxAccountMultipliers(t *testing.T) {
	got, err := parseSubarxAccountMultipliers("13:0.1, 14:0.2")
	if err != nil {
		t.Fatal(err)
	}
	if !got[13].Equal(decimal.RequireFromString("0.1")) || !got[14].Equal(decimal.RequireFromString("0.2")) || len(got) != 2 {
		t.Fatalf("map=%v", got)
	}
	if _, err := parseSubarxAccountMultipliers("13:nope"); err == nil {
		t.Fatal("expected invalid multiplier error")
	}
}

func TestMigrateRemovesEstimateColumnsAndResetsUsageCursor(t *testing.T) {
	dir := t.TempDir()
	db, err := sql.Open("sqlite", filepath.Join(dir, "old.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	db.SetMaxOpenConns(1)
	_, err = db.Exec(`CREATE TABLE state (key TEXT PRIMARY KEY, value TEXT NOT NULL);
CREATE TABLE usage_snapshots (source_id INTEGER PRIMARY KEY, request_id TEXT NOT NULL, user_id INTEGER NOT NULL, api_key_id INTEGER NOT NULL, account_id INTEGER NOT NULL, model TEXT NOT NULL, upstream_model TEXT NOT NULL DEFAULT '', actual_cost TEXT NOT NULL, input_tokens INTEGER NOT NULL DEFAULT 0, output_tokens INTEGER NOT NULL DEFAULT 0, cache_read_tokens INTEGER NOT NULL DEFAULT 0, cache_creation_tokens INTEGER NOT NULL DEFAULT 0, created_at TEXT NOT NULL);
INSERT INTO state(key,value) VALUES('usage_source_id','99');`)
	if err != nil {
		t.Fatal(err)
	}
	a := &app{cfg: config{Sub2APIUnitToCNY: decimal.NewFromInt(1)}, db: db}
	if err := a.migrate(); err != nil {
		t.Fatal(err)
	}
	if got := a.stateInt64("usage_source_id"); got != 0 {
		t.Fatalf("cursor=%d want=0", got)
	}
	rows, err := db.Query(`PRAGMA table_info(usage_snapshots)`)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	columns := map[string]bool{}
	for rows.Next() {
		var cid, notNull, primaryKey int
		var name, dataType string
		var defaultValue any
		if err := rows.Scan(&cid, &name, &dataType, &notNull, &defaultValue, &primaryKey); err != nil {
			t.Fatal(err)
		}
		columns[name] = true
	}
	if columns["estimated_cost"] || columns["estimated_cost_cny"] || columns["estimate_source"] || !columns["actual_cost_cny"] || !columns["input_tokens"] || !columns["user_email"] || !columns["group_id"] || !columns["group_name"] || !columns["input_cost"] || !columns["output_cost"] || !columns["cache_read_cost"] || !columns["cache_creation_cost"] {
		t.Fatalf("unexpected columns=%v", columns)
	}
}

func TestPricingContract(t *testing.T) {
	a := testApp(t)
	a.cfg.PricingFile = filepath.Join("..", "..", "config", "pricing.json")
	w := httptest.NewRecorder()
	a.pricing(w, httptest.NewRequest(http.MethodGet, "/api/provider/pricing", nil))
	if w.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", w.Code, w.Body.String())
	}
	var got struct {
		SchemaVersion string `json:"schema_version"`
		Success       bool   `json:"success"`
		Data          struct {
			SiteName   string           `json:"site_name"`
			SiteDomain string           `json:"site_domain"`
			Currency   string           `json:"currency"`
			PriceUnit  string           `json:"price_unit"`
			Models     []map[string]any `json:"models"`
		} `json:"data"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if !got.Success || got.SchemaVersion != "1.0" || got.Data.SiteName == "" || got.Data.SiteDomain == "" || len(got.Data.Models) != 6 {
		t.Fatalf("contract=%+v", got)
	}
	wantModels := map[string]bool{
		"gpt-5.4-mini":  true,
		"gpt-5.4":       true,
		"gpt-5.5":       true,
		"gpt-5.6-sol":   true,
		"gpt-5.6-luna":  true,
		"gpt-5.6-terra": true,
	}
	for _, model := range got.Data.Models {
		name, ok := model["model_name"].(string)
		if !ok || !wantModels[name] {
			t.Fatalf("unexpected model_name: %#v", model["model_name"])
		}
		if _, ok := model["input_price"].(float64); !ok {
			t.Fatalf("input_price must be a JSON number for %s: %#v", name, model["input_price"])
		}
		if _, ok := model["output_price"].(float64); !ok {
			t.Fatalf("output_price must be a JSON number for %s: %#v", name, model["output_price"])
		}
		if _, ok := model["cache_input_price"].(float64); !ok {
			t.Fatalf("cache_input_price must be a JSON number for %s: %#v", name, model["cache_input_price"])
		}
		cacheCreatePrice, hasCacheCreatePrice := model["cache_create_price"]
		if !hasCacheCreatePrice {
			t.Fatalf("cache_create_price must be present for %s", name)
		}
		if strings.HasPrefix(name, "gpt-5.6-") {
			if _, ok := cacheCreatePrice.(float64); !ok {
				t.Fatalf("cache_create_price must be a JSON number for %s: %#v", name, cacheCreatePrice)
			}
		} else if cacheCreatePrice != nil {
			t.Fatalf("cache_create_price must be null for %s: %#v", name, cacheCreatePrice)
		}
		cacheCreatePrice1H, hasCacheCreatePrice1H := model["cache_create_price_1h"]
		if !hasCacheCreatePrice1H || cacheCreatePrice1H != nil {
			t.Fatalf("cache_create_price_1h must be present and null for %s: %#v", name, cacheCreatePrice1H)
		}
		delete(wantModels, name)
	}
	if len(wantModels) != 0 {
		t.Fatalf("missing models: %v", wantModels)
	}
}
