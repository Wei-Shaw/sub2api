package service

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/tidwall/gjson"
	"github.com/Wei-Shaw/sub2api/internal/pkg/logger"
	"github.com/gin-gonic/gin"
)

const (
	openAIImagePreviewContextKey = "openai_image_preview_bytes"
	openAIImagePreviewMaxCount   = 8
	openAIImagePreviewMaxBytes   = 20 << 20
	adminVideoPreviewDir         = "/app/data/admin-video-previews"
	adminVideoPreviewMaxBytes    = 100 << 20
	adminVideoPreviewRetention   = 7 * 24 * time.Hour
)

// Tests replace the storage root once, before any preview workers start.
var openAIImagePreviewDir = "/app/data/admin-image-previews"

// OpenAIImagePreviewAsset is an in-memory image returned by a synchronous image request.
// It is intentionally never exposed through a regular user DTO.
type OpenAIImagePreviewAsset struct {
	Data        []byte
	ContentType string
	DownloadURL string
}

func extractOpenAIImagePreviewAssets(body []byte) []OpenAIImagePreviewAsset {
	assets := collectOpenAIImageInlineAssets(body, "")
	if len(assets) == 0 {
		return nil
	}
	out := make([]OpenAIImagePreviewAsset, 0, minInt(len(assets), openAIImagePreviewMaxCount))
	seen := make(map[string]struct{}, len(assets))
	for _, asset := range assets {
		raw := strings.TrimSpace(asset.B64JSON)
		if raw == "" {
			raw = strings.TrimSpace(asset.DownloadURL)
		}
		if len(raw) > (openAIImagePreviewMaxBytes+2)/3*4+128 {
			continue
		}
		encoded := normalizeOpenAIImageBase64(raw)
		if encoded != "" {
			data, err := decodeOpenAIImageBase64(encoded)
			if err != nil || len(data) == 0 || len(data) > openAIImagePreviewMaxBytes {
				continue
			}
			digest := sha256.Sum256(data)
			key := hex.EncodeToString(digest[:])
			if _, ok := seen[key]; ok {
				continue
			}
			seen[key] = struct{}{}
			contentType := http.DetectContentType(data)
			if !strings.HasPrefix(strings.ToLower(contentType), "image/") {
				continue
			}
			out = append(out, OpenAIImagePreviewAsset{Data: data, ContentType: contentType})
		} else if downloadURL := strings.TrimSpace(asset.DownloadURL); isPreviewDownloadURL(downloadURL) {
			key := "url:" + downloadURL
			if _, ok := seen[key]; ok {
				continue
			}
			seen[key] = struct{}{}
			out = append(out, OpenAIImagePreviewAsset{DownloadURL: downloadURL, ContentType: strings.TrimSpace(asset.MimeType)})
		}
		if len(out) >= openAIImagePreviewMaxCount {
			break
		}
	}
	return out
}

func decodeOpenAIImageBase64(encoded string) ([]byte, error) {
	// normalizeOpenAIImageBase64 has already validated the standard encoding.
	return base64.StdEncoding.DecodeString(encoded)
}

func minInt(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func setOpenAIImagePreviewAssets(c *gin.Context, assets []OpenAIImagePreviewAsset) {
	if c != nil && len(assets) > 0 {
		c.Set(openAIImagePreviewContextKey, assets)
	}
}

// CaptureOpenAIImagePreviewResponse extracts image data from an upstream JSON/SSE payload.
// It is safe to call for every streamed event; duplicate assets are removed.
func CaptureOpenAIImagePreviewResponse(c *gin.Context, body []byte) {
	// Ordinary text responses must not pay for another full JSON decode.
	if !bytes.Contains(body, []byte("b64_json")) && !bytes.Contains(body, []byte("image_generation_call")) && !bytes.Contains(body, []byte("image_base64")) && !bytes.Contains(body, []byte("download_url")) && !bytes.Contains(body, []byte("image_url")) && !bytes.Contains(body, []byte(`"url"`)) {
		return
	}
	appendOpenAIImagePreviewAssets(c, extractOpenAIImagePreviewAssets(body))
}

func appendOpenAIImagePreviewAssets(c *gin.Context, assets []OpenAIImagePreviewAsset) {
	if c == nil || len(assets) == 0 {
		return
	}
	existing := OpenAIImagePreviewAssetsFromContext(c)
	merged := append(append([]OpenAIImagePreviewAsset(nil), existing...), assets...)
	seen := make(map[string]struct{}, len(merged))
	out := make([]OpenAIImagePreviewAsset, 0, minInt(len(merged), openAIImagePreviewMaxCount))
	totalBytes := 0
	for _, asset := range merged {
		if totalBytes+len(asset.Data) > openAIImagePreviewMaxBytes {
			continue
		}
		key := strings.TrimSpace(asset.DownloadURL)
		if len(asset.Data) > 0 {
			digest := sha256.Sum256(asset.Data)
			key = hex.EncodeToString(digest[:])
		}
		if key == "" {
			continue
		}
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		out = append(out, asset)
		totalBytes += len(asset.Data)
		if len(out) >= openAIImagePreviewMaxCount {
			break
		}
	}
	setOpenAIImagePreviewAssets(c, out)
}

// OpenAIImagePreviewAssetsFromContext returns assets captured by the non-streaming handler.
func OpenAIImagePreviewAssetsFromContext(c *gin.Context) []OpenAIImagePreviewAsset {
	if c == nil {
		return nil
	}
	value, ok := c.Get(openAIImagePreviewContextKey)
	if !ok {
		return nil
	}
	assets, _ := value.([]OpenAIImagePreviewAsset)
	return assets
}

// SaveOpenAIImagePreviewsAsync queues a best-effort disk write without delaying billing.
var imagePreviewWorkers = make(chan struct{}, 4)

func clearOpenAIImagePreviews(c *gin.Context) {
	if c != nil {
		c.Set(openAIImagePreviewContextKey, []OpenAIImagePreviewAsset(nil))
	}
}

func persistCapturedImagePreviews(ctx context.Context, c *gin.Context, result *OpenAIForwardResult) {
	if result == nil || result.ImageCount <= 0 {
		return
	}
	SaveOpenAIImagePreviewsAsync(ctx, OpenAIImagePreviewAssetsFromContext(c), ResolveUsageRecordID(ctx, result.RequestID), getAPIKeyIDFromContext(c))
}

func SaveOpenAIImagePreviewsAsync(ctx context.Context, assets []OpenAIImagePreviewAsset, requestID string, apiKeyID int64) {
	if len(assets) == 0 || requestID == "" || apiKeyID <= 0 {
		return
	}
	select {
	case imagePreviewWorkers <- struct{}{}:
	default:
		logger.LegacyPrintf("service.image_preview", "preview worker capacity reached; skipping best-effort preview")
		return
	}
	go func() {
		defer func() { <-imagePreviewWorkers }()
		if err := SaveOpenAIImagePreviews(assets, requestID, apiKeyID); err != nil {
			logger.LegacyPrintf("service.image_preview", "save image previews failed: %v", err)
		}
	}()
}

func isPreviewDownloadURL(raw string) bool {
	u, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || u.Host == "" || u.User != nil {
		return false
	}
	if u.Scheme != "https" && u.Scheme != "http" {
		return false
	}
	host := strings.TrimSuffix(strings.ToLower(u.Hostname()), ".")
	if host == "localhost" || strings.HasSuffix(host, ".localhost") {
		return false
	}
	if ip := net.ParseIP(host); ip != nil {
		return isPublicPreviewIP(ip)
	}
	return true
}

func isPublicPreviewIP(ip net.IP) bool {
	if !ip.IsGlobalUnicast() || ip.IsPrivate() || ip.IsLoopback() || ip.IsLinkLocalUnicast() {
		return false
	}
	if v4 := ip.To4(); v4 != nil && (v4[0] == 0 || v4[0] >= 224 || (v4[0] == 100 && v4[1] >= 64 && v4[1] <= 127)) {
		return false
	}
	return true
}

// Resolve and dial a validated IP in the same operation; redirects use this
// transport too. Never follow environment proxies or re-resolve after checking.
func dialPublicPreview(ctx context.Context, network, address string) (net.Conn, error) {
	host, port, err := net.SplitHostPort(address)
	if err != nil {
		return nil, err
	}
	ips, err := net.DefaultResolver.LookupIPAddr(ctx, host)
	if err != nil {
		return nil, err
	}
	if len(ips) == 0 {
		return nil, fmt.Errorf("image host has no addresses")
	}
	for _, ip := range ips {
		if !isPublicPreviewIP(ip.IP) {
			return nil, fmt.Errorf("non-public image host rejected")
		}
	}
	dialer := net.Dialer{Timeout: 10 * time.Second}
	return dialer.DialContext(ctx, network, net.JoinHostPort(ips[0].IP.String(), port))
}

func downloadOpenAIImagePreview(urlValue string) ([]byte, string, error) {
	if !isPreviewDownloadURL(urlValue) {
		return nil, "", fmt.Errorf("invalid image preview URL")
	}
	transport := &http.Transport{DialContext: dialPublicPreview, TLSHandshakeTimeout: 10 * time.Second}
	defer transport.CloseIdleConnections()
	client := &http.Client{Timeout: 15 * time.Second, Transport: transport, CheckRedirect: func(req *http.Request, via []*http.Request) error {
		if len(via) >= 5 || !isPreviewDownloadURL(req.URL.String()) {
			return fmt.Errorf("unsafe image redirect rejected")
		}
		return nil
	}}
	request, err := http.NewRequest(http.MethodGet, urlValue, nil)
	if err != nil {
		return nil, "", err
	}
	request.Header.Set("Accept", "image/*,*/*;q=0.8")
	response, err := client.Do(request)
	if err != nil {
		return nil, "", err
	}
	defer response.Body.Close()
	if response.StatusCode < http.StatusOK || response.StatusCode >= http.StatusMultipleChoices {
		return nil, "", fmt.Errorf("image preview download returned status %d", response.StatusCode)
	}
	if response.ContentLength > openAIImagePreviewMaxBytes {
		return nil, "", fmt.Errorf("image preview exceeds size limit")
	}
	data, err := io.ReadAll(io.LimitReader(response.Body, openAIImagePreviewMaxBytes+1))
	if err != nil {
		return nil, "", err
	}
	if len(data) == 0 || len(data) > openAIImagePreviewMaxBytes {
		return nil, "", fmt.Errorf("invalid image preview size")
	}
	contentType := http.DetectContentType(data)
	if !strings.HasPrefix(strings.ToLower(contentType), "image/") {
		return nil, "", fmt.Errorf("downloaded preview is not an image")
	}
	return data, contentType, nil
}

// ResolveUsageRecordID applies the same request-id precedence used by usage billing.
func ResolveUsageRecordID(ctx context.Context, upstreamRequestID string) string {
	return resolveUsageBillingRequestID(ctx, upstreamRequestID)
}

func imagePreviewKey(requestID string, apiKeyID int64) string {
	sum := sha256.Sum256([]byte(strings.TrimSpace(requestID) + "\x1f" + strconv.FormatInt(apiKeyID, 10)))
	return hex.EncodeToString(sum[:])
}

// SaveOpenAIImagePreviews persists generated images outside the database.
func SaveOpenAIImagePreviews(assets []OpenAIImagePreviewAsset, requestID string, apiKeyID int64) error {
	if len(assets) == 0 || strings.TrimSpace(requestID) == "" || apiKeyID <= 0 {
		return nil
	}
	dir := filepath.Join(openAIImagePreviewDir, imagePreviewKey(requestID, apiKeyID))
	if err := os.MkdirAll(dir, 0700); err != nil {
		return err
	}
	for index, asset := range assets {
		if index >= openAIImagePreviewMaxCount {
			continue
		}
		if len(asset.Data) == 0 && strings.TrimSpace(asset.DownloadURL) != "" {
			data, contentType, err := downloadOpenAIImagePreview(asset.DownloadURL)
			if err != nil {
				logger.LegacyPrintf("service.image_preview", "download image preview failed: %v", err)
				continue
			}
			asset.Data = data
			asset.ContentType = contentType
		}
		if len(asset.Data) == 0 || len(asset.Data) > openAIImagePreviewMaxBytes {
			continue
		}
		ext := ".bin"
		if strings.EqualFold(strings.TrimSpace(asset.ContentType), "image/png") {
			ext = ".png"
		} else if strings.EqualFold(strings.TrimSpace(asset.ContentType), "image/jpeg") {
			ext = ".jpg"
		} else if strings.EqualFold(strings.TrimSpace(asset.ContentType), "image/webp") {
			ext = ".webp"
		}
		target := filepath.Join(dir, strconv.Itoa(index)+ext)
		tmp, err := os.CreateTemp(dir, ".preview-*")
		if err != nil {
			return err
		}
		if err = tmp.Chmod(0600); err == nil {
			_, err = tmp.Write(asset.Data)
		}
		closeErr := tmp.Close()
		if err == nil {
			err = closeErr
		}
		if err == nil {
			err = os.Rename(tmp.Name(), target)
		} else {
			_ = os.Remove(tmp.Name())
		}
		if err != nil {
			return fmt.Errorf("write image preview %d: %w", index, err)
		}
	}
	return nil
}

// ReadOpenAIImagePreview reads one preview for an authenticated admin handler.
func ReadOpenAIImagePreview(requestID string, apiKeyID int64, index int) ([]byte, string, error) {
	path, err := findOpenAIImagePreview(requestID, apiKeyID, index)
	if err != nil {
		return nil, "", err
	}
	f, err := os.Open(path)
	if err != nil {
		return nil, "", err
	}
	defer f.Close()
	data, err := io.ReadAll(io.LimitReader(f, openAIImagePreviewMaxBytes+1))
	if err != nil {
		return nil, "", err
	}
	if len(data) > openAIImagePreviewMaxBytes {
		return nil, "", fmt.Errorf("preview exceeds size limit")
	}
	return data, http.DetectContentType(data), nil
}

func findOpenAIImagePreview(requestID string, apiKeyID int64, index int) (string, error) {
	if index < 0 || index >= openAIImagePreviewMaxCount {
		return "", os.ErrNotExist
	}
	dir := filepath.Join(openAIImagePreviewDir, imagePreviewKey(requestID, apiKeyID))
	entries, err := os.ReadDir(dir)
	if err != nil {
		return "", err
	}
	prefix := strconv.Itoa(index)
	for _, entry := range entries {
		if !entry.Type().IsRegular() || !strings.HasPrefix(entry.Name(), prefix+".") {
			continue
		}
		info, err := entry.Info()
		if err != nil {
			return "", err
		}
		if info.Size() <= 0 || info.Size() > openAIImagePreviewMaxBytes {
			continue
		}
		return filepath.Join(dir, entry.Name()), nil
	}
	return "", os.ErrNotExist
}

// ListOpenAIImagePreviewURLs returns only files that actually exist on disk.
func ListOpenAIImagePreviewURLs(requestID string, apiKeyID, usageID int64, imageCount int) []string {
	if imageCount <= 0 {
		return nil
	}
	limit := minInt(imageCount, openAIImagePreviewMaxCount)
	urls := make([]string, 0, limit)
	for index := 0; index < limit; index++ {
		if _, err := findOpenAIImagePreview(requestID, apiKeyID, index); err == nil {
			urls = append(urls, fmt.Sprintf("/api/v1/admin/usage/%d/images/%d", usageID, index))
		}
	}
	return urls
}

// SaveGrokVideoPreviewAsync persists a completed video without delaying the client response.
func SaveGrokVideoPreviewAsync(videoURL, requestID string, apiKeyID int64) {
	if strings.TrimSpace(videoURL) == "" || strings.TrimSpace(requestID) == "" || apiKeyID <= 0 {
		return
	}
	go func() {
		if err := saveGrokVideoPreview(videoURL, requestID, apiKeyID); err != nil {
			logger.LegacyPrintf("service.video_preview", "save video preview failed: %v", err)
		}
	}()
}

func videoPreviewKey(requestID string, apiKeyID int64) string {
	// Capture and usage billing must resolve to the same directory.
	requestID = StableGrokVideoBillingRequestID(strings.TrimSpace(requestID))
	sum := sha256.Sum256([]byte(requestID + "\x1f" + strconv.FormatInt(apiKeyID, 10)))
	return hex.EncodeToString(sum[:])
}

func saveGrokVideoPreview(videoURL, requestID string, apiKeyID int64) error {
	if !isTrustedVideoPreviewURL(videoURL) {
		return fmt.Errorf("invalid grok video preview URL")
	}
	dir := filepath.Join(adminVideoPreviewDir, videoPreviewKey(requestID, apiKeyID))
	if err := os.MkdirAll(dir, 0700); err != nil { return err }
	tmp, err := os.CreateTemp(dir, ".preview-url-*")
	if err != nil { return err }
	if err = tmp.Chmod(0600); err == nil { _, err = io.WriteString(tmp, strings.TrimSpace(videoURL)) }
	closeErr := tmp.Close(); if err == nil { err = closeErr }
	if err == nil { err = os.Rename(tmp.Name(), filepath.Join(dir, "0.url")) } else { _ = os.Remove(tmp.Name()) }
	return err
}

func bytesLooksLikeMP4(data []byte) bool {
	return len(data) >= 12 && string(data[4:8]) == "ftyp"
}

func ReadGrokVideoPreview(requestID string, apiKeyID int64) ([]byte, string, error) {
	if strings.TrimSpace(requestID) == "" || apiKeyID <= 0 { return nil, "", os.ErrNotExist }
	data, err := os.ReadFile(filepath.Join(adminVideoPreviewDir, videoPreviewKey(requestID, apiKeyID), "0.url"))
	if os.IsNotExist(err) {
		// Earlier releases captured the raw task ID before adding the billing prefix.
		rawID := strings.TrimPrefix(strings.TrimSpace(requestID), "grok-video:")
		sum := sha256.Sum256([]byte(rawID + "\x1f" + strconv.FormatInt(apiKeyID, 10)))
		data, err = os.ReadFile(filepath.Join(adminVideoPreviewDir, hex.EncodeToString(sum[:]), "0.url"))
	}
	if err != nil { return nil, "", err }
	return data, "text/uri-list", nil
}

func ListGrokVideoPreviewURLs(requestID string, apiKeyID, usageID int64, videoCount int) []string {
	if videoCount <= 0 { return nil }
	if _, _, err := ReadGrokVideoPreview(requestID, apiKeyID); err != nil { return nil }
	return []string{fmt.Sprintf("/api/v1/admin/usage/%d/videos/0", usageID)}
}

// CleanupAdminMediaPreviews removes image/video preview directories older than the retention window.
func CleanupAdminMediaPreviews(now time.Time) error {
	cutoff := now.Add(-adminVideoPreviewRetention)
	for _, root := range []string{openAIImagePreviewDir, adminVideoPreviewDir} {
		entries, err := os.ReadDir(root)
		if os.IsNotExist(err) { continue }
		if err != nil { return err }
		for _, entry := range entries {
			if !entry.IsDir() { continue }
			path := filepath.Join(root, entry.Name())
			info, err := entry.Info(); if err != nil { continue }
			if info.ModTime().Before(cutoff) { _ = os.RemoveAll(path) }
		}
	}
	return nil
}

// ExtractVideoPreviewURL finds a provider video URL in common OpenAI-compatible responses.
func ExtractVideoPreviewURL(body []byte) string {
	if !gjson.ValidBytes(body) { return "" }
	for _, path := range []string{"video.url", "video_url", "url", "download_url", "output.video_url", "output.url", "data.0.video.url", "data.0.video_url", "data.0.url"} {
		value := strings.TrimSpace(gjson.GetBytes(body, path).String())
		if isTrustedVideoPreviewURL(value) && (path != "data.0.url" || looksLikeVideoURL(value)) { return value }
	}
	return ""
}

func looksLikeVideoURL(raw string) bool {
	path := strings.ToLower(strings.SplitN(strings.TrimSpace(raw), "?", 2)[0])
	for _, ext := range []string{".mp4", ".webm", ".mov", ".mkv", ".avi", ".m3u8"} {
		if strings.HasSuffix(path, ext) { return true }
	}
	return strings.Contains(path, "video") || strings.Contains(path, "media")
}

func isTrustedVideoPreviewURL(raw string) bool {
	u, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || !strings.EqualFold(u.Scheme, "https") || u.User != nil || u.Hostname() == "" { return false }
	// Provider domains are intentionally limited to known media hosts; arbitrary URLs
	// in model text must never be fetched by the server.
	host := strings.ToLower(u.Hostname())
	return host == "vidgen.x.ai" || strings.Contains(host, "wan") || strings.Contains(host, "aliyuncs.com") || strings.Contains(host, "volces.com") || strings.Contains(host, "modelscope.cn") || strings.Contains(host, "tencentcos.cn") || strings.Contains(host, "cloudflarestorage.com") || strings.Contains(host, "replicate.delivery") || strings.Contains(host, "huggingface.co") || strings.HasSuffix(host, ".r2.dev") || strings.Contains(host, "amazonaws.com")
}
