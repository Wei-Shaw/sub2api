package service

import "testing"

func TestComputeActualCreditsFromParamsUsesTokenPriceDenominator(t *testing.T) {
	credits := computeActualCreditsFromParams(4, 1, 7.50)
	if credits != 533_333 {
		t.Fatalf("credits = %d, want 533333", credits)
	}

	legacyDiscountCredits := computeActualCreditsFromParams(4, 0.036247, 7.50)
	if legacyDiscountCredits != 14_713_861 {
		t.Fatalf("legacy discount credits = %d, want 14713861", legacyDiscountCredits)
	}

	wrongWithoutTokenPrice := computeActualCreditsFromParams(4, 0.036247, 1)
	if legacyDiscountCredits >= wrongWithoutTokenPrice/2 {
		t.Fatalf("credits = %d looks like missing token_price_per_million denominator; wrong formula would be about %d", legacyDiscountCredits, wrongWithoutTokenPrice)
	}
}

func TestComputeActualCreditsFromParamsMatchesApprovedBalancePackageTargetsAtRateOne(t *testing.T) {
	cases := []struct {
		name         string
		amountLedger float64
		want         int64
	}{
		{name: "standard", amountLedger: 202.50, want: 27_000_000},
		{name: "pro", amountLedger: 472.50, want: 63_000_000},
		{name: "expert", amountLedger: 975.00, want: 130_000_000},
		{name: "business", amountLedger: 2550.00, want: 340_000_000},
		{name: "enterprise", amountLedger: 5250.00, want: 700_000_000},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := computeActualCreditsFromParams(tc.amountLedger, 1, 7.50)
			if got != tc.want {
				t.Fatalf("credits = %d, want %d", got, tc.want)
			}
		})
	}
}
