package service

import (
	"context"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/model"
	"github.com/stretchr/testify/require"
)

// claudeDefaultStubSettingRepo 是最小 SettingRepository 桩;仅用于让 SettingService.settingRepo
// 非 nil(通过 getter 的守卫)。测试通过预填充 gatewayForwardingCache 走 fast-path,这些方法不会被调用。
type claudeDefaultStubSettingRepo struct{}

func (r *claudeDefaultStubSettingRepo) Get(context.Context, string) (*Setting, error) {
	return nil, nil
}
func (r *claudeDefaultStubSettingRepo) GetValue(context.Context, string) (string, error) {
	return "", nil
}
func (r *claudeDefaultStubSettingRepo) Set(context.Context, string, string) error { return nil }
func (r *claudeDefaultStubSettingRepo) GetMultiple(context.Context, []string) (map[string]string, error) {
	return map[string]string{}, nil
}
func (r *claudeDefaultStubSettingRepo) SetMultiple(context.Context, map[string]string) error {
	return nil
}
func (r *claudeDefaultStubSettingRepo) GetAll(context.Context) (map[string]string, error) {
	return map[string]string{}, nil
}
func (r *claudeDefaultStubSettingRepo) Delete(context.Context, string) error { return nil }

// TestGatewayService_ResolveTLSProfileForRequest_ClaudeCodeDefaultFallback 验证 Anthropic 账号维度
// 的全局默认 Claude Code 指纹回落:消除「claude-cli UA + Go 握手」不自洽,且严格向后兼容。
func TestGatewayService_ResolveTLSProfileForRequest_ClaudeCodeDefaultFallback(t *testing.T) {
	profileSvc := &TLSFingerprintProfileService{
		localCache: map[int64]*model.TLSFingerprintProfile{
			20: {ID: 20, Name: "claude-default"},
			30: {ID: 30, Name: "account-fixed"},
		},
	}

	// 账号自身未配指纹的 Anthropic OAuth 账号(默认态,出站会被伪装成 claude-cli)。
	anthropicNoProfile := &Account{Platform: PlatformAnthropic, Type: AccountTypeOAuth}

	// 未配置全局默认(settingService=nil)→ 返回 nil,行为逐字同现状(向后兼容,退化为 plain Do)。
	gwNoDefault := &GatewayService{tlsFPProfileService: profileSvc}
	require.Nil(t, gwNoDefault.resolveTLSProfileForRequest(nil, anthropicNoProfile),
		"未配全局默认时应返回 nil(向后兼容)")

	// 预填充网关转发设置缓存的 fast-path,使 GetDefaultClaudeCodeTLSFingerprintProfileID 返回 20(无需 DB)。
	gatewayForwardingCache.Store(&cachedGatewayForwardingSettings{
		defaultClaudeCodeTLSProfileID: 20,
		expiresAt:                     time.Now().Add(time.Hour).UnixNano(),
	})
	defer gatewayForwardingCache.Store(&cachedGatewayForwardingSettings{expiresAt: 0}) // 清理:置过期,后续读取自然重载

	gwWithDefault := &GatewayService{
		tlsFPProfileService: profileSvc,
		settingService:      &SettingService{settingRepo: &claudeDefaultStubSettingRepo{}},
	}

	// 配了全局默认 → Anthropic OAuth 账号无自有指纹时回落到全局默认 profile。
	got := gwWithDefault.resolveTLSProfileForRequest(nil, anthropicNoProfile)
	require.NotNil(t, got, "配置全局默认后应回落 profile 而非 nil")
	require.Equal(t, "claude-default", got.Name, "应回落到全局默认 Claude Code 指纹")

	// 账号自有固定 profile 优先于全局默认(级联:账号 profile > 全局默认)。
	anthropicOwnProfile := &Account{
		Platform: PlatformAnthropic,
		Type:     AccountTypeOAuth,
		Extra: map[string]any{
			"enable_tls_fingerprint":     true,
			"tls_fingerprint_profile_id": int64(30),
		},
	}
	gotOwn := gwWithDefault.resolveTLSProfileForRequest(nil, anthropicOwnProfile)
	require.NotNil(t, gotOwn)
	require.Equal(t, "account-fixed", gotOwn.Name, "账号自有 profile 应优先于全局默认")

	// 非 Anthropic 账号(如 OpenAI)不触发 Claude Code 全局默认回落。
	openAINoProfile := &Account{Platform: PlatformOpenAI, Type: AccountTypeOAuth}
	require.Nil(t, gwWithDefault.resolveTLSProfileForRequest(nil, openAINoProfile),
		"非 Anthropic 账号不应回落 Claude Code 全局默认")
}
