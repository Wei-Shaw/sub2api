package service

import (
	"context"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/pkg/ctxkey"
	"github.com/stretchr/testify/require"
)

type compositeOwnershipAccountRepo struct {
	AccountRepository
	accounts []Account
}

func (r *compositeOwnershipAccountRepo) ListModelAvailabilityCandidates(_ context.Context, groupID *int64, platforms []string, _ bool) ([]Account, error) {
	if groupID == nil {
		return nil, nil
	}
	if len(platforms) == 0 {
		return nil, nil
	}
	return r.accounts, nil
}

// Scenario: 唯一平台的精确别名可路由
func TestResolveCompositeModelOwnershipKeepsProviderAccountsIsolated(t *testing.T) {
	groupID := int64(7)
	repo := &compositeOwnershipAccountRepo{
		accounts: []Account{
			{
				ID:       1,
				Platform: PlatformOpenAI,
				Credentials: map[string]any{
					"model_mapping": map[string]any{"gpt-public": "gpt-5"},
				},
			},
			{
				ID:       2,
				Platform: PlatformDeepseek,
				Credentials: map[string]any{
					"model_mapping": map[string]any{"reasoning-alias": "deepseek-v4-pro"},
				},
			},
		},
	}
	svc := &GatewayService{accountRepo: repo}

	deepSeekOwnership, err := svc.resolveCompositeModelOwnership(context.Background(), groupID, "reasoning-alias")
	require.NoError(t, err)
	require.Equal(t, CompositeModelOwnership{TargetPlatform: PlatformDeepseek, Matched: true}, deepSeekOwnership)

	openAIOwnership, err := svc.resolveCompositeModelOwnership(context.Background(), groupID, "gpt-public")
	require.NoError(t, err)
	require.Equal(t, CompositeModelOwnership{TargetPlatform: PlatformOpenAI, Matched: true}, openAIOwnership)
}

// Scenario: 通配映射按既有通配语义声明；映射目标为空的别名不构成声明；
// 精确声明平台不被其他平台的通配 catch-all 冒领（声明强度分层）。
func TestResolveCompositeModelOwnershipHonorsWildcardMappings(t *testing.T) {
	groupID := int64(7)
	openAIAccount := Account{
		ID:       1,
		Platform: PlatformOpenAI,
		Credentials: map[string]any{
			"model_mapping": map[string]any{"*": "gpt-5", "gpt-*": "gpt-5", "empty-alias": ""},
		},
	}
	grokAccount := Account{
		ID:       2,
		Platform: PlatformGrok,
		Credentials: map[string]any{
			"model_mapping": map[string]any{"grok-public": "grok-4"},
		},
	}
	repo := &compositeOwnershipAccountRepo{accounts: []Account{openAIAccount, grokAccount}}
	svc := &GatewayService{accountRepo: repo}

	// 完全无强声明时才回退通配命中平台：gpt-5 / unknown-alias 仅 openai 通配命中。
	for _, model := range []string{"gpt-5", "unknown-alias"} {
		ownership, err := svc.resolveCompositeModelOwnership(context.Background(), groupID, model)
		require.NoError(t, err)
		require.Equal(t, CompositeModelOwnership{TargetPlatform: PlatformOpenAI, Matched: true}, ownership, "model=%s", model)
	}

	// 映射目标为空的条目不构成声明，也无其他平台可声明。
	ownership, err := svc.resolveCompositeModelOwnership(context.Background(), groupID, "empty-alias")
	require.NoError(t, err)
	require.Equal(t, CompositeModelOwnership{}, ownership)

	// grok 精确声明为强声明：openai 的 "*" 通配（弱声明）不与之混池，
	// 否则 grok 请求会被改写成 gpt-5（回归）。
	ownership, err = svc.resolveCompositeModelOwnership(context.Background(), groupID, "grok-public")
	require.NoError(t, err)
	require.Equal(t, CompositeModelOwnership{TargetPlatform: PlatformGrok, Matched: true}, ownership)
}

func TestResolveCompositeModelOwnershipAllowsSamePlatformAndRejectsCrossPlatformAliases(t *testing.T) {
	groupID := int64(7)
	repo := &compositeOwnershipAccountRepo{
		accounts: []Account{
			{ID: 1, Platform: PlatformOpenAI, Credentials: map[string]any{"model_mapping": map[string]any{"shared-openai": "gpt-5", "ambiguous": "gpt-5"}}},
			{ID: 2, Platform: PlatformOpenAI, Credentials: map[string]any{"model_mapping": map[string]any{"shared-openai": "gpt-5.1"}}},
			{ID: 3, Platform: PlatformDeepseek, Credentials: map[string]any{"model_mapping": map[string]any{"ambiguous": "deepseek-v4-pro"}}},
		},
	}
	svc := &GatewayService{accountRepo: repo}

	samePlatform, err := svc.resolveCompositeModelOwnership(context.Background(), groupID, "shared-openai")
	require.NoError(t, err)
	require.Equal(t, CompositeModelOwnership{TargetPlatform: PlatformOpenAI, Matched: true}, samePlatform)

	// 跨平台同名别名：返回稳定候选池（去重升序），不再是裸 Ambiguous。
	ambiguous, err := svc.resolveCompositeModelOwnership(context.Background(), groupID, "ambiguous")
	require.NoError(t, err)
	require.True(t, ambiguous.Matched)
	require.False(t, ambiguous.Ambiguous)
	require.Empty(t, ambiguous.TargetPlatform)
	require.Equal(t, []string{PlatformDeepseek, PlatformOpenAI}, ambiguous.CandidatePlatforms)
}

func TestNewGatewayServiceWiresCompositeModelOwnershipResolver(t *testing.T) {
	groupID := int64(7)
	repo := &compositeOwnershipAccountRepo{
		accounts: []Account{{
			ID:          1,
			Platform:    PlatformDeepseek,
			Credentials: map[string]any{"model_mapping": map[string]any{"reasoning-alias": "deepseek-v4-pro"}},
		}},
	}
	resolver := NewCompositeRouteResolver(nil)
	svc := NewGatewayService(
		repo,
		nil,
		nil,
		nil,
		nil,
		nil,
		nil,
		nil,
		nil,
		nil,
		nil,
		nil,
		nil,
		nil,
		nil,
		nil,
		nil,
		nil,
		nil,
		nil,
		nil,
		nil,
		nil,
		nil,
		nil,
		resolver,
		nil,
		nil,
	)
	require.Same(t, resolver, svc.compositeResolver)

	decision, err := resolver.Resolve(context.Background(), groupID, "reasoning-alias", CompositeRouteEndpointResponses)
	require.NoError(t, err)
	require.True(t, decision.Matched)
	require.Equal(t, CompositeRouteSourceAccount, decision.Source)
	require.Equal(t, PlatformDeepseek, decision.TargetPlatform)
}

func TestDetectModelPlatform(t *testing.T) {
	tests := []struct {
		name     string
		model    string
		platform string
		ok       bool
	}{
		{name: "claude", model: "claude-sonnet-4-5", platform: PlatformAnthropic, ok: true},
		{name: "anthropic prefix", model: "anthropic/claude-opus-4-5", platform: PlatformAnthropic, ok: true},
		{name: "gpt", model: "gpt-5.1", platform: PlatformOpenAI, ok: true},
		{name: "o series", model: "o3-mini", platform: PlatformOpenAI, ok: true},
		{name: "embedding", model: "text-embedding-3-large", platform: PlatformOpenAI, ok: true},
		{name: "gemini", model: "gemini-3-pro", platform: PlatformGemini, ok: true},
		{name: "gemini models prefix", model: "models/gemini-2.5-flash", platform: PlatformGemini, ok: true},
		{name: "learnlm", model: "learnlm-2.0-flash-experimental", platform: PlatformGemini, ok: true},
		{name: "grok", model: "grok-4", platform: PlatformGrok, ok: true},
		{name: "xai prefix", model: "xai/grok-4", platform: PlatformGrok, ok: true},
		{name: "kimi", model: "kimi-k2-thinking", platform: PlatformKimi, ok: true},
		{name: "kimi code bare k3", model: "K3", platform: PlatformKimi, ok: true},
		{name: "kimi code bare k3 256k", model: "k3-256k", platform: PlatformKimi, ok: true},
		{name: "kimi code provider prefix", model: "kimi-code/k3", platform: PlatformKimi, ok: true},
		{name: "moonshot prefix", model: "moonshot/moonshot-v1-32k", platform: PlatformKimi, ok: true},
		{name: "zhipu", model: "glm-5.2", platform: PlatformZhipu, ok: true},
		{name: "deepseek", model: "deepseek-v4-pro", platform: PlatformDeepseek, ok: true},
		{name: "minimax", model: "MiniMax-M3", platform: PlatformMiniMax, ok: true},
		{name: "minimax prefix", model: "minimax/MiniMax-M2.5", platform: PlatformMiniMax, ok: true},
		{name: "abab legacy", model: "abab6.5-chat", platform: PlatformMiniMax, ok: true},
		{name: "abab7 legacy", model: "abab7-chat-preview", platform: PlatformMiniMax, ok: true},
		{name: "abab unrelated namespace", model: "abab-other", ok: false},
		{name: "unknown k3 alias", model: "k3-preview", ok: false},
		{name: "unknown", model: "llama-4-maverick", ok: false},
		// --- 行为钉（设计决策，不是漏配）---
		// DetectModelPlatform 刻意不加 ollama 分支（无 host 入参，ollama 转售
		// gpt-oss/kimi-k2/glm/deepseek 家族与上方前缀规则直接冲突，按字面加
		// 分支 = 静默错投）。以下用例钉住 ollama_cloud 默认目录
		// （DefaultOllamaCloudModelIDs）中的典型模型名的当前返回：将来有人
		// 「顺手加 ollama 分支」时这些用例会红灯，提示先回到该决策。
		{
			name:     "ollama-cloud resale gpt-oss pins to openai (intentional: no ollama branch)",
			model:    "gpt-oss:120b",
			platform: PlatformOpenAI,
			ok:       true,
		},
		{
			name:     "ollama-cloud resale glm pins to zhipu (intentional: no ollama branch)",
			model:    "glm-5.3",
			platform: PlatformZhipu,
			ok:       true,
		},
		{
			name:     "ollama-cloud resale kimi pins to kimi (intentional: no ollama branch)",
			model:    "kimi-k2.6",
			platform: PlatformKimi,
			ok:       true,
		},
		{
			name:     "ollama-cloud resale deepseek pins to deepseek (intentional: no ollama branch)",
			model:    "deepseek-v4.1-flash",
			platform: PlatformDeepseek,
			ok:       true,
		},
		{
			name:  "ollama-cloud qwen stays unmatched (intentional: fail closed)",
			model: "qwen3.5:397b",
			ok:    false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			platform, ok := DetectModelPlatform(tt.model)
			require.Equal(t, tt.ok, ok)
			require.Equal(t, tt.platform, platform)
		})
	}
}

func TestQuotaPlatformCompositeUsesResolvedOrForceOnly(t *testing.T) {
	apiKey := &APIKey{Group: &Group{Platform: PlatformComposite}}

	require.Equal(t, "", QuotaPlatform(context.Background(), apiKey))
	require.Equal(t, PlatformGemini, QuotaPlatform(WithResolvedTargetPlatform(context.Background(), PlatformGemini), apiKey))
	require.Equal(t, PlatformAntigravity, QuotaPlatform(context.WithValue(context.Background(), ctxkey.ForcePlatform, PlatformAntigravity), apiKey))

	ctx := WithResolvedTargetPlatform(context.Background(), PlatformAnthropic)
	ctx = context.WithValue(ctx, ctxkey.ForcePlatform, PlatformAntigravity)
	require.Equal(t, PlatformAntigravity, QuotaPlatform(ctx, apiKey))
}

func TestCompositeGroupSchedulerHasAllCanonicalPlatformBuckets(t *testing.T) {
	seen := make(map[string]struct{})
	for _, bucket := range schedulerCanonicalBuckets(99) {
		seen[bucket.Platform] = struct{}{}
	}
	platforms := make([]string, 0, len(seen))
	for platform := range seen {
		platforms = append(platforms, platform)
	}
	require.ElementsMatch(t,
		[]string{PlatformAnthropic, PlatformGemini, PlatformOpenAI, PlatformAntigravity, PlatformGrok, PlatformKimi, PlatformZhipu, PlatformDeepseek, PlatformMiniMax, PlatformOpenCodeGo, PlatformOllamaCloud},
		platforms,
	)
}

func TestCompositeConcretePlatformsIncludeCNProviders(t *testing.T) {
	for _, platform := range []string{PlatformKimi, PlatformZhipu, PlatformDeepseek, PlatformMiniMax, PlatformOpenCodeGo} {
		require.True(t, isConcreteRequestPlatform(platform))
		require.True(t, canCopyAccountsFromGroupPlatform(PlatformComposite, platform))
	}
}

// 组合路由准入：ollama_cloud 必须被视为具体请求平台，组合路由才能指向它
// （迁移 239 已放宽 DB CHECK；代码侧缺这一条会出现「数据库允许、代码不放行」）。
func TestCompositeConcretePlatformsIncludeOllamaCloud(t *testing.T) {
	require.True(t, isConcreteRequestPlatform(PlatformOllamaCloud))
	require.True(t, canCopyAccountsFromGroupPlatform(PlatformComposite, PlatformOllamaCloud))

	route, err := compositeRouteFromInput(101, CompositeRouteInput{
		PublicModel:    "qwen3-coder",
		TargetPlatform: PlatformOllamaCloud,
	})
	require.NoError(t, err)
	require.Equal(t, PlatformOllamaCloud, route.TargetPlatform)
}
