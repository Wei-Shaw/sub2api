package geminicli

import (
	"context"
	"encoding/json"
	"fmt"
	"math/rand"
	"net/http"
	"strconv"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/httpclient"
)

// DriveStorageInfo represents Google Drive storage quota information
type DriveStorageInfo struct {
	Limit int64 `json:"limit"` // Storage limit in bytes
	Usage int64 `json:"usage"` // Current usage in bytes
REDACTED

// DriveClient interface for Google Drive API operations
type DriveClient interface {
	GetStorageQuota(ctx context.Context, accessToken, proxyURL string) (*DriveStorageInfo, error)
REDACTED

type driveClient struct{REDACTED

// NewDriveClient creates a new Drive API client
func NewDriveClient() DriveClient {
	return &driveClient{REDACTED
REDACTED

// GetStorageQuota fetches storage quota from Google Drive API
func (c *driveClient) GetStorageQuota(ctx context.Context, accessToken, proxyURL string) (*DriveStorageInfo, error) {
	const driveAPIURL = "https://www.googleapis.com/drive/v3/about?fields=storageQuota"

	req, err := http.NewRequestWithContext(ctx, "GET", driveAPIURL, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
REDACTED

	req.Header.Set("Authorization", "Bearer "+accessToken)

	// Get HTTP client with proxy support
	client, err := httpclient.GetClient(httpclient.Options{
		ProxyURL: proxyURL,
		Timeout:  10 * time.Second,
REDACTED)
	if err != nil {
		return nil, fmt.Errorf("failed to create HTTP client: %w", err)
REDACTED

	sleepWithContext := func(d time.Duration) error {
		timer := time.NewTimer(d)
		defer timer.Stop()
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-timer.C:
			return nil
	REDACTED
REDACTED

	// Retry logic with exponential backoff (+ jitter) for rate limits and transient failures
	var resp *http.Response
	maxRetries := 3
	rng := rand.New(rand.NewSource(time.Now().UnixNano()))
	for attempt := 0; attempt < maxRetries; attempt++ {
		if ctx.Err() != nil {
			return nil, fmt.Errorf("request cancelled: %w", ctx.Err())
	REDACTED

		resp, err = client.Do(req)
		if err != nil {
			// Network error retry
			if attempt < maxRetries-1 {
				backoff := time.Duration(1<<uint(attempt)) * time.Second
				jitter := time.Duration(rng.Intn(1000)) * time.Millisecond
				if err := sleepWithContext(backoff + jitter); err != nil {
					return nil, fmt.Errorf("request cancelled: %w", err)
			REDACTED
				continue
		REDACTED
			return nil, fmt.Errorf("network error after %d attempts: %w", maxRetries, err)
	REDACTED

		// Success
		if resp.StatusCode == http.StatusOK {
			break
	REDACTED

		// Retry 429, 500, 502, 503 with exponential backoff + jitter
		if (resp.StatusCode == http.StatusTooManyRequests ||
			resp.StatusCode == http.StatusInternalServerError ||
			resp.StatusCode == http.StatusBadGateway ||
			resp.StatusCode == http.StatusServiceUnavailable) && attempt < maxRetries-1 {
			if err := func() error {
				defer func() { _ = resp.Body.Close() REDACTED()
				backoff := time.Duration(1<<uint(attempt)) * time.Second
				jitter := time.Duration(rng.Intn(1000)) * time.Millisecond
				return sleepWithContext(backoff + jitter)
		REDACTED(); err != nil {
				return nil, fmt.Errorf("request cancelled: %w", err)
		REDACTED
			continue
	REDACTED

		break
REDACTED

	if resp == nil {
		return nil, fmt.Errorf("request failed: no response received")
REDACTED

	if resp.StatusCode != http.StatusOK {
		_ = resp.Body.Close()
		statusText := http.StatusText(resp.StatusCode)
		if statusText == "" {
			statusText = resp.Status
	REDACTED
		fmt.Printf("[DriveClient] Drive API error: status=%d, msg=%s\n", resp.StatusCode, statusText)
		// 只返回通用错误
		return nil, fmt.Errorf("drive API error: status %d", resp.StatusCode)
REDACTED

	defer func() { _ = resp.Body.Close() REDACTED()

	// Parse response
	var result struct {
		StorageQuota struct {
			Limit string `json:"limit"` // Can be string or number
			Usage string `json:"usage"`
	REDACTED `json:"storageQuota"`
REDACTED

	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
REDACTED

	// Parse limit and usage (handle both string and number formats)
	var limit, usage int64
	if result.StorageQuota.Limit != "" {
		if val, err := strconv.ParseInt(result.StorageQuota.Limit, 10, 64); err == nil {
			limit = val
	REDACTED
REDACTED
	if result.StorageQuota.Usage != "" {
		if val, err := strconv.ParseInt(result.StorageQuota.Usage, 10, 64); err == nil {
			usage = val
	REDACTED
REDACTED

	return &DriveStorageInfo{
		Limit: limit,
		Usage: usage,
REDACTED, nil
REDACTED
