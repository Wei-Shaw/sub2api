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

		// 直通模式：chat 入站必须保持原生 CC，与探测结果无关
		{"passthrough keeps chat native when probe says unsupported", map[string]any{ExtraKeyResponsesMode: string(ResponsesSupportModePassthrough), ExtraKeyResponsesSupported: false}, false},
		{"passthrough keeps chat native when probe says supported", map[string]any{ExtraKeyResponsesMode: string(ResponsesSupportModePassthrough), ExtraKeyResponsesSupported: true}, false},
		{"passthrough keeps chat native when unprobed", map[string]any{ExtraKeyResponsesMode: string(ResponsesSupportModePassthrough)}, false},
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
		{"passthrough", "passthrough", ResponsesSupportModePassthrough},
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

func TestResponsesPassthroughMode(t *testing.T) {
	tests := []struct {
		name  string
		extra map[string]any
		want  bool
	}{
		{"nil extra", nil, false},
		{"empty extra", map[string]any{}, false},
		{"auto is not passthrough", map[string]any{ExtraKeyResponsesMode: string(ResponsesSupportModeAuto)}, false},
		{"force responses is not passthrough", map[string]any{ExtraKeyResponsesMode: string(ResponsesSupportModeForceResponses)}, false},
		{"force chat completions is not passthrough", map[string]any{ExtraKeyResponsesMode: string(ResponsesSupportModeForceChatCompletions)}, false},
		{"invalid mode is not passthrough", map[string]any{ExtraKeyResponsesMode: "bogus"}, false},
		{"wrong type is not passthrough", map[string]any{ExtraKeyResponsesMode: true}, false},
		{"passthrough", map[string]any{ExtraKeyResponsesMode: string(ResponsesSupportModePassthrough)}, true},
		// 探测标记不影响直通判定
		{"passthrough ignores unsupported probe", map[string]any{ExtraKeyResponsesMode: string(ResponsesSupportModePassthrough), ExtraKeyResponsesSupported: false}, true},
		{"passthrough ignores supported probe", map[string]any{ExtraKeyResponsesMode: string(ResponsesSupportModePassthrough), ExtraKeyResponsesSupported: true}, true},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := ResponsesPassthroughMode(tc.extra)
			if got != tc.want {
				t.Errorf("ResponsesPassthroughMode(%v) = %v, want %v", tc.extra, got, tc.want)
			}
		})
	}
}
