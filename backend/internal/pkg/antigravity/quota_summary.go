package antigravity

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"

	"github.com/Wei-Shaw/sub2api/internal/pkg/servertiming"
)

// QuotaSummary is the upstream's grouped quota contract. A model catalog's
// quotaInfo has no window identity and cannot substitute for these buckets.
type QuotaSummary struct {
	Groups []QuotaGroup `json:"groups"`
}

type QuotaGroup struct {
	DisplayName string        `json:"displayName"`
	Description string        `json:"description,omitempty"`
	Buckets     []QuotaBucket `json:"buckets"`
}

type QuotaBucket struct {
	ID                string   `json:"bucketId"`
	DisplayName       string   `json:"displayName,omitempty"`
	Window            string   `json:"window,omitempty"`
	RemainingFraction *float64 `json:"remainingFraction"`
	ResetTime         string   `json:"resetTime,omitempty"`
	Disabled          bool     `json:"disabled,omitempty"`
}

// RetrieveUserQuotaSummary uses production first: daily can return a partial
// catalog. Never merge snapshots from different endpoints or infer a window
// from its time until reset. Authentication and throttling errors stop retries.
func (c *Client) RetrieveUserQuotaSummary(ctx context.Context, token, projectID string, bodyLimit int64) (*QuotaSummary, error) {
	if c == nil || c.httpClient == nil || bodyLimit <= 0 {
		return nil, errors.New("quota summary client or response limit is invalid")
	}
	client := *c.httpClient
	client.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }
	payload, err := json.Marshal(struct {
		Project string `json:"project,omitempty"`
	}{Project: projectID})
	if err != nil {
		return nil, err
	}
	lastErr := errors.New("quota summary endpoints unavailable")
	for _, baseURL := range quotaSummaryBaseURLs() {
		req, err := NewAPIRequestWithURL(ctx, baseURL, "retrieveUserQuotaSummary", token, payload)
		if err != nil {
			return nil, err
		}
		req.Header.Set("User-Agent", quotaSummaryUserAgent(ctx))
		resp, err := servertiming.Do(&client, req)
		if err != nil {
			// A cancelled deadline must not start another request.
			if ctx.Err() != nil {
				return nil, ctx.Err()
			}
			lastErr = fmt.Errorf("quota summary request failed: %w", err)
			continue
		}
		body, readErr := io.ReadAll(io.LimitReader(resp.Body, bodyLimit+1))
		_ = resp.Body.Close()
		if readErr != nil {
			return nil, fmt.Errorf("read quota summary: %w", readErr)
		}
		if int64(len(body)) > bodyLimit {
			return nil, errors.New("quota summary response exceeds limit")
		}
		if resp.StatusCode == http.StatusForbidden {
			return nil, &ForbiddenError{StatusCode: resp.StatusCode, Body: string(body)}
		}
		if resp.StatusCode != http.StatusOK {
			// Do not reflect upstream bodies (which may contain account data).
			lastErr = fmt.Errorf("quota summary failed (HTTP %d)", resp.StatusCode)
			if resp.StatusCode == http.StatusNotFound || resp.StatusCode >= 500 {
				continue
			}
			return nil, lastErr
		}
		var summary QuotaSummary
		if err := json.Unmarshal(body, &summary); err != nil {
			return nil, fmt.Errorf("decode quota summary: %w", err)
		}
		if summary.Groups == nil {
			return nil, errors.New("quota summary groups missing")
		}
		return &summary, nil
	}
	return nil, lastErr
}

// Reorder the existing endpoint configuration without adding new destinations.
// This also keeps transport tests isolated from the real Google API.
func quotaSummaryBaseURLs() []string {
	urls := make([]string, 0, len(BaseURLs))
	for _, baseURL := range BaseURLs {
		if baseURL == antigravityProdBaseURL {
			urls = append(urls, baseURL)
		}
	}
	for _, baseURL := range BaseURLs {
		if baseURL != antigravityProdBaseURL {
			urls = append(urls, baseURL)
		}
	}
	return urls
}

// Older Hub quota probes can omit Gemini/Argon buckets in an otherwise valid
// response. Use the verified 2.24.0 minimum for this endpoint only; retain newer
// configured versions and leave generation/OAuth client headers unchanged.
func quotaSummaryUserAgent(ctx context.Context) string {
	version := GetUserAgentVersionForContext(ctx)
	parts := strings.Split(version, ".")
	minimum := []int{2, 24, 0}
	for i, floor := range minimum {
		n, err := strconv.Atoi(parts[i]) // version has already been normalized.
		if err != nil || n < floor {
			version = "2.24.0"
			break
		}
		if n > floor {
			break
		}
	}
	return BuildUserAgent(version)
}
