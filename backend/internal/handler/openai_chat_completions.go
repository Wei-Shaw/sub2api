package handler

import (
	"bytes"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
)

// ChatCompletions handles OpenAI Chat Completions API compatibility.
// POST /v1/chat/completions
func (h *OpenAIGatewayHandler) ChatCompletions(c *gin.Context) {
	body, err := io.ReadAll(c.Request.Body)
	if err != nil {
		if maxErr, ok := extractMaxBytesError(err); ok {
			h.errorResponse(c, http.StatusRequestEntityTooLarge, "invalid_request_error", buildBodyTooLargeMessage(maxErr.Limit))
			return
	REDACTED
		h.errorResponse(c, http.StatusBadRequest, "invalid_request_error", "Failed to read request body")
		return
REDACTED
	if len(body) == 0 {
		h.errorResponse(c, http.StatusBadRequest, "invalid_request_error", "Request body is empty")
		return
REDACTED

	// Preserve original chat-completions request for upstream passthrough when needed.
	c.Set(service.OpenAIChatCompletionsBodyKey, body)

	var chatReq map[string]any
	if err := json.Unmarshal(body, &chatReq); err != nil {
		h.errorResponse(c, http.StatusBadRequest, "invalid_request_error", "Failed to parse request body")
		return
REDACTED

	includeUsage := false
	if streamOptions, ok := chatReq["stream_options"].(map[string]any); ok {
		if v, ok := streamOptions["include_usage"].(bool); ok {
			includeUsage = v
	REDACTED
REDACTED
	c.Set(service.OpenAIChatCompletionsIncludeUsageKey, includeUsage)

	converted, err := service.ConvertChatCompletionsToResponses(chatReq)
	if err != nil {
		h.errorResponse(c, http.StatusBadRequest, "invalid_request_error", err.Error())
		return
REDACTED

	convertedBody, err := json.Marshal(converted)
	if err != nil {
		h.errorResponse(c, http.StatusInternalServerError, "api_error", "Failed to process request")
		return
REDACTED

	stream, _ := converted["stream"].(bool)
	model, _ := converted["model"].(string)
	originalWriter := c.Writer
	writer := newChatCompletionsResponseWriter(c.Writer, stream, includeUsage, model)
	c.Writer = writer
	c.Request.Body = io.NopCloser(bytes.NewReader(convertedBody))
	c.Request.ContentLength = int64(len(convertedBody))

	h.Responses(c)
	writer.Finalize()
	c.Writer = originalWriter
REDACTED

type chatCompletionsResponseWriter struct {
	gin.ResponseWriter
	stream       bool
	includeUsage bool
	buffer       bytes.Buffer
	streamBuf    bytes.Buffer
	state        *chatCompletionStreamState
	corrector    *service.CodexToolCorrector
	finalized    bool
	passthrough  bool
REDACTED

type chatCompletionStreamState struct {
	id            string
	model         string
	created       int64
	sentRole      bool
	sawToolCall   bool
	sawText       bool
	toolCallIndex map[string]int
	usage         map[string]any
REDACTED

func newChatCompletionsResponseWriter(w gin.ResponseWriter, stream bool, includeUsage bool, model string) *chatCompletionsResponseWriter {
	return &chatCompletionsResponseWriter{
		ResponseWriter: w,
		stream:         stream,
		includeUsage:   includeUsage,
		state: &chatCompletionStreamState{
			model:         strings.TrimSpace(model),
			toolCallIndex: make(map[string]int),
	REDACTED,
		corrector: service.NewCodexToolCorrector(),
REDACTED
REDACTED

func (w *chatCompletionsResponseWriter) Write(data []byte) (int, error) {
	if w.passthrough {
		return w.ResponseWriter.Write(data)
REDACTED
	if w.stream {
		n, err := w.streamBuf.Write(data)
		if err != nil {
			return n, err
	REDACTED
		w.flushStreamBuffer()
		return n, nil
REDACTED

	if w.finalized {
		return len(data), nil
REDACTED
	return w.buffer.Write(data)
REDACTED

func (w *chatCompletionsResponseWriter) WriteString(s string) (int, error) {
	return w.Write([]byte(s))
REDACTED

func (w *chatCompletionsResponseWriter) Finalize() {
	if w.finalized {
		return
REDACTED
	w.finalized = true
	if w.passthrough {
		return
REDACTED
	if w.stream {
		return
REDACTED

	body := w.buffer.Bytes()
	if len(body) == 0 {
		return
REDACTED

	w.ResponseWriter.Header().Del("Content-Length")

	converted, err := service.ConvertResponsesToChatCompletion(body)
	if err != nil {
		_, _ = w.ResponseWriter.Write(body)
		return
REDACTED

	corrected := converted
	if correctedStr, ok := w.corrector.CorrectToolCallsInSSEData(string(converted)); ok {
		corrected = []byte(correctedStr)
REDACTED

	_, _ = w.ResponseWriter.Write(corrected)
REDACTED

func (w *chatCompletionsResponseWriter) SetPassthrough() {
	w.passthrough = true
REDACTED

func (w *chatCompletionsResponseWriter) Status() int {
	if w.ResponseWriter == nil {
		return 0
REDACTED
	return w.ResponseWriter.Status()
REDACTED

func (w *chatCompletionsResponseWriter) Written() bool {
	if w.ResponseWriter == nil {
		return false
REDACTED
	return w.ResponseWriter.Written()
REDACTED

func (w *chatCompletionsResponseWriter) flushStreamBuffer() {
	for {
		buf := w.streamBuf.Bytes()
		idx := bytes.IndexByte(buf, '\n')
		if idx == -1 {
			return
	REDACTED
		lineBytes := w.streamBuf.Next(idx + 1)
		line := strings.TrimRight(string(lineBytes), "\r\n")
		w.handleStreamLine(line)
REDACTED
REDACTED

func (w *chatCompletionsResponseWriter) handleStreamLine(line string) {
	if line == "" {
		return
REDACTED
	if strings.HasPrefix(line, ":") {
		_, _ = w.ResponseWriter.Write([]byte(line + "\n\n"))
		return
REDACTED
	if !strings.HasPrefix(line, "data:") {
		return
REDACTED

	data := strings.TrimSpace(strings.TrimPrefix(line, "data:"))
	for _, chunk := range w.convertResponseDataToChatChunks(data) {
		if chunk == "" {
			continue
	REDACTED
		if chunk == "[DONE]" {
			_, _ = w.ResponseWriter.Write([]byte("data: [DONE]\n\n"))
			continue
	REDACTED
		_, _ = w.ResponseWriter.Write([]byte("data: " + chunk + "\n\n"))
REDACTED
REDACTED

func (w *chatCompletionsResponseWriter) convertResponseDataToChatChunks(data string) []string {
	if data == "" {
		return nil
REDACTED
	if data == "[DONE]" {
		return []string{"[DONE]"REDACTED
REDACTED

	var payload map[string]any
	if err := json.Unmarshal([]byte(data), &payload); err != nil {
		return []string{dataREDACTED
REDACTED

	if _, ok := payload["error"]; ok {
		return []string{dataREDACTED
REDACTED

	eventType := strings.TrimSpace(getString(payload["type"]))
	if eventType == "" {
		return []string{dataREDACTED
REDACTED

	w.state.applyMetadata(payload)

	switch eventType {
	case "response.created":
		return nil
	case "response.output_text.delta":
		delta := getString(payload["delta"])
		if delta == "" {
			return nil
	REDACTED
		w.state.sawText = true
		return []string{w.buildTextDeltaChunk(delta)REDACTED
	case "response.output_text.done":
		if w.state.sawText {
			return nil
	REDACTED
		text := getString(payload["text"])
		if text == "" {
			return nil
	REDACTED
		w.state.sawText = true
		return []string{w.buildTextDeltaChunk(text)REDACTED
	case "response.output_item.added", "response.output_item.delta":
		if item, ok := payload["item"].(map[string]any); ok {
			if callID, name, args, ok := extractToolCallFromItem(item); ok {
				w.state.sawToolCall = true
				return []string{w.buildToolCallChunk(callID, name, args)REDACTED
		REDACTED
	REDACTED
	case "response.completed", "response.done":
		if responseObj, ok := payload["response"].(map[string]any); ok {
			w.state.applyResponseUsage(responseObj)
	REDACTED
		return []string{w.buildFinalChunk()REDACTED
REDACTED

	if strings.Contains(eventType, "tool_call") || strings.Contains(eventType, "function_call") {
		callID := strings.TrimSpace(getString(payload["call_id"]))
		if callID == "" {
			callID = strings.TrimSpace(getString(payload["tool_call_id"]))
	REDACTED
		if callID == "" {
			callID = strings.TrimSpace(getString(payload["id"]))
	REDACTED
		args := getString(payload["delta"])
		name := strings.TrimSpace(getString(payload["name"]))
		if callID != "" && (args != "" || name != "") {
			w.state.sawToolCall = true
			return []string{w.buildToolCallChunk(callID, name, args)REDACTED
	REDACTED
REDACTED

	return nil
REDACTED

func (w *chatCompletionsResponseWriter) buildTextDeltaChunk(delta string) string {
	w.state.ensureDefaults()
	payload := map[string]any{
		"content": delta,
REDACTED
	if !w.state.sentRole {
		payload["role"] = "assistant"
		w.state.sentRole = true
REDACTED
	return w.buildChunk(payload, nil, nil)
REDACTED

func (w *chatCompletionsResponseWriter) buildToolCallChunk(callID, name, args string) string {
	w.state.ensureDefaults()
	index := w.state.toolCallIndexFor(callID)
	function := map[string]any{REDACTED
	if name != "" {
		function["name"] = name
REDACTED
	if args != "" {
		function["arguments"] = args
REDACTED
	toolCall := map[string]any{
		"index":    index,
		"id":       callID,
		"type":     "function",
		"function": function,
REDACTED

	delta := map[string]any{
		"tool_calls": []any{toolCallREDACTED,
REDACTED
	if !w.state.sentRole {
		delta["role"] = "assistant"
		w.state.sentRole = true
REDACTED

	return w.buildChunk(delta, nil, nil)
REDACTED

func (w *chatCompletionsResponseWriter) buildFinalChunk() string {
	w.state.ensureDefaults()
	finishReason := "stop"
	if w.state.sawToolCall {
		finishReason = "tool_calls"
REDACTED
	usage := map[string]any(nil)
	if w.includeUsage && w.state.usage != nil {
		usage = w.state.usage
REDACTED
	return w.buildChunk(map[string]any{REDACTED, finishReason, usage)
REDACTED

func (w *chatCompletionsResponseWriter) buildChunk(delta map[string]any, finishReason any, usage map[string]any) string {
	w.state.ensureDefaults()
	chunk := map[string]any{
		"id":      w.state.id,
		"object":  "chat.completion.chunk",
		"created": w.state.created,
		"model":   w.state.model,
		"choices": []any{
			map[string]any{
				"index":         0,
				"delta":         delta,
				"finish_reason": finishReason,
		REDACTED,
	REDACTED,
REDACTED
	if usage != nil {
		chunk["usage"] = usage
REDACTED

	data, _ := json.Marshal(chunk)
	if corrected, ok := w.corrector.CorrectToolCallsInSSEData(string(data)); ok {
		return corrected
REDACTED
	return string(data)
REDACTED

func (s *chatCompletionStreamState) ensureDefaults() {
	if s.id == "" {
		s.id = "chatcmpl-" + randomHexUnsafe(12)
REDACTED
	if s.model == "" {
		s.model = "unknown"
REDACTED
	if s.created == 0 {
		s.created = time.Now().Unix()
REDACTED
REDACTED

func (s *chatCompletionStreamState) toolCallIndexFor(callID string) int {
	if idx, ok := s.toolCallIndex[callID]; ok {
		return idx
REDACTED
	idx := len(s.toolCallIndex)
	s.toolCallIndex[callID] = idx
	return idx
REDACTED

func (s *chatCompletionStreamState) applyMetadata(payload map[string]any) {
	if responseObj, ok := payload["response"].(map[string]any); ok {
		s.applyResponseMetadata(responseObj)
REDACTED

	if s.id == "" {
		if id := strings.TrimSpace(getString(payload["response_id"])); id != "" {
			s.id = id
	REDACTED else if id := strings.TrimSpace(getString(payload["id"])); id != "" {
			s.id = id
	REDACTED
REDACTED
	if s.model == "" {
		if model := strings.TrimSpace(getString(payload["model"])); model != "" {
			s.model = model
	REDACTED
REDACTED
	if s.created == 0 {
		if created := getInt64(payload["created_at"]); created != 0 {
			s.created = created
	REDACTED else if created := getInt64(payload["created"]); created != 0 {
			s.created = created
	REDACTED
REDACTED
REDACTED

func (s *chatCompletionStreamState) applyResponseMetadata(responseObj map[string]any) {
	if s.id == "" {
		if id := strings.TrimSpace(getString(responseObj["id"])); id != "" {
			s.id = id
	REDACTED
REDACTED
	if s.model == "" {
		if model := strings.TrimSpace(getString(responseObj["model"])); model != "" {
			s.model = model
	REDACTED
REDACTED
	if s.created == 0 {
		if created := getInt64(responseObj["created_at"]); created != 0 {
			s.created = created
	REDACTED
REDACTED
REDACTED

func (s *chatCompletionStreamState) applyResponseUsage(responseObj map[string]any) {
	usage, ok := responseObj["usage"].(map[string]any)
	if !ok {
		return
REDACTED
	promptTokens := int(getNumber(usage["input_tokens"]))
	completionTokens := int(getNumber(usage["output_tokens"]))
	if promptTokens == 0 && completionTokens == 0 {
		return
REDACTED
	s.usage = map[string]any{
		"prompt_tokens":     promptTokens,
		"completion_tokens": completionTokens,
		"total_tokens":      promptTokens + completionTokens,
REDACTED
REDACTED

func extractToolCallFromItem(item map[string]any) (string, string, string, bool) {
	itemType := strings.TrimSpace(getString(item["type"]))
	if itemType != "tool_call" && itemType != "function_call" {
		return "", "", "", false
REDACTED
	callID := strings.TrimSpace(getString(item["call_id"]))
	if callID == "" {
		callID = strings.TrimSpace(getString(item["id"]))
REDACTED
	name := strings.TrimSpace(getString(item["name"]))
	args := getString(item["arguments"])
	if fn, ok := item["function"].(map[string]any); ok {
		if name == "" {
			name = strings.TrimSpace(getString(fn["name"]))
	REDACTED
		if args == "" {
			args = getString(fn["arguments"])
	REDACTED
REDACTED
	if callID == "" && name == "" && args == "" {
		return "", "", "", false
REDACTED
	if callID == "" {
		callID = "call_" + randomHexUnsafe(6)
REDACTED
	return callID, name, args, true
REDACTED

func getString(value any) string {
	switch v := value.(type) {
	case string:
		return v
	case []byte:
		return string(v)
	case json.Number:
		return v.String()
	default:
		return ""
REDACTED
REDACTED

func getNumber(value any) float64 {
	switch v := value.(type) {
	case float64:
		return v
	case float32:
		return float64(v)
	case int:
		return float64(v)
	case int64:
		return float64(v)
	case json.Number:
		f, _ := v.Float64()
		return f
	default:
		return 0
REDACTED
REDACTED

func getInt64(value any) int64 {
	switch v := value.(type) {
	case int64:
		return v
	case int:
		return int64(v)
	case float64:
		return int64(v)
	case json.Number:
		i, _ := v.Int64()
		return i
	default:
		return 0
REDACTED
REDACTED

func randomHexUnsafe(byteLength int) string {
	if byteLength <= 0 {
		byteLength = 8
REDACTED
	buf := make([]byte, byteLength)
	if _, err := rand.Read(buf); err != nil {
		return "000000"
REDACTED
	return hex.EncodeToString(buf)
REDACTED
