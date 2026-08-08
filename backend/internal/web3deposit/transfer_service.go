package web3deposit

import "context"

type TransferCacheInvalidator interface {
	InvalidateTransferCaches(ctx context.Context, userID int64) error
}

type TransferService struct {
	store AccountingStore
	cache TransferCacheInvalidator
}

func NewTransferService(store AccountingStore, cache TransferCacheInvalidator) *TransferService {
	return &TransferService{store: store, cache: cache}
}

func (s *TransferService) TransferToMainBalance(ctx context.Context, request TransferToMainBalanceRequest) (TransferToMainBalanceResult, error) {
	result, err := s.store.TransferToMainBalance(ctx, request)
	if err != nil {
		return TransferToMainBalanceResult{}, err
	}
	if s.cache != nil {
		if err := s.cache.InvalidateTransferCaches(ctx, result.Transfer.UserID); err != nil {
			return result, err
		}
	}
	return result, nil
}
