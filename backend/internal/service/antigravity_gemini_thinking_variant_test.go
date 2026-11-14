package service

import (
	"context"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/antigravity"
	"github.com/stretchr/testify/require"
)

func newAntigravityAccountWithMapping(mapping map[string]string) *Account {
	mappingAny := make(map[string]any, len(mapping))
	for k, v := range mapping {
		mappingAny[k] = v
	}
	return &Account{
		ID:       1,
		Name:     "ag",
		Platform: PlatformAntigravity,
		Credentials: map[string]any{
			"model_mapping": mappingAny,
		},
	}
}

func TestGeminiThinkingLevelFromBody(t *testing.T) {
	tests := []struct {
		name string
		body string
		want string
	}{
		{"empty body", ``, "high"},
		{"no thinkingConfig", `{"contents":[]}`, "high"},
		{"budget -1 dynamic", `{"generationConfig":{"thinkingConfig":{"thinkingBudget":-1}}}`, "high"},
		{"budget 0 off", `{"generationConfig":{"thinkingConfig":{"thinkingBudget":0}}}`, "low"},
		{"budget 1000 (agy low)", `{"generationConfig":{"thinkingConfig":{"thinkingBudget":1000}}}`, "low"},
		{"budget 4000 (agy medium)", `{"generationConfig":{"thinkingConfig":{"thinkingBudget":4000}}}`, "medium"},
		{"budget 24576", `{"generationConfig":{"thinkingConfig":{"thinkingBudget":24576}}}`, "high"},
		{"thinkingLevel wins over budget", `{"generationConfig":{"thinkingConfig":{"thinkingBudget":24576,"thinkingLevel":"low"}}}`, "low"},
		{"thinkingLevel minimal wins over high budget", `{"generationConfig":{"thinkingConfig":{"thinkingBudget":24576,"thinkingLevel":" MINIMAL "}}}`, "low"},
		{"thinkingLevel minimal without budget", `{"generationConfig":{"thinkingConfig":{"thinkingLevel":"minimal"}}}`, "low"},
		{"thinkingLevel medium", `{"generationConfig":{"thinkingConfig":{"thinkingLevel":"MEDIUM"}}}`, "medium"},
		{"thinkingLevel xhigh", `{"generationConfig":{"thinkingConfig":{"thinkingLevel":"XHIGH"}}}`, "xhigh"},
		{"snake_case thinking_level xhigh", `{"generation_config":{"thinking_config":{"thinking_level":"xhigh"}}}`, "xhigh"},
		{"invalid json", `{"generationConfig":`, "high"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			require.Equal(t, tt.want, geminiThinkingLevelFromBody([]byte(tt.body)))
		})
	}
}

func TestResolveGeminiThinkingVariant(t *testing.T) {
	// 与生产 Antigravity 账号从上游同步来的目录一致：只有带后缀的变体，没有裸名。
	catalog := map[string]string{
		"gemini-3.8-flash-low":    "gemini-3.8-flash-low",
		"gemini-3.8-flash-medium": "gemini-3.8-flash-medium",
		"gemini-3.8-flash-high":   "gemini-3.8-flash-high",
		"gemini-3.8-flash-tiered": "gemini-3.8-flash-tiered",
		"gemini-3.6-flash-high":   "gemini-3.6-flash-high",
		"gemini-3.1-pro-high":     "gemini-3.1-pro-high",
		"gemini-2.5-flash":        "gemini-2.5-flash",
	}
	budget := func(v string) []byte {
		return []byte(`{"contents":[],"generationConfig":{"thinkingConfig":{"thinkingBudget":` + v + `}}}`)
	}

	tests := []struct {
		name      string
		mapping   map[string]string
		model     string
		body      []byte
		want      string
		wantMatch bool
	}{
		{"agy high (-1) -> -high", catalog, "gemini-3.8-flash", budget("-1"), "gemini-3.8-flash-high", true},
		{"agy medium (4000) -> -medium", catalog, "gemini-3.8-flash", budget("4000"), "gemini-3.8-flash-medium", true},
		{"agy low (1000) -> -low", catalog, "gemini-3.8-flash", budget("1000"), "gemini-3.8-flash-low", true},
		{"models/ prefix stripped", catalog, "models/gemini-3.8-flash", budget("1000"), "gemini-3.8-flash-low", true},
		{"no thinkingConfig -> -high", catalog, "gemini-3.8-flash", []byte(`{"contents":[]}`), "gemini-3.8-flash-high", true},
		{"only -high exists: low request degrades to high", map[string]string{
			"gemini-3.5-flash-high": "gemini-3.5-flash-high",
		}, "gemini-3.5-flash", budget("1000"), "gemini-3.5-flash-high", true},
		// gemini-3.6/3.7/3.8-flash 的四个变体会被 resolveModelMapping 自动补齐（存量显式映射账号的兼容透传）。
		{"injected default variants are usable", catalog, "gemini-3.6-flash", budget("1000"), "gemini-3.6-flash-low", true},
		{"already suffixed: untouched", catalog, "gemini-3.8-flash-low", budget("-1"), "", false},
		{"bare name explicitly mapped: untouched", catalog, "gemini-2.5-flash", budget("-1"), "", false},
		{"no variants in mapping: untouched", catalog, "gemini-9.9-flash", budget("-1"), "", false},
		{"non-gemini model: untouched", catalog, "claude-sonnet-4-6", budget("-1"), "", false},
		{"explicit bare alias wins over variants", map[string]string{
			"gemini-3.8-flash":      "gemini-3.8-flash-tiered",
			"gemini-3.8-flash-high": "gemini-3.8-flash-high",
		}, "gemini-3.8-flash", budget("-1"), "", false},
		{"variant value is itself a mapping target", map[string]string{
			"gemini-3.8-flash-high": "gemini-3.8-flash-tiered",
		}, "gemini-3.8-flash", budget("-1"), "gemini-3.8-flash-tiered", true},
		// 前端持久化的默认目录含裸名自映射；-low 由默认透传补齐，按预算命中。
		{"persisted default bare identity passthrough still resolves when variants exist", map[string]string{
			"gemini-3.8-flash":      "gemini-3.8-flash",
			"gemini-3.8-flash-high": "gemini-3.8-flash-high",
		}, "gemini-3.8-flash", budget("1000"), "gemini-3.8-flash-low", true},
		{"bare identity passthrough resolves a configured variant outside the default passthroughs", map[string]string{
			"gemini-3.5-flash":      "gemini-3.5-flash",
			"gemini-3.5-flash-high": "gemini-3.5-flash-high",
		}, "gemini-3.5-flash", budget("1000"), "gemini-3.5-flash-high", true},
		{"wildcard mapping does not fabricate variants for non-tiered bare model", map[string]string{
			"gemini-2.5*": "gemini-2.5-flash",
		}, "gemini-2.5-flash", budget("-1"), "", false},
		{"bare model absent from whitelist: configured variant still resolved", map[string]string{
			"gemini-3.7-flash-medium": "gemini-3.7-flash-medium",
		}, "gemini-3.7-flash", budget("4000"), "gemini-3.7-flash-medium", true},
		// 空 credentials 映射保留默认目录兼容行为。
		{"empty mapping falls back to default catalog", map[string]string{}, "gemini-3.8-flash", budget("-1"), "gemini-3.8-flash-high", true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			account := newAntigravityAccountWithMapping(tt.mapping)
			got, matched := resolveGeminiThinkingVariant(account, tt.model, tt.body)
			require.Equal(t, tt.wantMatch, matched)
			require.Equal(t, tt.want, got)
		})
	}

	t.Run("nil account", func(t *testing.T) {
		got, matched := resolveGeminiThinkingVariant(nil, "gemini-3.8-flash", budget("-1"))
		require.False(t, matched)
		require.Empty(t, got)
	})
}

func TestGeminiThinkingLevelFromClaudeThinking(t *testing.T) {
	tests := []struct {
		name     string
		thinking *antigravity.ThinkingConfig
		want     string
	}{
		{"nil thinking", nil, "high"},
		{"disabled", &antigravity.ThinkingConfig{Type: "disabled"}, "low"},
		{"enabled without budget", &antigravity.ThinkingConfig{Type: "enabled"}, "high"},
		{"budget 1024 -> low", &antigravity.ThinkingConfig{Type: "enabled", BudgetTokens: 1024}, "low"},
		{"budget 4000 -> medium", &antigravity.ThinkingConfig{Type: "enabled", BudgetTokens: 4000}, "medium"},
		{"budget 20000 -> high", &antigravity.ThinkingConfig{Type: "enabled", BudgetTokens: 20000}, "high"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			require.Equal(t, tt.want, geminiThinkingLevelFromClaudeRequest(&antigravity.ClaudeRequest{Thinking: tt.thinking}))
		})
	}
}

func TestGeminiThinkingLevelFromClaudeRequest_HonorsOutputEffortAndCarbonTiers(t *testing.T) {
	svc := &AntigravityGatewayService{}
	account := newAntigravityAccountWithMapping(map[string]string{
		"gemini-4-carbon":        "gemini-4-carbon",
		"gemini-4-carbon-low":    "gemini-4-carbon-low",
		"gemini-4-carbon-medium": "gemini-4-carbon-medium",
		"gemini-4-carbon-high":   "gemini-4-carbon-high",
	})

	for _, tc := range []struct {
		name string
		req  antigravity.ClaudeRequest
		want string
	}{
		{"effort low without thinking", antigravity.ClaudeRequest{OutputConfig: &antigravity.ClaudeOutputConfig{Effort: "low"}}, "gemini-4-carbon-low"},
		{"effort wins over high budget", antigravity.ClaudeRequest{
			Thinking:     &antigravity.ThinkingConfig{Type: "enabled", BudgetTokens: 32768},
			OutputConfig: &antigravity.ClaudeOutputConfig{Effort: "medium"},
		}, "gemini-4-carbon-medium"},
		{"disabled wins over high effort", antigravity.ClaudeRequest{
			Thinking:     &antigravity.ThinkingConfig{Type: "disabled"},
			OutputConfig: &antigravity.ClaudeOutputConfig{Effort: "high"},
		}, "gemini-4-carbon-low"},
		{"xhigh degrades to high when model lacks xhigh variant", antigravity.ClaudeRequest{
			OutputConfig: &antigravity.ClaudeOutputConfig{Effort: "xhigh"},
		}, "gemini-4-carbon-high"},
		{"default falls back to high variant", antigravity.ClaudeRequest{}, "gemini-4-carbon-high"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got := svc.getMappedModelForThinkingLevel(account, "gemini-4-carbon", geminiThinkingLevelFromClaudeRequest(&tc.req))
			require.Equal(t, tc.want, got)
		})
	}

	hydrogenAccount := newAntigravityAccountWithMapping(map[string]string{
		"gemini-4-hydrogen-low":    "gemini-4-hydrogen-low",
		"gemini-4-hydrogen-medium": "gemini-4-hydrogen-medium",
		"gemini-4-hydrogen-high":   "gemini-4-hydrogen-high",
		"gemini-4-hydrogen-xhigh":  "gemini-4-hydrogen-xhigh",
	})
	for _, effort := range []string{"xhigh", "max"} {
		req := antigravity.ClaudeRequest{OutputConfig: &antigravity.ClaudeOutputConfig{Effort: effort}}
		require.Equal(t, "gemini-4-hydrogen-xhigh",
			svc.getMappedModelForThinkingLevel(hydrogenAccount, "gemini-4-hydrogen", geminiThinkingLevelFromClaudeRequest(&req)))
	}
	require.Equal(t, "gemini-4-hydrogen-xhigh", svc.getMappedModel(hydrogenAccount, "gemini-4-hydrogen-xhigh"))
	require.Equal(t, "gemini-4-hydrogen-low",
		svc.getMappedModelForThinkingLevel(hydrogenAccount, " models/gemini-4-hydrogen ", "low"))

	// 默认映射（rawMapping 为空）与持久化默认映射下，裸 gemini-4-hydrogen 应支持按 effort 动态分档，缺省落到 -high。
	defaultAccount := &Account{ID: 2, Name: "ag-default", Platform: PlatformAntigravity}
	for _, tc := range []struct {
		effort string
		want   string
	}{
		{"", "gemini-4-hydrogen-high"},
		{"low", "gemini-4-hydrogen-low"},
		{"medium", "gemini-4-hydrogen-medium"},
		{"high", "gemini-4-hydrogen-high"},
		{"xhigh", "gemini-4-hydrogen-xhigh"},
	} {
		req := antigravity.ClaudeRequest{}
		if tc.effort != "" {
			req.OutputConfig = &antigravity.ClaudeOutputConfig{Effort: tc.effort}
		}
		require.Equal(t, tc.want,
			svc.getMappedModelForThinkingLevel(defaultAccount, "gemini-4-hydrogen", geminiThinkingLevelFromClaudeRequest(&req)),
			"effort=%q", tc.effort)
	}

	// 调度准入：仅包含同步变体后缀（包括仅含 -xhigh）的账号必须放行裸模型请求，而显式仅放行 Claude 的账号仍须拒绝。
	gwSvc := &GatewayService{}
	require.True(t, gwSvc.isModelSupportedByAccountWithContext(context.Background(), hydrogenAccount, "gemini-4-hydrogen"))
	require.True(t, gwSvc.isModelSupportedByAccount(hydrogenAccount, "gemini-4-hydrogen"))
	xhighOnlyAccount := newAntigravityAccountWithMapping(map[string]string{"gemini-4-hydrogen-xhigh": "gemini-4-hydrogen-xhigh"})
	require.True(t, gwSvc.isModelSupportedByAccountWithContext(context.Background(), xhighOnlyAccount, "gemini-4-hydrogen"))
	require.True(t, gwSvc.isModelSupportedByAccount(xhighOnlyAccount, "gemini-4-hydrogen"))
	require.Equal(t, "gemini-4-hydrogen-xhigh", svc.getMappedModel(xhighOnlyAccount, "gemini-4-hydrogen"))
	claudeOnlyAccount := newAntigravityAccountWithMapping(map[string]string{"claude-sonnet-4-5": "claude-sonnet-4-5"})
	require.False(t, gwSvc.isModelSupportedByAccountWithContext(context.Background(), claudeOnlyAccount, "gemini-4-hydrogen"))
	require.False(t, gwSvc.isModelSupportedByAccount(claudeOnlyAccount, "gemini-4-hydrogen"))

	// 模型级冷却键归属：裸请求解析为自映射 -low 时冷却 -low，显式单向别名不因二次映射变空。
	lowKey := antigravityRateLimitModelKey(hydrogenAccount, "gemini-4-hydrogen", "gemini-4-hydrogen-low")
	require.Equal(t, "gemini-4-hydrogen-low", resolveFinalAntigravityModelKey(context.Background(), hydrogenAccount, lowKey))
	require.Equal(t, "gemini-4-hydrogen-low", resolveFinalAntigravityModelKey(context.Background(), defaultAccount, antigravityRateLimitModelKey(defaultAccount, "gemini-4-hydrogen", "gemini-4-hydrogen-low")))
	aliasOnlyAccount := newAntigravityAccountWithMapping(map[string]string{"gemini-4-hydrogen": "gemini-4-hydrogen-high"})
	aliasKey := antigravityRateLimitModelKey(aliasOnlyAccount, "gemini-4-hydrogen", "gemini-4-hydrogen-high")
	require.Equal(t, "gemini-4-hydrogen-high", resolveFinalAntigravityModelKey(context.Background(), aliasOnlyAccount, aliasKey))

	// web_search 降级把实际外发模型改写为 gemini-2.5-flash 时，不得再按原 Hydrogen 档位后缀收取 xhigh 倍率。
	webSearchBody := []byte(`{"model":"gemini-2.5-flash","request":{"contents":[{"role":"user","parts":[{"text":"hi"}]}]}}`)
	require.Nil(t, effectiveAntigravityGeminiReasoningEffort("gemini-4-hydrogen-xhigh", webSearchBody, true))
}

func TestClaude55ThinkingVariantResolutionOnAntigravity(t *testing.T) {
	svc := &AntigravityGatewayService{}
	gwSvc := &GatewayService{}
	defaultAccount := &Account{ID: 55, Name: "ag-default", Platform: PlatformAntigravity}

	for _, model := range []string{"claude-opus-5-5", "claude-sonnet-5-5"} {
		for _, tc := range []struct {
			name string
			req  antigravity.ClaudeRequest
			want string
		}{
			{"default falls back to high", antigravity.ClaudeRequest{}, model + "-high"},
			{"adaptive without effort falls back to high", antigravity.ClaudeRequest{Thinking: &antigravity.ThinkingConfig{Type: "adaptive"}}, model + "-high"},
			{"effort low", antigravity.ClaudeRequest{OutputConfig: &antigravity.ClaudeOutputConfig{Effort: "low"}}, model + "-low"},
			{"effort medium", antigravity.ClaudeRequest{
				Thinking:     &antigravity.ThinkingConfig{Type: "adaptive"},
				OutputConfig: &antigravity.ClaudeOutputConfig{Effort: "medium"},
			}, model + "-medium"},
			{"effort high", antigravity.ClaudeRequest{OutputConfig: &antigravity.ClaudeOutputConfig{Effort: "high"}}, model + "-high"},
			{"xhigh degrades to high", antigravity.ClaudeRequest{OutputConfig: &antigravity.ClaudeOutputConfig{Effort: "xhigh"}}, model + "-high"},
			{"disabled maps to low", antigravity.ClaudeRequest{Thinking: &antigravity.ThinkingConfig{Type: "disabled"}}, model + "-low"},
			{"between_tools with low effort", antigravity.ClaudeRequest{
				Thinking:     &antigravity.ThinkingConfig{Type: "between_tools"},
				OutputConfig: &antigravity.ClaudeOutputConfig{Effort: "low"},
			}, model + "-low"},
		} {
			t.Run(model+"/"+tc.name, func(t *testing.T) {
				got := svc.getMappedModelForThinkingLevel(defaultAccount, model, geminiThinkingLevelFromClaudeRequest(&tc.req))
				require.Equal(t, tc.want, got)
			})
		}
	}

	// 仅配置分档变体后缀的账号支持调度准入与按档位转发，且自定义别名指向裸 claude-opus-5-5 也能二次解析到档位。
	tieredOnly := newAntigravityAccountWithMapping(map[string]string{
		"claude-opus-5-5-low":    "claude-opus-5-5-low",
		"claude-opus-5-5-medium": "claude-opus-5-5-medium",
		"claude-opus-5-5-high":   "claude-opus-5-5-high",
		"my-opus":                "claude-opus-5-5",
	})
	require.True(t, gwSvc.isModelSupportedByAccountWithContext(context.Background(), tieredOnly, "claude-opus-5-5"))
	require.True(t, gwSvc.isModelSupportedByAccount(tieredOnly, "claude-opus-5-5"))
	require.Equal(t, "claude-opus-5-5-medium", svc.getMappedModelForThinkingLevel(tieredOnly, "my-opus", "medium"))
	require.Equal(t, "claude-opus-5-5-low", resolveFinalAntigravityModelKey(context.Background(), tieredOnly, antigravityRateLimitModelKey(tieredOnly, "claude-opus-5-5", "claude-opus-5-5-low")))

	// 显式固定映射（如 Opus 5.5→High 预设）优先于动态分档。
	pinnedHigh := newAntigravityAccountWithMapping(map[string]string{
		"claude-opus-5-5":     "claude-opus-5-5-high",
		"claude-opus-5-5-low": "claude-opus-5-5-low",
	})
	require.Equal(t, "claude-opus-5-5-high", svc.getMappedModelForThinkingLevel(pinnedHigh, "claude-opus-5-5", "low"))

	// xhigh 降级到 claude-opus-5-5-high 时，实际计费 effort 记为 high。
	wrappedBody := []byte(`{"model":"claude-opus-5-5-high","request":{"contents":[{"role":"user","parts":[{"text":"hi"}]}]}}`)
	require.Equal(t, "high", optionalStringValue(effectiveAntigravityGeminiReasoningEffort("claude-opus-5-5-high", wrappedBody, true)))
	require.Nil(t, effectiveAntigravityGeminiReasoningEffort("claude-opus-5-5-high", wrappedBody, false))

	// 单独配置裸动态预设（{"claude-opus-5-5": "claude-opus-5-5"}）时，回退官方默认分档变体而非发裸名 404。
	barePresetOnly := newAntigravityAccountWithMapping(map[string]string{"claude-opus-5-5": "claude-opus-5-5"})
	require.Equal(t, "claude-opus-5-5-low", svc.getMappedModelForThinkingLevel(barePresetOnly, "claude-opus-5-5", "low"))
	require.Equal(t, "claude-opus-5-5-medium", svc.getMappedModelForThinkingLevel(barePresetOnly, "claude-opus-5-5", "medium"))
	require.Equal(t, "claude-opus-5-5-high", svc.getMappedModelForThinkingLevel(barePresetOnly, "claude-opus-5-5", ""))

	// 原生 Anthropic 订阅隔离与混合分组冷却对齐：
	// 1) PlatformAnthropic OAuth / SetupToken 订阅账号保持裸模型名不变，绝不附加 -high/-medium/-low 后缀；
	// 2) 混合分组中 Antigravity 账号的 claude-opus-5-5-high 处于冷却时，调度阶段对默认/high 请求直接跳过该账号，
	//    不消耗切换预算即可路由到健康的原生 Anthropic 订阅账号；而对 low 请求仍可正常命中 -low 变体。
	for _, acctType := range []string{AccountTypeOAuth, AccountTypeSetupToken} {
		anthropicSub := &Account{ID: 101, Name: "claude-sub", Platform: PlatformAnthropic, Type: acctType, Status: StatusActive}
		require.Equal(t, "claude-opus-5-5", anthropicSub.GetMappedModel("claude-opus-5-5"))
		require.Equal(t, "claude-sonnet-5-5", anthropicSub.GetMappedModel("claude-sonnet-5-5"))
		require.True(t, gwSvc.isModelSupportedByAccountWithContext(context.Background(), anthropicSub, "claude-opus-5-5"))
		require.False(t, anthropicSub.isModelRateLimitedWithContext(context.Background(), "claude-opus-5-5"))
	}

	coolingAG := &Account{
		ID:       102,
		Name:     "ag-cooling-high",
		Platform: PlatformAntigravity,
		Type:     AccountTypeOAuth,
		Status:   StatusActive,
		Extra: map[string]any{
			modelRateLimitsKey: map[string]any{
				"claude-opus-5-5-high": map[string]any{
					"rate_limit_reset_at": time.Now().Add(5 * time.Minute).UTC().Format(time.RFC3339),
				},
			},
		},
	}
	require.True(t, coolingAG.isModelRateLimitedWithContext(context.Background(), "claude-opus-5-5"),
		"bare default-high request must detect claude-opus-5-5-high cooldown during scheduling")
	require.True(t, coolingAG.isModelRateLimitedWithContext(WithRequestedReasoningEffort(context.Background(), "high"), "claude-opus-5-5"))
	require.False(t, coolingAG.isModelRateLimitedWithContext(WithRequestedReasoningEffort(context.Background(), "low"), "claude-opus-5-5"),
		"low effort request resolves to claude-opus-5-5-low and must not be blocked by -high cooldown")

	// 分组策略将入站 high 改写为 low 后，调度冷却预检必须按改写后的实际有效档位（-low）判定，而非改写前的 high。
	coolingLowAG := &Account{
		ID:       103,
		Name:     "ag-cooling-low",
		Platform: PlatformAntigravity,
		Type:     AccountTypeOAuth,
		Status:   StatusActive,
		Extra: map[string]any{
			modelRateLimitsKey: map[string]any{
				"claude-opus-5-5-low": map[string]any{
					"rate_limit_reset_at": time.Now().Add(5 * time.Minute).UTC().Format(time.RFC3339),
				},
			},
		},
	}
	rewrittenBody, changed, err := ApplyReasoningEffortPolicy(
		[]byte(`{"model":"claude-opus-5-5","output_config":{"effort":"high"}}`),
		"low", nil, "",
	)
	require.NoError(t, err)
	require.True(t, changed)
	policyCtx := WithRequestedReasoningEffort(context.Background(), "high")
	policyCtx = WithAntigravityThinkingLevelFromClaudeBody(policyCtx, rewrittenBody)
	require.True(t, coolingLowAG.isModelRateLimitedWithContext(policyCtx, "claude-opus-5-5"),
		"post-policy low request must detect claude-opus-5-5-low cooldown during scheduling")
	require.False(t, coolingAG.isModelRateLimitedWithContext(policyCtx, "claude-opus-5-5"),
		"post-policy low request must not be blocked by claude-opus-5-5-high cooldown")
}

// 锁定线上 404 回归：Chat Completions / Messages 兼容层收到裸 Gemini 模型名时，
// 必须和 Gemini 原生路径一样解析到带后缀的上游变体，而不是把裸名透传给上游。
func TestResolveGeminiThinkingVariantForLevel_CompatPathResolvesBareModel(t *testing.T) {
	catalog := map[string]string{
		"gemini-3.8-flash-low":    "gemini-3.8-flash-low",
		"gemini-3.8-flash-medium": "gemini-3.8-flash-medium",
		"gemini-3.8-flash-high":   "gemini-3.8-flash-high",
	}
	account := newAntigravityAccountWithMapping(catalog)

	for _, tc := range []struct{ level, want string }{
		{"low", "gemini-3.8-flash-low"},
		{"medium", "gemini-3.8-flash-medium"},
		{"high", "gemini-3.8-flash-high"},
		{"", "gemini-3.8-flash-high"},
	} {
		t.Run("level="+tc.level, func(t *testing.T) {
			got, matched := resolveGeminiThinkingVariantForLevel(account, "gemini-3.8-flash", tc.level)
			require.True(t, matched)
			require.Equal(t, tc.want, got)
		})
	}

	t.Run("already suffixed model is untouched", func(t *testing.T) {
		got, matched := resolveGeminiThinkingVariantForLevel(account, "gemini-3.8-flash-high", "low")
		require.False(t, matched)
		require.Empty(t, got)
	})
}

// 同一请求无论走 Gemini 原生还是兼容层，都必须落到同一个上游变体。
func TestGeminiThinkingLevel_NativeAndCompatAgree(t *testing.T) {
	native := geminiThinkingLevelFromBody([]byte(`{"generationConfig":{"thinkingConfig":{"thinkingBudget":4000}}}`))
	compat := geminiThinkingLevelFromClaudeRequest(&antigravity.ClaudeRequest{Thinking: &antigravity.ThinkingConfig{Type: "enabled", BudgetTokens: 4000}})
	require.Equal(t, native, compat)
}

// 回归锁：裸名解析必须发生在 getMappedModel 这一层，从而覆盖全部 Antigravity
// 转发入口（/v1beta 原生、/v1/chat/completions、/v1/messages）以及账号测试。
// 早前只在单个入口打补丁，导致 /v1/messages 仍把裸名透传给上游并触发
// 404 "Requested entity was not found."
func TestGetMappedModelResolvesBareGeminiModelForAllEntrypoints(t *testing.T) {
	svc := &AntigravityGatewayService{}
	account := newAntigravityAccountWithMapping(map[string]string{
		"gemini-3.8-flash-low":    "gemini-3.8-flash-low",
		"gemini-3.8-flash-medium": "gemini-3.8-flash-medium",
		"gemini-3.8-flash-high":   "gemini-3.8-flash-high",
	})

	t.Run("default entrypoint falls back to high", func(t *testing.T) {
		require.Equal(t, "gemini-3.8-flash-high", svc.getMappedModel(account, "gemini-3.8-flash"))
	})

	t.Run("caller supplied level wins", func(t *testing.T) {
		require.Equal(t, "gemini-3.8-flash-low",
			svc.getMappedModelForThinkingLevel(account, "gemini-3.8-flash", "low"))
		require.Equal(t, "gemini-3.8-flash-medium",
			svc.getMappedModelForThinkingLevel(account, "gemini-3.8-flash", "medium"))
	})

	t.Run("models/ prefix is handled", func(t *testing.T) {
		require.Equal(t, "gemini-3.8-flash-high", svc.getMappedModel(account, "models/gemini-3.8-flash"))
	})

	t.Run("suffixed model keeps normal mapping", func(t *testing.T) {
		require.Equal(t, "gemini-3.8-flash-low", svc.getMappedModel(account, "gemini-3.8-flash-low"))
	})

	t.Run("unsupported model still reports unsupported", func(t *testing.T) {
		require.Empty(t, svc.getMappedModel(account, "gemini-9.9-flash"))
	})
}
