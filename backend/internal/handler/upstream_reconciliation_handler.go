package handler

import (
	"context"
	"net/http"
	"strconv"

	"github.com/Wei-Shaw/sub2api/internal/domain"
	"github.com/Wei-Shaw/sub2api/internal/pkg/response"
	"github.com/Wei-Shaw/sub2api/internal/server/middleware"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
)

type upstreamReconciliation interface {
	Connections(context.Context) ([]service.BillingConnection, error)
	Accounts(context.Context) ([]service.BillingAccountOption, error)
	SaveConnection(context.Context, int64, int64, service.SaveBillingConnectionInput) (*service.BillingConnection, error)
	Sync(context.Context, int64, string) error
	Report(context.Context, string, int64, bool) (*service.BillingReport, error)
	Summary(context.Context, string, int64, bool) (*service.BillingReport, error)
	RawBillSource(context.Context, int64) (*service.BillingBillSource, error)
	AdminBills(context.Context, service.AdminBillingListInput) (*service.AdminBillingList, error)
	AdminBill(context.Context, int64, service.AdminBillingFilter) (*service.AdminBillingRecord, error)
}

// UpstreamReconciliationHandler deliberately keeps cloud billing credentials out
// of the regular account DTO and separates official costs from gateway charges.
type UpstreamReconciliationHandler struct {
	service upstreamReconciliation
}

func NewUpstreamReconciliationHandler(svc *service.UpstreamReconciliationService) *UpstreamReconciliationHandler {
	return &UpstreamReconciliationHandler{service: svc}
}

func reconciliationSubject(c *gin.Context, adminOnly bool) (middleware.AuthSubject, bool, bool) {
	c.Header("Cache-Control", "no-store")
	subject, ok := middleware.GetAuthSubjectFromContext(c)
	if !ok || subject.UserID <= 0 {
		response.Unauthorized(c, "User not authenticated")
		return subject, false, false
	}
	role, _ := middleware.GetUserRoleFromContext(c)
	admin := role == domain.RoleAdmin
	if adminOnly && !admin {
		response.Forbidden(c, "Only administrators can manage upstream billing connections")
		return subject, false, false
	}
	return subject, admin, true
}

func (h *UpstreamReconciliationHandler) Connections(c *gin.Context) {
	if _, _, ok := reconciliationSubject(c, true); !ok {
		return
	}
	rows, err := h.service.Connections(c.Request.Context())
	if !response.ErrorFrom(c, err) {
		response.Success(c, rows)
	}
}

func (h *UpstreamReconciliationHandler) Accounts(c *gin.Context) {
	if _, _, ok := reconciliationSubject(c, true); !ok {
		return
	}
	rows, err := h.service.Accounts(c.Request.Context())
	if !response.ErrorFrom(c, err) {
		response.Success(c, rows)
	}
}

func reconciliationID(c *gin.Context) (int64, bool) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil || id <= 0 {
		response.BadRequest(c, "Invalid billing connection ID")
		return 0, false
	}
	return id, true
}

func (h *UpstreamReconciliationHandler) SaveConnection(c *gin.Context) {
	subject, _, ok := reconciliationSubject(c, true)
	if !ok {
		return
	}
	var id int64
	if c.Request.Method == http.MethodPut {
		id, ok = reconciliationID(c)
		if !ok {
			return
		}
	}
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, 128<<10)
	var input service.SaveBillingConnectionInput
	if err := c.ShouldBindJSON(&input); err != nil {
		// Binding errors may contain supplied credentials. Never echo them.
		response.BadRequest(c, "Invalid billing connection request")
		return
	}
	connection, err := h.service.SaveConnection(c.Request.Context(), id, subject.UserID, input)
	if !response.ErrorFrom(c, err) {
		response.Success(c, connection)
	}
}

func (h *UpstreamReconciliationHandler) Sync(c *gin.Context) {
	if _, _, ok := reconciliationSubject(c, true); !ok {
		return
	}
	id, ok := reconciliationID(c)
	if !ok {
		return
	}
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, 1024)
	var input struct {
		Month string `json:"month" binding:"required"`
	}
	if err := c.ShouldBindJSON(&input); err != nil {
		response.BadRequest(c, "A billing month in YYYY-MM format is required")
		return
	}
	if !response.ErrorFrom(c, h.service.Sync(c.Request.Context(), id, input.Month)) {
		response.Success(c, gin.H{"success": true})
	}
}

func (h *UpstreamReconciliationHandler) Report(c *gin.Context) {
	subject, admin, ok := reconciliationSubject(c, false)
	if !ok {
		return
	}
	// The viewer identity always comes from the authenticated session, never a
	// query parameter. Regular users cannot opt into the administrator report.
	report, err := h.service.Report(c.Request.Context(), c.Query("month"), subject.UserID, admin)
	if !response.ErrorFrom(c, err) {
		response.Success(c, report)
	}
}

func (h *UpstreamReconciliationHandler) Summary(c *gin.Context) {
	subject, admin, ok := reconciliationSubject(c, false)
	if !ok {
		return
	}
	report, err := h.service.Summary(c.Request.Context(), c.Query("month"), subject.UserID, admin)
	if !response.ErrorFrom(c, err) {
		response.Success(c, report)
	}
}

func (h *UpstreamReconciliationHandler) BillSource(c *gin.Context) {
	if _, _, ok := reconciliationSubject(c, true); !ok {
		return
	}
	id, ok := reconciliationID(c)
	if !ok {
		return
	}
	source, err := h.service.RawBillSource(c.Request.Context(), id)
	if !response.ErrorFrom(c, err) {
		response.Success(c, source)
	}
}

// These handlers are mounted under the existing administrator authentication
// group, which accepts the administrator API key as well as administrator JWTs.
// Ordinary model/virtual keys are never an authorization mechanism here.
func (h *UpstreamReconciliationHandler) AdminCapabilities(c *gin.Context) {
	if _, _, ok := reconciliationSubject(c, true); !ok {
		return
	}
	response.Success(c, service.GetAdminBillingCapabilities())
}

func adminBillingRequestFilter(c *gin.Context) (service.AdminBillingFilter, bool) {
	filter := service.AdminBillingFilter{Month: c.Query("month"), Provider: c.Query("provider"), ResourceID: c.Query("resource_id")}
	for _, name := range []string{"month", "provider", "resource_id", "connection_id", "limit", "cursor"} {
		if len(c.Request.URL.Query()[name]) > 1 {
			response.BadRequest(c, "Billing query parameters must not be repeated")
			return filter, false
		}
	}
	if filter.Month == "" {
		response.BadRequest(c, "month in YYYY-MM format is required")
		return filter, false
	}
	if value, exists := c.GetQuery("connection_id"); exists {
		id, err := strconv.ParseInt(value, 10, 64)
		if err != nil || id <= 0 {
			response.BadRequest(c, "connection_id must be a positive integer")
			return filter, false
		}
		filter.ConnectionID = id
	}
	return filter, true
}

func (h *UpstreamReconciliationHandler) AdminBills(c *gin.Context) {
	if _, _, ok := reconciliationSubject(c, true); !ok {
		return
	}
	filter, ok := adminBillingRequestFilter(c)
	if !ok {
		return
	}
	input := service.AdminBillingListInput{AdminBillingFilter: filter, Cursor: c.Query("cursor")}
	if value, exists := c.GetQuery("limit"); exists {
		limit, err := strconv.Atoi(value)
		if err != nil || limit < 1 || limit > service.AdminBillingMaxPageSize {
			response.BadRequest(c, "limit must be between 1 and 20")
			return
		}
		input.Limit = limit
	}
	result, err := h.service.AdminBills(c.Request.Context(), input)
	if !response.ErrorFrom(c, err) {
		response.Success(c, result)
	}
}

func (h *UpstreamReconciliationHandler) AdminBill(c *gin.Context) {
	if _, _, ok := reconciliationSubject(c, true); !ok {
		return
	}
	id, ok := reconciliationID(c)
	if !ok {
		return
	}
	filter, ok := adminBillingRequestFilter(c)
	if !ok {
		return
	}
	result, err := h.service.AdminBill(c.Request.Context(), id, filter)
	if !response.ErrorFrom(c, err) {
		response.Success(c, result)
	}
}
