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
// does not: sub2api's default "antigravity/hub/2.23.0 windows/amd64" is answered
// with 403 PERMISSION_DENIED ("Verify your account to continue."). Measured on
// 2026-11-07: the official Antigravity Hub fingerprint used by the native client
// and third-party managers (e.g. Antigravity-Manager),
// "antigravity/hub/{ver} (aidev_client; os_type=windows; arch=amd64; cl={cl})",
// is accepted on both prod and daily endpoints — the gate checks the
// "aidev_client" token and the cl field, which sub2api's short format lacks.
// The plain product format "antigravity/1.15.7" is also accepted but returns a
// partial response: only the "Claude and GPT models" group, dropping
// "Gemini Models" and "Gemini 4 Argon" (the same partial-group trap reported in
// CLIProxyAPI Management Center issue #499). So the fallback must use the full
// official hub fingerprint, not the plain product format.
var quotaSummaryFallbackUserAgent = "antigravity/hub/2.28.0 (aidev_client; os_type=windows; arch=amd64; cl=1008300195)"

// quotaSummaryBaseURLs is the endpoint order for the quota summary: daily first
// (fresh fractions for every group), prod as fallback. Tests may override it.
var quotaSummaryBaseURLs = []string{antigravityDailyBaseURL, antigravityProdBaseURL}

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
	// Measured 2026-11-07: the prod endpoint (cloudcode-pa.googleapis.com) answers
	// the summary with 200 but STALE fractions for the Gemini and Argon groups
	// (e.g. gemini-argon-wtu-weekly 0.0039% used vs 12.56% actual), while the
	// daily endpoint returns fresh values matching the official Antigravity client.
	// Antigravity-Manager likewise puts daily first for quota queries. So the
	// summary must prefer daily, unlike fetchAvailableModels which stays on prod.
	endpoints := quotaSummaryBaseURLs
	var lastErr error
	for urlIdx, baseURL := range endpoints {
		summary, status, err := c.retrieveUserQuotaSummary(ctx, baseURL, accessToken, projectID, userAgent)
		if err == nil {
			DefaultURLAvailability.MarkSuccess(baseURL)
			return summary, nil
		}
		lastErr = err
		if urlIdx < len(endpoints)-1 && shouldFallbackToNextURL(nil, status) {
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
