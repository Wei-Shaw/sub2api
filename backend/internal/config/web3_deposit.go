package config

import (
	"fmt"
	"net"
	"net/url"
	"regexp"
	"sort"
	"strconv"
	"strings"

	"github.com/btcsuite/btcd/btcutil/hdkeychain"
	"github.com/ethereum/go-ethereum/common"
	"github.com/shopspring/decimal"
)

const (
	maxWeb3DepositConfigKeyLength = 64
	maxWeb3DepositRPCEndpoints    = 16
	maxWeb3DepositRPCURLLength    = 2048
	maxWeb3DepositPollInterval    = 300
	maxWeb3DepositBlockBatchSize  = 10000
	maxWeb3DepositTokenDecimals   = 36
	web3DepositBalanceScale       = 8
)

var (
	web3DepositConfigKeyPattern   = regexp.MustCompile(`^[a-z0-9_]{1,64}$`)
	web3DepositAccountPathPattern = regexp.MustCompile(`^m/44'/60'/(0|[1-9][0-9]*)'$`)
	web3DepositAmountPattern      = regexp.MustCompile(`^(0|[1-9][0-9]*)(\.[0-9]+)?$`)
	web3DepositMaxBalanceAmount   = decimal.RequireFromString("999999999999.99999999")
)

func validateWeb3DepositConfig(cfg *Web3DepositConfig) error {
	if cfg == nil {
		return nil
	}
	if !cfg.Enabled {
		if cfg.ScannerEnabled {
			return fmt.Errorf("web3_deposit.scanner_enabled requires web3_deposit.enabled")
		}
		if cfg.CreditEnabled {
			return fmt.Errorf("web3_deposit.credit_enabled requires web3_deposit.enabled")
		}
		if cfg.UserEntryEnabled {
			return fmt.Errorf("web3_deposit.user_entry_enabled requires web3_deposit.enabled")
		}
	}
	if cfg.CreditEnabled && !cfg.ScannerEnabled {
		return fmt.Errorf("web3_deposit.credit_enabled requires web3_deposit.scanner_enabled")
	}

	if err := normalizeAndValidateWeb3DepositWallets(cfg); err != nil {
		return err
	}
	enabledNetworks, err := normalizeAndValidateWeb3DepositNetworks(cfg)
	if err != nil {
		return err
	}
	if !cfg.Enabled && enabledNetworks > 0 {
		return fmt.Errorf("enabled web3_deposit networks require web3_deposit.enabled")
	}
	if cfg.Enabled && enabledNetworks == 0 {
		return fmt.Errorf("web3_deposit.enabled requires at least one enabled network")
	}
	return nil
}

func normalizeAndValidateWeb3DepositWallets(cfg *Web3DepositConfig) error {
	seenXpubs := make(map[string]string)
	for _, walletKey := range sortedWeb3DepositKeys(cfg.Wallets) {
		path := "web3_deposit.wallets." + walletKey
		if err := validateWeb3DepositConfigKey(walletKey, "wallet"); err != nil {
			return err
		}

		wallet := cfg.Wallets[walletKey]
		wallet.AccountXPub = strings.TrimSpace(wallet.AccountXPub)
		wallet.AccountPath = strings.TrimSpace(wallet.AccountPath)
		var accountChildIndex uint32
		if wallet.AccountPath != "" {
			var err error
			accountChildIndex, err = parseWeb3DepositAccountPath(wallet.AccountPath)
			if err != nil {
				return fmt.Errorf("%s.account_path must match m/44'/60'/{account}' with account below %d", path, hdkeychain.HardenedKeyStart)
			}
		}
		if wallet.AccountXPub != "" {
			normalizedXpub, xpubChildIndex, err := validateWeb3DepositAccountXPub(wallet.AccountXPub)
			if err != nil {
				return fmt.Errorf("%s.account_xpub is invalid", path)
			}
			if wallet.AccountPath != "" && xpubChildIndex != accountChildIndex {
				return fmt.Errorf("%s.account_path does not match account_xpub", path)
			}
			wallet.AccountXPub = normalizedXpub
			if existingWallet, exists := seenXpubs[normalizedXpub]; exists {
				return fmt.Errorf("%s.account_xpub duplicates wallet %q", path, existingWallet)
			}
			seenXpubs[normalizedXpub] = walletKey
		}
		cfg.Wallets[walletKey] = wallet
	}
	return nil
}

func parseWeb3DepositAccountPath(path string) (uint32, error) {
	matches := web3DepositAccountPathPattern.FindStringSubmatch(path)
	if len(matches) != 2 {
		return 0, fmt.Errorf("invalid account path")
	}
	accountIndex, err := strconv.ParseUint(matches[1], 10, 31)
	if err != nil {
		return 0, err
	}
	return hdkeychain.HardenedKeyStart + uint32(accountIndex), nil
}

func validateWeb3DepositAccountXPub(raw string) (string, uint32, error) {
	key, err := hdkeychain.NewKeyFromString(raw)
	if err != nil {
		return "", 0, err
	}
	defer key.Zero()
	if key.IsPrivate() || key.Depth() != 3 || key.ChildIndex() < hdkeychain.HardenedKeyStart {
		return "", 0, fmt.Errorf("account extended public key required")
	}

	external, err := key.Derive(0)
	if err != nil {
		return "", 0, err
	}
	defer external.Zero()
	child, err := external.Derive(0)
	if err != nil {
		return "", 0, err
	}
	defer child.Zero()
	if _, err := child.ECPubKey(); err != nil {
		return "", 0, err
	}
	return key.String(), key.ChildIndex(), nil
}

func normalizeAndValidateWeb3DepositNetworks(cfg *Web3DepositConfig) (int, error) {
	enabledNetworks := 0
	enabledChainIDs := make(map[uint64]string)
	for _, networkKey := range sortedWeb3DepositKeys(cfg.Networks) {
		path := "web3_deposit.networks." + networkKey
		if err := validateWeb3DepositConfigKey(networkKey, "network"); err != nil {
			return 0, err
		}

		network := cfg.Networks[networkKey]
		network.DisplayName = strings.TrimSpace(network.DisplayName)
		network.WalletID = strings.TrimSpace(network.WalletID)
		network.RPCURLs = normalizeWeb3DepositRPCURLs(network.RPCURLs)
		if err := validateWeb3DepositNetworkStatic(path, network); err != nil {
			return 0, err
		}
		if err := normalizeAndValidateWeb3DepositAssets(path, &network); err != nil {
			return 0, err
		}
		if networkKey == DefaultWeb3DepositNetworkKey {
			if err := validateDefaultConfluxDepositNetwork(path, network); err != nil {
				return 0, err
			}
		}
		if network.Enabled {
			enabledNetworks++
			if existingNetwork, exists := enabledChainIDs[network.ChainID]; exists {
				return 0, fmt.Errorf("%s.chain_id duplicates enabled network %q", path, existingNetwork)
			}
			enabledChainIDs[network.ChainID] = networkKey
			if err := validateEnabledWeb3DepositNetwork(path, network, cfg.Wallets); err != nil {
				return 0, err
			}
		}
		cfg.Networks[networkKey] = network
	}
	return enabledNetworks, nil
}

func validateWeb3DepositNetworkStatic(path string, network Web3DepositNetworkConfig) error {
	if network.Enabled && network.ChainID == 0 {
		return fmt.Errorf("%s.chain_id must be positive", path)
	}
	if network.PollIntervalSeconds != 0 && (network.PollIntervalSeconds < 1 || network.PollIntervalSeconds > maxWeb3DepositPollInterval) {
		return fmt.Errorf("%s.poll_interval_seconds must be between 1 and %d", path, maxWeb3DepositPollInterval)
	}
	if network.Enabled && network.PollIntervalSeconds == 0 {
		return fmt.Errorf("%s.poll_interval_seconds must be between 1 and %d", path, maxWeb3DepositPollInterval)
	}
	if network.BlockBatchSize > maxWeb3DepositBlockBatchSize {
		return fmt.Errorf("%s.block_batch_size must be between 1 and %d", path, maxWeb3DepositBlockBatchSize)
	}
	if network.Enabled && network.BlockBatchSize == 0 {
		return fmt.Errorf("%s.block_batch_size must be between 1 and %d", path, maxWeb3DepositBlockBatchSize)
	}
	if network.BlockBatchSize > 0 && network.OverlapBlocks >= network.BlockBatchSize {
		return fmt.Errorf("%s.overlap_blocks must be less than block_batch_size", path)
	}
	if len(network.RPCURLs) > maxWeb3DepositRPCEndpoints {
		return fmt.Errorf("%s.rpc_urls must contain at most %d endpoints", path, maxWeb3DepositRPCEndpoints)
	}
	for index, rpcURL := range network.RPCURLs {
		if err := validateWeb3DepositRPCURL(rpcURL); err != nil {
			return fmt.Errorf("%s.rpc_urls[%d] is invalid", path, index)
		}
	}
	return nil
}

func validateEnabledWeb3DepositNetwork(path string, network Web3DepositNetworkConfig, wallets map[string]Web3DepositWalletConfig) error {
	if network.DisplayName == "" {
		return fmt.Errorf("%s.display_name is required when enabled", path)
	}
	if network.WalletID == "" {
		return fmt.Errorf("%s.wallet_id is required when enabled", path)
	}
	wallet, exists := wallets[network.WalletID]
	if !exists {
		return fmt.Errorf("%s.wallet_id references an unknown wallet", path)
	}
	if wallet.AccountXPub == "" {
		return fmt.Errorf("web3_deposit.wallets.%s.account_xpub is required by enabled network %q", network.WalletID, strings.TrimPrefix(path, "web3_deposit.networks."))
	}
	if wallet.AccountPath == "" {
		return fmt.Errorf("web3_deposit.wallets.%s.account_path is required by enabled network %q", network.WalletID, strings.TrimPrefix(path, "web3_deposit.networks."))
	}
	if len(network.RPCURLs) == 0 {
		return fmt.Errorf("%s.rpc_urls is required when enabled", path)
	}
	if len(network.Assets) == 0 {
		return fmt.Errorf("%s.assets must contain at least one asset when enabled", path)
	}
	return nil
}

func normalizeAndValidateWeb3DepositAssets(path string, network *Web3DepositNetworkConfig) error {
	seenContracts := make(map[common.Address]string)
	for _, assetKey := range sortedWeb3DepositKeys(network.Assets) {
		assetPath := path + ".assets." + assetKey
		if err := validateWeb3DepositConfigKey(assetKey, "asset"); err != nil {
			return err
		}

		asset := network.Assets[assetKey]
		asset.ContractAddress = strings.TrimSpace(asset.ContractAddress)
		asset.MinimumDeposit = strings.TrimSpace(asset.MinimumDeposit)
		asset.AutoCreditLimit = strings.TrimSpace(asset.AutoCreditLimit)
		if !common.IsHexAddress(asset.ContractAddress) {
			return fmt.Errorf("%s.contract_address is invalid", assetPath)
		}
		contractAddress := common.HexToAddress(asset.ContractAddress)
		if contractAddress == (common.Address{}) {
			return fmt.Errorf("%s.contract_address must not be the zero address", assetPath)
		}
		if existingAsset, exists := seenContracts[contractAddress]; exists {
			return fmt.Errorf("%s.contract_address duplicates asset %q", assetPath, existingAsset)
		}
		seenContracts[contractAddress] = assetKey
		asset.ContractAddress = strings.ToLower(contractAddress.Hex())
		if asset.Decimals < 0 || asset.Decimals > maxWeb3DepositTokenDecimals {
			return fmt.Errorf("%s.decimals must be between 0 and %d", assetPath, maxWeb3DepositTokenDecimals)
		}

		maximumRuleScale := asset.Decimals
		if maximumRuleScale > web3DepositBalanceScale {
			maximumRuleScale = web3DepositBalanceScale
		}
		minimumDeposit, err := parseWeb3DepositAmount(assetPath+".minimum_deposit", asset.MinimumDeposit, maximumRuleScale)
		if err != nil {
			return err
		}
		autoCreditLimit, err := parseWeb3DepositAmount(assetPath+".auto_credit_limit", asset.AutoCreditLimit, maximumRuleScale)
		if err != nil {
			return err
		}
		if autoCreditLimit.LessThan(minimumDeposit) {
			return fmt.Errorf("%s.auto_credit_limit must be greater than or equal to minimum_deposit", assetPath)
		}
		asset.MinimumDeposit = minimumDeposit.StringFixed(asset.Decimals)
		asset.AutoCreditLimit = autoCreditLimit.StringFixed(asset.Decimals)
		network.Assets[assetKey] = asset
	}
	return nil
}

func parseWeb3DepositAmount(path, raw string, maximumScale int32) (decimal.Decimal, error) {
	if !web3DepositAmountPattern.MatchString(raw) {
		return decimal.Zero, fmt.Errorf("%s must be a plain positive decimal string", path)
	}
	amount, err := decimal.NewFromString(raw)
	if err != nil || !amount.IsPositive() {
		return decimal.Zero, fmt.Errorf("%s must be a plain positive decimal string", path)
	}
	if amount.Exponent() < -maximumScale {
		return decimal.Zero, fmt.Errorf("%s has too many decimal places", path)
	}
	if amount.GreaterThan(web3DepositMaxBalanceAmount) {
		return decimal.Zero, fmt.Errorf("%s exceeds the platform balance range", path)
	}
	return amount, nil
}

func validateDefaultConfluxDepositNetwork(path string, network Web3DepositNetworkConfig) error {
	if network.ChainID != DefaultWeb3DepositChainID {
		return fmt.Errorf("%s.chain_id must be %d", path, DefaultWeb3DepositChainID)
	}
	asset, exists := network.Assets[DefaultWeb3DepositAssetKey]
	if !exists {
		return fmt.Errorf("%s.assets.%s is required", path, DefaultWeb3DepositAssetKey)
	}
	if asset.ContractAddress != DefaultWeb3DepositTokenAddress {
		return fmt.Errorf("%s.assets.%s.contract_address must match the configured Conflux eSpace USDT0 contract", path, DefaultWeb3DepositAssetKey)
	}
	if asset.Decimals != DefaultWeb3DepositTokenDecimals {
		return fmt.Errorf("%s.assets.%s.decimals must be %d", path, DefaultWeb3DepositAssetKey, DefaultWeb3DepositTokenDecimals)
	}
	if network.Enabled && network.ScanStartBlock == 0 {
		return fmt.Errorf("%s.scan_start_block must be positive when enabled", path)
	}
	return nil
}

func validateWeb3DepositConfigKey(key, kind string) error {
	if len(key) > maxWeb3DepositConfigKeyLength || !web3DepositConfigKeyPattern.MatchString(key) {
		return fmt.Errorf("web3_deposit %s key must match [a-z0-9_]{1,64}", kind)
	}
	return nil
}

func normalizeWeb3DepositRPCURLs(values []string) []string {
	if len(values) == 0 {
		return nil
	}
	result := make([]string, 0, len(values))
	seen := make(map[string]struct{}, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		if _, exists := seen[value]; exists {
			continue
		}
		seen[value] = struct{}{}
		result = append(result, value)
	}
	return result
}

func validateWeb3DepositRPCURL(raw string) error {
	if len(raw) > maxWeb3DepositRPCURLLength {
		return fmt.Errorf("url too long")
	}
	parsed, err := url.Parse(raw)
	if err != nil || parsed.Host == "" || parsed.User != nil || parsed.Fragment != "" {
		return fmt.Errorf("invalid url")
	}
	switch strings.ToLower(parsed.Scheme) {
	case "https", "wss":
		return nil
	case "http", "ws":
		if isLoopbackWeb3DepositHost(parsed.Hostname()) {
			return nil
		}
	}
	return fmt.Errorf("unsupported url scheme")
}

func isLoopbackWeb3DepositHost(host string) bool {
	if strings.EqualFold(host, "localhost") {
		return true
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}

func sortedWeb3DepositKeys[T any](values map[string]T) []string {
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}
