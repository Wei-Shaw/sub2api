package admin

import (
	"strconv"

	"github.com/Wei-Shaw/sub2api/internal/pkg/response"
	"github.com/gin-gonic/gin"
)

// GetClientExportOptions lists group-scoped native template model choices.
// The admin middleware protects this route; it never returns upstream secrets.
func (h *GroupHandler) GetClientExportOptions(c *gin.Context) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil || id < 0 {
		response.BadRequest(c, "Invalid group ID")
		return
	}
	options, err := h.adminService.GetGroupClientExportOptions(c.Request.Context(), id, c.Query("platform"))
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, options)
}
