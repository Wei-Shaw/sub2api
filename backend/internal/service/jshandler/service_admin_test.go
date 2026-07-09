package jshandler

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/config"
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