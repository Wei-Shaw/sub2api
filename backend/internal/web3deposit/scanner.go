package web3deposit

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"time"
)

var (
	ErrScannerBatchSizeInvalid = errors.New("web3 scanner block batch size must be positive")
	ErrScannerOverlapInvalid   = errors.New("web3 scanner overlap must be smaller than block batch size")
	ErrScannerHeadBehindCursor = errors.New("web3 scanner latest block is behind persisted cursor")
)

type ScannerCursorSource interface {
	GetByKey(ctx context.Context, scannerKey string) (ScannerCursor, error)
}

type ScannerTransferSource interface {
	LatestBlock(ctx context.Context) (uint64, error)
	Fetch(ctx context.Context, fromBlock, toBlock uint64) ([]TransferEvent, error)
}

type ScannerRecipientMatcher interface {
	Match(ctx context.Context, events []TransferEvent) ([]MatchedTransferEvent, error)
}

type ScannerOptions struct {
	ScannerKey     string
	BlockBatchSize uint64
	OverlapBlocks  uint64
	ChainConfig    ChainConfig
}

type ScannerResult struct {
	HeadBlock    uint64
	FromBlock    uint64
	ToBlock      uint64
	EventCount   int
	MatchedCount int
	DepositCount int
	Advanced     bool
}

type Scanner struct {
	cursorSource ScannerCursorSource
	transfer     ScannerTransferSource
	matcher      ScannerRecipientMatcher
	batchStore   ScannerBatchStore
	options      ScannerOptions
}

func NewScanner(
	cursorSource ScannerCursorSource,
	transfer ScannerTransferSource,
	matcher ScannerRecipientMatcher,
	batchStore ScannerBatchStore,
	options ScannerOptions,
) (*Scanner, error) {
	if options.BlockBatchSize == 0 {
		return nil, ErrScannerBatchSizeInvalid
	}
	if options.OverlapBlocks >= options.BlockBatchSize {
		return nil, ErrScannerOverlapInvalid
	}
	return &Scanner{
		cursorSource: cursorSource,
		transfer:     transfer,
		matcher:      matcher,
		batchStore:   batchStore,
		options:      options,
	}, nil
}

func (s *Scanner) ScanNext(ctx context.Context, leaseToken string, now time.Time) (ScannerResult, error) {
	cursor, err := s.cursorSource.GetByKey(ctx, s.options.ScannerKey)
	if err != nil {
		return ScannerResult{}, fmt.Errorf("get web3 scanner cursor: %w", err)
	}
	headBlock, err := s.transfer.LatestBlock(ctx)
	if err != nil {
		return ScannerResult{}, fmt.Errorf("get web3 scanner latest block: %w", err)
	}
	if headBlock < cursor.LastScannedBlock {
		return ScannerResult{}, ErrScannerHeadBehindCursor
	}

	fromBlock := scannerOverlapStart(cursor, s.options.OverlapBlocks)
	toBlock := scannerBatchEnd(fromBlock, headBlock, s.options.BlockBatchSize)
	events, err := s.transfer.Fetch(ctx, fromBlock, toBlock)
	if err != nil {
		return ScannerResult{}, fmt.Errorf("fetch web3 scanner transfer events: %w", err)
	}
	sortTransferEvents(events)

	matches, err := s.matcher.Match(ctx, events)
	if err != nil {
		return ScannerResult{}, fmt.Errorf("match web3 scanner transfer recipients: %w", err)
	}
	deposits, err := s.batchStore.CommitDetectedBatch(ctx, ScannerBatch{
		ScannerKey:     s.options.ScannerKey,
		LeaseToken:     leaseToken,
		ScannedThrough: toBlock,
		Now:            now,
		Config:         s.options.ChainConfig,
		Matches:        matches,
	})
	if err != nil {
		return ScannerResult{}, fmt.Errorf("commit web3 scanner batch: %w", err)
	}
	return ScannerResult{
		HeadBlock:    headBlock,
		FromBlock:    fromBlock,
		ToBlock:      toBlock,
		EventCount:   len(events),
		MatchedCount: len(matches),
		DepositCount: len(deposits),
		Advanced:     true,
	}, nil
}

func scannerOverlapStart(cursor ScannerCursor, overlapBlocks uint64) uint64 {
	if cursor.LastScannedBlock <= cursor.ScanStartBlock {
		return cursor.ScanStartBlock
	}
	distanceFromStart := cursor.LastScannedBlock - cursor.ScanStartBlock
	if overlapBlocks >= distanceFromStart {
		return cursor.ScanStartBlock
	}
	return cursor.LastScannedBlock - overlapBlocks
}

func scannerBatchEnd(fromBlock, headBlock, blockBatchSize uint64) uint64 {
	if headBlock-fromBlock >= blockBatchSize {
		return fromBlock + blockBatchSize - 1
	}
	return headBlock
}

func sortTransferEvents(events []TransferEvent) {
	sort.SliceStable(events, func(left, right int) bool {
		if events[left].BlockNumber != events[right].BlockNumber {
			return events[left].BlockNumber < events[right].BlockNumber
		}
		if events[left].TransactionIndex != events[right].TransactionIndex {
			return events[left].TransactionIndex < events[right].TransactionIndex
		}
		return events[left].ID.LogIndex < events[right].ID.LogIndex
	})
}
