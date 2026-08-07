package config

import (
	"bytes"
	"strings"
	"testing"

	"github.com/btcsuite/btcd/btcutil/hdkeychain"
	"github.com/btcsuite/btcd/chaincfg"
	"github.com/stretchr/testify/require"
)

func testWeb3DepositAccountKeys(t *testing.T) (string, string) {
	return testWeb3DepositAccountKeysForAccount(t, 0)
}

func testWeb3DepositAccountKeysForAccount(t *testing.T, account uint32) (string, string) {
	t.Helper()
	master, err := hdkeychain.NewMaster(bytes.Repeat([]byte{0x2a}, 32), &chaincfg.MainNetParams)
	require.NoError(t, err)
	defer master.Zero()

	key := master
	for _, index := range []uint32{44, 60, account} {
		child, err := key.Derive(hdkeychain.HardenedKeyStart + index)
		require.NoError(t, err)
		if key != master {
			key.Zero()
		}
		key = child
	}
	defer key.Zero()

	publicKey, err := key.Neuter()
	require.NoError(t, err)
	defer publicKey.Zero()
	return publicKey.String(), key.String()
}

func validWeb3DepositConfig(t *testing.T) Web3DepositConfig {
	t.Helper()
	accountXPub, _ := testWeb3DepositAccountKeys(t)
	return Web3DepositConfig{
		Enabled:          true,
		ScannerEnabled:   true,
		UserEntryEnabled: true,
		Wallets: map[string]Web3DepositWalletConfig{
			DefaultWeb3DepositWalletKey: {
				AccountXPub: accountXPub,
				AccountPath: DefaultWeb3DepositAccountPath,
			},
		},
		Networks: map[string]Web3DepositNetworkConfig{
			DefaultWeb3DepositNetworkKey: {
				Enabled:             true,
				DisplayName:         DefaultWeb3DepositNetworkDisplayName,
				ChainID:             DefaultWeb3DepositChainID,
				WalletID:            DefaultWeb3DepositWalletKey,
				ScanStartBlock:      1,
				RPCURLs:             []string{"https://rpc.example"},
				PollIntervalSeconds: DefaultWeb3DepositPollIntervalSecs,
				BlockBatchSize:      DefaultWeb3DepositBlockBatchSize,
				OverlapBlocks:       DefaultWeb3DepositOverlapBlocks,
				Assets: map[string]Web3DepositAssetConfig{
					DefaultWeb3DepositAssetKey: {
						ContractAddress: DefaultWeb3DepositTokenAddress,
						Decimals:        DefaultWeb3DepositTokenDecimals,
						MinimumDeposit:  DefaultWeb3DepositMinimumAmount,
						AutoCreditLimit: DefaultWeb3DepositAutoCreditLimit,
					},
				},
			},
		},
	}
}

func TestValidateWeb3DepositConfigNormalizesValues(t *testing.T) {
	cfg := validWeb3DepositConfig(t)
	network := cfg.Networks[DefaultWeb3DepositNetworkKey]
	network.DisplayName = "  Conflux eSpace Mainnet  "
	network.WalletID = "  " + DefaultWeb3DepositWalletKey + "  "
	network.RPCURLs = []string{" https://rpc.example ", "https://rpc.example", "https://backup.example"}
	asset := network.Assets[DefaultWeb3DepositAssetKey]
	asset.ContractAddress = "0xAf37E8B6C9ED7f6318979f56Fc287d76c30847ff"
	asset.MinimumDeposit = "1"
	asset.AutoCreditLimit = "10000"
	network.Assets[DefaultWeb3DepositAssetKey] = asset
	cfg.Networks[DefaultWeb3DepositNetworkKey] = network

	require.NoError(t, validateWeb3DepositConfig(&cfg))
	network = cfg.Networks[DefaultWeb3DepositNetworkKey]
	require.Equal(t, DefaultWeb3DepositNetworkDisplayName, network.DisplayName)
	require.Equal(t, DefaultWeb3DepositWalletKey, network.WalletID)
	require.Equal(t, []string{"https://rpc.example", "https://backup.example"}, network.RPCURLs)
	require.Equal(t, DefaultWeb3DepositTokenAddress, network.Assets[DefaultWeb3DepositAssetKey].ContractAddress)
	require.Equal(t, DefaultWeb3DepositMinimumAmount, network.Assets[DefaultWeb3DepositAssetKey].MinimumDeposit)
	require.Equal(t, DefaultWeb3DepositAutoCreditLimit, network.Assets[DefaultWeb3DepositAssetKey].AutoCreditLimit)
}

func TestValidateWeb3DepositConfigFeatureDependencies(t *testing.T) {
	tests := []struct {
		name    string
		mutate  func(*Web3DepositConfig)
		wantErr string
	}{
		{name: "scanner requires master", mutate: func(cfg *Web3DepositConfig) { cfg.Enabled = false }, wantErr: "scanner_enabled requires"},
		{name: "credit requires scanner", mutate: func(cfg *Web3DepositConfig) {
			cfg.CreditEnabled = true
			cfg.ScannerEnabled = false
		}, wantErr: "credit_enabled requires"},
		{name: "user entry requires master", mutate: func(cfg *Web3DepositConfig) { cfg.Enabled = false; cfg.ScannerEnabled = false }, wantErr: "user_entry_enabled requires"},
		{name: "network requires master", mutate: func(cfg *Web3DepositConfig) {
			cfg.Enabled = false
			cfg.ScannerEnabled = false
			cfg.UserEntryEnabled = false
		}, wantErr: "enabled web3_deposit networks require"},
		{name: "master requires network", mutate: func(cfg *Web3DepositConfig) {
			network := cfg.Networks[DefaultWeb3DepositNetworkKey]
			network.Enabled = false
			cfg.Networks[DefaultWeb3DepositNetworkKey] = network
		}, wantErr: "requires at least one enabled network"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			cfg := validWeb3DepositConfig(t)
			test.mutate(&cfg)
			err := validateWeb3DepositConfig(&cfg)
			require.ErrorContains(t, err, test.wantErr)
		})
	}
}

func TestValidateWeb3DepositConfigRejectsPrivateOrInvalidXPubWithoutLeaking(t *testing.T) {
	accountXPub, accountXPrv := testWeb3DepositAccountKeys(t)
	tests := []string{accountXPrv, "xpub-secret-invalid-value"}
	for _, sensitiveValue := range tests {
		cfg := validWeb3DepositConfig(t)
		wallet := cfg.Wallets[DefaultWeb3DepositWalletKey]
		wallet.AccountXPub = sensitiveValue
		cfg.Wallets[DefaultWeb3DepositWalletKey] = wallet

		err := validateWeb3DepositConfig(&cfg)
		require.ErrorContains(t, err, ".account_xpub is invalid")
		require.NotContains(t, err.Error(), sensitiveValue)
		require.NotContains(t, err.Error(), accountXPub)
	}
}

func TestValidateWeb3DepositConfigWalletAndNetworkReferences(t *testing.T) {
	t.Run("invalid account path", func(t *testing.T) {
		cfg := validWeb3DepositConfig(t)
		wallet := cfg.Wallets[DefaultWeb3DepositWalletKey]
		wallet.AccountPath = "m/44'/0'/0'"
		cfg.Wallets[DefaultWeb3DepositWalletKey] = wallet
		require.ErrorContains(t, validateWeb3DepositConfig(&cfg), "account_path")
	})

	t.Run("account path does not match xpub", func(t *testing.T) {
		cfg := validWeb3DepositConfig(t)
		wallet := cfg.Wallets[DefaultWeb3DepositWalletKey]
		wallet.AccountPath = "m/44'/60'/1'"
		cfg.Wallets[DefaultWeb3DepositWalletKey] = wallet
		require.ErrorContains(t, validateWeb3DepositConfig(&cfg), "does not match account_xpub")
	})

	t.Run("unknown wallet", func(t *testing.T) {
		cfg := validWeb3DepositConfig(t)
		network := cfg.Networks[DefaultWeb3DepositNetworkKey]
		network.WalletID = "missing_wallet"
		cfg.Networks[DefaultWeb3DepositNetworkKey] = network
		require.ErrorContains(t, validateWeb3DepositConfig(&cfg), "unknown wallet")
	})

	t.Run("duplicate xpub", func(t *testing.T) {
		cfg := validWeb3DepositConfig(t)
		cfg.Wallets["duplicate_wallet"] = cfg.Wallets[DefaultWeb3DepositWalletKey]
		require.ErrorContains(t, validateWeb3DepositConfig(&cfg), "duplicates wallet")
	})

	t.Run("disabled incomplete template is allowed", func(t *testing.T) {
		cfg := validWeb3DepositConfig(t)
		cfg.Networks["future_evm"] = Web3DepositNetworkConfig{}
		require.NoError(t, validateWeb3DepositConfig(&cfg))
	})
}

func TestValidateWeb3DepositConfigRPCURLs(t *testing.T) {
	validURLs := []string{
		"https://rpc.example/api/key",
		"wss://rpc.example/ws",
		"http://127.0.0.1:8545",
		"http://localhost:8545",
		"ws://[::1]:8546",
	}
	for _, rpcURL := range validURLs {
		cfg := validWeb3DepositConfig(t)
		network := cfg.Networks[DefaultWeb3DepositNetworkKey]
		network.RPCURLs = []string{rpcURL}
		cfg.Networks[DefaultWeb3DepositNetworkKey] = network
		require.NoError(t, validateWeb3DepositConfig(&cfg), rpcURL)
	}

	invalidURLs := []string{
		"http://rpc.example/secret-api-key",
		"https://user:password@rpc.example",
		"https://rpc.example/#secret-fragment",
		"file:///tmp/geth.ipc",
		"not-a-url",
	}
	for _, rpcURL := range invalidURLs {
		cfg := validWeb3DepositConfig(t)
		network := cfg.Networks[DefaultWeb3DepositNetworkKey]
		network.RPCURLs = []string{rpcURL}
		cfg.Networks[DefaultWeb3DepositNetworkKey] = network
		err := validateWeb3DepositConfig(&cfg)
		require.ErrorContains(t, err, "rpc_urls[0] is invalid")
		require.NotContains(t, err.Error(), rpcURL)
	}
}

func TestValidateWeb3DepositConfigScannerBounds(t *testing.T) {
	tests := []struct {
		name    string
		mutate  func(*Web3DepositNetworkConfig)
		wantErr string
	}{
		{name: "zero poll", mutate: func(network *Web3DepositNetworkConfig) { network.PollIntervalSeconds = 0 }, wantErr: "poll_interval_seconds"},
		{name: "large poll", mutate: func(network *Web3DepositNetworkConfig) { network.PollIntervalSeconds = maxWeb3DepositPollInterval + 1 }, wantErr: "poll_interval_seconds"},
		{name: "zero batch", mutate: func(network *Web3DepositNetworkConfig) { network.BlockBatchSize = 0 }, wantErr: "block_batch_size"},
		{name: "large batch", mutate: func(network *Web3DepositNetworkConfig) { network.BlockBatchSize = maxWeb3DepositBlockBatchSize + 1 }, wantErr: "block_batch_size"},
		{name: "overlap equals batch", mutate: func(network *Web3DepositNetworkConfig) { network.OverlapBlocks = network.BlockBatchSize }, wantErr: "overlap_blocks"},
		{name: "mainnet zero start", mutate: func(network *Web3DepositNetworkConfig) { network.ScanStartBlock = 0 }, wantErr: "scan_start_block"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			cfg := validWeb3DepositConfig(t)
			network := cfg.Networks[DefaultWeb3DepositNetworkKey]
			test.mutate(&network)
			cfg.Networks[DefaultWeb3DepositNetworkKey] = network
			require.ErrorContains(t, validateWeb3DepositConfig(&cfg), test.wantErr)
		})
	}
}

func TestValidateWeb3DepositConfigAssetRules(t *testing.T) {
	tests := []struct {
		name    string
		mutate  func(*Web3DepositAssetConfig)
		wantErr string
	}{
		{name: "invalid address", mutate: func(asset *Web3DepositAssetConfig) { asset.ContractAddress = "0x1234" }, wantErr: "contract_address is invalid"},
		{name: "zero address", mutate: func(asset *Web3DepositAssetConfig) {
			asset.ContractAddress = "0x0000000000000000000000000000000000000000"
		}, wantErr: "zero address"},
		{name: "negative decimals", mutate: func(asset *Web3DepositAssetConfig) { asset.Decimals = -1 }, wantErr: "decimals"},
		{name: "large decimals", mutate: func(asset *Web3DepositAssetConfig) { asset.Decimals = maxWeb3DepositTokenDecimals + 1 }, wantErr: "decimals"},
		{name: "scientific minimum", mutate: func(asset *Web3DepositAssetConfig) { asset.MinimumDeposit = "1e2" }, wantErr: "plain positive decimal"},
		{name: "zero minimum", mutate: func(asset *Web3DepositAssetConfig) { asset.MinimumDeposit = "0" }, wantErr: "plain positive decimal"},
		{name: "too precise", mutate: func(asset *Web3DepositAssetConfig) { asset.MinimumDeposit = "1.0000001" }, wantErr: "too many decimal places"},
		{name: "balance overflow", mutate: func(asset *Web3DepositAssetConfig) { asset.AutoCreditLimit = "1000000000000.000000" }, wantErr: "platform balance range"},
		{name: "limit below minimum", mutate: func(asset *Web3DepositAssetConfig) { asset.MinimumDeposit = "2"; asset.AutoCreditLimit = "1" }, wantErr: "greater than or equal"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			cfg := validWeb3DepositConfig(t)
			network := cfg.Networks[DefaultWeb3DepositNetworkKey]
			asset := network.Assets[DefaultWeb3DepositAssetKey]
			test.mutate(&asset)
			network.Assets[DefaultWeb3DepositAssetKey] = asset
			cfg.Networks[DefaultWeb3DepositNetworkKey] = network
			require.ErrorContains(t, validateWeb3DepositConfig(&cfg), test.wantErr)
		})
	}

	t.Run("duplicate contract", func(t *testing.T) {
		cfg := validWeb3DepositConfig(t)
		network := cfg.Networks[DefaultWeb3DepositNetworkKey]
		network.Assets["duplicate"] = network.Assets[DefaultWeb3DepositAssetKey]
		cfg.Networks[DefaultWeb3DepositNetworkKey] = network
		require.ErrorContains(t, validateWeb3DepositConfig(&cfg), "duplicates asset")
	})
}

func TestValidateWeb3DepositConfigConfluxIdentity(t *testing.T) {
	tests := []struct {
		name    string
		mutate  func(*Web3DepositNetworkConfig)
		wantErr string
	}{
		{name: "wrong chain", mutate: func(network *Web3DepositNetworkConfig) { network.ChainID = 71 }, wantErr: "chain_id must be 1030"},
		{name: "missing usdt0", mutate: func(network *Web3DepositNetworkConfig) { delete(network.Assets, DefaultWeb3DepositAssetKey) }, wantErr: ".assets.usdt0 is required"},
		{name: "wrong token", mutate: func(network *Web3DepositNetworkConfig) {
			asset := network.Assets[DefaultWeb3DepositAssetKey]
			asset.ContractAddress = "0x1111111111111111111111111111111111111111"
			network.Assets[DefaultWeb3DepositAssetKey] = asset
		}, wantErr: "must match the configured"},
		{name: "wrong decimals", mutate: func(network *Web3DepositNetworkConfig) {
			asset := network.Assets[DefaultWeb3DepositAssetKey]
			asset.Decimals = 18
			network.Assets[DefaultWeb3DepositAssetKey] = asset
		}, wantErr: "decimals must be 6"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			cfg := validWeb3DepositConfig(t)
			network := cfg.Networks[DefaultWeb3DepositNetworkKey]
			test.mutate(&network)
			cfg.Networks[DefaultWeb3DepositNetworkKey] = network
			require.ErrorContains(t, validateWeb3DepositConfig(&cfg), test.wantErr)
		})
	}
}

func TestValidateWeb3DepositConfigAllowsEnabledNetworksToShareWallet(t *testing.T) {
	cfg := validWeb3DepositConfig(t)
	cfg.Networks["conflux_espace_testnet"] = Web3DepositNetworkConfig{
		Enabled:             true,
		DisplayName:         "Conflux eSpace Testnet",
		ChainID:             71,
		WalletID:            DefaultWeb3DepositWalletKey,
		RPCURLs:             []string{"https://evmtestnet.confluxrpc.com"},
		PollIntervalSeconds: 10,
		BlockBatchSize:      500,
		OverlapBlocks:       20,
		Assets: map[string]Web3DepositAssetConfig{
			"mock_usdt0": {
				ContractAddress: "0x1111111111111111111111111111111111111111",
				Decimals:        6,
				MinimumDeposit:  "1.000000",
				AutoCreditLimit: "10000.000000",
			},
		},
	}
	require.NoError(t, validateWeb3DepositConfig(&cfg))
}

func TestValidateWeb3DepositConfigRejectsDifferentWalletsAcrossEnabledNetworks(t *testing.T) {
	cfg := validWeb3DepositConfig(t)
	accountXPub, _ := testWeb3DepositAccountKeysForAccount(t, 1)
	cfg.Wallets["other_wallet"] = Web3DepositWalletConfig{
		AccountXPub: accountXPub,
		AccountPath: "m/44'/60'/1'",
	}
	cfg.Networks["other_network"] = Web3DepositNetworkConfig{
		Enabled:             true,
		DisplayName:         "Other Network",
		ChainID:             71,
		WalletID:            "other_wallet",
		RPCURLs:             []string{"https://rpc.example"},
		PollIntervalSeconds: 10,
		BlockBatchSize:      500,
		OverlapBlocks:       20,
		Assets: map[string]Web3DepositAssetConfig{
			"usdt": {
				ContractAddress: "0x1111111111111111111111111111111111111111",
				Decimals:        6,
				MinimumDeposit:  "1.000000",
				AutoCreditLimit: "10000.000000",
			},
		},
	}

	require.ErrorContains(t, validateWeb3DepositConfig(&cfg), "wallet_id must match other enabled networks")
}

func TestValidateWeb3DepositConfigRejectsDuplicateEnabledChain(t *testing.T) {
	cfg := validWeb3DepositConfig(t)
	duplicate := cfg.Networks[DefaultWeb3DepositNetworkKey]
	duplicate.ScanStartBlock = 0
	cfg.Networks["duplicate_chain"] = duplicate
	require.ErrorContains(t, validateWeb3DepositConfig(&cfg), "duplicates enabled network")
}

func TestValidateWeb3DepositConfigKey(t *testing.T) {
	cfg := validWeb3DepositConfig(t)
	cfg.Networks["Invalid-Key"] = Web3DepositNetworkConfig{}
	err := validateWeb3DepositConfig(&cfg)
	require.ErrorContains(t, err, "network key must match")
	require.NotContains(t, err.Error(), "Invalid-Key")
}

func TestValidateWeb3DepositConfigRPCErrorDoesNotLeakCredentials(t *testing.T) {
	cfg := validWeb3DepositConfig(t)
	network := cfg.Networks[DefaultWeb3DepositNetworkKey]
	secretURL := "https://api-key:super-secret@rpc.example/path?token=secret"
	network.RPCURLs = []string{secretURL}
	cfg.Networks[DefaultWeb3DepositNetworkKey] = network
	err := validateWeb3DepositConfig(&cfg)
	require.Error(t, err)
	require.False(t, strings.Contains(err.Error(), "super-secret"))
	require.False(t, strings.Contains(err.Error(), "token=secret"))
}
