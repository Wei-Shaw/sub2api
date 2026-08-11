package web3deposit

import (
	"context"
	"errors"
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

type boundedRescanFinalityStub struct {
	finalized uint64
	err       error
	calls     int
}

func (s *boundedRescanFinalityStub) FinalizedBlockNumber(context.Context) (uint64, error) {
	s.calls++
	if s.err != nil {
		return 0, s.err
	}
	return s.finalized, nil
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
	finality := &boundedRescanFinalityStub{finalized: 120}
	matcher := &boundedRescanMatcherStub{matches: []MatchedTransferEvent{{Event: event, UserID: 42, DepositAddressID: 9}}}
	store := &boundedRescanStoreStub{}
	rescanner := &BoundedRescanner{transfer: transfer, finality: finality, matcher: matcher, persister: NewDepositEventPersister(store, ChainConfig{ChainID: 1030, TokenAddress: common.HexToAddress("0xaf37e8b6c9ed7f6318979f56fc287d76c30847ff"), TokenDecimals: 6}), maxRange: 100, scanStartBlock: 90}

	result, err := rescanner.Rescan(context.Background(), 100, 110)
	require.NoError(t, err)
	require.Equal(t, 1, result.EventCount)
	require.Equal(t, 1, result.MatchedCount)
	require.Equal(t, 1, result.DepositCount)
	require.Equal(t, 1, transfer.calls)
	require.Equal(t, 1, finality.calls)
	require.Equal(t, 1, matcher.calls)
	require.Equal(t, 1, store.calls)
}

func TestBoundedRescannerRejectsInvalidRangeBeforeRPC(t *testing.T) {
	transfer := &boundedRescanTransferStub{}
	finality := &boundedRescanFinalityStub{finalized: 200}
	rescanner := &BoundedRescanner{transfer: transfer, finality: finality, maxRange: 10}
	_, err := rescanner.Rescan(context.Background(), 100, 110)
	require.Error(t, err)
	require.ErrorIs(t, err, ErrRescanRangeTooLarge)
	require.Zero(t, transfer.calls)
	require.Zero(t, finality.calls)
}

func TestBoundedRescannerRejectsRangeBeforeScanStartBlockBeforeRPC(t *testing.T) {
	transfer := &boundedRescanTransferStub{}
	finality := &boundedRescanFinalityStub{finalized: 200}
	rescanner := &BoundedRescanner{transfer: transfer, finality: finality, maxRange: 10, scanStartBlock: 100}
	_, err := rescanner.Rescan(context.Background(), 99, 100)
	require.ErrorIs(t, err, ErrRescanBeforeScanStartBlock)
	require.Zero(t, transfer.calls)
	require.Zero(t, finality.calls)
}

func TestBoundedRescannerRejectsBeyondFinalizedBeforeFetchingLogs(t *testing.T) {
	transfer := &boundedRescanTransferStub{}
	finality := &boundedRescanFinalityStub{finalized: 150}
	rescanner := &BoundedRescanner{transfer: transfer, finality: finality, maxRange: 10, scanStartBlock: 100}
	_, err := rescanner.Rescan(context.Background(), 149, 151)
	require.ErrorIs(t, err, ErrRescanBeyondFinalizedBlock)
	require.Zero(t, transfer.calls)
	require.Equal(t, 1, finality.calls)
}

func TestBoundedRescannerReturnsFinalityReadErrorBeforeFetchingLogs(t *testing.T) {
	transfer := &boundedRescanTransferStub{}
	finality := &boundedRescanFinalityStub{err: errors.New("rpc failed")}
	rescanner := &BoundedRescanner{transfer: transfer, finality: finality, maxRange: 10, scanStartBlock: 100}
	_, err := rescanner.Rescan(context.Background(), 100, 101)
	require.ErrorContains(t, err, "rpc failed")
	require.Zero(t, transfer.calls)
	require.Equal(t, 1, finality.calls)
}

func TestBoundedRescannerRegistryRoutesByNetworkAndAsset(t *testing.T) {
	first := &boundedRescanTransferStub{}
	second := &boundedRescanTransferStub{}
	registry := &BoundedRescannerRegistry{rescanners: map[RuntimeKey]*BoundedRescanner{
		{NetworkKey: "network_a", AssetKey: "asset_a"}: {transfer: first, finality: &boundedRescanFinalityStub{finalized: 10}, matcher: &boundedRescanMatcherStub{}, persister: NewDepositEventPersister(&boundedRescanStoreStub{}, ChainConfig{ChainID: 1, TokenDecimals: 6}), maxRange: 10},
		{NetworkKey: "network_b", AssetKey: "asset_b"}: {transfer: second, finality: &boundedRescanFinalityStub{finalized: 10}, matcher: &boundedRescanMatcherStub{}, persister: NewDepositEventPersister(&boundedRescanStoreStub{}, ChainConfig{ChainID: 2, TokenDecimals: 6}), maxRange: 10},
	}}

	_, err := registry.Rescan(context.Background(), "network_b", "asset_b", 1, 2)
	require.NoError(t, err)
	require.Zero(t, first.calls)
	require.Equal(t, 1, second.calls)
}
