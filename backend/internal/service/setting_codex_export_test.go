//go:build unit

package service

import (
	"context"
	"strings"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/stretchr/testify/require"
)

// TestNormalizeCodexDefaultModel covers empty, namespaced, boundary and unsafe IDs.
func TestNormalizeCodexDefaultModel(t *testing.T) {
	for _, raw := range []string{"", "   ", " gpt-6.1-sol ", "provider/model:latest", strings.Repeat("a", 200)} {
		t.Run(raw, func(t *testing.T) {
			model, err := normalizeCodexDefaultModel(raw)
			require.NoError(t, err)
			require.Equal(t, strings.TrimSpace(raw), model)
		})
	}
	for _, raw := range []string{"model name", "model\nname", `model"`, `model\name`, "模型", strings.Repeat("a", 201)} {
		_, err := normalizeCodexDefaultModel(raw)
		require.Error(t, err)
	}
}

// TestCodexDefaultModelPublicSettings ensures API and SSR both expose the saved preference.
func TestCodexDefaultModelPublicSettings(t *testing.T) {
	for _, model := range []string{"", "gpt-6.1-sol"} {
		repo := &settingPublicRepoStub{values: map[string]string{SettingKeyCodexDefaultModel: model}}
		svc := NewSettingService(repo, &config.Config{})
		settings, err := svc.GetPublicSettings(context.Background())
		require.NoError(t, err)
		require.Equal(t, model, settings.CodexDefaultModel)
		payload, err := svc.GetPublicSettingsForInjection(context.Background())
		require.NoError(t, err)
		require.IsType(t, &PublicSettingsInjectionPayload{}, payload)
		require.Equal(t, model, payload.(*PublicSettingsInjectionPayload).CodexDefaultModel)
	}
}
