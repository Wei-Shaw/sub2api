package web3deposit

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestTransferServiceRunsSideEffectsOnlyAfterCommittedTransfer(t *testing.T) {
	store := &accountingStoreStub{result: TransferToMainBalanceResult{Transfer: BalanceTransfer{ID: 7, UserID: 42}}}
	cache := &transferCacheStub{}
	notifier := &transferNotifierStub{}
	service := NewTransferService(store, cache, notifier)

	result, err := service.TransferToMainBalance(context.Background(), TransferToMainBalanceRequest{})

	require.NoError(t, err)
	require.Equal(t, int64(7), result.Transfer.ID)
	require.Equal(t, 1, cache.calls)
	require.Equal(t, 1, notifier.calls)
}

func TestTransferServiceSideEffectFailureDoesNotRepeatTransfer(t *testing.T) {
	wantErr := errors.New("notification unavailable")
	store := &accountingStoreStub{result: TransferToMainBalanceResult{Transfer: BalanceTransfer{ID: 7, UserID: 42}, AlreadyDone: true}}
	service := NewTransferService(store, nil, &transferNotifierStub{err: wantErr})

	result, err := service.TransferToMainBalance(context.Background(), TransferToMainBalanceRequest{})

	require.ErrorIs(t, err, wantErr)
	require.True(t, result.AlreadyDone)
	require.Equal(t, 1, store.transferCalls)
}

func TestTransferServiceSkipsSideEffectsWhenTransactionFails(t *testing.T) {
	store := &accountingStoreStub{err: errors.New("commit failed")}
	cache := &transferCacheStub{}
	notifier := &transferNotifierStub{}
	service := NewTransferService(store, cache, notifier)

	_, err := service.TransferToMainBalance(context.Background(), TransferToMainBalanceRequest{})

	require.Error(t, err)
	require.Zero(t, cache.calls)
	require.Zero(t, notifier.calls)
}

type accountingStoreStub struct {
	result        TransferToMainBalanceResult
	err           error
	transferCalls int
}

func (s *accountingStoreStub) CreditDeposit(context.Context, CreditDepositRequest) (CreditDepositResult, error) {
	return CreditDepositResult{}, nil
}
func (s *accountingStoreStub) TransferToMainBalance(context.Context, TransferToMainBalanceRequest) (TransferToMainBalanceResult, error) {
	s.transferCalls++
	return s.result, s.err
}

type transferCacheStub struct {
	calls int
	err   error
}

func (s *transferCacheStub) InvalidateTransferCaches(context.Context, int64) error {
	s.calls++
	return s.err
}

type transferNotifierStub struct {
	calls int
	err   error
}

func (s *transferNotifierStub) NotifyTransferCompleted(context.Context, BalanceTransfer) error {
	s.calls++
	return s.err
}
