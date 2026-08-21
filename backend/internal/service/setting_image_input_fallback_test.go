package service

import (
	"context"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/stretchr/testify/require"
)

type imageInputFallbackSettingRepo struct {
	values map[string]string
}

func (r *imageInputFallbackSettingRepo) Get(_ context.Context, key string) (*Setting, error) {
	value, ok := r.values[key]
	if !ok {
		return nil, ErrSettingNotFound
	}
	return &Setting{Key: key, Value: value}, nil
}

func (r *imageInputFallbackSettingRepo) GetValue(_ context.Context, key string) (string, error) {
	value, ok := r.values[key]
	if !ok {
		return "", ErrSettingNotFound
	}
	return value, nil
}

func (r *imageInputFallbackSettingRepo) Set(_ context.Context, key, value string) error {
	if r.values == nil {
		r.values = make(map[string]string)
	}
	r.values[key] = value
	return nil
}

func (r *imageInputFallbackSettingRepo) GetMultiple(_ context.Context, keys []string) (map[string]string, error) {
	out := make(map[string]string, len(keys))
	for _, key := range keys {
		if value, ok := r.values[key]; ok {
			out[key] = value
		}
	}
	return out, nil
}

func (r *imageInputFallbackSettingRepo) SetMultiple(_ context.Context, settings map[string]string) error {
	for key, value := range settings {
		if err := r.Set(context.Background(), key, value); err != nil {
			return err
		}
	}
	return nil
}

func (r *imageInputFallbackSettingRepo) GetAll(_ context.Context) (map[string]string, error) {
	out := make(map[string]string, len(r.values))
	for key, value := range r.values {
		out[key] = value
	}
	return out, nil
}

func (r *imageInputFallbackSettingRepo) Delete(_ context.Context, key string) error {
	delete(r.values, key)
	return nil
}

func TestImageInputFallbackSettingsDefaultsAndRoundTrip(t *testing.T) {
	repo := &imageInputFallbackSettingRepo{values: map[string]string{}}
	svc := NewSettingService(repo, &config.Config{})
	ctx := context.Background()

	// 未配置时返回默认（mode 为空）
	settings, err := svc.GetImageInputFallbackSettings(ctx)
	require.NoError(t, err)
	require.Equal(t, "", settings.Mode)

	// 保存后能读回
	err = svc.SetImageInputFallbackSettings(ctx, &ImageInputFallbackSettings{
		Mode:                 ImageInputFallbackModeDescribe,
		Models:               "gpt-4,gpt-3.5-turbo",
		VisionBaseURL:        "https://api.example.com/v1",
		VisionAPIKey:         "sk-test",
		VisionModel:          "gpt-4o-mini",
		VisionTimeoutSeconds: 30,
	})
	require.NoError(t, err)

	settings, err = svc.GetImageInputFallbackSettings(ctx)
	require.NoError(t, err)
	require.Equal(t, ImageInputFallbackModeDescribe, settings.Mode)
	require.Equal(t, "gpt-4,gpt-3.5-turbo", settings.Models)
	require.Equal(t, "https://api.example.com/v1", settings.VisionBaseURL)
	require.Equal(t, "sk-test", settings.VisionAPIKey)
	require.Equal(t, "gpt-4o-mini", settings.VisionModel)
	require.Equal(t, 30, settings.VisionTimeoutSeconds)
}

func TestImageInputFallbackSettingsRejectsInvalidMode(t *testing.T) {
	repo := &imageInputFallbackSettingRepo{values: map[string]string{}}
	svc := NewSettingService(repo, &config.Config{})
	err := svc.SetImageInputFallbackSettings(context.Background(), &ImageInputFallbackSettings{Mode: "bogus"})
	require.Error(t, err)
}

func TestImageInputFallbackEffectiveSettingsFallsBackToEnv(t *testing.T) {
	repo := &imageInputFallbackSettingRepo{values: map[string]string{}}
	cfg := &config.Config{}
	cfg.Gateway.ImageInputFallback = "strip"
	cfg.Gateway.ImageInputVision.BaseURL = "https://api.env.example.com/v1"
	svc := NewSettingService(repo, cfg)
	ctx := context.Background()

	// DB 未配置 → 回退环境变量
	effective, err := svc.GetImageInputFallbackEffectiveSettings(ctx)
	require.NoError(t, err)
	require.Equal(t, "strip", effective.Mode)
	require.Equal(t, "https://api.env.example.com/v1", effective.VisionBaseURL)

	// DB 配置后 → DB 优先
	err = svc.SetImageInputFallbackSettings(ctx, &ImageInputFallbackSettings{
		Mode:          ImageInputFallbackModeDescribe,
		VisionBaseURL: "https://api.db.example.com/v1",
		VisionAPIKey:  "sk-db",
		VisionModel:   "gpt-4o",
	})
	require.NoError(t, err)
	effective, err = svc.GetImageInputFallbackEffectiveSettings(ctx)
	require.NoError(t, err)
	require.Equal(t, ImageInputFallbackModeDescribe, effective.Mode)
	require.Equal(t, "https://api.db.example.com/v1", effective.VisionBaseURL)
}
