package repository

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"time"

	"sub2api/internal/service"
)

type githubReleaseClient struct {
	httpClient *http.Client
REDACTED

func NewGitHubReleaseClient() service.GitHubReleaseClient {
	return &githubReleaseClient{
		httpClient: &http.Client{
			Timeout: 30 * time.Second,
	REDACTED,
REDACTED
REDACTED

func (c *githubReleaseClient) FetchLatestRelease(ctx context.Context, repo string) (*service.GitHubRelease, error) {
	url := fmt.Sprintf("https://api.github.com/repos/%s/releases/latest", repo)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
REDACTED
	req.Header.Set("Accept", "application/vnd.github.v3+json")
	req.Header.Set("User-Agent", "Sub2API-Updater")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, err
REDACTED
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("GitHub API returned %d", resp.StatusCode)
REDACTED

	var release service.GitHubRelease
	if err := json.NewDecoder(resp.Body).Decode(&release); err != nil {
		return nil, err
REDACTED

	return &release, nil
REDACTED

func (c *githubReleaseClient) DownloadFile(ctx context.Context, url, dest string, maxSize int64) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return err
REDACTED

	client := &http.Client{Timeout: 10 * time.MinuteREDACTED
	resp, err := client.Do(req)
	if err != nil {
		return err
REDACTED
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("download returned %d", resp.StatusCode)
REDACTED

	// SECURITY: Check Content-Length if available
	if resp.ContentLength > maxSize {
		return fmt.Errorf("file too large: %d bytes (max %d)", resp.ContentLength, maxSize)
REDACTED

	out, err := os.Create(dest)
	if err != nil {
		return err
REDACTED
	defer out.Close()

	// SECURITY: Use LimitReader to enforce max download size even if Content-Length is missing/wrong
	limited := io.LimitReader(resp.Body, maxSize+1)
	written, err := io.Copy(out, limited)
	if err != nil {
		return err
REDACTED

	// Check if we hit the limit (downloaded more than maxSize)
	if written > maxSize {
		os.Remove(dest) // Clean up partial file
		return fmt.Errorf("download exceeded maximum size of %d bytes", maxSize)
REDACTED

	return nil
REDACTED

func (c *githubReleaseClient) FetchChecksumFile(ctx context.Context, url string) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
REDACTED

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, err
REDACTED
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("HTTP %d", resp.StatusCode)
REDACTED

	return io.ReadAll(resp.Body)
REDACTED
