package admin

import (
	"bytes"
	"encoding/json"
	"image"
	"image/png"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func logoUploadRequest(t *testing.T, router http.Handler, data []byte) *httptest.ResponseRecorder {
	t.Helper()
	var body bytes.Buffer
	w := multipart.NewWriter(&body)
	part, err := w.CreateFormFile("file", "../../logo.png")
	require.NoError(t, err)
	_, err = part.Write(data)
	require.NoError(t, err)
	require.NoError(t, w.Close())
	req := httptest.NewRequest(http.MethodPost, "/upload", &body)
	req.Header.Set("Content-Type", w.FormDataContentType())
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	return rec
}

func TestSiteLogoUploadAndServe(t *testing.T) {
	dir := t.TempDir()
	h := &SettingHandler{}
	h.SetSiteLogoDataDir(dir)
	r := gin.New()
	r.POST("/upload", h.UploadSiteLogo)
	r.GET("/api/v1/site-logo/:filename", h.ServeSiteLogo)
	var pngBytes bytes.Buffer
	require.NoError(t, png.Encode(&pngBytes, image.NewRGBA(image.Rect(0, 0, 2, 2))))
	var firstURL string
	for _, data := range [][]byte{pngBytes.Bytes(), []byte(`<?xml version="1.0"?><svg xmlns="http://www.w3.org/2000/svg" width="2" height="2"/>`)} {
		rec := logoUploadRequest(t, r, data)
		require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
		var result struct {
			Data struct {
				URL string `json:"url"`
			} `json:"data"`
		}
		require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &result))
		require.True(t, strings.HasPrefix(result.Data.URL, "/api/v1/site-logo/logo-"))
		stored, err := os.ReadFile(filepath.Join(dir, "site-logo", filepath.Base(result.Data.URL)))
		require.NoError(t, err)
		require.Equal(t, data, stored)
		read := httptest.NewRecorder()
		r.ServeHTTP(read, httptest.NewRequest(http.MethodGet, result.Data.URL, nil))
		require.Equal(t, http.StatusOK, read.Code)
		require.Equal(t, data, read.Body.Bytes())
		require.Equal(t, "nosniff", read.Header().Get("X-Content-Type-Options"))
		require.Contains(t, read.Header().Get("Content-Security-Policy"), "sandbox")
		if firstURL == "" {
			firstURL = result.Data.URL
		}
	}
	// Uploading a new draft must not break the logo currently saved in settings.
	read := httptest.NewRecorder()
	r.ServeHTTP(read, httptest.NewRequest(http.MethodGet, firstURL, nil))
	require.Equal(t, http.StatusOK, read.Code)
	require.Equal(t, pngBytes.Bytes(), read.Body.Bytes())
}

func TestSiteLogoRejectsInvalidUploads(t *testing.T) {
	h := &SettingHandler{}
	h.SetSiteLogoDataDir(t.TempDir())
	r := gin.New()
	r.POST("/upload", h.UploadSiteLogo)
	for _, data := range [][]byte{[]byte("not an image"), []byte("<svg><broken>"), []byte("<html><svg/></html>"), bytes.Repeat([]byte("x"), int(siteLogoMaxBytes+1))} {
		rec := logoUploadRequest(t, r, data)
		require.GreaterOrEqual(t, rec.Code, http.StatusBadRequest)
	}
}
