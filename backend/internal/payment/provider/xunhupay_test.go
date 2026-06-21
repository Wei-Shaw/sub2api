package provider

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/payment"
)

func TestXunhuPayVerifyNotification(t *testing.T) {
	prov, err := NewXunhuPay("x1", map[string]string{
		"appId":      "app-1",
		"secret":     "sec-1",
		"gatewayUrl": "https://pay.example.invalid",
		"notifyUrl":  "https://merchant.example/notify",
		"returnUrl":  "https://merchant.example/return",
	})
	if err != nil {
		t.Fatalf("NewXunhuPay error: %v", err)
	}
	params := map[string]string{
		"appid":          "app-1",
		"trade_order_id": "ord-1",
		"transaction_id": "tx-1",
		"total_fee":      "12.34",
		"status":         "OD",
	}
	params["hash"] = xunhuPaySign(params, "sec-1")

	body := "appid=app-1&trade_order_id=ord-1&transaction_id=tx-1&total_fee=12.34&status=OD&hash=" + params["hash"]
	notice, err := prov.VerifyNotification(context.Background(), body, nil)
	if err != nil {
		t.Fatalf("VerifyNotification error: %v", err)
	}
	if notice.OrderID != "ord-1" || notice.TradeNo != "tx-1" || notice.Status != payment.ProviderStatusSuccess {
		t.Fatalf("notification = %+v", notice)
	}
	if notice.Metadata["appid"] != "app-1" {
		t.Fatalf("metadata appid = %q", notice.Metadata["appid"])
	}
}

func TestXunhuPayCreatePayment(t *testing.T) {
	var gotBody string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		gotBody = string(body)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"errcode":0,"order_id":"up-1","url":"https://pay.example/checkout","url_qrcode":"weixin://wxpay/test"}`))
	}))
	defer server.Close()

	prov, err := NewXunhuPay("x1", map[string]string{
		"appId":      "app-1",
		"secret":     "sec-1",
		"gatewayUrl": server.URL,
		"notifyUrl":  "https://merchant.example/notify",
		"returnUrl":  "https://merchant.example/return",
	})
	if err != nil {
		t.Fatalf("NewXunhuPay error: %v", err)
	}
	resp, err := prov.CreatePayment(context.Background(), payment.CreatePaymentRequest{
		OrderID: "ord-1", Amount: "12.34", Subject: "Recharge",
	})
	if err != nil {
		t.Fatalf("CreatePayment error: %v", err)
	}
	if resp.TradeNo != "up-1" || resp.PayURL == "" || resp.QRCode == "" {
		t.Fatalf("resp = %+v", resp)
	}
	if !strings.Contains(gotBody, "trade_order_id=ord-1") || !strings.Contains(gotBody, "hash=") {
		t.Fatalf("request body missing expected params: %s", gotBody)
	}
}

func TestXunhuPayCreatePaymentKeepsRemoteQRCodeImage(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"errcode":0,"order_id":"up-2","url_qrcode":"https://pay.example/qrcode/order-2.png"}`))
	}))
	defer server.Close()

	prov, err := NewXunhuPay("x1", map[string]string{
		"appId":      "app-1",
		"secret":     "sec-1",
		"gatewayUrl": server.URL,
		"notifyUrl":  "https://merchant.example/notify",
		"returnUrl":  "https://merchant.example/return",
	})
	if err != nil {
		t.Fatalf("NewXunhuPay error: %v", err)
	}
	resp, err := prov.CreatePayment(context.Background(), payment.CreatePaymentRequest{
		OrderID: "ord-2", Amount: "12.34", Subject: "Recharge",
	})
	if err != nil {
		t.Fatalf("CreatePayment error: %v", err)
	}
	if resp.PayURL != "" {
		t.Fatalf("pay_url = %q, want empty for remote qr image", resp.PayURL)
	}
	if resp.QRCode != "https://pay.example/qrcode/order-2.png" {
		t.Fatalf("qr_code = %q", resp.QRCode)
	}
}

func TestXunhuPayCreatePaymentTreatsHTTPURLQRCodeAsPayURL(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"errcode":0,"order_id":"up-2","url_qrcode":"https://pay.example/checkout-from-qrcode"}`))
	}))
	defer server.Close()

	prov, err := NewXunhuPay("x1", map[string]string{
		"appId":      "app-1",
		"secret":     "sec-1",
		"gatewayUrl": server.URL,
		"notifyUrl":  "https://merchant.example/notify",
		"returnUrl":  "https://merchant.example/return",
	})
	if err != nil {
		t.Fatalf("NewXunhuPay error: %v", err)
	}
	resp, err := prov.CreatePayment(context.Background(), payment.CreatePaymentRequest{
		OrderID: "ord-2", Amount: "12.34", Subject: "Recharge",
	})
	if err != nil {
		t.Fatalf("CreatePayment error: %v", err)
	}
	if resp.PayURL != "https://pay.example/checkout-from-qrcode" {
		t.Fatalf("pay_url = %q", resp.PayURL)
	}
	if resp.QRCode != "" {
		t.Fatalf("qr_code = %q, want empty when url_qrcode is a checkout URL", resp.QRCode)
	}
}
