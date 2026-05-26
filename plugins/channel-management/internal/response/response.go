// Package response mirrors backend/internal/pkg/response for the plugin so
// HTTP responses share the same envelope shape as the core API.
package response

import (
	stderrors "errors"
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
//
// When err is not already an ApplicationError and carries the structured pq
// metadata emitted by the host gRPC layer, ClassifyDBError promotes it to a
// user-visible 4xx response instead of the generic 500 "internal error".
func ErrorFrom(c *gin.Context, err error) bool {
	if err == nil {
		return false
	}

	// Try to classify raw DB errors before falling through to FromError,
	// which would otherwise wrap them as 500 "internal error".
	classified := classifyIfNeeded(err)

	statusCode, st := errors.ToHTTP(classified)
	if statusCode >= 500 && c.Request != nil {
		slog.Error("plugin handler error",
			"method", c.Request.Method,
			"path", c.Request.URL.Path,
			"error", errors.SanitizeCauseForLog(err),
		)
	}
	ErrorWithDetails(c, statusCode, st.Message, st.Reason, st.Metadata)
	return true
}

// classifyIfNeeded returns err unchanged when it is already an
// ApplicationError. Otherwise it tries ClassifyDBError to promote known
// database errors to structured 4xx responses.
func classifyIfNeeded(err error) error {
	// If err is already a typed ApplicationError, respect it as-is.
	var appErr *errors.ApplicationError
	if stderrors.As(err, &appErr) {
		return err
	}
	// Not a typed error — try to classify the underlying DB or gRPC error.
	if classified := errors.ClassifyDBError(err); classified != nil {
		return classified
	}
	if classified := errors.ClassifyGRPCError(err); classified != nil {
		return classified
	}
	return err
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
