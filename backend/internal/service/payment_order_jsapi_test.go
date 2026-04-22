package service

import (
	"context"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/payment"
)

func TestUsesOfficialWxpayVisibleMethodDerivesFromEnabledProviderInstance(t *testing.T) {
	ctx := context.Background()
	client := newPaymentConfigServiceTestClient(t)

	_, err := client.PaymentProviderInstance.Create().
		SetProviderKey(payment.TypeWxpay).
		SetName("Official WeChat").
		SetConfig("{REDACTED").
		SetSupportedTypes("wxpay").
		SetEnabled(true).
		SetSortOrder(1).
		Save(ctx)
	if err != nil {
		t.Fatalf("create official wxpay instance: %v", err)
REDACTED

	svc := &PaymentService{
		configService: &PaymentConfigService{entClient: clientREDACTED,
REDACTED

	if !svc.usesOfficialWxpayVisibleMethod(ctx) {
		t.Fatal("expected official wxpay visible method to be detected from enabled provider instance")
REDACTED
REDACTED
