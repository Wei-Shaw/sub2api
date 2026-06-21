package openai_compat

import "testing"

func TestResolveResponsesSupport(t *testing.T) {
	tests := []struct {
		name  string
		extra map[string]any
		want  AccountResponsesSupport
	}{
		{"nil extra", nil, ResponsesSupportUnknown},
		{"empty extra", map[string]any{}, ResponsesSupportUnknown},
		{"key missing", map[string]any{"other": "value"}, ResponsesSupportUnknown},
		{"value true", map[string]any{ExtraKeyResponsesSupported: true}, ResponsesSupportYes},
		{"value false", map[string]any{ExtraKeyResponsesSupported: false}, ResponsesSupportNo},
		{"value wrong type string", map[string]any{ExtraKeyResponsesSupported: "true"}, ResponsesSupportUnknown},
		{"value wrong type number", map[string]any{ExtraKeyResponsesSupported: 1}, ResponsesSupportUnknown},
		{"value nil", map[string]any{ExtraKeyResponsesSupported: nil}, ResponsesSupportUnknown},
		{"force responses", map[string]any{ExtraKeyResponsesMode: string(ResponsesSupportModeForceResponses)}, ResponsesSupportYes},
		{"force chat completions", map[string]any{ExtraKeyResponsesMode: string(ResponsesSupportModeForceChatCompletions)}, ResponsesSupportNo},
		{"auto follows probe", map[string]any{ExtraKeyResponsesMode: string(ResponsesSupportModeAuto), ExtraKeyResponsesSupported: false}, ResponsesSupportNo},
		{"invalid mode follows probe", map[string]any{ExtraKeyResponsesMode: "bogus", ExtraKeyResponsesSupported: true}, ResponsesSupportYes},
		{"force responses overrides probe false", map[string]any{ExtraKeyResponsesMode: string(ResponsesSupportModeForceResponses), ExtraKeyResponsesSupported: false}, ResponsesSupportYes},
		{"force chat completions overrides probe true", map[string]any{ExtraKeyResponsesMode: string(ResponsesSupportModeForceChatCompletions), ExtraKeyResponsesSupported: true}, ResponsesSupportNo},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := ResolveResponsesSupport(tc.extra)
			if got != tc.want {
				t.Errorf("ResolveResponsesSupport(%v) = %v, want %v", tc.extra, got, tc.want)
			}
		})
	}
}

func TestShouldUseResponsesAPI(t *testing.T) {
	tests := []struct {
		name  string
		extra map[string]any
		want  bool
	}{
		// 关键不变量：未探测必须返回 true（保留旧行为）
		{"unknown defaults to true (preserve old behavior)", nil, true},
		{"unknown empty defaults to true", map[string]any{}, true},
		{"unknown wrong type defaults to true", map[string]any{ExtraKeyResponsesSupported: "yes"}, true},

		// 已探测：标记决定
		{"explicitly supported", map[string]any{ExtraKeyResponsesSupported: true}, true},
		{"explicitly unsupported", map[string]any{ExtraKeyResponsesSupported: false}, false},

		// 手动覆盖：覆盖自动探测结果
		{"force responses overrides unsupported probe", map[string]any{ExtraKeyResponsesMode: string(ResponsesSupportModeForceResponses), ExtraKeyResponsesSupported: false}, true},
		{"force chat completions overrides supported probe", map[string]any{ExtraKeyResponsesMode: string(ResponsesSupportModeForceChatCompletions), ExtraKeyResponsesSupported: true}, false},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := ShouldUseResponsesAPI(tc.extra)
			if got != tc.want {
				t.Errorf("ShouldUseResponsesAPI(%v) = %v, want %v", tc.extra, got, tc.want)
			}
		})
	}
}

func TestNormalizeResponsesSupportMode(t *testing.T) {
	tests := []struct {
		name string
		mode string
		want ResponsesSupportMode
	}{
		{"empty", "", ResponsesSupportModeAuto},
		{"auto", "auto", ResponsesSupportModeAuto},
		{"force responses", "force_responses", ResponsesSupportModeForceResponses},
		{"force chat completions", "force_chat_completions", ResponsesSupportModeForceChatCompletions},
		{"native", "native", ResponsesSupportModeNative},
		{"invalid", "enabled", ResponsesSupportModeAuto},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := NormalizeResponsesSupportMode(tc.mode)
			if got != tc.want {
				t.Errorf("NormalizeResponsesSupportMode(%q) = %q, want %q", tc.mode, got, tc.want)
			}
		})
	}
}

func TestResolveOpenAITextRoute(t *testing.T) {
	tests := []struct {
		name  string
		extra map[string]any
		want  OpenAITextRoute
	}{
		// auto / 未探测：保留"走 Responses"存量行为
		{"nil extra -> responses", nil, RouteResponses},
		{"empty extra -> responses", map[string]any{}, RouteResponses},
		{"auto unprobed -> responses", map[string]any{ExtraKeyResponsesMode: string(ResponsesSupportModeAuto)}, RouteResponses},
		{"supported wrong type -> responses", map[string]any{ExtraKeyResponsesSupported: "true"}, RouteResponses},

		// 智能自动：auto + 探测确认支持 → 原生透传
		{"auto supported -> native", map[string]any{ExtraKeyResponsesSupported: true}, RouteNative},
		{"auto explicit supported -> native", map[string]any{ExtraKeyResponsesMode: string(ResponsesSupportModeAuto), ExtraKeyResponsesSupported: true}, RouteNative},

		// 探测确认不支持 → CC（火山方舟工具坏掉等场景）
		{"auto unsupported -> chat", map[string]any{ExtraKeyResponsesSupported: false}, RouteChatCompletions},
		{"auto explicit unsupported -> chat", map[string]any{ExtraKeyResponsesMode: string(ResponsesSupportModeAuto), ExtraKeyResponsesSupported: false}, RouteChatCompletions},

		// 手动 native：忽略探测标记，恒为原生透传
		{"native + supported true -> native", map[string]any{ExtraKeyResponsesMode: string(ResponsesSupportModeNative), ExtraKeyResponsesSupported: true}, RouteNative},
		{"native + supported false -> native", map[string]any{ExtraKeyResponsesMode: string(ResponsesSupportModeNative), ExtraKeyResponsesSupported: false}, RouteNative},
		{"native + unprobed -> native", map[string]any{ExtraKeyResponsesMode: string(ResponsesSupportModeNative)}, RouteNative},

		// force_responses：忽略探测标记
		{"force responses + unsupported -> responses", map[string]any{ExtraKeyResponsesMode: string(ResponsesSupportModeForceResponses), ExtraKeyResponsesSupported: false}, RouteResponses},
		{"force responses + supported -> responses", map[string]any{ExtraKeyResponsesMode: string(ResponsesSupportModeForceResponses), ExtraKeyResponsesSupported: true}, RouteResponses},

		// force_chat_completions：忽略探测标记
		{"force chat + supported -> chat", map[string]any{ExtraKeyResponsesMode: string(ResponsesSupportModeForceChatCompletions), ExtraKeyResponsesSupported: true}, RouteChatCompletions},
		{"force chat + unsupported -> chat", map[string]any{ExtraKeyResponsesMode: string(ResponsesSupportModeForceChatCompletions), ExtraKeyResponsesSupported: false}, RouteChatCompletions},

		// 非法 mode 跟随探测
		{"invalid mode follows probe supported -> native", map[string]any{ExtraKeyResponsesMode: "bogus", ExtraKeyResponsesSupported: true}, RouteNative},
		{"invalid mode follows probe unsupported -> chat", map[string]any{ExtraKeyResponsesMode: "bogus", ExtraKeyResponsesSupported: false}, RouteChatCompletions},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := ResolveOpenAITextRoute(tc.extra)
			if got != tc.want {
				t.Errorf("ResolveOpenAITextRoute(%v) = %v, want %v", tc.extra, got, tc.want)
			}
		})
	}
}
