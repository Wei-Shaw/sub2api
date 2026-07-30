package handler

import (
	"testing"

	dbent "github.com/Wei-Shaw/sub2api/ent"
)

func TestPaymentOrderResponsesHideRefundGenerationToken(t *testing.T) {
	token := "r:abcdefghijklmnop"
	order := &dbent.PaymentOrder{UserID: 42, RefundRequestedBy: &token}

	authenticated := sanitizePaymentOrderForResponse(order)
	if authenticated == nil || authenticated.RefundRequestedBy == nil || *authenticated.RefundRequestedBy != "42" {
		t.Fatalf("authenticated response leaked or lost refund requester: %#v", authenticated)
	}
	public := buildPublicOrderResult(order)
	if public.RefundRequestedBy == nil || *public.RefundRequestedBy != "42" {
		t.Fatalf("signed public response leaked or lost refund requester: %#v", public.RefundRequestedBy)
	}
}
