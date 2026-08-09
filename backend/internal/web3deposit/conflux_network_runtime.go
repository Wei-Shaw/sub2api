package web3deposit

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/ethereum/go-ethereum/common"
)

type ConfluxNetworkRuntime struct {
	enabled           bool
	ready             bool
	pool              *ConfluxRPCPool
	verificationError error
}

func NewConfluxNetworkRuntime(cfg *config.Config) *ConfluxNetworkRuntime {
	return newConfluxNetworkRuntime(context.Background(), cfg, ConfluxRPCPoolOptions{})
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
	if !ok || !common.IsHexAddress(asset.ContractAddress) {
		runtime.verificationError = fmt.Errorf("verify conflux network: token configuration is invalid")
		return runtime
	}

	pool, err := NewConfluxRPCPool(ctx, network.RPCURLs, options)
	if err != nil {
		runtime.verificationError = err
		web3RuntimeMetrics.rpcHealthy.Store(false)
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
		web3RuntimeMetrics.rpcHealthy.Store(false)
		slog.Warn("web3 deposit conflux network verification failed", "error", err)
		return runtime
	}
	runtime.ready = true
	web3RuntimeMetrics.rpcHealthy.Store(true)
	slog.Info("web3_deposit_rpc_ready", "chain_id", network.ChainID, "token_contract", asset.ContractAddress)
	return runtime
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

func (r *ConfluxNetworkRuntime) Close() {
	if r != nil && r.pool != nil {
		r.pool.Close()
	}
}
