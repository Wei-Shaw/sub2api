package service

import (
	"bufio"
	"bytes"
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync/atomic"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/logger"
	"github.com/Wei-Shaw/sub2api/internal/util/responseheaders"
	"github.com/gin-gonic/gin"
	"github.com/tidwall/gjson"
	"github.com/tidwall/sjson"
)

type openAIResponsesImageResult struct {
	Result        string
	RevisedPrompt string
	OutputFormat  string
	Size          string
	Background    string
	Quality       string
	Model         string
REDACTED

type OpenAIImagesUpstreamError struct {
	StatusCode        int
	ErrorType         string
	Code              string
	Message           string
	Param             string
	UpstreamRequestID string
REDACTED

func (e *OpenAIImagesUpstreamError) Error() string {
	if e == nil {
		return ""
REDACTED
	code := strings.TrimSpace(e.Code)
	if code == "" {
		code = strings.TrimSpace(e.ErrorType)
REDACTED
	message := strings.TrimSpace(e.Message)
	if code != "" && message != "" {
		return fmt.Sprintf("openai images upstream error: %s: %s", code, message)
REDACTED
	if message != "" {
		return "openai images upstream error: " + message
REDACTED
	if code != "" {
		return "openai images upstream error: " + code
REDACTED
	return "openai images upstream error"
REDACTED

func (e *OpenAIImagesUpstreamError) clientStatusCode() int {
	if e == nil {
		return http.StatusBadGateway
REDACTED
	if e.StatusCode > 0 {
		return e.StatusCode
REDACTED
	return http.StatusBadGateway
REDACTED

func (e *OpenAIImagesUpstreamError) clientErrorType() string {
	if e == nil {
		return "upstream_error"
REDACTED
	if trimmed := strings.TrimSpace(e.ErrorType); trimmed != "" {
		return trimmed
REDACTED
	return "upstream_error"
REDACTED

func (e *OpenAIImagesUpstreamError) clientMessage() string {
	if e == nil {
		return "Upstream request failed"
REDACTED
	if trimmed := strings.TrimSpace(e.Message); trimmed != "" {
		return trimmed
REDACTED
	if trimmed := strings.TrimSpace(e.Code); trimmed != "" {
		return trimmed
REDACTED
	return "Upstream request failed"
REDACTED

// IsOpenAIImagesRetryableUpstreamError reports whether an Images error is an
// upstream server failure that may be retried on another account.
func IsOpenAIImagesRetryableUpstreamError(err *OpenAIImagesUpstreamError) bool {
	return err != nil && err.StatusCode >= http.StatusInternalServerError
REDACTED

func openAIImagesSSEErrorStatus(errType, code string) int {
	errType = strings.ToLower(strings.TrimSpace(errType))
	code = strings.ToLower(strings.TrimSpace(code))

	switch {
	case strings.Contains(errType, "rate_limit"), strings.Contains(code, "rate_limit"):
		return http.StatusTooManyRequests
	case strings.Contains(errType, "authentication"), strings.Contains(code, "invalid_api_key"), code == "unauthorized":
		return http.StatusUnauthorized
	case strings.Contains(errType, "permission"), code == "forbidden":
		return http.StatusForbidden
	case strings.Contains(errType, "not_found"), strings.Contains(code, "not_found"):
		return http.StatusNotFound
	case strings.Contains(errType, "invalid_request"),
		errType == "image_generation_user_error",
		code == "moderation_blocked",
		strings.Contains(code, "content_policy"),
		strings.Contains(code, "policy_violation"),
		strings.Contains(code, "safety_violation"):
		return http.StatusBadRequest
	default:
		return http.StatusBadGateway
REDACTED
REDACTED

func openAIImagesUpstreamErrorResponseBody(err *OpenAIImagesUpstreamError) []byte {
	if err == nil {
		return nil
REDACTED
	body := []byte(`{"error":{"type":"","message":""REDACTEDREDACTED`)
	body, _ = sjson.SetBytes(body, "error.type", err.clientErrorType())
	body, _ = sjson.SetBytes(body, "error.message", err.clientMessage())
	if code := strings.TrimSpace(err.Code); code != "" {
		body, _ = sjson.SetBytes(body, "error.code", code)
REDACTED
	if param := strings.TrimSpace(err.Param); param != "" {
		body, _ = sjson.SetBytes(body, "error.param", param)
REDACTED
	return body
REDACTED

func openAIResponsesImageResultKey(itemID string, result openAIResponsesImageResult) string {
	if strings.TrimSpace(result.Result) != "" {
		return strings.TrimSpace(result.OutputFormat) + "|" + strings.TrimSpace(result.Result)
REDACTED
	return "item:" + strings.TrimSpace(itemID)
REDACTED

func appendOpenAIResponsesImageResultDedup(results *[]openAIResponsesImageResult, seen map[string]struct{REDACTED, itemID string, result openAIResponsesImageResult) bool {
	if results == nil {
		return false
REDACTED
	key := openAIResponsesImageResultKey(itemID, result)
	if key != "" {
		if _, exists := seen[key]; exists {
			return false
	REDACTED
		seen[key] = struct{REDACTED{REDACTED
REDACTED
	*results = append(*results, result)
	return true
REDACTED

func mergeOpenAIResponsesImageMeta(dst *openAIResponsesImageResult, src openAIResponsesImageResult) {
	if dst == nil {
		return
REDACTED
	if trimmed := strings.TrimSpace(src.OutputFormat); trimmed != "" {
		dst.OutputFormat = trimmed
REDACTED
	if trimmed := strings.TrimSpace(src.Size); trimmed != "" {
		dst.Size = trimmed
REDACTED
	if trimmed := strings.TrimSpace(src.Background); trimmed != "" {
		dst.Background = trimmed
REDACTED
	if trimmed := strings.TrimSpace(src.Quality); trimmed != "" {
		dst.Quality = trimmed
REDACTED
	if trimmed := strings.TrimSpace(src.Model); trimmed != "" {
		dst.Model = trimmed
REDACTED
REDACTED

func openAIResponsesImageResultSizes(results []openAIResponsesImageResult) []string {
	if len(results) == 0 {
		return nil
REDACTED
	sizes := make([]string, 0, len(results))
	for _, result := range results {
		if size := strings.TrimSpace(result.Size); size != "" {
			sizes = append(sizes, size)
	REDACTED
REDACTED
	if len(sizes) == 0 {
		return nil
REDACTED
	return sizes
REDACTED

func extractOpenAIResponsesImageMetaFromLifecycleEvent(payload []byte) (openAIResponsesImageResult, int64, bool) {
	switch gjson.GetBytes(payload, "type").String() {
	case "response.created", "response.in_progress", "response.completed":
	default:
		return openAIResponsesImageResult{REDACTED, 0, false
REDACTED

	response := gjson.GetBytes(payload, "response")
	if !response.Exists() {
		return openAIResponsesImageResult{REDACTED, 0, false
REDACTED

	meta := openAIResponsesImageResult{
		OutputFormat: strings.TrimSpace(response.Get("tools.0.output_format").String()),
		Size:         strings.TrimSpace(response.Get("tools.0.size").String()),
		Background:   strings.TrimSpace(response.Get("tools.0.background").String()),
		Quality:      strings.TrimSpace(response.Get("tools.0.quality").String()),
		Model:        strings.TrimSpace(response.Get("tools.0.model").String()),
REDACTED
	return meta, response.Get("created_at").Int(), true
REDACTED

func buildOpenAIImagesStreamPartialPayload(
	eventType string,
	b64 string,
	partialImageIndex int64,
	responseFormat string,
	createdAt int64,
	meta openAIResponsesImageResult,
) []byte {
	if createdAt <= 0 {
		createdAt = time.Now().Unix()
REDACTED

	payload := []byte(`{"type":"","created_at":0,"partial_image_index":0,"b64_json":""REDACTED`)
	payload, _ = sjson.SetBytes(payload, "type", eventType)
	payload, _ = sjson.SetBytes(payload, "created_at", createdAt)
	payload, _ = sjson.SetBytes(payload, "partial_image_index", partialImageIndex)
	payload, _ = sjson.SetBytes(payload, "b64_json", b64)
	if strings.EqualFold(strings.TrimSpace(responseFormat), "url") {
		payload, _ = sjson.SetBytes(payload, "url", "data:"+openAIImageOutputMIMEType(meta.OutputFormat)+";base64,"+b64)
REDACTED
	if meta.Background != "" {
		payload, _ = sjson.SetBytes(payload, "background", meta.Background)
REDACTED
	if meta.OutputFormat != "" {
		payload, _ = sjson.SetBytes(payload, "output_format", meta.OutputFormat)
REDACTED
	if meta.Quality != "" {
		payload, _ = sjson.SetBytes(payload, "quality", meta.Quality)
REDACTED
	if meta.Size != "" {
		payload, _ = sjson.SetBytes(payload, "size", meta.Size)
REDACTED
	if meta.Model != "" {
		payload, _ = sjson.SetBytes(payload, "model", meta.Model)
REDACTED
	return payload
REDACTED

func buildOpenAIImagesStreamCompletedPayload(
	eventType string,
	img openAIResponsesImageResult,
	responseFormat string,
	createdAt int64,
	usageRaw []byte,
) []byte {
	if createdAt <= 0 {
		createdAt = time.Now().Unix()
REDACTED

	payload := []byte(`{"type":"","created_at":0,"b64_json":""REDACTED`)
	payload, _ = sjson.SetBytes(payload, "type", eventType)
	payload, _ = sjson.SetBytes(payload, "created_at", createdAt)
	payload, _ = sjson.SetBytes(payload, "b64_json", img.Result)
	if strings.EqualFold(strings.TrimSpace(responseFormat), "url") {
		payload, _ = sjson.SetBytes(payload, "url", "data:"+openAIImageOutputMIMEType(img.OutputFormat)+";base64,"+img.Result)
REDACTED
	if img.Background != "" {
		payload, _ = sjson.SetBytes(payload, "background", img.Background)
REDACTED
	if img.OutputFormat != "" {
		payload, _ = sjson.SetBytes(payload, "output_format", img.OutputFormat)
REDACTED
	if img.Quality != "" {
		payload, _ = sjson.SetBytes(payload, "quality", img.Quality)
REDACTED
	if img.Size != "" {
		payload, _ = sjson.SetBytes(payload, "size", img.Size)
REDACTED
	if img.Model != "" {
		payload, _ = sjson.SetBytes(payload, "model", img.Model)
REDACTED
	if len(usageRaw) > 0 && gjson.ValidBytes(usageRaw) {
		payload, _ = sjson.SetRawBytes(payload, "usage", usageRaw)
REDACTED
	return payload
REDACTED

func openAIImageOutputMIMEType(outputFormat string) string {
	if outputFormat == "" {
		return "image/png"
REDACTED
	if strings.Contains(outputFormat, "/") {
		return outputFormat
REDACTED
	switch strings.ToLower(strings.TrimSpace(outputFormat)) {
	case "png":
		return "image/png"
	case "jpg", "jpeg":
		return "image/jpeg"
	case "webp":
		return "image/webp"
	default:
		return "image/png"
REDACTED
REDACTED

func openAIImageUploadToDataURL(upload OpenAIImagesUpload) (string, error) {
	if len(upload.Data) == 0 {
		return "", fmt.Errorf("upload %q is empty", strings.TrimSpace(upload.FileName))
REDACTED
	contentType := strings.TrimSpace(upload.ContentType)
	if contentType == "" {
		contentType = http.DetectContentType(upload.Data)
REDACTED
	return "data:" + contentType + ";base64," + base64.StdEncoding.EncodeToString(upload.Data), nil
REDACTED

func buildOpenAIImagesResponsesRequest(parsed *OpenAIImagesRequest, toolModel string) ([]byte, error) {
	if parsed == nil {
		return nil, fmt.Errorf("parsed images request is required")
REDACTED
	prompt := strings.TrimSpace(parsed.Prompt)
	if prompt == "" {
		return nil, fmt.Errorf("prompt is required")
REDACTED

	inputImages := make([]string, 0, len(parsed.InputImageURLs)+len(parsed.Uploads))
	for _, imageURL := range parsed.InputImageURLs {
		if trimmed := strings.TrimSpace(imageURL); trimmed != "" {
			inputImages = append(inputImages, trimmed)
	REDACTED
REDACTED
	for _, upload := range parsed.Uploads {
		dataURL, err := openAIImageUploadToDataURL(upload)
		if err != nil {
			return nil, err
	REDACTED
		inputImages = append(inputImages, dataURL)
REDACTED
	if parsed.IsEdits() && len(inputImages) == 0 {
		return nil, fmt.Errorf("image input is required")
REDACTED

	req := []byte(`{"instructions":"","stream":true,"reasoning":{"effort":"medium","summary":"auto"REDACTED,"parallel_tool_calls":true,"include":["reasoning.encrypted_content"],"model":"","store":false,"tool_choice":{"type":"image_generation"REDACTEDREDACTED`)
	req, _ = sjson.SetBytes(req, "model", openAIImagesResponsesMainModel)

	input := []byte(`[{"type":"message","role":"user","content":[{"type":"input_text","text":""REDACTED]REDACTED]`)
	input, _ = sjson.SetBytes(input, "0.content.0.text", prompt)
	for index, imageURL := range inputImages {
		part := []byte(`{"type":"input_image","image_url":""REDACTED`)
		part, _ = sjson.SetBytes(part, "image_url", imageURL)
		input, _ = sjson.SetRawBytes(input, fmt.Sprintf("0.content.%d", index+1), part)
REDACTED
	req, _ = sjson.SetRawBytes(req, "input", input)

	action := "generate"
	if parsed.IsEdits() {
		action = "edit"
REDACTED
	tool := []byte(`{"type":"image_generation","action":"","model":""REDACTED`)
	tool, _ = sjson.SetBytes(tool, "action", action)
	tool, _ = sjson.SetBytes(tool, "model", strings.TrimSpace(toolModel))
	if shouldPassOpenAIImagesN(toolModel, parsed.N) {
		tool, _ = sjson.SetBytes(tool, "n", parsed.N)
REDACTED

	for _, field := range []struct {
		path  string
		value string
REDACTED{
		{path: "size", value: parsed.SizeREDACTED,
		{path: "quality", value: parsed.QualityREDACTED,
		{path: "background", value: parsed.BackgroundREDACTED,
		{path: "output_format", value: parsed.OutputFormatREDACTED,
		{path: "moderation", value: parsed.ModerationREDACTED,
		{path: "style", value: parsed.StyleREDACTED,
REDACTED {
		if trimmed := strings.TrimSpace(field.value); trimmed != "" {
			tool, _ = sjson.SetBytes(tool, field.path, trimmed)
	REDACTED
REDACTED
	if parsed.OutputCompression != nil {
		tool, _ = sjson.SetBytes(tool, "output_compression", *parsed.OutputCompression)
REDACTED
	if parsed.PartialImages != nil {
		tool, _ = sjson.SetBytes(tool, "partial_images", *parsed.PartialImages)
REDACTED

	maskImageURL := strings.TrimSpace(parsed.MaskImageURL)
	if parsed.MaskUpload != nil {
		dataURL, err := openAIImageUploadToDataURL(*parsed.MaskUpload)
		if err != nil {
			return nil, err
	REDACTED
		maskImageURL = dataURL
REDACTED
	if maskImageURL != "" {
		tool, _ = sjson.SetBytes(tool, "input_image_mask.image_url", maskImageURL)
REDACTED

	req, _ = sjson.SetRawBytes(req, "tools", []byte(`[]`))
	req, _ = sjson.SetRawBytes(req, "tools.-1", tool)
	return req, nil
REDACTED

func shouldPassOpenAIImagesN(model string, n int) bool {
	if n <= 1 {
		return false
REDACTED
	return !strings.EqualFold(strings.TrimSpace(model), "dall-e-3")
REDACTED

func extractOpenAIImagesFromResponsesCompleted(payload []byte) ([]openAIResponsesImageResult, int64, []byte, openAIResponsesImageResult, error) {
	if gjson.GetBytes(payload, "type").String() != "response.completed" {
		return nil, 0, nil, openAIResponsesImageResult{REDACTED, fmt.Errorf("unexpected event type")
REDACTED

	createdAt := gjson.GetBytes(payload, "response.created_at").Int()
	if createdAt <= 0 {
		createdAt = time.Now().Unix()
REDACTED

	var (
		results   []openAIResponsesImageResult
		firstMeta openAIResponsesImageResult
	)
	output := gjson.GetBytes(payload, "response.output")
	if output.IsArray() {
		for _, item := range output.Array() {
			if item.Get("type").String() != "image_generation_call" {
				continue
		REDACTED
			result := strings.TrimSpace(item.Get("result").String())
			if result == "" {
				continue
		REDACTED
			entry := openAIResponsesImageResult{
				Result:        result,
				RevisedPrompt: strings.TrimSpace(item.Get("revised_prompt").String()),
				OutputFormat:  strings.TrimSpace(item.Get("output_format").String()),
				Size:          strings.TrimSpace(item.Get("size").String()),
				Background:    strings.TrimSpace(item.Get("background").String()),
				Quality:       strings.TrimSpace(item.Get("quality").String()),
		REDACTED
			if len(results) == 0 {
				firstMeta = entry
		REDACTED
			results = append(results, entry)
	REDACTED
REDACTED

	var usageRaw []byte
	if usage := gjson.GetBytes(payload, "response.tool_usage.image_gen"); usage.Exists() && usage.IsObject() {
		usageRaw = []byte(usage.Raw)
REDACTED
	return results, createdAt, usageRaw, firstMeta, nil
REDACTED

func extractOpenAIImageFromResponsesOutputItemDone(payload []byte) (openAIResponsesImageResult, string, bool, error) {
	if gjson.GetBytes(payload, "type").String() != "response.output_item.done" {
		return openAIResponsesImageResult{REDACTED, "", false, fmt.Errorf("unexpected event type")
REDACTED

	item := gjson.GetBytes(payload, "item")
	if !item.Exists() || item.Get("type").String() != "image_generation_call" {
		return openAIResponsesImageResult{REDACTED, "", false, nil
REDACTED

	result := strings.TrimSpace(item.Get("result").String())
	if result == "" {
		return openAIResponsesImageResult{REDACTED, "", false, nil
REDACTED

	entry := openAIResponsesImageResult{
		Result:        result,
		RevisedPrompt: strings.TrimSpace(item.Get("revised_prompt").String()),
		OutputFormat:  strings.TrimSpace(item.Get("output_format").String()),
		Size:          strings.TrimSpace(item.Get("size").String()),
		Background:    strings.TrimSpace(item.Get("background").String()),
		Quality:       strings.TrimSpace(item.Get("quality").String()),
REDACTED
	return entry, strings.TrimSpace(item.Get("id").String()), true, nil
REDACTED

func collectOpenAIImagesFromResponsesBody(body []byte) ([]openAIResponsesImageResult, int64, []byte, openAIResponsesImageResult, bool, error) {
	var (
		fallbackResults []openAIResponsesImageResult
		fallbackSeen    = make(map[string]struct{REDACTED)
		finalResults    []openAIResponsesImageResult
		finalMeta       openAIResponsesImageResult
		collectErr      error
		createdAt       int64
		usageRaw        []byte
		foundFinal      bool
		responseMeta    openAIResponsesImageResult
	)

	forEachOpenAISSEDataPayload(string(body), func(payload []byte) {
		if collectErr != nil || len(finalResults) > 0 {
			return
	REDACTED
		if !gjson.ValidBytes(payload) {
			return
	REDACTED
		if meta, eventCreatedAt, ok := extractOpenAIResponsesImageMetaFromLifecycleEvent(payload); ok {
			mergeOpenAIResponsesImageMeta(&responseMeta, meta)
			if eventCreatedAt > 0 {
				createdAt = eventCreatedAt
		REDACTED
	REDACTED

		switch gjson.GetBytes(payload, "type").String() {
		case "response.output_item.done":
			result, itemID, ok, err := extractOpenAIImageFromResponsesOutputItemDone(payload)
			if err != nil {
				collectErr = err
				return
		REDACTED
			if ok {
				mergeOpenAIResponsesImageMeta(&result, responseMeta)
				appendOpenAIResponsesImageResultDedup(&fallbackResults, fallbackSeen, itemID, result)
		REDACTED
		case "response.completed":
			results, completedAt, completedUsageRaw, firstMeta, err := extractOpenAIImagesFromResponsesCompleted(payload)
			if err != nil {
				collectErr = err
				return
		REDACTED
			foundFinal = true
			if completedAt > 0 {
				createdAt = completedAt
		REDACTED
			if len(completedUsageRaw) > 0 {
				usageRaw = completedUsageRaw
		REDACTED
			if len(results) > 0 {
				mergeOpenAIResponsesImageMeta(&firstMeta, responseMeta)
				finalResults = results
				finalMeta = firstMeta
				return
		REDACTED
			if len(fallbackResults) > 0 {
				firstMeta = fallbackResults[0]
				mergeOpenAIResponsesImageMeta(&firstMeta, responseMeta)
				finalResults = fallbackResults
				finalMeta = firstMeta
				return
		REDACTED
	REDACTED
REDACTED)
	if collectErr != nil {
		return nil, 0, nil, openAIResponsesImageResult{REDACTED, false, collectErr
REDACTED
	if len(finalResults) > 0 {
		return finalResults, createdAt, usageRaw, finalMeta, true, nil
REDACTED

	if len(fallbackResults) > 0 {
		firstMeta := fallbackResults[0]
		mergeOpenAIResponsesImageMeta(&firstMeta, responseMeta)
		return fallbackResults, createdAt, usageRaw, firstMeta, foundFinal, nil
REDACTED
	return nil, createdAt, usageRaw, openAIResponsesImageResult{REDACTED, foundFinal, nil
REDACTED

func extractOpenAIImagesUpstreamError(body []byte) *OpenAIImagesUpstreamError {
	var upstreamErr *OpenAIImagesUpstreamError
	forEachOpenAISSEDataPayload(string(body), func(payload []byte) {
		if upstreamErr != nil || !gjson.ValidBytes(payload) {
			return
	REDACTED
		upstreamErr = openAIImagesUpstreamErrorFromSSEPayload(payload)
REDACTED)
	return upstreamErr
REDACTED

func openAIImagesUpstreamErrorFromSSEPayload(payload []byte) *OpenAIImagesUpstreamError {
	if !gjson.ValidBytes(payload) {
		return nil
REDACTED
	switch gjson.GetBytes(payload, "type").String() {
	case "error":
		return openAIImagesUpstreamErrorFromGJSON(gjson.GetBytes(payload, "error"), "")
	case "response.failed":
		response := gjson.GetBytes(payload, "response")
		return openAIImagesUpstreamErrorFromGJSON(response.Get("error"), response.Get("id").String())
	case "response.incomplete":
		// 上游在生成预算内未产出图片（超时/被截断），返回 response.incomplete 而非 error。
		// 旧逻辑识别不到，统一报成模糊的 "upstream did not return image output" + 502，
		// 且不触发 failover。这里把它显式建模为可重试的上游错误，使其能换账号重试。
		return openAIImagesIncompleteUpstreamError(gjson.GetBytes(payload, "response"))
	default:
		return nil
REDACTED
REDACTED

// summarizeOpenAIImagesNoOutputBody 从上游 SSE 响应体提取诊断摘要，用于软失败时
// 记录到 ops 日志（上游无图、无标准错误的场景）。提取最终事件类型、response.status、
// incomplete_details.reason，并附 body 截断片段，便于事后定位上游到底返回了什么。
func summarizeOpenAIImagesNoOutputBody(body []byte) string {
	var lastType, status, incompleteReason string
	forEachOpenAISSEDataPayload(string(body), func(payload []byte) {
		if !gjson.ValidBytes(payload) {
			return
	REDACTED
		if t := strings.TrimSpace(gjson.GetBytes(payload, "type").String()); t != "" {
			lastType = t
	REDACTED
		if resp := gjson.GetBytes(payload, "response"); resp.Exists() {
			if s := strings.TrimSpace(resp.Get("status").String()); s != "" {
				status = s
		REDACTED
			if r := strings.TrimSpace(resp.Get("incomplete_details.reason").String()); r != "" {
				incompleteReason = r
		REDACTED
	REDACTED
REDACTED)
	var b strings.Builder
	_, _ = b.WriteString("no_image_output")
	if lastType != "" {
		fmt.Fprintf(&b, " last_event=%s", lastType)
REDACTED
	if status != "" {
		fmt.Fprintf(&b, " status=%s", status)
REDACTED
	if incompleteReason != "" {
		fmt.Fprintf(&b, " incomplete_reason=%s", incompleteReason)
REDACTED
	// 附 body 截断片段（脱敏后），上限 1KB，避免日志膨胀。
	snippet := strings.TrimSpace(string(body))
	const maxSnippet = 1024
	if len(snippet) > maxSnippet {
		snippet = snippet[:maxSnippet] + "...(truncated)"
REDACTED
	if snippet != "" {
		fmt.Fprintf(&b, " body=%s", snippet)
REDACTED
	return b.String()
REDACTED

// openAIImagesIncompleteUpstreamError 从 response.incomplete 事件构建可重试的上游错误。
// incomplete_details.reason 常见取值：max_output_tokens / content_filter 等。
// content_filter 视为客户端错误（400，重试无意义）；其余（生成超时/截断）视为
// 可重试的 502，触发 failover 换账号重试。
func openAIImagesIncompleteUpstreamError(response gjson.Result) *OpenAIImagesUpstreamError {
	if !response.Exists() {
		return nil
REDACTED
	reason := strings.TrimSpace(response.Get("incomplete_details.reason").String())
	statusCode := http.StatusBadGateway // 默认可重试（生成未完成）
	errType := "incomplete_error"
	if strings.Contains(strings.ToLower(reason), "content_filter") ||
		strings.Contains(strings.ToLower(reason), "moderation") {
		statusCode = http.StatusBadRequest // 内容过滤，重试无意义
		errType = "image_generation_user_error"
REDACTED
	message := "Upstream did not complete image generation"
	if reason != "" {
		message = fmt.Sprintf("Upstream image generation incomplete: %s", reason)
REDACTED
	return &OpenAIImagesUpstreamError{
		StatusCode:        statusCode,
		ErrorType:         errType,
		Code:              "response_incomplete",
		Message:           sanitizeUpstreamErrorMessage(message),
		UpstreamRequestID: strings.TrimSpace(response.Get("id").String()),
REDACTED
REDACTED

func openAIImagesUpstreamErrorFromGJSON(errorObj gjson.Result, upstreamRequestID string) *OpenAIImagesUpstreamError {
	if !errorObj.Exists() {
		return nil
REDACTED
	code := strings.TrimSpace(errorObj.Get("code").String())
	errType := strings.TrimSpace(errorObj.Get("type").String())
	message := strings.TrimSpace(errorObj.Get("message").String())
	param := strings.TrimSpace(errorObj.Get("param").String())
	statusCode := openAIImagesSSEErrorStatus(errType, code)
	if message == "" {
		message = "Upstream request failed"
REDACTED
	return &OpenAIImagesUpstreamError{
		StatusCode:        statusCode,
		ErrorType:         errType,
		Code:              code,
		Message:           sanitizeUpstreamErrorMessage(message),
		Param:             param,
		UpstreamRequestID: strings.TrimSpace(upstreamRequestID),
REDACTED
REDACTED

// openAIImagesErrorTypeForStatus returns an OpenAI-style error type when the
// upstream body does not provide one of its own.
func openAIImagesErrorTypeForStatus(status int) string {
	switch {
	case status == http.StatusBadRequest:
		return "invalid_request_error"
	case status == http.StatusUnauthorized:
		return "authentication_error"
	case status == http.StatusForbidden:
		return "permission_error"
	case status == http.StatusNotFound:
		return "not_found_error"
	case status == http.StatusTooManyRequests:
		return "rate_limit_error"
	case status >= 500:
		return "api_error"
	default:
		return "upstream_error"
REDACTED
REDACTED

// openAIImagesUpstreamErrorFromHTTP builds an OpenAIImagesUpstreamError from a
// non-2xx upstream HTTP response, preserving the real status code, type, code,
// message and param so the client sees the actual upstream error instead of a
// generic 502.
func openAIImagesUpstreamErrorFromHTTP(statusCode int, header http.Header, body []byte) *OpenAIImagesUpstreamError {
	errType := strings.TrimSpace(gjson.GetBytes(body, "error.type").String())
	code := strings.TrimSpace(extractUpstreamErrorCode(body))
	param := strings.TrimSpace(gjson.GetBytes(body, "error.param").String())
	message := sanitizeUpstreamErrorMessage(strings.TrimSpace(extractUpstreamErrorMessage(body)))
	if message == "" {
		message = fmt.Sprintf("Upstream request failed (status %d)", statusCode)
REDACTED
	if errType == "" {
		errType = openAIImagesErrorTypeForStatus(statusCode)
REDACTED
	requestID := ""
	if header != nil {
		requestID = strings.TrimSpace(header.Get("x-request-id"))
REDACTED
	return &OpenAIImagesUpstreamError{
		StatusCode:        statusCode,
		ErrorType:         errType,
		Code:              code,
		Message:           message,
		Param:             param,
		UpstreamRequestID: requestID,
REDACTED
REDACTED

// handleOpenAIImagesErrorResponse is the non-failover error handler for the
// images endpoints (/v1/images/generations and /v1/images/edits). Unlike the
// generic handleErrorResponse — which collapses every non-failover upstream
// error into a generic 502 "Upstream request failed" — it surfaces the real
// upstream status code and error message/type/code/param to the client. This
// mirrors how the Chat Completions and Messages compat paths use
// handleCompatErrorResponse.
//
// It returns an *OpenAIImagesUpstreamError (already written to the client) so
// the images handler treats it as a terminal user-facing error rather than
// re-writing a fallback response.
func (s *OpenAIGatewayService) handleOpenAIImagesErrorResponse(
	ctx context.Context,
	resp *http.Response,
	c *gin.Context,
	account *Account,
	requestedModel ...string,
) (*OpenAIForwardResult, error) {
	body := s.readUpstreamErrorBody(resp)

	upstreamMsg := sanitizeUpstreamErrorMessage(strings.TrimSpace(extractUpstreamErrorMessage(body)))
	upstreamDetail := ""
	if s.cfg != nil && s.cfg.Gateway.LogUpstreamErrorBody {
		maxBytes := s.cfg.Gateway.LogUpstreamErrorBodyMaxBytes
		if maxBytes <= 0 {
			maxBytes = 2048
	REDACTED
		upstreamDetail = truncateString(string(body), maxBytes)
REDACTED
	setOpsUpstreamError(c, resp.StatusCode, upstreamMsg, upstreamDetail)

	if s.cfg != nil && s.cfg.Gateway.LogUpstreamErrorBody {
		logger.LegacyPrintf("service.openai_gateway",
			"OpenAI images upstream error %d (account=%d platform=%s type=%s): %s",
			resp.StatusCode,
			account.ID,
			account.Platform,
			account.Type,
			truncateForLog(body, s.cfg.Gateway.LogUpstreamErrorBodyMaxBytes),
		)
REDACTED

	// Honor admin-configured error passthrough rules first.
	if status, errType, errMsg, matched := applyErrorPassthroughRule(
		c,
		account.Platform,
		resp.StatusCode,
		body,
		http.StatusBadGateway,
		"upstream_error",
		"Upstream request failed",
	); matched {
		upErr := &OpenAIImagesUpstreamError{
			StatusCode:        status,
			ErrorType:         errType,
			Message:           errMsg,
			UpstreamRequestID: strings.TrimSpace(resp.Header.Get("x-request-id")),
	REDACTED
		writeOpenAIImagesUpstreamErrorResponse(c, upErr)
		return nil, upErr
REDACTED

	// If the account is not configured to handle this status code, fall back to
	// a generic gateway error without exposing upstream internals (mirrors
	// handleCompatErrorResponse).
	if !account.ShouldHandleErrorCode(resp.StatusCode) {
		appendOpsUpstreamError(c, OpsUpstreamErrorEvent{
			Platform:           account.Platform,
			AccountID:          account.ID,
			AccountName:        account.Name,
			UpstreamStatusCode: resp.StatusCode,
			UpstreamRequestID:  resp.Header.Get("x-request-id"),
			Kind:               "http_error",
			Message:            upstreamMsg,
			Detail:             upstreamDetail,
	REDACTED)
		upErr := &OpenAIImagesUpstreamError{
			StatusCode:        http.StatusInternalServerError,
			ErrorType:         "upstream_error",
			Message:           "Upstream gateway error",
			UpstreamRequestID: strings.TrimSpace(resp.Header.Get("x-request-id")),
	REDACTED
		writeOpenAIImagesUpstreamErrorResponse(c, upErr)
		return nil, upErr
REDACTED

	// Track rate limits / decide whether to disable the account (secondary failover).
	var modelForCooldown string
	if len(requestedModel) > 0 {
		modelForCooldown = strings.TrimSpace(requestedModel[0])
REDACTED
	shouldDisable := s.handleOpenAIAccountUpstreamError(ctx, account, resp.StatusCode, resp.Header, body, modelForCooldown)
	kind := "http_error"
	if shouldDisable {
		kind = "failover"
REDACTED
	appendOpsUpstreamError(c, OpsUpstreamErrorEvent{
		Platform:           account.Platform,
		AccountID:          account.ID,
		AccountName:        account.Name,
		UpstreamStatusCode: resp.StatusCode,
		UpstreamRequestID:  resp.Header.Get("x-request-id"),
		Kind:               kind,
		Message:            upstreamMsg,
		Detail:             upstreamDetail,
REDACTED)
	if shouldDisable {
		return nil, &UpstreamFailoverError{
			StatusCode:             resp.StatusCode,
			ResponseBody:           body,
			RetryableOnSameAccount: account.IsPoolMode() && account.IsPoolModeRetryableStatus(resp.StatusCode),
	REDACTED
REDACTED

	// Surface the real upstream error to the client.
	upErr := openAIImagesUpstreamErrorFromHTTP(resp.StatusCode, resp.Header, body)
	writeOpenAIImagesUpstreamErrorResponse(c, upErr)
	return nil, upErr
REDACTED

func buildOpenAIImagesAPIResponse(
	results []openAIResponsesImageResult,
	createdAt int64,
	usageRaw []byte,
	firstMeta openAIResponsesImageResult,
	responseFormat string,
) ([]byte, error) {
	if createdAt <= 0 {
		createdAt = time.Now().Unix()
REDACTED
	out := []byte(`{"created":0,"data":[]REDACTED`)
	out, _ = sjson.SetBytes(out, "created", createdAt)

	format := strings.ToLower(strings.TrimSpace(responseFormat))
	if format == "" {
		format = "b64_json"
REDACTED
	for _, img := range results {
		item := []byte(`{REDACTED`)
		if format == "url" {
			item, _ = sjson.SetBytes(item, "url", "data:"+openAIImageOutputMIMEType(img.OutputFormat)+";base64,"+img.Result)
	REDACTED else {
			item, _ = sjson.SetBytes(item, "b64_json", img.Result)
	REDACTED
		if img.RevisedPrompt != "" {
			item, _ = sjson.SetBytes(item, "revised_prompt", img.RevisedPrompt)
	REDACTED
		out, _ = sjson.SetRawBytes(out, "data.-1", item)
REDACTED
	if firstMeta.Background != "" {
		out, _ = sjson.SetBytes(out, "background", firstMeta.Background)
REDACTED
	if firstMeta.OutputFormat != "" {
		out, _ = sjson.SetBytes(out, "output_format", firstMeta.OutputFormat)
REDACTED
	if firstMeta.Quality != "" {
		out, _ = sjson.SetBytes(out, "quality", firstMeta.Quality)
REDACTED
	if firstMeta.Size != "" {
		out, _ = sjson.SetBytes(out, "size", firstMeta.Size)
REDACTED
	if firstMeta.Model != "" {
		out, _ = sjson.SetBytes(out, "model", firstMeta.Model)
REDACTED
	if len(usageRaw) > 0 && gjson.ValidBytes(usageRaw) {
		out, _ = sjson.SetRawBytes(out, "usage", usageRaw)
REDACTED
	return out, nil
REDACTED

func openAIImagesStreamPrefix(parsed *OpenAIImagesRequest) string {
	if parsed != nil && parsed.IsEdits() {
		return "image_edit"
REDACTED
	return "image_generation"
REDACTED

func buildOpenAIImagesStreamErrorBody(message string) []byte {
	body := []byte(`{"type":"error","error":{"type":"upstream_error","message":""REDACTEDREDACTED`)
	if strings.TrimSpace(message) == "" {
		message = "upstream request failed"
REDACTED
	body, _ = sjson.SetBytes(body, "error.message", message)
	return body
REDACTED

func buildOpenAIImagesStreamErrorBodyFromUpstream(err *OpenAIImagesUpstreamError) []byte {
	if err == nil {
		return buildOpenAIImagesStreamErrorBody("")
REDACTED
	body := buildOpenAIImagesStreamErrorBody(err.clientMessage())
	body, _ = sjson.SetBytes(body, "error.type", err.clientErrorType())
	if code := strings.TrimSpace(err.Code); code != "" {
		body, _ = sjson.SetBytes(body, "error.code", code)
REDACTED
	if param := strings.TrimSpace(err.Param); param != "" {
		body, _ = sjson.SetBytes(body, "error.param", param)
REDACTED
	return body
REDACTED

func writeOpenAIImagesUpstreamErrorResponse(c *gin.Context, err *OpenAIImagesUpstreamError) bool {
	if c == nil || c.Writer == nil || c.Writer.Written() || err == nil {
		return false
REDACTED
	errorObj := gin.H{
		"type":    err.clientErrorType(),
		"message": err.clientMessage(),
REDACTED
	if code := strings.TrimSpace(err.Code); code != "" {
		errorObj["code"] = code
REDACTED
	if param := strings.TrimSpace(err.Param); param != "" {
		errorObj["param"] = param
REDACTED
	c.JSON(err.clientStatusCode(), gin.H{
		"error": errorObj,
REDACTED)
	return true
REDACTED

func (s *OpenAIGatewayService) writeOpenAIImagesStreamEvent(c *gin.Context, flusher http.Flusher, eventName string, payload []byte) error {
	if strings.TrimSpace(eventName) != "" {
		if _, err := fmt.Fprintf(c.Writer, "event: %s\n", eventName); err != nil {
			return err
	REDACTED
REDACTED
	if _, err := fmt.Fprintf(c.Writer, "data: %s\n\n", payload); err != nil {
		return err
REDACTED
	flusher.Flush()
	return nil
REDACTED

func (s *OpenAIGatewayService) tryWriteOpenAIImagesStreamEvent(
	c *gin.Context,
	flusher http.Flusher,
	clientDisconnected *bool,
	lastWriteAt *time.Time,
	eventName string,
	payload []byte,
) bool {
	if clientDisconnected != nil && *clientDisconnected {
		return false
REDACTED
	if err := s.writeOpenAIImagesStreamEvent(c, flusher, eventName, payload); err != nil {
		if clientDisconnected != nil {
			*clientDisconnected = true
	REDACTED
		logger.LegacyPrintf("service.openai_gateway", "[OpenAI] Images stream client disconnected, continue draining upstream for billing")
		return false
REDACTED
	if lastWriteAt != nil {
		*lastWriteAt = time.Now()
REDACTED
	return true
REDACTED

func (s *OpenAIGatewayService) handleOpenAIImagesOAuthNonStreamingResponse(
	resp *http.Response,
	c *gin.Context,
	responseFormat string,
	fallbackModel string,
) (OpenAIUsage, int, []string, error) {
	body, err := ReadUpstreamResponseBody(resp.Body, s.cfg, c, openAITooLargeError)
	if err != nil {
		return OpenAIUsage{REDACTED, 0, nil, err
REDACTED

	var usage OpenAIUsage
	forEachOpenAISSEDataPayload(string(body), func(data []byte) {
		s.parseSSEUsageBytes(data, &usage)
REDACTED)
	results, createdAt, usageRaw, firstMeta, _, err := collectOpenAIImagesFromResponsesBody(body)
	if err != nil {
		return OpenAIUsage{REDACTED, 0, nil, err
REDACTED
	if len(results) == 0 {
		if upstreamErr := extractOpenAIImagesUpstreamError(body); upstreamErr != nil {
			setOpsUpstreamError(c, upstreamErr.clientStatusCode(), upstreamErr.clientMessage(), "")
			if !IsOpenAIImagesRetryableUpstreamError(upstreamErr) {
				writeOpenAIImagesUpstreamErrorResponse(c, upstreamErr)
		REDACTED
			return OpenAIUsage{REDACTED, 0, nil, upstreamErr
	REDACTED
		// 软失败兜底：上游既无图、又无任何可识别的 error/failed/incomplete 事件
		// （实测：上游偶发把请求路由到 gpt-5.x-mini，返回 response.completed 但 output 为空、
		// image_gen 工具未执行）。这是上游的概率性失败——同账号有时成功有时失败。
		// 处理：① 记录上游诊断摘要到 ops（last_event/status/model/body 片段）便于排查；
		// ② 返回 UpstreamFailoverError 触发重试。因实测为「同账号概率性失败」，优先
		//    RetryableOnSameAccount 同账号快速重试（默认 3 次，大概率某次正常出图），
		//    用尽后由 handler 自然换账号 failover（switchCount 上限保护），既提高成功率
		//    又不无谓消耗其他账号配额。
		setOpsUpstreamError(c, http.StatusBadGateway, "upstream did not return image output", summarizeOpenAIImagesNoOutputBody(body))
		return OpenAIUsage{REDACTED, 0, nil, &UpstreamFailoverError{
			StatusCode:             http.StatusBadGateway,
			ResponseBody:           body,
			RetryableOnSameAccount: true,
	REDACTED
REDACTED
	if strings.TrimSpace(firstMeta.Model) == "" {
		firstMeta.Model = strings.TrimSpace(fallbackModel)
REDACTED

	responseBody, err := buildOpenAIImagesAPIResponse(results, createdAt, usageRaw, firstMeta, responseFormat)
	if err != nil {
		return OpenAIUsage{REDACTED, 0, nil, err
REDACTED
	responseheaders.WriteFilteredHeaders(c.Writer.Header(), resp.Header, s.responseHeaderFilter)
	c.Data(resp.StatusCode, "application/json; charset=utf-8", responseBody)
	return usage, len(results), openAIResponsesImageResultSizes(results), nil
REDACTED

func (s *OpenAIGatewayService) handleOpenAIImagesOAuthStreamingResponse(
	resp *http.Response,
	c *gin.Context,
	startTime time.Time,
	responseFormat string,
	streamPrefix string,
	fallbackModel string,
) (OpenAIUsage, int, []string, *int, error) {
	responseheaders.WriteFilteredHeaders(c.Writer.Header(), resp.Header, s.responseHeaderFilter)
	c.Header("Content-Type", "text/event-stream")
	c.Header("Cache-Control", "no-cache")
	c.Header("Connection", "keep-alive")
	c.Status(resp.StatusCode)

	flusher, ok := c.Writer.(http.Flusher)
	if !ok {
		return OpenAIUsage{REDACTED, 0, nil, nil, fmt.Errorf("streaming is not supported by response writer")
REDACTED

	format := strings.ToLower(strings.TrimSpace(responseFormat))
	if format == "" {
		format = "b64_json"
REDACTED

	usage := OpenAIUsage{REDACTED
	imageCount := 0
	var imageOutputSizes []string
	var firstTokenMs *int
	emitted := make(map[string]struct{REDACTED)
	pendingResults := make([]openAIResponsesImageResult, 0, 1)
	pendingSeen := make(map[string]struct{REDACTED)
	streamMeta := openAIResponsesImageResult{Model: strings.TrimSpace(fallbackModel)REDACTED
	var createdAt int64
	clientDisconnected := false
	lastDownstreamWriteAt := time.Now()
	var sseData openAISSEDataAccumulator
	var processDataErr error
	processDataDone := false
	writerSizeBeforeResponse := c.Writer.Size()

	processData := func(dataBytes []byte) {
		if processDataDone || processDataErr != nil {
			return
	REDACTED
		if firstTokenMs == nil {
			ms := int(time.Since(startTime).Milliseconds())
			firstTokenMs = &ms
	REDACTED
		s.parseSSEUsageBytes(dataBytes, &usage)
		if !gjson.ValidBytes(dataBytes) {
			return
	REDACTED
		if meta, eventCreatedAt, ok := extractOpenAIResponsesImageMetaFromLifecycleEvent(dataBytes); ok {
			mergeOpenAIResponsesImageMeta(&streamMeta, meta)
			if eventCreatedAt > 0 {
				createdAt = eventCreatedAt
		REDACTED
	REDACTED
		switch gjson.GetBytes(dataBytes, "type").String() {
		case "response.image_generation_call.partial_image":
			b64 := strings.TrimSpace(gjson.GetBytes(dataBytes, "partial_image_b64").String())
			if b64 == "" {
				return
		REDACTED
			eventName := streamPrefix + ".partial_image"
			partialMeta := streamMeta
			mergeOpenAIResponsesImageMeta(&partialMeta, openAIResponsesImageResult{
				OutputFormat: strings.TrimSpace(gjson.GetBytes(dataBytes, "output_format").String()),
				Background:   strings.TrimSpace(gjson.GetBytes(dataBytes, "background").String()),
		REDACTED)
			payload := buildOpenAIImagesStreamPartialPayload(
				eventName,
				b64,
				gjson.GetBytes(dataBytes, "partial_image_index").Int(),
				format,
				createdAt,
				partialMeta,
			)
			s.tryWriteOpenAIImagesStreamEvent(c, flusher, &clientDisconnected, &lastDownstreamWriteAt, eventName, payload)
		case "response.output_item.done":
			img, itemID, ok, extractErr := extractOpenAIImageFromResponsesOutputItemDone(dataBytes)
			if extractErr != nil {
				s.tryWriteOpenAIImagesStreamEvent(c, flusher, &clientDisconnected, &lastDownstreamWriteAt, "error", buildOpenAIImagesStreamErrorBody(extractErr.Error()))
				processDataErr = extractErr
				processDataDone = true
				return
		REDACTED
			if !ok {
				return
		REDACTED
			mergeOpenAIResponsesImageMeta(&streamMeta, img)
			mergeOpenAIResponsesImageMeta(&img, streamMeta)
			key := openAIResponsesImageResultKey(itemID, img)
			if _, exists := emitted[key]; exists {
				return
		REDACTED
			if _, exists := pendingSeen[key]; exists {
				return
		REDACTED
			pendingSeen[key] = struct{REDACTED{REDACTED
			pendingResults = append(pendingResults, img)
		case "response.completed":
			results, _, usageRaw, firstMeta, extractErr := extractOpenAIImagesFromResponsesCompleted(dataBytes)
			if extractErr != nil {
				s.tryWriteOpenAIImagesStreamEvent(c, flusher, &clientDisconnected, &lastDownstreamWriteAt, "error", buildOpenAIImagesStreamErrorBody(extractErr.Error()))
				processDataErr = extractErr
				processDataDone = true
				return
		REDACTED
			mergeOpenAIResponsesImageMeta(&streamMeta, firstMeta)
			finalResults := make([]openAIResponsesImageResult, 0, len(results)+len(pendingResults))
			finalSeen := make(map[string]struct{REDACTED)
			for _, img := range results {
				mergeOpenAIResponsesImageMeta(&img, streamMeta)
				appendOpenAIResponsesImageResultDedup(&finalResults, finalSeen, "", img)
		REDACTED
			for _, img := range pendingResults {
				mergeOpenAIResponsesImageMeta(&img, streamMeta)
				appendOpenAIResponsesImageResultDedup(&finalResults, finalSeen, "", img)
		REDACTED
			if len(finalResults) == 0 {
				outputErr := fmt.Errorf("upstream did not return image output")
				// 软失败：response.completed 事件里没有图片。记录上游诊断摘要到 ops，
				// 与非流式路径保持一致，避免上游响应信息丢失。
				setOpsUpstreamError(c, http.StatusBadGateway, "upstream did not return image output", summarizeOpenAIImagesNoOutputBody(dataBytes))
				s.tryWriteOpenAIImagesStreamEvent(c, flusher, &clientDisconnected, &lastDownstreamWriteAt, "error", buildOpenAIImagesStreamErrorBody(outputErr.Error()))
				processDataErr = outputErr
				processDataDone = true
				return
		REDACTED
			eventName := streamPrefix + ".completed"
			for _, img := range finalResults {
				key := openAIResponsesImageResultKey("", img)
				if _, exists := emitted[key]; exists {
					continue
			REDACTED
				payload := buildOpenAIImagesStreamCompletedPayload(eventName, img, format, createdAt, usageRaw)
				emitted[key] = struct{REDACTED{REDACTED
				s.tryWriteOpenAIImagesStreamEvent(c, flusher, &clientDisconnected, &lastDownstreamWriteAt, eventName, payload)
		REDACTED
			imageCount = len(emitted)
			imageOutputSizes = openAIResponsesImageResultSizes(finalResults)
			processDataDone = true
		case "error", "response.failed":
			if upstreamErr := openAIImagesUpstreamErrorFromSSEPayload(dataBytes); upstreamErr != nil {
				retryable := IsOpenAIImagesRetryableUpstreamError(upstreamErr)
				if !clientDisconnected && (!retryable || c.Writer.Size() != writerSizeBeforeResponse) {
					s.tryWriteOpenAIImagesStreamEvent(c, flusher, &clientDisconnected, &lastDownstreamWriteAt, "error", buildOpenAIImagesStreamErrorBodyFromUpstream(upstreamErr))
			REDACTED
				setOpsUpstreamError(c, upstreamErr.clientStatusCode(), upstreamErr.clientMessage(), "")
				processDataErr = upstreamErr
				processDataDone = true
				return
		REDACTED
	REDACTED
REDACTED

	processLine := func(line []byte) (bool, error) {
		if len(line) == 0 {
			return false, nil
	REDACTED
		sseData.AddLine(string(line), processData)
		if processDataErr != nil {
			return true, processDataErr
	REDACTED
		return processDataDone, nil
REDACTED

	flushData := func() (bool, error) {
		sseData.Flush(processData)
		if processDataErr != nil {
			return true, processDataErr
	REDACTED
		return processDataDone, nil
REDACTED

	finalizePending := func() error {
		if imageCount > 0 {
			return nil
	REDACTED
		if len(pendingResults) > 0 {
			eventName := streamPrefix + ".completed"
			for _, img := range pendingResults {
				mergeOpenAIResponsesImageMeta(&img, streamMeta)
				key := openAIResponsesImageResultKey("", img)
				if _, exists := emitted[key]; exists {
					continue
			REDACTED
				payload := buildOpenAIImagesStreamCompletedPayload(eventName, img, format, createdAt, nil)
				emitted[key] = struct{REDACTED{REDACTED
				s.tryWriteOpenAIImagesStreamEvent(c, flusher, &clientDisconnected, &lastDownstreamWriteAt, eventName, payload)
		REDACTED
			imageCount = len(emitted)
			imageOutputSizes = openAIResponsesImageResultSizes(pendingResults)
			return nil
	REDACTED

		streamErr := fmt.Errorf("stream disconnected before image generation completed")
		s.tryWriteOpenAIImagesStreamEvent(c, flusher, &clientDisconnected, &lastDownstreamWriteAt, "error", buildOpenAIImagesStreamErrorBody(streamErr.Error()))
		return streamErr
REDACTED

	streamInterval := s.openAIImageStreamDataInterval()
	keepaliveInterval := s.openAIImageStreamKeepaliveInterval()
	if streamInterval <= 0 && keepaliveInterval <= 0 {
		reader := bufio.NewReader(resp.Body)
		for {
			line, err := reader.ReadBytes('\n')
			done, processErr := processLine(line)
			if processErr != nil {
				return usage, imageCount, imageOutputSizes, firstTokenMs, processErr
		REDACTED
			if done {
				return usage, imageCount, imageOutputSizes, firstTokenMs, nil
		REDACTED
			if err == io.EOF {
				break
		REDACTED
			if err != nil {
				if done, processErr := flushData(); processErr != nil {
					return usage, imageCount, imageOutputSizes, firstTokenMs, processErr
			REDACTED else if done {
					return usage, imageCount, imageOutputSizes, firstTokenMs, nil
			REDACTED
				s.tryWriteOpenAIImagesStreamEvent(c, flusher, &clientDisconnected, &lastDownstreamWriteAt, "error", buildOpenAIImagesStreamErrorBody(err.Error()))
				return usage, imageCount, imageOutputSizes, firstTokenMs, err
		REDACTED
	REDACTED
		if done, processErr := flushData(); processErr != nil {
			return usage, imageCount, imageOutputSizes, firstTokenMs, processErr
	REDACTED else if done {
			return usage, imageCount, imageOutputSizes, firstTokenMs, nil
	REDACTED
		if err := finalizePending(); err != nil {
			return usage, imageCount, imageOutputSizes, firstTokenMs, err
	REDACTED
		return usage, imageCount, imageOutputSizes, firstTokenMs, nil
REDACTED

	type readEvent struct {
		line []byte
		err  error
REDACTED
	events := make(chan readEvent, 16)
	done := make(chan struct{REDACTED)
	sendEvent := func(ev readEvent) bool {
		select {
		case events <- ev:
			return true
		case <-done:
			return false
	REDACTED
REDACTED
	var lastReadAt int64
	atomic.StoreInt64(&lastReadAt, time.Now().UnixNano())
	go func() {
		defer close(events)
		reader := bufio.NewReader(resp.Body)
		for {
			line, err := reader.ReadBytes('\n')
			if len(line) > 0 {
				atomic.StoreInt64(&lastReadAt, time.Now().UnixNano())
		REDACTED
			if len(line) > 0 && !sendEvent(readEvent{line: lineREDACTED) {
				return
		REDACTED
			if err == io.EOF {
				return
		REDACTED
			if err != nil {
				_ = sendEvent(readEvent{err: errREDACTED)
				return
		REDACTED
	REDACTED
REDACTED()
	defer close(done)

	var intervalTicker *time.Ticker
	if streamInterval > 0 {
		intervalTicker = time.NewTicker(streamInterval)
		defer intervalTicker.Stop()
REDACTED
	var intervalCh <-chan time.Time
	if intervalTicker != nil {
		intervalCh = intervalTicker.C
REDACTED

	var keepaliveTicker *time.Ticker
	if keepaliveInterval > 0 {
		keepaliveTicker = time.NewTicker(keepaliveInterval)
		defer keepaliveTicker.Stop()
REDACTED
	var keepaliveCh <-chan time.Time
	if keepaliveTicker != nil {
		keepaliveCh = keepaliveTicker.C
REDACTED

	for {
		select {
		case ev, ok := <-events:
			if !ok {
				if done, processErr := flushData(); processErr != nil {
					return usage, imageCount, imageOutputSizes, firstTokenMs, processErr
			REDACTED else if done {
					return usage, imageCount, imageOutputSizes, firstTokenMs, nil
			REDACTED
				if err := finalizePending(); err != nil {
					return usage, imageCount, imageOutputSizes, firstTokenMs, err
			REDACTED
				return usage, imageCount, imageOutputSizes, firstTokenMs, nil
		REDACTED
			if ev.err != nil {
				if done, processErr := flushData(); processErr != nil {
					return usage, imageCount, imageOutputSizes, firstTokenMs, processErr
			REDACTED else if done {
					return usage, imageCount, imageOutputSizes, firstTokenMs, nil
			REDACTED
				s.tryWriteOpenAIImagesStreamEvent(c, flusher, &clientDisconnected, &lastDownstreamWriteAt, "error", buildOpenAIImagesStreamErrorBody(ev.err.Error()))
				return usage, imageCount, imageOutputSizes, firstTokenMs, ev.err
		REDACTED
			done, processErr := processLine(ev.line)
			if processErr != nil {
				return usage, imageCount, imageOutputSizes, firstTokenMs, processErr
		REDACTED
			if done {
				return usage, imageCount, imageOutputSizes, firstTokenMs, nil
		REDACTED
		case <-intervalCh:
			lastRead := time.Unix(0, atomic.LoadInt64(&lastReadAt))
			if time.Since(lastRead) < streamInterval {
				continue
		REDACTED
			if clientDisconnected {
				return usage, imageCount, imageOutputSizes, firstTokenMs, fmt.Errorf("image stream incomplete after timeout")
		REDACTED
			logger.LegacyPrintf("service.openai_gateway", "[OpenAI] Images responses stream data interval timeout: interval=%s", streamInterval)
			s.tryWriteOpenAIImagesStreamEvent(c, flusher, &clientDisconnected, &lastDownstreamWriteAt, "error", buildOpenAIImagesStreamErrorBody(fmt.Sprintf("upstream image stream idle for %s", streamInterval)))
			return usage, imageCount, imageOutputSizes, firstTokenMs, fmt.Errorf("image stream data interval timeout")
		case <-keepaliveCh:
			if clientDisconnected || time.Since(lastDownstreamWriteAt) < keepaliveInterval {
				continue
		REDACTED
			if _, writeErr := io.WriteString(c.Writer, ":\n\n"); writeErr != nil {
				clientDisconnected = true
				logger.LegacyPrintf("service.openai_gateway", "[OpenAI] Images responses stream client disconnected during keepalive, continue draining upstream for billing")
				continue
		REDACTED
			flusher.Flush()
			lastDownstreamWriteAt = time.Now()
	REDACTED
REDACTED
REDACTED

func (s *OpenAIGatewayService) forwardOpenAIImagesOAuth(
	ctx context.Context,
	c *gin.Context,
	account *Account,
	parsed *OpenAIImagesRequest,
	channelMappedModel string,
) (*OpenAIForwardResult, error) {
	startTime := time.Now()
	requestModel := strings.TrimSpace(parsed.Model)
	if mapped := strings.TrimSpace(channelMappedModel); mapped != "" {
		requestModel = mapped
REDACTED
	if requestModel == "" {
		requestModel = "gpt-image-2"
REDACTED
	if err := validateOpenAIImagesModel(requestModel); err != nil {
		return nil, err
REDACTED
	logger.LegacyPrintf(
		"service.openai_gateway",
		"[OpenAI] Images request routing request_model=%s endpoint=%s account_type=%s uploads=%d",
		requestModel,
		parsed.Endpoint,
		account.Type,
		len(parsed.Uploads),
	)
	upstreamCtx, releaseUpstreamCtx := detachUpstreamContext(ctx)
	defer releaseUpstreamCtx()

	token, _, err := s.GetAccessToken(upstreamCtx, account)
	if err != nil {
		return nil, err
REDACTED

	responsesBody, err := buildOpenAIImagesResponsesRequest(parsed, requestModel)
	if err != nil {
		return nil, err
REDACTED
	upstreamReq, err := s.buildUpstreamRequest(upstreamCtx, c, account, responsesBody, token, true, parsed.StickySessionSeed(), false)
	if err != nil {
		return nil, err
REDACTED
	upstreamReq.Header.Set("Content-Type", "application/json")
	upstreamReq.Header.Set("Accept", "text/event-stream")

	proxyURL := ""
	if account.ProxyID != nil && account.Proxy != nil {
		proxyURL = account.Proxy.URL()
REDACTED
	upstreamStart := time.Now()
	resp, err := s.httpUpstream.Do(upstreamReq, proxyURL, account.ID, account.Concurrency)
	SetOpsLatencyMs(c, OpsUpstreamLatencyMsKey, time.Since(upstreamStart).Milliseconds())
	if err != nil {
		safeErr := sanitizeUpstreamErrorMessage(err.Error())
		setOpsUpstreamError(c, 0, safeErr, "")
		appendOpsUpstreamError(c, OpsUpstreamErrorEvent{
			Platform:           account.Platform,
			AccountID:          account.ID,
			AccountName:        account.Name,
			UpstreamStatusCode: 0,
			UpstreamURL:        safeUpstreamURL(upstreamReq.URL.String()),
			Kind:               "request_error",
			Message:            safeErr,
	REDACTED)
		return nil, fmt.Errorf("upstream request failed: %s", safeErr)
REDACTED
	if resp.StatusCode >= 400 {
		respBody := s.readUpstreamErrorBody(resp)
		_ = resp.Body.Close()
		resp.Body = io.NopCloser(bytes.NewReader(respBody))
		upstreamMsg := strings.TrimSpace(extractUpstreamErrorMessage(respBody))
		upstreamMsg = sanitizeUpstreamErrorMessage(upstreamMsg)
		if s.shouldFailoverOpenAIUpstreamResponse(resp.StatusCode, upstreamMsg, respBody) {
			appendOpsUpstreamError(c, OpsUpstreamErrorEvent{
				Platform:           account.Platform,
				AccountID:          account.ID,
				AccountName:        account.Name,
				UpstreamStatusCode: resp.StatusCode,
				UpstreamRequestID:  resp.Header.Get("x-request-id"),
				UpstreamURL:        safeUpstreamURL(upstreamReq.URL.String()),
				Kind:               "failover",
				Message:            upstreamMsg,
		REDACTED)
			s.handleFailoverSideEffects(upstreamCtx, resp, account, respBody, requestModel)
			return nil, &UpstreamFailoverError{
				StatusCode:             resp.StatusCode,
				ResponseBody:           respBody,
				RetryableOnSameAccount: account.IsPoolMode() && account.IsPoolModeRetryableStatus(resp.StatusCode),
		REDACTED
	REDACTED
		return s.handleOpenAIImagesErrorResponse(upstreamCtx, resp, c, account, requestModel)
REDACTED
	defer func() { _ = resp.Body.Close() REDACTED()

	var (
		usage            OpenAIUsage
		imageCount       int
		imageOutputSizes []string
		firstTokenMs     *int
	)
	writerSizeBeforeResponse := c.Writer.Size()
	if parsed.Stream {
		usage, imageCount, imageOutputSizes, firstTokenMs, err = s.handleOpenAIImagesOAuthStreamingResponse(resp, c, startTime, parsed.ResponseFormat, openAIImagesStreamPrefix(parsed), requestModel)
		if err != nil {
			if imageCount > 0 {
				return &OpenAIForwardResult{
					RequestID:        resp.Header.Get("x-request-id"),
					Usage:            usage,
					Model:            requestModel,
					UpstreamModel:    requestModel,
					Stream:           parsed.Stream,
					ResponseHeaders:  resp.Header.Clone(),
					Duration:         time.Since(startTime),
					FirstTokenMs:     firstTokenMs,
					ImageCount:       imageCount,
					ImageSize:        parsed.SizeTier,
					ImageInputSize:   parsed.Size,
					ImageOutputSizes: imageOutputSizes,
			REDACTED, err
		REDACTED
			return nil, s.handleOpenAIImagesOAuthResponseError(
				upstreamCtx,
				c,
				account,
				requestModel,
				safeUpstreamURL(upstreamReq.URL.String()),
				resp,
				writerSizeBeforeResponse,
				err,
			)
	REDACTED
REDACTED else {
		usage, imageCount, imageOutputSizes, err = s.handleOpenAIImagesOAuthNonStreamingResponse(resp, c, parsed.ResponseFormat, requestModel)
		if err != nil {
			return nil, s.handleOpenAIImagesOAuthResponseError(
				upstreamCtx,
				c,
				account,
				requestModel,
				safeUpstreamURL(upstreamReq.URL.String()),
				resp,
				writerSizeBeforeResponse,
				err,
			)
	REDACTED
REDACTED
	if imageCount <= 0 {
		imageCount = parsed.N
REDACTED
	return &OpenAIForwardResult{
		RequestID:        resp.Header.Get("x-request-id"),
		Usage:            usage,
		Model:            requestModel,
		UpstreamModel:    requestModel,
		Stream:           parsed.Stream,
		ResponseHeaders:  resp.Header.Clone(),
		Duration:         time.Since(startTime),
		FirstTokenMs:     firstTokenMs,
		ImageCount:       imageCount,
		ImageSize:        parsed.SizeTier,
		ImageInputSize:   parsed.Size,
		ImageOutputSizes: imageOutputSizes,
REDACTED, nil
REDACTED

func (s *OpenAIGatewayService) handleOpenAIImagesOAuthResponseError(
	ctx context.Context,
	c *gin.Context,
	account *Account,
	requestedModel string,
	upstreamURL string,
	resp *http.Response,
	writerSizeBeforeResponse int,
	err error,
) error {
	var upstreamErr *OpenAIImagesUpstreamError
	if !errors.As(err, &upstreamErr) {
		return err
REDACTED

	retryable := IsOpenAIImagesRetryableUpstreamError(upstreamErr)
	responseWritten := c != nil && c.Writer != nil && c.Writer.Size() != writerSizeBeforeResponse
	kind := "http_error"
	if retryable {
		kind = "failover"
		if responseWritten {
			kind = "retry_exhausted_failover"
	REDACTED
REDACTED

	requestID := strings.TrimSpace(upstreamErr.UpstreamRequestID)
	headers := http.Header(nil)
	if resp != nil {
		headers = resp.Header.Clone()
		if requestID == "" {
			requestID = strings.TrimSpace(resp.Header.Get("x-request-id"))
	REDACTED
REDACTED
	appendOpsUpstreamError(c, OpsUpstreamErrorEvent{
		Platform:           account.Platform,
		AccountID:          account.ID,
		AccountName:        account.Name,
		UpstreamStatusCode: upstreamErr.StatusCode,
		UpstreamRequestID:  requestID,
		UpstreamURL:        upstreamURL,
		Kind:               kind,
		Message:            upstreamErr.clientMessage(),
REDACTED)

	if !retryable || responseWritten {
		return err
REDACTED

	responseBody := openAIImagesUpstreamErrorResponseBody(upstreamErr)
	s.handleOpenAIAccountUpstreamError(ctx, account, upstreamErr.StatusCode, headers, responseBody, requestedModel)
	return &UpstreamFailoverError{
		StatusCode:             upstreamErr.StatusCode,
		ResponseBody:           responseBody,
		ResponseHeaders:        headers,
		RetryableOnSameAccount: account.IsPoolMode() && account.IsPoolModeRetryableStatus(upstreamErr.StatusCode),
REDACTED
REDACTED
