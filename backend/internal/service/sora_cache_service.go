package service

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"strings"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/pkg/uuidv7"
	"github.com/Wei-Shaw/sub2api/internal/util/urlvalidator"
)

// SoraCacheService 提供 Sora 视频缓存能力。
type SoraCacheService struct {
	cfg            *config.Config
	cacheRepo      SoraCacheFileRepository
	settingService *SettingService
	accountRepo    AccountRepository
	httpUpstream   HTTPUpstream
REDACTED

// NewSoraCacheService 创建 SoraCacheService。
func NewSoraCacheService(cfg *config.Config, cacheRepo SoraCacheFileRepository, settingService *SettingService, accountRepo AccountRepository, httpUpstream HTTPUpstream) *SoraCacheService {
	return &SoraCacheService{
		cfg:            cfg,
		cacheRepo:      cacheRepo,
		settingService: settingService,
		accountRepo:    accountRepo,
		httpUpstream:   httpUpstream,
REDACTED
REDACTED

func (s *SoraCacheService) CacheVideo(ctx context.Context, accountID, userID int64, taskID, mediaURL string) (*SoraCacheFile, error) {
	cfg := s.getSoraConfig(ctx)
	if !cfg.Cache.Enabled {
		return nil, nil
REDACTED
	trimmed := strings.TrimSpace(mediaURL)
	if trimmed == "" {
		return nil, nil
REDACTED

	allowedHosts := cfg.Cache.AllowedHosts
	useAllowlist := true
	if len(allowedHosts) == 0 {
		if s.cfg != nil {
			allowedHosts = s.cfg.Security.URLAllowlist.UpstreamHosts
			useAllowlist = s.cfg.Security.URLAllowlist.Enabled
	REDACTED else {
			useAllowlist = false
	REDACTED
REDACTED

	if useAllowlist {
		if _, err := urlvalidator.ValidateHTTPSURL(trimmed, urlvalidator.ValidationOptions{
			AllowedHosts:     allowedHosts,
			RequireAllowlist: true,
			AllowPrivate:     s.cfg != nil && s.cfg.Security.URLAllowlist.AllowPrivateHosts,
	REDACTED); err != nil {
			return nil, fmt.Errorf("缓存下载地址不合法: %w", err)
	REDACTED
REDACTED else {
		allowInsecure := false
		if s.cfg != nil {
			allowInsecure = s.cfg.Security.URLAllowlist.AllowInsecureHTTP
	REDACTED
		if _, err := urlvalidator.ValidateURLFormat(trimmed, allowInsecure); err != nil {
			return nil, fmt.Errorf("缓存下载地址不合法: %w", err)
	REDACTED
REDACTED

	videoDir := strings.TrimSpace(cfg.Cache.VideoDir)
	if videoDir == "" {
		return nil, nil
REDACTED

	if cfg.Cache.MaxBytes > 0 {
		size, err := dirSize(videoDir)
		if err != nil {
			return nil, err
	REDACTED
		if size >= cfg.Cache.MaxBytes {
			return nil, nil
	REDACTED
REDACTED

	relativeDir := ""
	if cfg.Cache.UserDirEnabled && userID > 0 {
		relativeDir = fmt.Sprintf("u_%d", userID)
REDACTED

	targetDir := filepath.Join(videoDir, relativeDir)
	if err := os.MkdirAll(targetDir, 0o755); err != nil {
		return nil, err
REDACTED

	uuid, err := uuidv7.New()
	if err != nil {
		return nil, err
REDACTED

	name := deriveFileName(trimmed)
	if name == "" {
		name = "video.mp4"
REDACTED
	name = sanitizeFileName(name)
	filename := uuid + "_" + name
	cachePath := filepath.Join(targetDir, filename)

	resp, err := s.downloadMedia(ctx, accountID, trimmed, time.Duration(cfg.Timeout)*time.Second)
	if err != nil {
		return nil, err
REDACTED
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("缓存下载失败: %d", resp.StatusCode)
REDACTED

	out, err := os.Create(cachePath)
	if err != nil {
		return nil, err
REDACTED
	defer out.Close()

	written, err := io.Copy(out, resp.Body)
	if err != nil {
		return nil, err
REDACTED

	cacheURL := buildCacheURL(relativeDir, filename)

	record := &SoraCacheFile{
		TaskID:      taskID,
		AccountID:   accountID,
		UserID:      userID,
		MediaType:   "video",
		OriginalURL: trimmed,
		CachePath:   cachePath,
		CacheURL:    cacheURL,
		SizeBytes:   written,
		CreatedAt:   time.Now(),
REDACTED
	if s.cacheRepo != nil {
		if err := s.cacheRepo.Create(ctx, record); err != nil {
			return nil, err
	REDACTED
REDACTED
	return record, nil
REDACTED

func buildCacheURL(relativeDir, filename string) string {
	base := "/data/video"
	if relativeDir != "" {
		return path.Join(base, relativeDir, filename)
REDACTED
	return path.Join(base, filename)
REDACTED

func (s *SoraCacheService) getSoraConfig(ctx context.Context) config.SoraConfig {
	if s.settingService != nil {
		return s.settingService.GetSoraConfig(ctx)
REDACTED
	if s.cfg != nil {
		return s.cfg.Sora
REDACTED
	return config.SoraConfig{REDACTED
REDACTED

func (s *SoraCacheService) downloadMedia(ctx context.Context, accountID int64, mediaURL string, timeout time.Duration) (*http.Response, error) {
	if timeout <= 0 {
		timeout = 120 * time.Second
REDACTED
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, "GET", mediaURL, nil)
	if err != nil {
		return nil, err
REDACTED
	req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/131.0.0.0 Safari/537.36")

	if s.httpUpstream == nil {
		client := &http.Client{Timeout: timeoutREDACTED
		return client.Do(req)
REDACTED

	var accountConcurrency int
	proxyURL := ""
	if s.accountRepo != nil && accountID > 0 {
		account, err := s.accountRepo.GetByID(ctx, accountID)
		if err == nil && account != nil {
			accountConcurrency = account.Concurrency
			if account.Proxy != nil {
				proxyURL = account.Proxy.URL()
		REDACTED
	REDACTED
REDACTED
	enableTLS := false
	if s.cfg != nil {
		enableTLS = s.cfg.Gateway.TLSFingerprint.Enabled
REDACTED
	return s.httpUpstream.DoWithTLS(req, proxyURL, accountID, accountConcurrency, enableTLS)
REDACTED

func deriveFileName(rawURL string) string {
	parsed, err := url.Parse(rawURL)
	if err != nil {
		return ""
REDACTED
	name := path.Base(parsed.Path)
	if name == "/" || name == "." {
		return ""
REDACTED
	return name
REDACTED

func sanitizeFileName(name string) string {
	name = strings.TrimSpace(name)
	if name == "" {
		return ""
REDACTED
	sanitized := strings.Map(func(r rune) rune {
		switch {
		case r >= 'a' && r <= 'z':
			return r
		case r >= 'A' && r <= 'Z':
			return r
		case r >= '0' && r <= '9':
			return r
		case r == '-' || r == '_' || r == '.':
			return r
		case r == ' ': // 空格替换为下划线
			return '_'
		default:
			return -1
	REDACTED
REDACTED, name)
	return strings.TrimLeft(sanitized, ".")
REDACTED
