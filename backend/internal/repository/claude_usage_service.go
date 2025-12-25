package repository

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/service"
)

type claudeUsageService struct{}

func NewClaudeUsageFetcher() service.ClaudeUsageFetcher {
	return &claudeUsageService{}
}

func (s *claudeUsageService) FetchUsage(ctx context.Context, accessToken, proxyURL string) (*service.ClaudeUsageResponse, error) {
	transport, ok := http.DefaultTransport.(*http.Transport)
	if !ok {
		return nil, fmt.Errorf("failed to get default transport")
	}
	transport = transport.Clone()
	if proxyURL != "" {
		if parsedURL, err := url.Parse(proxyURL); err == nil {
			transport.Proxy = http.ProxyURL(parsedURL)
		}
	}

	client := &http.Client{
		Transport: transport,
		Timeout:   30 * time.Second,
	}

	req, err := http.NewRequestWithContext(ctx, "GET", "https://api.anthropic.com/api/oauth/usage", nil)
	if err != nil {
		return nil, fmt.Errorf("create request failed: %w", err)
	}

	req.Header.Set("Authorization", "Bearer "+accessToken)
	req.Header.Set("anthropic-beta", "oauth-2025-04-20")

	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("request failed: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("API returned status %d: %s", resp.StatusCode, string(body))
	}

	var usageResp service.ClaudeUsageResponse
	if err := json.NewDecoder(resp.Body).Decode(&usageResp); err != nil {
		return nil, fmt.Errorf("decode response failed: %w", err)
	}

	return &usageResp, nil
}
