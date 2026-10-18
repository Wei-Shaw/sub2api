package admin

import (
	"github.com/Wei-Shaw/sub2api/internal/pkg/qualitytrace"
	"github.com/Wei-Shaw/sub2api/internal/pkg/response"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
	"net/http"
	"strconv"
)

type AccountQualityHandler struct {
	svc      *service.AccountQualityService
	accounts *AccountHandler
}

func NewAccountQualityHandler(svc *service.AccountQualityService) *AccountQualityHandler {
	return &AccountQualityHandler{svc: svc}
}
func (h *AccountQualityHandler) SetAccountHandler(accounts *AccountHandler) {
	h.accounts = accounts
}

func qualityID(c *gin.Context, key string) (int64, bool) {
	id, err := strconv.ParseInt(c.Param(key), 10, 64)
	if err != nil || id < 0 {
		response.BadRequest(c, "invalid identifier")
		return 0, false
	}
	return id, true
}
func (h *AccountQualityHandler) Groups(c *gin.Context) {
	v, err := h.svc.Groups(c.Request.Context())
	if err != nil {
		response.InternalError(c, "Unable to load account groups")
		return
	}
	response.Success(c, v)
}
func (h *AccountQualityHandler) Page(c *gin.Context) {
	id, ok := qualityID(c, "groupID")
	if !ok {
		return
	}
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	size, _ := strconv.Atoi(c.DefaultQuery("page_size", "10"))
	v, err := h.svc.Page(c.Request.Context(), id, page, size, c.Query("search"), c.Query("platform"), c.Query("status"), c.Query("sort"))
	if err != nil {
		response.InternalError(c, "Unable to load account observations")
		return
	}
	response.Success(c, h.pageWithCapacity(c.Request.Context(), v))
}
func (h *AccountQualityHandler) Save(c *gin.Context) {
	id, ok := qualityID(c, "groupID")
	if !ok {
		return
	}
	p := service.DefaultAccountQualityPolicy()
	if err := c.ShouldBindJSON(&p); err != nil {
		response.BadRequest(c, "Invalid policy")
		return
	}
	if err := h.svc.SavePolicy(c.Request.Context(), id, p); err != nil {
		response.BadRequest(c, err.Error())
		return
	}
	response.Success(c, p)
}
func (h *AccountQualityHandler) Run(c *gin.Context) {
	id, ok := qualityID(c, "groupID")
	if !ok {
		return
	}
	var req struct {
		AccountID int64  `json:"account_id"`
		Benchmark string `json:"benchmark"`
	}
	if err := c.ShouldBindJSON(&req); err != nil || req.AccountID < 0 {
		response.BadRequest(c, "Invalid inspection request")
		return
	}
	v, err := h.svc.RunBenchmark(c.Request.Context(), id, req.AccountID, req.Benchmark)
	if err != nil {
		response.Error(c, http.StatusConflict, err.Error())
		return
	}
	response.Accepted(c, v)
}
func (h *AccountQualityHandler) Runs(c *gin.Context) {
	id, ok := qualityID(c, "groupID")
	if !ok {
		return
	}
	v, err := h.svc.Runs(c.Request.Context(), id)
	if err != nil {
		response.InternalError(c, "Unable to load runs")
		return
	}
	response.Success(c, v)
}
func (h *AccountQualityHandler) Results(c *gin.Context) {
	id, ok := qualityID(c, "groupID")
	if !ok {
		return
	}
	a, _ := strconv.ParseInt(c.DefaultQuery("account_id", "0"), 10, 64)
	v, err := h.svc.Results(c.Request.Context(), id, a)
	if err != nil {
		response.InternalError(c, "Unable to load results")
		return
	}
	response.Success(c, v)
}
func (h *AccountQualityHandler) Cancel(c *gin.Context) {
	id, ok := qualityID(c, "runID")
	if !ok {
		return
	}
	if err := h.svc.Cancel(c.Request.Context(), id); err != nil {
		response.InternalError(c, "Unable to cancel run")
		return
	}
	response.Success(c, gin.H{"cancelled": true})
}

func (h *AccountQualityHandler) Catalog(c *gin.Context) {
	response.Success(c, gin.H{"reference_models": qualitytrace.Models(), "bank_revision": qualitytrace.Revision(), "benchmarks": []string{"candy", "modeltrace", "context"}, "default_policy": service.DefaultAccountQualityPolicy()})
}

func (h *AccountQualityHandler) RoutingPreview(c *gin.Context) {
	id, ok := qualityID(c, "groupID")
	if !ok {
		return
	}
	scope := service.AccountQualityRequestScope{Model: c.Query("model"), Effort: c.Query("effort"), ServiceTier: c.Query("service_tier"), Endpoint: c.Query("endpoint")}
	v, err := h.svc.RoutingPreview(c.Request.Context(), id, scope)
	if err != nil {
		response.InternalError(c, "Unable to preview routing")
		return
	}
	response.Success(c, v)
}
func (h *AccountQualityHandler) RoutingEvents(c *gin.Context) {
	id, ok := qualityID(c, "groupID")
	if !ok {
		return
	}
	response.Success(c, h.svc.RoutingEvents(id, c.Query("request_id")))
}
