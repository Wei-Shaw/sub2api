package muse

import "testing"

func TestUncertainWorkCannotBeResubmitted(t *testing.T) {
	for _, from := range []State{Submitting, Accepted, Running, CancelPending, Ambiguous, OwnerReview, Completed, Failed, Cancelled} {
		if CanTransition(from, Submitting) || CanTransition(from, Reserved) {
			t.Fatalf("accepted/uncertain state %q allows replay", from)
		}
	}
	if !CanTransition(Ambiguous, Completed) || !CanTransition(CancelPending, Completed) {
		t.Fatal("an observed terminal result must reconcile interrupted or cancelled work")
	}
}

func TestPricingRequiresExplicitPolicy(t *testing.T) {
	for _, p := range []Pricing{
		{Mode: "flat_request", UnitPrice: "0.01234567", Multiplier: "1.25"},
		{Mode: "test_free", UnitPrice: "0", Multiplier: "1"},
	} {
		if err := p.Validate(); err != nil {
			t.Fatal(err)
		}
	}
	for _, p := range []Pricing{
		{}, {Mode: "flat_request", UnitPrice: "0", Multiplier: "1"},
		{Mode: "test_free", UnitPrice: "0.01", Multiplier: "1"},
		{Mode: "flat_request", UnitPrice: "NaN", Multiplier: "1"},
		{Mode: "flat_request", UnitPrice: "1e-2", Multiplier: "1"},
		{Mode: "flat_request", UnitPrice: "0.123456789", Multiplier: "1"},
		{Mode: "flat_request", UnitPrice: "0.1", Multiplier: "-1"},
	} {
		if err := p.Validate(); err == nil {
			t.Fatalf("accepted ambiguous/unbounded pricing: %+v", p)
		}
	}
}
