package service

import (
	"context"
	"encoding/json"
	"reflect"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/pkg/openai"
	"github.com/stretchr/testify/require"
)

func TestGetCodexRestrictionPolicy(t *testing.T) {
	svc := NewSettingService(&codexPolicyMigrationRepoStub{values: map[string]string{
		SettingKeyMinCodexVersion:       "0.141.0",
		SettingKeyMaxCodexVersion:       "0.200.0",
		SettingKeyCodexCLIOnlyWhitelist: `[{"originator":"opencode","ua_contains":["opencode/"]}]`,
		SettingKeyCodexCLIOnlyBlacklist: `[{"originator":"evil"}]`,
	}}, &config.Config{})

	pol := svc.GetCodexRestrictionPolicy(context.Background())
	require.Equal(t, "0.141.0", pol.MinCodexVersion)
	require.Equal(t, "0.200.0", pol.MaxCodexVersion)
	require.Len(t, pol.Whitelist, 1)
	require.Equal(t, "opencode", pol.Whitelist[0].Originator)
	require.Equal(t, []string{"opencode/"}, pol.Whitelist[0].UAContains)
	require.Len(t, pol.Blacklist, 1)
	require.Equal(t, "evil", pol.Blacklist[0].Originator)
}

func TestGetCodexRestrictionPolicy_DefaultsSafe(t *testing.T) {
	svc := NewSettingService(&codexPolicyMigrationRepoStub{values: map[string]string{}}, &config.Config{})

	pol := svc.GetCodexRestrictionPolicy(context.Background())
	require.Empty(t, pol.MinCodexVersion)
	require.Empty(t, pol.Whitelist)
	require.Empty(t, pol.Blacklist)
}

func TestGetCodexRestrictionPolicy_InvalidJSONSafe(t *testing.T) {
	svc := NewSettingService(&codexPolicyMigrationRepoStub{values: map[string]string{
		SettingKeyCodexCLIOnlyWhitelist: "not-json",
		SettingKeyCodexCLIOnlyBlacklist: "{bad",
	}}, &config.Config{})

	pol := svc.GetCodexRestrictionPolicy(context.Background())
	require.Empty(t, pol.Whitelist, "非法 JSON → 安全空名单")
	require.Empty(t, pol.Blacklist, "非法 JSON → 安全空名单")
}

type codexPolicyMigrationRepoStub struct {
	values map[string]string
	sets   map[string]string
}

func (s *codexPolicyMigrationRepoStub) Get(ctx context.Context, key string) (*Setting, error) {
	panic("unused")
}
func (s *codexPolicyMigrationRepoStub) GetValue(ctx context.Context, key string) (string, error) {
	if v, ok := s.values[key]; ok {
		return v, nil
	}
	return "", ErrSettingNotFound
}
func (s *codexPolicyMigrationRepoStub) Set(ctx context.Context, key, value string) error {
	if s.sets == nil {
		s.sets = map[string]string{}
	}
	s.sets[key] = value
	s.values[key] = value
	return nil
}
func (s *codexPolicyMigrationRepoStub) GetMultiple(ctx context.Context, keys []string) (map[string]string, error) {
	panic("unused")
}
func (s *codexPolicyMigrationRepoStub) SetMultiple(ctx context.Context, settings map[string]string) error {
	panic("unused")
}
func (s *codexPolicyMigrationRepoStub) GetAll(ctx context.Context) (map[string]string, error) {
	panic("unused")
}
func (s *codexPolicyMigrationRepoStub) Delete(ctx context.Context, key string) error {
	panic("unused")
}

func TestMigrateOpenAIAllowClaudeCodeCodexPluginSetting(t *testing.T) {
	t.Run("legacy true appends Claude Code entry to whitelist", func(t *testing.T) {
		repo := &codexPolicyMigrationRepoStub{values: map[string]string{
			SettingKeyOpenAIAllowClaudeCodeCodexPlugin: "true",
			SettingKeyCodexCLIOnlyWhitelist:            `[{"originator":"opencode","ua_contains":["opencode/"]}]`,
		}}
		svc := NewSettingService(repo, &config.Config{})

		require.NoError(t, svc.MigrateOpenAIAllowClaudeCodeCodexPluginSetting(context.Background()))

		raw := repo.sets[SettingKeyCodexCLIOnlyWhitelist]
		require.NotEmpty(t, raw)
		var entries []struct {
			Originator string   `json:"originator"`
			UAContains []string `json:"ua_contains"`
		}
		require.NoError(t, json.Unmarshal([]byte(raw), &entries))
		require.Len(t, entries, 2)
		require.Equal(t, "opencode", entries[0].Originator)
		require.Equal(t, "Claude Code", entries[1].Originator)
		require.Equal(t, []string{"Claude Code/"}, entries[1].UAContains)
	})

	t.Run("legacy true does not duplicate existing Claude Code entry", func(t *testing.T) {
		repo := &codexPolicyMigrationRepoStub{values: map[string]string{
			SettingKeyOpenAIAllowClaudeCodeCodexPlugin: "true",
			SettingKeyCodexCLIOnlyWhitelist:            `[{"originator":"Claude Code","ua_contains":["Claude Code/"]}]`,
		}}
		svc := NewSettingService(repo, &config.Config{})

		require.NoError(t, svc.MigrateOpenAIAllowClaudeCodeCodexPluginSetting(context.Background()))

		_, wrote := repo.sets[SettingKeyCodexCLIOnlyWhitelist]
		require.False(t, wrote)
	})
}

func TestGetCodexRestrictionPolicy_AllowAppServerClients(t *testing.T) {
	t.Run("显式 true 开启", func(t *testing.T) {
		svc := NewSettingService(&codexPolicyMigrationRepoStub{values: map[string]string{
			SettingKeyCodexCLIOnlyAllowAppServerClients: "true",
		}}, &config.Config{})
		require.True(t, svc.GetCodexRestrictionPolicy(context.Background()).AllowAppServerClients)
	})
	t.Run("缺失默认 false", func(t *testing.T) {
		svc := NewSettingService(&codexPolicyMigrationRepoStub{values: map[string]string{}}, &config.Config{})
		require.False(t, svc.GetCodexRestrictionPolicy(context.Background()).AllowAppServerClients)
	})
	t.Run("非 true 值视为 false", func(t *testing.T) {
		svc := NewSettingService(&codexPolicyMigrationRepoStub{values: map[string]string{
			SettingKeyCodexCLIOnlyAllowAppServerClients: "1",
		}}, &config.Config{})
		require.False(t, svc.GetCodexRestrictionPolicy(context.Background()).AllowAppServerClients)
	})
}

func TestGetCodexRestrictionPolicy_EngineFingerprintSignals(t *testing.T) {
	t.Run("键缺失 → 默认种子(只勾x-codex-)", func(t *testing.T) {
		svc := NewSettingService(&codexPolicyMigrationRepoStub{values: map[string]string{}}, &config.Config{})
		pol := svc.GetCodexRestrictionPolicy(context.Background())
		require.True(t, len(pol.EngineFingerprintSignals) > 0)
		require.True(t, openaiEngineSignalsEqual(pol.EngineFingerprintSignals, openai.DefaultEngineFingerprintSignals))
	})
	t.Run("显式配置 → 原样采用", func(t *testing.T) {
		raw := `[{"type":"header_exact","match":["session-id"],"required":true}]`
		svc := NewSettingService(&codexPolicyMigrationRepoStub{values: map[string]string{
			SettingKeyCodexCLIOnlyEngineFingerprintSignals: raw,
		}}, &config.Config{})
		pol := svc.GetCodexRestrictionPolicy(context.Background())
		require.Len(t, pol.EngineFingerprintSignals, 1)
		require.Equal(t, "session-id", pol.EngineFingerprintSignals[0].Match[0])
	})
	t.Run("非法JSON → 回落默认种子", func(t *testing.T) {
		svc := NewSettingService(&codexPolicyMigrationRepoStub{values: map[string]string{
			SettingKeyCodexCLIOnlyEngineFingerprintSignals: "not json",
		}}, &config.Config{})
		pol := svc.GetCodexRestrictionPolicy(context.Background())
		require.True(t, openaiEngineSignalsEqual(pol.EngineFingerprintSignals, openai.DefaultEngineFingerprintSignals))
	})
}

func openaiEngineSignalsEqual(a, b []openai.EngineFingerprintSignal) bool {
	return reflect.DeepEqual(a, b)
}

func TestMigrateCodexBodyFingerprintToSignals(t *testing.T) {
	t.Run("信号键已存在 → 不动", func(t *testing.T) {
		repo := &codexPolicyMigrationRepoStub{values: map[string]string{
			SettingKeyCodexCLIOnlyEngineFingerprintSignals:   `[{"type":"header_exact","match":["session-id"],"required":true}]`,
			SettingKeyCodexCLIOnlyAllowBodyEngineFingerprint: "true",
		}}
		svc := NewSettingService(repo, &config.Config{})
		require.NoError(t, svc.MigrateCodexBodyFingerprintToSignals(context.Background()))
		require.Equal(t, `[{"type":"header_exact","match":["session-id"],"required":true}]`, repo.values[SettingKeyCodexCLIOnlyEngineFingerprintSignals])
	})
	t.Run("信号键缺失 + 旧body=true → 写种子且body行Required=true", func(t *testing.T) {
		repo := &codexPolicyMigrationRepoStub{values: map[string]string{
			SettingKeyCodexCLIOnlyAllowBodyEngineFingerprint: "true",
		}}
		svc := NewSettingService(repo, &config.Config{})
		require.NoError(t, svc.MigrateCodexBodyFingerprintToSignals(context.Background()))
		sigs, ok := openai.ParseEngineFingerprintSignals(repo.values[SettingKeyCodexCLIOnlyEngineFingerprintSignals])
		require.True(t, ok)
		var bodyReq bool
		for _, s := range sigs {
			if s.Type == openai.FingerprintSignalBodyPath {
				bodyReq = s.Required
			}
		}
		require.True(t, bodyReq)
	})
	t.Run("信号键缺失 + 旧body=false/缺 → 写种子(body不勾)", func(t *testing.T) {
		repo := &codexPolicyMigrationRepoStub{values: map[string]string{}}
		svc := NewSettingService(repo, &config.Config{})
		require.NoError(t, svc.MigrateCodexBodyFingerprintToSignals(context.Background()))
		require.Equal(t, openai.DefaultEngineFingerprintSignalsJSON(), repo.values[SettingKeyCodexCLIOnlyEngineFingerprintSignals])
	})
}
