package service

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
)

// ForwardAgnesVideo forwards Agnes' asynchronous video create and status
// requests through the normal Sub2API account/proxy path.  Agnes uses the
// OpenAI-compatible /v1/videos create endpoint but polls results from the
// provider-root /agnesapi endpoint, so this adapter intentionally handles both
// paths instead of treating the provider as a normal synchronous OpenAI video
// API.
func (s *OpenAIGatewayService) ForwardAgnesVideo(
	ctx context.Context,
	c *gin.Context,
	account *Account,
	model string,
	videoID string,
	body []byte,
	contentType string,
	channelMappedModel string,
) (*OpenAIForwardResult, error) {
	startTime := time.Now()
	if account == nil {
		return nil, fmt.Errorf("Agnes account is required")
	}
	if !account.IsOpenAI() {
		return nil, fmt.Errorf("account platform %s is not supported for Agnes video", account.Platform)
	}

	requestModel := strings.TrimSpace(model)
	if requestModel == "" {
		requestModel = strings.TrimSpace(channelMappedModel)
	}
	upstreamModel := requestModel
	if mapped := strings.TrimSpace(channelMappedModel); mapped != "" {
		upstreamModel = mapped
	}
	if accountMapped := strings.TrimSpace(account.GetMappedModel(upstreamModel)); accountMapped != "" {
		upstreamModel = accountMapped
	}

	createRequest := strings.TrimSpace(videoID) == ""
	targetURL, err := s.buildAgnesVideoURL(account, requestModel, videoID, createRequest)
	if err != nil {
		return nil, err
	}
	SetActualOpenAIUpstreamEndpoint(c, agnesVideoEndpointPath(createRequest))

	forwardBody := body
	if createRequest && len(body) > 0 && upstreamModel != requestModel {
		forwardBody, err = replaceJSONModel(body, upstreamModel)
		if err != nil {
			return nil, fmt.Errorf("rewrite Agnes video model: %w", err)
		}
	}

	upstreamCtx, releaseUpstreamCtx := detachUpstreamContext(ctx)
	defer releaseUpstreamCtx()
	var bodyReader io.Reader
	method := http.MethodGet
	if createRequest {
		method = http.MethodPost
		bodyReader = bytes.NewReader(forwardBody)
	}
	upstreamReq, err := http.NewRequestWithContext(upstreamCtx, method, targetURL, bodyReader)
	if err != nil {
		return nil, err
	}
	token, _, err := s.GetAccessToken(upstreamCtx, account)
	if err != nil {
		return nil, err
	}
	upstreamReq.Header.Set("Authorization", "Bearer "+token)
	upstreamReq.Header.Set("Accept", "application/json")
	if createRequest {
		if strings.TrimSpace(contentType) == "" {
			contentType = "application/json"
		}
		upstreamReq.Header.Set("Content-Type", contentType)
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

	respBody, err := ReadUpstreamResponseBody(resp.Body, s.cfg, c, openAITooLargeError)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode >= 400 {
		writeGrokMediaResponse(c, resp, respBody, s.responseHeaderFilter)
		return nil, fmt.Errorf("Agnes video upstream returned HTTP %d", resp.StatusCode)
	}
	respBody = normalizeAgnesVideoResponse(respBody, requestModel, videoID, createRequest)
	writeGrokMediaResponse(c, resp, respBody, s.responseHeaderFilter)
	requestID := firstNonEmpty(
		resp.Header.Get("x-request-id"),
		resp.Header.Get("request-id"),
		extractAgnesVideoResponseID(respBody),
	)
	requestInfo := ParseGrokMediaRequest(contentType, body)

	result := &OpenAIForwardResult{
		RequestID:            requestID,
		ResponseID:           requestID,
		Model:                requestModel,
		BillingModel:         requestModel,
		UpstreamModel:        upstreamModel,
		UpstreamEndpoint:     agnesVideoEndpointPath(createRequest),
		ResponseHeaders:      resp.Header.Clone(),
		Duration:             time.Since(startTime),
		ImageCount:           1,
		VideoCount:           1,
		VideoResolution:      requestInfo.Resolution,
		VideoDurationSeconds: requestInfo.DurationSeconds,
	}
	return result, nil
}

func (s *OpenAIGatewayService) buildAgnesVideoURL(account *Account, model, videoID string, create bool) (string, error) {
	baseURL := strings.TrimSpace(account.GetOpenAIBaseURL())
	if baseURL == "" {
		return "", fmt.Errorf("Agnes account base URL is empty")
	}
	validatedBaseURL, err := s.validateUpstreamBaseURL(baseURL)
	if err != nil {
		return "", err
	}
	if create {
		return buildOpenAIEndpointURL(validatedBaseURL, "/v1/videos"), nil
	}

	parsed, err := url.Parse(validatedBaseURL)
	if err != nil || parsed.Scheme == "" || parsed.Host == "" {
		return "", fmt.Errorf("invalid Agnes base URL")
	}
	path := strings.TrimRight(parsed.Path, "/")
	if openAIBaseURLHasVersionSuffix(path) {
		lastSlash := strings.LastIndex(path, "/")
		path = strings.TrimRight(path[:lastSlash], "/")
	}
	parsed.Path = path + "/agnesapi"
	parsed.RawPath = ""
	query := parsed.Query()
	query.Set("video_id", strings.TrimSpace(videoID))
	query.Set("model_name", strings.TrimSpace(model))
	parsed.RawQuery = query.Encode()
	parsed.Fragment = ""
	return parsed.String(), nil
}

func agnesVideoEndpointPath(create bool) string {
	if create {
		return "/v1/videos"
	}
	return "/agnesapi"
}

func replaceJSONModel(body []byte, model string) ([]byte, error) {
	var payload map[string]any
	if err := json.Unmarshal(body, &payload); err != nil {
		return nil, err
	}
	payload["model"] = model
	return json.Marshal(payload)
}

// normalizeAgnesVideoResponse keeps the provider response intact while adding
// the fields expected by infinite-canvas.  Agnes currently returns the same
// raw task identifier (usually beginning with "task_") in id, task_id and
// video_id.  That identifier must not be rewritten: the provider-root status
// endpoint rejects the synthetic "video_"-prefixed form.
func normalizeAgnesVideoResponse(body []byte, requestModel, knownVideoID string, create bool) []byte {
	var payload any
	if err := json.Unmarshal(body, &payload); err != nil {
		return body
	}
	if root, ok := payload.(map[string]any); ok {
		normalizeAgnesVideoObject(root, requestModel, knownVideoID, create)
		if data, ok := root["data"].(map[string]any); ok {
			normalizeAgnesVideoObject(data, requestModel, knownVideoID, create)
		}
		if result, ok := root["result"].(map[string]any); ok {
			normalizeAgnesVideoObject(result, requestModel, knownVideoID, create)
		}
	}
	normalized, err := json.Marshal(payload)
	if err != nil {
		return body
	}
	return normalized
}

func normalizeAgnesVideoObject(object map[string]any, requestModel, knownVideoID string, create bool) {
	if object == nil {
		return
	}
	videoID := strings.TrimSpace(knownVideoID)
	if videoID == "" {
		videoID = firstAgnesString(object, "video_id", "videoId", "videoID")
	}
	if videoID == "" && create {
		videoID = firstAgnesString(object, "id")
	}
	if videoID != "" {
		object["video_id"] = videoID
		if strings.TrimSpace(firstAgnesString(object, "id")) == "" || create {
			object["id"] = videoID
		}
	}
	if taskID := firstAgnesString(object, "task_id", "taskId", "taskID"); taskID != "" {
		object["task_id"] = taskID
	}
	if strings.TrimSpace(firstAgnesString(object, "model")) == "" && requestModel != "" {
		object["model"] = requestModel
	}
	if strings.TrimSpace(firstAgnesString(object, "status", "state")) == "" {
		if firstAgnesString(object, "video_url", "videoUrl", "url", "output_url", "download_url") != "" {
			object["status"] = "completed"
		} else if videoID != "" {
			object["status"] = "queued"
		}
	}
}

func firstAgnesString(object map[string]any, keys ...string) string {
	for _, key := range keys {
		if value, ok := object[key].(string); ok && strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}

func extractAgnesVideoResponseID(body []byte) string {
	var payload any
	if err := json.Unmarshal(body, &payload); err != nil {
		return ""
	}
	if root, ok := payload.(map[string]any); ok {
		if id := firstAgnesString(root, "id", "video_id", "videoId", "task_id", "taskId", "taskID"); id != "" {
			return id
		}
		for _, key := range []string{"data", "result"} {
			if nested, ok := root[key].(map[string]any); ok {
				if id := firstAgnesString(nested, "id", "video_id", "videoId", "task_id", "taskId", "taskID"); id != "" {
					return id
				}
			}
		}
	}
	return ""
}
