package web3deposit

import (
	"context"
	"fmt"
	"log/slog"
	"sort"
	"strings"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/ethereum/go-ethereum/common"
)

type ConfluxNetworkRuntime struct {
	enabled           bool
	ready             bool
	chainID           uint64
	tokenContract     string
	pool              *ConfluxRPCPool
	verificationError error
}

type RuntimeKey struct {
	NetworkKey string
	AssetKey   string
}

type ConfluxNetworkRuntimeRegistry struct {
	enabled  bool
	runtimes map[RuntimeKey]*ConfluxNetworkRuntime
}

func NewConfluxNetworkRuntime(cfg *config.Config) *ConfluxNetworkRuntime {
	return newConfluxNetworkRuntime(context.Background(), cfg, ConfluxRPCPoolOptions{})
}

func NewConfluxNetworkRuntimeRegistry(cfg *config.Config) *ConfluxNetworkRuntimeRegistry {
	return newConfluxNetworkRuntimeRegistry(context.Background(), cfg, ConfluxRPCPoolOptions{})
}

func newConfluxNetworkRuntime(ctx context.Context, cfg *config.Config, options ConfluxRPCPoolOptions) *ConfluxNetworkRuntime {
	runtime := &ConfluxNetworkRuntime{}
	if cfg == nil || !cfg.Web3Deposit.Enabled {
		web3RuntimeMetrics.rpcHealthy.Store(false)
		return runtime
	}
	runtime.enabled = true

	network, ok := cfg.Web3Deposit.Networks[config.DefaultWeb3DepositNetworkKey]
	if !ok || !network.Enabled {
		runtime.verificationError = fmt.Errorf("verify conflux network: enabled network configuration is missing")
		return runtime
	}
	asset, ok := network.Assets[config.DefaultWeb3DepositAssetKey]
	if !ok {
		runtime.verificationError = fmt.Errorf("verify conflux network: token configuration is invalid")
		return runtime
	}
	runtime = newConfluxNetworkRuntimeForAsset(ctx, network, asset, options)
	web3RuntimeMetrics.rpcHealthy.Store(runtime.Ready())
	return runtime
}

func newConfluxNetworkRuntimeRegistry(ctx context.Context, cfg *config.Config, options ConfluxRPCPoolOptions) *ConfluxNetworkRuntimeRegistry {
	registry := &ConfluxNetworkRuntimeRegistry{runtimes: make(map[RuntimeKey]*ConfluxNetworkRuntime)}
	if cfg == nil || !cfg.Web3Deposit.Enabled {
		web3RuntimeMetrics.rpcHealthy.Store(false)
		return registry
	}
	registry.enabled = true
	for _, networkKey := range sortedRuntimeKeys(cfg.Web3Deposit.Networks) {
		network := cfg.Web3Deposit.Networks[networkKey]
		if !network.Enabled {
			continue
		}
		for _, assetKey := range sortedRuntimeKeys(network.Assets) {
			registry.runtimes[RuntimeKey{NetworkKey: networkKey, AssetKey: assetKey}] = newConfluxNetworkRuntimeForAsset(ctx, network, network.Assets[assetKey], options)
		}
	}
	web3RuntimeMetrics.rpcHealthy.Store(registry.AllReady())
	return registry
}

func newConfluxNetworkRuntimeForAsset(ctx context.Context, network config.Web3DepositNetworkConfig, asset config.Web3DepositAssetConfig, options ConfluxRPCPoolOptions) *ConfluxNetworkRuntime {
	runtime := &ConfluxNetworkRuntime{enabled: true, chainID: network.ChainID, tokenContract: strings.ToLower(asset.ContractAddress)}
	if !common.IsHexAddress(asset.ContractAddress) {
		runtime.verificationError = fmt.Errorf("verify conflux network: token configuration is invalid")
		return runtime
	}

	pool, err := NewConfluxRPCPool(ctx, network.RPCURLs, options)
	if err != nil {
		runtime.verificationError = err
		return runtime
	}
	runtime.pool = pool
	_, err = pool.VerifyNetwork(ctx, ConfluxNetworkExpectation{
		ChainID:       network.ChainID,
		TokenAddress:  common.HexToAddress(asset.ContractAddress),
		TokenDecimals: asset.Decimals,
	})
	if err != nil {
		runtime.verificationError = err
		slog.Warn("web3 deposit conflux network verification failed", "error", err)
		return runtime
	}
	runtime.ready = true
	slog.Info("web3_deposit_rpc_ready", "chain_id", network.ChainID, "token_contract", asset.ContractAddress)
	return runtime
}

func (r *ConfluxNetworkRuntimeRegistry) AssetReady(networkKey, assetKey string) bool {
	runtime, ok := r.Runtime(networkKey, assetKey)
	return ok && runtime.Ready()
}

func (r *ConfluxNetworkRuntimeRegistry) Runtime(networkKey, assetKey string) (*ConfluxNetworkRuntime, bool) {
	if r == nil {
		return nil, false
	}
	runtime, ok := r.runtimes[RuntimeKey{NetworkKey: networkKey, AssetKey: assetKey}]
	return runtime, ok
}

func (r *ConfluxNetworkRuntimeRegistry) Find(chainID uint64, tokenContract string) (*ConfluxNetworkRuntime, bool) {
	if r == nil {
		return nil, false
	}
	normalizedContract := strings.ToLower(tokenContract)
	for _, runtime := range r.runtimes {
		if runtime.chainID == chainID && runtime.tokenContract == normalizedContract {
			return runtime, true
		}
	}
	return nil, false
}

func (r *ConfluxNetworkRuntimeRegistry) AllReady() bool {
	if r == nil || !r.enabled || len(r.runtimes) == 0 {
		return false
	}
	for _, runtime := range r.runtimes {
		if !runtime.Ready() {
			return false
		}
	}
	return true
}

func (r *ConfluxNetworkRuntimeRegistry) Keys() []RuntimeKey {
	if r == nil {
		return nil
	}
	keys := make([]RuntimeKey, 0, len(r.runtimes))
	for key := range r.runtimes {
		keys = append(keys, key)
	}
	sort.Slice(keys, func(i, j int) bool {
		if keys[i].NetworkKey != keys[j].NetworkKey {
			return keys[i].NetworkKey < keys[j].NetworkKey
		}
		return keys[i].AssetKey < keys[j].AssetKey
	})
	return keys
}

func (r *ConfluxNetworkRuntimeRegistry) Close() {
	if r == nil {
		return
	}
	for _, runtime := range r.runtimes {
		runtime.Close()
	}
}

func sortedRuntimeKeys[T any](values map[string]T) []string {
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}

func (r *ConfluxNetworkRuntime) Ready() bool {
	if r == nil || !r.enabled || !r.ready || r.pool == nil {
		return false
	}
	for _, endpoint := range r.pool.EndpointStates() {
		if endpoint.Healthy {
			return true
		}
	}
	return false
}

func (r *ConfluxNetworkRuntime) VerificationError() error {
	if r == nil {
		return nil
	}
	return r.verificationError
}

func (r *ConfluxNetworkRuntime) Pool() *ConfluxRPCPool {
	if r == nil || !r.ready {
		return nil
	}
	return r.pool
}

func (r *ConfluxNetworkRuntime) EndpointStates() []ConfluxRPCEndpointState {
	if r == nil || r.pool == nil {
		return nil
	}
	return r.pool.EndpointStates()
}

func (r *ConfluxNetworkRuntime) Close() {
	if r != nil && r.pool != nil {
		r.pool.Close()
	}
}
