package web3deposit

import (
	"context"
	"errors"
	"fmt"
	"time"
)

var (
	ErrFinalizerBatchSizeInvalid = errors.New("web3 finalizer block batch size must be positive")
	ErrFinalizedHeadBehindCursor = errors.New("web3 finalized block is behind persisted finalizer cursor")
)

type PendingFinalizationSource interface {
	ListPendingFinalization(ctx context.Context, fromBlock, toBlock uint64) ([]Deposit, error)
}

type FinalizerOptions struct {
	ScannerKey     string
	BlockBatchSize uint64
	ChainConfig    ChainConfig
}

type FinalizerResult struct {
	FinalizedHead   uint64
	FromBlock       uint64
	ToBlock         uint64
	CandidateCount  int
	FinalizedCount  int
	OrphanedCount   int
	OverflowedCount int
	Advanced        bool
}

type Finalizer struct {
	cursorSource ScannerCursorSource
	canonical    CanonicalDepositSource
	verifier     *CanonicalDepositVerifier
	pending      PendingFinalizationSource
	eligibility  DepositCreditEligibilitySource
	batchStore   FinalizerBatchStore
	options      FinalizerOptions
}

func NewFinalizer(
	cursorSource ScannerCursorSource,
	canonical CanonicalDepositSource,
	verifier *CanonicalDepositVerifier,
	pending PendingFinalizationSource,
	eligibility DepositCreditEligibilitySource,
	batchStore FinalizerBatchStore,
	options FinalizerOptions,
) (*Finalizer, error) {
	if options.BlockBatchSize == 0 {
		return nil, ErrFinalizerBatchSizeInvalid
	}
	if cursorSource == nil || canonical == nil || verifier == nil || pending == nil || eligibility == nil || batchStore == nil || options.ScannerKey == "" {
		return nil, fmt.Errorf("web3 finalizer dependencies are invalid")
	}
	return &Finalizer{
		cursorSource: cursorSource,
		canonical:    canonical,
		verifier:     verifier,
		pending:      pending,
		eligibility:  eligibility,
		batchStore:   batchStore,
		options:      options,
	}, nil
}

func (f *Finalizer) FinalizeNext(ctx context.Context, leaseToken string, now time.Time) (FinalizerResult, error) {
	cursor, err := f.cursorSource.GetByKey(ctx, f.options.ScannerKey)
	if err != nil {
		return FinalizerResult{}, fmt.Errorf("get web3 finalizer cursor: %w", err)
	}
	finalizedHead, err := f.canonical.FinalizedBlockNumber(ctx)
	if err != nil {
		return FinalizerResult{}, fmt.Errorf("get web3 finalized head: %w", err)
	}
	if finalizedHead < cursor.LastFinalizedBlock {
		return FinalizerResult{}, ErrFinalizedHeadBehindCursor
	}
	targetBlock := min(finalizedHead, cursor.LastScannedBlock)
	fromBlock := cursor.LastFinalizedBlock
	toBlock := finalizerBatchEnd(fromBlock, targetBlock, f.options.BlockBatchSize)
	candidates, err := f.pending.ListPendingFinalization(ctx, fromBlock, toBlock)
	if err != nil {
		return FinalizerResult{}, fmt.Errorf("list pending web3 deposits for finalization: %w", err)
	}

	decisions := make([]FinalizedDepositDecision, 0, len(candidates))
	orphanedCount := 0
	overflowedCount := 0
	for _, deposit := range candidates {
		decision, err := f.finalizeDeposit(ctx, deposit)
		if err != nil {
			return FinalizerResult{}, fmt.Errorf("finalize web3 deposit %d: %w", deposit.ID, err)
		}
		if decision.Status == DepositStatusOrphaned {
			orphanedCount++
		}
		if decision.FailureReason == FailureReasonAmountExceedsPlatformBalance {
			overflowedCount++
		}
		decisions = append(decisions, decision)
	}
	updated, err := f.batchStore.CommitFinalizedBatch(ctx, FinalizerBatch{
		ScannerKey:       f.options.ScannerKey,
		LeaseToken:       leaseToken,
		FinalizedThrough: toBlock,
		Now:              now,
		Decisions:        decisions,
	})
	if err != nil {
		return FinalizerResult{}, fmt.Errorf("commit web3 finalizer batch: %w", err)
	}
	return FinalizerResult{
		FinalizedHead:   finalizedHead,
		FromBlock:       fromBlock,
		ToBlock:         toBlock,
		CandidateCount:  len(candidates),
		FinalizedCount:  updated,
		OrphanedCount:   orphanedCount,
		OverflowedCount: overflowedCount,
		Advanced:        true,
	}, nil
}

func (f *Finalizer) finalizeDeposit(ctx context.Context, deposit Deposit) (FinalizedDepositDecision, error) {
	verification, err := f.verifier.Verify(ctx, deposit)
	if err != nil {
		return FinalizedDepositDecision{}, err
	}
	if !verification.Valid {
		return FinalizedDepositDecision{
			DepositID:     deposit.ID,
			Status:        DepositStatusOrphaned,
			FailureReason: string(verification.Reason),
		}, nil
	}
	classification, err := ClassifyFinalizedDepositAmount(deposit.TokenAmount, f.options.ChainConfig)
	if err != nil {
		return FinalizedDepositDecision{}, err
	}
	if classification.Status == DepositStatusReadyToCredit {
		eligibility, err := f.eligibility.CheckCreditEligibility(ctx, deposit)
		if err != nil {
			return FinalizedDepositDecision{}, err
		}
		if !eligibility.Eligible {
			classification.Status = DepositStatusManualReview
			classification.ReviewReason = eligibility.ReviewReason
		}
	}
	return FinalizedDepositDecision{
		DepositID:     deposit.ID,
		Status:        classification.Status,
		ReviewReason:  classification.ReviewReason,
		FailureReason: classification.FailureReason,
	}, nil
}

func finalizerBatchEnd(fromBlock, targetBlock, blockBatchSize uint64) uint64 {
	if targetBlock <= fromBlock {
		return fromBlock
	}
	if targetBlock-fromBlock >= blockBatchSize {
		return fromBlock + blockBatchSize - 1
	}
	return targetBlock
}
