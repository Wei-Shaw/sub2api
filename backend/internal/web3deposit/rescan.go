package web3deposit

import (
	"context"
	"fmt"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/ethereum/go-ethereum/common"
)

type BoundedRescanResult struct {
	FromBlock    uint64 `json:"from_block"`
	ToBlock      uint64 `json:"to_block"`
	EventCount   int    `json:"event_count"`
	MatchedCount int    `json:"matched_count"`
	DepositCount int    `json:"deposit_count"`
}

type BoundedRescanner struct {
	transfer  ScannerTransferSource
	matcher   ScannerRecipientMatcher
	persister *DepositEventPersister
	maxRange  uint64
	initErr   error
}

func NewBoundedRescanner(cfg *config.Config, runtime *ConfluxNetworkRuntime, lookup ActiveDepositAddressLookup, store DetectedDepositStore) *BoundedRescanner {
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
	chain, err := scannerChainConfig(network, asset)
	if err != nil {
		r.initErr = err
		return r
	}
	r.transfer = NewTransferLogFetcher(runtime.Pool(), chain.ChainID, common.HexToAddress(asset.ContractAddress), TransferLogFetcherOptions{MaxBlockRange: network.BlockBatchSize})
	r.matcher = NewRecipientMatcher(lookup, DefaultRecipientLookupChunkSize)
	r.persister = NewDepositEventPersister(store, chain)
	r.maxRange = network.BlockBatchSize
	return r
}

func (r *BoundedRescanner) Rescan(ctx context.Context, fromBlock, toBlock uint64) (BoundedRescanResult, error) {
	if r == nil || r.initErr != nil {
		return BoundedRescanResult{}, r.initErr
	}
	if toBlock < fromBlock || toBlock-fromBlock+1 > r.maxRange {
		return BoundedRescanResult{}, fmt.Errorf("rescan range must be ordered and no larger than %d blocks", r.maxRange)
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
