package admin

import (
	"context"
	"errors"
	"net/http"
	"strconv"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/response"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
)

// VideoTasks is a read-only view of accepted tasks and unresolved submissions.
func (h *UsageHandler) VideoTasks(c *gin.Context) {
	f := service.VideoTaskAdminFilter{Page: 1, PageSize: 20, Days: 7}
	for _, param := range []struct {
		name string
		dest *int
	}{{"page", &f.Page}, {"page_size", &f.PageSize}, {"days", &f.Days}} {
		if values, exists := c.Request.URL.Query()[param.name]; exists {
			if len(values) != 1 {
				response.BadRequest(c, "Invalid "+param.name)
				return
			}
			n, err := strconv.Atoi(values[0])
			if err != nil {
				response.BadRequest(c, "Invalid "+param.name)
				return
			}
			*param.dest = n
		}
	}
	for _, param := range []struct {
		name string
		dest *string
	}{{"search", &f.Search}, {"status", &f.Status}} {
		if values, exists := c.Request.URL.Query()[param.name]; exists {
			if len(values) != 1 {
				response.BadRequest(c, "Invalid "+param.name)
				return
			}
			*param.dest = values[0]
		}
	}
	if err := f.Validate(); err != nil {
		response.BadRequest(c, err.Error())
		return
	}
	// Bound DB work even if the caller keeps a request open indefinitely.
	ctx, cancel := context.WithTimeout(c.Request.Context(), 10*time.Second)
	defer cancel()
	items, total, err := h.videoGateway.ListAdminVideoTasks(ctx, f)
	if err != nil {
		if errors.Is(err, service.ErrVideoTaskRepositoryUnavailable) {
			response.Error(c, http.StatusServiceUnavailable, "Video task list is unavailable")
		} else {
			response.InternalError(c, "Unable to list video tasks")
		}
		return
	}
	if items == nil {
		items = []service.VideoTaskAdminItem{}
	}
	c.Header("Cache-Control", "private, no-store")
	response.Paginated(c, items, total, f.Page, f.PageSize)
}
