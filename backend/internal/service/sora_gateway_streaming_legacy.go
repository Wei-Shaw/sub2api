//nolint:unused
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
	"strings"
	"time"

	"github.com/gin-gonic/gin"
)

var soraSSEDataRe = regexp.MustCompile(`^data:\s*`)
var soraImageMarkdownRe = regexp.MustCompile(`!\[[^\]]*\]\(([^)]+)\)`)
var soraVideoHTMLRe = regexp.MustCompile(`(?i)<video[^>]+src=['"]([^'"]+)['"]`)

const soraRewriteBufferLimit = 2048

type soraStreamingResult struct {
	mediaType    string
	mediaURLs    []string
	imageCount   int
	imageSize    string
	firstTokenMs *int
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

func isSoraPromptEnhanceModel(model string) bool {
	return strings.HasPrefix(strings.ToLower(strings.TrimSpace(model)), "prompt-enhance")
REDACTED
