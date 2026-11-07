package antigravity

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/Wei-Shaw/sub2api/internal/pkg/servertiming"
)

// quotaSummaryBodyLimit bounds the retrieveUserQuotaSummary response (a few hundred bytes today).
const quotaSummaryBodyLimit = 1 << 20

// QuotaSummaryBucket is one quota window of a group, e.g. "gemini-5h" or "gemini-weekly".
type QuotaSummaryBucket struct {
	BucketID          string  `json:"bucketId"`
	DisplayName       string  `json:"displayName,omitempty"`
	Window            string  `json:"window"` // "5h" or "weekly"
	ResetTime         string  `json:"resetTime,omitempty"`
	RemainingFraction float64 `json:"remainingFraction"`
}

// QuotaSummaryGroup is a set of models that share their quota windows.
type QuotaSummaryGroup struct {
	DisplayName string               `json:"displayName"`
	Description string               `json:"description,omitempty"`
	Buckets     []QuotaSummaryBucket `json:"buckets"`
}

// QuotaSummaryResponse is the retrieveUserQuotaSummary response.
//
// Unlike fetchAvailableModels, which reports one fraction per model (whichever window is
// tighter), this reports every window of every group, so the 5-hour and the weekly limit can
// be told apart. Within a group all models share both windows.
type QuotaSummaryResponse struct {
	Groups      []QuotaSummaryGroup `json:"groups"`
	Description string              `json:"description,omitempty"`
}

// RetrieveUserQuotaSummary reads the account's per-group 5-hour and weekly quota windows from
// the endpoint generation uses (daily first, see ForwardBaseURLs), falling back to the others.
//
// The prod endpoint can report a different, stale window: on 2026-11-06 Google reset two Ultra
// accounts' Gemini quota, and prod kept returning the old weekly figure (45.8%) with the 5-hour
// window never starting, while daily and the Antigravity app showed the real 5.9% / 7.5%.
func (c *Client) RetrieveUserQuotaSummary(ctx context.Context, accessToken, projectID string) (*QuotaSummaryResponse, error) {
	urls := ForwardBaseURLs()
	var lastErr error
	for urlIdx, baseURL := range urls {
		summary, status, err := c.retrieveUserQuotaSummary(ctx, baseURL, accessToken, projectID)
		if err == nil {
			DefaultURLAvailability.MarkSuccess(baseURL)
			return summary, nil
		}
		lastErr = err
		if urlIdx < len(urls)-1 && shouldFallbackToNextURL(nil, status) {
			continue
		}
		return nil, err
	}
	return nil, lastErr
}

// RetrieveUserQuotaSummaryFromURL reads the quota summary from one endpoint, without fallback.
func (c *Client) RetrieveUserQuotaSummaryFromURL(ctx context.Context, baseURL, accessToken, projectID string) (*QuotaSummaryResponse, error) {
	baseURL = strings.TrimRight(strings.TrimSpace(baseURL), "/")
	if baseURL == "" {
		return nil, errors.New("retrieveUserQuotaSummary base URL is required")
	}
	summary, _, err := c.retrieveUserQuotaSummary(ctx, baseURL, accessToken, projectID)
	return summary, err
}

func (c *Client) retrieveUserQuotaSummary(ctx context.Context, baseURL, accessToken, projectID string) (*QuotaSummaryResponse, int, error) {
	if c == nil || c.httpClient == nil {
		return nil, 0, errors.New("antigravity client is not configured")
	}
	bodyBytes, err := json.Marshal(FetchAvailableModelsRequest{Project: projectID})
	if err != nil {
		return nil, 0, fmt.Errorf("marshal retrieveUserQuotaSummary request: %w", err)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, baseURL+"/v1internal:retrieveUserQuotaSummary", strings.NewReader(string(bodyBytes)))
	if err != nil {
		return nil, 0, fmt.Errorf("create retrieveUserQuotaSummary request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+accessToken)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", GetUserAgentForContext(ctx))

	resp, err := servertiming.Do(c.httpClient, req)
	if err != nil {
		return nil, 0, fmt.Errorf("retrieveUserQuotaSummary request failed: %w", err)
	}
	respBody, err := io.ReadAll(io.LimitReader(resp.Body, quotaSummaryBodyLimit+1))
	_ = resp.Body.Close()
	if err != nil {
		return nil, resp.StatusCode, fmt.Errorf("read retrieveUserQuotaSummary response: %w", err)
	}
	if len(respBody) > quotaSummaryBodyLimit {
		return nil, resp.StatusCode, fmt.Errorf("retrieveUserQuotaSummary response exceeds %d bytes", quotaSummaryBodyLimit)
	}
	if resp.StatusCode != http.StatusOK {
		return nil, resp.StatusCode, fmt.Errorf("retrieveUserQuotaSummary failed (HTTP %d): %s", resp.StatusCode, truncateQuotaSummaryBody(respBody))
	}
	var summary QuotaSummaryResponse
	if err := json.Unmarshal(respBody, &summary); err != nil {
		return nil, resp.StatusCode, fmt.Errorf("parse retrieveUserQuotaSummary response: %w", err)
	}
	return &summary, resp.StatusCode, nil
}

func truncateQuotaSummaryBody(body []byte) string {
	const max = 300
	if len(body) > max {
		return string(body[:max]) + "..."
	}
	return string(body)
}
