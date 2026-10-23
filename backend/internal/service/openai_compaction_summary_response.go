package service

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/apicompat"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/tidwall/gjson"
	"github.com/tidwall/sjson"
)

var ErrOpenAICompactionSummaryGeneration = errors.New("compaction summary generation failed")

func validateOpenAICompactionSummaryAnthropicTerminal(c *gin.Context, response *apicompat.AnthropicResponse) error {
	if _, ok := openAICompactionSummaryExecutionFromContext(c); !ok || response == nil {
		return nil
	}
	reason := strings.TrimSpace(apicompat.AnthropicStopReasonString(response.StopReason))
	if reason == "end_turn" || reason == "stop_sequence" {
		return nil
	}
	return fmt.Errorf("%w: Anthropic terminal stop_reason is %q", ErrOpenAICompactionSummaryGeneration, reason)
}

func validateOpenAICompactionSummaryChatTerminal(c *gin.Context, response *apicompat.ChatCompletionsResponse) error {
	if _, ok := openAICompactionSummaryExecutionFromContext(c); !ok || response == nil {
		return nil
	}
	if len(response.Choices) == 0 {
		return fmt.Errorf("%w: Chat Completions response has no choices", ErrOpenAICompactionSummaryGeneration)
	}
	reason := strings.TrimSpace(response.Choices[0].FinishReason)
	if reason == "stop" {
		return nil
	}
	return fmt.Errorf("%w: Chat Completions finish_reason is %q", ErrOpenAICompactionSummaryGeneration, reason)
}

func (s *OpenAIGatewayService) finalizeOpenAICompactionSummaryResponse(c *gin.Context, statusCode int, body []byte) ([]byte, error) {
	if _, ok := openAICompactionSummaryExecutionFromContext(c); !ok || statusCode < 200 || statusCode >= 300 {
		return body, nil
	}
	if len(body) == 0 || !gjson.ValidBytes(body) || !gjson.ParseBytes(body).IsObject() {
		return body, fmt.Errorf("%w: upstream response is not a JSON object", ErrOpenAICompactionSummaryGeneration)
	}
	responseStatus := strings.ToLower(strings.TrimSpace(gjson.GetBytes(body, "status").String()))
	incompleteDetails := gjson.GetBytes(body, "incomplete_details")
	if responseStatus != "completed" || (incompleteDetails.Exists() && incompleteDetails.Type != gjson.Null && strings.TrimSpace(incompleteDetails.Raw) != "{}") {
		if responseStatus == "" {
			responseStatus = "missing"
		}
		return body, fmt.Errorf("%w: upstream summary terminal status is %s", ErrOpenAICompactionSummaryGeneration, responseStatus)
	}
	summary := openAICompactionSummaryResponseText(body)
	if summary == "" {
		return body, fmt.Errorf("%w: upstream summary text is empty", ErrOpenAICompactionSummaryGeneration)
	}
	apiKey := getAPIKeyFromContext(c)
	if apiKey == nil {
		return body, fmt.Errorf("%w: authenticated API key is unavailable", ErrOpenAICompactionSummaryInvalid)
	}
	scope, err := NewOpenAICompactionOwnerScope(apiKey.UserID, apiKey.GroupID)
	if err != nil {
		return body, err
	}
	key, err := s.openAICompactionSummaryCipherKey(c.Request.Context(), true)
	if err != nil {
		return body, err
	}
	encrypted, err := SealOpenAICompactionSummary(key, scope, summary, time.Now())
	if err != nil {
		return body, err
	}
	item := map[string]any{
		"id":                "cmp_" + strings.ReplaceAll(uuid.NewString(), "-", ""),
		"type":              "compaction",
		"status":            "completed",
		"encrypted_content": encrypted,
	}
	itemJSON, err := json.Marshal(item)
	if err != nil {
		return body, fmt.Errorf("encode compaction summary item: %w", err)
	}
	updated, err := sjson.SetRawBytes(body, "output", []byte("["+string(itemJSON)+"]"))
	if err != nil {
		return body, fmt.Errorf("replace compaction summary output: %w", err)
	}
	updated, err = sjson.SetBytes(updated, "status", "completed")
	if err != nil {
		return body, fmt.Errorf("set compaction summary status: %w", err)
	}
	updated, _ = sjson.DeleteBytes(updated, "output_text")
	updated, _ = sjson.DeleteBytes(updated, "incomplete_details")
	return updated, nil
}

func openAICompactionSummaryResponseText(body []byte) string {
	var texts []string
	for _, item := range gjson.GetBytes(body, "output").Array() {
		if strings.TrimSpace(item.Get("type").String()) != "message" {
			continue
		}
		for _, part := range item.Get("content").Array() {
			switch strings.TrimSpace(part.Get("type").String()) {
			case "output_text", "text", "input_text":
				if text := strings.TrimSpace(part.Get("text").String()); text != "" {
					texts = append(texts, text)
				}
			}
		}
	}
	if len(texts) == 0 {
		if text := strings.TrimSpace(gjson.GetBytes(body, "output_text").String()); text != "" {
			texts = append(texts, text)
		}
	}
	return strings.Join(texts, "\n")
}

func writeOpenAICompactionSummaryFinalizationError(c *gin.Context, err error) {
	if c == nil {
		return
	}
	if openAICompactClientWantsStream(c) {
		StopOpenAICompactSSEKeepaliveCommitted(c)
		c.Writer.Header().Set("Content-Type", "text/event-stream")
		c.Writer.Header().Set("Cache-Control", "no-cache")
		c.Writer.WriteHeader(http.StatusBadGateway)
		writeOpenAICompactSSEFailureMessage(c, http.StatusBadGateway, "compaction_summary_failed", sanitizeUpstreamErrorMessage(err.Error()))
		return
	}
	c.JSON(http.StatusBadGateway, gin.H{"error": gin.H{
		"type":    "upstream_error",
		"code":    "compaction_summary_failed",
		"message": "The gateway could not create a valid compaction summary",
	}})
}
