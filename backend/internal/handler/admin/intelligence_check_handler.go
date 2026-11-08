package admin

import (
	"context"
	"database/sql"
	"errors"
	"log"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/response"
	"github.com/Wei-Shaw/sub2api/internal/server/middleware"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
)

// intelligenceCheckBackgroundTimeout 是后台跑测 goroutine 的兜底上界。
// 单次跑测的真实超时由 service 层按设置控制在更小的值。
const intelligenceCheckBackgroundTimeout = 10 * time.Minute

// IntelligenceCheckHandler 处理智力检测（鹈鹕测试）的管理端接口。
type IntelligenceCheckHandler struct {
	intelligenceCheckSvc *service.IntelligenceCheckService
}

// NewIntelligenceCheckHandler 构造智力检测 handler。
func NewIntelligenceCheckHandler(intelligenceCheckSvc *service.IntelligenceCheckService) *IntelligenceCheckHandler {
	return &IntelligenceCheckHandler{intelligenceCheckSvc: intelligenceCheckSvc}
}

type intelligenceCheckRunResponse struct {
	ID              int64      `json:"id"`
	AccountID       int64      `json:"account_id"`
	BatchID         string     `json:"batch_id"`
	TriggerSource   string     `json:"trigger_source"`
	ModelID         string     `json:"model_id"`
	UpstreamModel   string     `json:"upstream_model"`
	ReasoningEffort string     `json:"reasoning_effort"`
	PromptVariant   string     `json:"prompt_variant"`
	Status          string     `json:"status"`
	Verdict         string     `json:"verdict"`
	HasHTML         bool       `json:"has_html"`
	HTMLBytes       int        `json:"html_bytes"`
	LatencyMs       int64      `json:"latency_ms"`
	Attempt         int        `json:"attempt"`
	ErrorCode       string     `json:"error_code"`
	ErrorMessage    string     `json:"error_message"`
	StartedAt       time.Time  `json:"started_at"`
	FinishedAt      *time.Time `json:"finished_at"`
	CreatedAt       time.Time  `json:"created_at"`
	// ReviewedAt 为 null 即「待评审」；ReviewedBy 是评审的管理员用户 ID。
	ReviewedBy int64      `json:"reviewed_by"`
	ReviewedAt *time.Time `json:"reviewed_at"`
}

func newIntelligenceCheckRunResponse(run *service.IntelligenceCheckRun) intelligenceCheckRunResponse {
	if run == nil {
		return intelligenceCheckRunResponse{}
	}
	return intelligenceCheckRunResponse{
		ID:              run.ID,
		AccountID:       run.AccountID,
		BatchID:         run.BatchID,
		TriggerSource:   run.TriggerSource,
		ModelID:         run.ModelID,
		UpstreamModel:   run.UpstreamModel,
		ReasoningEffort: run.ReasoningEffort,
		PromptVariant:   run.PromptVariant,
		Status:          run.Status,
		Verdict:         run.Verdict,
		HasHTML:         run.HasHTML,
		HTMLBytes:       run.HTMLBytes,
		LatencyMs:       run.LatencyMs,
		Attempt:         run.Attempt,
		ErrorCode:       run.ErrorCode,
		ErrorMessage:    run.ErrorMessage,
		StartedAt:       run.StartedAt,
		FinishedAt:      run.FinishedAt,
		CreatedAt:       run.CreatedAt,
		ReviewedBy:      run.ReviewedBy,
		ReviewedAt:      run.ReviewedAt,
	}
}

type createIntelligenceCheckRunRequest struct {
	AccountID       int64  `json:"account_id" binding:"required"`
	ModelID         string `json:"model_id"`
	ReasoningEffort string `json:"reasoning_effort"`
	PromptVariant   string `json:"prompt_variant"`
}

// ListRuns GET /admin/intelligence-check/runs
func (h *IntelligenceCheckHandler) ListRuns(c *gin.Context) {
	page, pageSize := response.ParsePagination(c)

	filter := service.IntelligenceCheckRunFilter{
		Status:  strings.TrimSpace(c.Query("status")),
		Verdict: strings.TrimSpace(c.Query("verdict")),
		ModelID: strings.TrimSpace(c.Query("model_id")),
		Limit:   pageSize,
		Offset:  (page - 1) * pageSize,
	}
	if raw := strings.TrimSpace(c.Query("account_id")); raw != "" {
		accountID, err := strconv.ParseInt(raw, 10, 64)
		if err != nil || accountID <= 0 {
			response.BadRequest(c, "invalid account_id")
			return
		}
		filter.AccountIDs = []int64{accountID}
	}

	runs, total, err := h.intelligenceCheckSvc.ListRuns(c.Request.Context(), filter)
	if err != nil {
		response.InternalError(c, err.Error())
		return
	}

	items := make([]intelligenceCheckRunResponse, 0, len(runs))
	for _, run := range runs {
		items = append(items, newIntelligenceCheckRunResponse(run))
	}
	response.Paginated(c, items, total, page, pageSize)
}

// GetRun GET /admin/intelligence-check/runs/:id
func (h *IntelligenceCheckHandler) GetRun(c *gin.Context) {
	runID, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil || runID <= 0 {
		response.BadRequest(c, "invalid run id")
		return
	}

	run, err := h.intelligenceCheckSvc.GetRun(c.Request.Context(), runID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			response.NotFound(c, "run not found")
			return
		}
		response.InternalError(c, err.Error())
		return
	}
	response.Success(c, newIntelligenceCheckRunResponse(run))
}

// GetArtifact GET /admin/intelligence-check/runs/:id/artifact
// 以纯文本返回作品原文。作品是模型产出的不可信内容，因此：
//   - 用 text/plain 返回，浏览器不会把它当页面执行；
//   - 附带禁止脚本与外联的 CSP，前端再用 sandbox iframe 承载。
func (h *IntelligenceCheckHandler) GetArtifact(c *gin.Context) {
	runID, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil || runID <= 0 {
		response.BadRequest(c, "invalid run id")
		return
	}

	artifact, err := h.intelligenceCheckSvc.GetArtifact(c.Request.Context(), runID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			response.NotFound(c, "artifact not found")
			return
		}
		response.InternalError(c, err.Error())
		return
	}

	c.Header("Content-Type", "text/plain; charset=utf-8")
	c.Header("Content-Security-Policy", "default-src 'none'; style-src 'unsafe-inline'; img-src data:; script-src 'none'")
	c.Header("X-Content-Type-Options", "nosniff")
	c.String(http.StatusOK, artifact.HTMLText)
}

// CreateRun POST /admin/intelligence-check/runs
// 立即返回 queued 记录，跑测在后台执行，前端轮询列表获取状态。
func (h *IntelligenceCheckHandler) CreateRun(c *gin.Context) {
	var req createIntelligenceCheckRunRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, err.Error())
		return
	}
	if req.AccountID <= 0 {
		response.BadRequest(c, "account_id is required")
		return
	}

	runReq := service.IntelligenceCheckRequest{
		AccountID:       req.AccountID,
		ModelID:         strings.TrimSpace(req.ModelID),
		ReasoningEffort: strings.TrimSpace(req.ReasoningEffort),
		PromptVariant:   strings.TrimSpace(req.PromptVariant),
		TriggerSource:   service.IntelligenceCheckTriggerManual,
	}

	run, err := h.intelligenceCheckSvc.StartRun(c.Request.Context(), runReq)
	if err != nil {
		response.InternalError(c, err.Error())
		return
	}

	// 用独立 context：请求返回后原 context 会被取消，后台跑测必须与之解耦。
	go func(runID int64, request service.IntelligenceCheckRequest) {
		defer func() {
			if recovered := recover(); recovered != nil {
				log.Printf("intelligence check run %d panicked: %v", runID, recovered)
			}
		}()
		bg, cancel := context.WithTimeout(context.Background(), intelligenceCheckBackgroundTimeout)
		defer cancel()
		if err := h.intelligenceCheckSvc.ExecuteRun(bg, runID, request); err != nil {
			log.Printf("intelligence check run %d failed to execute: %v", runID, err)
		}
	}(run.ID, runReq)

	response.Success(c, newIntelligenceCheckRunResponse(run))
}

type reviewIntelligenceCheckRunRequest struct {
	Verdict string `json:"verdict" binding:"required"`
}

// intelligenceCheckStatusSyncResponse 描述评审联动账号状态的结果。
// Enabled=false 表示全局开关关闭（账号状态未被触碰）；Applied=false 表示无需变更或未联动。
type intelligenceCheckStatusSyncResponse struct {
	Enabled        bool   `json:"enabled"`
	Applied        bool   `json:"applied"`
	AccountID      int64  `json:"account_id"`
	PreviousStatus string `json:"previous_status"`
	AccountStatus  string `json:"account_status"`
}

type reviewIntelligenceCheckRunResponse struct {
	Run        intelligenceCheckRunResponse        `json:"run"`
	StatusSync intelligenceCheckStatusSyncResponse `json:"status_sync"`
}

// ReviewRun PATCH /admin/intelligence-check/runs/:id/review
// 人工评审一次跑测的结论（pass / fail），评审人取自当前登录管理员。
// 按全局开关决定是否联动账号状态：fail -> error，pass -> active。
func (h *IntelligenceCheckHandler) ReviewRun(c *gin.Context) {
	runID, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil || runID <= 0 {
		response.BadRequest(c, "invalid run id")
		return
	}

	var req reviewIntelligenceCheckRunRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, err.Error())
		return
	}

	// 评审人必须是已登录管理员。路由已挂在 admin 组下，这里再兜一层，
	// 确保「谁评的」永远可追溯，不会写出无主的评审记录。
	subject, ok := middleware.GetAuthSubjectFromContext(c)
	if !ok || subject.UserID <= 0 {
		response.Unauthorized(c, "Unauthorized")
		return
	}

	run, sync, err := h.intelligenceCheckSvc.ReviewRun(c.Request.Context(), runID, req.Verdict, subject.UserID)
	if err != nil {
		switch {
		case errors.Is(err, sql.ErrNoRows):
			response.NotFound(c, "run not found")
		case errors.Is(err, service.ErrIntelligenceCheckInvalidVerdict),
			errors.Is(err, service.ErrIntelligenceCheckNotReviewable):
			response.ErrorFrom(c, err)
		default:
			response.InternalError(c, err.Error())
		}
		return
	}

	out := reviewIntelligenceCheckRunResponse{Run: newIntelligenceCheckRunResponse(run)}
	if sync != nil {
		out.StatusSync = intelligenceCheckStatusSyncResponse{
			Enabled:        sync.Enabled,
			Applied:        sync.Applied,
			AccountID:      sync.AccountID,
			PreviousStatus: sync.PreviousStatus,
			AccountStatus:  sync.CurrentStatus,
		}
	}
	response.Success(c, out)
}
