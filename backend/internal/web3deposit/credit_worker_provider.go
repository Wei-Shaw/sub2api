package web3deposit

import (
	"context"

	"github.com/Wei-Shaw/sub2api/internal/config"
)

func ProvideCreditWorkerRuntime(cfg *config.Config, jobs CreditJobStore, accounting AccountingStore) *CreditWorkerRuntime {
	enabled := cfg != nil && cfg.Web3Deposit.Enabled && cfg.Web3Deposit.ScannerEnabled && cfg.Web3Deposit.CreditEnabled
	runtime := NewCreditWorkerRuntime(jobs, accounting, enabled)
	runtime.Start(context.Background())
	return runtime
}
