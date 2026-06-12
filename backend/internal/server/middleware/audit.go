package middleware

import (
	"bufio"
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io"
	"log/slog"
	"net"
	"net/http"
	"strings"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/ctxkey"
	"github.com/Wei-Shaw/sub2api/internal/pkg/ip"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
)

type auditResponseWriter struct {
	gin.ResponseWriter
	buf       bytes.Buffer
	limit     int
	truncated bool
}

func (w *auditResponseWriter) Write(data []byte) (int, error) {
	n, err := w.ResponseWriter.Write(data)
	if n > 0 {
		w.capture(data[:n])
	}
	return n, err
}

func (w *auditResponseWriter) WriteString(data string) (int, error) {
	n, err := w.ResponseWriter.WriteString(data)
	if n > 0 {
		w.capture([]byte(data[:n]))
	}
	return n, err
}

func (w *auditResponseWriter) ReadFrom(r io.Reader) (int64, error) {
	return io.Copy(w.ResponseWriter, &auditCaptureReader{
		reader: r,
		writer: w,
	})
}

func (w *auditResponseWriter) Flush() {
	w.ResponseWriter.Flush()
}

func (w *auditResponseWriter) CloseNotify() <-chan bool {
	return w.ResponseWriter.CloseNotify()
}

func (w *auditResponseWriter) Hijack() (net.Conn, *bufio.ReadWriter, error) {
	return w.ResponseWriter.Hijack()
}

func (w *auditResponseWriter) Pusher() http.Pusher {
	return w.ResponseWriter.Pusher()
}

func (w *auditResponseWriter) Unwrap() http.ResponseWriter {
	if unwrapper, ok := w.ResponseWriter.(interface{ Unwrap() http.ResponseWriter }); ok {
		return unwrapper.Unwrap()
	}
	return w.ResponseWriter
}

type auditCaptureReader struct {
	reader io.Reader
	writer *auditResponseWriter
}

func (r *auditCaptureReader) Read(p []byte) (int, error) {
	n, err := r.reader.Read(p)
	if n > 0 {
		r.writer.capture(p[:n])
	}
	return n, err
}

func (w *auditResponseWriter) capture(data []byte) {
	remaining := w.limit - w.buf.Len()
	if remaining <= 0 {
		if len(data) > 0 {
			w.truncated = true
		}
		return
	}
	if len(data) > remaining {
		w.buf.Write(data[:remaining])
		w.truncated = true
		return
	}
	w.buf.Write(data)
}

func AuditCapture(auditService *service.AuditService) gin.HandlerFunc {
	return func(c *gin.Context) {
		if auditService == nil || c.Request == nil || c.Request.Body == nil || c.Request.Method != http.MethodPost {
			c.Next()
			return
		}

		start := time.Now()
		requestBody, rawRequestBody, requestTruncated := readAndRestoreRequestBody(c, service.AuditCaptureMaxBytes)
		auditRequestBody, auditRequestTruncated := auditRequestBody(rawRequestBody, requestBody, requestTruncated)
		requestStream := extractAuditStream(rawRequestBody)
		originalWriter := c.Writer
		var writer *auditResponseWriter
		if !requestStream {
			writer = &auditResponseWriter{
				ResponseWriter: originalWriter,
				limit:          service.AuditCaptureMaxBytes,
			}
			c.Writer = writer
		}

		c.Next()
		if writer != nil && c.Writer == writer {
			c.Writer = originalWriter
		}

		responseBody, responseTruncated := auditResponseBody(c, writer)
		apiKey, _ := GetAPIKeyFromContext(c)
		sessionID := extractAuditSessionID(c, rawRequestBody)
		log := &service.AuditLog{
			RequestID:         requestIDFromContext(c),
			SessionID:         sessionID,
			Platform:          platformFromAPIKey(apiKey),
			Endpoint:          c.Request.URL.Path,
			Method:            c.Request.Method,
			Model:             extractAuditModel(rawRequestBody),
			StatusCode:        c.Writer.Status(),
			RequestBody:       auditRequestBody,
			ResponseBody:      responseBody,
			RequestTruncated:  auditRequestTruncated,
			ResponseTruncated: responseTruncated,
			DurationMS:        int(time.Since(start).Milliseconds()),
			IPAddress:         ip.GetClientIP(c),
			UserAgent:         truncateAuditString(c.GetHeader("User-Agent"), 512),
		}
		if apiKey != nil {
			log.APIKeyID = &apiKey.ID
			log.APIKeyName = apiKey.Name
			if apiKey.User != nil {
				log.UserID = &apiKey.User.ID
				log.UserEmail = apiKey.User.Email
			}
			if apiKey.Group != nil {
				log.GroupID = &apiKey.Group.ID
				log.GroupName = apiKey.Group.Name
			}
		}
		if log.SessionID == "" {
			log.SessionID = fallbackAuditSessionID(c, rawRequestBody)
		}
		go func(item *service.AuditLog) {
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			if err := auditService.Create(ctx, item); err != nil {
				slog.Warn("audit.capture.write_failed", "request_id", item.RequestID, "session_id", item.SessionID, "err", err)
			}
		}(log)
	}
}

func auditRequestBody(rawBody, capturedBody []byte, capturedTruncated bool) (string, bool) {
	if text := extractAuditUserInput(rawBody); text != "" {
		return truncateAuditContent(text, service.AuditCaptureMaxBytes)
	}
	if len(capturedBody) == 0 {
		return "", false
	}
	return string(capturedBody), capturedTruncated
}

func truncateAuditContent(value string, max int) (string, bool) {
	if len(value) <= max {
		return value, false
	}
	return value[:max], true
}

func readAndRestoreRequestBody(c *gin.Context, limit int) ([]byte, []byte, bool) {
	body, err := io.ReadAll(c.Request.Body)
	if err != nil {
		c.Request.Body = io.NopCloser(bytes.NewReader(nil))
		return nil, nil, false
	}
	c.Request.Body = io.NopCloser(bytes.NewReader(body))

	if len(body) > limit {
		return body[:limit], body, true
	}
	return body, body, false
}

func auditResponseBody(c *gin.Context, writer *auditResponseWriter) (string, bool) {
	if writer != nil {
		return normalizeAuditResponseBody(writer.buf.Bytes(), c.Writer.Header().Get("Content-Type")), writer.truncated
	}
	value := strings.TrimSpace(c.GetString("audit_response_body"))
	if value == "" {
		return "", false
	}
	if len(value) > service.AuditCaptureMaxBytes {
		return value[:service.AuditCaptureMaxBytes], true
	}
	return value, false
}

func extractAuditModel(body []byte) string {
	var payload map[string]any
	if len(body) == 0 || json.Unmarshal(body, &payload) != nil {
		return ""
	}
	if model, ok := payload["model"].(string); ok {
		return truncateAuditString(model, 255)
	}
	return ""
}

func extractAuditStream(body []byte) bool {
	var payload map[string]any
	if len(body) == 0 || json.Unmarshal(body, &payload) != nil {
		return false
	}
	stream, _ := payload["stream"].(bool)
	return stream
}

func extractAuditUserInput(body []byte) string {
	var payload map[string]any
	if len(body) == 0 || json.Unmarshal(body, &payload) != nil {
		return ""
	}
	if text := lastAuditUserText(payload["messages"], "content"); text != "" {
		return text
	}
	if text := lastAuditUserText(payload["input"], "content"); text != "" {
		return text
	}
	if text := lastAuditUserText(payload["contents"], "parts"); text != "" {
		return text
	}
	if text := lastUsefulAuditText(auditContentFragments(payload["prompt"])); text != "" {
		return text
	}
	if text := extractAuditTaggedSession(body); text != "" {
		return text
	}
	return ""
}

func lastAuditUserText(value any, contentKey string) string {
	items, ok := value.([]any)
	if !ok {
		return ""
	}
	texts := make([]string, 0)
	for _, item := range items {
		obj, ok := item.(map[string]any)
		if !ok {
			continue
		}
		if role, _ := obj["role"].(string); role != "user" {
			continue
		}
		texts = append(texts, auditContentFragments(obj[contentKey])...)
	}
	return lastUsefulAuditText(texts)
}

func lastUsefulAuditText(values []string) string {
	for i := len(values) - 1; i >= 0; i-- {
		text := cleanAuditUserText(values[i])
		if isAuditUserText(text) {
			return text
		}
	}
	return ""
}

func cleanAuditUserText(value string) string {
	value = strings.TrimSpace(value)
	for {
		start := strings.Index(value, "<system-reminder>")
		if start < 0 {
			break
		}
		end := strings.Index(value[start:], "</system-reminder>")
		if end < 0 {
			value = strings.TrimSpace(value[:start])
			break
		}
		value = strings.TrimSpace(value[:start] + value[start+end+len("</system-reminder>"):])
	}
	return strings.TrimSpace(value)
}

func isAuditUserText(value string) bool {
	text := strings.TrimSpace(value)
	if text == "" {
		return false
	}
	for _, prefix := range []string{
		"<system-reminder>",
		"<command-message>",
		"<local-command-stdout>",
		"<tool_use_id>",
		"x-anthropic-billing-header:",
		"You are Claude Code, Anthropic's official CLI for Claude.",
		"You are an interactive agent that helps users with software engineering tasks.",
	} {
		if strings.HasPrefix(text, prefix) {
			return false
		}
	}
	return !strings.Contains(text, "The following skills are available for use with the Skill tool")
}

func extractAuditSessionID(c *gin.Context, body []byte) string {
	if c != nil {
		for _, header := range []string{
			"X-Claude-Code-Session-Id",
			"X-Session-Id",
			"session_id",
			"conversation_id",
		} {
			if value := strings.TrimSpace(c.GetHeader(header)); value != "" {
				return truncateAuditString(value, 255)
			}
		}
	}

	var payload map[string]any
	if len(body) == 0 || json.Unmarshal(body, &payload) != nil {
		return ""
	}
	for _, key := range []string{"session_id", "conversation_id", "prompt_cache_key"} {
		if value, ok := payload[key].(string); ok && strings.TrimSpace(value) != "" {
			return truncateAuditString(strings.TrimSpace(value), 255)
		}
	}
	if metadata, ok := payload["metadata"].(map[string]any); ok {
		if value, ok := metadata["session_id"].(string); ok && strings.TrimSpace(value) != "" {
			return truncateAuditString(strings.TrimSpace(value), 255)
		}
		if value, ok := metadata["conversation_id"].(string); ok && strings.TrimSpace(value) != "" {
			return truncateAuditString(strings.TrimSpace(value), 255)
		}
		if value, ok := metadata["user_id"].(string); ok && strings.TrimSpace(value) != "" {
			if parsed := extractAuditSessionFromMetadataUserID(value); parsed != "" {
				return truncateAuditString(parsed, 255)
			}
			return truncateAuditString(strings.TrimSpace(value), 255)
		}
	}
	if value := extractAuditTaggedSession(body); value != "" {
		return "tag:" + auditStableHash(value)
	}
	return ""
}

func extractAuditSessionFromMetadataUserID(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return ""
	}
	var obj map[string]any
	if json.Unmarshal([]byte(value), &obj) == nil {
		if sessionID, ok := obj["session_id"].(string); ok && strings.TrimSpace(sessionID) != "" {
			return strings.TrimSpace(sessionID)
		}
	}
	if idx := strings.LastIndex(value, "session_"); idx >= 0 {
		return strings.TrimSpace(strings.TrimPrefix(value[idx:], "session_"))
	}
	return ""
}

func fallbackAuditSessionID(c *gin.Context, body []byte) string {
	if !isClaudeCodeAuditRequest(c, body) {
		return ""
	}
	if signature := extractAuditClaudeSignature(body); signature != "" {
		return "claude-signature:" + auditStableHash(signature)
	}
	parts := []string{"claude-cli"}
	if c != nil {
		parts = append(parts, strings.TrimSpace(c.GetHeader("User-Agent")))
		parts = append(parts, ip.GetClientIP(c))
	}
	return "claude:" + auditStableHash(strings.Join(parts, "|"))
}

func extractAuditClaudeSignature(body []byte) string {
	var payload map[string]any
	if len(body) == 0 || json.Unmarshal(body, &payload) != nil {
		return ""
	}
	return firstAuditStringField(payload, "signature")
}

func firstAuditStringField(value any, key string) string {
	switch item := value.(type) {
	case map[string]any:
		if raw, ok := item[key].(string); ok && strings.TrimSpace(raw) != "" {
			return strings.TrimSpace(raw)
		}
		for _, child := range item {
			if found := firstAuditStringField(child, key); found != "" {
				return found
			}
		}
	case []any:
		for _, child := range item {
			if found := firstAuditStringField(child, key); found != "" {
				return found
			}
		}
	}
	return ""
}

func isClaudeCodeAuditRequest(c *gin.Context, body []byte) bool {
	if c != nil {
		userAgent := strings.ToLower(c.GetHeader("User-Agent"))
		if strings.Contains(userAgent, "claude-cli") || strings.Contains(userAgent, "claude-code") {
			return true
		}
	}
	raw := strings.ToLower(string(body))
	return strings.Contains(raw, "you are claude code") ||
		strings.Contains(raw, "x-anthropic-billing-header") ||
		strings.Contains(raw, "cc_entrypoint=cli")
}

func extractAuditTaggedSession(body []byte) string {
	raw := string(body)
	start := strings.Index(raw, "<session>")
	if start < 0 {
		return ""
	}
	start += len("<session>")
	end := strings.Index(raw[start:], "</session>")
	if end < 0 {
		return ""
	}
	value := strings.TrimSpace(raw[start : start+end])
	if value == "" {
		return ""
	}
	return value
}

func auditStableHash(value string) string {
	sum := sha256.Sum256([]byte(strings.TrimSpace(value)))
	return hex.EncodeToString(sum[:])[:24]
}

func platformFromAPIKey(apiKey *service.APIKey) string {
	if apiKey == nil || apiKey.Group == nil {
		return ""
	}
	return apiKey.Group.Platform
}

func requestIDFromContext(c *gin.Context) string {
	if c.Request != nil {
		if value, _ := c.Request.Context().Value(ctxkey.RequestID).(string); strings.TrimSpace(value) != "" {
			return truncateAuditString(strings.TrimSpace(value), 64)
		}
		if value, _ := c.Request.Context().Value(ctxkey.ClientRequestID).(string); strings.TrimSpace(value) != "" {
			return truncateAuditString(strings.TrimSpace(value), 64)
		}
	}
	if value := strings.TrimSpace(c.GetHeader("X-Client-Request-ID")); value != "" {
		return truncateAuditString(value, 64)
	}
	if value := strings.TrimSpace(c.GetHeader("X-Request-ID")); value != "" {
		return truncateAuditString(value, 64)
	}
	return truncateAuditString(c.GetHeader("X-Client-Request-Id"), 64)
}

func truncateAuditString(value string, max int) string {
	if len(value) <= max {
		return value
	}
	return value[:max]
}

func normalizeAuditResponseBody(body []byte, contentType string) string {
	raw := string(body)
	if strings.TrimSpace(raw) == "" {
		return raw
	}
	if !isAuditSSEBody(raw, contentType) {
		return raw
	}

	text := extractAuditSSEText(raw)
	if strings.TrimSpace(text) == "" {
		return raw
	}
	return text
}

func isAuditSSEBody(raw, contentType string) bool {
	if strings.Contains(strings.ToLower(contentType), "text/event-stream") {
		return true
	}
	trimmed := strings.TrimLeft(raw, "\ufeff\r\n\t ")
	return strings.HasPrefix(trimmed, "event:") || strings.HasPrefix(trimmed, "data:")
}

func extractAuditSSEText(raw string) string {
	events := splitAuditSSEEvents(raw)
	var deltas strings.Builder
	finalTexts := make([]string, 0)
	errors := make([]string, 0)

	for _, event := range events {
		eventName, data := parseAuditSSEEvent(event)
		data = strings.TrimSpace(data)
		if data == "" || data == "[DONE]" {
			continue
		}

		var payload any
		if err := json.Unmarshal([]byte(data), &payload); err != nil {
			continue
		}
		fragments := auditTextFragments(payload, eventName)
		if len(fragments) > 0 {
			for _, fragment := range fragments {
				deltas.WriteString(fragment)
			}
			continue
		}
		finalTexts = append(finalTexts, auditFinalTextFragments(payload)...)
		errors = append(errors, auditErrorFragments(payload)...)
	}

	if deltas.Len() > 0 {
		return deltas.String()
	}
	if len(finalTexts) > 0 {
		return strings.Join(finalTexts, "")
	}
	if len(errors) > 0 {
		return strings.Join(errors, "\n")
	}
	return ""
}

func splitAuditSSEEvents(raw string) []string {
	raw = strings.ReplaceAll(raw, "\r\n", "\n")
	parts := strings.Split(raw, "\n\n")
	events := make([]string, 0, len(parts))
	for _, part := range parts {
		if strings.TrimSpace(part) != "" {
			events = append(events, part)
		}
	}
	return events
}

func parseAuditSSEEvent(event string) (string, string) {
	lines := strings.Split(event, "\n")
	dataLines := make([]string, 0, len(lines))
	eventName := ""
	for _, line := range lines {
		line = strings.TrimSuffix(line, "\r")
		if strings.HasPrefix(line, "event:") {
			eventName = strings.TrimSpace(strings.TrimPrefix(line, "event:"))
			continue
		}
		if strings.HasPrefix(line, "data:") {
			dataLines = append(dataLines, strings.TrimSpace(strings.TrimPrefix(line, "data:")))
		}
	}
	return eventName, strings.Join(dataLines, "\n")
}

func auditTextFragments(payload any, eventName string) []string {
	obj, ok := payload.(map[string]any)
	if !ok {
		return nil
	}
	eventType, _ := obj["type"].(string)
	if eventType == "" {
		eventType = eventName
	}

	fragments := make([]string, 0)
	if strings.Contains(eventType, "delta") {
		fragments = append(fragments, auditDeltaFragments(obj)...)
	}
	if strings.Contains(eventType, "content_block_start") {
		if block, ok := obj["content_block"].(map[string]any); ok {
			fragments = append(fragments, auditStringField(block, "text"))
		}
	}
	if strings.Contains(eventType, "message_start") {
		if message, ok := obj["message"].(map[string]any); ok {
			fragments = append(fragments, auditContentFragments(message["content"])...)
		}
	}
	if choices, ok := obj["choices"].([]any); ok {
		for _, choice := range choices {
			choiceObj, ok := choice.(map[string]any)
			if !ok {
				continue
			}
			if delta, ok := choiceObj["delta"].(map[string]any); ok {
				fragments = append(fragments, auditStringField(delta, "content"))
			}
			if message, ok := choiceObj["message"].(map[string]any); ok {
				fragments = append(fragments, auditStringField(message, "content"))
				fragments = append(fragments, auditContentFragments(message["content"])...)
			}
		}
	}
	if candidates, ok := obj["candidates"].([]any); ok {
		for _, candidate := range candidates {
			candidateObj, ok := candidate.(map[string]any)
			if !ok {
				continue
			}
			if content, ok := candidateObj["content"].(map[string]any); ok {
				fragments = append(fragments, auditGeminiParts(content["parts"])...)
			}
		}
	}
	return compactAuditFragments(fragments)
}

func auditDeltaFragments(obj map[string]any) []string {
	fragments := make([]string, 0)
	if delta, ok := obj["delta"].(string); ok {
		fragments = append(fragments, delta)
	}
	if delta, ok := obj["delta"].(map[string]any); ok {
		fragments = append(fragments, auditStringField(delta, "text"))
		fragments = append(fragments, auditStringField(delta, "content"))
	}
	return fragments
}

func auditFinalTextFragments(payload any) []string {
	obj, ok := payload.(map[string]any)
	if !ok {
		return nil
	}
	fragments := make([]string, 0)
	fragments = append(fragments, auditContentFragments(obj["content"])...)
	if response, ok := obj["response"].(map[string]any); ok {
		fragments = append(fragments, auditOutputFragments(response["output"])...)
		fragments = append(fragments, auditContentFragments(response["content"])...)
	}
	fragments = append(fragments, auditOutputFragments(obj["output"])...)
	return compactAuditFragments(fragments)
}

func auditOutputFragments(value any) []string {
	output, ok := value.([]any)
	if !ok {
		return nil
	}
	fragments := make([]string, 0)
	for _, item := range output {
		itemObj, ok := item.(map[string]any)
		if !ok {
			continue
		}
		fragments = append(fragments, auditStringField(itemObj, "text"))
		fragments = append(fragments, auditContentFragments(itemObj["content"])...)
	}
	return fragments
}

func auditContentFragments(value any) []string {
	switch content := value.(type) {
	case string:
		return []string{content}
	case []any:
		fragments := make([]string, 0, len(content))
		for _, item := range content {
			itemObj, ok := item.(map[string]any)
			if !ok {
				continue
			}
			fragments = append(fragments, auditStringField(itemObj, "text"))
			fragments = append(fragments, auditStringField(itemObj, "content"))
		}
		return fragments
	default:
		return nil
	}
}

func auditGeminiParts(value any) []string {
	parts, ok := value.([]any)
	if !ok {
		return nil
	}
	fragments := make([]string, 0, len(parts))
	for _, part := range parts {
		partObj, ok := part.(map[string]any)
		if ok {
			fragments = append(fragments, auditStringField(partObj, "text"))
		}
	}
	return fragments
}

func auditErrorFragments(payload any) []string {
	obj, ok := payload.(map[string]any)
	if !ok {
		return nil
	}
	if errObj, ok := obj["error"].(map[string]any); ok {
		return compactAuditFragments([]string{auditStringField(errObj, "message")})
	}
	return compactAuditFragments([]string{auditStringField(obj, "message")})
}

func auditStringField(obj map[string]any, key string) string {
	if value, ok := obj[key].(string); ok {
		return value
	}
	return ""
}

func compactAuditFragments(values []string) []string {
	out := make([]string, 0, len(values))
	for _, value := range values {
		if value != "" {
			out = append(out, value)
		}
	}
	return out
}
