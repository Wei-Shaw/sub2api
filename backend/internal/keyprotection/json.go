package keyprotection

import (
	"bytes"
	"encoding/json"
	"io"
	"strings"
	"unicode/utf8"
)

const maxContentDepth = 100

func decodeJSON(body []byte) (any, error) {
	if !utf8.Valid(body) {
		return nil, ErrContent
	}
	decoder := json.NewDecoder(bytes.NewReader(body))
	decoder.UseNumber()
	value, err := decodeJSONValue(decoder, 0)
	if err != nil {
		return nil, err
	}
	if _, err := decoder.Token(); err != io.EOF {
		return nil, ErrContent
	}
	return value, nil
}

func decodeJSONValue(decoder *json.Decoder, depth int) (any, error) {
	if depth > maxContentDepth {
		return nil, ErrContent
	}
	token, err := decoder.Token()
	if err != nil {
		return nil, ErrContent
	}
	delimiter, isDelimiter := token.(json.Delim)
	if !isDelimiter {
		return token, nil
	}
	switch delimiter {
	case '{':
		object := make(map[string]any)
		for decoder.More() {
			keyToken, err := decoder.Token()
			if err != nil {
				return nil, ErrContent
			}
			key, ok := keyToken.(string)
			if !ok {
				return nil, ErrContent
			}
			if _, duplicate := object[key]; duplicate {
				return nil, ErrContent
			}
			value, err := decodeJSONValue(decoder, depth+1)
			if err != nil {
				return nil, err
			}
			object[key] = value
		}
		end, err := decoder.Token()
		if err != nil || end != json.Delim('}') {
			return nil, ErrContent
		}
		return object, nil
	case '[':
		list := make([]any, 0)
		for decoder.More() {
			value, err := decodeJSONValue(decoder, depth+1)
			if err != nil {
				return nil, err
			}
			list = append(list, value)
		}
		end, err := decoder.Token()
		if err != nil || end != json.Delim(']') {
			return nil, ErrContent
		}
		return list, nil
	default:
		return nil, ErrContent
	}
}

// DecodeObject preserves JSON numbers, including integers larger than 2^53.
func DecodeObject(body []byte) (map[string]any, error) {
	value, err := decodeJSON(body)
	if err != nil {
		return nil, err
	}
	object, ok := value.(map[string]any)
	if !ok {
		return nil, ErrContent
	}
	return object, nil
}

type transformer struct {
	state   *State
	restore bool
}

func (t transformer) text(value string) (string, error) {
	if !t.restore {
		return t.state.ProtectText(value)
	}
	return t.state.RestoreText(value), nil
}

func (t transformer) field(object map[string]any, name string) error {
	value, exists := object[name]
	if !exists || value == nil {
		return nil
	}
	text, ok := value.(string)
	if !ok {
		return ErrContent
	}
	result, err := t.text(text)
	if err != nil {
		return err
	}
	object[name] = result
	return nil
}

// values handles user-owned arbitrary JSON (tool inputs/outputs), where field
// names such as "name" or "id" are user data rather than protocol identifiers.
func (t transformer) values(value any, depth int) (any, error) {
	if depth > maxContentDepth {
		return nil, ErrContent
	}
	switch v := value.(type) {
	case string:
		return t.text(v)
	case []any:
		for i, child := range v {
			transformed, err := t.values(child, depth+1)
			if err != nil {
				return nil, err
			}
			v[i] = transformed
		}
	case map[string]any:
		for key, child := range v {
			transformed, err := t.values(child, depth+1)
			if err != nil {
				return nil, err
			}
			v[key] = transformed
		}
	}
	return value, nil
}

// arguments round-trips the inner JSON separately; restoring quoted secrets in
// the serialized JSON string directly would corrupt quotes and backslashes.
func (t transformer) arguments(arguments string) (string, error) {
	if strings.TrimSpace(arguments) == "" {
		return arguments, nil
	}
	value, err := decodeJSON([]byte(arguments))
	if err != nil {
		return "", err
	}
	value, err = t.values(value, 0)
	if err != nil {
		return "", err
	}
	body, err := json.Marshal(value)
	if err != nil {
		return "", ErrContent
	}
	return string(body), nil
}

func (s *State) RestoreArguments(arguments string) (string, error) {
	return (transformer{state: s, restore: true}).arguments(arguments)
}

func (t transformer) argumentField(object map[string]any, name string, serialized bool) error {
	value, exists := object[name]
	if !exists || value == nil {
		return nil
	}
	if serialized {
		text, ok := value.(string)
		if !ok {
			return ErrContent
		}
		result, err := t.arguments(text)
		if err != nil {
			return err
		}
		object[name] = result
		return nil
	}
	result, err := t.values(value, 0)
	if err != nil {
		return err
	}
	object[name] = result
	return nil
}

func nonempty(value any) bool { return value != nil && value != "" }

// content only changes content fields. Opaque media are retained; encrypted or
// signed input is explicitly rejected because it cannot be safely rewritten.
func (t transformer) content(value any, depth int) (any, error) {
	if depth > maxContentDepth {
		return nil, ErrContent
	}
	switch v := value.(type) {
	case nil:
		return nil, nil
	case string:
		return t.text(v)
	case []any:
		for i, child := range v {
			result, err := t.content(child, depth+1)
			if err != nil {
				return nil, err
			}
			v[i] = result
		}
		return v, nil
	case map[string]any:
		kind, _ := v["type"].(string)
		// Anthropic thinking may acquire a signature after streaming starts.
		// Never rewrite its text in either response mode. Responses reasoning
		// summaries are separate visible fields; encrypted_content is opaque and
		// remains untouched while those summaries retain normal text handling.
		if t.restore && (nonempty(v["signature"]) || kind == "thinking" || kind == "redacted_thinking" || kind == "compaction" || kind == "encrypted_content") {
			return v, nil
		}
		if !t.restore && (nonempty(v["signature"]) || nonempty(v["encrypted_content"]) || kind == "redacted_thinking" || kind == "encrypted_content" || kind == "compaction") {
			return nil, ErrUnsupported
		}
		if !t.restore {
			if tools, exists := v["additional_tools"]; exists {
				result, err := t.definitions(tools, depth+1)
				if err != nil {
					return nil, err
				}
				v["additional_tools"] = result
			}
		}
		switch kind {
		case "text", "input_text", "output_text", "summary_text", "refusal", "thinking":
			for _, key := range []string{"text", "refusal", "thinking"} {
				if err := t.field(v, key); err != nil {
					return nil, err
				}
			}
		case "tool_use", "server_tool_use":
			if err := t.argumentField(v, "input", false); err != nil {
				return nil, err
			}
		case "function_call":
			if err := t.argumentField(v, "arguments", true); err != nil {
				return nil, err
			}
		case "custom_tool_call":
			if err := t.field(v, "input"); err != nil {
				return nil, err
			}
		case "tool_result", "function_call_output", "custom_tool_call_output":
			for _, key := range []string{"content", "output"} {
				if child, exists := v[key]; exists {
					// Tool output may be serialized JSON, plain text, or content blocks.
					var result any
					var err error
					if key == "content" {
						result, err = t.content(child, depth+1)
					} else {
						result, err = t.values(child, depth+1)
					}
					if err != nil {
						return nil, err
					}
					v[key] = result
				}
			}
		case "message", "reasoning":
			for _, key := range []string{"content", "summary"} {
				if child, exists := v[key]; exists {
					result, err := t.content(child, depth+1)
					if err != nil {
						return nil, err
					}
					v[key] = result
				}
			}
		case "image", "image_url", "input_image", "input_audio", "audio", "input_file", "file":
			// This feature makes no claim about credentials embedded in media.
			if err := t.field(v, "transcript"); err != nil {
				return nil, err
			}
		case "document":
			for _, key := range []string{"title", "context"} {
				if err := t.field(v, key); err != nil {
					return nil, err
				}
			}
			if source, ok := v["source"].(map[string]any); ok {
				if source["type"] == "text" {
					if err := t.field(source, "data"); err != nil {
						return nil, err
					}
				}
				if source["type"] == "content" {
					child, err := t.content(source["content"], depth+1)
					if err != nil {
						return nil, err
					}
					source["content"] = child
				}
			}
		case "redacted_thinking", "compaction":
			if !t.restore {
				return nil, ErrUnsupported
			}
		case "":
			// Messages in Responses input can omit their type.
			if _, exists := v["role"]; !exists {
				return nil, ErrContent
			}
			if child, exists := v["content"]; exists {
				result, err := t.content(child, depth+1)
				if err != nil {
					return nil, err
				}
				v["content"] = result
			}
		default:
			// New protocol content types require explicit integration. Unknown
			// output types are retained, without expanding restoration authority.
			if !t.restore {
				return nil, ErrUnsupported
			}
		}
		if !t.restore && kind != "image" && kind != "image_url" && kind != "input_image" && kind != "input_audio" && kind != "audio" && kind != "input_file" && kind != "file" {
			if err := t.rejectUnprocessed(v); err != nil {
				return nil, err
			}
		}
		return v, nil
	default:
		return nil, ErrContent
	}
}

func (t transformer) messages(value any) error {
	messages, ok := value.([]any)
	if !ok {
		return ErrContent
	}
	for _, item := range messages {
		message, ok := item.(map[string]any)
		if !ok {
			return ErrContent
		}
		if child, exists := message["content"]; exists {
			result, err := t.content(child, 0)
			if err != nil {
				return err
			}
			message["content"] = result
		}
		for _, key := range []string{"reasoning_content", "reasoning", "refusal"} {
			if err := t.field(message, key); err != nil {
				return err
			}
		}
		if err := t.chatTools(message); err != nil {
			return err
		}
		if !t.restore {
			if err := t.rejectUnprocessed(message); err != nil {
				return err
			}
		}
	}
	return nil
}

// Fail closed when an extension or a case-insensitive alias would be consumed
// by a downstream protocol decoder but was not transformed by this module.
func (t transformer) rejectUnprocessed(object map[string]any) error {
	if t.unprocessedSecret(object, 0) {
		return ErrUnsupported
	}
	return nil
}

func (t transformer) unprocessedSecret(value any, depth int) bool {
	if depth > maxContentDepth {
		return true
	}
	switch v := value.(type) {
	case string:
		return len(t.state.matches(v)) > 0
	case []any:
		for _, child := range v {
			if t.unprocessedSecret(child, depth+1) {
				return true
			}
		}
	case map[string]any:
		switch v["type"] {
		case "image", "image_url", "input_image", "input_audio", "audio", "input_file", "file":
			return false
		}
		for key, child := range v {
			switch key {
			case "id", "name", "role", "type", "model", "metadata", "call_id", "tool_call_id", "tool_use_id", "signature", "encrypted_content", "source", "image_url", "file", "audio", "input_audio":
				continue
			}
			if t.unprocessedSecret(child, depth+1) {
				return true
			}
		}
	}
	return false
}

func (t transformer) chatTools(message map[string]any) error {
	if function, ok := message["function_call"].(map[string]any); ok {
		if err := t.argumentField(function, "arguments", true); err != nil {
			return err
		}
	}
	if value, exists := message["tool_calls"]; exists && value != nil {
		calls, ok := value.([]any)
		if !ok {
			return ErrContent
		}
		for _, value := range calls {
			call, ok := value.(map[string]any)
			if !ok {
				return ErrContent
			}
			if function, ok := call["function"].(map[string]any); ok {
				if err := t.argumentField(function, "arguments", true); err != nil {
					return err
				}
			}
			if custom, ok := call["custom"].(map[string]any); ok {
				if err := t.field(custom, "input"); err != nil {
					return err
				}
			}
		}
	}
	return nil
}

func (t transformer) definitions(value any, depth int) (any, error) {
	if depth > maxContentDepth {
		return nil, ErrContent
	}
	switch v := value.(type) {
	case string:
		return t.text(v)
	case []any:
		for i, child := range v {
			result, err := t.definitions(child, depth+1)
			if err != nil {
				return nil, err
			}
			v[i] = result
		}
	case map[string]any:
		for key, child := range v {
			if _, scalar := child.(string); scalar {
				switch key {
				case "name", "type", "$id", "$ref", "$schema", "format":
					continue
				}
			}
			result, err := t.definitions(child, depth+1)
			if err != nil {
				return nil, err
			}
			v[key] = result
		}
	}
	return value, nil
}

// ProtectJSON runs after authentication and structural/size checks and before
// any prompt snapshot, moderation, logging, retry body, or model forwarding.
func (s *State) ProtectJSON(body []byte, protocol string) ([]byte, error) {
	if protocol != "chat" && protocol != "responses" && protocol != "messages" {
		return nil, ErrUnsupported
	}
	root, err := DecodeObject(body)
	if err != nil {
		return nil, err
	}
	// Only a complete client-supplied history can rebuild request-local maps.
	// Match the case-insensitive aliases accepted by downstream JSON decoders.
	for key, value := range root {
		switch strings.ToLower(key) {
		case "background":
			if value != false && value != nil {
				return nil, ErrUnsupported
			}
		case "previous_response_id", "conversation":
			if nonempty(value) {
				return nil, ErrUnsupported
			}
		}
	}
	t := transformer{state: s}
	processed := map[string]bool{"system": true, "instructions": true, "tools": true, "functions": true, "response_format": true, "text": true}
	if protocol == "responses" {
		processed["input"] = true
		if input, exists := root["input"]; exists {
			result, err := t.content(input, 0)
			if err != nil {
				return nil, err
			}
			root["input"] = result
		}
	} else {
		processed["messages"] = true
		if err := t.messages(root["messages"]); err != nil {
			return nil, err
		}
	}
	for _, key := range []string{"system", "instructions"} {
		if value, exists := root[key]; exists {
			result, err := t.content(value, 0)
			if err != nil {
				return nil, err
			}
			root[key] = result
		}
	}
	for _, key := range []string{"tools", "functions", "response_format", "text"} {
		if value, exists := root[key]; exists {
			result, err := t.definitions(value, 0)
			if err != nil {
				return nil, err
			}
			root[key] = result
		}
	}
	// Refuse a recognized secret in an unknown extension instead of quietly
	// forwarding an unprocessed field. These explicit exclusions are identifiers
	// and opaque protocol metadata, never a place to carry protected messages.
	for key, value := range root {
		if processed[key] {
			continue
		}
		switch key {
		case "model", "metadata", "user", "id", "previous_response_id", "prompt_cache_key", "safety_identifier", "tool_choice", "function_call", "service_tier", "request_id":
			continue
		}
		if s.containsSecret(value, 0) {
			return nil, ErrUnsupported
		}
	}
	result, err := json.Marshal(root)
	if err != nil {
		return nil, ErrContent
	}
	return result, nil
}

func (s *State) containsSecret(value any, depth int) bool {
	if depth > maxContentDepth {
		return true
	}
	switch v := value.(type) {
	case string:
		return len(s.matches(v)) > 0
	case []any:
		for _, child := range v {
			if s.containsSecret(child, depth+1) {
				return true
			}
		}
	case map[string]any:
		for _, child := range v {
			if s.containsSecret(child, depth+1) {
				return true
			}
		}
	}
	return false
}

// RestoreJSON is applied to the final client protocol, after protocol conversion.
// It only restores ordinary output content and tool input fields; IDs, metadata,
// usage, error details, and signed/encrypted fields never receive restoration.
func (s *State) RestoreJSON(body []byte, protocol string) ([]byte, error) {
	root, err := DecodeObject(body)
	if err != nil {
		return nil, err
	}
	t := transformer{state: s, restore: true}
	switch protocol {
	case "chat":
		if choices, exists := root["choices"]; exists {
			list, ok := choices.([]any)
			if !ok {
				return nil, ErrContent
			}
			for _, value := range list {
				choice, ok := value.(map[string]any)
				if !ok {
					return nil, ErrContent
				}
				for _, key := range []string{"message", "delta"} {
					if message, exists := choice[key]; exists && message != nil {
						if err := t.messages([]any{message}); err != nil {
							return nil, err
						}
					}
				}
				if err := t.field(choice, "text"); err != nil {
					return nil, err
				}
			}
		}
	case "responses":
		if output, exists := root["output"]; exists {
			result, err := t.content(output, 0)
			if err != nil {
				return nil, err
			}
			root["output"] = result
		}
		if err := t.field(root, "output_text"); err != nil {
			return nil, err
		}
	case "messages":
		if content, exists := root["content"]; exists {
			result, err := t.content(content, 0)
			if err != nil {
				return nil, err
			}
			root["content"] = result
		}
	default:
		return nil, ErrUnsupported
	}
	result, err := json.Marshal(root)
	if err != nil {
		return nil, ErrContent
	}
	return result, nil
}
