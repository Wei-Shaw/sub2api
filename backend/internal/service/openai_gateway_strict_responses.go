package service

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/util/responseheaders"
	"github.com/gin-gonic/gin"
)

const strictResponsesObservationLimit = 8 << 20

var strictResponsesRequestHeaders = map[string]struct{}{
	"accept":             {},
	"accept-language":    {},
	"content-type":       {},
	"conversation_id":    {},
	"openai-beta":        {},
	"originator":         {},
	"session_id":         {},
	"user-agent":         {},
	"x-conversation-id":  {},
	"x-opencode-session": {},
	"x-session-affinity": {},
	"x-session-id":       {},
}

var strictResponsesBlockedHeaders = map[string]struct{}{
	"authorization":         {},
	"connection":            {},
	"content-length":        {},
	"cookie":                {},
	"host":                  {},
	"keep-alive":            {},
	"proxy-authenticate":    {},
	"proxy-authorization":   {},
	"set-cookie":            {},
	"te":                    {},
	"trailer":               {},
	"transfer-encoding":     {},
	"upgrade":               {},
	"x-api-key":             {},
	"x-codex-api-key":       {},
	"x-codex-auth":          {},
	"x-codex-authorization": {},
	"x-codex-token":         {},
	"x-goog-api-key":        {},
}

type openAIStrictResponsesHTTPError struct {
	status int
}

func (e *openAIStrictResponsesHTTPError) Error() string {
	// The handler recognizes this established prefix as an upstream error body
	// that has already been written and must not append a synthetic SSE failure.
	return fmt.Sprintf("upstream response failed: strict Responses upstream returned HTTP %d", e.status)
}

func strictOpenAIResponsesPathSuffix(c *gin.Context) (string, bool) {
	if c == nil || c.Request == nil || c.Request.URL == nil {
		return "", false
	}
	path := strings.TrimRight(strings.TrimSpace(c.Request.URL.Path), "/")
	switch path {
	case "/v1/responses", "/openai/v1/responses", "/responses", "/backend-api/codex/responses":
		return "", true
	case "/v1/responses/compact", "/openai/v1/responses/compact", "/responses/compact", "/backend-api/codex/responses/compact":
		return "/compact", true
	default:
		return "", false
	}
}

func buildOpenAIStrictResponsesURL(baseURL, suffix string) (string, error) {
	target := buildOpenAIResponsesURL(baseURL)
	if suffix == "" {
		return target, nil
	}
	parsed, err := url.Parse(target)
	if err != nil {
		return "", fmt.Errorf("parse strict Responses upstream URL: %w", err)
	}
	parsed.Path = strings.TrimRight(parsed.Path, "/") + suffix
	return parsed.String(), nil
}

func isStrictResponsesRequestHeaderAllowed(key string) bool {
	lower := strings.ToLower(strings.TrimSpace(key))
	if lower == "" {
		return false
	}
	if isStrictResponsesCredentialOrTransportHeader(lower) {
		return false
	}
	if strings.HasPrefix(lower, "x-codex-") {
		return true
	}
	_, allowed := strictResponsesRequestHeaders[lower]
	return allowed
}

func isStrictResponsesCredentialOrTransportHeader(lower string) bool {
	if _, blocked := strictResponsesBlockedHeaders[lower]; blocked {
		return true
	}
	return strings.Contains(lower, "authorization") ||
		strings.Contains(lower, "api-key") ||
		strings.Contains(lower, "cookie") ||
		strings.Contains(lower, "secret") ||
		strings.HasSuffix(lower, "-token") ||
		strings.Contains(lower, "-token-")
}

func copyOpenAIStrictResponsesRequestHeaders(dst, src http.Header) {
	for key, values := range src {
		if !isStrictResponsesRequestHeaderAllowed(key) {
			continue
		}
		for _, value := range values {
			dst.Add(key, value)
		}
	}
}

func copyOpenAIStrictResponsesResponseHeaders(dst, src http.Header, filter *responseheaders.CompiledHeaderFilter) {
	responseheaders.WriteFilteredHeaders(dst, src, filter)
	for key, values := range src {
		lower := strings.ToLower(strings.TrimSpace(key))
		if isStrictResponsesCredentialOrTransportHeader(lower) {
			continue
		}
		if !strings.HasPrefix(lower, "x-codex-") && !strings.HasPrefix(lower, "openai-") {
			continue
		}
		dst.Del(key)
		for _, value := range values {
			dst.Add(key, value)
		}
	}
}

func (s *OpenAIGatewayService) buildOpenAIStrictResponsesRequest(
	ctx context.Context,
	c *gin.Context,
	account *Account,
	body []byte,
) (*http.Request, error) {
	if account == nil || !account.IsOpenAIStrictResponsesPassthroughEnabled() {
		return nil, errors.New("strict Responses API-key account is required")
	}
	suffix, ok := strictOpenAIResponsesPathSuffix(c)
	if !ok {
		return nil, errors.New("strict Responses forwarding only supports /v1/responses and /v1/responses/compact")
	}
	baseURL := account.GetOpenAIBaseURL()
	validatedURL, err := s.validateUpstreamBaseURL(baseURL)
	if err != nil {
		return nil, err
	}
	targetURL, err := buildOpenAIStrictResponsesURL(validatedURL, suffix)
	if err != nil {
		return nil, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, targetURL, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req = req.WithContext(WithHTTPUpstreamProfile(req.Context(), HTTPUpstreamProfileOpenAI))
	if c != nil && c.Request != nil {
		copyOpenAIStrictResponsesRequestHeaders(req.Header, c.Request.Header)
	}
	for blocked := range strictResponsesBlockedHeaders {
		req.Header.Del(blocked)
	}
	if !account.UsesNoOpenAIUpstreamAuth() {
		token, _, tokenErr := s.GetAccessToken(ctx, account)
		if tokenErr != nil {
			return nil, tokenErr
		}
		authHeaders, authErr := s.buildOpenAIAuthenticationHeaders(ctx, account, token)
		if authErr != nil {
			return nil, fmt.Errorf("build strict Responses authentication headers: %w", authErr)
		}
		for key, values := range authHeaders {
			for _, value := range values {
				req.Header.Add(key, value)
			}
		}
	}
	if _, exists := req.Header["User-Agent"]; !exists {
		// Suppress net/http's synthetic Go User-Agent. Strict mode only forwards
		// identity metadata that the authenticated downstream client supplied.
		req.Header["User-Agent"] = nil
	}
	if req.Header.Get("Content-Type") == "" {
		req.Header.Set("Content-Type", "application/json")
	}
	// Keep the observed response bytes and Content-Encoding in agreement. Go's
	// transport otherwise adds gzip automatically and transparently decodes it.
	req.Header.Set("Accept-Encoding", "identity")
	account.ApplyHeaderOverrides(req.Header)
	return req, nil
}

type openAIStrictResponsesObserver struct {
	service    *OpenAIGatewayService
	stream     bool
	lineBuffer []byte
	jsonBuffer []byte
	usage      OpenAIUsage
	responseID string
}

func newOpenAIStrictResponsesObserver(s *OpenAIGatewayService, stream bool) *openAIStrictResponsesObserver {
	return &openAIStrictResponsesObserver{service: s, stream: stream}
}

func (o *openAIStrictResponsesObserver) observe(chunk []byte) {
	if len(chunk) == 0 {
		return
	}
	if !o.stream {
		if len(o.jsonBuffer) < strictResponsesObservationLimit {
			remaining := strictResponsesObservationLimit - len(o.jsonBuffer)
			if remaining > len(chunk) {
				remaining = len(chunk)
			}
			o.jsonBuffer = append(o.jsonBuffer, chunk[:remaining]...)
		}
		return
	}
	o.lineBuffer = append(o.lineBuffer, chunk...)
	for {
		idx := bytes.IndexByte(o.lineBuffer, '\n')
		if idx < 0 {
			if len(o.lineBuffer) > strictResponsesObservationLimit {
				o.lineBuffer = o.lineBuffer[:0]
			}
			return
		}
		line := strings.TrimSuffix(string(o.lineBuffer[:idx]), "\r")
		o.lineBuffer = o.lineBuffer[idx+1:]
		data, ok := extractOpenAISSEDataLine(line)
		if !ok || strings.TrimSpace(data) == "[DONE]" {
			continue
		}
		payload := []byte(data)
		if o.service != nil {
			o.service.parseSSEUsageBytes(payload, &o.usage)
		}
		if o.responseID == "" {
			o.responseID = extractOpenAIResponseIDFromJSONBytes(payload)
		}
	}
}

func (o *openAIStrictResponsesObserver) finish() {
	if o == nil || o.stream || len(o.jsonBuffer) == 0 {
		return
	}
	if usage, ok := extractOpenAIUsageFromJSONBytes(o.jsonBuffer); ok {
		o.usage = usage
	}
	o.responseID = extractOpenAIResponseIDFromJSONBytes(o.jsonBuffer)
}

func (s *OpenAIGatewayService) forwardOpenAIStrictResponses(
	ctx context.Context,
	c *gin.Context,
	account *Account,
	body []byte,
	startTime time.Time,
) (*OpenAIForwardResult, error) {
	reqModel, reqStream, _ := extractOpenAIRequestMetaFromBody(body)
	request, err := s.buildOpenAIStrictResponsesRequest(ctx, c, account, body)
	if err != nil {
		return nil, err
	}
	SetActualOpenAIUpstreamEndpoint(c, request.URL.Path)
	proxyURL := ""
	if account.ProxyID != nil && account.Proxy != nil {
		proxyURL = account.Proxy.URL()
	}
	upstreamStart := time.Now()
	resp, err := s.httpUpstream.Do(request, proxyURL, account.ID, account.Concurrency)
	SetOpsLatencyMs(c, OpsUpstreamLatencyMsKey, time.Since(upstreamStart).Milliseconds())
	if err != nil {
		return nil, s.handleOpenAIUpstreamTransportError(ctx, c, account, err, true)
	}
	defer func() { _ = resp.Body.Close() }()

	copyOpenAIStrictResponsesResponseHeaders(c.Writer.Header(), resp.Header, s.responseHeaderFilter)
	isSSE := isEventStreamResponse(resp.Header)
	if isSSE {
		c.Header("Content-Type", "text/event-stream")
		c.Header("Cache-Control", "no-cache")
		c.Header("X-Accel-Buffering", "no")
	}
	c.Status(resp.StatusCode)

	observer := newOpenAIStrictResponsesObserver(s, isSSE)
	buffer := make([]byte, 32*1024)
	var firstTokenMs *int
	reasoningEffort := extractOpenAIReasoningEffortFromBody(body, reqModel)
	serviceTier := extractOpenAIServiceTierFromBody(body)
	resultWithObserver := func(clientDisconnected bool) *OpenAIForwardResult {
		return &OpenAIForwardResult{
			RequestID:        resp.Header.Get("x-request-id"),
			ResponseID:       observer.responseID,
			Usage:            observer.usage,
			Model:            reqModel,
			ServiceTier:      serviceTier,
			ReasoningEffort:  reasoningEffort,
			Stream:           reqStream,
			Duration:         time.Since(startTime),
			FirstTokenMs:     firstTokenMs,
			ClientDisconnect: clientDisconnected,
			ResponseHeaders:  cloneHeader(resp.Header),
			UpstreamEndpoint: request.URL.Path,
		}
	}
	for {
		n, readErr := resp.Body.Read(buffer)
		if n > 0 {
			chunk := buffer[:n]
			observer.observe(chunk)
			if firstTokenMs == nil {
				ms := int(time.Since(startTime).Milliseconds())
				firstTokenMs = &ms
			}
			written, writeErr := c.Writer.Write(chunk)
			if writeErr != nil || written != len(chunk) {
				if writeErr == nil {
					writeErr = io.ErrShortWrite
				}
				return resultWithObserver(true), fmt.Errorf("upstream response failed: write strict Responses downstream: %w", writeErr)
			}
			if isSSE {
				if flusher, ok := c.Writer.(http.Flusher); ok {
					flusher.Flush()
				}
			}
		}
		if readErr != nil {
			if errors.Is(readErr, io.EOF) {
				break
			}
			return resultWithObserver(false), fmt.Errorf("upstream response failed: read strict Responses upstream: %w", readErr)
		}
	}
	observer.finish()
	s.bindHTTPResponseAccount(ctx, c, account, observer.responseID)

	result := resultWithObserver(false)
	if resp.StatusCode >= http.StatusBadRequest {
		return result, &openAIStrictResponsesHTTPError{status: resp.StatusCode}
	}
	return result, nil
}
