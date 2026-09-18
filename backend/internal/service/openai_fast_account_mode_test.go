//go:build unit

package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/pkg/ctxkey"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

// ---------------------------------------------------------------------------
// 账号级 OpenAI Fast 开关（accounts.extra.openai_fast_mode）
//
// 覆盖三个生效点共用的语义：
//   - 纯函数 resolveOpenAIFastModeAction
//   - applyOpenAIFastPolicyToBody（HTTP body 版）
//   - applyOpenAIFastPolicyToWSResponseCreate（WS response.create 帧版）
//   - Forward 内联 patch 版（真实端到端：请求体必须按最终字节到达上游）
//
// 已决策的语义：force 不做模型能力判定，只要管理员配了 force，白名单外
// 模型（gpt-4o / gpt-4.1 / o 系列 / gpt-5.0~5.3 / 自定义上游模型名）同样
// 必须写入 service_tier=priority；客户端已选 ultrafast 时保留 ultrafast。
// ---------------------------------------------------------------------------

func openAIFastAccountWithMode(mode string, extra map[string]any) *Account {
	merged := map[string]any{}
	for k, v := range extra {
		merged[k] = v
	}
	if mode != "" {
		merged[OpenAIFastModeExtraKey] = mode
	}
	return &Account{
		ID:          7,
		Name:        "openai-apikey",
		Platform:    PlatformOpenAI,
		Type:        AccountTypeAPIKey,
		Concurrency: 1,
		Credentials: map[string]any{"api_key": "sk-test"},
		Extra:       merged,
		Status:      StatusActive,
		Schedulable: true,
	}
}

func openAIFastGroupCtx(groupForce bool) context.Context {
	if !groupForce {
		return context.Background()
	}
	return context.WithValue(context.Background(), ctxkey.Group, &Group{
		ID: 7, Platform: PlatformOpenAI, Status: StatusActive, Hydrated: true, ForceOpenAIFast: true,
	})
}

func openAIFastForcePriorityAnyPolicy() *OpenAIFastPolicySettings {
	return &OpenAIFastPolicySettings{Rules: []OpenAIFastPolicyRule{{
		ServiceTier: OpenAIFastTierAny,
		Action:      OpenAIFastPolicyActionForcePriority,
		Scope:       BetaPolicyScopeAll,
	}}}
}

func openAIFastForcePriorityMissingPolicy() *OpenAIFastPolicySettings {
	return &OpenAIFastPolicySettings{Rules: []OpenAIFastPolicyRule{{
		ServiceTier: OpenAIFastTierMissing,
		Action:      OpenAIFastPolicyActionForcePriority,
		Scope:       BetaPolicyScopeAll,
	}}}
}

func openAIFastBlockPriorityPolicy() *OpenAIFastPolicySettings {
	return &OpenAIFastPolicySettings{Rules: []OpenAIFastPolicyRule{{
		ServiceTier:    OpenAIFastTierPriority,
		Action:         BetaPolicyActionBlock,
		Scope:          BetaPolicyScopeAll,
		ErrorMessage:   "fast mode is blocked by admin",
		ModelWhitelist: []string{},
		FallbackAction: BetaPolicyActionPass,
	}}}
}

func openAIFastRequestBodyWithTier(model, clientTier string) []byte {
	if clientTier == "" {
		return []byte(fmt.Sprintf(`{"model":%q,"input":"hi"}`, model))
	}
	return []byte(fmt.Sprintf(`{"model":%q,"service_tier":%q,"input":"hi"}`, model, clientTier))
}

// ---------------------------------------------------------------------------
// A. 纯函数 resolveOpenAIFastModeAction
// ---------------------------------------------------------------------------

func TestResolveOpenAIFastModeAction_AccountModeMatrix(t *testing.T) {
	tests := []struct {
		name    string
		mode    string
		rawTier string
		wantAct string
		wantTie string
	}{
		// ""（跟随）：恒 keep。
		{name: "follow/no tier", mode: "", rawTier: "", wantAct: openAIFastModeActionKeep},
		{name: "follow/fast", mode: "", rawTier: "fast", wantAct: openAIFastModeActionKeep},
		{name: "follow/priority", mode: "", rawTier: "priority", wantAct: openAIFastModeActionKeep},
		{name: "follow/ultrafast", mode: "", rawTier: "ultrafast", wantAct: openAIFastModeActionKeep},
		{name: "follow/unknown", mode: "", rawTier: "client-unknown", wantAct: openAIFastModeActionKeep},
		// force：至少开 fast；只有客户端已选 ultrafast 才保留 ultrafast。
		{name: "force/ultrafast stays ultrafast", mode: OpenAIFastModeForce, rawTier: "ultrafast",
			wantAct: openAIFastModeActionSet, wantTie: OpenAIFastTierUltrafast},
		{name: "force/mixed case ultrafast stays ultrafast", mode: OpenAIFastModeForce, rawTier: "  ULTRAFAST ",
			wantAct: openAIFastModeActionSet, wantTie: OpenAIFastTierUltrafast},
		{name: "force/fast writes priority", mode: OpenAIFastModeForce, rawTier: "fast",
			wantAct: openAIFastModeActionSet, wantTie: OpenAIFastTierPriority},
		{name: "force/priority writes priority", mode: OpenAIFastModeForce, rawTier: "priority",
			wantAct: openAIFastModeActionSet, wantTie: OpenAIFastTierPriority},
		{name: "force/flex writes priority", mode: OpenAIFastModeForce, rawTier: "flex",
			wantAct: openAIFastModeActionSet, wantTie: OpenAIFastTierPriority},
		{name: "force/default writes priority", mode: OpenAIFastModeForce, rawTier: "default",
			wantAct: openAIFastModeActionSet, wantTie: OpenAIFastTierPriority},
		{name: "force/auto writes priority", mode: OpenAIFastModeForce, rawTier: "auto",
			wantAct: openAIFastModeActionSet, wantTie: OpenAIFastTierPriority},
		{name: "force/scale writes priority", mode: OpenAIFastModeForce, rawTier: "scale",
			wantAct: openAIFastModeActionSet, wantTie: OpenAIFastTierPriority},
		{name: "force/missing tier writes priority", mode: OpenAIFastModeForce, rawTier: "",
			wantAct: openAIFastModeActionSet, wantTie: OpenAIFastTierPriority},
		{name: "force/unknown tier writes priority", mode: OpenAIFastModeForce, rawTier: "client-unknown",
			wantAct: openAIFastModeActionSet, wantTie: OpenAIFastTierPriority},
		// off：无条件 delete。
		{name: "off/missing tier deletes", mode: OpenAIFastModeOff, rawTier: "",
			wantAct: openAIFastModeActionDelete},
		{name: "off/fast deletes", mode: OpenAIFastModeOff, rawTier: "fast",
			wantAct: openAIFastModeActionDelete},
		{name: "off/priority deletes", mode: OpenAIFastModeOff, rawTier: "priority",
			wantAct: openAIFastModeActionDelete},
		{name: "off/ultrafast deletes", mode: OpenAIFastModeOff, rawTier: "ultrafast",
			wantAct: openAIFastModeActionDelete},
		{name: "off/flex deletes", mode: OpenAIFastModeOff, rawTier: "flex",
			wantAct: openAIFastModeActionDelete},
		{name: "off/unknown tier deletes", mode: OpenAIFastModeOff, rawTier: "client-unknown",
			wantAct: openAIFastModeActionDelete},
		// 未识别 mode（Account.OpenAIFastMode 已过滤，这里兜底）→ keep。
		{name: "unknown mode keeps", mode: "bogus", rawTier: "priority", wantAct: openAIFastModeActionKeep},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			action, tier := resolveOpenAIFastModeAction(tt.mode, tt.rawTier)
			require.Equal(t, tt.wantAct, action)
			require.Equal(t, tt.wantTie, tier)
		})
	}
}

// TestResolveOpenAIFastModeAction_ForceHasNoModelGate 是防止模型门槛回退的
// 显式断言：函数签名里已经没有 model 参数，force 只由 mode + rawTier 决定。
func TestResolveOpenAIFastModeAction_ForceHasNoModelGate(t *testing.T) {
	// 曾经不在 configuredCodexSupportsPriorityServiceTier 白名单里的模型，
	// 在纯函数层面已无任何模型输入可以影响判定。
	for _, model := range []string{"gpt-4o", "gpt-4.1", "o3", "gpt-5.3", "custom-upstream-model"} {
		t.Run(model, func(t *testing.T) {
			require.False(t, configuredCodexSupportsPriorityServiceTier(model),
				"precondition: %s was outside the legacy model allowlist", model)
			action, tier := resolveOpenAIFastModeAction(OpenAIFastModeForce, "")
			require.Equal(t, openAIFastModeActionSet, action)
			require.Equal(t, OpenAIFastTierPriority, tier)
		})
	}
}

// ---------------------------------------------------------------------------
// B. applyOpenAIFastPolicyToBody（HTTP body 版）
// ---------------------------------------------------------------------------

func TestAccountFastMode_Body_Matrix(t *testing.T) {
	tests := []struct {
		name        string
		mode        string
		groupForce  bool
		policy      *OpenAIFastPolicySettings
		model       string
		clientTier  string
		wantTier    string // "" = 期望最终 body 不含 service_tier
		wantBlocked bool
	}{
		// 账号级 ""（跟随）：与改动前完全一致。
		{name: "follow+default policy normalizes fast", mode: "", policy: DefaultOpenAIFastPolicySettings(),
			model: "gpt-5.5", clientTier: "fast", wantTier: OpenAIFastTierPriority},
		{name: "follow+default policy passes priority", mode: "", policy: DefaultOpenAIFastPolicySettings(),
			model: "gpt-5.5", clientTier: "priority", wantTier: OpenAIFastTierPriority},
		{name: "follow+default policy keeps missing tier", mode: "", policy: DefaultOpenAIFastPolicySettings(),
			model: "gpt-5.5", clientTier: "", wantTier: ""},
		{name: "follow+default policy passes unknown tier verbatim", mode: "", policy: DefaultOpenAIFastPolicySettings(),
			model: "gpt-5.5", clientTier: "client-unknown", wantTier: "client-unknown"},
		{name: "follow+filter strips field", mode: "", policy: openAIFastFilterPriorityPolicy(),
			model: "gpt-5.5", clientTier: "priority", wantTier: ""},
		{name: "follow+force_priority rewrites tier", mode: "", policy: openAIFastForcePriorityAnyPolicy(),
			model: "gpt-5.5", clientTier: "flex", wantTier: OpenAIFastTierPriority},
		{name: "follow+force_priority injects missing tier", mode: "", policy: openAIFastForcePriorityMissingPolicy(),
			model: "gpt-5.5", clientTier: "", wantTier: OpenAIFastTierPriority},
		{name: "follow+group force injects tier", mode: "", groupForce: true, policy: DefaultOpenAIFastPolicySettings(),
			model: "gpt-5.6-sol", clientTier: "", wantTier: OpenAIFastTierPriority},
		{name: "follow+group force still honors global filter", mode: "", groupForce: true,
			policy: openAIFastFilterPriorityPolicy(), model: "gpt-5.6-sol", clientTier: "", wantTier: ""},

		// 账号级 off：覆盖分组级 force 与全局策略的 filter / force_priority / pass。
		{name: "off beats group force when client sent no tier", mode: OpenAIFastModeOff, groupForce: true,
			policy: DefaultOpenAIFastPolicySettings(), model: "gpt-5.6-sol", clientTier: "", wantTier: ""},
		{name: "off beats group force with client priority", mode: OpenAIFastModeOff, groupForce: true,
			policy: DefaultOpenAIFastPolicySettings(), model: "gpt-5.6-sol", clientTier: "priority", wantTier: ""},
		{name: "off deletes client ultrafast", mode: OpenAIFastModeOff, policy: DefaultOpenAIFastPolicySettings(),
			model: "gpt-5.6-sol", clientTier: "ultrafast", wantTier: ""},
		{name: "off deletes unknown tier", mode: OpenAIFastModeOff, policy: DefaultOpenAIFastPolicySettings(),
			model: "gpt-5.5", clientTier: "client-unknown", wantTier: ""},
		{name: "off beats global force_priority", mode: OpenAIFastModeOff, policy: openAIFastForcePriorityAnyPolicy(),
			model: "gpt-5.5", clientTier: "flex", wantTier: ""},
		{name: "off beats global filter on missing tier", mode: OpenAIFastModeOff,
			policy: openAIFastFilterPriorityPolicy(), model: "gpt-5.5", clientTier: "", wantTier: ""},

		// 账号级 force：覆盖分组级 force 与 filter / pass，写 priority（或保留 ultrafast）。
		{name: "force injects missing tier", mode: OpenAIFastModeForce, policy: DefaultOpenAIFastPolicySettings(),
			model: "gpt-5.5", clientTier: "", wantTier: OpenAIFastTierPriority},
		{name: "force beats global filter", mode: OpenAIFastModeForce, policy: openAIFastFilterPriorityPolicy(),
			model: "gpt-5.5", clientTier: "priority", wantTier: OpenAIFastTierPriority},
		{name: "force beats global filter on missing tier", mode: OpenAIFastModeForce,
			policy: openAIFastFilterPriorityPolicy(), model: "gpt-5.5", clientTier: "", wantTier: OpenAIFastTierPriority},
		{name: "force rewrites client flex", mode: OpenAIFastModeForce, policy: DefaultOpenAIFastPolicySettings(),
			model: "gpt-5.5", clientTier: "flex", wantTier: OpenAIFastTierPriority},
		{name: "force rewrites unknown tier", mode: OpenAIFastModeForce, policy: DefaultOpenAIFastPolicySettings(),
			model: "gpt-5.5", clientTier: "client-unknown", wantTier: OpenAIFastTierPriority},
		{name: "force beats global force_priority", mode: OpenAIFastModeForce, policy: openAIFastForcePriorityAnyPolicy(),
			model: "gpt-5.5", clientTier: "flex", wantTier: OpenAIFastTierPriority},
		{name: "force keeps client ultrafast on capable-model name", mode: OpenAIFastModeForce,
			policy: DefaultOpenAIFastPolicySettings(), model: "gpt-5.6-sol", clientTier: "ultrafast",
			wantTier: OpenAIFastTierUltrafast},
		{name: "force keeps client ultrafast read before group force rewrite", mode: OpenAIFastModeForce,
			groupForce: true, policy: DefaultOpenAIFastPolicySettings(), model: "gpt-5.6-sol",
			clientTier: "ultrafast", wantTier: OpenAIFastTierUltrafast},
		{name: "force with group force writes priority", mode: OpenAIFastModeForce, groupForce: true,
			policy: DefaultOpenAIFastPolicySettings(), model: "gpt-5.6-sol", clientTier: "", wantTier: OpenAIFastTierPriority},

		// 全局策略 block 最高，账号级不可绕过。
		{name: "block beats follow", mode: "", policy: openAIFastBlockPriorityPolicy(),
			model: "gpt-5.5", clientTier: "priority", wantBlocked: true},
		{name: "block beats force", mode: OpenAIFastModeForce, policy: openAIFastBlockPriorityPolicy(),
			model: "gpt-5.5", clientTier: "priority", wantBlocked: true},
		{name: "block beats off", mode: OpenAIFastModeOff, policy: openAIFastBlockPriorityPolicy(),
			model: "gpt-5.5", clientTier: "priority", wantBlocked: true},
		{name: "block beats off with fast alias", mode: OpenAIFastModeOff, policy: openAIFastBlockPriorityPolicy(),
			model: "gpt-5.5", clientTier: "fast", wantBlocked: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			svc := newOpenAIGatewayServiceWithSettings(t, tt.policy)
			account := openAIFastAccountWithMode(tt.mode, nil)
			body := openAIFastRequestBodyWithTier(tt.model, tt.clientTier)

			updated, err := svc.applyOpenAIFastPolicyToBody(openAIFastGroupCtx(tt.groupForce), account, tt.model, body)
			if tt.wantBlocked {
				require.Error(t, err)
				var blocked *OpenAIFastBlockedError
				require.True(t, errors.As(err, &blocked), "must return *OpenAIFastBlockedError")
				require.Equal(t, string(body), string(updated), "body must not be mutated on block")
				return
			}
			require.NoError(t, err)
			if tt.wantTier == "" {
				require.False(t, gjson.GetBytes(updated, "service_tier").Exists(),
					"final body must not carry service_tier, got %s", string(updated))
				return
			}
			require.Equal(t, tt.wantTier, gjson.GetBytes(updated, "service_tier").String(), "body=%s", string(updated))
		})
	}
}

// TestAccountFastMode_Body_ForceWritesPriorityForNonAllowlistedModels 显式锁住
// 「去掉模型门槛」：旧白名单之外（含自定义上游模型名）在 force 下也必须写 priority。
func TestAccountFastMode_Body_ForceWritesPriorityForNonAllowlistedModels(t *testing.T) {
	svc := newOpenAIGatewayServiceWithSettings(t, DefaultOpenAIFastPolicySettings())

	for _, model := range []string{"gpt-4o", "gpt-4.1", "o3", "gpt-5.3", "gpt-5.6-max-unknown", "my-custom-upstream-model"} {
		t.Run(model, func(t *testing.T) {
			account := openAIFastAccountWithMode(OpenAIFastModeForce, nil)
			body := openAIFastRequestBodyWithTier(model, "")

			updated, err := svc.applyOpenAIFastPolicyToBody(context.Background(), account, model, body)
			require.NoError(t, err)
			require.Equal(t, OpenAIFastTierPriority, gjson.GetBytes(updated, "service_tier").String(),
				"force must not silently degrade to keep for model %s (body=%s)", model, string(updated))
		})
	}
}

// TestAccountFastMode_Body_NonOpenAIAccountIgnoresExtra 非 openai 平台账号即使
// extra 里写了 openai_fast_mode 也不生效（Account.OpenAIFastMode 恒返回 ""）。
func TestAccountFastMode_Body_NonOpenAIAccountIgnoresExtra(t *testing.T) {
	svc := newOpenAIGatewayServiceWithSettings(t, DefaultOpenAIFastPolicySettings())

	for _, mode := range []string{OpenAIFastModeForce, OpenAIFastModeOff} {
		for _, tier := range []string{"", "flex", "priority"} {
			t.Run(mode+"/tier="+tier, func(t *testing.T) {
				account := &Account{
					ID:       9,
					Platform: PlatformGrok,
					Type:     AccountTypeAPIKey,
					Extra:    map[string]any{OpenAIFastModeExtraKey: mode},
				}
				require.Empty(t, account.OpenAIFastMode(), "non-openai account must report no fast mode")

				body := openAIFastRequestBodyWithTier("grok-4.1", tier)
				updated, err := svc.applyOpenAIFastPolicyToBody(context.Background(), account, "grok-4.1", body)
				require.NoError(t, err)
				require.Equal(t, string(body), string(updated),
					"non-openai account must not be touched by the account-level fast switch")
			})
		}
	}
}

// ---------------------------------------------------------------------------
// C. applyOpenAIFastPolicyToWSResponseCreate（WS 帧版）
// ---------------------------------------------------------------------------

func TestAccountFastMode_WSResponseCreate_Matrix(t *testing.T) {
	tests := []struct {
		name        string
		mode        string
		groupForce  bool
		policy      *OpenAIFastPolicySettings
		model       string
		clientTier  string
		wantTier    string
		wantBlocked bool
	}{
		{name: "follow+default policy normalizes fast", mode: "", policy: DefaultOpenAIFastPolicySettings(),
			model: "gpt-5.5", clientTier: "fast", wantTier: OpenAIFastTierPriority},
		{name: "follow+group force injects tier", mode: "", groupForce: true,
			policy: DefaultOpenAIFastPolicySettings(), model: "gpt-5.6-sol", clientTier: "",
			wantTier: OpenAIFastTierPriority},
		{name: "off beats group force when client sent no tier", mode: OpenAIFastModeOff, groupForce: true,
			policy: DefaultOpenAIFastPolicySettings(), model: "gpt-5.6-sol", clientTier: "", wantTier: ""},
		{name: "off beats group force with client ultrafast", mode: OpenAIFastModeOff, groupForce: true,
			policy: DefaultOpenAIFastPolicySettings(), model: "gpt-5.6-sol", clientTier: "ultrafast", wantTier: ""},
		{name: "off deletes client priority", mode: OpenAIFastModeOff, policy: DefaultOpenAIFastPolicySettings(),
			model: "gpt-5.5", clientTier: "priority", wantTier: ""},
		{name: "off beats global filter", mode: OpenAIFastModeOff, policy: openAIFastFilterPriorityPolicy(),
			model: "gpt-5.5", clientTier: "flex", wantTier: ""},
		{name: "force writes priority", mode: OpenAIFastModeForce, policy: DefaultOpenAIFastPolicySettings(),
			model: "gpt-5.5", clientTier: "", wantTier: OpenAIFastTierPriority},
		{name: "force beats global filter", mode: OpenAIFastModeForce, policy: openAIFastFilterPriorityPolicy(),
			model: "gpt-5.5", clientTier: "priority", wantTier: OpenAIFastTierPriority},
		{name: "force keeps ultrafast on sol", mode: OpenAIFastModeForce, policy: DefaultOpenAIFastPolicySettings(),
			model: "gpt-5.6-sol", clientTier: "ultrafast", wantTier: OpenAIFastTierUltrafast},
		{name: "force writes priority for legacy-allowlisted-out gpt-4o", mode: OpenAIFastModeForce,
			policy: DefaultOpenAIFastPolicySettings(), model: "gpt-4o", clientTier: "flex",
			wantTier: OpenAIFastTierPriority},
		{name: "block beats follow", mode: "", policy: openAIFastBlockPriorityPolicy(),
			model: "gpt-5.5", clientTier: "priority", wantBlocked: true},
		{name: "block beats force", mode: OpenAIFastModeForce, policy: openAIFastBlockPriorityPolicy(),
			model: "gpt-5.5", clientTier: "priority", wantBlocked: true},
		{name: "block beats off", mode: OpenAIFastModeOff, policy: openAIFastBlockPriorityPolicy(),
			model: "gpt-5.5", clientTier: "priority", wantBlocked: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			svc := newOpenAIGatewayServiceWithSettings(t, tt.policy)
			account := openAIFastAccountWithMode(tt.mode, nil)
			var frame []byte
			if tt.clientTier == "" {
				frame = []byte(fmt.Sprintf(`{"type":"response.create","model":%q,"input":[{"type":"input_text","text":"hi"}]}`, tt.model))
			} else {
				frame = []byte(fmt.Sprintf(`{"type":"response.create","model":%q,"service_tier":%q}`, tt.model, tt.clientTier))
			}

			updated, blocked, err := svc.applyOpenAIFastPolicyToWSResponseCreate(
				openAIFastGroupCtx(tt.groupForce), account, tt.model, frame)
			if tt.wantBlocked {
				// WS 版把 block 放在独立的返回值里，err 保持 nil。
				require.NoError(t, err)
				require.NotNil(t, blocked)
				require.Contains(t, blocked.Message, "blocked")
				require.Equal(t, string(frame), string(updated), "frame must not be mutated on block")
				return
			}
			require.NoError(t, err)
			require.Nil(t, blocked)
			if tt.wantTier == "" {
				require.False(t, gjson.GetBytes(updated, "service_tier").Exists(),
					"final frame must not carry service_tier, got %s", string(updated))
				return
			}
			require.Equal(t, tt.wantTier, gjson.GetBytes(updated, "service_tier").String(), "frame=%s", string(updated))
		})
	}
}

// TestAccountFastMode_WSResponseCreate_IgnoresOtherFrameTypes 非 response.create
// 帧（包括空 type）不被账号级开关改动。
func TestAccountFastMode_WSResponseCreate_IgnoresOtherFrameTypes(t *testing.T) {
	svc := newOpenAIGatewayServiceWithSettings(t, DefaultOpenAIFastPolicySettings())

	frames := [][]byte{
		[]byte(`{"type":"response.cancel","service_tier":"priority"}`),
		[]byte(`{"type":"conversation.item.create","service_tier":"ultrafast"}`),
		[]byte(`{"service_tier":"priority"}`),
	}
	for _, mode := range []string{OpenAIFastModeForce, OpenAIFastModeOff} {
		for _, frame := range frames {
			t.Run(mode+"/"+gjson.GetBytes(frame, "type").String(), func(t *testing.T) {
				account := openAIFastAccountWithMode(mode, nil)
				updated, blocked, err := svc.applyOpenAIFastPolicyToWSResponseCreate(
					openAIFastGroupCtx(true), account, "gpt-5.5", frame)
				require.NoError(t, err)
				require.Nil(t, blocked)
				require.Equal(t, string(frame), string(updated),
					"non response.create frames must be passed through untouched")
			})
		}
	}
}

// ---------------------------------------------------------------------------
// D. Forward 内联 patch 段（真实端到端：最终出站字节体）
// ---------------------------------------------------------------------------

func newOpenAIFastForwardTestService(t *testing.T, policy *OpenAIFastPolicySettings) (*OpenAIGatewayService, *httpUpstreamRecorder) {
	t.Helper()
	repo := &openAIFastPolicyRepoStub{values: map[string]string{}}
	if policy != nil {
		raw, err := json.Marshal(policy)
		require.NoError(t, err)
		repo.values[SettingKeyOpenAIFastPolicySettings] = string(raw)
	}
	upstream := &httpUpstreamRecorder{resp: &http.Response{
		StatusCode: http.StatusOK,
		Header:     http.Header{"Content-Type": []string{"application/json"}, "x-request-id": []string{"rid-fast-mode"}},
		Body: io.NopCloser(strings.NewReader(
			`{"id":"resp_1","object":"response","status":"completed","model":"gpt-5.5","output":[],"usage":{"input_tokens":1,"output_tokens":1}}`,
		)),
	}}
	svc := &OpenAIGatewayService{
		cfg:            &config.Config{Security: config.SecurityConfig{URLAllowlist: config.URLAllowlistConfig{Enabled: false}}},
		httpUpstream:   upstream,
		settingService: NewSettingService(repo, &config.Config{}),
	}
	return svc, upstream
}

// openAIFastForwardBody 构造 /v1/responses 请求体（非流式），tier 为空时不带字段。
func openAIFastForwardBody(model, clientTier string) []byte {
	if clientTier == "" {
		return []byte(fmt.Sprintf(`{"model":%q,"input":"hi","stream":false}`, model))
	}
	return []byte(fmt.Sprintf(`{"model":%q,"service_tier":%q,"input":"hi","stream":false}`, model, clientTier))
}

func TestAccountFastMode_ForwardInlinePatch_Matrix(t *testing.T) {
	gin.SetMode(gin.TestMode)

	tests := []struct {
		name       string
		mode       string
		groupForce bool
		policy     *OpenAIFastPolicySettings
		model      string
		clientTier string
		wantTier   string // "" = 期望上游收到的 body 不含 service_tier
	}{
		{
			// 真实缺陷回归：客户端未传 tier + 分组 ForceOpenAIFast + 账号级 off
			// 曾漏删分组级刚写入的 priority，最终仍带 priority。
			name: "off deletes group-forced tier when client sent no tier",
			mode: OpenAIFastModeOff, groupForce: true, policy: DefaultOpenAIFastPolicySettings(),
			model: "gpt-5.5", clientTier: "", wantTier: "",
		},
		{
			name: "off deletes group-forced tier when client sent priority",
			mode: OpenAIFastModeOff, groupForce: true, policy: DefaultOpenAIFastPolicySettings(),
			model: "gpt-5.5", clientTier: "priority", wantTier: "",
		},
		{
			name: "off deletes client ultrafast",
			mode: OpenAIFastModeOff, policy: DefaultOpenAIFastPolicySettings(),
			model: "gpt-5.5", clientTier: "ultrafast", wantTier: "",
		},
		{
			name: "force writes priority when client sent no tier",
			mode: OpenAIFastModeForce, policy: DefaultOpenAIFastPolicySettings(),
			model: "gpt-5.5", clientTier: "", wantTier: OpenAIFastTierPriority,
		},
		{
			name: "force beats global filter",
			mode: OpenAIFastModeForce, policy: openAIFastFilterPriorityPolicy(),
			model: "gpt-5.5", clientTier: "priority", wantTier: OpenAIFastTierPriority,
		},
		{
			name: "off beats global filter",
			mode: OpenAIFastModeOff, policy: openAIFastFilterPriorityPolicy(),
			model: "gpt-5.5", clientTier: "priority", wantTier: "",
		},
		{
			name: "force keeps client ultrafast",
			mode: OpenAIFastModeForce, policy: DefaultOpenAIFastPolicySettings(),
			model: "gpt-5.6-sol", clientTier: "ultrafast", wantTier: OpenAIFastTierUltrafast,
		},
		{
			name: "force writes priority for legacy-allowlisted-out model",
			mode: OpenAIFastModeForce, policy: DefaultOpenAIFastPolicySettings(),
			model: "gpt-4o", clientTier: "", wantTier: OpenAIFastTierPriority,
		},
		{
			name: "follow keeps group force behavior",
			mode: "", groupForce: true, policy: DefaultOpenAIFastPolicySettings(),
			model: "gpt-5.5", clientTier: "", wantTier: OpenAIFastTierPriority,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			svc, upstream := newOpenAIFastForwardTestService(t, tt.policy)
			account := openAIFastAccountWithMode(tt.mode,
				map[string]any{"openai_responses_supported": true})

			rec := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(rec)
			body := openAIFastForwardBody(tt.model, tt.clientTier)
			c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", nil)
			c.Request.Header.Set("Content-Type", "application/json")

			result, err := svc.Forward(openAIFastGroupCtx(tt.groupForce), c, account, body)
			require.NoError(t, err)
			require.NotNil(t, result)
			require.NotNil(t, upstream.lastBody, "upstream must receive the patched body")

			if tt.wantTier == "" {
				require.False(t, gjson.GetBytes(upstream.lastBody, "service_tier").Exists(),
					"final upstream body must not carry service_tier, got %s", string(upstream.lastBody))
				require.Nil(t, result.ServiceTier, "billing context must not treat the request as fast")
				return
			}
			require.Equal(t, tt.wantTier, gjson.GetBytes(upstream.lastBody, "service_tier").String(),
				"upstream body=%s", string(upstream.lastBody))
			require.NotNil(t, result.ServiceTier)
			require.Equal(t, tt.wantTier, *result.ServiceTier)
		})
	}
}
