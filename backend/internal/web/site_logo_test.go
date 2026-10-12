//go:build embed

package web

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"image"
	"image/png"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
)

func TestExternalizeSiteLogo(t *testing.T) {
	var buf bytes.Buffer
	if err := png.Encode(&buf, image.NewRGBA(image.Rect(0, 0, 2, 2))); err != nil {
		t.Fatal(err)
	}
	input, _ := json.Marshal(map[string]string{"site_logo": "data:image/png;base64," + base64.StdEncoding.EncodeToString(buf.Bytes()), "site_name": "BUAGA"})
	path, mime, data := siteLogoAsset(input)
	if !strings.HasPrefix(path, siteLogoPrefix) || mime != "image/png" || !bytes.Equal(data, buf.Bytes()) {
		t.Fatal("logo asset changed")
	}
	var cfg map[string]string
	if err := json.Unmarshal(externalizeSiteLogo(input), &cfg); err != nil {
		t.Fatal(err)
	}
	if cfg["site_logo"] != path || cfg["site_name"] != "BUAGA" {
		t.Fatal("settings not preserved")
	}
	provider := &mockSettingsProvider{settings: map[string]string{"site_logo": "data:image/png;base64," + base64.StdEncoding.EncodeToString(buf.Bytes())}}
	server, err := NewFrontendServer(provider)
	if err != nil {
		t.Fatal(err)
	}
	router := gin.New()
	router.Use(server.Middleware())
	response := httptest.NewRecorder()
	router.ServeHTTP(response, httptest.NewRequest("GET", path, nil))
	if response.Code != 200 || !bytes.Equal(response.Body.Bytes(), buf.Bytes()) || response.Header().Get("Cache-Control") != staticAssetsCacheControl {
		t.Fatal("asset not served with immutable cache")
	}
	response = httptest.NewRecorder()
	router.ServeHTTP(response, httptest.NewRequest("GET", siteLogoPrefix+"incorrect", nil))
	if response.Code != 404 {
		t.Fatal("wrong hash must not serve different image")
	}
	for _, logo := range []string{"https://example.com/logo.png", "data:image/png;base64,invalid", "data:image/svg+xml;base64,PHN2Zz4=", "data:image/png;base64,PHNjcmlwdD4="} {
		original, _ := json.Marshal(map[string]string{"site_logo": logo})
		if !bytes.Equal(externalizeSiteLogo(original), original) {
			t.Fatalf("unexpected rewrite: %s", logo)
		}
	}
}
