package provider

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/payment"
)

func TestNormalizeEasyPayAPIBase(t *testing.T) {
	t.Parallel()

	tests := []struct {
		input string
		want  string
	}{
		{input: "https://zpayz.cn", want: "https://zpayz.cn"},
		{input: "https://zpayz.cn/", want: "https://zpayz.cn"},
		{input: "https://zpayz.cn/mapi.php", want: "https://zpayz.cn"},
		{input: "https://zpayz.cn/submit.php", want: "https://zpayz.cn"},
		{input: "https://zpayz.cn/api.php", want: "https://zpayz.cn"},
		{input: "https://zpayz.cn/api.php?act=refund", want: "https://zpayz.cn"},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			t.Parallel()
			if got := normalizeEasyPayAPIBase(tt.input); got != tt.want {
				t.Fatalf("normalizeEasyPayAPIBase(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

func TestEasyPayRefundNormalizesAPIBaseAndSendsOutTradeNoOnly(t *testing.T) {
	t.Parallel()

	var gotPath string
	var gotQuery url.Values
	var gotForm url.Values
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		gotQuery = r.URL.Query()
		if err := r.ParseForm(); err != nil {
			t.Errorf("ParseForm: %v", err)
		}
		gotForm = r.PostForm
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"code":1,"msg":"ok"}`))
	}))
	defer server.Close()

	provider := newTestEasyPay(t, server.URL+"/mapi.php")
	resp, err := provider.Refund(context.Background(), payment.RefundRequest{
		TradeNo: "trade-123",
		OrderID: "out-456",
		Amount:  "1.50",
	})
	if err != nil {
		t.Fatalf("Refund returned error: %v", err)
	}
	if resp == nil || resp.Status != payment.ProviderStatusSuccess {
		t.Fatalf("Refund response = %+v, want success", resp)
	}
	if gotPath != "/api.php" {
		t.Fatalf("refund path = %q, want /api.php", gotPath)
	}
	if gotQuery.Get("act") != "refund" {
		t.Fatalf("refund act query = %q, want refund", gotQuery.Get("act"))
	}
	for key, want := range map[string]string{
		"pid":          "pid-1",
		"key":          "pkey-1",
		"out_trade_no": "out-456",
		"money":        "1.50",
	} {
		if got := gotForm.Get(key); got != want {
			t.Fatalf("form[%s] = %q, want %q (form=%v)", key, got, want, gotForm)
		}
	}
	if got := gotForm.Get("trade_no"); got != "" {
		t.Fatalf("form[trade_no] = %q, want empty (form=%v)", got, gotForm)
	}
}

func TestEasyPayRefundRetriesWithTradeNoWhenOutTradeNoNotFound(t *testing.T) {
	t.Parallel()

	var gotForms []url.Values
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api.php" {
			t.Errorf("refund path = %q, want /api.php", r.URL.Path)
		}
		if r.URL.Query().Get("act") != "refund" {
			t.Errorf("refund act query = %q, want refund", r.URL.Query().Get("act"))
		}
		if err := r.ParseForm(); err != nil {
			t.Errorf("ParseForm: %v", err)
		}
		gotForms = append(gotForms, r.PostForm)
		w.Header().Set("Content-Type", "application/json")
		if len(gotForms) == 1 {
			_, _ = w.Write([]byte(`{"code":0,"msg":"订单编号不存在！"}`))
			return
		}
		_, _ = w.Write([]byte(`{"code":1,"msg":"ok"}`))
	}))
	defer server.Close()

	provider := newTestEasyPay(t, server.URL+"/mapi.php")
	resp, err := provider.Refund(context.Background(), payment.RefundRequest{
		TradeNo: "trade-123",
		OrderID: "out-456",
		Amount:  "1.50",
	})
	if err != nil {
		t.Fatalf("Refund returned error: %v", err)
	}
	if resp == nil || resp.Status != payment.ProviderStatusSuccess || resp.RefundID != "trade-123" {
		t.Fatalf("Refund response = %+v, want success with trade refund id", resp)
	}
	if len(gotForms) != 2 {
		t.Fatalf("refund attempts = %d, want 2", len(gotForms))
	}
	if got := gotForms[0].Get("out_trade_no"); got != "out-456" {
		t.Fatalf("first form[out_trade_no] = %q, want out-456 (form=%v)", got, gotForms[0])
	}
	if got := gotForms[0].Get("trade_no"); got != "" {
		t.Fatalf("first form[trade_no] = %q, want empty (form=%v)", got, gotForms[0])
	}
	if got := gotForms[1].Get("trade_no"); got != "trade-123" {
		t.Fatalf("second form[trade_no] = %q, want trade-123 (form=%v)", got, gotForms[1])
	}
	if got := gotForms[1].Get("out_trade_no"); got != "" {
		t.Fatalf("second form[out_trade_no] = %q, want empty (form=%v)", got, gotForms[1])
	}
}

func TestEasyPayRefundResponseErrors(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		statusCode int
		body       string
		want       string
	}{
		{name: "html response", statusCode: http.StatusOK, body: "<html>bad config</html>", want: "non-JSON response (HTTP 200): <html>bad config</html>"},
		{name: "non json response", statusCode: http.StatusOK, body: "not json", want: "non-JSON response (HTTP 200): not json"},
		{name: "non 2xx response", statusCode: http.StatusBadGateway, body: "bad gateway", want: "HTTP 502: bad gateway"},
		{name: "empty response", statusCode: http.StatusOK, body: "", want: "empty response (HTTP 200): <empty>"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(tt.statusCode)
				_, _ = w.Write([]byte(tt.body))
			}))
			defer server.Close()

			provider := newTestEasyPay(t, server.URL)
			_, err := provider.Refund(context.Background(), payment.RefundRequest{
				OrderID: "out-456",
				Amount:  "1.50",
			})
			if err == nil {
				t.Fatal("Refund returned nil error")
			}
			if !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("Refund error = %q, want substring %q", err.Error(), tt.want)
			}
		})
	}
}

func newTestEasyPay(t *testing.T, apiBase string) *EasyPay {
	t.Helper()

	provider, err := NewEasyPay("test-instance", map[string]string{
		"pid":       "pid-1",
		"pkey":      "pkey-1",
		"apiBase":   apiBase,
		"notifyUrl": "https://example.com/notify",
		"returnUrl": "https://example.com/return",
	})
	if err != nil {
		t.Fatalf("NewEasyPay: %v", err)
	}
	return provider
}
