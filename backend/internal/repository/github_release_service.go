package repository

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/httpclient"
	"github.com/Wei-Shaw/sub2api/internal/service"
)

type githubReleaseClient struct {
	httpClient         *http.Client
	downloadHTTPClient *http.Client
	updateGitHubToken  string
REDACTED

type githubReleaseClientError struct {
	err error
REDACTED

// NewGitHubReleaseClient 创建 GitHub Release 客户端
// proxyURL 为空时直连 GitHub，支持 http/https/socks5/socks5h 协议
// 代理配置失败时行为由 allowDirectOnProxyError 控制：
//   - false（默认）：返回错误占位客户端，禁止回退到直连
//   - true：回退到直连（仅限管理员显式开启）
func NewGitHubReleaseClient(proxyURL string, allowDirectOnProxyError bool) service.GitHubReleaseClient {
	// 安全说明：httpclient.GetClient 的错误链（url.Parse / proxyutil）不含明文代理凭据，
	// 但仍通过 slog 仅在服务端日志记录，不会暴露给 HTTP 响应。
	sharedClient, err := httpclient.GetClient(httpclient.Options{
		Timeout:  30 * time.Second,
		ProxyURL: proxyURL,
REDACTED)
	if err != nil {
		if strings.TrimSpace(proxyURL) != "" && !allowDirectOnProxyError {
			slog.Warn("proxy client init failed, all requests will fail", "service", "github_release", "error", err)
			return &githubReleaseClientError{err: fmt.Errorf("proxy client init failed and direct fallback is disabled; set security.proxy_fallback.allow_direct_on_error=true to allow fallback: %w", err)REDACTED
	REDACTED
		sharedClient = &http.Client{Timeout: 30 * time.SecondREDACTED
REDACTED
	apiClient := cloneHTTPClient(sharedClient)
	apiClient.CheckRedirect = githubAPICheckRedirect(apiClient.CheckRedirect)

	// 下载客户端需要更长的超时时间
	downloadClient, err := httpclient.GetClient(httpclient.Options{
		Timeout:  10 * time.Minute,
		ProxyURL: proxyURL,
REDACTED)
	if err != nil {
		if strings.TrimSpace(proxyURL) != "" && !allowDirectOnProxyError {
			slog.Warn("proxy download client init failed, all requests will fail", "service", "github_release", "error", err)
			return &githubReleaseClientError{err: fmt.Errorf("proxy client init failed and direct fallback is disabled; set security.proxy_fallback.allow_direct_on_error=true to allow fallback: %w", err)REDACTED
	REDACTED
		downloadClient = &http.Client{Timeout: 10 * time.MinuteREDACTED
REDACTED
	downloadClient = cloneHTTPClient(downloadClient)

	return &githubReleaseClient{
		httpClient:         apiClient,
		downloadHTTPClient: downloadClient,
		updateGitHubToken:  os.Getenv("UPDATE_GITHUB_TOKEN"),
REDACTED
REDACTED

func cloneHTTPClient(client *http.Client) *http.Client {
	cloned := *client
	return &cloned
REDACTED

func isGitHubAPIURL(url *url.URL) bool {
	return url != nil && strings.EqualFold(url.Scheme, "https") && url.User == nil &&
		strings.EqualFold(url.Host, "api.github.com")
REDACTED

func githubAPICheckRedirect(previous func(*http.Request, []*http.Request) error) func(*http.Request, []*http.Request) error {
	return func(req *http.Request, via []*http.Request) error {
		if !isGitHubAPIURL(req.URL) {
			req.Header.Del("Authorization")
	REDACTED
		if previous != nil {
			return previous(req, via)
	REDACTED
		return nil
REDACTED
REDACTED

func (c *githubReleaseClient) newAPIRequest(ctx context.Context, url string) (*http.Request, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
REDACTED
	req.Header.Set("Accept", "application/vnd.github.v3+json")
	req.Header.Set("User-Agent", "Sub2API-Updater")
	if c.updateGitHubToken != "" && isGitHubAPIURL(req.URL) {
		req.Header.Set("Authorization", "Bearer "+c.updateGitHubToken)
REDACTED
	return req, nil
REDACTED

func (c *githubReleaseClientError) FetchLatestRelease(ctx context.Context, repo string) (*service.GitHubRelease, error) {
	return nil, c.err
REDACTED

func (c *githubReleaseClientError) FetchRecentReleases(ctx context.Context, repo string, perPage int) ([]*service.GitHubRelease, error) {
	return nil, c.err
REDACTED

func (c *githubReleaseClientError) DownloadFile(ctx context.Context, url, dest string, maxSize int64) error {
	return c.err
REDACTED

func (c *githubReleaseClientError) FetchChecksumFile(ctx context.Context, url string) ([]byte, error) {
	return nil, c.err
REDACTED

func (c *githubReleaseClient) FetchLatestRelease(ctx context.Context, repo string) (*service.GitHubRelease, error) {
	url := fmt.Sprintf("https://api.github.com/repos/%s/releases/latest", repo)

	req, err := c.newAPIRequest(ctx, url)
	if err != nil {
		return nil, err
REDACTED

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, err
REDACTED
	defer func() { _ = resp.Body.Close() REDACTED()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("GitHub API returned %d", resp.StatusCode)
REDACTED

	var release service.GitHubRelease
	if err := json.NewDecoder(resp.Body).Decode(&release); err != nil {
		return nil, err
REDACTED

	return &release, nil
REDACTED

func (c *githubReleaseClient) FetchRecentReleases(ctx context.Context, repo string, perPage int) ([]*service.GitHubRelease, error) {
	if perPage <= 0 {
		perPage = 10
REDACTED
	if perPage > 100 {
		perPage = 100 // GitHub API hard limit
REDACTED
	url := fmt.Sprintf("https://api.github.com/repos/%s/releases?per_page=%d", repo, perPage)

	req, err := c.newAPIRequest(ctx, url)
	if err != nil {
		return nil, err
REDACTED

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, err
REDACTED
	defer func() { _ = resp.Body.Close() REDACTED()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("GitHub API returned %d", resp.StatusCode)
REDACTED

	var releases []*service.GitHubRelease
	if err := json.NewDecoder(resp.Body).Decode(&releases); err != nil {
		return nil, err
REDACTED

	return releases, nil
REDACTED

func (c *githubReleaseClient) DownloadFile(ctx context.Context, url, dest string, maxSize int64) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return err
REDACTED

	// 使用预配置的下载客户端（已包含代理配置）
	resp, err := c.downloadHTTPClient.Do(req)
	if err != nil {
		return err
REDACTED
	defer func() { _ = resp.Body.Close() REDACTED()

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

	// SECURITY: Use LimitReader to enforce max download size even if Content-Length is missing/wrong
	limited := io.LimitReader(resp.Body, maxSize+1)
	written, err := io.Copy(out, limited)

	// Close file before attempting to remove (required on Windows)
	_ = out.Close()

	if err != nil {
		_ = os.Remove(dest) // Clean up partial file (best-effort)
		return err
REDACTED

	// Check if we hit the limit (downloaded more than maxSize)
	if written > maxSize {
		_ = os.Remove(dest) // Clean up partial file (best-effort)
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
	defer func() { _ = resp.Body.Close() REDACTED()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("HTTP %d", resp.StatusCode)
REDACTED

	return io.ReadAll(resp.Body)
REDACTED
