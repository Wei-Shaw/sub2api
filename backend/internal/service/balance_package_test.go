package service

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestDeductBalancePackagesConsumesEarliestExpiryFirst(t *testing.T) {
	now := time.Date(2026, 6, 13, 10, 0, 0, 0, time.UTC)
	earlyExpiry := now.Add(2 * time.Hour)
	lateExpiry := now.Add(4 * time.Hour)
	packages := []UserBalancePackage{
		{ID: 1, UserID: 42, RemainingAmount: 3, ExpiresAt: lateExpiry, Status: BalancePackageStatusActive},
		{ID: 2, UserID: 42, RemainingAmount: 5, ExpiresAt: earlyExpiry, Status: BalancePackageStatusActive},
	}
	repo := newMemoryBalancePackageRepo(packages, 0)

	remaining, err := DeductUserBalanceWithPackages(context.Background(), repo, 42, 6, now)

	require.NoError(t, err)
	require.Zero(t, remaining)
	require.InDelta(t, 2, repo.packageByID(1).RemainingAmount, 1e-9)
	require.InDelta(t, 0, repo.packageByID(2).RemainingAmount, 1e-9)
	require.Equal(t, BalancePackageStatusActive, repo.packageByID(1).Status)
	require.Equal(t, BalancePackageStatusDepleted, repo.packageByID(2).Status)
	require.Empty(t, repo.userBalanceDeltas)
}

func TestDeductBalancePackagesFallsBackToUserBalance(t *testing.T) {
	now := time.Date(2026, 6, 13, 10, 0, 0, 0, time.UTC)
	repo := newMemoryBalancePackageRepo([]UserBalancePackage{
		{ID: 1, UserID: 42, RemainingAmount: 2, ExpiresAt: now.Add(time.Hour), Status: BalancePackageStatusActive},
	}, 10)

	remaining, err := DeductUserBalanceWithPackages(context.Background(), repo, 42, 5, now)

	require.NoError(t, err)
	require.Zero(t, remaining)
	require.InDelta(t, 0, repo.packageByID(1).RemainingAmount, 1e-9)
	require.Equal(t, BalancePackageStatusDepleted, repo.packageByID(1).Status)
	require.Equal(t, []float64{-3}, repo.userBalanceDeltas)
	require.InDelta(t, 7, repo.userBalance, 1e-9)
}

func TestAvailableBalanceIncludesOnlyUnexpiredPackages(t *testing.T) {
	now := time.Date(2026, 6, 13, 10, 0, 0, 0, time.UTC)
	repo := newMemoryBalancePackageRepo([]UserBalancePackage{
		{ID: 1, UserID: 42, RemainingAmount: 2, ExpiresAt: now.Add(time.Hour), Status: BalancePackageStatusActive},
		{ID: 2, UserID: 42, RemainingAmount: 9, ExpiresAt: now.Add(-time.Hour), Status: BalancePackageStatusActive},
		{ID: 3, UserID: 42, RemainingAmount: 4, ExpiresAt: now.Add(time.Hour), Status: BalancePackageStatusDepleted},
	}, 1.5)

	available, err := GetUserAvailableBalanceWithPackages(context.Background(), repo, 42, now)

	require.NoError(t, err)
	require.InDelta(t, 3.5, available, 1e-9)
}

func TestShouldDeductBalanceForBillingTypeOnlyBalance(t *testing.T) {
	require.True(t, shouldDeductBalanceForBillingType(BillingTypeBalance, 1))
	require.False(t, shouldDeductBalanceForBillingType(BillingTypeSubscription, 1))
	require.False(t, shouldDeductBalanceForBillingType(BillingTypeBalance, 0))
}

type memoryBalancePackageRepo struct {
	packages          []UserBalancePackage
	userBalance       float64
	userBalanceDeltas []float64
}

func newMemoryBalancePackageRepo(packages []UserBalancePackage, userBalance float64) *memoryBalancePackageRepo {
	cp := make([]UserBalancePackage, len(packages))
	copy(cp, packages)
	return &memoryBalancePackageRepo{packages: cp, userBalance: userBalance}
}

func (r *memoryBalancePackageRepo) ListActiveBalancePackagesForUpdate(ctx context.Context, userID int64, now time.Time) ([]UserBalancePackage, error) {
	out := make([]UserBalancePackage, 0, len(r.packages))
	for _, pkg := range r.packages {
		if pkg.UserID == userID && pkg.Status == BalancePackageStatusActive && pkg.RemainingAmount > 0 && pkg.ExpiresAt.After(now) {
			out = append(out, pkg)
		}
	}
	return out, nil
}

func (r *memoryBalancePackageRepo) ListUserVisibleBalancePackages(ctx context.Context, userID int64, now time.Time) ([]UserBalancePackage, error) {
	return r.ListActiveBalancePackagesForUpdate(ctx, userID, now)
}

func (r *memoryBalancePackageRepo) CreateBalancePackage(ctx context.Context, pkg *UserBalancePackage) error {
	if pkg.ID == 0 {
		pkg.ID = int64(len(r.packages) + 1)
	}
	r.packages = append(r.packages, *pkg)
	return nil
}

func (r *memoryBalancePackageRepo) UpdateBalancePackageRemaining(ctx context.Context, id int64, remaining float64, status string) error {
	for i := range r.packages {
		if r.packages[i].ID == id {
			r.packages[i].RemainingAmount = remaining
			r.packages[i].Status = status
			return nil
		}
	}
	return ErrBalancePackageNotFound
}

func (r *memoryBalancePackageRepo) GetUserBaseBalance(ctx context.Context, userID int64) (float64, error) {
	return r.userBalance, nil
}

func (r *memoryBalancePackageRepo) UpdateUserBalance(ctx context.Context, userID int64, amount float64) error {
	r.userBalance += amount
	r.userBalanceDeltas = append(r.userBalanceDeltas, amount)
	return nil
}

func (r *memoryBalancePackageRepo) SumActiveBalancePackages(ctx context.Context, userID int64, now time.Time) (float64, error) {
	sum := 0.0
	for _, pkg := range r.packages {
		if pkg.UserID == userID && pkg.Status == BalancePackageStatusActive && pkg.RemainingAmount > 0 && pkg.ExpiresAt.After(now) {
			sum += pkg.RemainingAmount
		}
	}
	return sum, nil
}

func (r *memoryBalancePackageRepo) packageByID(id int64) UserBalancePackage {
	for _, pkg := range r.packages {
		if pkg.ID == id {
			return pkg
		}
	}
	return UserBalancePackage{}
}
