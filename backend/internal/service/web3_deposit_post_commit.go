package service

import "context"

type Web3TransferCacheInvalidator struct {
	billing *BillingCacheService
	auth    APIKeyAuthCacheInvalidator
}

func NewWeb3TransferCacheInvalidator(billing *BillingCacheService, auth APIKeyAuthCacheInvalidator) *Web3TransferCacheInvalidator {
	return &Web3TransferCacheInvalidator{billing: billing, auth: auth}
}

func (i *Web3TransferCacheInvalidator) InvalidateTransferCaches(ctx context.Context, userID int64) error {
	if i.auth != nil {
		i.auth.InvalidateAuthCacheByUserID(ctx, userID)
	}
	if i.billing != nil {
		return i.billing.InvalidateUserBalance(ctx, userID)
	}
	return nil
}
