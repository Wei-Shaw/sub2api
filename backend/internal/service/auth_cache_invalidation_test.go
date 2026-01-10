//go:build unit

package service

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestUsageService_InvalidateUsageCaches(t *testing.T) {
	invalidator := &authCacheInvalidatorStub{REDACTED
	svc := &UsageService{authCacheInvalidator: invalidatorREDACTED

	svc.invalidateUsageCaches(context.Background(), 7, false)
	require.Empty(t, invalidator.userIDs)

	svc.invalidateUsageCaches(context.Background(), 7, true)
	require.Equal(t, []int64{7REDACTED, invalidator.userIDs)
REDACTED

func TestRedeemService_InvalidateRedeemCaches_AuthCache(t *testing.T) {
	invalidator := &authCacheInvalidatorStub{REDACTED
	svc := &RedeemService{authCacheInvalidator: invalidatorREDACTED

	svc.invalidateRedeemCaches(context.Background(), 11, &RedeemCode{Type: RedeemTypeBalanceREDACTED)
	svc.invalidateRedeemCaches(context.Background(), 11, &RedeemCode{Type: RedeemTypeConcurrencyREDACTED)
	groupID := int64(3)
	svc.invalidateRedeemCaches(context.Background(), 11, &RedeemCode{Type: RedeemTypeSubscription, GroupID: &groupIDREDACTED)

	require.Equal(t, []int64{11, 11, 11REDACTED, invalidator.userIDs)
REDACTED
