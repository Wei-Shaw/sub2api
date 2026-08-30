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
)

const (
	turbProvisionPath    = "/api/automation/provision"
	turbReauthorizePath  = "/api/automation/reauthorize"
	maxAutomationRespLen = 8 << 10
)

type openAIProvisionCommand struct {
	RequestID   string `json:"request_id"`
	Count       int    `json:"count"`
	Workers     int    `json:"workers"`
	EmailSource string `json:"email_source,omitempty"`
	CallbackURL string `json:"callback_url"`
}

type openAIReauthorizeCommand struct {
	RequestID   string `json:"request_id"`
	AccountID   int64  `json:"account_id"`
	Email       string `json:"email"`
	CallbackURL string `json:"callback_url"`
}

type openAIProvisionClient interface {
	Provision(context.Context, openAIProvisionCommand, string, string) error
	Reauthorize(context.Context, openAIReauthorizeCommand, string, string) error
}

type turbOpenAIProvisionClient struct {
	httpClient *http.Client
}

func newTurbOpenAIProvisionClient(httpClient *http.Client) *turbOpenAIProvisionClient {
	if httpClient == nil {
		httpClient = &http.Client{Timeout: 20 * time.Second}
	}
	return &turbOpenAIProvisionClient{httpClient: httpClient}
}

func (c *turbOpenAIProvisionClient) Provision(ctx context.Context, command openAIProvisionCommand, settingsURL, authCode string) error {
	return c.post(ctx, turbProvisionPath, command, settingsURL, authCode)
}

func (c *turbOpenAIProvisionClient) Reauthorize(ctx context.Context, command openAIReauthorizeCommand, settingsURL, authCode string) error {
	return c.post(ctx, turbReauthorizePath, command, settingsURL, authCode)
}

func (c *turbOpenAIProvisionClient) post(ctx context.Context, path string, payload any, settingsURL, authCode string) error {
	base, err := validatedAutomationBaseURL(settingsURL)
	if err != nil {
		return err
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("marshal automation command: %w", err)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, strings.TrimRight(base, "/")+path, bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("create automation request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Auth-Code", strings.TrimSpace(authCode))
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("send automation command: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()
	responseBody, _ := io.ReadAll(io.LimitReader(resp.Body, maxAutomationRespLen))
	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		return fmt.Errorf("automation command returned HTTP %d: %s", resp.StatusCode, strings.TrimSpace(string(responseBody)))
	}
	return nil
}

func validatedAutomationBaseURL(raw string) (string, error) {
	value := strings.TrimSpace(raw)
	parsed, err := url.Parse(value)
	if err != nil || parsed.Scheme == "" || parsed.Host == "" || parsed.User != nil {
		return "", fmt.Errorf("automation turb URL must be an absolute HTTP(S) URL without userinfo")
	}
	if parsed.Scheme != "http" && parsed.Scheme != "https" {
		return "", fmt.Errorf("automation turb URL must use http or https")
	}
	return strings.TrimRight(value, "/"), nil
}
