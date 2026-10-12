//go:build embed

package web

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
)

const siteLogoPrefix = "/site-branding/"

// Only raster data URLs are served; other configured URLs keep their behavior.
func siteLogoAsset(settingsJSON []byte) (path, mime string, data []byte) {
	var cfg struct {
		SiteLogo string `json:"site_logo"`
	}
	if json.Unmarshal(settingsJSON, &cfg) != nil {
		return
	}
	header, payload, ok := strings.Cut(cfg.SiteLogo, ",")
	if !ok {
		return
	}
	switch header {
	case "data:image/png;base64":
		mime = "image/png"
	case "data:image/jpeg;base64":
		mime = "image/jpeg"
	case "data:image/webp;base64":
		mime = "image/webp"
	case "data:image/gif;base64":
		mime = "image/gif"
	default:
		return
	}
	decoded, err := base64.StdEncoding.DecodeString(payload)
	if err != nil || len(decoded) == 0 {
		return "", "", nil
	}
	if http.DetectContentType(decoded) != mime {
		return "", "", nil
	}
	sum := sha256.Sum256(decoded)
	return siteLogoPrefix + hex.EncodeToString(sum[:]), mime, decoded
}

func externalizeSiteLogo(settingsJSON []byte) []byte {
	path, _, _ := siteLogoAsset(settingsJSON)
	if path == "" {
		return settingsJSON
	}
	var cfg map[string]json.RawMessage
	if json.Unmarshal(settingsJSON, &cfg) != nil {
		return settingsJSON
	}
	cfg["site_logo"], _ = json.Marshal(path)
	result, err := json.Marshal(cfg)
	if err != nil {
		return settingsJSON
	}
	return result
}

func (s *FrontendServer) serveSiteLogo(c *gin.Context) {
	ctx, cancel := context.WithTimeout(c.Request.Context(), 2*time.Second)
	defer cancel()
	settings, err := s.settings.GetPublicSettingsForInjection(ctx)
	if err != nil {
		c.AbortWithStatus(http.StatusServiceUnavailable)
		return
	}
	settingsJSON, err := json.Marshal(settings)
	if err != nil {
		c.AbortWithStatus(http.StatusInternalServerError)
		return
	}
	path, mime, data := siteLogoAsset(settingsJSON)
	if path == "" || c.Request.URL.Path != path {
		c.AbortWithStatus(http.StatusNotFound)
		return
	}
	c.Header("Cache-Control", staticAssetsCacheControl)
	c.Header("X-Content-Type-Options", "nosniff")
	c.Data(http.StatusOK, mime, data)
	c.Abort()
}
