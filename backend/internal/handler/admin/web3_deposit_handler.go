package admin

import (
	"errors"
	"strconv"
	"strings"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/handler/dto"
	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
	"github.com/Wei-Shaw/sub2api/internal/pkg/response"
	"github.com/Wei-Shaw/sub2api/internal/web3deposit"
	"github.com/gin-gonic/gin"
)

type Web3DepositHandler struct {
	deposits  web3deposit.AdminDepositReader
	operator  web3deposit.AdminDepositOperator
	runtime   *web3deposit.ScannerRuntime
	network   *web3deposit.ConfluxNetworkRuntime
	rescanner *web3deposit.BoundedRescanner
}

func NewWeb3DepositHandler(deposits web3deposit.AdminDepositReader, operator web3deposit.AdminDepositOperator, runtime *web3deposit.ScannerRuntime, network *web3deposit.ConfluxNetworkRuntime, rescanner *web3deposit.BoundedRescanner) *Web3DepositHandler {
	return &Web3DepositHandler{deposits: deposits, operator: operator, runtime: runtime, network: network, rescanner: rescanner}
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
	result := make([]dto.AdminWeb3Deposit, 0, len(items))
	for _, item := range items {
		result = append(result, dto.Web3DepositFromDomainAdmin(item))
	}
	response.Paginated(c, result, total, page, pageSize)
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
	response.Success(c, dto.Web3DepositFromDomainAdmin(item))
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
	counts, _ := h.deposits.CountAdminDepositsByStatus(c.Request.Context())
	response.Success(c, gin.H{"state": status.State, "leader": status.LeaseHeld, "last_error": status.LastError, "latest_block": strconv.FormatUint(status.LastResult.HeadBlock, 10), "scanned_block": strconv.FormatUint(status.LastResult.ToBlock, 10), "lag_blocks": strconv.FormatUint(lag, 10), "metrics": web3deposit.SnapshotRuntimeMetrics(), "status_counts": counts})
}

func (h *Web3DepositHandler) Approve(c *gin.Context) {
	id, ok := adminDepositID(c)
	if !ok {
		return
	}
	deposit, err := h.deposits.GetAdminDeposit(c.Request.Context(), id)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	if deposit.Status != web3deposit.DepositStatusManualReview {
		response.ErrorFrom(c, infraerrors.Conflict("WEB3_DEPOSIT_STATE_CONFLICT", "deposit is not awaiting review"))
		return
	}
	if h.network == nil || !h.network.Ready() || h.network.Pool() == nil {
		response.ErrorFrom(c, infraerrors.ServiceUnavailable("WEB3_DEPOSIT_UNAVAILABLE", "web3 deposit network is unavailable"))
		return
	}
	source, _ := web3deposit.NewRPCCanonicalDepositSource(h.network.Pool())
	finalized, err := source.FinalizedBlockNumber(c.Request.Context())
	if err != nil || deposit.BlockNumber > finalized {
		response.ErrorFrom(c, infraerrors.Conflict("WEB3_DEPOSIT_NOT_FINALIZED", "deposit is not finalized"))
		return
	}
	verifier, _ := web3deposit.NewCanonicalDepositVerifier(source)
	verification, err := verifier.Verify(c.Request.Context(), deposit)
	if err != nil {
		response.ErrorFrom(c, infraerrors.ServiceUnavailable("WEB3_DEPOSIT_VERIFY_FAILED", "failed to verify deposit").WithCause(err))
		return
	}
	if !verification.Valid {
		response.ErrorFrom(c, infraerrors.Conflict("WEB3_DEPOSIT_CANONICAL_MISMATCH", string(verification.Reason)))
		return
	}
	if err := h.operator.ApproveReviewedDeposit(c.Request.Context(), id); err != nil {
		response.ErrorFrom(c, adminDepositOperationError(err))
		return
	}
	response.Success(c, gin.H{"status": web3deposit.DepositStatusReadyToCredit})
}

func (h *Web3DepositHandler) Ignore(c *gin.Context) {
	id, ok := adminDepositID(c)
	if !ok {
		return
	}
	var input struct {
		Reason string `json:"reason"`
	}
	if err := c.ShouldBindJSON(&input); err != nil || strings.TrimSpace(input.Reason) == "" {
		response.BadRequest(c, "Reason is required")
		return
	}
	if err := h.operator.IgnoreReviewedDeposit(c.Request.Context(), id, input.Reason); err != nil {
		response.ErrorFrom(c, adminDepositOperationError(err))
		return
	}
	response.Success(c, gin.H{"status": web3deposit.DepositStatusIgnored})
}

func (h *Web3DepositHandler) Retry(c *gin.Context) {
	id, ok := adminDepositID(c)
	if !ok {
		return
	}
	if err := h.operator.RetryFailedDeposit(c.Request.Context(), id); err != nil {
		response.ErrorFrom(c, adminDepositOperationError(err))
		return
	}
	response.Success(c, gin.H{"status": web3deposit.DepositStatusReadyToCredit})
}

func (h *Web3DepositHandler) Rescan(c *gin.Context) {
	var input struct {
		FromBlock string `json:"from_block"`
		ToBlock   string `json:"to_block"`
	}
	if err := c.ShouldBindJSON(&input); err != nil {
		response.BadRequest(c, "Invalid rescan request")
		return
	}
	fromBlock, err1 := strconv.ParseUint(input.FromBlock, 10, 64)
	toBlock, err2 := strconv.ParseUint(input.ToBlock, 10, 64)
	if err1 != nil || err2 != nil {
		response.BadRequest(c, "Invalid block range")
		return
	}
	result, err := h.rescanner.Rescan(c.Request.Context(), fromBlock, toBlock)
	if err != nil {
		response.ErrorFrom(c, infraerrors.BadRequest("WEB3_DEPOSIT_RESCAN_INVALID", err.Error()))
		return
	}
	response.Success(c, result)
}

func adminDepositID(c *gin.Context) (int64, bool) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil || id <= 0 {
		response.BadRequest(c, "Invalid deposit id")
		return 0, false
	}
	return id, true
}
func adminDepositOperationError(err error) error {
	if errors.Is(err, web3deposit.ErrAdminDepositStateConflict) {
		return infraerrors.Conflict("WEB3_DEPOSIT_STATE_CONFLICT", "deposit state does not allow this operation")
	}
	return infraerrors.InternalServer("WEB3_DEPOSIT_ADMIN_OPERATION_FAILED", "web3 deposit operation failed").WithCause(err)
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
