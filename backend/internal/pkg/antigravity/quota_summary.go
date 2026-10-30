package antigravity

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
)

type QuotaSummary struct {
	Groups []QuotaSummaryGroup `json:"groups"`
}
type QuotaSummaryGroup struct {
	DisplayName string               `json:"displayName"`
	Buckets     []QuotaSummaryBucket `json:"buckets"`
}
type QuotaSummaryBucket struct {
	ID                string   `json:"bucketId"`
	Window            string   `json:"window"`
	RemainingFraction *float64 `json:"remainingFraction"`
	ResetTime         string   `json:"resetTime"`
}

// FetchQuotaSummary uses this account's OAuth token; failures are not retried or used to change account status.
func (c *Client) FetchQuotaSummary(ctx context.Context, token, project string) (*QuotaSummary, error) {
	body, err := json.Marshal(FetchAvailableModelsRequest{Project: project})
	if err != nil {
		return nil, err
	}
	req, err := NewAPIRequestWithURL(ctx, BaseURLs[0], "retrieveUserQuotaSummary", token, body)
	if err != nil {
		return nil, err
	}
	resp, err := c.fetchAvailableModelsHTTPClient().Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("quota summary HTTP %d", resp.StatusCode)
	}
	var result QuotaSummary
	if err := json.NewDecoder(io.LimitReader(resp.Body, 1<<20)).Decode(&result); err != nil {
		return nil, err
	}
	return &result, nil
}
