package web3deposit

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"
)

type FinalizerRunner interface {
	FinalizeNext(ctx context.Context, leaseToken string, now time.Time) (FinalizerResult, error)
}

type ScannerFinalizerRunner struct {
	scanner   ScannerRunner
	finalizer FinalizerRunner
}

func NewScannerFinalizerRunner(scanner ScannerRunner, finalizer FinalizerRunner) (*ScannerFinalizerRunner, error) {
	if scanner == nil || finalizer == nil {
		return nil, fmt.Errorf("web3 scanner finalizer runner dependencies are invalid")
	}
	return &ScannerFinalizerRunner{scanner: scanner, finalizer: finalizer}, nil
}

func (r *ScannerFinalizerRunner) ScanNext(ctx context.Context, leaseToken string, now time.Time) (ScannerResult, error) {
	scannerResult, scannerErr := r.scanner.ScanNext(ctx, leaseToken, now)
	if errors.Is(scannerErr, ErrLeaseNotHeld) || ctx.Err() != nil {
		return scannerResult, scannerErr
	}
	finalizerResult, finalizerErr := r.finalizer.FinalizeNext(ctx, leaseToken, now)
	if scannerErr != nil {
		web3RuntimeMetrics.scannerFailures.Add(1)
		slog.Error("web3_deposit_scan_failed", "error", scannerErr)
	}
	if finalizerErr != nil {
		web3RuntimeMetrics.finalizerFailures.Add(1)
		slog.Error("web3_deposit_finalize_failed", "error", finalizerErr)
	} else {
		lag := uint64(0)
		if finalizerResult.FinalizedHead > finalizerResult.ToBlock {
			lag = finalizerResult.FinalizedHead - finalizerResult.ToBlock
		}
		web3RuntimeMetrics.finalizerLag.Store(lag)
		if finalizerResult.OrphanedCount > 0 {
			web3RuntimeMetrics.orphaned.Add(uint64(finalizerResult.OrphanedCount))
			slog.Warn("web3_deposit_orphaned", "count", finalizerResult.OrphanedCount, "from_block", finalizerResult.FromBlock, "to_block", finalizerResult.ToBlock)
		}
		if finalizerResult.OverflowedCount > 0 {
			web3RuntimeMetrics.amountOverflows.Add(uint64(finalizerResult.OverflowedCount))
			slog.Error("web3_deposit_amount_overflow", "count", finalizerResult.OverflowedCount, "from_block", finalizerResult.FromBlock, "to_block", finalizerResult.ToBlock)
		}
	}
	return scannerResult, errors.Join(scannerErr, finalizerErr)
}
