package web3deposit

import (
	"context"
	"math/big"
	"testing"

	"github.com/ethereum/go-ethereum/common"
	"github.com/stretchr/testify/require"
)

type boundedRescanTransferStub struct {
	events []TransferEvent
	calls  int
}

func (s *boundedRescanTransferStub) LatestBlock(context.Context) (uint64, error) { return 0, nil }
func (s *boundedRescanTransferStub) Fetch(context.Context, uint64, uint64) ([]TransferEvent, error) {
	s.calls++
	return s.events, nil
}

type boundedRescanMatcherStub struct {
	matches []MatchedTransferEvent
	calls   int
}

func (s *boundedRescanMatcherStub) Match(context.Context, []TransferEvent) ([]MatchedTransferEvent, error) {
	s.calls++
	return s.matches, nil
}

type boundedRescanStoreStub struct{ calls int }

func (s *boundedRescanStoreStub) UpsertDetected(_ context.Context, deposit Deposit) (Deposit, error) {
	s.calls++
	deposit.ID = int64(s.calls)
	return deposit, nil
}

func TestBoundedRescannerPersistsWithoutCursorDependency(t *testing.T) {
	event, err := NewTransferEvent(DepositEventID{ChainID: 1030, TxHash: common.HexToHash("0x1"), LogIndex: 2}, 101, common.HexToHash("0x2"), common.HexToAddress("0x1111111111111111111111111111111111111111"), common.HexToAddress("0x2222222222222222222222222222222222222222"), big.NewInt(1_000_000))
	require.NoError(t, err)
	transfer := &boundedRescanTransferStub{events: []TransferEvent{event}}
	matcher := &boundedRescanMatcherStub{matches: []MatchedTransferEvent{{Event: event, UserID: 42, DepositAddressID: 9}}}
	store := &boundedRescanStoreStub{}
	rescanner := &BoundedRescanner{transfer: transfer, matcher: matcher, persister: NewDepositEventPersister(store, ChainConfig{ChainID: 1030, TokenAddress: common.HexToAddress("0xaf37e8b6c9ed7f6318979f56fc287d76c30847ff"), TokenDecimals: 6}), maxRange: 100}

	result, err := rescanner.Rescan(context.Background(), 100, 110)
	require.NoError(t, err)
	require.Equal(t, 1, result.EventCount)
	require.Equal(t, 1, result.MatchedCount)
	require.Equal(t, 1, result.DepositCount)
	require.Equal(t, 1, transfer.calls)
	require.Equal(t, 1, matcher.calls)
	require.Equal(t, 1, store.calls)
}

func TestBoundedRescannerRejectsInvalidRangeBeforeRPC(t *testing.T) {
	transfer := &boundedRescanTransferStub{}
	rescanner := &BoundedRescanner{transfer: transfer, maxRange: 10}
	_, err := rescanner.Rescan(context.Background(), 100, 110)
	require.Error(t, err)
	require.Zero(t, transfer.calls)
}
