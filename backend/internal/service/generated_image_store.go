package service

import (
	"crypto/rand"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"net/http"
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
	if c == nil || c.Request == nil {
		return generatedImageURLPathPrefix + id
	}
	scheme := strings.TrimSpace(c.GetHeader("X-Forwarded-Proto"))
	if scheme == "" {
		scheme = "http"
		if c.Request.TLS != nil {
			scheme = "https"
		}
	}
	host := strings.TrimSpace(c.GetHeader("X-Forwarded-Host"))
	if host == "" {
		host = strings.TrimSpace(c.Request.Host)
	}
	if host == "" {
		return generatedImageURLPathPrefix + id
	}
	return scheme + "://" + host + generatedImageURLPathPrefix + id
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
