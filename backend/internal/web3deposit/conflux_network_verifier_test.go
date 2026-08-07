package web3deposit

import (
	"context"
	"encoding/hex"
	"encoding/json"
	"errors"
	"math/big"
	"net/http"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/ethereum/go-ethereum/common"
	"github.com/stretchr/testify/require"
)

func TestConfluxNetworkVerifierIsolatesBadEndpoint(t *testing.T) {
	expected := testConfluxNetworkExpectation()
	bad := newConfluxRPCResponseCaller(map[string]any{
		"eth_chainId": "0x1",
	})
	good := healthyConfluxRPCResponseCaller(expected.TokenDecimals)
	pool := newConfluxRPCPoolForTest(t, map[string]confluxRPCCaller{
		"https://bad.example":  bad,
		"https://good.example": good,
	}, []string{"https://bad.example", "https://good.example"}, ConfluxRPCPoolOptions{
		RequestTimeout:  time.Second,
		FailureCooldown: time.Minute,
	})

	results, err := pool.VerifyNetwork(context.Background(), expected)
	require.NoError(t, err)
	require.Equal(t, []ConfluxEndpointVerification{
		{EndpointID: "endpoint_1", Failure: "chain_id_mismatch"},
		{EndpointID: "endpoint_2", Healthy: true},
	}, results)
	require.False(t, pool.EndpointStates()[0].Healthy)
	require.True(t, pool.EndpointStates()[1].Healthy)
}

func TestConfluxNetworkVerifierRejectsInvalidNetworkResponses(t *testing.T) {
	expected := testConfluxNetworkExpectation()
	tests := []struct {
		name      string
		responses map[string]any
		failure   string
	}{
		{name: "chain ID", responses: map[string]any{"eth_chainId": "0x1"}, failure: "chain_id_mismatch"},
		{name: "missing code", responses: map[string]any{"eth_chainId": "0x406", "eth_getCode": "0x"}, failure: "token_code_missing"},
		{name: "wrong decimals", responses: map[string]any{
			"eth_chainId": "0x406", "eth_getCode": "0x6000", "eth_call": encodedConfluxTokenDecimals(18),
		}, failure: "token_decimals_mismatch"},
		{name: "missing finalized block", responses: map[string]any{
			"eth_chainId": "0x406", "eth_getCode": "0x6000", "eth_call": encodedConfluxTokenDecimals(6), "eth_getBlockByNumber": nil,
		}, failure: "finalized_invalid"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			caller := newConfluxRPCResponseCaller(test.responses)
			pool := newConfluxRPCPoolForTest(t, map[string]confluxRPCCaller{
				"https://secret.example/provider-token": caller,
			}, []string{"https://secret.example/provider-token"}, ConfluxRPCPoolOptions{})

			results, err := pool.VerifyNetwork(context.Background(), expected)
			require.ErrorIs(t, err, ErrConfluxNetworkVerificationFailed)
			require.Equal(t, test.failure, results[0].Failure)
			require.NotContains(t, err.Error(), "provider-token")
		})
	}
}

func TestConfluxNetworkRuntimeVerifiesEnabledConfiguration(t *testing.T) {
	cfg := testConfluxNetworkRuntimeConfig()
	expected := testConfluxNetworkExpectation()
	options := ConfluxRPCPoolOptions{
		dial: func(_ context.Context, _ string, _ *http.Client) (confluxRPCCaller, error) {
			return healthyConfluxRPCResponseCaller(expected.TokenDecimals), nil
		},
	}

	runtime := newConfluxNetworkRuntime(context.Background(), cfg, options)
	t.Cleanup(runtime.Close)

	require.True(t, runtime.Ready())
	require.NoError(t, runtime.VerificationError())
	require.NotNil(t, runtime.Pool())
}

func TestConfluxNetworkRuntimeFailsClosed(t *testing.T) {
	cfg := testConfluxNetworkRuntimeConfig()
	options := ConfluxRPCPoolOptions{
		dial: func(_ context.Context, _ string, _ *http.Client) (confluxRPCCaller, error) {
			return newConfluxRPCResponseCaller(map[string]any{"eth_chainId": "0x1"}), nil
		},
	}

	runtime := newConfluxNetworkRuntime(context.Background(), cfg, options)
	t.Cleanup(runtime.Close)

	require.False(t, runtime.Ready())
	require.ErrorIs(t, runtime.VerificationError(), ErrConfluxNetworkVerificationFailed)
	require.Nil(t, runtime.Pool())
}

func TestConfluxNetworkRuntimeSkipsDisabledFeature(t *testing.T) {
	dialed := false
	runtime := newConfluxNetworkRuntime(context.Background(), &config.Config{}, ConfluxRPCPoolOptions{
		dial: func(context.Context, string, *http.Client) (confluxRPCCaller, error) {
			dialed = true
			return nil, errors.New("unexpected dial")
		},
	})

	require.False(t, runtime.Ready())
	require.NoError(t, runtime.VerificationError())
	require.False(t, dialed)
}

type confluxRPCResponseCaller struct {
	responses map[string]any
}

func newConfluxRPCResponseCaller(responses map[string]any) *confluxRPCResponseCaller {
	return &confluxRPCResponseCaller{responses: responses}
}

func healthyConfluxRPCResponseCaller(decimals int32) *confluxRPCResponseCaller {
	return newConfluxRPCResponseCaller(map[string]any{
		"eth_chainId": "0x406",
		"eth_getCode": "0x6000",
		"eth_call":    encodedConfluxTokenDecimals(decimals),
		"eth_getBlockByNumber": map[string]any{
			"number": "0x10",
			"hash":   "0x1111111111111111111111111111111111111111111111111111111111111111",
		},
	})
}

func (c *confluxRPCResponseCaller) CallContext(_ context.Context, result any, method string, _ ...any) error {
	value, ok := c.responses[method]
	if !ok {
		return errors.New("method unavailable")
	}
	payload, err := json.Marshal(value)
	if err != nil {
		return err
	}
	return json.Unmarshal(payload, result)
}

func (c *confluxRPCResponseCaller) Close() {}

func testConfluxNetworkExpectation() ConfluxNetworkExpectation {
	return ConfluxNetworkExpectation{
		ChainID:       config.DefaultWeb3DepositChainID,
		TokenAddress:  common.HexToAddress(config.DefaultWeb3DepositTokenAddress),
		TokenDecimals: config.DefaultWeb3DepositTokenDecimals,
	}
}

func testConfluxNetworkRuntimeConfig() *config.Config {
	return &config.Config{Web3Deposit: config.Web3DepositConfig{
		Enabled: true,
		Networks: map[string]config.Web3DepositNetworkConfig{
			config.DefaultWeb3DepositNetworkKey: {
				Enabled: true,
				ChainID: config.DefaultWeb3DepositChainID,
				RPCURLs: []string{"https://rpc.example"},
				Assets: map[string]config.Web3DepositAssetConfig{
					config.DefaultWeb3DepositAssetKey: {
						ContractAddress: config.DefaultWeb3DepositTokenAddress,
						Decimals:        config.DefaultWeb3DepositTokenDecimals,
					},
				},
			},
		},
	}}
}

func encodedConfluxTokenDecimals(decimals int32) string {
	value := make([]byte, common.HashLength)
	new(big.Int).SetInt64(int64(decimals)).FillBytes(value)
	return "0x" + hex.EncodeToString(value)
}
