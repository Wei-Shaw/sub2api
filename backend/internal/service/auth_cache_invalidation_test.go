//go:build unit

package service

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestUsageService_InvalidateUsageCaches(t *testing.T) {
	invalidator := &authCacheInvalidatorStub{}
	svc := &UsageService{authCacheInvalidator: invalidator}

	svc.invalidateUsageCaches(context.Background(), 7, false)
	require.Empty(t, invalidator.userIDs)

	svc.invalidateUsageCaches(context.Background(), 7, true)
	require.Equal(t, []int64{7}, invalidator.userIDs)
}

func TestRedeemService_InvalidateRedeemCaches_AuthCache(t *testing.T) {
	invalidator := &authCacheInvalidatorStub{}
	svc := &RedeemService{authCacheInvalidator: invalidator}

	svc.invalidateRedeemCaches(context.Background(), 11, &RedeemCode{Type: RedeemTypeBalance})
	svc.invalidateRedeemCaches(context.Background(), 11, &RedeemCode{Type: RedeemTypeConcurrency})
	groupID := int64(3)
	svc.invalidateRedeemCaches(context.Background(), 11, &RedeemCode{Type: RedeemTypeSubscription, GroupID: &groupID})

	require.Equal(t, []int64{11, 11, 11}, invalidator.userIDs)
}
