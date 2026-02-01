package service

import (
	"bufio"
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime"
	"net"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/gin-gonic/gin"
)

var soraSSEDataRe = regexp.MustCompile(`^data:\s*`)
var soraImageMarkdownRe = regexp.MustCompile(`!\[[^\]]*\]\(([^)]+)\)`)
var soraVideoHTMLRe = regexp.MustCompile(`(?i)<video[^>]+src=['"]([^'"]+)['"]`)

const soraRewriteBufferLimit = 2048
const soraImageInputMaxBytes = 20 << 20
const soraImageInputMaxRedirects = 3
const soraImageInputTimeout = 20 * time.Second

var soraImageSizeMap = map[string]string{
	"gpt-image":           "360",
	"gpt-image-landscape": "540",
	"gpt-image-portrait":  "540",
REDACTED

var soraBlockedHostnames = map[string]struct{REDACTED{
	"localhost":                 {REDACTED,
	"localhost.localdomain":     {REDACTED,
	"metadata.google.internal":  {REDACTED,
	"metadata.google.internal.": {REDACTED,
REDACTED

var soraBlockedCIDRs = mustParseCIDRs([]string{
	"0.0.0.0/8",
	"10.0.0.0/8",
	"100.64.0.0/10",
	"127.0.0.0/8",
	"169.254.0.0/16",
	"172.16.0.0/12",
	"192.168.0.0/16",
	"224.0.0.0/4",
	"240.0.0.0/4",
	"::/128",
	"::1/128",
	"fc00::/7",
	"fe80::/10",
REDACTED)

type soraStreamingResult struct {
	mediaType    string
	mediaURLs    []string
	imageCount   int
	imageSize    string
	firstTokenMs *int
REDACTED

// SoraGatewayService handles forwarding requests to Sora upstream.
type SoraGatewayService struct {
	soraClient       SoraClient
	mediaStorage     *SoraMediaStorage
	rateLimitService *RateLimitService
	cfg              *config.Config
REDACTED

func NewSoraGatewayService(
	soraClient SoraClient,
	mediaStorage *SoraMediaStorage,
	rateLimitService *RateLimitService,
	cfg *config.Config,
) *SoraGatewayService {
	return &SoraGatewayService{
		soraClient:       soraClient,
		mediaStorage:     mediaStorage,
		rateLimitService: rateLimitService,
		cfg:              cfg,
REDACTED
REDACTED

func (s *SoraGatewayService) Forward(ctx context.Context, c *gin.Context, account *Account, body []byte, clientStream bool) (*ForwardResult, error) {
	startTime := time.Now()

	if s.soraClient == nil || !s.soraClient.Enabled() {
		if c != nil {
			c.JSON(http.StatusServiceUnavailable, gin.H{
				"error": gin.H{
					"type":    "api_error",
					"message": "Sora 上游未配置",
			REDACTED,
		REDACTED)
	REDACTED
		return nil, errors.New("sora upstream not configured")
REDACTED

	var reqBody map[string]any
	if err := json.Unmarshal(body, &reqBody); err != nil {
		s.writeSoraError(c, http.StatusBadRequest, "invalid_request_error", "Failed to parse request body", clientStream)
		return nil, fmt.Errorf("parse request: %w", err)
REDACTED
	reqModel, _ := reqBody["model"].(string)
	reqStream, _ := reqBody["stream"].(bool)
	if strings.TrimSpace(reqModel) == "" {
		s.writeSoraError(c, http.StatusBadRequest, "invalid_request_error", "model is required", clientStream)
		return nil, errors.New("model is required")
REDACTED

	mappedModel := account.GetMappedModel(reqModel)
	if mappedModel != "" && mappedModel != reqModel {
		reqModel = mappedModel
REDACTED

	modelCfg, ok := GetSoraModelConfig(reqModel)
	if !ok {
		s.writeSoraError(c, http.StatusBadRequest, "invalid_request_error", "Unsupported Sora model", clientStream)
		return nil, fmt.Errorf("unsupported model: %s", reqModel)
REDACTED
	if modelCfg.Type == "prompt_enhance" {
		s.writeSoraError(c, http.StatusBadRequest, "invalid_request_error", "Prompt-enhance 模型暂未支持", clientStream)
		return nil, fmt.Errorf("prompt-enhance not supported")
REDACTED

	prompt, imageInput, videoInput, remixTargetID := extractSoraInput(reqBody)
	if strings.TrimSpace(prompt) == "" {
		s.writeSoraError(c, http.StatusBadRequest, "invalid_request_error", "prompt is required", clientStream)
		return nil, errors.New("prompt is required")
REDACTED
	if strings.TrimSpace(videoInput) != "" {
		s.writeSoraError(c, http.StatusBadRequest, "invalid_request_error", "Video input is not supported yet", clientStream)
		return nil, errors.New("video input not supported")
REDACTED

	reqCtx, cancel := s.withSoraTimeout(ctx, reqStream)
	if cancel != nil {
		defer cancel()
REDACTED

	var imageData []byte
	imageFilename := ""
	if strings.TrimSpace(imageInput) != "" {
		decoded, filename, err := decodeSoraImageInput(reqCtx, imageInput)
		if err != nil {
			s.writeSoraError(c, http.StatusBadRequest, "invalid_request_error", err.Error(), clientStream)
			return nil, err
	REDACTED
		imageData = decoded
		imageFilename = filename
REDACTED

	mediaID := ""
	if len(imageData) > 0 {
		uploadID, err := s.soraClient.UploadImage(reqCtx, account, imageData, imageFilename)
		if err != nil {
			return nil, s.handleSoraRequestError(ctx, account, err, reqModel, c, clientStream)
	REDACTED
		mediaID = uploadID
REDACTED

	taskID := ""
	var err error
	switch modelCfg.Type {
	case "image":
		taskID, err = s.soraClient.CreateImageTask(reqCtx, account, SoraImageRequest{
			Prompt:  prompt,
			Width:   modelCfg.Width,
			Height:  modelCfg.Height,
			MediaID: mediaID,
	REDACTED)
	case "video":
		taskID, err = s.soraClient.CreateVideoTask(reqCtx, account, SoraVideoRequest{
			Prompt:        prompt,
			Orientation:   modelCfg.Orientation,
			Frames:        modelCfg.Frames,
			Model:         modelCfg.Model,
			Size:          modelCfg.Size,
			MediaID:       mediaID,
			RemixTargetID: remixTargetID,
	REDACTED)
	default:
		err = fmt.Errorf("unsupported model type: %s", modelCfg.Type)
REDACTED
	if err != nil {
		return nil, s.handleSoraRequestError(ctx, account, err, reqModel, c, clientStream)
REDACTED

	if clientStream && c != nil {
		s.prepareSoraStream(c, taskID)
REDACTED

	var mediaURLs []string
	mediaType := modelCfg.Type
	imageCount := 0
	imageSize := ""
	if modelCfg.Type == "image" {
		urls, pollErr := s.pollImageTask(reqCtx, c, account, taskID, clientStream)
		if pollErr != nil {
			return nil, s.handleSoraRequestError(ctx, account, pollErr, reqModel, c, clientStream)
	REDACTED
		mediaURLs = urls
		imageCount = len(urls)
		imageSize = soraImageSizeFromModel(reqModel)
REDACTED else if modelCfg.Type == "video" {
		urls, pollErr := s.pollVideoTask(reqCtx, c, account, taskID, clientStream)
		if pollErr != nil {
			return nil, s.handleSoraRequestError(ctx, account, pollErr, reqModel, c, clientStream)
	REDACTED
		mediaURLs = urls
REDACTED else {
		mediaType = "prompt"
REDACTED

	finalURLs := mediaURLs
	if len(mediaURLs) > 0 && s.mediaStorage != nil && s.mediaStorage.Enabled() {
		stored, storeErr := s.mediaStorage.StoreFromURLs(reqCtx, mediaType, mediaURLs)
		if storeErr != nil {
			return nil, s.handleSoraRequestError(ctx, account, storeErr, reqModel, c, clientStream)
	REDACTED
		finalURLs = s.normalizeSoraMediaURLs(stored)
REDACTED else {
		finalURLs = s.normalizeSoraMediaURLs(mediaURLs)
REDACTED

	content := buildSoraContent(mediaType, finalURLs)
	var firstTokenMs *int
	if clientStream {
		ms, streamErr := s.writeSoraStream(c, reqModel, content, startTime)
		if streamErr != nil {
			return nil, streamErr
	REDACTED
		firstTokenMs = ms
REDACTED else if c != nil {
		response := buildSoraNonStreamResponse(content, reqModel)
		if len(finalURLs) > 0 {
			response["media_url"] = finalURLs[0]
			if len(finalURLs) > 1 {
				response["media_urls"] = finalURLs
		REDACTED
	REDACTED
		c.JSON(http.StatusOK, response)
REDACTED

	return &ForwardResult{
		RequestID:    taskID,
		Model:        reqModel,
		Stream:       clientStream,
		Duration:     time.Since(startTime),
		FirstTokenMs: firstTokenMs,
		Usage:        ClaudeUsage{REDACTED,
		MediaType:    mediaType,
		MediaURL:     firstMediaURL(finalURLs),
		ImageCount:   imageCount,
		ImageSize:    imageSize,
REDACTED, nil
REDACTED

func (s *SoraGatewayService) withSoraTimeout(ctx context.Context, stream bool) (context.Context, context.CancelFunc) {
	if s == nil || s.cfg == nil {
		return ctx, nil
REDACTED
	timeoutSeconds := s.cfg.Gateway.SoraRequestTimeoutSeconds
	if stream {
		timeoutSeconds = s.cfg.Gateway.SoraStreamTimeoutSeconds
REDACTED
	if timeoutSeconds <= 0 {
		return ctx, nil
REDACTED
	return context.WithTimeout(ctx, time.Duration(timeoutSeconds)*time.Second)
REDACTED

func (s *SoraGatewayService) setUpstreamRequestError(c *gin.Context, account *Account, err error) {
	safeErr := sanitizeUpstreamErrorMessage(err.Error())
	setOpsUpstreamError(c, 0, safeErr, "")
	appendOpsUpstreamError(c, OpsUpstreamErrorEvent{
		Platform:           account.Platform,
		AccountID:          account.ID,
		AccountName:        account.Name,
		UpstreamStatusCode: 0,
		Kind:               "request_error",
		Message:            safeErr,
REDACTED)
	if c != nil {
		c.JSON(http.StatusBadGateway, gin.H{
			"error": gin.H{
				"type":    "upstream_error",
				"message": "Upstream request failed",
		REDACTED,
	REDACTED)
REDACTED
REDACTED

func (s *SoraGatewayService) shouldFailoverUpstreamError(statusCode int) bool {
	switch statusCode {
	case 401, 402, 403, 429, 529:
		return true
	default:
		return statusCode >= 500
REDACTED
REDACTED

func (s *SoraGatewayService) handleFailoverSideEffects(ctx context.Context, resp *http.Response, account *Account) {
	if s.rateLimitService == nil || account == nil || resp == nil {
		return
REDACTED
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 2<<20))
	s.rateLimitService.HandleUpstreamError(ctx, account, resp.StatusCode, resp.Header, body)
REDACTED

func (s *SoraGatewayService) handleErrorResponse(ctx context.Context, resp *http.Response, c *gin.Context, account *Account, reqModel string) (*ForwardResult, error) {
	respBody, _ := io.ReadAll(io.LimitReader(resp.Body, 2<<20))
	_ = resp.Body.Close()
	resp.Body = io.NopCloser(bytes.NewReader(respBody))

	upstreamMsg := strings.TrimSpace(extractUpstreamErrorMessage(respBody))
	upstreamMsg = sanitizeUpstreamErrorMessage(upstreamMsg)
	if msg := soraProErrorMessage(reqModel, upstreamMsg); msg != "" {
		upstreamMsg = msg
REDACTED

	upstreamDetail := ""
	if s.cfg != nil && s.cfg.Gateway.LogUpstreamErrorBody {
		maxBytes := s.cfg.Gateway.LogUpstreamErrorBodyMaxBytes
		if maxBytes <= 0 {
			maxBytes = 2048
	REDACTED
		upstreamDetail = truncateString(string(respBody), maxBytes)
REDACTED
	setOpsUpstreamError(c, resp.StatusCode, upstreamMsg, upstreamDetail)
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

	if c != nil {
		responsePayload := s.buildErrorPayload(respBody, upstreamMsg)
		c.JSON(resp.StatusCode, responsePayload)
REDACTED
	if upstreamMsg == "" {
		return nil, fmt.Errorf("upstream error: %d", resp.StatusCode)
REDACTED
	return nil, fmt.Errorf("upstream error: %d message=%s", resp.StatusCode, upstreamMsg)
REDACTED

func (s *SoraGatewayService) buildErrorPayload(respBody []byte, overrideMessage string) map[string]any {
	if len(respBody) > 0 {
		var payload map[string]any
		if err := json.Unmarshal(respBody, &payload); err == nil {
			if errObj, ok := payload["error"].(map[string]any); ok {
				if overrideMessage != "" {
					errObj["message"] = overrideMessage
			REDACTED
				payload["error"] = errObj
				return payload
		REDACTED
	REDACTED
REDACTED
	return map[string]any{
		"error": map[string]any{
			"type":    "upstream_error",
			"message": overrideMessage,
	REDACTED,
REDACTED
REDACTED

func (s *SoraGatewayService) handleStreamingResponse(ctx context.Context, resp *http.Response, c *gin.Context, account *Account, startTime time.Time, originalModel string, clientStream bool) (*soraStreamingResult, error) {
	if resp == nil {
		return nil, errors.New("empty response")
REDACTED

	if clientStream {
		c.Header("Content-Type", "text/event-stream")
		c.Header("Cache-Control", "no-cache")
		c.Header("Connection", "keep-alive")
		c.Header("X-Accel-Buffering", "no")
		if v := resp.Header.Get("x-request-id"); v != "" {
			c.Header("x-request-id", v)
	REDACTED
REDACTED

	w := c.Writer
	flusher, _ := w.(http.Flusher)

	contentBuilder := strings.Builder{REDACTED
	var firstTokenMs *int
	var upstreamError error
	rewriteBuffer := ""

	scanner := bufio.NewScanner(resp.Body)
	maxLineSize := defaultMaxLineSize
	if s.cfg != nil && s.cfg.Gateway.MaxLineSize > 0 {
		maxLineSize = s.cfg.Gateway.MaxLineSize
REDACTED
	scanner.Buffer(make([]byte, 64*1024), maxLineSize)

	sendLine := func(line string) error {
		if !clientStream {
			return nil
	REDACTED
		if _, err := fmt.Fprintf(w, "%s\n", line); err != nil {
			return err
	REDACTED
		if flusher != nil {
			flusher.Flush()
	REDACTED
		return nil
REDACTED

	for scanner.Scan() {
		line := scanner.Text()
		if soraSSEDataRe.MatchString(line) {
			data := soraSSEDataRe.ReplaceAllString(line, "")
			if data == "[DONE]" {
				if rewriteBuffer != "" {
					flushLine, flushContent, err := s.flushSoraRewriteBuffer(rewriteBuffer, originalModel)
					if err != nil {
						return nil, err
				REDACTED
					if flushLine != "" {
						if flushContent != "" {
							if _, err := contentBuilder.WriteString(flushContent); err != nil {
								return nil, err
						REDACTED
					REDACTED
						if err := sendLine(flushLine); err != nil {
							return nil, err
					REDACTED
				REDACTED
					rewriteBuffer = ""
			REDACTED
				if err := sendLine("data: [DONE]"); err != nil {
					return nil, err
			REDACTED
				break
		REDACTED
			updatedLine, contentDelta, errEvent := s.processSoraSSEData(data, originalModel, &rewriteBuffer)
			if errEvent != nil && upstreamError == nil {
				upstreamError = errEvent
		REDACTED
			if contentDelta != "" {
				if firstTokenMs == nil {
					ms := int(time.Since(startTime).Milliseconds())
					firstTokenMs = &ms
			REDACTED
				if _, err := contentBuilder.WriteString(contentDelta); err != nil {
					return nil, err
			REDACTED
		REDACTED
			if err := sendLine(updatedLine); err != nil {
				return nil, err
		REDACTED
			continue
	REDACTED
		if err := sendLine(line); err != nil {
			return nil, err
	REDACTED
REDACTED

	if err := scanner.Err(); err != nil {
		if errors.Is(err, bufio.ErrTooLong) {
			if clientStream {
				_, _ = fmt.Fprintf(w, "event: error\ndata: {\"error\":\"response_too_large\"REDACTED\n\n")
				if flusher != nil {
					flusher.Flush()
			REDACTED
		REDACTED
			return nil, err
	REDACTED
		if ctx.Err() == context.DeadlineExceeded && s.rateLimitService != nil && account != nil {
			s.rateLimitService.HandleStreamTimeout(ctx, account, originalModel)
	REDACTED
		if clientStream {
			_, _ = fmt.Fprintf(w, "event: error\ndata: {\"error\":\"stream_read_error\"REDACTED\n\n")
			if flusher != nil {
				flusher.Flush()
		REDACTED
	REDACTED
		return nil, err
REDACTED

	content := contentBuilder.String()
	mediaType, mediaURLs := s.extractSoraMedia(content)
	if mediaType == "" && isSoraPromptEnhanceModel(originalModel) {
		mediaType = "prompt"
REDACTED
	imageSize := ""
	imageCount := 0
	if mediaType == "image" {
		imageSize = soraImageSizeFromModel(originalModel)
		imageCount = len(mediaURLs)
REDACTED

	if upstreamError != nil && !clientStream {
		if c != nil {
			c.JSON(http.StatusBadGateway, map[string]any{
				"error": map[string]any{
					"type":    "upstream_error",
					"message": upstreamError.Error(),
			REDACTED,
		REDACTED)
	REDACTED
		return nil, upstreamError
REDACTED

	if !clientStream {
		response := buildSoraNonStreamResponse(content, originalModel)
		if len(mediaURLs) > 0 {
			response["media_url"] = mediaURLs[0]
			if len(mediaURLs) > 1 {
				response["media_urls"] = mediaURLs
		REDACTED
	REDACTED
		c.JSON(http.StatusOK, response)
REDACTED

	return &soraStreamingResult{
		mediaType:    mediaType,
		mediaURLs:    mediaURLs,
		imageCount:   imageCount,
		imageSize:    imageSize,
		firstTokenMs: firstTokenMs,
REDACTED, nil
REDACTED

func (s *SoraGatewayService) processSoraSSEData(data string, originalModel string, rewriteBuffer *string) (string, string, error) {
	if strings.TrimSpace(data) == "" {
		return "data: ", "", nil
REDACTED

	var payload map[string]any
	if err := json.Unmarshal([]byte(data), &payload); err != nil {
		return "data: " + data, "", nil
REDACTED

	if errObj, ok := payload["error"].(map[string]any); ok {
		if msg, ok := errObj["message"].(string); ok && strings.TrimSpace(msg) != "" {
			return "data: " + data, "", errors.New(msg)
	REDACTED
REDACTED

	if model, ok := payload["model"].(string); ok && model != "" && originalModel != "" {
		payload["model"] = originalModel
REDACTED

	contentDelta, updated := extractSoraContent(payload)
	if updated {
		var rewritten string
		if rewriteBuffer != nil {
			rewritten = s.rewriteSoraContentWithBuffer(contentDelta, rewriteBuffer)
	REDACTED else {
			rewritten = s.rewriteSoraContent(contentDelta)
	REDACTED
		if rewritten != contentDelta {
			applySoraContent(payload, rewritten)
			contentDelta = rewritten
	REDACTED
REDACTED

	updatedData, err := json.Marshal(payload)
	if err != nil {
		return "data: " + data, contentDelta, nil
REDACTED
	return "data: " + string(updatedData), contentDelta, nil
REDACTED

func extractSoraContent(payload map[string]any) (string, bool) {
	choices, ok := payload["choices"].([]any)
	if !ok || len(choices) == 0 {
		return "", false
REDACTED
	choice, ok := choices[0].(map[string]any)
	if !ok {
		return "", false
REDACTED
	if delta, ok := choice["delta"].(map[string]any); ok {
		if content, ok := delta["content"].(string); ok {
			return content, true
	REDACTED
REDACTED
	if message, ok := choice["message"].(map[string]any); ok {
		if content, ok := message["content"].(string); ok {
			return content, true
	REDACTED
REDACTED
	return "", false
REDACTED

func applySoraContent(payload map[string]any, content string) {
	choices, ok := payload["choices"].([]any)
	if !ok || len(choices) == 0 {
		return
REDACTED
	choice, ok := choices[0].(map[string]any)
	if !ok {
		return
REDACTED
	if delta, ok := choice["delta"].(map[string]any); ok {
		delta["content"] = content
		choice["delta"] = delta
		return
REDACTED
	if message, ok := choice["message"].(map[string]any); ok {
		message["content"] = content
		choice["message"] = message
REDACTED
REDACTED

func (s *SoraGatewayService) rewriteSoraContentWithBuffer(contentDelta string, buffer *string) string {
	if buffer == nil {
		return s.rewriteSoraContent(contentDelta)
REDACTED
	if contentDelta == "" && *buffer == "" {
		return ""
REDACTED
	combined := *buffer + contentDelta
	rewritten := s.rewriteSoraContent(combined)
	bufferStart := s.findSoraRewriteBufferStart(rewritten)
	if bufferStart < 0 {
		*buffer = ""
		return rewritten
REDACTED
	if len(rewritten)-bufferStart > soraRewriteBufferLimit {
		bufferStart = len(rewritten) - soraRewriteBufferLimit
REDACTED
	output := rewritten[:bufferStart]
	*buffer = rewritten[bufferStart:]
	return output
REDACTED

func (s *SoraGatewayService) findSoraRewriteBufferStart(content string) int {
	minIndex := -1
	start := 0
	for {
		idx := strings.Index(content[start:], "![")
		if idx < 0 {
			break
	REDACTED
		idx += start
		if !hasSoraImageMatchAt(content, idx) {
			if minIndex == -1 || idx < minIndex {
				minIndex = idx
		REDACTED
	REDACTED
		start = idx + 2
REDACTED
	lower := strings.ToLower(content)
	start = 0
	for {
		idx := strings.Index(lower[start:], "<video")
		if idx < 0 {
			break
	REDACTED
		idx += start
		if !hasSoraVideoMatchAt(content, idx) {
			if minIndex == -1 || idx < minIndex {
				minIndex = idx
		REDACTED
	REDACTED
		start = idx + len("<video")
REDACTED
	return minIndex
REDACTED

func hasSoraImageMatchAt(content string, idx int) bool {
	if idx < 0 || idx >= len(content) {
		return false
REDACTED
	loc := soraImageMarkdownRe.FindStringIndex(content[idx:])
	return loc != nil && loc[0] == 0
REDACTED

func hasSoraVideoMatchAt(content string, idx int) bool {
	if idx < 0 || idx >= len(content) {
		return false
REDACTED
	loc := soraVideoHTMLRe.FindStringIndex(content[idx:])
	return loc != nil && loc[0] == 0
REDACTED

func (s *SoraGatewayService) rewriteSoraContent(content string) string {
	if content == "" {
		return content
REDACTED
	content = soraImageMarkdownRe.ReplaceAllStringFunc(content, func(match string) string {
		sub := soraImageMarkdownRe.FindStringSubmatch(match)
		if len(sub) < 2 {
			return match
	REDACTED
		rewritten := s.rewriteSoraURL(sub[1])
		if rewritten == sub[1] {
			return match
	REDACTED
		return strings.Replace(match, sub[1], rewritten, 1)
REDACTED)
	content = soraVideoHTMLRe.ReplaceAllStringFunc(content, func(match string) string {
		sub := soraVideoHTMLRe.FindStringSubmatch(match)
		if len(sub) < 2 {
			return match
	REDACTED
		rewritten := s.rewriteSoraURL(sub[1])
		if rewritten == sub[1] {
			return match
	REDACTED
		return strings.Replace(match, sub[1], rewritten, 1)
REDACTED)
	return content
REDACTED

func (s *SoraGatewayService) flushSoraRewriteBuffer(buffer string, originalModel string) (string, string, error) {
	if buffer == "" {
		return "", "", nil
REDACTED
	rewritten := s.rewriteSoraContent(buffer)
	payload := map[string]any{
		"choices": []any{
			map[string]any{
				"delta": map[string]any{
					"content": rewritten,
			REDACTED,
				"index": 0,
		REDACTED,
	REDACTED,
REDACTED
	if originalModel != "" {
		payload["model"] = originalModel
REDACTED
	updatedData, err := json.Marshal(payload)
	if err != nil {
		return "", "", err
REDACTED
	return "data: " + string(updatedData), rewritten, nil
REDACTED

func (s *SoraGatewayService) rewriteSoraURL(raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return raw
REDACTED
	parsed, err := url.Parse(raw)
	if err != nil {
		return raw
REDACTED
	path := parsed.Path
	if !strings.HasPrefix(path, "/tmp/") && !strings.HasPrefix(path, "/static/") {
		return raw
REDACTED
	return s.buildSoraMediaURL(path, parsed.RawQuery)
REDACTED

func (s *SoraGatewayService) extractSoraMedia(content string) (string, []string) {
	if content == "" {
		return "", nil
REDACTED
	if match := soraVideoHTMLRe.FindStringSubmatch(content); len(match) > 1 {
		return "video", []string{match[1]REDACTED
REDACTED
	imageMatches := soraImageMarkdownRe.FindAllStringSubmatch(content, -1)
	if len(imageMatches) == 0 {
		return "", nil
REDACTED
	urls := make([]string, 0, len(imageMatches))
	for _, match := range imageMatches {
		if len(match) > 1 {
			urls = append(urls, match[1])
	REDACTED
REDACTED
	return "image", urls
REDACTED

func buildSoraNonStreamResponse(content, model string) map[string]any {
	return map[string]any{
		"id":      fmt.Sprintf("chatcmpl-%d", time.Now().UnixNano()),
		"object":  "chat.completion",
		"created": time.Now().Unix(),
		"model":   model,
		"choices": []any{
			map[string]any{
				"index": 0,
				"message": map[string]any{
					"role":    "assistant",
					"content": content,
			REDACTED,
				"finish_reason": "stop",
		REDACTED,
	REDACTED,
REDACTED
REDACTED

func soraImageSizeFromModel(model string) string {
	modelLower := strings.ToLower(model)
	if size, ok := soraImageSizeMap[modelLower]; ok {
		return size
REDACTED
	if strings.Contains(modelLower, "landscape") || strings.Contains(modelLower, "portrait") {
		return "540"
REDACTED
	return "360"
REDACTED

func isSoraPromptEnhanceModel(model string) bool {
	return strings.HasPrefix(strings.ToLower(strings.TrimSpace(model)), "prompt-enhance")
REDACTED

func soraProErrorMessage(model, upstreamMsg string) string {
	modelLower := strings.ToLower(model)
	if strings.Contains(modelLower, "sora2pro-hd") {
		return "当前账号无法使用 Sora Pro-HD 模型，请更换模型或账号"
REDACTED
	if strings.Contains(modelLower, "sora2pro") {
		return "当前账号无法使用 Sora Pro 模型，请更换模型或账号"
REDACTED
	return ""
REDACTED

func firstMediaURL(urls []string) string {
	if len(urls) == 0 {
		return ""
REDACTED
	return urls[0]
REDACTED

func (s *SoraGatewayService) buildSoraMediaURL(path string, rawQuery string) string {
	if path == "" {
		return path
REDACTED
	prefix := "/sora/media"
	values := url.Values{REDACTED
	if rawQuery != "" {
		if parsed, err := url.ParseQuery(rawQuery); err == nil {
			values = parsed
	REDACTED
REDACTED

	signKey := ""
	ttlSeconds := 0
	if s != nil && s.cfg != nil {
		signKey = strings.TrimSpace(s.cfg.Gateway.SoraMediaSigningKey)
		ttlSeconds = s.cfg.Gateway.SoraMediaSignedURLTTLSeconds
REDACTED
	values.Del("sig")
	values.Del("expires")
	signingQuery := values.Encode()
	if signKey != "" && ttlSeconds > 0 {
		expires := time.Now().Add(time.Duration(ttlSeconds) * time.Second).Unix()
		signature := SignSoraMediaURL(path, signingQuery, expires, signKey)
		if signature != "" {
			values.Set("expires", strconv.FormatInt(expires, 10))
			values.Set("sig", signature)
			prefix = "/sora/media-signed"
	REDACTED
REDACTED

	encoded := values.Encode()
	if encoded == "" {
		return prefix + path
REDACTED
	return prefix + path + "?" + encoded
REDACTED

func (s *SoraGatewayService) prepareSoraStream(c *gin.Context, requestID string) {
	if c == nil {
		return
REDACTED
	c.Header("Content-Type", "text/event-stream")
	c.Header("Cache-Control", "no-cache")
	c.Header("Connection", "keep-alive")
	c.Header("X-Accel-Buffering", "no")
	if strings.TrimSpace(requestID) != "" {
		c.Header("x-request-id", requestID)
REDACTED
REDACTED

func (s *SoraGatewayService) writeSoraStream(c *gin.Context, model, content string, startTime time.Time) (*int, error) {
	if c == nil {
		return nil, nil
REDACTED
	writer := c.Writer
	flusher, _ := writer.(http.Flusher)

	chunk := map[string]any{
		"id":      fmt.Sprintf("chatcmpl-%d", time.Now().UnixNano()),
		"object":  "chat.completion.chunk",
		"created": time.Now().Unix(),
		"model":   model,
		"choices": []any{
			map[string]any{
				"index": 0,
				"delta": map[string]any{
					"content": content,
			REDACTED,
		REDACTED,
	REDACTED,
REDACTED
	encoded, _ := json.Marshal(chunk)
	if _, err := fmt.Fprintf(writer, "data: %s\n\n", encoded); err != nil {
		return nil, err
REDACTED
	if flusher != nil {
		flusher.Flush()
REDACTED
	ms := int(time.Since(startTime).Milliseconds())
	finalChunk := map[string]any{
		"id":      chunk["id"],
		"object":  "chat.completion.chunk",
		"created": time.Now().Unix(),
		"model":   model,
		"choices": []any{
			map[string]any{
				"index":         0,
				"delta":         map[string]any{REDACTED,
				"finish_reason": "stop",
		REDACTED,
	REDACTED,
REDACTED
	finalEncoded, _ := json.Marshal(finalChunk)
	if _, err := fmt.Fprintf(writer, "data: %s\n\n", finalEncoded); err != nil {
		return &ms, err
REDACTED
	if _, err := fmt.Fprint(writer, "data: [DONE]\n\n"); err != nil {
		return &ms, err
REDACTED
	if flusher != nil {
		flusher.Flush()
REDACTED
	return &ms, nil
REDACTED

func (s *SoraGatewayService) writeSoraError(c *gin.Context, status int, errType, message string, stream bool) {
	if c == nil {
		return
REDACTED
	if stream {
		flusher, _ := c.Writer.(http.Flusher)
		errorEvent := fmt.Sprintf(`event: error`+"\n"+`data: {"error": {"type": "%s", "message": "%s"REDACTEDREDACTED`+"\n\n", errType, message)
		_, _ = fmt.Fprint(c.Writer, errorEvent)
		_, _ = fmt.Fprint(c.Writer, "data: [DONE]\n\n")
		if flusher != nil {
			flusher.Flush()
	REDACTED
		return
REDACTED
	c.JSON(status, gin.H{
		"error": gin.H{
			"type":    errType,
			"message": message,
	REDACTED,
REDACTED)
REDACTED

func (s *SoraGatewayService) handleSoraRequestError(ctx context.Context, account *Account, err error, model string, c *gin.Context, stream bool) error {
	if err == nil {
		return nil
REDACTED
	var upstreamErr *SoraUpstreamError
	if errors.As(err, &upstreamErr) {
		if s.rateLimitService != nil && account != nil {
			s.rateLimitService.HandleUpstreamError(ctx, account, upstreamErr.StatusCode, upstreamErr.Headers, upstreamErr.Body)
	REDACTED
		if s.shouldFailoverUpstreamError(upstreamErr.StatusCode) {
			return &UpstreamFailoverError{StatusCode: upstreamErr.StatusCodeREDACTED
	REDACTED
		msg := upstreamErr.Message
		if override := soraProErrorMessage(model, msg); override != "" {
			msg = override
	REDACTED
		s.writeSoraError(c, upstreamErr.StatusCode, "upstream_error", msg, stream)
		return err
REDACTED
	if errors.Is(err, context.DeadlineExceeded) {
		s.writeSoraError(c, http.StatusGatewayTimeout, "timeout_error", "Sora generation timeout", stream)
		return err
REDACTED
	s.writeSoraError(c, http.StatusBadGateway, "api_error", err.Error(), stream)
	return err
REDACTED

func (s *SoraGatewayService) pollImageTask(ctx context.Context, c *gin.Context, account *Account, taskID string, stream bool) ([]string, error) {
	interval := s.pollInterval()
	maxAttempts := s.pollMaxAttempts()
	lastPing := time.Now()
	for attempt := 0; attempt < maxAttempts; attempt++ {
		status, err := s.soraClient.GetImageTask(ctx, account, taskID)
		if err != nil {
			return nil, err
	REDACTED
		switch strings.ToLower(status.Status) {
		case "succeeded", "completed":
			return status.URLs, nil
		case "failed":
			if status.ErrorMsg != "" {
				return nil, errors.New(status.ErrorMsg)
		REDACTED
			return nil, errors.New("Sora image generation failed")
	REDACTED
		if stream {
			s.maybeSendPing(c, &lastPing)
	REDACTED
		if err := sleepWithContext(ctx, interval); err != nil {
			return nil, err
	REDACTED
REDACTED
	return nil, errors.New("Sora image generation timeout")
REDACTED

func (s *SoraGatewayService) pollVideoTask(ctx context.Context, c *gin.Context, account *Account, taskID string, stream bool) ([]string, error) {
	interval := s.pollInterval()
	maxAttempts := s.pollMaxAttempts()
	lastPing := time.Now()
	for attempt := 0; attempt < maxAttempts; attempt++ {
		status, err := s.soraClient.GetVideoTask(ctx, account, taskID)
		if err != nil {
			return nil, err
	REDACTED
		switch strings.ToLower(status.Status) {
		case "completed", "succeeded":
			return status.URLs, nil
		case "failed":
			if status.ErrorMsg != "" {
				return nil, errors.New(status.ErrorMsg)
		REDACTED
			return nil, errors.New("Sora video generation failed")
	REDACTED
		if stream {
			s.maybeSendPing(c, &lastPing)
	REDACTED
		if err := sleepWithContext(ctx, interval); err != nil {
			return nil, err
	REDACTED
REDACTED
	return nil, errors.New("Sora video generation timeout")
REDACTED

func (s *SoraGatewayService) pollInterval() time.Duration {
	if s == nil || s.cfg == nil {
		return 2 * time.Second
REDACTED
	interval := s.cfg.Sora.Client.PollIntervalSeconds
	if interval <= 0 {
		interval = 2
REDACTED
	return time.Duration(interval) * time.Second
REDACTED

func (s *SoraGatewayService) pollMaxAttempts() int {
	if s == nil || s.cfg == nil {
		return 600
REDACTED
	maxAttempts := s.cfg.Sora.Client.MaxPollAttempts
	if maxAttempts <= 0 {
		maxAttempts = 600
REDACTED
	return maxAttempts
REDACTED

func (s *SoraGatewayService) maybeSendPing(c *gin.Context, lastPing *time.Time) {
	if c == nil {
		return
REDACTED
	interval := 10 * time.Second
	if s != nil && s.cfg != nil && s.cfg.Concurrency.PingInterval > 0 {
		interval = time.Duration(s.cfg.Concurrency.PingInterval) * time.Second
REDACTED
	if time.Since(*lastPing) < interval {
		return
REDACTED
	if _, err := fmt.Fprint(c.Writer, ":\n\n"); err == nil {
		if flusher, ok := c.Writer.(http.Flusher); ok {
			flusher.Flush()
	REDACTED
		*lastPing = time.Now()
REDACTED
REDACTED

func (s *SoraGatewayService) normalizeSoraMediaURLs(urls []string) []string {
	if len(urls) == 0 {
		return urls
REDACTED
	output := make([]string, 0, len(urls))
	for _, raw := range urls {
		raw = strings.TrimSpace(raw)
		if raw == "" {
			continue
	REDACTED
		if strings.HasPrefix(raw, "http://") || strings.HasPrefix(raw, "https://") {
			output = append(output, raw)
			continue
	REDACTED
		pathVal := raw
		if !strings.HasPrefix(pathVal, "/") {
			pathVal = "/" + pathVal
	REDACTED
		output = append(output, s.buildSoraMediaURL(pathVal, ""))
REDACTED
	return output
REDACTED

func buildSoraContent(mediaType string, urls []string) string {
	switch mediaType {
	case "image":
		parts := make([]string, 0, len(urls))
		for _, u := range urls {
			parts = append(parts, fmt.Sprintf("![image](%s)", u))
	REDACTED
		return strings.Join(parts, "\n")
	case "video":
		if len(urls) == 0 {
			return ""
	REDACTED
		return fmt.Sprintf("```html\n<video src='%s' controls></video>\n```", urls[0])
	default:
		return ""
REDACTED
REDACTED

func extractSoraInput(body map[string]any) (prompt, imageInput, videoInput, remixTargetID string) {
	if body == nil {
		return "", "", "", ""
REDACTED
	if v, ok := body["remix_target_id"].(string); ok {
		remixTargetID = v
REDACTED
	if v, ok := body["image"].(string); ok {
		imageInput = v
REDACTED
	if v, ok := body["video"].(string); ok {
		videoInput = v
REDACTED
	if v, ok := body["prompt"].(string); ok && strings.TrimSpace(v) != "" {
		prompt = v
REDACTED
	if messages, ok := body["messages"].([]any); ok {
		builder := strings.Builder{REDACTED
		for _, raw := range messages {
			msg, ok := raw.(map[string]any)
			if !ok {
				continue
		REDACTED
			role, _ := msg["role"].(string)
			if role != "" && role != "user" {
				continue
		REDACTED
			content := msg["content"]
			text, img, vid := parseSoraMessageContent(content)
			if text != "" {
				if builder.Len() > 0 {
					builder.WriteString("\n")
			REDACTED
				builder.WriteString(text)
		REDACTED
			if imageInput == "" && img != "" {
				imageInput = img
		REDACTED
			if videoInput == "" && vid != "" {
				videoInput = vid
		REDACTED
	REDACTED
		if prompt == "" {
			prompt = builder.String()
	REDACTED
REDACTED
	return prompt, imageInput, videoInput, remixTargetID
REDACTED

func parseSoraMessageContent(content any) (text, imageInput, videoInput string) {
	switch val := content.(type) {
	case string:
		return val, "", ""
	case []any:
		builder := strings.Builder{REDACTED
		for _, item := range val {
			itemMap, ok := item.(map[string]any)
			if !ok {
				continue
		REDACTED
			t, _ := itemMap["type"].(string)
			switch t {
			case "text":
				if txt, ok := itemMap["text"].(string); ok && strings.TrimSpace(txt) != "" {
					if builder.Len() > 0 {
						builder.WriteString("\n")
				REDACTED
					builder.WriteString(txt)
			REDACTED
			case "image_url":
				if imageInput == "" {
					if urlVal, ok := itemMap["image_url"].(map[string]any); ok {
						imageInput = fmt.Sprintf("%v", urlVal["url"])
				REDACTED else if urlStr, ok := itemMap["image_url"].(string); ok {
						imageInput = urlStr
				REDACTED
			REDACTED
			case "video_url":
				if videoInput == "" {
					if urlVal, ok := itemMap["video_url"].(map[string]any); ok {
						videoInput = fmt.Sprintf("%v", urlVal["url"])
				REDACTED else if urlStr, ok := itemMap["video_url"].(string); ok {
						videoInput = urlStr
				REDACTED
			REDACTED
		REDACTED
	REDACTED
		return builder.String(), imageInput, videoInput
	default:
		return "", "", ""
REDACTED
REDACTED

func decodeSoraImageInput(ctx context.Context, input string) ([]byte, string, error) {
	raw := strings.TrimSpace(input)
	if raw == "" {
		return nil, "", errors.New("empty image input")
REDACTED
	if strings.HasPrefix(raw, "data:") {
		parts := strings.SplitN(raw, ",", 2)
		if len(parts) != 2 {
			return nil, "", errors.New("invalid data url")
	REDACTED
		meta := parts[0]
		payload := parts[1]
		decoded, err := base64.StdEncoding.DecodeString(payload)
		if err != nil {
			return nil, "", err
	REDACTED
		ext := ""
		if strings.HasPrefix(meta, "data:") {
			metaParts := strings.SplitN(meta[5:], ";", 2)
			if len(metaParts) > 0 {
				if exts, err := mime.ExtensionsByType(metaParts[0]); err == nil && len(exts) > 0 {
					ext = exts[0]
			REDACTED
		REDACTED
	REDACTED
		filename := "image" + ext
		return decoded, filename, nil
REDACTED
	if strings.HasPrefix(raw, "http://") || strings.HasPrefix(raw, "https://") {
		return downloadSoraImageInput(ctx, raw)
REDACTED
	decoded, err := base64.StdEncoding.DecodeString(raw)
	if err != nil {
		return nil, "", errors.New("invalid base64 image")
REDACTED
	return decoded, "image.png", nil
REDACTED

func downloadSoraImageInput(ctx context.Context, rawURL string) ([]byte, string, error) {
	parsed, err := validateSoraImageURL(rawURL)
	if err != nil {
		return nil, "", err
REDACTED
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, parsed.String(), nil)
	if err != nil {
		return nil, "", err
REDACTED
	client := &http.Client{
		Timeout: soraImageInputTimeout,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			if len(via) >= soraImageInputMaxRedirects {
				return errors.New("too many redirects")
		REDACTED
			return validateSoraImageURLValue(req.URL)
	REDACTED,
REDACTED
	resp, err := client.Do(req)
	if err != nil {
		return nil, "", err
REDACTED
	defer func() { _ = resp.Body.Close() REDACTED()
	if resp.StatusCode != http.StatusOK {
		return nil, "", fmt.Errorf("download image failed: %d", resp.StatusCode)
REDACTED
	data, err := io.ReadAll(io.LimitReader(resp.Body, soraImageInputMaxBytes))
	if err != nil {
		return nil, "", err
REDACTED
	ext := fileExtFromURL(parsed.String())
	if ext == "" {
		ext = fileExtFromContentType(resp.Header.Get("Content-Type"))
REDACTED
	filename := "image" + ext
	return data, filename, nil
REDACTED

func validateSoraImageURL(raw string) (*url.URL, error) {
	if strings.TrimSpace(raw) == "" {
		return nil, errors.New("empty image url")
REDACTED
	parsed, err := url.Parse(raw)
	if err != nil {
		return nil, fmt.Errorf("invalid image url: %w", err)
REDACTED
	if err := validateSoraImageURLValue(parsed); err != nil {
		return nil, err
REDACTED
	return parsed, nil
REDACTED

func validateSoraImageURLValue(parsed *url.URL) error {
	if parsed == nil {
		return errors.New("invalid image url")
REDACTED
	scheme := strings.ToLower(strings.TrimSpace(parsed.Scheme))
	if scheme != "http" && scheme != "https" {
		return errors.New("only http/https image url is allowed")
REDACTED
	if parsed.User != nil {
		return errors.New("image url cannot contain userinfo")
REDACTED
	host := strings.ToLower(strings.TrimSpace(parsed.Hostname()))
	if host == "" {
		return errors.New("image url missing host")
REDACTED
	if _, blocked := soraBlockedHostnames[host]; blocked {
		return errors.New("image url is not allowed")
REDACTED
	if ip := net.ParseIP(host); ip != nil {
		if isSoraBlockedIP(ip) {
			return errors.New("image url is not allowed")
	REDACTED
		return nil
REDACTED
	ips, err := net.LookupIP(host)
	if err != nil {
		return fmt.Errorf("resolve image url failed: %w", err)
REDACTED
	for _, ip := range ips {
		if isSoraBlockedIP(ip) {
			return errors.New("image url is not allowed")
	REDACTED
REDACTED
	return nil
REDACTED

func isSoraBlockedIP(ip net.IP) bool {
	if ip == nil {
		return true
REDACTED
	for _, cidr := range soraBlockedCIDRs {
		if cidr.Contains(ip) {
			return true
	REDACTED
REDACTED
	return false
REDACTED

func mustParseCIDRs(values []string) []*net.IPNet {
	out := make([]*net.IPNet, 0, len(values))
	for _, val := range values {
		_, cidr, err := net.ParseCIDR(val)
		if err != nil {
			continue
	REDACTED
		out = append(out, cidr)
REDACTED
	return out
REDACTED
