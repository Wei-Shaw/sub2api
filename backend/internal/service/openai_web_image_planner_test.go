package service

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

func TestFreeWebImagePlannerAliasesAndDriverMappingsStayLocal(t *testing.T) {
	t.Setenv("SUB2API_IMAGES_MAIN_MODEL", openAIImagesResponsesMainModel)
	for _, tc := range []struct {
		name, model string
		mapping     map[string]any
	}{
		{"native_without_mapping", "gpt-image-2.5-flare", nil},
		{"explicit_alias", "paint", map[string]any{"paint": "gpt-image-2.5-flare"}},
		{"wildcard_alias_and_driver", "paint", map[string]any{"*": "gpt-image-2"}},
		{"native_with_wildcard_driver", "gpt-image-2.5-flare", map[string]any{"*": "gpt-image-2"}},
		{"native_with_exact_driver_image_mapping", "gpt-image-2.5-flare", map[string]any{openAIImagesResponsesMainModel: "gpt-image-2"}},
		{"native_with_old_text_mapping", "gpt-image-2.5-flare", map[string]any{"gpt-image-2.5-flare": "gpt-6-luna"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			a := webImagePolicyAccount(tc.mapping)
			a.Extra[featureKeyCodexImageGenerationBridge] = true
			cached := make(map[string]string)
			for name, target := range a.GetModelMapping() {
				cached[name] = target
			}
			credentialsBefore, err := json.Marshal(a.Credentials)
			require.NoError(t, err)
			original, err := json.Marshal(map[string]any{"model": tc.model, "prompt": "draw a cat"})
			require.NoError(t, err)
			prepared, selection, err := prepareWebImageResponsesForAccount(original, a, true)
			require.NoError(t, err)
			require.Equal(t, tc.model, selection.RequestedModel)
			require.Equal(t, openAIImagesResponsesMainModel, gjson.GetBytes(prepared, "model").String())
			require.Equal(t, "draw a cat", gjson.GetBytes(prepared, "input").String())
			require.Equal(t, webImageFunctionName, gjson.GetBytes(prepared, "tool_choice.name").String())
			require.False(t, openAIRequestBodyHasImageGenerationDeclaration(prepared))

			planner, plannerErr := webImageResponsesPlannerAccount(a, original, prepared)
			require.Nil(t, plannerErr)
			billing, upstream := resolveOpenAIForwardMappedModels(planner, openAIImagesResponsesMainModel, false)
			require.Equal(t, openAIImagesResponsesMainModel, billing)
			require.Equal(t, openAIImagesResponsesMainModel, upstream)
			require.False(t, planner.getExtraBool(featureKeyCodexImageGenerationBridge))
			require.True(t, a.getExtraBool(featureKeyCodexImageGenerationBridge))
			require.NotEqual(t, mapPtr(a.Credentials), mapPtr(planner.Credentials))
			credentialsAfter, err := json.Marshal(a.Credentials)
			require.NoError(t, err)
			require.JSONEq(t, string(credentialsBefore), string(credentialsAfter))
			if len(cached) > 0 {
				require.Equal(t, cached, a.modelMappingCache)
			} else {
				require.Empty(t, a.modelMappingCache)
			}
		})
	}
}

func TestFreeWebImagePlannerPreservesOrdinaryTextMapping(t *testing.T) {
	a := webImagePolicyAccount(map[string]any{"chat": "gpt-6-luna", "paint": "gpt-image-2"})
	original := []byte(`{"model":"chat","input":"hello","tools":[{"type":"function","name":"search","parameters":{"type":"object"}}]}`)
	prepared, _, err := prepareWebImageResponsesForAccount(original, a, true)
	require.NoError(t, err)
	planner, plannerErr := webImageResponsesPlannerAccount(a, original, prepared)
	require.Nil(t, plannerErr)
	require.Equal(t, "chat", gjson.GetBytes(prepared, "model").String())
	billing, upstream := resolveOpenAIForwardMappedModels(planner, "chat", false)
	require.Equal(t, "gpt-6-luna", billing)
	require.Equal(t, "gpt-6-luna", upstream)
	require.Equal(t, "search", gjson.GetBytes(prepared, "tools.0.name").String())
	require.Equal(t, a.GetModelMapping(), planner.GetModelMapping())
}

func TestFreeWebImagePlannerDisabledAliasIsRejectedBeforeForward(t *testing.T) {
	for _, configured := range []bool{false, true} {
		a := webImagePolicyAccount(map[string]any{"paint": "gpt-image-2"})
		if configured {
			a.Extra[AccountExtraChatGPTWebImageEnabled] = false
		} else {
			a.Extra = nil
		}
		prepared, selection, err := prepareWebImageResponsesForAccount([]byte(`{"model":"paint","prompt":"draw a cat"}`), a, true)
		require.ErrorContains(t, err, "not enabled")
		require.Nil(t, prepared)
		require.Nil(t, selection)
	}
}

func TestFreeWebImagePlannerInvalidDriverOverrideIsRejected(t *testing.T) {
	t.Setenv("SUB2API_IMAGES_MAIN_MODEL", "gpt-image-2")
	a := webImagePolicyAccount(nil)
	original := []byte(`{"model":"gpt-image-2.5-flare","prompt":"draw a cat"}`)
	prepared, _, err := prepareWebImageResponsesForAccount(original, a, true)
	require.NoError(t, err)
	planner, plannerErr := webImageResponsesPlannerAccount(a, original, prepared)
	require.Nil(t, planner)
	require.NotNil(t, plannerErr)
	require.Equal(t, http.StatusBadRequest, plannerErr.StatusCode)
	require.Equal(t, "chatgpt_web_image_invalid_planner", plannerErr.Code)
}

func TestFreeWebImagePlannerForwardGuardRejectsNativeImagePaths(t *testing.T) {
	gin.SetMode(gin.TestMode)
	for _, tc := range []struct {
		name, body string
		mapping    map[string]any
	}{
		{"native_model", `{"model":"gpt-image-2","input":"draw"}`, nil},
		{"mapped_model", `{"model":"chat","input":"draw"}`, map[string]any{"chat": "gpt-image-2"}},
		{"wildcard_model", `{"model":"gpt-6-luna","input":"draw"}`, map[string]any{"*": "gpt-image-2"}},
		{"non_gpt_media_model", `{"model":"chat","input":"draw"}`, map[string]any{"chat": "grok-imagine-image"}},
		{"hosted_tool", `{"model":"gpt-6-luna","input":"draw","tools":[{"type":"image_generation"}]}`, nil},
		{"embedded_tool", `{"model":"gpt-6-luna","input":[{"type":"additional_tools","tools":[{"type":"namespace","name":"image_gen"}]}]}`, nil},
		{"explicit_local_image_tool", `{"model":"gpt-6-luna","input":"draw","tool_choice":{"type":"function","name":"imagegen","namespace":"image_gen"}}`, nil},
	} {
		t.Run(tc.name, func(t *testing.T) {
			recorder := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(recorder)
			c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", nil)
			ctx := context.WithValue(context.Background(), webImageBridgeContextKey{}, true)
			// No transport or credentials are installed: rejection must happen
			// before any upstream work, not as a downstream native-image error.
			svc := &OpenAIGatewayService{}
			result, err := svc.Forward(ctx, c, webImagePolicyAccount(tc.mapping), []byte(tc.body))
			require.Nil(t, result)
			require.Error(t, err)
			require.Equal(t, http.StatusBadRequest, recorder.Code)
			require.Equal(t, "chatgpt_web_image_invalid_planner", gjson.GetBytes(recorder.Body.Bytes(), "error.code").String())
		})
	}
}

func TestFreeWebImagePlannerForwardDoesNotReinjectHostedTool(t *testing.T) {
	gin.SetMode(gin.TestMode)
	t.Setenv("SUB2API_IMAGES_MAIN_MODEL", openAIImagesResponsesMainModel)
	for _, tc := range []struct {
		name, model, wantUpstream string
		mapping                   map[string]any
	}{
		{"image_alias_and_wildcard", "paint", openAIImagesResponsesMainModel, map[string]any{"*": "gpt-image-2"}},
		{"native_and_mapped_driver", "gpt-image-2.5-flare", openAIImagesResponsesMainModel, map[string]any{openAIImagesResponsesMainModel: "gpt-image-2"}},
		{"ordinary_text_mapping", "chat", "gpt-6-luna", map[string]any{"chat": "gpt-6-luna"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			upstream := &httpUpstreamRecorder{resp: &http.Response{
				StatusCode: http.StatusOK,
				Header:     http.Header{"Content-Type": []string{"text/event-stream"}},
				Body:       io.NopCloser(strings.NewReader("data: " + `{"type":"response.completed","response":{"id":"resp_planner","status":"completed","output":[],"usage":{"input_tokens":1,"output_tokens":1}}}` + "\n\n")),
			}}
			svc := &OpenAIGatewayService{cfg: &config.Config{}, httpUpstream: upstream}
			a := webImagePolicyAccount(tc.mapping)
			a.ID, a.Concurrency = 270, 1
			a.Credentials["access_token"] = "test-oauth-token"
			a.Extra[featureKeyCodexImageGenerationBridge] = true
			body, err := json.Marshal(map[string]any{"model": tc.model, "input": "draw a cat"})
			require.NoError(t, err)
			recorder := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(recorder)
			c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", nil)
			c.Request.Header.Set("User-Agent", "codex_cli_rs/0.9.0")
			SetOpenAIClientTransport(c, OpenAIClientTransportHTTP)
			result, err := svc.Forward(context.Background(), c, a, body)
			require.NoError(t, err)
			require.NotNil(t, result)
			require.Len(t, upstream.requests, 1)
			require.Equal(t, tc.wantUpstream, gjson.GetBytes(upstream.lastBody, "model").String())
			require.False(t, openAIRequestBodyHasImageGenerationDeclaration(upstream.lastBody), string(upstream.lastBody))
			require.Contains(t, string(upstream.lastBody), webImageFunctionName)
		})
	}
}
