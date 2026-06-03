package service

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/logger"
)

// ThemeMetadata represents the parsed sub2api-theme.json
type ThemeMetadata struct {
	Name        string            `json:"name"`
	Short       string            `json:"short"`
	Version     string            `json:"version"`
	Author      string            `json:"author"`
	Description string            `json:"description"`
	Preview     string            `json:"preview"`
	MainCSS     string            `json:"main_css"`
	Config      []ThemeConfigItem `json:"config,omitempty"`
}

// ThemeConfigItem declares a configurable option exposed to the admin UI
type ThemeConfigItem struct {
	Key         string              `json:"key"`
	Label       string              `json:"label"`
	Type        string              `json:"type"`
	Default     string              `json:"default"`
	Options     []ThemeConfigOption `json:"options,omitempty"`
	Description string              `json:"description,omitempty"`
}

// ThemeConfigOption is a select option
type ThemeConfigOption struct {
	Label string `json:"label"`
	Value string `json:"value"`
}

// Theme represents a fully installed theme with metadata
type Theme struct {
	Metadata    ThemeMetadata     `json:"metadata"`
	InstalledAt time.Time         `json:"installed_at"`
	IsActive    bool              `json:"is_active"`
	Config      map[string]string `json:"config,omitempty"`
}

const (
	themeActiveKey    = "active_theme"
	themeConfigPrefix = "theme_config_"

	ThemeMaxZipSize   = 10 * 1024 * 1024 // 10MB
	themeMaxCSSSize   = 512 * 1024       // 512KB
	themeMaxAssetSize = 2 * 1024 * 1024  // 2MB per file
	themeMaxTotalSize = 20 * 1024 * 1024 // 20MB total extracted
)

var themeShortRegex = regexp.MustCompile(`^[a-z0-9][a-z0-9_-]{0,31}$`)

// Pre-compiled regexes for CSS sanitization
var (
	cssCommentRe = regexp.MustCompile(`(?s)/\*.*?\*/`)
	// Match url() with single-quoted, double-quoted, or unquoted argument
	cssURLRe = regexp.MustCompile(`(?i)url\s*\(\s*(?:'([^']*)'|"([^"]*)"|([^)]*))\s*\)`)
	// Match entire @import url(...) statement (including the @import keyword)
	cssImportRe = regexp.MustCompile(`(?i)@import\s+url\s*\(\s*(?:'[^']*'|"[^"]*"|[^)]*)\s*\)\s*;?`)
)

var allowedThemeExtensions = map[string]bool{
	".css":   true,
	".woff":  true,
	".woff2": true,
	".ttf":   true,
	".otf":   true,
	".png":   true,
	".jpg":   true,
	".jpeg":  true,
	".gif":   true,
	".svg":   true,
	".ico":   true,
	".webp":  true,
	".json":  true,
}

// ThemeService manages themes
type ThemeService struct {
	settingRepo SettingRepository
	dataDir     string
	onUpdate    func()
}

// NewThemeService creates a new ThemeService
func NewThemeService(settingRepo SettingRepository) *ThemeService {
	return &ThemeService{
		settingRepo: settingRepo,
		dataDir:     filepath.Join("data", "themes"),
	}
}

// SetOnUpdateCallback sets the callback for when active theme changes
func (s *ThemeService) SetOnUpdateCallback(callback func()) {
	s.onUpdate = callback
}

// InstallFromZip extracts a zip archive, validates it, and installs the theme
func (s *ThemeService) InstallFromZip(ctx context.Context, zipData []byte) (*Theme, error) {
	if len(zipData) > ThemeMaxZipSize {
		return nil, fmt.Errorf("zip file too large (max %dMB)", ThemeMaxZipSize/1024/1024)
	}

	// Read zip
	reader, err := zip.NewReader(bytes.NewReader(zipData), int64(len(zipData)))
	if err != nil {
		return nil, fmt.Errorf("invalid zip file: %w", err)
	}

	// Find and parse sub2api-theme.json, extract all files to temp dir
	tmpDir, err := os.MkdirTemp("", "sub2api-theme-*")
	if err != nil {
		return nil, fmt.Errorf("failed to create temp dir: %w", err)
	}
	defer func() { _ = os.RemoveAll(tmpDir) }()

	var metadata *ThemeMetadata
	var totalSize int64

	for _, f := range reader.File {
		// Check total size
		totalSize += int64(f.UncompressedSize64)
		if totalSize > themeMaxTotalSize {
			return nil, fmt.Errorf("extracted content too large (max %dMB)", themeMaxTotalSize/1024/1024)
		}

		// Sanitize path
		cleanName := filepath.Clean(f.Name)
		if strings.Contains(cleanName, "..") || strings.HasPrefix(cleanName, "/") || strings.HasPrefix(cleanName, "\\") {
			return nil, fmt.Errorf("invalid file path in zip: %s", f.Name)
		}

		// Determine the output path - strip leading directory if present
		outName := cleanName
		// Some zips have a root directory (e.g., "theme-main/style.css"), strip it
		parts := strings.SplitN(outName, "/", 2)
		if len(parts) == 2 && parts[0] != "" && parts[1] != "" {
			outName = parts[1]
		}
		if outName == "" {
			continue
		}

		// Parse metadata
		if filepath.Base(outName) == "sub2api-theme.json" {
			rc, err := f.Open()
			if err != nil {
				return nil, fmt.Errorf("failed to read theme metadata: %w", err)
			}
			data, err := io.ReadAll(rc)
			_ = rc.Close()
			if err != nil {
				return nil, fmt.Errorf("failed to read theme metadata: %w", err)
			}
			var m ThemeMetadata
			if err := json.Unmarshal(data, &m); err != nil {
				return nil, fmt.Errorf("invalid theme metadata: %w", err)
			}
			metadata = &m
			// Write metadata to temp dir
			destPath := filepath.Join(tmpDir, outName)
			if err := os.MkdirAll(filepath.Dir(destPath), 0o755); err != nil {
				return nil, fmt.Errorf("failed to create dir: %w", err)
			}
			if err := os.WriteFile(destPath, data, 0o644); err != nil {
				return nil, fmt.Errorf("failed to write metadata: %w", err)
			}
			continue
		}

		// Validate file extension
		ext := strings.ToLower(filepath.Ext(outName))
		if !allowedThemeExtensions[ext] {
			return nil, fmt.Errorf("file type not allowed: %s (%s)", outName, ext)
		}

		// Check individual file size
		if int64(f.UncompressedSize64) > themeMaxAssetSize {
			return nil, fmt.Errorf("file too large: %s (max %dMB)", outName, themeMaxAssetSize/1024/1024)
		}

		// Extract file
		destPath := filepath.Join(tmpDir, outName)
		if err := os.MkdirAll(filepath.Dir(destPath), 0o755); err != nil {
			return nil, fmt.Errorf("failed to create dir: %w", err)
		}

		rc, err := f.Open()
		if err != nil {
			return nil, fmt.Errorf("failed to open %s: %w", f.Name, err)
		}

		data, err := io.ReadAll(rc)
		_ = rc.Close()
		if err != nil {
			return nil, fmt.Errorf("failed to read %s: %w", f.Name, err)
		}

		// Sanitize CSS files
		if ext == ".css" {
			data, err = sanitizeCSS(data)
			if err != nil {
				return nil, fmt.Errorf("CSS sanitization failed for %s: %w", outName, err)
			}
			if len(data) > themeMaxCSSSize {
				return nil, fmt.Errorf("CSS file too large: %s (max %dKB)", outName, themeMaxCSSSize/1024)
			}
		}

		if err := os.WriteFile(destPath, data, 0o644); err != nil {
			return nil, fmt.Errorf("failed to write %s: %w", outName, err)
		}
	}

	if metadata == nil {
		return nil, fmt.Errorf("sub2api-theme.json not found in zip")
	}

	// Validate metadata
	if err := validateThemeMetadata(metadata); err != nil {
		return nil, err
	}

	// Set defaults
	if metadata.MainCSS == "" {
		metadata.MainCSS = "style.css"
	}

	// Move from temp to final location
	themeDir := filepath.Join(s.dataDir, metadata.Short)
	if err := os.RemoveAll(themeDir); err != nil && !os.IsNotExist(err) {
		return nil, fmt.Errorf("failed to clean existing theme: %w", err)
	}
	if err := os.MkdirAll(s.dataDir, 0o755); err != nil {
		return nil, fmt.Errorf("failed to create themes dir: %w", err)
	}
	if err := os.Rename(tmpDir, themeDir); err != nil {
		// Fallback: copy
		if err := copyDir(tmpDir, themeDir); err != nil {
			return nil, fmt.Errorf("failed to install theme: %w", err)
		}
	}

	// Check if this theme was previously active
	activeShort := s.getActiveThemeShort(ctx)
	isActive := activeShort == metadata.Short

	theme := &Theme{
		Metadata:    *metadata,
		InstalledAt: time.Now(),
		IsActive:    isActive,
	}

	// Load config if theme has config items and is active
	if isActive && len(metadata.Config) > 0 {
		theme.Config = s.getThemeConfig(ctx, metadata.Short)
	}

	logger.LegacyPrintf("service.theme", "Theme installed: %s (%s)", metadata.Name, metadata.Short)
	return theme, nil
}

// InstallFromGitHub downloads a theme from a GitHub repo URL
func (s *ThemeService) InstallFromGitHub(ctx context.Context, repoURL string) (*Theme, error) {
	owner, repo, err := parseGitHubURL(repoURL)
	if err != nil {
		return nil, err
	}

	// Try main branch first, then master
	for _, branch := range []string{"main", "master"} {
		zipURL := fmt.Sprintf("https://github.com/%s/%s/archive/refs/heads/%s.zip", owner, repo, branch)
		zipData, err := downloadFile(ctx, zipURL)
		if err != nil {
			continue
		}
		return s.InstallFromZip(ctx, zipData)
	}

	return nil, fmt.Errorf("failed to download theme from GitHub (tried main and master branches)")
}

// List returns all installed themes
func (s *ThemeService) List(ctx context.Context) ([]Theme, error) {
	activeShort := s.getActiveThemeShort(ctx)

	entries, err := os.ReadDir(s.dataDir)
	if err != nil {
		if os.IsNotExist(err) {
			return []Theme{}, nil
		}
		return nil, fmt.Errorf("failed to read themes directory: %w", err)
	}

	var themes []Theme
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		theme, err := s.loadThemeFromDir(ctx, entry.Name(), activeShort)
		if err != nil {
			logger.LegacyPrintf("service.theme", "Warning: skipping invalid theme %s: %v", entry.Name(), err)
			continue
		}
		themes = append(themes, *theme)
	}

	return themes, nil
}

// Get returns a specific theme by its short identifier
func (s *ThemeService) Get(ctx context.Context, short string) (*Theme, error) {
	if !themeShortRegex.MatchString(short) {
		return nil, fmt.Errorf("invalid theme identifier")
	}
	activeShort := s.getActiveThemeShort(ctx)
	return s.loadThemeFromDir(ctx, short, activeShort)
}

// GetActiveTheme returns the currently active theme, or nil if none
func (s *ThemeService) GetActiveTheme(ctx context.Context) (*Theme, error) {
	short := s.getActiveThemeShort(ctx)
	if short == "" {
		return nil, nil
	}
	return s.Get(ctx, short)
}

// Activate sets a theme as the active theme
func (s *ThemeService) Activate(ctx context.Context, short string) error {
	if !themeShortRegex.MatchString(short) {
		return fmt.Errorf("invalid theme identifier")
	}

	// Verify theme exists
	metaPath := filepath.Join(s.dataDir, short, "sub2api-theme.json")
	if _, err := os.Stat(metaPath); err != nil {
		return fmt.Errorf("theme not found: %s", short)
	}

	if err := s.settingRepo.Set(ctx, themeActiveKey, short); err != nil {
		return fmt.Errorf("failed to activate theme: %w", err)
	}

	logger.LegacyPrintf("service.theme", "Theme activated: %s", short)

	if s.onUpdate != nil {
		s.onUpdate()
	}
	return nil
}

// Deactivate removes the active theme
func (s *ThemeService) Deactivate(ctx context.Context) error {
	if err := s.settingRepo.Delete(ctx, themeActiveKey); err != nil {
		return fmt.Errorf("failed to deactivate theme: %w", err)
	}

	logger.LegacyPrintf("service.theme", "Theme deactivated")

	if s.onUpdate != nil {
		s.onUpdate()
	}
	return nil
}

// Delete removes a theme from disk
func (s *ThemeService) Delete(ctx context.Context, short string) error {
	if !themeShortRegex.MatchString(short) {
		return fmt.Errorf("invalid theme identifier")
	}

	// Deactivate if active
	activeShort := s.getActiveThemeShort(ctx)
	if activeShort == short {
		if err := s.Deactivate(ctx); err != nil {
			return err
		}
	}

	themeDir := filepath.Join(s.dataDir, short)
	if err := os.RemoveAll(themeDir); err != nil {
		return fmt.Errorf("failed to delete theme: %w", err)
	}

	// Clean up config (best effort)
	_ = s.settingRepo.Delete(ctx, themeConfigPrefix+short)

	logger.LegacyPrintf("service.theme", "Theme deleted: %s", short)
	return nil
}

// UpdateConfig updates theme-specific configuration values
func (s *ThemeService) UpdateConfig(ctx context.Context, short string, config map[string]string) error {
	if !themeShortRegex.MatchString(short) {
		return fmt.Errorf("invalid theme identifier")
	}

	data, err := json.Marshal(config)
	if err != nil {
		return fmt.Errorf("failed to marshal config: %w", err)
	}

	if err := s.settingRepo.Set(ctx, themeConfigPrefix+short, string(data)); err != nil {
		return fmt.Errorf("failed to save theme config: %w", err)
	}

	// Invalidate cache since config affects CSS variables
	if s.onUpdate != nil {
		s.onUpdate()
	}
	return nil
}

// GetActiveThemeShort returns the short ID of the active theme
func (s *ThemeService) GetActiveThemeShort(ctx context.Context) string {
	return s.getActiveThemeShort(ctx)
}

// GetThemeConfigCSS generates CSS custom properties from the active theme's config
func (s *ThemeService) GetThemeConfigCSS(ctx context.Context) string {
	short := s.getActiveThemeShort(ctx)
	if short == "" {
		return ""
	}

	config := s.getThemeConfig(ctx, short)
	if len(config) == 0 {
		return ""
	}

	// Load theme metadata to get config item definitions
	metadata, err := s.loadMetadata(short)
	if err != nil || len(metadata.Config) == 0 {
		return ""
	}

	var css strings.Builder
	_, _ = css.WriteString(":root {\n")
	for _, item := range metadata.Config {
		val, ok := config[item.Key]
		if !ok || val == "" {
			val = item.Default
		}
		if val != "" {
			_, _ = css.WriteString(fmt.Sprintf("  --theme-%s: %s;\n", item.Key, val))
		}
	}
	_, _ = css.WriteString("}\n")
	return css.String()
}

// ServeThemeAsset serves a file from a theme's directory
func (s *ThemeService) ServeThemeAsset(ctx context.Context, short, assetPath string) ([]byte, string, error) {
	if !themeShortRegex.MatchString(short) {
		return nil, "", fmt.Errorf("invalid theme identifier")
	}

	// Clean and validate asset path
	cleaned := filepath.Clean("/" + assetPath)
	cleaned = strings.TrimPrefix(cleaned, "/")

	// Reject empty or directory-only paths
	if cleaned == "" || cleaned == "." || cleaned == ".." {
		return nil, "", fmt.Errorf("invalid asset path")
	}

	// Default to main CSS when path is just the theme root
	if assetPath == "" || assetPath == "/" {
		metadata, err := s.loadMetadata(short)
		if err != nil {
			return nil, "", fmt.Errorf("theme not found")
		}
		cleaned = metadata.MainCSS
		if cleaned == "" {
			cleaned = "style.css"
		}
	}

	// Prevent directory traversal
	if strings.Contains(cleaned, "..") || strings.HasPrefix(cleaned, "/") {
		return nil, "", fmt.Errorf("invalid asset path")
	}

	// Resolve full path and verify it's within the theme directory
	themeDir := filepath.Join(s.dataDir, short)
	fullPath := filepath.Join(themeDir, cleaned)

	// Ensure resolved path is within theme directory
	absThemeDir, _ := filepath.Abs(themeDir)
	absFullPath, _ := filepath.Abs(fullPath)
	if !strings.HasPrefix(absFullPath, absThemeDir) {
		return nil, "", fmt.Errorf("path traversal detected")
	}

	// Verify file exists and is not a directory
	info, err := os.Stat(fullPath)
	if err != nil {
		return nil, "", fmt.Errorf("file not found")
	}
	if info.IsDir() {
		return nil, "", fmt.Errorf("is a directory")
	}

	content, err := os.ReadFile(fullPath)
	if err != nil {
		return nil, "", fmt.Errorf("failed to read file: %w", err)
	}

	contentType := mimeByExt(filepath.Ext(fullPath))
	return content, contentType, nil
}

// --- Internal helpers ---

func (s *ThemeService) getActiveThemeShort(ctx context.Context) string {
	val, err := s.settingRepo.GetValue(ctx, themeActiveKey)
	if err != nil {
		return ""
	}
	return val
}

func (s *ThemeService) getThemeConfig(ctx context.Context, short string) map[string]string {
	val, err := s.settingRepo.GetValue(ctx, themeConfigPrefix+short)
	if err != nil {
		return nil
	}
	var config map[string]string
	if err := json.Unmarshal([]byte(val), &config); err != nil {
		return nil
	}
	return config
}

func (s *ThemeService) loadMetadata(short string) (*ThemeMetadata, error) {
	metaPath := filepath.Join(s.dataDir, short, "sub2api-theme.json")
	data, err := os.ReadFile(metaPath)
	if err != nil {
		return nil, err
	}
	var m ThemeMetadata
	if err := json.Unmarshal(data, &m); err != nil {
		return nil, err
	}
	return &m, nil
}

func (s *ThemeService) loadThemeFromDir(ctx context.Context, short, activeShort string) (*Theme, error) {
	metadata, err := s.loadMetadata(short)
	if err != nil {
		return nil, err
	}

	info, err := os.Stat(filepath.Join(s.dataDir, short))
	if err != nil {
		return nil, err
	}

	isActive := short == activeShort

	theme := &Theme{
		Metadata:    *metadata,
		InstalledAt: info.ModTime(),
		IsActive:    isActive,
	}

	if isActive && len(metadata.Config) > 0 {
		theme.Config = s.getThemeConfig(ctx, short)
	}

	return theme, nil
}

func validateThemeMetadata(m *ThemeMetadata) error {
	if m.Name == "" {
		return fmt.Errorf("theme name is required")
	}
	if len(m.Name) > 64 {
		return fmt.Errorf("theme name too long (max 64 characters)")
	}
	if m.Short == "" {
		return fmt.Errorf("theme short identifier is required")
	}
	if !themeShortRegex.MatchString(m.Short) {
		return fmt.Errorf("invalid theme short identifier: must match [a-z0-9][a-z0-9_-]{0,31}")
	}
	if m.Version == "" {
		return fmt.Errorf("theme version is required")
	}
	if len(m.Author) > 128 {
		return fmt.Errorf("theme author too long (max 128 characters)")
	}
	if len(m.Description) > 512 {
		return fmt.Errorf("theme description too long (max 512 characters)")
	}

	// Validate config items
	for _, item := range m.Config {
		if item.Key == "" {
			return fmt.Errorf("theme config item key is required")
		}
		if item.Label == "" {
			return fmt.Errorf("theme config item label is required")
		}
		switch item.Type {
		case "title", "color", "text", "select", "number", "boolean":
			// valid
		default:
			return fmt.Errorf("invalid theme config type: %s", item.Type)
		}
		if item.Type == "select" && len(item.Options) == 0 {
			return fmt.Errorf("select config item must have options")
		}
	}

	return nil
}

// sanitizeCSS removes dangerous CSS constructs
func sanitizeCSS(data []byte) ([]byte, error) {
	content := string(data)

	// Remove CSS comments (to prevent hiding malicious content in comments)
	content = removeComments(content)

	// Check for dangerous constructs
	dangerous := []string{
		"expression(",
		"-moz-binding",
		"behavior:",
		"behavior ",
		"javascript:",
		"vbscript:",
		"data:text/html",
	}
	lower := strings.ToLower(content)
	for _, d := range dangerous {
		if strings.Contains(lower, d) {
			return nil, fmt.Errorf("dangerous CSS construct detected: %s", d)
		}
	}

	// Remove url() references with absolute URLs or data: URIs (except data: for fonts)
	// Allow relative url() references and data:font/*
	content = cssURLRe.ReplaceAllStringFunc(content, func(match string) string {
		inner := cssURLRe.FindStringSubmatch(match)
		if len(inner) < 4 {
			return ""
		}
		// Pick the capture group that matched (single-quoted, double-quoted, or unquoted)
		u := strings.TrimSpace(inner[1] + inner[2] + inner[3])
		lowerU := strings.ToLower(u)

		// Allow relative paths
		if !strings.HasPrefix(lowerU, "http://") &&
			!strings.HasPrefix(lowerU, "https://") &&
			!strings.HasPrefix(lowerU, "data:") &&
			!strings.HasPrefix(lowerU, "javascript:") {
			return match
		}

		// Allow data: for fonts
		if strings.HasPrefix(lowerU, "data:font/") || strings.HasPrefix(lowerU, "data:application/font") {
			return match
		}

		// Block everything else
		return ""
	})

	// Remove @import with external URLs
	content = cssImportRe.ReplaceAllString(content, "")

	return []byte(content), nil
}

func removeComments(content string) string {
	return cssCommentRe.ReplaceAllString(content, "")
}

func parseGitHubURL(url string) (owner, repo string, err error) {
	// Accept formats:
	// https://github.com/owner/repo
	// https://github.com/owner/repo/
	// github.com/owner/repo
	url = strings.TrimSpace(url)
	url = strings.TrimRight(url, "/")

	// Remove protocol
	url = strings.TrimPrefix(url, "https://")
	url = strings.TrimPrefix(url, "http://")

	// Remove domain
	if !strings.HasPrefix(url, "github.com/") {
		return "", "", fmt.Errorf("not a GitHub URL")
	}
	url = strings.TrimPrefix(url, "github.com/")

	parts := strings.SplitN(url, "/", 2)
	if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
		return "", "", fmt.Errorf("invalid GitHub URL format")
	}

	// Remove .git suffix
	repoName := strings.TrimSuffix(parts[1], ".git")

	return parts[0], repoName, nil
}

func downloadFile(ctx context.Context, url string) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return nil, err
	}

	client := &http.Client{Timeout: 60 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("download failed with status %d", resp.StatusCode)
	}

	// Limit download size
	limitedReader := io.LimitReader(resp.Body, int64(ThemeMaxZipSize))
	data, err := io.ReadAll(limitedReader)
	if err != nil {
		return nil, err
	}

	return data, nil
}

func copyDir(src, dst string) error {
	return filepath.Walk(src, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(src, path)
		if err != nil {
			return err
		}
		destPath := filepath.Join(dst, rel)
		if info.IsDir() {
			return os.MkdirAll(destPath, info.Mode())
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		return os.WriteFile(destPath, data, info.Mode())
	})
}

func mimeByExt(ext string) string {
	switch strings.ToLower(ext) {
	case ".css":
		return "text/css; charset=utf-8"
	case ".woff":
		return "font/woff"
	case ".woff2":
		return "font/woff2"
	case ".ttf":
		return "font/ttf"
	case ".otf":
		return "font/otf"
	case ".png":
		return "image/png"
	case ".jpg", ".jpeg":
		return "image/jpeg"
	case ".gif":
		return "image/gif"
	case ".svg":
		return "image/svg+xml"
	case ".ico":
		return "image/x-icon"
	case ".webp":
		return "image/webp"
	case ".json":
		return "application/json"
	default:
		return "application/octet-stream"
	}
}
