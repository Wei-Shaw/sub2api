package service

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/tidwall/gjson"
)

const (
	imageAsyncTaskRelayEnabledKey  = "async_image_task_relay_enabled"
	imageAsyncTaskStatusURLKey     = "async_image_task_status_url"
	imageAsyncTaskPollIntervalKey  = "async_image_task_poll_interval_seconds"
	imageAsyncTaskUpstreamAsyncKey = "async_image_task_upstream_async_enabled"
	defaultImageTaskPollInterval   = 3 * time.Second
	defaultImageTaskStatusPath     = "/v1/image-tasks/{task_id}"
)

type asyncImageTaskContextKey struct{}

// WithAsyncImageTask marks a request executed by the local asynchronous image
// task worker. The marker prevents the upstream task relay from changing the
// normal synchronous images API behavior.
func WithAsyncImageTask(ctx context.Context) context.Context {
	if ctx == nil {
		ctx = context.Background()
	}
	return context.WithValue(ctx, asyncImageTaskContextKey{}, true)
}

func IsAsyncImageTask(ctx context.Context) bool {
	if ctx == nil {
		return false
	}
	marked, _ := ctx.Value(asyncImageTaskContextKey{}).(bool)
	return marked
}

// AsyncImageTaskRelayEnabled returns the account-level opt-in for upstream
// providers that return a task ID instead of an image immediately.
func (a *Account) AsyncImageTaskRelayEnabled() bool {
	if a == nil {
		return false
	}
	return accountBoolSetting(a, imageAsyncTaskRelayEnabledKey)
}

func (a *Account) AsyncImageTaskStatusURL() string {
	if a == nil {
		return ""
	}
	return strings.TrimSpace(accountStringSetting(a, imageAsyncTaskStatusURLKey))
}

// AsyncImageTaskUpstreamAsyncEnabled controls whether an opted-in JSON image
// request asks the upstream relay to create its own asynchronous task. Missing
// configuration defaults to true for OpenAI-compatible providers.
func (a *Account) AsyncImageTaskUpstreamAsyncEnabled() bool {
	return accountBoolSettingWithDefault(a, imageAsyncTaskUpstreamAsyncKey, true)
}

func (a *Account) AsyncImageTaskPollInterval() time.Duration {
	if a == nil {
		return defaultImageTaskPollInterval
	}
	seconds := accountIntSetting(a, imageAsyncTaskPollIntervalKey)
	if seconds < 1 {
		return defaultImageTaskPollInterval
	}
	if seconds > 60 {
		seconds = 60
	}
	return time.Duration(seconds) * time.Second
}

func accountBoolSetting(a *Account, key string) bool {
	return accountBoolSettingWithDefault(a, key, false)
}

func accountBoolSettingWithDefault(a *Account, key string, fallback bool) bool {
	if a == nil || a.Extra == nil {
		return fallback
	}
	if value, ok := a.Extra[key].(bool); ok {
		return value
	}
	if nested, ok := a.Extra[a.Platform].(map[string]any); ok {
		if value, ok := nested[key].(bool); ok {
			return value
		}
	}
	return fallback
}

func accountStringSetting(a *Account, key string) string {
	if a == nil || a.Extra == nil {
		return ""
	}
	if value, ok := a.Extra[key].(string); ok {
		return value
	}
	if nested, ok := a.Extra[a.Platform].(map[string]any); ok {
		value, _ := nested[key].(string)
		return value
	}
	return ""
}

func accountIntSetting(a *Account, key string) int {
	if a == nil || a.Extra == nil {
		return 0
	}
	value, ok := a.Extra[key]
	if !ok {
		if nested, nestedOK := a.Extra[a.Platform].(map[string]any); nestedOK {
			value, ok = nested[key]
		}
	}
	if !ok {
		return 0
	}
	switch typed := value.(type) {
	case int:
		return typed
	case int8:
		return int(typed)
	case int16:
		return int(typed)
	case int32:
		return int(typed)
	case int64:
		return int(typed)
	case float32:
		return int(typed)
	case float64:
		return int(typed)
	case string:
		parsed, err := strconv.Atoi(strings.TrimSpace(typed))
		if err == nil {
			return parsed
		}
	}
	return 0
}

type imageTaskRelayResult struct {
	body           []byte
	responseHeader http.Header
	relayed        bool
}

// pollImageTaskRelay handles the small protocol shared by most OpenAI-style
// image relays: the create response contains task_id (or id), and a later GET
// returns either a processing state or an image result.
func (s *OpenAIGatewayService) pollImageTaskRelay(
	ctx context.Context,
	c *gin.Context,
	account *Account,
	baseURL string,
	requestHeaders http.Header,
	initialBody []byte,
) (*imageTaskRelayResult, error) {
	if s == nil || s.httpUpstream == nil {
		return nil, fmt.Errorf("upstream image task relay is unavailable")
	}
	if result, ok := normalizeImageTaskRelayResult(initialBody); ok {
		return &imageTaskRelayResult{body: result, relayed: true}, nil
	}
	taskID, found := imageTaskRelayID(initialBody)
	if !found {
		return &imageTaskRelayResult{body: initialBody}, nil
	}

	statusPath := account.AsyncImageTaskStatusURL()
	if statusPath == "" {
		statusPath = imageTaskRelayStatusURL(initialBody)
	}
	statusURL, err := buildImageTaskRelayStatusURL(baseURL, statusPath, taskID)
	if err != nil {
		return nil, err
	}
	requestHeaders = requestHeaders.Clone()
	requestHeaders.Set("Accept", "application/json")
	requestHeaders.Del("Content-Type")
	requestHeaders.Del("Content-Length")

	for {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-time.After(account.AsyncImageTaskPollInterval()):
		}

		pollReq, err := http.NewRequestWithContext(
			WithHTTPUpstreamRedirectsDisabled(ctx),
			http.MethodGet,
			statusURL,
			nil,
		)
		if err != nil {
			return nil, fmt.Errorf("build upstream image task status request: %w", err)
		}
		pollReq = pollReq.WithContext(WithHTTPUpstreamProfile(pollReq.Context(), HTTPUpstreamProfileOpenAI))
		pollReq.Header = requestHeaders.Clone()

		proxyURL := ""
		if account.ProxyID != nil && account.Proxy != nil {
			proxyURL = account.Proxy.URL()
		}
		resp, err := s.httpUpstream.Do(pollReq, proxyURL, account.ID, account.Concurrency)
		if err != nil {
			return nil, fmt.Errorf("poll upstream image task: %w", err)
		}
		body, readErr := ReadUpstreamResponseBody(resp.Body, s.cfg, c, openAITooLargeError)
		_ = resp.Body.Close()
		if readErr != nil {
			return nil, readErr
		}
		if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
			return nil, fmt.Errorf("upstream image task status returned HTTP %d: %s", resp.StatusCode, truncateString(string(body), 512))
		}

		status := imageTaskRelayStatus(body)
		if imageTaskRelayFailed(status) {
			return nil, fmt.Errorf("upstream image task failed: %s", imageTaskRelayErrorMessage(body))
		}
		if result, ok := normalizeImageTaskRelayResult(body); ok {
			return &imageTaskRelayResult{
				body:           result,
				responseHeader: resp.Header.Clone(),
				relayed:        true,
			}, nil
		}
		if status != "" && !imageTaskRelayPending(status) {
			return nil, fmt.Errorf("upstream image task returned unsupported status %q", status)
		}
	}
}

func imageTaskRelayID(body []byte) (string, bool) {
	if len(body) == 0 || !gjson.ValidBytes(body) || imageTaskRelayHasImage(body) {
		return "", false
	}
	for _, path := range []string{
		"task_id", "taskId", "data.task_id", "data.taskId", "data.id",
		"result.task_id", "result.taskId", "result.id",
	} {
		if value := strings.TrimSpace(gjson.GetBytes(body, path).String()); value != "" {
			return value, true
		}
	}
	status := imageTaskRelayStatus(body)
	if status == "" || imageTaskRelayPending(status) {
		if value := strings.TrimSpace(gjson.GetBytes(body, "id").String()); value != "" {
			return value, true
		}
	}
	return "", false
}

func imageTaskRelayStatusURL(body []byte) string {
	if !gjson.ValidBytes(body) {
		return ""
	}
	for _, path := range []string{
		"poll_url", "status_url", "data.poll_url", "data.status_url",
		"result.poll_url", "result.status_url",
	} {
		if value := strings.TrimSpace(gjson.GetBytes(body, path).String()); value != "" {
			return value
		}
	}
	return ""
}

func imageTaskRelayStatus(body []byte) string {
	for _, path := range []string{"status", "state", "data.status", "result.status", "result.state", "output.status"} {
		if value := strings.ToLower(strings.TrimSpace(gjson.GetBytes(body, path).String())); value != "" {
			return value
		}
	}
	return ""
}

func imageTaskRelayPending(status string) bool {
	switch strings.ToLower(strings.TrimSpace(status)) {
	case "", "created", "accepted", "submitted", "scheduled", "queued", "in_queue", "in-queue", "pending", "waiting", "processing", "running", "started", "generating", "in_progress", "in-progress", "not_started":
		return true
	default:
		return false
	}
}

func imageTaskRelayFailed(status string) bool {
	switch strings.ToLower(strings.TrimSpace(status)) {
	case "failed", "failure", "error", "cancelled", "canceled", "expired":
		return true
	default:
		return false
	}
}

func imageTaskRelayHasImage(body []byte) bool {
	if !gjson.ValidBytes(body) {
		return false
	}
	if strings.HasPrefix(strings.TrimSpace(string(body)), "[") && imageTaskRelayArrayHasImage(body, "") {
		return true
	}
	for _, path := range []string{"data", "result.data", "output", "result.output"} {
		if imageTaskRelayArrayHasImage(body, path) {
			return true
		}
	}
	for _, path := range []string{"url", "image_url", "result.url", "result.image_url", "output.url", "output.image_url"} {
		if strings.TrimSpace(gjson.GetBytes(body, path).String()) != "" {
			return true
		}
	}
	return false
}

func normalizeImageTaskRelayResult(body []byte) ([]byte, bool) {
	if !gjson.ValidBytes(body) {
		return nil, false
	}
	if imageTaskRelayArrayHasImage(body, "data") {
		return stripImageTaskRelayMetadata(normalizeImageTaskRelayImageFields(body)), true
	}
	for _, path := range []string{"result", "output", "response"} {
		raw := gjson.GetBytes(body, path)
		if raw.Exists() && imageTaskRelayHasImage([]byte(raw.Raw)) {
			if imageTaskRelayArrayHasImage([]byte(raw.Raw), "") {
				return wrapImageTaskArray(normalizeImageTaskRelayImageFields([]byte(raw.Raw))), true
			}
			if imageTaskRelayArrayHasImage([]byte(raw.Raw), "data") {
				return stripImageTaskRelayMetadata(normalizeImageTaskRelayImageFields([]byte(raw.Raw))), true
			}
			if urlValue := strings.TrimSpace(gjson.GetBytes([]byte(raw.Raw), "url").String()); urlValue != "" {
				return wrapImageTaskURL(urlValue), true
			}
			if urlValue := strings.TrimSpace(gjson.GetBytes([]byte(raw.Raw), "image_url").String()); urlValue != "" {
				return wrapImageTaskURL(urlValue), true
			}
		}
	}
	for _, path := range []string{"url", "image_url"} {
		if urlValue := strings.TrimSpace(gjson.GetBytes(body, path).String()); urlValue != "" {
			return wrapImageTaskURL(urlValue), true
		}
	}
	return nil, false
}

func imageTaskRelayArrayHasImage(body []byte, path string) bool {
	value := gjson.GetBytes(body, path)
	if path == "" {
		value = gjson.ParseBytes(body)
	}
	for _, item := range value.Array() {
		if strings.TrimSpace(item.Get("url").String()) != "" ||
			strings.TrimSpace(item.Get("image_url").String()) != "" ||
			strings.TrimSpace(item.Get("b64_json").String()) != "" {
			return true
		}
	}
	return false
}

func normalizeImageTaskRelayImageFields(body []byte) []byte {
	var envelope map[string]json.RawMessage
	if json.Unmarshal(body, &envelope) == nil {
		if rawData, ok := envelope["data"]; ok {
			var items []map[string]json.RawMessage
			if json.Unmarshal(rawData, &items) == nil {
				for _, item := range items {
					if _, hasURL := item["url"]; hasURL {
						continue
					}
					if imageURL, ok := item["image_url"]; ok {
						item["url"] = imageURL
						delete(item, "image_url")
					}
				}
				if normalized, err := json.Marshal(items); err == nil {
					envelope["data"] = normalized
					if normalizedBody, err := json.Marshal(envelope); err == nil {
						return normalizedBody
					}
				}
			}
		}
	}

	var items []map[string]json.RawMessage
	if json.Unmarshal(body, &items) != nil {
		return body
	}
	for _, item := range items {
		if _, hasURL := item["url"]; hasURL {
			continue
		}
		if imageURL, ok := item["image_url"]; ok {
			item["url"] = imageURL
			delete(item, "image_url")
		}
	}
	normalized, err := json.Marshal(items)
	if err != nil {
		return body
	}
	return normalized
}

func stripImageTaskRelayMetadata(body []byte) []byte {
	var envelope map[string]json.RawMessage
	if json.Unmarshal(body, &envelope) != nil {
		return body
	}
	for _, key := range []string{"id", "task_id", "taskId", "status", "state", "poll_url", "status_url"} {
		delete(envelope, key)
	}
	cleaned, err := json.Marshal(envelope)
	if err != nil {
		return body
	}
	return cleaned
}

func wrapImageTaskURL(imageURL string) []byte {
	body, _ := json.Marshal(map[string]any{
		"created": time.Now().Unix(),
		"data":    []map[string]string{{"url": imageURL}},
	})
	return body
}

func wrapImageTaskArray(items []byte) []byte {
	body, _ := json.Marshal(map[string]any{
		"created": time.Now().Unix(),
		"data":    json.RawMessage(items),
	})
	return body
}

func imageTaskRelayErrorMessage(body []byte) string {
	for _, path := range []string{"error.message", "message", "result.error.message"} {
		if value := strings.TrimSpace(gjson.GetBytes(body, path).String()); value != "" {
			return value
		}
	}
	return "unknown upstream image task error"
}

func buildImageTaskRelayStatusURL(baseURL, template, taskID string) (string, error) {
	taskID = url.PathEscape(strings.TrimSpace(taskID))
	if taskID == "" {
		return "", fmt.Errorf("upstream image task id is empty")
	}
	path := strings.TrimSpace(template)
	if path == "" {
		path = defaultImageTaskStatusPath
	}
	path = strings.ReplaceAll(path, "{task_id}", taskID)
	if parsed, err := url.Parse(path); err == nil && parsed.IsAbs() {
		if parsed.Scheme != "http" && parsed.Scheme != "https" {
			return "", fmt.Errorf("upstream image task status URL must use HTTP or HTTPS")
		}
		return parsed.String(), nil
	}
	if strings.TrimSpace(baseURL) == "" {
		return "", fmt.Errorf("upstream image task status URL requires a base URL")
	}
	return buildOpenAIEndpointURL(baseURL, path), nil
}
