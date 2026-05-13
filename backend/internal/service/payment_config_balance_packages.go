package service

import (
	"context"
	"math"
	"strings"

	dbent "github.com/Wei-Shaw/sub2api/ent"
	"github.com/Wei-Shaw/sub2api/ent/balancepackage"
	"github.com/Wei-Shaw/sub2api/ent/group"
	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
)

const balancePackageCreditsPerLedgerUnit = 10000.0

func validateBalancePackageRequired(code, label string, amountLedger, creditLedger, creditMultiplier float64) error {
	if strings.TrimSpace(code) == "" {
		return infraerrors.BadRequest("BALANCE_PACKAGE_CODE_REQUIRED", "package code is required")
	}
	if strings.TrimSpace(label) == "" {
		return infraerrors.BadRequest("BALANCE_PACKAGE_LABEL_REQUIRED", "package label is required")
	}
	if math.IsNaN(amountLedger) || math.IsInf(amountLedger, 0) || amountLedger <= 0 {
		return infraerrors.BadRequest("BALANCE_PACKAGE_AMOUNT_INVALID", "amount must be > 0")
	}
	if creditLedger <= 0 && creditMultiplier <= 0 {
		return infraerrors.BadRequest("BALANCE_PACKAGE_CREDIT_INVALID", "credit amount or multiplier is required")
	}
	if creditLedger > 0 && (math.IsNaN(creditLedger) || math.IsInf(creditLedger, 0)) {
		return infraerrors.BadRequest("BALANCE_PACKAGE_CREDIT_INVALID", "credit amount must be valid")
	}
	if creditMultiplier > 0 && (math.IsNaN(creditMultiplier) || math.IsInf(creditMultiplier, 0)) {
		return infraerrors.BadRequest("BALANCE_PACKAGE_MULTIPLIER_INVALID", "credit multiplier must be valid")
	}
	return nil
}

func validateBalancePackagePatch(req UpdateBalancePackageRequest) error {
	if req.Code != nil && strings.TrimSpace(*req.Code) == "" {
		return infraerrors.BadRequest("BALANCE_PACKAGE_CODE_REQUIRED", "package code is required")
	}
	if req.Label != nil && strings.TrimSpace(*req.Label) == "" {
		return infraerrors.BadRequest("BALANCE_PACKAGE_LABEL_REQUIRED", "package label is required")
	}
	if req.AmountLedger != nil && (math.IsNaN(*req.AmountLedger) || math.IsInf(*req.AmountLedger, 0) || *req.AmountLedger <= 0) {
		return infraerrors.BadRequest("BALANCE_PACKAGE_AMOUNT_INVALID", "amount must be > 0")
	}
	if req.CreditLedger != nil && (math.IsNaN(*req.CreditLedger) || math.IsInf(*req.CreditLedger, 0) || *req.CreditLedger <= 0) {
		return infraerrors.BadRequest("BALANCE_PACKAGE_CREDIT_INVALID", "credit amount must be > 0")
	}
	if req.CreditMultiplier != nil && (math.IsNaN(*req.CreditMultiplier) || math.IsInf(*req.CreditMultiplier, 0) || *req.CreditMultiplier <= 0) {
		return infraerrors.BadRequest("BALANCE_PACKAGE_MULTIPLIER_INVALID", "credit multiplier must be > 0")
	}
	return nil
}

func normalizeBalancePackageAmounts(amountLedger, creditLedger, creditMultiplier float64) (float64, float64, float64, float64) {
	amountLedger = roundLedgerAmountForCredit(amountLedger, defaultLedgerCurrency)
	if creditLedger <= 0 && creditMultiplier > 0 {
		creditLedger = amountLedger * creditMultiplier
	}
	creditLedger = roundLedgerAmountForCredit(creditLedger, defaultLedgerCurrency)
	if creditMultiplier <= 0 && amountLedger > 0 {
		creditMultiplier = creditLedger / amountLedger
	}
	bonusLedger := roundLedgerAmountForCredit(creditLedger-amountLedger, defaultLedgerCurrency)
	if bonusLedger < 0 {
		bonusLedger = 0
	}
	return amountLedger, creditLedger, bonusLedger, creditMultiplier
}

func balancePackageGroupID(balanceGroupID, legacyGroupID *int64) *int64 {
	if balanceGroupID != nil {
		return balanceGroupID
	}
	return legacyGroupID
}

func normalizeBalancePackageCreditUnit(value string) string {
	unit := strings.TrimSpace(value)
	if unit == "" {
		return "credits"
	}
	return unit
}

func computeBalancePackageActualCredits(amountLedger, creditMultiplier float64, balanceGroup *dbent.Group) int64 {
	if amountLedger <= 0 || creditMultiplier <= 0 || balanceGroup == nil || balanceGroup.RateMultiplier <= 0 {
		return 0
	}
	ledgerCredits := amountLedger * creditMultiplier
	return int64(math.Round((ledgerCredits / balanceGroup.RateMultiplier) * balancePackageCreditsPerLedgerUnit))
}

func (s *PaymentConfigService) loadBalancePackageGroup(ctx context.Context, groupID *int64) (*dbent.Group, error) {
	if groupID == nil {
		return nil, nil
	}
	if *groupID <= 0 {
		return nil, infraerrors.BadRequest("BALANCE_PACKAGE_GROUP_INVALID", "balance group is invalid")
	}
	if s == nil || s.entClient == nil {
		return nil, infraerrors.ServiceUnavailable("BALANCE_PACKAGE_STORE_UNAVAILABLE", "balance package store is not available")
	}
	g, err := s.entClient.Group.Query().Where(group.IDEQ(*groupID)).Only(ctx)
	if err != nil || g.Status != StatusActive {
		return nil, infraerrors.NotFound("BALANCE_PACKAGE_GROUP_NOT_FOUND", "balance group not found or inactive")
	}
	if g.SubscriptionType == SubscriptionTypeSubscription {
		return nil, infraerrors.BadRequest("BALANCE_PACKAGE_GROUP_TYPE_MISMATCH", "balance package group must be a standard balance group, not a subscription group")
	}
	return g, nil
}

func (s *PaymentConfigService) ListBalanceRechargePackages(ctx context.Context) ([]*dbent.BalancePackage, error) {
	if s == nil || s.entClient == nil {
		return nil, infraerrors.ServiceUnavailable("BALANCE_PACKAGE_STORE_UNAVAILABLE", "balance package store is not available")
	}
	return s.entClient.BalancePackage.Query().Order(balancepackage.BySortOrder(), balancepackage.ByAmountLedger()).All(ctx)
}

func (s *PaymentConfigService) ListBalanceRechargePackagesForSale(ctx context.Context) ([]*dbent.BalancePackage, error) {
	if s == nil || s.entClient == nil {
		return []*dbent.BalancePackage{}, nil
	}
	return s.entClient.BalancePackage.Query().Where(balancepackage.ForSaleEQ(true)).Order(balancepackage.BySortOrder(), balancepackage.ByAmountLedger()).All(ctx)
}

func (s *PaymentConfigService) CreateBalancePackage(ctx context.Context, req CreateBalancePackageRequest) (*dbent.BalancePackage, error) {
	if s == nil || s.entClient == nil {
		return nil, infraerrors.ServiceUnavailable("BALANCE_PACKAGE_STORE_UNAVAILABLE", "balance package store is not available")
	}
	if err := validateBalancePackageRequired(req.Code, req.Label, req.AmountLedger, req.CreditLedger, req.CreditMultiplier); err != nil {
		return nil, err
	}
	groupID := balancePackageGroupID(req.BalanceGroupID, req.GroupID)
	balanceGroup, err := s.loadBalancePackageGroup(ctx, groupID)
	if err != nil {
		return nil, err
	}
	amount, credit, bonus, multiplier := normalizeBalancePackageAmounts(req.AmountLedger, req.CreditLedger, req.CreditMultiplier)
	actualCredits := computeBalancePackageActualCredits(amount, multiplier, balanceGroup)
	b := s.entClient.BalancePackage.Create().
		SetCode(strings.TrimSpace(req.Code)).
		SetLabel(strings.TrimSpace(req.Label)).
		SetDescription(strings.TrimSpace(req.Description)).
		SetAmountLedger(amount).
		SetCreditLedger(credit).
		SetBonusLedger(bonus).
		SetCreditMultiplier(multiplier).
		SetActualCredits(actualCredits).
		SetCreditUnit(normalizeBalancePackageCreditUnit(req.CreditUnit)).
		SetBadge(strings.TrimSpace(req.Badge)).
		SetPopular(req.Popular).
		SetForSale(req.ForSale).
		SetSortOrder(req.SortOrder)
	if groupID != nil {
		b.SetGroupID(*groupID)
	}
	return b.Save(ctx)
}

// UpdateBalancePackage updates a balance package by ID (patch semantics).
func (s *PaymentConfigService) UpdateBalancePackage(ctx context.Context, id int64, req UpdateBalancePackageRequest) (*dbent.BalancePackage, error) {
	if s == nil || s.entClient == nil {
		return nil, infraerrors.ServiceUnavailable("BALANCE_PACKAGE_STORE_UNAVAILABLE", "balance package store is not available")
	}
	if err := validateBalancePackagePatch(req); err != nil {
		return nil, err
	}
	groupID := balancePackageGroupID(req.BalanceGroupID, req.GroupID)
	balanceGroup, err := s.loadBalancePackageGroup(ctx, groupID)
	if err != nil {
		return nil, err
	}
	existing, err := s.entClient.BalancePackage.Get(ctx, id)
	if err != nil {
		return nil, infraerrors.NotFound("BALANCE_PACKAGE_NOT_FOUND", "balance package not found")
	}
	if groupID == nil && existing.GroupID != nil {
		balanceGroup, err = s.loadBalancePackageGroup(ctx, existing.GroupID)
		if err != nil {
			return nil, err
		}
	}
	amount := existing.AmountLedger
	credit := existing.CreditLedger
	multiplier := existing.CreditMultiplier
	if req.AmountLedger != nil {
		amount = *req.AmountLedger
	}
	if req.CreditLedger != nil {
		credit = *req.CreditLedger
	}
	if req.CreditMultiplier != nil && req.CreditLedger == nil {
		multiplier = *req.CreditMultiplier
		credit = 0
	}
	amount, credit, bonus, multiplier := normalizeBalancePackageAmounts(amount, credit, multiplier)
	actualCredits := computeBalancePackageActualCredits(amount, multiplier, balanceGroup)
	u := s.entClient.BalancePackage.UpdateOneID(id).
		SetAmountLedger(amount).
		SetCreditLedger(credit).
		SetBonusLedger(bonus).
		SetCreditMultiplier(multiplier).
		SetActualCredits(actualCredits)
	if req.Code != nil {
		u.SetCode(strings.TrimSpace(*req.Code))
	}
	if req.Label != nil {
		u.SetLabel(strings.TrimSpace(*req.Label))
	}
	if req.Description != nil {
		u.SetDescription(strings.TrimSpace(*req.Description))
	}
	if groupID != nil {
		u.SetGroupID(*groupID)
	}
	if req.CreditUnit != nil {
		u.SetCreditUnit(normalizeBalancePackageCreditUnit(*req.CreditUnit))
	}
	if req.Badge != nil {
		u.SetBadge(strings.TrimSpace(*req.Badge))
	}
	if req.Popular != nil {
		u.SetPopular(*req.Popular)
	}
	if req.ForSale != nil {
		u.SetForSale(*req.ForSale)
	}
	if req.SortOrder != nil {
		u.SetSortOrder(*req.SortOrder)
	}
	return u.Save(ctx)
}

func (s *PaymentConfigService) DeleteBalancePackage(ctx context.Context, id int64) error {
	if s == nil || s.entClient == nil {
		return infraerrors.ServiceUnavailable("BALANCE_PACKAGE_STORE_UNAVAILABLE", "balance package store is not available")
	}
	return s.entClient.BalancePackage.DeleteOneID(id).Exec(ctx)
}

func balancePackageEntitiesToConfig(packages []*dbent.BalancePackage) []BalanceRechargePackage {
	out := make([]BalanceRechargePackage, 0, len(packages))
	for _, pkg := range packages {
		if pkg == nil {
			continue
		}
		out = append(out, BalanceRechargePackage{
			ID:               pkg.Code,
			Label:            pkg.Label,
			Description:      pkg.Description,
			AmountLedger:     pkg.AmountLedger,
			CreditLedger:     pkg.CreditLedger,
			BonusLedger:      pkg.BonusLedger,
			CreditMultiplier: pkg.CreditMultiplier,
			ActualCredits:    pkg.ActualCredits,
			CreditUnit:       normalizeBalancePackageCreditUnit(pkg.CreditUnit),
			BalanceGroupID:   pkg.GroupID,
			GroupID:          pkg.GroupID,
			Badge:            pkg.Badge,
			Popular:          pkg.Popular,
			SortOrder:        pkg.SortOrder,
		})
	}
	return out
}
