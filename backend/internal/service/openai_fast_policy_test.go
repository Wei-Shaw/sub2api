package service

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/pkg/ctxkey"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

type openAIFastPolicyRepoStub struct {
	values map[string]string
}

func (s *openAIFastPolicyRepoStub) Get(ctx context.Context, key string) (*Setting, error) {
	panic("unexpected Get call")
}

func (s *openAIFastPolicyRepoStub) GetValue(ctx context.Context, key string) (string, error) {
	if v, ok := s.values[key]; ok {
		return v, nil
	}
	return "", ErrSettingNotFound
}

func (s *openAIFastPolicyRepoStub) Set(ctx context.Context, key, value string) error {
	if s.values == nil {
		s.values = map[string]string{}
	}
	s.values[key] = value
	return nil
}

func (s *openAIFastPolicyRepoStub) GetMultiple(ctx context.Context, keys []string) (map[string]string, error) {
	result := make(map[string]string, len(keys))
	for _, key := range keys {
		if value, ok := s.values[key]; ok {
			result[key] = value
		}
	}
	return result, nil
}

func (s *openAIFastPolicyRepoStub) SetMultiple(ctx context.Context, settings map[string]string) error {
	panic("unexpected SetMultiple call")
}

func (s *openAIFastPolicyRepoStub) GetAll(ctx context.Context) (map[string]string, error) {
	panic("unexpected GetAll call")
}

func (s *openAIFastPolicyRepoStub) Delete(ctx context.Context, key string) error {
	panic("unexpected Delete call")
}

func newOpenAIGatewayServiceWithSettings(t *testing.T, settings *OpenAIFastPolicySettings) *OpenAIGatewayService {
	t.Helper()
	repo := &openAIFastPolicyRepoStub{values: map[string]string{}}
	if settings != nil {
		raw, err := json.Marshal(settings)
		require.NoError(t, err)
		repo.values[SettingKeyOpenAIFastPolicySettings] = string(raw)
	}
	return &OpenAIGatewayService{
		settingService: NewSettingService(repo, &config.Config{}),
	}
}

func openAIFastFilterPriorityPolicy() *OpenAIFastPolicySettings {
	return &OpenAIFastPolicySettings{
		Rules: []OpenAIFastPolicyRule{{
			ServiceTier:    OpenAIFastTierPriority,
			Action:         BetaPolicyActionFilter,
			Scope:          BetaPolicyScopeAll,
			ModelWhitelist: []string{},
			FallbackAction: BetaPolicyActionPass,
		}},
	}
}

func TestEvaluateOpenAIFastPolicy_DefaultPassesKnownTiers(t *testing.T) {
	require.Empty(t, DefaultOpenAIFastPolicySettings().Rules, "default policy must not rewrite service_tier unless admin configured rules")

	svc := newOpenAIGatewayServiceWithSettings(t, DefaultOpenAIFastPolicySettings())
	account := &Account{Platform: PlatformOpenAI, Type: AccountTypeAPIKey}

	action, _ := svc.evaluateOpenAIFastPolicy(context.Background(), account, "gpt-5.5", OpenAIFastTierPriority)
	require.Equal(t, BetaPolicyActionPass, action)

	action, _ = svc.evaluateOpenAIFastPolicy(context.Background(), account, "gpt-5.5-turbo", OpenAIFastTierPriority)
	require.Equal(t, BetaPolicyActionPass, action)

	action, _ = svc.evaluateOpenAIFastPolicy(context.Background(), account, "gpt-4", OpenAIFastTierPriority)
	require.Equal(t, BetaPolicyActionPass, action)

	action, _ = svc.evaluateOpenAIFastPolicy(context.Background(), account, "gpt-5.5", OpenAIFastTierFlex)
	require.Equal(t, BetaPolicyActionPass, action)

	action, _ = svc.evaluateOpenAIFastPolicy(context.Background(), account, "gpt-5.6-sol", OpenAIFastTierUltrafast)
	require.Equal(t, BetaPolicyActionPass, action)

	// empty tier → pass
	action, _ = svc.evaluateOpenAIFastPolicy(context.Background(), account, "gpt-5.5", "")
	require.Equal(t, BetaPolicyActionPass, action)
}

func TestEvaluateOpenAIFastPolicy_BlockRuleCarriesMessage(t *testing.T) {
	settings := &OpenAIFastPolicySettings{
		Rules: []OpenAIFastPolicyRule{{
			ServiceTier:    OpenAIFastTierPriority,
			Action:         BetaPolicyActionBlock,
			Scope:          BetaPolicyScopeAll,
			ErrorMessage:   "fast mode is not allowed",
			ModelWhitelist: []string{"gpt-5.5"},
			FallbackAction: BetaPolicyActionPass,
		}},
	}
	svc := newOpenAIGatewayServiceWithSettings(t, settings)
	account := &Account{Platform: PlatformOpenAI, Type: AccountTypeAPIKey}

	action, msg := svc.evaluateOpenAIFastPolicy(context.Background(), account, "gpt-5.5", OpenAIFastTierPriority)
	require.Equal(t, BetaPolicyActionBlock, action)
	require.Equal(t, "fast mode is not allowed", msg)
}

func TestEvaluateOpenAIFastPolicy_ScopeFiltersOAuth(t *testing.T) {
	settings := &OpenAIFastPolicySettings{
		Rules: []OpenAIFastPolicyRule{{
			ServiceTier: OpenAIFastTierAny,
			Action:      BetaPolicyActionFilter,
			Scope:       BetaPolicyScopeOAuth,
		}},
	}
	svc := newOpenAIGatewayServiceWithSettings(t, settings)

	// OAuth account → rule matches
	oauthAccount := &Account{Platform: PlatformOpenAI, Type: AccountTypeOAuth}
	action, _ := svc.evaluateOpenAIFastPolicy(context.Background(), oauthAccount, "gpt-4", OpenAIFastTierPriority)
	require.Equal(t, BetaPolicyActionFilter, action)

	// API Key account → rule skipped → pass
	apiKeyAccount := &Account{Platform: PlatformOpenAI, Type: AccountTypeAPIKey}
	action, _ = svc.evaluateOpenAIFastPolicy(context.Background(), apiKeyAccount, "gpt-4", OpenAIFastTierPriority)
	require.Equal(t, BetaPolicyActionPass, action)
}

func TestEvaluateOpenAIFastPolicy_UserScopedRuleOverridesGlobalRule(t *testing.T) {
	settings := &OpenAIFastPolicySettings{
		Rules: []OpenAIFastPolicyRule{
			{
				ServiceTier: OpenAIFastTierPriority,
				Action:      BetaPolicyActionFilter,
				Scope:       BetaPolicyScopeAll,
			},
			{
				ServiceTier: OpenAIFastTierPriority,
				Action:      BetaPolicyActionPass,
				Scope:       BetaPolicyScopeAll,
				UserIDs:     []int64{42},
			},
		},
	}
	svc := newOpenAIGatewayServiceWithSettings(t, settings)
	account := &Account{Platform: PlatformOpenAI, Type: AccountTypeAPIKey}

	allowedUserCtx := context.WithValue(context.Background(), ctxkey.UserID, int64(42))
	action, _ := svc.evaluateOpenAIFastPolicy(allowedUserCtx, account, "gpt-5.5", OpenAIFastTierPriority)
	require.Equal(t, BetaPolicyActionPass, action)

	otherUserCtx := context.WithValue(context.Background(), ctxkey.UserID, int64(43))
	action, _ = svc.evaluateOpenAIFastPolicy(otherUserCtx, account, "gpt-5.5", OpenAIFastTierPriority)
	require.Equal(t, BetaPolicyActionFilter, action)

	action, _ = svc.evaluateOpenAIFastPolicy(context.Background(), account, "gpt-5.5", OpenAIFastTierPriority)
	require.Equal(t, BetaPolicyActionFilter, action)
}

func TestApplyOpenAIFastPolicyToBody_DefaultPassesPriorityAndFast(t *testing.T) {
	svc := newOpenAIGatewayServiceWithSettings(t, DefaultOpenAIFastPolicySettings())
	account := &Account{Platform: PlatformOpenAI, Type: AccountTypeAPIKey}

	body := []byte(`{"model":"gpt-5.5","service_tier":"priority","messages":[]}`)
	updated, err := svc.applyOpenAIFastPolicyToBody(context.Background(), account, "gpt-5.5", body)
	require.NoError(t, err)
	require.Equal(t, string(body), string(updated))

	body = []byte(`{"model":"gpt-5.5","service_tier":"fast"}`)
	updated, err = svc.applyOpenAIFastPolicyToBody(context.Background(), account, "gpt-5.5", body)
	require.NoError(t, err)
	require.Equal(t, "priority", gjson.GetBytes(updated, "service_tier").String())

	body = []byte(`{"model":"gpt-4","service_tier":"priority"}`)
	updated, err = svc.applyOpenAIFastPolicyToBody(context.Background(), account, "gpt-4", body)
	require.NoError(t, err)
	require.Equal(t, string(body), string(updated))

	// No service_tier → no-op
	body = []byte(`{"model":"gpt-5.5"}`)
	updated, err = svc.applyOpenAIFastPolicyToBody(context.Background(), account, "gpt-5.5", body)
	require.NoError(t, err)
	require.Equal(t, string(body), string(updated))
}

func TestApplyOpenAIFastPolicyToBody_ForcePriorityInjectsMissingTier(t *testing.T) {
	settings := &OpenAIFastPolicySettings{
		Rules: []OpenAIFastPolicyRule{{
			ServiceTier: OpenAIFastTierMissing,
			Action:      OpenAIFastPolicyActionForcePriority,
			Scope:       BetaPolicyScopeAll,
			UserIDs:     []int64{42},
		}},
	}
	svc := newOpenAIGatewayServiceWithSettings(t, settings)
	account := &Account{Platform: PlatformOpenAI, Type: AccountTypeOAuth}
	body := []byte(`{"model":"gpt-5.5"}`)

	matchingCtx := context.WithValue(context.Background(), ctxkey.UserID, int64(42))
	updated, err := svc.applyOpenAIFastPolicyToBody(matchingCtx, account, "gpt-5.5", body)
	require.NoError(t, err)
	require.Equal(t, OpenAIFastTierPriority, gjson.GetBytes(updated, "service_tier").String())

	otherCtx := context.WithValue(context.Background(), ctxkey.UserID, int64(43))
	updated, err = svc.applyOpenAIFastPolicyToBody(otherCtx, account, "gpt-5.5", body)
	require.NoError(t, err)
	require.False(t, gjson.GetBytes(updated, "service_tier").Exists())
}

func TestApplyOpenAIFastPolicyToBody_LegacyAllRuleDoesNotInjectMissingTier(t *testing.T) {
	settings := &OpenAIFastPolicySettings{
		Rules: []OpenAIFastPolicyRule{{
			ServiceTier: OpenAIFastTierAny,
			Action:      OpenAIFastPolicyActionForcePriority,
			Scope:       BetaPolicyScopeAll,
		}},
	}
	svc := newOpenAIGatewayServiceWithSettings(t, settings)
	account := &Account{Platform: PlatformOpenAI, Type: AccountTypeAPIKey}
	body := []byte(`{"model":"gpt-5.5"}`)

	updated, err := svc.applyOpenAIFastPolicyToBody(context.Background(), account, "gpt-5.5", body)

	require.NoError(t, err)
	require.Equal(t, string(body), string(updated))

	// Explicitly selected tiers continue to be forced by the existing rule.
	body = []byte(`{"model":"gpt-5.5","service_tier":"flex"}`)
	updated, err = svc.applyOpenAIFastPolicyToBody(context.Background(), account, "gpt-5.5", body)
	require.NoError(t, err)
	require.Equal(t, OpenAIFastTierPriority, gjson.GetBytes(updated, "service_tier").String())
}

func TestApplyOpenAIFastPolicyToBody_MissingTierIgnoresNonForceRule(t *testing.T) {
	svc := newOpenAIGatewayServiceWithSettings(t, openAIFastFilterPriorityPolicy())
	account := &Account{Platform: PlatformOpenAI, Type: AccountTypeAPIKey}
	body := []byte(`{"model":"gpt-5.5"}`)

	updated, err := svc.applyOpenAIFastPolicyToBody(context.Background(), account, "gpt-5.5", body)
	require.NoError(t, err)
	require.Equal(t, string(body), string(updated))
}

func TestApplyOpenAIFastPolicyToBody_ExplicitFilterRemovesField(t *testing.T) {
	svc := newOpenAIGatewayServiceWithSettings(t, openAIFastFilterPriorityPolicy())
	account := &Account{Platform: PlatformOpenAI, Type: AccountTypeAPIKey}

	body := []byte(`{"model":"gpt-5.5","service_tier":"priority","messages":[]}`)
	updated, err := svc.applyOpenAIFastPolicyToBody(context.Background(), account, "gpt-5.5", body)
	require.NoError(t, err)
	require.NotContains(t, string(updated), `"service_tier"`)

	body = []byte(`{"model":"gpt-5.5","service_tier":"fast"}`)
	updated, err = svc.applyOpenAIFastPolicyToBody(context.Background(), account, "gpt-5.5", body)
	require.NoError(t, err)
	require.NotContains(t, string(updated), `"service_tier"`)
}

func TestApplyOpenAIFastPolicyToBody_UserScopedRuleOverridesGlobalRule(t *testing.T) {
	settings := &OpenAIFastPolicySettings{
		Rules: []OpenAIFastPolicyRule{
			{
				ServiceTier: OpenAIFastTierPriority,
				Action:      BetaPolicyActionFilter,
				Scope:       BetaPolicyScopeAll,
			},
			{
				ServiceTier: OpenAIFastTierPriority,
				Action:      BetaPolicyActionPass,
				Scope:       BetaPolicyScopeAll,
				UserIDs:     []int64{42},
			},
		},
	}
	svc := newOpenAIGatewayServiceWithSettings(t, settings)
	account := &Account{Platform: PlatformOpenAI, Type: AccountTypeAPIKey}
	body := []byte(`{"model":"gpt-5.5","service_tier":"priority"}`)

	allowedUserCtx := context.WithValue(context.Background(), ctxkey.UserID, int64(42))
	updated, err := svc.applyOpenAIFastPolicyToBody(allowedUserCtx, account, "gpt-5.5", body)
	require.NoError(t, err)
	require.Equal(t, "priority", gjson.GetBytes(updated, "service_tier").String())

	otherUserCtx := context.WithValue(context.Background(), ctxkey.UserID, int64(43))
	updated, err = svc.applyOpenAIFastPolicyToBody(otherUserCtx, account, "gpt-5.5", body)
	require.NoError(t, err)
	require.NotContains(t, string(updated), `"service_tier"`)
}

func TestApplyOpenAIFastPolicyToBody_PriorityFilterLeavesUltrafast(t *testing.T) {
	svc := newOpenAIGatewayServiceWithSettings(t, openAIFastFilterPriorityPolicy())
	account := &Account{Platform: PlatformOpenAI, Type: AccountTypeAPIKey}
	body := []byte(`{"model":"gpt-5.6-sol","service_tier":"ultrafast"}`)

	updated, err := svc.applyOpenAIFastPolicyToBody(context.Background(), account, "gpt-5.6-sol", body)
	require.NoError(t, err)
	require.Equal(t, OpenAIFastTierUltrafast, gjson.GetBytes(updated, "service_tier").String())
}

func TestApplyOpenAIFastPolicyToBody_ForcePriorityRewritesKnownTier(t *testing.T) {
	settings := &OpenAIFastPolicySettings{
		Rules: []OpenAIFastPolicyRule{{
			ServiceTier: OpenAIFastTierAny,
			Action:      OpenAIFastPolicyActionForcePriority,
			Scope:       BetaPolicyScopeAll,
		}},
	}
	svc := newOpenAIGatewayServiceWithSettings(t, settings)
	account := &Account{Platform: PlatformOpenAI, Type: AccountTypeAPIKey}

	for _, tier := range []string{"flex", "auto", "default", "scale", "fast", "priority", "ultrafast"} {
		body := []byte(`{"model":"gpt-5.5","service_tier":"` + tier + `"}`)
		updated, err := svc.applyOpenAIFastPolicyToBody(context.Background(), account, "gpt-5.5", body)
		require.NoError(t, err)
		require.Equal(t, OpenAIFastTierPriority, gjson.GetBytes(updated, "service_tier").String(),
			"tier %q should be forced to priority", tier)
	}
}

func TestApplyOpenAIFastPolicyToBody_GroupForceInjectsAndOverridesTier(t *testing.T) {
	svc := newOpenAIGatewayServiceWithSettings(t, DefaultOpenAIFastPolicySettings())
	account := &Account{Platform: PlatformOpenAI, Type: AccountTypeOAuth}
	ctx := context.WithValue(context.Background(), ctxkey.Group, &Group{
		ID: 7, Platform: PlatformOpenAI, Status: StatusActive, Hydrated: true, ForceOpenAIFast: true,
	})

	for _, body := range [][]byte{
		[]byte(`{"model":"gpt-5.6-sol","input":"hi"}`),
		[]byte(`{"model":"gpt-5.6-sol","service_tier":"default"}`),
		[]byte(`{"model":"gpt-5.6-sol","service_tier":"flex"}`),
		[]byte(`{"model":"gpt-5.6-sol","service_tier":"client-unknown"}`),
	} {
		updated, err := svc.applyOpenAIFastPolicyToBody(ctx, account, "gpt-5.6-sol", body)
		require.NoError(t, err)
		require.Equal(t, OpenAIFastTierPriority, gjson.GetBytes(updated, "service_tier").String())
	}
}

func TestApplyOpenAIFastPolicyToBody_GroupForceStillHonorsGlobalPolicy(t *testing.T) {
	svc := newOpenAIGatewayServiceWithSettings(t, openAIFastFilterPriorityPolicy())
	account := &Account{Platform: PlatformOpenAI, Type: AccountTypeOAuth}
	ctx := context.WithValue(context.Background(), ctxkey.Group, &Group{
		ID: 7, Platform: PlatformOpenAI, Status: StatusActive, Hydrated: true, ForceOpenAIFast: true,
	})

	updated, err := svc.applyOpenAIFastPolicyToBody(ctx, account, "gpt-5.6-sol", []byte(`{"model":"gpt-5.6-sol"}`))
	require.NoError(t, err)
	require.False(t, gjson.GetBytes(updated, "service_tier").Exists(),
		"the global filter remains authoritative after the group requests priority")
}

func TestApplyOpenAIFastPolicyToBody_GroupForceRequiresHydratedGroup(t *testing.T) {
	svc := newOpenAIGatewayServiceWithSettings(t, DefaultOpenAIFastPolicySettings())
	account := &Account{Platform: PlatformOpenAI, Type: AccountTypeOAuth}
	body := []byte(`{"model":"gpt-5.6-sol"}`)
	ctx := context.WithValue(context.Background(), ctxkey.Group, &Group{
		ID: 7, Platform: PlatformOpenAI, Status: StatusActive, ForceOpenAIFast: true,
	})

	updated, err := svc.applyOpenAIFastPolicyToBody(ctx, account, "gpt-5.6-sol", body)
	require.NoError(t, err)
	require.Equal(t, string(body), string(updated))
}

func TestApplyOpenAIFastPolicyToBody_GroupForceOnlyTargetsOpenAIAccounts(t *testing.T) {
	svc := newOpenAIGatewayServiceWithSettings(t, DefaultOpenAIFastPolicySettings())
	body := []byte(`{"model":"grok-4.1"}`)
	ctx := context.WithValue(context.Background(), ctxkey.Group, &Group{
		ID: 7, Platform: PlatformComposite, Status: StatusActive, Hydrated: true, ForceOpenAIFast: true,
	})

	updated, err := svc.applyOpenAIFastPolicyToBody(
		ctx,
		&Account{Platform: PlatformGrok, Type: AccountTypeOAuth},
		"grok-4.1",
		body,
	)
	require.NoError(t, err)
	require.Equal(t, string(body), string(updated))
}

func TestApplyOpenAIFastPolicyToBody_GroupForceRequiresSupportedGroupPlatform(t *testing.T) {
	svc := newOpenAIGatewayServiceWithSettings(t, DefaultOpenAIFastPolicySettings())
	body := []byte(`{"model":"gpt-5.6-sol"}`)
	ctx := context.WithValue(context.Background(), ctxkey.Group, &Group{
		ID: 7, Platform: PlatformAnthropic, Status: StatusActive, Hydrated: true, ForceOpenAIFast: true,
	})

	updated, err := svc.applyOpenAIFastPolicyToBody(
		ctx,
		&Account{Platform: PlatformOpenAI, Type: AccountTypeOAuth},
		"gpt-5.6-sol",
		body,
	)
	require.NoError(t, err)
	require.Equal(t, string(body), string(updated))
}

// TestApplyOpenAIFastPolicyToBody_OfficialTiersBypassDefaultRule 验证默认配置
// 下客户端显式发送的 OpenAI 官方合法 tier 能透传到上游而不被静默剥离。
func TestApplyOpenAIFastPolicyToBody_OfficialTiersBypassDefaultRule(t *testing.T) {
	svc := newOpenAIGatewayServiceWithSettings(t, DefaultOpenAIFastPolicySettings())
	account := &Account{Platform: PlatformOpenAI, Type: AccountTypeAPIKey}

	for _, tier := range []string{"auto", "default", "scale"} {
		body := []byte(`{"model":"gpt-5.5","service_tier":"` + tier + `"}`)
		updated, err := svc.applyOpenAIFastPolicyToBody(context.Background(), account, "gpt-5.5", body)
		require.NoError(t, err, "tier %q should pass without error", tier)
		require.Contains(t, string(updated), `"service_tier":"`+tier+`"`,
			"tier %q should be preserved in body under default policy", tier)
	}

	// evaluate 层也应判定为 pass（默认配置没有内置规则）。
	for _, tier := range []string{"auto", "default", "scale"} {
		action, _ := svc.evaluateOpenAIFastPolicy(context.Background(), account, "gpt-5.5", tier)
		require.Equal(t, BetaPolicyActionPass, action, "tier %q should evaluate to pass", tier)
	}
}

// TestApplyOpenAIFastPolicyToBody_AllRuleStripsOfficialTiers 验证管理员显式配置
// ServiceTier=all + Action=filter 规则后，auto/default/scale 等官方 tier 也会
// 被剥离。这是符合预期的——首条匹配 short-circuit，"all" 覆盖任意已识别 tier。
func TestApplyOpenAIFastPolicyToBody_AllRuleStripsOfficialTiers(t *testing.T) {
	settings := &OpenAIFastPolicySettings{
		Rules: []OpenAIFastPolicyRule{{
			ServiceTier: OpenAIFastTierAny,
			Action:      BetaPolicyActionFilter,
			Scope:       BetaPolicyScopeAll,
		}},
	}
	svc := newOpenAIGatewayServiceWithSettings(t, settings)
	account := &Account{Platform: PlatformOpenAI, Type: AccountTypeAPIKey}

	for _, tier := range []string{"auto", "default", "scale", "priority", "flex"} {
		body := []byte(`{"model":"gpt-5.5","service_tier":"` + tier + `"}`)
		updated, err := svc.applyOpenAIFastPolicyToBody(context.Background(), account, "gpt-5.5", body)
		require.NoError(t, err)
		require.NotContains(t, string(updated), `"service_tier"`,
			"tier %q should be stripped under ServiceTier=all + filter rule", tier)
	}
}

// TestApplyOpenAIFastPolicyToBody_UnknownTierStripped 验证真未知 tier 仍被剥离
// （normalize 返回 nil → normalizeResponsesBodyServiceTier 删除字段；
// applyOpenAIFastPolicyToBody 在 normTier 为空时直接 no-op，因为字段已不可能存在
// 于经过前置归一化的请求里。这里直接调 apply 验证它对未识别值不会异常）。
func TestApplyOpenAIFastPolicyToBody_UnknownTierStripped(t *testing.T) {
	svc := newOpenAIGatewayServiceWithSettings(t, DefaultOpenAIFastPolicySettings())
	account := &Account{Platform: PlatformOpenAI, Type: AccountTypeAPIKey}

	// normalize 阶段会将未知值剥离
	require.Nil(t, normalizeOpenAIServiceTier("xxx"))

	// applyOpenAIFastPolicyToBody 收到未识别 tier 时不报错，body 透传不变
	// （不属于本函数职责——上层 normalizeResponsesBodyServiceTier 已剥离）
	body := []byte(`{"model":"gpt-5.5","service_tier":"xxx"}`)
	updated, err := svc.applyOpenAIFastPolicyToBody(context.Background(), account, "gpt-5.5", body)
	require.NoError(t, err)
	require.Equal(t, string(body), string(updated))
}

func TestApplyOpenAIFastPolicyToBody_BlockReturnsTypedError(t *testing.T) {
	settings := &OpenAIFastPolicySettings{
		Rules: []OpenAIFastPolicyRule{{
			ServiceTier:    OpenAIFastTierPriority,
			Action:         BetaPolicyActionBlock,
			Scope:          BetaPolicyScopeAll,
			ErrorMessage:   "fast mode is blocked for gpt-5.5",
			ModelWhitelist: []string{"gpt-5.5"},
			FallbackAction: BetaPolicyActionPass,
		}},
	}
	svc := newOpenAIGatewayServiceWithSettings(t, settings)
	account := &Account{Platform: PlatformOpenAI, Type: AccountTypeAPIKey}

	body := []byte(`{"model":"gpt-5.5","service_tier":"priority"}`)
	updated, err := svc.applyOpenAIFastPolicyToBody(context.Background(), account, "gpt-5.5", body)
	require.Error(t, err)
	var blocked *OpenAIFastBlockedError
	require.True(t, errors.As(err, &blocked))
	require.Contains(t, blocked.Message, "fast mode is blocked")
	require.Equal(t, string(body), string(updated)) // body not mutated on block
}

func TestSetOpenAIFastPolicySettings_Validation(t *testing.T) {
	repo := &openAIFastPolicyRepoStub{values: map[string]string{}}
	svc := NewSettingService(repo, &config.Config{})

	// Invalid action rejected
	err := svc.SetOpenAIFastPolicySettings(context.Background(), &OpenAIFastPolicySettings{
		Rules: []OpenAIFastPolicyRule{{
			ServiceTier: OpenAIFastTierPriority,
			Action:      "bogus",
			Scope:       BetaPolicyScopeAll,
		}},
	})
	require.Error(t, err)

	// Invalid service_tier rejected
	err = svc.SetOpenAIFastPolicySettings(context.Background(), &OpenAIFastPolicySettings{
		Rules: []OpenAIFastPolicyRule{{
			ServiceTier: "turbo",
			Action:      BetaPolicyActionPass,
			Scope:       BetaPolicyScopeAll,
		}},
	})
	require.Error(t, err)

	// Non-positive and duplicate user IDs are rejected.
	err = svc.SetOpenAIFastPolicySettings(context.Background(), &OpenAIFastPolicySettings{
		Rules: []OpenAIFastPolicyRule{{
			ServiceTier: OpenAIFastTierPriority,
			Action:      BetaPolicyActionPass,
			Scope:       BetaPolicyScopeAll,
			UserIDs:     []int64{0},
		}},
	})
	require.Error(t, err)

	err = svc.SetOpenAIFastPolicySettings(context.Background(), &OpenAIFastPolicySettings{
		Rules: []OpenAIFastPolicyRule{{
			ServiceTier: OpenAIFastTierPriority,
			Action:      BetaPolicyActionPass,
			Scope:       BetaPolicyScopeAll,
			UserIDs:     []int64{42, 42},
		}},
	})
	require.Error(t, err)

	// Valid settings persisted
	err = svc.SetOpenAIFastPolicySettings(context.Background(), &OpenAIFastPolicySettings{
		Rules: []OpenAIFastPolicyRule{
			{
				ServiceTier: OpenAIFastTierPriority,
				Action:      OpenAIFastPolicyActionForcePriority,
				Scope:       BetaPolicyScopeAll,
				UserIDs:     []int64{42, 43},
			},
			{
				ServiceTier: OpenAIFastTierUltrafast,
				Action:      BetaPolicyActionPass,
				Scope:       BetaPolicyScopeAll,
			},
			{
				ServiceTier: OpenAIFastTierMissing,
				Action:      OpenAIFastPolicyActionForcePriority,
				Scope:       BetaPolicyScopeAll,
			},
		},
	})
	require.NoError(t, err)

	got, err := svc.GetOpenAIFastPolicySettings(context.Background())
	require.NoError(t, err)
	require.Len(t, got.Rules, 3)
	require.Equal(t, OpenAIFastTierPriority, got.Rules[0].ServiceTier)
	require.Equal(t, OpenAIFastTierUltrafast, got.Rules[1].ServiceTier)
	require.Equal(t, OpenAIFastTierMissing, got.Rules[2].ServiceTier)
	require.Equal(t, OpenAIFastPolicyActionForcePriority, got.Rules[0].Action)
	require.Equal(t, []int64{42, 43}, got.Rules[0].UserIDs)
	require.Equal(t, BetaPolicyActionPass, got.Rules[1].Action)
	require.Equal(t, OpenAIFastPolicyActionForcePriority, got.Rules[2].Action)
}

func openAIUltrafastGroupContext() context.Context {
	return context.WithValue(context.Background(), ctxkey.Group, &Group{
		ID: 7, Platform: PlatformOpenAI, Status: StatusActive, Hydrated: true, ForceOpenAIUltrafast: true,
	})
}

func TestApplyOpenAIFastPolicyToBody_GroupForceUltrafastInjectsAndOverridesTier(t *testing.T) {
	svc := newOpenAIGatewayServiceWithSettings(t, DefaultOpenAIFastPolicySettings())
	account := &Account{Platform: PlatformOpenAI, Type: AccountTypeOAuth}

	for _, body := range [][]byte{
		[]byte(`{"model":"gpt-6.1-sol","input":"hi"}`),
		[]byte(`{"model":"gpt-6.1-sol","service_tier":"default"}`),
		[]byte(`{"model":"gpt-6.1-sol","service_tier":"fast"}`),
		[]byte(`{"model":"gpt-6.1-sol","service_tier":"priority"}`),
		[]byte(`{"model":"gpt-6.1-sol","service_tier":"flex"}`),
		[]byte(`{"model":"gpt-6.1-sol","service_tier":"ultrafast"}`),
		[]byte(`{"model":"gpt-6.1-sol","service_tier":"client-unknown"}`),
	} {
		updated, err := svc.applyOpenAIFastPolicyToBody(openAIUltrafastGroupContext(), account, "gpt-6.1-sol", body)
		require.NoError(t, err)
		require.Equal(t, OpenAIFastTierUltrafast, gjson.GetBytes(updated, "service_tier").String(), "body %s", body)
	}
}

// 全局策略在分组强制之后裁决：针对 ultrafast 的 filter / block / force_priority
// 规则必须作用在分组写入的 ultrafast 上，而只针对 priority 的规则不应误伤。
func TestApplyOpenAIFastPolicyToBody_GroupForceUltrafastStillHonorsGlobalPolicy(t *testing.T) {
	account := &Account{Platform: PlatformOpenAI, Type: AccountTypeOAuth}
	body := []byte(`{"model":"gpt-6.1-sol"}`)
	rule := func(tier, action string) *OpenAIFastPolicySettings {
		return &OpenAIFastPolicySettings{Rules: []OpenAIFastPolicyRule{{
			ServiceTier: tier, Action: action, Scope: BetaPolicyScopeAll, ErrorMessage: "ultrafast disabled",
		}}}
	}

	t.Run("ultrafast filter strips the forced tier", func(t *testing.T) {
		svc := newOpenAIGatewayServiceWithSettings(t, rule(OpenAIFastTierUltrafast, BetaPolicyActionFilter))
		updated, err := svc.applyOpenAIFastPolicyToBody(openAIUltrafastGroupContext(), account, "gpt-6.1-sol", body)
		require.NoError(t, err)
		require.False(t, gjson.GetBytes(updated, "service_tier").Exists())
	})

	t.Run("ultrafast block rejects the request", func(t *testing.T) {
		svc := newOpenAIGatewayServiceWithSettings(t, rule(OpenAIFastTierUltrafast, BetaPolicyActionBlock))
		_, err := svc.applyOpenAIFastPolicyToBody(openAIUltrafastGroupContext(), account, "gpt-6.1-sol", body)
		var blocked *OpenAIFastBlockedError
		require.ErrorAs(t, err, &blocked)
		require.Equal(t, "ultrafast disabled", blocked.Message)
	})

	t.Run("ultrafast force_priority downgrades to priority", func(t *testing.T) {
		svc := newOpenAIGatewayServiceWithSettings(t, rule(OpenAIFastTierUltrafast, OpenAIFastPolicyActionForcePriority))
		updated, err := svc.applyOpenAIFastPolicyToBody(openAIUltrafastGroupContext(), account, "gpt-6.1-sol", body)
		require.NoError(t, err)
		require.Equal(t, OpenAIFastTierPriority, gjson.GetBytes(updated, "service_tier").String())
	})

	t.Run("priority-only filter leaves ultrafast alone", func(t *testing.T) {
		svc := newOpenAIGatewayServiceWithSettings(t, openAIFastFilterPriorityPolicy())
		updated, err := svc.applyOpenAIFastPolicyToBody(openAIUltrafastGroupContext(), account, "gpt-6.1-sol", body)
		require.NoError(t, err)
		require.Equal(t, OpenAIFastTierUltrafast, gjson.GetBytes(updated, "service_tier").String())
	})

	t.Run("model whitelist fallback filters unsupported models", func(t *testing.T) {
		svc := newOpenAIGatewayServiceWithSettings(t, &OpenAIFastPolicySettings{Rules: []OpenAIFastPolicyRule{{
			ServiceTier: OpenAIFastTierUltrafast, Action: BetaPolicyActionPass, Scope: BetaPolicyScopeAll,
			ModelWhitelist: []string{"gpt-6.1-sol"}, FallbackAction: BetaPolicyActionFilter,
		}}})
		supported, err := svc.applyOpenAIFastPolicyToBody(openAIUltrafastGroupContext(), account, "gpt-6.1-sol", body)
		require.NoError(t, err)
		require.Equal(t, OpenAIFastTierUltrafast, gjson.GetBytes(supported, "service_tier").String())

		// Astra 支持 Ultrafast，分组会写入；不在白名单内，由 fallback 过滤掉。
		notWhitelisted, err := svc.applyOpenAIFastPolicyToBody(
			openAIUltrafastGroupContext(), account, "gpt-6-astra", []byte(`{"model":"gpt-6-astra"}`))
		require.NoError(t, err)
		require.False(t, gjson.GetBytes(notWhitelisted, "service_tier").Exists())
	})
}

func TestOpenAIModelSupportsUltrafast(t *testing.T) {
	for _, model := range []string{
		"gpt-6-astra", "gpt-6", "GPT-6-Astra", "openai/gpt-6-astra",
		"gpt-6.1-sol", "gpt-6.1-sol-high", "openai/gpt-6.1-sol", "gpt-6.1-sol-openai-compact",
	} {
		require.True(t, openAIModelSupportsUltrafast(model), model)
	}
	for _, model := range []string{
		"", "gpt-6.1-terra", "gpt-6-sol", "gpt-6-luna", "gpt-5.6-sol", "gpt-5.6-terra", "gpt-5.5", "gpt-5.4-mini",
		"gpt-6.1-solar", "grok-4", "claude-opus-5-5",
	} {
		require.False(t, openAIModelSupportsUltrafast(model), model)
	}
}

// 门控与计费共用同一判定：凡是分组会强制写入 ultrafast 的模型，都必须按 6x 计费。
func TestOpenAIUltrafastGateMatchesBilling(t *testing.T) {
	svc := newTestBillingService()
	for _, tt := range []struct {
		model string
		want  float64
	}{
		{model: "gpt-6-astra", want: 6},
		{model: "gpt-6.1-sol", want: 6},
		{model: "gpt-6.1-sol-high", want: 6},
		{model: "gpt-6.1-terra", want: 2},
		{model: "gpt-6-sol", want: 2},
		{model: "gpt-5.6-sol", want: 2},
	} {
		pricing, err := svc.GetModelPricing(tt.model)
		require.NoError(t, err, tt.model)
		require.Equal(t, tt.want, configuredServiceTierMultiplier(OpenAIFastTierUltrafast, pricing), tt.model)
		require.Equal(t, tt.want == 6, openAIModelSupportsUltrafast(tt.model), tt.model)

		// 渠道自定义价格与显式 Fast 倍率都不改变 Ultrafast 倍率。
		custom, fast := 1e-6, 3.0
		channelPricing, err := svc.GetModelPricingWithChannel(tt.model, &ChannelModelPricing{
			InputPrice: &custom, OutputPrice: &custom, FastMultiplier: &fast,
		})
		require.NoError(t, err, tt.model)
		require.Equal(t, tt.want, configuredServiceTierMultiplier(OpenAIFastTierUltrafast, channelPricing), tt.model)
	}
}

func TestOpenAIGroupForcedServiceTier(t *testing.T) {
	openAIAccount := &Account{Platform: PlatformOpenAI, Type: AccountTypeOAuth}
	const sol, astra, unsupported = "gpt-6.1-sol", "gpt-6-astra", "gpt-5.6-sol"
	group := func(platform string, fast, ultrafast bool) context.Context {
		return context.WithValue(context.Background(), ctxkey.Group, &Group{
			ID: 7, Platform: platform, Status: StatusActive, Hydrated: true,
			ForceOpenAIFast: fast, ForceOpenAIUltrafast: ultrafast,
		})
	}
	for _, tt := range []struct {
		name    string
		ctx     context.Context
		account *Account
		model   string
		want    string
	}{
		{name: "no group", ctx: context.Background(), account: openAIAccount, model: sol, want: ""},
		{name: "nil account", ctx: group(PlatformOpenAI, true, true), account: nil, model: sol, want: ""},
		{name: "follow client", ctx: group(PlatformOpenAI, false, false), account: openAIAccount, model: sol, want: ""},

		{name: "fast only, ultrafast-capable model", ctx: group(PlatformOpenAI, true, false), account: openAIAccount, model: sol, want: OpenAIFastTierPriority},
		{name: "fast only, other model", ctx: group(PlatformOpenAI, true, false), account: openAIAccount, model: unsupported, want: OpenAIFastTierPriority},
		{name: "fast only, empty model", ctx: group(PlatformOpenAI, true, false), account: openAIAccount, model: "", want: OpenAIFastTierPriority},

		{name: "ultrafast only, sol", ctx: group(PlatformOpenAI, false, true), account: openAIAccount, model: sol, want: OpenAIFastTierUltrafast},
		{name: "ultrafast only, astra", ctx: group(PlatformOpenAI, false, true), account: openAIAccount, model: astra, want: OpenAIFastTierUltrafast},
		{name: "ultrafast only, sol effort alias", ctx: group(PlatformOpenAI, false, true), account: openAIAccount, model: "gpt-6.1-sol-high", want: OpenAIFastTierUltrafast},
		{name: "ultrafast only, unsupported model follows client", ctx: group(PlatformOpenAI, false, true), account: openAIAccount, model: unsupported, want: ""},
		{name: "ultrafast only, empty model follows client", ctx: group(PlatformOpenAI, false, true), account: openAIAccount, model: "", want: ""},

		{name: "both, supported model gets ultrafast", ctx: group(PlatformOpenAI, true, true), account: openAIAccount, model: sol, want: OpenAIFastTierUltrafast},
		{name: "both, unsupported model falls back to fast", ctx: group(PlatformOpenAI, true, true), account: openAIAccount, model: unsupported, want: OpenAIFastTierPriority},

		{name: "composite group with openai account", ctx: group(PlatformComposite, false, true), account: openAIAccount, model: sol, want: OpenAIFastTierUltrafast},
		{name: "composite group with grok account", ctx: group(PlatformComposite, true, true), account: &Account{Platform: PlatformGrok}, model: sol, want: ""},
		{name: "unsupported group platform", ctx: group(PlatformAnthropic, true, true), account: openAIAccount, model: sol, want: ""},
		{name: "unhydrated group", ctx: context.WithValue(context.Background(), ctxkey.Group, &Group{ID: 7, Platform: PlatformOpenAI, Status: StatusActive, ForceOpenAIFast: true, ForceOpenAIUltrafast: true}), account: openAIAccount, model: sol, want: ""},
	} {
		t.Run(tt.name, func(t *testing.T) {
			require.Equal(t, tt.want, openAIGroupForcedServiceTier(tt.ctx, tt.account, tt.model))
		})
	}
}

// 仅强制 Ultra Fast 的分组：不支持的模型保留客户端传入的档位（含省略）。
func TestApplyOpenAIFastPolicyToBody_GroupForceUltrafastSkipsUnsupportedModels(t *testing.T) {
	svc := newOpenAIGatewayServiceWithSettings(t, DefaultOpenAIFastPolicySettings())
	account := &Account{Platform: PlatformOpenAI, Type: AccountTypeOAuth}

	for _, tt := range []struct {
		body string
		want string
	}{
		{body: `{"model":"gpt-5.6-sol","input":"hi"}`, want: ""},
		{body: `{"model":"gpt-5.6-sol","service_tier":"fast"}`, want: OpenAIFastTierPriority},
		{body: `{"model":"gpt-5.6-sol","service_tier":"flex"}`, want: OpenAIFastTierFlex},
		{body: `{"model":"gpt-5.6-sol","service_tier":"ultrafast"}`, want: OpenAIFastTierUltrafast},
	} {
		updated, err := svc.applyOpenAIFastPolicyToBody(openAIUltrafastGroupContext(), account, "gpt-5.6-sol", []byte(tt.body))
		require.NoError(t, err)
		require.Equal(t, tt.want, gjson.GetBytes(updated, "service_tier").String(), "body %s", tt.body)
	}
}

// 两个开关同时开启：支持的模型写 ultrafast，其余模型写 priority。
func TestApplyOpenAIFastPolicyToBody_GroupForceFastAndUltrafastTogether(t *testing.T) {
	svc := newOpenAIGatewayServiceWithSettings(t, DefaultOpenAIFastPolicySettings())
	account := &Account{Platform: PlatformOpenAI, Type: AccountTypeOAuth}
	ctx := context.WithValue(context.Background(), ctxkey.Group, &Group{
		ID: 7, Platform: PlatformOpenAI, Status: StatusActive, Hydrated: true, ForceOpenAIFast: true, ForceOpenAIUltrafast: true,
	})

	for _, tt := range []struct {
		model string
		want  string
	}{
		{model: "gpt-6.1-sol", want: OpenAIFastTierUltrafast},
		{model: "gpt-6-astra", want: OpenAIFastTierUltrafast},
		{model: "gpt-5.6-sol", want: OpenAIFastTierPriority},
		{model: "gpt-6.1-terra", want: OpenAIFastTierPriority},
	} {
		for _, body := range []string{
			`{"model":"` + tt.model + `"}`,
			`{"model":"` + tt.model + `","service_tier":"flex"}`,
			`{"model":"` + tt.model + `","service_tier":"ultrafast"}`,
		} {
			updated, err := svc.applyOpenAIFastPolicyToBody(ctx, account, tt.model, []byte(body))
			require.NoError(t, err)
			require.Equal(t, tt.want, gjson.GetBytes(updated, "service_tier").String(), "body %s", body)
		}
	}
}
