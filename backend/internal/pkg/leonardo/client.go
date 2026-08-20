package leonardo

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"strings"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/logger"
	"github.com/Wei-Shaw/sub2api/internal/pkg/proxyurl"
	"github.com/Wei-Shaw/sub2api/internal/pkg/proxyutil"
	"go.uber.org/zap"
)

const responseBodyLimit int64 = 8 << 20

const debugPromptLimit = 2000

type APIError struct {
	StatusCode int
	Body       string
}

func (e *APIError) Error() string {
	return fmt.Sprintf("leonardo upstream error (HTTP %d): %s", e.StatusCode, e.Body)
}

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

func NewClient(cfg Config) (*Client, error) {
	if strings.TrimSpace(cfg.APIKey) == "" {
		return nil, errors.New("leonardo: api key is required")
	}
	timeout := cfg.Timeout
	if timeout <= 0 {
		timeout = 120 * time.Second
	}
	httpClient := &http.Client{Timeout: timeout}
	_, parsedProxy, err := proxyurl.Parse(cfg.ProxyURL)
	if err != nil {
		return nil, err
	}
	if parsedProxy != nil {
		transport := &http.Transport{
			DialContext:         (&net.Dialer{Timeout: 10 * time.Second}).DialContext,
			TLSHandshakeTimeout: 10 * time.Second,
		}
		if err := proxyutil.ConfigureTransportProxy(transport, parsedProxy); err != nil {
			return nil, fmt.Errorf("leonardo: configure proxy: %w", err)
		}
		httpClient.Transport = transport
	}
	baseURL := strings.TrimRight(strings.TrimSpace(cfg.BaseURL), "/")
	if baseURL == "" {
		baseURL = DefaultBaseURL
	}
	return &Client{httpClient: httpClient, apiKey: strings.TrimSpace(cfg.APIKey), baseURL: baseURL}, nil
}

func (c *Client) Submit(ctx context.Context, request *SubmitRequest, idempotencyKey string) (*Task, error) {
	if strings.TrimSpace(idempotencyKey) == "" {
		return nil, errors.New("leonardo: idempotency key is required")
	}
	var task Task
	if err := c.doJSON(ctx, http.MethodPost, c.baseURL+"/v1/tasks", request, idempotencyKey, &task); err != nil {
		return nil, err
	}
	if strings.TrimSpace(task.TaskUUID) == "" {
		return nil, errors.New("leonardo: submit response missing task_uuid")
	}
	return &task, nil
}

func (c *Client) GetTask(ctx context.Context, taskUUID string) (*Task, error) {
	if strings.TrimSpace(taskUUID) == "" {
		return nil, errors.New("leonardo: task uuid is required")
	}
	var task Task
	if err := c.doJSON(ctx, http.MethodGet, c.BuildTaskURL(taskUUID), nil, "", &task); err != nil {
		return nil, err
	}
	if task.TaskUUID == "" {
		task.TaskUUID = strings.TrimSpace(taskUUID)
	}
	return &task, nil
}

func (c *Client) BuildTaskURL(taskUUID string) string {
	return c.baseURL + "/v1/tasks/" + strings.TrimSpace(taskUUID)
}

func (c *Client) TasksURL() string { return c.baseURL + "/v1/tasks" }

func (c *Client) doJSON(ctx context.Context, method, endpoint string, body any, idempotencyKey string, out any) error {
	var reader io.Reader
	var requestBody []byte
	if body != nil {
		raw, err := json.Marshal(body)
		if err != nil {
			return fmt.Errorf("leonardo: marshal request: %w", err)
		}
		requestBody = raw
		reader = bytes.NewReader(raw)
	}
	log := logger.FromContext(ctx)
	requestFields := []zap.Field{
		zap.String("method", method),
		zap.String("url", endpoint),
	}
	if idempotencyKey != "" {
		requestFields = append(requestFields, zap.String("idempotency_key", idempotencyKey))
	}
	if len(requestBody) > 0 {
		requestFields = append(requestFields, zap.String("request_body", truncatePromptInJSON(requestBody)))
	}
	log.Debug("leonardo.http.request", requestFields...)
	req, err := http.NewRequestWithContext(ctx, method, endpoint, reader)
	if err != nil {
		return fmt.Errorf("leonardo: build request: %w", err)
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("X-API-Key", c.apiKey)
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if idempotencyKey != "" {
		req.Header.Set("Idempotency-Key", idempotencyKey)
	}
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("leonardo: do request: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()
	raw, err := io.ReadAll(io.LimitReader(resp.Body, responseBodyLimit+1))
	if err != nil {
		return fmt.Errorf("leonardo: read response: %w", err)
	}
	if int64(len(raw)) > responseBodyLimit {
		return errors.New("leonardo: response body too large")
	}
	log.Debug("leonardo.http.response",
		zap.String("method", method),
		zap.String("url", endpoint),
		zap.Int("status_code", resp.StatusCode),
		zap.String("response_body", string(raw)),
	)
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return &APIError{StatusCode: resp.StatusCode, Body: strings.TrimSpace(string(raw))}
	}
	if out != nil && len(bytes.TrimSpace(raw)) > 0 {
		if err := json.Unmarshal(raw, out); err != nil {
			return fmt.Errorf("leonardo: decode response: %w", err)
		}
	}
	return nil
}

// truncatePromptInJSON keeps every request field visible while limiting only
// the potentially very large image prompt in debug logs.
func truncatePromptInJSON(raw []byte) string {
	var value any
	if err := json.Unmarshal(raw, &value); err != nil {
		return string(raw)
	}
	truncatePromptValue(value)
	encoded, err := json.Marshal(value)
	if err != nil {
		return string(raw)
	}
	return string(encoded)
}

func truncatePromptValue(value any) {
	switch current := value.(type) {
	case map[string]any:
		for key, child := range current {
			if strings.EqualFold(key, "prompt") {
				if prompt, ok := child.(string); ok {
					current[key] = truncatePrompt(prompt, debugPromptLimit)
				}
				continue
			}
			truncatePromptValue(child)
		}
	case []any:
		for _, child := range current {
			truncatePromptValue(child)
		}
	}
}

func truncatePrompt(prompt string, limit int) string {
	runes := []rune(prompt)
	if len(runes) <= limit {
		return prompt
	}
	return string(runes[:limit]) + "...(truncated)"
}
