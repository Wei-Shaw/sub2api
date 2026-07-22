//go:build unit

package service

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestPublicRedeemAllowsLastAttemptBeforeLimit(t *testing.T) {
	cache := &paymentFulfillmentRedeemCacheStub{count: redeemMaxFailedAttempts - 1}
	redeemRepo := &redeemRejectRepo{code: RedeemCode{Code: "OTHER"}}
	svc := &RedeemService{redeemRepo: redeemRepo, cache: cache}

	result, err := svc.Redeem(context.Background(), 42, "MISSING")

	require.Nil(t, result)
	require.ErrorIs(t, err, ErrRedeemCodeNotFound)
	require.Equal(t, redeemMaxFailedAttempts, cache.count)
	require.Equal(t, 1, cache.getCalls)
	require.Equal(t, 1, cache.incrementCalls)
	require.Equal(t, 1, cache.acquireCalls)
	require.Equal(t, 1, cache.releaseCalls)
}

func TestPublicRedeemCountsOnlyConfiguredDomainFailures(t *testing.T) {
	past := time.Now().Add(-time.Minute)
	tests := []struct {
		name    string
		code    RedeemCode
		input   string
		wantErr error
	}{
		{
			name:    "not found",
			code:    RedeemCode{Code: "OTHER"},
			input:   "MISSING",
			wantErr: ErrRedeemCodeNotFound,
		},
		{
			name: "expired",
			code: RedeemCode{
				ID: 1, Code: "EXPIRED", Type: RedeemTypeBalance, Value: 10,
				Status: StatusUnused, ExpiresAt: &past,
			},
			input:   "EXPIRED",
			wantErr: ErrRedeemCodeExpired,
		},
		{
			name: "used",
			code: RedeemCode{
				ID: 2, Code: "USED", Type: RedeemTypeBalance, Value: 10,
				Status: StatusUsed,
			},
			input:   "USED",
			wantErr: ErrRedeemCodeUsed,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cache := &paymentFulfillmentRedeemCacheStub{}
			svc := &RedeemService{
				redeemRepo: &redeemRejectRepo{code: tt.code},
				cache:      cache,
			}

			result, err := svc.Redeem(context.Background(), 42, tt.input)

			require.Nil(t, result)
			require.ErrorIs(t, err, tt.wantErr)
			require.Equal(t, 1, cache.getCalls)
			require.Equal(t, 1, cache.incrementCalls)
			require.Equal(t, 1, cache.count)
		})
	}
}

func TestPublicRedeemRateLimitFailsOpenWhenCounterReadFails(t *testing.T) {
	cache := &paymentFulfillmentRedeemCacheStub{
		count:  redeemMaxFailedAttempts,
		getErr: errors.New("redis unavailable"),
	}
	svc := &RedeemService{
		redeemRepo: &redeemRejectRepo{code: RedeemCode{Code: "OTHER"}},
		cache:      cache,
	}

	result, err := svc.Redeem(context.Background(), 42, "MISSING")

	require.Nil(t, result)
	require.ErrorIs(t, err, ErrRedeemCodeNotFound)
	require.Equal(t, 1, cache.getCalls)
	require.Equal(t, 1, cache.incrementCalls)
}

func TestPublicRedeemCounterWriteFailurePreservesDomainError(t *testing.T) {
	cache := &paymentFulfillmentRedeemCacheStub{incrementErr: errors.New("redis unavailable")}
	svc := &RedeemService{
		redeemRepo: &redeemRejectRepo{code: RedeemCode{Code: "OTHER"}},
		cache:      cache,
	}

	result, err := svc.Redeem(context.Background(), 42, "MISSING")

	require.Nil(t, result)
	require.ErrorIs(t, err, ErrRedeemCodeNotFound)
	require.Equal(t, 1, cache.incrementCalls)
	require.Zero(t, cache.count)
}

func TestSuccessfulPublicRedeemDoesNotMutateFailureCounter(t *testing.T) {
	ctx := context.Background()
	client := newPaymentConfigServiceTestClient(t)
	userID := int64(42)
	code := &RedeemCode{
		ID: 101, Code: "PUBLIC-SUCCESS", Type: RedeemTypeBalance, Value: 10, Status: StatusUnused,
	}
	redeemRepo := &paymentFulfillmentRedeemRepo{
		paymentOrderLifecycleRedeemRepo: paymentOrderLifecycleRedeemRepo{
			codesByCode: map[string]*RedeemCode{code.Code: code},
		},
	}
	userRepo := &mockUserRepo{getByIDUser: &User{ID: userID}}
	userRepo.updateBalanceFn = func(context.Context, int64, float64) error { return nil }
	cache := &paymentFulfillmentRedeemCacheStub{count: 7}
	svc := NewRedeemService(redeemRepo, userRepo, nil, cache, nil, client, nil, nil)

	result, err := svc.Redeem(ctx, userID, code.Code)

	require.NoError(t, err)
	require.NotNil(t, result)
	require.Equal(t, 1, cache.getCalls)
	require.Zero(t, cache.incrementCalls)
	require.Equal(t, 7, cache.count)
	require.Equal(t, 1, cache.acquireCalls)
	require.Equal(t, 1, cache.releaseCalls)
	require.Len(t, redeemRepo.useCalls, 1)
}

func TestAdminFulfillmentBypassesLimitAndKeepsRedeemAffiliate(t *testing.T) {
	ctx := context.Background()
	client := newPaymentConfigServiceTestClient(t)
	userID := int64(42)
	inviterID := int64(9001)
	code := &RedeemCode{
		ID: 102, Code: "ADMIN-SUCCESS", Type: RedeemTypeBalance, Value: 20, Status: StatusUnused,
	}
	redeemRepo := &paymentFulfillmentRedeemRepo{
		paymentOrderLifecycleRedeemRepo: paymentOrderLifecycleRedeemRepo{
			codesByCode: map[string]*RedeemCode{code.Code: code},
		},
	}
	userRepo := &mockUserRepo{getByIDUser: &User{ID: userID}}
	userRepo.updateBalanceFn = func(context.Context, int64, float64) error { return nil }
	cache := &paymentFulfillmentRedeemCacheStub{count: redeemMaxFailedAttempts}
	affiliateRepo := &paymentFulfillmentAffiliateRepoStub{
		inviteeSummary: &AffiliateSummary{
			UserID: userID, AffCode: "INVITEE", InviterID: &inviterID, CreatedAt: time.Now().Add(-time.Hour),
		},
		inviterSummary: &AffiliateSummary{
			UserID: inviterID, AffCode: "INVITER", CreatedAt: time.Now().Add(-2 * time.Hour),
		},
	}
	settingSvc := NewSettingService(&paymentFulfillmentSettingRepoStub{values: map[string]string{
		SettingKeyAffiliateEnabled:           "true",
		SettingKeyAffiliateRebateRate:        "10",
		SettingKeyAffiliateRebateFreezeHours: "0",
	}}, nil)
	affiliateSvc := NewAffiliateService(affiliateRepo, settingSvc, nil, nil)
	svc := NewRedeemService(redeemRepo, userRepo, nil, cache, nil, client, nil, affiliateSvc)

	result, err := svc.RedeemForAdminFulfillment(ctx, userID, code.Code)

	require.NoError(t, err)
	require.NotNil(t, result)
	require.Zero(t, cache.getCalls)
	require.Zero(t, cache.incrementCalls)
	require.Equal(t, 1, cache.acquireCalls)
	require.Equal(t, 1, cache.releaseCalls)
	require.Len(t, redeemRepo.useCalls, 1)
	require.Len(t, affiliateRepo.accrueCalls, 1)
	require.Equal(t, inviterID, affiliateRepo.accrueCalls[0].inviterID)
	require.Equal(t, userID, affiliateRepo.accrueCalls[0].inviteeUserID)
	require.InDelta(t, 2, affiliateRepo.accrueCalls[0].amount, 1e-8)
	require.Nil(t, affiliateRepo.accrueCalls[0].sourceOrderID)
}
