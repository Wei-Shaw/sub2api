package service

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/logger"
	"github.com/gin-gonic/gin"
)

// forwardToUpstream 将请求 HTTP 透传到上游 Sora 服务（用于 apikey 类型账号）。
// 上游地址为 account.GetBaseURL() + "/sora/v1/chat/completions"，
// 使用 account.GetCredential("api_key") 作为 Bearer Token。
// 支持流式和非流式响应的直接透传。
func (s *SoraGatewayService) forwardToUpstream(
	ctx context.Context,
	c *gin.Context,
	account *Account,
	body []byte,
	clientStream bool,
	startTime time.Time,
) (*ForwardResult, error) {
	apiKey := account.GetCredential("api_key")
	if apiKey == "" {
		s.writeSoraError(c, http.StatusBadGateway, "upstream_error", "Sora apikey account missing api_key credential", clientStream)
		return nil, fmt.Errorf("sora apikey account %d missing api_key", account.ID)
REDACTED

	baseURL := account.GetBaseURL()
	if baseURL == "" {
		s.writeSoraError(c, http.StatusBadGateway, "upstream_error", "Sora apikey account missing base_url", clientStream)
		return nil, fmt.Errorf("sora apikey account %d missing base_url", account.ID)
REDACTED
	// 校验 scheme 合法性（仅允许 http/https）
	if !strings.HasPrefix(baseURL, "http://") && !strings.HasPrefix(baseURL, "https://") {
		s.writeSoraError(c, http.StatusBadGateway, "upstream_error", "Sora apikey base_url must start with http:// or https://", clientStream)
		return nil, fmt.Errorf("sora apikey account %d invalid base_url scheme: %s", account.ID, baseURL)
REDACTED
	upstreamURL := strings.TrimRight(baseURL, "/") + "/sora/v1/chat/completions"

	// 构建上游请求
	upstreamReq, err := http.NewRequestWithContext(ctx, http.MethodPost, upstreamURL, bytes.NewReader(body))
	if err != nil {
		s.writeSoraError(c, http.StatusInternalServerError, "api_error", "Failed to create upstream request", clientStream)
		return nil, fmt.Errorf("create upstream request: %w", err)
REDACTED

	upstreamReq.Header.Set("Content-Type", "application/json")
	upstreamReq.Header.Set("Authorization", "Bearer "+apiKey)

	// 透传客户端的部分请求头
	for _, header := range []string{"Accept", "Accept-Encoding"REDACTED {
		if v := c.GetHeader(header); v != "" {
			upstreamReq.Header.Set(header, v)
	REDACTED
REDACTED

	logger.LegacyPrintf("service.sora", "[ForwardUpstream] account=%d url=%s", account.ID, upstreamURL)

	// 获取代理 URL
	proxyURL := ""
	if account.ProxyID != nil && account.Proxy != nil {
		proxyURL = account.Proxy.URL()
REDACTED

	// 发送请求
	resp, err := s.httpUpstream.Do(upstreamReq, proxyURL, account.ID, account.Concurrency)
	if err != nil {
		s.writeSoraError(c, http.StatusBadGateway, "upstream_error", "Failed to connect to upstream Sora service", clientStream)
		return nil, &UpstreamFailoverError{
			StatusCode: http.StatusBadGateway,
	REDACTED
REDACTED
	defer func() {
		_ = resp.Body.Close()
REDACTED()

	// 错误响应处理
	if resp.StatusCode >= 400 {
		respBody, _ := io.ReadAll(io.LimitReader(resp.Body, 64*1024))

		if s.shouldFailoverUpstreamError(resp.StatusCode) {
			return nil, &UpstreamFailoverError{
				StatusCode:      resp.StatusCode,
				ResponseBody:    respBody,
				ResponseHeaders: resp.Header.Clone(),
		REDACTED
	REDACTED

		// 非转移错误，直接透传给客户端
		c.Status(resp.StatusCode)
		for key, values := range resp.Header {
			for _, v := range values {
				c.Writer.Header().Add(key, v)
		REDACTED
	REDACTED
		if _, err := c.Writer.Write(respBody); err != nil {
			return nil, fmt.Errorf("write upstream error response: %w", err)
	REDACTED
		return nil, fmt.Errorf("upstream error: %d", resp.StatusCode)
REDACTED

	// 成功响应 — 直接透传
	c.Status(resp.StatusCode)
	for key, values := range resp.Header {
		lower := strings.ToLower(key)
		// 透传内容相关头部
		if lower == "content-type" || lower == "transfer-encoding" ||
			lower == "cache-control" || lower == "x-request-id" {
			for _, v := range values {
				c.Writer.Header().Add(key, v)
		REDACTED
	REDACTED
REDACTED

	// 流式复制响应体
	if flusher, ok := c.Writer.(http.Flusher); ok && clientStream {
		buf := make([]byte, 4096)
		for {
			n, readErr := resp.Body.Read(buf)
			if n > 0 {
				if _, err := c.Writer.Write(buf[:n]); err != nil {
					return nil, fmt.Errorf("stream upstream response write: %w", err)
			REDACTED
				flusher.Flush()
		REDACTED
			if readErr != nil {
				break
		REDACTED
	REDACTED
REDACTED else {
		if _, err := io.Copy(c.Writer, resp.Body); err != nil {
			return nil, fmt.Errorf("copy upstream response: %w", err)
	REDACTED
REDACTED

	duration := time.Since(startTime)
	return &ForwardResult{
		RequestID: resp.Header.Get("x-request-id"),
		Model:     "", // 由调用方填充
		Stream:    clientStream,
		Duration:  duration,
REDACTED, nil
REDACTED
