package web3deposit

import (
	"sort"
	"strconv"
	"strings"

	"github.com/Wei-Shaw/sub2api/internal/config"
)

type PublicConfig struct {
	Enabled  bool            `json:"enabled"`
	Networks []PublicNetwork `json:"networks"`
}

type PublicNetwork struct {
	Key         string        `json:"key"`
	DisplayName string        `json:"display_name"`
	ChainID     string        `json:"chain_id"`
	Assets      []PublicAsset `json:"assets"`
}

type PublicAsset struct {
	Key                  string `json:"key"`
	DisplayName          string `json:"display_name"`
	ContractAddress      string `json:"contract_address"`
	Decimals             int32  `json:"decimals"`
	MinimumDeposit       string `json:"minimum_deposit"`
	AutomaticCreditLimit string `json:"automatic_credit_limit"`
}

func BuildPublicConfig(cfg config.Web3DepositConfig) PublicConfig {
	result := PublicConfig{Networks: make([]PublicNetwork, 0)}
	if !cfg.Enabled || !cfg.UserEntryEnabled {
		return result
	}

	networkKeys := make([]string, 0, len(cfg.Networks))
	for networkKey, network := range cfg.Networks {
		if network.Enabled {
			networkKeys = append(networkKeys, networkKey)
		}
	}
	sort.Strings(networkKeys)

	result.Enabled = true
	result.Networks = make([]PublicNetwork, 0, len(networkKeys))
	for _, networkKey := range networkKeys {
		network := cfg.Networks[networkKey]
		assetKeys := make([]string, 0, len(network.Assets))
		for assetKey := range network.Assets {
			assetKeys = append(assetKeys, assetKey)
		}
		sort.Strings(assetKeys)

		assets := make([]PublicAsset, 0, len(assetKeys))
		for _, assetKey := range assetKeys {
			asset := network.Assets[assetKey]
			assets = append(assets, PublicAsset{
				Key:                  assetKey,
				DisplayName:          strings.ToUpper(assetKey),
				ContractAddress:      asset.ContractAddress,
				Decimals:             asset.Decimals,
				MinimumDeposit:       asset.MinimumDeposit,
				AutomaticCreditLimit: asset.AutoCreditLimit,
			})
		}

		result.Networks = append(result.Networks, PublicNetwork{
			Key:         networkKey,
			DisplayName: network.DisplayName,
			ChainID:     strconv.FormatUint(network.ChainID, 10),
			Assets:      assets,
		})
	}
	return result
}
