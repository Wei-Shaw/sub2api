package admin

import (
	"context"
	"errors"
	"strconv"
	"strings"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/handler/dto"
	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
	"github.com/Wei-Shaw/sub2api/internal/pkg/response"
	servermiddleware "github.com/Wei-Shaw/sub2api/internal/server/middleware"
	"github.com/Wei-Shaw/sub2api/internal/web3deposit"
	"github.com/gin-gonic/gin"
)

type Web3DepositHandler struct {
	deposits   web3deposit.AdminDepositReader
	operator   web3deposit.AdminDepositOperator
	runtimes   web3DepositScannerRuntimeRegistry
	networks   web3DepositNetworkRuntimeRegistry
	rescanJobs web3DepositRescanJobs
}

type web3DepositScannerRuntimeRegistry interface {
	Keys() []web3deposit.RuntimeKey
	Runtime(networkKey, assetKey string) (*web3deposit.ScannerRuntime, bool)
}

type web3DepositNetworkRuntimeRegistry interface {
	Runtime(networkKey, assetKey string) (*web3deposit.ConfluxNetworkRuntime, bool)
	Find(chainID uint64, tokenContract string) (*web3deposit.ConfluxNetworkRuntime, bool)
}

type web3DepositRescanJobs interface {
	Enqueue(ctx context.Context, networkKey, assetKey string, fromBlock, toBlock uint64, requestedBy int64) (web3deposit.RescanJob, error)
	List(ctx context.Context, limit int) ([]web3deposit.RescanJob, error)
	Get(ctx context.Context, id int64) (web3deposit.RescanJob, error)
}

func NewWeb3DepositHandler(deposits web3deposit.AdminDepositReader, operator web3deposit.AdminDepositOperator, runtimes *web3deposit.ScannerRuntimeRegistry, networks *web3deposit.ConfluxNetworkRuntimeRegistry, rescanJobs *web3deposit.RescanJobRuntime) *Web3DepositHandler {
	return &Web3DepositHandler{deposits: deposits, operator: operator, runtimes: runtimes, networks: networks, rescanJobs: rescanJobs}
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
	type endpointResponse struct {
		ID             string     `json:"id"`
		Healthy        bool       `json:"healthy"`
		UnhealthyUntil *time.Time `json:"unhealthy_until,omitempty"`
	}
	type runtimeResponse struct {
		NetworkKey           string             `json:"network_key"`
		AssetKey             string             `json:"asset_key"`
		State                string             `json:"state"`
		Leader               bool               `json:"leader"`
		LastError            string             `json:"last_error"`
		LatestBlock          string             `json:"latest_block"`
		ScannedBlock         string             `json:"scanned_block"`
		FinalizedBlock       string             `json:"finalized_block"`
		FinalizedCursorBlock string             `json:"finalized_cursor_block"`
		ScannerLagBlocks     string             `json:"scanner_lag_blocks"`
		FinalizerLagBlocks   string             `json:"finalizer_lag_blocks"`
		LagBlocks            string             `json:"lag_blocks"`
		Endpoints            []endpointResponse `json:"endpoints"`
	}

	items := make([]runtimeResponse, 0)
	if h.runtimes != nil {
		for _, key := range h.runtimes.Keys() {
			item := runtimeResponse{NetworkKey: key.NetworkKey, AssetKey: key.AssetKey, State: string(web3deposit.ScannerRuntimeStateUnhealthy), Endpoints: []endpointResponse{}}
			if runtime, ok := h.runtimes.Runtime(key.NetworkKey, key.AssetKey); ok && runtime != nil {
				status := runtime.Status()
				scannerLag := uint64(0)
				if status.LastResult.HeadBlock > status.LastResult.ToBlock {
					scannerLag = status.LastResult.HeadBlock - status.LastResult.ToBlock
				}
				finalizerLag := uint64(0)
				if status.LastResult.FinalizedHead > status.LastResult.FinalizedThrough {
					finalizerLag = status.LastResult.FinalizedHead - status.LastResult.FinalizedThrough
				}
				item.State = string(status.State)
				item.Leader = status.LeaseHeld
				item.LastError = status.LastError
				item.LatestBlock = strconv.FormatUint(status.LastResult.HeadBlock, 10)
				item.ScannedBlock = strconv.FormatUint(status.LastResult.ToBlock, 10)
				item.FinalizedBlock = strconv.FormatUint(status.LastResult.FinalizedHead, 10)
				item.FinalizedCursorBlock = strconv.FormatUint(status.LastResult.FinalizedThrough, 10)
				item.ScannerLagBlocks = strconv.FormatUint(scannerLag, 10)
				item.FinalizerLagBlocks = strconv.FormatUint(finalizerLag, 10)
				item.LagBlocks = item.ScannerLagBlocks
			}
			if h.networks != nil {
				if network, ok := h.networks.Runtime(key.NetworkKey, key.AssetKey); ok && network != nil {
					for _, endpoint := range network.EndpointStates() {
						entry := endpointResponse{ID: endpoint.ID, Healthy: endpoint.Healthy}
						if !endpoint.UnhealthyUntil.IsZero() {
							until := endpoint.UnhealthyUntil
							entry.UnhealthyUntil = &until
						}
						item.Endpoints = append(item.Endpoints, entry)
					}
				}
			}
			items = append(items, item)
		}
	}
	counts, _ := h.deposits.CountAdminDepositsByStatus(c.Request.Context())
	response.Success(c, gin.H{"runtimes": items, "metrics": web3deposit.SnapshotRuntimeMetrics(), "status_counts": counts})
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
	if h.networks == nil {
		response.ErrorFrom(c, infraerrors.ServiceUnavailable("WEB3_DEPOSIT_UNAVAILABLE", "web3 deposit network is unavailable"))
		return
	}
	network, found := h.networks.Find(deposit.ChainID, deposit.TokenContract)
	if !found || network == nil || !network.Ready() || network.Pool() == nil {
		response.ErrorFrom(c, infraerrors.ServiceUnavailable("WEB3_DEPOSIT_UNAVAILABLE", "web3 deposit network is unavailable"))
		return
	}
	source, _ := web3deposit.NewRPCCanonicalDepositSource(network.Pool())
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
	setWeb3DepositAuditExtra(c, deposit.ID, deposit.Status, web3deposit.DepositStatusReadyToCredit, "")
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
	deposit, err := h.deposits.GetAdminDeposit(c.Request.Context(), id)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	if err := h.operator.IgnoreReviewedDeposit(c.Request.Context(), id, input.Reason); err != nil {
		response.ErrorFrom(c, adminDepositOperationError(err))
		return
	}
	setWeb3DepositAuditExtra(c, deposit.ID, deposit.Status, web3deposit.DepositStatusIgnored, input.Reason)
	response.Success(c, gin.H{"status": web3deposit.DepositStatusIgnored})
}

func (h *Web3DepositHandler) Retry(c *gin.Context) {
	id, ok := adminDepositID(c)
	if !ok {
		return
	}
	deposit, err := h.deposits.GetAdminDeposit(c.Request.Context(), id)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	if err := h.operator.RetryFailedDeposit(c.Request.Context(), id); err != nil {
		response.ErrorFrom(c, adminDepositOperationError(err))
		return
	}
	setWeb3DepositAuditExtra(c, deposit.ID, deposit.Status, web3deposit.DepositStatusReadyToCredit, "")
	response.Success(c, gin.H{"status": web3deposit.DepositStatusReadyToCredit})
}

func (h *Web3DepositHandler) Rescan(c *gin.Context) {
	var input struct {
		NetworkKey string `json:"network_key"`
		AssetKey   string `json:"asset_key"`
		FromBlock  string `json:"from_block"`
		ToBlock    string `json:"to_block"`
	}
	if err := c.ShouldBindJSON(&input); err != nil || strings.TrimSpace(input.NetworkKey) == "" || strings.TrimSpace(input.AssetKey) == "" {
		response.BadRequest(c, "Invalid rescan request")
		return
	}
	fromBlock, err1 := strconv.ParseUint(input.FromBlock, 10, 64)
	toBlock, err2 := strconv.ParseUint(input.ToBlock, 10, 64)
	if err1 != nil || err2 != nil {
		response.BadRequest(c, "Invalid block range")
		return
	}
	if h.rescanJobs == nil {
		response.ErrorFrom(c, infraerrors.ServiceUnavailable("WEB3_DEPOSIT_UNAVAILABLE", "web3 deposit rescan is unavailable"))
		return
	}
	job, err := h.rescanJobs.Enqueue(c.Request.Context(), strings.TrimSpace(input.NetworkKey), strings.TrimSpace(input.AssetKey), fromBlock, toBlock, getAdminIDFromContext(c))
	if errors.Is(err, web3deposit.ErrRescanJobUnavailable) {
		response.ErrorFrom(c, infraerrors.ServiceUnavailable("WEB3_DEPOSIT_UNAVAILABLE", "web3 deposit rescan is unavailable"))
		return
	}
	if err != nil {
		response.ErrorFrom(c, web3DepositRescanJobError(err))
		return
	}
	servermiddleware.SetAuditExtra(c, map[string]any{
		"job_id":      job.ID,
		"network_key": strings.TrimSpace(input.NetworkKey),
		"asset_key":   strings.TrimSpace(input.AssetKey),
		"from_block":  fromBlock,
		"to_block":    toBlock,
		"result":      "queued",
	})
	response.Success(c, web3RescanJobFromDomain(job))
}

func (h *Web3DepositHandler) ListRescanJobs(c *gin.Context) {
	if h.rescanJobs == nil {
		response.ErrorFrom(c, infraerrors.ServiceUnavailable("WEB3_DEPOSIT_UNAVAILABLE", "web3 deposit rescan is unavailable"))
		return
	}
	limit, _ := strconv.Atoi(c.DefaultQuery("limit", "20"))
	jobs, err := h.rescanJobs.List(c.Request.Context(), limit)
	if err != nil {
		response.ErrorFrom(c, infraerrors.InternalServer("WEB3_DEPOSIT_RESCAN_JOB_LIST_FAILED", "failed to list web3 deposit rescan jobs").WithCause(err))
		return
	}
	items := make([]web3RescanJobResponse, 0, len(jobs))
	for _, job := range jobs {
		items = append(items, web3RescanJobFromDomain(job))
	}
	response.Success(c, items)
}

func (h *Web3DepositHandler) GetRescanJob(c *gin.Context) {
	if h.rescanJobs == nil {
		response.ErrorFrom(c, infraerrors.ServiceUnavailable("WEB3_DEPOSIT_UNAVAILABLE", "web3 deposit rescan is unavailable"))
		return
	}
	id, err := strconv.ParseInt(c.Param("jobId"), 10, 64)
	if err != nil || id <= 0 {
		response.BadRequest(c, "Invalid rescan job id")
		return
	}
	job, err := h.rescanJobs.Get(c.Request.Context(), id)
	if errors.Is(err, web3deposit.ErrRescanJobNotFound) {
		response.ErrorFrom(c, infraerrors.NotFound("WEB3_DEPOSIT_RESCAN_JOB_NOT_FOUND", "web3 deposit rescan job not found"))
		return
	}
	if err != nil {
		response.ErrorFrom(c, infraerrors.InternalServer("WEB3_DEPOSIT_RESCAN_JOB_GET_FAILED", "failed to load web3 deposit rescan job").WithCause(err))
		return
	}
	response.Success(c, web3RescanJobFromDomain(job))
}

type web3RescanJobResponse struct {
	ID           int64                       `json:"id"`
	NetworkKey   string                      `json:"network_key"`
	AssetKey     string                      `json:"asset_key"`
	FromBlock    string                      `json:"from_block"`
	ToBlock      string                      `json:"to_block"`
	Status       web3deposit.RescanJobStatus `json:"status"`
	RequestedBy  int64                       `json:"requested_by"`
	AttemptCount int                         `json:"attempt_count"`
	EventCount   int                         `json:"event_count"`
	MatchedCount int                         `json:"matched_count"`
	DepositCount int                         `json:"deposit_count"`
	ErrorMessage string                      `json:"error_message,omitempty"`
	StartedAt    *time.Time                  `json:"started_at,omitempty"`
	CompletedAt  *time.Time                  `json:"completed_at,omitempty"`
	CreatedAt    time.Time                   `json:"created_at"`
	UpdatedAt    time.Time                   `json:"updated_at"`
}

func web3RescanJobFromDomain(job web3deposit.RescanJob) web3RescanJobResponse {
	return web3RescanJobResponse{
		ID: job.ID, NetworkKey: job.NetworkKey, AssetKey: job.AssetKey,
		FromBlock: strconv.FormatUint(job.FromBlock, 10), ToBlock: strconv.FormatUint(job.ToBlock, 10),
		Status: job.Status, RequestedBy: job.RequestedBy, AttemptCount: job.AttemptCount,
		EventCount: job.EventCount, MatchedCount: job.MatchedCount, DepositCount: job.DepositCount,
		ErrorMessage: job.ErrorMessage, StartedAt: job.StartedAt, CompletedAt: job.CompletedAt,
		CreatedAt: job.CreatedAt, UpdatedAt: job.UpdatedAt,
	}
}

func web3DepositRescanJobError(err error) error {
	if errors.Is(err, web3deposit.ErrRescanJobCreateFailed) {
		return infraerrors.InternalServer("WEB3_DEPOSIT_RESCAN_JOB_CREATE_FAILED", "failed to create web3 deposit rescan job").WithCause(err)
	}
	for _, candidate := range []error{
		web3deposit.ErrRescanRangeInvalid,
		web3deposit.ErrRescanRangeTooLarge,
		web3deposit.ErrRescanBeforeScanStartBlock,
		web3deposit.ErrRescanBeyondFinalizedBlock,
		web3deposit.ErrRescanTargetUnavailable,
	} {
		if errors.Is(err, candidate) {
			return infraerrors.BadRequest("WEB3_DEPOSIT_RESCAN_INVALID", err.Error())
		}
	}
	return infraerrors.ServiceUnavailable("WEB3_DEPOSIT_RESCAN_VALIDATION_FAILED", "failed to validate web3 deposit rescan range").WithCause(err)
}

func setWeb3DepositAuditExtra(c *gin.Context, depositID int64, oldStatus, newStatus web3deposit.DepositStatus, reason string) {
	fields := map[string]any{
		"deposit_id": depositID,
		"old_status": string(oldStatus),
		"new_status": string(newStatus),
		"result":     "success",
	}
	if strings.TrimSpace(reason) != "" {
		fields["reason"] = reason
	}
	servermiddleware.SetAuditExtra(c, fields)
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
