package service

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func webImagePolicyAccount(mapping map[string]any) *Account {
	return &Account{Platform: PlatformOpenAI, Type: AccountTypeOAuth,
		Credentials: map[string]any{"plan_type": "free", "model_mapping": mapping},
		Extra:       map[string]any{AccountExtraChatGPTWebImageEnabled: true, AccountExtraChatGPTWebImageRegularChat: true}}
}

func TestChatGPTWebImageModelsIgnoreChatMapping(t *testing.T) {
	cases := []struct {
		name, requested, configured, want string
		mapping                           map[string]any
	}{
		{"fixed_default", "", "poster", "gpt-image-2", map[string]any{"poster": "gpt-image-1.5"}},
		{"no_mapping", "gpt-image-2.5-flare", "", "gpt-image-2.5-flare", nil},
		{"chat_only_mapping", "gpt-image-2.5-sunburst", "", "gpt-image-2.5-sunburst", map[string]any{"gpt-6-luna": "gpt-6-luna"}},
		{"native_ignores_image_remap", "gpt-image-2", "", "gpt-image-2", map[string]any{"gpt-image-2": "gpt-image-1"}},
		{"native_ignores_text_remap", "gpt-image-2", "", "gpt-image-2", map[string]any{"gpt-image-2": "gpt-6-luna"}},
		{"native_ignores_wildcard", "gpt-image-2.5-flare", "", "gpt-image-2.5-flare", map[string]any{"gpt-image-*": "gpt-image-2"}},
		{"existing_alias", "picture", "", "gpt-image-2.5-flare", map[string]any{"picture": "gpt-image-2.5-flare"}},
		{"text_is_not_image", "gpt-6-luna", "", "", map[string]any{"gpt-6-luna": "gpt-6-luna"}},
		{"unknown_alias", "picture", "", "", nil},
		{"literal_wildcard_target", "picture", "", "", map[string]any{"*": "*"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			a := webImagePolicyAccount(tc.mapping)
			a.Extra[AccountExtraChatGPTWebImageDefaultModel] = tc.configured
			resolved, err := resolveChatGPTWebImageModel(a, tc.requested)
			if tc.want == "" {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
			require.Equal(t, tc.want, resolved.MappedModel)
			if tc.requested != "" {
				require.Equal(t, tc.requested, resolved.RequestedModel)
			}
		})
	}
	a := webImagePolicyAccount(nil)
	require.True(t, a.IsModelSupported("gpt-6-luna"))
	require.True(t, a.IsModelSupported("gpt-image-2.5-flare"))
	a.Extra[AccountExtraChatGPTWebImageEnabled] = false
	require.False(t, a.IsModelSupported("gpt-image-2"))
	require.True(t, a.IsModelSupported("gpt-6-luna"))
	_, err := resolveChatGPTWebImageModel(a, "gpt-image-2")
	require.Error(t, err)
}

func TestWebImageResponsesModelPolicyAndOrdinaryChat(t *testing.T) {
	cases := []struct {
		name, body             string
		mapping                map[string]any
		group, wantImage, fail bool
	}{
		{"default_allowed", `{"model":"gpt-6-luna","input":"draw"}`, map[string]any{"gpt-image-2": "gpt-image-2"}, true, true, false},
		{"previous_response_text_chat", `{"model":"gpt-6-luna","previous_response_id":"resp_text","input":"hello"}`, map[string]any{"gpt-6-luna": "gpt-6-luna"}, true, false, false},
		{"no_mapping_plain_chat", `{"model":"gpt-6-luna","input":"hello"}`, nil, true, true, false},
		{"group_off_plain_chat", `{"model":"gpt-6-luna","input":"hello"}`, map[string]any{"gpt-image-2": "gpt-image-2"}, false, false, false},
		{"explicit_allowed_default_denied", `{"model":"gpt-6-luna","tools":[{"type":"image_generation","model":"poster"}]}`, map[string]any{"poster": "gpt-image-1"}, true, true, false},
		{"unlisted_image_allowed", `{"model":"gpt-6-luna","tools":[{"type":"image_generation","model":"gpt-image-1"}]}`, map[string]any{"gpt-image-2": "gpt-image-2"}, true, true, false},
		{"declared_group_off", `{"model":"gpt-6-luna","tools":[{"type":"image_generation","model":"gpt-image-2","action":"edit","quality":"high"}]}`, map[string]any{"gpt-image-2": "gpt-image-2"}, false, false, false},
		{"forced_group_off", `{"model":"gpt-6-luna","tools":[{"type":"image_generation","model":"gpt-image-2"}],"tool_choice":{"type":"image_generation"}}`, map[string]any{"gpt-image-2": "gpt-image-2"}, false, false, true},
		{"forced_unlisted_image_allowed", `{"model":"gpt-6-luna","tools":[{"type":"image_generation","model":"gpt-image-1"}],"tool_choice":{"type":"image_generation"}}`, map[string]any{"gpt-image-2": "gpt-image-2"}, true, true, false},
		{"required_image_only_allowed", `{"model":"gpt-6-luna","tools":[{"type":"namespace","name":"image_gen","tools":[]}],"tool_choice":"required"}`, nil, true, true, false},
		{"required_additional_image_only_allowed", `{"model":"gpt-6-luna","input":[{"type":"additional_tools","tools":[{"type":"image_generation","model":"gpt-image-1"}]}],"tool_choice":"required"}`, map[string]any{"gpt-image-2": "gpt-image-2"}, true, true, false},
		{"additional_edit_rejected", `{"model":"gpt-6-luna","input":[{"type":"additional_tools","tools":[{"type":"image_generation","model":"gpt-image-2","action":"edit"}]}]}`, map[string]any{"gpt-image-2": "gpt-image-2"}, true, false, true},
		{"additional_quality_rejected", `{"model":"gpt-6-luna","input":[{"type":"additional_tools","tools":[{"type":"image_generation","model":"gpt-image-2","quality":"high"}]}]}`, map[string]any{"gpt-image-2": "gpt-image-2"}, true, false, true},
		{"unlisted_additional_edit_rejected", `{"model":"gpt-6-luna","input":[{"type":"additional_tools","tools":[{"type":"image_generation","model":"gpt-image-2","action":"edit","quality":"high"}]}]}`, nil, true, false, true},
		{"explicit_wrong_type", `{"model":"gpt-6-luna","tools":[{"type":"image_generation","model":42}]}`, map[string]any{"gpt-image-2": "gpt-image-2"}, true, false, true},
		{"ambiguous_models", `{"tools":[{"type":"image_generation","model":"gpt-image-1"},{"type":"image_generation","model":"gpt-image-2"}]}`, map[string]any{"gpt-image-*": "gpt-image-2"}, true, false, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			prepared, model, err := prepareWebImageResponsesForAccount([]byte(tc.body), webImagePolicyAccount(tc.mapping), tc.group)
			if tc.fail {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
			require.Equal(t, tc.wantImage, model != nil)
			var got map[string]any
			require.NoError(t, json.Unmarshal(prepared, &got))
			tools := requireWebImageTestValue[[]any](t, got["tools"])
			if tc.wantImage {
				require.Len(t, tools, 1)
				require.Equal(t, webImageFunctionName, requireWebImageTestValue[map[string]any](t, tools[0])["name"])
			} else {
				require.Empty(t, tools)
				require.NotContains(t, got["instructions"], "supplies the server-side")
			}
		})
	}
}

func TestWebImageDisabledRequiredToolCanStillSelectTextTool(t *testing.T) {
	body := []byte(`{"model":"gpt-6-luna","tool_choice":"required","tools":[{"type":"image_generation","model":"gpt-image-2"}],"input":[{"type":"additional_tools","tools":[{"type":"function","name":"clock"}]}]}`)
	a := webImagePolicyAccount(nil)
	a.Extra[AccountExtraChatGPTWebImageEnabled] = false
	prepared, selected, err := prepareWebImageResponsesForAccount(body, a, true)
	require.NoError(t, err)
	require.Nil(t, selected)
	require.NotContains(t, string(prepared), `"type":"image_generation"`)
	require.Contains(t, string(prepared), `"name":"clock"`)
	require.Contains(t, string(prepared), `"tool_choice":"required"`)
}

func TestWebImageDisabledPreservesTextToolsAndCleansBridgeHistory(t *testing.T) {
	a := webImagePolicyAccount(map[string]any{"gpt-image-2": "gpt-image-2"})
	a.Extra[AccountExtraChatGPTWebImageEnabled] = false
	body := []byte(`{"model":"gpt-6-luna","tools":[{"type":"function","name":"exec_command"},{"type":"namespace","name":"image_gen","tools":[]}],"input":[{"type":"additional_tools","tools":[{"type":"namespace","name":"image_gen","tools":[]},{"type":"function","name":"clock"}]},{"type":"image_generation_call","id":"ig_web_prior","result":"private-png"},{"role":"assistant","content":[{"type":"output_text","text":"![Generated image](data:image/jpeg;base64,AAAA)"}]},{"role":"user","content":"hello"}]}`)
	prepared, selected, err := prepareWebImageResponsesForAccount(body, a, true)
	require.NoError(t, err)
	require.Nil(t, selected)
	require.NotContains(t, string(prepared), "private-png")
	require.NotContains(t, string(prepared), "data:image/")
	require.NotContains(t, string(prepared), `"name":"image_gen"`)
	require.Contains(t, string(prepared), `"name":"exec_command"`)
	require.Contains(t, string(prepared), `"name":"clock"`)
	// Disabled Free must still enter the adapter, preventing a hosted fallback.
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", nil)
	require.True(t, ShouldBridgeChatGPTWebResponses(context.Background(), c, a))
}

func TestWebImageResponsesUnconfiguredFreeCannotUseNativeImages(t *testing.T) {
	cases := []struct {
		name  string
		plan  string
		extra map[string]any
		want  bool
	}{
		{"absent_free", "free", nil, true},
		{"empty_free", "free", map[string]any{}, true},
		{"invalid_free", "free", map[string]any{AccountExtraChatGPTWebImageEnabled: "true"}, true},
		{"enabled_free", "free", map[string]any{AccountExtraChatGPTWebImageEnabled: true}, true},
		{"disabled_free", "free", map[string]any{AccountExtraChatGPTWebImageEnabled: false}, true},
		{"absent_pro", "pro", nil, false},
		{"disabled_pro", "pro", map[string]any{AccountExtraChatGPTWebImageEnabled: false}, false},
		{"enabled_pro_is_validated", "pro", map[string]any{AccountExtraChatGPTWebImageEnabled: true}, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			a := webImagePolicyAccount(nil)
			a.Credentials["plan_type"] = tc.plan
			a.Extra = tc.extra
			c, _ := gin.CreateTestContext(httptest.NewRecorder())
			c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", nil)
			require.Equal(t, tc.want, ShouldBridgeChatGPTWebResponses(context.Background(), c, a))
			ctx := context.WithValue(context.Background(), webImageBridgeContextKey{}, true)
			require.False(t, ShouldBridgeChatGPTWebResponses(ctx, c, a), "nested forwarding cannot enter twice")
			c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses/compact", nil)
			require.False(t, ShouldBridgeChatGPTWebResponses(context.Background(), c, a))
		})
	}
}
