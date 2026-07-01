package service

import (
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/model"
	"github.com/stretchr/testify/require"
)

func TestTLSFingerprintProfileService_ResolveTLSProfileOpenAI(t *testing.T) {
	svc := &TLSFingerprintProfileService{}

	openAIOAuth := &Account{
		Platform: PlatformOpenAI,
		Type:     AccountTypeOAuth,
		Extra:    map[string]any{"enable_tls_fingerprint": true},
	}
	require.NotNil(t, svc.ResolveTLSProfile(openAIOAuth), "OpenAI OAuth 开启后应返回内置默认 profile")

	openAIAPIKey := &Account{
		Platform: PlatformOpenAI,
		Type:     AccountTypeAPIKey,
		Extra:    map[string]any{"enable_tls_fingerprint": true},
	}
	require.Nil(t, svc.ResolveTLSProfile(openAIAPIKey), "OpenAI API Key 不应启用 TLS 指纹伪装")
}

func TestOpenAIGatewayService_ResolveTLSProfileRouterFallback(t *testing.T) {
	account := &Account{
		Platform: PlatformOpenAI,
		Type:     AccountTypeOAuth,
		Extra: map[string]any{
			"enable_tls_fingerprint":     true,
			"tls_fingerprint_profile_id": int64(10),
		},
	}
	profileSvc := &TLSFingerprintProfileService{
		localCache: map[int64]*model.TLSFingerprintProfile{
			10: {ID: 10, Name: "fixed"},
			20: {ID: 20, Name: "router"},
		},
	}
	svc := &OpenAIGatewayService{tlsFPProfileService: profileSvc}

	// 路由器命中优先使用规则目标模板。
	routerProfile := svc.resolveOpenAITLSProfile(account, TLSFingerprintRouterMatchResult{
		Matched:                 true,
		TLSFingerprintProfileID: 20,
	})
	require.NotNil(t, routerProfile)
	require.Equal(t, "router", routerProfile.Name)

	// 规则目标模板不可用时安全回退账号固定模板。
	fallbackProfile := svc.resolveOpenAITLSProfile(account, TLSFingerprintRouterMatchResult{
		Matched:                 true,
		TLSFingerprintProfileID: 404,
	})
	require.NotNil(t, fallbackProfile)
	require.Equal(t, "fixed", fallbackProfile.Name)
}
