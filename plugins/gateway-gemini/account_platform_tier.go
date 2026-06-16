package main

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/Wei-Shaw/sub2api/plugin-sdk/gatewayutil"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	pb "github.com/Wei-Shaw/sub2api/plugin-sdk/proto/pluginsdk"
)

// Google One tier constants — canonical tier IDs used by sub2api.
const (
	tierGoogleOneFree    = "google_one_free"
	tierGoogleAIPro      = "google_ai_pro"
	tierGoogleAIUltra    = "google_ai_ultra"
	tierGoogleOneUnknown = "google_one_unknown"
)

// Storage thresholds (in bytes) used to infer Google One tier.
const (
	gb = 1024 * 1024 * 1024
	tb = 1024 * gb

	storageTierUnlimited = 100 * tb // 100 TB
	storageTierAIPremium = 2 * tb   // 2 TB
	storageTierFree      = 15 * gb  // 15 GB
)

const driveAboutURL = "https://www.googleapis.com/drive/v3/about?fields=storageQuota"

// RefreshTier refreshes Google One tier info by querying Google Drive
// storage quota. Only google_one OAuth accounts support this operation.
func (s *accountPlatformServer) RefreshTier(
	ctx context.Context,
	req *pb.RefreshTierRequest,
) (*pb.RefreshTierResponse, error) {
	// Only google_one OAuth accounts support tier refresh.
	creds, err := parseCredentials(req.GetCredentialsJson())
	if err != nil {
		return &pb.RefreshTierResponse{
			Success: false,
			Error:   "failed to parse credentials: " + err.Error(),
		}, nil
	}

	if creds.OAuthType != "google_one" {
		return &pb.RefreshTierResponse{
			Success: false,
			Error:   "tier refresh is only supported for google_one OAuth accounts",
		}, nil
	}

	accessToken := strings.TrimSpace(creds.AccessToken)
	if accessToken == "" {
		return &pb.RefreshTierResponse{
			Success: false,
			Error:   "missing access_token",
		}, nil
	}

	tierID, storageLimit, storageUsage, err := fetchGoogleOneTier(ctx, s.httpClient, accessToken)
	if err != nil {
		return &pb.RefreshTierResponse{
			Success: false,
			Error:   "failed to fetch Google One tier: " + err.Error(),
		}, nil
	}

	updatedExtra := buildTierExtra(req.GetExtraJson(), tierID, storageLimit, storageUsage)
	extraJSON, err := json.Marshal(updatedExtra)
	if err != nil {
		return &pb.RefreshTierResponse{
			Success: false,
			Error:   "failed to marshal updated extra: " + err.Error(),
		}, nil
	}

	return &pb.RefreshTierResponse{
		Success:          true,
		UpdatedExtraJson: extraJSON,
	}, nil
}

// fetchGoogleOneTier queries the Google Drive API to infer the Google One
// tier from the storage quota limit.
func fetchGoogleOneTier(
	ctx context.Context, client *http.Client, accessToken string,
) (tierID string, storageLimit, storageUsage int64, err error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, driveAboutURL, nil)
	if err != nil {
		return "", 0, 0, fmt.Errorf("build request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+accessToken)

	resp, err := client.Do(req)
	if err != nil {
		return "", 0, 0, fmt.Errorf("http request: %w", err)
	}
	defer resp.Body.Close()

	const maxBodySize = 1 << 20
	body, err := io.ReadAll(io.LimitReader(resp.Body, maxBodySize))
	if err != nil {
		return "", 0, 0, fmt.Errorf("read response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return "", 0, 0, fmt.Errorf("drive API status %d: %s", resp.StatusCode, gatewayutil.TruncateBody(body))
	}

	storageLimit, storageUsage, err = parseDriveStorageQuota(body)
	if err != nil {
		return "", 0, 0, err
	}

	tierID = inferGoogleOneTier(storageLimit)
	return tierID, storageLimit, storageUsage, nil
}

// parseDriveStorageQuota parses the Google Drive about response to extract
// storage quota limit and usage.
func parseDriveStorageQuota(body []byte) (limit, usage int64, err error) {
	var result struct {
		StorageQuota struct {
			Limit string `json:"limit"`
			Usage string `json:"usage"`
		} `json:"storageQuota"`
	}
	if err := json.Unmarshal(body, &result); err != nil {
		return 0, 0, fmt.Errorf("parse drive response: %w", err)
	}

	if result.StorageQuota.Limit != "" {
		if val, e := strconv.ParseInt(result.StorageQuota.Limit, 10, 64); e == nil {
			limit = val
		}
	}
	if result.StorageQuota.Usage != "" {
		if val, e := strconv.ParseInt(result.StorageQuota.Usage, 10, 64); e == nil {
			usage = val
		}
	}
	return limit, usage, nil
}

// inferGoogleOneTier maps storage quota limit to a canonical tier ID.
func inferGoogleOneTier(storageBytes int64) string {
	if storageBytes <= 0 {
		return tierGoogleOneUnknown
	}
	if storageBytes > storageTierUnlimited {
		return tierGoogleAIUltra
	}
	if storageBytes >= storageTierAIPremium {
		return tierGoogleAIPro
	}
	if storageBytes >= storageTierFree {
		return tierGoogleOneFree
	}
	return tierGoogleOneUnknown
}

// buildTierExtra merges tier information into the existing extra JSON,
// preserving other fields.
func buildTierExtra(extraJSON []byte, tierID string, storageLimit, storageUsage int64) map[string]any {
	extra := make(map[string]any)
	if len(extraJSON) > 0 {
		_ = json.Unmarshal(extraJSON, &extra)
	}

	extra["drive_storage_limit"] = storageLimit
	extra["drive_storage_usage"] = storageUsage
	extra["drive_tier_updated_at"] = time.Now().Format(time.RFC3339)
	extra["tier_id"] = tierID

	return extra
}
