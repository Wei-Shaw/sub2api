package securityaudit

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"regexp"
	"sort"
	"strings"
	"unicode/utf8"
)

var (
	ErrNoPromptText = errors.New("prompt audit request contains no auditable text")

	bearerPattern = regexp.MustCompile(`(?i)\bBearer\s+[A-Za-z0-9._~+\-/]+=*`)
	apiKeyPattern = regexp.MustCompile(`(?i)\b(sk|rk|pk|api[_-]?key|token|secret|password)[-_:=\s]+[A-Za-z0-9._~+\-/]{8,}`)
	canaryPattern = regexp.MustCompile(`(?i)([A-Z]+_CANARY_)[A-Za-z0-9_-]+`)
	emailPattern  = regexp.MustCompile(`(?i)\b[A-Z0-9._%+\-]+@[A-Z0-9.\-]+\.[A-Z]{2,}\b`)
	phonePattern  = regexp.MustCompile(`(?:\+?\d[\d\s().-]{8,}\d)`)
)

const promptAuditPrioritySeparator = "\x00SUB2API_PROMPT_AUDIT_PRIORITY_END\x00"

type promptSegment struct {
	text    string
	kind    promptSegmentKind
	message int
}

type promptSegmentKind uint8

const (
	promptSegmentSystem promptSegmentKind = iota
	promptSegmentUser
	promptSegmentToolResult
	promptSegmentHistory
)

type promptExtraction struct {
	segments      []promptSegment
	nextMessage   int
	hasHistory    bool
	hasToolResult bool
}

func (e *promptExtraction) newMessage() int {
	message := e.nextMessage
	e.nextMessage++
	return message
}

func (e *promptExtraction) appendTexts(kind promptSegmentKind, texts []string) {
	e.appendTextsToMessage(kind, e.newMessage(), texts)
}

func (e *promptExtraction) appendTextsToMessage(kind promptSegmentKind, message int, texts []string) {
	for _, text := range texts {
		e.segments = append(e.segments, promptSegment{text: text, kind: kind, message: message})
	}
}

func (e *promptExtraction) appendToolResult(value any) {
	e.hasToolResult = true
	e.appendTexts(promptSegmentToolResult, toolResultTexts(value))
}

func ExtractPromptSnapshot(req Request) (PromptSnapshot, error) {
	var document any
	if err := json.Unmarshal(req.Body, &document); err != nil {
		return PromptSnapshot{}, errors.New("prompt audit request JSON is invalid")
	}
	extracted := extractProtocolSegments(req.Protocol, document)
	segments := selectAuditSegments(req.Stage, extracted)
	if len(segments) == 0 {
		return PromptSnapshot{}, ErrNoPromptText
	}
	scanText, metadataText := buildPrioritizedScanText(segments)
	digest := sha256.Sum256([]byte(metadataText))
	stage := strings.TrimSpace(req.Stage)
	if stage == "" {
		stage = "http"
	}
	return PromptSnapshot{
		RequestID: req.RequestID, UserID: req.UserID, UsernameSnapshot: req.Username,
		UserEmailSnapshot: req.UserEmail, APIKeyID: req.APIKeyID, APIKeyNameSnapshot: req.APIKeyName,
		GroupID: cloneInt64Ptr(req.GroupID), GroupName: req.GroupName, Provider: req.Provider,
		Endpoint: req.Endpoint, Protocol: req.Protocol, Model: req.Model,
		PromptHash: hex.EncodeToString(digest[:]), RedactedPreview: BuildPromptPreview(metadataText, DefaultPromptPreviewMaxRunes),
		FullPrompt:   BuildFullPrompt(metadataText, DefaultFullPromptMaxRunes),
		PromptLength: utf8.RuneCountInString(metadataText), MessageCount: len(segments), Stage: stage,
		ScanText: scanText,
	}, nil
}

// DefaultPromptPreviewMaxRunes caps how much sanitized prompt text may be
// considered before BuildPromptPreview withholds the majority for storage/UI.
const DefaultPromptPreviewMaxRunes = 96

// DefaultFullPromptMaxRunes caps how much unredacted prompt text is persisted
// on an audit event for admin review. It is deliberately generous so realistic
// prompts are kept intact while bounding per-row storage.
const DefaultFullPromptMaxRunes = 65536

func extractProtocolSegments(protocol string, document any) promptExtraction {
	var extracted promptExtraction
	root, _ := document.(map[string]any)
	protocol = strings.ToLower(strings.TrimSpace(protocol))
	switch protocol {
	case "openai_chat_completions", "openai_chat", "chat_completions":
		extractChatLikeSegments(&extracted, root)
	case "anthropic_messages", "claude_messages", "messages":
		extractAnthropicSystem(&extracted, root["system"])
		extractMessages(&extracted, root["messages"])
	case "gemini", "gemini_generate_content":
		extractGeminiRoot(&extracted, root)
	case "openai_responses", "responses", "responses_websocket":
		if frameType := stringValue(root["type"]); frameType != "" || protocol == "responses_websocket" {
			if frameType != "response.create" {
				return extracted
			}
			if input, exists := root["input"]; exists && input != nil {
				extractInstructions(&extracted, root["instructions"])
				extractResponses(&extracted, input)
				return extracted
			}
			if response, ok := root["response"].(map[string]any); ok {
				extractInstructions(&extracted, response["instructions"])
				extractResponses(&extracted, response["input"])
				return extracted
			}
			extractInstructions(&extracted, root["instructions"])
			return extracted
		}
		extractInstructions(&extracted, root["instructions"])
		extractResponses(&extracted, root["input"])
	case "openai_images", "grok_media", "media", "images":
		extracted.appendTexts(promptSegmentUser, extractMediaPrompts(root))
	default:
		extractChatLikeSegments(&extracted, root)
		if len(extracted.segments) > 0 || extracted.hasHistory || extracted.hasToolResult {
			return extracted
		}
		extractInstructions(&extracted, root["instructions"])
		extractResponses(&extracted, root["input"])
		if len(extracted.segments) > 0 || extracted.hasHistory || extracted.hasToolResult {
			return extracted
		}
		extractGeminiRoot(&extracted, root)
		if len(extracted.segments) > 0 || extracted.hasHistory || extracted.hasToolResult {
			return extracted
		}
		extracted.appendTexts(promptSegmentUser, extractMediaPrompts(root))
	}
	return extracted
}

func extractChatLikeSegments(extracted *promptExtraction, root map[string]any) {
	if root == nil {
		return
	}
	extractMessages(extracted, root["messages"])
}

func extractMessages(extracted *promptExtraction, value any) {
	items, ok := value.([]any)
	if !ok {
		return
	}
	for _, item := range items {
		message, ok := item.(map[string]any)
		if !ok {
			continue
		}
		role := strings.ToLower(stringValue(message["role"]))
		switch role {
		case "system", "developer":
			extracted.appendTexts(promptSegmentSystem, contentTexts(message["content"]))
		case "user":
			extractUserMessageContent(extracted, message["content"])
		case "tool":
			extracted.appendToolResult(message["content"])
		case "assistant", "model":
			extracted.hasHistory = true
			extracted.appendTexts(promptSegmentHistory, contentTexts(message["content"]))
		}
	}
}

func extractUserMessageContent(extracted *promptExtraction, value any) {
	message := extracted.newMessage()
	extracted.appendTextsToMessage(promptSegmentUser, message, contentTexts(value))
	blocks, ok := value.([]any)
	if !ok {
		return
	}
	for _, block := range blocks {
		object, ok := block.(map[string]any)
		if !ok || strings.ToLower(stringValue(object["type"])) != "tool_result" {
			continue
		}
		extracted.appendToolResult(object["content"])
	}
}

func extractInstructions(extracted *promptExtraction, value any) {
	extracted.appendTexts(promptSegmentSystem, contentTexts(value))
}

func extractAnthropicSystem(extracted *promptExtraction, value any) {
	extracted.appendTexts(promptSegmentSystem, contentTexts(value))
}

func extractResponses(extracted *promptExtraction, value any) {
	switch typed := value.(type) {
	case string:
		extracted.appendTexts(promptSegmentUser, []string{typed})
	case []any:
		for _, item := range typed {
			switch entry := item.(type) {
			case string:
				extracted.appendTexts(promptSegmentUser, []string{entry})
			case map[string]any:
				extractResponseEntry(extracted, entry)
			}
		}
	case map[string]any:
		extractResponseEntry(extracted, typed)
	}
}

func extractResponseEntry(extracted *promptExtraction, entry map[string]any) {
	typeName := strings.ToLower(stringValue(entry["type"]))
	switch typeName {
	case "function_call_output", "custom_tool_call_output", "mcp_tool_call_output", "tool_search_output", "mcp_call":
		// All supported Responses/Codex result items expose the model-visible
		// payload through output. Do not inspect tool definitions or call inputs.
		extracted.appendToolResult(entry["output"])
		return
	case "function_call", "tool_call", "local_shell_call", "tool_search_call", "custom_tool_call", "mcp_tool_call":
		// Calls are assistant history, not a tool result. In particular, never
		// send their arguments or freeform input to the guard model.
		extracted.hasHistory = true
		return
	}
	role := strings.ToLower(stringValue(entry["role"]))
	switch role {
	case "system", "developer":
		extracted.appendTexts(promptSegmentSystem, contentTexts(entry["content"]))
	case "user":
		extractUserMessageContent(extracted, entry["content"])
		if _, exists := entry["content"]; !exists {
			extracted.appendTexts(promptSegmentUser, []string{stringValue(entry["text"])})
		}
	case "tool":
		extracted.appendToolResult(entry["content"])
	case "assistant", "model":
		extracted.hasHistory = true
		if content, exists := entry["content"]; exists {
			extracted.appendTexts(promptSegmentHistory, contentTexts(content))
		} else {
			extracted.appendTexts(promptSegmentHistory, []string{stringValue(entry["text"])})
		}
	default:
		if text := stringValue(entry["text"]); text != "" {
			extracted.appendTexts(promptSegmentUser, []string{text})
		}
	}
}

func extractGemini(extracted *promptExtraction, value any) {
	var contents []any
	switch typed := value.(type) {
	case []any:
		contents = typed
	case map[string]any:
		contents = []any{typed}
	default:
		return
	}
	for _, item := range contents {
		content, ok := item.(map[string]any)
		if !ok {
			continue
		}
		role := strings.ToLower(stringValue(content["role"]))
		kind := promptSegmentUser
		if role == "model" || role == "assistant" {
			extracted.hasHistory = true
			kind = promptSegmentHistory
		}
		if role != "" && role != "user" && role != "model" && role != "assistant" {
			continue
		}
		message := extracted.newMessage()
		parts, _ := content["parts"].([]any)
		for _, part := range parts {
			if object, ok := part.(map[string]any); ok {
				if response, ok := object["functionResponse"].(map[string]any); ok {
					extracted.appendToolResult(response["response"])
					continue
				}
				if _, exists := object["functionCall"]; exists {
					extracted.hasHistory = true
					continue
				}
				if text := stringValue(object["text"]); text != "" {
					extracted.appendTextsToMessage(kind, message, []string{text})
				}
			}
		}
	}
}

func extractGeminiRoot(extracted *promptExtraction, root map[string]any) {
	if root == nil {
		return
	}
	extractGeminiSystemInstruction(extracted, root["systemInstruction"])
	extractGeminiSystemInstruction(extracted, root["system_instruction"])
	extractGemini(extracted, root["contents"])
	extractGemini(extracted, root["content"])
	extractGeminiInstances(extracted, root["instances"])
	if requests, ok := root["requests"].([]any); ok {
		for _, item := range requests {
			request, ok := item.(map[string]any)
			if !ok {
				continue
			}
			extractGeminiSystemInstruction(extracted, request["systemInstruction"])
			extractGeminiSystemInstruction(extracted, request["system_instruction"])
			extractGemini(extracted, request["contents"])
			extractGemini(extracted, request["content"])
			extractGeminiInstances(extracted, request["instances"])
		}
	}
}

func extractGeminiSystemInstruction(extracted *promptExtraction, value any) {
	switch typed := value.(type) {
	case string:
		extracted.appendTexts(promptSegmentSystem, []string{typed})
	case map[string]any:
		if parts, ok := typed["parts"].([]any); ok {
			message := extracted.newMessage()
			for _, part := range parts {
				if object, ok := part.(map[string]any); ok {
					if text := stringValue(object["text"]); text != "" {
						extracted.appendTextsToMessage(promptSegmentSystem, message, []string{text})
					}
				}
			}
			return
		}
		extracted.appendTexts(promptSegmentSystem, contentTexts(typed))
	case []any:
		for _, text := range contentTexts(typed) {
			extracted.appendTexts(promptSegmentSystem, []string{text})
		}
	}
}

func extractGeminiInstances(extracted *promptExtraction, value any) {
	instances, ok := value.([]any)
	if !ok {
		return
	}
	for _, item := range instances {
		if instance, ok := item.(map[string]any); ok {
			if prompt := stringValue(instance["prompt"]); prompt != "" {
				extracted.appendTexts(promptSegmentUser, []string{prompt})
			}
		}
	}
}

func extractMediaPrompts(root map[string]any) []string {
	if root == nil {
		return nil
	}
	result := make([]string, 0, 4)
	seen := map[string]struct{}{}
	var walk func(any, string)
	walk = func(value any, key string) {
		switch typed := value.(type) {
		case map[string]any:
			keys := make([]string, 0, len(typed))
			for childKey := range typed {
				keys = append(keys, childKey)
			}
			sort.Strings(keys)
			for _, childKey := range keys {
				walk(typed[childKey], childKey)
			}
		case []any:
			for _, item := range typed {
				walk(item, key)
			}
		case string:
			if !isMediaPromptKey(key) || looksLikeMediaPayload(typed) {
				return
			}
			text := strings.TrimSpace(typed)
			if text == "" {
				return
			}
			if _, duplicate := seen[text]; duplicate {
				return
			}
			seen[text] = struct{}{}
			result = append(result, text)
		}
	}
	walk(root, "")
	return result
}

func isMediaPromptKey(key string) bool {
	normalized := strings.NewReplacer("_", "", "-", "").Replace(strings.ToLower(strings.TrimSpace(key)))
	switch normalized {
	case "prompt", "inputprompt", "textprompt", "description", "query", "lyrics", "negativeprompt",
		"positiveprompt", "gptdescriptionprompt", "prompten", "finalprompt", "finalzhprompt",
		"origprompt", "actualprompt", "imageprompt", "input":
		return true
	default:
		return false
	}
}

func looksLikeMediaPayload(value string) bool {
	trimmed := strings.TrimSpace(value)
	lower := strings.ToLower(trimmed)
	if strings.HasPrefix(lower, "data:") ||
		strings.HasPrefix(lower, "http://") || strings.HasPrefix(lower, "https://") {
		return true
	}
	if len(trimmed) >= 256 {
		for _, r := range trimmed {
			alphaNumeric := (r >= 'A' && r <= 'Z') || (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9')
			if !alphaNumeric && r != '+' && r != '/' && r != '=' {
				return false
			}
		}
		return true
	}
	return false
}

func contentTexts(value any) []string {
	switch typed := value.(type) {
	case string:
		return []string{typed}
	case []any:
		result := make([]string, 0, len(typed))
		for _, part := range typed {
			object, ok := part.(map[string]any)
			if !ok {
				continue
			}
			typeName := strings.ToLower(stringValue(object["type"]))
			if typeName != "" && typeName != "text" && typeName != "input_text" {
				continue
			}
			if text := stringValue(object["text"]); text != "" {
				result = append(result, text)
			}
		}
		return result
	case map[string]any:
		if text := stringValue(typed["text"]); text != "" {
			return []string{text}
		}
	}
	return nil
}

func selectAuditSegments(stage string, extracted promptExtraction) []string {
	values := normalizedPromptSegments(extracted.segments)
	if len(values) == 0 {
		return nil
	}
	latestUserMessage, latestUserEnd := -1, -1
	for index, value := range values {
		if value.kind != promptSegmentUser {
			continue
		}
		latestUserMessage, latestUserEnd = value.message, index
	}
	priority := make([]string, 0, 2)
	for _, value := range values {
		if value.kind == promptSegmentUser && value.message == latestUserMessage {
			priority = append(priority, value.text)
		}
	}
	remaining := make([]string, 0, len(values))
	if isSubsequentPromptAuditTurn(stage, extracted) {
		for index, value := range values {
			if value.kind != promptSegmentToolResult {
				continue
			}
			if latestUserMessage == -1 || index > latestUserEnd {
				remaining = append(remaining, value.text)
			}
		}
	} else {
		for _, value := range values {
			if value.kind == promptSegmentSystem {
				remaining = append(remaining, value.text)
			}
		}
	}
	if len(priority) == 0 {
		return remaining
	}
	result := make([]string, 0, len(remaining)+1)
	result = append(result, strings.Join(priority, "\n\n"))
	return append(result, remaining...)
}

func normalizedPromptSegments(values []promptSegment) []promptSegment {
	normalized := make([]promptSegment, 0, len(values))
	for _, value := range values {
		value.text = strings.TrimSpace(value.text)
		if value.text != "" {
			normalized = append(normalized, value)
		}
	}
	return normalized
}

func isSubsequentPromptAuditTurn(stage string, extracted promptExtraction) bool {
	switch strings.TrimSpace(stage) {
	case "first_turn":
		return false
	case "subsequent_turn":
		return true
	default:
		return extracted.hasHistory || extracted.hasToolResult
	}
}

func buildPrioritizedScanText(segments []string) (scanText string, metadataText string) {
	metadataText = strings.Join(segments, "\n\n")
	if len(segments) <= 1 {
		return metadataText, metadataText
	}
	return segments[0] + promptAuditPrioritySeparator + strings.Join(segments[1:], "\n\n"), metadataText
}

func toolResultTexts(value any) []string {
	result := make([]string, 0, 4)
	var walk func(any, string)
	walk = func(current any, key string) {
		switch typed := current.(type) {
		case string:
			if text := strings.TrimSpace(typed); text != "" && !looksLikeMediaPayload(text) {
				result = append(result, text)
			}
		case []any:
			for _, item := range typed {
				walk(item, key)
			}
		case map[string]any:
			keys := make([]string, 0, len(typed))
			for childKey := range typed {
				keys = append(keys, childKey)
			}
			sort.Strings(keys)
			for _, childKey := range keys {
				if isToolResultMetadataKey(childKey) {
					continue
				}
				walk(typed[childKey], childKey)
			}
		}
	}
	walk(value, "")
	return result
}

func isToolResultMetadataKey(key string) bool {
	normalized := strings.NewReplacer("_", "", "-", "").Replace(strings.ToLower(strings.TrimSpace(key)))
	switch normalized {
	case "type", "name", "id", "callid", "tooluseid", "mimetype", "mime", "url", "image", "imageurl", "binary", "base64", "bytes", "blob", "arguments", "input", "functioncall":
		return true
	default:
		return false
	}
}

func RedactPreview(value string, maxRunes int) string {
	value = bearerPattern.ReplaceAllString(value, "Bearer ***")
	value = apiKeyPattern.ReplaceAllStringFunc(value, func(match string) string {
		if index := strings.IndexAny(match, ":= \t"); index >= 0 {
			return match[:index+1] + "***"
		}
		return "***"
	})
	value = canaryPattern.ReplaceAllString(value, "${1}***")
	value = emailPattern.ReplaceAllString(value, "***@***")
	value = phonePattern.ReplaceAllString(value, "***PHONE***")
	return TrimRunes(value, maxRunes)
}

// BuildPromptPreview stores only a short, non-recoverable head of sanitized
// input. Ordinary confidential prompts must not land nearly intact in PostgreSQL
// or the admin UI merely because no secret regex matched.
func BuildPromptPreview(value string, maxRunes int) string {
	if maxRunes <= 0 {
		maxRunes = DefaultPromptPreviewMaxRunes
	}
	redacted := strings.TrimSpace(RedactPreview(value, maxRunes))
	if redacted == "" {
		return ""
	}
	runes := []rune(redacted)
	hadTruncation := strings.HasSuffix(redacted, "…")
	if hadTruncation && len(runes) > 0 {
		runes = runes[:len(runes)-1]
	}
	if len(runes) == 0 {
		return "***…"
	}
	// Short unlabelled secrets would otherwise leak a recoverable prefix (e.g.
	// 20 runes → 5 visible). Fully withhold anything below the keep threshold.
	const minLengthForPartialPreview = 32
	if len(runes) < minLengthForPartialPreview {
		if hadTruncation {
			return "***…"
		}
		return "***"
	}
	// Keep at most a quarter of the already-truncated text, and never more than
	// 24 runes, so the majority of prompt content is withheld by default.
	keep := len(runes) / 4
	if keep > 24 {
		keep = 24
	}
	preview := string(runes[:keep]) + "***"
	if hadTruncation || keep < len(runes) {
		preview += "…"
	}
	return preview
}

// BuildFullPrompt returns the complete prompt text for audit-event storage and
// admin review, without redaction. NUL bytes are stripped because PostgreSQL
// TEXT rejects them, and the result is capped at maxRunes.
func BuildFullPrompt(value string, maxRunes int) string {
	if maxRunes <= 0 {
		maxRunes = DefaultFullPromptMaxRunes
	}
	value = strings.ReplaceAll(value, "\x00", "")
	return TrimRunes(strings.TrimSpace(value), maxRunes)
}

// FullPromptFromScanText reconstructs the display prompt from the worker scan
// payload. buildPrioritizedScanText inserts exactly one priority separator
// between the prioritized segment and the remainder, so replacing it with the
// metadata joiner yields the original multi-segment text.
func FullPromptFromScanText(scanText string) string {
	return BuildFullPrompt(strings.ReplaceAll(scanText, promptAuditPrioritySeparator, "\n\n"), DefaultFullPromptMaxRunes)
}

func TrimRunes(value string, limit int) string {
	if limit <= 0 {
		return ""
	}
	runes := []rune(value)
	if len(runes) <= limit {
		return value
	}
	return string(runes[:limit]) + "…"
}

func stringValue(value any) string {
	text, _ := value.(string)
	return strings.TrimSpace(text)
}

func cloneInt64Ptr(value *int64) *int64 {
	if value == nil {
		return nil
	}
	cloned := *value
	return &cloned
}
