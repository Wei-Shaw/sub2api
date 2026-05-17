package service

import (
	"testing"

	dbent "github.com/Wei-Shaw/sub2api/ent"
)

func TestPaymentServiceBuildPaymentSubjectDefaultsToDevRouter(t *testing.T) {
	svc := &PaymentService{}

	if got := svc.buildPaymentSubject(nil, 12.5, &PaymentConfig{}); got != "DevRouter 12.50 CNY" {
		t.Fatalf("buildPaymentSubject balance default = %q, want %q", got, "DevRouter 12.50 CNY")
	}

	if got := svc.buildPaymentSubject(&dbent.SubscriptionPlan{Name: "Pro"}, 0, &PaymentConfig{}); got != "DevRouter Subscription Pro" {
		t.Fatalf("buildPaymentSubject subscription default = %q, want %q", got, "DevRouter Subscription Pro")
	}
}
