package handler

import (
	"errors"
	"fmt"
	"net/http"

	"github.com/Wei-Shaw/sub2api/internal/config"
	pkghttputil "github.com/Wei-Shaw/sub2api/internal/pkg/httputil"
	"github.com/gin-gonic/gin"
	"go.uber.org/zap"
)

func extractMaxBytesError(err error) (*http.MaxBytesError, bool) {
	var maxErr *http.MaxBytesError
	if errors.As(err, &maxErr) {
		return maxErr, true
	}
	return nil, false
}

func formatBodyLimit(limit int64) string {
	const mb = 1024 * 1024
	if limit >= mb {
		return fmt.Sprintf("%dMB", limit/mb)
	}
	return fmt.Sprintf("%dB", limit)
}

func buildBodyTooLargeMessage(limit int64) string {
	return fmt.Sprintf("Request body too large, limit is %s", formatBodyLimit(limit))
}

// RequestBodyClientErrorDetails is the safe, protocol-neutral error payload
// used when an inbound request body cannot be read completely.
type RequestBodyClientErrorDetails struct {
	Status  int
	Code    string
	Message string
}

func RequestBodyClientErrorDetailsFrom(err error) RequestBodyClientErrorDetails {
	status, code, message := requestBodyClientError(err)
	return RequestBodyClientErrorDetails{Status: status, Code: code, Message: message}
}

// requestBodyClientError converts an internal request-body read failure into a
// stable, actionable client-facing error. The underlying error is intentionally
// not exposed because it may contain implementation details or request data.
func requestBodyClientError(err error) (status int, code, message string) {
	status = http.StatusBadRequest
	diagnostic := pkghttputil.RequestBodyDiagnostics(err)

	if maxErr, ok := extractMaxBytesError(err); ok {
		return http.StatusRequestEntityTooLarge,
			"request_body_too_large",
			buildBodyTooLargeMessage(maxErr.Limit)
	}

	switch diagnostic.Kind {
	case pkghttputil.RequestBodyErrorUnexpectedEOF:
		if diagnostic.ContentLength > 0 && diagnostic.BytesRead >= 0 {
			return status, "request_body_truncated", fmt.Sprintf(
				"Request body was truncated before upload completed. Received %d of %d bytes. Please retry the request. If the problem persists, check the client or reverse-proxy upload timeout.",
				diagnostic.BytesRead,
				diagnostic.ContentLength,
			)
		}
		return status, "request_body_truncated",
			"Request body was truncated before upload completed. Please retry the request."
	case pkghttputil.RequestBodyErrorReadCanceled:
		return status, "request_body_canceled",
			"Request upload was canceled before completion. Please retry the request."
	case pkghttputil.RequestBodyErrorInvalidCompression:
		return status, "invalid_compressed_body",
			"Compressed request body is invalid or incomplete. Please re-encode it and retry."
	case pkghttputil.RequestBodyErrorUnsupportedEncoding:
		return status, "unsupported_content_encoding",
			"Unsupported request Content-Encoding. Please use identity, gzip, deflate, or zstd."
	default:
		return status, "request_body_read_failed",
			"Unable to read the request body. Please retry the request and provide the request ID if the problem persists."
	}
}

func (h *OpenAIGatewayHandler) requestBodyErrorResponse(c *gin.Context, err error) {
	status, code, message := requestBodyClientError(err)
	h.errorResponseWithCode(c, status, "invalid_request_error", code, message)
}

func (h *OpenAIGatewayHandler) requestBodyAnthropicErrorResponse(c *gin.Context, err error) {
	status, code, message := requestBodyClientError(err)
	h.anthropicErrorResponseWithCode(c, status, "invalid_request_error", code, message)
}

func (h *GatewayHandler) requestBodyResponsesErrorResponse(c *gin.Context, err error) {
	status, code, message := requestBodyClientError(err)
	h.responsesErrorResponseWithCode(c, status, "invalid_request_error", code, message)
}

func (h *GatewayHandler) requestBodyChatCompletionsErrorResponse(c *gin.Context, err error) {
	status, code, message := requestBodyClientError(err)
	h.chatCompletionsErrorResponseWithCode(c, status, "invalid_request_error", code, message)
}

func (h *GatewayHandler) requestBodyErrorResponse(c *gin.Context, err error) {
	status, code, message := requestBodyClientError(err)
	h.errorResponseWithCode(c, status, "invalid_request_error", code, message)
}

func requestBodyImageTaskErrorResponse(c *gin.Context, err error) {
	details := RequestBodyClientErrorDetailsFrom(err)
	c.Header("Cache-Control", "no-store")
	c.JSON(details.Status, gin.H{"error": gin.H{
		"type":    "invalid_request_error",
		"code":    details.Code,
		"message": details.Message,
	}})
}

func readLenientJSONRequestBodyWithPrealloc(req *http.Request, cfg *config.Config) ([]byte, error) {
	return pkghttputil.ReadLenientJSONRequestBodyWithPrealloc(req, gatewayMaxBodySize(cfg))
}

func readRequestBodyWithDiagnostics(c *gin.Context, reqLog *zap.Logger) ([]byte, error) {
	body, err := pkghttputil.ReadRequestBodyWithPrealloc(c.Request)
	if err != nil {
		recordRequestBodyReadFailure(c, reqLog, err)
	}
	return body, err
}

func readLenientJSONRequestBodyWithDiagnostics(c *gin.Context, cfg *config.Config, reqLog *zap.Logger) ([]byte, error) {
	body, err := readLenientJSONRequestBodyWithPrealloc(c.Request, cfg)
	if err != nil {
		recordRequestBodyReadFailure(c, reqLog, err)
	}
	return body, err
}

func gatewayMaxBodySize(cfg *config.Config) int64 {
	if cfg == nil {
		return 0
	}
	return cfg.Gateway.MaxBodySize
}
