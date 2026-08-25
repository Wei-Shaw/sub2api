package service

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/tidwall/gjson"
)

// OpenAIVideoEndpoint describes one OpenAI Videos API operation.
type OpenAIVideoEndpoint string

const (
	OpenAIVideoCreate  OpenAIVideoEndpoint = "create"
	OpenAIVideoStatus  OpenAIVideoEndpoint = "status"
	OpenAIVideoContent OpenAIVideoEndpoint = "content"
)

// ForwardOpenAIVideo forwards a Videos API request through one already-selected
// OpenAI account. Account selection and task ownership belong to the handler;
// this method must never select a different account for a status request.
func (s *OpenAIGatewayService) ForwardOpenAIVideo(
	ctx context.Context,
	c *gin.Context,
	account *Account,
	endpoint OpenAIVideoEndpoint,
	requestID string,
	body []byte,
	contentType string,
) (*OpenAIForwardResult, error) {
	if account == nil || account.Platform != PlatformOpenAI {
		return nil, fmt.Errorf("openai account is required for videos")
	}
	token, _, err := s.getRequestCredential(ctx, c, account)
	if err != nil {
		return nil, err
	}
	path := "/v1/videos"
	method := http.MethodPost
	if endpoint == OpenAIVideoStatus || endpoint == OpenAIVideoContent {
		path += "/" + requestID
		method = http.MethodGet
	}
	if endpoint == OpenAIVideoContent {
		path += "/content"
	}
	targetURL := buildOpenAIEndpointURL(account.GetOpenAIFormatBaseURL(), path)
	var bodyReader io.Reader
	if method == http.MethodPost {
		bodyReader = strings.NewReader(string(body))
	}
	upstreamCtx, release := detachUpstreamContext(ctx)
	defer release()
	req, err := http.NewRequestWithContext(upstreamCtx, method, targetURL, bodyReader)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+token)
	if method == http.MethodPost {
		if contentType == "" {
			contentType = "application/json"
		}
		req.Header.Set("Content-Type", contentType)
	}
	if endpoint == OpenAIVideoContent {
		req.Header.Set("Accept", "video/*,application/octet-stream;q=0.9,*/*;q=0.8")
	} else {
		req.Header.Set("Accept", "application/json")
	}
	account.ApplyHeaderOverrides(req.Header)
	proxyURL := ""
	if account.ProxyID != nil && account.Proxy != nil {
		proxyURL = account.Proxy.URL()
	}
	started := time.Now()
	resp, err := s.httpUpstream.Do(req, proxyURL, account.ID, account.Concurrency)
	SetOpsLatencyMs(c, OpsUpstreamLatencyMsKey, time.Since(started).Milliseconds())
	if err != nil {
		return nil, s.handleOpenAIUpstreamTransportError(ctx, c, account, err, false)
	}
	defer resp.Body.Close()
	responseID := firstNonEmpty(resp.Header.Get("x-request-id"), resp.Header.Get("openai-request-id"))
	if endpoint == OpenAIVideoContent {
		if resp.StatusCode >= 400 {
			return s.forwardOpenAIVideoJSONError(c, resp, responseID)
		}
		if err := writeGrokMediaContentResponse(c, resp); err != nil {
			return nil, err
		}
		return &OpenAIForwardResult{RequestID: responseID, ResponseHeaders: resp.Header.Clone(), Duration: time.Since(started)}, nil
	}
	respBody, err := ReadUpstreamResponseBody(resp.Body, s.cfg, c, openAITooLargeError)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode >= 400 {
		return s.forwardOpenAIVideoBodyError(c, resp, respBody, responseID)
	}
	writeOpenAIPassthroughResponseHeaders(c.Writer.Header(), resp.Header, s.responseHeaderFilter)
	contentType = strings.TrimSpace(resp.Header.Get("Content-Type"))
	if contentType == "" {
		contentType = "application/json"
	}
	c.Data(resp.StatusCode, contentType, respBody)
	return parseOpenAIVideoResult(respBody, responseID, time.Since(started)), nil
}

func (s *OpenAIGatewayService) forwardOpenAIVideoJSONError(c *gin.Context, resp *http.Response, requestID string) (*OpenAIForwardResult, error) {
	body, err := ReadUpstreamResponseBody(resp.Body, s.cfg, c, openAITooLargeError)
	if err != nil {
		return nil, err
	}
	return s.forwardOpenAIVideoBodyError(c, resp, body, requestID)
}

func (s *OpenAIGatewayService) forwardOpenAIVideoBodyError(c *gin.Context, resp *http.Response, body []byte, requestID string) (*OpenAIForwardResult, error) {
	writeOpenAIPassthroughResponseHeaders(c.Writer.Header(), resp.Header, s.responseHeaderFilter)
	// Do not write the body here. The handler owns the response lifecycle and
	// maps UpstreamFailoverError to one client-facing JSON response. Writing the
	// upstream body here would make the handler append a second JSON object.
	return nil, &UpstreamFailoverError{
		StatusCode:      resp.StatusCode,
		ResponseBody:    body,
		ResponseHeaders: resp.Header.Clone(),
	}
}

func parseOpenAIVideoResult(body []byte, requestID string, duration time.Duration) *OpenAIForwardResult {
	result := &OpenAIForwardResult{
		RequestID:    requestID,
		ResponseID:   strings.TrimSpace(gjson.GetBytes(body, "id").String()),
		Model:        strings.TrimSpace(gjson.GetBytes(body, "model").String()),
		BillingModel: strings.TrimSpace(gjson.GetBytes(body, "model").String()),
		Duration:     duration,
	}
	result.VideoResolution = openAIVideoBillingResolution(gjson.GetBytes(body, "size").String())
	seconds := strings.TrimSpace(gjson.GetBytes(body, "seconds").String())
	if parsed, err := strconv.Atoi(seconds); err == nil {
		result.VideoDurationSeconds = parsed
	}
	status := strings.ToLower(strings.TrimSpace(gjson.GetBytes(body, "status").String()))
	if status == "completed" {
		result.VideoCount = 1
	}
	return result
}

func openAIVideoBillingResolution(size string) string {
	size = strings.ToLower(strings.TrimSpace(size))
	if strings.Contains(size, "1024") || strings.Contains(size, "1792") {
		return VideoBillingResolution1080P
	}
	if strings.Contains(size, "720") || strings.Contains(size, "1280") {
		return VideoBillingResolution720P
	}
	return ""
}
