package handler

import (
	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/pkg/response"
	"github.com/Wei-Shaw/sub2api/internal/web3deposit"

	"github.com/gin-gonic/gin"
)

type Web3DepositHandler struct {
	cfg *config.Config
}

func NewWeb3DepositHandler(cfg *config.Config) *Web3DepositHandler {
	return &Web3DepositHandler{cfg: cfg}
}

// GetConfig returns the publicly safe Web3 deposit configuration.
// GET /api/v1/payment/web3/config
func (h *Web3DepositHandler) GetConfig(c *gin.Context) {
	response.Success(c, web3deposit.BuildPublicConfig(h.cfg.Web3Deposit))
}
