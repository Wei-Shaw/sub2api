package web3deposit

import (
	"context"
	"time"
)

type ScannerBatch struct {
	ScannerKey     string
	LeaseToken     string
	ScannedThrough uint64
	Now            time.Time
	Config         ChainConfig
	Matches        []MatchedTransferEvent
}

type ScannerBatchStore interface {
	CommitDetectedBatch(ctx context.Context, batch ScannerBatch) ([]Deposit, error)
}
