//go:build unit

package service

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

type accountRepoStubForClearAccountError struct {
	mockAccountRepoForGemini
	account                  *Account
	clearErrorCalls          int
	clearRateLimitCalls      int
	clearAntigravityCalls    int
	clearModelRateLimitCalls int
	clearTempUnschedCalls    int
}

func (r *accountRepoStubForClearAccountError) GetByID(ctx context.Context, id int64) (*Account, error) {
	return r.account, nil
}

func (r *accountRepoStubForClearAccountError) ClearError(ctx context.Context, id int64) error {
	r.clearErrorCalls++
	r.account.Status = StatusActive
	r.account.ErrorMessage = ""
	return nil
}

func (r *accountRepoStubForClearAccountError) ClearRateLimit(ctx context.Context, id int64) error {
	r.clearRateLimitCalls++
	if r.account.IsGrokFreeRecoveryPending() {
		return nil
	}
	r.account.RateLimitedAt = nil
	r.account.RateLimitResetAt = nil
	return nil
}

func (r *accountRepoStubForClearAccountError) ClearAntigravityQuotaScopes(ctx context.Context, id int64) error {
	r.clearAntigravityCalls++
	return nil
}

func (r *accountRepoStubForClearAccountError) ClearModelRateLimits(ctx context.Context, id int64) error {
	r.clearModelRateLimitCalls++
	return nil
}

func (r *accountRepoStubForClearAccountError) ClearTempUnschedulable(ctx context.Context, id int64) error {
	r.clearTempUnschedCalls++
	r.account.TempUnschedulableUntil = nil
	r.account.TempUnschedulableReason = ""
	return nil
}

func TestAdminService_ClearAccountError_AlsoClearsRecoverableRuntimeState(t *testing.T) {
	until := time.Now().Add(10 * time.Minute)
	resetAt := time.Now().Add(5 * time.Minute)
	repo := &accountRepoStubForClearAccountError{
		account: &Account{
			ID:                      31,
			Platform:                PlatformOpenAI,
			Type:                    AccountTypeOAuth,
			Status:                  StatusError,
			ErrorMessage:            "refresh failed",
			RateLimitResetAt:        &resetAt,
			TempUnschedulableUntil:  &until,
			TempUnschedulableReason: "missing refresh token",
		},
	}
	blocker := &runtimeBlockRecorder{}
	svc := &adminServiceImpl{accountRepo: repo, runtimeBlocker: blocker}

	updated, err := svc.ClearAccountError(context.Background(), 31)
	require.NoError(t, err)
	require.NotNil(t, updated)
	require.Equal(t, 1, repo.clearErrorCalls)
	require.Equal(t, 1, repo.clearRateLimitCalls)
	require.Equal(t, 1, repo.clearAntigravityCalls)
	require.Equal(t, 1, repo.clearModelRateLimitCalls)
	require.Equal(t, 1, repo.clearTempUnschedCalls)
	require.Nil(t, updated.RateLimitResetAt)
	require.Nil(t, updated.TempUnschedulableUntil)
	require.Empty(t, updated.TempUnschedulableReason)
	require.Equal(t, []int64{31}, blocker.clearedIDs)
}

func TestAdminService_ClearAccountError_PreservesGrokFreePendingForSingleAndBulkResetPath(t *testing.T) {
	limitedAt := time.Now().Add(-time.Minute)
	resetAt := time.Now().Add(9 * time.Minute)
	nextProbeAt := time.Now().Add(4 * time.Minute).UTC().Format(time.RFC3339Nano)
	repo := &accountRepoStubForClearAccountError{
		account: &Account{
			ID:               52,
			Platform:         PlatformGrok,
			Type:             AccountTypeOAuth,
			Status:           StatusError,
			ErrorMessage:     "unrelated transient error",
			RateLimitedAt:    &limitedAt,
			RateLimitResetAt: &resetAt,
			Extra: map[string]any{
				GrokFreeRecoveryPendingExtraKey:     true,
				GrokFreeRecoveryNextProbeAtExtraKey: nextProbeAt,
			},
		},
	}
	blocker := &runtimeBlockRecorder{}
	svc := &adminServiceImpl{accountRepo: repo, runtimeBlocker: blocker}

	updated, err := svc.ClearAccountError(context.Background(), 52)

	require.NoError(t, err)
	require.Equal(t, StatusActive, updated.Status, "generic reset may clear an unrelated error")
	require.Equal(t, 1, repo.clearRateLimitCalls, "single and bulk handlers share this service path")
	require.Same(t, &limitedAt, updated.RateLimitedAt)
	require.Same(t, &resetAt, updated.RateLimitResetAt)
	require.True(t, updated.IsGrokFreeRecoveryPending())
	require.Equal(t, nextProbeAt, updated.Extra[GrokFreeRecoveryNextProbeAtExtraKey])
	require.Empty(t, blocker.clearedIDs, "pending recovery must retain the in-memory scheduling block")
}
