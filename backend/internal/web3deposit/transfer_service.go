package web3deposit

import (
	"context"
	"errors"
)

type TransferCacheInvalidator interface {
	InvalidateTransferCaches(ctx context.Context, userID int64) error
}

type TransferNotifier interface {
	NotifyTransferCompleted(ctx context.Context, transfer BalanceTransfer) error
}

type TransferService struct {
	store    AccountingStore
	cache    TransferCacheInvalidator
	notifier TransferNotifier
}

func NewTransferService(store AccountingStore, cache TransferCacheInvalidator, notifier TransferNotifier) *TransferService {
	return &TransferService{store: store, cache: cache, notifier: notifier}
}

func (s *TransferService) TransferToMainBalance(ctx context.Context, request TransferToMainBalanceRequest) (TransferToMainBalanceResult, error) {
	result, err := s.store.TransferToMainBalance(ctx, request)
	if err != nil {
		return TransferToMainBalanceResult{}, err
	}
	var sideEffectErrors []error
	if s.cache != nil {
		if err := s.cache.InvalidateTransferCaches(ctx, result.Transfer.UserID); err != nil {
			sideEffectErrors = append(sideEffectErrors, err)
		}
	}
	if s.notifier != nil {
		if err := s.notifier.NotifyTransferCompleted(ctx, result.Transfer); err != nil {
			sideEffectErrors = append(sideEffectErrors, err)
		}
	}
	return result, errors.Join(sideEffectErrors...)
}
