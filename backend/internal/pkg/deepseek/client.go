package deepseek

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

// Client is a DeepSeek web API client.
type Client struct {
	httpClient *http.Client
}

// defaultClient is a shared HTTP client with connection pooling.
var defaultClient = &http.Client{Timeout: 300 * time.Second}

// NewClient creates a new DeepSeek client. Pass nil for the shared default client.
func NewClient(httpClient *http.Client) *Client {
	if httpClient == nil {
		httpClient = defaultClient
	}
	return &Client{httpClient: httpClient}
}

// CreateSession creates a new chat session and returns the session ID.
func (c *Client) CreateSession(ctx context.Context, token, cookie string) (string, error) {
	body, err := c.doJSON(ctx, http.MethodPost, DeepSeekCreateSessionURL, token, cookie, map[string]any{})
	if err != nil {
		return "", fmt.Errorf("create session: %w", err)
	}
	bizData, err := extractBizData(body)
	if err != nil {
		return "", err
	}
	// Try data.biz_data.chat_session.id first (some versions)
	if chatSession, ok := bizData["chat_session"].(map[string]any); ok {
		if id, ok := chatSession["id"].(string); ok && id != "" {
			return id, nil
		}
	}
	// Fallback: data.biz_data.id (direct session id)
	if id, ok := bizData["id"].(string); ok && id != "" {
		return id, nil
	}
	return "", fmt.Errorf("create session: no session id in response")
}

// DeleteSession deletes a chat session.
func (c *Client) DeleteSession(ctx context.Context, token, cookie, sessionID string) error {
	_, err := c.doJSON(ctx, http.MethodPost, DeepSeekDeleteSessionURL, token, cookie, map[string]any{
		"chat_session_id": sessionID,
	})
	return err
}

// CreatePowChallenge gets a PoW challenge for the completion endpoint.
func (c *Client) CreatePowChallenge(ctx context.Context, token, cookie string) (*PowChallenge, error) {
	body, err := c.doJSON(ctx, http.MethodPost, DeepSeekCreatePowURL, token, cookie, map[string]any{
		"target_path": CompletionTargetPath,
	})
	if err != nil {
		return nil, fmt.Errorf("get pow challenge: %w", err)
	}
	bizData, err := extractBizData(body)
	if err != nil {
		return nil, err
	}
	challengeData, _ := bizData["challenge"].(map[string]any)
	if challengeData == nil {
		return nil, fmt.Errorf("get pow challenge: missing challenge in response")
	}
	b, _ := json.Marshal(challengeData)
	var challenge PowChallenge
	if err := json.Unmarshal(b, &challenge); err != nil {
		return nil, fmt.Errorf("parse challenge: %w", err)
	}
	return &challenge, nil
}

// CallCompletion sends a chat completion request and returns the streaming response.
func (c *Client) CallCompletion(ctx context.Context, token, cookie, powHeader string, payload map[string]any) (*http.Response, error) {
	b, err := json.Marshal(payload)
	if err != nil {
		return nil, err
	}
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, DeepSeekCompletionURL, bytes.NewReader(b))
	if err != nil {
		return nil, err
	}
	c.setHeaders(httpReq, token, cookie)
	httpReq.Header.Set("x-ds-pow-response", powHeader)

	resp, err := c.httpClient.Do(httpReq)
	if err != nil {
		return nil, err
	}
	return resp, nil
}

// ValidateToken checks if a DeepSeek token is valid by creating and deleting a session.
func (c *Client) ValidateToken(ctx context.Context, token, cookie string) error {
	sessionID, err := c.CreateSession(ctx, token, cookie)
	if err != nil {
		return fmt.Errorf("token validation failed: %w", err)
	}
	_ = c.DeleteSession(ctx, token, cookie, sessionID)
	return nil
}

func (c *Client) doJSON(ctx context.Context, method, url, token, cookie string, payload any) (map[string]any, error) {
	var body io.Reader
	if payload != nil {
		b, err := json.Marshal(payload)
		if err != nil {
			return nil, err
		}
		body = bytes.NewReader(b)
	}
	req, err := http.NewRequestWithContext(ctx, method, url, body)
	if err != nil {
		return nil, err
	}
	c.setHeaders(req, token, cookie)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("status %d: %s", resp.StatusCode, string(respBody))
	}
	var result map[string]any
	if err := json.Unmarshal(respBody, &result); err != nil {
		return nil, err
	}
	return result, nil
}

func (c *Client) setHeaders(req *http.Request, token, cookie string) {
	for k, v := range defaultHeaders {
		req.Header.Set(k, v)
	}
	req.Header.Set("Authorization", token)
	if cookie != "" {
		req.Header.Set("Cookie", cookie)
	}
	req.Header.Set("Origin", DeepSeekBaseURL)
	req.Header.Set("Referer", DeepSeekBaseURL+"/")
}

func extractBizData(body map[string]any) (map[string]any, error) {
	data, ok := body["data"].(map[string]any)
	if !ok {
		return nil, fmt.Errorf("missing data in response")
	}
	bizData, ok := data["biz_data"].(map[string]any)
	if !ok {
		return nil, fmt.Errorf("missing biz_data in response")
	}
	return bizData, nil
}
