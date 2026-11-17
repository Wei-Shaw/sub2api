package service

import (
	"bytes"
	"encoding/json"

	"github.com/tidwall/gjson"
)

// openAIResponsesInputMayNeedOrphanCleanup only proves that cleanup is a no-op.
// Possible outputs still use the existing decoder and sanitizer, including its
// named standalone outputs, call/id fallback and item-reference rules. All byte
// spans below borrow the immutable request only for this synchronous call.
func openAIResponsesInputMayNeedOrphanCleanup(body []byte) bool {
	// Use Get directly: Parse(...).Get(...) differs from GetBytes on malformed
	// prefixes, where the old gate can still find an input and invoke the decoder.
	input := gjson.Get(openAIWSPayloadStringView(body), "input")
	if !input.IsArray() {
		return false
	}
	if !json.Valid(body) {
		// Preserve the existing decoder's malformed/trailing-JSON error path.
		return true
	}
	if body[skipOpenAIWSJSONSpace(body, 0)] != '{' || input.Index <= 0 || input.Index > len(body)-len(input.Raw) {
		return true
	}

	hasPreviousResponseID := false
	inputSeen, previousResponseIDSeen := false, false
	if !visitOpenAIResponsesOrphanPrecheckFields(body, func(key, value []byte) bool {
		switch string(key) {
		case `"input"`:
			if inputSeen {
				return false
			}
			inputSeen = true
		case `"previous_response_id"`:
			if previousResponseIDSeen {
				return false
			}
			previousResponseIDSeen = true
			if value[0] == '"' {
				if bytes.IndexByte(value, '\\') >= 0 {
					return false
				}
				hasPreviousResponseID = len(bytes.TrimSpace(value[1:len(value)-1])) != 0
			}
		}
		return true
	}) {
		// GJSON selects first duplicate keys; encoding/json selects the last.
		// Escaped keys and duplicate relevant keys conservatively use the latter.
		return true
	}
	if hasPreviousResponseID {
		return false
	}

	rawInput := body[input.Index : input.Index+len(input.Raw)]
	for i := 1; ; {
		i = skipOpenAIWSJSONSpace(rawInput, i)
		if rawInput[i] == ']' {
			return false
		}
		start := i
		switch rawInput[i] {
		case '{', '[', '"':
			i = skipOpenAIWSJSONValue(rawInput, i)
		default:
			// The shared object-value scanner stops at '}', whereas scalar
			// array members end at ']'. No scalar item can contain a tool output.
			for rawInput[i] != ',' && rawInput[i] != ']' {
				i++
			}
		}
		if rawInput[start] == '{' {
			typeSeen := false
			if !visitOpenAIResponsesOrphanPrecheckFields(rawInput[start:i], func(key, value []byte) bool {
				if string(key) != `"type"` {
					return true
				}
				if typeSeen {
					return false
				}
				typeSeen = true
				if value[0] != '"' {
					return true
				}
				if bytes.IndexByte(value, '\\') >= 0 {
					return false
				}
				return !isCodexToolCallOutputItemType(parseRawJSONView(value).Str)
			}) {
				return true
			}
		}
		i = skipOpenAIWSJSONSpace(rawInput, i)
		if rawInput[i] == ',' {
			i++
		}
	}
}

// visitOpenAIResponsesOrphanPrecheckFields visits an already-validated object
// without materializing unrelated strings (including large escaped content).
// Escaped keys are deliberately left to the full decoder rather than decoded
// into potentially request-sized temporary strings here.
func visitOpenAIResponsesOrphanPrecheckFields(object []byte, visit func(key, value []byte) bool) bool {
	i := skipOpenAIWSJSONSpace(object, 0) + 1
	for {
		i = skipOpenAIWSJSONSpace(object, i)
		if object[i] == '}' {
			return true
		}
		keyStart := i
		keyEnd := scanOpenAIWSJSONString(object, i)
		key := object[keyStart:keyEnd]
		if bytes.IndexByte(key, '\\') >= 0 {
			return false
		}
		i = skipOpenAIWSJSONSpace(object, keyEnd) + 1 // Valid JSON guarantees ':'.
		valueStart := skipOpenAIWSJSONSpace(object, i)
		i = skipOpenAIWSJSONValue(object, valueStart)
		if !visit(key, bytes.TrimSpace(object[valueStart:i])) {
			return false
		}
		i = skipOpenAIWSJSONSpace(object, i)
		if object[i] == ',' {
			i++
		}
	}
}
