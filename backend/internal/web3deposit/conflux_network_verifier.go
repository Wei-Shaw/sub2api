package web3deposit

import (
	"context"
	"errors"
	"fmt"
	"math/big"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/common/hexutil"
)

const confluxTokenDecimalsCallData = "0x313ce567"

var ErrConfluxNetworkVerificationFailed = errors.New("conflux network verification failed")

type ConfluxNetworkExpectation struct {
	ChainID       uint64
	TokenAddress  common.Address
	TokenDecimals int32
}

type ConfluxEndpointVerification struct {
	EndpointID string
	Healthy    bool
	Failure    string
}

type confluxFinalizedBlock struct {
	Number *hexutil.Big `json:"number"`
	Hash   common.Hash  `json:"hash"`
}

func (p *ConfluxRPCPool) VerifyNetwork(ctx context.Context, expected ConfluxNetworkExpectation) ([]ConfluxEndpointVerification, error) {
	p.mu.Lock()
	endpoints := append([]*confluxRPCEndpoint(nil), p.endpoints...)
	p.mu.Unlock()

	results := make([]ConfluxEndpointVerification, 0, len(endpoints))
	healthyCount := 0
	for _, endpoint := range endpoints {
		if endpoint.client == nil {
			results = append(results, ConfluxEndpointVerification{EndpointID: endpoint.id, Failure: "dial_failed"})
			continue
		}
		failure, err := p.verifyEndpoint(ctx, endpoint, expected)
		if err != nil {
			if ctxErr := ctx.Err(); ctxErr != nil {
				return results, ctxErr
			}
			p.markUnhealthy(endpoint)
			results = append(results, ConfluxEndpointVerification{EndpointID: endpoint.id, Failure: failure})
			continue
		}
		p.markHealthy(endpoint)
		healthyCount++
		results = append(results, ConfluxEndpointVerification{EndpointID: endpoint.id, Healthy: true})
	}
	if healthyCount == 0 {
		return results, fmt.Errorf("verify conflux endpoints %v: %w", endpointIDs(endpoints), ErrConfluxNetworkVerificationFailed)
	}
	return results, nil
}

func (p *ConfluxRPCPool) verifyEndpoint(ctx context.Context, endpoint *confluxRPCEndpoint, expected ConfluxNetworkExpectation) (string, error) {
	var chainID hexutil.Uint64
	if err := p.callEndpoint(ctx, endpoint, &chainID, "eth_chainId"); err != nil {
		return "chain_id_unavailable", err
	}
	if uint64(chainID) != expected.ChainID {
		return "chain_id_mismatch", ErrConfluxNetworkVerificationFailed
	}

	var code hexutil.Bytes
	if err := p.callEndpoint(ctx, endpoint, &code, "eth_getCode", expected.TokenAddress.Hex(), "latest"); err != nil {
		return "token_code_unavailable", err
	}
	if len(code) == 0 {
		return "token_code_missing", ErrConfluxNetworkVerificationFailed
	}

	var encodedDecimals hexutil.Bytes
	call := map[string]string{"to": expected.TokenAddress.Hex(), "data": confluxTokenDecimalsCallData}
	if err := p.callEndpoint(ctx, endpoint, &encodedDecimals, "eth_call", call, "latest"); err != nil {
		return "token_decimals_unavailable", err
	}
	if len(encodedDecimals) != common.HashLength || new(big.Int).SetBytes(encodedDecimals).Cmp(big.NewInt(int64(expected.TokenDecimals))) != 0 {
		return "token_decimals_mismatch", ErrConfluxNetworkVerificationFailed
	}

	var finalized *confluxFinalizedBlock
	if err := p.callEndpoint(ctx, endpoint, &finalized, "eth_getBlockByNumber", "finalized", false); err != nil {
		return "finalized_unsupported", err
	}
	if finalized == nil || finalized.Number == nil || finalized.Hash == (common.Hash{}) {
		return "finalized_invalid", ErrConfluxNetworkVerificationFailed
	}
	return "", nil
}

func (p *ConfluxRPCPool) callEndpoint(ctx context.Context, endpoint *confluxRPCEndpoint, result any, method string, args ...any) error {
	attemptCtx, cancel := context.WithTimeout(ctx, p.requestTimeout)
	defer cancel()
	return endpoint.client.CallContext(attemptCtx, result, method, args...)
}

func endpointIDs(endpoints []*confluxRPCEndpoint) []string {
	ids := make([]string, 0, len(endpoints))
	for _, endpoint := range endpoints {
		ids = append(ids, endpoint.id)
	}
	return ids
}
