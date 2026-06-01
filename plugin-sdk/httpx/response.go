// Package httpx is the single-source gin HTTP response envelope shared by the
// plugins that serve admin/user routes (channel-management, payment). Before
// this package, each carried its own copy under internal/response and the two
// copies had drifted (one named the type Response and set message="success",
// the other named it envelope and grew Created/Accepted/BadRequest wrappers;
// one classified raw DB errors, the other did not). Parallel implementations
// of the same wire envelope violate the CLAUDE.md "复用 (Reuse)" principle.
//
// This package owns the superset surface. It depends on gin (kept out of the
// plugin-sdk root module so non-gin plugins like content-moderation are not
// forced to pull gin) and on the shared apperr classifiers. The envelope shape
// mirrors backend/internal/pkg/response so plugin responses share the same
// contract as the core API.
package httpx

import (
	stderrors "errors"
	"log/slog"
	"math"
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/Wei-Shaw/sub2api/plugin-sdk/apperr"
)

const (
	// successCode / successMessage are the envelope fields emitted on a 2xx
	// response. message="success" matches the core's response envelope.
	successCode    = 0
	successMessage = "success"
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
	c.JSON(http.StatusOK, Response{Code: successCode, Message: successMessage, Data: data})
}

// Created writes a 201 envelope with the given data.
func Created(c *gin.Context, data any) {
	c.JSON(http.StatusCreated, Response{Code: successCode, Message: successMessage, Data: data})
}

// Accepted writes a 202 envelope with the given data.
func Accepted(c *gin.Context, data any) {
	c.JSON(http.StatusAccepted, Response{Code: successCode, Message: successMessage, Data: data})
}

// Error writes a generic error response carrying only a message.
func Error(c *gin.Context, statusCode int, message string) {
	c.JSON(statusCode, Response{Code: statusCode, Message: message})
}

// ErrorWithDetails writes the structured error envelope.
func ErrorWithDetails(c *gin.Context, statusCode int, message, reason string, metadata map[string]string) {
	c.JSON(statusCode, Response{Code: statusCode, Message: message, Reason: reason, Metadata: metadata})
}

// BadRequest is a thin wrapper for 400 responses.
func BadRequest(c *gin.Context, message string) { Error(c, http.StatusBadRequest, message) }

// Unauthorized is the 401 wrapper.
func Unauthorized(c *gin.Context, message string) { Error(c, http.StatusUnauthorized, message) }

// Forbidden is the 403 wrapper.
func Forbidden(c *gin.Context, message string) { Error(c, http.StatusForbidden, message) }

// NotFound is the 404 wrapper.
func NotFound(c *gin.Context, message string) { Error(c, http.StatusNotFound, message) }

// InternalError is the 500 wrapper.
func InternalError(c *gin.Context, message string) { Error(c, http.StatusInternalServerError, message) }

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
	classified := classifyIfNeeded(err)
	statusCode, st := apperr.ToHTTP(classified)
	if statusCode >= http.StatusInternalServerError && c.Request != nil {
		slog.Error("plugin handler error",
			"method", c.Request.Method,
			"path", c.Request.URL.Path,
			"error", apperr.SanitizeCauseForLog(err),
		)
	}
	ErrorWithDetails(c, statusCode, st.Message, st.Reason, st.Metadata)
	return true
}

// classifyIfNeeded returns err unchanged when it is already an
// ApplicationError. Otherwise it tries the DB / gRPC classifiers to promote
// known errors to structured 4xx/5xx responses.
func classifyIfNeeded(err error) error {
	var appErr *apperr.ApplicationError
	if stderrors.As(err, &appErr) {
		return err
	}
	if classified := apperr.ClassifyDBError(err); classified != nil {
		return classified
	}
	if classified := apperr.ClassifyGRPCError(err); classified != nil {
		return classified
	}
	return err
}

// Paginated wraps Success with PaginatedData.
func Paginated(c *gin.Context, items any, total int64, page, pageSize int) {
	pages := 1
	if pageSize > 0 {
		pages = int(math.Ceil(float64(total) / float64(pageSize)))
	}
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
		if val := parsePositiveInt(p); val > 0 {
			page = val
		}
	}
	if ps := c.Query("page_size"); ps != "" {
		if val := parsePositiveInt(ps); val > 0 && val <= 1000 {
			pageSize = val
		}
	} else if l := c.Query("limit"); l != "" {
		if val := parsePositiveInt(l); val > 0 && val <= 1000 {
			pageSize = val
		}
	}
	return page, pageSize
}

// parsePositiveInt parses a decimal string into a non-negative int, returning
// 0 on any non-digit input. It bounds the result at math.MaxInt32 so a long
// digit string cannot overflow.
func parsePositiveInt(s string) int {
	var result int
	for _, c := range s {
		if c < '0' || c > '9' {
			return 0
		}
		result = result*10 + int(c-'0')
		if result > math.MaxInt32 {
			return math.MaxInt32
		}
	}
	return result
}
