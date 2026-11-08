package main

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/shopspring/decimal"
)

func configureFXTest(a *app) {
	a.cfg.A6USDToCNY = decimal.RequireFromString("7.2")
	a.cfg.FXRefreshInterval = time.Hour
	a.cfg.FXHTTPTimeout = 2 * time.Second
	a.cfg.FXMaxStaleness = 6 * time.Hour
}

func TestEnsureCurrentFXRateFreezesFirstRateWithinHour(t *testing.T) {
	a := testApp(t)
	configureFXTest(a)
	var calls int
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		calls++
		fmt.Fprintf(w, `{"data":{"currency":"USD","rates":{"CNY":"%s"}}}`, map[bool]string{true: "7.11", false: "7.22"}[calls == 1])
	}))
	defer server.Close()
	a.cfg.FXPrimaryURL = server.URL
	a.cfg.FXFallbackURL = ""

	now := time.Date(2026, 7, 29, 12, 15, 0, 0, time.UTC)
	first, err := a.ensureCurrentFXRate(now)
	if err != nil {
		t.Fatal(err)
	}
	second, err := a.ensureCurrentFXRate(now.Add(30 * time.Minute))
	if err != nil {
		t.Fatal(err)
	}
	if calls != 1 || !first.Rate.Equal(decimal.RequireFromString("7.11")) || !second.Rate.Equal(first.Rate) {
		t.Fatalf("calls=%d first=%s second=%s", calls, first.Rate, second.Rate)
	}
}

func TestEnsureCurrentFXRateFallsBackToERAPI(t *testing.T) {
	a := testApp(t)
	configureFXTest(a)
	primary := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { http.Error(w, "no", http.StatusBadGateway) }))
	defer primary.Close()
	fallback := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		fmt.Fprint(w, `{"result":"success","base_code":"USD","rates":{"CNY":7.08}}`)
	}))
	defer fallback.Close()
	a.cfg.FXPrimaryURL = primary.URL
	a.cfg.FXFallbackURL = fallback.URL

	got, err := a.ensureCurrentFXRate(time.Date(2026, 7, 29, 13, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatal(err)
	}
	if got.Source != "er-api" || !got.Rate.Equal(decimal.RequireFromString("7.08")) || got.Stale {
		t.Fatalf("snapshot=%+v", got)
	}
}

func TestA6LateBillUsesOccurrenceRateAndDuplicateKeepsSnapshot(t *testing.T) {
	a := testApp(t)
	configureFXTest(a)
	oldHour := time.Date(2026, 7, 29, 9, 0, 0, 0, time.UTC)
	newHour := oldHour.Add(time.Hour)
	if err := a.insertFXRate(fxRateSnapshot{Rate: decimal.RequireFromString("7.01"), Source: "test-old", EffectiveAt: oldHour, FetchedAt: oldHour}); err != nil {
		t.Fatal(err)
	}
	if err := a.insertFXRate(fxRateSnapshot{Rate: decimal.RequireFromString("7.19"), Source: "test-new", EffectiveAt: newHour, FetchedAt: newHour}); err != nil {
		t.Fatal(err)
	}
	item := a6LogItem{RequestID: "immutable-fx", TokenName: "0.06", ModelName: "gpt-5.6-sol", PromptTokens: 25, CompletionTokens: 5}
	item.Quota = "15"
	occurred := oldHour.Add(20 * time.Minute)
	if err := a.storeA6Item(item, 17, decimal.NewFromInt(500000), occurred); err != nil {
		t.Fatal(err)
	}

	item.Quota = "30"
	if err := a.storeA6Item(item, 17, decimal.NewFromInt(500000), newHour.Add(20*time.Minute)); err != nil {
		t.Fatal(err)
	}
	var cost, rate, cny, source, effective string
	if err := a.db.QueryRow(`SELECT cost,fx_rate_to_cny,cost_cny,fx_rate_source,fx_rate_effective_at FROM upstream_usage WHERE provider='a6' AND upstream_request_id='immutable-fx'`).Scan(&cost, &rate, &cny, &source, &effective); err != nil {
		t.Fatal(err)
	}
	if cost != "0.00003" || rate != "7.01" || cny != "0.0002103" || source != "test-old" || effective != oldHour.Format(time.RFC3339Nano) {
		t.Fatalf("cost=%s rate=%s cny=%s source=%s effective=%s", cost, rate, cny, source, effective)
	}
}

func TestFXFailureStoresStaleFixedFallback(t *testing.T) {
	a := testApp(t)
	configureFXTest(a)
	a.cfg.FXPrimaryURL = "http://127.0.0.1:1"
	a.cfg.FXFallbackURL = "http://127.0.0.1:1"
	now := time.Date(2026, 7, 29, 14, 0, 0, 0, time.UTC)
	got, err := a.ensureCurrentFXRate(now)
	if err == nil {
		t.Fatal("expected provider failure")
	}
	if !got.Rate.Equal(decimal.RequireFromString("7.2")) || !got.Stale || got.Source != "bootstrap_fixed_stale" {
		t.Fatalf("snapshot=%+v err=%v", got, err)
	}
	persisted, ok := a.fxRateAtExactHour(now)
	if !ok || !persisted.Stale || !persisted.Rate.Equal(got.Rate) {
		t.Fatalf("persisted=%+v ok=%v", persisted, ok)
	}
}
