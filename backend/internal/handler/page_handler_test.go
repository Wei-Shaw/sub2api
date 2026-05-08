package handler

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
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
