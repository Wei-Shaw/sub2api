package handler

import (
	"strconv"

	"github.com/Wei-Shaw/sub2api/internal/pkg/response"
	middleware2 "github.com/Wei-Shaw/sub2api/internal/server/middleware"
	"github.com/Wei-Shaw/sub2api/internal/service"

	"github.com/gin-gonic/gin"
)

// ListInvoiceProfiles returns invoice title profiles for the authenticated user.
// GET /api/v1/invoice/profiles
func (h *PaymentHandler) ListInvoiceProfiles(c *gin.Context) {
	subject, ok := requireAuth(c)
	if !ok {
		return
	}
	profiles, err := h.paymentService.ListInvoiceProfiles(c.Request.Context(), subject.UserID)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, profiles)
}

// CreateInvoiceProfile creates an invoice title profile for the authenticated user.
// POST /api/v1/invoice/profiles
func (h *PaymentHandler) CreateInvoiceProfile(c *gin.Context) {
	subject, ok := requireAuth(c)
	if !ok {
		return
	}
	var req service.InvoiceProfileInput
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "Invalid request: "+err.Error())
		return
	}
	profile, err := h.paymentService.CreateInvoiceProfile(c.Request.Context(), subject.UserID, req)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, profile)
}

// UpdateInvoiceProfile updates an invoice title profile owned by the authenticated user.
// PUT /api/v1/invoice/profiles/:id
func (h *PaymentHandler) UpdateInvoiceProfile(c *gin.Context) {
	subject, profileID, ok := h.invoiceProfileSubjectAndID(c)
	if !ok {
		return
	}
	var req service.InvoiceProfileInput
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "Invalid request: "+err.Error())
		return
	}
	profile, err := h.paymentService.UpdateInvoiceProfile(c.Request.Context(), subject.UserID, profileID, req)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, profile)
}

// DeleteInvoiceProfile deletes an invoice title profile owned by the authenticated user.
// DELETE /api/v1/invoice/profiles/:id
func (h *PaymentHandler) DeleteInvoiceProfile(c *gin.Context) {
	subject, profileID, ok := h.invoiceProfileSubjectAndID(c)
	if !ok {
		return
	}
	if err := h.paymentService.DeleteInvoiceProfile(c.Request.Context(), subject.UserID, profileID); err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, gin.H{"message": "invoice profile deleted"})
}

// SetDefaultInvoiceProfile marks an invoice title profile as the user's default.
// PUT /api/v1/invoice/profiles/:id/default
func (h *PaymentHandler) SetDefaultInvoiceProfile(c *gin.Context) {
	subject, profileID, ok := h.invoiceProfileSubjectAndID(c)
	if !ok {
		return
	}
	profile, err := h.paymentService.SetDefaultInvoiceProfile(c.Request.Context(), subject.UserID, profileID)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, profile)
}

// ListInvoiceableOrders returns paid orders that can still be submitted for invoicing.
// GET /api/v1/invoice/orders
func (h *PaymentHandler) ListInvoiceableOrders(c *gin.Context) {
	subject, ok := requireAuth(c)
	if !ok {
		return
	}
	page, pageSize := response.ParsePagination(c)
	orders, total, err := h.paymentService.ListInvoiceableOrders(c.Request.Context(), subject.UserID, page, pageSize)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Paginated(c, orders, int64(total), page, pageSize)
}

// CreateInvoiceRequest submits an invoice request for one or more paid orders.
// POST /api/v1/invoice/requests
func (h *PaymentHandler) CreateInvoiceRequest(c *gin.Context) {
	subject, ok := requireAuth(c)
	if !ok {
		return
	}
	var req service.CreateInvoiceRequestInput
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "Invalid request: "+err.Error())
		return
	}
	invoiceRequest, err := h.paymentService.CreateInvoiceRequest(c.Request.Context(), subject.UserID, req)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, invoiceRequest)
}

// ListInvoiceRequests returns invoice requests submitted by the authenticated user.
// GET /api/v1/invoice/requests
func (h *PaymentHandler) ListInvoiceRequests(c *gin.Context) {
	subject, ok := requireAuth(c)
	if !ok {
		return
	}
	page, pageSize := response.ParsePagination(c)
	requests, total, err := h.paymentService.ListInvoiceRequests(c.Request.Context(), subject.UserID, service.InvoiceRequestListParams{
		Page:      page,
		PageSize:  pageSize,
		Status:    c.Query("status"),
		StartTime: c.Query("start_time"),
		EndTime:   c.Query("end_time"),
	})
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Paginated(c, requests, int64(total), page, pageSize)
}

func (h *PaymentHandler) invoiceProfileSubjectAndID(c *gin.Context) (subject middleware2.AuthSubject, profileID int64, ok bool) {
	authSubject, ok := requireAuth(c)
	if !ok {
		return middleware2.AuthSubject{}, 0, false
	}
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil || id <= 0 {
		response.BadRequest(c, "Invalid invoice profile ID")
		return middleware2.AuthSubject{}, 0, false
	}
	return authSubject, id, true
}
