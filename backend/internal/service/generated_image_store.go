package service

import (
	"crypto/rand"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"net"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
)

const (
	generatedImageURLPathPrefix = "/api/v1/generated-images/"
	generatedImageMaxBytes      = 32 << 20
	generatedImageTTL           = 30 * time.Minute
)

type generatedImageAsset struct {
	Data        []byte
	ContentType string
	ExpiresAt   time.Time
}

var generatedImages = struct {
	sync.Mutex
	items map[string]generatedImageAsset
}{
	items: make(map[string]generatedImageAsset),
}

func storeGeneratedImageFromBase64(b64, outputFormat string) (string, error) {
	b64 = strings.TrimSpace(b64)
	if b64 == "" {
		return "", errors.New("empty generated image")
	}
	raw, err := base64.StdEncoding.DecodeString(b64)
	if err != nil {
		return "", err
	}
	if len(raw) == 0 {
		return "", errors.New("empty generated image")
	}
	if len(raw) > generatedImageMaxBytes {
		return "", errors.New("generated image too large")
	}
	contentType := openAIImageOutputMIMEType(outputFormat)
	if contentType == "" || contentType == "application/octet-stream" {
		contentType = http.DetectContentType(raw)
	}
	id, err := newGeneratedImageID()
	if err != nil {
		return "", err
	}
	generatedImages.Lock()
	pruneExpiredGeneratedImagesLocked(time.Now())
	generatedImages.items[id] = generatedImageAsset{
		Data:        raw,
		ContentType: contentType,
		ExpiresAt:   time.Now().Add(generatedImageTTL),
	}
	generatedImages.Unlock()
	return id, nil
}

func loadGeneratedImage(id string) (generatedImageAsset, bool) {
	id = strings.TrimSpace(id)
	if id == "" {
		return generatedImageAsset{}, false
	}
	now := time.Now()
	generatedImages.Lock()
	defer generatedImages.Unlock()
	pruneExpiredGeneratedImagesLocked(now)
	item, ok := generatedImages.items[id]
	return item, ok
}

func pruneExpiredGeneratedImagesLocked(now time.Time) {
	for id, item := range generatedImages.items {
		if !item.ExpiresAt.After(now) {
			delete(generatedImages.items, id)
		}
	}
}

func newGeneratedImageID() (string, error) {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "", err
	}
	return hex.EncodeToString(b[:]), nil
}

func generatedImageURLForRequest(c *gin.Context, id string) string {
	return generatedImageURLForRequestWithBase(c, id, "")
}

func generatedImageURLForRequestWithBase(c *gin.Context, id, publicBaseURL string) string {
	if c == nil || c.Request == nil {
		return joinGeneratedImageBase(publicBaseURL, id)
	}

	scheme, host := generatedImageSchemeHostFromRequest(c)
	if host != "" && !isInternalGeneratedImageHost(host) {
		return scheme + "://" + host + generatedImageURLPathPrefix + id
	}
	if strings.TrimSpace(publicBaseURL) != "" {
		return joinGeneratedImageBase(publicBaseURL, id)
	}
	if host != "" {
		return scheme + "://" + host + generatedImageURLPathPrefix + id
	}
	return generatedImageURLPathPrefix + id
}

func generatedImageSchemeHostFromRequest(c *gin.Context) (string, string) {
	forwarded := strings.TrimSpace(c.GetHeader("Forwarded"))
	forwardedProto := forwardedHeaderValue(forwarded, "proto")
	forwardedHost := forwardedHeaderValue(forwarded, "host")

	scheme := firstForwardedHeaderValue(
		forwardedProto,
		c.GetHeader("X-Forwarded-Proto"),
		c.GetHeader("X-Forwarded-Scheme"),
		c.GetHeader("X-Scheme"),
		c.GetHeader("X-Url-Scheme"),
	)
	if scheme == "" {
		scheme = "http"
		if c.Request.TLS != nil {
			scheme = "https"
		}
	}

	host := firstForwardedHeaderValue(
		forwardedHost,
		c.GetHeader("X-Forwarded-Host"),
		c.GetHeader("X-Original-Host"),
		c.GetHeader("X-Host"),
		c.Request.Host,
	)
	if host == "" {
		if ref := originLikeHost(c.GetHeader("Origin")); ref != "" {
			host = ref
		} else if ref := originLikeHost(c.GetHeader("Referer")); ref != "" {
			host = ref
		}
	}
	return normalizeGeneratedImageScheme(scheme), sanitizeGeneratedImageHost(host)
}

func firstForwardedHeaderValue(values ...string) string {
	for _, value := range values {
		for _, part := range strings.Split(value, ",") {
			if trimmed := strings.Trim(strings.TrimSpace(part), "\""); trimmed != "" {
				return trimmed
			}
		}
	}
	return ""
}

func forwardedHeaderValue(header, key string) string {
	if strings.TrimSpace(header) == "" || strings.TrimSpace(key) == "" {
		return ""
	}
	first := strings.Split(header, ",")[0]
	for _, part := range strings.Split(first, ";") {
		pair := strings.SplitN(strings.TrimSpace(part), "=", 2)
		if len(pair) != 2 {
			continue
		}
		if strings.EqualFold(strings.TrimSpace(pair[0]), key) {
			return strings.Trim(strings.TrimSpace(pair[1]), "\"")
		}
	}
	return ""
}

func normalizeGeneratedImageScheme(scheme string) string {
	scheme = strings.ToLower(strings.TrimSpace(scheme))
	switch scheme {
	case "http", "https":
		return scheme
	default:
		return "http"
	}
}

func sanitizeGeneratedImageHost(host string) string {
	host = strings.TrimSpace(host)
	if host == "" {
		return ""
	}
	host = strings.Split(host, ",")[0]
	host = strings.Trim(strings.TrimSpace(host), "\"")
	host = strings.TrimSuffix(host, ".")
	return host
}

func originLikeHost(raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return ""
	}
	u, err := url.Parse(raw)
	if err != nil || u == nil {
		return ""
	}
	return sanitizeGeneratedImageHost(u.Host)
}

func isInternalGeneratedImageHost(host string) bool {
	host = sanitizeGeneratedImageHost(host)
	if host == "" {
		return true
	}
	hostOnly := host
	if parsedHost, _, err := net.SplitHostPort(host); err == nil {
		hostOnly = parsedHost
	}
	hostOnly = strings.Trim(strings.ToLower(strings.TrimSpace(hostOnly)), "[]")
	switch hostOnly {
	case "", "localhost", "0.0.0.0", "::", "::1":
		return true
	}
	ip := net.ParseIP(hostOnly)
	if ip == nil {
		return false
	}
	return ip.IsLoopback() || ip.IsPrivate() || ip.IsUnspecified()
}

func joinGeneratedImageBase(publicBaseURL, id string) string {
	base := strings.TrimRight(strings.TrimSpace(publicBaseURL), "/")
	if base == "" {
		return generatedImageURLPathPrefix + id
	}
	lower := strings.ToLower(base)
	for _, suffix := range []string{"/api/v1", "/v1"} {
		if strings.HasSuffix(lower, suffix) {
			base = strings.TrimRight(base[:len(base)-len(suffix)], "/")
			break
		}
	}
	if base == "" {
		return generatedImageURLPathPrefix + id
	}
	return base + generatedImageURLPathPrefix + id
}

func ServeGeneratedImage(c *gin.Context) {
	id := strings.TrimSpace(c.Param("id"))
	item, ok := loadGeneratedImage(id)
	if !ok {
		c.JSON(http.StatusNotFound, gin.H{"error": gin.H{
			"type":    "not_found_error",
			"message": "generated image not found or expired",
		}})
		return
	}
	c.Header("Cache-Control", "private, max-age=1800")
	c.Data(http.StatusOK, item.ContentType, item.Data)
}
