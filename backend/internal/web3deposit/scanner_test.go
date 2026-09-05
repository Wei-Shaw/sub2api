package web3deposit

import (
	"context"
	"errors"
	"math/big"
	"testing"
	"time"

	"github.com/ethereum/go-ethereum/common"
	"github.com/stretchr/testify/require"
)

func TestScannerScansOverlapSortsEventsAndCommitsBatch(t *testing.T) {
	now := time.Date(2026, time.August, 8, 11, 0, 0, 0, time.UTC)
	cursorSource := &scannerCursorSourceStub{cursor: ScannerCursor{
		ScannerKey:       "conflux_espace_mainnet:usdt0",
		ChainID:          1030,
		TokenContract:    "0xaf37e8b6c9ed7f6318979f56fc287d76c30847ff",
		ScanStartBlock:   100,
		LastScannedBlock: 120,
	}}
	events := []TransferEvent{
		newScannerTestEvent(t, 130, 2, 9),
		newScannerTestEvent(t, 129, 2, 8),
		newScannerTestEvent(t, 129, 1, 7),
	}
	source := &scannerTransferSourceStub{latestBlock: 200, events: events}
	matcher := &scannerRecipientMatcherStub{}
	batchStore := &scannerBatchStoreStub{}
	scanner, err := NewScanner(
		cursorSource,
		source,
		matcher,
		batchStore,
		testScannerOptions(50, 20),
	)
	require.NoError(t, err)

	result, err := scanner.ScanNext(context.Background(), "lease-token-01", now)
	require.NoError(t, err)
	require.Equal(t, ScannerResult{
		HeadBlock:    200,
		FromBlock:    100,
		ToBlock:      149,
		EventCount:   3,
		MatchedCount: 3,
		DepositCount: 3,
		Advanced:     true,
	}, result)
	require.Equal(t, []scannerBlockRange{{from: 100, to: 149}}, source.ranges)
	require.Equal(t, []uint64{7, 8, 9}, scannerEventLogIndexes(matcher.events))
	require.Len(t, batchStore.batches, 1)
	require.Equal(t, "lease-token-01", batchStore.batches[0].LeaseToken)
	require.Equal(t, uint64(149), batchStore.batches[0].ScannedThrough)
	require.Equal(t, now, batchStore.batches[0].Now)
	require.Equal(t, []uint64{7, 8, 9}, scannerMatchedLogIndexes(batchStore.batches[0].Matches))
}

func TestScannerReloadsCursorBetweenOverlapBatches(t *testing.T) {
	now := time.Date(2026, time.August, 8, 11, 0, 0, 0, time.UTC)
	cursorSource := &scannerCursorSourceStub{cursor: ScannerCursor{
		ScannerKey:       "conflux_espace_mainnet:usdt0",
		ChainID:          1030,
		TokenContract:    "0xaf37e8b6c9ed7f6318979f56fc287d76c30847ff",
		ScanStartBlock:   100,
		LastScannedBlock: 120,
	}}
	source := &scannerTransferSourceStub{latestBlock: 200}
	matcher := &scannerRecipientMatcherStub{}
	batchStore := &scannerBatchStoreStub{afterCommit: func(batch ScannerBatch) {
		cursorSource.cursor.LastScannedBlock = batch.ScannedThrough
	}}
	scanner, err := NewScanner(cursorSource, source, matcher, batchStore, testScannerOptions(50, 20))
	require.NoError(t, err)

	_, err = scanner.ScanNext(context.Background(), "lease-token-01", now)
	require.NoError(t, err)
	_, err = scanner.ScanNext(context.Background(), "lease-token-01", now.Add(time.Second))
	require.NoError(t, err)
	require.Equal(t, 2, cursorSource.calls)
	require.Equal(t, []scannerBlockRange{{from: 100, to: 149}, {from: 129, to: 178}}, source.ranges)
}

func TestScannerClampsOverlapToScanStartBlock(t *testing.T) {
	cursorSource := &scannerCursorSourceStub{cursor: ScannerCursor{
		ScannerKey:       "conflux_espace_mainnet:usdt0",
		ScanStartBlock:   100,
		LastScannedBlock: 105,
	}}
	source := &scannerTransferSourceStub{latestBlock: 130}
	scanner, err := NewScanner(
		cursorSource,
		source,
		&scannerRecipientMatcherStub{},
		&scannerBatchStoreStub{},
		testScannerOptions(20, 10),
	)
	require.NoError(t, err)

	_, err = scanner.ScanNext(context.Background(), "lease-token-01", time.Now())
	require.NoError(t, err)
	require.Equal(t, []scannerBlockRange{{from: 100, to: 119}}, source.ranges)
}

func TestScannerStopsBeforeCommitWhenPipelineFails(t *testing.T) {
	wantErr := errors.New("pipeline unavailable")
	tests := []struct {
		name       string
		sourceErr  error
		matcherErr error
		wantFetch  int
		wantMatch  int
	}{
		{name: "fetch", sourceErr: wantErr, wantFetch: 1},
		{name: "match", matcherErr: wantErr, wantFetch: 1, wantMatch: 1},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			cursorSource := &scannerCursorSourceStub{cursor: ScannerCursor{ScanStartBlock: 100, LastScannedBlock: 100}}
			source := &scannerTransferSourceStub{
				latestBlock: 110,
				events:      []TransferEvent{newScannerTestEvent(t, 100, 0, 1)},
				fetchErr:    test.sourceErr,
			}
			matcher := &scannerRecipientMatcherStub{err: test.matcherErr}
			batchStore := &scannerBatchStoreStub{}
			scanner, err := NewScanner(cursorSource, source, matcher, batchStore, testScannerOptions(10, 0))
			require.NoError(t, err)

			result, err := scanner.ScanNext(context.Background(), "lease-token-01", time.Now())
			require.ErrorIs(t, err, wantErr)
			require.Equal(t, ScannerResult{}, result)
			require.Equal(t, test.wantFetch, len(source.ranges))
			require.Equal(t, test.wantMatch, matcher.calls)
			require.Empty(t, batchStore.batches)
		})
	}
}

func TestScannerDoesNotAdvanceWhenBatchCommitFails(t *testing.T) {
	wantErr := errors.New("transaction failed")
	cursorSource := &scannerCursorSourceStub{cursor: ScannerCursor{ScanStartBlock: 100, LastScannedBlock: 100}}
	source := &scannerTransferSourceStub{latestBlock: 110}
	batchStore := &scannerBatchStoreStub{err: wantErr}
	scanner, err := NewScanner(
		cursorSource,
		source,
		&scannerRecipientMatcherStub{},
		batchStore,
		testScannerOptions(10, 0),
	)
	require.NoError(t, err)

	result, err := scanner.ScanNext(context.Background(), "lease-token-01", time.Now())
	require.ErrorIs(t, err, wantErr)
	require.Equal(t, ScannerResult{}, result)
	require.Len(t, batchStore.batches, 1)
	require.Equal(t, uint64(100), cursorSource.cursor.LastScannedBlock)
}

func TestScannerRejectsHeadBehindCursor(t *testing.T) {
	cursorSource := &scannerCursorSourceStub{cursor: ScannerCursor{ScanStartBlock: 100, LastScannedBlock: 120}}
	source := &scannerTransferSourceStub{latestBlock: 119}
	scanner, err := NewScanner(
		cursorSource,
		source,
		&scannerRecipientMatcherStub{},
		&scannerBatchStoreStub{},
		testScannerOptions(10, 0),
	)
	require.NoError(t, err)

	_, err = scanner.ScanNext(context.Background(), "lease-token-01", time.Now())
	require.ErrorIs(t, err, ErrScannerHeadBehindCursor)
	require.Empty(t, source.ranges)
}

func TestNewScannerValidatesRangeOptions(t *testing.T) {
	dependencies := func(options ScannerOptions) error {
		_, err := NewScanner(
			&scannerCursorSourceStub{},
			&scannerTransferSourceStub{},
			&scannerRecipientMatcherStub{},
			&scannerBatchStoreStub{},
			options,
		)
		return err
	}

	require.ErrorIs(t, dependencies(testScannerOptions(0, 0)), ErrScannerBatchSizeInvalid)
	require.ErrorIs(t, dependencies(testScannerOptions(10, 10)), ErrScannerOverlapInvalid)
	require.NoError(t, dependencies(testScannerOptions(10, 9)))
}

func testScannerOptions(blockBatchSize, overlapBlocks uint64) ScannerOptions {
	return ScannerOptions{
		ScannerKey:     "conflux_espace_mainnet:usdt0",
		BlockBatchSize: blockBatchSize,
		OverlapBlocks:  overlapBlocks,
		ChainConfig: ChainConfig{
			ChainID:       1030,
			TokenAddress:  common.HexToAddress("0xaf37e8b6c9ed7f6318979f56fc287d76c30847ff"),
			TokenDecimals: USDT0Decimals,
		},
	}
}

func newScannerTestEvent(t *testing.T, blockNumber, transactionIndex, logIndex uint64) TransferEvent {
	t.Helper()
	event, err := NewTransferEvent(
		DepositEventID{ChainID: 1030, TxHash: common.BigToHash(new(big.Int).SetUint64(logIndex)), LogIndex: logIndex},
		blockNumber,
		common.BigToHash(new(big.Int).SetUint64(blockNumber)),
		common.HexToAddress("0x1111111111111111111111111111111111111111"),
		common.HexToAddress("0x2222222222222222222222222222222222222222"),
		big.NewInt(1_000_000),
	)
	require.NoError(t, err)
	event.TransactionIndex = transactionIndex
	return event
}

func scannerEventLogIndexes(events []TransferEvent) []uint64 {
	indexes := make([]uint64, 0, len(events))
	for _, event := range events {
		indexes = append(indexes, event.ID.LogIndex)
	}
	return indexes
}

func scannerMatchedLogIndexes(matches []MatchedTransferEvent) []uint64 {
	indexes := make([]uint64, 0, len(matches))
	for _, match := range matches {
		indexes = append(indexes, match.Event.ID.LogIndex)
	}
	return indexes
}

type scannerCursorSourceStub struct {
	cursor ScannerCursor
	err    error
	calls  int
}

func (s *scannerCursorSourceStub) GetByKey(context.Context, string) (ScannerCursor, error) {
	s.calls++
	return s.cursor, s.err
}

type scannerBlockRange struct {
	from uint64
	to   uint64
}

type scannerTransferSourceStub struct {
	latestBlock uint64
	latestErr   error
	events      []TransferEvent
	fetchErr    error
	ranges      []scannerBlockRange
}

func (s *scannerTransferSourceStub) LatestBlock(context.Context) (uint64, error) {
	return s.latestBlock, s.latestErr
}

func (s *scannerTransferSourceStub) Fetch(_ context.Context, fromBlock, toBlock uint64) ([]TransferEvent, error) {
	s.ranges = append(s.ranges, scannerBlockRange{from: fromBlock, to: toBlock})
	return append([]TransferEvent(nil), s.events...), s.fetchErr
}

type scannerRecipientMatcherStub struct {
	events []TransferEvent
	err    error
	calls  int
}

func (s *scannerRecipientMatcherStub) Match(_ context.Context, events []TransferEvent) ([]MatchedTransferEvent, error) {
	s.calls++
	s.events = append([]TransferEvent(nil), events...)
	if s.err != nil {
		return nil, s.err
	}
	matches := make([]MatchedTransferEvent, 0, len(events))
	for _, event := range events {
		matches = append(matches, MatchedTransferEvent{Event: event, DepositAddressID: 9, UserID: 42})
	}
	return matches, nil
}

type scannerBatchStoreStub struct {
	batches     []ScannerBatch
	err         error
	afterCommit func(ScannerBatch)
}

func (s *scannerBatchStoreStub) CommitDetectedBatch(_ context.Context, batch ScannerBatch) ([]Deposit, error) {
	s.batches = append(s.batches, batch)
	if s.err != nil {
		return nil, s.err
	}
	if s.afterCommit != nil {
		s.afterCommit(batch)
	}
	deposits := make([]Deposit, len(batch.Matches))
	return deposits, nil
}
