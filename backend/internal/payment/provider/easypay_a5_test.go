package provider

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/payment"
)

func TestEasyPayFactoryKeepsStandardAndA5ConcreteTypesSeparate(t *testing.T) {
	t.Parallel()

	standardConfig := testEasyPayConfig("https://pay.example.com")
	standard, err := CreateProvider(payment.TypeEasyPay, "standard", standardConfig)
	if err != nil {
		t.Fatalf("CreateProvider standard: %v", err)
	}
	if _, ok := standard.(*EasyPay); !ok {
		t.Fatalf("standard provider type = %T, want *EasyPay", standard)
	}
	if _, ok := standard.(payment.RefundQueryProvider); ok {
		t.Fatalf("standard provider unexpectedly implements RefundQueryProvider")
	}

	a5Config := testEasyPayConfig("https://pay.example.com")
	a5Config[EasyPayCompatibilityModeKey] = EasyPayCompatibilityA5
	a5, err := CreateProvider(payment.TypeEasyPay, "a5", a5Config)
	if err != nil {
		t.Fatalf("CreateProvider A5: %v", err)
	}
	if _, ok := a5.(*A5EasyPay); !ok {
		t.Fatalf("A5 provider type = %T, want *A5EasyPay", a5)
	}
	if _, ok := a5.(payment.RefundQueryProvider); !ok {
		t.Fatalf("A5 provider does not implement RefundQueryProvider")
	}

	invalidConfig := testEasyPayConfig("https://pay.example.com")
	invalidConfig[EasyPayCompatibilityModeKey] = "unknown"
	if _, err := CreateProvider(payment.TypeEasyPay, "invalid", invalidConfig); err == nil {
		t.Fatal("invalid compatibility mode was accepted")
	}
}

func TestA5CreatePaymentCleansReturnURLWithoutChangingStandardEasyPay(t *testing.T) {
	t.Parallel()

	const returnURL = "https://merchant.example/payment/result?order_id=42&resume_token=resume-42#done"
	for _, tc := range []struct {
		name       string
		a5Mode     bool
		wantReturn string
	}{
		{name: "standard preserves return URL", wantReturn: returnURL},
		{name: "A5 removes query and fragment", a5Mode: true, wantReturn: "https://merchant.example/payment/result"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if err := r.ParseForm(); err != nil {
					t.Fatalf("ParseForm: %v", err)
				}
				if got := r.PostForm.Get("return_url"); got != tc.wantReturn {
					t.Errorf("return_url = %q, want %q", got, tc.wantReturn)
				}
				w.Header().Set("Content-Type", "application/json")
				_, _ = w.Write([]byte(`{"code":1,"trade_no":"trade-1","payurl":"https://pay.example/1"}`))
			}))
			defer server.Close()

			base, err := NewEasyPay("test", testEasyPayConfig(server.URL))
			if err != nil {
				t.Fatalf("NewEasyPay: %v", err)
			}
			base.httpClient = server.Client()
			var provider payment.Provider = base
			if tc.a5Mode {
				provider = NewA5EasyPay(base)
			}
			if _, err := provider.CreatePayment(context.Background(), payment.CreatePaymentRequest{
				OrderID: "sub2-create-test", PaymentType: payment.TypeAlipay, Amount: "1.00",
				Subject: "test", ClientIP: "127.0.0.1", ReturnURL: returnURL,
			}); err != nil {
				t.Fatalf("CreatePayment: %v", err)
			}
		})
	}
}

func TestA5QueryOrderUsesGETAndParsesStringStatuses(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		name       string
		statusJSON string
		wantStatus string
	}{
		{name: "paid", statusJSON: `"1"`, wantStatus: payment.ProviderStatusPaid},
		{name: "refunded", statusJSON: `"2"`, wantStatus: payment.ProviderStatusRefunded},
		{name: "numeric pending", statusJSON: `0`, wantStatus: payment.ProviderStatusPending},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Method != http.MethodGet {
					t.Errorf("method = %s, want GET", r.Method)
				}
				query := r.URL.Query()
				for key, want := range map[string]string{
					"act": "order", "pid": "pid-1", "key": "pkey-1", "out_trade_no": "sub2-query-test",
				} {
					if got := query.Get(key); got != want {
						t.Errorf("query[%s] = %q, want %q", key, got, want)
					}
				}
				w.Header().Set("Content-Type", "application/json")
				_, _ = w.Write([]byte(`{"code":1,"msg":"succ","trade_no":"trade-1","out_trade_no":"sub2-query-test","pid":"pid-1","money":"1.00","status":` + tc.statusJSON + `}`))
			}))
			defer server.Close()

			provider := newTestA5EasyPay(t, server)
			resp, err := provider.QueryOrder(context.Background(), "sub2-query-test")
			if err != nil {
				t.Fatalf("QueryOrder: %v", err)
			}
			if resp.Status != tc.wantStatus || resp.TradeNo != "trade-1" || resp.Amount != 1 {
				t.Fatalf("QueryOrder response = %+v", resp)
			}
		})
	}
}

func TestA5QueryOrderRefusesCrossHostRedirect(t *testing.T) {
	t.Parallel()

	target := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		t.Error("cross-host redirect leaked the A5 query")
	}))
	defer target.Close()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, target.URL, http.StatusFound)
	}))
	defer server.Close()

	provider := newTestA5EasyPay(t, server)
	if _, err := provider.QueryOrder(context.Background(), "sub2-query-test"); err == nil {
		t.Fatal("cross-host redirect was accepted")
	}
}

func TestA5QueryOrderTransportErrorDoesNotLeakCredentials(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.NotFoundHandler())
	defer server.Close()
	provider := newTestA5EasyPay(t, server)
	provider.httpClient = &http.Client{Transport: roundTripFunc(func(*http.Request) (*http.Response, error) {
		return nil, context.DeadlineExceeded
	})}
	_, err := provider.QueryOrder(context.Background(), "sub2-secret-test")
	if err == nil {
		t.Fatal("QueryOrder unexpectedly succeeded")
	}
	for _, secret := range []string{"pid-1", "pkey-1", "sub2-secret-test"} {
		if strings.Contains(err.Error(), secret) {
			t.Fatalf("query error leaked %q: %v", secret, err)
		}
	}
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (fn roundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return fn(req)
}

func TestA5RefundAcceptsObservedCodeZeroResponse(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/api.php" || r.URL.Query().Get("act") != "refund" {
			t.Fatalf("unexpected request: %s %s", r.Method, r.URL.String())
		}
		if err := r.ParseForm(); err != nil {
			t.Fatalf("ParseForm: %v", err)
		}
		if got := r.PostForm.Get("out_trade_no"); got != "sub2_a5_fixture_order" {
			t.Fatalf("out_trade_no = %q", got)
		}
		_, _ = w.Write([]byte(`{"code":0,"refund_no":"refund-fixture-1","out_refund_no":"refund-fixture-1","trade_no":"trade-fixture-1","out_trade_no":"sub2_a5_fixture_order","uid":"pid-1","money":"1.00","reducemoney":"0.96","msg":"退款成功！退款金额¥1.00"}`))
	}))
	defer server.Close()

	provider := newTestA5EasyPay(t, server)
	resp, err := provider.Refund(context.Background(), payment.RefundRequest{
		TradeNo: "trade-fixture-1", OrderID: "sub2_a5_fixture_order", Amount: "1.00", FullRefund: true,
	})
	if err != nil {
		t.Fatalf("Refund: %v", err)
	}
	if resp.Status != payment.ProviderStatusSuccess || resp.RefundID != "refund-fixture-1" {
		t.Fatalf("Refund response = %+v", resp)
	}
}

func TestA5RefundAlreadyFullyRefundedRequiresQueryConfirmation(t *testing.T) {
	t.Parallel()

	var queries atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost {
			_, _ = w.Write([]byte(`{"code":-1,"msg":"该订单已全额退款！"}`))
			return
		}
		queries.Add(1)
		_, _ = w.Write([]byte(`{"code":1,"msg":"succ","trade_no":"trade-1","out_trade_no":"out-1","pid":"pid-1","money":"1.00","status":"2"}`))
	}))
	defer server.Close()

	provider := newTestA5EasyPay(t, server)
	resp, err := provider.Refund(context.Background(), payment.RefundRequest{
		TradeNo: "trade-1", OrderID: "out-1", Amount: "1.00", FullRefund: true,
	})
	if err != nil {
		t.Fatalf("Refund: %v", err)
	}
	if resp.Status != payment.ProviderStatusRefunded || queries.Load() != 1 {
		t.Fatalf("Refund response = %+v, queries=%d", resp, queries.Load())
	}
}

func TestA5RefundUnverifiedSuccessStaysPending(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost {
			_, _ = w.Write([]byte(`{"code":0,"refund_no":"refund-1","out_refund_no":"refund-1","trade_no":"trade-1","out_trade_no":"out-1","uid":"pid-1","money":"9.99","msg":"退款成功！退款金额¥9.99"}`))
			return
		}
		_, _ = w.Write([]byte(`{"code":1,"msg":"succ","trade_no":"trade-1","out_trade_no":"out-1","pid":"pid-1","money":"1.00","status":"1"}`))
	}))
	defer server.Close()

	provider := newTestA5EasyPay(t, server)
	resp, err := provider.Refund(context.Background(), payment.RefundRequest{
		TradeNo: "trade-1", OrderID: "out-1", Amount: "1.00", FullRefund: true,
	})
	if err != nil {
		t.Fatalf("Refund: %v", err)
	}
	if resp.Status != payment.ProviderStatusPending {
		t.Fatalf("Refund response = %+v, want pending", resp)
	}
}

func TestA5RefundRejectsPartialBeforeGatewayCall(t *testing.T) {
	t.Parallel()

	var calls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		calls.Add(1)
	}))
	defer server.Close()

	provider := newTestA5EasyPay(t, server)
	if _, err := provider.Refund(context.Background(), payment.RefundRequest{
		TradeNo: "trade-1", OrderID: "out-1", Amount: "0.50", FullRefund: false,
	}); err == nil {
		t.Fatal("partial A5 refund was accepted")
	}
	if calls.Load() != 0 {
		t.Fatalf("gateway calls = %d, want 0", calls.Load())
	}
}

func TestA5QueryRefundConfirmsOnlyFullRefunds(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"code":1,"msg":"succ","trade_no":"trade-1","out_trade_no":"out-1","pid":"pid-1","money":"1.00","status":"2"}`))
	}))
	defer server.Close()

	provider := newTestA5EasyPay(t, server)
	resp, err := provider.QueryRefund(context.Background(), payment.RefundQueryRequest{
		TradeNo: "trade-1", OrderID: "out-1", Amount: "1.00", FullRefund: true,
	})
	if err != nil || resp.Status != payment.ProviderStatusRefunded {
		t.Fatalf("full QueryRefund response=%+v err=%v", resp, err)
	}
	resp, err = provider.QueryRefund(context.Background(), payment.RefundQueryRequest{
		TradeNo: "trade-1", OrderID: "out-1", Amount: "0.50", FullRefund: false,
	})
	if err != nil || resp.Status != payment.ProviderStatusPending {
		t.Fatalf("partial QueryRefund response=%+v err=%v", resp, err)
	}
}

func newTestA5EasyPay(t *testing.T, server *httptest.Server) *A5EasyPay {
	t.Helper()
	base, err := NewEasyPay("test-a5", testEasyPayConfig(server.URL))
	if err != nil {
		t.Fatalf("NewEasyPay: %v", err)
	}
	base.httpClient = server.Client()
	return NewA5EasyPay(base)
}

func testEasyPayConfig(apiBase string) map[string]string {
	return map[string]string{
		"pid": "pid-1", "pkey": "pkey-1", "apiBase": apiBase,
		"notifyUrl": "https://merchant.example/notify", "returnUrl": "https://merchant.example/payment/result",
	}
}
