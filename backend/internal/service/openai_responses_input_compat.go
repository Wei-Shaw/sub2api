package service

import (
	"encoding/json"
	"sort"
	"strings"
)

const openAIResponsesInputTextMaxChars = 10000000

// sanitizeOpenAIResponsesOrphanToolOutputs removes tool-output items that have
// no matching call or item reference anywhere in the current input. Named
// function outputs without a call ID are standalone inputs, not orphan results.
func sanitizeOpenAIResponsesOrphanToolOutputs(reqBody map[string]any, input []any, hasPreviousResponseID bool) bool {
	return sanitizeOpenAIResponsesOrphanToolOutputsWithPolicy(reqBody, input, hasPreviousResponseID, false, nil)
}

// strictHTTP rejects wire-invalid IDs even when server history is available.
// Native WS retains its named delegation contract. emit preserves raw items.
func sanitizeOpenAIResponsesOrphanToolOutputsWithPolicy(reqBody map[string]any, input []any, hasPreviousResponseID, strictHTTP bool, emit func(int, any)) bool {
	if len(input) == 0 || (hasPreviousResponseID && !strictHTTP) {
		return false
	}

	toolCallIDs := make(map[string]struct{}, len(input))
	referenceIDs := make(map[string]struct{}, len(input))
	for _, rawItem := range input {
		item, ok := rawItem.(map[string]any)
		if !ok {
			continue
		}
		itemType := strings.TrimSpace(firstNonEmptyString(item["type"]))
		if itemType == "item_reference" {
			if id := strings.TrimSpace(firstNonEmptyString(item["id"])); id != "" {
				referenceIDs[id] = struct{}{}
			}
			continue
		}
		if !isCodexToolCallContextItemType(itemType) {
			continue
		}
		if id := strings.TrimSpace(firstNonEmptyString(item["call_id"], item["id"])); id != "" {
			toolCallIDs[id] = struct{}{}
		}
	}

	modified := false
	normalized := make([]any, 0, len(input))
	for index, rawItem := range input {
		keep := func(replacement any) {
			if emit != nil {
				emit(index, replacement)
			}
			if replacement != nil {
				normalized = append(normalized, replacement)
			} else {
				normalized = append(normalized, rawItem)
			}
		}
		item, ok := rawItem.(map[string]any)
		if !ok || !isCodexToolCallOutputItemType(strings.TrimSpace(firstNonEmptyString(item["type"]))) {
			keep(nil)
			continue
		}

		callID := strings.TrimSpace(firstNonEmptyString(item["call_id"]))
		// Codex sends externally supplied inputs (such as task delegation) as
		// named function outputs without a preceding function call. Preserve
		// their native type, namespace and output instead of silently dropping
		// the request that started the turn.
		if callID == "" && strings.TrimSpace(firstNonEmptyString(item["type"])) == "function_call_output" &&
			strings.TrimSpace(firstNonEmptyString(item["name"])) != "" {
			if !strictHTTP {
				keep(nil)
				continue
			}
			// Quote arbitrary structured output as exact JSON data. For text,
			// keep the original text as its own block without truncation.
			output, ok := item["output"].(string)
			if !ok {
				raw, err := json.Marshal(item["output"])
				if err == nil {
					output = string(raw)
				}
			}
			context, _ := json.Marshal(map[string]any{"name": item["name"], "namespace": item["namespace"]})
			keep(map[string]any{"type": "message", "role": "user", "content": []any{
				map[string]any{"type": "input_text", "text": "[Recovered tool/delegation context; quoted output data] " + string(context)},
				map[string]any{"type": "input_text", "text": output},
			}})
			modified = true
			continue
		}
		_, hasToolCall := toolCallIDs[callID]
		_, hasReference := referenceIDs[callID]
		if callID != "" && (hasToolCall || hasReference || hasPreviousResponseID) {
			keep(nil)
			continue
		}

		modified = true
	}
	if !modified {
		return false
	}
	reqBody["input"] = normalized
	return true
}

func truncateOpenAIResponsesInputText(_ map[string]any) bool {
	// Do not silently rewrite client or tool output. If an upstream enforces a
	// text limit, forwarding the original value preserves its explicit error for
	// the client and the normal Ops error pipeline. This compatibility shim is
	// retained until the two callers can remove the old mutation hook together.
	return false
}

func openAIResponsesInputMayNeedTruncation(_ []byte) bool {
	// See truncateOpenAIResponsesInputText. Returning false also avoids decoding
	// very large bodies solely for a mutation that must not happen.
	return false
}

// sanitizeStrictHTTPResponsesBody patches only input; untouched items retain
// their raw JSON, including encrypted provider data and unknown numeric fields.
func sanitizeStrictHTTPResponsesBody(body []byte) ([]byte, bool, error) {
	if !openAIResponsesInputMayNeedOrphanCleanup(body, true) {
		return body, false, nil
	}
	var envelope map[string]json.RawMessage
	if err := json.Unmarshal(body, &envelope); err != nil {
		return body, false, err
	}
	var rawInput []json.RawMessage
	if len(envelope["input"]) == 0 || json.Unmarshal(envelope["input"], &rawInput) != nil {
		return body, false, nil
	}
	var decoded map[string]any
	if err := decodeOpenAIJSONUseNumber(body, &decoded); err != nil {
		return body, false, err
	}
	input, ok := decoded["input"].([]any)
	if !ok {
		return body, false, nil
	}
	repaired := make([]json.RawMessage, 0, len(rawInput))
	changed := sanitizeOpenAIResponsesOrphanToolOutputsWithPolicy(decoded, input,
		strings.TrimSpace(firstNonEmptyString(decoded["previous_response_id"])) != "", true,
		func(index int, replacement any) {
			if replacement == nil {
				repaired = append(repaired, rawInput[index])
				return
			}
			raw, _ := json.Marshal(replacement)
			repaired = append(repaired, raw)
		})
	if !changed {
		return body, false, nil
	}
	// Marshal of RawMessage compacts its bytes. Assemble the repaired array
	// directly so provider-owned input items retain their original JSON.
	rawArray := []byte{'['}
	for i, item := range repaired {
		if i > 0 {
			rawArray = append(rawArray, ',')
		}
		rawArray = append(rawArray, item...)
	}
	envelope["input"] = append(rawArray, ']')
	// Rebuild unique top-level keys from encoding/json's last-key-wins view.
	// Patching the first matching key would leave a later duplicate input intact.
	keys := make([]string, 0, len(envelope))
	for key := range envelope {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	next := []byte{'{'}
	for i, key := range keys {
		if i > 0 {
			next = append(next, ',')
		}
		encodedKey, _ := json.Marshal(key)
		next = append(next, encodedKey...)
		next = append(next, ':')
		next = append(next, envelope[key]...)
	}
	next = append(next, '}')
	var err error
	next, _, err = NormalizeCompactionTriggerInputOrder(next)
	return next, err == nil, err
}
