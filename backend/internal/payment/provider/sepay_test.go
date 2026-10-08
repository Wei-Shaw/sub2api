//go:build unit

package provider

import (
	"context"
	"net/url"
	"strings"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/payment"
)

const testSepayWebhookKey = "fixture-webhook-key"

func testSepayConfig(webhookKey string) map[string]string {
	return map[string]string{
		"bankCode":             "MBBank",
		"accountNo":            "1900123456789",
		"webhookApiKey":        webhookKey,
		"paymentContentPrefix": "",
	}
}

func TestSepayCreatePaymentBuildsOfflineQRCode(t *testing.T) {
	provider, err := NewSepay("test", testSepayConfig("secret"))
	if err != nil {
		t.Fatal(err)
	}
	response, err := provider.CreatePayment(context.Background(), paymentRequest("200000", "VND"))
	if err != nil {
		t.Fatal(err)
	}
	parsed, err := url.Parse(response.QRCode)
	if err != nil {
		t.Fatal(err)
	}
	query := parsed.Query()
	if query.Get("acc") != "1900123456789" || query.Get("bank") != "MBBank" || query.Get("amount") != "200000" {
		t.Fatalf("unexpected QR query: %v", query)
	}
	if query.Get("des") != "CK sub2_20250409aB3kX9mQ" {
		t.Fatalf("unexpected QR description: %q", query.Get("des"))
	}
	if strings.Contains(parsed.RawQuery, "template=") || strings.Contains(parsed.RawQuery, "accountName=") {
		t.Fatalf("QR contains unsupported legacy parameters: %s", parsed.RawQuery)
	}
	if response.TradeNo != "sub2_20250409aB3kX9mQ" {
		t.Fatalf("TradeNo = %q", response.TradeNo)
	}
}

func TestSepayCreatePaymentUsesExplicitContentAliases(t *testing.T) {
	config := testSepayConfig(testSepayWebhookKey)
	config["paymentContentAliases"] = "pay|topup"
	config["paymentContentTemplates"] = `["{code}"]`
	provider, err := NewSepay("test", config)
	if err != nil {
		t.Fatal(err)
	}
	response, err := provider.CreatePayment(context.Background(), paymentRequest("200000", "VND"))
	if err != nil {
		t.Fatal(err)
	}
	parsed, err := url.Parse(response.QRCode)
	if err != nil {
		t.Fatal(err)
	}
	description := parsed.Query().Get("des")
	if description != "paysub2_20250409aB3kX9mQ" && description != "topupsub2_20250409aB3kX9mQ" {
		t.Fatalf("unexpected QR description: %q", description)
	}
}

func TestSepayRejectsInvalidQRContentTemplates(t *testing.T) {
	for _, templates := range []string{`["without-code"]`, `["{code} {code}"]`} {
		config := testSepayConfig(testSepayWebhookKey)
		config["paymentContentTemplates"] = templates
		if _, err := NewSepay("test", config); err == nil || !strings.Contains(err.Error(), "SEPAY_PAYMENT_CONTENT_TEMPLATE_INVALID") {
			t.Fatalf("templates %s returned error %v, want invalid QR template", templates, err)
		}
	}
}

func TestSepayCreatePaymentValidatesWholeVND(t *testing.T) {
	for _, amount := range []string{"0", "-1", "NaN", "Inf", "200000.5"} {
		provider, err := NewSepay("test", testSepayConfig("secret"))
		if err != nil {
			t.Fatal(err)
		}
		_, err = provider.CreatePayment(context.Background(), paymentRequest(amount, "VND"))
		if err == nil {
			t.Fatalf("CreatePayment(%q) unexpectedly succeeded", amount)
		}
	}

	provider, err := NewSepay("test", testSepayConfig("secret"))
	if err != nil {
		t.Fatal(err)
	}
	_, err = provider.CreatePayment(context.Background(), paymentRequest("200000", "USD"))
	if err == nil || !strings.Contains(err.Error(), "SEPAY_CURRENCY_NOT_VND") {
		t.Fatalf("non-VND error = %v, want SEPAY_CURRENCY_NOT_VND", err)
	}
}

func TestSepayCreatePaymentRequiresBankConfig(t *testing.T) {
	config := testSepayConfig("secret")
	config["bankCode"] = ""
	_, err := NewSepay("test", config)
	if err == nil || !strings.Contains(err.Error(), "SEPAY_BANK_CONFIG_MISSING") {
		t.Fatalf("constructor error = %v, want SEPAY_BANK_CONFIG_MISSING", err)
	}
}

func TestSepayWebhookAuthorizationAndNormalization(t *testing.T) {
	provider, err := NewSepay("test", testSepayConfig("secret"))
	if err != nil {
		t.Fatal(err)
	}
	body := `{"id":"1001","code":"SUB2_20250409AB3KX9MQ","transferType":"in","transferAmount":"200000","content":"noise SUB2_20250409AB3KX9MQ","referenceCode":"FT-1"}`
	notification, err := provider.VerifyNotification(context.Background(), body, map[string]string{"authorization": "Apikey secret"})
	if err != nil {
		t.Fatal(err)
	}
	if notification.OrderID != "sub2_20250409AB3KX9MQ" || notification.TradeNo != "1001" || notification.Amount != 200000 || notification.Currency != "VND" {
		t.Fatalf("unexpected notification: %+v", notification)
	}

	if _, err := provider.VerifyNotification(context.Background(), body, map[string]string{"authorization": "Bearer secret"}); err == nil {
		t.Fatal("Bearer authorization unexpectedly accepted")
	}
	if _, err := provider.VerifyNotification(context.Background(), body, map[string]string{"authorization": "Apikey wrong"}); err == nil {
		t.Fatal("wrong Apikey unexpectedly accepted")
	}

	outgoing := strings.Replace(body, `"in"`, `"out"`, 1)
	ignored, err := provider.VerifyNotification(context.Background(), outgoing, map[string]string{"authorization": "Apikey secret"})
	if err != nil || ignored != nil {
		t.Fatalf("outgoing notification = %#v, error=%v; want nil, nil", ignored, err)
	}
}

func TestSepayWebhookContentTakesPriorityAndRequiresTransactionID(t *testing.T) {
	provider, err := NewSepay("test", testSepayConfig(testSepayWebhookKey))
	if err != nil {
		t.Fatal(err)
	}
	body := `{"id":0,"code":"not-an-order","transferType":"in","amount_in":200000,"content":"CK sub2_20250409aB3kX9mQ","reference_number":"FT-2"}`
	notification, err := provider.VerifyNotification(context.Background(), body, map[string]string{"authorization": "Apikey " + testSepayWebhookKey})
	if err != nil {
		t.Fatal(err)
	}
	if notification.OrderID != "sub2_20250409aB3kX9mQ" || notification.TradeNo != "FT-2" {
		t.Fatalf("unexpected content-first notification: %+v", notification)
	}

	missingID := `{"code":"sub2_20250409aB3kX9mQ","transferType":"in","amount":200000,"content":"CK sub2_20250409aB3kX9mQ"}`
	if _, err := provider.VerifyNotification(context.Background(), missingID, map[string]string{"authorization": "Apikey " + testSepayWebhookKey}); err == nil || !strings.Contains(err.Error(), "SEPAY_TRANSACTION_ID_MISSING") {
		t.Fatalf("missing transaction id error = %v", err)
	}
}

func TestSepayWebhookUsesConfiguredRecognitionPatterns(t *testing.T) {
	config := testSepayConfig(testSepayWebhookKey)
	config["paymentContentRecognitionPatterns"] = `["PAY {code}","TOPUP {code}"]`
	provider, err := NewSepay("test", config)
	if err != nil {
		t.Fatal(err)
	}

	body := `{"id":"1002","code":"not-an-order","transferType":"in","amount":200000,"content":"bank PAY SUB2_20250409aB3kX9mQ paid","reference_number":"FT-3"}`
	notification, err := provider.VerifyNotification(context.Background(), body, map[string]string{"authorization": "Apikey " + testSepayWebhookKey})
	if err != nil {
		t.Fatal(err)
	}
	if notification == nil || notification.OrderID != "sub2_20250409aB3kX9mQ" {
		t.Fatalf("unexpected configured-pattern notification: %+v", notification)
	}

	unknownPatternBody := `{"id":"1003","code":"not-an-order","transferType":"in","amount":200000,"content":"bank CK SUB2_20250409aB3kX9mQ paid","reference_number":"FT-4"}`
	ignored, err := provider.VerifyNotification(context.Background(), unknownPatternBody, map[string]string{"authorization": "Apikey " + testSepayWebhookKey})
	if err != nil {
		t.Fatal(err)
	}
	if ignored != nil {
		t.Fatalf("unconfigured content was accepted: %+v", ignored)
	}
}

func TestSepayRejectsRecognitionPatternWithoutOrderPlaceholder(t *testing.T) {
	config := testSepayConfig(testSepayWebhookKey)
	config["paymentContentRecognitionPatterns"] = `["PAY sub2_"]`
	if _, err := NewSepay("test", config); err == nil || !strings.Contains(err.Error(), "SEPAY_PAYMENT_CONTENT_PATTERN_INVALID") {
		t.Fatalf("constructor error = %v, want invalid recognition pattern", err)
	}
}

func TestSepayConfiguredRecognitionRequiresOneUnambiguousOrderCode(t *testing.T) {
	config := testSepayConfig(testSepayWebhookKey)
	config["paymentContentRecognitionPatterns"] = `["PAY {code}"]`
	provider, err := NewSepay("test", config)
	if err != nil {
		t.Fatal(err)
	}
	body := `{"id":"1004","transferType":"in","amount":200000,"content":"PAY SUB2_20250409aB3kX9mQ and SUB2_20250409bC4lY0nR","reference_number":"FT-5"}`
	notification, err := provider.VerifyNotification(context.Background(), body, map[string]string{"authorization": "Apikey " + testSepayWebhookKey})
	if err != nil {
		t.Fatal(err)
	}
	if notification != nil {
		t.Fatalf("ambiguous configured content was accepted: %+v", notification)
	}
}

func TestSepayRejectsMissingWebhookAuthentication(t *testing.T) {
	if _, err := NewSepay("test", testSepayConfig("")); err == nil || !strings.Contains(err.Error(), "SEPAY_WEBHOOK_API_KEY_MISSING") {
		t.Fatalf("constructor error = %v, want missing webhook key", err)
	}
}

func TestSepayOrderCodecPreservesSuffixCaseAndRejectsLegacyFormats(t *testing.T) {
	cases := map[string]string{
		"sub2_20250409aB3kX9mQ":         "sub2_20250409aB3kX9mQ",
		"SUB2_20250409AB3KX9MQ":         "sub2_20250409AB3KX9MQ",
		"CK SUB2_20250409AB3KX9MQ paid": "sub2_20250409AB3KX9MQ",
	}
	for input, want := range cases {
		got := ExtractSepayOrderIDFromContent(input)
		if got != want {
			t.Fatalf("ExtractSepayOrderIDFromContent(%q) = %q, want %q", input, got, want)
		}
		if got := NormalizeSepayOrderID(input); got != want && input != "CK SUB2_20250409AB3KX9MQ paid" {
			t.Fatalf("NormalizeSepayOrderID(%q) = %q, want %q", input, got, want)
		}
	}
	for _, input := range []string{
		"sub2_20250409aB3kX9",
		"sub2_20250409aB3kX9mQ1",
		"vclaw_20250409aB3kX9mQ",
		"VC20250409aB3kX9mQ",
	} {
		if got := ExtractSepayOrderIDFromContent(input); got != "" {
			t.Fatalf("legacy/invalid order %q normalized to %q", input, got)
		}
	}
}

func TestSepayUnsupportedOperations(t *testing.T) {
	provider, err := NewSepay("test", testSepayConfig("secret"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := provider.QueryOrder(context.Background(), "sub2_20250409aB3kX9mQ"); err == nil {
		t.Fatal("QueryOrder unexpectedly succeeded")
	}
	if _, err := provider.Refund(context.Background(), RefundRequestForTest()); err == nil {
		t.Fatal("Refund unexpectedly succeeded")
	}
}

func paymentRequest(amount, currency string) payment.CreatePaymentRequest {
	return payment.CreatePaymentRequest{
		OrderID:         "sub2_20250409aB3kX9mQ",
		Amount:          amount,
		PaymentCurrency: currency,
		PaymentType:     payment.TypeSepay,
	}
}

func RefundRequestForTest() payment.RefundRequest {
	return payment.RefundRequest{OrderID: "sub2_20250409aB3kX9mQ", TradeNo: "1001", Amount: "200000"}
}
