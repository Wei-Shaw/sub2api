//go:build unit

package web3deposit

// SetRuntimeMetricsForTest replaces the process-wide Web3 runtime metrics for
// tests and returns a restore function.
func SetRuntimeMetricsForTest(snapshot RuntimeMetricsSnapshot) func() {
	previous := SnapshotRuntimeMetrics()
	web3RuntimeMetrics.rpcHealthy.Store(snapshot.RPCHealthy)
	web3RuntimeMetrics.scannerLag.Store(snapshot.ScannerLagBlocks)
	web3RuntimeMetrics.finalizerLag.Store(snapshot.FinalizerLagBlocks)
	web3RuntimeMetrics.scannerFailures.Store(snapshot.ScannerFailures)
	web3RuntimeMetrics.finalizerFailures.Store(snapshot.FinalizerFailures)
	web3RuntimeMetrics.orphaned.Store(snapshot.OrphanedDeposits)
	web3RuntimeMetrics.creditRetries.Store(snapshot.CreditRetries)
	web3RuntimeMetrics.creditFailures.Store(snapshot.CreditFailures)
	web3RuntimeMetrics.amountOverflows.Store(snapshot.AmountOverflows)
	return func() {
		web3RuntimeMetrics.rpcHealthy.Store(previous.RPCHealthy)
		web3RuntimeMetrics.scannerLag.Store(previous.ScannerLagBlocks)
		web3RuntimeMetrics.finalizerLag.Store(previous.FinalizerLagBlocks)
		web3RuntimeMetrics.scannerFailures.Store(previous.ScannerFailures)
		web3RuntimeMetrics.finalizerFailures.Store(previous.FinalizerFailures)
		web3RuntimeMetrics.orphaned.Store(previous.OrphanedDeposits)
		web3RuntimeMetrics.creditRetries.Store(previous.CreditRetries)
		web3RuntimeMetrics.creditFailures.Store(previous.CreditFailures)
		web3RuntimeMetrics.amountOverflows.Store(previous.AmountOverflows)
	}
}
