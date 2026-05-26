// Package response provides standardized HTTP response helpers for the
// payment plugin. It mirrors the surface of backend/internal/pkg/response
// but uses the plugin-local infraerrors package so the plugin compiles
// without any backend imports.
package response

import (
	"net/http"

	"github.com/gin-gonic/gin"

	infraerrors "github.com/Wei-Shaw/sub2api/plugins/payment/internal/errors"
)

// envelope is the JSON shape every response uses.
type envelope struct {
	Code     int               `json:"code"`
	Message  string            `json:"message,omitempty"`
	Reason   string            `json:"reason,omitempty"`
	Data     any               `json:"data,omitempty"`
	Metadata map[string]string `json:"metadata,omitempty"`
}

// Success writes a 200 OK with the provided data.
func Success(c *gin.Context, data any) {
	c.JSON(http.StatusOK, envelope{Code: 0, Data: data})
}

// Created writes a 201 with the provided data.
func Created(c *gin.Context, data any) {
	c.JSON(http.StatusCreated, envelope{Code: 0, Data: data})
}

// Accepted writes a 202.
func Accepted(c *gin.Context, data any) {
	c.JSON(http.StatusAccepted, envelope{Code: 0, Data: data})
}

// Error writes a generic error response.
func Error(c *gin.Context, statusCode int, message string) {
	c.JSON(statusCode, envelope{Code: statusCode, Message: message})
}

// ErrorWithDetails writes an error response with a reason and metadata.
func ErrorWithDetails(c *gin.Context, statusCode int, message, reason string, metadata map[string]string) {
	c.JSON(statusCode, envelope{
		Code:     statusCode,
		Message:  message,
		Reason:   reason,
		Metadata: metadata,
	})
}

// ErrorFrom inspects err for an *ApplicationError and writes the matching
// HTTP response. Returns true when the error was an ApplicationError.
func ErrorFrom(c *gin.Context, err error) bool {
	if err == nil {
		return false
	}
	statusCode, status := infraerrors.ToHTTP(err)
	c.JSON(statusCode, envelope{
		Code:     int(status.Code),
		Message:  status.Message,
		Reason:   status.Reason,
		Metadata: status.Metadata,
	})
	return statusCode != http.StatusOK
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

// PaginatedData is the standard paginated payload.
type PaginatedData struct {
	Items    any   `json:"items"`
	Total    int64 `json:"total"`
	Page     int   `json:"page"`
	PageSize int   `json:"page_size"`
}

// Paginated writes a paginated success response.
func Paginated(c *gin.Context, items any, total int64, page, pageSize int) {
	Success(c, PaginatedData{Items: items, Total: total, Page: page, PageSize: pageSize})
}
