//go:build unit

package service

import (
	"testing"

	"github.com/stretchr/testify/require"
)

// TestNormalizeLimiterValue_IntegerTypesRound 验证 rpm/tpm/tpd/concurrency 的 LimitValue
// 在 normalize 阶段被强制取整，截断浮点漂移。
//
// 真实回归场景：用户输入 1000000，前端 IEEE 754 累积误差让 API 收到 999999.999997；
// math.Round 还原成 1000000，落库后再读出仍然是 1000000。
func TestNormalizeLimiterValue_IntegerTypesRound(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name  string
		kind  string
		input float64
		want  float64
	}{
		{"rpm 浮点漂移 999999.999997 → 1000000", ServiceQuotaLimiterRPM, 999999.999997, 1000000},
		{"tpm 浮点漂移 1000000.000003 → 1000000", ServiceQuotaLimiterTPM, 1000000.000003, 1000000},
		{"tpd 1000.5 → 1001（向上 round）", ServiceQuotaLimiterTPD, 1000.5, 1001},
		{"tpd 1000.4 → 1000（向下 round）", ServiceQuotaLimiterTPD, 1000.4, 1000},
		{"concurrency 5.7 → 6", ServiceQuotaLimiterConcurrency, 5.7, 6},
		{"rpm 整数 1000000 不变（round-trip 安全）", ServiceQuotaLimiterRPM, 1000000, 1000000},
		{"rpm 1.0 → 1", ServiceQuotaLimiterRPM, 1.0, 1},
	}
	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			l := &ServiceQuotaLimiterInput{LimiterType: tc.kind, LimitValue: tc.input}
			normalizeLimiterValue(l)
			require.Equal(t, tc.want, l.LimitValue,
				"normalize 后 LimitValue 应为整数 %v，实际 %v", tc.want, l.LimitValue)
		})
	}
}

// TestNormalizeLimiterValue_DailyUSDPreservesFraction 验证 daily_usd 不被取整，
// 0.123456 这类小数金额必须原样保留（金额业务必须保留小数精度）。
func TestNormalizeLimiterValue_DailyUSDPreservesFraction(t *testing.T) {
	t.Parallel()

	cases := []float64{0.123456, 1234.56789, 0.000001, 99.99}
	for _, v := range cases {
		v := v
		t.Run("", func(t *testing.T) {
			t.Parallel()
			l := &ServiceQuotaLimiterInput{LimiterType: ServiceQuotaLimiterDailyUSD, LimitValue: v}
			normalizeLimiterValue(l)
			require.Equal(t, v, l.LimitValue, "daily_usd 不应被取整")
		})
	}
}

// TestServiceQuotaLimiterTypeIsInteger 验证类型分类 helper 的判定。
func TestServiceQuotaLimiterTypeIsInteger(t *testing.T) {
	t.Parallel()

	cases := []struct {
		kind string
		want bool
	}{
		{ServiceQuotaLimiterRPM, true},
		{ServiceQuotaLimiterTPM, true},
		{ServiceQuotaLimiterTPD, true},
		{ServiceQuotaLimiterConcurrency, true},
		{ServiceQuotaLimiterDailyUSD, false},
		{"unknown_type", false},
	}
	for _, tc := range cases {
		tc := tc
		t.Run(tc.kind, func(t *testing.T) {
			t.Parallel()
			require.Equal(t, tc.want, ServiceQuotaLimiterTypeIsInteger(tc.kind))
		})
	}
}

// TestNormalizeLimiters_RoundsIntegerLimitValuesViaPipeline 集成验证：
// 通过完整 normalizeLimiters 入口而不是直接调 normalizeLimiterValue，
// 确保 round 真的在 normalize pipeline 里执行，而不是被忘在某个分支。
func TestNormalizeLimiters_RoundsIntegerLimitValuesViaPipeline(t *testing.T) {
	t.Parallel()
	input := &ServiceQuotaRuleInput{
		Limiters: []ServiceQuotaLimiterInput{
			{LimiterType: ServiceQuotaLimiterRPM, WindowMode: ServiceQuotaWindowFixed, LimitValue: 999999.999997},
			{LimiterType: ServiceQuotaLimiterDailyUSD, WindowMode: ServiceQuotaWindowFixed, LimitValue: 99.99},
		},
	}
	require.NoError(t, normalizeLimiters(input))
	require.Equal(t, float64(1000000), input.Limiters[0].LimitValue, "rpm 应被 round 到 1000000")
	require.Equal(t, 99.99, input.Limiters[1].LimitValue, "daily_usd 应保留小数")
}
