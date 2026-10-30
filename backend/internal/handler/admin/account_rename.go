package admin

import (
	"context"

	"github.com/Wei-Shaw/sub2api/internal/pkg/response"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
)

// PreviewAccountRenames resolves a fixed selection and stable numbering without writing.
func (h *AccountHandler) PreviewAccountRenames(c *gin.Context) {
	var req service.BulkAccountRenameInput
	if err := c.ShouldBindJSON(&req); err != nil {
		response.ErrorFrom(c, service.ErrAccountRenameInvalid)
		return
	}
	result, err := h.adminService.PreviewAccountRenames(c.Request.Context(), &req)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, result)
}

func (h *AccountHandler) ApplyAccountRenames(c *gin.Context) {
	var req struct {
		Items []service.AccountRenameItem `json:"items"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		response.ErrorFrom(c, service.ErrAccountRenameInvalid)
		return
	}
	executeAdminIdempotentJSON(c, "admin.accounts.bulk_rename", req, service.DefaultWriteIdempotencyTTL(), func(ctx context.Context) (any, error) { return h.adminService.ApplyAccountRenames(ctx, req.Items) })
}
