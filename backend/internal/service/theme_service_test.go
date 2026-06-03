package service

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// --- Stub repository ---

type themeRepoStub struct {
	values map[string]string
}

func (s *themeRepoStub) GetValue(_ context.Context, key string) (string, error) {
	v, ok := s.values[key]
	if !ok {
		return "", nil
	}
	return v, nil
}

func (s *themeRepoStub) Set(_ context.Context, key, value string) error {
	if s.values == nil {
		s.values = make(map[string]string)
	}
	s.values[key] = value
	return nil
}

func (s *themeRepoStub) Delete(_ context.Context, key string) error {
	delete(s.values, key)
	return nil
}

func (s *themeRepoStub) Get(_ context.Context, key string) (*Setting, error) {
	panic("unexpected Get call")
}

func (s *themeRepoStub) GetMultiple(_ context.Context, keys []string) (map[string]string, error) {
	panic("unexpected GetMultiple call")
}

func (s *themeRepoStub) SetMultiple(_ context.Context, settings map[string]string) error {
	panic("unexpected SetMultiple call")
}

func (s *themeRepoStub) GetAll(_ context.Context) (map[string]string, error) {
	panic("unexpected GetAll call")
}

// --- Test helpers ---

func newTestThemeService(t *testing.T, repo SettingRepository) *ThemeService {
	t.Helper()
	svc := NewThemeService(repo)
	svc.dataDir = t.TempDir()
	return svc
}

func createThemeZip(t *testing.T, files map[string]string) []byte {
	t.Helper()
	var buf bytes.Buffer
	w := zip.NewWriter(&buf)

	for name, content := range files {
		f, err := w.Create(name)
		require.NoError(t, err)
		_, err = f.Write([]byte(content))
		require.NoError(t, err)
	}

	err := w.Close()
	require.NoError(t, err)
	return buf.Bytes()
}

func validMetadata(name, short string) string {
	return `{
		"name": "` + name + `",
		"short": "` + short + `",
		"version": "1.0.0",
		"author": "Test",
		"description": "A test theme",
		"main_css": "style.css"
	}`
}

// --- InstallFromZip tests ---

func TestThemeService_InstallFromZip_Success(t *testing.T) {
	repo := &themeRepoStub{}
	svc := newTestThemeService(t, repo)

	zipData := createThemeZip(t, map[string]string{
		"theme/sub2api-theme.json": validMetadata("Test Theme", "test"),
		"theme/style.css":          "body { color: red; }\n",
	})

	theme, err := svc.InstallFromZip(context.Background(), zipData)
	require.NoError(t, err)
	require.NotNil(t, theme)
	assert.Equal(t, "Test Theme", theme.Metadata.Name)
	assert.Equal(t, "test", theme.Metadata.Short)
	assert.Equal(t, "1.0.0", theme.Metadata.Version)
	assert.Equal(t, "Test", theme.Metadata.Author)
	assert.False(t, theme.IsActive)
}

func TestThemeService_InstallFromZip_DefaultMainCSS(t *testing.T) {
	repo := &themeRepoStub{}
	svc := newTestThemeService(t, repo)

	zipData := createThemeZip(t, map[string]string{
		"theme/sub2api-theme.json": `{"name":"T","short":"t","version":"1"}`,
	})

	theme, err := svc.InstallFromZip(context.Background(), zipData)
	require.NoError(t, err)
	assert.Equal(t, "style.css", theme.Metadata.MainCSS)
}

func TestThemeService_InstallFromZip_ActiveOnReinstall(t *testing.T) {
	repo := &themeRepoStub{}
	_ = repo.Set(context.Background(), "active_theme", "mytheme")
	svc := newTestThemeService(t, repo)

	zipData := createThemeZip(t, map[string]string{
		"theme/sub2api-theme.json": `{"name":"M","short":"mytheme","version":"1"}`,
	})

	theme, err := svc.InstallFromZip(context.Background(), zipData)
	require.NoError(t, err)
	assert.True(t, theme.IsActive)
}

func TestThemeService_InstallFromZip_InvalidZip(t *testing.T) {
	svc := newTestThemeService(t, &themeRepoStub{})
	_, err := svc.InstallFromZip(context.Background(), []byte("not a zip"))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "invalid zip file")
}

func TestThemeService_InstallFromZip_MissingMetadata(t *testing.T) {
	svc := newTestThemeService(t, &themeRepoStub{})
	zipData := createThemeZip(t, map[string]string{
		"style.css": "body { color: red; }",
	})
	_, err := svc.InstallFromZip(context.Background(), zipData)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "not found")
}

func TestThemeService_InstallFromZip_InvalidJSONMetadata(t *testing.T) {
	svc := newTestThemeService(t, &themeRepoStub{})
	zipData := createThemeZip(t, map[string]string{
		"sub2api-theme.json": "{invalid}",
	})
	_, err := svc.InstallFromZip(context.Background(), zipData)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "invalid theme metadata")
}

func TestThemeService_InstallFromZip_MissingName(t *testing.T) {
	svc := newTestThemeService(t, &themeRepoStub{})
	zipData := createThemeZip(t, map[string]string{
		"sub2api-theme.json": `{"short":"t","version":"1"}`,
	})
	_, err := svc.InstallFromZip(context.Background(), zipData)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "name is required")
}

func TestThemeService_InstallFromZip_NameTooLong(t *testing.T) {
	svc := newTestThemeService(t, &themeRepoStub{})
	name := strings.Repeat("a", 65)
	zipData := createThemeZip(t, map[string]string{
		"sub2api-theme.json": `{"name":"` + name + `","short":"t","version":"1"}`,
	})
	_, err := svc.InstallFromZip(context.Background(), zipData)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "name too long")
}

func TestThemeService_InstallFromZip_InvalidShort(t *testing.T) {
	svc := newTestThemeService(t, &themeRepoStub{})
	tests := []struct {
		name  string
		short string
	}{
		{"uppercase", "MY-THEME"},
		{"too_long", strings.Repeat("a", 33)},
		{"empty", ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			zipData := createThemeZip(t, map[string]string{
				"sub2api-theme.json": `{"name":"T","short":"` + tt.short + `","version":"1"}`,
			})
			_, err := svc.InstallFromZip(context.Background(), zipData)
			require.Error(t, err)
		})
	}
}

func TestThemeService_InstallFromZip_MissingVersion(t *testing.T) {
	svc := newTestThemeService(t, &themeRepoStub{})
	zipData := createThemeZip(t, map[string]string{
		"sub2api-theme.json": `{"name":"T","short":"t"}`,
	})
	_, err := svc.InstallFromZip(context.Background(), zipData)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "version is required")
}

func TestThemeService_InstallFromZip_PathTraversal(t *testing.T) {
	svc := newTestThemeService(t, &themeRepoStub{})
	tests := []string{
		"../outside.txt",
		"../../etc/passwd",
		"foo/../../../etc/passwd",
	}
	for _, path := range tests {
		t.Run(path, func(t *testing.T) {
			zipData := createThemeZip(t, map[string]string{
				path: "evil",
			})
			_, err := svc.InstallFromZip(context.Background(), zipData)
			require.Error(t, err)
			assert.Contains(t, err.Error(), "invalid file path")
		})
	}
}

func TestThemeService_InstallFromZip_DisallowedFileType(t *testing.T) {
	svc := newTestThemeService(t, &themeRepoStub{})
	tests := []string{"script.js", "page.html", "file.exe", "file.sh", "file.php"}
	for _, name := range tests {
		t.Run(name, func(t *testing.T) {
			zipData := createThemeZip(t, map[string]string{
				"sub2api-theme.json": validMetadata("T", "t"),
				name:                 "content",
			})
			_, err := svc.InstallFromZip(context.Background(), zipData)
			require.Error(t, err)
			assert.Contains(t, err.Error(), "file type not allowed")
		})
	}
}

func TestThemeService_InstallFromZip_TooLargeTotal(t *testing.T) {
	svc := newTestThemeService(t, &themeRepoStub{})

	// Create 11 x 2MB files of highly compressible data (repeated 'a').
	// After Deflate, each compresses to ~1KB, so total zip stays well under 10MB.
	// But UncompressedSize64 is 22MB, which exceeds the 20MB total limit.
	files := map[string]string{
		"sub2api-theme.json": validMetadata("T", "t"),
	}
	for i := 0; i < 11; i++ {
		files[fmt.Sprintf("data%d.json", i)] = strings.Repeat("a", 2*1024*1024)
	}
	zipData := createThemeZip(t, files)

	_, err := svc.InstallFromZip(context.Background(), zipData)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "extracted content too large")
}

func TestThemeService_InstallFromZip_FileTooLarge(t *testing.T) {
	svc := newTestThemeService(t, &themeRepoStub{})
	// Create a zip with a file over the 2MB limit by padding
	largeContent := strings.Repeat("x", 3*1024*1024)
	zipData := createThemeZip(t, map[string]string{
		"sub2api-theme.json": validMetadata("T", "t"),
		"large.png":          largeContent,
	})
	_, err := svc.InstallFromZip(context.Background(), zipData)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "file too large")
}

func TestThemeService_InstallFromZip_DangerousCSS(t *testing.T) {
	svc := newTestThemeService(t, &themeRepoStub{})
	tests := []struct {
		name    string
		css     string
		keyword string
	}{
		{"javascript", "a { background: url(javascript:alert(1)); }", "javascript:"},
		{"expression", "a { width: expression(1); }", "expression("},
		{"behavior", "a { behavior: url(h.htc); }", "behavior:"},
		{"moz_binding", "a { -moz-binding: url(x); }", "-moz-binding"},
		{"vbscript", "a { background: url(vbscript:msg); }", "vbscript:"},
		{"data_html", "a { background: url(data:text/html,<script>); }", "data:text/html"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			zipData := createThemeZip(t, map[string]string{
				"sub2api-theme.json": validMetadata("T", "t"),
				"style.css":          tt.css,
			})
			_, err := svc.InstallFromZip(context.Background(), zipData)
			require.Error(t, err)
			assert.Contains(t, err.Error(), tt.keyword)
		})
	}
}

func TestThemeService_InstallFromZip_ExternalCSSURLStripped(t *testing.T) {
	svc := newTestThemeService(t, &themeRepoStub{})
	// External URLs in CSS should be stripped, but install should succeed
	css := `.a { background: url(https://evil.com/hack.css); }
.b { background: url("http://evil.com/hack.css"); }`
	zipData := createThemeZip(t, map[string]string{
		"sub2api-theme.json": validMetadata("T", "t"),
		"style.css":          css,
	})
	_, err := svc.InstallFromZip(context.Background(), zipData)
	require.NoError(t, err)
}

func TestThemeService_InstallFromZip_ExternalCSSImportRemoved(t *testing.T) {
	svc := newTestThemeService(t, &themeRepoStub{})
	css := `@import url(https://evil.com/hack.css);
body { color: red; }`
	zipData := createThemeZip(t, map[string]string{
		"sub2api-theme.json": validMetadata("T", "t"),
		"style.css":          css,
	})
	_, err := svc.InstallFromZip(context.Background(), zipData)
	require.NoError(t, err)
}

func TestThemeService_InstallFromZip_DataFontCSSAllowed(t *testing.T) {
	svc := newTestThemeService(t, &themeRepoStub{})
	css := `@font-face {
	font-family: 'Custom';
	src: url(data:font/woff2;base64,abc123);
}`
	zipData := createThemeZip(t, map[string]string{
		"sub2api-theme.json": validMetadata("T", "t"),
		"style.css":          css,
	})
	_, err := svc.InstallFromZip(context.Background(), zipData)
	require.NoError(t, err)
}

func TestThemeService_InstallFromZip_CommentsRemovedFromCSS(t *testing.T) {
	svc := newTestThemeService(t, &themeRepoStub{})
	css := `/* comment */ body { color: red; } /* another */`
	zipData := createThemeZip(t, map[string]string{
		"sub2api-theme.json": validMetadata("T", "t"),
		"style.css":          css,
	})
	_, err := svc.InstallFromZip(context.Background(), zipData)
	require.NoError(t, err)
}

func TestThemeService_InstallFromZip_ReinstallOverwrites(t *testing.T) {
	repo := &themeRepoStub{}
	svc := newTestThemeService(t, repo)

	zip1 := createThemeZip(t, map[string]string{
		"sub2api-theme.json": validMetadata("V1", "test"),
		"style.css":          "body { color: red; }",
	})
	_, err := svc.InstallFromZip(context.Background(), zip1)
	require.NoError(t, err)

	zip2 := createThemeZip(t, map[string]string{
		"sub2api-theme.json": validMetadata("V2", "test"),
		"style.css":          "body { color: blue; }",
	})
	theme2, err := svc.InstallFromZip(context.Background(), zip2)
	require.NoError(t, err)
	assert.Equal(t, "V2", theme2.Metadata.Name)
}

func TestThemeService_InstallFromZip_InvalidConfigType(t *testing.T) {
	svc := newTestThemeService(t, &themeRepoStub{})
	meta := `{
		"name": "T",
		"short": "t",
		"version": "1",
		"config": [{"key":"x","label":"X","type":"invalid_type"}]
	}`
	zipData := createThemeZip(t, map[string]string{
		"sub2api-theme.json": meta,
	})
	_, err := svc.InstallFromZip(context.Background(), zipData)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "invalid theme config type")
}

func TestThemeService_InstallFromZip_ConfigItemMissingKey(t *testing.T) {
	svc := newTestThemeService(t, &themeRepoStub{})
	meta := `{
		"name": "T",
		"short": "t",
		"version": "1",
		"config": [{"label":"X","type":"text"}]
	}`
	zipData := createThemeZip(t, map[string]string{
		"sub2api-theme.json": meta,
	})
	_, err := svc.InstallFromZip(context.Background(), zipData)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "config item key is required")
}

func TestThemeService_InstallFromZip_SelectWithoutOptions(t *testing.T) {
	svc := newTestThemeService(t, &themeRepoStub{})
	meta := `{
		"name": "T",
		"short": "t",
		"version": "1",
		"config": [{"key":"x","label":"X","type":"select"}]
	}`
	zipData := createThemeZip(t, map[string]string{
		"sub2api-theme.json": meta,
	})
	_, err := svc.InstallFromZip(context.Background(), zipData)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "must have options")
}

// --- InstallFromGitHub tests ---

func TestThemeService_InstallFromGitHub_InvalidURL(t *testing.T) {
	svc := newTestThemeService(t, &themeRepoStub{})
	tests := []string{"not-a-url", "https://gitlab.com/user/repo", ""}
	for _, url := range tests {
		_, err := svc.InstallFromGitHub(context.Background(), url)
		require.Error(t, err)
	}
}

// --- List tests ---

func TestThemeService_List_Empty(t *testing.T) {
	svc := newTestThemeService(t, &themeRepoStub{})
	themes, err := svc.List(context.Background())
	require.NoError(t, err)
	assert.Empty(t, themes)
}

func TestThemeService_List_Multiple(t *testing.T) {
	repo := &themeRepoStub{}
	svc := newTestThemeService(t, repo)

	zip1 := createThemeZip(t, map[string]string{
		"sub2api-theme.json": validMetadata("Alpha", "alpha"),
	})
	_, err := svc.InstallFromZip(context.Background(), zip1)
	require.NoError(t, err)

	zip2 := createThemeZip(t, map[string]string{
		"sub2api-theme.json": validMetadata("Beta", "beta"),
	})
	_, err = svc.InstallFromZip(context.Background(), zip2)
	require.NoError(t, err)

	themes, err := svc.List(context.Background())
	require.NoError(t, err)
	assert.Len(t, themes, 2)
}

func TestThemeService_List_ActiveFlag(t *testing.T) {
	repo := &themeRepoStub{}
	_ = repo.Set(context.Background(), "active_theme", "active")
	svc := newTestThemeService(t, repo)

	for _, short := range []string{"active", "inactive"} {
		zipData := createThemeZip(t, map[string]string{
			"sub2api-theme.json": validMetadata(short, short),
		})
		_, err := svc.InstallFromZip(context.Background(), zipData)
		require.NoError(t, err)
	}

	themes, err := svc.List(context.Background())
	require.NoError(t, err)
	for _, th := range themes {
		if th.Metadata.Short == "active" {
			assert.True(t, th.IsActive)
		} else {
			assert.False(t, th.IsActive)
		}
	}
}

// --- Get tests ---

func TestThemeService_Get_Success(t *testing.T) {
	svc := newTestThemeService(t, &themeRepoStub{})
	zipData := createThemeZip(t, map[string]string{
		"sub2api-theme.json": validMetadata("MyTheme", "mytheme"),
	})
	_, err := svc.InstallFromZip(context.Background(), zipData)
	require.NoError(t, err)

	theme, err := svc.Get(context.Background(), "mytheme")
	require.NoError(t, err)
	require.NotNil(t, theme)
	assert.Equal(t, "MyTheme", theme.Metadata.Name)
}

func TestThemeService_Get_NotFound(t *testing.T) {
	svc := newTestThemeService(t, &themeRepoStub{})
	_, err := svc.Get(context.Background(), "nonexistent")
	require.Error(t, err)
}

func TestThemeService_Get_InvalidShort(t *testing.T) {
	svc := newTestThemeService(t, &themeRepoStub{})
	_, err := svc.Get(context.Background(), "UPPERCASE")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "invalid theme identifier")
}

// --- GetActiveTheme tests ---

func TestThemeService_GetActiveTheme_None(t *testing.T) {
	svc := newTestThemeService(t, &themeRepoStub{})
	theme, err := svc.GetActiveTheme(context.Background())
	require.NoError(t, err)
	assert.Nil(t, theme)
}

func TestThemeService_GetActiveTheme_Success(t *testing.T) {
	repo := &themeRepoStub{}
	_ = repo.Set(context.Background(), "active_theme", "myt")
	svc := newTestThemeService(t, repo)

	zipData := createThemeZip(t, map[string]string{
		"sub2api-theme.json": validMetadata("MyTheme", "myt"),
	})
	_, err := svc.InstallFromZip(context.Background(), zipData)
	require.NoError(t, err)

	theme, err := svc.GetActiveTheme(context.Background())
	require.NoError(t, err)
	require.NotNil(t, theme)
	assert.Equal(t, "myt", theme.Metadata.Short)
	assert.True(t, theme.IsActive)
}

// --- Activate tests ---

func TestThemeService_Activate_Success(t *testing.T) {
	repo := &themeRepoStub{}
	svc := newTestThemeService(t, repo)

	zipData := createThemeZip(t, map[string]string{
		"sub2api-theme.json": validMetadata("T", "myt"),
	})
	_, err := svc.InstallFromZip(context.Background(), zipData)
	require.NoError(t, err)

	err = svc.Activate(context.Background(), "myt")
	require.NoError(t, err)

	active, _ := repo.GetValue(context.Background(), "active_theme")
	assert.Equal(t, "myt", active)
}

func TestThemeService_Activate_NotFound(t *testing.T) {
	svc := newTestThemeService(t, &themeRepoStub{})
	err := svc.Activate(context.Background(), "nonexistent")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "theme not found")
}

func TestThemeService_Activate_InvalidShort(t *testing.T) {
	svc := newTestThemeService(t, &themeRepoStub{})
	err := svc.Activate(context.Background(), "UPPERCASE")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "invalid theme identifier")
}

func TestThemeService_Activate_TriggersCallback(t *testing.T) {
	repo := &themeRepoStub{}
	svc := newTestThemeService(t, repo)

	zipData := createThemeZip(t, map[string]string{
		"sub2api-theme.json": validMetadata("T", "myt"),
	})
	_, err := svc.InstallFromZip(context.Background(), zipData)
	require.NoError(t, err)

	called := false
	svc.SetOnUpdateCallback(func() { called = true })

	err = svc.Activate(context.Background(), "myt")
	require.NoError(t, err)
	assert.True(t, called)
}

// --- Deactivate tests ---

func TestThemeService_Deactivate_Success(t *testing.T) {
	repo := &themeRepoStub{}
	svc := newTestThemeService(t, repo)

	_ = repo.Set(context.Background(), "active_theme", "myt")

	err := svc.Deactivate(context.Background())
	require.NoError(t, err)

	active, _ := repo.GetValue(context.Background(), "active_theme")
	assert.Empty(t, active)
}

func TestThemeService_Deactivate_NoActive(t *testing.T) {
	repo := &themeRepoStub{}
	svc := newTestThemeService(t, repo)

	err := svc.Deactivate(context.Background())
	require.NoError(t, err)
}

func TestThemeService_Deactivate_TriggersCallback(t *testing.T) {
	repo := &themeRepoStub{}
	svc := newTestThemeService(t, repo)

	_ = repo.Set(context.Background(), "active_theme", "myt")

	called := false
	svc.SetOnUpdateCallback(func() { called = true })

	err := svc.Deactivate(context.Background())
	require.NoError(t, err)
	assert.True(t, called)
}

// --- Delete tests ---

func TestThemeService_Delete_Success(t *testing.T) {
	svc := newTestThemeService(t, &themeRepoStub{})
	zipData := createThemeZip(t, map[string]string{
		"sub2api-theme.json": validMetadata("T", "myt"),
	})
	_, err := svc.InstallFromZip(context.Background(), zipData)
	require.NoError(t, err)

	err = svc.Delete(context.Background(), "myt")
	require.NoError(t, err)

	_, err = svc.Get(context.Background(), "myt")
	require.Error(t, err)
}

func TestThemeService_Delete_DeactivatesIfActive(t *testing.T) {
	repo := &themeRepoStub{}
	svc := newTestThemeService(t, repo)

	zipData := createThemeZip(t, map[string]string{
		"sub2api-theme.json": validMetadata("T", "myt"),
	})
	_, err := svc.InstallFromZip(context.Background(), zipData)
	require.NoError(t, err)

	_ = repo.Set(context.Background(), "active_theme", "myt")

	err = svc.Delete(context.Background(), "myt")
	require.NoError(t, err)

	active, _ := repo.GetValue(context.Background(), "active_theme")
	assert.Empty(t, active, "active_theme should be cleared")
}

func TestThemeService_Delete_InvalidShort(t *testing.T) {
	svc := newTestThemeService(t, &themeRepoStub{})
	err := svc.Delete(context.Background(), "UPPERCASE")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "invalid theme identifier")
}

// --- UpdateConfig tests ---

func TestThemeService_UpdateConfig_Success(t *testing.T) {
	repo := &themeRepoStub{}
	svc := newTestThemeService(t, repo)

	err := svc.UpdateConfig(context.Background(), "myt", map[string]string{
		"primary": "#ff0000",
	})
	require.NoError(t, err)

	stored, _ := repo.GetValue(context.Background(), "theme_config_myt")
	var cfg map[string]string
	err = json.Unmarshal([]byte(stored), &cfg)
	require.NoError(t, err)
	assert.Equal(t, "#ff0000", cfg["primary"])
}

func TestThemeService_UpdateConfig_TriggersCallback(t *testing.T) {
	repo := &themeRepoStub{}
	svc := newTestThemeService(t, repo)

	called := false
	svc.SetOnUpdateCallback(func() { called = true })

	err := svc.UpdateConfig(context.Background(), "myt", map[string]string{"x": "y"})
	require.NoError(t, err)
	assert.True(t, called)
}

func TestThemeService_UpdateConfig_InvalidShort(t *testing.T) {
	svc := newTestThemeService(t, &themeRepoStub{})
	err := svc.UpdateConfig(context.Background(), "UPPERCASE", map[string]string{})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "invalid theme identifier")
}

// --- GetActiveThemeShort tests ---

func TestThemeService_GetActiveThemeShort_None(t *testing.T) {
	svc := newTestThemeService(t, &themeRepoStub{})
	assert.Empty(t, svc.GetActiveThemeShort(context.Background()))
}

func TestThemeService_GetActiveThemeShort_Active(t *testing.T) {
	repo := &themeRepoStub{}
	_ = repo.Set(context.Background(), "active_theme", "myt")
	svc := newTestThemeService(t, repo)
	assert.Equal(t, "myt", svc.GetActiveThemeShort(context.Background()))
}

// --- GetThemeConfigCSS tests ---

func TestThemeService_GetThemeConfigCSS_NoActiveTheme(t *testing.T) {
	svc := newTestThemeService(t, &themeRepoStub{})
	assert.Empty(t, svc.GetThemeConfigCSS(context.Background()))
}

func TestThemeService_GetThemeConfigCSS_NoConfig(t *testing.T) {
	repo := &themeRepoStub{}
	_ = repo.Set(context.Background(), "active_theme", "myt")
	svc := newTestThemeService(t, repo)

	zipData := createThemeZip(t, map[string]string{
		"sub2api-theme.json": validMetadata("T", "myt"),
	})
	_, err := svc.InstallFromZip(context.Background(), zipData)
	require.NoError(t, err)

	assert.Empty(t, svc.GetThemeConfigCSS(context.Background()))
}

func TestThemeService_GetThemeConfigCSS_GeneratesVars(t *testing.T) {
	repo := &themeRepoStub{}
	_ = repo.Set(context.Background(), "active_theme", "myt")
	svc := newTestThemeService(t, repo)

	meta := `{
		"name": "T", "short": "myt", "version": "1",
		"config": [
			{"key":"primary","label":"Primary","type":"color","default":"#000"},
			{"key":"radius","label":"Radius","type":"text","default":"8px"}
		]
	}`
	zipData := createThemeZip(t, map[string]string{
		"sub2api-theme.json": meta,
	})
	_, err := svc.InstallFromZip(context.Background(), zipData)
	require.NoError(t, err)

	_ = svc.UpdateConfig(context.Background(), "myt", map[string]string{
		"primary": "#ff0000",
	})

	css := svc.GetThemeConfigCSS(context.Background())
	assert.Contains(t, css, "--theme-primary: #ff0000;")
	assert.Contains(t, css, "--theme-radius: 8px;")
	assert.Contains(t, css, ":root {")
	assert.Contains(t, css, "}")
}

// --- ServeThemeAsset tests ---

func TestThemeService_ServeThemeAsset_Success(t *testing.T) {
	svc := newTestThemeService(t, &themeRepoStub{})
	zipData := createThemeZip(t, map[string]string{
		"sub2api-theme.json": validMetadata("T", "myt"),
		"style.css":          "body { color: red; }",
		"logo.png":           "fake-png-data",
	})
	_, err := svc.InstallFromZip(context.Background(), zipData)
	require.NoError(t, err)

	content, contentType, err := svc.ServeThemeAsset(context.Background(), "myt", "style.css")
	require.NoError(t, err)
	assert.Contains(t, string(content), "color: red")
	assert.Equal(t, "text/css; charset=utf-8", contentType)

	content, contentType, err = svc.ServeThemeAsset(context.Background(), "myt", "logo.png")
	require.NoError(t, err)
	assert.Equal(t, "fake-png-data", string(content))
	assert.Equal(t, "image/png", contentType)
}

func TestThemeService_ServeThemeAsset_DefaultsToMainCSS(t *testing.T) {
	svc := newTestThemeService(t, &themeRepoStub{})
	zipData := createThemeZip(t, map[string]string{
		"sub2api-theme.json": validMetadata("T", "myt"),
		"style.css":          "body { color: red; }",
	})
	_, err := svc.InstallFromZip(context.Background(), zipData)
	require.NoError(t, err)

	// Empty asset path should serve main CSS
	content, contentType, err := svc.ServeThemeAsset(context.Background(), "myt", "")
	require.NoError(t, err)
	assert.Contains(t, string(content), "color: red")
	assert.Equal(t, "text/css; charset=utf-8", contentType)
}

func TestThemeService_ServeThemeAsset_PathTraversal(t *testing.T) {
	svc := newTestThemeService(t, &themeRepoStub{})
	zipData := createThemeZip(t, map[string]string{
		"sub2api-theme.json": validMetadata("T", "myt"),
		"style.css":          "body { color: red; }",
	})
	_, err := svc.InstallFromZip(context.Background(), zipData)
	require.NoError(t, err)

	tests := []string{"../secret.txt", "../../etc/passwd", "foo/../../../etc/passwd"}
	for _, path := range tests {
		_, _, err := svc.ServeThemeAsset(context.Background(), "myt", path)
		require.Error(t, err, "path: %s", path)
	}
}

func TestThemeService_ServeThemeAsset_NotFound(t *testing.T) {
	svc := newTestThemeService(t, &themeRepoStub{})
	zipData := createThemeZip(t, map[string]string{
		"sub2api-theme.json": validMetadata("T", "myt"),
	})
	_, err := svc.InstallFromZip(context.Background(), zipData)
	require.NoError(t, err)

	_, _, err = svc.ServeThemeAsset(context.Background(), "myt", "nonexistent.css")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "file not found")
}

func TestThemeService_ServeThemeAsset_InvalidShort(t *testing.T) {
	svc := newTestThemeService(t, &themeRepoStub{})
	_, _, err := svc.ServeThemeAsset(context.Background(), "UPPERCASE", "style.css")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "invalid theme identifier")
}

func TestThemeService_ServeThemeAsset_DirectoryRefused(t *testing.T) {
	svc := newTestThemeService(t, &themeRepoStub{})
	zipData := createThemeZip(t, map[string]string{
		"sub2api-theme.json": validMetadata("T", "myt"),
		"style.css":          "body { color: red; }",
	})
	_, err := svc.InstallFromZip(context.Background(), zipData)
	require.NoError(t, err)

	// The theme directory itself exists, trying to access it as a file should fail
	_, _, err = svc.ServeThemeAsset(context.Background(), "myt", ".")
	require.Error(t, err)
}

func TestThemeService_ServeThemeAsset_ThemeNotInstalled(t *testing.T) {
	svc := newTestThemeService(t, &themeRepoStub{})
	_, _, err := svc.ServeThemeAsset(context.Background(), "nonexistent", "style.css")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "file not found")
}

// --- sanitizeCSS tests ---

func TestSanitizeCSS_CommentsRemoved(t *testing.T) {
	input := []byte("/* comment */ body { color: red; } /* another */")
	out, err := sanitizeCSS(input)
	require.NoError(t, err)
	assert.NotContains(t, string(out), "/*")
}

func TestSanitizeCSS_DangerousConstructs(t *testing.T) {
	tests := []struct {
		name    string
		css     string
		keyword string
	}{
		{"expression", "a { width: expression(1); }", "expression("},
		{"moz_binding", "a { -moz-binding: url(x); }", "-moz-binding"},
		{"behavior_colon", "a { behavior: url(h.htc); }", "behavior:"},
		{"javascript_url", "a { background: url(javascript:alert(1)); }", "javascript:"},
		{"vbscript_url", "a { background: url(vbscript:msg); }", "vbscript:"},
		{"data_html", "a { background: url(data:text/html,<script>); }", "data:text/html"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := sanitizeCSS([]byte(tt.css))
			require.Error(t, err)
			assert.Contains(t, err.Error(), tt.keyword)
		})
	}
}

func TestSanitizeCSS_ExternalURLStripped(t *testing.T) {
	input := []byte(`.a { background: url(https://evil.com/hack.css); }`)
	out, err := sanitizeCSS(input)
	require.NoError(t, err)
	assert.NotContains(t, string(out), "evil.com")
	assert.NotContains(t, string(out), "url(")
}

func TestSanitizeCSS_RelativeURLAllowed(t *testing.T) {
	input := []byte(`.a { background: url(images/bg.png); }`)
	out, err := sanitizeCSS(input)
	require.NoError(t, err)
	assert.Contains(t, string(out), "url(images/bg.png)")
}

func TestSanitizeCSS_DataFontAllowed(t *testing.T) {
	input := []byte(`@font-face { src: url(data:font/woff2;base64,abc123); }`)
	out, err := sanitizeCSS(input)
	require.NoError(t, err)
	assert.Contains(t, string(out), "data:font/woff2")
}

func TestSanitizeCSS_DataApplicationFontAllowed(t *testing.T) {
	input := []byte(`@font-face { src: url(data:application/font-woff;base64,abc); }`)
	out, err := sanitizeCSS(input)
	require.NoError(t, err)
	assert.Contains(t, string(out), "data:application/font")
}

func TestSanitizeCSS_ExternalImportRemoved(t *testing.T) {
	input := []byte("@import url(https://evil.com/hack.css);\nbody { color: red; }")
	out, err := sanitizeCSS(input)
	require.NoError(t, err)
	assert.NotContains(t, string(out), "evil.com")
	assert.NotContains(t, string(out), "@import")
}

func TestSanitizeCSS_SafeCSSPreserved(t *testing.T) {
	input := []byte(`body { color: red; font-size: 16px; }
.class { margin: 0 auto; }
#id { display: flex; }`)
	out, err := sanitizeCSS(input)
	require.NoError(t, err)
	assert.Equal(t, string(input), string(out))
}

// --- parseGitHubURL tests ---

func TestParseGitHubURL(t *testing.T) {
	tests := []struct {
		name      string
		url       string
		wantOwner string
		wantRepo  string
		wantErr   bool
	}{
		{"full_url", "https://github.com/owner/repo", "owner", "repo", false},
		{"trailing_slash", "https://github.com/owner/repo/", "owner", "repo", false},
		{"no_protocol", "github.com/owner/repo", "owner", "repo", false},
		{"with_dot_git", "https://github.com/owner/repo.git", "owner", "repo", false},
		{"not_github", "https://gitlab.com/owner/repo", "", "", true},
		{"empty", "", "", "", true},
		{"just_domain", "github.com", "", "", true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			owner, repo, err := parseGitHubURL(tt.url)
			if tt.wantErr {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
				assert.Equal(t, tt.wantOwner, owner)
				assert.Equal(t, tt.wantRepo, repo)
			}
		})
	}
}

// --- mimeByExt tests ---

func TestMimeByExt(t *testing.T) {
	tests := []struct {
		ext      string
		expected string
	}{
		{".css", "text/css; charset=utf-8"},
		{".CSS", "text/css; charset=utf-8"},
		{".woff", "font/woff"},
		{".woff2", "font/woff2"},
		{".ttf", "font/ttf"},
		{".otf", "font/otf"},
		{".png", "image/png"},
		{".jpg", "image/jpeg"},
		{".jpeg", "image/jpeg"},
		{".gif", "image/gif"},
		{".svg", "image/svg+xml"},
		{".ico", "image/x-icon"},
		{".webp", "image/webp"},
		{".json", "application/json"},
		{".unknown", "application/octet-stream"},
		{"", "application/octet-stream"},
	}
	for _, tt := range tests {
		t.Run(tt.ext, func(t *testing.T) {
			assert.Equal(t, tt.expected, mimeByExt(tt.ext))
		})
	}
}

// --- OnUpdateCallback integration ---

func TestThemeService_OnUpdateCallback(t *testing.T) {
	repo := &themeRepoStub{}
	svc := newTestThemeService(t, repo)

	zipData := createThemeZip(t, map[string]string{
		"sub2api-theme.json": validMetadata("T", "myt"),
	})
	_, err := svc.InstallFromZip(context.Background(), zipData)
	require.NoError(t, err)

	// Install should NOT trigger onUpdate
	called := 0
	svc.SetOnUpdateCallback(func() { called++ })

	// Activate triggers callback
	_ = svc.Activate(context.Background(), "myt")
	assert.Equal(t, 1, called)

	// Deactivate triggers callback
	_ = svc.Deactivate(context.Background())
	assert.Equal(t, 2, called)

	// UpdateConfig triggers callback
	_ = svc.UpdateConfig(context.Background(), "myt", map[string]string{"x": "y"})
	assert.Equal(t, 3, called)
}

// --- Edge: Zip with a root directory ---

func TestThemeService_InstallFromZip_RootDirStripped(t *testing.T) {
	svc := newTestThemeService(t, &themeRepoStub{})
	zipData := createThemeZip(t, map[string]string{
		"theme-main/sub2api-theme.json": validMetadata("T", "t"),
		"theme-main/style.css":          "body { color: red; }",
	})
	_, err := svc.InstallFromZip(context.Background(), zipData)
	require.NoError(t, err)

	theme, err := svc.Get(context.Background(), "t")
	require.NoError(t, err)
	assert.Equal(t, "T", theme.Metadata.Name)
}

// --- Edge: Verify directory content on disk ---

func TestThemeService_InstallFromZip_FilesOnDisk(t *testing.T) {
	svc := newTestThemeService(t, &themeRepoStub{})
	zipData := createThemeZip(t, map[string]string{
		"sub2api-theme.json": validMetadata("T", "myt"),
		"style.css":          "body { color: red; }",
		"logo.png":           "png-data",
	})
	_, err := svc.InstallFromZip(context.Background(), zipData)
	require.NoError(t, err)

	// Verify files exist on disk
	cssContent, err := os.ReadFile(filepath.Join(svc.dataDir, "myt", "style.css"))
	require.NoError(t, err)
	assert.Contains(t, string(cssContent), "color: red")

	pngContent, err := os.ReadFile(filepath.Join(svc.dataDir, "myt", "logo.png"))
	require.NoError(t, err)
	assert.Equal(t, "png-data", string(pngContent))
}

// --- Edge: Large zip file rejected ---

func TestThemeService_InstallFromZip_ZipTooLarge(t *testing.T) {
	svc := newTestThemeService(t, &themeRepoStub{})
	// Create data larger than 10MB limit
	data := make([]byte, 11*1024*1024)
	_, err := svc.InstallFromZip(context.Background(), data)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "zip file too large")
}

// --- Edge: CSS with nested/weird comments ---

func TestSanitizeCSS_NestedComments(t *testing.T) {
	// CSS doesn't support nested comments, but our regex handles them
	input := []byte("/* outer /* inner */ still text */")
	out, err := sanitizeCSS(input)
	require.NoError(t, err)
	assert.NotContains(t, string(out), "/*")
	// After removing non-nested comments, " still text */" has trailing garbage
	// but that's acceptable
}

// --- Edge: Empty config yields empty CSS ---

func TestGetThemeConfigCSS_EmptyConfig(t *testing.T) {
	repo := &themeRepoStub{}
	_ = repo.Set(context.Background(), "active_theme", "myt")
	svc := newTestThemeService(t, repo)

	zipData := createThemeZip(t, map[string]string{
		"sub2api-theme.json": validMetadata("T", "myt"),
	})
	_, err := svc.InstallFromZip(context.Background(), zipData)
	require.NoError(t, err)

	css := svc.GetThemeConfigCSS(context.Background())
	assert.Empty(t, css)
}

// --- Edge: Author too long ---

func TestInstallFromZip_AuthorTooLong(t *testing.T) {
	svc := newTestThemeService(t, &themeRepoStub{})
	author := strings.Repeat("a", 129)
	meta := `{"name":"T","short":"t","version":"1","author":"` + author + `"}`
	zipData := createThemeZip(t, map[string]string{
		"sub2api-theme.json": meta,
	})
	_, err := svc.InstallFromZip(context.Background(), zipData)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "author too long")
}

// --- Edge: Description too long ---

func TestInstallFromZip_DescriptionTooLong(t *testing.T) {
	svc := newTestThemeService(t, &themeRepoStub{})
	desc := strings.Repeat("a", 513)
	meta := `{"name":"T","short":"t","version":"1","description":"` + desc + `"}`
	zipData := createThemeZip(t, map[string]string{
		"sub2api-theme.json": meta,
	})
	_, err := svc.InstallFromZip(context.Background(), zipData)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "description too long")
}

// --- Edge: CSS sanitize blocks dangerous content even inside comments ---

func TestSanitizeCSS_DangerousInCommentBlocked(t *testing.T) {
	// Comments are removed first, so dangerous content hidden in comments
	// should be checked against the remaining content (which won't catch it,
	// but since comments are removed, the dangerous content is gone too).
	// Edge case: comment removal happens before danger check.
	input := []byte("/* javascript:alert(1) */ body { color: red; }")
	out, err := sanitizeCSS(input)
	require.NoError(t, err)
	assert.NotContains(t, string(out), "javascript:")
	assert.NotContains(t, string(out), "/*")
}
