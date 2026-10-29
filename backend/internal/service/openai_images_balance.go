package service

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"strings"
	"time"
)

const (
	OpenAIImagesInsufficientBalanceReason  GatewayFailureReason = "openai_images_insufficient_balance"
	OpenAIImagesInsufficientBalanceCode                         = "insufficient_balance"
	OpenAIImagesInsufficientBalanceMessage                      = "Upstream image account has insufficient balance"
	openAIImagesBalanceCooldown                                 = 5 * time.Minute
	openAIImagesBalanceRateLimitReason                          = "openai_images_insufficient_balance"
)

// isOpenAIImagesInsufficientBalance only accepts explicit values in structured
// error fields. This avoids treating echoed prompts or other arbitrary text as
// an account balance signal.
func isOpenAIImagesInsufficientBalance(body []byte) bool {
	var value any
	if len(body) == 0 || json.Unmarshal(body, &value) != nil {
		return false
	}
	return hasOpenAIImagesInsufficientBalance(value, 0)
}

func hasOpenAIImagesInsufficientBalance(value any, depth int) bool {
	if depth > 6 {
		return false
	}
	switch typed := value.(type) {
	case map[string]any:
		for key, child := range typed {
			normalizedKey := strings.ToLower(strings.TrimSpace(key))
			if text, ok := child.(string); ok {
				normalizedValue := strings.ToLower(strings.Trim(strings.TrimSpace(text), "."))
				switch normalizedKey {
				case "errorkey", "error_key", "code":
					if normalizedValue == OpenAIImagesInsufficientBalanceCode {
						return true
					}
				}
			}
			if isOpenAIImagesErrorContainer(normalizedKey) && hasOpenAIImagesInsufficientBalance(child, depth+1) {
				return true
			}
		}
	case []any:
		for _, child := range typed {
			if hasOpenAIImagesInsufficientBalance(child, depth+1) {
				return true
			}
		}
	}
	return false
}

func isOpenAIImagesErrorContainer(key string) bool {
	switch key {
	case "error", "errors", "cause", "detail", "details", "inner", "innererror", "inner_error", "response":
		return true
	default:
		return false
	}
}

func (s *OpenAIGatewayService) coolOpenAIImagesInsufficientBalance(ctx context.Context, account *Account) {
	if s == nil || s.accountRepo == nil || account == nil {
		return
	}
	now := time.Now()
	resetAt := now.Add(openAIImagesBalanceCooldown)
	setAccountModelRateLimitSnapshot(account, openAIImageGenerationRateLimitKey, resetAt, openAIImagesBalanceRateLimitReason, now)
	if err := s.accountRepo.SetModelRateLimit(ctx, account.ID, openAIImageGenerationRateLimitKey, resetAt, openAIImagesBalanceRateLimitReason); err != nil {
		slog.Warn("openai_images_balance_cooldown_failed", "account_id", account.ID, "error", err)
	}
}

func newOpenAIImagesInsufficientBalanceFailoverError(statusCode int, headers http.Header, body []byte) *UpstreamFailoverError {
	return &UpstreamFailoverError{
		StatusCode:        statusCode,
		ResponseHeaders:   headers.Clone(),
		ResponseBody:      body,
		Reason:            OpenAIImagesInsufficientBalanceReason,
		ClientStatusCode:  402,
		ClientMessage:     OpenAIImagesInsufficientBalanceMessage,
		NextAccountAction: NextAccountRetry,
	}
}
