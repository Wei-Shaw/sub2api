// Package response mirrors backend/internal/pkg/response for the plugin so
// HTTP responses share the same envelope shape as the core API.
package response

import (
	"log/slog"
	"math"
	"net/http"

	"github.com/Wei-Shaw/sub2api/plugins/channel-management/internal/errors"

	"github.com/gin-gonic/gin"
)

// Response is the standard JSON envelope for all plugin responses.
type Response struct {
	Code     int               `json:"code"`
	Message  string            `json:"message"`
	Reason   string            `json:"reason,omitempty"`
	Metadata map[string]string `json:"metadata,omitempty"`
	Data     any               `json:"data,omitempty"`
}

// PaginatedData wraps a list response with pagination metadata.
type PaginatedData struct {
	Items    any   `json:"items"`
	Total    int64 `json:"total"`
	Page     int   `json:"page"`
	PageSize int   `json:"page_size"`
	Pages    int   `json:"pages"`
}

// Success writes a 200 envelope with the given data.
func Success(c *gin.Context, data any) {
	c.JSON(http.StatusOK, Response{Code: 0, Message: "success", Data: data})
}

// ErrorWithDetails writes the structured error envelope.
func ErrorWithDetails(c *gin.Context, statusCode int, message, reason string, metadata map[string]string) {
	c.JSON(statusCode, Response{Code: statusCode, Message: message, Reason: reason, Metadata: metadata})
}

// ErrorFrom converts an error into the envelope-compatible error response.
// Returns true when an error was actually written.
func ErrorFrom(c *gin.Context, err error) bool {
	if err == nil {
		return false
	}
	statusCode, status := errors.ToHTTP(err)
	if statusCode >= 500 && c.Request != nil {
		// 用 SanitizeCauseForLog 包一道：err.Error() 已经会脱敏 cause，但顶层
		// err 本身可能直接是 *pq.Error / *fmt.wrapError 等含查询参数的实现，
		// 这里再过一遍以兜住非 ApplicationError 路径。
		slog.Error("plugin handler error",
			"method", c.Request.Method,
			"path", c.Request.URL.Path,
			"error", errors.SanitizeCauseForLog(err),
		)
	}
	ErrorWithDetails(c, statusCode, status.Message, status.Reason, status.Metadata)
	return true
}

// Paginated wraps Success with PaginatedData.
func Paginated(c *gin.Context, items any, total int64, page, pageSize int) {
	pages := int(math.Ceil(float64(total) / float64(pageSize)))
	if pages < 1 {
		pages = 1
	}
	Success(c, PaginatedData{Items: items, Total: total, Page: page, PageSize: pageSize, Pages: pages})
}

// ParsePagination reads the `page` / `page_size` (or `limit`) query params
// and returns sensible defaults when they are missing or malformed.
func ParsePagination(c *gin.Context) (page, pageSize int) {
	page = 1
	pageSize = 20
	if p := c.Query("page"); p != "" {
		if val, err := parseInt(p); err == nil && val > 0 {
			page = val
		}
	}
	if ps := c.Query("page_size"); ps != "" {
		if val, err := parseInt(ps); err == nil && val > 0 && val <= 1000 {
			pageSize = val
		}
	} else if l := c.Query("limit"); l != "" {
		if val, err := parseInt(l); err == nil && val > 0 && val <= 1000 {
			pageSize = val
		}
	}
	return page, pageSize
}

func parseInt(s string) (int, error) {
	var result int
	for _, c := range s {
		if c < '0' || c > '9' {
			return 0, nil
		}
		result = result*10 + int(c-'0')
	}
	return result, nil
}
