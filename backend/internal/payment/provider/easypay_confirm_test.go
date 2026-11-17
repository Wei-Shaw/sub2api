package provider

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/payment"
)

func newConfirmTestEasyPay(t *testing.T, body string, status int) (*EasyPay, *atomic.Int32) {
	t.Helper()
	var calls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		if err := r.ParseForm(); err != nil {
			t.Errorf("ParseForm: %v", err)
		}
		q := r.URL.Query()
		if r.URL.Path != "/api.php" || q.Get("act") != "order" || q.Get("pid") != "1001" || q.Get("key") != "pkey-1" || q.Get("out_trade_no") != "sub2_ORDER1" {
			t.Errorf("unexpected query request: path=%s query=%v", r.URL.Path, q)
		}
		w.WriteHeader(status)
		_, _ = w.Write([]byte(body))
	}))
	t.Cleanup(server.Close)
	provider, err := NewEasyPay("test-instance", map[string]string{
		"pid":       "1001",
		"pkey":      "pkey-1",
		"apiBase":   server.URL,
		"notifyUrl": "https://example.com/notify",
		"returnUrl": "https://example.com/return",
	})
	if err != nil {
		t.Fatalf("NewEasyPay: %v", err)
	}
	return provider, &calls
}

func confirmTestNotification() *payment.PaymentNotification {
	return &payment.PaymentNotification{
		TradeNo: "2026101600001",
		OrderID: "sub2_ORDER1",
		Amount:  10,
		Status:  payment.NotificationStatusSuccess,
	}
}

func TestEasyPayConfirmNotification(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		body        string
		status      int
		notifyTrade string
		wantErr     string
		wantTradeNo string
	}{
		{
			name:        "paid with matching amount and trade_no",
			body:        `{"code":1,"pid":1001,"out_trade_no":"sub2_ORDER1","trade_no":"2026101600001","money":"10.00","status":1}`,
			notifyTrade: "2026101600001",
			wantTradeNo: "2026101600001",
		},
		{
			name:        "missing notify trade_no is filled from upstream",
			body:        `{"code":1,"trade_no":"2026101600001","money":"10.00","trade_status":"TRADE_SUCCESS"}`,
			wantTradeNo: "2026101600001",
		},
		{
			name:        "upstream not paid",
			body:        `{"code":1,"trade_no":"2026101600001","money":"10.00","status":0}`,
			notifyTrade: "2026101600001",
			wantErr:     "not paid",
		},
		{
			name:        "upstream failure code with paid status",
			body:        `{"code":-1,"msg":"订单号不存在","status":1,"money":"10.00"}`,
			notifyTrade: "2026101600001",
			wantErr:     "订单号不存在",
		},
		{
			name:        "amount mismatch",
			body:        `{"code":1,"trade_no":"2026101600001","money":"0.01","status":1}`,
			notifyTrade: "2026101600001",
			wantErr:     "amount mismatch",
		},
		{
			name:        "missing upstream amount",
			body:        `{"code":1,"trade_no":"2026101600001","status":1}`,
			notifyTrade: "2026101600001",
			wantErr:     "amount mismatch",
		},
		{
			name:        "trade_no conflict",
			body:        `{"code":1,"trade_no":"2026101699999","money":"10.00","status":1}`,
			notifyTrade: "2026101600001",
			wantErr:     "trade_no mismatch",
		},
		{
			name:        "string status from PHP deployment",
			body:        `{"code":"1","pid":"1001","out_trade_no":"sub2_ORDER1","trade_no":"2026101600001","money":"10.00","status":"1"}`,
			notifyTrade: "2026101600001",
			wantTradeNo: "2026101600001",
		},
		{
			name:        "foreign order echo",
			body:        `{"code":1,"out_trade_no":"sub2_OTHER","trade_no":"2026101600001","money":"10.00","status":1}`,
			notifyTrade: "2026101600001",
			wantErr:     "out_trade_no mismatch",
		},
		{
			name:        "upstream HTTP error body",
			body:        `<html>502 Bad Gateway</html>`,
			status:      http.StatusBadGateway,
			notifyTrade: "2026101600001",
			wantErr:     "parse query",
		},
	}
	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			status := tt.status
			if status == 0 {
				status = http.StatusOK
			}
			provider, calls := newConfirmTestEasyPay(t, tt.body, status)
			n := confirmTestNotification()
			n.TradeNo = tt.notifyTrade
			err := provider.ConfirmNotification(context.Background(), n)
			if calls.Load() != 1 {
				t.Fatalf("upstream query calls = %d, want 1", calls.Load())
			}
			if tt.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
					t.Fatalf("ConfirmNotification error = %v, want containing %q", err, tt.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("ConfirmNotification returned error: %v", err)
			}
			if n.TradeNo != tt.wantTradeNo {
				t.Fatalf("trade_no = %q, want %q", n.TradeNo, tt.wantTradeNo)
			}
		})
	}
}

func TestEasyPayConfirmNotificationSkipsNonSuccess(t *testing.T) {
	t.Parallel()

	provider, calls := newConfirmTestEasyPay(t, `{}`, http.StatusOK)
	n := confirmTestNotification()
	n.Status = payment.ProviderStatusFailed
	if err := provider.ConfirmNotification(context.Background(), n); err != nil {
		t.Fatalf("ConfirmNotification returned error: %v", err)
	}
	if calls.Load() != 0 {
		t.Fatalf("upstream query calls = %d, want 0", calls.Load())
	}
}

func TestEasyPayConfirmNotificationQueryTransportError(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))
	apiBase := server.URL
	server.Close()
	provider := newTestEasyPay(t, apiBase)
	err := provider.ConfirmNotification(context.Background(), confirmTestNotification())
	if err == nil {
		t.Fatal("ConfirmNotification must fail when the upstream is unreachable")
	}
	// The query URL carries the merchant key; errors are logged and stored in
	// the payment audit trail, so they must not echo the URL.
	if strings.Contains(err.Error(), "pkey-1") || strings.Contains(err.Error(), "key=") {
		t.Fatalf("error contains the merchant key: %v", err)
	}
}

func TestEasyPayNotifyRejectsForeignPid(t *testing.T) {
	t.Parallel()
	e := easyPayPoCProvider()

	for _, pid := range []string{"2000", ""} {
		params := map[string]string{
			"trade_no":     "T1",
			"out_trade_no": "ORDER123",
			"type":         "alipay",
			"name":         "balance recharge",
			"money":        "650.00",
			"trade_status": tradeStatusSuccess,
		}
		if pid != "" {
			params["pid"] = pid
		}
		params["sign"] = easyPaySign(params, e.config["pkey"])
		q := url.Values{}
		for k, v := range params {
			q.Set(k, v)
		}
		if _, err := e.VerifyNotification(context.Background(), q.Encode(), nil); err == nil {
			t.Fatalf("callback with pid %q must be rejected", pid)
		}
	}
}

func TestEasyPayNotifyAllowsFreeTextNameOnly(t *testing.T) {
	t.Parallel()
	e := easyPayPoCProvider()

	base := map[string]string{
		"pid":          "1000",
		"trade_no":     "T1",
		"out_trade_no": "ORDER123",
		"type":         "alipay",
		"name":         "Plan A&B = Pro",
		"money":        "650.00",
		"trade_status": tradeStatusSuccess,
	}
	encode := func(params map[string]string) string {
		signed := map[string]string{}
		for k, v := range params {
			signed[k] = v
		}
		signed["sign"] = easyPaySign(params, e.config["pkey"])
		q := url.Values{}
		for k, v := range signed {
			q.Set(k, v)
		}
		return q.Encode()
	}
	if _, err := e.VerifyNotification(context.Background(), encode(base), nil); err != nil {
		t.Fatalf("free-text name must be accepted: %v", err)
	}
	for _, field := range []string{"trade_no", "out_trade_no", "type", "money", "param"} {
		params := map[string]string{}
		for k, v := range base {
			params[k] = v
		}
		params[field] = "v&k=x"
		if _, err := e.VerifyNotification(context.Background(), encode(params), nil); err == nil {
			t.Fatalf("value with &/= in %s must be rejected", field)
		}
	}
}
