package web3deposit

import "sync/atomic"

type RuntimeMetricsSnapshot struct {
	RPCHealthy         bool   `json:"rpc_healthy"`
	ScannerLagBlocks   uint64 `json:"scanner_lag_blocks"`
	FinalizerLagBlocks uint64 `json:"finalizer_lag_blocks"`
	ScannerFailures    uint64 `json:"scanner_failures_total"`
	FinalizerFailures  uint64 `json:"finalizer_failures_total"`
	OrphanedDeposits   uint64 `json:"orphaned_deposits_total"`
	CreditRetries      uint64 `json:"credit_retries_total"`
	CreditFailures     uint64 `json:"credit_failures_total"`
}

var web3RuntimeMetrics struct {
	rpcHealthy        atomic.Bool
	scannerLag        atomic.Uint64
	finalizerLag      atomic.Uint64
	scannerFailures   atomic.Uint64
	finalizerFailures atomic.Uint64
	orphaned          atomic.Uint64
	creditRetries     atomic.Uint64
	creditFailures    atomic.Uint64
}

func SnapshotRuntimeMetrics() RuntimeMetricsSnapshot {
	return RuntimeMetricsSnapshot{RPCHealthy: web3RuntimeMetrics.rpcHealthy.Load(), ScannerLagBlocks: web3RuntimeMetrics.scannerLag.Load(), FinalizerLagBlocks: web3RuntimeMetrics.finalizerLag.Load(), ScannerFailures: web3RuntimeMetrics.scannerFailures.Load(), FinalizerFailures: web3RuntimeMetrics.finalizerFailures.Load(), OrphanedDeposits: web3RuntimeMetrics.orphaned.Load(), CreditRetries: web3RuntimeMetrics.creditRetries.Load(), CreditFailures: web3RuntimeMetrics.creditFailures.Load()}
}
