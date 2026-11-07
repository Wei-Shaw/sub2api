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

// The summary endpoint gates on the User-Agent format while fetchAvailableModels
// does not: "antigravity/hub/2.23.0 windows/amd64" is answered with 403
// PERMISSION_DENIED ("Verify your account to continue."), while the plain
// product format "antigravity/1.15.7 ..." is accepted. Measured on
// 2026-11-07: versions up to and including 1.15.7 pass, 1.15.8 and anything
// 2.x are rejected, on both prod and daily endpoints, for every account tried.
// So when the configured hub UA gets a 403 we retry once with the plain
// product format before giving up.
var quotaSummaryFallbackUserAgent = "antigravity/1.15.7"

// QuotaSummaryBucket is one quota window of a group, e.g. "gemini-5h" or "gemini-weekly".
// An untouched bucket carries no description; a used one does (e.g. "You have used
// some of your 5-hour limit, it will fully refresh in 3 hours, 13 minutes.").
type QuotaSummaryBucket struct {
	BucketID          string  `json:"bucketId"`
	DisplayName       string  `json:"displayName,omitempty"`
	Window            string  `json:"window"` // "5h" or "weekly"
	Description       string  `json:"description,omitempty"`
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

// isPermissionDenied reports whether err is the summary endpoint's 403 answer.
// retrieveUserQuotaSummary wraps the status into the error text, so match it.
func isPermissionDenied(err error) bool {
	if err == nil {
		return false
	}
	return strings.Contains(err.Error(), "(HTTP 403)")
}

// RetrieveUserQuotaSummary reads the account's per-group 5-hour and weekly quota windows,
// falling back across BaseURLs like FetchAvailableModels.
func (c *Client) RetrieveUserQuotaSummary(ctx context.Context, accessToken, projectID string) (*QuotaSummaryResponse, error) {
	summary, err := c.retrieveUserQuotaSummaryWithUserAgent(ctx, accessToken, projectID, GetUserAgentForContext(ctx))
	if err == nil {
		return summary, nil
	}
	// The summary endpoint, unlike fetchAvailableModels, rejects the hub UA format
	// with 403. Retry once with the plain product UA before giving up.
	if isPermissionDenied(err) {
		if summary, fbErr := c.retrieveUserQuotaSummaryWithUserAgent(ctx, accessToken, projectID, quotaSummaryFallbackUserAgent); fbErr == nil {
			return summary, nil
		}
	}
	return nil, err
}

func (c *Client) retrieveUserQuotaSummaryWithUserAgent(ctx context.Context, accessToken, projectID, userAgent string) (*QuotaSummaryResponse, error) {
	var lastErr error
	for urlIdx, baseURL := range BaseURLs {
		summary, status, err := c.retrieveUserQuotaSummary(ctx, baseURL, accessToken, projectID, userAgent)
		if err == nil {
			DefaultURLAvailability.MarkSuccess(baseURL)
			return summary, nil
		}
		lastErr = err
		if urlIdx < len(BaseURLs)-1 && shouldFallbackToNextURL(nil, status) {
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
	summary, _, err := c.retrieveUserQuotaSummary(ctx, baseURL, accessToken, projectID, GetUserAgentForContext(ctx))
	return summary, err
}

func (c *Client) retrieveUserQuotaSummary(ctx context.Context, baseURL, accessToken, projectID, userAgent string) (*QuotaSummaryResponse, int, error) {
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
	req.Header.Set("User-Agent", userAgent)

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
