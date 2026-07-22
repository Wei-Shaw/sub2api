package service

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/tidwall/gjson"
)

// BailianVideoEndpoint identifies the two DashScope async video operations the
// gateway relays: task creation and task status lookup. Generated videos are
// delivered as signed OSS URLs inside the status response, so there is no
// content proxy endpoint.
type BailianVideoEndpoint string

const (
	BailianVideoEndpointGeneration BailianVideoEndpoint = "videos_generations"
	BailianVideoEndpointStatus     BailianVideoEndpoint = "video_status"
)

func (e BailianVideoEndpoint) IsGenerationRequest() bool {
	return e == BailianVideoEndpointGeneration
}

func (e BailianVideoEndpoint) IsVideoLookupRequest() bool {
	return e == BailianVideoEndpointStatus
}

func (e BailianVideoEndpoint) httpMethod() string {
	if e.IsVideoLookupRequest() {
		return http.MethodGet
	}
	return http.MethodPost
}

// DashScope defaults differ from xAI: an omitted duration generates 5 seconds.
// The normalized values are always written into the upstream request body so
// the billed amount and the generated amount cannot diverge.
const (
	bailianVideoDefaultDurationSeconds = 5
	bailianVideoDefaultResolution      = VideoBillingResolution720P
)

func normalizeBailianVideoDurationSeconds(durationSeconds int) int {
	if durationSeconds <= 0 {
		return bailianVideoDefaultDurationSeconds
	}
	if durationSeconds < VideoBillingMinDurationSeconds {
		return VideoBillingMinDurationSeconds
	}
	if durationSeconds > VideoBillingMaxDurationSeconds {
		return VideoBillingMaxDurationSeconds
	}
	return durationSeconds
}

func normalizeBailianVideoResolution(resolution string) string {
	if strings.TrimSpace(resolution) == "" {
		return bailianVideoDefaultResolution
	}
	return NormalizeVideoBillingResolutionOrDefault(resolution)
}

// BailianVideoRequestInfo is the parsed client request in the flat OpenAI
// style shared with the Grok video endpoints.
type BailianVideoRequestInfo struct {
	Model           string
	Prompt          string
	NegativePrompt  string
	ImageURL        string
	Ratio           string
	Resolution      string
	DurationSeconds int
	Seed            *int64
	Watermark       *bool
	N               int
}

func (r BailianVideoRequestInfo) HasInputImage() bool {
	return strings.TrimSpace(r.ImageURL) != ""
}

func (r BailianVideoRequestInfo) ModerationBody() []byte {
	payload := map[string]any{}
	if prompt := strings.TrimSpace(r.Prompt); prompt != "" {
		payload["prompt"] = prompt
	}
	if imageURL := strings.TrimSpace(r.ImageURL); imageURL != "" {
		payload["images"] = []map[string]string{{"image_url": imageURL}}
	}
	if len(payload) == 0 {
		return nil
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return nil
	}
	return body
}

func ExtractBailianVideoModel(body []byte) string {
	return strings.TrimSpace(gjson.GetBytes(body, "model").String())
}

// ParseBailianVideoRequest parses the flat JSON request body. Multipart is not
// accepted: DashScope video inputs are URLs or data URLs, never file uploads.
func ParseBailianVideoRequest(body []byte) BailianVideoRequestInfo {
	info := BailianVideoRequestInfo{N: 1}
	if !gjson.ValidBytes(body) {
		return info
	}
	info.Model = strings.TrimSpace(gjson.GetBytes(body, "model").String())
	info.Prompt = strings.TrimSpace(gjson.GetBytes(body, "prompt").String())
	info.NegativePrompt = strings.TrimSpace(gjson.GetBytes(body, "negative_prompt").String())
	info.Ratio = strings.TrimSpace(gjson.GetBytes(body, "ratio").String())
	info.Resolution = strings.TrimSpace(gjson.GetBytes(body, "resolution").String())
	if duration := gjson.GetBytes(body, "duration"); duration.Exists() && duration.Type == gjson.Number {
		info.DurationSeconds = int(duration.Int())
	}
	if n := gjson.GetBytes(body, "n"); n.Exists() && n.Type == gjson.Number {
		info.N = int(n.Int())
	}
	if seed := gjson.GetBytes(body, "seed"); seed.Exists() && seed.Type == gjson.Number {
		value := seed.Int()
		info.Seed = &value
	}
	if watermark := gjson.GetBytes(body, "watermark"); watermark.IsBool() {
		value := watermark.Bool()
		info.Watermark = &value
	}
	if image := gjson.GetBytes(body, "image"); image.Exists() {
		if imageURL := grokMediaJSONImageURL(image); imageURL != "" {
			info.ImageURL = imageURL
		} else if image.Type == gjson.String {
			info.ImageURL = strings.TrimSpace(image.String())
		}
	}
	info.Resolution = normalizeBailianVideoResolution(info.Resolution)
	info.DurationSeconds = normalizeBailianVideoDurationSeconds(info.DurationSeconds)
	if info.N <= 0 {
		info.N = 1
	}
	return info
}

// buildDashScopeVideoSynthesisBody converts the flat request into DashScope's
// nested {model, input, parameters} shape. The normalized duration and
// resolution are always written explicitly (see billing note above); unknown
// client fields are dropped by construction.
func buildDashScopeVideoSynthesisBody(info BailianVideoRequestInfo, upstreamModel string) ([]byte, error) {
	input := map[string]any{
		"prompt": info.Prompt,
	}
	if info.NegativePrompt != "" {
		input["negative_prompt"] = info.NegativePrompt
	}
	if imageURL := strings.TrimSpace(info.ImageURL); imageURL != "" {
		input["media"] = []map[string]string{{
			"type": "first_frame",
			"url":  imageURL,
		}}
	}
	parameters := map[string]any{
		"resolution": strings.ToUpper(info.Resolution),
		"duration":   info.DurationSeconds,
	}
	if info.Ratio != "" {
		parameters["ratio"] = info.Ratio
	}
	if info.Seed != nil {
		parameters["seed"] = *info.Seed
	}
	if info.Watermark != nil {
		parameters["watermark"] = *info.Watermark
	}
	return json.Marshal(map[string]any{
		"model":      upstreamModel,
		"input":      input,
		"parameters": parameters,
	})
}

// bailianVideoTaskBindingTTL matches the DashScope task/result validity window
// (24h). The OpenAIWS sticky-session TTL is deliberately not applied here: it
// models conversational affinity, not task ownership, and its default hour
// would 404 overnight polls.
const bailianVideoTaskBindingTTL = 24 * time.Hour

func BailianVideoTaskSessionHash(taskID string, userID, apiKeyID int64) string {
	taskID = strings.TrimSpace(taskID)
	if taskID == "" || userID <= 0 || apiKeyID <= 0 {
		return ""
	}
	ownerSeed := fmt.Sprintf("%d:%d:%s", userID, apiKeyID, taskID)
	return "bailian-video:" + DeriveSessionHashFromSeed(ownerSeed)
}

func (s *OpenAIGatewayService) BindBailianVideoTaskAccount(
	ctx context.Context,
	groupID *int64,
	taskID string,
	userID, apiKeyID, accountID int64,
) error {
	if s == nil || s.cache == nil {
		return fmt.Errorf("bailian video task binding cache is unavailable")
	}
	cacheKey := s.openAISessionCacheKey(BailianVideoTaskSessionHash(taskID, userID, apiKeyID))
	if cacheKey == "" || accountID <= 0 {
		return fmt.Errorf("bailian video task binding is invalid")
	}
	return s.cache.SetSessionAccountID(ctx, derefGroupID(groupID), cacheKey, accountID, bailianVideoTaskBindingTTL)
}

func (s *OpenAIGatewayService) ResolveBailianVideoTaskAccount(
	ctx context.Context,
	groupID *int64,
	taskID string,
	userID, apiKeyID int64,
) (int64, error) {
	if s == nil || s.cache == nil {
		return 0, fmt.Errorf("bailian video task binding cache is unavailable")
	}
	cacheKey := s.openAISessionCacheKey(BailianVideoTaskSessionHash(taskID, userID, apiKeyID))
	if cacheKey == "" {
		return 0, fmt.Errorf("bailian video task binding is invalid")
	}
	return s.cache.GetSessionAccountID(ctx, derefGroupID(groupID), cacheKey)
}

// ForwardBailianVideo relays a task creation or status lookup to DashScope.
// Status responses are passed through verbatim: output.video_url is a signed
// public OSS link the client downloads directly.
func (s *OpenAIGatewayService) ForwardBailianVideo(
	ctx context.Context,
	c *gin.Context,
	account *Account,
	endpoint BailianVideoEndpoint,
	taskID string,
	body []byte,
) (*OpenAIForwardResult, error) {
	startTime := time.Now()
	if account == nil {
		return nil, fmt.Errorf("bailian account is required")
	}
	if account.Platform != PlatformBailian {
		return nil, fmt.Errorf("account platform %s is not supported for bailian video", account.Platform)
	}

	token, _, err := s.getRequestCredential(ctx, c, account)
	if err != nil {
		return nil, err
	}

	var targetURL string
	requestInfo := BailianVideoRequestInfo{}
	upstreamModel := ""
	if endpoint.IsGenerationRequest() {
		targetURL, err = s.bailianVideoSynthesisURL(account)
		if err != nil {
			return nil, err
		}
		requestInfo = ParseBailianVideoRequest(body)
		upstreamModel = requestInfo.Model
		if mappedModel := strings.TrimSpace(account.GetMappedModel(requestInfo.Model)); mappedModel != "" {
			upstreamModel = mappedModel
		}
		body, err = buildDashScopeVideoSynthesisBody(requestInfo, upstreamModel)
		if err != nil {
			return nil, fmt.Errorf("build dashscope video synthesis body: %w", err)
		}
	} else {
		targetURL, err = s.bailianTaskURL(account, taskID)
		if err != nil {
			return nil, err
		}
	}

	var bodyReader io.Reader
	if endpoint.IsGenerationRequest() {
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
	if endpoint.IsGenerationRequest() {
		upstreamReq.Header.Set("Content-Type", "application/json")
		// DashScope rejects synchronous video synthesis calls.
		upstreamReq.Header.Set("X-DashScope-Async", "enable")
	}
	// 账号级请求头覆写最后应用，配置值优先于内置默认头。
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

	requestIDHeader := strings.TrimSpace(resp.Header.Get("x-request-id"))
	requestModel := requestInfo.Model
	if resp.StatusCode >= 400 {
		return s.handleBailianVideoErrorResponse(resp, c, account, requestIDHeader, endpoint)
	}

	respBody, err := ReadUpstreamResponseBody(resp.Body, s.cfg, c, openAITooLargeError)
	if err != nil {
		return nil, err
	}
	writeGrokMediaResponse(c, resp, respBody, s.responseHeaderFilter)

	result := &OpenAIForwardResult{
		RequestID:       firstNonEmpty(requestIDHeader, strings.TrimSpace(gjson.GetBytes(respBody, "request_id").String())),
		Model:           requestModel,
		BillingModel:    requestModel,
		UpstreamModel:   upstreamModel,
		ResponseHeaders: resp.Header.Clone(),
		Duration:        time.Since(startTime),
	}
	if endpoint.IsGenerationRequest() {
		result.ResponseID = strings.TrimSpace(gjson.GetBytes(respBody, "output.task_id").String())
		result.VideoCount = 1
		result.VideoResolution = requestInfo.Resolution
		result.VideoDurationSeconds = requestInfo.DurationSeconds
		// Legacy usage dashboards count media requests through ImageCount.
		result.ImageCount = 1
	}
	return result, nil
}

func extractBailianUpstreamErrorMessage(body []byte) string {
	for _, path := range []string{"output.message", "message", "error.message"} {
		if msg := strings.TrimSpace(gjson.GetBytes(body, path).String()); msg != "" {
			if code := strings.TrimSpace(gjson.GetBytes(body, strings.Replace(path, "message", "code", 1)).String()); code != "" {
				return fmt.Sprintf("%s: %s", code, msg)
			}
			return msg
		}
	}
	return ""
}

// shouldFailoverBailianUpstreamError reports whether another account may
// succeed: credential problems (401/403), rate limits (429) and upstream
// failures (5xx). Client errors (400: invalid parameter, content inspection
// rejection) are passed through instead.
func shouldFailoverBailianUpstreamError(statusCode int) bool {
	switch statusCode {
	case http.StatusUnauthorized, http.StatusForbidden, http.StatusTooManyRequests:
		return true
	}
	return statusCode >= 500
}

func (s *OpenAIGatewayService) handleBailianVideoErrorResponse(
	resp *http.Response,
	c *gin.Context,
	account *Account,
	requestIDHeader string,
	endpoint BailianVideoEndpoint,
) (*OpenAIForwardResult, error) {
	body := s.readUpstreamErrorBody(resp)
	upstreamMsg := sanitizeUpstreamErrorMessage(extractBailianUpstreamErrorMessage(body))
	if upstreamMsg == "" {
		upstreamMsg = fmt.Sprintf("DashScope upstream returned status %d", resp.StatusCode)
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
		writeGrokMediaErrorResponse(c, status, errType, errMsg)
		return nil, fmt.Errorf("upstream error: %d (passthrough rule matched) message=%s", resp.StatusCode, upstreamMsg)
	}

	if !account.ShouldHandleErrorCode(resp.StatusCode) {
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
		MarkResponseCommitted(c)
		writeGrokMediaErrorResponse(c, http.StatusInternalServerError, "upstream_error", "Upstream gateway error")
		return nil, fmt.Errorf("upstream error: %d (not in custom error codes) message=%s", resp.StatusCode, upstreamMsg)
	}

	// Lookup requests never fail over: they are pinned to the account that
	// created the task, so switching accounts cannot help.
	kind := "http_error"
	if endpoint.IsGenerationRequest() && shouldFailoverBailianUpstreamError(resp.StatusCode) {
		kind = "failover"
	}
	appendOpsUpstreamError(c, OpsUpstreamErrorEvent{
		Platform:           account.Platform,
		AccountID:          account.ID,
		AccountName:        account.Name,
		UpstreamStatusCode: resp.StatusCode,
		UpstreamRequestID:  requestIDHeader,
		Kind:               kind,
		Message:            upstreamMsg,
		Detail:             upstreamDetail,
	})
	if kind == "failover" {
		return nil, &UpstreamFailoverError{
			StatusCode:             resp.StatusCode,
			ResponseBody:           body,
			ResponseHeaders:        resp.Header.Clone(),
			RetryableOnSameAccount: account.IsPoolMode() && account.IsPoolModeRetryableStatus(resp.StatusCode),
		}
	}

	MarkResponseCommitted(c)
	writeGrokMediaErrorResponse(c, resp.StatusCode, grokMediaErrorType(resp.StatusCode), upstreamMsg)
	return nil, fmt.Errorf("upstream error: %d %s", resp.StatusCode, upstreamMsg)
}
