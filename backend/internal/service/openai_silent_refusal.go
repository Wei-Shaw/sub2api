package service

import (
	"bytes"
	"encoding/json"
	"net/http"
	"strings"

	"github.com/Wei-Shaw/sub2api/internal/pkg/apicompat"
	"github.com/gin-gonic/gin"
	"github.com/tidwall/gjson"
)

const (
	openAISilentRefusalMinRequestBodyBytes = 64 * 1024
	openAISilentRefusalErrorCode           = "openai_silent_refusal"
	openAISilentRefusalUpstreamMessage     = "OpenAI upstream returned an empty completion stream with finish_reason=stop and no usage"
	openAISilentRefusalClientMessage       = "Upstream returned an empty completion without usage; no fallback account was available"
	openAIResponsesEmptyCompletedMessage   = "OpenAI upstream returned an empty response.completed stream with no output and no usage"
)

type openAIChatSilentRefusalDetector struct {
	enabled         bool
	sawContent      bool
	sawToolCall     bool
	sawFunctionCall bool
	sawUsage        bool
	sawError        bool
	sawReasoning    bool
	sawFinish       bool
	finishReason    string
REDACTED

func newOpenAIChatSilentRefusalDetector(requestBodyLen int) *openAIChatSilentRefusalDetector {
	return &openAIChatSilentRefusalDetector{
		enabled: requestBodyLen >= openAISilentRefusalMinRequestBodyBytes,
REDACTED
REDACTED

func (d *openAIChatSilentRefusalDetector) Enabled() bool {
	return d != nil && d.enabled
REDACTED

func (d *openAIChatSilentRefusalDetector) ObserveSSELine(line string) {
	if d == nil || !d.enabled {
		return
REDACTED
	if eventType, ok := extractOpenAISSEEventLine(line); ok {
		d.observeEventType(eventType)
		return
REDACTED
	if payload, ok := extractOpenAISSEDataLine(line); ok {
		d.ObservePayload([]byte(payload))
REDACTED
REDACTED

func (d *openAIChatSilentRefusalDetector) ObservePayload(payload []byte) {
	if d == nil || !d.enabled {
		return
REDACTED
	payload = bytes.TrimSpace(payload)
	if len(payload) == 0 || bytes.Equal(payload, []byte("[DONE]")) {
		return
REDACTED
	if !gjson.ValidBytes(payload) {
		return
REDACTED

	eventType := strings.TrimSpace(gjson.GetBytes(payload, "type").String())
	d.observeEventType(eventType)

	if gjson.GetBytes(payload, "error").Exists() {
		d.sawError = true
REDACTED
	if usage := gjson.GetBytes(payload, "usage"); usage.Exists() && usage.IsObject() {
		d.sawUsage = true
REDACTED
	if usage := gjson.GetBytes(payload, "response.usage"); usage.Exists() && usage.IsObject() {
		d.sawUsage = true
REDACTED

	d.observeChatChoicesPayload(payload)
	d.observeResponsesPayload(payload, eventType)
REDACTED

func (d *openAIChatSilentRefusalDetector) ObserveChatChunk(chunk apicompat.ChatCompletionsChunk) {
	if d == nil || !d.enabled {
		return
REDACTED
	if chunk.Usage != nil {
		d.sawUsage = true
REDACTED
	for _, choice := range chunk.Choices {
		if choice.FinishReason != nil {
			d.observeFinishReason(*choice.FinishReason)
	REDACTED
		delta := choice.Delta
		if delta.Content != nil && *delta.Content != "" {
			d.sawContent = true
	REDACTED
		if delta.ReasoningContent != nil {
			d.sawReasoning = true
	REDACTED
		if len(delta.ToolCalls) > 0 {
			d.sawToolCall = true
	REDACTED
REDACTED
REDACTED

func (d *openAIChatSilentRefusalDetector) ShouldReleaseClientOutput() bool {
	if d == nil || !d.enabled {
		return true
REDACTED
	if d.sawContent || d.sawToolCall || d.sawFunctionCall || d.sawUsage || d.sawError || d.sawReasoning {
		return true
REDACTED
	return d.sawFinish && d.finishReason != "" && d.finishReason != "stop"
REDACTED

func (d *openAIChatSilentRefusalDetector) IsSilentRefusal() bool {
	if d == nil || !d.enabled {
		return false
REDACTED
	return !d.sawContent &&
		!d.sawToolCall &&
		!d.sawFunctionCall &&
		!d.sawUsage &&
		!d.sawError &&
		!d.sawReasoning &&
		d.sawFinish &&
		d.finishReason == "stop"
REDACTED

func (d *openAIChatSilentRefusalDetector) observeEventType(eventType string) {
	eventType = strings.TrimSpace(eventType)
	if eventType == "" {
		return
REDACTED
	if eventType == "error" || eventType == "response.failed" {
		d.sawError = true
REDACTED
	if strings.Contains(eventType, "reasoning") || strings.Contains(eventType, "reasoning_summary") {
		d.sawReasoning = true
REDACTED
REDACTED

func (d *openAIChatSilentRefusalDetector) observeFinishReason(reason string) {
	reason = strings.TrimSpace(reason)
	if reason == "" {
		return
REDACTED
	d.sawFinish = true
	d.finishReason = reason
REDACTED

func (d *openAIChatSilentRefusalDetector) observeChatChoicesPayload(payload []byte) {
	choices := gjson.GetBytes(payload, "choices")
	if !choices.Exists() || !choices.IsArray() {
		return
REDACTED
	for _, choice := range choices.Array() {
		if finish := choice.Get("finish_reason"); finish.Exists() {
			d.observeFinishReason(finish.String())
	REDACTED
		delta := choice.Get("delta")
		if !delta.Exists() {
			continue
	REDACTED
		if content := delta.Get("content"); content.Exists() && content.String() != "" {
			d.sawContent = true
	REDACTED
		if delta.Get("tool_calls").Exists() {
			d.sawToolCall = true
	REDACTED
		if delta.Get("function_call").Exists() {
			d.sawFunctionCall = true
	REDACTED
		if delta.Get("reasoning").Exists() ||
			delta.Get("reasoning_content").Exists() ||
			delta.Get("reasoning_summary").Exists() {
			d.sawReasoning = true
	REDACTED
REDACTED
REDACTED

func (d *openAIChatSilentRefusalDetector) observeResponsesPayload(payload []byte, eventType string) {
	switch eventType {
	case "response.output_text.delta":
		if gjson.GetBytes(payload, "delta").String() != "" {
			d.sawContent = true
	REDACTED
	case "response.output_item.added":
		switch strings.TrimSpace(gjson.GetBytes(payload, "item.type").String()) {
		case "function_call":
			d.sawToolCall = true
		case "reasoning":
			d.sawReasoning = true
	REDACTED
	case "response.function_call_arguments.delta":
		d.sawToolCall = true
	case "response.reasoning_summary_text.delta", "response.reasoning_summary_text.done":
		d.sawReasoning = true
	case "response.completed", "response.done":
		d.observeFinishReason("stop")
	case "response.incomplete":
		d.observeFinishReason("length")
	case "response.failed":
		d.sawError = true
REDACTED

	if output := gjson.GetBytes(payload, "response.output"); output.Exists() && output.IsArray() {
		for _, item := range output.Array() {
			switch strings.TrimSpace(item.Get("type").String()) {
			case "function_call":
				d.sawToolCall = true
			case "reasoning":
				d.sawReasoning = true
			case "message":
				d.observeResponseMessageItem(item)
		REDACTED
	REDACTED
REDACTED
REDACTED

func (d *openAIChatSilentRefusalDetector) observeResponseMessageItem(item gjson.Result) {
	content := item.Get("content")
	if !content.Exists() || !content.IsArray() {
		return
REDACTED
	for _, part := range content.Array() {
		if part.Get("text").String() != "" {
			d.sawContent = true
			return
	REDACTED
REDACTED
REDACTED

func newOpenAISilentRefusalFailoverError(c *gin.Context, account *Account, upstreamRequestID string) *UpstreamFailoverError {
	accountID := int64(0)
	accountName := ""
	platform := PlatformOpenAI
	if account != nil {
		accountID = account.ID
		accountName = account.Name
		platform = account.Platform
REDACTED

	setOpsUpstreamError(c, http.StatusBadGateway, openAISilentRefusalUpstreamMessage, "")
	appendOpsUpstreamError(c, OpsUpstreamErrorEvent{
		Platform:           platform,
		AccountID:          accountID,
		AccountName:        accountName,
		UpstreamStatusCode: http.StatusBadGateway,
		UpstreamRequestID:  upstreamRequestID,
		Kind:               "failover",
		Message:            openAISilentRefusalUpstreamMessage,
REDACTED)

	headers := http.Header{REDACTED
	if strings.TrimSpace(upstreamRequestID) != "" {
		headers.Set("x-request-id", strings.TrimSpace(upstreamRequestID))
REDACTED
	return &UpstreamFailoverError{
		StatusCode:      http.StatusBadGateway,
		ResponseBody:    openAISilentRefusalErrorBody(),
		ResponseHeaders: headers,
REDACTED
REDACTED

// newOpenAIResponsesEmptyCompletedFailoverError marks an empty
// response.completed terminal event as a retryable upstream anomaly. OpenAI
// Responses streams that deliver only response.created + response.completed
// with no output, no usage and no error are treated as silent upstream
// refusals rather than successful empty replies (issue #5009).
func newOpenAIResponsesEmptyCompletedFailoverError(c *gin.Context, account *Account, upstreamRequestID string) *UpstreamFailoverError {
	accountID := int64(0)
	accountName := ""
	platform := PlatformOpenAI
	if account != nil {
		accountID = account.ID
		accountName = account.Name
		platform = account.Platform
REDACTED

	setOpsUpstreamError(c, http.StatusBadGateway, openAIResponsesEmptyCompletedMessage, "")
	appendOpsUpstreamError(c, OpsUpstreamErrorEvent{
		Platform:           platform,
		AccountID:          accountID,
		AccountName:        accountName,
		UpstreamStatusCode: http.StatusBadGateway,
		UpstreamRequestID:  upstreamRequestID,
		Kind:               "failover",
		Message:            openAIResponsesEmptyCompletedMessage,
REDACTED)

	headers := http.Header{REDACTED
	if strings.TrimSpace(upstreamRequestID) != "" {
		headers.Set("x-request-id", strings.TrimSpace(upstreamRequestID))
REDACTED
	return &UpstreamFailoverError{
		StatusCode:      http.StatusBadGateway,
		ResponseBody:    openAISilentRefusalErrorBody(),
		ResponseHeaders: headers,
REDACTED
REDACTED

func openAISilentRefusalErrorBody() []byte {
	body, err := json.Marshal(map[string]any{
		"error": map[string]any{
			"type":    "upstream_error",
			"code":    openAISilentRefusalErrorCode,
			"message": openAISilentRefusalUpstreamMessage,
	REDACTED,
REDACTED)
	if err != nil {
		return []byte(`{"error":{"type":"upstream_error","code":"openai_silent_refusal","message":"OpenAI upstream returned an empty completion stream with finish_reason=stop and no usage"REDACTEDREDACTED`)
REDACTED
	return body
REDACTED

// IsOpenAISilentRefusalErrorBody reports whether a failover body was produced
// by the OpenAI silent-refusal detector.
func IsOpenAISilentRefusalErrorBody(body []byte) bool {
	return strings.TrimSpace(gjson.GetBytes(body, "error.code").String()) == openAISilentRefusalErrorCode
REDACTED

// OpenAISilentRefusalClientMessage returns the exhausted-failover client message
// for OpenAI silent refusals.
func OpenAISilentRefusalClientMessage() string {
	return openAISilentRefusalClientMessage
REDACTED
