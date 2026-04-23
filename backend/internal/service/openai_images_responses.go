package service

import (
	"bufio"
	"bytes"
	"context"
	"encoding/base64"
	"fmt"
	"io"
	"net/http"
	"strings"
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

	for _, field := range []struct {
		path  string
		value string
REDACTED{
		{path: "size", value: parsed.SizeREDACTED,
		{path: "quality", value: parsed.QualityREDACTED,
		{path: "background", value: parsed.BackgroundREDACTED,
		{path: "output_format", value: parsed.OutputFormatREDACTED,
		{path: "moderation", value: parsed.ModerationREDACTED,
		{path: "input_fidelity", value: parsed.InputFidelityREDACTED,
		{path: "style", value: parsed.StyleREDACTED,
REDACTED {
		if trimmed := strings.TrimSpace(field.value); trimmed != "" {
			tool, _ = sjson.SetBytes(tool, field.path, trimmed)
	REDACTED
REDACTED
	if parsed.N > 1 {
		return nil, fmt.Errorf("codex /responses image tool currently supports only n=1")
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
		createdAt       int64
		usageRaw        []byte
		foundFinal      bool
	)

	for _, line := range bytes.Split(body, []byte("\n")) {
		line = bytes.TrimRight(line, "\r")
		data, ok := extractOpenAISSEDataLine(string(line))
		if !ok || data == "" || data == "[DONE]" {
			continue
	REDACTED
		payload := []byte(data)
		if !gjson.ValidBytes(payload) {
			continue
	REDACTED

		switch gjson.GetBytes(payload, "type").String() {
		case "response.created":
			if createdAt <= 0 {
				createdAt = gjson.GetBytes(payload, "response.created_at").Int()
		REDACTED
		case "response.output_item.done":
			result, itemID, ok, err := extractOpenAIImageFromResponsesOutputItemDone(payload)
			if err != nil {
				return nil, 0, nil, openAIResponsesImageResult{REDACTED, false, err
		REDACTED
			if ok {
				appendOpenAIResponsesImageResultDedup(&fallbackResults, fallbackSeen, itemID, result)
		REDACTED
		case "response.completed":
			results, completedAt, completedUsageRaw, firstMeta, err := extractOpenAIImagesFromResponsesCompleted(payload)
			if err != nil {
				return nil, 0, nil, openAIResponsesImageResult{REDACTED, false, err
		REDACTED
			foundFinal = true
			if completedAt > 0 {
				createdAt = completedAt
		REDACTED
			if len(completedUsageRaw) > 0 {
				usageRaw = completedUsageRaw
		REDACTED
			if len(results) > 0 {
				return results, createdAt, usageRaw, firstMeta, true, nil
		REDACTED
			if len(fallbackResults) > 0 {
				return fallbackResults, createdAt, usageRaw, fallbackResults[0], true, nil
		REDACTED
	REDACTED
REDACTED

	if len(fallbackResults) > 0 {
		return fallbackResults, createdAt, usageRaw, fallbackResults[0], foundFinal, nil
REDACTED
	return nil, createdAt, usageRaw, openAIResponsesImageResult{REDACTED, foundFinal, nil
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

func (s *OpenAIGatewayService) handleOpenAIImagesOAuthNonStreamingResponse(
	resp *http.Response,
	c *gin.Context,
	responseFormat string,
) (OpenAIUsage, int, error) {
	body, err := ReadUpstreamResponseBody(resp.Body, s.cfg, c, openAITooLargeError)
	if err != nil {
		return OpenAIUsage{REDACTED, 0, err
REDACTED

	var usage OpenAIUsage
	for _, line := range bytes.Split(body, []byte("\n")) {
		line = bytes.TrimRight(line, "\r")
		data, ok := extractOpenAISSEDataLine(string(line))
		if !ok || data == "" || data == "[DONE]" {
			continue
	REDACTED
		dataBytes := []byte(data)
		s.parseSSEUsageBytes(dataBytes, &usage)
REDACTED
	results, createdAt, usageRaw, firstMeta, _, err := collectOpenAIImagesFromResponsesBody(body)
	if err != nil {
		return OpenAIUsage{REDACTED, 0, err
REDACTED
	if len(results) == 0 {
		return OpenAIUsage{REDACTED, 0, fmt.Errorf("upstream did not return image output")
REDACTED

	responseBody, err := buildOpenAIImagesAPIResponse(results, createdAt, usageRaw, firstMeta, responseFormat)
	if err != nil {
		return OpenAIUsage{REDACTED, 0, err
REDACTED
	responseheaders.WriteFilteredHeaders(c.Writer.Header(), resp.Header, s.responseHeaderFilter)
	c.Data(resp.StatusCode, "application/json; charset=utf-8", responseBody)
	return usage, len(results), nil
REDACTED

func (s *OpenAIGatewayService) handleOpenAIImagesOAuthStreamingResponse(
	resp *http.Response,
	c *gin.Context,
	startTime time.Time,
	responseFormat string,
	streamPrefix string,
) (OpenAIUsage, int, *int, error) {
	responseheaders.WriteFilteredHeaders(c.Writer.Header(), resp.Header, s.responseHeaderFilter)
	c.Header("Content-Type", "text/event-stream")
	c.Header("Cache-Control", "no-cache")
	c.Header("Connection", "keep-alive")
	c.Status(resp.StatusCode)

	flusher, ok := c.Writer.(http.Flusher)
	if !ok {
		return OpenAIUsage{REDACTED, 0, nil, fmt.Errorf("streaming is not supported by response writer")
REDACTED

	format := strings.ToLower(strings.TrimSpace(responseFormat))
	if format == "" {
		format = "b64_json"
REDACTED

	reader := bufio.NewReader(resp.Body)
	usage := OpenAIUsage{REDACTED
	imageCount := 0
	var firstTokenMs *int
	emitted := make(map[string]struct{REDACTED)

	for {
		line, err := reader.ReadBytes('\n')
		if len(line) > 0 {
			trimmedLine := strings.TrimRight(string(line), "\r\n")
			data, ok := extractOpenAISSEDataLine(trimmedLine)
			if ok && data != "" && data != "[DONE]" {
				if firstTokenMs == nil {
					ms := int(time.Since(startTime).Milliseconds())
					firstTokenMs = &ms
			REDACTED
				dataBytes := []byte(data)
				s.parseSSEUsageBytes(dataBytes, &usage)
				if gjson.ValidBytes(dataBytes) {
					switch gjson.GetBytes(dataBytes, "type").String() {
					case "response.image_generation_call.partial_image":
						b64 := strings.TrimSpace(gjson.GetBytes(dataBytes, "partial_image_b64").String())
						if b64 != "" {
							eventName := streamPrefix + ".partial_image"
							payload := []byte(`{"type":"","partial_image_index":0REDACTED`)
							payload, _ = sjson.SetBytes(payload, "type", eventName)
							payload, _ = sjson.SetBytes(payload, "partial_image_index", gjson.GetBytes(dataBytes, "partial_image_index").Int())
							if format == "url" {
								outputFormat := strings.TrimSpace(gjson.GetBytes(dataBytes, "output_format").String())
								payload, _ = sjson.SetBytes(payload, "url", "data:"+openAIImageOutputMIMEType(outputFormat)+";base64,"+b64)
						REDACTED else {
								payload, _ = sjson.SetBytes(payload, "b64_json", b64)
						REDACTED
							if writeErr := s.writeOpenAIImagesStreamEvent(c, flusher, eventName, payload); writeErr != nil {
								return OpenAIUsage{REDACTED, imageCount, firstTokenMs, writeErr
						REDACTED
					REDACTED
					case "response.output_item.done":
						img, itemID, ok, extractErr := extractOpenAIImageFromResponsesOutputItemDone(dataBytes)
						if extractErr != nil {
							_ = s.writeOpenAIImagesStreamEvent(c, flusher, "error", buildOpenAIImagesStreamErrorBody(extractErr.Error()))
							return OpenAIUsage{REDACTED, imageCount, firstTokenMs, extractErr
					REDACTED
						if !ok {
							break
					REDACTED
						key := openAIResponsesImageResultKey(itemID, img)
						if _, exists := emitted[key]; exists {
							break
					REDACTED
						eventName := streamPrefix + ".completed"
						payload := []byte(`{"type":""REDACTED`)
						payload, _ = sjson.SetBytes(payload, "type", eventName)
						if format == "url" {
							payload, _ = sjson.SetBytes(payload, "url", "data:"+openAIImageOutputMIMEType(img.OutputFormat)+";base64,"+img.Result)
					REDACTED else {
							payload, _ = sjson.SetBytes(payload, "b64_json", img.Result)
					REDACTED
						if img.RevisedPrompt != "" {
							payload, _ = sjson.SetBytes(payload, "revised_prompt", img.RevisedPrompt)
					REDACTED
						if writeErr := s.writeOpenAIImagesStreamEvent(c, flusher, eventName, payload); writeErr != nil {
							return OpenAIUsage{REDACTED, imageCount, firstTokenMs, writeErr
					REDACTED
						emitted[key] = struct{REDACTED{REDACTED
						imageCount = len(emitted)
					case "response.completed":
						results, _, usageRaw, _, extractErr := extractOpenAIImagesFromResponsesCompleted(dataBytes)
						if extractErr != nil {
							_ = s.writeOpenAIImagesStreamEvent(c, flusher, "error", buildOpenAIImagesStreamErrorBody(extractErr.Error()))
							return OpenAIUsage{REDACTED, imageCount, firstTokenMs, extractErr
					REDACTED
						if len(results) == 0 {
							if imageCount > 0 {
								return usage, imageCount, firstTokenMs, nil
						REDACTED
							err = fmt.Errorf("upstream did not return image output")
							_ = s.writeOpenAIImagesStreamEvent(c, flusher, "error", buildOpenAIImagesStreamErrorBody(err.Error()))
							return OpenAIUsage{REDACTED, imageCount, firstTokenMs, err
					REDACTED
						eventName := streamPrefix + ".completed"
						for _, img := range results {
							key := openAIResponsesImageResultKey("", img)
							if _, exists := emitted[key]; exists {
								continue
						REDACTED
							payload := []byte(`{"type":""REDACTED`)
							payload, _ = sjson.SetBytes(payload, "type", eventName)
							if format == "url" {
								payload, _ = sjson.SetBytes(payload, "url", "data:"+openAIImageOutputMIMEType(img.OutputFormat)+";base64,"+img.Result)
						REDACTED else {
								payload, _ = sjson.SetBytes(payload, "b64_json", img.Result)
						REDACTED
							if img.RevisedPrompt != "" {
								payload, _ = sjson.SetBytes(payload, "revised_prompt", img.RevisedPrompt)
						REDACTED
							if len(usageRaw) > 0 && gjson.ValidBytes(usageRaw) {
								payload, _ = sjson.SetRawBytes(payload, "usage", usageRaw)
						REDACTED
							if writeErr := s.writeOpenAIImagesStreamEvent(c, flusher, eventName, payload); writeErr != nil {
								return OpenAIUsage{REDACTED, imageCount, firstTokenMs, writeErr
						REDACTED
							emitted[key] = struct{REDACTED{REDACTED
					REDACTED
						imageCount = len(emitted)
						return usage, imageCount, firstTokenMs, nil
				REDACTED
			REDACTED
		REDACTED
	REDACTED
		if err == io.EOF {
			break
	REDACTED
		if err != nil {
			_ = s.writeOpenAIImagesStreamEvent(c, flusher, "error", buildOpenAIImagesStreamErrorBody(err.Error()))
			return OpenAIUsage{REDACTED, imageCount, firstTokenMs, err
	REDACTED
REDACTED

	if imageCount > 0 {
		return usage, imageCount, firstTokenMs, nil
REDACTED

	streamErr := fmt.Errorf("stream disconnected before image generation completed")
	_ = s.writeOpenAIImagesStreamEvent(c, flusher, "error", buildOpenAIImagesStreamErrorBody(streamErr.Error()))
	return OpenAIUsage{REDACTED, imageCount, firstTokenMs, streamErr
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

	token, _, err := s.GetAccessToken(ctx, account)
	if err != nil {
		return nil, err
REDACTED

	responsesBody, err := buildOpenAIImagesResponsesRequest(parsed, requestModel)
	if err != nil {
		return nil, err
REDACTED
	setOpsUpstreamRequestBody(c, responsesBody)

	upstreamReq, err := s.buildUpstreamRequest(ctx, c, account, responsesBody, token, true, parsed.StickySessionSeed(), false)
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
		respBody, _ := io.ReadAll(io.LimitReader(resp.Body, 2<<20))
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
			s.handleFailoverSideEffects(ctx, resp, account)
			return nil, &UpstreamFailoverError{
				StatusCode:             resp.StatusCode,
				ResponseBody:           respBody,
				RetryableOnSameAccount: account.IsPoolMode() && isPoolModeRetryableStatus(resp.StatusCode),
		REDACTED
	REDACTED
		return s.handleErrorResponse(ctx, resp, c, account, responsesBody)
REDACTED
	defer func() { _ = resp.Body.Close() REDACTED()

	var (
		usage        OpenAIUsage
		imageCount   = parsed.N
		firstTokenMs *int
	)
	if parsed.Stream {
		usage, imageCount, firstTokenMs, err = s.handleOpenAIImagesOAuthStreamingResponse(resp, c, startTime, parsed.ResponseFormat, openAIImagesStreamPrefix(parsed))
		if err != nil {
			return nil, err
	REDACTED
REDACTED else {
		usage, imageCount, err = s.handleOpenAIImagesOAuthNonStreamingResponse(resp, c, parsed.ResponseFormat)
		if err != nil {
			return nil, err
	REDACTED
REDACTED
	if imageCount <= 0 {
		imageCount = parsed.N
REDACTED
	return &OpenAIForwardResult{
		RequestID:       resp.Header.Get("x-request-id"),
		Usage:           usage,
		Model:           requestModel,
		UpstreamModel:   requestModel,
		Stream:          parsed.Stream,
		ResponseHeaders: resp.Header.Clone(),
		Duration:        time.Since(startTime),
		FirstTokenMs:    firstTokenMs,
		ImageCount:      imageCount,
		ImageSize:       parsed.SizeTier,
REDACTED, nil
REDACTED
