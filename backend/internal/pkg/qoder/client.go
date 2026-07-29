// Package qoder implements a lightweight client for the Qoder Cloud Agents API
// (https://api.qoder.com/api/v1/cloud). The API is stateful: callers create an
// Agent and an Environment, open a Session, post user events, then read agent
// output from a Server-Sent Events (SSE) stream.
//
// The client does not own an *http.Client. Instead callers supply a Doer that
// executes prepared requests, allowing the sub2api service layer to reuse its
// shared upstream transport (per-account proxy, concurrency limiting, etc.).
package qoder

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
)

// DefaultBaseURL is the Qoder Cloud Agents API root.
const DefaultBaseURL = "https://api.qoder.com/api/v1/cloud"

// Doer executes an HTTP request and returns the response. Implementations bind
// account-scoped concerns (proxy, concurrency) so the client stays transport
// agnostic. It mirrors the semantics of http.Client.Do.
type Doer func(req *http.Request) (*http.Response, error)

// Client talks to a single Qoder account's Cloud Agents API.
type Client struct {
	BaseURL string
	APIKey  string
	Do      Doer
}

// NewClient builds a Client. When baseURL is empty DefaultBaseURL is used.
func NewClient(baseURL, apiKey string, do Doer) *Client {
	if strings.TrimSpace(baseURL) == "" {
		baseURL = DefaultBaseURL
	}
	return &Client{
		BaseURL: strings.TrimRight(strings.TrimSpace(baseURL), "/"),
		APIKey:  strings.TrimSpace(apiKey),
		Do:      do,
	}
}

// APIError describes a non-2xx response from the Qoder API.
type APIError struct {
	StatusCode int
	Body       string
}

func (e *APIError) Error() string {
	return fmt.Sprintf("qoder api error: status=%d body=%s", e.StatusCode, e.Body)
}

// IsConflict reports whether the error is a 409 (e.g. session busy).
func (e *APIError) IsConflict() bool { return e != nil && e.StatusCode == http.StatusConflict }

func (c *Client) newRequest(ctx context.Context, method, path string, body []byte) (*http.Request, error) {
	var reader io.Reader
	if body != nil {
		reader = bytes.NewReader(body)
	}
	req, err := http.NewRequestWithContext(ctx, method, c.BaseURL+path, reader)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+c.APIKey)
	req.Header.Set("Accept", "application/json")
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	return req, nil
}

// doJSON executes a request expecting a JSON response and decodes it into out
// (out may be nil to discard the body).
func (c *Client) doJSON(ctx context.Context, method, path string, payload any, out any) error {
	var body []byte
	if payload != nil {
		var err error
		body, err = json.Marshal(payload)
		if err != nil {
			return fmt.Errorf("marshal request: %w", err)
		}
	}
	req, err := c.newRequest(ctx, method, path, body)
	if err != nil {
		return err
	}
	resp, err := c.Do(req)
	if err != nil {
		return fmt.Errorf("qoder request failed: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	respBody, _ := io.ReadAll(resp.Body)
	if resp.StatusCode >= 400 {
		return &APIError{StatusCode: resp.StatusCode, Body: string(respBody)}
	}
	if out != nil && len(respBody) > 0 {
		if err := json.Unmarshal(respBody, out); err != nil {
			return fmt.Errorf("decode response: %w", err)
		}
	}
	return nil
}

// idEnvelope tolerates both a bare {"id":...} and a {"data":{"id":...}} wrapper.
type idEnvelope struct {
	ID     string `json:"id"`
	Status string `json:"status"`
	Data   *struct {
		ID     string `json:"id"`
		Status string `json:"status"`
	} `json:"data"`
}

func (e idEnvelope) resolve() (id, status string) {
	id, status = e.ID, e.Status
	if id == "" && e.Data != nil {
		id, status = e.Data.ID, e.Data.Status
	}
	return id, status
}

// CreateAgent creates an agent and returns its id.
func (c *Client) CreateAgent(ctx context.Context, name, model string) (string, error) {
	payload := map[string]any{"name": name, "model": model}
	var env idEnvelope
	if err := c.doJSON(ctx, http.MethodPost, "/agents", payload, &env); err != nil {
		return "", err
	}
	id, _ := env.resolve()
	if id == "" {
		return "", fmt.Errorf("qoder create agent: empty id in response")
	}
	return id, nil
}

// CreateEnvironment creates a cloud environment with limited networking and
// returns its id.
func (c *Client) CreateEnvironment(ctx context.Context, name string) (string, error) {
	payload := map[string]any{
		"name": name,
		"config": map[string]any{
			"type": "cloud",
			"networking": map[string]any{
				"type": "limited",
			},
		},
	}
	var env idEnvelope
	if err := c.doJSON(ctx, http.MethodPost, "/environments", payload, &env); err != nil {
		return "", err
	}
	id, _ := env.resolve()
	if id == "" {
		return "", fmt.Errorf("qoder create environment: empty id in response")
	}
	return id, nil
}

// CreateSession opens a session bound to an agent and environment, returning the
// session id.
func (c *Client) CreateSession(ctx context.Context, agentID, environmentID string) (string, error) {
	payload := map[string]any{"agent": agentID, "environment_id": environmentID}
	var env idEnvelope
	if err := c.doJSON(ctx, http.MethodPost, "/sessions", payload, &env); err != nil {
		return "", err
	}
	id, _ := env.resolve()
	if id == "" {
		return "", fmt.Errorf("qoder create session: empty id in response")
	}
	return id, nil
}

// SendUserMessage posts a single user.message text event to a session.
func (c *Client) SendUserMessage(ctx context.Context, sessionID, text string) error {
	payload := map[string]any{
		"events": []any{
			map[string]any{
				"type": "user.message",
				"content": []any{
					map[string]any{"type": "text", "text": text},
				},
			},
		},
	}
	path := "/sessions/" + url.PathEscape(sessionID) + "/events"
	return c.doJSON(ctx, http.MethodPost, path, payload, nil)
}

// StreamEvents opens the session SSE stream filtered to agent.message deltas.
// When lastEventID is non-empty it is sent as the Last-Event-ID header so the
// upstream replays only events newer than the previous turn. The caller owns
// closing the returned response body.
func (c *Client) StreamEvents(ctx context.Context, sessionID, lastEventID string) (*http.Response, error) {
	path := "/sessions/" + url.PathEscape(sessionID) + "/events/stream?event_deltas[]=agent.message"
	req, err := c.newRequest(ctx, http.MethodGet, path, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "text/event-stream")
	if strings.TrimSpace(lastEventID) != "" {
		req.Header.Set("Last-Event-ID", lastEventID)
	}
	resp, err := c.Do(req)
	if err != nil {
		return nil, fmt.Errorf("qoder stream request failed: %w", err)
	}
	if resp.StatusCode >= 400 {
		respBody, _ := io.ReadAll(resp.Body)
		_ = resp.Body.Close()
		return nil, &APIError{StatusCode: resp.StatusCode, Body: string(respBody)}
	}
	return resp, nil
}

// Model describes an entry returned by GET /models.
type Model struct {
	ID          string `json:"id"`
	DisplayName string `json:"display_name"`
	IsEnabled   bool   `json:"is_enabled"`
}

// ListModels returns the available model identifiers for the account.
func (c *Client) ListModels(ctx context.Context) ([]Model, error) {
	var out struct {
		Data   []Model `json:"data"`
		Models []Model `json:"models"`
	}
	if err := c.doJSON(ctx, http.MethodGet, "/models", nil, &out); err != nil {
		return nil, err
	}
	if len(out.Data) > 0 {
		return out.Data, nil
	}
	return out.Models, nil
}

// Ping verifies the PAT by listing a single agent. A nil error means the token
// is accepted by the Qoder API.
func (c *Client) Ping(ctx context.Context) error {
	req, err := c.newRequest(ctx, http.MethodGet, "/agents?limit=1", nil)
	if err != nil {
		return err
	}
	resp, err := c.Do(req)
	if err != nil {
		return fmt.Errorf("qoder ping failed: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()
	respBody, _ := io.ReadAll(resp.Body)
	if resp.StatusCode >= 400 {
		return &APIError{StatusCode: resp.StatusCode, Body: string(respBody)}
	}
	return nil
}

const defaultExchangeURL = "https://openapi.qoder.sh/api/v1/jobToken/exchange"

type JobTokenResponse struct {
	Token        string `json:"token"`
	RefreshToken string `json:"refresh_token"`
	ExpiresAt    string `json:"expires_at"`
	ExpiresIn    int64  `json:"expires_in"` // milliseconds
}

func ExchangeJobToken(ctx context.Context, do Doer, personalToken string) (*JobTokenResponse, error) {
	payload, err := json.Marshal(map[string]string{"personal_token": personalToken})
	if err != nil {
		return nil, fmt.Errorf("marshal exchange request: %w", err)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, defaultExchangeURL, bytes.NewReader(payload))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	resp, err := do(req)
	if err != nil {
		return nil, fmt.Errorf("qoder token exchange failed: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()
	respBody, _ := io.ReadAll(resp.Body)
	if resp.StatusCode >= 400 {
		return nil, &APIError{StatusCode: resp.StatusCode, Body: string(respBody)}
	}
	var out JobTokenResponse
	if err := json.Unmarshal(respBody, &out); err != nil {
		return nil, fmt.Errorf("decode exchange response: %w", err)
	}
	if out.Token == "" {
		return nil, fmt.Errorf("qoder token exchange: empty token in response")
	}
	return &out, nil
}
