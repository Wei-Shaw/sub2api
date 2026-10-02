package service

import (
	"fmt"
	"strings"

	"github.com/Wei-Shaw/sub2api/internal/pkg/apicompat"
	"github.com/google/uuid"
)

// buildDeepSeekCompactChatBody rewrites a native remote compaction v2 request
// into a normal chat turn DeepSeek can answer. DeepSeek's /responses ignores
// the compaction_trigger item and just continues the conversation, so the
// trigger is dropped and a summary instruction is appended as a trailing user
// message. stream is forced false so the bridge buffers the reply for
// compaction synthesis (a compact response is a single short item).
func buildDeepSeekCompactChatBody(body []byte) ([]byte, error) {
	var payload map[string]any
	if err := decodeOpenAIJSONUseNumber(body, &payload); err != nil {
		return nil, fmt.Errorf("decode deepseek compact body: %w", err)
	}

	input, _ := payload["input"].([]any)
	filtered := make([]any, 0, len(input)+1)
	for _, raw := range input {
		if item, ok := raw.(map[string]any); ok && strings.TrimSpace(stringValue(item["type"])) == "compaction_trigger" {
			continue
		}
		filtered = append(filtered, raw)
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

	encoded, err := marshalOpenAIUpstreamJSON(payload)
	if err != nil {
		return nil, fmt.Errorf("encode deepseek compact body: %w", err)
	}
	return encoded, nil
}

// compactSummaryTextFromResponses joins the visible text of every message item
// in a converted Responses output. The Chat bridge turns a DeepSeek chat reply
// into a single message item whose output_text parts hold the summary.
func compactSummaryTextFromResponses(output []apicompat.ResponsesOutput) string {
	var parts []string
	for _, item := range output {
		if item.Type != "message" {
			continue
		}
		for _, part := range item.Content {
			if text := strings.TrimSpace(part.Text); text != "" {
				parts = append(parts, text)
			}
		}
	}
	return strings.Join(parts, "\n")
}

// buildDeepSeekCompactResponse replaces a normal Responses output with a single
// remote-compaction item carrying the summary. DeepSeek has no encrypted
// reasoning, so the item carries only the visible summary (summary-only
// compaction is accepted by Codex; encrypted_content stays absent).
func buildDeepSeekCompactResponse(resp *apicompat.ResponsesResponse, summary string) *apicompat.ResponsesResponse {
	out := *resp
	out.Status = "completed"
	out.Output = []apicompat.ResponsesOutput{{
		Type:   "compaction",
		ID:     "cmp_" + strings.ReplaceAll(uuid.NewString(), "-", ""),
		Status: "completed",
		// Codex only emits ResponseEvent::OutputItemDone for a compaction item
		// when the wire object carries encrypted_content. DeepSeek has no
		// encrypted reasoning channel, so retain the visible summary as the
		// opaque compaction payload accepted by the client.
		EncryptedContent: strings.TrimSpace(summary),
		Summary: []apicompat.ResponsesSummary{{
			Type: "summary_text",
			Text: strings.TrimSpace(summary),
		}},
	}}
	return &out
}
