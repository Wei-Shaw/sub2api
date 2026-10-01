//go:build unit

package service

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/pkg/xai"
)

func TestGetGrokCLIIdentityPolicyPriority(t *testing.T) {
	t.Setenv(xai.CLIVersionEnv, "1.0.20")

	for _, tt := range []struct {
		name   string
		unify  string
		manual string
		want   xai.CLIIdentityPolicy
	}{
		{
			name:   "默认统一并使用手填版本",
			unify:  "true",
			manual: "1.0.45",
			want:   xai.CLIIdentityPolicy{Unify: true, Version: "1.0.45"},
		},
		{
			name:   "手填优先于环境变量",
			unify:  "true",
			manual: "1.0.30",
			want:   xai.CLIIdentityPolicy{Unify: true, Version: "1.0.30"},
		},
		{
			name:  "空手填回退环境变量",
			unify: "true",
			want:  xai.CLIIdentityPolicy{Unify: true, Version: "1.0.20"},
		},
		{
			name:   "无效手填回退环境变量",
			unify:  "true",
			manual: "0.2.120",
			want:   xai.CLIIdentityPolicy{Unify: true, Version: "1.0.20"},
		},
		{
			name:   "关闭统一仍解析版本",
			unify:  "false",
			manual: "1.0.30",
			want:   xai.CLIIdentityPolicy{Unify: false, Version: "1.0.30"},
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			repo := &authSourceDefaultsRepoStub{values: map[string]string{
				SettingKeyUnifyGrokClientVersion: tt.unify,
				SettingKeyGrokCLIClientVersion:   tt.manual,
			}}
			svc := NewSettingService(repo, &config.Config{})
			got := svc.GetGrokCLIIdentityPolicy(context.Background())
			require.Equal(t, tt.want, got)
		})
	}
}

func TestUpdateSettingsGrokCLIIdentityTakesEffectImmediately(t *testing.T) {
	t.Setenv(xai.CLIVersionEnv, "")
	ctx := context.Background()
	repo := &authSourceDefaultsRepoStub{values: map[string]string{
		SettingKeyUnifyGrokClientVersion: "true",
	}}
	svc := NewSettingService(repo, &config.Config{})
	require.Equal(t, xai.CLIIdentityPolicy{Unify: true, Version: xai.CLIClientVersion}, svc.GetGrokCLIIdentityPolicy(ctx))

	settings := &SystemSettings{
		UnifyGrokClientVersion: false,
		GrokCLIClientVersion:   "1.0.20",
	}
	require.NoError(t, svc.UpdateSettings(ctx, settings))
	require.Equal(t, xai.CLIIdentityPolicy{Unify: false, Version: "1.0.20"}, svc.GetGrokCLIIdentityPolicy(ctx))
	require.Equal(t, "false", repo.values[SettingKeyUnifyGrokClientVersion])
	require.Equal(t, "1.0.20", repo.values[SettingKeyGrokCLIClientVersion])
}

func TestNormalizeGrokCLIClientVersion(t *testing.T) {
	require.Equal(t, "1.0.44", NormalizeGrokCLIClientVersion(" v1.0.44 "))
	require.Equal(t, "1.0.13", NormalizeGrokCLIClientVersion("1.0.13"))
	require.Empty(t, NormalizeGrokCLIClientVersion("0.2.120"))
	require.Empty(t, NormalizeGrokCLIClientVersion("1.0.12"))
	require.Empty(t, NormalizeGrokCLIClientVersion(""))
}
