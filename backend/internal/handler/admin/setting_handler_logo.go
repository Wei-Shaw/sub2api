package admin

import (
	"bytes"
	"crypto/sha256"
	"encoding/xml"
	"fmt"
	"image"
	_ "image/gif"
	_ "image/jpeg"
	_ "image/png"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/Wei-Shaw/sub2api/internal/pkg/response"
	"github.com/gin-gonic/gin"
)

const siteLogoMaxBytes int64 = 300 * 1024

var siteLogoFilename = regexp.MustCompile(`^logo-[a-f0-9]{64}\.(png|jpg|gif|svg)$`)

// UploadSiteLogo stores a logo as a static file and returns its same-origin URL.
func (h *SettingHandler) UploadSiteLogo(c *gin.Context) {
	if strings.TrimSpace(h.siteLogoDir) == "" {
		response.InternalError(c, "站点 Logo 存储目录未配置")
		return
	}
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, siteLogoMaxBytes+1024)
	defer func() {
		if c.Request.MultipartForm != nil {
			_ = c.Request.MultipartForm.RemoveAll()
		}
	}()
	file, _, err := c.Request.FormFile("file")
	if err != nil {
		response.BadRequest(c, "请选择有效的 Logo 图片")
		return
	}
	defer file.Close()
	content, err := io.ReadAll(io.LimitReader(file, siteLogoMaxBytes+1))
	if err != nil {
		response.BadRequest(c, "读取 Logo 图片失败")
		return
	}
	if int64(len(content)) > siteLogoMaxBytes {
		response.Error(c, http.StatusRequestEntityTooLarge, "Logo 图片不能超过 300KB")
		return
	}
	contentType, extension, ok := siteLogoContentType(content)
	if !ok {
		response.BadRequest(c, "Logo 仅支持 PNG、JPG、GIF 或 SVG 图片")
		return
	}

	dir := filepath.Join(h.siteLogoDir, "site-logo")
	if err := os.MkdirAll(dir, 0755); err != nil {
		response.InternalError(c, "创建 Logo 存储目录失败")
		return
	}
	name := fmt.Sprintf("logo-%x%s", sha256.Sum256(content), extension)
	path := filepath.Join(dir, name)
	if err := writeSiteLogoFile(dir, path, content); err != nil {
		response.InternalError(c, "保存 Logo 图片失败")
		return
	}
	// Keep prior assets: the current settings (or another open editor) may still reference them.
	response.Success(c, gin.H{"url": "/api/v1/site-logo/" + name, "content_type": contentType})
}

// Publish complete files atomically; identical uploads reuse the same immutable URL.
func writeSiteLogoFile(dir, path string, content []byte) error {
	if _, err := os.Stat(path); err == nil {
		return nil
	}
	file, err := os.CreateTemp(dir, ".logo-*")
	if err != nil {
		return err
	}
	defer os.Remove(file.Name())
	if _, err := file.Write(content); err != nil {
		_ = file.Close()
		return err
	}
	if err := file.Close(); err != nil {
		return err
	}
	if err := os.Chmod(file.Name(), 0644); err != nil {
		return err
	}
	if err := os.Rename(file.Name(), path); err != nil {
		// Windows cannot rename over a file published by a concurrent identical upload.
		if _, statErr := os.Stat(path); statErr != nil {
			return err
		}
	}
	return nil
}

// ServeSiteLogo serves a previously uploaded site logo without authentication.
func (h *SettingHandler) ServeSiteLogo(c *gin.Context) {
	filename := c.Param("filename")
	if h.siteLogoDir == "" || !siteLogoFilename.MatchString(filename) {
		c.Status(http.StatusNotFound)
		return
	}
	path := filepath.Join(h.siteLogoDir, "site-logo", filename)
	if _, err := os.Stat(path); err != nil {
		c.Status(http.StatusNotFound)
		return
	}
	c.Header("Cache-Control", "public, max-age=31536000, immutable")
	c.Header("X-Content-Type-Options", "nosniff")
	// SVG logos must remain passive images even if their URL is opened directly.
	c.Header("Content-Security-Policy", "sandbox; default-src 'none'; style-src 'unsafe-inline'")
	c.File(path)
}

func siteLogoContentType(content []byte) (string, string, bool) {
	decoder := xml.NewDecoder(bytes.NewReader(content))
	depth, rootFound := 0, false
	for {
		token, err := decoder.Token()
		if err == io.EOF {
			if rootFound && depth == 0 {
				return "image/svg+xml", ".svg", true
			}
			break
		}
		if err != nil {
			break
		}
		switch element := token.(type) {
		case xml.StartElement:
			if depth == 0 {
				if rootFound || element.Name.Local != "svg" {
					return "", "", false
				}
				rootFound = true
			}
			depth++
		case xml.EndElement:
			depth--
		}
	}
	if _, _, err := image.DecodeConfig(bytes.NewReader(content)); err != nil {
		return "", "", false
	}
	contentType := http.DetectContentType(content)
	switch contentType {
	case "image/png":
		return contentType, ".png", true
	case "image/jpeg":
		return contentType, ".jpg", true
	case "image/gif":
		return contentType, ".gif", true
	default:
		return "", "", false
	}
}
