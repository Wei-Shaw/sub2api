package service

import (
	"archive/tar"
	"bufio"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/redis/go-redis/v9"
)

const (
	updateCacheKey = "update_check_cache"
	updateCacheTTL = 1200 // 20 minutes
	githubRepo     = "Wei-Shaw/sub2api"

	// Security: allowed download domains for updates
	allowedDownloadHost = "github.com"
	allowedAssetHost    = "objects.githubusercontent.com"

	// Security: max download size (500MB)
	maxDownloadSize = 500 * 1024 * 1024
)

// UpdateService handles software updates
type UpdateService struct {
	rdb            *redis.Client
	currentVersion string
	buildType      string // "source" for manual builds, "release" for CI builds
REDACTED

// NewUpdateService creates a new UpdateService
func NewUpdateService(rdb *redis.Client, version, buildType string) *UpdateService {
	return &UpdateService{
		rdb:            rdb,
		currentVersion: version,
		buildType:      buildType,
REDACTED
REDACTED

// UpdateInfo contains update information
type UpdateInfo struct {
	CurrentVersion string       `json:"current_version"`
	LatestVersion  string       `json:"latest_version"`
	HasUpdate      bool         `json:"has_update"`
	ReleaseInfo    *ReleaseInfo `json:"release_info,omitempty"`
	Cached         bool         `json:"cached"`
	Warning        string       `json:"warning,omitempty"`
	BuildType      string       `json:"build_type"` // "source" or "release"
REDACTED

// ReleaseInfo contains GitHub release details
type ReleaseInfo struct {
	Name        string  `json:"name"`
	Body        string  `json:"body"`
	PublishedAt string  `json:"published_at"`
	HtmlURL     string  `json:"html_url"`
	Assets      []Asset `json:"assets,omitempty"`
REDACTED

// Asset represents a release asset
type Asset struct {
	Name        string `json:"name"`
	DownloadURL string `json:"download_url"`
	Size        int64  `json:"size"`
REDACTED

// GitHubRelease represents GitHub API response
type GitHubRelease struct {
	TagName     string        `json:"tag_name"`
	Name        string        `json:"name"`
	Body        string        `json:"body"`
	PublishedAt string        `json:"published_at"`
	HtmlUrl     string        `json:"html_url"`
	Assets      []GitHubAsset `json:"assets"`
REDACTED

type GitHubAsset struct {
	Name               string `json:"name"`
	BrowserDownloadUrl string `json:"browser_download_url"`
	Size               int64  `json:"size"`
REDACTED

// CheckUpdate checks for available updates
func (s *UpdateService) CheckUpdate(ctx context.Context, force bool) (*UpdateInfo, error) {
	// Try cache first
	if !force {
		if cached, err := s.getFromCache(ctx); err == nil && cached != nil {
			return cached, nil
	REDACTED
REDACTED

	// Fetch from GitHub
	info, err := s.fetchLatestRelease(ctx)
	if err != nil {
		// Return cached on error
		if cached, cacheErr := s.getFromCache(ctx); cacheErr == nil && cached != nil {
			cached.Warning = "Using cached data: " + err.Error()
			return cached, nil
	REDACTED
		return &UpdateInfo{
			CurrentVersion: s.currentVersion,
			LatestVersion:  s.currentVersion,
			HasUpdate:      false,
			Warning:        err.Error(),
			BuildType:      s.buildType,
	REDACTED, nil
REDACTED

	// Cache result
	s.saveToCache(ctx, info)
	return info, nil
REDACTED

// PerformUpdate downloads and applies the update
func (s *UpdateService) PerformUpdate(ctx context.Context) error {
	info, err := s.CheckUpdate(ctx, true)
	if err != nil {
		return err
REDACTED

	if !info.HasUpdate {
		return fmt.Errorf("no update available")
REDACTED

	// Find matching archive and checksum for current platform
	archiveName := s.getArchiveName()
	var downloadURL string
	var checksumURL string

	for _, asset := range info.ReleaseInfo.Assets {
		if strings.Contains(asset.Name, archiveName) && !strings.HasSuffix(asset.Name, ".txt") {
			downloadURL = asset.DownloadURL
	REDACTED
		if asset.Name == "checksums.txt" {
			checksumURL = asset.DownloadURL
	REDACTED
REDACTED

	if downloadURL == "" {
		return fmt.Errorf("no compatible release found for %s/%s", runtime.GOOS, runtime.GOARCH)
REDACTED

	// SECURITY: Validate download URL is from trusted domain
	if err := validateDownloadURL(downloadURL); err != nil {
		return fmt.Errorf("invalid download URL: %w", err)
REDACTED
	if checksumURL != "" {
		if err := validateDownloadURL(checksumURL); err != nil {
			return fmt.Errorf("invalid checksum URL: %w", err)
	REDACTED
REDACTED

	// Get current executable path
	exePath, err := os.Executable()
	if err != nil {
		return fmt.Errorf("failed to get executable path: %w", err)
REDACTED
	exePath, err = filepath.EvalSymlinks(exePath)
	if err != nil {
		return fmt.Errorf("failed to resolve symlinks: %w", err)
REDACTED

	// Create temp directory for extraction
	tempDir, err := os.MkdirTemp("", "sub2api-update-*")
	if err != nil {
		return fmt.Errorf("failed to create temp dir: %w", err)
REDACTED
	defer os.RemoveAll(tempDir)

	// Download archive
	archivePath := filepath.Join(tempDir, filepath.Base(downloadURL))
	if err := s.downloadFile(ctx, downloadURL, archivePath); err != nil {
		return fmt.Errorf("download failed: %w", err)
REDACTED

	// Verify checksum if available
	if checksumURL != "" {
		if err := s.verifyChecksum(ctx, archivePath, checksumURL); err != nil {
			return fmt.Errorf("checksum verification failed: %w", err)
	REDACTED
REDACTED

	// Extract binary from archive
	newBinaryPath := filepath.Join(tempDir, "sub2api")
	if err := s.extractBinary(archivePath, newBinaryPath); err != nil {
		return fmt.Errorf("extraction failed: %w", err)
REDACTED

	// Backup current binary
	backupFile := exePath + ".backup"
	if err := os.Rename(exePath, backupFile); err != nil {
		return fmt.Errorf("backup failed: %w", err)
REDACTED

	// Replace with new binary
	if err := copyFile(newBinaryPath, exePath); err != nil {
		os.Rename(backupFile, exePath)
		return fmt.Errorf("replace failed: %w", err)
REDACTED

	// Make executable
	if err := os.Chmod(exePath, 0755); err != nil {
		return fmt.Errorf("chmod failed: %w", err)
REDACTED

	return nil
REDACTED

// Rollback restores the previous version
func (s *UpdateService) Rollback() error {
	exePath, err := os.Executable()
	if err != nil {
		return fmt.Errorf("failed to get executable path: %w", err)
REDACTED
	exePath, err = filepath.EvalSymlinks(exePath)
	if err != nil {
		return fmt.Errorf("failed to resolve symlinks: %w", err)
REDACTED

	backupFile := exePath + ".backup"
	if _, err := os.Stat(backupFile); os.IsNotExist(err) {
		return fmt.Errorf("no backup found")
REDACTED

	// Replace current with backup
	if err := os.Rename(backupFile, exePath); err != nil {
		return fmt.Errorf("rollback failed: %w", err)
REDACTED

	return nil
REDACTED

// RestartService triggers a service restart via systemd
func (s *UpdateService) RestartService() error {
	if runtime.GOOS != "linux" {
		return fmt.Errorf("systemd restart only available on Linux")
REDACTED

	// Try direct systemctl first (works if running as root or with proper permissions)
	cmd := exec.Command("systemctl", "restart", "sub2api")
	if err := cmd.Run(); err != nil {
		// Try with sudo (requires NOPASSWD sudoers entry)
		sudoCmd := exec.Command("sudo", "systemctl", "restart", "sub2api")
		if sudoErr := sudoCmd.Run(); sudoErr != nil {
			return fmt.Errorf("systemctl restart failed: %w (sudo also failed: %v)", err, sudoErr)
	REDACTED
REDACTED
	return nil
REDACTED

func (s *UpdateService) fetchLatestRelease(ctx context.Context) (*UpdateInfo, error) {
	url := fmt.Sprintf("https://api.github.com/repos/%s/releases/latest", githubRepo)

	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return nil, err
REDACTED
	req.Header.Set("Accept", "application/vnd.github.v3+json")
	req.Header.Set("User-Agent", "Sub2API-Updater")

	client := &http.Client{Timeout: 30 * time.SecondREDACTED
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
REDACTED
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusNotFound {
		return &UpdateInfo{
			CurrentVersion: s.currentVersion,
			LatestVersion:  s.currentVersion,
			HasUpdate:      false,
			Warning:        "No releases found",
			BuildType:      s.buildType,
	REDACTED, nil
REDACTED

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("GitHub API returned %d", resp.StatusCode)
REDACTED

	var release GitHubRelease
	if err := json.NewDecoder(resp.Body).Decode(&release); err != nil {
		return nil, err
REDACTED

	latestVersion := strings.TrimPrefix(release.TagName, "v")

	assets := make([]Asset, len(release.Assets))
	for i, a := range release.Assets {
		assets[i] = Asset{
			Name:        a.Name,
			DownloadURL: a.BrowserDownloadUrl,
			Size:        a.Size,
	REDACTED
REDACTED

	return &UpdateInfo{
		CurrentVersion: s.currentVersion,
		LatestVersion:  latestVersion,
		HasUpdate:      compareVersions(s.currentVersion, latestVersion) < 0,
		ReleaseInfo: &ReleaseInfo{
			Name:        release.Name,
			Body:        release.Body,
			PublishedAt: release.PublishedAt,
			HtmlURL:     release.HtmlUrl,
			Assets:      assets,
	REDACTED,
		Cached:    false,
		BuildType: s.buildType,
REDACTED, nil
REDACTED

func (s *UpdateService) downloadFile(ctx context.Context, downloadURL, dest string) error {
	req, err := http.NewRequestWithContext(ctx, "GET", downloadURL, nil)
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
	if resp.ContentLength > maxDownloadSize {
		return fmt.Errorf("file too large: %d bytes (max %d)", resp.ContentLength, maxDownloadSize)
REDACTED

	out, err := os.Create(dest)
	if err != nil {
		return err
REDACTED
	defer out.Close()

	// SECURITY: Use LimitReader to enforce max download size even if Content-Length is missing/wrong
	limited := io.LimitReader(resp.Body, maxDownloadSize+1)
	written, err := io.Copy(out, limited)
	if err != nil {
		return err
REDACTED

	// Check if we hit the limit (downloaded more than maxDownloadSize)
	if written > maxDownloadSize {
		os.Remove(dest) // Clean up partial file
		return fmt.Errorf("download exceeded maximum size of %d bytes", maxDownloadSize)
REDACTED

	return nil
REDACTED

func (s *UpdateService) getArchiveName() string {
	osName := runtime.GOOS
	arch := runtime.GOARCH
	return fmt.Sprintf("%s_%s", osName, arch)
REDACTED

// validateDownloadURL checks if the URL is from an allowed domain
// SECURITY: This prevents SSRF and ensures downloads only come from trusted GitHub domains
func validateDownloadURL(rawURL string) error {
	parsedURL, err := url.Parse(rawURL)
	if err != nil {
		return fmt.Errorf("invalid URL: %w", err)
REDACTED

	// Must be HTTPS
	if parsedURL.Scheme != "https" {
		return fmt.Errorf("only HTTPS URLs are allowed")
REDACTED

	// Check against allowed hosts
	host := parsedURL.Host
	// GitHub release URLs can be from github.com or objects.githubusercontent.com
	if host != allowedDownloadHost &&
		!strings.HasSuffix(host, "."+allowedDownloadHost) &&
		host != allowedAssetHost &&
		!strings.HasSuffix(host, "."+allowedAssetHost) {
		return fmt.Errorf("download from untrusted host: %s", host)
REDACTED

	return nil
REDACTED

func (s *UpdateService) verifyChecksum(ctx context.Context, filePath, checksumURL string) error {
	// Download checksums file
	req, err := http.NewRequestWithContext(ctx, "GET", checksumURL, nil)
	if err != nil {
		return err
REDACTED

	client := &http.Client{Timeout: 30 * time.SecondREDACTED
	resp, err := client.Do(req)
	if err != nil {
		return err
REDACTED
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("failed to download checksums: %d", resp.StatusCode)
REDACTED

	// Calculate file hash
	f, err := os.Open(filePath)
	if err != nil {
		return err
REDACTED
	defer f.Close()

	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return err
REDACTED
	actualHash := hex.EncodeToString(h.Sum(nil))

	// Find expected hash in checksums file
	fileName := filepath.Base(filePath)
	scanner := bufio.NewScanner(resp.Body)
	for scanner.Scan() {
		line := scanner.Text()
		parts := strings.Fields(line)
		if len(parts) == 2 && parts[1] == fileName {
			if parts[0] == actualHash {
				return nil
		REDACTED
			return fmt.Errorf("checksum mismatch: expected %s, got %s", parts[0], actualHash)
	REDACTED
REDACTED

	return fmt.Errorf("checksum not found for %s", fileName)
REDACTED

func (s *UpdateService) extractBinary(archivePath, destPath string) error {
	f, err := os.Open(archivePath)
	if err != nil {
		return err
REDACTED
	defer f.Close()

	var reader io.Reader = f

	// Handle gzip compression
	if strings.HasSuffix(archivePath, ".gz") || strings.HasSuffix(archivePath, ".tar.gz") || strings.HasSuffix(archivePath, ".tgz") {
		gzr, err := gzip.NewReader(f)
		if err != nil {
			return err
	REDACTED
		defer gzr.Close()
		reader = gzr
REDACTED

	// Handle tar archive
	if strings.Contains(archivePath, ".tar") {
		tr := tar.NewReader(reader)
		for {
			hdr, err := tr.Next()
			if err == io.EOF {
				break
		REDACTED
			if err != nil {
				return err
		REDACTED

			// SECURITY: Prevent Zip Slip / Path Traversal attack
			// Only allow files with safe base names, no directory traversal
			baseName := filepath.Base(hdr.Name)

			// Check for path traversal attempts
			if strings.Contains(hdr.Name, "..") {
				return fmt.Errorf("path traversal attempt detected: %s", hdr.Name)
		REDACTED

			// Validate the entry is a regular file
			if hdr.Typeflag != tar.TypeReg {
				continue // Skip directories and special files
		REDACTED

			// Only extract the specific binary we need
			if baseName == "sub2api" || baseName == "sub2api.exe" {
				// Additional security: limit file size (max 500MB)
				const maxBinarySize = 500 * 1024 * 1024
				if hdr.Size > maxBinarySize {
					return fmt.Errorf("binary too large: %d bytes (max %d)", hdr.Size, maxBinarySize)
			REDACTED

				out, err := os.Create(destPath)
				if err != nil {
					return err
			REDACTED

				// Use LimitReader to prevent decompression bombs
				limited := io.LimitReader(tr, maxBinarySize)
				if _, err := io.Copy(out, limited); err != nil {
					out.Close()
					return err
			REDACTED
				out.Close()
				return nil
		REDACTED
	REDACTED
		return fmt.Errorf("binary not found in archive")
REDACTED

	// Direct copy for non-tar files (with size limit)
	const maxBinarySize = 500 * 1024 * 1024
	out, err := os.Create(destPath)
	if err != nil {
		return err
REDACTED
	defer out.Close()

	limited := io.LimitReader(reader, maxBinarySize)
	_, err = io.Copy(out, limited)
	return err
REDACTED

func copyFile(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
REDACTED
	defer in.Close()

	out, err := os.Create(dst)
	if err != nil {
		return err
REDACTED
	defer out.Close()

	_, err = io.Copy(out, in)
	return err
REDACTED

func (s *UpdateService) getFromCache(ctx context.Context) (*UpdateInfo, error) {
	data, err := s.rdb.Get(ctx, updateCacheKey).Result()
	if err != nil {
		return nil, err
REDACTED

	var cached struct {
		Latest      string       `json:"latest"`
		ReleaseInfo *ReleaseInfo `json:"release_info"`
		Timestamp   int64        `json:"timestamp"`
REDACTED
	if err := json.Unmarshal([]byte(data), &cached); err != nil {
		return nil, err
REDACTED

	if time.Now().Unix()-cached.Timestamp > updateCacheTTL {
		return nil, fmt.Errorf("cache expired")
REDACTED

	return &UpdateInfo{
		CurrentVersion: s.currentVersion,
		LatestVersion:  cached.Latest,
		HasUpdate:      compareVersions(s.currentVersion, cached.Latest) < 0,
		ReleaseInfo:    cached.ReleaseInfo,
		Cached:         true,
		BuildType:      s.buildType,
REDACTED, nil
REDACTED

func (s *UpdateService) saveToCache(ctx context.Context, info *UpdateInfo) {
	cacheData := struct {
		Latest      string       `json:"latest"`
		ReleaseInfo *ReleaseInfo `json:"release_info"`
		Timestamp   int64        `json:"timestamp"`
REDACTED{
		Latest:      info.LatestVersion,
		ReleaseInfo: info.ReleaseInfo,
		Timestamp:   time.Now().Unix(),
REDACTED

	data, _ := json.Marshal(cacheData)
	s.rdb.Set(ctx, updateCacheKey, data, time.Duration(updateCacheTTL)*time.Second)
REDACTED

// compareVersions compares two semantic versions
func compareVersions(current, latest string) int {
	currentParts := parseVersion(current)
	latestParts := parseVersion(latest)

	for i := 0; i < 3; i++ {
		if currentParts[i] < latestParts[i] {
			return -1
	REDACTED
		if currentParts[i] > latestParts[i] {
			return 1
	REDACTED
REDACTED
	return 0
REDACTED

func parseVersion(v string) [3]int {
	v = strings.TrimPrefix(v, "v")
	parts := strings.Split(v, ".")
	result := [3]int{0, 0, 0REDACTED
	for i := 0; i < len(parts) && i < 3; i++ {
		fmt.Sscanf(parts[i], "%d", &result[i])
REDACTED
	return result
REDACTED
