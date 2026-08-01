package service

import (
	"context"
	"testing"
)

func TestNormalizeBalancePackageActualCreditsUsesExplicitMetadata(t *testing.T) {
	if got := normalizeBalancePackageActualCredits(27_000_000); got != 27_000_000 {
		t.Fatalf("credits = %d, want explicit package metadata", got)
	}
	if got := normalizeBalancePackageActualCredits(0); got != 0 {
		t.Fatalf("credits = %d, want no synthetic credits when package omits metadata", got)
	}
	if got := normalizeBalancePackageActualCredits(-1); got != 0 {
		t.Fatalf("credits = %d, want negative metadata clamped to 0", got)
	}
}

func TestCreateBalancePackageDoesNotSynthesizeCreditsFromGroupRate(t *testing.T) {
	ctx := context.Background()
	client := newPaymentConfigServiceTestClient(t)

	group, err := client.Group.Create().
		SetName("OpenAI Standard Balance").
		SetPlatform(PlatformOpenAI).
		SetStatus(StatusActive).
		SetSubscriptionType(SubscriptionTypeStandard).
		SetRateMultiplier(0.036247).
		Save(ctx)
	if err != nil {
		t.Fatalf("create balance group: %v", err)
	}
	groupID := group.ID
	svc := &PaymentConfigService{entClient: client}

	pkg, err := svc.CreateBalancePackage(ctx, CreateBalancePackageRequest{
		Code:           "standard-topup",
		Label:          "Standard top-up",
		AmountLedger:   4.00,
		BalanceGroupID: &groupID,
		CreditUnit:     "tokens",
		ForSale:        true,
	})
	if err != nil {
		t.Fatalf("CreateBalancePackage returned error: %v", err)
	}
	if pkg.AmountLedger != 4.00 {
		t.Fatalf("amount_ledger = %v, want fixed wallet credit 4.00", pkg.AmountLedger)
	}
	if pkg.ActualCredits != 0 {
		t.Fatalf("actual_credits = %d, want 0 when admin did not configure package metadata", pkg.ActualCredits)
	}

	amount := 8.00
	pkg, err = svc.UpdateBalancePackage(ctx, pkg.ID, UpdateBalancePackageRequest{AmountLedger: &amount})
	if err != nil {
		t.Fatalf("UpdateBalancePackage amount returned error: %v", err)
	}
	if pkg.ActualCredits != 0 {
		t.Fatalf("actual_credits after amount update = %d, want still 0", pkg.ActualCredits)
	}

	explicitCredits := int64(27_000_000)
	pkg, err = svc.UpdateBalancePackage(ctx, pkg.ID, UpdateBalancePackageRequest{ActualCredits: &explicitCredits})
	if err != nil {
		t.Fatalf("UpdateBalancePackage credits returned error: %v", err)
	}
	if pkg.ActualCredits != explicitCredits {
		t.Fatalf("actual_credits = %d, want explicit package metadata %d", pkg.ActualCredits, explicitCredits)
	}
}
