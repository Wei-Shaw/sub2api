package handler

import (
	"context"
	"errors"
	"sort"
	"strconv"
	"time"

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
	runtime          web3DepositRuntimeReadiness
	deposits         web3deposit.UserDepositReader
}

type web3DepositRuntimeReadiness interface {
	AssetReady(networkKey, assetKey string) bool
}

type web3DepositAddressAllocator interface {
	GetOrCreate(context.Context, int64, web3deposit.ConfiguredWallet) (web3deposit.DepositAddress, error)
}

type web3DepositAddressResponse struct {
	Assigned bool     `json:"assigned"`
	Address  string   `json:"address,omitempty"`
	Networks []string `json:"networks"`
}

func NewWeb3DepositHandler(
	cfg *config.Config,
	addressAllocator *web3deposit.AddressAllocator,
	runtime *web3deposit.ScannerRuntimeRegistry,
	deposits web3deposit.UserDepositReader,
) *Web3DepositHandler {
	return &Web3DepositHandler{cfg: cfg, addressAllocator: addressAllocator, runtime: runtime, deposits: deposits}
}

// GetConfig returns the publicly safe Web3 deposit configuration.
// GET /api/v1/payment/web3/config
func (h *Web3DepositHandler) GetConfig(c *gin.Context) {
	response.Success(c, web3deposit.BuildPublicConfig(h.cfg.Web3Deposit, h.runtime))
}

// GetOrCreateAddress returns the authenticated user's long-lived EVM deposit address.
// POST /api/v1/payment/web3/address
func (h *Web3DepositHandler) GetOrCreateAddress(c *gin.Context) {
	subject, ok := middleware2.GetAuthSubjectFromContext(c)
	if !ok || subject.UserID <= 0 {
		response.Unauthorized(c, "User not authenticated")
		return
	}

	wallet, _, err := resolveWeb3DepositWallet(h.cfg.Web3Deposit)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	availableNetworks := availableWeb3DepositNetworks(h.cfg.Web3Deposit, h.runtime)
	if len(availableNetworks) == 0 {
		response.ErrorFrom(c, infraerrors.ServiceUnavailable("WEB3_DEPOSIT_UNAVAILABLE", "web3 deposit network is temporarily unavailable"))
		return
	}
	address, err := h.addressAllocator.GetOrCreate(c.Request.Context(), subject.UserID, wallet)
	if err != nil {
		response.ErrorFrom(c, web3DepositAddressError(err))
		return
	}

	response.Success(c, web3DepositAddressResponse{
		Assigned: true,
		Address:  address.Address,
		Networks: availableNetworks,
	})
}

func availableWeb3DepositNetworks(cfg config.Web3DepositConfig, readiness web3DepositRuntimeReadiness) []string {
	if readiness == nil {
		return nil
	}
	networks := make([]string, 0, len(cfg.Networks))
	for networkKey, network := range cfg.Networks {
		if !network.Enabled {
			continue
		}
		for assetKey := range network.Assets {
			if readiness.AssetReady(networkKey, assetKey) {
				networks = append(networks, networkKey)
				break
			}
		}
	}
	sort.Strings(networks)
	return networks
}

type web3DepositUserResponse struct {
	ID             int64      `json:"id"`
	ChainID        string     `json:"chain_id"`
	TokenContract  string     `json:"token_contract"`
	TxHash         string     `json:"tx_hash"`
	LogIndex       string     `json:"log_index"`
	BlockNumber    string     `json:"block_number"`
	FromAddress    string     `json:"from_address"`
	ToAddress      string     `json:"to_address"`
	TokenAmount    string     `json:"token_amount"`
	CreditedAmount *string    `json:"credited_amount,omitempty"`
	Status         string     `json:"status"`
	DetectedAt     time.Time  `json:"detected_at"`
	FinalizedAt    *time.Time `json:"finalized_at,omitempty"`
	CreditedAt     *time.Time `json:"credited_at,omitempty"`
}

// ListDeposits returns the authenticated user's Web3 deposit history.
// GET /api/v1/payment/web3/deposits
func (h *Web3DepositHandler) ListDeposits(c *gin.Context) {
	subject, ok := middleware2.GetAuthSubjectFromContext(c)
	if !ok || subject.UserID <= 0 {
		response.Unauthorized(c, "User not authenticated")
		return
	}
	page, pageSize := response.ParsePagination(c)
	if pageSize > 100 {
		pageSize = 100
	}
	deposits, total, err := h.deposits.ListUserDeposits(c.Request.Context(), subject.UserID, page, pageSize)
	if err != nil {
		response.ErrorFrom(c, infraerrors.InternalServer("WEB3_DEPOSIT_HISTORY_FAILED", "failed to load web3 deposit history").WithCause(err))
		return
	}
	items := make([]web3DepositUserResponse, 0, len(deposits))
	for _, deposit := range deposits {
		items = append(items, web3DepositUserFromDomain(deposit))
	}
	response.Paginated(c, items, total, page, pageSize)
}

// GetDeposit returns one Web3 deposit owned by the authenticated user.
// GET /api/v1/payment/web3/deposits/:id
func (h *Web3DepositHandler) GetDeposit(c *gin.Context) {
	subject, ok := middleware2.GetAuthSubjectFromContext(c)
	if !ok || subject.UserID <= 0 {
		response.Unauthorized(c, "User not authenticated")
		return
	}
	depositID, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil || depositID <= 0 {
		response.ErrorFrom(c, infraerrors.BadRequest("WEB3_DEPOSIT_ID_INVALID", "invalid web3 deposit id"))
		return
	}
	deposit, err := h.deposits.GetUserDeposit(c.Request.Context(), subject.UserID, depositID)
	if errors.Is(err, web3deposit.ErrDepositNotFound) {
		response.ErrorFrom(c, infraerrors.NotFound("WEB3_DEPOSIT_NOT_FOUND", "web3 deposit not found"))
		return
	}
	if err != nil {
		response.ErrorFrom(c, infraerrors.InternalServer("WEB3_DEPOSIT_DETAIL_FAILED", "failed to load web3 deposit").WithCause(err))
		return
	}
	response.Success(c, web3DepositUserFromDomain(deposit))
}

func web3DepositUserFromDomain(deposit web3deposit.Deposit) web3DepositUserResponse {
	return web3DepositUserResponse{
		ID:             deposit.ID,
		ChainID:        strconv.FormatUint(deposit.ChainID, 10),
		TokenContract:  deposit.TokenContract,
		TxHash:         deposit.TxHash,
		LogIndex:       strconv.FormatUint(deposit.LogIndex, 10),
		BlockNumber:    strconv.FormatUint(deposit.BlockNumber, 10),
		FromAddress:    deposit.FromAddress,
		ToAddress:      deposit.ToAddress,
		TokenAmount:    deposit.TokenAmount,
		CreditedAmount: deposit.CreditedAmount,
		Status:         web3DepositUserStatus(deposit.Status),
		DetectedAt:     deposit.DetectedAt,
		FinalizedAt:    deposit.FinalizedAt,
		CreditedAt:     deposit.CreditedAt,
	}
}

func web3DepositUserStatus(status web3deposit.DepositStatus) string {
	switch status {
	case web3deposit.DepositStatusCredited:
		return "credited"
	case web3deposit.DepositStatusBelowMinimum:
		return "below_minimum"
	case web3deposit.DepositStatusManualReview:
		return "under_review"
	case web3deposit.DepositStatusOrphaned, web3deposit.DepositStatusFailed, web3deposit.DepositStatusIgnored:
		return "failed"
	default:
		return "confirming"
	}
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
