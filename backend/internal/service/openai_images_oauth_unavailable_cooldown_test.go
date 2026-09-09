//go:build unit

package service

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/stretchr/testify/require"
)

func TestOpenAIImagesOAuthUnavailableCooldownSettingsDefaultAndStoredValue(t *testing.T) {
	repo := newMockSettingRepo()
	svc := NewSettingService(repo, &config.Config{})

	settings, err := svc.GetOpenAIImagesOAuthUnavailableCooldownSettings(context.Background())
	require.NoError(t, err)
	require.Equal(t, 30, settings.CooldownMinutes)

	require.NoError(t, svc.SetOpenAIImagesOAuthUnavailableCooldownSettings(context.Background(), &OpenAIImagesOAuthUnavailableCooldownSettings{CooldownMinutes: 7}))
	settings, err = svc.GetOpenAIImagesOAuthUnavailableCooldownSettings(context.Background())
	require.NoError(t, err)
	require.Equal(t, 7, settings.CooldownMinutes)
}

func TestSetOpenAIImagesOAuthUnavailableCooldownSettingsBoundaries(t *testing.T) {
	svc := NewSettingService(newMockSettingRepo(), &config.Config{})

	for _, minutes := range []int{1, openAIImagesOAuthUnavailableMaxCooldownMinutes} {
		err := svc.SetOpenAIImagesOAuthUnavailableCooldownSettings(context.Background(), &OpenAIImagesOAuthUnavailableCooldownSettings{CooldownMinutes: minutes})
		require.NoError(t, err, "should accept cooldown_minutes=%d", minutes)
	}

	for _, minutes := range []int{0, -1, openAIImagesOAuthUnavailableMaxCooldownMinutes + 1} {
		err := svc.SetOpenAIImagesOAuthUnavailableCooldownSettings(context.Background(), &OpenAIImagesOAuthUnavailableCooldownSettings{CooldownMinutes: minutes})
		require.ErrorContains(t, err, "cooldown_minutes must be between 1-120")
	}
}

func TestOpenAIImagesOAuthUnavailableCooldownSettingsRejectsOverflowingValue(t *testing.T) {
	maxInt := int(^uint(0) >> 1)
	repo := newMockSettingRepo()
	svc := NewSettingService(repo, &config.Config{})

	err := svc.SetOpenAIImagesOAuthUnavailableCooldownSettings(context.Background(), &OpenAIImagesOAuthUnavailableCooldownSettings{CooldownMinutes: maxInt})
	require.ErrorContains(t, err, "cooldown_minutes must be between 1-120")

	for _, minutes := range []int{openAIImagesOAuthUnavailableMaxCooldownMinutes + 1, maxInt} {
		data, marshalErr := json.Marshal(OpenAIImagesOAuthUnavailableCooldownSettings{CooldownMinutes: minutes})
		require.NoError(t, marshalErr)
		repo.data[SettingKeyOpenAIImagesOAuthUnavailableCooldownSettings] = string(data)

		settings, getErr := svc.GetOpenAIImagesOAuthUnavailableCooldownSettings(context.Background())
		require.NoError(t, getErr)
		require.Equal(t, openAIImagesOAuthUnavailableDefaultCooldownMinutes, settings.CooldownMinutes)
	}
}
