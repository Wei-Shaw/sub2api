package service

import (
	"context"
	"sort"
	"time"

	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
)

const (
	BalancePackageStatusActive   = "active"
	BalancePackageStatusDepleted = "depleted"
	BalancePackageStatusExpired  = "expired"
)

var ErrBalancePackageNotFound = infraerrors.NotFound("BALANCE_PACKAGE_NOT_FOUND", "balance package not found")

type UserBalancePackage struct {
	ID              int64
	UserID          int64
	RedeemCodeID    *int64
	RedeemCode      string
	Amount          float64
	RemainingAmount float64
	ExpiresAt       time.Time
	Status          string
	CreatedAt       time.Time
	UpdatedAt       time.Time
}

type BalancePackageRepository interface {
	CreateBalancePackage(ctx context.Context, pkg *UserBalancePackage) error
	ListActiveBalancePackagesForUpdate(ctx context.Context, userID int64, now time.Time) ([]UserBalancePackage, error)
	UpdateBalancePackageRemaining(ctx context.Context, id int64, remaining float64, status string) error
	GetUserBaseBalance(ctx context.Context, userID int64) (float64, error)
	UpdateUserBalance(ctx context.Context, userID int64, amount float64) error
	SumActiveBalancePackages(ctx context.Context, userID int64, now time.Time) (float64, error)
	ListUserVisibleBalancePackages(ctx context.Context, userID int64, now time.Time) ([]UserBalancePackage, error)
}

func DeductUserBalanceWithPackages(ctx context.Context, repo BalancePackageRepository, userID int64, amount float64, now time.Time) (float64, error) {
	if repo == nil || amount <= 0 {
		return amount, nil
	}
	packages, err := repo.ListActiveBalancePackagesForUpdate(ctx, userID, now)
	if err != nil {
		return amount, err
	}
	sort.SliceStable(packages, func(i, j int) bool {
		if packages[i].ExpiresAt.Equal(packages[j].ExpiresAt) {
			return packages[i].ID < packages[j].ID
		}
		return packages[i].ExpiresAt.Before(packages[j].ExpiresAt)
	})

	remainingCost := amount
	for _, pkg := range packages {
		if remainingCost <= 0 {
			break
		}
		if pkg.RemainingAmount <= 0 {
			continue
		}
		deduct := pkg.RemainingAmount
		if deduct > remainingCost {
			deduct = remainingCost
		}
		newRemaining := pkg.RemainingAmount - deduct
		status := BalancePackageStatusActive
		if newRemaining <= 0 {
			newRemaining = 0
			status = BalancePackageStatusDepleted
		}
		if err := repo.UpdateBalancePackageRemaining(ctx, pkg.ID, newRemaining, status); err != nil {
			return remainingCost, err
		}
		remainingCost -= deduct
	}

	if remainingCost > 0 {
		if err := repo.UpdateUserBalance(ctx, userID, -remainingCost); err != nil {
			return remainingCost, err
		}
	}
	return 0, nil
}

func GetUserAvailableBalanceWithPackages(ctx context.Context, repo BalancePackageRepository, userID int64, now time.Time) (float64, error) {
	if repo == nil {
		return 0, nil
	}
	base, err := repo.GetUserBaseBalance(ctx, userID)
	if err != nil {
		return 0, err
	}
	pkgBalance, err := repo.SumActiveBalancePackages(ctx, userID, now)
	if err != nil {
		return 0, err
	}
	return base + pkgBalance, nil
}
