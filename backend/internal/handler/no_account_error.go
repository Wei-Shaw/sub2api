package handler

import (
	"context"
	"fmt"
	"math"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/Wei-Shaw/sub2api/internal/service"
)

// noAccountErrorClassification describes the HTTP response to emit when
// account selection failed with ErrNoAvailableAccounts. Handlers obtain it
// via classifyNoAccountError and choose between:
//
//   - 404 model_not_found — the group has accounts, but none of them are
//     configured to serve the requested model (config / typo / unsupported
//     model). Returning 503 here misleads operators and trips reverse-proxy
//     health checks; 404 lets the client surface the real problem.
//
//   - 429 rate_limit_error — every matching Anthropic account is waiting for
//     a known rate-limit reset time.
//
//   - 503 api_error — accounts that could serve the model exist but are
//     temporarily exhausted (rate limit, quota auto-pause, runtime block) OR
//     the group has no accounts at all. Both stay on 503 because retrying
//     after a backoff can plausibly succeed (or, in the empty-pool case, the
//     operator may be in the middle of adding accounts).
type noAccountErrorClassification struct {
	Status            int
	ErrType           string
	Message           string
	ModelNotFound     bool // true when this is a 404 model_not_found classification
	RetryAfterSeconds int
	ResetsAt          *time.Time
}

// classifyNoAccountError decides between 404 model_not_found, 429
// rate_limit_error, and 503 api_error for "no available accounts" failures.
//
// The classifier intentionally does not consume the original selection error.
// Instead, it re-checks pool composition through
// DiagnoseModelAvailabilityForPlatform. The dedicated database query considers
// persistent eligibility and model mapping while retaining transient reset
// metadata. This keeps all no-account reason classification in one place and
// avoids forcing every selection call site to propagate a new error type.
//
// routingModel is the model name that account selection actually compared
// against (i.e. after group-level dispatch mapping). displayModel is the raw
// model the caller asked for; it is used only in the user-facing error message.
//
// platform scopes diagnosis to the actual routed platform. Anthropic/Gemini
// routes can additionally include mixed-scheduled Antigravity accounts.
func classifyNoAccountError(
	ctx context.Context,
	diag service.ModelAvailabilityDiagnoser,
	apiKey *service.APIKey,
	routingModel string,
	displayModel string,
	platform string,
) noAccountErrorClassification {
	fallback := noAccountErrorClassification{
		Status:  http.StatusServiceUnavailable,
		ErrType: "api_error",
		Message: "Service temporarily unavailable",
	}

	routingModel = strings.TrimSpace(routingModel)
	displayModel = strings.TrimSpace(displayModel)
	if displayModel == "" {
		displayModel = routingModel
	}
	if diag == nil || apiKey == nil || apiKey.GroupID == nil || routingModel == "" {
		return fallback
	}

	result := diag.DiagnoseModelAvailabilityForPlatform(ctx, apiKey.GroupID, routingModel, platform)
	if result.AllMatchingAccountsRateLimited && result.MinRateLimitResetAt != nil {
		retryAfterSeconds := int(math.Ceil(time.Until(*result.MinRateLimitResetAt).Seconds()))
		if retryAfterSeconds > 0 {
			return noAccountErrorClassification{
				Status:            http.StatusTooManyRequests,
				ErrType:           "rate_limit_error",
				Message:           fmt.Sprintf("All available accounts are rate limited. Try again in %d seconds.", retryAfterSeconds),
				RetryAfterSeconds: retryAfterSeconds,
				ResetsAt:          result.MinRateLimitResetAt,
			}
		}
	}
	if result.HasAccountsInPool && !result.HasModelSupport {
		return noAccountErrorClassification{
			Status:        http.StatusNotFound,
			ErrType:       "model_not_found",
			Message:       fmt.Sprintf("Model %q is not supported by any configured account in this group", displayModel),
			ModelNotFound: true,
		}
	}
	return fallback
}

// classifyNoAccountErrorFromGin is a thin wrapper that forwards the gin
// context's underlying request context. Most call sites already have a
// *gin.Context handy, so this keeps the call sites uncluttered.
func classifyNoAccountErrorFromGin(
	c *gin.Context,
	diag service.ModelAvailabilityDiagnoser,
	apiKey *service.APIKey,
	routingModel string,
	displayModel string,
	platform string,
) noAccountErrorClassification {
	ctx := context.Background()
	if c != nil && c.Request != nil {
		ctx = c.Request.Context()
	}
	cls := classifyNoAccountError(ctx, diag, apiKey, routingModel, displayModel, platform)
	setNoAccountRetryAfterHeader(c, cls)
	return cls
}

func classifyOpenAICompatibleNoAccountErrorFromGin(
	c *gin.Context,
	diag service.ModelAvailabilityDiagnoser,
	apiKey *service.APIKey,
	routingModel string,
	displayModel string,
) noAccountErrorClassification {
	ctx := context.Background()
	if c != nil && c.Request != nil {
		ctx = c.Request.Context()
	}
	return classifyNoAccountErrorFromGin(
		c,
		diag,
		apiKey,
		routingModel,
		displayModel,
		openAICompatibleRequestPlatform(ctx, apiKey),
	)
}

type noAccountRateLimitProtocol int

const (
	noAccountRateLimitAnthropic noAccountRateLimitProtocol = iota
	noAccountRateLimitOpenAI
)

func setNoAccountRetryAfterHeader(c *gin.Context, cls noAccountErrorClassification) {
	if c == nil || cls.Status != http.StatusTooManyRequests || cls.RetryAfterSeconds <= 0 {
		return
	}
	c.Header("Retry-After", strconv.Itoa(cls.RetryAfterSeconds))
}

// writeAllAccountsRateLimitedError writes the precise reset metadata using
// the inbound protocol's error shape. It returns false when cls is not the
// full-pool rate-limit classification, allowing callers to use their existing
// generic error writer unchanged.
func writeAllAccountsRateLimitedError(c *gin.Context, cls noAccountErrorClassification, protocol noAccountRateLimitProtocol) bool {
	if c == nil || cls.Status != http.StatusTooManyRequests || cls.RetryAfterSeconds <= 0 || cls.ResetsAt == nil {
		return false
	}

	errorBody := gin.H{
		"type":              cls.ErrType,
		"message":           cls.Message,
		"resets_in_seconds": cls.RetryAfterSeconds,
		"resets_at":         cls.ResetsAt.UTC().Format(time.RFC3339),
	}
	responseBody := gin.H{"error": errorBody}
	if protocol == noAccountRateLimitOpenAI {
		errorBody["code"] = "rate_limit_exceeded"
		errorBody["param"] = nil
	} else {
		responseBody["type"] = "error"
	}
	c.JSON(http.StatusTooManyRequests, responseBody)
	return true
}

func openAICompatibleSelectionErrorForLog(err error, platform string) error {
	if err == nil || platform != service.PlatformGrok {
		return err
	}
	message := strings.ReplaceAll(err.Error(), "OpenAI accounts", "Grok accounts")
	if message == err.Error() {
		return err
	}
	return fmt.Errorf("%s", message)
}
