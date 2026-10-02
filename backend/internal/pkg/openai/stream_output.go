package openai

import (
	"strings"

	"github.com/tidwall/gjson"
)

// ResponsesStreamHasOutput reports model content, not transport activity or
// structural progress. Semantic TTFT includes encrypted reasoning/compaction;
// visible TTFT does not. Whitespace is a valid token, so do not trim payloads.
// This must not be used as the general client-commit or retry boundary.
func ResponsesStreamHasOutput(data, eventType string, includeEncrypted bool) bool {
	if !gjson.Valid(data) {
		return false
	}
	payload := gjson.Parse(data)
	eventType = strings.TrimSpace(eventType)
	if eventType == "" {
		eventType = strings.TrimSpace(payload.Get("type").Str)
	}
	if strings.HasPrefix(eventType, "response.") && strings.HasSuffix(eventType, ".delta") {
		return hasOutputString(payload, "delta")
	}
	switch eventType {
	case "response.output_text.done", "response.reasoning_summary_text.done",
		"response.reasoning_text.done", "response.audio_transcript.done", "response.output_audio_transcript.done":
		return hasOutputString(payload, "text")
	case "response.refusal.done":
		return hasOutputString(payload, "refusal")
	case "response.function_call_arguments.done":
		return hasOutputString(payload, "arguments")
	case "response.custom_tool_call_input.done":
		return hasOutputString(payload, "input")
	case "response.code_interpreter_call_code.done":
		return hasOutputString(payload, "code")
	case "response.image_generation_call.partial_image":
		return hasOutputString(payload, "partial_image_b64")
	case "response.content_part.added", "response.content_part.done",
		"response.reasoning_summary_part.added", "response.reasoning_summary_part.done":
		return partHasOutput(payload.Get("part"))
	case "response.output_item.added", "response.output_item.done":
		return itemHasOutput(payload.Get("item"), includeEncrypted)
	case "response.completed", "response.done":
		output := payload.Get("response.output")
		if output.IsArray() {
			for _, item := range output.Array() {
				if itemHasOutput(item, includeEncrypted) {
					return true
				}
			}
		}
	}
	return false
}

func hasOutputString(value gjson.Result, paths ...string) bool {
	for _, path := range paths {
		field := value.Get(path)
		if field.Type == gjson.String && field.Str != "" {
			return true
		}
	}
	return false
}

func partHasOutput(part gjson.Result) bool {
	return hasOutputString(part, "text", "refusal", "transcript", "audio", "data")
}

func itemHasOutput(item gjson.Result, includeEncrypted bool) bool {
	if hasOutputString(item, "arguments", "input", "result", "code") {
		return true
	}
	if includeEncrypted && hasOutputString(item, "encrypted_content") {
		return true
	}
	for _, path := range []string{"content", "summary"} {
		parts := item.Get(path)
		if parts.IsArray() {
			for _, part := range parts.Array() {
				if partHasOutput(part) {
					return true
				}
			}
		}
	}
	return false
}
