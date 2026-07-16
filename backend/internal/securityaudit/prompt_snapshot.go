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
	ErrNoPromptText = errors.New("prompt audit request contains no user text")

	bearerPattern = regexp.MustCompile(`(?i)\bBearer\s+[A-Za-z0-9._~+\-/]+=*`)
	apiKeyPattern = regexp.MustCompile(`(?i)\b(sk|rk|pk|api[_-]?key|token|secret|password)[-_:=\s]+[A-Za-z0-9._~+\-/]{8,REDACTED`)
	canaryPattern = regexp.MustCompile(`(?i)([A-Z]+_CANARY_)[A-Za-z0-9_-]+`)
	emailPattern  = regexp.MustCompile(`(?i)\b[A-Z0-9._%+\-]+@[A-Z0-9.\-]+\.[A-Z]{2,REDACTED\b`)
	phonePattern  = regexp.MustCompile(`(?:\+?\d[\d\s().-]{8,REDACTED\d)`)
)

func ExtractPromptSnapshot(req Request) (PromptSnapshot, error) {
	var document any
	if err := json.Unmarshal(req.Body, &document); err != nil {
		return PromptSnapshot{REDACTED, errors.New("prompt audit request JSON is invalid")
REDACTED
	segments := extractProtocolSegments(req.Protocol, document)
	segments = normalizeSegmentsLatestFirst(segments)
	if len(segments) == 0 {
		return PromptSnapshot{REDACTED, ErrNoPromptText
REDACTED
	scanText := strings.Join(segments, "\n\n")
	digest := sha256.Sum256([]byte(scanText))
	stage := strings.TrimSpace(req.Stage)
	if stage == "" {
		stage = "http"
REDACTED
	return PromptSnapshot{
		RequestID: req.RequestID, UserID: req.UserID, UsernameSnapshot: req.Username,
		UserEmailSnapshot: req.UserEmail, APIKeyID: req.APIKeyID, APIKeyNameSnapshot: req.APIKeyName,
		GroupID: cloneInt64Ptr(req.GroupID), GroupName: req.GroupName, Provider: req.Provider,
		Endpoint: req.Endpoint, Protocol: req.Protocol, Model: req.Model,
		PromptHash: hex.EncodeToString(digest[:]), RedactedPreview: BuildPromptPreview(scanText, 480),
		PromptLength: utf8.RuneCountInString(scanText), MessageCount: len(segments), Stage: stage,
		ScanText: scanText,
REDACTED, nil
REDACTED

func extractProtocolSegments(protocol string, document any) []string {
	root, _ := document.(map[string]any)
	protocol = strings.ToLower(strings.TrimSpace(protocol))
	switch protocol {
	case "openai_chat_completions", "openai_chat", "chat_completions":
		return extractMessages(root["messages"], "user")
	case "anthropic_messages", "claude_messages", "messages":
		return extractMessages(root["messages"], "user")
	case "gemini", "gemini_generate_content":
		return extractGeminiRoot(root)
	case "openai_responses", "responses", "responses_websocket":
		if frameType := stringValue(root["type"]); frameType != "" || protocol == "responses_websocket" {
			if frameType != "response.create" {
				return nil
		REDACTED
			if input, exists := root["input"]; exists && input != nil {
				return extractResponses(input)
		REDACTED
			if response, ok := root["response"].(map[string]any); ok {
				return extractResponses(response["input"])
		REDACTED
			return nil
	REDACTED
		return extractResponses(root["input"])
	case "openai_images", "grok_media", "media", "images":
		return extractMediaPrompts(root)
	default:
		if messages := extractMessages(root["messages"], "user"); len(messages) > 0 {
			return messages
	REDACTED
		if responses := extractResponses(root["input"]); len(responses) > 0 {
			return responses
	REDACTED
		if gemini := extractGeminiRoot(root); len(gemini) > 0 {
			return gemini
	REDACTED
		return extractMediaPrompts(root)
REDACTED
REDACTED

func extractMessages(value any, wantedRole string) []string {
	items, ok := value.([]any)
	if !ok {
		return nil
REDACTED
	result := make([]string, 0, len(items))
	for _, item := range items {
		message, ok := item.(map[string]any)
		if !ok || !strings.EqualFold(stringValue(message["role"]), wantedRole) {
			continue
	REDACTED
		texts := contentTexts(message["content"])
		if len(texts) > 0 {
			result = append(result, strings.Join(texts, "\n"))
	REDACTED
REDACTED
	return result
REDACTED

func extractResponses(value any) []string {
	switch typed := value.(type) {
	case string:
		return []string{typedREDACTED
	case []any:
		result := make([]string, 0, len(typed))
		for _, item := range typed {
			switch entry := item.(type) {
			case string:
				result = append(result, entry)
			case map[string]any:
				role := strings.ToLower(stringValue(entry["role"]))
				if role != "" && role != "user" {
					continue
			REDACTED
				if content, exists := entry["content"]; exists {
					if texts := contentTexts(content); len(texts) > 0 {
						result = append(result, strings.Join(texts, "\n"))
				REDACTED
			REDACTED else if text := stringValue(entry["text"]); text != "" {
					result = append(result, text)
			REDACTED
		REDACTED
	REDACTED
		return result
	case map[string]any:
		role := strings.ToLower(stringValue(typed["role"]))
		if role != "" && role != "user" {
			return nil
	REDACTED
		return contentTexts(typed["content"])
	default:
		return nil
REDACTED
REDACTED

func extractGemini(value any) []string {
	var contents []any
	switch typed := value.(type) {
	case []any:
		contents = typed
	case map[string]any:
		contents = []any{typedREDACTED
	default:
		return nil
REDACTED
	result := make([]string, 0, len(contents))
	for _, item := range contents {
		content, ok := item.(map[string]any)
		if !ok {
			continue
	REDACTED
		role := strings.ToLower(stringValue(content["role"]))
		if role != "" && role != "user" {
			continue
	REDACTED
		parts, _ := content["parts"].([]any)
		for _, part := range parts {
			if object, ok := part.(map[string]any); ok {
				if text := stringValue(object["text"]); text != "" {
					result = append(result, text)
			REDACTED
		REDACTED
	REDACTED
REDACTED
	return result
REDACTED

func extractGeminiRoot(root map[string]any) []string {
	if root == nil {
		return nil
REDACTED
	result := extractGemini(root["contents"])
	result = append(result, extractGemini(root["content"])...)
	result = append(result, extractGeminiInstances(root["instances"])...)
	if requests, ok := root["requests"].([]any); ok {
		for _, item := range requests {
			request, ok := item.(map[string]any)
			if !ok {
				continue
		REDACTED
			result = append(result, extractGemini(request["contents"])...)
			result = append(result, extractGemini(request["content"])...)
			result = append(result, extractGeminiInstances(request["instances"])...)
	REDACTED
REDACTED
	return result
REDACTED

func extractGeminiInstances(value any) []string {
	instances, ok := value.([]any)
	if !ok {
		return nil
REDACTED
	result := make([]string, 0, len(instances))
	for _, item := range instances {
		if instance, ok := item.(map[string]any); ok {
			if prompt := stringValue(instance["prompt"]); prompt != "" {
				result = append(result, prompt)
		REDACTED
	REDACTED
REDACTED
	return result
REDACTED

func extractMediaPrompts(root map[string]any) []string {
	if root == nil {
		return nil
REDACTED
	result := make([]string, 0, 4)
	seen := map[string]struct{REDACTED{REDACTED
	var walk func(any, string)
	walk = func(value any, key string) {
		switch typed := value.(type) {
		case map[string]any:
			keys := make([]string, 0, len(typed))
			for childKey := range typed {
				keys = append(keys, childKey)
		REDACTED
			sort.Strings(keys)
			for _, childKey := range keys {
				walk(typed[childKey], childKey)
		REDACTED
		case []any:
			for _, item := range typed {
				walk(item, key)
		REDACTED
		case string:
			if !isMediaPromptKey(key) || looksLikeMediaPayload(typed) {
				return
		REDACTED
			text := strings.TrimSpace(typed)
			if text == "" {
				return
		REDACTED
			if _, duplicate := seen[text]; duplicate {
				return
		REDACTED
			seen[text] = struct{REDACTED{REDACTED
			result = append(result, text)
	REDACTED
REDACTED
	walk(root, "")
	return result
REDACTED

func isMediaPromptKey(key string) bool {
	normalized := strings.NewReplacer("_", "", "-", "").Replace(strings.ToLower(strings.TrimSpace(key)))
	switch normalized {
	case "prompt", "inputprompt", "textprompt", "description", "query", "lyrics", "negativeprompt",
		"positiveprompt", "gptdescriptionprompt", "prompten", "finalprompt", "finalzhprompt",
		"origprompt", "actualprompt", "imageprompt", "input":
		return true
	default:
		return false
REDACTED
REDACTED

func looksLikeMediaPayload(value string) bool {
	trimmed := strings.TrimSpace(value)
	lower := strings.ToLower(trimmed)
	if strings.HasPrefix(lower, "data:image/") || strings.HasPrefix(lower, "data:video/") ||
		strings.HasPrefix(lower, "http://") || strings.HasPrefix(lower, "https://") {
		return true
REDACTED
	if len(trimmed) >= 256 {
		for _, r := range trimmed {
			alphaNumeric := (r >= 'A' && r <= 'Z') || (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9')
			if !alphaNumeric && r != '+' && r != '/' && r != '=' {
				return false
		REDACTED
	REDACTED
		return true
REDACTED
	return false
REDACTED

func contentTexts(value any) []string {
	switch typed := value.(type) {
	case string:
		return []string{typedREDACTED
	case []any:
		result := make([]string, 0, len(typed))
		for _, part := range typed {
			object, ok := part.(map[string]any)
			if !ok {
				continue
		REDACTED
			typeName := strings.ToLower(stringValue(object["type"]))
			if typeName != "" && typeName != "text" && typeName != "input_text" {
				continue
		REDACTED
			if text := stringValue(object["text"]); text != "" {
				result = append(result, text)
		REDACTED
	REDACTED
		return result
	case map[string]any:
		if text := stringValue(typed["text"]); text != "" {
			return []string{textREDACTED
	REDACTED
REDACTED
	return nil
REDACTED

func normalizeSegmentsLatestFirst(values []string) []string {
	normalized := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value != "" {
			normalized = append(normalized, value)
	REDACTED
REDACTED
	if len(normalized) <= 1 {
		return normalized
REDACTED
	latest := normalized[len(normalized)-1]
	result := make([]string, 0, len(normalized))
	result = append(result, latest)
	result = append(result, normalized[:len(normalized)-1]...)
	return result
REDACTED

func RedactPreview(value string, maxRunes int) string {
	value = bearerPattern.ReplaceAllString(value, "Bearer ***")
	value = apiKeyPattern.ReplaceAllStringFunc(value, func(match string) string {
		if index := strings.IndexAny(match, ":= \t"); index >= 0 {
			return match[:index+1] + "***"
	REDACTED
		return "***"
REDACTED)
	value = canaryPattern.ReplaceAllString(value, "${1REDACTED***")
	value = emailPattern.ReplaceAllString(value, "***@***")
	value = phonePattern.ReplaceAllString(value, "***PHONE***")
	return TrimRunes(value, maxRunes)
REDACTED

// BuildPromptPreview always withholds part of the sanitized input. Even short,
// otherwise-benign prompts must not become a recoverable raw-prompt database
// field merely because no secret pattern happened to match.
func BuildPromptPreview(value string, maxRunes int) string {
	redacted := strings.TrimSpace(RedactPreview(value, maxRunes))
	if redacted == "" {
		return ""
REDACTED
	runes := []rune(redacted)
	hadTruncation := strings.HasSuffix(redacted, "…")
	visibleLength := len(runes)
	if hadTruncation && visibleLength > 0 {
		visibleLength--
REDACTED
	maskCount := visibleLength / 4
	if maskCount < 1 {
		maskCount = 1
REDACTED
	if maskCount > 16 {
		maskCount = 16
REDACTED
	keep := visibleLength - maskCount
	if keep < 0 {
		keep = 0
REDACTED
	preview := string(runes[:keep]) + "***"
	if hadTruncation {
		preview += "…"
REDACTED
	return preview
REDACTED

func TrimRunes(value string, limit int) string {
	if limit <= 0 {
		return ""
REDACTED
	runes := []rune(value)
	if len(runes) <= limit {
		return value
REDACTED
	return string(runes[:limit]) + "…"
REDACTED

func stringValue(value any) string {
	text, _ := value.(string)
	return strings.TrimSpace(text)
REDACTED

func cloneInt64Ptr(value *int64) *int64 {
	if value == nil {
		return nil
REDACTED
	cloned := *value
	return &cloned
REDACTED
