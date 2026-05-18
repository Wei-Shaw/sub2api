//go:build unit

package provider

import (
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/payment"
)

func TestCreatePaymentCardUsesAutomaticPaymentMethods(t *testing.T) {
	req := payment.CreatePaymentRequest{
		OrderID:            "test-order-card",
		Amount:             "100.00",
		PaymentType:        "card",
		Subject:            "Test",
		InstanceSubMethods: "card",
	}
	params, err := buildPaymentIntentParams(req, payment.DefaultPaymentCurrency)
	if err != nil {
		t.Fatalf("buildPaymentIntentParams: %v", err)
	}
	if params.AutomaticPaymentMethods == nil {
		t.Fatal("expected AutomaticPaymentMethods to be set for PaymentType=card")
	}
	if params.AutomaticPaymentMethods.Enabled == nil || !*params.AutomaticPaymentMethods.Enabled {
		t.Errorf("AutomaticPaymentMethods.Enabled should be true")
	}
	if len(params.PaymentMethodTypes) != 0 {
		t.Errorf("PaymentMethodTypes should be empty when automatic methods enabled, got %d entries", len(params.PaymentMethodTypes))
	}
}

func TestCreatePaymentAlipayUsesExplicitMethodTypes(t *testing.T) {
	req := payment.CreatePaymentRequest{
		OrderID:            "test-order-alipay",
		Amount:             "100.00",
		PaymentType:        "alipay",
		Subject:            "Test",
		InstanceSubMethods: "alipay",
	}
	params, err := buildPaymentIntentParams(req, payment.DefaultPaymentCurrency)
	if err != nil {
		t.Fatalf("buildPaymentIntentParams: %v", err)
	}
	if params.AutomaticPaymentMethods != nil {
		t.Errorf("AutomaticPaymentMethods should be nil for alipay")
	}
	if len(params.PaymentMethodTypes) != 1 || *params.PaymentMethodTypes[0] != "alipay" {
		t.Errorf("PaymentMethodTypes should be [alipay], got %+v", params.PaymentMethodTypes)
	}
}

func TestCreatePaymentWxpayUsesExplicitMethodTypesAndOptions(t *testing.T) {
	req := payment.CreatePaymentRequest{
		OrderID:            "test-order-wxpay",
		Amount:             "100.00",
		PaymentType:        "wxpay",
		Subject:            "Test",
		InstanceSubMethods: "wxpay",
	}
	params, err := buildPaymentIntentParams(req, payment.DefaultPaymentCurrency)
	if err != nil {
		t.Fatalf("buildPaymentIntentParams: %v", err)
	}
	if params.AutomaticPaymentMethods != nil {
		t.Errorf("AutomaticPaymentMethods should be nil for wxpay")
	}
	if len(params.PaymentMethodTypes) != 1 || *params.PaymentMethodTypes[0] != "wechat_pay" {
		t.Errorf("PaymentMethodTypes should be [wechat_pay], got %+v", params.PaymentMethodTypes)
	}
	if params.PaymentMethodOptions == nil || params.PaymentMethodOptions.WeChatPay == nil {
		t.Fatal("expected PaymentMethodOptions.WeChatPay to be set")
	}
	if params.PaymentMethodOptions.WeChatPay.Client == nil || *params.PaymentMethodOptions.WeChatPay.Client != "web" {
		t.Errorf("WeChatPay.Client should be 'web'")
	}
}
