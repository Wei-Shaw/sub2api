package web3deposit

import (
	"encoding/json"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/stretchr/testify/require"
)

func TestBuildPublicConfigReturnsEnabledEntriesInStableOrder(t *testing.T) {
	cfg := config.Web3DepositConfig{
		Enabled:          true,
		UserEntryEnabled: true,
		Networks: map[string]config.Web3DepositNetworkConfig{
			"z_network": {
				Enabled:     true,
				DisplayName: "Z Network",
				ChainID:     999,
				Assets: map[string]config.Web3DepositAssetConfig{
					"z_token": {ContractAddress: "0x2", Decimals: 18, MinimumDeposit: "2.00", AutoCreditLimit: "20.00"},
					"a_token": {ContractAddress: "0x1", Decimals: 6, MinimumDeposit: "1.000000", AutoCreditLimit: "10.000000"},
				},
			},
			"a_network": {
				Enabled:     true,
				DisplayName: "A Network",
				ChainID:     1030,
				Assets: map[string]config.Web3DepositAssetConfig{
					"usdt0": {ContractAddress: "0xaf37", Decimals: 6, MinimumDeposit: "1.000000", AutoCreditLimit: "10000.000000"},
				},
			},
			"disabled_network": {Enabled: false},
		},
	}

	got := BuildPublicConfig(cfg)

	require.True(t, got.Enabled)
	require.Equal(t, []string{"a_network", "z_network"}, []string{got.Networks[0].Key, got.Networks[1].Key})
	require.Equal(t, "1030", got.Networks[0].ChainID)
	require.Equal(t, "USDT0", got.Networks[0].Assets[0].DisplayName)
	require.Equal(t, []string{"a_token", "z_token"}, []string{got.Networks[1].Assets[0].Key, got.Networks[1].Assets[1].Key})
	require.Equal(t, "1.000000", got.Networks[0].Assets[0].MinimumDeposit)
	require.Equal(t, "10000.000000", got.Networks[0].Assets[0].AutomaticCreditLimit)
}

func TestBuildPublicConfigDisabledReturnsNoNetworks(t *testing.T) {
	for _, cfg := range []config.Web3DepositConfig{
		{Enabled: false, UserEntryEnabled: true},
		{Enabled: true, UserEntryEnabled: false},
	} {
		got := BuildPublicConfig(cfg)
		require.False(t, got.Enabled)
		require.Empty(t, got.Networks)
		require.NotNil(t, got.Networks)
	}
}

func TestBuildPublicConfigJSONDoesNotExposeInternalConfiguration(t *testing.T) {
	cfg := config.Web3DepositConfig{
		Enabled:          true,
		ScannerEnabled:   true,
		CreditEnabled:    true,
		UserEntryEnabled: true,
		Wallets: map[string]config.Web3DepositWalletConfig{
			"secret_wallet_id": {AccountXPub: "secret_account_xpub", AccountPath: "m/44'/60'/0'"},
		},
		Networks: map[string]config.Web3DepositNetworkConfig{
			"conflux": {
				Enabled:             true,
				DisplayName:         "Conflux eSpace",
				ChainID:             1030,
				WalletID:            "secret_wallet_id",
				ScanStartBlock:      123,
				RPCURLs:             []string{"https://user:password@secret-rpc.example"},
				PollIntervalSeconds: 15,
				BlockBatchSize:      500,
				OverlapBlocks:       20,
				Assets: map[string]config.Web3DepositAssetConfig{
					"usdt0": {ContractAddress: "0xaf37", Decimals: 6, MinimumDeposit: "1.000000", AutoCreditLimit: "10000.000000"},
				},
			},
		},
	}

	payload, err := json.Marshal(BuildPublicConfig(cfg))
	require.NoError(t, err)

	serialized := string(payload)
	for _, secret := range []string{
		"secret_account_xpub",
		"secret_wallet_id",
		"secret-rpc.example",
		"fingerprint",
		"account_xpub",
		"account_path",
		"wallet_id",
		"scan_start_block",
		"rpc_urls",
		"poll_interval_seconds",
		"block_batch_size",
		"overlap_blocks",
		"scanner_enabled",
		"credit_enabled",
	} {
		require.NotContains(t, serialized, secret)
	}
}
