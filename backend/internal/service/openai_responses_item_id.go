package service

import (
	"errors"
	"fmt"
	"strings"

	"github.com/tidwall/gjson"
)

func openAIResponsesInputItemIDPrefix(itemType string) (string, bool) {
	switch strings.TrimSpace(itemType) {
	case "message":
		return "msg", true
	case "reasoning":
		return "rs", true
	case "web_search_call":
		return "ws", true
	case "custom_tool_call":
		return openAIResponsesToolCallIDPrefix(itemType), true
	case "tool_search_call":
		return openAIResponsesToolCallIDPrefix(itemType), true
	case "custom_tool_call_output":
		// Although custom calls use ctc IDs, OpenAI validates replayed custom
		// call output item IDs against the generic fc namespace.
		return "fc", true
	default:
		if isCodexToolCallInputType(itemType) {
			return openAIResponsesToolCallIDPrefix(itemType), true
		}
		return "", false
	}
}

func openAIResponsesToolCallIDPrefix(itemType string) string {
	switch strings.TrimSpace(itemType) {
	case "custom_tool_call", "custom_tool_call_output":
		return "ctc"
	case "tool_search_call", "tool_search_output":
		return "tsc"
	default:
		return "fc"
	}
}

// Invalid replayed IDs are removed rather than rewritten because a fabricated
// ID may point at a different upstream object.
func shouldStripOpenAIResponsesInputItemID(itemType, id string) bool {
	prefix, constrained := openAIResponsesInputItemIDPrefix(itemType)
	if !constrained {
		return false
	}
	return id == "" || len(id) > 64 || !strings.HasPrefix(id, prefix)
}

func shouldStripOpenAIResponsesNonPairCallID(itemType string) bool {
	switch strings.TrimSpace(itemType) {
	case "message", "reasoning", "image_generation_call":
		return true
	default:
		return false
	}
}

// openAIResponsesInputItemSanitization reports which identifiers of one input
// item have to be removed.
func openAIResponsesInputItemSanitization(item gjson.Result) (stripID, stripCallID bool) {
	if !item.IsObject() {
		return false, false
	}
	itemType := strings.TrimSpace(item.Get("type").String())
	stripCallID = item.Get("call_id").Exists() && shouldStripOpenAIResponsesNonPairCallID(itemType)
	if id := item.Get("id"); id.Type == gjson.String {
		stripID = shouldStripOpenAIResponsesInputItemID(itemType, id.String())
	}
	return stripID, stripCallID
}

func sanitizeOpenAIResponsesInputItemIDs(body []byte) ([]byte, bool, error) {
	input := parseRawJSONView(body).Get("input")
	if !input.IsArray() {
		return body, false, nil
	}

	hasSanitization := false
	input.ForEach(func(_, item gjson.Result) bool {
		stripID, stripCallID := openAIResponsesInputItemSanitization(item)
		hasSanitization = stripID || stripCallID
		return !hasSanitization
	})
	if !hasSanitization {
		return body, false, nil
	}

	// Codex replays tool outputs with IDs that have to be stripped, and those
	// outputs (including images) are most of the body. Write the result once:
	// unchanged items are copied as they are and changed items are rewritten
	// straight into the result, so no item is copied more than once. Removing
	// members and the whitespace between items never makes the body longer.
	result := make([]byte, 0, len(body))
	result = append(result, body[:input.Index]...)
	result = append(result, '[')
	index := 0
	var err error
	input.ForEach(func(_, item gjson.Result) bool {
		if index > 0 {
			result = append(result, ',')
		}
		if stripID, stripCallID := openAIResponsesInputItemSanitization(item); stripID || stripCallID {
			result, err = appendOpenAIResponsesInputItemWithoutIDs(result, item.Raw, stripID, stripCallID)
			if err != nil {
				err = fmt.Errorf("sanitize input.%d: %w", index, err)
				return false
			}
		} else {
			result = append(result, item.Raw...)
		}
		index++
		return true
	})
	if err != nil {
		return nil, false, err
	}
	result = append(result, ']')
	return append(result, body[input.Index+len(input.Raw):]...), true, nil
}

var errMalformedOpenAIResponsesInputItem = errors.New("malformed input item")

// appendOpenAIResponsesInputItemWithoutIDs appends the raw object item without
// its first "id" member (stripID) and its first "call_id" member
// (stripCallID), which is what sjson.DeleteBytes removes. The other members
// keep their raw key and value bytes; whitespace between members is dropped.
// Members are scanned rather than visited with gjson's ForEach, which would
// unescape every string value and so copy large tool outputs once more.
func appendOpenAIResponsesInputItemWithoutIDs(dst []byte, item string, stripID, stripCallID bool) ([]byte, error) {
	start := len(dst)
	i := skipRawJSONSpace(item, 0)
	if i == len(item) || item[i] != '{' {
		return dst[:start], errMalformedOpenAIResponsesInputItem
	}
	dst = append(dst, '{')
	members := 0
	i = skipRawJSONSpace(item, i+1)
	if i < len(item) && item[i] == '}' {
		i++
	} else {
		for {
			keyStart := i
			keyEnd, ok := rawJSONStringEnd(item, keyStart)
			if !ok {
				return dst[:start], errMalformedOpenAIResponsesInputItem
			}
			i = skipRawJSONSpace(item, keyEnd)
			if i == len(item) || item[i] != ':' {
				return dst[:start], errMalformedOpenAIResponsesInputItem
			}
			valueStart := skipRawJSONSpace(item, i+1)
			valueEnd, ok := rawJSONValueEnd(item, valueStart)
			if !ok {
				return dst[:start], errMalformedOpenAIResponsesInputItem
			}
			key := item[keyStart+1 : keyEnd-1]
			if strings.IndexByte(key, '\\') >= 0 {
				key = gjson.Parse(item[keyStart:keyEnd]).Str
			}
			switch {
			case stripID && key == "id":
				stripID = false
			case stripCallID && key == "call_id":
				stripCallID = false
			default:
				if members > 0 {
					dst = append(dst, ',')
				}
				dst = append(dst, item[keyStart:keyEnd]...)
				dst = append(dst, ':')
				dst = append(dst, item[valueStart:valueEnd]...)
				members++
			}
			i = skipRawJSONSpace(item, valueEnd)
			if i < len(item) && item[i] == ',' {
				i = skipRawJSONSpace(item, i+1)
				continue
			}
			if i == len(item) || item[i] != '}' {
				return dst[:start], errMalformedOpenAIResponsesInputItem
			}
			i++
			break
		}
	}
	if skipRawJSONSpace(item, i) != len(item) {
		return dst[:start], errMalformedOpenAIResponsesInputItem
	}
	return append(dst, '}'), nil
}
