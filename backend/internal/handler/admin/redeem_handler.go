package admin

import (
	"bytes"
	"encoding/csv"
	"fmt"
	"strconv"

	"github.com/Wei-Shaw/sub2api/internal/pkg/response"
	"github.com/Wei-Shaw/sub2api/internal/service"

	"github.com/gin-gonic/gin"
)

// RedeemHandler handles admin redeem code management
type RedeemHandler struct {
	adminService service.AdminService
}

// NewRedeemHandler creates a new admin redeem handler
func NewRedeemHandler(adminService service.AdminService) *RedeemHandler {
	return &RedeemHandler{
		adminService: adminService,
	}
}

// GenerateRedeemCodesRequest represents generate redeem codes request
type GenerateRedeemCodesRequest struct {
	Count        int     `json:"count" binding:"required,min=1,max=100"`
	Type         string  `json:"type" binding:"required,oneof=balance concurrency subscription"`
	Value        float64 `json:"value" binding:"min=0"`
	GroupID      *int64  `json:"group_id"`      // 订阅类型必填
	ValidityDays int     `json:"validity_days"` // 订阅类型使用，默认30天
}

// List handles listing all redeem codes with pagination
// GET /api/v1/admin/redeem-codes
func (h *RedeemHandler) List(c *gin.Context) {
	page, pageSize := response.ParsePagination(c)
	codeType := c.Query("type")
	status := c.Query("status")
	search := c.Query("search")

	codes, total, err := h.adminService.ListRedeemCodes(c.Request.Context(), page, pageSize, codeType, status, search)
	if err != nil {
		response.InternalError(c, "Failed to list redeem codes: "+err.Error())
		return
	}

	response.Paginated(c, codes, total, page, pageSize)
}

// GetByID handles getting a redeem code by ID
// GET /api/v1/admin/redeem-codes/:id
func (h *RedeemHandler) GetByID(c *gin.Context) {
	codeID, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		response.BadRequest(c, "Invalid redeem code ID")
		return
	}

	code, err := h.adminService.GetRedeemCode(c.Request.Context(), codeID)
	if err != nil {
		response.NotFound(c, "Redeem code not found")
		return
	}

	response.Success(c, code)
}

// Generate handles generating new redeem codes
// POST /api/v1/admin/redeem-codes/generate
func (h *RedeemHandler) Generate(c *gin.Context) {
	var req GenerateRedeemCodesRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "Invalid request: "+err.Error())
		return
	}

	codes, err := h.adminService.GenerateRedeemCodes(c.Request.Context(), &service.GenerateRedeemCodesInput{
		Count:        req.Count,
		Type:         req.Type,
		Value:        req.Value,
		GroupID:      req.GroupID,
		ValidityDays: req.ValidityDays,
	})
	if err != nil {
		response.InternalError(c, "Failed to generate redeem codes: "+err.Error())
		return
	}

	response.Success(c, codes)
}

// Delete handles deleting a redeem code
// DELETE /api/v1/admin/redeem-codes/:id
func (h *RedeemHandler) Delete(c *gin.Context) {
	codeID, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		response.BadRequest(c, "Invalid redeem code ID")
		return
	}

	err = h.adminService.DeleteRedeemCode(c.Request.Context(), codeID)
	if err != nil {
		response.InternalError(c, "Failed to delete redeem code: "+err.Error())
		return
	}

	response.Success(c, gin.H{"message": "Redeem code deleted successfully"})
}

// BatchDelete handles batch deleting redeem codes
// POST /api/v1/admin/redeem-codes/batch-delete
func (h *RedeemHandler) BatchDelete(c *gin.Context) {
	var req struct {
		IDs []int64 `json:"ids" binding:"required,min=1"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "Invalid request: "+err.Error())
		return
	}

	deleted, err := h.adminService.BatchDeleteRedeemCodes(c.Request.Context(), req.IDs)
	if err != nil {
		response.InternalError(c, "Failed to batch delete redeem codes: "+err.Error())
		return
	}

	response.Success(c, gin.H{
		"deleted": deleted,
		"message": "Redeem codes deleted successfully",
	})
}

// Expire handles expiring a redeem code
// POST /api/v1/admin/redeem-codes/:id/expire
func (h *RedeemHandler) Expire(c *gin.Context) {
	codeID, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		response.BadRequest(c, "Invalid redeem code ID")
		return
	}

	code, err := h.adminService.ExpireRedeemCode(c.Request.Context(), codeID)
	if err != nil {
		response.InternalError(c, "Failed to expire redeem code: "+err.Error())
		return
	}

	response.Success(c, code)
}

// GetStats handles getting redeem code statistics
// GET /api/v1/admin/redeem-codes/stats
func (h *RedeemHandler) GetStats(c *gin.Context) {
	// Return mock data for now
	response.Success(c, gin.H{
		"total_codes":             0,
		"active_codes":            0,
		"used_codes":              0,
		"expired_codes":           0,
		"total_value_distributed": 0.0,
		"by_type": gin.H{
			"balance":     0,
			"concurrency": 0,
			"trial":       0,
		},
	})
}

// Export handles exporting redeem codes to CSV
// GET /api/v1/admin/redeem-codes/export
func (h *RedeemHandler) Export(c *gin.Context) {
	codeType := c.Query("type")
	status := c.Query("status")

	// Get all codes without pagination (use large page size)
	codes, _, err := h.adminService.ListRedeemCodes(c.Request.Context(), 1, 10000, codeType, status, "")
	if err != nil {
		response.InternalError(c, "Failed to export redeem codes: "+err.Error())
		return
	}

	// Create CSV buffer
	var buf bytes.Buffer
	writer := csv.NewWriter(&buf)

	// Write header
	if err := writer.Write([]string{"id", "code", "type", "value", "status", "used_by", "used_at", "created_at"}); err != nil {
		response.InternalError(c, "Failed to export redeem codes: "+err.Error())
		return
	}

	// Write data rows
	for _, code := range codes {
		usedBy := ""
		if code.UsedBy != nil {
			usedBy = fmt.Sprintf("%d", *code.UsedBy)
		}
		usedAt := ""
		if code.UsedAt != nil {
			usedAt = code.UsedAt.Format("2006-01-02 15:04:05")
		}
		if err := writer.Write([]string{
			fmt.Sprintf("%d", code.ID),
			code.Code,
			code.Type,
			fmt.Sprintf("%.2f", code.Value),
			code.Status,
			usedBy,
			usedAt,
			code.CreatedAt.Format("2006-01-02 15:04:05"),
		}); err != nil {
			response.InternalError(c, "Failed to export redeem codes: "+err.Error())
			return
		}
	}

	writer.Flush()
	if err := writer.Error(); err != nil {
		response.InternalError(c, "Failed to export redeem codes: "+err.Error())
		return
	}

	c.Header("Content-Type", "text/csv")
	c.Header("Content-Disposition", "attachment; filename=redeem_codes.csv")
	c.Data(200, "text/csv", buf.Bytes())
}
