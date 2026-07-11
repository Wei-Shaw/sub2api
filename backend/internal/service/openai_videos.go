package service

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/util/responseheaders"
	"github.com/gin-gonic/gin"
	"github.com/tidwall/gjson"
)

type OpenAIVideoEndpoint string

const (
	OpenAIVideoEndpointGenerations OpenAIVideoEndpoint = "generations"
	OpenAIVideoEndpointStatus      OpenAIVideoEndpoint = "status"
)

func (e OpenAIVideoEndpoint) RequiresRequestBody() bool {
	return e == OpenAIVideoEndpointGenerations
}

func (e OpenAIVideoEndpoint) IsGenerationRequest() bool {
	return e == OpenAIVideoEndpointGenerations
}

func (e OpenAIVideoEndpoint) httpMethod() string {
	if e == OpenAIVideoEndpointStatus {
		return http.MethodGet
	}
	return http.MethodPost
}

type OpenAIVideoRequestInfo struct {
	Model           string
	Prompt          string
	Resolution      string
	DurationSeconds int
}

func ParseOpenAIVideoRequest(body []byte) OpenAIVideoRequestInfo {
	info := OpenAIVideoRequestInfo{}
	if len(body) == 0 || !gjson.ValidBytes(body) {
		return info
	}
	info.Model = strings.TrimSpace(gjson.GetBytes(body, "model").String())
	info.Prompt = strings.TrimSpace(gjson.GetBytes(body, "prompt").String())
	info.Resolution = strings.TrimSpace(firstNonEmpty(
		gjson.GetBytes(body, "resolution").String(),
		gjson.GetBytes(body, "size").String(),
	))
	for _, path := range []string{"duration", "seconds", "duration_seconds"} {
		if value := gjson.GetBytes(body, path); value.Exists() && value.Type == gjson.Number {
			info.DurationSeconds = int(value.Int())
			break
		}
	}
	info.Resolution = NormalizeVideoBillingResolutionOrDefault(info.Resolution)
	info.DurationSeconds = NormalizeVideoBillingDurationSecondsOrDefault(info.DurationSeconds)
	return info
}

func (s *OpenAIGatewayService) BindOpenAIVideoRequestAccount(ctx context.Context, groupID *int64, requestID string, accountID int64) error {
	return s.BindStickySession(ctx, groupID, OpenAIVideoRequestSessionHash(requestID), accountID)
}

func OpenAIVideoRequestSessionHash(requestID string) string {
	requestID = strings.TrimSpace(requestID)
	if requestID == "" {
		return ""
	}
	return "openai-video:" + DeriveSessionHashFromSeed(requestID)
}

func (s *OpenAIGatewayService) ForwardOpenAIVideo(
	ctx context.Context,
	c *gin.Context,
	account *Account,
	endpoint OpenAIVideoEndpoint,
	requestID string,
	body []byte,
	contentType string,
) (*OpenAIForwardResult, error) {
	startTime := time.Now()
	if account == nil {
		return nil, fmt.Errorf("openai account is required")
	}
	if account.Platform != PlatformOpenAI {
		return nil, fmt.Errorf("account platform %s is not supported for openai videos", account.Platform)
	}
	if account.Type != AccountTypeAPIKey {
		return nil, fmt.Errorf("openai videos require an API key account")
	}

	requestInfo := ParseOpenAIVideoRequest(body)
	requestModel := requestInfo.Model
	if mapped := strings.TrimSpace(account.GetMappedModel(requestModel)); mapped != "" {
		requestModel = mapped
	}

	token, _, err := s.GetAccessToken(ctx, account)
	if err != nil {
		return nil, err
	}
	inboundPath := ""
	if c != nil && c.Request != nil && c.Request.URL != nil {
		inboundPath = c.Request.URL.Path
	}
	targetURL, err := openAIVideoTargetURL(account.GetOpenAIBaseURL(), endpoint, requestID, inboundPath)
	if err != nil {
		return nil, err
	}

	var bodyReader io.Reader
	if endpoint.RequiresRequestBody() {
		bodyReader = bytes.NewReader(body)
	}
	upstreamCtx, releaseUpstreamCtx := detachUpstreamContext(ctx)
	defer releaseUpstreamCtx()
	upstreamReq, err := http.NewRequestWithContext(upstreamCtx, endpoint.httpMethod(), targetURL, bodyReader)
	if err != nil {
		return nil, err
	}
	upstreamReq.Header.Set("Authorization", "Bearer "+token)
	upstreamReq.Header.Set("Accept", "application/json")
	if endpoint.RequiresRequestBody() {
		contentType = strings.TrimSpace(contentType)
		if contentType == "" {
			contentType = "application/json"
		}
		upstreamReq.Header.Set("Content-Type", contentType)
	}
	if customUA := strings.TrimSpace(account.GetOpenAIUserAgent()); customUA != "" {
		upstreamReq.Header.Set("User-Agent", customUA)
	}
	account.ApplyHeaderOverrides(upstreamReq.Header)

	proxyURL := ""
	if account.ProxyID != nil && account.Proxy != nil {
		proxyURL = account.Proxy.URL()
	}
	upstreamStart := time.Now()
	resp, err := s.httpUpstream.Do(upstreamReq, proxyURL, account.ID, account.Concurrency)
	SetOpsLatencyMs(c, OpsUpstreamLatencyMsKey, time.Since(upstreamStart).Milliseconds())
	if err != nil {
		return nil, s.handleOpenAIUpstreamTransportError(ctx, c, account, err, false)
	}
	defer func() { _ = resp.Body.Close() }()

	requestIDHeader := resp.Header.Get("x-request-id")
	if resp.StatusCode >= 400 {
		return s.handleOpenAIVideoErrorResponse(ctx, resp, c, account, requestIDHeader, requestModel)
	}

	respBody, err := ReadUpstreamResponseBody(resp.Body, s.cfg, c, openAITooLargeError)
	if err != nil {
		return nil, err
	}
	writeOpenAIVideoResponse(c, resp, respBody, s.responseHeaderFilter)
	usage := openAIVideoUsageFromResponse(endpoint, requestInfo, respBody)
	return &OpenAIForwardResult{
		RequestID:            requestIDHeader,
		ResponseID:           usage.ResponseID,
		Usage:                usage.Usage,
		Model:                requestInfo.Model,
		BillingModel:         requestModel,
		UpstreamModel:        requestModel,
		ResponseHeaders:      resp.Header.Clone(),
		Duration:             time.Since(startTime),
		VideoCount:           usage.VideoCount,
		VideoResolution:      usage.VideoResolution,
		VideoDurationSeconds: usage.VideoDurationSeconds,
	}, nil
}

func openAIVideoTargetURL(base string, endpoint OpenAIVideoEndpoint, requestID string, inboundPath string) (string, error) {
	base = strings.TrimRight(strings.TrimSpace(base), "/")
	if base == "" {
		base = "https://api.openai.com"
	}
	switch endpoint {
	case OpenAIVideoEndpointGenerations:
		if strings.Contains(inboundPath, "/videos/generations") {
			return base + "/v1/videos/generations", nil
		}
		return base + "/v1/videos", nil
	case OpenAIVideoEndpointStatus:
		requestID = strings.TrimSpace(requestID)
		if requestID == "" {
			return "", fmt.Errorf("request_id is required")
		}
		return base + "/v1/videos/" + url.PathEscape(requestID), nil
	default:
		return "", fmt.Errorf("unsupported openai video endpoint: %s", endpoint)
	}
}

type openAIVideoUsageMetadata struct {
	ResponseID           string
	Usage                OpenAIUsage
	VideoCount           int
	VideoResolution      string
	VideoDurationSeconds int
}

func openAIVideoUsageFromResponse(endpoint OpenAIVideoEndpoint, requestInfo OpenAIVideoRequestInfo, responseBody []byte) openAIVideoUsageMetadata {
	usage, _ := extractOpenAIUsageFromJSONBytes(responseBody)
	meta := openAIVideoUsageMetadata{Usage: usage}
	if endpoint == OpenAIVideoEndpointGenerations {
		meta.ResponseID = extractOpenAIVideoRequestID(responseBody)
		meta.VideoCount = 1
		meta.VideoResolution = requestInfo.Resolution
		meta.VideoDurationSeconds = requestInfo.DurationSeconds
	}
	return meta
}

func extractOpenAIVideoRequestID(body []byte) string {
	if len(body) == 0 || !gjson.ValidBytes(body) {
		return ""
	}
	for _, path := range []string{"id", "request_id", "data.id", "data.request_id", "video.id", "video.request_id"} {
		if id := strings.TrimSpace(gjson.GetBytes(body, path).String()); id != "" {
			return id
		}
	}
	return ""
}

func (s *OpenAIGatewayService) handleOpenAIVideoErrorResponse(
	ctx context.Context,
	resp *http.Response,
	c *gin.Context,
	account *Account,
	requestIDHeader string,
	requestedModel string,
) (*OpenAIForwardResult, error) {
	body := s.readUpstreamErrorBody(resp)
	upstreamMsg := sanitizeUpstreamErrorMessage(strings.TrimSpace(extractUpstreamErrorMessage(body)))
	if upstreamMsg == "" {
		upstreamMsg = fmt.Sprintf("OpenAI video upstream returned status %d", resp.StatusCode)
	}

	upstreamDetail := ""
	if s.cfg != nil && s.cfg.Gateway.LogUpstreamErrorBody {
		maxBytes := s.cfg.Gateway.LogUpstreamErrorBodyMaxBytes
		if maxBytes <= 0 {
			maxBytes = 2048
		}
		upstreamDetail = truncateString(string(body), maxBytes)
	}
	setOpsUpstreamError(c, resp.StatusCode, upstreamMsg, upstreamDetail)

	if status, errType, errMsg, matched := applyErrorPassthroughRule(
		c,
		account.Platform,
		resp.StatusCode,
		body,
		http.StatusBadGateway,
		"upstream_error",
		"Upstream request failed",
	); matched {
		MarkResponseCommitted(c)
		writeOpenAIVideoErrorResponse(c, status, errType, errMsg)
		return nil, fmt.Errorf("upstream error: %d (passthrough rule matched) message=%s", resp.StatusCode, upstreamMsg)
	}

	appendOpsUpstreamError(c, OpsUpstreamErrorEvent{
		Platform:           account.Platform,
		AccountID:          account.ID,
		AccountName:        account.Name,
		UpstreamStatusCode: resp.StatusCode,
		UpstreamRequestID:  requestIDHeader,
		Kind:               "http_error",
		Message:            upstreamMsg,
		Detail:             upstreamDetail,
	})

	if s.shouldFailoverUpstreamError(resp.StatusCode) && account.ShouldHandleErrorCode(resp.StatusCode) {
		s.handleOpenAIAccountUpstreamError(ctx, account, resp.StatusCode, resp.Header, body, requestedModel)
		return nil, &UpstreamFailoverError{
			StatusCode:             resp.StatusCode,
			ResponseBody:           body,
			ResponseHeaders:        resp.Header.Clone(),
			RetryableOnSameAccount: account.IsPoolMode() && account.IsPoolModeRetryableStatus(resp.StatusCode),
		}
	}

	MarkResponseCommitted(c)
	writeOpenAIVideoErrorResponse(c, resp.StatusCode, "upstream_error", upstreamMsg)
	return nil, fmt.Errorf("upstream error: %d message=%s", resp.StatusCode, upstreamMsg)
}

func writeOpenAIVideoErrorResponse(c *gin.Context, statusCode int, errType, message string) {
	if c == nil || c.Writer == nil || c.Writer.Written() {
		return
	}
	c.JSON(statusCode, gin.H{
		"error": gin.H{
			"type":    strings.TrimSpace(errType),
			"message": strings.TrimSpace(message),
		},
	})
}

func writeOpenAIVideoResponse(c *gin.Context, resp *http.Response, body []byte, filter *responseheaders.CompiledHeaderFilter) {
	if c == nil || resp == nil {
		return
	}
	writeOpenAIPassthroughResponseHeaders(c.Writer.Header(), resp.Header, filter)
	contentType := strings.TrimSpace(resp.Header.Get("Content-Type"))
	if contentType == "" {
		contentType = "application/json"
	}
	c.Data(resp.StatusCode, contentType, body)
}
