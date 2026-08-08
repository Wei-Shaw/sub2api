package web3deposit

import (
	"context"
	"errors"
	"fmt"
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
	_, finalizerErr := r.finalizer.FinalizeNext(ctx, leaseToken, now)
	return scannerResult, errors.Join(scannerErr, finalizerErr)
}
