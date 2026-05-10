package handler

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
)

func TestCleanPageImageRelativePath(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want string
		ok   bool
	}{
		{name: "single filename", in: "logo.png", want: "logo.png", ok: true},
		{name: "nested path", in: "images/logo.png", want: filepath.Join("images", "logo.png"), ok: true},
		{name: "dot prefix", in: "./logo.png", want: "logo.png", ok: true},
		{name: "url escaped slash", in: "images%2Flogo.png", want: filepath.Join("images", "logo.png"), ok: true},
		{name: "parent traversal", in: "../secret.png", ok: false},
		{name: "encoded parent traversal", in: "%2e%2e/secret.png", ok: false},
		{name: "backslash traversal", in: `images\secret.png`, ok: false},
		{name: "absolute path", in: "/etc/passwd", ok: false},
		{name: "encoded absolute path", in: "%2fetc/passwd", ok: false},
		{name: "encoded nul byte", in: "logo.png%00", ok: false},
		{name: "invalid escape", in: "logo.png%zz", ok: false},
		{name: "empty path", in: "", ok: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, ok := cleanPageImageRelativePath(tt.in)
			if ok != tt.ok {
				t.Fatalf("ok = %v, want %v", ok, tt.ok)
			}
			if got != tt.want {
				t.Fatalf("path = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestResolvePageImagePath(t *testing.T) {
	root := t.TempDir()
	pagesDir := filepath.Join(root, "pages")
	base := filepath.Join(pagesDir, "guide")
	if err := os.MkdirAll(filepath.Join(base, "images"), 0755); err != nil {
		t.Fatalf("create images dir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(base, "logo.png"), []byte("fake"), 0644); err != nil {
		t.Fatalf("create direct image: %v", err)
	}
	if err := os.WriteFile(filepath.Join(base, "images", "logo.png"), []byte("fake"), 0644); err != nil {
		t.Fatalf("create image: %v", err)
	}

	got, ok := resolvePageImagePath(pagesDir, base, "logo.png")
	if !ok {
		t.Fatal("expected direct image path to be accepted")
	}
	want := filepath.Join(base, "logo.png")
	gotEval, err := filepath.EvalSymlinks(got)
	if err != nil {
		t.Fatalf("eval got path: %v", err)
	}
	wantEval, err := filepath.EvalSymlinks(want)
	if err != nil {
		t.Fatalf("eval want path: %v", err)
	}
	if gotEval != wantEval {
		t.Fatalf("path = %q, want %q", gotEval, wantEval)
	}

	got, ok = resolvePageImagePath(pagesDir, base, "images/logo.png")
	if !ok {
		t.Fatal("expected nested image path to be accepted")
	}
	want = filepath.Join(base, "images", "logo.png")
	gotEval, err = filepath.EvalSymlinks(got)
	if err != nil {
		t.Fatalf("eval nested got path: %v", err)
	}
	wantEval, err = filepath.EvalSymlinks(want)
	if err != nil {
		t.Fatalf("eval nested want path: %v", err)
	}
	if gotEval != wantEval {
		t.Fatalf("path = %q, want %q", gotEval, wantEval)
	}

	if got, ok := resolvePageImagePath(pagesDir, base, "../guide.md"); ok {
		t.Fatalf("expected traversal to be rejected, got %q", got)
	}
}

func TestResolvePageImagePathRejectsSymlinkEscape(t *testing.T) {
	root := t.TempDir()
	pagesDir := filepath.Join(root, "pages")
	base := filepath.Join(pagesDir, "guide")
	outside := filepath.Join(root, "outside")

	if err := os.MkdirAll(base, 0755); err != nil {
		t.Fatalf("create page dir: %v", err)
	}
	if err := os.MkdirAll(outside, 0755); err != nil {
		t.Fatalf("create outside dir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(outside, "secret.png"), []byte("secret"), 0644); err != nil {
		t.Fatalf("create outside file: %v", err)
	}
	if err := os.Symlink(outside, filepath.Join(base, "images")); err != nil {
		t.Skipf("symlink not supported: %v", err)
	}

	if got, ok := resolvePageImagePath(pagesDir, base, "images/secret.png"); ok {
		t.Fatalf("expected symlink escape to be rejected, got %q", got)
	}
}

type pageHandlerSettingRepoStub struct {
	values map[string]string
}

func (s *pageHandlerSettingRepoStub) Get(_ context.Context, _ string) (*service.Setting, error) {
	panic("unexpected call")
}
func (s *pageHandlerSettingRepoStub) GetValue(_ context.Context, key string) (string, error) {
	if value, ok := s.values[key]; ok {
		return value, nil
	}
	return "", nil
}
func (s *pageHandlerSettingRepoStub) Set(_ context.Context, _, _ string) error {
	panic("unexpected call")
}
func (s *pageHandlerSettingRepoStub) SetMultiple(_ context.Context, _ map[string]string) error {
	panic("unexpected call")
}
func (s *pageHandlerSettingRepoStub) GetAll(_ context.Context) (map[string]string, error) {
	panic("unexpected call")
}
func (s *pageHandlerSettingRepoStub) Delete(_ context.Context, _ string) error {
	panic("unexpected call")
}
func (s *pageHandlerSettingRepoStub) GetMultiple(_ context.Context, keys []string) (map[string]string, error) {
	out := make(map[string]string, len(keys))
	for _, key := range keys {
		if value, ok := s.values[key]; ok {
			out[key] = value
		}
	}
	return out, nil
}

func TestGetPublicPageContentHonorsVisibility(t *testing.T) {
	gin.SetMode(gin.TestMode)
	root := t.TempDir()
	pagesDir := filepath.Join(root, "pages")
	if err := os.MkdirAll(pagesDir, 0o755); err != nil {
		t.Fatalf("create pages dir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(pagesDir, "guide.md"), []byte("# Guide"), 0o644); err != nil {
		t.Fatalf("create page: %v", err)
	}
	if err := os.WriteFile(filepath.Join(pagesDir, "admin-guide.md"), []byte("# Admin"), 0o644); err != nil {
		t.Fatalf("create admin page: %v", err)
	}

	repo := &pageHandlerSettingRepoStub{
		values: map[string]string{
			service.SettingKeyCustomMenuItems: `[{"id":"guide","page_slug":"guide","visibility":"user"},{"id":"admin-guide","page_slug":"admin-guide","visibility":"admin"}]`,
		},
	}
	handler := NewPageHandler(root, service.NewSettingService(repo, &config.Config{}))

	router := gin.New()
	router.GET("/api/v1/public/pages/:slug", handler.GetPublicPageContent)

	wPublic := httptest.NewRecorder()
	reqPublic := httptest.NewRequest(http.MethodGet, "/api/v1/public/pages/guide", nil)
	router.ServeHTTP(wPublic, reqPublic)
	if wPublic.Code != http.StatusOK {
		t.Fatalf("public page status = %d, want %d", wPublic.Code, http.StatusOK)
	}

	wAdmin := httptest.NewRecorder()
	reqAdmin := httptest.NewRequest(http.MethodGet, "/api/v1/public/pages/admin-guide", nil)
	router.ServeHTTP(wAdmin, reqAdmin)
	if wAdmin.Code != http.StatusNotFound {
		t.Fatalf("admin page status = %d, want %d", wAdmin.Code, http.StatusNotFound)
	}
}

func TestBuiltInTutorialPageIsPublicWithoutCustomMenu(t *testing.T) {
	gin.SetMode(gin.TestMode)
	root := t.TempDir()
	repo := &pageHandlerSettingRepoStub{
		values: map[string]string{
			service.SettingKeyCustomMenuItems: `[]`,
		},
	}
	handler := NewPageHandler(root, service.NewSettingService(repo, &config.Config{}))

	router := gin.New()
	router.GET("/api/v1/public/pages/:slug", handler.GetPublicPageContent)

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/public/pages/tutorial", nil)
	router.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("tutorial status = %d, want %d", w.Code, http.StatusOK)
	}
	if body := w.Body.String(); body == "" || body[:1] != "#" {
		t.Fatalf("unexpected tutorial body: %q", body)
	}
}

func TestGetTutorialDocumentRendersMarkdownFallbackAsHTML(t *testing.T) {
	gin.SetMode(gin.TestMode)
	root := t.TempDir()
	repo := &pageHandlerSettingRepoStub{
		values: map[string]string{
			service.SettingKeyCustomMenuItems: `[]`,
		},
	}
	handler := NewPageHandler(root, service.NewSettingService(repo, &config.Config{}))

	router := gin.New()
	router.GET("/api/v1/tutorial-document", handler.GetTutorialDocument)

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/tutorial-document", nil)
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusOK)
	}

	var resp struct {
		Code int `json:"code"`
		Data struct {
			ContentHTML string `json:"content_html"`
		} `json:"data"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal response: %v", err)
	}
	if resp.Code != 0 {
		t.Fatalf("code = %d, want 0", resp.Code)
	}
	if resp.Data.ContentHTML == "" {
		t.Fatalf("expected content_html to be non-empty, raw body: %s", w.Body.String())
	}
	if resp.Data.ContentHTML[0] == '#' {
		t.Fatalf("content_html should be rendered HTML, got markdown: %q", resp.Data.ContentHTML)
	}
	if want := "<h1"; len(resp.Data.ContentHTML) < len(want) || resp.Data.ContentHTML[:len(want)] != want {
		t.Fatalf("expected rendered heading HTML, got %q", resp.Data.ContentHTML)
	}
}

func TestRenderTutorialMarkdownToHTML_DefaultTutorialNotEmpty(t *testing.T) {
	var raw bytes.Buffer
	if err := tutorialMarkdownRenderer.Convert([]byte(defaultTutorialMarkdown), &raw); err != nil {
		t.Fatalf("render markdown raw: %v", err)
	}
	html, err := renderTutorialMarkdownToHTML(defaultTutorialMarkdown)
	if err != nil {
		t.Fatalf("render markdown: %v", err)
	}
	if html == "" {
		t.Fatalf("expected rendered html to be non-empty; raw=%q sanitized=%q", raw.String(), html)
	}
}

func TestDetectPageImageExtensionRejectsSVG(t *testing.T) {
	t.Parallel()

	svg := []byte(`<svg xmlns="http://www.w3.org/2000/svg"><script>alert(1)</script></svg>`)
	if ext, ok := detectPageImageExtension(svg, "image/svg+xml", "demo.svg"); ok || ext != "" {
		t.Fatalf("expected svg upload to be rejected, got ext=%q ok=%v", ext, ok)
	}
}

func TestDetectPageImageExtensionRejectsOctetStreamDisguisedAsImage(t *testing.T) {
	t.Parallel()

	fake := []byte("not an image")
	if ext, ok := detectPageImageExtension(fake, "application/octet-stream", "demo.png"); ok || ext != "" {
		t.Fatalf("expected disguised upload to be rejected, got ext=%q ok=%v", ext, ok)
	}
}

func TestSanitizePageAssetBaseName_PreservesSafeChineseFilename(t *testing.T) {
	t.Parallel()

	got := sanitizePageAssetBaseName("教程截图-中文文件名 01")
	if got != "教程截图-中文文件名-01" {
		t.Fatalf("sanitizePageAssetBaseName = %q, want %q", got, "教程截图-中文文件名-01")
	}
}

func TestServePageImage_SupportsChineseFilename(t *testing.T) {
	gin.SetMode(gin.TestMode)
	root := t.TempDir()
	pagesDir := filepath.Join(root, "pages")
	imageDir := filepath.Join(pagesDir, "tutorial")
	if err := os.MkdirAll(imageDir, 0o755); err != nil {
		t.Fatalf("create image dir: %v", err)
	}

	imageName := "教程截图-中文文件名.png"
	imagePath := filepath.Join(imageDir, imageName)
	imageBytes := []byte("png")
	if err := os.WriteFile(imagePath, imageBytes, 0o644); err != nil {
		t.Fatalf("write image: %v", err)
	}

	repo := &pageHandlerSettingRepoStub{
		values: map[string]string{
			service.SettingKeyCustomMenuItems: `[]`,
		},
	}
	handler := NewPageHandler(root, service.NewSettingService(repo, &config.Config{}))

	router := gin.New()
	router.GET("/api/v1/pages/:slug/images/*filename", handler.ServePageImage)

	encoded := url.PathEscape(imageName)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/pages/tutorial/images/"+encoded, nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d, body=%q", w.Code, http.StatusOK, w.Body.String())
	}
	if string(w.Body.Bytes()) != string(imageBytes) {
		t.Fatalf("image body = %q, want %q", w.Body.String(), string(imageBytes))
	}
}

func TestWriteUniquePageImage_DoesNotOverwriteExistingFile(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	first, err := writeUniquePageImage(dir, "教程截图-中文文件名", ".png", []byte("one"))
	if err != nil {
		t.Fatalf("write first image: %v", err)
	}
	second, err := writeUniquePageImage(dir, "教程截图-中文文件名", ".png", []byte("two"))
	if err != nil {
		t.Fatalf("write second image: %v", err)
	}

	if first != "教程截图-中文文件名.png" {
		t.Fatalf("first name = %q", first)
	}
	if second != "教程截图-中文文件名-2.png" {
		t.Fatalf("second name = %q", second)
	}

	rawFirst, err := os.ReadFile(filepath.Join(dir, first))
	if err != nil {
		t.Fatalf("read first image: %v", err)
	}
	rawSecond, err := os.ReadFile(filepath.Join(dir, second))
	if err != nil {
		t.Fatalf("read second image: %v", err)
	}
	if string(rawFirst) != "one" {
		t.Fatalf("first body = %q", rawFirst)
	}
	if string(rawSecond) != "two" {
		t.Fatalf("second body = %q", rawSecond)
	}
}

func TestTutorialDocumentRoundTripStable(t *testing.T) {
	gin.SetMode(gin.TestMode)
	root := t.TempDir()
	repo := &pageHandlerSettingRepoStub{
		values: map[string]string{
			service.SettingKeyCustomMenuItems: `[]`,
		},
	}
	handler := NewPageHandler(root, service.NewSettingService(repo, &config.Config{}))

	router := gin.New()
	router.GET("/api/v1/tutorial-document", handler.GetTutorialDocument)
	router.PUT("/api/v1/tutorial-document", handler.UpdateTutorialDocument)

	input := `<h1>sub2api 接入教程：中文显示测试</h1><p>这是一个用于验证中文显示的教程页面。</p><p><a href="https://example.com">查看完整中文接入说明</a></p><blockquote>“这是一个中文引用块。”</blockquote><pre><code class="language-javascript">console.log('safe code')</code></pre>`

	putBody := `{"content_html":` + strconv.Quote(input) + `}`
	w1 := httptest.NewRecorder()
	req1 := httptest.NewRequest(http.MethodPut, "/api/v1/tutorial-document", strings.NewReader(putBody))
	req1.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(w1, req1)
	if w1.Code != http.StatusOK {
		t.Fatalf("first put status = %d, want %d, body=%s", w1.Code, http.StatusOK, w1.Body.String())
	}

	var putResp1 struct {
		Code int `json:"code"`
		Data struct {
			ContentHTML string `json:"content_html"`
		} `json:"data"`
	}
	if err := json.Unmarshal(w1.Body.Bytes(), &putResp1); err != nil {
		t.Fatalf("unmarshal first put response: %v", err)
	}

	w2 := httptest.NewRecorder()
	req2 := httptest.NewRequest(http.MethodGet, "/api/v1/tutorial-document", nil)
	router.ServeHTTP(w2, req2)
	if w2.Code != http.StatusOK {
		t.Fatalf("get status = %d, want %d, body=%s", w2.Code, http.StatusOK, w2.Body.String())
	}
	var getResp struct {
		Code int `json:"code"`
		Data struct {
			ContentHTML string `json:"content_html"`
		} `json:"data"`
	}
	if err := json.Unmarshal(w2.Body.Bytes(), &getResp); err != nil {
		t.Fatalf("unmarshal get response: %v", err)
	}

	putBody2 := `{"content_html":` + strconv.Quote(getResp.Data.ContentHTML) + `}`
	w3 := httptest.NewRecorder()
	req3 := httptest.NewRequest(http.MethodPut, "/api/v1/tutorial-document", strings.NewReader(putBody2))
	req3.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(w3, req3)
	if w3.Code != http.StatusOK {
		t.Fatalf("second put status = %d, want %d, body=%s", w3.Code, http.StatusOK, w3.Body.String())
	}
	var putResp2 struct {
		Code int `json:"code"`
		Data struct {
			ContentHTML string `json:"content_html"`
		} `json:"data"`
	}
	if err := json.Unmarshal(w3.Body.Bytes(), &putResp2); err != nil {
		t.Fatalf("unmarshal second put response: %v", err)
	}

	if putResp1.Data.ContentHTML != getResp.Data.ContentHTML {
		t.Fatalf("stored content should match first sanitized response\nput=%s\nget=%s", putResp1.Data.ContentHTML, getResp.Data.ContentHTML)
	}
	if getResp.Data.ContentHTML != putResp2.Data.ContentHTML {
		t.Fatalf("tutorial document should be stable on save-read-save\nget=%s\nput2=%s", getResp.Data.ContentHTML, putResp2.Data.ContentHTML)
	}
	if !strings.Contains(putResp2.Data.ContentHTML, "<h1>sub2api 接入教程：中文显示测试</h1>") {
		t.Fatalf("expected heading to survive round trip, got %s", putResp2.Data.ContentHTML)
	}
	if !strings.Contains(putResp2.Data.ContentHTML, "查看完整中文接入说明") {
		t.Fatalf("expected safe link text to survive round trip, got %s", putResp2.Data.ContentHTML)
	}
}
