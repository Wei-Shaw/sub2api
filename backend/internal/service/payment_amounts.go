package service

import (
	"math"
	"strconv"

	"github.com/Wei-Shaw/sub2api/internal/payment"
	"github.com/shopspring/decimal"
)

const defaultBalanceRechargeMultiplier = 1.0

type BalanceRechargePackage struct {
	Amount     float64 `json:"amount"`
	TrialBonus float64 `json:"trial_bonus"`
	TrialDays  int     `json:"trial_days"`
	Label      string  `json:"label,omitempty"`
	SortOrder  int     `json:"sort_order"`
	Enabled    bool    `json:"enabled"`
}

var defaultBalanceRechargePackages = []BalanceRechargePackage{
	{Amount: 10, TrialBonus: 0, TrialDays: 30, SortOrder: 1, Enabled: true},
	{Amount: 20, TrialBonus: 0.5, TrialDays: 1, Label: "推荐", SortOrder: 2, Enabled: true},
	{Amount: 30, TrialBonus: 1, TrialDays: 1, Label: "推荐", SortOrder: 3, Enabled: true},
	{Amount: 50, TrialBonus: 2, TrialDays: 3, Label: "推荐", SortOrder: 5, Enabled: true},
	{Amount: 100, TrialBonus: 5, TrialDays: 30, Label: "推荐", SortOrder: 6, Enabled: true},
	{Amount: 300, TrialBonus: 20, TrialDays: 15, Label: "推荐", SortOrder: 7, Enabled: true},
	{Amount: 500, TrialBonus: 40, TrialDays: 30, Label: "推荐", SortOrder: 8, Enabled: true},
	{Amount: 888, TrialBonus: 88, TrialDays: 30, Label: "88或加入代理", SortOrder: 9, Enabled: true},
}

func normalizeBalanceRechargeMultiplier(multiplier float64) float64 {
	if math.IsNaN(multiplier) || math.IsInf(multiplier, 0) || multiplier <= 0 {
		return defaultBalanceRechargeMultiplier
	}
	return multiplier
}

func calculateCreditedBalance(paymentAmount, multiplier float64) float64 {
	return decimal.NewFromFloat(paymentAmount).
		Mul(decimal.NewFromFloat(normalizeBalanceRechargeMultiplier(multiplier))).
		Round(2).
		InexactFloat64()
}

func lookupBalanceRechargePackage(amount float64, packages []BalanceRechargePackage) (BalanceRechargePackage, bool) {
	normalized := decimal.NewFromFloat(amount).Round(2)
	for _, pkg := range packages {
		if !pkg.Enabled {
			continue
		}
		if normalized.Equal(decimal.NewFromFloat(pkg.Amount).Round(2)) {
			return pkg, true
		}
	}
	return BalanceRechargePackage{}, false
}

func matchBalanceRechargePackageByPayAmount(payAmount, feeRate float64, packages []BalanceRechargePackage) (BalanceRechargePackage, bool) {
	normalizedPay := decimal.NewFromFloat(payAmount).Round(2)
	for _, pkg := range packages {
		if !pkg.Enabled {
			continue
		}
		expectedPay, err := strconv.ParseFloat(payment.CalculatePayAmount(pkg.Amount, feeRate), 64)
		if err != nil {
			continue
		}
		if normalizedPay.Equal(decimal.NewFromFloat(expectedPay).Round(2)) {
			return pkg, true
		}
	}
	return BalanceRechargePackage{}, false
}

func calculateGatewayRefundAmount(orderAmount, payAmount, refundAmount float64) float64 {
	if orderAmount <= 0 || payAmount <= 0 || refundAmount <= 0 {
		return 0
	}
	if math.Abs(refundAmount-orderAmount) <= amountToleranceCNY {
		return decimal.NewFromFloat(payAmount).Round(2).InexactFloat64()
	}
	return decimal.NewFromFloat(payAmount).
		Mul(decimal.NewFromFloat(refundAmount)).
		Div(decimal.NewFromFloat(orderAmount)).
		Round(2).
		InexactFloat64()
}
