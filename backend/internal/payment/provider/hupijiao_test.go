package provider

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/payment"
)

func TestHupijiaoCreateQueryAndNotification(t *testing.T) {
	const secret = "test-app-secret"
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		if r.Method != http.MethodPost {
			t.Fatalf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
		var params map[string]string
		if strings.HasPrefix(r.Header.Get("Content-Type"), "application/json") {
			if err := json.Unmarshal(body, &params); err != nil {
				t.Fatalf("decode JSON request: %v", err)
			}
		} else {
			values, err := url.ParseQuery(string(body))
			if err != nil {
				t.Fatalf("decode form request: %v", err)
			}
			params = make(map[string]string, len(values))
			for key := range values {
				params[key] = values.Get(key)
			}
		}
		if !hupijiaoVerifyHash(params, secret, params["hash"]) {
			t.Fatalf("request hash invalid: %#v", params)
		}
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/payment/do.html":
			resp := hupijiaoCreateResponse{OpenID: json.RawMessage("123456"), OpenOrderID: "hp-123", URLQRCode: "https://pay.example/qr", URL: "https://pay.example/h5", ErrCode: 0, ErrMsg: "success!"}
			resp.Hash = hupijiaoHash(resp.rawFields(), secret)
			_ = json.NewEncoder(w).Encode(resp)
		case "/payment/query.html":
			resp := hupijiaoQueryResponse{ErrCode: 0, ErrMsg: "success!", Data: hupijiaoQueryData{Status: hupijiaoStatusPaid, OpenOrderID: "hp-123", TransactionID: "tx-123", TotalFee: "2.00"}}
			resp.Hash = hupijiaoHash(resp.rawFields(), secret)
			_ = json.NewEncoder(w).Encode(resp)
		case "/payment/refund.html":
			resp := hupijiaoRefundResponse{TradeOrderID: "sub2-order-1", TransactionID: "tx-123", OutRefundNo: "refund-1", RefundStatus: hupijiaoStatusRefunded, ErrCode: 0, ErrMsg: "success!"}
			resp.Hash = hupijiaoHash(resp.rawFields(), secret)
			_ = json.NewEncoder(w).Encode(resp)
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	prov, err := NewHupijiao("1", map[string]string{
		"appid": "app-123", "appsecret": secret, "apiBase": server.URL,
		"notifyUrl": "https://merchant.example/api/v1/payment/webhook/hupijiao",
		"returnUrl": "https://merchant.example/payment/result",
	})
	if err != nil {
		t.Fatal(err)
	}

	created, err := prov.CreatePayment(context.Background(), payment.CreatePaymentRequest{
		OrderID: "sub2-order-1", Amount: "2.00", PaymentType: payment.TypeAlipay,
		Subject: "Sub2API Recharge", IsMobile: false,
	})
	if err != nil {
		t.Fatal(err)
	}
	if created.TradeNo != "hp-123" || created.QRCode != "https://pay.example/h5" || created.PayURL != "https://pay.example/h5" {
		t.Fatalf("unexpected create response: %#v", created)
	}

	queried, err := prov.QueryOrder(context.Background(), "sub2-order-1")
	if err != nil {
		t.Fatal(err)
	}
	if queried.Status != payment.ProviderStatusPaid || queried.Amount != 2 || queried.TradeNo != "hp-123" {
		t.Fatalf("unexpected query response: %#v", queried)
	}

	params := url.Values{
		"trade_order_id": {"sub2-order-1"}, "total_fee": {"2.00"},
		"transaction_id": {"tx-123"}, "open_order_id": {"hp-123"},
		"status": {hupijiaoStatusPaid}, "appid": {"app-123"},
		"time": {"1710000000"}, "nonce_str": {"nonce"},
	}
	flat := make(map[string]string, len(params))
	for key := range params {
		flat[key] = params.Get(key)
	}
	params.Set("hash", hupijiaoHash(flat, secret))
	notification, err := prov.VerifyNotification(context.Background(), params.Encode(), nil)
	if err != nil {
		t.Fatal(err)
	}
	if notification.Status != payment.ProviderStatusSuccess || notification.OrderID != "sub2-order-1" || notification.TradeNo != "tx-123" {
		t.Fatalf("unexpected notification: %#v", notification)
	}
}

func TestHupijiaoRejectsInvalidNotificationHash(t *testing.T) {
	prov, err := NewHupijiao("1", map[string]string{
		"appid": "app-123", "appsecret": "secret", "apiBase": "https://api.example",
		"notifyUrl": "https://merchant.example/notify", "returnUrl": "https://merchant.example/return",
	})
	if err != nil {
		t.Fatal(err)
	}
	_, err = prov.VerifyNotification(context.Background(), "trade_order_id=x&hash=bad", nil)
	if err == nil {
		t.Fatal("expected invalid hash error")
	}
}
