package web3deposit

import (
	"context"
	"errors"
	"fmt"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/ethereum/go-ethereum/common"
)

var (
	ErrRescanRangeInvalid         = errors.New("web3 deposit rescan range is invalid")
	ErrRescanRangeTooLarge        = errors.New("web3 deposit rescan range is too large")
	ErrRescanBeforeScanStartBlock = errors.New("web3 deposit rescan range is before scan start block")
	ErrRescanBeyondFinalizedBlock = errors.New("web3 deposit rescan range is beyond finalized block")
)

type RescanFinalitySource interface {
	FinalizedBlockNumber(ctx context.Context) (uint64, error)
}

type BoundedRescanResult struct {
	FromBlock    uint64 `json:"from_block"`
	ToBlock      uint64 `json:"to_block"`
	EventCount   int    `json:"event_count"`
	MatchedCount int    `json:"matched_count"`
	DepositCount int    `json:"deposit_count"`
}

type BoundedRescanner struct {
	transfer       ScannerTransferSource
	finality       RescanFinalitySource
	matcher        ScannerRecipientMatcher
	persister      *DepositEventPersister
	maxRange       uint64
	scanStartBlock uint64
	initErr        error
}

type BoundedRescannerRegistry struct {
	rescanners map[RuntimeKey]*BoundedRescanner
}

func NewBoundedRescannerRegistry(cfg *config.Config, runtimes *ConfluxNetworkRuntimeRegistry, lookup DepositAddressLookup, store DetectedDepositStore) *BoundedRescannerRegistry {
	registry := &BoundedRescannerRegistry{rescanners: make(map[RuntimeKey]*BoundedRescanner)}
	if cfg == nil || runtimes == nil {
		return registry
	}
	for _, key := range runtimes.Keys() {
		networkRuntime, ok := runtimes.Runtime(key.NetworkKey, key.AssetKey)
		if !ok {
			continue
		}
		network := cfg.Web3Deposit.Networks[key.NetworkKey]
		asset := network.Assets[key.AssetKey]
		registry.rescanners[key] = newBoundedRescannerForAsset(network, asset, networkRuntime, lookup, store)
	}
	return registry
}

func NewBoundedRescanner(cfg *config.Config, runtime *ConfluxNetworkRuntime, lookup DepositAddressLookup, store DetectedDepositStore) *BoundedRescanner {
	r := &BoundedRescanner{}
	if cfg == nil || runtime == nil || !runtime.Ready() || runtime.Pool() == nil {
		r.initErr = fmt.Errorf("web3 deposit network is unavailable")
		return r
	}
	network, ok := cfg.Web3Deposit.Networks[config.DefaultWeb3DepositNetworkKey]
	if !ok {
		r.initErr = fmt.Errorf("web3 deposit network config missing")
		return r
	}
	asset, ok := network.Assets[config.DefaultWeb3DepositAssetKey]
	if !ok {
		r.initErr = fmt.Errorf("web3 deposit asset config missing")
		return r
	}
	return newBoundedRescannerForAsset(network, asset, runtime, lookup, store)
}

func newBoundedRescannerForAsset(network config.Web3DepositNetworkConfig, asset config.Web3DepositAssetConfig, runtime *ConfluxNetworkRuntime, lookup DepositAddressLookup, store DetectedDepositStore) *BoundedRescanner {
	r := &BoundedRescanner{}
	if runtime == nil || !runtime.Ready() || runtime.Pool() == nil {
		r.initErr = fmt.Errorf("web3 deposit network is unavailable")
		return r
	}
	chain, err := scannerChainConfig(network, asset)
	if err != nil {
		r.initErr = err
		return r
	}
	finality, err := NewRPCCanonicalDepositSource(runtime.Pool())
	if err != nil {
		r.initErr = err
		return r
	}
	r.transfer = NewTransferLogFetcher(runtime.Pool(), chain.ChainID, common.HexToAddress(asset.ContractAddress), TransferLogFetcherOptions{MaxBlockRange: network.BlockBatchSize})
	r.finality = finality
	r.matcher = NewRecipientMatcher(lookup, DefaultRecipientLookupChunkSize)
	r.persister = NewDepositEventPersister(store, chain)
	r.maxRange = network.BlockBatchSize
	r.scanStartBlock = network.ScanStartBlock
	return r
}

func (r *BoundedRescannerRegistry) Rescan(ctx context.Context, networkKey, assetKey string, fromBlock, toBlock uint64) (BoundedRescanResult, error) {
	if r == nil {
		return BoundedRescanResult{}, fmt.Errorf("web3 deposit rescan registry is unavailable")
	}
	rescanner, ok := r.rescanners[RuntimeKey{NetworkKey: networkKey, AssetKey: assetKey}]
	if !ok {
		return BoundedRescanResult{}, fmt.Errorf("web3 deposit rescan target is unavailable")
	}
	return rescanner.Rescan(ctx, fromBlock, toBlock)
}

func (r *BoundedRescanner) Rescan(ctx context.Context, fromBlock, toBlock uint64) (BoundedRescanResult, error) {
	if r == nil || r.initErr != nil {
		return BoundedRescanResult{}, r.initErr
	}
	if toBlock < fromBlock {
		return BoundedRescanResult{}, ErrRescanRangeInvalid
	}
	if r.maxRange == 0 || toBlock-fromBlock+1 > r.maxRange {
		return BoundedRescanResult{}, fmt.Errorf("%w: no larger than %d blocks", ErrRescanRangeTooLarge, r.maxRange)
	}
	if fromBlock < r.scanStartBlock {
		return BoundedRescanResult{}, fmt.Errorf("%w: scan_start_block=%d", ErrRescanBeforeScanStartBlock, r.scanStartBlock)
	}
	if r.finality == nil {
		return BoundedRescanResult{}, fmt.Errorf("web3 deposit rescan finality source is unavailable")
	}
	finalizedBlock, err := r.finality.FinalizedBlockNumber(ctx)
	if err != nil {
		return BoundedRescanResult{}, fmt.Errorf("read web3 deposit rescan finalized block: %w", err)
	}
	if toBlock > finalizedBlock {
		return BoundedRescanResult{}, fmt.Errorf("%w: finalized_block=%d", ErrRescanBeyondFinalizedBlock, finalizedBlock)
	}
	events, err := r.transfer.Fetch(ctx, fromBlock, toBlock)
	if err != nil {
		return BoundedRescanResult{}, err
	}
	sortTransferEvents(events)
	matches, err := r.matcher.Match(ctx, events)
	if err != nil {
		return BoundedRescanResult{}, err
	}
	deposits, err := r.persister.PersistDetected(ctx, matches)
	if err != nil {
		return BoundedRescanResult{}, err
	}
	return BoundedRescanResult{FromBlock: fromBlock, ToBlock: toBlock, EventCount: len(events), MatchedCount: len(matches), DepositCount: len(deposits)}, nil
}
