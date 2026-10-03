package service

import (
	"net/http"

	"github.com/gin-gonic/gin"
)

// UsageRequestResult separates the application outcome from transport and billing.
// A nil result on historical rows means unknown, not success.
type UsageRequestResult struct {
	Outcome        string `json:"outcome"`
	UsageStatus    string `json:"usage_status"`
	StatusCode     int    `json:"status_code,omitempty"`
	HTTPStatusCode int    `json:"http_status_code,omitempty"`
	ErrorCode      string `json:"error_code,omitempty"`
}

func openAIStreamErrorCode(payload []byte) string {
	if code := openAIStreamFailedEventErrorCode(payload); code != "" {
		return code
	}
	return "upstream_stream_error"
}

func markOpenAITerminalStreamFailure(c *gin.Context, payload []byte, message string) {
	MarkOpsStreamFailure(c, "upstream_error", openAIStreamErrorCode(payload),
		sanitizeUpstreamErrorMessage(message), openAIStreamFailedEventSemanticStatus(payload, message))
}

// FinalizeOpenAIUsageResult must run on the request goroutine, only after retries
// have finished. Attempt errors must not poison a subsequently recovered request.
func FinalizeOpenAIUsageResult(c *gin.Context, result *OpenAIForwardResult, forwardErr error) {
	if result == nil {
		return
	}
	meta := &UsageRequestResult{Outcome: "succeeded", UsageStatus: "unavailable", StatusCode: http.StatusOK}
	if result.Usage.Reported || openAIUsageHasTokens(&result.Usage) {
		meta.UsageStatus = "reported"
	}
	if c != nil && c.Writer != nil {
		meta.HTTPStatusCode = c.Writer.Status()
	}
	failed := forwardErr != nil || result.UpstreamTerminalEvent == "response.failed" || result.UpstreamTerminalEvent == "error"
	mark, marked := GetOpsStreamError(c)
	if marked {
		failed = true
		meta.StatusCode, meta.ErrorCode = mark.IntendedStatus, mark.Code
	}
	if failed {
		meta.Outcome = "failed"
		if meta.StatusCode < 400 {
			meta.StatusCode = http.StatusBadGateway
			if meta.HTTPStatusCode >= 400 {
				meta.StatusCode = meta.HTTPStatusCode
			}
		}
		if meta.ErrorCode == "" {
			meta.ErrorCode = "upstream_stream_error"
		}
		if result.Stream && !marked {
			message := "Upstream stream failed"
			if forwardErr != nil {
				message = sanitizeUpstreamErrorMessage(forwardErr.Error())
			}
			MarkOpsStreamFailure(c, "upstream_error", meta.ErrorCode, message, meta.StatusCode)
		}
	}
	result.RequestResult = meta
}
