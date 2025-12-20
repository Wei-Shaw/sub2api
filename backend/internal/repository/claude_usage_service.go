package repository

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"time"

	"sub2api/internal/service"
)

type claudeUsageService struct{REDACTED

func NewClaudeUsageFetcher() service.ClaudeUsageFetcher {
	return &claudeUsageService{REDACTED
REDACTED

func (s *claudeUsageService) FetchUsage(ctx context.Context, accessToken, proxyURL string) (*service.ClaudeUsageResponse, error) {
	transport := http.DefaultTransport.(*http.Transport).Clone()
	if proxyURL != "" {
		if parsedURL, err := url.Parse(proxyURL); err == nil {
			transport.Proxy = http.ProxyURL(parsedURL)
	REDACTED
REDACTED

	client := &http.Client{
		Transport: transport,
		Timeout:   30 * time.Second,
REDACTED

	req, err := http.NewRequestWithContext(ctx, "GET", "https://api.anthropic.com/api/oauth/usage", nil)
	if err != nil {
		return nil, fmt.Errorf("create request failed: %w", err)
REDACTED

	req.Header.Set("Authorization", "Bearer "+accessToken)
	req.Header.Set("anthropic-beta", "oauth-2025-04-20")

	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("request failed: %w", err)
REDACTED
	defer func() { _ = resp.Body.Close() REDACTED()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("API returned status %d: %s", resp.StatusCode, string(body))
REDACTED

	var usageResp service.ClaudeUsageResponse
	if err := json.NewDecoder(resp.Body).Decode(&usageResp); err != nil {
		return nil, fmt.Errorf("decode response failed: %w", err)
REDACTED

	return &usageResp, nil
REDACTED
