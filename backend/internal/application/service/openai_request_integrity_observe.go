package service

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"reflect"
	"sort"
	"strings"

	"github.com/gin-gonic/gin"
)

const openAIRequestIntegrityDifferencesContextKey = "openai_request_integrity_differences"

const openAIRequestIntegrityMaxObservedBytes = 8 << 20

var openAIRequestIntegrityProtectedFields = []string{
	"input",
	"instructions",
	"reasoning",
	"tools",
	"tool_choice",
	"parallel_tool_calls",
	"previous_response_id",
}

func (s *OpenAIGatewayService) openAIRequestIntegrityObserveEnabled(ctx context.Context) bool {
	if s == nil {
		return false
	}
	return s.openAIAdvancedSchedulerRuntimeSettings(ctx).requestIntegrityObserveEnabled
}

// Capture before timezone/compatibility transforms, but only for requests the
// observer can inspect. Admission reuses the existing cached runtime setting.
func (s *OpenAIGatewayService) captureOpenAIRequestIntegritySnapshot(ctx context.Context, account *Account, body []byte) []byte {
	if account == nil || !account.IsOpenAIOAuth() || len(body) == 0 ||
		len(body) > openAIRequestIntegrityMaxObservedBytes || !s.openAIRequestIntegrityObserveEnabled(ctx) {
		return nil
	}
	return bytes.Clone(body)
}

func decodeOpenAIIntegrityBody(raw []byte) (map[string]any, error) {
	var value map[string]any
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.UseNumber()
	if err := decoder.Decode(&value); err != nil {
		return nil, err
	}
	var trailing any
	if value == nil || decoder.Decode(&trailing) != io.EOF {
		return nil, io.ErrUnexpectedEOF
	}
	return value, nil
}

func canonicalOpenAIIntegrityProjection(raw []byte) (map[string]any, error) {
	body, err := decodeOpenAIIntegrityBody(raw)
	if err != nil {
		return nil, err
	}
	canonicalizeOpenAIIntegrityBody(body)
	projection := make(map[string]any, len(openAIRequestIntegrityProtectedFields))
	for _, field := range openAIRequestIntegrityProtectedFields {
		if value, ok := body[field]; ok {
			projection[field] = value
		}
	}
	return projection, nil
}

func canonicalizeOpenAIIntegrityBody(body map[string]any) {
	if body == nil {
		return
	}
	if text, ok := body["input"].(string); ok {
		body["input"] = []any{map[string]any{
			"type": "message", "role": "user",
			"content": []any{map[string]any{"type": "input_text", "text": text}},
		}}
	}
	extractSystemMessagesFromInput(body, true)
	if reasoning, ok := body["reasoning"].(map[string]any); ok {
		if reasoning["effort"] == "minimal" {
			reasoning["effort"] = "none"
		}
	}
	if _, hasTools := body["tools"]; !hasTools {
		if functions, ok := body["functions"].([]any); ok {
			tools := make([]any, 0, len(functions))
			for _, function := range functions {
				tools = append(tools, map[string]any{"type": "function", "function": function})
			}
			body["tools"] = tools
		}
	}
	if _, hasChoice := body["tool_choice"]; !hasChoice {
		if choice, exists := body["function_call"]; exists {
			body["tool_choice"] = choice
		}
	}
	canonicalizeOpenAIIntegrityValue(body)
}

func canonicalizeOpenAIIntegrityValue(value any) {
	switch typed := value.(type) {
	case map[string]any:
		itemType, _ := typed["type"].(string)
		if itemType == "message" || itemType == "reasoning" || itemType == "compaction_summary" {
			delete(typed, "id")
		}
		if content, ok := typed["content"].(string); ok {
			typed["content"] = []any{map[string]any{"type": "input_text", "text": content}}
		}
		if partType, ok := typed["type"].(string); ok && (partType == "text" || partType == "output_text") {
			typed["type"] = "input_text"
		}
		if function, ok := typed["function"].(map[string]any); ok && typed["type"] == "function" {
			for key, child := range function {
				if _, exists := typed[key]; !exists {
					typed[key] = child
				}
			}
			delete(typed, "function")
		}
		for _, child := range typed {
			canonicalizeOpenAIIntegrityValue(child)
		}
	case []any:
		for _, child := range typed {
			canonicalizeOpenAIIntegrityValue(child)
		}
	}
}

func openAIRequestIntegrityDifferences(original, forwarded []byte) ([]string, error) {
	before, err := canonicalOpenAIIntegrityProjection(original)
	if err != nil {
		return nil, err
	}
	after, err := canonicalOpenAIIntegrityProjection(forwarded)
	if err != nil {
		return nil, err
	}
	differences := make([]string, 0, len(openAIRequestIntegrityProtectedFields))
	for _, field := range openAIRequestIntegrityProtectedFields {
		beforeValue, beforeExists := before[field]
		afterValue, afterExists := after[field]
		// Default instructions may be added for Codex OAuth. Only an existing
		// caller instruction is protected from deletion or replacement.
		if field == "instructions" && !beforeExists {
			continue
		}
		if beforeExists != afterExists || !reflect.DeepEqual(beforeValue, afterValue) {
			differences = append(differences, field)
		}
	}
	sort.Strings(differences)
	return differences, nil
}

func (s *OpenAIGatewayService) observeOpenAIRequestIntegrity(ctx context.Context, c *gin.Context, account *Account, original, forwarded []byte, transport string) {
	if len(original) == 0 || account == nil || !account.IsOpenAIOAuth() || !s.openAIRequestIntegrityObserveEnabled(ctx) ||
		len(original) > openAIRequestIntegrityMaxObservedBytes || len(forwarded) > openAIRequestIntegrityMaxObservedBytes ||
		bytes.Equal(original, forwarded) {
		return
	}
	differences, err := openAIRequestIntegrityDifferences(original, forwarded)
	if err != nil || len(differences) == 0 {
		return
	}
	signature := strings.Join(differences, ",") + "|" + transport
	if c != nil {
		if previous, ok := c.Get(openAIRequestIntegrityDifferencesContextKey); ok && previous == signature {
			return
		}
		c.Set(openAIRequestIntegrityDifferencesContextKey, signature)
	}
	slog.Warn("openai_request_integrity_difference",
		"account_id", account.ID,
		"transport", transport,
		"fields", strings.Join(differences, ","),
	)
}
