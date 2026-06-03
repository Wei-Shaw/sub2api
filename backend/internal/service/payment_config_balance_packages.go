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

// normalizeBalancePackageActualCredits keeps actual_credits as package/admin
// metadata. Balance packages credit amount_ledger to the user's wallet; runtime
// burn follows upstream provider/model TotalCost × rate_multiplier and must not
// use token_price_per_million or derive synthetic credits from $/rate.
func normalizeBalancePackageActualCredits(actualCredits int64) int64 {
	if actualCredits < 0 {
		return 0
	}
	return actualCredits
}

func computeDisplayCreditsFromLedgerPrice(amountLedger, rateMultiplier, tokenPricePerMillion float64) int64 {
	if amountLedger <= 0 || rateMultiplier <= 0 || tokenPricePerMillion <= 0 {
		return 0
	}
	credits := amountLedger / rateMultiplier / tokenPricePerMillion * 1_000_000
	return int64(math.Round(credits))
}

func validateBalancePackageRequired(code, label string, amountLedger float64) error {
	if strings.TrimSpace(code) == "" {
		return infraerrors.BadRequest("BALANCE_PACKAGE_CODE_REQUIRED", "package code is required")
	}
	if strings.TrimSpace(label) == "" {
		return infraerrors.BadRequest("BALANCE_PACKAGE_LABEL_REQUIRED", "package label is required")
	}
	if math.IsNaN(amountLedger) || math.IsInf(amountLedger, 0) || amountLedger <= 0 {
		return infraerrors.BadRequest("BALANCE_PACKAGE_AMOUNT_INVALID", "amount must be > 0")
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
	return nil
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
		return "tokens"
	}
	return unit
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
	if err := validateBalancePackageRequired(req.Code, req.Label, req.AmountLedger); err != nil {
		return nil, err
	}
	groupID := balancePackageGroupID(req.BalanceGroupID, req.GroupID)
	if _, err := s.loadBalancePackageGroup(ctx, groupID); err != nil {
		return nil, err
	}
	amount := roundLedgerAmountForCredit(req.AmountLedger, defaultLedgerCurrency)
	actualCredits := normalizeBalancePackageActualCredits(req.ActualCredits)
	b := s.entClient.BalancePackage.Create().
		SetCode(strings.TrimSpace(req.Code)).
		SetLabel(strings.TrimSpace(req.Label)).
		SetDescription(strings.TrimSpace(req.Description)).
		SetAmountLedger(amount).
		SetActualCredits(actualCredits).
		SetCreditUnit(normalizeBalancePackageCreditUnit(req.CreditUnit)).
		SetBadge(strings.TrimSpace(req.Badge)).
		SetPopular(req.Popular).
		SetForSale(req.ForSale).
		SetSortOrder(req.SortOrder)
	if groupID != nil {
		b.SetGroupID(*groupID)
	}
	if len(req.CurrencyOverrides) > 0 {
		b.SetCurrencyOverrides(normalizeCurrencyOverrides(req.CurrencyOverrides))
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
	if _, err := s.loadBalancePackageGroup(ctx, groupID); err != nil {
		return nil, err
	}
	existing, err := s.entClient.BalancePackage.Get(ctx, id)
	if err != nil {
		return nil, infraerrors.NotFound("BALANCE_PACKAGE_NOT_FOUND", "balance package not found")
	}
	amount := existing.AmountLedger
	if req.AmountLedger != nil {
		amount = *req.AmountLedger
	}
	amount = roundLedgerAmountForCredit(amount, defaultLedgerCurrency)
	u := s.entClient.BalancePackage.UpdateOneID(id).
		SetAmountLedger(amount)
	if req.ActualCredits != nil {
		u.SetActualCredits(normalizeBalancePackageActualCredits(*req.ActualCredits))
	}
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
	if req.CurrencyOverrides != nil {
		u.SetCurrencyOverrides(normalizeCurrencyOverrides(req.CurrencyOverrides))
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
			ID:                pkg.Code,
			Label:             pkg.Label,
			Description:       pkg.Description,
			AmountLedger:      pkg.AmountLedger,
			ActualCredits:     pkg.ActualCredits,
			CreditUnit:        normalizeBalancePackageCreditUnit(pkg.CreditUnit),
			BalanceGroupID:    pkg.GroupID,
			GroupID:           pkg.GroupID,
			CurrencyOverrides: pkg.CurrencyOverrides,
			Badge:             pkg.Badge,
			Popular:           pkg.Popular,
			SortOrder:         pkg.SortOrder,
		})
	}
	return out
}
