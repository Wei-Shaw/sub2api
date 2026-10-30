package admin

import (
	"strconv"

	"github.com/Wei-Shaw/sub2api/internal/handler/dto"
	"github.com/Wei-Shaw/sub2api/internal/pkg/response"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
)

// Incremental exports all retained usage in ingestion order, independent of request time.
func (h *UsageHandler) Incremental(c *gin.Context) {
	for name := range c.Request.URL.Query() {
		if name != "cursor" && name != "limit" {
			response.BadRequest(c, "Unsupported incremental usage parameter: "+name)
			return
		}
	}
	cursor, err := service.ParseUsageCursor(c.Query("cursor"))
	if err != nil {
		response.BadRequest(c, "Invalid cursor")
		return
	}
	limit, err := strconv.Atoi(c.DefaultQuery("limit", "100"))
	if err != nil || limit < 1 || limit > 1000 {
		response.BadRequest(c, "limit must be between 1 and 1000")
		return
	}
	page, err := h.usageService.ListIncremental(c.Request.Context(), cursor, limit)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	records := make([]dto.AdminUsageLog, 0, len(page.Records))
	for i := range page.Records {
		records = append(records, *dto.UsageLogFromServiceAdmin(&page.Records[i]))
	}
	response.Success(c, gin.H{"items": records, "next_cursor": page.NextCursor.String(), "has_more": page.HasMore})
}
