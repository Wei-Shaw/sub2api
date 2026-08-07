//go:build unit

package service

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

// codingPlanTestServer 返回一个按路径分发的 httptest server，用于替代真实上游。
func codingPlanTestServer(t *testing.T, handler http.HandlerFunc) (endpoint string) {
	t.Helper()
	swapMonitorHTTPClient(t)
	srv := httptest.NewServer(handler)
	t.Cleanup(srv.Close)
	return srv.URL
}

func TestRunCodingPlanCheck_DeepseekBalance(t *testing.T) {
	endpoint := codingPlanTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != codingPlanDeepseekBalancePath {
			http.NotFound(w, r)
			return
		}
		wantAuth := "Bearer" + " " + "test-key"
		if got := r.Header.Get("Authorization"); got != wantAuth {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
			"is_available": true,
			"balance_infos": [{
				"currency": "CNY",
				"total_balance": "110.00",
				"granted_balance": "10.00",
				"topped_up_balance": "100.00"
			}]
		}`))
	})

	res := runCodingPlanCheck(context.Background(), MonitorProviderDeepseek, endpoint, "test-key", "balance")
	if res.Status != MonitorStatusOperational {
		t.Fatalf("status = %s, message=%q", res.Status, res.Message)
	}
	if res.Quota == nil || len(res.Quota.Tiers) == 0 {
		t.Fatalf("expected quota tiers, got %+v", res.Quota)
	}
	tier := res.Quota.Tiers[0]
	if tier.Name != QuotaTierBalance {
		t.Fatalf("tier name = %q", tier.Name)
	}
	if tier.Balance == nil || *tier.Balance != "110.00" {
		t.Fatalf("balance = %v", tier.Balance)
	}
	if tier.Currency == nil || *tier.Currency != "CNY" {
		t.Fatalf("currency = %v", tier.Currency)
	}
	if res.Quota.Available == nil || !*res.Quota.Available {
		t.Fatalf("available = %v", res.Quota.Available)
	}
}

// DeepSeek 多币种账户：CNY/USD 并存时应为每个币种各生成一条 balance tier，
// 即使某币种余额为 0 也不能丢弃（balance_infos.0 只取第一条会漏掉 USD）。
func TestRunCodingPlanCheck_DeepseekMultiCurrency(t *testing.T) {
	endpoint := codingPlanTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
			"is_available": true,
			"balance_infos": [
				{"currency": "CNY", "total_balance": "318.87", "granted_balance": "0.00", "topped_up_balance": "318.87"},
				{"currency": "USD", "total_balance": "0.00", "granted_balance": "0.00", "topped_up_balance": "0.00"}
			]
		}`))
	})

	res := runCodingPlanCheck(context.Background(), MonitorProviderDeepseek, endpoint, "test-key", "balance")
	if res.Status != MonitorStatusOperational {
		t.Fatalf("status = %s, message=%q", res.Status, res.Message)
	}
	if res.Quota == nil || len(res.Quota.Tiers) != 2 {
		t.Fatalf("expected 2 balance tiers, got %+v", res.Quota)
	}
	cny, usd := res.Quota.Tiers[0], res.Quota.Tiers[1]
	if cny.Name != QuotaTierBalance || cny.Currency == nil || *cny.Currency != "CNY" || cny.Balance == nil || *cny.Balance != "318.87" {
		t.Fatalf("cny tier = %+v", cny)
	}
	if usd.Name != QuotaTierBalance || usd.Currency == nil || *usd.Currency != "USD" || usd.Balance == nil || *usd.Balance != "0.00" {
		t.Fatalf("usd tier = %+v", usd)
	}
}

// 空/缺字段的 balance_infos 记录不应产生余额 tier。
// is_available=false 仅影响快照 Available 徽章，不影响渠道可用性判定（仍是 operational）。
func TestRunCodingPlanCheck_DeepseekSkipsEmptyBalanceInfo(t *testing.T) {
	endpoint := codingPlanTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
			"is_available": false,
			"balance_infos": [{}, {"currency": "USD", "total_balance": "1.50"}]
		}`))
	})

	res := runCodingPlanCheck(context.Background(), MonitorProviderDeepseek, endpoint, "test-key", "balance")
	if res.Status != MonitorStatusOperational {
		t.Fatalf("status = %s, message=%q", res.Status, res.Message)
	}
	if len(res.Quota.Tiers) != 1 {
		t.Fatalf("expected 1 tier, got %+v", res.Quota.Tiers)
	}
	tier := res.Quota.Tiers[0]
	if tier.Currency == nil || *tier.Currency != "USD" || tier.Balance == nil || *tier.Balance != "1.50" {
		t.Fatalf("tier = %+v", tier)
	}
}

func TestRunCodingPlanCheck_DeepseekAuthError(t *testing.T) {
	endpoint := codingPlanTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
	})
	res := runCodingPlanCheck(context.Background(), MonitorProviderDeepseek, endpoint, "bad", "balance")
	if res.Status != MonitorStatusFailed {
		t.Fatalf("status = %s, want failed", res.Status)
	}
}

func TestRunCodingPlanCheck_KimiUsage(t *testing.T) {
	endpoint := codingPlanTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != codingPlanKimiUsagesPath {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
			"limits": [{"detail": {"limit": 100, "remaining": 25, "resetTime": "2026-08-07T12:00:00Z"}}],
			"usage": {"limit": 1000, "remaining": 500, "resetTime": 1786000000000}
		}`))
	})

	res := runCodingPlanCheck(context.Background(), MonitorProviderKimi, endpoint, "k", "kimi-for-coding")
	if res.Status != MonitorStatusOperational {
		t.Fatalf("status = %s, message=%q", res.Status, res.Message)
	}
	if res.Quota == nil || len(res.Quota.Tiers) != 2 {
		t.Fatalf("expected 2 tiers, got %+v", res.Quota)
	}
	five := res.Quota.Tiers[0]
	if five.Name != QuotaTierFiveHour || five.Utilization == nil || *five.Utilization != 75 {
		t.Fatalf("five_hour tier = %+v", five)
	}
	weekly := res.Quota.Tiers[1]
	if weekly.Name != QuotaTierWeekly || weekly.Utilization == nil || *weekly.Utilization != 50 {
		t.Fatalf("weekly tier = %+v", weekly)
	}
}

func TestRunCodingPlanCheck_GLMQuota_CreditLimit(t *testing.T) {
	// issue #6153: type 已从 TOKENS_LIMIT 变更为 CREDIT_LIMIT，两者都要识别。
	endpoint := codingPlanTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != codingPlanGLMQuotaPath {
			http.NotFound(w, r)
			return
		}
		// 智谱 Authorization 不加 ******
		if got := r.Header.Get("Authorization"); got != "glm-key" {
			w.WriteHeader(http.StatusForbidden)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
			"code": 200, "msg": "ok",
			"data": {
				"level": "pro",
				"limits": [
					{"type": "CREDIT_LIMIT", "unit": 3, "number": 5, "percentage": 26, "nextResetTime": 1786000000000},
					{"type": "CREDIT_LIMIT", "unit": 6, "number": 1, "percentage": 37}
				]
			}
		}`))
	})

	res := runCodingPlanCheck(context.Background(), MonitorProviderGLM, endpoint, "glm-key", "coding-plan")
	if res.Status != MonitorStatusOperational {
		t.Fatalf("status = %s, message=%q", res.Status, res.Message)
	}
	if res.Quota == nil || len(res.Quota.Tiers) != 2 {
		t.Fatalf("expected 2 tiers, got %+v", res.Quota)
	}
	if res.Quota.PlanLevel != "pro" {
		t.Fatalf("plan level = %q", res.Quota.PlanLevel)
	}
	if res.Quota.Tiers[0].Name != QuotaTierFiveHour || *res.Quota.Tiers[0].Utilization != 26 {
		t.Fatalf("five_hour = %+v", res.Quota.Tiers[0])
	}
	if res.Quota.Tiers[1].Name != QuotaTierWeekly || *res.Quota.Tiers[1].Utilization != 37 {
		t.Fatalf("weekly = %+v", res.Quota.Tiers[1])
	}
}

func TestRunCodingPlanCheck_GLMLegacyTokensLimit(t *testing.T) {
	endpoint := codingPlanTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
			"data": {"limits": [{"type": "TOKENS_LIMIT", "unit": 3, "percentage": 10}]}
		}`))
	})
	res := runCodingPlanCheck(context.Background(), MonitorProviderGLM, endpoint, "k", "coding-plan")
	if res.Status != MonitorStatusOperational {
		t.Fatalf("status = %s, message=%q", res.Status, res.Message)
	}
	if len(res.Quota.Tiers) != 1 || res.Quota.Tiers[0].Name != QuotaTierFiveHour {
		t.Fatalf("tiers = %+v", res.Quota.Tiers)
	}
}

func TestMillisToTime(t *testing.T) {
	sec := millisToTime(1786000000)
	ms := millisToTime(1786000000000)
	if !sec.Equal(ms) {
		t.Fatalf("sec=%v ms=%v", sec, ms)
	}
	if sec.Location() != time.UTC {
		t.Fatalf("location = %v", sec.Location())
	}
}

// 确保 MonitorQuotaSnapshot JSON 往返不丢字段（repo 持久化依赖 JSON）。
func TestMonitorQuotaSnapshotJSONRoundTrip(t *testing.T) {
	util := 42.5
	bal := "110.00"
	cur := "CNY"
	avail := true
	snap := &MonitorQuotaSnapshot{
		PlanLevel: "pro",
		Available: &avail,
		Tiers: []MonitorQuotaTier{
			{Name: QuotaTierFiveHour, Utilization: &util},
			{Name: QuotaTierBalance, Balance: &bal, Currency: &cur},
		},
	}
	raw, err := json.Marshal(snap)
	if err != nil {
		t.Fatal(err)
	}
	var back MonitorQuotaSnapshot
	if err := json.Unmarshal(raw, &back); err != nil {
		t.Fatal(err)
	}
	if len(back.Tiers) != 2 || back.Tiers[1].Balance == nil || *back.Tiers[1].Balance != "110.00" {
		t.Fatalf("round trip = %+v", back)
	}
}
