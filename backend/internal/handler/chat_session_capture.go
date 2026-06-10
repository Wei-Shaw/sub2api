package handler

import (
	"bytes"
	"compress/gzip"
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/observability"
	"github.com/Wei-Shaw/sub2api/internal/pkg/logger"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/tidwall/gjson"
	"go.uber.org/zap"
)

const (
	chatSessionRecordQueueSize = 2048
	chatSessionRecordWorkers   = 4
	chatSessionRecordTimeout   = 5 * time.Second
	chatSessionInlineMaxBytes  = 256 * 1024
)

type chatSessionRecordTask struct {
	recorder           *service.ChatSessionService
	payload            *service.ChatSessionRecordInput
	requestBody        []byte
	requestBodyRef     json.RawMessage
	requestBodySummary string
	finalOutputText    string
	finalOutputJSON    json.RawMessage
	finalOutputJSONRef json.RawMessage
}

var (
	chatSessionRecordQueue     chan chatSessionRecordTask
	chatSessionRecordQueueOnce sync.Once
)

func recordChatSessionAsync(
	ctx context.Context,
	recorder *service.ChatSessionService,
	apiKey *service.APIKey,
	account *service.Account,
	input *service.ChatSessionRecordInput,
	requestBody []byte,
	finalOutputText string,
	finalOutputJSON json.RawMessage,
) {
	if recorder == nil || apiKey == nil || input == nil {
		return
	}

	input.UserID = apiKey.UserID
	input.APIKeyID = apiKey.ID
	input.GroupID = apiKey.GroupID
	if account != nil {
		input.AccountID = &account.ID
	}
	if input.CreatedAt.IsZero() {
		input.CreatedAt = time.Now()
	}

	enqueueChatSessionRecord(recorder, input, requestBody, finalOutputText, finalOutputJSON)
}

func enqueueChatSessionRecord(
	recorder *service.ChatSessionService,
	payload *service.ChatSessionRecordInput,
	requestBody []byte,
	finalOutputText string,
	finalOutputJSON json.RawMessage,
) {
	if recorder == nil || payload == nil {
		return
	}
	requestBodyRef, requestBodySummary := maybeExternalizeChatSessionCapturePayload(
		requestBody,
		payload.CreatedAt,
		"request",
		func(raw []byte) string {
			endpoint := ""
			if payload.InboundEndpoint != nil {
				endpoint = *payload.InboundEndpoint
			}
			return summarizeInboundRequestBody(endpoint, raw)
		},
	)
	if len(requestBodyRef) > 0 || shouldDropInlineChatSessionCapturePayload(requestBody) {
		requestBody = nil
	}
	finalOutputJSONRef, finalOutputSummary := maybeExternalizeChatSessionCapturePayload(
		finalOutputJSON,
		payload.CreatedAt,
		"response",
		func(raw []byte) string { return summarizeChatContentJSON(raw) },
	)
	if len(finalOutputJSONRef) > 0 || shouldDropInlineChatSessionCapturePayload(finalOutputJSON) {
		finalOutputJSON = nil
		if strings.TrimSpace(finalOutputText) == "" {
			finalOutputText = finalOutputSummary
		}
	}
	chatSessionRecordQueueOnce.Do(startChatSessionRecordWorkers)
	select {
	case chatSessionRecordQueue <- chatSessionRecordTask{
		recorder:           recorder,
		payload:            payload,
		requestBody:        requestBody,
		requestBodyRef:     requestBodyRef,
		requestBodySummary: requestBodySummary,
		finalOutputText:    finalOutputText,
		finalOutputJSON:    finalOutputJSON,
		finalOutputJSONRef: finalOutputJSONRef,
	}:
		observability.IncChatSessionCaptureEnqueued()
		observability.SetChatSessionCaptureQueueStats(len(chatSessionRecordQueue), cap(chatSessionRecordQueue))
	default:
		observability.IncChatSessionCaptureDropped()
		observability.SetChatSessionCaptureQueueStats(len(chatSessionRecordQueue), cap(chatSessionRecordQueue))
		fields := []zap.Field{
			zap.Int64("user_id", payload.UserID),
			zap.Int64("api_key_id", payload.APIKeyID),
			zap.String("session_key", payload.SessionKey),
			zap.String("request_id", payload.RequestID),
			zap.Int("queue_size", chatSessionRecordQueueSize),
		}
		if payload.AccountID != nil {
			fields = append(fields, zap.Int64("account_id", *payload.AccountID))
		}
		logger.L().Warn("chat_session.record_queue_full", fields...)
	}
}

func startChatSessionRecordWorkers() {
	chatSessionRecordQueue = make(chan chatSessionRecordTask, chatSessionRecordQueueSize)
	observability.SetChatSessionCaptureQueueStats(0, chatSessionRecordQueueSize)
	for i := 0; i < chatSessionRecordWorkers; i++ {
		go func() {
			for task := range chatSessionRecordQueue {
				observability.SetChatSessionCaptureQueueStats(len(chatSessionRecordQueue), cap(chatSessionRecordQueue))
				recordChatSessionTaskWithTimeout(task)
				observability.SetChatSessionCaptureQueueStats(len(chatSessionRecordQueue), cap(chatSessionRecordQueue))
			}
		}()
	}
}

func recordChatSessionTaskWithTimeout(task chatSessionRecordTask) {
	if task.recorder == nil || task.payload == nil {
		return
	}
	messages, events := buildChatSessionMessages(
		task.payload.InboundEndpoint,
		task.requestBody,
		task.requestBodyRef,
		task.requestBodySummary,
		task.finalOutputText,
		task.finalOutputJSON,
		task.finalOutputJSONRef,
	)
	if len(messages) == 0 && len(events) == 0 {
		return
	}
	task.payload.Messages = messages
	task.payload.Events = events
	recordChatSessionWithTimeout(task.recorder, task.payload)
}

func recordChatSessionWithTimeout(recorder *service.ChatSessionService, payload *service.ChatSessionRecordInput) {
	if recorder == nil || payload == nil {
		return
	}
	taskCtx, cancel := context.WithTimeout(context.Background(), chatSessionRecordTimeout)
	defer cancel()
	startedAt := time.Now()
	if err := recorder.RecordSession(taskCtx, payload); err != nil {
		duration := time.Since(startedAt)
		observability.ObserveChatSessionCaptureFailure(duration, err)
		fields := []zap.Field{
			zap.Int64("user_id", payload.UserID),
			zap.Int64("api_key_id", payload.APIKeyID),
			zap.String("session_key", payload.SessionKey),
			zap.String("request_id", payload.RequestID),
			zap.Int64("duration_ms", duration.Milliseconds()),
			zap.Error(err),
		}
		if payload.AccountID != nil {
			fields = append(fields, zap.Int64("account_id", *payload.AccountID))
		}
		logger.L().Warn("chat_session.record_failed", fields...)
		return
	}
	duration := time.Since(startedAt)
	observability.ObserveChatSessionCaptureSuccess(duration)
	if duration >= observability.ChatSessionCaptureSlowThreshold {
		observability.ObserveChatSessionCaptureSlow(duration)
		fields := []zap.Field{
			zap.Int64("user_id", payload.UserID),
			zap.Int64("api_key_id", payload.APIKeyID),
			zap.String("session_key", payload.SessionKey),
			zap.String("request_id", payload.RequestID),
			zap.Int64("duration_ms", duration.Milliseconds()),
			zap.Int("message_count", len(payload.Messages)),
			zap.Int("event_count", len(payload.Events)),
		}
		if payload.AccountID != nil {
			fields = append(fields, zap.Int64("account_id", *payload.AccountID))
		}
		logger.L().Warn("chat_session.record_slow", fields...)
	}
}

func buildChatSessionMessages(
	inboundEndpoint *string,
	requestBody []byte,
	requestBodyRef json.RawMessage,
	requestBodySummary string,
	finalOutputText string,
	finalOutputJSON json.RawMessage,
	finalOutputJSONRef json.RawMessage,
) ([]service.ChatMessageRecordInput, []service.ChatMessageEventRecordInput) {
	endpoint := ""
	if inboundEndpoint != nil {
		endpoint = strings.TrimSpace(*inboundEndpoint)
	}

	var inboundMessages []service.ChatMessageRecordInput
	if len(requestBodyRef) > 0 {
		inboundMessages = []service.ChatMessageRecordInput{{
			Role:        "user",
			Direction:   "inbound",
			ContentText: strings.TrimSpace(requestBodySummary),
			ContentJSON: requestBodyRef,
		}}
	} else if len(requestBody) == 0 && strings.TrimSpace(requestBodySummary) != "" {
		inboundMessages = []service.ChatMessageRecordInput{{
			Role:        "user",
			Direction:   "inbound",
			ContentText: strings.TrimSpace(requestBodySummary),
		}}
	} else {
		inboundMessages, _ = parseInboundChatMessages(endpoint, requestBody)
		inboundMessages = keepLatestInboundMessages(inboundMessages)
	}
	messages := make([]service.ChatMessageRecordInput, 0, len(inboundMessages)+1)
	messages = append(messages, inboundMessages...)
	if text := strings.TrimSpace(finalOutputText); text != "" {
		contentJSON := chooseAssistantContentJSON(finalOutputJSON, text)
		if len(finalOutputJSONRef) > 0 {
			contentJSON = finalOutputJSONRef
		}
		messages = append(messages, service.ChatMessageRecordInput{
			Role:        "assistant",
			Direction:   "outbound",
			ContentText: text,
			ContentJSON: contentJSON,
		})
	} else if len(finalOutputJSON) > 0 {
		messages = append(messages, service.ChatMessageRecordInput{
			Role:        "assistant",
			Direction:   "outbound",
			ContentText: summarizeChatContentJSON(finalOutputJSON),
			ContentJSON: cloneChatRawJSON(finalOutputJSON),
		})
	} else if len(finalOutputJSONRef) > 0 {
		messages = append(messages, service.ChatMessageRecordInput{
			Role:        "assistant",
			Direction:   "outbound",
			ContentJSON: finalOutputJSONRef,
		})
	}
	return messages, nil
}

func keepLatestInboundMessages(items []service.ChatMessageRecordInput) []service.ChatMessageRecordInput {
	for i := len(items) - 1; i >= 0; i-- {
		if strings.TrimSpace(items[i].Direction) == "inbound" {
			return []service.ChatMessageRecordInput{items[i]}
		}
	}
	return items
}

func parseInboundChatMessages(endpoint string, body []byte) ([]service.ChatMessageRecordInput, []service.ChatMessageEventRecordInput) {
	switch {
	case strings.Contains(endpoint, "/chat/completions"):
		return parseChatCompletionsRequestMessages(body)
	case strings.Contains(endpoint, "/responses"):
		return parseResponsesRequestMessages(body)
	case strings.Contains(endpoint, "/messages"):
		return parseAnthropicRequestMessages(body)
	default:
		return parseGenericTextMessages(body, "user", "inbound"), nil
	}
}

func parseOutboundChatMessages(endpoint string, body []byte) ([]service.ChatMessageRecordInput, []service.ChatMessageEventRecordInput) {
	switch {
	case strings.Contains(endpoint, "/chat/completions"):
		return parseChatCompletionsResponseMessages(body)
	case strings.Contains(endpoint, "/responses"):
		return parseResponsesResponseMessages(body)
	case strings.Contains(endpoint, "/messages"):
		return parseAnthropicResponseMessages(body)
	default:
		return parseGenericTextMessages(body, "assistant", "outbound"), nil
	}
}

func parseChatCompletionsRequestMessages(body []byte) ([]service.ChatMessageRecordInput, []service.ChatMessageEventRecordInput) {
	items := make([]service.ChatMessageRecordInput, 0)
	for _, msg := range gjson.GetBytes(body, "messages").Array() {
		role := strings.TrimSpace(msg.Get("role").String())
		text := extractChatCompletionsMessageText(msg)
		raw := rawMessageFromGJSON(msg)
		if text == "" && len(raw) == 0 {
			continue
		}
		if role == "" || role == "user" {
			items = append(items, service.ChatMessageRecordInput{Role: "user", Direction: "inbound", ContentText: text, ContentJSON: raw})
		}
	}
	return items, nil
}

func parseChatCompletionsResponseMessages(body []byte) ([]service.ChatMessageRecordInput, []service.ChatMessageEventRecordInput) {
	trimmed := bytes.TrimSpace(body)
	if len(trimmed) == 0 {
		return nil, nil
	}
	if bytes.HasPrefix(trimmed, []byte("data:")) {
		text := collectChatCompletionsSSEText(trimmed)
		if text == "" {
			return nil, nil
		}
		return []service.ChatMessageRecordInput{{Role: "assistant", Direction: "outbound", ContentText: text}}, nil
	}

	items := make([]service.ChatMessageRecordInput, 0)
	for _, choice := range gjson.GetBytes(trimmed, "choices").Array() {
		message := choice.Get("message")
		text := extractChatCompletionsMessageText(message)
		if text == "" {
			text = extractChatCompletionsContent(choice.Get("delta.content"))
		}
		raw := rawMessageFromGJSON(message)
		if len(raw) == 0 {
			raw = rawMessageFromGJSON(choice)
		}
		if text == "" && len(raw) == 0 {
			continue
		}
		items = append(items, service.ChatMessageRecordInput{Role: "assistant", Direction: "outbound", ContentText: text, ContentJSON: raw})
	}
	return keepLastAssistantMessage(items), nil
}

func extractChatCompletionsMessageText(msg gjson.Result) string {
	text := extractChatCompletionsContent(msg.Get("content"))
	if text != "" {
		return text
	}
	if toolCalls := msg.Get("tool_calls"); toolCalls.IsArray() {
		return summarizeToolCalls(toolCalls)
	}
	if toolCallID := strings.TrimSpace(msg.Get("tool_call_id").String()); toolCallID != "" {
		if content := extractChatCompletionsContent(msg.Get("content")); content != "" {
			return "tool_result " + toolCallID + ": " + content
		}
		return "tool_result " + toolCallID
	}
	return ""
}

func extractChatCompletionsContent(result gjson.Result) string {
	switch result.Type {
	case gjson.String:
		return strings.TrimSpace(result.String())
	case gjson.JSON:
		if result.IsArray() {
			parts := make([]string, 0, len(result.Array()))
			for _, item := range result.Array() {
				if item.Type == gjson.String {
					if text := strings.TrimSpace(item.String()); text != "" {
						parts = append(parts, text)
					}
					continue
				}
				text := strings.TrimSpace(item.Get("text").String())
				if text == "" {
					text = strings.TrimSpace(item.Get("content").String())
				}
				if text != "" {
					parts = append(parts, text)
				}
			}
			return strings.TrimSpace(strings.Join(parts, "\n"))
		}
	}
	return ""
}

func parseAnthropicRequestMessages(body []byte) ([]service.ChatMessageRecordInput, []service.ChatMessageEventRecordInput) {
	items := make([]service.ChatMessageRecordInput, 0)
	for _, msg := range gjson.GetBytes(body, "messages").Array() {
		role := strings.TrimSpace(msg.Get("role").String())
		text := extractAnthropicContent(msg.Get("content"))
		raw := rawMessageFromGJSON(msg)
		if text == "" && len(raw) == 0 {
			continue
		}
		if role == "" || role == "user" {
			items = append(items, service.ChatMessageRecordInput{Role: "user", Direction: "inbound", ContentText: text, ContentJSON: raw})
		}
	}
	return items, nil
}

func parseAnthropicResponseMessages(body []byte) ([]service.ChatMessageRecordInput, []service.ChatMessageEventRecordInput) {
	trimmed := bytes.TrimSpace(body)
	if len(trimmed) == 0 {
		return nil, nil
	}
	if bytes.HasPrefix(trimmed, []byte("event:")) || bytes.HasPrefix(trimmed, []byte("data:")) {
		text := collectAnthropicSSEText(trimmed)
		if text == "" {
			return nil, nil
		}
		return []service.ChatMessageRecordInput{{Role: "assistant", Direction: "outbound", ContentText: text}}, nil
	}

	text := extractAnthropicContent(gjson.GetBytes(trimmed, "content"))
	raw := rawMessageFromGJSON(gjson.ParseBytes(trimmed))
	if text == "" && len(raw) == 0 {
		return nil, nil
	}
	return []service.ChatMessageRecordInput{{Role: "assistant", Direction: "outbound", ContentText: text, ContentJSON: raw}}, nil
}

func extractAnthropicContent(result gjson.Result) string {
	switch {
	case result.Type == gjson.String:
		return strings.TrimSpace(result.String())
	case result.IsArray():
		parts := make([]string, 0, len(result.Array()))
		for _, item := range result.Array() {
			text := strings.TrimSpace(item.Get("text").String())
			if text == "" {
				text = strings.TrimSpace(item.Get("thinking").String())
			}
			if text == "" {
				text = summarizeAnthropicToolBlock(item)
			}
			if text != "" {
				parts = append(parts, text)
			}
		}
		return strings.TrimSpace(strings.Join(parts, "\n"))
	default:
		return ""
	}
}

func parseResponsesRequestMessages(body []byte) ([]service.ChatMessageRecordInput, []service.ChatMessageEventRecordInput) {
	items := make([]service.ChatMessageRecordInput, 0)

	input := gjson.GetBytes(body, "input")
	switch {
	case input.Type == gjson.String:
		text := strings.TrimSpace(input.String())
		if text != "" {
			items = append(items, service.ChatMessageRecordInput{Role: "user", Direction: "inbound", ContentText: text})
		}
	case input.IsArray():
		for _, item := range input.Array() {
			role := strings.TrimSpace(item.Get("role").String())
			text := extractResponsesContent(item)
			raw := rawMessageFromGJSON(item)
			if text == "" && len(raw) == 0 {
				continue
			}
			if role == "" || role == "user" {
				items = append(items, service.ChatMessageRecordInput{Role: "user", Direction: "inbound", ContentText: text, ContentJSON: raw})
			}
		}
	}

	if len(items) == 0 {
		text := strings.TrimSpace(gjson.GetBytes(body, "prompt").String())
		if text != "" {
			items = append(items, service.ChatMessageRecordInput{Role: "user", Direction: "inbound", ContentText: text, ContentJSON: marshalChatContentJSON(map[string]any{
				"type": "prompt",
				"text": text,
			})})
		}
	}
	return items, nil
}

func parseResponsesResponseMessages(body []byte) ([]service.ChatMessageRecordInput, []service.ChatMessageEventRecordInput) {
	trimmed := bytes.TrimSpace(body)
	if len(trimmed) == 0 {
		return nil, nil
	}
	if bytes.HasPrefix(trimmed, []byte("data:")) {
		text := collectResponsesSSEText(trimmed)
		if text == "" {
			return nil, nil
		}
		return []service.ChatMessageRecordInput{{Role: "assistant", Direction: "outbound", ContentText: text}}, nil
	}

	items := make([]service.ChatMessageRecordInput, 0)
	for _, output := range gjson.GetBytes(trimmed, "output").Array() {
		role := strings.TrimSpace(output.Get("role").String())
		if role == "" {
			role = "assistant"
		}
		text := extractResponsesContent(output)
		raw := rawMessageFromGJSON(output)
		if text == "" && len(raw) == 0 {
			continue
		}
		items = append(items, service.ChatMessageRecordInput{Role: role, Direction: "outbound", ContentText: text, ContentJSON: raw})
	}
	return keepLastAssistantMessage(items), nil
}

func extractResponsesContent(result gjson.Result) string {
	if result.Type == gjson.String {
		return strings.TrimSpace(result.String())
	}
	switch result.Get("type").String() {
	case "input_text", "output_text", "text":
		if text := strings.TrimSpace(result.Get("text").String()); text != "" {
			return text
		}
	}
	content := result.Get("content")
	if content.IsArray() {
		parts := make([]string, 0, len(content.Array()))
		for _, item := range content.Array() {
			switch item.Get("type").String() {
			case "input_text", "output_text", "text":
				if text := strings.TrimSpace(item.Get("text").String()); text != "" {
					parts = append(parts, text)
				}
			default:
				if text := summarizeResponsesContentItem(item); text != "" {
					parts = append(parts, text)
				}
			}
		}
		return strings.TrimSpace(strings.Join(parts, "\n"))
	}
	if text := strings.TrimSpace(result.Get("text").String()); text != "" {
		return text
	}
	if text := summarizeResponsesContentItem(result); text != "" {
		return text
	}
	return ""
}

func parseGenericTextMessages(body []byte, role string, direction string) []service.ChatMessageRecordInput {
	if !gjson.ValidBytes(body) {
		return nil
	}
	if text := strings.TrimSpace(gjson.GetBytes(body, "text").String()); text != "" {
		return []service.ChatMessageRecordInput{{Role: role, Direction: direction, ContentText: text, ContentJSON: rawMessageFromGJSON(gjson.ParseBytes(body))}}
	}
	return nil
}

func rawMessageFromGJSON(result gjson.Result) json.RawMessage {
	raw := strings.TrimSpace(result.Raw)
	if raw == "" {
		return nil
	}
	if !json.Valid([]byte(raw)) {
		return nil
	}
	return json.RawMessage(raw)
}

func marshalChatContentJSON(value any) json.RawMessage {
	body, err := json.Marshal(value)
	if err != nil || !json.Valid(body) {
		return nil
	}
	return json.RawMessage(body)
}

func cloneChatRawJSON(body json.RawMessage) json.RawMessage {
	body = bytes.TrimSpace(body)
	if len(body) == 0 || !json.Valid(body) {
		return nil
	}
	out := make([]byte, len(body))
	copy(out, body)
	return json.RawMessage(out)
}

func chooseAssistantContentJSON(raw json.RawMessage, text string) json.RawMessage {
	if cloned := cloneChatRawJSON(raw); len(cloned) > 0 {
		return cloned
	}
	return marshalChatContentJSON(map[string]any{
		"type": "assistant_text",
		"text": text,
	})
}

func summarizeChatContentJSON(raw json.RawMessage) string {
	if len(raw) == 0 || !json.Valid(raw) {
		return ""
	}
	result := gjson.ParseBytes(raw)
	if text := extractResponsesContent(result); text != "" {
		return text
	}
	if text := extractChatCompletionsMessageText(result); text != "" {
		return text
	}
	if text := extractAnthropicContent(result.Get("content")); text != "" {
		return text
	}
	if output := result.Get("output"); output.IsArray() {
		parts := make([]string, 0, len(output.Array()))
		for _, item := range output.Array() {
			if text := extractResponsesContent(item); text != "" {
				parts = append(parts, text)
			}
		}
		if len(parts) > 0 {
			return strings.TrimSpace(strings.Join(parts, "\n"))
		}
	}
	if typ := strings.TrimSpace(result.Get("type").String()); typ != "" {
		return typ
	}
	return ""
}

type chatSessionCapturePayloadRef struct {
	Storage          string `json:"storage"`
	Path             string `json:"path"`
	SHA256           string `json:"sha256"`
	Bytes            int64  `json:"bytes"`
	StoredBytes      int64  `json:"stored_bytes,omitempty"`
	Compression      string `json:"compression,omitempty"`
	CompressionLevel int    `json:"compression_level,omitempty"`
}

func maybeExternalizeChatSessionCapturePayload(raw []byte, createdAt time.Time, kind string, summarize func([]byte) string) (json.RawMessage, string) {
	raw = bytes.TrimSpace(raw)
	if !shouldDropInlineChatSessionCapturePayload(raw) || !json.Valid(raw) {
		return nil, ""
	}
	if createdAt.IsZero() {
		createdAt = time.Now()
	}
	summary := ""
	if summarize != nil {
		summary = strings.TrimSpace(summarize(raw))
	}
	ref, err := writeChatSessionCapturePayload(raw, createdAt, kind)
	if err != nil {
		logger.L().Warn("chat_session.capture_payload_externalize_failed",
			zap.String("kind", kind),
			zap.Int("bytes", len(raw)),
			zap.Error(err),
		)
		return nil, summary
	}
	body, err := json.Marshal(ref)
	if err != nil || !json.Valid(body) {
		return nil, summary
	}
	return json.RawMessage(body), summary
}

func shouldDropInlineChatSessionCapturePayload(raw []byte) bool {
	return len(bytes.TrimSpace(raw)) > chatSessionInlineMaxBytes
}

func writeChatSessionCapturePayload(raw []byte, createdAt time.Time, kind string) (chatSessionCapturePayloadRef, error) {
	sum := sha256.Sum256(raw)
	sumHex := hex.EncodeToString(sum[:])
	compressed, err := gzipChatSessionCapturePayload(raw)
	if err != nil {
		return chatSessionCapturePayloadRef{}, err
	}
	dateDir := createdAt.UTC().Format("2006-01-02")
	relPath := filepath.ToSlash(filepath.Join(dateDir, sumHex+"-"+randomChatSessionCaptureHex(8)+"-"+sanitizeChatSessionCapturePayloadKind(kind)+".json.gz"))
	baseDir := chatSessionCapturePayloadBaseDir()
	fullPath, err := safeChatSessionCapturePayloadPath(baseDir, relPath)
	if err != nil {
		return chatSessionCapturePayloadRef{}, err
	}
	if err := os.MkdirAll(filepath.Dir(fullPath), 0750); err != nil {
		return chatSessionCapturePayloadRef{}, err
	}
	if err := os.WriteFile(fullPath, compressed, 0640); err != nil {
		return chatSessionCapturePayloadRef{}, err
	}
	return chatSessionCapturePayloadRef{
		Storage:          "file",
		Path:             relPath,
		SHA256:           sumHex,
		Bytes:            int64(len(raw)),
		StoredBytes:      int64(len(compressed)),
		Compression:      "gzip",
		CompressionLevel: gzip.BestSpeed,
	}, nil
}

func chatSessionCapturePayloadBaseDir() string {
	if value := strings.TrimSpace(os.Getenv("CHAT_SESSION_RETENTION_PAYLOAD_DIR")); value != "" {
		return value
	}
	return "./data/chat_session_payloads"
}

func safeChatSessionCapturePayloadPath(baseDir string, relPath string) (string, error) {
	baseDir = strings.TrimSpace(baseDir)
	if baseDir == "" {
		return "", os.ErrInvalid
	}
	cleanRel := filepath.Clean(filepath.FromSlash(strings.TrimSpace(relPath)))
	if cleanRel == "." || filepath.IsAbs(cleanRel) || cleanRel == ".." || strings.HasPrefix(cleanRel, ".."+string(filepath.Separator)) {
		return "", os.ErrInvalid
	}
	baseAbs, err := filepath.Abs(filepath.Clean(baseDir))
	if err != nil {
		return "", err
	}
	fullAbs, err := filepath.Abs(filepath.Join(baseAbs, cleanRel))
	if err != nil {
		return "", err
	}
	if fullAbs != baseAbs && !strings.HasPrefix(fullAbs, baseAbs+string(filepath.Separator)) {
		return "", os.ErrInvalid
	}
	return fullAbs, nil
}

func gzipChatSessionCapturePayload(raw []byte) ([]byte, error) {
	var buf bytes.Buffer
	writer, err := gzip.NewWriterLevel(&buf, gzip.BestSpeed)
	if err != nil {
		return nil, err
	}
	if _, err := writer.Write(raw); err != nil {
		_ = writer.Close()
		return nil, err
	}
	if err := writer.Close(); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

func randomChatSessionCaptureHex(n int) string {
	if n <= 0 {
		n = 8
	}
	buf := make([]byte, n)
	if _, err := rand.Read(buf); err != nil {
		return strconv.FormatInt(time.Now().UnixNano(), 36)
	}
	return hex.EncodeToString(buf)
}

func sanitizeChatSessionCapturePayloadKind(kind string) string {
	kind = strings.ToLower(strings.TrimSpace(kind))
	switch kind {
	case "request", "response", "message", "event":
		return kind
	default:
		return "payload"
	}
}

func summarizeInboundRequestBody(endpoint string, body []byte) string {
	switch {
	case strings.Contains(endpoint, "/chat/completions"):
		return summarizeChatCompletionsRequestBody(body)
	case strings.Contains(endpoint, "/responses"):
		return summarizeResponsesRequestBody(body)
	case strings.Contains(endpoint, "/messages"):
		return summarizeAnthropicRequestBody(body)
	default:
		return strings.TrimSpace(gjson.GetBytes(body, "text").String())
	}
}

func summarizeChatCompletionsRequestBody(body []byte) string {
	messages := gjson.GetBytes(body, "messages")
	if !messages.IsArray() {
		return ""
	}
	var text string
	for _, msg := range messages.Array() {
		role := strings.TrimSpace(msg.Get("role").String())
		if role != "" && role != "user" {
			continue
		}
		if candidate := extractChatCompletionsMessageText(msg); candidate != "" {
			text = candidate
		}
	}
	return truncateChatSessionCapturePreview(text, 4000)
}

func summarizeResponsesRequestBody(body []byte) string {
	input := gjson.GetBytes(body, "input")
	switch {
	case input.Type == gjson.String:
		return truncateChatSessionCapturePreview(input.String(), 4000)
	case input.IsArray():
		var text string
		for _, item := range input.Array() {
			role := strings.TrimSpace(item.Get("role").String())
			if role != "" && role != "user" {
				continue
			}
			if candidate := extractResponsesContent(item); candidate != "" {
				text = candidate
			}
		}
		return truncateChatSessionCapturePreview(text, 4000)
	default:
		return truncateChatSessionCapturePreview(gjson.GetBytes(body, "prompt").String(), 4000)
	}
}

func summarizeAnthropicRequestBody(body []byte) string {
	messages := gjson.GetBytes(body, "messages")
	if !messages.IsArray() {
		return ""
	}
	var text string
	for _, msg := range messages.Array() {
		role := strings.TrimSpace(msg.Get("role").String())
		if role != "" && role != "user" {
			continue
		}
		if candidate := extractAnthropicContent(msg.Get("content")); candidate != "" {
			text = candidate
		}
	}
	return truncateChatSessionCapturePreview(text, 4000)
}

func truncateChatSessionCapturePreview(value string, maxLen int) string {
	value = strings.TrimSpace(value)
	if value == "" || maxLen <= 0 {
		return ""
	}
	runes := []rune(value)
	if len(runes) <= maxLen {
		return value
	}
	return string(runes[:maxLen]) + "..."
}

func summarizeToolCalls(result gjson.Result) string {
	if !result.IsArray() {
		return ""
	}
	parts := make([]string, 0, len(result.Array()))
	for _, item := range result.Array() {
		name := strings.TrimSpace(item.Get("function.name").String())
		if name == "" {
			name = strings.TrimSpace(item.Get("name").String())
		}
		if name == "" {
			name = strings.TrimSpace(item.Get("type").String())
		}
		if name != "" {
			parts = append(parts, "tool_call "+name)
		}
	}
	return strings.TrimSpace(strings.Join(parts, "\n"))
}

func summarizeAnthropicToolBlock(item gjson.Result) string {
	switch item.Get("type").String() {
	case "tool_use":
		name := strings.TrimSpace(item.Get("name").String())
		if name == "" {
			name = strings.TrimSpace(item.Get("id").String())
		}
		if name != "" {
			return "tool_use " + name
		}
	case "tool_result":
		toolID := strings.TrimSpace(item.Get("tool_use_id").String())
		content := extractAnthropicContent(item.Get("content"))
		if toolID != "" && content != "" {
			return "tool_result " + toolID + ": " + content
		}
		if toolID != "" {
			return "tool_result " + toolID
		}
	}
	return ""
}

func summarizeResponsesContentItem(item gjson.Result) string {
	switch item.Get("type").String() {
	case "function_call":
		name := strings.TrimSpace(item.Get("name").String())
		if name == "" {
			name = strings.TrimSpace(item.Get("call_id").String())
		}
		if name != "" {
			return "function_call " + name
		}
	case "function_call_output":
		callID := strings.TrimSpace(item.Get("call_id").String())
		output := strings.TrimSpace(item.Get("output").String())
		if callID != "" && output != "" {
			return "function_call_output " + callID + ": " + output
		}
		if callID != "" {
			return "function_call_output " + callID
		}
	case "image_generation_call":
		if id := strings.TrimSpace(item.Get("id").String()); id != "" {
			return "image_generation_call " + id
		}
		return "image_generation_call"
	case "input_image", "image_url":
		return "image"
	}
	if imageURL := strings.TrimSpace(item.Get("image_url.url").String()); imageURL != "" {
		return "image"
	}
	if imageURL := strings.TrimSpace(item.Get("image_url").String()); imageURL != "" {
		return "image"
	}
	return ""
}

func collectChatCompletionsSSEText(body []byte) string {
	var parts []string
	for _, line := range bytes.Split(body, []byte{'\n'}) {
		line = bytes.TrimSpace(line)
		if !bytes.HasPrefix(line, []byte("data:")) {
			continue
		}
		payload := bytes.TrimSpace(bytes.TrimPrefix(line, []byte("data:")))
		if bytes.Equal(payload, []byte("[DONE]")) || len(payload) == 0 || !gjson.ValidBytes(payload) {
			continue
		}
		for _, choice := range gjson.GetBytes(payload, "choices").Array() {
			if text := extractChatCompletionsContent(choice.Get("delta.content")); text != "" {
				parts = append(parts, text)
			}
		}
	}
	return strings.TrimSpace(strings.Join(parts, ""))
}

func collectResponsesSSEText(body []byte) string {
	var parts []string
	for _, line := range bytes.Split(body, []byte{'\n'}) {
		line = bytes.TrimSpace(line)
		if !bytes.HasPrefix(line, []byte("data:")) {
			continue
		}
		payload := bytes.TrimSpace(bytes.TrimPrefix(line, []byte("data:")))
		if bytes.Equal(payload, []byte("[DONE]")) || len(payload) == 0 || !gjson.ValidBytes(payload) {
			continue
		}
		switch gjson.GetBytes(payload, "type").String() {
		case "response.output_text.delta":
			if text := strings.TrimSpace(gjson.GetBytes(payload, "delta").String()); text != "" {
				parts = append(parts, text)
			}
		case "response.output_item.added", "response.output_item.done":
			if text := extractResponsesContent(gjson.GetBytes(payload, "item")); text != "" {
				parts = append(parts, text)
			}
		case "response.completed", "response.done":
			if text := extractResponsesContent(gjson.GetBytes(payload, "response")); text != "" {
				parts = append(parts, text)
			}
		}
	}
	return strings.TrimSpace(strings.Join(parts, ""))
}

func collectAnthropicSSEText(body []byte) string {
	var parts []string
	for _, line := range bytes.Split(body, []byte{'\n'}) {
		line = bytes.TrimSpace(line)
		if !bytes.HasPrefix(line, []byte("data:")) {
			continue
		}
		payload := bytes.TrimSpace(bytes.TrimPrefix(line, []byte("data:")))
		if len(payload) == 0 || bytes.Equal(payload, []byte("[DONE]")) || !gjson.ValidBytes(payload) {
			continue
		}
		if gjson.GetBytes(payload, "type").String() == "content_block_delta" {
			if text := strings.TrimSpace(gjson.GetBytes(payload, "delta.text").String()); text != "" {
				parts = append(parts, text)
			}
		}
	}
	return strings.TrimSpace(strings.Join(parts, ""))
}

func buildChatSessionRecordInput(
	apiKey *service.APIKey,
	account *service.Account,
	sessionKey string,
	requestID string,
	reqModel string,
	stream bool,
	requestType service.RequestType,
	httpStatus int,
	inboundEndpoint string,
	upstreamEndpoint string,
	requestedModel string,
	upstreamModel string,
) *service.ChatSessionRecordInput {
	if apiKey == nil {
		return nil
	}
	input := &service.ChatSessionRecordInput{
		SessionKey:     strings.TrimSpace(sessionKey),
		RequestID:      strings.TrimSpace(requestID),
		Platform:       resolveChatSessionPlatform(apiKey, account),
		Model:          strings.TrimSpace(reqModel),
		RequestType:    requestType,
		Stream:         stream,
		Status:         http.StatusText(httpStatus),
		HTTPStatusCode: httpStatus,
		CreatedAt:      time.Now(),
	}
	if input.Status == "" {
		input.Status = "completed"
	}
	if inbound := strings.TrimSpace(inboundEndpoint); inbound != "" {
		input.InboundEndpoint = &inbound
	}
	if upstream := strings.TrimSpace(upstreamEndpoint); upstream != "" {
		input.UpstreamEndpoint = &upstream
	}
	if requested := strings.TrimSpace(requestedModel); requested != "" {
		input.RequestedModel = &requested
	}
	if upstreamModel = strings.TrimSpace(upstreamModel); upstreamModel != "" {
		input.UpstreamModel = &upstreamModel
	}
	if apiKey.GroupID != nil {
		input.GroupID = apiKey.GroupID
	}
	if account != nil {
		input.AccountID = &account.ID
	}
	return input
}

func resolveChatSessionPlatform(apiKey *service.APIKey, account *service.Account) string {
	if account != nil && strings.TrimSpace(account.Platform) != "" {
		return strings.TrimSpace(account.Platform)
	}
	if apiKey != nil && apiKey.Group != nil && strings.TrimSpace(apiKey.Group.Platform) != "" {
		return strings.TrimSpace(apiKey.Group.Platform)
	}
	return ""
}

func keepLastAssistantMessage(items []service.ChatMessageRecordInput) []service.ChatMessageRecordInput {
	if len(items) == 0 {
		return nil
	}
	return []service.ChatMessageRecordInput{items[len(items)-1]}
}
