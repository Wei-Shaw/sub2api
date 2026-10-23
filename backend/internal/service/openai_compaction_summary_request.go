package service

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/tidwall/gjson"
)

type openAICompactionSummaryExecution struct {
	mode     string
	protocol string
}

const (
	openAICompactionExecutionNative          = "native"
	openAICompactionExecutionSummary         = "summary"
	openAICompactionExecutionUnsupported     = "unsupported"
	openAICompactionKindV2                   = "native_v2"
	openAICompactionKindLegacy               = "legacy_compact"
	OpenAICompactionSummaryOutputTokenBudget = 8192
)

const openAICompactionSummaryExecutionKey = "openai_compaction_summary_execution"

func OpenAICompactionSummaryPromptForReservation() []byte {
	return []byte(grokCompactSummaryPrompt)
}

func openAICompactionKindForContext(c *gin.Context) string {
	if c == nil {
		return ""
	}
	if isOpenAINativeCompactionV2(c) {
		return openAICompactionKindV2
	}
	if isOpenAIResponsesCompactPath(c) {
		return openAICompactionKindLegacy
	}
	return ""
}

func (s *OpenAIGatewayService) openAICompactionExecutionForAccount(c *gin.Context, account *Account, requestedModel, actualProtocol string) string {
	if c == nil || account == nil || !isExplicitOpenAICompactContext(c) {
		return ""
	}
	kind := openAICompactionKindForContext(c)
	if kind == "" {
		return ""
	}
	ctx := context.Background()
	if c.Request != nil {
		ctx = c.Request.Context()
	}
	return s.openAICompactionRequestDecisionWithProtocol(ctx, account, requestedModel, kind, true, actualProtocol).Execution
}

func markOpenAICompactionSummaryExecution(c *gin.Context, protocol string) {
	if c == nil {
		return
	}
	c.Set(openAICompactionSummaryExecutionKey, openAICompactionSummaryExecution{
		mode:     openAICompactionExecutionSummary,
		protocol: protocol,
	})
}

func openAICompactionSummaryExecutionFromContext(c *gin.Context) (openAICompactionSummaryExecution, bool) {
	if c == nil {
		return openAICompactionSummaryExecution{}, false
	}
	value, ok := c.Get(openAICompactionSummaryExecutionKey)
	if !ok {
		return openAICompactionSummaryExecution{}, false
	}
	execution, _ := value.(openAICompactionSummaryExecution)
	return execution, execution.mode == openAICompactionExecutionSummary
}

func IsOpenAICompactionSummaryExecution(c *gin.Context) bool {
	_, ok := openAICompactionSummaryExecutionFromContext(c)
	return ok
}

func buildOpenAICompactionSummaryRequestBody(body []byte) ([]byte, error) {
	if err := ValidateOpenAICompactionInputShape(body); err != nil {
		return nil, err
	}
	var payload map[string]any
	if err := decodeOpenAIJSONUseNumber(body, &payload); err != nil {
		return nil, fmt.Errorf("decode compaction summary request: %w", err)
	}
	if HasOpenAIEncryptedCompactionInput(body) {
		return nil, fmt.Errorf("compaction summary cannot replace unrecoverable native compaction state")
	}
	if previousResponseID := strings.TrimSpace(stringValue(payload["previous_response_id"])); previousResponseID != "" {
		return nil, fmt.Errorf("compaction summary cannot rely on previous_response_id without verified full-history replay")
	}
	input, err := normalizeOpenAICompactionSummaryInput(payload["input"])
	if err != nil {
		return nil, err
	}
	if openAICompactionSummaryInputContainsMedia(input) {
		return nil, fmt.Errorf("compaction summary does not support image, file, audio, or video input")
	}
	filtered := make([]any, 0, len(input)+1)
	for _, item := range input {
		itemMap, _ := item.(map[string]any)
		if itemMap != nil && strings.TrimSpace(stringValue(itemMap["type"])) == "compaction_trigger" {
			continue
		}
		filtered = append(filtered, item)
	}
	if len(filtered) == 0 {
		return nil, fmt.Errorf("compaction summary requires visible conversation history; previous_response_id alone is not recoverable")
	}
	filtered = append(filtered, map[string]any{
		"type": "message",
		"role": "user",
		"content": []any{map[string]any{
			"type": "input_text",
			"text": grokCompactSummaryPrompt,
		}},
	})
	payload["input"] = filtered
	payload["stream"] = false
	payload["store"] = false
	payload["max_output_tokens"] = OpenAICompactionSummaryOutputTokenBudget
	delete(payload, "previous_response_id")
	delete(payload, "tools")
	delete(payload, "tool_choice")
	delete(payload, "parallel_tool_calls")
	delete(payload, "response_format")
	delete(payload, "text")
	// Responses defaults to disabled truncation. Do not inherit truncation:auto,
	// because the summary request must fail rather than silently shorten history.
	delete(payload, "truncation")
	encoded, err := marshalOpenAIUpstreamJSON(payload)
	if err != nil {
		return nil, fmt.Errorf("encode compaction summary request: %w", err)
	}
	return encoded, nil
}

func (s *OpenAIGatewayService) PrepareOpenAICompactionSummaryRequest(_ *Account, body []byte) ([]byte, error) {
	return buildOpenAICompactionSummaryRequestBody(body)
}

func validateOpenAICompactionSummaryContext(account *Account, body []byte, upstreamModel string) error {
	if account == nil {
		return fmt.Errorf("compaction summary account is unavailable")
	}
	upstreamModel = strings.TrimSpace(upstreamModel)
	if upstreamModel == "" {
		return fmt.Errorf("compaction summary final upstream model is unavailable")
	}
	metadata, ok := account.GetUpstreamModelMetadata(upstreamModel)
	contextWindow := metadata.ContextWindow
	if contextWindow <= 0 {
		contextWindow = metadata.MaxContextWindow
	}
	if !ok || contextWindow <= 0 {
		return fmt.Errorf("compaction summary context window is unavailable for model %q", upstreamModel)
	}
	estimated, err := estimateOpenAIInputTokens(openAIInputTokensCountRequest{
		Model:        upstreamModel,
		Instructions: gjson.GetBytes(body, "instructions").String(),
		Input:        json.RawMessage(gjson.GetBytes(body, "input").Raw),
	})
	if err != nil {
		return fmt.Errorf("estimate compaction summary input: %w", err)
	}
	outputTokens := int(gjson.GetBytes(body, "max_output_tokens").Int())
	if outputTokens <= 0 {
		outputTokens = OpenAICompactionSummaryOutputTokenBudget
	}
	required := int64(estimated + outputTokens + 1024)
	if required > contextWindow {
		return fmt.Errorf("compaction summary input requires %d tokens but model %q context window is %d", required, upstreamModel, contextWindow)
	}
	return nil
}

func validateOpenAICompactionSummaryContextForExecution(c *gin.Context, account *Account, body []byte, upstreamModel string) error {
	if !IsOpenAICompactionSummaryExecution(c) {
		return nil
	}
	if err := validateOpenAICompactionSummaryContext(account, body, upstreamModel); err != nil {
		writeOpenAICompactionSummaryRequestError(c, err)
		return err
	}
	return nil
}

func writeOpenAICompactionSummaryRequestError(c *gin.Context, err error) {
	if c == nil {
		return
	}
	setOpsUpstreamError(c, http.StatusBadRequest, err.Error(), "")
	c.JSON(http.StatusBadRequest, gin.H{"error": gin.H{
		"type":    "invalid_request_error",
		"code":    "compaction_summary_unsupported",
		"message": err.Error(),
	}})
}

func normalizeOpenAICompactionSummaryInput(value any) ([]any, error) {
	switch input := value.(type) {
	case nil:
		return nil, nil
	case []any:
		return input, nil
	case map[string]any:
		return []any{input}, nil
	case string:
		if strings.TrimSpace(input) == "" {
			return nil, nil
		}
		return []any{map[string]any{
			"type": "message",
			"role": "user",
			"content": []any{map[string]any{
				"type": "input_text",
				"text": input,
			}},
		}}, nil
	default:
		return nil, fmt.Errorf("compaction summary input must be a string, object, or array")
	}
}

func openAICompactionSummaryInputContainsMedia(value any) bool {
	switch typed := value.(type) {
	case []any:
		for _, item := range typed {
			if openAICompactionSummaryInputContainsMedia(item) {
				return true
			}
		}
	case map[string]any:
		switch strings.TrimSpace(stringValue(typed["type"])) {
		case "input_image", "input_file", "input_audio", "input_video", "image_url", "computer_screenshot":
			return true
		}
		for _, item := range typed {
			if openAICompactionSummaryInputContainsMedia(item) {
				return true
			}
		}
	}
	return false
}
