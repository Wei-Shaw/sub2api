package web3deposit

import (
	"context"
	"time"
)

type FinalizedDepositDecision struct {
	DepositID     int64
	Status        DepositStatus
	ReviewReason  string
	FailureReason string
}

type FinalizerBatch struct {
	ScannerKey       string
	LeaseToken       string
	FinalizedThrough uint64
	Now              time.Time
	Decisions        []FinalizedDepositDecision
}

type FinalizerBatchStore interface {
	CommitFinalizedBatch(ctx context.Context, batch FinalizerBatch) (int, error)
}
