package admin

import (
	"net/http"
	"strconv"

	"github.com/Wei-Shaw/sub2api/internal/pkg/response"
	"github.com/gin-gonic/gin"
)

// GetQuotaProviders returns provider metadata used by the shared account form.
func (h *AccountHandler) GetQuotaProviders(c *gin.Context) {
	if h.accountQuotaService == nil {
		response.Error(c, http.StatusServiceUnavailable, "Account quota service is unavailable")
		return
	}
	response.Success(c, h.accountQuotaService.Providers())
}

// GetQuota returns normalized manual, OAuth or upstream-key quota information.
// GET /api/v1/admin/accounts/:id/quota?source=passive|active&force=true
func (h *AccountHandler) GetQuota(c *gin.Context) {
	if h.accountQuotaService == nil {
		response.Error(c, http.StatusServiceUnavailable, "Account quota service is unavailable")
		return
	}
	accountID, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		response.BadRequest(c, "Invalid account ID")
		return
	}
	source := c.DefaultQuery("source", "active")
	if source != "active" && source != "passive" {
		response.BadRequest(c, "Invalid quota source")
		return
	}
	quota, err := h.accountQuotaService.GetQuota(c.Request.Context(), accountID, source, c.Query("force") == "true")
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, quota)
}
