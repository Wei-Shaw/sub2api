package admin

import (
	"errors"
	"net/http"
	"strconv"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/response"
	"github.com/Wei-Shaw/sub2api/internal/server/middleware"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
)

type CostCenterHandler struct{ svc *service.CostCenterService }

func NewCostCenterHandler(svc *service.CostCenterService) *CostCenterHandler {
	return &CostCenterHandler{svc: svc}
}

func (h *CostCenterHandler) Service() *service.CostCenterService { return h.svc }

func parseCostCenterFilter(c *gin.Context) (service.CostCenterReportFilter, error) {
	start, err := time.Parse(time.RFC3339, c.Query("start"))
	if err != nil {
		return service.CostCenterReportFilter{}, err
	}
	end, err := time.Parse(time.RFC3339, c.Query("end"))
	if err != nil {
		return service.CostCenterReportFilter{}, err
	}
	if !end.After(start) {
		return service.CostCenterReportFilter{}, errors.New("end must be after start")
	}
	f := service.CostCenterReportFilter{Start: start, End: end, Category: c.Query("category"), SourceType: c.Query("source_type")}
	f.Platform = c.Query("platform")
	f.Model = c.Query("model")
	if v := c.Query("account_id"); v != "" {
		id, e := strconv.ParseInt(v, 10, 64)
		if e != nil {
			return f, e
		}
		f.AccountID = &id
	}
	for key, dst := range map[string]**int64{"user_id": &f.UserID, "group_id": &f.GroupID, "plan_id": &f.PlanID} {
		if v := c.Query(key); v != "" {
			id, e := strconv.ParseInt(v, 10, 64)
			if e != nil {
				return f, e
			}
			*dst = &id
		}
	}
	return f, nil
}
func (h *CostCenterHandler) Summary(c *gin.Context) {
	f, err := parseCostCenterFilter(c)
	if err != nil {
		response.Error(c, http.StatusBadRequest, "invalid start/end range")
		return
	}
	v, err := h.svc.Summarize(c.Request.Context(), f)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, v)
}
func (h *CostCenterHandler) Events(c *gin.Context) {
	f, err := parseCostCenterFilter(c)
	if err != nil {
		response.Error(c, http.StatusBadRequest, "invalid start/end range")
		return
	}
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	size, _ := strconv.Atoi(c.DefaultQuery("page_size", "50"))
	items, total, err := h.svc.ListEvents(c.Request.Context(), f, page, size)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Paginated(c, items, total, page, size)
}
func (h *CostCenterHandler) CreateExpense(c *gin.Context) {
	var in service.CreateCostCenterEventInput
	if err := c.ShouldBindJSON(&in); err != nil {
		response.Error(c, http.StatusBadRequest, err.Error())
		return
	}
	in.EventType = service.CostEventExpense
	if subject, ok := middleware.GetAuthSubjectFromContext(c); ok {
		in.OperatorID = &subject.UserID
	}
	v, err := h.svc.CreateEvent(c.Request.Context(), &in)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Created(c, v)
}

func (h *CostCenterHandler) CreateExpensePlan(c *gin.Context) {
	var in service.CreateExpensePlanInput
	if err := c.ShouldBindJSON(&in); err != nil {
		response.Error(c, http.StatusBadRequest, err.Error())
		return
	}
	if subject, ok := middleware.GetAuthSubjectFromContext(c); ok {
		in.OperatorID = &subject.UserID
	}
	v, err := h.svc.CreateExpensePlan(c.Request.Context(), &in)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Created(c, v)
}

func (h *CostCenterHandler) MaterializeExpensePlans(c *gin.Context) {
	n, err := h.svc.MaterializeExpensePlans(c.Request.Context(), time.Now().UTC())
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, gin.H{"materialized": n})
}

type costCenterEventActionRequest struct {
	Status string `json:"status"`
	Reason string `json:"reason"`
}

func (h *CostCenterHandler) UpdateEventStatus(c *gin.Context) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		response.Error(c, http.StatusBadRequest, "invalid event id")
		return
	}
	var req costCenterEventActionRequest
	if err := c.ShouldBindJSON(&req); err != nil || req.Reason == "" {
		response.Error(c, http.StatusBadRequest, "status and reason are required")
		return
	}
	var op *int64
	if subject, ok := middleware.GetAuthSubjectFromContext(c); ok {
		op = &subject.UserID
	}
	v, err := h.svc.UpdateEventStatus(c.Request.Context(), id, req.Status, req.Reason, op)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, v)
}
func (h *CostCenterHandler) ReverseEvent(c *gin.Context) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		response.Error(c, http.StatusBadRequest, "invalid event id")
		return
	}
	var req struct {
		Reason string `json:"reason"`
	}
	if err := c.ShouldBindJSON(&req); err != nil || req.Reason == "" {
		response.Error(c, http.StatusBadRequest, "reason is required")
		return
	}
	var op *int64
	if subject, ok := middleware.GetAuthSubjectFromContext(c); ok {
		op = &subject.UserID
	}
	v, err := h.svc.ReverseEvent(c.Request.Context(), id, req.Reason, op)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Created(c, v)
}
func (h *CostCenterHandler) Reconcile(c *gin.Context) {
	f, err := parseCostCenterFilter(c)
	if err != nil {
		response.Error(c, http.StatusBadRequest, "invalid start/end range")
		return
	}
	v, err := h.svc.Reconcile(c.Request.Context(), f)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, v)
}
