package web3deposit

import (
	"encoding/json"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/stretchr/testify/require"
)

type publicConfigReadinessStub map[RuntimeKey]bool

func (s publicConfigReadinessStub) AssetReady(networkKey, assetKey string) bool {
	return s[RuntimeKey{NetworkKey: networkKey, AssetKey: assetKey}]
}

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

	got := BuildPublicConfig(cfg, publicConfigReadinessStub{
		{NetworkKey: "a_network", AssetKey: "usdt0"}:   true,
		{NetworkKey: "z_network", AssetKey: "a_token"}: true,
		{NetworkKey: "z_network", AssetKey: "z_token"}: true,
	})

	require.True(t, got.Enabled)
	require.Empty(t, got.UnavailableReason)
	require.Equal(t, []string{"a_network", "z_network"}, []string{got.Networks[0].Key, got.Networks[1].Key})
	require.Equal(t, "1030", got.Networks[0].ChainID)
	require.Equal(t, "USDT0", got.Networks[0].Assets[0].DisplayName)
	require.Equal(t, []string{"a_token", "z_token"}, []string{got.Networks[1].Assets[0].Key, got.Networks[1].Assets[1].Key})
	require.Equal(t, "1.000000", got.Networks[0].Assets[0].MinimumDeposit)
	require.Equal(t, "10000.000000", got.Networks[0].Assets[0].AutomaticCreditLimit)
	require.Equal(t, "0", got.Networks[0].Assets[0].FeeRate)
	require.Equal(t, "finalized", got.Networks[0].Assets[0].CreditFinality)
}

func TestBuildPublicConfigUnavailableReturnsReasonAndNoNetworks(t *testing.T) {
	for _, test := range []struct {
		cfg        config.Web3DepositConfig
		readiness  PublicConfigReadiness
		wantReason PublicConfigUnavailableReason
	}{
		{cfg: config.Web3DepositConfig{Enabled: false, UserEntryEnabled: true}, readiness: publicConfigReadinessStub{}, wantReason: PublicConfigUnavailableFeatureDisabled},
		{cfg: config.Web3DepositConfig{Enabled: true, UserEntryEnabled: false}, readiness: publicConfigReadinessStub{}, wantReason: PublicConfigUnavailableUserEntryDisabled},
		{cfg: config.Web3DepositConfig{Enabled: true, UserEntryEnabled: true}, readiness: publicConfigReadinessStub{}, wantReason: PublicConfigUnavailableRuntimeUnhealthy},
	} {
		got := BuildPublicConfig(test.cfg, test.readiness)
		require.False(t, got.Enabled)
		require.Equal(t, test.wantReason, got.UnavailableReason)
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

	payload, err := json.Marshal(BuildPublicConfig(cfg, publicConfigReadinessStub{{NetworkKey: "conflux", AssetKey: "usdt0"}: true}))
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

func TestBuildPublicConfigOmitsUnhealthyAssetsAndNetworks(t *testing.T) {
	cfg := config.Web3DepositConfig{Enabled: true, UserEntryEnabled: true, Networks: map[string]config.Web3DepositNetworkConfig{
		"network_a": {Enabled: true, Assets: map[string]config.Web3DepositAssetConfig{"healthy": {}, "unhealthy": {}}},
		"network_b": {Enabled: true, Assets: map[string]config.Web3DepositAssetConfig{"unhealthy": {}}},
	}}
	got := BuildPublicConfig(cfg, publicConfigReadinessStub{{NetworkKey: "network_a", AssetKey: "healthy"}: true})
	require.True(t, got.Enabled)
	require.Len(t, got.Networks, 1)
	require.Equal(t, "network_a", got.Networks[0].Key)
	require.Equal(t, "healthy", got.Networks[0].Assets[0].Key)
}
