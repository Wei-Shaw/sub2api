package handler

import (
	"context"
	"errors"
	"sort"

	"github.com/Wei-Shaw/sub2api/internal/config"
	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
	"github.com/Wei-Shaw/sub2api/internal/pkg/response"
	middleware2 "github.com/Wei-Shaw/sub2api/internal/server/middleware"
	"github.com/Wei-Shaw/sub2api/internal/web3deposit"

	"github.com/gin-gonic/gin"
)

type Web3DepositHandler struct {
	cfg              *config.Config
	addressAllocator web3DepositAddressAllocator
	networkRuntime   *web3deposit.ConfluxNetworkRuntime
}

type web3DepositAddressAllocator interface {
	GetOrCreate(context.Context, int64, web3deposit.ConfiguredWallet) (web3deposit.DepositAddress, error)
}

type web3DepositAddressResponse struct {
	Address  string   `json:"address"`
	Networks []string `json:"networks"`
}

func NewWeb3DepositHandler(
	cfg *config.Config,
	addressAllocator *web3deposit.AddressAllocator,
	networkRuntime *web3deposit.ConfluxNetworkRuntime,
) *Web3DepositHandler {
	return &Web3DepositHandler{cfg: cfg, addressAllocator: addressAllocator, networkRuntime: networkRuntime}
}

// GetConfig returns the publicly safe Web3 deposit configuration.
// GET /api/v1/payment/web3/config
func (h *Web3DepositHandler) GetConfig(c *gin.Context) {
	response.Success(c, web3deposit.BuildPublicConfig(h.cfg.Web3Deposit))
}

// GetOrCreateAddress returns the authenticated user's long-lived EVM deposit address.
// POST /api/v1/payment/web3/address
func (h *Web3DepositHandler) GetOrCreateAddress(c *gin.Context) {
	subject, ok := middleware2.GetAuthSubjectFromContext(c)
	if !ok || subject.UserID <= 0 {
		response.Unauthorized(c, "User not authenticated")
		return
	}

	wallet, networks, err := resolveWeb3DepositWallet(h.cfg.Web3Deposit)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	if h.networkRuntime != nil && !h.networkRuntime.Ready() {
		response.ErrorFrom(c, infraerrors.ServiceUnavailable("WEB3_DEPOSIT_UNAVAILABLE", "web3 deposit network is temporarily unavailable"))
		return
	}
	address, err := h.addressAllocator.GetOrCreate(c.Request.Context(), subject.UserID, wallet)
	if err != nil {
		response.ErrorFrom(c, web3DepositAddressError(err))
		return
	}

	response.Success(c, web3DepositAddressResponse{
		Address:  address.Address,
		Networks: networks,
	})
}

func resolveWeb3DepositWallet(cfg config.Web3DepositConfig) (web3deposit.ConfiguredWallet, []string, error) {
	if !cfg.Enabled || !cfg.UserEntryEnabled {
		return web3deposit.ConfiguredWallet{}, nil, infraerrors.ServiceUnavailable("WEB3_DEPOSIT_DISABLED", "web3 deposits are not enabled")
	}

	networks := make([]string, 0, len(cfg.Networks))
	walletID := ""
	for networkKey, network := range cfg.Networks {
		if !network.Enabled {
			continue
		}
		if walletID == "" {
			walletID = network.WalletID
		} else if network.WalletID != walletID {
			return web3deposit.ConfiguredWallet{}, nil, infraerrors.ServiceUnavailable("WEB3_DEPOSIT_CONFIG_INVALID", "web3 deposit configuration is invalid")
		}
		networks = append(networks, networkKey)
	}
	if walletID == "" {
		return web3deposit.ConfiguredWallet{}, nil, infraerrors.ServiceUnavailable("WEB3_DEPOSIT_CONFIG_INVALID", "web3 deposit configuration is invalid")
	}
	wallet, ok := cfg.Wallets[walletID]
	if !ok || wallet.AccountXPub == "" || wallet.AccountPath == "" {
		return web3deposit.ConfiguredWallet{}, nil, infraerrors.ServiceUnavailable("WEB3_DEPOSIT_CONFIG_INVALID", "web3 deposit configuration is invalid")
	}

	sort.Strings(networks)
	return web3deposit.ConfiguredWallet{
		WalletID:    walletID,
		AccountPath: wallet.AccountPath,
		AccountXPub: wallet.AccountXPub,
	}, networks, nil
}

func web3DepositAddressError(err error) error {
	switch {
	case errors.Is(err, web3deposit.ErrAddressDisabled):
		return infraerrors.Conflict("WEB3_DEPOSIT_ADDRESS_DISABLED", "web3 deposit address is disabled")
	case errors.Is(err, web3deposit.ErrAddressAllocationConflict):
		return infraerrors.Conflict("WEB3_DEPOSIT_ADDRESS_CONFLICT", "web3 deposit address allocation conflicted; retry the request")
	case errors.Is(err, web3deposit.ErrWalletDisabled),
		errors.Is(err, web3deposit.ErrWalletFingerprintMismatch),
		errors.Is(err, web3deposit.ErrWalletAccountPathMismatch),
		errors.Is(err, web3deposit.ErrDerivationIndexExhausted):
		return infraerrors.ServiceUnavailable("WEB3_DEPOSIT_UNAVAILABLE", "web3 deposit address is temporarily unavailable")
	default:
		return infraerrors.InternalServer("WEB3_DEPOSIT_ADDRESS_FAILED", "failed to allocate web3 deposit address").WithCause(err)
	}
}
