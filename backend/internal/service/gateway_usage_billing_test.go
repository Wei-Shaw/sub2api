//go:build unit

package service

import (
	"testing"
)

// TestBuildUsageBillingCommand_TokenSubscriptionSetsSubscriptionTokens locks in the
// token-type subscription metering path: for a SubscriptionTypeSubscriptionToken
// group, the command carries effective tokens in SubscriptionTokens (复用 recordUsageCore
// 经 computeSubscriptionTokens 预填) 且不记 USD — token 型无 USD 限额，只累加 *_usage_tokens。
func TestBuildUsageBillingCommand_TokenSubscriptionSetsSubscriptionTokens(t *testing.T) {
	t.Parallel()

	daily := int64(0)
	grp := &Group{
		SubscriptionType: SubscriptionTypeSubscriptionToken,
		RateMultiplier:   2.0,
		DailyLimitTokens: &daily, // 仅用于标识，不参与本断言
	}
	usageLog := &UsageLog{InputTokens: 100, OutputTokens: 50, CacheCreationTokens: 10, CacheReadTokens: 40}
	p := &postUsageBillingParams{
		Cost:                     &CostBreakdown{TotalCost: 1.0, ActualCost: 2.0},
		User:                     &User{ID: 1},
		APIKey:                   &APIKey{ID: 2, GroupID: ptrInt64(1), Group: grp},
		Account:                  &Account{ID: 3, Type: AccountTypeAPIKey},
		Subscription:             &UserSubscription{ID: 9},
		IsSubscriptionBill:       true,
		EffectiveTokenMultiplier: 2.0, // 与 USD 同源：text 倍率（含高峰），由 recordUsageCore 透传
	}

	// 模拟 recordUsageCore：构造方负责预填 SubscriptionTokens（buildUsageBillingCommand 复用，不重算）。
	p.SubscriptionTokens = computeSubscriptionTokens(usageLog, p)
	cmd := buildUsageBillingCommand("req-1", usageLog, p)
	if cmd == nil || cmd.SubscriptionID == nil || *cmd.SubscriptionID != 9 {
		t.Fatalf("token 订阅应进 SubscriptionID 分支，cmd=%+v", cmd)
	}
	// TotalTokens=200，倍率 2.0 → 有效 token 400（无高峰）
	if cmd.SubscriptionTokens != 400 {
		t.Errorf("SubscriptionTokens 应为 400（200×2.0），得到 %d", cmd.SubscriptionTokens)
	}
	if !cmd.IsSubscriptionToken {
		t.Error("IsSubscriptionToken 应为 true")
	}
	if cmd.SubscriptionCost != 0 {
		t.Errorf("token 订阅不应记 USD（无 USD 限额），SubscriptionCost=%v，期望 0", cmd.SubscriptionCost)
	}
}

// TestSubscriptionTokensFormulaConsistency 锁定 token 型订阅有效 token 数在两条
// 计算路径上必须相等：buildUsageBillingCommand 产出的 cmd.SubscriptionTokens（DB 累加）
// 与 computeSubscriptionTokens 的返回值（Redis 累加）。任一公式独立改动都会导致
// Redis 缓存的 *_usage_tokens 与 DB 的 *_usage_tokens 静默漂移。
func TestSubscriptionTokensFormulaConsistency(t *testing.T) {
	cases := []struct {
		name       string
		usageLog   *UsageLog
		multiplier float64
		isToken    bool
	}{
		{"token group multiplier 2x", &UsageLog{InputTokens: 100, OutputTokens: 50, CacheCreationTokens: 10, CacheReadTokens: 40}, 2.0, true},
		{"token group multiplier 1x", &UsageLog{InputTokens: 300, OutputTokens: 0, CacheCreationTokens: 0, CacheReadTokens: 0}, 1.0, true},
		{"token group zero tokens", &UsageLog{InputTokens: 0, OutputTokens: 0, CacheCreationTokens: 0, CacheReadTokens: 0}, 3.0, true},
		{"usd group (both must be 0)", &UsageLog{InputTokens: 100, OutputTokens: 100, CacheCreationTokens: 0, CacheReadTokens: 0}, 2.0, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			subType := SubscriptionTypeSubscription
			if tc.isToken {
				subType = SubscriptionTypeSubscriptionToken
			}
			grp := &Group{SubscriptionType: subType, RateMultiplier: tc.multiplier}
			p := &postUsageBillingParams{
				Cost:                     &CostBreakdown{TotalCost: 1.0, ActualCost: tc.multiplier},
				User:                     &User{ID: 1},
				APIKey:                   &APIKey{ID: 2, GroupID: ptrInt64(1), Group: grp},
				Account:                  &Account{ID: 3, Type: AccountTypeAPIKey},
				Subscription:             &UserSubscription{ID: 9},
				IsSubscriptionBill:       true,
				EffectiveTokenMultiplier: tc.multiplier,
			}
			p.SubscriptionTokens = computeSubscriptionTokens(tc.usageLog, p) // 模拟 recordUsageCore 预填
			cmd := buildUsageBillingCommand("req-x", tc.usageLog, p)
			direct := computeSubscriptionTokens(tc.usageLog, p)
			if cmd.SubscriptionTokens != direct {
				t.Errorf("%s: cmd.SubscriptionTokens=%d != computeSubscriptionTokens=%d (drift between DB and Redis paths)",
					tc.name, cmd.SubscriptionTokens, direct)
			}
		})
	}
}
