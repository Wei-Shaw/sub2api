package kiro

import (
	"bytes"
	"context"
	"encoding/base64"
	"image"
	_ "image/gif"
	_ "image/jpeg"
	_ "image/png"
	"io"
	"math"
	"net/http"
	"sort"
	"strings"
	"sync"
	"time"

	"golang.org/x/image/webp"
	"golang.org/x/sync/singleflight"
)

const (
	kiroImageTokenLongEdge      = 1568
	kiroImageTokenMaxPixels     = 1_150_000
	kiroImagePixelsPerToken     = 750
	kiroImageTokenFallback      = 1600
	kiroImageTokenCacheMaxItems = 256
	kiroImageTokenSuccessTTL    = 5 * time.Minute
	kiroImageTokenFailureTTL    = 30 * time.Second
)

type imageTokenCacheEntry struct {
	tokens    int
	expiresAt time.Time
	createdAt time.Time
}

var imageTokenEstimates = struct {
	sync.Mutex
	entries map[string]imageTokenCacheEntry
}{entries: make(map[string]imageTokenCacheEntry)}

var imageTokenEstimateGroup singleflight.Group

// EstimateImageTokens estimates the visual tokens for a Kiro image source.
// source may be raw base64, a data URL, or an HTTP(S) URL.
func EstimateImageTokens(ctx context.Context, mediaType, source string) int {
	source = strings.TrimSpace(source)
	if source == "" {
		return kiroImageTokenFallback
	}
	if isRemoteImageURL(source) {
		return estimateRemoteImageTokens(ctx, source)
	}

	if data, ok := imageDataURLPayload(source); ok {
		source = data
	}
	if tokens, ok := estimateBase64ImageTokens(mediaType, source); ok {
		return tokens
	}
	return kiroImageTokenFallback
}

func estimateRemoteImageTokens(ctx context.Context, rawURL string) int {
	if tokens, ok := loadImageTokenCache(rawURL, time.Now()); ok {
		return tokens
	}

	result := imageTokenEstimateGroup.DoChan(rawURL, func() (any, error) {
		if tokens, ok := loadImageTokenCache(rawURL, time.Now()); ok {
			return tokens, nil
		}
		tokens, ok := fetchRemoteImageTokens(ctx, rawURL)
		ttl := kiroImageTokenSuccessTTL
		if !ok {
			tokens = kiroImageTokenFallback
			ttl = kiroImageTokenFailureTTL
		}
		storeImageTokenCache(rawURL, tokens, ttl, time.Now())
		return tokens, nil
	})

	select {
	case <-ctx.Done():
		return kiroImageTokenFallback
	case resolved := <-result:
		if resolved.Err != nil {
			return kiroImageTokenFallback
		}
		tokens, ok := resolved.Val.(int)
		if !ok || tokens < 1 {
			return kiroImageTokenFallback
		}
		return tokens
	}
}

func fetchRemoteImageTokens(ctx context.Context, rawURL string) (int, bool) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return 0, false
	}
	req.Header.Set("Accept", "image/*,*/*;q=0.8")
	resp, err := kiroRemoteImageHTTPClient.Do(req)
	if err != nil {
		return 0, false
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return 0, false
	}

	body, err := io.ReadAll(io.LimitReader(resp.Body, kiroRemoteImageMaxBytes+1))
	if err != nil || len(body) == 0 || len(body) > kiroRemoteImageMaxBytes {
		return 0, false
	}
	return estimateImageBytesTokens(body)
}

func estimateBase64ImageTokens(mediaType, encoded string) (int, bool) {
	encoded = strings.TrimSpace(encoded)
	if encoded == "" {
		return 0, false
	}
	for _, encoding := range []*base64.Encoding{base64.StdEncoding, base64.RawStdEncoding} {
		decoder := base64.NewDecoder(encoding, strings.NewReader(encoded))
		if tokens, ok := estimateImageReaderTokens(mediaType, decoder); ok {
			return tokens, true
		}
	}
	return 0, false
}

func estimateImageBytesTokens(data []byte) (int, bool) {
	return estimateImageReaderTokens("", bytes.NewReader(data))
}

func estimateImageReaderTokens(mediaType string, reader io.Reader) (int, bool) {
	var cfg image.Config
	var err error
	switch normalizeKiroImageFormat(strings.TrimPrefix(strings.ToLower(strings.TrimSpace(mediaType)), "image/")) {
	case "webp":
		cfg, err = webp.DecodeConfig(reader)
	default:
		cfg, _, err = image.DecodeConfig(reader)
	}
	if err != nil || cfg.Width <= 0 || cfg.Height <= 0 {
		return 0, false
	}
	return imageTokensForDimensions(cfg.Width, cfg.Height), true
}

func imageTokensForDimensions(width, height int) int {
	if width <= 0 || height <= 0 {
		return kiroImageTokenFallback
	}
	w, h := float64(width), float64(height)
	scale := math.Min(1, math.Min(float64(kiroImageTokenLongEdge)/w, float64(kiroImageTokenLongEdge)/h))
	if pixels := w * h; pixels*scale*scale > kiroImageTokenMaxPixels {
		scale = math.Min(scale, math.Sqrt(float64(kiroImageTokenMaxPixels)/pixels))
	}
	resizedWidth := math.Max(1, math.Floor(w*scale))
	resizedHeight := math.Max(1, math.Floor(h*scale))
	return max(1, int(math.Ceil(resizedWidth*resizedHeight/kiroImagePixelsPerToken)))
}

func imageDataURLPayload(value string) (string, bool) {
	if !strings.HasPrefix(strings.ToLower(value), "data:") {
		return "", false
	}
	comma := strings.IndexByte(value, ',')
	if comma < 0 || !strings.Contains(strings.ToLower(value[:comma]), ";base64") {
		return "", false
	}
	return strings.TrimSpace(value[comma+1:]), true
}

func isRemoteImageURL(value string) bool {
	lower := strings.ToLower(strings.TrimSpace(value))
	return strings.HasPrefix(lower, "http://") || strings.HasPrefix(lower, "https://")
}

func loadImageTokenCache(key string, now time.Time) (int, bool) {
	imageTokenEstimates.Lock()
	defer imageTokenEstimates.Unlock()
	entry, ok := imageTokenEstimates.entries[key]
	if !ok {
		return 0, false
	}
	if !now.Before(entry.expiresAt) {
		delete(imageTokenEstimates.entries, key)
		return 0, false
	}
	return entry.tokens, true
}

func storeImageTokenCache(key string, tokens int, ttl time.Duration, now time.Time) {
	imageTokenEstimates.Lock()
	defer imageTokenEstimates.Unlock()
	for cachedKey, entry := range imageTokenEstimates.entries {
		if !now.Before(entry.expiresAt) {
			delete(imageTokenEstimates.entries, cachedKey)
		}
	}
	if len(imageTokenEstimates.entries) >= kiroImageTokenCacheMaxItems {
		keys := make([]string, 0, len(imageTokenEstimates.entries))
		for cachedKey := range imageTokenEstimates.entries {
			keys = append(keys, cachedKey)
		}
		sort.Slice(keys, func(i, j int) bool {
			return imageTokenEstimates.entries[keys[i]].createdAt.Before(imageTokenEstimates.entries[keys[j]].createdAt)
		})
		delete(imageTokenEstimates.entries, keys[0])
	}
	imageTokenEstimates.entries[key] = imageTokenCacheEntry{tokens: tokens, expiresAt: now.Add(ttl), createdAt: now}
}
