//go:build unit

package service

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestQuoteRechargeBonusConverted(t *testing.T) {
	tiers := []RechargeBonusTier{{MinAmount: 500000, BonusPercent: 10}}

	// 赠金模式：阶梯按 VND 命中，赠送按 USD 基数算，实付不变。
	q := quoteRechargeBonusConverted(&PaymentConfig{RechargeBonusTiers: tiers, RechargeBonusMode: RechargeBonusModeBonus}, 1000000, 1000000, 40, "VND")
	require.Equal(t, rechargeBonusQuote{PayBase: 1000000, Credited: 44, Bonus: 4, Percent: 10}, q)

	// 折扣模式：实付打九折，到账仍为基数，免费部分记为 Bonus。
	q = quoteRechargeBonusConverted(&PaymentConfig{RechargeBonusTiers: tiers, RechargeBonusMode: RechargeBonusModeDiscount}, 1000000, 1000000, 40, "VND")
	require.Equal(t, rechargeBonusQuote{PayBase: 900000, Credited: 40, Bonus: 4, Percent: 10}, q)

	// 未达门槛：无优惠。
	q = quoteRechargeBonusConverted(&PaymentConfig{RechargeBonusTiers: tiers}, 400000, 400000, 16, "VND")
	require.Equal(t, rechargeBonusQuote{PayBase: 400000, Credited: 16}, q)

	// crypto 网关按 USD 计价：$40 折合 1000000 VND，按 VND 命中阶梯，赠送仍按 USD 基数。
	q = quoteRechargeBonusConverted(&PaymentConfig{RechargeBonusTiers: tiers, RechargeBonusMode: RechargeBonusModeBonus}, 40, 1000000, 40, "USD")
	require.Equal(t, rechargeBonusQuote{PayBase: 40, Credited: 44, Bonus: 4, Percent: 10}, q)

	// crypto 折扣模式：实付按 USD 打九折。
	q = quoteRechargeBonusConverted(&PaymentConfig{RechargeBonusTiers: tiers, RechargeBonusMode: RechargeBonusModeDiscount}, 40, 1000000, 40, "USD")
	require.Equal(t, rechargeBonusQuote{PayBase: 36, Credited: 40, Bonus: 4, Percent: 10}, q)

	// 折合 VND 未达门槛（汇率不可用时 tierAmount=0 同理）：无优惠。
	q = quoteRechargeBonusConverted(&PaymentConfig{RechargeBonusTiers: tiers}, 10, 250000, 10, "USD")
	require.Equal(t, rechargeBonusQuote{PayBase: 10, Credited: 10}, q)

	// 到账基数为 0 时绝不能退化成「VND 当 USD」。
	q = quoteRechargeBonusConverted(&PaymentConfig{RechargeBonusTiers: tiers}, 1000000, 1000000, 0, "VND")
	require.Equal(t, rechargeBonusQuote{PayBase: 1000000, Credited: 0}, q)
}
