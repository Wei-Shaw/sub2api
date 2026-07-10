package service

import (
	"context"
	"math"
	"strconv"
	"strings"
)

// MonthlyBudgetUsagePercent returns spend/budget*100. Budget <= 0 means unset → 0.
func MonthlyBudgetUsagePercent(spendCNY, budgetCNY float64) float64 {
	if budgetCNY <= 0 || math.IsNaN(budgetCNY) || math.IsInf(budgetCNY, 0) {
		return 0
	}
	safeSpend := spendCNY
	if math.IsNaN(safeSpend) || math.IsInf(safeSpend, 0) || safeSpend < 0 {
		safeSpend = 0
	}
	return (safeSpend / budgetCNY) * 100
}

// GetCompanyMonthlyBudgetCNY returns the company monthly budget in CNY (0 = unset).
func (s *SettingService) GetCompanyMonthlyBudgetCNY(ctx context.Context) float64 {
	if s == nil || s.settingRepo == nil {
		return 0
	}
	raw, err := s.settingRepo.GetValue(ctx, SettingKeyCompanyMonthlyBudgetCNY)
	if err != nil {
		return 0
	}
	v, err := strconv.ParseFloat(strings.TrimSpace(raw), 64)
	if err != nil || v < 0 || math.IsNaN(v) || math.IsInf(v, 0) {
		return 0
	}
	return v
}

// SetCompanyMonthlyBudgetCNY persists the company monthly budget in CNY (0 clears).
func (s *SettingService) SetCompanyMonthlyBudgetCNY(ctx context.Context, budgetCNY float64) error {
	if s == nil || s.settingRepo == nil {
		return ErrSettingNotFound
	}
	if budgetCNY < 0 || math.IsNaN(budgetCNY) || math.IsInf(budgetCNY, 0) {
		budgetCNY = 0
	}
	return s.settingRepo.Set(ctx, SettingKeyCompanyMonthlyBudgetCNY, strconv.FormatFloat(budgetCNY, 'f', 2, 64))
}
