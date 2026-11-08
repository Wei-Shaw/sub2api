package main

import (
	"bytes"
	"database/sql"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"
	"time"

	"github.com/shopspring/decimal"
)

func attachCurrentAccountTopology(t *testing.T, a *app) *sql.DB {
	t.Helper()
	source, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	source.SetMaxOpenConns(1)
	_, err = source.Exec(`
CREATE TABLE groups (id INTEGER PRIMARY KEY,name TEXT NOT NULL,deleted_at TEXT);
CREATE TABLE accounts (id INTEGER PRIMARY KEY,name TEXT NOT NULL,platform TEXT NOT NULL,status TEXT NOT NULL,schedulable INTEGER NOT NULL,deleted_at TEXT);
CREATE TABLE account_groups (account_id INTEGER NOT NULL,group_id INTEGER NOT NULL,priority INTEGER NOT NULL);`)
	if err != nil {
		source.Close()
		t.Fatal(err)
	}
	a.source = source
	t.Cleanup(func() { source.Close() })
	return source
}

func TestAccountRulesDetectUnconfiguredAndHotReload(t *testing.T) {
	a := testApp(t)
	now := time.Now().UTC()
	_, err := a.db.Exec(`INSERT INTO usage_snapshots(source_id,request_id,user_id,user_email,api_key_id,account_id,group_id,group_name,model,actual_cost,input_cost,output_cost,actual_cost_cny,created_at) VALUES(1,'client:new-account',1,'masked@local',1,20,15,'claude-kiro','claude-sonnet-5','1','0.01','0.02','1',?)`, now.Format(time.RFC3339Nano))
	if err != nil {
		t.Fatal(err)
	}

	from := now.Add(-time.Hour).Format(time.RFC3339)
	to := now.Add(time.Hour).Format(time.RFC3339)
	w := httptest.NewRecorder()
	a.accountRules(w, httptest.NewRequest(http.MethodGet, "/ops/api/account-rules?from="+from+"&to="+to, nil))
	if w.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", w.Code, w.Body.String())
	}
	var before struct {
		Unconfigured int               `json:"unconfigured_accounts"`
		Items        []accountRuleView `json:"items"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &before); err != nil {
		t.Fatal(err)
	}
	if before.Unconfigured != 1 || len(before.Items) != 1 || before.Items[0].Configured {
		t.Fatalf("unexpected detection: %+v", before)
	}

	request := httptest.NewRequest(http.MethodPut, "/ops/api/account-rules/20", bytes.NewBufferString(`{"provider":"subarx","multiplier":"0.1"}`))
	request.SetPathValue("account_id", "20")
	w = httptest.NewRecorder()
	a.putAccountRule(w, request)
	if w.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", w.Code, w.Body.String())
	}
	_, subarxRules, providers := a.accountRuleSnapshot()
	if !subarxRules[20].Equal(decimal.RequireFromString("0.1")) || providers[20] != "subarx" {
		t.Fatalf("hot reload failed: rules=%v providers=%v", subarxRules, providers)
	}
	rows, err := a.profitRows("2000-01-01T00:00:00Z", 10)
	if err != nil || len(rows) != 1 || rows[0].CostSource != "subarx_rule" {
		t.Fatalf("rule was not applied: rows=%+v err=%v", rows, err)
	}
}

func TestAccountRuleValidation(t *testing.T) {
	if _, err := validateAccountRule(21, accountRuleInput{Provider: "a6"}); err == nil {
		t.Fatal("expected missing A6 token_name error")
	}
	if _, err := validateAccountRule(20, accountRuleInput{Provider: "subarx", Multiplier: "0"}); err == nil {
		t.Fatal("expected invalid Subarx multiplier error")
	}
	if rule, err := validateAccountRule(21, accountRuleInput{Provider: "A6", TokenName: "2.0-kimik3"}); err != nil || rule.ExternalKey != "2.0-kimik3" {
		t.Fatalf("rule=%+v err=%v", rule, err)
	}
}

func TestAccountRuleAllowsDuplicateA6TokenAcrossAccounts(t *testing.T) {
	a := testApp(t)
	for _, accountID := range []int64{21, 22} {
		request := httptest.NewRequest(http.MethodPut, "/ops/api/account-rules/"+strconv.FormatInt(accountID, 10), bytes.NewBufferString(`{"provider":"a6","token_name":"shared-token"}`))
		request.SetPathValue("account_id", strconv.FormatInt(accountID, 10))
		w := httptest.NewRecorder()
		a.putAccountRule(w, request)
		if w.Code != http.StatusOK {
			t.Fatalf("account %d status=%d body=%s", accountID, w.Code, w.Body.String())
		}
	}
	accounts := a.a6TokenAccountsSnapshot()["shared-token"]
	if len(accounts) != 2 || accounts[0] != 21 || accounts[1] != 22 {
		t.Fatalf("shared token accounts=%v", accounts)
	}
}

func TestDuplicateA6TokenMatchesWithinAllConfiguredAccounts(t *testing.T) {
	a := testApp(t)
	configureTestRule(t, a, 21, "a6", "shared-token", "")
	configureTestRule(t, a, 22, "a6", "shared-token", "")
	created := time.Now().UTC().Truncate(time.Second)
	for _, accountID := range []int64{21, 22} {
		model := "model"
		if accountID == 22 {
			model = "other"
		}
		_, err := a.db.Exec(`INSERT INTO usage_snapshots(source_id,request_id,user_id,api_key_id,account_id,model,input_tokens,output_tokens,cache_read_tokens,actual_cost,actual_cost_cny,created_at,rule_provider,rule_external_key,rule_version,rule_snapshot_at)
VALUES(?,?,?,?,?,?,?,?,?,?,?,?,?,?,1,?)`, accountID, "client:shared-"+strconv.FormatInt(accountID, 10), 1, 1, accountID, model, 10, 2, 0, "1", "1", created.Format(time.RFC3339Nano), "a6", "shared-token", created.Format(time.RFC3339Nano))
		if err != nil {
			t.Fatal(err)
		}
	}
	matched, method, err := a.matchUpstreamRecordForToken("a6-shared", created.Format(time.RFC3339Nano), "model", 10, 2, 0, 0, "shared-token")
	if err != nil || matched != 21 || method == "" {
		t.Fatalf("duplicate token matching failed: source=%d method=%q err=%v", matched, method, err)
	}
}

func TestRenamedA6TokenMatchesHistoricalRuleSnapshot(t *testing.T) {
	a := testApp(t)
	configureTestRule(t, a, 15, "a6", "0.12", "")
	created := time.Date(2026, 8, 20, 10, 0, 0, 0, time.UTC)
	_, err := a.db.Exec(`INSERT INTO usage_snapshots(source_id,request_id,user_id,api_key_id,account_id,model,input_tokens,output_tokens,cache_read_tokens,actual_cost,actual_cost_cny,created_at,rule_provider,rule_external_key,rule_version,rule_snapshot_at)
VALUES(?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)`, 1, "client:renamed-token", 1, 1, 15, "gpt-5.6-sol", 10, 2, 0, "1", "1", created.Format(time.RFC3339Nano), "a6", "0.12", 1, created.Format(time.RFC3339Nano))
	if err != nil {
		t.Fatal(err)
	}
	configureTestRule(t, a, 15, "a6", "openai-0.12-95%", "")
	_, err = a.db.Exec(`INSERT INTO upstream_usage(provider,upstream_request_id,cost,currency,fx_rate_to_cny,cost_cny,occurred_at,model,input_tokens,output_tokens,cache_tokens,token_name,account_id_hint,source,imported_at)
VALUES('a6','a6-renamed-token','0.1','USD','7.2','0.72',?,?,?,?,?,?,15,'a6_api',?)`, created.Format(time.RFC3339Nano), "gpt-5.6-sol", 10, 2, 0, "0.12", created.Format(time.RFC3339Nano))
	if err != nil {
		t.Fatal(err)
	}
	if err := a.reconcileUpstream(); err != nil {
		t.Fatal(err)
	}
	var sourceID int64
	var method string
	if err := a.db.QueryRow(`SELECT usage_source_id,match_method FROM upstream_usage WHERE upstream_request_id='a6-renamed-token'`).Scan(&sourceID, &method); err != nil {
		t.Fatal(err)
	}
	if sourceID != 1 || method != "composite_account_model_tokens_time" {
		t.Fatalf("historical token snapshot was not matched: source=%d method=%q", sourceID, method)
	}
}

func TestOldA6TokenIncludesAccountsSharingTheRenamedToken(t *testing.T) {
	a := testApp(t)
	created := time.Date(2026, 8, 20, 10, 0, 0, 0, time.UTC)
	configureTestRule(t, a, 15, "a6", "0.12", "")
	_, err := a.db.Exec(`INSERT INTO usage_snapshots(source_id,request_id,user_id,api_key_id,account_id,model,input_tokens,output_tokens,actual_cost,actual_cost_cny,created_at,rule_provider,rule_external_key,rule_version,rule_snapshot_at)
VALUES(?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)`, 1, "client:old-owner", 1, 1, 15, "other-model", 10, 2, "1", "1", created.Format(time.RFC3339Nano), "a6", "0.12", 1, created.Format(time.RFC3339Nano))
	if err != nil {
		t.Fatal(err)
	}
	configureTestRule(t, a, 15, "a6", "openai-0.12-95%", "")
	configureTestRule(t, a, 30, "a6", "openai-0.12-95%", "")
	_, err = a.db.Exec(`INSERT INTO usage_snapshots(source_id,request_id,user_id,api_key_id,account_id,model,input_tokens,output_tokens,actual_cost,actual_cost_cny,created_at,rule_provider,rule_external_key,rule_version,rule_snapshot_at)
VALUES(?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)`, 2, "client:shared-peer", 1, 1, 30, "gpt-5.6-sol", 25, 5, "1", "1", created.Format(time.RFC3339Nano), "a6", "openai-0.12-95%", 1, created.Format(time.RFC3339Nano))
	if err != nil {
		t.Fatal(err)
	}
	if err := a.loadA6HistoricalTokenAccounts(); err != nil {
		t.Fatal(err)
	}
	matched, method, err := a.matchUpstreamRecordForToken("a6-old-shared", created.Format(time.RFC3339Nano), "gpt-5.6-sol", 25, 5, 0, 15, "0.12")
	if err != nil || matched != 2 || method == "" {
		t.Fatalf("old shared token did not include renamed peer: source=%d method=%q err=%v", matched, method, err)
	}
}

func TestRuleSnapshotSurvivesCurrentRuleEdit(t *testing.T) {
	a := testApp(t)
	now := time.Now().UTC()
	configureTestRule(t, a, 20, "subarx", "", "0.1")
	_, err := a.db.Exec(`INSERT INTO usage_snapshots(source_id,request_id,user_id,user_email,api_key_id,account_id,model,actual_cost,input_cost,output_cost,actual_cost_cny,created_at,rule_provider,rule_multiplier,rule_version,rule_snapshot_at)
VALUES(1,'client:immutable-rule',1,'user@example.com',1,20,'model','1','1','0','1',?,'subarx','0.1',1,?)`, now.Format(time.RFC3339Nano), now.Format(time.RFC3339Nano))
	if err != nil {
		t.Fatal(err)
	}
	configureTestRule(t, a, 20, "subarx", "", "0.9")
	rows, err := a.profitRows("2000-01-01T00:00:00Z", 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 1 || rows[0].CostSource != "subarx_rule" || rows[0].CostText != "0.10000000" {
		t.Fatalf("snapshot was recalculated from current rule: %+v", rows)
	}
}

func TestBackfillRuleSnapshotsOnlyTouchesUnmatchedBlankSnapshots(t *testing.T) {
	a := testApp(t)
	configureTestRule(t, a, 22, "a6", "new-token", "")
	now := time.Now().UTC().Format(time.RFC3339Nano)
	for _, sourceID := range []int64{1, 2} {
		_, err := a.db.Exec(`INSERT INTO usage_snapshots(source_id,request_id,user_id,api_key_id,account_id,model,actual_cost,actual_cost_cny,created_at) VALUES(?,?,?,?,?,'model','1','1',?)`, sourceID, "client:backfill-"+strconv.FormatInt(sourceID, 10), 1, 1, 22, now)
		if err != nil {
			t.Fatal(err)
		}
	}
	_, err := a.db.Exec(`INSERT INTO upstream_usage(provider,upstream_request_id,cost,currency,fx_rate_to_cny,cost_cny,occurred_at,usage_source_id,source,imported_at) VALUES('a6','already-billed','1','USD','7.2','7.2',?,?, 'test',?)`, now, 1, now)
	if err != nil {
		t.Fatal(err)
	}
	if err := a.backfillRuleSnapshots(); err != nil {
		t.Fatal(err)
	}
	var provider, externalKey string
	if err := a.db.QueryRow(`SELECT rule_provider,rule_external_key FROM usage_snapshots WHERE source_id=1`).Scan(&provider, &externalKey); err != nil {
		t.Fatal(err)
	}
	if provider != "" || externalKey != "" {
		t.Fatalf("matched blank snapshot was changed: provider=%q key=%q", provider, externalKey)
	}
	if err := a.db.QueryRow(`SELECT rule_provider,rule_external_key FROM usage_snapshots WHERE source_id=2`).Scan(&provider, &externalKey); err != nil {
		t.Fatal(err)
	}
	if provider != "a6" || externalKey != "new-token" {
		t.Fatalf("unmatched snapshot was not backfilled: provider=%q key=%q", provider, externalKey)
	}
}

func TestAccountRulesIncludeConfiguredAccountsWithoutUsageInRange(t *testing.T) {
	a := testApp(t)
	source := attachCurrentAccountTopology(t, a)
	_, err := source.Exec(`
INSERT INTO groups(id,name) VALUES(5,'current-group');
INSERT INTO accounts(id,name,platform,status,schedulable) VALUES(13,'0.2-subarx','openai','active',0);
INSERT INTO account_groups(account_id,group_id,priority) VALUES(13,5,1);`)
	if err != nil {
		t.Fatal(err)
	}
	old := time.Now().UTC().Add(-48 * time.Hour)
	for sourceID, values := range []struct {
		group string
		model string
	}{
		{group: "older-group", model: "older-model"},
		{group: "latest-group", model: "latest-model"},
	} {
		_, err := a.db.Exec(`INSERT INTO usage_snapshots(source_id,request_id,user_id,user_email,api_key_id,account_id,group_id,group_name,model,actual_cost,input_cost,output_cost,actual_cost_cny,created_at) VALUES(?,?,?,?,?,?,?,?,?,?,?,?,?,?)`, sourceID+1, "client:historical", 1, "masked@local", 1, 13, 1, values.group, values.model, "1", "0", "0", "1", old.Add(time.Duration(sourceID)*time.Hour).Format(time.RFC3339Nano))
		if err != nil {
			t.Fatal(err)
		}
	}
	request := httptest.NewRequest(http.MethodPut, "/ops/api/account-rules/13", bytes.NewBufferString(`{"provider":"subarx","multiplier":"0.1"}`))
	request.SetPathValue("account_id", "13")
	w := httptest.NewRecorder()
	a.putAccountRule(w, request)
	if w.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", w.Code, w.Body.String())
	}

	from := time.Now().UTC().Add(-time.Hour).Format(time.RFC3339)
	to := time.Now().UTC().Format(time.RFC3339)
	w = httptest.NewRecorder()
	a.accountRules(w, httptest.NewRequest(http.MethodGet, "/ops/api/account-rules?from="+from+"&to="+to, nil))
	if w.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", w.Code, w.Body.String())
	}
	var response struct {
		Items []accountRuleView `json:"items"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &response); err != nil {
		t.Fatal(err)
	}
	if len(response.Items) != 1 || response.Items[0].AccountID != 13 || !response.Items[0].Configured || !response.Items[0].Current || response.Items[0].UsageCount != 0 || response.Items[0].GroupName != "current-group" || response.Items[0].AccountName != "0.2-subarx" || response.Items[0].Models != "latest-model" {
		t.Fatalf("configured rule without range activity was omitted: %+v", response.Items)
	}
}

func TestAccountRulesShowMultipleCurrentChannelsUnderOneGroup(t *testing.T) {
	a := testApp(t)
	source := attachCurrentAccountTopology(t, a)
	_, err := source.Exec(`
INSERT INTO groups(id,name) VALUES(5,'codex-plus优质稳定-0.2x');
INSERT INTO accounts(id,name,platform,status,schedulable) VALUES
  (13,'0.2-subarx','openai','active',0),
  (22,'0.2-a6api','openai','active',1);
INSERT INTO account_groups(account_id,group_id,priority) VALUES(13,5,1),(22,5,2);`)
	if err != nil {
		t.Fatal(err)
	}
	configureTestRule(t, a, 13, "subarx", "", "0.13")
	configureTestRule(t, a, 22, "a6", "0.2", "")

	views, err := a.accountRuleViews("2000-01-01T00:00:00Z", "9999-12-31T00:00:00Z")
	if err != nil {
		t.Fatal(err)
	}
	if len(views) != 2 || views[0].GroupID != 5 || views[1].GroupID != 5 || views[0].GroupName != views[1].GroupName {
		t.Fatalf("current group channels were not grouped: %+v", views)
	}
	if views[0].AccountID != 13 || views[0].Provider != "subarx" || views[1].AccountID != 22 || views[1].Provider != "a6" {
		t.Fatalf("unexpected channel rules: %+v", views)
	}
}
