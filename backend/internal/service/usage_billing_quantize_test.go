package service

import (
	"math"
	"testing"

	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/require"
)

// decimalPlaces 返回 float64 最短十进制表示的小数位数，
// 即 PostgreSQL 把 float8 参数转成 numeric 时看到的刻度。
func decimalPlaces(v float64) int32 {
	return -decimal.NewFromFloat(v).Exponent()
}

// 复现 #5229：同一笔 ActualCost 分别流向
//
//	balance    = balance - $1
//	quota_used = quota_used + $1
//
// 两列都是 NUMERIC(20,8)，PostgreSQL 按 half-away-from-zero 舍入「运算结果」。
// 金额在第 9 位落在 half 边界时，减法与加法会朝相反方向舍入，
// 导致余额 delta 与 Key 配额 delta 相差 1e-8，永远无法精确对账。
//
// 修复后命令里的金额已经落在 8 位刻度上，存储阶段不再舍入。
func TestUsageBillingCommandQuantizesBalanceAndQuotaIdentically(t *testing.T) {
	// 10 input × 0.00000125 + 5 output × 0.00001000 = 0.0000625
	// 0.0000625 × 1.25（分组倍率） = 0.000078125
	const actualCost = 0.000078125

	cmd := &UsageBillingCommand{
		RequestID:       "req-5229",
		UserID:          1,
		APIKeyID:        2,
		AccountID:       3,
		BalanceCost:     actualCost,
		APIKeyQuotaCost: actualCost,
	}
	cmd.Normalize()

	require.Equal(t, cmd.BalanceCost, cmd.APIKeyQuotaCost,
		"余额扣减与 API Key 配额累加必须使用同一个规范金额")
	require.LessOrEqual(t, decimalPlaces(cmd.BalanceCost), int32(UsageBillingMonetaryScale),
		"金额超过 NUMERIC(20,8) 刻度时 PostgreSQL 仍会在存储阶段舍入")
}

// 第 9 位 half 边界的表驱动覆盖。
func TestQuantizeUsageBillingAmountBoundaries(t *testing.T) {
	cases := []struct {
		name string
		in   float64
	}{
		{"below_half", 0.000078120},
		{"just_below_half", 0.000078124},
		{"exact_half", 0.000078125},
		{"just_above_half", 0.000078126},
		{"above_half", 0.000078130},
		{"long_tail", 0.0000781234567},
		{"already_quantized", 0.00007813},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := QuantizeUsageBillingAmount(tc.in)

			require.LessOrEqual(t, decimalPlaces(got), int32(UsageBillingMonetaryScale))

			want, _ := decimal.NewFromFloat(tc.in).Round(UsageBillingMonetaryScale).Float64()
			require.Equal(t, want, got)

			// 量化误差不得超过半个刻度（5e-9）。
			require.LessOrEqual(t, math.Abs(got-tc.in), 5e-9)
		})
	}
}

// 累积对账：重复应用同一笔金额，余额侧与配额侧的总量必须精确相等，
// 不允许依赖 epsilon 比较。
func TestQuantizedAmountsReconcileExactlyOverManyApplications(t *testing.T) {
	const actualCost = 0.000078125

	cmd := &UsageBillingCommand{
		RequestID:           "req-5229-bulk",
		UserID:              1,
		APIKeyID:            2,
		AccountID:           3,
		BalanceCost:         actualCost,
		SubscriptionCost:    0,
		APIKeyQuotaCost:     actualCost,
		APIKeyRateLimitCost: actualCost,
	}
	cmd.Normalize()

	unit := decimal.NewFromFloat(cmd.BalanceCost)
	for _, n := range []int64{1, 10, 100, 1000} {
		total := unit.Mul(decimal.NewFromInt(n))

		balance := decimal.NewFromInt(10000).Sub(total)
		quotaUsed := total

		// NUMERIC(20,8) 存储不改变任何一侧的值。
		require.True(t, balance.Equal(balance.Round(UsageBillingMonetaryScale)),
			"n=%d 余额结果超出 NUMERIC(20,8) 刻度", n)
		require.True(t, quotaUsed.Equal(quotaUsed.Round(UsageBillingMonetaryScale)),
			"n=%d 配额结果超出 NUMERIC(20,8) 刻度", n)

		balanceDelta := decimal.NewFromInt(10000).Sub(balance)
		require.True(t, balanceDelta.Equal(quotaUsed),
			"n=%d 余额 delta 与配额 delta 不相等: %s vs %s", n, balanceDelta, quotaUsed)
	}
}

// 所有金额字段都要量化，避免订阅用量 / 账号配额留在未规范化的刻度上。
func TestNormalizeQuantizesEveryMonetaryField(t *testing.T) {
	const raw = 0.0000781234567

	cmd := &UsageBillingCommand{
		RequestID:           "req-5229-fields",
		UserID:              1,
		APIKeyID:            2,
		AccountID:           3,
		BalanceCost:         raw,
		SubscriptionCost:    raw,
		APIKeyQuotaCost:     raw,
		APIKeyRateLimitCost: raw,
		AccountQuotaCost:    raw,
	}
	cmd.Normalize()

	for name, got := range map[string]float64{
		"BalanceCost":         cmd.BalanceCost,
		"SubscriptionCost":    cmd.SubscriptionCost,
		"APIKeyQuotaCost":     cmd.APIKeyQuotaCost,
		"APIKeyRateLimitCost": cmd.APIKeyRateLimitCost,
		"AccountQuotaCost":    cmd.AccountQuotaCost,
	} {
		require.LessOrEqual(t, decimalPlaces(got), int32(UsageBillingMonetaryScale), name)
	}
}

// 指纹是请求幂等键，必须仍由原始金额派生：
// 若量化发生在指纹之前，升级前后同一 request_id 的重试会算出不同指纹，
// 被误判为 fingerprint conflict。
func TestNormalizeKeepsFingerprintDerivedFromRawAmounts(t *testing.T) {
	const raw = 0.000078125

	newCmd := func() *UsageBillingCommand {
		return &UsageBillingCommand{
			RequestID:       "req-5229-fp",
			UserID:          1,
			APIKeyID:        2,
			AccountID:       3,
			BalanceCost:     raw,
			APIKeyQuotaCost: raw,
		}
	}

	cmd := newCmd()
	expected := buildUsageBillingFingerprint(newCmd())

	cmd.Normalize()
	require.Equal(t, expected, cmd.RequestFingerprint)
}

// 显式设置的指纹不被覆盖，且金额仍会被量化。
func TestNormalizePreservesExplicitFingerprint(t *testing.T) {
	cmd := &UsageBillingCommand{
		RequestID:          "req-5229-explicit",
		RequestFingerprint: "preset-fingerprint",
		BalanceCost:        0.0000781234567,
	}
	cmd.Normalize()

	require.Equal(t, "preset-fingerprint", cmd.RequestFingerprint)
	require.LessOrEqual(t, decimalPlaces(cmd.BalanceCost), int32(UsageBillingMonetaryScale))
}

func TestQuantizeUsageBillingAmountPassesThroughNonFinite(t *testing.T) {
	require.Equal(t, 0.0, QuantizeUsageBillingAmount(0))
	require.True(t, math.IsNaN(QuantizeUsageBillingAmount(math.NaN())))
	require.True(t, math.IsInf(QuantizeUsageBillingAmount(math.Inf(1)), 1))
	require.True(t, math.IsInf(QuantizeUsageBillingAmount(math.Inf(-1)), -1))
}

// 退款/负向金额同样按 half-away-from-zero 对称处理。
func TestQuantizeUsageBillingAmountHandlesNegativeAmounts(t *testing.T) {
	got := QuantizeUsageBillingAmount(-0.000078125)
	want, _ := decimal.NewFromFloat(-0.000078125).Round(UsageBillingMonetaryScale).Float64()

	require.Equal(t, want, got)
	require.Equal(t, -QuantizeUsageBillingAmount(0.000078125), got,
		"正负金额必须对称量化")
}
