package web3deposit

import (
	"context"

	"github.com/Wei-Shaw/sub2api/internal/config"
)

func ProvideRescanJobRuntime(cfg *config.Config, store RescanJobStore, rescanner *BoundedRescannerRegistry) *RescanJobRuntime {
	enabled := cfg != nil && cfg.Web3Deposit.Enabled && cfg.Web3Deposit.ScannerEnabled
	runtime := NewRescanJobRuntime(store, rescanner, enabled)
	runtime.Start(context.Background())
	return runtime
}
