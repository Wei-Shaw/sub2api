//go:build integration

package repository

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/stretchr/testify/require"
)

func TestPluginRepositoryLifecycleIsAtomicAndOptimistic(t *testing.T) {
	ctx := context.Background()
	repo := &pluginRepository{db: integrationDB}
	pluginKey := "local.test.repository-" + strings.ToLower(time.Now().Format("150405.000000000"))
	defer func() {
		_, _ = integrationDB.ExecContext(ctx, `DELETE FROM sub2api_plugin_installations WHERE plugin_key = $1`, pluginKey)
	}()

	manifest := service.PluginManifest{SchemaVersion: 1, ID: pluginKey, Name: "测试插件", Version: "1.0.0"}
	first := &service.PluginInstallation{
		PluginKey: pluginKey, Name: "测试插件", Version: "1.0.0", Manifest: manifest,
		ArtifactData: []byte("first-package"), ArtifactPath: "/tmp/first.s2plugin",
		InstallPath: "/tmp/first", BinaryPath: "/tmp/first/plugin",
		BinarySHA256: strings.Repeat("a", 64), SignatureStatus: service.PluginSignatureTrusted,
		State: service.PluginStateDisabled,
	}
	bindings := []service.PluginBinding{{
		Capability: service.PluginCapabilityOpenAIOAuthOutbound,
		Platform:   service.PlatformOpenAI, AccountType: service.AccountTypeOAuth,
		RolloutPercent: 100,
	}}
	installed, err := repo.Install(ctx, first, bindings)
	require.NoError(t, err)
	artifact, err := repo.GetArtifact(ctx, installed.ID)
	require.NoError(t, err)
	require.Equal(t, first.ArtifactData, artifact)

	require.NoError(t, repo.BeginEnable(ctx, installed.ID, first.BinarySHA256, service.PluginStateDisabled))
	bindings[0].Enabled = true
	now := time.Now()
	require.NoError(t, repo.UpdateBindingsAndState(
		ctx, installed.ID, bindings, service.PluginStateEnabled, "", &now,
		service.PluginStateStarting, first.BinarySHA256,
	))

	second := *first
	second.Version = "1.1.0"
	second.Manifest.Version = second.Version
	second.BinarySHA256 = strings.Repeat("b", 64)
	second.ArtifactData = []byte("second-package")
	_, err = repo.Install(ctx, &second, bindings)
	require.ErrorIs(t, err, service.ErrPluginStateChanged)

	bindings[0].Enabled = false
	require.NoError(t, repo.UpdateBindingsAndState(
		ctx, installed.ID, bindings, service.PluginStateDisabled, "", nil, "", first.BinarySHA256,
	))
	replaced, err := repo.Install(ctx, &second, bindings)
	require.NoError(t, err)
	require.Equal(t, installed.ID, replaced.ID)

	err = repo.Delete(ctx, replaced.ID, first.BinarySHA256)
	require.True(t, errors.Is(err, service.ErrPluginStateChanged))
	require.NoError(t, repo.Delete(ctx, replaced.ID, second.BinarySHA256))
}
