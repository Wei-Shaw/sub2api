package web3deposit

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/ethereum/go-ethereum/common"
	"github.com/google/uuid"
	"github.com/shopspring/decimal"
)

func ProvideScannerRuntime(
	cfg *config.Config,
	networkRuntime *ConfluxNetworkRuntime,
	leaseStore ScannerLeaseStore,
	cursorSource ScannerCursorSource,
	addressLookup ActiveDepositAddressLookup,
	batchStore ScannerBatchStore,
) *ScannerRuntime {
	if cfg == nil || !cfg.Web3Deposit.Enabled || !cfg.Web3Deposit.ScannerEnabled {
		runtime, _ := NewScannerRuntime(nil, nil, ScannerRuntimeOptions{Enabled: false})
		return runtime
	}
	if cfg.Web3Deposit.CreditEnabled {
		return failedScannerRuntime(ErrScannerCreditMustBeDisabled)
	}
	if networkRuntime == nil || !networkRuntime.Ready() || networkRuntime.Pool() == nil {
		err := fmt.Errorf("web3 scanner network runtime is unavailable")
		if networkRuntime != nil && networkRuntime.VerificationError() != nil {
			err = networkRuntime.VerificationError()
		}
		return failedScannerRuntime(err)
	}

	network, ok := cfg.Web3Deposit.Networks[config.DefaultWeb3DepositNetworkKey]
	if !ok || !network.Enabled {
		return failedScannerRuntime(fmt.Errorf("web3 scanner network configuration is missing"))
	}
	asset, ok := network.Assets[config.DefaultWeb3DepositAssetKey]
	if !ok || !common.IsHexAddress(asset.ContractAddress) {
		return failedScannerRuntime(fmt.Errorf("web3 scanner asset configuration is invalid"))
	}
	chainConfig, err := scannerChainConfig(network, asset)
	if err != nil {
		return failedScannerRuntime(err)
	}

	transfer := NewTransferLogFetcher(
		networkRuntime.Pool(),
		chainConfig.ChainID,
		chainConfig.TokenAddress,
		TransferLogFetcherOptions{MaxBlockRange: network.BlockBatchSize},
	)
	scanner, err := NewScanner(
		cursorSource,
		transfer,
		NewRecipientMatcher(addressLookup, DefaultRecipientLookupChunkSize),
		batchStore,
		ScannerOptions{
			ScannerKey:     scannerKey(config.DefaultWeb3DepositNetworkKey, config.DefaultWeb3DepositAssetKey),
			BlockBatchSize: network.BlockBatchSize,
			OverlapBlocks:  network.OverlapBlocks,
			ChainConfig:    chainConfig,
		},
	)
	if err != nil {
		return failedScannerRuntime(err)
	}

	pollInterval := time.Duration(network.PollIntervalSeconds) * time.Second
	leaseTTL, renewInterval := scannerLeaseTiming(pollInterval)
	runtime, err := NewScannerRuntime(scanner, leaseStore, ScannerRuntimeOptions{
		Enabled:         true,
		ScannerKey:      scannerKey(config.DefaultWeb3DepositNetworkKey, config.DefaultWeb3DepositAssetKey),
		Owner:           "scanner-" + uuid.NewString(),
		ChainID:         chainConfig.ChainID,
		TokenContract:   strings.ToLower(chainConfig.TokenAddress.Hex()),
		ScanStartBlock:  chainConfig.ScanStartBlock,
		PollInterval:    pollInterval,
		LeaseTTL:        leaseTTL,
		RenewInterval:   renewInterval,
		AcquireInterval: min(pollInterval, DefaultScannerLeaseAcquireInterval),
	})
	if err != nil {
		return failedScannerRuntime(err)
	}
	if err := runtime.Start(context.Background()); err != nil {
		slog.Warn("web3 deposit scanner runtime failed to start", "error", err)
	}
	return runtime
}

func scannerChainConfig(network config.Web3DepositNetworkConfig, asset config.Web3DepositAssetConfig) (ChainConfig, error) {
	minimumDeposit, err := decimal.NewFromString(asset.MinimumDeposit)
	if err != nil {
		return ChainConfig{}, fmt.Errorf("parse web3 scanner minimum deposit: %w", err)
	}
	autoCreditLimit, err := decimal.NewFromString(asset.AutoCreditLimit)
	if err != nil {
		return ChainConfig{}, fmt.Errorf("parse web3 scanner auto credit limit: %w", err)
	}
	return ChainConfig{
		ChainID:         network.ChainID,
		TokenAddress:    common.HexToAddress(asset.ContractAddress),
		TokenDecimals:   asset.Decimals,
		WalletID:        network.WalletID,
		ScanStartBlock:  network.ScanStartBlock,
		MinimumDeposit:  minimumDeposit,
		AutoCreditLimit: autoCreditLimit,
	}, nil
}

func scannerKey(networkKey, assetKey string) string {
	return networkKey + ":" + assetKey
}

func scannerLeaseTiming(pollInterval time.Duration) (time.Duration, time.Duration) {
	leaseTTL := max(DefaultScannerLeaseTTL, pollInterval*3)
	return leaseTTL, leaseTTL / 3
}

func failedScannerRuntime(err error) *ScannerRuntime {
	return &ScannerRuntime{status: ScannerRuntimeStatus{
		State:     ScannerRuntimeStateUnhealthy,
		LastError: err.Error(),
	}}
}
