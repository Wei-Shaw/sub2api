package main

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/shopspring/decimal"
)

func TestParseA6TokenAccountMap(t *testing.T) {
	got, err := parseA6TokenAccountMap("0.06:17, 0.12:18")
	if err != nil {
		t.Fatal(err)
	}
	if got["0.06"] != 17 || got["0.12"] != 18 || len(got) != 2 {
		t.Fatalf("map=%v", got)
	}
	if _, err := parseA6TokenAccountMap("0.06:not-an-id"); err == nil {
		t.Fatal("expected invalid account id error")
	}
}

func TestA6BackfillSplitsLargeRangesAndPersistsProgress(t *testing.T) {
	start := time.Date(2026, 7, 1, 0, 0, 0, 0, time.UTC)
	end := start.Add(2 * time.Hour)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/api/status":
			fmt.Fprint(w, `{"success":true,"data":{"quota_per_unit":500000}}`)
		case "/api/log/self":
			fromUnix, _ := strconv.ParseInt(r.URL.Query().Get("start_timestamp"), 10, 64)
			toUnix, _ := strconv.ParseInt(r.URL.Query().Get("end_timestamp"), 10, 64)
			if time.Duration(toUnix-fromUnix)*time.Second > time.Hour {
				fmt.Fprint(w, `{"success":true,"data":{"page":1,"page_size":1,"total":2,"items":[]}}`)
				return
			}
			requestID := fmt.Sprintf("history-%d", fromUnix)
			occurred := fromUnix + (toUnix-fromUnix)/2
			fmt.Fprintf(w, `{"success":true,"data":{"page":1,"page_size":1,"total":1,"items":[{"created_at":%d,"request_id":"%s","token_name":"0.06","model_name":"gpt-5.5","prompt_tokens":10,"completion_tokens":2,"quota":5,"other":"{}"}]}}`, occurred, requestID)
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	a := testApp(t)
	a.cfg.A6BaseURL = server.URL
	a.cfg.A6UserID = "1233"
	a.cfg.A6AccessToken = "test-system-token"
	configureTestRule(t, a, 17, "a6", "0.06", "")
	a.cfg.A6USDToCNY = decimal.RequireFromString("7.2")
	a.cfg.A6HTTPTimeout = 5 * time.Second
	a.cfg.A6PageSize = 1
	a.cfg.A6MaxPages = 1
	a.cfg.A6BackfillChunk = 2 * time.Hour
	a.cfg.A6BackfillPageDelay = 0
	a.backfillRunning = true
	a.runA6Backfill(start, end, 0)

	if got := a.stateString(a6BackfillStatusKey); got != "completed" {
		t.Fatalf("status=%q error=%q", got, a.stateString(a6BackfillErrorKey))
	}
	if got := a.stateString(a6BackfillProcessedKey); got != "2" {
		t.Fatalf("processed=%q", got)
	}
	var count int
	if err := a.db.QueryRow(`SELECT COUNT(*) FROM upstream_usage WHERE source='a6_api'`).Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 2 {
		t.Fatalf("stored history rows=%d want=2", count)
	}

	a.backfillRunning = true
	a.runA6Backfill(start, end, 0)
	if got := a.stateString(a6BackfillProcessedKey); got != "0" {
		t.Fatalf("idempotent rerun processed=%q want=0", got)
	}
	if err := a.db.QueryRow(`SELECT COUNT(*) FROM upstream_usage WHERE source='a6_api'`).Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 2 {
		t.Fatalf("idempotent rerun stored history rows=%d want=2", count)
	}
}

func TestCollectA6MatchesRealBillingShape(t *testing.T) {
	created := time.Now().UTC().Add(-time.Minute).Truncate(time.Second)
	var sawAuth bool
	var logRequests int
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") == "Bearer test-system-token" && r.Header.Get("New-API-User") == "1233" {
			sawAuth = true
		}
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/api/status":
			fmt.Fprint(w, `{"success":true,"data":{"quota_per_unit":500000}}`)
		case "/api/log/self":
			logRequests++
			if r.URL.Query().Get("token_name") != "0.06" {
				t.Fatalf("token_name=%q", r.URL.Query().Get("token_name"))
			}
			fmt.Fprintf(w, `{"success":true,"data":{"page":1,"page_size":100,"total":1,"items":[{"created_at":%d,"request_id":"a6-request-1","token_name":"0.06","group":"default","model_name":"gpt-5.4-mini","prompt_tokens":4390,"completion_tokens":5,"quota":12,"channel":1469,"channel_name":"self-z2","other":"{\"cache_tokens\":3840,\"model_ratio\":0.0126,\"cache_ratio\":0.1,\"completion_ratio\":6,\"group_ratio\":1,\"billing_source\":\"wallet\",\"request_path\":\"/v1/responses\"}"}]}}`, created.Add(4*time.Second).Unix())
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	a := testApp(t)
	a.cfg.A6BaseURL = server.URL
	a.cfg.A6UserID = "1233"
	a.cfg.A6AccessToken = "test-system-token"
	configureTestRule(t, a, 17, "a6", "0.06", "")
	a.cfg.A6USDToCNY = decimal.RequireFromString("7.2")
	a.cfg.A6BootstrapWindow = 24 * time.Hour
	a.cfg.A6SyncInterval = 5 * time.Minute
	a.cfg.A6HTTPTimeout = 5 * time.Second
	a.cfg.A6PageSize = 100
	a.cfg.A6MaxPages = 2

	_, err := a.db.Exec(`INSERT INTO usage_snapshots(source_id,request_id,user_id,api_key_id,account_id,model,actual_cost,input_tokens,output_tokens,cache_read_tokens,source_currency,fx_rate_to_cny,actual_cost_cny,created_at)
VALUES(1,'client:local-1',1,1,17,'gpt-5.4-mini','0.00004338',550,5,3840,'PLATFORM_UNIT','1','0.00004338',?)`, created.Format(time.RFC3339Nano))
	if err != nil {
		t.Fatal(err)
	}
	if err := a.collectA6(true); err != nil {
		t.Fatal(err)
	}
	if err := a.collectA6(false); err != nil {
		t.Fatal(err)
	}
	if logRequests != 1 {
		t.Fatalf("log requests=%d want=1 within sync interval", logRequests)
	}
	if err := a.reconcileUpstream(); err != nil {
		t.Fatal(err)
	}
	if !sawAuth {
		t.Fatal("A6 authentication headers were not sent")
	}

	var cost, costCNY, tokenName, matchMethod string
	var sourceID, cacheTokens, channelID int64
	err = a.db.QueryRow(`SELECT cost,cost_cny,token_name,match_method,usage_source_id,cache_tokens,channel_id FROM upstream_usage WHERE provider='a6' AND upstream_request_id='a6-request-1'`).Scan(&cost, &costCNY, &tokenName, &matchMethod, &sourceID, &cacheTokens, &channelID)
	if err != nil {
		t.Fatal(err)
	}
	if cost != "0.000024" || costCNY != "0.0001728" || tokenName != "0.06" || matchMethod != "composite_account_model_tokens_time" || sourceID != 1 || cacheTokens != 3840 || channelID != 1469 {
		t.Fatalf("stored cost=%q cny=%q token=%q match=%q source=%d cache=%d channel=%d", cost, costCNY, tokenName, matchMethod, sourceID, cacheTokens, channelID)
	}
	rows, err := a.profitRows(created.Add(-time.Minute).Format(time.RFC3339Nano), 10)
	if err != nil || len(rows) != 1 {
		t.Fatalf("rows=%v err=%v", rows, err)
	}
	if !rows[0].Matched || rows[0].CostSource != "billed" || rows[0].RevenueText != "0.00004338" || rows[0].BilledCostText != "0.00017280" || rows[0].UpstreamCostText != "0.00002400" || rows[0].UpstreamCurrency != "USD" || rows[0].InputTokens != 550 || rows[0].OutputTokens != 5 || rows[0].CacheTokens != 3840 || rows[0].ProfitText != "-0.00012942" {
		t.Fatalf("row=%+v", rows[0])
	}
}

func TestCollectA6HistoricalTokenUsesBoundedLookback(t *testing.T) {
	a := testApp(t)
	created := time.Now().UTC().Add(-72 * time.Hour)
	_, err := a.db.Exec(`INSERT INTO usage_snapshots(source_id,request_id,user_id,api_key_id,account_id,model,actual_cost,actual_cost_cny,created_at,rule_provider,rule_external_key,rule_version)
VALUES(1,'client:old-token',1,1,15,'gpt-5.6-sol','1','1',?,'a6','0.12',1)`, created.Format(time.RFC3339Nano))
	if err != nil {
		t.Fatal(err)
	}
	a.cfg.A6BaseURL = ""
	a.cfg.A6UserID = "1233"
	a.cfg.A6AccessToken = "test-system-token"
	a.cfg.A6USDToCNY = decimal.RequireFromString("7.2")
	a.cfg.A6BootstrapWindow = 24 * time.Hour
	a.cfg.A6HistoricalTokenLookback = 48 * time.Hour
	a.cfg.A6HTTPTimeout = 5 * time.Second
	a.cfg.A6PageSize = 100
	a.cfg.A6MaxPages = 2
	var gotStart int64
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/api/status":
			fmt.Fprint(w, `{"success":true,"data":{"quota_per_unit":500000}}`)
		case "/api/log/self":
			gotStart, _ = strconv.ParseInt(r.URL.Query().Get("start_timestamp"), 10, 64)
			fmt.Fprint(w, `{"success":true,"data":{"page":1,"page_size":100,"total":0,"items":[]}}`)
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()
	a.cfg.A6BaseURL = server.URL

	if err := a.collectA6(true); err != nil {
		t.Fatal(err)
	}
	if gotStart <= time.Now().UTC().Add(-49*time.Hour).Unix() || gotStart >= time.Now().UTC().Unix() {
		t.Fatalf("historical start timestamp=%d was not bounded to the configured lookback", gotStart)
	}
	if got := a.stateString("a6_bootstrap_done:0.12"); got != "1" {
		t.Fatalf("historical bootstrap state=%q", got)
	}
}

func TestCollectA6ContinuesAfterTokenError(t *testing.T) {
	a := testApp(t)
	configureTestRule(t, a, 15, "a6", "openai-0.12-95%", "")
	created := time.Now().UTC().Add(-72 * time.Hour)
	_, err := a.db.Exec(`INSERT INTO usage_snapshots(source_id,request_id,user_id,api_key_id,account_id,model,actual_cost,actual_cost_cny,created_at,rule_provider,rule_external_key,rule_version)
VALUES(1,'client:old-token',1,1,15,'gpt-5.6-sol','1','1',?,'a6','0.12',1)`, created.Format(time.RFC3339Nano))
	if err != nil {
		t.Fatal(err)
	}
	a.cfg.A6UserID = "1233"
	a.cfg.A6AccessToken = "test-system-token"
	a.cfg.A6USDToCNY = decimal.RequireFromString("7.2")
	a.cfg.A6BootstrapWindow = 24 * time.Hour
	a.cfg.A6HistoricalTokenLookback = 48 * time.Hour
	a.cfg.A6HTTPTimeout = 5 * time.Second
	a.cfg.A6PageSize = 100
	a.cfg.A6MaxPages = 2
	currentCreated := time.Now().UTC().Add(-time.Minute).Truncate(time.Second)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/api/status":
			fmt.Fprint(w, `{"success":true,"data":{"quota_per_unit":500000}}`)
		case "/api/log/self":
			if r.URL.Query().Get("token_name") == "0.12" {
				http.Error(w, "historical token unavailable", http.StatusBadGateway)
				return
			}
			fmt.Fprintf(w, `{"success":true,"data":{"page":1,"page_size":100,"total":1,"items":[{"created_at":%d,"request_id":"new-token-bill","token_name":"openai-0.12-95%%","model_name":"gpt-5.6-sol","prompt_tokens":10,"completion_tokens":2,"quota":5,"other":"{}"}]}}`, currentCreated.Unix())
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()
	a.cfg.A6BaseURL = server.URL

	err = a.collectA6(true)
	if err == nil || !strings.Contains(err.Error(), `token "0.12"`) {
		t.Fatalf("expected isolated historical token error, got %v", err)
	}
	var count int
	if err := a.db.QueryRow(`SELECT COUNT(*) FROM upstream_usage WHERE upstream_request_id='new-token-bill'`).Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 1 {
		t.Fatalf("successful token was not collected after another token failed: count=%d", count)
	}
}

func TestMatchUpstreamRecordAllowsUniqueOneTokenCacheDifference(t *testing.T) {
	a := testApp(t)
	created := time.Date(2026, 8, 14, 18, 5, 5, 554311000, time.UTC)
	_, err := a.db.Exec(`INSERT INTO usage_snapshots(source_id,request_id,user_id,api_key_id,account_id,model,actual_cost,input_tokens,output_tokens,cache_read_tokens,source_currency,fx_rate_to_cny,actual_cost_cny,created_at)
VALUES(1,'client:claude-1',1,1,27,'claude-sonnet-5','0.01',5,1,63,'PLATFORM_UNIT','1','0.01',?)`, created.Format(time.RFC3339Nano))
	if err != nil {
		t.Fatal(err)
	}

	sourceID, method, err := a.matchUpstreamRecord("a6-claude-1", created.Truncate(time.Second).Format(time.RFC3339Nano), "claude-sonnet-5", 5, 1, 62, 27)
	if err != nil {
		t.Fatal(err)
	}
	if sourceID != 1 || method != "composite_account_model_tokens_time_cache_tolerance" {
		t.Fatalf("source=%d method=%q", sourceID, method)
	}
}

func TestMatchUpstreamRecordUsesA6CacheReadTokens(t *testing.T) {
	a := testApp(t)
	configureTestRule(t, a, 27, "a6", "anthropic-0.15-95%", "")
	created := time.Date(2026, 8, 20, 13, 8, 42, 684953000, time.UTC)
	_, err := a.db.Exec(`INSERT INTO usage_snapshots(source_id,request_id,user_id,api_key_id,account_id,model,actual_cost,input_tokens,output_tokens,cache_read_tokens,cache_creation_tokens,source_currency,fx_rate_to_cny,actual_cost_cny,created_at)
VALUES(1,'client:claude-cache-read',1,1,27,'claude-sonnet-5','0.01',56,1,22,36,'PLATFORM_UNIT','1','0.01',?)`, created.Format(time.RFC3339Nano))
	if err != nil {
		t.Fatal(err)
	}
	_, err = a.db.Exec(`INSERT INTO upstream_usage(provider,upstream_request_id,cost,currency,fx_rate_to_cny,cost_cny,occurred_at,model,input_tokens,output_tokens,cache_tokens,token_name,source,imported_at)
VALUES('a6','a6-claude-cache-read','0.01','USD','7.2','0.072',?,'claude-sonnet-5',56,1,22,'anthropic-0.15-95%','test',?)`, created.Truncate(time.Second).Format(time.RFC3339Nano), created.Format(time.RFC3339Nano))
	if err != nil {
		t.Fatal(err)
	}

	sourceID, method, err := a.matchUpstreamRecordForToken("a6-claude-cache-read", created.Truncate(time.Second).Format(time.RFC3339Nano), "claude-sonnet-5", 56, 1, 22, 27, "anthropic-0.15-95%")
	if err != nil {
		t.Fatal(err)
	}
	if sourceID != 1 || method != "composite_account_model_tokens_time_cache_read" {
		t.Fatalf("source=%d method=%q", sourceID, method)
	}
}

func TestMatchUpstreamRecordRejectsAmbiguousCacheTolerance(t *testing.T) {
	a := testApp(t)
	occurred := time.Date(2026, 8, 14, 18, 5, 5, 0, time.UTC)
	for sourceID, created := range []time.Time{occurred.Add(-300 * time.Millisecond), occurred.Add(300 * time.Millisecond)} {
		_, err := a.db.Exec(`INSERT INTO usage_snapshots(source_id,request_id,user_id,api_key_id,account_id,model,actual_cost,input_tokens,output_tokens,cache_read_tokens,source_currency,fx_rate_to_cny,actual_cost_cny,created_at)
VALUES(?,?,?,?,27,'claude-sonnet-5','0.01',5,1,63,'PLATFORM_UNIT','1','0.01',?)`, sourceID+1, fmt.Sprintf("client:claude-%d", sourceID+1), sourceID+1, sourceID+1, created.Format(time.RFC3339Nano))
		if err != nil {
			t.Fatal(err)
		}
	}

	sourceID, method, err := a.matchUpstreamRecord("a6-claude-ambiguous", occurred.Format(time.RFC3339Nano), "claude-sonnet-5", 5, 1, 62, 27)
	if err != nil {
		t.Fatal(err)
	}
	if sourceID != 0 || method != "" {
		t.Fatalf("ambiguous cache tolerance matched source=%d method=%q", sourceID, method)
	}
}
