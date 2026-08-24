// Package higgsfield adapts the Higgsfield Cloud request API to the common
// asynchronous video client used by the gateway.
//
// Higgsfield's SDK protocol is:
//   - POST /{application} with the model arguments as the JSON body
//   - GET /requests/{id}/status for both status and the completed result
//   - POST /requests/{id}/cancel to cancel a queued request
//
// Authentication uses Authorization: Key {api_key}.
package higgsfield

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/domain"
	"github.com/Wei-Shaw/sub2api/internal/pkg/fal"
	"github.com/Wei-Shaw/sub2api/internal/pkg/proxyurl"
	"github.com/Wei-Shaw/sub2api/internal/pkg/proxyutil"
)

const (
	proxyDialTimeout               = 10 * time.Second
	proxyTLSHandshakeTimeout       = 10 * time.Second
	defaultClientTimeout           = 120 * time.Second
	bodyLimit                int64 = 32 << 20
)

type Config struct {
	APIKey   string
	BaseURL  string
	ProxyURL string
	Timeout  time.Duration
}

type Client struct {
	httpClient *http.Client
	apiKey     string
	baseURL    string
}

type responseEnvelope map[string]any

// NewClient creates a Higgsfield API client.
func NewClient(cfg Config) (*Client, error) {
	if strings.TrimSpace(cfg.APIKey) == "" {
		return nil, errors.New("higgsfield: api key is required")
	}
	baseURL := strings.TrimRight(strings.TrimSpace(cfg.BaseURL), "/")
	if baseURL == "" {
		baseURL = domain.HiggsfieldBaseURL
	}
	timeout := cfg.Timeout
	if timeout <= 0 {
		timeout = defaultClientTimeout
	}
	httpClient := &http.Client{Timeout: timeout}
	_, parsedProxy, err := proxyurl.Parse(cfg.ProxyURL)
	if err != nil {
		return nil, err
	}
	if parsedProxy != nil {
		transport := &http.Transport{
			DialContext:         (&net.Dialer{Timeout: proxyDialTimeout}).DialContext,
			TLSHandshakeTimeout: proxyTLSHandshakeTimeout,
		}
		if err := proxyutil.ConfigureTransportProxy(transport, parsedProxy); err != nil {
			return nil, fmt.Errorf("higgsfield: configure proxy: %w", err)
		}
		httpClient.Transport = transport
	}
	return &Client{httpClient: httpClient, apiKey: strings.TrimSpace(cfg.APIKey), baseURL: baseURL}, nil
}

// SubmitRaw posts the supplied arguments to the Higgsfield application path.
func (c *Client) SubmitRaw(ctx context.Context, model string, body any) (*fal.SubmitResponse, error) {
	model = strings.Trim(model, "/")
	if model == "" {
		return nil, errors.New("higgsfield: model is required")
	}
	raw, err := c.doJSON(ctx, http.MethodPost, c.baseURL+"/"+model, body)
	if err != nil {
		return nil, err
	}
	response := responseEnvelope(raw)
	requestID := firstString(response, "request_id", "id")
	if requestID == "" {
		return nil, &fal.APIError{StatusCode: http.StatusBadGateway, Body: "higgsfield: submit response missing request_id"}
	}
	statusURL := resolveURL(c.baseURL, firstString(response, "status_url"))
	if statusURL == "" {
		statusURL = c.buildStatusURL(requestID)
	}
	cancelURL := resolveURL(c.baseURL, firstString(response, "cancel_url"))
	return &fal.SubmitResponse{
		RequestID:   requestID,
		Status:      fal.StatusInQueue,
		StatusURL:   statusURL,
		ResponseURL: statusURL,
		CancelURL:   cancelURL,
	}, nil
}

// Status maps Higgsfield's queued/in_progress/completed/failed states to fal semantics.
func (c *Client) Status(ctx context.Context, statusURL string) (*fal.StatusResponse, error) {
	statusURL = strings.TrimSpace(statusURL)
	if statusURL == "" {
		return nil, errors.New("higgsfield: status url is empty")
	}
	raw, err := c.doJSON(ctx, http.MethodGet, statusURL, nil)
	if err != nil {
		return nil, err
	}
	response := responseEnvelope(raw)
	status := strings.ToLower(firstString(response, "status"))
	out := &fal.StatusResponse{
		RequestID:   firstString(response, "request_id", "id"),
		ResponseURL: statusURL,
	}
	switch status {
	case "completed", "complete", "succeeded", "success", "finished", "done":
		out.Status = fal.StatusCompleted
	case "failed", "error", "nsfw", "canceled", "cancelled", "timeout":
		reason := firstString(response, "error", "message", "detail", "status")
		return nil, &fal.APIError{StatusCode: http.StatusBadRequest, Body: fmt.Sprintf("higgsfield upstream %s: %s", status, reason)}
	default:
		out.Status = fal.StatusInProgress
	}
	return out, nil
}

// ResultRaw returns the completed Higgsfield response and adds the common video
// and videos fields when the upstream response uses a provider-specific shape.
func (c *Client) ResultRaw(ctx context.Context, responseURL string) (map[string]any, error) {
	responseURL = strings.TrimSpace(responseURL)
	if responseURL == "" {
		return nil, errors.New("higgsfield: response url is empty")
	}
	raw, err := c.doJSON(ctx, http.MethodGet, responseURL, nil)
	if err != nil {
		return nil, err
	}
	return normalizeResult(raw), nil
}

func (c *Client) BuildStatusURL(_ string, requestID string) string {
	return c.buildStatusURL(requestID)
}

func (c *Client) BuildResponseURL(_ string, requestID string) string {
	return c.buildStatusURL(requestID)
}

func (c *Client) BuildCancelURL(_ string, requestID string) string {
	if strings.TrimSpace(requestID) == "" {
		return ""
	}
	return c.baseURL + "/requests/" + url.PathEscape(strings.TrimSpace(requestID)) + "/cancel"
}

func (c *Client) Cancel(ctx context.Context, cancelURL string) error {
	if strings.TrimSpace(cancelURL) == "" {
		return nil
	}
	_, err := c.doJSON(ctx, http.MethodPost, cancelURL, nil)
	return err
}

func (c *Client) buildStatusURL(requestID string) string {
	return c.baseURL + "/requests/" + url.PathEscape(strings.TrimSpace(requestID)) + "/status"
}

func (c *Client) doJSON(ctx context.Context, method, endpoint string, body any) (map[string]any, error) {
	var reader io.Reader
	if body != nil {
		encoded, err := json.Marshal(body)
		if err != nil {
			return nil, fmt.Errorf("higgsfield: marshal request: %w", err)
		}
		reader = bytes.NewReader(encoded)
	}
	req, err := http.NewRequestWithContext(ctx, method, endpoint, reader)
	if err != nil {
		return nil, fmt.Errorf("higgsfield: build request: %w", err)
	}
	req.Header.Set("Authorization", "Key "+c.apiKey)
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Content-Type", "application/json")
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("higgsfield: do request: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()
	raw, err := io.ReadAll(io.LimitReader(resp.Body, bodyLimit))
	if err != nil {
		return nil, fmt.Errorf("higgsfield: read response: %w", err)
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, &fal.APIError{StatusCode: resp.StatusCode, Body: string(raw)}
	}
	if len(bytes.TrimSpace(raw)) == 0 {
		return map[string]any{}, nil
	}
	var decoded map[string]any
	if err := json.Unmarshal(raw, &decoded); err != nil {
		return nil, fmt.Errorf("higgsfield: decode response: %w", err)
	}
	return decoded, nil
}

func firstString(value map[string]any, keys ...string) string {
	for _, key := range keys {
		if raw, ok := value[key]; ok {
			if s, ok := raw.(string); ok && strings.TrimSpace(s) != "" {
				return strings.TrimSpace(s)
			}
		}
	}
	if data, ok := value["data"].(map[string]any); ok {
		return firstString(data, keys...)
	}
	return ""
}

func resolveURL(baseURL, raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return ""
	}
	parsed, err := url.Parse(raw)
	if err != nil {
		return raw
	}
	if parsed.IsAbs() {
		return parsed.String()
	}
	base, err := url.Parse(strings.TrimRight(baseURL, "/") + "/")
	if err != nil {
		return raw
	}
	return base.ResolveReference(parsed).String()
}

func normalizeResult(raw map[string]any) map[string]any {
	if raw == nil {
		return map[string]any{}
	}
	urls := make([]string, 0, 2)
	collectVideoURLs(raw, false, &urls)
	urls = dedup(urls)
	if len(urls) == 0 {
		return raw
	}
	if _, exists := raw["video"]; !exists {
		raw["video"] = map[string]any{"url": urls[0], "file_name": fileNameFromURL(urls[0])}
	}
	if _, exists := raw["videos"]; !exists {
		videos := make([]any, 0, len(urls))
		for _, item := range urls {
			videos = append(videos, map[string]any{"url": item, "file_name": fileNameFromURL(item)})
		}
		raw["videos"] = videos
	}
	return raw
}

func collectVideoURLs(value any, videoContext bool, out *[]string) {
	switch current := value.(type) {
	case map[string]any:
		for key, child := range current {
			lower := strings.ToLower(strings.TrimSpace(key))
			childVideoContext := videoContext || strings.Contains(lower, "video") || lower == "output" || lower == "result"
			if s, ok := child.(string); ok && childVideoContext && (lower == "url" || strings.HasSuffix(lower, "_url")) {
				if strings.HasPrefix(strings.ToLower(strings.TrimSpace(s)), "http") {
					*out = append(*out, strings.TrimSpace(s))
				}
			}
			collectVideoURLs(child, childVideoContext, out)
		}
	case []any:
		for _, child := range current {
			collectVideoURLs(child, videoContext, out)
		}
	}
}

func dedup(values []string) []string {
	seen := make(map[string]struct{}, len(values))
	out := make([]string, 0, len(values))
	for _, value := range values {
		if _, exists := seen[value]; exists {
			continue
		}
		seen[value] = struct{}{}
		out = append(out, value)
	}
	return out
}

func fileNameFromURL(raw string) string {
	parsed, err := url.Parse(strings.TrimSpace(raw))
	if err != nil {
		return ""
	}
	path := strings.TrimRight(parsed.Path, "/")
	if index := strings.LastIndex(path, "/"); index >= 0 {
		return path[index+1:]
	}
	return path
}
