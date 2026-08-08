package admin

import (
	"errors"
	"strconv"
	"strings"
	"time"

	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
	"github.com/Wei-Shaw/sub2api/internal/pkg/response"
	"github.com/Wei-Shaw/sub2api/internal/web3deposit"
	"github.com/gin-gonic/gin"
)

type Web3DepositHandler struct {
	deposits web3deposit.AdminDepositReader
	runtime  *web3deposit.ScannerRuntime
}

func NewWeb3DepositHandler(deposits web3deposit.AdminDepositReader, runtime *web3deposit.ScannerRuntime) *Web3DepositHandler {
	return &Web3DepositHandler{deposits: deposits, runtime: runtime}
}

func (h *Web3DepositHandler) List(c *gin.Context) {
	page, pageSize := response.ParsePagination(c)
	if pageSize > 100 {
		pageSize = 100
	}
	filter := web3deposit.AdminDepositFilter{Page: page, PageSize: pageSize, Status: web3deposit.DepositStatus(strings.TrimSpace(c.Query("status"))), Address: c.Query("address"), TxHash: c.Query("tx_hash")}
	if raw := c.Query("user_id"); raw != "" {
		filter.UserID, _ = strconv.ParseInt(raw, 10, 64)
	}
	filter.CreatedAtFrom = parseAdminDepositTime(c.Query("from"))
	filter.CreatedAtTo = parseAdminDepositTime(c.Query("to"))
	items, total, err := h.deposits.ListAdminDeposits(c.Request.Context(), filter)
	if err != nil {
		response.ErrorFrom(c, infraerrors.InternalServer("WEB3_DEPOSIT_ADMIN_LIST_FAILED", "failed to list web3 deposits").WithCause(err))
		return
	}
	response.Paginated(c, items, total, page, pageSize)
}

func (h *Web3DepositHandler) Get(c *gin.Context) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil || id <= 0 {
		response.BadRequest(c, "Invalid deposit id")
		return
	}
	item, err := h.deposits.GetAdminDeposit(c.Request.Context(), id)
	if errors.Is(err, web3deposit.ErrDepositNotFound) {
		response.ErrorFrom(c, infraerrors.NotFound("WEB3_DEPOSIT_NOT_FOUND", "web3 deposit not found"))
		return
	}
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, item)
}

func (h *Web3DepositHandler) Stats(c *gin.Context) {
	counts, err := h.deposits.CountAdminDepositsByStatus(c.Request.Context())
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, counts)
}

func (h *Web3DepositHandler) Runtime(c *gin.Context) {
	if h.runtime == nil {
		response.Success(c, gin.H{"state": "disabled"})
		return
	}
	status := h.runtime.Status()
	lag := uint64(0)
	if status.LastResult.HeadBlock > status.LastResult.ToBlock {
		lag = status.LastResult.HeadBlock - status.LastResult.ToBlock
	}
	response.Success(c, gin.H{"state": status.State, "leader": status.LeaseHeld, "last_error": status.LastError, "latest_block": strconv.FormatUint(status.LastResult.HeadBlock, 10), "scanned_block": strconv.FormatUint(status.LastResult.ToBlock, 10), "lag_blocks": strconv.FormatUint(lag, 10)})
}

func parseAdminDepositTime(value string) *time.Time {
	if value == "" {
		return nil
	}
	parsed, err := time.Parse(time.RFC3339, value)
	if err != nil {
		return nil
	}
	return &parsed
}
