package service

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/pkg/apicompat"
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
	panic("unexpected GetMultiple call")
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

func TestEvaluateOpenAIFastPolicy_DefaultFiltersAllModelsPriority(t *testing.T) {
	svc := newOpenAIGatewayServiceWithSettings(t, DefaultOpenAIFastPolicySettings())
	account := &Account{Platform: PlatformOpenAI, Type: AccountTypeAPIKey}

	// 默认策略对所有模型生效（whitelist 为空），因为 codex 的 service_tier=fast
	// 是用户级开关，与 model 正交。
	// gpt-5.5 + priority → filter
	action, _ := svc.evaluateOpenAIFastPolicy(context.Background(), account, "gpt-5.5", OpenAIFastTierPriority)
	require.Equal(t, BetaPolicyActionFilter, action)

	// gpt-5.5-turbo → filter
	action, _ = svc.evaluateOpenAIFastPolicy(context.Background(), account, "gpt-5.5-turbo", OpenAIFastTierPriority)
	require.Equal(t, BetaPolicyActionFilter, action)

	// gpt-4 + priority → filter（默认策略覆盖所有模型）
	action, _ = svc.evaluateOpenAIFastPolicy(context.Background(), account, "gpt-4", OpenAIFastTierPriority)
	require.Equal(t, BetaPolicyActionFilter, action)

	// gpt-5.5 + flex → pass (tier doesn't match)
	action, _ = svc.evaluateOpenAIFastPolicy(context.Background(), account, "gpt-5.5", OpenAIFastTierFlex)
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

func TestApplyOpenAIFastPolicyToBody_FilterRemovesField(t *testing.T) {
	svc := newOpenAIGatewayServiceWithSettings(t, DefaultOpenAIFastPolicySettings())
	account := &Account{Platform: PlatformOpenAI, Type: AccountTypeAPIKey}

	// gpt-5.5 fast → service_tier stripped
	body := []byte(`{"model":"gpt-5.5","service_tier":"priority","messages":[]}`)
	updated, err := svc.applyOpenAIFastPolicyToBody(context.Background(), account, "gpt-5.5", body)
	require.NoError(t, err)
	require.NotContains(t, string(updated), `"service_tier"`)

	// Client sending "fast" (alias for priority) also filtered
	body = []byte(`{"model":"gpt-5.5","service_tier":"fast"}`)
	updated, err = svc.applyOpenAIFastPolicyToBody(context.Background(), account, "gpt-5.5", body)
	require.NoError(t, err)
	require.NotContains(t, string(updated), `"service_tier"`)

	// gpt-4 priority → 默认策略对所有模型 filter，service_tier 被移除
	body = []byte(`{"model":"gpt-4","service_tier":"priority"}`)
	updated, err = svc.applyOpenAIFastPolicyToBody(context.Background(), account, "gpt-4", body)
	require.NoError(t, err)
	require.NotContains(t, string(updated), `"service_tier"`)

	// No service_tier → no-op
	body = []byte(`{"model":"gpt-5.5"}`)
	updated, err = svc.applyOpenAIFastPolicyToBody(context.Background(), account, "gpt-5.5", body)
	require.NoError(t, err)
	require.Equal(t, string(body), string(updated))
}

// TestApplyOpenAIFastPolicyToBody_OfficialTiersBypassDefaultRule 验证扩展白名单后
// 客户端显式发送的 OpenAI 官方合法 tier（auto/default/scale）能透传到上游而不被
// 静默剥离。默认策略只针对 priority，所以这些 tier 落在 fall-through pass 分支。
func TestApplyOpenAIFastPolicyToBody_OfficialTiersBypassDefaultRule(t *testing.T) {
	svc := newOpenAIGatewayServiceWithSettings(t, DefaultOpenAIFastPolicySettings())
	account := &Account{Platform: PlatformOpenAI, Type: AccountTypeAPIKey}

	for _, tier := range []string{"auto", "default", "scale"} {
		body := []byte(`{"model":"gpt-5.5","service_tier":"` + tier + `"}`)
		updated, err := svc.applyOpenAIFastPolicyToBody(context.Background(), account, "gpt-5.5", body)
		require.NoError(t, err, "tier %q should pass without error", tier)
		require.Contains(t, string(updated), `"service_tier":"`+tier+`"`,
			"tier %q should be preserved in body under default rule", tier)
	}

	// evaluate 层也应判定为 pass（默认规则 ServiceTier=priority 与 auto/default/scale 不匹配）
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

func TestApplyOpenAIFastPolicyToBody_ForcePriorityInjectsResponsesTierAndReasoning(t *testing.T) {
	settings := &OpenAIFastPolicySettings{
		Rules: []OpenAIFastPolicyRule{{
			ServiceTier:     OpenAIFastTierAny,
			Action:          OpenAIFastPolicyActionForcePriority,
			Scope:           BetaPolicyScopeAll,
			ModelWhitelist:  []string{"gpt-5.*"},
			Endpoints:       []string{OpenAIFastPolicyEndpointResponses},
			ReasoningEffort: "x-high",
			FallbackAction:  BetaPolicyActionPass,
		}},
	}
	svc := newOpenAIGatewayServiceWithSettings(t, settings)
	account := &Account{Platform: PlatformOpenAI, Type: AccountTypeAPIKey}

	body := []byte(`{"model":"gpt-5.5","input":[]}`)
	updated, err := svc.applyOpenAIFastPolicyToBodyForEndpoint(
		context.Background(),
		nil,
		account,
		"gpt-5.5",
		OpenAIFastPolicyEndpointResponses,
		openAIFastPolicyBodyShapeResponses,
		body,
	)
	require.NoError(t, err)
	require.Equal(t, "priority", gjson.GetBytes(updated, "service_tier").String())
	require.Equal(t, "xhigh", gjson.GetBytes(updated, "reasoning.effort").String())
}

func TestApplyOpenAIFastPolicyToBody_ForceActionIgnoresConfiguredServiceTier(t *testing.T) {
	settings := &OpenAIFastPolicySettings{
		Rules: []OpenAIFastPolicyRule{{
			ServiceTier:    OpenAIFastTierPriority,
			Action:         OpenAIFastPolicyActionForcePriority,
			Scope:          BetaPolicyScopeAll,
			ModelWhitelist: []string{"gpt-5.5-fast"},
			FallbackAction: BetaPolicyActionPass,
		}},
	}
	svc := newOpenAIGatewayServiceWithSettings(t, settings)
	account := &Account{Platform: PlatformOpenAI, Type: AccountTypeAPIKey}

	body := []byte(`{"model":"gpt-5.5","input":[]}`)
	updated, err := svc.applyOpenAIFastPolicyToBodyForEndpoint(
		context.Background(),
		nil,
		account,
		"gpt-5.5",
		OpenAIFastPolicyEndpointResponses,
		openAIFastPolicyBodyShapeResponses,
		body,
		"gpt-5.5-fast",
	)
	require.NoError(t, err)
	require.Equal(t, "priority", gjson.GetBytes(updated, "service_tier").String())
}

func TestEvaluateOpenAIFastPolicy_ModelWhitelistMatchesAliases(t *testing.T) {
	settings := &OpenAIFastPolicySettings{
		Rules: []OpenAIFastPolicyRule{{
			ServiceTier:    OpenAIFastTierAny,
			Action:         OpenAIFastPolicyActionForcePriority,
			Scope:          BetaPolicyScopeAll,
			ModelWhitelist: []string{"gpt-5.5-fast"},
			FallbackAction: BetaPolicyActionPass,
		}},
	}
	account := &Account{Platform: PlatformOpenAI, Type: AccountTypeAPIKey}

	decision := evaluateOpenAIFastPolicyWithSettings(settings, openAIFastPolicyRequest{
		Account:      account,
		Model:        "gpt-5.5",
		ModelAliases: []string{"gpt-5.5-fast"},
	})
	require.Equal(t, OpenAIFastPolicyActionForcePriority, decision.Action)
	require.Equal(t, OpenAIFastTierPriority, decision.TargetServiceTier)
}

func TestApplyOpenAIFastPolicyToBody_NonWhitelistedModelFallbackFiltersFast(t *testing.T) {
	settings := &OpenAIFastPolicySettings{
		Rules: []OpenAIFastPolicyRule{{
			ServiceTier:    OpenAIFastTierAny,
			Action:         OpenAIFastPolicyActionForcePriority,
			Scope:          BetaPolicyScopeAll,
			ModelWhitelist: []string{"gpt-5.*"},
			FallbackAction: BetaPolicyActionFilter,
		}},
	}
	svc := newOpenAIGatewayServiceWithSettings(t, settings)
	account := &Account{Platform: PlatformOpenAI, Type: AccountTypeAPIKey}

	for _, tier := range []string{"priority", "fast"} {
		body := []byte(`{"model":"gpt-4.1","service_tier":"` + tier + `","input":[]}`)
		updated, err := svc.applyOpenAIFastPolicyToBodyForEndpoint(
			context.Background(),
			nil,
			account,
			"gpt-4.1",
			OpenAIFastPolicyEndpointResponses,
			openAIFastPolicyBodyShapeResponses,
			body,
		)
		require.NoError(t, err)
		require.False(t, gjson.GetBytes(updated, "service_tier").Exists(), "tier %q should be filtered", tier)
	}

	body := []byte(`{"model":"gpt-4.1","input":[]}`)
	updated, err := svc.applyOpenAIFastPolicyToBodyForEndpoint(
		context.Background(),
		nil,
		account,
		"gpt-4.1",
		OpenAIFastPolicyEndpointResponses,
		openAIFastPolicyBodyShapeResponses,
		body,
	)
	require.NoError(t, err)
	require.Equal(t, string(body), string(updated))
}

func TestApplyOpenAIFastPolicyToBody_ForcePriorityInjectsRawChatTierAndReasoningEffort(t *testing.T) {
	settings := &OpenAIFastPolicySettings{
		Rules: []OpenAIFastPolicyRule{{
			ServiceTier:     OpenAIFastTierAny,
			Action:          OpenAIFastPolicyActionForcePriority,
			Scope:           BetaPolicyScopeAll,
			Endpoints:       []string{OpenAIFastPolicyEndpointChatCompletions},
			ReasoningEffort: "xhigh",
		}},
	}
	svc := newOpenAIGatewayServiceWithSettings(t, settings)
	account := &Account{Platform: PlatformOpenAI, Type: AccountTypeAPIKey}

	body := []byte(`{"model":"gpt-5.5","messages":[]}`)
	updated, err := svc.applyOpenAIFastPolicyToBodyForEndpoint(
		context.Background(),
		nil,
		account,
		"gpt-5.5",
		OpenAIFastPolicyEndpointChatCompletions,
		openAIFastPolicyBodyShapeChatCompletions,
		body,
	)
	require.NoError(t, err)
	require.Equal(t, "priority", gjson.GetBytes(updated, "service_tier").String())
	require.Equal(t, "xhigh", gjson.GetBytes(updated, "reasoning_effort").String())
	require.False(t, gjson.GetBytes(updated, "reasoning.effort").Exists())
}

func TestApplyOpenAIFastPolicyToBody_MessagesToResponsesForcePriorityAfterConversion(t *testing.T) {
	settings := &OpenAIFastPolicySettings{
		Rules: []OpenAIFastPolicyRule{{
			ServiceTier:     OpenAIFastTierAny,
			Action:          OpenAIFastPolicyActionForcePriority,
			Scope:           BetaPolicyScopeAll,
			Endpoints:       []string{OpenAIFastPolicyEndpointMessagesToResponses},
			ModelWhitelist:  []string{"gpt-5.*"},
			ReasoningEffort: "xhigh",
			FallbackAction:  BetaPolicyActionPass,
		}},
	}
	svc := newOpenAIGatewayServiceWithSettings(t, settings)
	account := &Account{Platform: PlatformOpenAI, Type: AccountTypeAPIKey}
	anthropicReq := &apicompat.AnthropicRequest{
		Model:     "gpt-5.5",
		MaxTokens: 128,
		Messages: []apicompat.AnthropicMessage{{
			Role:    "user",
			Content: json.RawMessage(`[{"type":"text","text":"hi"}]`),
		}},
	}
	responsesReq, err := apicompat.AnthropicToResponses(anthropicReq)
	require.NoError(t, err)
	responsesReq.Model = "gpt-5.5"
	body, err := json.Marshal(responsesReq)
	require.NoError(t, err)

	updated, err := svc.applyOpenAIFastPolicyToBodyForEndpoint(
		context.Background(),
		nil,
		account,
		"gpt-5.5",
		OpenAIFastPolicyEndpointMessagesToResponses,
		openAIFastPolicyBodyShapeResponses,
		body,
	)
	require.NoError(t, err)
	require.Equal(t, "priority", gjson.GetBytes(updated, "service_tier").String())
	require.Equal(t, "xhigh", gjson.GetBytes(updated, "reasoning.effort").String())
}

func TestApplyOpenAIFastPolicyToBody_ForceDefaultOverridesPriorityAndDropsFastBypassFields(t *testing.T) {
	groupID := int64(10)
	settings := &OpenAIFastPolicySettings{
		Rules: []OpenAIFastPolicyRule{{
			ServiceTier: OpenAIFastTierAny,
			Action:      OpenAIFastPolicyActionForceDefault,
			Scope:       BetaPolicyScopeAll,
			GroupIDs:    []int64{groupID},
		}},
	}
	svc := newOpenAIGatewayServiceWithSettings(t, settings)
	account := &Account{Platform: PlatformOpenAI, Type: AccountTypeAPIKey}
	ctx := withOpenAIFastPolicyAPIKeyContext(context.Background(), &APIKey{
		ID:      1,
		GroupID: &groupID,
		Group:   &Group{ID: groupID},
	})

	body := []byte(`{"model":"gpt-5.5","service_tier":"priority","speed":"fast","input":[]}`)
	updated, err := svc.applyOpenAIFastPolicyToBodyForEndpoint(
		ctx,
		nil,
		account,
		"gpt-5.5",
		OpenAIFastPolicyEndpointResponses,
		openAIFastPolicyBodyShapeResponses,
		body,
	)
	require.NoError(t, err)
	require.Equal(t, "default", gjson.GetBytes(updated, "service_tier").String())
	require.False(t, gjson.GetBytes(updated, "speed").Exists())
}

func TestEvaluateOpenAIFastPolicy_MoreSpecificGroupRuleWinsOverGlobalRule(t *testing.T) {
	groupID := int64(20)
	settings := &OpenAIFastPolicySettings{
		Rules: []OpenAIFastPolicyRule{
			{
				ServiceTier: OpenAIFastTierAny,
				Action:      OpenAIFastPolicyActionForceDefault,
				Scope:       BetaPolicyScopeAll,
			},
			{
				ServiceTier:    OpenAIFastTierAny,
				Action:         OpenAIFastPolicyActionForcePriority,
				Scope:          BetaPolicyScopeAll,
				GroupIDs:       []int64{groupID},
				ModelWhitelist: []string{"gpt-5.*"},
			},
		},
	}
	account := &Account{Platform: PlatformOpenAI, Type: AccountTypeAPIKey}
	apiKey := &APIKey{ID: 1, GroupID: &groupID, Group: &Group{ID: groupID}}

	decision := evaluateOpenAIFastPolicyWithSettings(settings, openAIFastPolicyRequest{
		Account: account,
		APIKey:  apiKey,
		Model:   "gpt-5.5",
	})
	require.Equal(t, OpenAIFastPolicyActionForcePriority, decision.Action)
}

func TestApplyOpenAIFastPolicyToBody_EndpointMismatchKeepsOriginalBody(t *testing.T) {
	settings := &OpenAIFastPolicySettings{
		Rules: []OpenAIFastPolicyRule{{
			ServiceTier: OpenAIFastTierAny,
			Action:      OpenAIFastPolicyActionForcePriority,
			Scope:       BetaPolicyScopeAll,
			Endpoints:   []string{OpenAIFastPolicyEndpointResponses},
		}},
	}
	svc := newOpenAIGatewayServiceWithSettings(t, settings)
	account := &Account{Platform: PlatformOpenAI, Type: AccountTypeAPIKey}

	body := []byte(`{"model":"gpt-5.5","messages":[]}`)
	updated, err := svc.applyOpenAIFastPolicyToBodyForEndpoint(
		context.Background(),
		nil,
		account,
		"gpt-5.5",
		OpenAIFastPolicyEndpointChatCompletions,
		openAIFastPolicyBodyShapeChatCompletions,
		body,
	)
	require.NoError(t, err)
	require.Equal(t, string(body), string(updated))
}

func TestApplyOpenAIFastPolicyToBody_NonOpenAIAccountUnaffected(t *testing.T) {
	settings := &OpenAIFastPolicySettings{
		Rules: []OpenAIFastPolicyRule{{
			ServiceTier: OpenAIFastTierAny,
			Action:      OpenAIFastPolicyActionForcePriority,
			Scope:       BetaPolicyScopeAll,
		}},
	}
	svc := newOpenAIGatewayServiceWithSettings(t, settings)
	account := &Account{Platform: PlatformAnthropic, Type: AccountTypeAPIKey}

	body := []byte(`{"model":"claude-sonnet-4","input":[]}`)
	updated, err := svc.applyOpenAIFastPolicyToBodyForEndpoint(
		context.Background(),
		nil,
		account,
		"claude-sonnet-4",
		OpenAIFastPolicyEndpointResponses,
		openAIFastPolicyBodyShapeResponses,
		body,
	)
	require.NoError(t, err)
	require.Equal(t, string(body), string(updated))
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

	// Invalid endpoint rejected
	err = svc.SetOpenAIFastPolicySettings(context.Background(), &OpenAIFastPolicySettings{
		Rules: []OpenAIFastPolicyRule{{
			ServiceTier: OpenAIFastTierAny,
			Action:      OpenAIFastPolicyActionForcePriority,
			Scope:       BetaPolicyScopeAll,
			Endpoints:   []string{"images"},
		}},
	})
	require.Error(t, err)

	// Invalid reasoning effort rejected
	err = svc.SetOpenAIFastPolicySettings(context.Background(), &OpenAIFastPolicySettings{
		Rules: []OpenAIFastPolicyRule{{
			ServiceTier:     OpenAIFastTierAny,
			Action:          OpenAIFastPolicyActionForcePriority,
			Scope:           BetaPolicyScopeAll,
			ReasoningEffort: "turbo",
		}},
	})
	require.Error(t, err)

	// Valid settings persisted
	err = svc.SetOpenAIFastPolicySettings(context.Background(), &OpenAIFastPolicySettings{
		Rules: []OpenAIFastPolicyRule{{
			ServiceTier:     OpenAIFastTierAny,
			Action:          OpenAIFastPolicyActionForcePriority,
			Scope:           BetaPolicyScopeAll,
			Endpoints:       []string{OpenAIFastPolicyEndpointResponses},
			ReasoningEffort: "x-high",
		}},
	})
	require.NoError(t, err)

	got, err := svc.GetOpenAIFastPolicySettings(context.Background())
	require.NoError(t, err)
	require.Len(t, got.Rules, 1)
	require.Equal(t, OpenAIFastTierAny, got.Rules[0].ServiceTier)
	require.Equal(t, OpenAIFastPolicyActionForcePriority, got.Rules[0].Action)
	require.Equal(t, "xhigh", got.Rules[0].ReasoningEffort)
}
