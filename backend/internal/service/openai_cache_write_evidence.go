package service

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io"
	"unicode/utf8"
)

const (
	openAICacheWriteEvidenceMaxBodyBytes = 4 << 20
	openAICacheWriteEvidenceMaxItems     = 256
	openAICacheWriteEvidenceMaxDepth     = 64
	openAICacheWriteEvidenceMaxNodes     = 262144
)

type openAICacheWriteEvidenceKind uint8

const (
	openAICacheWriteEvidenceUnknown openAICacheWriteEvidenceKind = iota
	openAICacheWriteEvidenceUser
	openAICacheWriteEvidenceAssistant
	openAICacheWriteEvidenceSystem
	openAICacheWriteEvidenceDeveloper
	openAICacheWriteEvidenceFunctionCall
	openAICacheWriteEvidenceFunctionResult
	openAICacheWriteEvidenceReasoning
)

// Evidence contains only hashes and structural tags, never prompt, completion,
// function arguments, result text, or raw identifiers. Its caller must keep it
// local and expire it with the bounded-lifetime inference tracker.
type openAICacheWriteItemEvidence struct {
	kind       openAICacheWriteEvidenceKind
	callIDHash string
}

type openAICacheWritePromptEvidence struct {
	ConfigHash  string
	InputHashes []string
	Valid       bool
	items       []openAICacheWriteItemEvidence
}

type openAICacheWriteOutputEvidence struct {
	OutputHashes []string
	Valid        bool
	items        []openAICacheWriteItemEvidence
}

// buildOpenAICacheWritePromptEvidence supports explicit, full-history Responses
// requests only. Stateful references, automatic truncation, prompt templates,
// compaction, multimodal input, and unknown input/configuration shapes fail
// closed. Configuration is hashed in full except known stream transport knobs.
func buildOpenAICacheWritePromptEvidence(body []byte) openAICacheWritePromptEvidence {
	value, ok := decodeOpenAICacheWriteEvidenceJSON(body)
	if !ok {
		return openAICacheWritePromptEvidence{}
	}
	request, ok := value.(map[string]any)
	if !ok {
		return openAICacheWritePromptEvidence{}
	}
	if model, ok := request["model"].(string); !ok || model == "" {
		return openAICacheWritePromptEvidence{}
	}
	config := make(map[string]any, len(request))
	for key, value := range request {
		switch key {
		case "input":
			continue
		case "type":
			// WebSocket response.create is only a transport envelope. Never
			// normalize arbitrary event types into a Responses request.
			if value != "response.create" {
				return openAICacheWritePromptEvidence{}
			}
			continue
		case "stream":
			if _, ok := value.(bool); !ok {
				return openAICacheWritePromptEvidence{}
			}
			continue
		case "stream_options":
			options, ok := value.(map[string]any)
			if !ok || !openAICacheWriteEvidenceOnlyKeys(options, "include_obfuscation") {
				return openAICacheWritePromptEvidence{}
			}
			if option, exists := options["include_obfuscation"]; exists {
				if _, ok := option.(bool); !ok {
					return openAICacheWritePromptEvidence{}
				}
			}
			continue
		case "previous_response_id", "conversation", "prompt", "context_management":
			// Even unchanged opaque references can resolve to different upstream
			// history. Null is harmless, but is still included in the config hash.
			if value != nil {
				return openAICacheWritePromptEvidence{}
			}
		case "truncation":
			if value != nil && value != "disabled" {
				return openAICacheWritePromptEvidence{}
			}
		case "generate":
			// The WS builder uses generate=false for prewarming. That is not a
			// completed generation eligible for attribution to an observed turn.
			if value != true {
				return openAICacheWritePromptEvidence{}
			}
		case "model", "instructions", "tools", "tool_choice", "parallel_tool_calls",
			"temperature", "top_p", "top_logprobs", "max_output_tokens", "max_tool_calls", "reasoning",
			"text", "include", "service_tier", "prompt_cache_key", "prompt_cache_retention",
			"store", "metadata", "safety_identifier", "user", "background":
			// Preserve every nested field and number exactly. New/unknown fields
			// inside known configuration are not discarded by normalization.
		case "client_metadata":
			metadata, ok := canonicalOpenAICacheWriteClientMetadata(value)
			if !ok {
				return openAICacheWritePromptEvidence{}
			}
			value = metadata
		case "prompt_cache_options":
			// Cache mode/TTL/breakpoints and all unknown nested options are
			// preserved in full, never treated as transport-only metadata.
		default:
			return openAICacheWritePromptEvidence{}
		}
		config[key] = value
	}

	input := request["input"]
	if text, ok := input.(string); ok {
		input = []any{map[string]any{"role": "user", "content": text}}
	}
	items, ok := input.([]any)
	if !ok || len(items) == 0 || len(items) > openAICacheWriteEvidenceMaxItems {
		return openAICacheWritePromptEvidence{}
	}
	hashes, metadata, ok := buildOpenAICacheWriteItemEvidence(items, false)
	if !ok {
		return openAICacheWritePromptEvidence{}
	}
	return openAICacheWritePromptEvidence{
		ConfigHash:  openAICacheWriteEvidenceHash("config", config),
		InputHashes: hashes, Valid: true, items: metadata,
	}
}

// canonicalOpenAICacheWriteClientMetadata removes only three verified Codex
// transport-correlation paths: client_metadata.turn_id, and turn_id plus
// turn_started_at_unix_ms within its exact x-codex-turn-metadata JSON string.
// resolveCodexFingerprintIDs creates their UUID/time independently of prompt
// content; applyCodexFingerprintToClientMetadataMap and
// rewriteClientMetadataEmbeddedTurnMetadata mirror them into HTTP/WS metadata.
// Session/thread/installation/window IDs, request_kind, all unknown fields, and
// even the unverified flat turn_started_at_unix_ms path remain significant.
// This changes only the temporary hash input, never the outgoing request.
func canonicalOpenAICacheWriteClientMetadata(value any) (map[string]any, bool) {
	metadata, ok := value.(map[string]any)
	if !ok {
		return nil, false
	}
	canonical := make(map[string]any, len(metadata))
	for key, value := range metadata {
		switch key {
		case "turn_id":
			if _, ok := value.(string); !ok {
				return nil, false
			}
			continue
		case "x-codex-turn-metadata":
			raw, ok := value.(string)
			if !ok {
				return nil, false
			}
			decoded, ok := decodeOpenAICacheWriteEvidenceJSON([]byte(raw))
			if !ok {
				return nil, false
			}
			embedded, ok := decoded.(map[string]any)
			if !ok {
				return nil, false
			}
			if turnID, exists := embedded["turn_id"]; exists {
				if _, ok := turnID.(string); !ok {
					return nil, false
				}
				delete(embedded, "turn_id")
			}
			if timestamp, exists := embedded["turn_started_at_unix_ms"]; exists {
				number, ok := timestamp.(json.Number)
				if !ok {
					return nil, false
				}
				milliseconds, err := number.Int64()
				if err != nil || milliseconds < 0 {
					return nil, false
				}
				delete(embedded, "turn_started_at_unix_ms")
			}
			value = embedded
		}
		canonical[key] = value
	}
	return canonical, true
}

// buildOpenAICacheWriteOutputEvidence is called only for a complete HTTP
// response or terminal response.completed/response.done event, before lossy
// protocol conversion. Missing output differs from an explicit empty output[].
// Unknown/partial output invalidates the entire evidence, never just one item.
func buildOpenAICacheWriteOutputEvidence(body []byte) openAICacheWriteOutputEvidence {
	value, ok := decodeOpenAICacheWriteEvidenceJSON(body)
	if !ok {
		return openAICacheWriteOutputEvidence{}
	}
	response, ok := value.(map[string]any)
	if !ok {
		return openAICacheWriteOutputEvidence{}
	}
	if eventType, exists := response["type"]; exists {
		if eventType != "response.completed" && eventType != "response.done" {
			return openAICacheWriteOutputEvidence{}
		}
		if !openAICacheWriteEvidenceCompleted(response) {
			return openAICacheWriteOutputEvidence{}
		}
		response, ok = response["response"].(map[string]any)
		if !ok {
			return openAICacheWriteOutputEvidence{}
		}
	}
	if !openAICacheWriteEvidenceCompleted(response) {
		return openAICacheWriteOutputEvidence{}
	}
	items, ok := response["output"].([]any)
	if !ok || len(items) > openAICacheWriteEvidenceMaxItems {
		return openAICacheWriteOutputEvidence{}
	}
	hashes, metadata, ok := buildOpenAICacheWriteItemEvidence(items, true)
	if !ok {
		return openAICacheWriteOutputEvidence{}
	}
	return openAICacheWriteOutputEvidence{OutputHashes: hashes, Valid: true, items: metadata}
}

// openAICacheWritePromptsIdentical establishes payload equality only. The caller
// must independently prove local non-overlapping admission/completion order;
// neither identical input nor direct lineage proves a real upstream cache write.
func openAICacheWritePromptsIdentical(previous, next openAICacheWritePromptEvidence) bool {
	return previous.Valid && next.Valid && previous.ConfigHash != "" &&
		previous.ConfigHash == next.ConfigHash && len(previous.InputHashes) != 0 &&
		len(previous.InputHashes) == len(next.InputHashes) &&
		openAICacheWriteEvidencePrefix(next.InputHashes, previous.InputHashes)
}

// openAICacheWriteDirectSuccessor requires the exact prior input, every prior
// output item, then exactly one user message or one complete tool-result batch.
// It cannot accept an unobserved intervening assistant turn, skipped response,
// truncated prefix, or partial/different set of function results.
func openAICacheWriteDirectSuccessor(previous openAICacheWritePromptEvidence, output openAICacheWriteOutputEvidence, next openAICacheWritePromptEvidence) bool {
	if !previous.Valid || !output.Valid || !next.Valid || previous.ConfigHash == "" ||
		previous.ConfigHash != next.ConfigHash || len(previous.InputHashes) == 0 ||
		len(next.InputHashes) != len(next.items) || len(output.OutputHashes) != len(output.items) ||
		!openAICacheWriteEvidencePrefix(next.InputHashes, previous.InputHashes) {
		return false
	}
	remaining := next.InputHashes[len(previous.InputHashes):]
	if !openAICacheWriteEvidencePrefix(remaining, output.OutputHashes) {
		return false
	}
	tail := next.items[len(previous.InputHashes)+len(output.OutputHashes):]
	if len(tail) == 0 {
		return false
	}
	calls := make(map[string]bool)
	for _, item := range output.items {
		if item.kind != openAICacheWriteEvidenceFunctionCall {
			continue
		}
		if item.callIDHash == "" || calls[item.callIDHash] {
			return false
		}
		calls[item.callIDHash] = true
	}
	if len(calls) == 0 {
		return len(tail) == 1 && tail[0].kind == openAICacheWriteEvidenceUser
	}
	if len(tail) != len(calls) {
		return false
	}
	for _, item := range tail {
		if item.kind != openAICacheWriteEvidenceFunctionResult || !calls[item.callIDHash] {
			return false
		}
		delete(calls, item.callIDHash)
	}
	return len(calls) == 0
}

func openAICacheWriteEvidencePrefix(full, prefix []string) bool {
	if len(full) < len(prefix) {
		return false
	}
	for i, hash := range prefix {
		if hash == "" || full[i] != hash {
			return false
		}
	}
	return true
}

func buildOpenAICacheWriteItemEvidence(items []any, output bool) ([]string, []openAICacheWriteItemEvidence, bool) {
	hashes := make([]string, 0, len(items))
	metadata := make([]openAICacheWriteItemEvidence, 0, len(items))
	for _, value := range items {
		canonical, item, ok := canonicalOpenAICacheWriteEvidenceItem(value, output)
		if !ok {
			return nil, nil, false
		}
		hashes = append(hashes, openAICacheWriteEvidenceHash("item", canonical))
		metadata = append(metadata, item)
	}
	return hashes, metadata, true
}

func canonicalOpenAICacheWriteEvidenceItem(value any, output bool) (map[string]any, openAICacheWriteItemEvidence, bool) {
	item, ok := value.(map[string]any)
	if !ok || !openAICacheWriteEvidenceCompleted(item) {
		return nil, openAICacheWriteItemEvidence{}, false
	}
	if id, exists := item["id"]; exists {
		if _, ok := id.(string); !ok {
			return nil, openAICacheWriteItemEvidence{}, false
		}
	}
	typeName, exists := item["type"]
	if !exists {
		typeName = "message"
	}
	canonical := map[string]any{"type": typeName}
	metadata := openAICacheWriteItemEvidence{}
	switch typeName {
	case "message":
		if !openAICacheWriteEvidenceOnlyKeys(item, "type", "id", "status", "role", "content", "phase") {
			return nil, metadata, false
		}
		role, ok := item["role"].(string)
		if !ok || (output && role != "assistant") {
			return nil, metadata, false
		}
		switch role {
		case "user":
			metadata.kind = openAICacheWriteEvidenceUser
		case "assistant":
			metadata.kind = openAICacheWriteEvidenceAssistant
		case "system":
			metadata.kind = openAICacheWriteEvidenceSystem
		case "developer":
			metadata.kind = openAICacheWriteEvidenceDeveloper
		default:
			return nil, metadata, false
		}
		content, ok := canonicalOpenAICacheWriteTextContent(item["content"])
		if !ok {
			return nil, metadata, false
		}
		canonical["role"], canonical["content"] = role, content
		if phase, exists := item["phase"]; exists {
			if _, ok := phase.(string); !ok || role != "assistant" {
				return nil, metadata, false
			}
			canonical["phase"] = phase
		}
	case "function_call":
		if !openAICacheWriteEvidenceOnlyKeys(item, "type", "id", "status", "call_id", "name", "arguments", "namespace") {
			return nil, metadata, false
		}
		callID, ok := item["call_id"].(string)
		name, nameOK := item["name"].(string)
		arguments, argsOK := item["arguments"].(string)
		if !ok || callID == "" || !nameOK || name == "" || !argsOK {
			return nil, metadata, false
		}
		canonical["call_id"], canonical["name"], canonical["arguments"] = callID, name, arguments
		if namespace, exists := item["namespace"]; exists {
			if _, ok := namespace.(string); !ok {
				return nil, metadata, false
			}
			canonical["namespace"] = namespace
		}
		metadata.kind = openAICacheWriteEvidenceFunctionCall
		metadata.callIDHash = openAICacheWriteEvidenceHash("call_id", callID)
	case "function_call_output":
		if output || !openAICacheWriteEvidenceOnlyKeys(item, "type", "id", "status", "call_id", "output") {
			return nil, metadata, false
		}
		callID, ok := item["call_id"].(string)
		content, contentOK := canonicalOpenAICacheWriteTextContent(item["output"])
		if !ok || callID == "" || !contentOK {
			return nil, metadata, false
		}
		canonical["call_id"], canonical["output"] = callID, content
		metadata.kind = openAICacheWriteEvidenceFunctionResult
		metadata.callIDHash = openAICacheWriteEvidenceHash("call_id", callID)
	case "reasoning":
		if !openAICacheWriteEvidenceOnlyKeys(item, "type", "id", "status", "encrypted_content", "summary") {
			return nil, metadata, false
		}
		encrypted, ok := item["encrypted_content"].(string)
		if !ok || encrypted == "" {
			return nil, metadata, false
		}
		canonical["encrypted_content"] = encrypted
		if value, exists := item["summary"]; exists {
			summary, ok := value.([]any)
			if !ok || len(summary) > openAICacheWriteEvidenceMaxItems {
				return nil, metadata, false
			}
			for _, value := range summary {
				part, ok := value.(map[string]any)
				if !ok || !openAICacheWriteEvidenceOnlyKeys(part, "type", "text") || part["type"] != "summary_text" {
					return nil, metadata, false
				}
				if _, ok := part["text"].(string); !ok {
					return nil, metadata, false
				}
			}
			canonical["summary"] = summary
		}
		metadata.kind = openAICacheWriteEvidenceReasoning
	default:
		return nil, metadata, false
	}
	return canonical, metadata, true
}

func canonicalOpenAICacheWriteTextContent(value any) ([]any, bool) {
	if text, ok := value.(string); ok {
		return []any{map[string]any{"type": "text", "text": text}}, true
	}
	blocks, ok := value.([]any)
	if !ok || len(blocks) > openAICacheWriteEvidenceMaxItems {
		return nil, false
	}
	canonical := make([]any, 0, len(blocks))
	for _, value := range blocks {
		block, ok := value.(map[string]any)
		if !ok || !openAICacheWriteEvidenceOnlyKeys(block, "type", "text", "annotations") {
			return nil, false
		}
		switch block["type"] {
		case "text", "input_text", "output_text":
		default:
			return nil, false
		}
		text, ok := block["text"].(string)
		if !ok {
			return nil, false
		}
		if annotations, exists := block["annotations"]; exists {
			if _, ok := annotations.([]any); !ok {
				return nil, false
			}
		}
		canonical = append(canonical, map[string]any{"type": "text", "text": text})
	}
	return canonical, true
}

func openAICacheWriteEvidenceCompleted(object map[string]any) bool {
	if status, exists := object["status"]; exists && status != "completed" {
		return false
	}
	return object["error"] == nil && object["incomplete_details"] == nil
}

func openAICacheWriteEvidenceOnlyKeys(object map[string]any, allowed ...string) bool {
	for key := range object {
		found := false
		for _, candidate := range allowed {
			if key == candidate {
				found = true
				break
			}
		}
		if !found {
			return false
		}
	}
	return true
}

func openAICacheWriteEvidenceHash(domain string, value any) string {
	// All callers pass values produced by the restricted JSON decoder or
	// canonicalizer, so marshaling cannot encounter an unsupported type.
	encoded, err := json.Marshal(value)
	if err != nil {
		return ""
	}
	hasher := sha256.New()
	_, _ = hasher.Write([]byte("openai-cache-write-evidence/v1/" + domain + "\x00"))
	_, _ = hasher.Write(encoded)
	return hex.EncodeToString(hasher.Sum(nil))
}

// A strict decoder rejects duplicate keys rather than choosing a different
// interpretation from the upstream JSON parser. Depth, node count, item count,
// and byte limits bound work and retained metadata for adversarial requests.
func decodeOpenAICacheWriteEvidenceJSON(body []byte) (any, bool) {
	if len(body) == 0 || len(body) > openAICacheWriteEvidenceMaxBodyBytes ||
		!utf8.Valid(body) || !openAICacheWriteEvidenceValidUnicodeEscapes(body) {
		return nil, false
	}
	decoder := json.NewDecoder(bytes.NewReader(body))
	decoder.UseNumber()
	nodes := openAICacheWriteEvidenceMaxNodes
	value, ok := decodeOpenAICacheWriteEvidenceValue(decoder, 0, &nodes)
	if !ok {
		return nil, false
	}
	_, err := decoder.Token()
	return value, err == io.EOF
}

// encoding/json replaces unpaired UTF-16 surrogates with U+FFFD. Reject those
// ambiguous strings rather than hashing them as an actual replacement character.
func openAICacheWriteEvidenceValidUnicodeEscapes(body []byte) bool {
	inString := false
	for i := 0; i < len(body); i++ {
		if body[i] == '"' {
			inString = !inString
			continue
		}
		if !inString || body[i] != '\\' {
			continue
		}
		i++
		if i >= len(body) {
			return false
		}
		if body[i] != 'u' {
			continue
		}
		codepoint, ok := openAICacheWriteEvidenceHexCodepoint(body[i+1:])
		if !ok {
			return false
		}
		i += 4
		if codepoint >= 0xdc00 && codepoint <= 0xdfff {
			return false
		}
		if codepoint >= 0xd800 && codepoint <= 0xdbff {
			if i+6 >= len(body) || body[i+1] != '\\' || body[i+2] != 'u' {
				return false
			}
			low, ok := openAICacheWriteEvidenceHexCodepoint(body[i+3:])
			if !ok || low < 0xdc00 || low > 0xdfff {
				return false
			}
			i += 6
		}
	}
	return !inString
}

func openAICacheWriteEvidenceHexCodepoint(value []byte) (uint16, bool) {
	if len(value) < 4 {
		return 0, false
	}
	var result uint16
	for _, char := range value[:4] {
		result <<= 4
		switch {
		case char >= '0' && char <= '9':
			result |= uint16(char - '0')
		case char >= 'a' && char <= 'f':
			result |= uint16(char - 'a' + 10)
		case char >= 'A' && char <= 'F':
			result |= uint16(char - 'A' + 10)
		default:
			return 0, false
		}
	}
	return result, true
}

func decodeOpenAICacheWriteEvidenceValue(decoder *json.Decoder, depth int, nodes *int) (any, bool) {
	if depth > openAICacheWriteEvidenceMaxDepth || *nodes <= 0 {
		return nil, false
	}
	*nodes--
	token, err := decoder.Token()
	if err != nil {
		return nil, false
	}
	delim, compound := token.(json.Delim)
	if !compound {
		return token, true
	}
	switch delim {
	case '{':
		object := make(map[string]any)
		for decoder.More() {
			keyToken, err := decoder.Token()
			key, ok := keyToken.(string)
			if err != nil || !ok {
				return nil, false
			}
			if _, exists := object[key]; exists {
				return nil, false
			}
			value, ok := decodeOpenAICacheWriteEvidenceValue(decoder, depth+1, nodes)
			if !ok {
				return nil, false
			}
			object[key] = value
		}
		end, err := decoder.Token()
		return object, err == nil && end == json.Delim('}')
	case '[':
		array := make([]any, 0)
		for decoder.More() {
			value, ok := decodeOpenAICacheWriteEvidenceValue(decoder, depth+1, nodes)
			if !ok {
				return nil, false
			}
			array = append(array, value)
		}
		end, err := decoder.Token()
		return array, err == nil && end == json.Delim(']')
	default:
		return nil, false
	}
}
