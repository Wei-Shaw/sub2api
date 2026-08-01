package service

import (
	"context"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

type customMediaModelSettingRepo struct {
	values map[string]string
}

func (r *customMediaModelSettingRepo) Get(context.Context, string) (*Setting, error) {
	return nil, ErrSettingNotFound
}
func (r *customMediaModelSettingRepo) GetValue(_ context.Context, key string) (string, error) {
	value, ok := r.values[key]
	if !ok {
		return "", ErrSettingNotFound
	}
	return value, nil
}
func (r *customMediaModelSettingRepo) Set(context.Context, string, string) error { return nil }
func (r *customMediaModelSettingRepo) GetMultiple(context.Context, []string) (map[string]string, error) {
	return map[string]string{}, nil
}
func (r *customMediaModelSettingRepo) SetMultiple(context.Context, map[string]string) error {
	return nil
}
func (r *customMediaModelSettingRepo) GetAll(context.Context) (map[string]string, error) {
	return r.values, nil
}
func (r *customMediaModelSettingRepo) Delete(context.Context, string) error { return nil }

func TestCustomMediaModelPatterns(t *testing.T) {
	previous := activeCustomMediaModelRules.Load()
	t.Cleanup(func() { activeCustomMediaModelRules.Store(previous) })

	setCustomMediaModelPatterns("acme-image-*\nexact-image", "acme-video-*, acme-image-video-*")

	require.True(t, isOpenAIImageGenerationModel("ACME-IMAGE-V2"))
	require.True(t, isOpenAIImageGenerationModel("exact-image"))
	require.False(t, isOpenAIImageGenerationModel("acme-video-v1"))
	require.Equal(t, mediaModelVideo, classifyCustomMediaModel("acme-video-v1"))
	require.Equal(t, mediaModelVideo, classifyCustomMediaModel("acme-image-video-v1"))
	require.False(t, isOpenAIImageGenerationModel("unlisted-model"))
}

func TestCustomMediaModelPatternsVideoWins(t *testing.T) {
	previous := activeCustomMediaModelRules.Load()
	t.Cleanup(func() { activeCustomMediaModelRules.Store(previous) })

	setCustomMediaModelPatterns("shared-*", "shared-video-*")

	require.Equal(t, mediaModelVideo, classifyCustomMediaModel("shared-video-v2"))
	require.False(t, isOpenAIImageGenerationModel("shared-video-v2"))
	require.True(t, isOpenAIImageGenerationModel("shared-image-v2"))
}

func TestNormalizeCustomMediaModelPatterns(t *testing.T) {
	require.Equal(t, "foo-*\nbar", normalizeCustomMediaModelPatterns(" Foo-* , bar\nfoo-* "))
	require.NoError(t, ValidateCustomMediaModelPatterns(strings.Repeat("a", 256)))
	require.Error(t, ValidateCustomMediaModelPatterns(strings.Repeat("a", 257)))
}

func TestLoadCustomMediaModelPatterns(t *testing.T) {
	previous := activeCustomMediaModelRules.Load()
	t.Cleanup(func() { activeCustomMediaModelRules.Store(previous) })

	svc := NewSettingService(&customMediaModelSettingRepo{values: map[string]string{
		SettingKeyCustomImageModelPatterns: "db-image-*",
		SettingKeyCustomVideoModelPatterns: "db-video-*",
	}}, nil)

	require.NoError(t, svc.LoadCustomMediaModelPatterns(context.Background()))
	require.True(t, isOpenAIImageGenerationModel("db-image-v1"))
	require.Equal(t, mediaModelVideo, classifyCustomMediaModel("db-video-v1"))
}
