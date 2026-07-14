package jshandler

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/config"
	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
	"github.com/stretchr/testify/require"
)

type stubSettingRepoRW struct {
	values map[string]string
}

func (s *stubSettingRepoRW) GetValue(_ context.Context, key string) (string, error) {
	if s.values == nil {
		return "", nil
	}
	return s.values[key], nil
}

func (s *stubSettingRepoRW) Set(_ context.Context, key, value string) error {
	if s.values == nil {
		s.values = make(map[string]string)
	}
	s.values[key] = value
	return nil
}

func TestUpdateConfig_PersistsAndInvalidates(t *testing.T) {
	repo := &stubSettingRepoRW{}
	dir := t.TempDir()
	scriptsDir := filepath.Join(dir, "jshandler", "scripts")
	require.NoError(t, os.MkdirAll(scriptsDir, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(scriptsDir, "x.js"), []byte("//"), 0o600))
	svc := NewService(repo, &config.Config{Pricing: config.PricingConfig{DataDir: dir}})
	svc.InvalidateCache()
	_, err := svc.UpdateConfig(context.Background(), Config{Enabled: true, Timeout: "2s"})
	require.NoError(t, err)
	require.Contains(t, repo.values[SettingKeyJSHandlerConfig], `"enabled":true`)
	svc.InvalidateCache()
	require.True(t, svc.Enabled(context.Background()))
}

func TestReadScriptContent(t *testing.T) {
	repo := &stubSettingRepoRW{}
	dir := t.TempDir()
	svc := NewService(repo, &config.Config{Pricing: config.PricingConfig{DataDir: dir}})
	src := []byte("function on_after_auth_request(ctx) { return ctx; }\n")
	entry, err := svc.AddScript("preview-me", src)
	require.NoError(t, err)
	gotEntry, content, err := svc.ReadScriptContent(entry.ID)
	require.NoError(t, err)
	require.Equal(t, entry.ID, gotEntry.ID)
	require.Equal(t, "preview-me", gotEntry.Name)
	require.Equal(t, src, content)
	_, _, err = svc.ReadScriptContent("does-not-exist")
	require.Error(t, err)
	require.True(t, infraerrors.IsNotFound(err))
	_, _, err = svc.ReadScriptContent("../evil")
	require.Error(t, err)
	require.True(t, infraerrors.IsBadRequest(err))
}

func TestUpdateScript(t *testing.T) {
	repo := &stubSettingRepoRW{}
	dir := t.TempDir()
	svc := NewService(repo, &config.Config{Pricing: config.PricingConfig{DataDir: dir}})
	entry, err := svc.AddScript("old-name", []byte("function a(){}\n"))
	require.NoError(t, err)
	updated, err := svc.UpdateScript(entry.ID, "new-name", []byte("function b(){ return 1; }\n"))
	require.NoError(t, err)
	require.Equal(t, "new-name", updated.Name)
	_, content, err := svc.ReadScriptContent(entry.ID)
	require.NoError(t, err)
	require.Equal(t, "function b(){ return 1; }\n", string(content))
	// name only
	updated, err = svc.UpdateScript(entry.ID, "name-only", nil)
	require.NoError(t, err)
	require.Equal(t, "name-only", updated.Name)
	_, content, err = svc.ReadScriptContent(entry.ID)
	require.NoError(t, err)
	require.Equal(t, "function b(){ return 1; }\n", string(content))
	// empty content rejected
	_, err = svc.UpdateScript(entry.ID, "", []byte(""))
	require.Error(t, err)
	require.True(t, infraerrors.IsBadRequest(err))
	// whitespace content rejected
	_, err = svc.UpdateScript(entry.ID, "", []byte("   \n\t"))
	require.Error(t, err)
	require.True(t, infraerrors.IsBadRequest(err))
	// empty name + nil content rejected
	_, err = svc.UpdateScript(entry.ID, "  ", nil)
	require.Error(t, err)
	require.True(t, infraerrors.IsBadRequest(err))
	// same name is no-op success (no error)
	before, err := svc.UpdateScript(entry.ID, "name-only", nil)
	require.NoError(t, err)
	require.Equal(t, "name-only", before.Name)
	// not found
	_, err = svc.UpdateScript("missingid", "x", []byte("function z(){}"))
	require.Error(t, err)
	require.True(t, infraerrors.IsNotFound(err))
	// max size
	big := []byte(strings.Repeat("a", MaxScriptUploadBytes()+1))
	_, err = svc.UpdateScript(entry.ID, "", big)
	require.Error(t, err)
	require.True(t, infraerrors.IsBadRequest(err))
	// atomic write leaves no .tmp
	_, err = svc.UpdateScript(entry.ID, "final", []byte("function final(){}\n"))
	require.NoError(t, err)
	scriptsDir := filepath.Join(dir, "jshandler", "scripts")
	entries, err := os.ReadDir(scriptsDir)
	require.NoError(t, err)
	for _, e := range entries {
		require.False(t, strings.HasSuffix(e.Name(), ".tmp"), "leftover tmp file %s", e.Name())
	}
}
