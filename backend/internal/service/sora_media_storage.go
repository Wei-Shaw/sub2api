package service

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log"
	"mime"
	"net/http"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"strings"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/google/uuid"
)

const (
	soraStorageDefaultRoot = "/app/data/sora"
)

// SoraMediaStorage 负责下载并落地 Sora 媒体
type SoraMediaStorage struct {
	cfg                *config.Config
	root               string
	imageRoot          string
	videoRoot          string
	downloadTimeout    time.Duration
	maxDownloadBytes   int64
	fallbackToUpstream bool
	debug              bool
	sem                chan struct{REDACTED
	ready              bool
REDACTED

func NewSoraMediaStorage(cfg *config.Config) *SoraMediaStorage {
	storage := &SoraMediaStorage{cfg: cfgREDACTED
	storage.refreshConfig()
	if storage.Enabled() {
		if err := storage.EnsureLocalDirs(); err != nil {
			log.Printf("[SoraStorage] 初始化失败: %v", err)
	REDACTED
REDACTED
	return storage
REDACTED

func (s *SoraMediaStorage) Enabled() bool {
	if s == nil || s.cfg == nil {
		return false
REDACTED
	return strings.ToLower(strings.TrimSpace(s.cfg.Sora.Storage.Type)) == "local"
REDACTED

func (s *SoraMediaStorage) Root() string {
	if s == nil {
		return ""
REDACTED
	return s.root
REDACTED

func (s *SoraMediaStorage) ImageRoot() string {
	if s == nil {
		return ""
REDACTED
	return s.imageRoot
REDACTED

func (s *SoraMediaStorage) VideoRoot() string {
	if s == nil {
		return ""
REDACTED
	return s.videoRoot
REDACTED

func (s *SoraMediaStorage) refreshConfig() {
	if s == nil || s.cfg == nil {
		return
REDACTED
	root := strings.TrimSpace(s.cfg.Sora.Storage.LocalPath)
	if root == "" {
		root = soraStorageDefaultRoot
REDACTED
	root = filepath.Clean(root)
	if !filepath.IsAbs(root) {
		if absRoot, err := filepath.Abs(root); err == nil {
			root = absRoot
	REDACTED
REDACTED
	s.root = root
	s.imageRoot = filepath.Join(root, "image")
	s.videoRoot = filepath.Join(root, "video")

	maxConcurrent := s.cfg.Sora.Storage.MaxConcurrentDownloads
	if maxConcurrent <= 0 {
		maxConcurrent = 4
REDACTED
	timeoutSeconds := s.cfg.Sora.Storage.DownloadTimeoutSeconds
	if timeoutSeconds <= 0 {
		timeoutSeconds = 120
REDACTED
	s.downloadTimeout = time.Duration(timeoutSeconds) * time.Second

	maxBytes := s.cfg.Sora.Storage.MaxDownloadBytes
	if maxBytes <= 0 {
		maxBytes = 200 << 20
REDACTED
	s.maxDownloadBytes = maxBytes
	s.fallbackToUpstream = s.cfg.Sora.Storage.FallbackToUpstream
	s.debug = s.cfg.Sora.Storage.Debug
	s.sem = make(chan struct{REDACTED, maxConcurrent)
REDACTED

// EnsureLocalDirs 创建并校验本地目录
func (s *SoraMediaStorage) EnsureLocalDirs() error {
	if s == nil || !s.Enabled() {
		return nil
REDACTED
	if err := os.MkdirAll(s.imageRoot, 0o755); err != nil {
		return fmt.Errorf("create image dir: %w", err)
REDACTED
	if err := os.MkdirAll(s.videoRoot, 0o755); err != nil {
		return fmt.Errorf("create video dir: %w", err)
REDACTED
	s.ready = true
	return nil
REDACTED

// StoreFromURLs 下载并存储媒体，返回相对路径或回退 URL
func (s *SoraMediaStorage) StoreFromURLs(ctx context.Context, mediaType string, urls []string) ([]string, error) {
	if len(urls) == 0 {
		return nil, nil
REDACTED
	if s == nil || !s.Enabled() {
		return urls, nil
REDACTED
	if !s.ready {
		if err := s.EnsureLocalDirs(); err != nil {
			return nil, err
	REDACTED
REDACTED
	results := make([]string, 0, len(urls))
	for _, raw := range urls {
		relative, err := s.downloadAndStore(ctx, mediaType, raw)
		if err != nil {
			if s.fallbackToUpstream {
				results = append(results, raw)
				continue
		REDACTED
			return nil, err
	REDACTED
		results = append(results, relative)
REDACTED
	return results, nil
REDACTED

func (s *SoraMediaStorage) downloadAndStore(ctx context.Context, mediaType, rawURL string) (string, error) {
	if strings.TrimSpace(rawURL) == "" {
		return "", errors.New("empty url")
REDACTED
	root := s.imageRoot
	if mediaType == "video" {
		root = s.videoRoot
REDACTED
	if root == "" {
		return "", errors.New("storage root not configured")
REDACTED

	retries := 3
	for attempt := 1; attempt <= retries; attempt++ {
		release, err := s.acquire(ctx)
		if err != nil {
			return "", err
	REDACTED
		relative, err := s.downloadOnce(ctx, root, mediaType, rawURL)
		release()
		if err == nil {
			return relative, nil
	REDACTED
		if s.debug {
			log.Printf("[SoraStorage] 下载失败(%d/%d): %s err=%v", attempt, retries, sanitizeMediaLogURL(rawURL), err)
	REDACTED
		if attempt < retries {
			time.Sleep(time.Duration(attempt*attempt) * time.Second)
			continue
	REDACTED
		return "", err
REDACTED
	return "", errors.New("download retries exhausted")
REDACTED

func (s *SoraMediaStorage) downloadOnce(ctx context.Context, root, mediaType, rawURL string) (string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return "", err
REDACTED
	client := &http.Client{Timeout: s.downloadTimeoutREDACTED
	resp, err := client.Do(req)
	if err != nil {
		return "", err
REDACTED
	defer func() { _ = resp.Body.Close() REDACTED()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 2<<20))
		return "", fmt.Errorf("download failed: %d %s", resp.StatusCode, string(body))
REDACTED

	ext := normalizeSoraFileExt(fileExtFromURL(rawURL))
	if ext == "" {
		ext = normalizeSoraFileExt(fileExtFromContentType(resp.Header.Get("Content-Type")))
REDACTED
	if ext == "" {
		ext = ".bin"
REDACTED
	if s.maxDownloadBytes > 0 && resp.ContentLength > s.maxDownloadBytes {
		return "", fmt.Errorf("download size exceeds limit: %d", resp.ContentLength)
REDACTED

	storageRoot, err := os.OpenRoot(root)
	if err != nil {
		return "", err
REDACTED
	defer func() { _ = storageRoot.Close() REDACTED()

	datePath := time.Now().Format("2006/01/02")
	datePathFS := filepath.FromSlash(datePath)
	if err := storageRoot.MkdirAll(datePathFS, 0o755); err != nil {
		return "", err
REDACTED
	filename := uuid.NewString() + ext
	filePath := filepath.Join(datePathFS, filename)
	out, err := storageRoot.OpenFile(filePath, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o644)
	if err != nil {
		return "", err
REDACTED
	defer func() { _ = out.Close() REDACTED()

	limited := io.LimitReader(resp.Body, s.maxDownloadBytes+1)
	written, err := io.Copy(out, limited)
	if err != nil {
		removePartialDownload(storageRoot, filePath)
		return "", err
REDACTED
	if s.maxDownloadBytes > 0 && written > s.maxDownloadBytes {
		removePartialDownload(storageRoot, filePath)
		return "", fmt.Errorf("download size exceeds limit: %d", written)
REDACTED

	relative := path.Join("/", mediaType, datePath, filename)
	if s.debug {
		log.Printf("[SoraStorage] 已落地 %s -> %s", sanitizeMediaLogURL(rawURL), relative)
REDACTED
	return relative, nil
REDACTED

func (s *SoraMediaStorage) acquire(ctx context.Context) (func(), error) {
	if s.sem == nil {
		return func() {REDACTED, nil
REDACTED
	select {
	case s.sem <- struct{REDACTED{REDACTED:
		return func() { <-s.sem REDACTED, nil
	case <-ctx.Done():
		return nil, ctx.Err()
REDACTED
REDACTED

func fileExtFromURL(raw string) string {
	parsed, err := url.Parse(raw)
	if err != nil {
		return ""
REDACTED
	ext := path.Ext(parsed.Path)
	return strings.ToLower(ext)
REDACTED

func fileExtFromContentType(ct string) string {
	if ct == "" {
		return ""
REDACTED
	if exts, err := mime.ExtensionsByType(ct); err == nil && len(exts) > 0 {
		return strings.ToLower(exts[0])
REDACTED
	return ""
REDACTED

func normalizeSoraFileExt(ext string) string {
	ext = strings.ToLower(strings.TrimSpace(ext))
	switch ext {
	case ".png", ".jpg", ".jpeg", ".gif", ".webp", ".bmp", ".svg", ".tif", ".tiff", ".heic",
		".mp4", ".mov", ".webm", ".m4v", ".avi", ".mkv", ".3gp", ".flv":
		return ext
	default:
		return ""
REDACTED
REDACTED

func removePartialDownload(root *os.Root, filePath string) {
	if root == nil || strings.TrimSpace(filePath) == "" {
		return
REDACTED
	_ = root.Remove(filePath)
REDACTED

// sanitizeMediaLogURL 脱敏 URL 用于日志记录（去除 query 参数中可能的 token 信息）
func sanitizeMediaLogURL(rawURL string) string {
	parsed, err := url.Parse(rawURL)
	if err != nil {
		if len(rawURL) > 80 {
			return rawURL[:80] + "..."
	REDACTED
		return rawURL
REDACTED
	safe := parsed.Scheme + "://" + parsed.Host + parsed.Path
	if len(safe) > 120 {
		return safe[:120] + "..."
REDACTED
	return safe
REDACTED
