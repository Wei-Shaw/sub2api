package service

import (
	"math"
	"testing"
)

// statsPtrFloat64 returns a pointer to f. Local helper so tests stay
// independent of any helper that may live in non-test code.
func statsPtrFloat64(f float64) *float64 { return &f }

func nearlyEqual(a, b, eps float64) bool { return math.Abs(a-b) <= eps }

// ---------------------------------------------------------------------------
// matchAccountStatsRule
// ---------------------------------------------------------------------------

func TestMatchAccountStatsRule(t *testing.T) {
	cases := []struct {
		name      string
		rule      AccountStatsPricingRule
		accountID int64
		groupID   int64
		want      bool
	}{
		{"both empty no match", AccountStatsPricingRule{}, 1, 10, false},
		{"account match", AccountStatsPricingRule{AccountIDs: []int64{1, 2, 3}}, 2, 999, true},
		{"group match", AccountStatsPricingRule{GroupIDs: []int64{10, 20}}, 999, 20, true},
		{
			"both configured account hits",
			AccountStatsPricingRule{AccountIDs: []int64{1, 2}, GroupIDs: []int64{10, 20}},
			2, 999, true,
		},
		{
			"both configured group hits",
			AccountStatsPricingRule{AccountIDs: []int64{1, 2}, GroupIDs: []int64{10, 20}},
			999, 10, true,
		},
		{
			"both configured neither hits",
			AccountStatsPricingRule{AccountIDs: []int64{1, 2}, GroupIDs: []int64{10, 20}},
			999, 999, false,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := matchAccountStatsRule(&tc.rule, tc.accountID, tc.groupID)
			if got != tc.want {
				t.Fatalf("got=%v want=%v", got, tc.want)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// findAccountStatsPricingForModel
// ---------------------------------------------------------------------------

func TestFindAccountStatsPricingForModel(t *testing.T) {
	exact := ChannelModelPricing{ID: 1, Models: []string{"claude-opus-4"}}
	wild := ChannelModelPricing{ID: 2, Models: []string{"claude-*"}}
	openaiPlat := ChannelModelPricing{ID: 3, Platform: "openai", Models: []string{"gpt-4o"}}
	emptyPlat := ChannelModelPricing{ID: 4, Models: []string{"gemini-2.5-pro"}}

	tests := []struct {
		name     string
		list     []ChannelModelPricing
		platform string
		model    string
		wantID   int64
		wantNil  bool
	}{
		{"exact match", []ChannelModelPricing{exact}, "anthropic", "claude-opus-4", 1, false},
		{"exact match case-insensitive",
			[]ChannelModelPricing{{ID: 5, Models: []string{"Claude-Opus-4"}}},
			"", "claude-opus-4", 5, false,
		},
		{"wildcard match", []ChannelModelPricing{wild}, "anthropic", "claude-opus-4", 2, false},
		{"exact beats wildcard",
			[]ChannelModelPricing{wild, exact}, "anthropic", "claude-opus-4", 1, false,
		},
		{"platform mismatch skipped",
			[]ChannelModelPricing{openaiPlat}, "anthropic", "gpt-4o", 0, true,
		},
		{"empty platform in pricing matches any",
			[]ChannelModelPricing{emptyPlat}, "gemini", "gemini-2.5-pro", 4, false,
		},
		{"empty platform in query matches any pricing platform",
			[]ChannelModelPricing{openaiPlat}, "", "gpt-4o", 3, false,
		},
		{"no match at all",
			[]ChannelModelPricing{exact, wild}, "anthropic", "gpt-4o", 0, true,
		},
		{"empty list returns nil", nil, "", "claude-opus-4", 0, true},
		{"longest-prefix wildcard wins",
			[]ChannelModelPricing{
				{ID: 10, Models: []string{"claude-*"}},
				{ID: 11, Models: []string{"claude-opus-*"}},
			}, "", "claude-opus-4", 11, false,
		},
		{"shorter wildcard used when longer does not match",
			[]ChannelModelPricing{
				{ID: 10, Models: []string{"claude-*"}},
				{ID: 11, Models: []string{"claude-opus-*"}},
			}, "", "claude-sonnet-4", 10, false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := findAccountStatsPricingForModel(tt.list, tt.platform, tt.model)
			if tt.wantNil {
				if got != nil {
					t.Fatalf("expected nil, got id=%d", got.ID)
				}
				return
			}
			if got == nil {
				t.Fatalf("expected pricing id=%d, got nil", tt.wantID)
			}
			if got.ID != tt.wantID {
				t.Fatalf("id mismatch: got=%d want=%d", got.ID, tt.wantID)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// calculateAccountStatsCost
// ---------------------------------------------------------------------------

func TestCalculateAccountStatsCost_NilPricing(t *testing.T) {
	if got := calculateAccountStatsCost(nil, UsageTokens{}, 1); got != nil {
		t.Fatalf("expected nil, got=%v", *got)
	}
}

func TestCalculateAccountStatsCost_TokenBilling(t *testing.T) {
	pricing := &ChannelModelPricing{
		BillingMode: BillingModeToken,
		InputPrice:  statsPtrFloat64(0.001),
		OutputPrice: statsPtrFloat64(0.002),
	}
	tokens := UsageTokens{InputTokens: 100, OutputTokens: 50}
	got := calculateAccountStatsCost(pricing, tokens, 1)
	if got == nil || !nearlyEqual(*got, 0.2, 1e-12) {
		t.Fatalf("got=%v want=0.2", got)
	}
}

func TestCalculateAccountStatsCost_TokenBilling_WithCacheAndImage(t *testing.T) {
	pricing := &ChannelModelPricing{
		BillingMode:      BillingModeToken,
		InputPrice:       statsPtrFloat64(0.001),
		OutputPrice:      statsPtrFloat64(0.002),
		CacheWritePrice:  statsPtrFloat64(0.003),
		CacheReadPrice:   statsPtrFloat64(0.0005),
		ImageOutputPrice: statsPtrFloat64(0.01),
	}
	tokens := UsageTokens{
		InputTokens:         100,
		OutputTokens:        50,
		CacheCreationTokens: 200,
		CacheReadTokens:     300,
		ImageOutputTokens:   10,
	}
	got := calculateAccountStatsCost(pricing, tokens, 1)
	// 0.1 + 0.1 + 0.6 + 0.15 + 0.1 = 1.05
	if got == nil || !nearlyEqual(*got, 1.05, 1e-12) {
		t.Fatalf("got=%v want=1.05", got)
	}
}

func TestCalculateAccountStatsCost_TokenBilling_PartialPricesNil(t *testing.T) {
	pricing := &ChannelModelPricing{
		BillingMode: BillingModeToken,
		InputPrice:  statsPtrFloat64(0.001),
	}
	tokens := UsageTokens{InputTokens: 100, OutputTokens: 50, CacheCreationTokens: 200}
	got := calculateAccountStatsCost(pricing, tokens, 1)
	if got == nil || !nearlyEqual(*got, 0.1, 1e-12) {
		t.Fatalf("got=%v want=0.1", got)
	}
}

func TestCalculateAccountStatsCost_TokenBilling_AllTokensZero(t *testing.T) {
	pricing := &ChannelModelPricing{
		BillingMode: BillingModeToken,
		InputPrice:  statsPtrFloat64(0.001),
		OutputPrice: statsPtrFloat64(0.002),
	}
	if got := calculateAccountStatsCost(pricing, UsageTokens{}, 1); got != nil {
		t.Fatalf("expected nil, got=%v", *got)
	}
}

func TestCalculateAccountStatsCost_PerRequestBilling(t *testing.T) {
	pricing := &ChannelModelPricing{
		BillingMode:     BillingModePerRequest,
		PerRequestPrice: statsPtrFloat64(0.05),
	}
	tokens := UsageTokens{InputTokens: 999, OutputTokens: 999}
	got := calculateAccountStatsCost(pricing, tokens, 3)
	if got == nil || !nearlyEqual(*got, 0.15, 1e-12) {
		t.Fatalf("got=%v want=0.15", got)
	}
}

func TestCalculateAccountStatsCost_PerRequestBilling_NilOrZeroPrice(t *testing.T) {
	for _, p := range []*ChannelModelPricing{
		{BillingMode: BillingModePerRequest},
		{BillingMode: BillingModePerRequest, PerRequestPrice: statsPtrFloat64(0)},
	} {
		if got := calculateAccountStatsCost(p, UsageTokens{}, 1); got != nil {
			t.Fatalf("expected nil, got=%v (price=%v)", *got, p.PerRequestPrice)
		}
	}
}

func TestCalculateAccountStatsCost_ImageBilling(t *testing.T) {
	pricing := &ChannelModelPricing{
		BillingMode:     BillingModeImage,
		PerRequestPrice: statsPtrFloat64(0.10),
	}
	got := calculateAccountStatsCost(pricing, UsageTokens{}, 2)
	if got == nil || !nearlyEqual(*got, 0.20, 1e-12) {
		t.Fatalf("got=%v want=0.20", got)
	}
}

func TestCalculateAccountStatsCost_DefaultBillingMode_FallsToToken(t *testing.T) {
	pricing := &ChannelModelPricing{
		InputPrice:  statsPtrFloat64(0.001),
		OutputPrice: statsPtrFloat64(0.002),
	}
	tokens := UsageTokens{InputTokens: 100, OutputTokens: 50}
	got := calculateAccountStatsCost(pricing, tokens, 1)
	if got == nil || !nearlyEqual(*got, 0.2, 1e-12) {
		t.Fatalf("got=%v want=0.2", got)
	}
}

// ---------------------------------------------------------------------------
// tryCustomRules — multi-rule traversal
// ---------------------------------------------------------------------------

func TestTryCustomRules_FirstMatchWins(t *testing.T) {
	channel := &Channel{
		AccountStatsPricingRules: []AccountStatsPricingRule{
			{
				GroupIDs: []int64{1},
				Pricing: []ChannelModelPricing{
					{ID: 100, Models: []string{"claude-opus-4"}, InputPrice: statsPtrFloat64(0.01), OutputPrice: statsPtrFloat64(0.02)},
				},
			},
			{
				GroupIDs: []int64{1},
				Pricing: []ChannelModelPricing{
					{ID: 200, Models: []string{"claude-opus-4"}, InputPrice: statsPtrFloat64(0.99), OutputPrice: statsPtrFloat64(0.99)},
				},
			},
		},
	}
	tokens := UsageTokens{InputTokens: 100, OutputTokens: 50}
	got := tryCustomRules(channel, 999, 1, "", "claude-opus-4", tokens, 1)
	if got == nil || !nearlyEqual(*got, 2.0, 1e-12) {
		t.Fatalf("got=%v want=2.0", got)
	}
}

func TestTryCustomRules_SkipsNonMatchingRules(t *testing.T) {
	channel := &Channel{
		AccountStatsPricingRules: []AccountStatsPricingRule{
			{
				AccountIDs: []int64{888},
				Pricing: []ChannelModelPricing{
					{ID: 100, Models: []string{"claude-opus-4"}, InputPrice: statsPtrFloat64(0.99)},
				},
			},
			{
				GroupIDs: []int64{1},
				Pricing: []ChannelModelPricing{
					{ID: 200, Models: []string{"claude-opus-4"}, InputPrice: statsPtrFloat64(0.05)},
				},
			},
		},
	}
	tokens := UsageTokens{InputTokens: 100}
	got := tryCustomRules(channel, 999, 1, "", "claude-opus-4", tokens, 1)
	if got == nil || !nearlyEqual(*got, 5.0, 1e-12) {
		t.Fatalf("got=%v want=5.0", got)
	}
}

func TestTryCustomRules_NoMatch_ReturnsNil(t *testing.T) {
	channel := &Channel{
		AccountStatsPricingRules: []AccountStatsPricingRule{
			{
				AccountIDs: []int64{888},
				Pricing: []ChannelModelPricing{
					{ID: 100, Models: []string{"claude-opus-4"}, InputPrice: statsPtrFloat64(0.01)},
				},
			},
		},
	}
	tokens := UsageTokens{InputTokens: 100}
	if got := tryCustomRules(channel, 999, 2, "", "claude-opus-4", tokens, 1); got != nil {
		t.Fatalf("expected nil, got=%v", *got)
	}
}

func TestTryCustomRules_RuleMatchesButModelNot_ContinuesToNext(t *testing.T) {
	channel := &Channel{
		AccountStatsPricingRules: []AccountStatsPricingRule{
			{
				GroupIDs: []int64{1},
				Pricing: []ChannelModelPricing{
					{ID: 100, Models: []string{"gpt-4o"}, InputPrice: statsPtrFloat64(0.01)},
				},
			},
			{
				GroupIDs: []int64{1},
				Pricing: []ChannelModelPricing{
					{ID: 200, Models: []string{"claude-opus-4"}, InputPrice: statsPtrFloat64(0.05)},
				},
			},
		},
	}
	tokens := UsageTokens{InputTokens: 100}
	got := tryCustomRules(channel, 999, 1, "", "claude-opus-4", tokens, 1)
	if got == nil || !nearlyEqual(*got, 5.0, 1e-12) {
		t.Fatalf("got=%v want=5.0", got)
	}
}
