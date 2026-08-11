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

type ScannerRuntimeRegistry struct {
	enabled  bool
	networks *ConfluxNetworkRuntimeRegistry
	runtimes map[RuntimeKey]*ScannerRuntime
}

func ProvideScannerRuntimeRegistry(
	cfg *config.Config,
	networks *ConfluxNetworkRuntimeRegistry,
	walletStore WalletMetadataStore,
	leaseStore ScannerLeaseStore,
	cursorSource ScannerCursorSource,
	addressLookup DepositAddressLookup,
	batchStore ScannerBatchStore,
	pendingSource PendingFinalizationSource,
	eligibilitySource DepositCreditEligibilitySource,
	finalizerBatchStore FinalizerBatchStore,
) *ScannerRuntimeRegistry {
	registry := &ScannerRuntimeRegistry{networks: networks, runtimes: make(map[RuntimeKey]*ScannerRuntime)}
	if cfg == nil || !cfg.Web3Deposit.Enabled || !cfg.Web3Deposit.ScannerEnabled || networks == nil {
		return registry
	}
	registry.enabled = true
	walletVerifier := NewWalletIdentityVerifier(walletStore)
	walletErrors := make(map[string]error)
	for _, key := range networks.Keys() {
		network := cfg.Web3Deposit.Networks[key.NetworkKey]
		asset := network.Assets[key.AssetKey]
		walletErr, verified := walletErrors[network.WalletID]
		if !verified {
			wallet, ok := cfg.Web3Deposit.Wallets[network.WalletID]
			if !ok || walletStore == nil {
				walletErr = fmt.Errorf("web3 deposit wallet %q is unavailable", network.WalletID)
			} else {
				_, walletErr = walletVerifier.Verify(context.Background(), ConfiguredWallet{
					WalletID:    network.WalletID,
					AccountPath: wallet.AccountPath,
					AccountXPub: wallet.AccountXPub,
				})
			}
			walletErrors[network.WalletID] = walletErr
		}
		if walletErr != nil {
			registry.runtimes[key] = failedScannerRuntime(walletErr)
			continue
		}
		networkRuntime, ok := networks.Runtime(key.NetworkKey, key.AssetKey)
		if !ok || !networkRuntime.Ready() || networkRuntime.Pool() == nil {
			registry.runtimes[key] = failedScannerRuntime(runtimeUnavailableError(networkRuntime))
			continue
		}
		registry.runtimes[key] = provideScannerRuntimeForAsset(key, network, asset, networkRuntime.Pool(), leaseStore, cursorSource, addressLookup, batchStore, pendingSource, eligibilitySource, finalizerBatchStore)
	}
	return registry
}

func ProvideScannerRuntime(
	cfg *config.Config,
	networkRuntime *ConfluxNetworkRuntime,
	leaseStore ScannerLeaseStore,
	cursorSource ScannerCursorSource,
	addressLookup DepositAddressLookup,
	batchStore ScannerBatchStore,
	pendingSource PendingFinalizationSource,
	eligibilitySource DepositCreditEligibilitySource,
	finalizerBatchStore FinalizerBatchStore,
) *ScannerRuntime {
	if cfg == nil || !cfg.Web3Deposit.Enabled || !cfg.Web3Deposit.ScannerEnabled {
		runtime, _ := NewScannerRuntime(nil, nil, ScannerRuntimeOptions{Enabled: false})
		return runtime
	}
	if networkRuntime == nil || !networkRuntime.Ready() || networkRuntime.Pool() == nil {
		return failedScannerRuntime(runtimeUnavailableError(networkRuntime))
	}
	network, ok := cfg.Web3Deposit.Networks[config.DefaultWeb3DepositNetworkKey]
	if !ok || !network.Enabled {
		return failedScannerRuntime(fmt.Errorf("web3 scanner network configuration is missing"))
	}
	asset, ok := network.Assets[config.DefaultWeb3DepositAssetKey]
	if !ok {
		return failedScannerRuntime(fmt.Errorf("web3 scanner asset configuration is invalid"))
	}
	return provideScannerRuntimeForAsset(RuntimeKey{NetworkKey: config.DefaultWeb3DepositNetworkKey, AssetKey: config.DefaultWeb3DepositAssetKey}, network, asset, networkRuntime.Pool(), leaseStore, cursorSource, addressLookup, batchStore, pendingSource, eligibilitySource, finalizerBatchStore)
}

func provideScannerRuntimeForAsset(
	key RuntimeKey,
	network config.Web3DepositNetworkConfig,
	asset config.Web3DepositAssetConfig,
	pool *ConfluxRPCPool,
	leaseStore ScannerLeaseStore,
	cursorSource ScannerCursorSource,
	addressLookup DepositAddressLookup,
	batchStore ScannerBatchStore,
	pendingSource PendingFinalizationSource,
	eligibilitySource DepositCreditEligibilitySource,
	finalizerBatchStore FinalizerBatchStore,
) *ScannerRuntime {
	if pool == nil || !common.IsHexAddress(asset.ContractAddress) {
		return failedScannerRuntime(fmt.Errorf("web3 scanner asset runtime is invalid"))
	}
	chainConfig, err := scannerChainConfig(network, asset)
	if err != nil {
		return failedScannerRuntime(err)
	}
	scannerID := scannerKey(key.NetworkKey, key.AssetKey)
	transfer := NewTransferLogFetcher(pool, chainConfig.ChainID, chainConfig.TokenAddress, TransferLogFetcherOptions{MaxBlockRange: network.BlockBatchSize})
	scanner, err := NewScanner(cursorSource, transfer, NewRecipientMatcher(addressLookup, DefaultRecipientLookupChunkSize), batchStore, ScannerOptions{ScannerKey: scannerID, BlockBatchSize: network.BlockBatchSize, OverlapBlocks: network.OverlapBlocks, ChainConfig: chainConfig})
	if err != nil {
		return failedScannerRuntime(err)
	}
	canonicalSource, err := NewRPCCanonicalDepositSource(pool)
	if err != nil {
		return failedScannerRuntime(err)
	}
	canonicalVerifier, err := NewCanonicalDepositVerifier(canonicalSource)
	if err != nil {
		return failedScannerRuntime(err)
	}
	finalizer, err := NewFinalizer(cursorSource, canonicalSource, canonicalVerifier, pendingSource, eligibilitySource, finalizerBatchStore, FinalizerOptions{ScannerKey: scannerID, BlockBatchSize: network.BlockBatchSize, ChainConfig: chainConfig})
	if err != nil {
		return failedScannerRuntime(err)
	}
	runner, err := NewScannerFinalizerRunner(scanner, finalizer)
	if err != nil {
		return failedScannerRuntime(err)
	}
	pollInterval := time.Duration(network.PollIntervalSeconds) * time.Second
	leaseTTL, renewInterval := scannerLeaseTiming(pollInterval)
	runtime, err := NewScannerRuntime(runner, leaseStore, ScannerRuntimeOptions{Enabled: true, ScannerKey: scannerID, Owner: "scanner-" + uuid.NewString(), ChainID: chainConfig.ChainID, TokenContract: strings.ToLower(chainConfig.TokenAddress.Hex()), ScanStartBlock: chainConfig.ScanStartBlock, PollInterval: pollInterval, LeaseTTL: leaseTTL, RenewInterval: renewInterval, AcquireInterval: min(pollInterval, DefaultScannerLeaseAcquireInterval)})
	if err != nil {
		return failedScannerRuntime(err)
	}
	if err := runtime.Start(context.Background()); err != nil {
		slog.Warn("web3 deposit scanner runtime failed to start", "network", key.NetworkKey, "asset", key.AssetKey, "error", err)
	}
	return runtime
}

func runtimeUnavailableError(runtime *ConfluxNetworkRuntime) error {
	if runtime != nil && runtime.VerificationError() != nil {
		return runtime.VerificationError()
	}
	return fmt.Errorf("web3 scanner network runtime is unavailable")
}

func (r *ScannerRuntimeRegistry) AssetReady(networkKey, assetKey string) bool {
	if r == nil || !r.enabled || r.networks == nil || !r.networks.AssetReady(networkKey, assetKey) {
		return false
	}
	runtime, ok := r.runtimes[RuntimeKey{NetworkKey: networkKey, AssetKey: assetKey}]
	if !ok || runtime == nil {
		return false
	}
	state := runtime.Status().State
	return state == ScannerRuntimeStateStandby || state == ScannerRuntimeStateLeader
}

func (r *ScannerRuntimeRegistry) Runtime(networkKey, assetKey string) (*ScannerRuntime, bool) {
	if r == nil {
		return nil, false
	}
	runtime, ok := r.runtimes[RuntimeKey{NetworkKey: networkKey, AssetKey: assetKey}]
	return runtime, ok
}

func (r *ScannerRuntimeRegistry) Keys() []RuntimeKey {
	if r == nil || r.networks == nil {
		return nil
	}
	return r.networks.Keys()
}

func (r *ScannerRuntimeRegistry) Stop() {
	if r == nil {
		return
	}
	for _, runtime := range r.runtimes {
		runtime.Stop()
	}
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
	return ChainConfig{ChainID: network.ChainID, TokenAddress: common.HexToAddress(asset.ContractAddress), TokenDecimals: asset.Decimals, WalletID: network.WalletID, ScanStartBlock: network.ScanStartBlock, MinimumDeposit: minimumDeposit, AutoCreditLimit: autoCreditLimit}, nil
}

func scannerKey(networkKey, assetKey string) string { return networkKey + ":" + assetKey }

func scannerLeaseTiming(pollInterval time.Duration) (time.Duration, time.Duration) {
	leaseTTL := max(DefaultScannerLeaseTTL, pollInterval*3)
	return leaseTTL, leaseTTL / 3
}

func failedScannerRuntime(err error) *ScannerRuntime {
	return &ScannerRuntime{status: ScannerRuntimeStatus{State: ScannerRuntimeStateUnhealthy, LastError: err.Error()}}
}
