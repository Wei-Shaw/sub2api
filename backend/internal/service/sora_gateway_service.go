package service

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
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

var soraImageSizeMap = map[string]string{
	"gpt-image":           "360",
	"gpt-image-landscape": "540",
	"gpt-image-portrait":  "540",
REDACTED

type soraStreamingResult struct {
	content      string
	mediaType    string
	mediaURLs    []string
	imageCount   int
	imageSize    string
	firstTokenMs *int
REDACTED

// SoraGatewayService handles forwarding requests to sora2api.
type SoraGatewayService struct {
	sora2api         *Sora2APIService
	httpUpstream     HTTPUpstream
	rateLimitService *RateLimitService
	cfg              *config.Config
REDACTED

func NewSoraGatewayService(
	sora2api *Sora2APIService,
	httpUpstream HTTPUpstream,
	rateLimitService *RateLimitService,
	cfg *config.Config,
) *SoraGatewayService {
	return &SoraGatewayService{
		sora2api:         sora2api,
		httpUpstream:     httpUpstream,
		rateLimitService: rateLimitService,
		cfg:              cfg,
REDACTED
REDACTED

func (s *SoraGatewayService) Forward(ctx context.Context, c *gin.Context, account *Account, body []byte, clientStream bool) (*ForwardResult, error) {
	startTime := time.Now()

	if s.sora2api == nil || !s.sora2api.Enabled() {
		if c != nil {
			c.JSON(http.StatusServiceUnavailable, gin.H{
				"error": gin.H{
					"type":    "api_error",
					"message": "sora2api 未配置",
			REDACTED,
		REDACTED)
	REDACTED
		return nil, errors.New("sora2api not configured")
REDACTED

	var reqBody map[string]any
	if err := json.Unmarshal(body, &reqBody); err != nil {
		return nil, fmt.Errorf("parse request: %w", err)
REDACTED
	reqModel, _ := reqBody["model"].(string)
	reqStream, _ := reqBody["stream"].(bool)

	mappedModel := account.GetMappedModel(reqModel)
	if mappedModel != reqModel && mappedModel != "" {
		reqBody["model"] = mappedModel
		if updated, err := json.Marshal(reqBody); err == nil {
			body = updated
	REDACTED
REDACTED

	reqCtx, cancel := s.withSoraTimeout(ctx, reqStream)
	if cancel != nil {
		defer cancel()
REDACTED

	upstreamReq, err := s.sora2api.NewAPIRequest(reqCtx, http.MethodPost, "/v1/chat/completions", body)
	if err != nil {
		return nil, err
REDACTED
	if c != nil {
		if ua := strings.TrimSpace(c.GetHeader("User-Agent")); ua != "" {
			upstreamReq.Header.Set("User-Agent", ua)
	REDACTED
REDACTED
	if reqStream {
		upstreamReq.Header.Set("Accept", "text/event-stream")
REDACTED

	if c != nil {
		c.Set(OpsUpstreamRequestBodyKey, string(body))
REDACTED

	proxyURL := ""
	if account != nil && account.ProxyID != nil && account.Proxy != nil {
		proxyURL = account.Proxy.URL()
REDACTED

	var resp *http.Response
	if s.httpUpstream != nil {
		resp, err = s.httpUpstream.Do(upstreamReq, proxyURL, account.ID, account.Concurrency)
REDACTED else {
		resp, err = http.DefaultClient.Do(upstreamReq)
REDACTED
	if err != nil {
		s.setUpstreamRequestError(c, account, err)
		return nil, fmt.Errorf("upstream request failed: %w", err)
REDACTED
	defer func() { _ = resp.Body.Close() REDACTED()

	if resp.StatusCode >= 400 {
		if s.shouldFailoverUpstreamError(resp.StatusCode) {
			respBody, _ := io.ReadAll(io.LimitReader(resp.Body, 2<<20))
			_ = resp.Body.Close()
			resp.Body = io.NopCloser(bytes.NewReader(respBody))
			upstreamMsg := strings.TrimSpace(extractUpstreamErrorMessage(respBody))
			upstreamMsg = sanitizeUpstreamErrorMessage(upstreamMsg)
			appendOpsUpstreamError(c, OpsUpstreamErrorEvent{
				Platform:           account.Platform,
				AccountID:          account.ID,
				AccountName:        account.Name,
				UpstreamStatusCode: resp.StatusCode,
				UpstreamRequestID:  resp.Header.Get("x-request-id"),
				Kind:               "failover",
				Message:            upstreamMsg,
		REDACTED)
			s.handleFailoverSideEffects(ctx, resp, account)
			return nil, &UpstreamFailoverError{StatusCode: resp.StatusCodeREDACTED
	REDACTED
		return s.handleErrorResponse(ctx, resp, c, account, reqModel)
REDACTED

	streamResult, err := s.handleStreamingResponse(ctx, resp, c, account, startTime, reqModel, clientStream)
	if err != nil {
		return nil, err
REDACTED

	result := &ForwardResult{
		RequestID:    resp.Header.Get("x-request-id"),
		Model:        reqModel,
		Stream:       clientStream,
		Duration:     time.Since(startTime),
		FirstTokenMs: streamResult.firstTokenMs,
		Usage:        ClaudeUsage{REDACTED,
		MediaType:    streamResult.mediaType,
		MediaURL:     firstMediaURL(streamResult.mediaURLs),
		ImageCount:   streamResult.imageCount,
		ImageSize:    streamResult.imageSize,
REDACTED

	return result, nil
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
				if err := sendLine("data: [DONE]"); err != nil {
					return nil, err
			REDACTED
				break
		REDACTED
			updatedLine, contentDelta, errEvent := s.processSoraSSEData(data, originalModel)
			if errEvent != nil && upstreamError == nil {
				upstreamError = errEvent
		REDACTED
			if contentDelta != "" {
				if firstTokenMs == nil {
					ms := int(time.Since(startTime).Milliseconds())
					firstTokenMs = &ms
			REDACTED
				contentBuilder.WriteString(contentDelta)
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
		content:      content,
		mediaType:    mediaType,
		mediaURLs:    mediaURLs,
		imageCount:   imageCount,
		imageSize:    imageSize,
		firstTokenMs: firstTokenMs,
REDACTED, nil
REDACTED

func (s *SoraGatewayService) processSoraSSEData(data string, originalModel string) (string, string, error) {
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
		rewritten := s.rewriteSoraContent(contentDelta)
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
