package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/shopspring/decimal"
)

func TestSubarxUsageAPIAllocatesExactDailyModelCostAndPreservesOldSnapshotOnFailure(t *testing.T) {
	a := testApp(t)
	a.cfg.SubarxTimezone = "Asia/Shanghai"
	a.cfg.SubarxLookbackDays = 90
	a.cfg.SubarxSyncInterval = 0
	a.cfg.SubarxHTTPTimeout = 5e9
	a.cfg.SubarxAccountAPIKeys = map[int64]string{28: "test-key"}
	insertSubarxUsage(t, a, 1, 28, "gpt-5.6-sol", "0.01", "2026-08-13T04:00:00Z")
	insertSubarxUsage(t, a, 2, 28, "gpt-5.6-sol", "0.02", "2026-08-13T05:00:00Z")

	failDaily := false
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer test-key" {
			t.Errorf("authorization header was not set")
		}
		switch r.URL.Path {
		case "/v1/sub2api/billing":
			fmt.Fprint(w, `{"schema_version":"1","billing_scope":"api_key","group_rate_multiplier":0.13,"resolved_rate_multiplier":0.13,"peak_rate_enabled":false,"effective_rate_multiplier":0.13,"observed_at":"2026-08-13T06:00:00Z"}`)
		case "/v1/usage":
			if r.URL.Query().Get("start_date") == "" {
				fmt.Fprint(w, `{"isValid":true,"unit":"USD","daily_usage":[{"date":"2026-08-13","requests":2,"input_tokens":30,"output_tokens":3,"cache_read_tokens":0,"cache_write_tokens":0,"total_tokens":33,"cost":0.03,"actual_cost":0.003}],"model_stats":[]}`)
				return
			}
			if failDaily {
				http.Error(w, "temporary failure", http.StatusBadGateway)
				return
			}
			fmt.Fprint(w, `{"isValid":true,"unit":"USD","daily_usage":[{"date":"2026-08-15","requests":999,"input_tokens":999,"output_tokens":999,"cache_read_tokens":999,"cache_write_tokens":999,"total_tokens":3996,"cost":9.99,"actual_cost":0.999}],"model_stats":[{"model":"gpt-5.6-sol","requests":2,"input_tokens":30,"output_tokens":3,"cache_creation_tokens":0,"cache_read_tokens":0,"total_tokens":33,"cost":0.03,"actual_cost":0.003,"account_cost":0.02}]}`)
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()
	a.cfg.SubarxBaseURL = server.URL

	if err := a.collectSubarx(true); err != nil {
		t.Fatal(err)
	}
	assertSubarxAllocationTotals(t, a, 28, "2026-08-13", "0.003", "0", 2)
	if !a.hasSubarxDailySnapshot(28, "2026-08-13") {
		t.Fatal("fully allocated historical day should be settled")
	}
	var multiplier string
	if err := a.db.QueryRow(`SELECT multiplier FROM upstream_account_rules WHERE account_id=28 AND provider='subarx'`).Scan(&multiplier); err != nil {
		t.Fatal(err)
	}
	if multiplier != "0.13" {
		t.Fatalf("billing multiplier=%s", multiplier)
	}
	rows, err := a.profitRows("2026-01-01T00:00:00Z", 10)
	if err != nil || len(rows) != 2 {
		t.Fatalf("rows=%+v err=%v", rows, err)
	}
	for _, row := range rows {
		if !row.Matched || row.CostSource != "subarx_billed_allocation" || row.CostSourceLabel != "已对账" {
			t.Fatalf("unexpected row=%+v", row)
		}
	}

	if err := a.collectSubarx(true); err != nil {
		t.Fatal(err)
	}
	assertSubarxAllocationTotals(t, a, 28, "2026-08-13", "0.003", "0", 2)

	failDaily = true
	expected := subarxDailyUsage{Date: "2026-08-13", Requests: 2, Cost: json.Number("0.03"), ActualCost: json.Number("0.003")}
	if err := a.collectSubarxDay(28, "test-key", expected); err == nil {
		t.Fatal("expected daily API failure")
	}
	assertSubarxAllocationTotals(t, a, 28, "2026-08-13", "0.003", "0", 2)
}

func TestSubarxPartialCoverageKeepsExactUnallocatedDifference(t *testing.T) {
	a := testApp(t)
	a.cfg.SubarxTimezone = "Asia/Shanghai"
	insertSubarxUsage(t, a, 1, 28, "gpt-5.6-sol", "0.01", "2026-08-13T04:00:00Z")
	insertSubarxUsage(t, a, 2, 28, "gpt-5.6-sol", "0.02", "2026-08-13T05:00:00Z")
	models := []subarxModelUsage{{
		Model: "gpt-5.6-sol", Requests: 3, InputTokens: 60, OutputTokens: 6,
		Cost: json.Number("0.06"), ActualCost: json.Number("0.006"), AccountCost: json.Number("0.04"),
	}}
	if err := a.storeSubarxDay(28, subarxDailyUsage{Date: "2026-08-13"}, models); err != nil {
		t.Fatal(err)
	}
	assertSubarxAllocationTotals(t, a, 28, "2026-08-13", "0.003", "0.003", 3)
	var partial int
	if err := a.db.QueryRow(`SELECT COUNT(*) FROM subarx_usage_scope_status WHERE allocation_status='allocated_partial'`).Scan(&partial); err != nil {
		t.Fatal(err)
	}
	if partial != 2 {
		t.Fatalf("partial statuses=%d", partial)
	}
	if !a.hasSubarxDailySnapshot(28, "2026-08-13") {
		t.Fatal("valid partial coverage should be settled with an explicit difference")
	}
}

func TestCollectSubarxDayUsesFreshMatchingDailyUsage(t *testing.T) {
	a := testApp(t)
	a.cfg.SubarxTimezone = "Asia/Shanghai"
	a.cfg.SubarxHTTPTimeout = 5e9
	insertSubarxUsage(t, a, 1, 28, "gpt-5.6-sol", "0.01", "2026-08-13T04:00:00Z")

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, `{"isValid":true,"unit":"USD","daily_usage":[{"date":"2026-08-13","requests":2,"input_tokens":20,"output_tokens":2,"cache_read_tokens":0,"cache_write_tokens":0,"total_tokens":22,"cost":0.02,"actual_cost":0.002}],"model_stats":[{"model":"gpt-5.6-sol","requests":2,"input_tokens":20,"output_tokens":2,"cache_creation_tokens":0,"cache_read_tokens":0,"total_tokens":22,"cost":0.02,"actual_cost":0.002,"account_cost":0.02}]}`)
	}))
	defer server.Close()
	a.cfg.SubarxBaseURL = server.URL

	expected := subarxDailyUsage{
		Date: "2026-08-13", Requests: 1, InputTokens: 10, OutputTokens: 1,
		Cost: json.Number("0.01"), ActualCost: json.Number("0.001"),
	}
	if err := a.collectSubarxDay(28, "test-key", expected); err != nil {
		t.Fatal(err)
	}
	assertSubarxAllocationTotals(t, a, 28, "2026-08-13", "0.001", "0.001", 2)
	var requests int64
	if err := a.db.QueryRow(`SELECT SUM(requests) FROM subarx_daily_usage WHERE account_id=28 AND usage_date='2026-08-13'`).Scan(&requests); err != nil {
		t.Fatal(err)
	}
	if requests != 2 {
		t.Fatalf("requests=%d", requests)
	}
}

func TestSubarxInconsistentScopeSuppressesRuleFallback(t *testing.T) {
	a := testApp(t)
	a.cfg.SubarxTimezone = "Asia/Shanghai"
	configureTestRule(t, a, 13, "subarx", "", "0.1")
	insertSubarxUsage(t, a, 1, 13, "gpt-5.6-sol", "0.01", "2026-08-13T04:00:00Z")
	insertSubarxUsage(t, a, 2, 13, "gpt-5.6-sol", "0.02", "2026-08-13T05:00:00Z")
	models := []subarxModelUsage{{
		Model: "gpt-5.6-sol", Requests: 1, InputTokens: 10, OutputTokens: 1,
		Cost: json.Number("0.01"), ActualCost: json.Number("0.001"), AccountCost: json.Number("0.008"),
	}}
	if err := a.storeSubarxDay(13, subarxDailyUsage{Date: "2026-08-13"}, models); err != nil {
		t.Fatal(err)
	}
	rows, err := a.profitRows("2026-01-01T00:00:00Z", 10)
	if err != nil || len(rows) != 2 {
		t.Fatalf("rows=%+v err=%v", rows, err)
	}
	for _, row := range rows {
		if row.Matched || row.CostSource != "subarx_pending" || row.CostText != "" || row.ProfitText != "" {
			t.Fatalf("rule fallback was not suppressed: %+v", row)
		}
	}
	assertSubarxAllocationTotals(t, a, 13, "2026-08-13", "0", "0.001", 1)
	if a.hasSubarxDailySnapshot(13, "2026-08-13") {
		t.Fatal("inconsistent historical day must remain eligible for refresh")
	}
}

func TestParseSubarxAccountAPIKeys(t *testing.T) {
	got, err := parseSubarxAccountAPIKeys("28:key-a, 29:key-b")
	if err != nil {
		t.Fatal(err)
	}
	if got[28] != "key-a" || got[29] != "key-b" || len(got) != 2 {
		t.Fatalf("keys=%v", got)
	}
	for _, raw := range []string{"28", "nope:key", "28:", "28:a,28:b"} {
		if _, err := parseSubarxAccountAPIKeys(raw); err == nil {
			t.Fatalf("expected invalid mapping for %q", raw)
		}
	}
}

func TestValidateSubarxDayRejectsIncoherentModelTotals(t *testing.T) {
	daily := subarxDailyUsage{Requests: 2, Cost: json.Number("0.03"), ActualCost: json.Number("0.003")}
	models := []subarxModelUsage{{Model: "gpt", Requests: 1, Cost: json.Number("0.03"), ActualCost: json.Number("0.003")}}
	if err := validateSubarxDay(daily, models); err == nil {
		t.Fatal("expected request total mismatch")
	}
}

func insertSubarxUsage(t *testing.T, a *app, sourceID, accountID int64, model, standardCost, createdAt string) {
	t.Helper()
	_, err := a.db.Exec(`INSERT INTO usage_snapshots(source_id,request_id,user_id,api_key_id,account_id,model,actual_cost,input_tokens,output_tokens,input_cost,actual_cost_cny,created_at) VALUES(?,?,?,?,?,?,?,?,?,?,?,?)`, sourceID, fmt.Sprintf("client:subarx-%d", sourceID), 1, 1, accountID, model, "1", 10, 1, standardCost, "1", createdAt)
	if err != nil {
		t.Fatal(err)
	}
}

func assertSubarxAllocationTotals(t *testing.T, a *app, accountID int64, usageDate, wantAllocated, wantUnallocated string, wantRecords int) {
	t.Helper()
	rows, err := a.db.Query(`SELECT cost,usage_source_id IS NULL FROM upstream_usage WHERE provider='subarx' AND source='subarx_usage_api' AND account_id_hint=? AND billing_date=?`, accountID, usageDate)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	allocated := decimal.Zero
	unallocated := decimal.Zero
	count := 0
	for rows.Next() {
		var raw string
		var orphan bool
		if err := rows.Scan(&raw, &orphan); err != nil {
			t.Fatal(err)
		}
		value := decimal.RequireFromString(raw)
		if orphan {
			unallocated = unallocated.Add(value)
		} else {
			allocated = allocated.Add(value)
		}
		count++
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	if !allocated.Equal(decimal.RequireFromString(wantAllocated)) || !unallocated.Equal(decimal.RequireFromString(wantUnallocated)) || count != wantRecords {
		t.Fatalf("allocated=%s unallocated=%s records=%d", allocated, unallocated, count)
	}
}
