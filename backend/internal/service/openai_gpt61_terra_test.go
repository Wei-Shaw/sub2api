//go:build unit

package service

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/pkg/apicompat"
	"github.com/Wei-Shaw/sub2api/internal/pkg/openai"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestGPT61TerraIdentityAndEffort(t *testing.T) {
	require.Contains(t, openai.DefaultModelIDs(), "gpt-6.1-terra")
	require.Equal(t, "gpt-5.6-sol", openai.DefaultModelIDs()[0])
	require.Equal(t, "gpt-5.4", openai.DefaultTestModel)
	for _, effort := range []string{"none", "low", "medium", "high", "xhigh", "max"} {
		model := "openai/GPT-6.1-Terra-" + effort
		require.True(t, openai.IsGPT61TerraModelSpelling(model))
		require.False(t, openai.IsGPT61SolModelSpelling(model))
		require.Equal(t, "gpt-6.1-terra", normalizeCodexModel(model))
		require.Equal(t, "gpt-6.1-terra", normalizeKnownOpenAICodexModel(model))
		require.Equal(t, "gpt-6.1-terra", NormalizeOpenAICompatRequestedModel(model))
		require.Equal(t, effort, normalizeOpenAIReasoningEffortForModel(effort, model))
		request := &apicompat.AnthropicRequest{Model: model}
		applyOpenAICompatModelNormalization(request)
		require.Equal(t, "gpt-6.1-terra", request.Model)
		require.Equal(t, effort, request.OutputConfig.Effort)
		require.Equal(t, effort, openAICompatAnthropicReasoningEffort(request, request.Model, effort))
		converted, err := apicompat.AnthropicToResponses(request)
		require.NoError(t, err)
		require.Equal(t, effort, converted.Reasoning.Effort)
		chatConverted, err := apicompat.ChatCompletionsToResponses(&apicompat.ChatCompletionsRequest{
			Model: "gpt-6.1-terra", ReasoningEffort: effort,
			Tools: []apicompat.ChatTool{{Type: "function", Function: &apicompat.ChatFunction{Name: "lookup"}}},
		})
		require.NoError(t, err)
		require.Equal(t, "gpt-6.1-terra", chatConverted.Model)
		require.Equal(t, effort, chatConverted.Reasoning.Effort)
		require.Len(t, chatConverted.Tools, 1)
		require.NoError(t, validateGPT61CompatRequest([]byte(`{"reasoning":{"effort":"`+effort+`"}}`), model))
	}
	for _, model := range []string{"gpt-6.1-terra", "gpt-6.1-terra-openai-compact", "openai/gpt_6.1_terra"} {
		require.Equal(t, "gpt-6.1-terra", normalizeKnownOpenAICodexModel(model))
	}
	for _, model := range []string{"gpt-6.1-sol", "gpt-5.6-terra", "gpt-6.1-terra-pro", "gpt-6.1-terra-ultra", "gpt-6.1-terra-preview"} {
		require.False(t, openai.IsGPT61TerraModelSpelling(model))
	}
	// Codex publishes a separate scale from the API model page.
	descriptor := newConfiguredCodexModelDescriptor("gpt-6.1-terra")
	require.Equal(t, "low", *descriptor.DefaultReasoningLevel)
	var efforts []string
	for _, level := range descriptor.SupportedReasoningLevels {
		efforts = append(efforts, level.Effort)
	}
	require.Equal(t, []string{"low", "medium", "high", "xhigh", "max", "ultra"}, efforts)
	require.Equal(t, openai.CodexGPT61TerraMetadata, []byte(descriptor.officialMetadata))
	converted, err := apicompat.AnthropicToResponses(&apicompat.AnthropicRequest{Model: "gpt-6.1-terra"})
	require.NoError(t, err)
	require.Equal(t, "medium", converted.Reasoning.Effort)
}

func TestGPT61TerraAccountDiscoveryPreservesCatalog(t *testing.T) {
	for _, catalog := range []string{`{"data":[{"id":"gpt-6.1-terra","owned_by":"openai"}]}`, `{"data":[]}`} {
		gateway := newCodexModelsAPIKeyTestService(&codexModelsHTTPUpstreamStub{do: func(_ *http.Request, _ string, _ int64, _ int) (*http.Response, error) {
			return ordinaryModelsUpstreamResponse(catalog), nil
		}})
		service := &AccountTestService{openaiGatewayService: gateway}
		account := newCodexModelsAPIKeyTestAccount("https://models.example/v1")
		before, err := gateway.FetchOpenAIModelsList(context.Background(), account)
		require.NoError(t, err)
		models, err := service.FetchOpenAIAccountModels(context.Background(), account)
		require.NoError(t, err)
		if catalog == `{"data":[]}` {
			require.Empty(t, models)
		} else {
			require.Len(t, models, 1)
			require.Equal(t, "gpt-6.1-terra", models[0].ID)
			require.Equal(t, "GPT-6.1 Terra", models[0].DisplayName)
		}
		after, err := gateway.FetchOpenAIModelsList(context.Background(), account)
		require.NoError(t, err)
		require.Equal(t, before.Body, after.Body)
	}
}

func TestGPT61TerraManifestUsesUpstreamCapabilities(t *testing.T) {
	account := Account{ID: 1, Platform: PlatformOpenAI, Type: AccountTypeAPIKey}
	reasoning := true
	account.SetUpstreamModelMetadataSnapshot(UpstreamModelMetadataSnapshot{Models: map[string]UpstreamModelMetadata{
		"gpt-6.1-terra": {ID: "gpt-6.1-terra", Reasoning: &reasoning, DefaultReasoningLevel: "high", SupportedReasoningLevels: []string{"high"}, InputModalities: []string{"text"}, ContextWindow: 100000},
	}})
	service := &GatewayService{accountRepo: codexModelsVisibilityAccountRepo{byGroup: map[int64][]Account{1: {account}}}}
	body, err := service.BuildCodexModelsManifestForGroup(context.Background(), &Group{ID: 1, Platform: PlatformOpenAI}, "", []string{"gpt-6.1-terra"})
	require.NoError(t, err)
	models := decodeCodexManifestModels(t, body)
	require.Len(t, models, 1)
	require.Equal(t, []any{"text"}, models[0]["input_modalities"])
	require.Equal(t, []string{"high"}, effortsFromManifestModel(t, models[0]))
	require.Equal(t, float64(100000), models[0]["context_window"])
}

func TestGPT61TerraChatFallbackRejectsTools(t *testing.T) {
	for _, protocol := range []string{"chat", "responses", "messages"} {
		for _, toolField := range []string{`"tools":[{"type":"function","function":{"name":"lookup","parameters":{"type":"object"}}}]`, `"functions":[{"name":"lookup","parameters":{"type":"object"}}]`} {
			if protocol != "chat" && strings.HasPrefix(toolField, `"functions"`) {
				continue
			}
			body := `{"model":"public","reasoning_effort":"none","messages":[{"role":"user","content":"hi"}],"input":"hi","max_tokens":100,` + toolField + `}`
			if protocol == "responses" {
				body = `{"model":"public","input":"hi","tools":[{"type":"function","name":"lookup","parameters":{"type":"object"}}]}`
			}
			if protocol == "messages" {
				body = `{"model":"public","max_tokens":100,"messages":[{"role":"user","content":"hi"}],"tools":[{"name":"lookup","input_schema":{"type":"object"}}]}`
			}
			t.Run(protocol+toolField, func(t *testing.T) {
				recorder := httptest.NewRecorder()
				ginContext, _ := gin.CreateTestContext(recorder)
				ginContext.Request = httptest.NewRequest(http.MethodPost, "/v1/"+protocol, strings.NewReader(body))
				account := &Account{ID: 1, Platform: PlatformOpenAI, Type: AccountTypeAPIKey, Credentials: map[string]any{"model_mapping": map[string]any{"public": "gpt-6.1-terra"}}}
				service := &OpenAIGatewayService{cfg: &config.Config{}}
				var err error
				switch protocol {
				case "chat":
					_, err = service.forwardAsRawChatCompletions(context.Background(), ginContext, account, []byte(body), "")
				case "responses":
					_, err = service.forwardResponsesViaRawChatCompletions(context.Background(), ginContext, account, []byte(body), time.Now())
				case "messages":
					_, err = service.forwardAnthropicViaRawChatCompletions(context.Background(), ginContext, account, []byte(body), "")
				}
				require.ErrorContains(t, err, "gpt-6.1-terra requires Responses")
				require.Equal(t, http.StatusBadRequest, recorder.Code)
			})
		}
	}
}

func TestGPT61TerraRejectsMinimalEffort(t *testing.T) {
	require.True(t, openai.IsGPT61TerraModelSpelling("openai/GPT-6.1-Terra-minimal"))
	require.Equal(t, "gpt-6.1-terra", normalizeKnownOpenAICodexModel("gpt-6.1-terra-minimal"))
	for _, body := range []string{
		`{"reasoning":{"effort":"minimal"}}`,
		`{"reasoning_effort":"MINIMAL"}`,
		`{"output_config":{"effort":"minimal"}}`,
		`{"model":"gpt-6.1-terra-minimal"}`,
	} {
		require.ErrorContains(t, validateGPT61CompatRequest([]byte(body), "gpt-6.1-terra"), "gpt-6.1-terra does not support reasoning effort", body)
	}
	for _, body := range []string{`{"model":"gpt-6.1-terra-none"}`, `{"thinking":{"type":"disabled"}}`, `{"reasoning":{"effort":"none"}}`} {
		require.NoError(t, validateGPT61CompatRequest([]byte(body), "gpt-6.1-terra"), body)
	}
	_, err := apicompat.ChatCompletionsToResponses(&apicompat.ChatCompletionsRequest{Model: "gpt-6.1-terra", ReasoningEffort: "minimal"})
	require.ErrorContains(t, err, "gpt-6.1-terra does not support reasoning effort")
	// Sol keeps its stricter scale.
	require.Error(t, validateGPT61CompatRequest([]byte(`{"reasoning":{"effort":"none"}}`), "gpt-6.1-sol"))
}

func TestGPT61TerraOfflineCodexCatalog(t *testing.T) {
	body, err := BuildCodexModelsManifest([]string{"gpt-6.1-terra"})
	require.NoError(t, err)
	models := decodeCodexManifestModels(t, body)
	require.Len(t, models, 1)
	m := models[0]
	require.Equal(t, "gpt-6.1-terra", m["slug"])
	require.Equal(t, "GPT-6.1 Terra", m["display_name"])
	require.Equal(t, "low", m["default_reasoning_level"])
	require.Equal(t, []string{"low", "medium", "high", "xhigh", "max", "ultra"}, effortsFromManifestModel(t, m))
	require.EqualValues(t, 500000, m["context_window"])
	require.EqualValues(t, 872000, m["max_context_window"])
	require.Equal(t, "xhigh", m["multi_agent_reasoning_effort"])
	require.Equal(t, "v2", m["multi_agent_version"])
	require.Equal(t, []any{map[string]any{"id": "priority", "name": "Fast", "description": "1.5x speed"}}, m["service_tiers"])
	messages, ok := m["model_messages"].(map[string]any)
	require.True(t, ok)
	require.Equal(t, openai.CodexBaseInstructionsForModel("gpt-6.1-terra"), messages["instructions_template"])
	require.NotEmpty(t, messages["persistent_instructions"])
}

func TestGPT61TerraBaseInstructionsUseOfficialTemplate(t *testing.T) {
	var descriptor struct {
		ModelMessages struct {
			InstructionsTemplate string `json:"instructions_template"`
		} `json:"model_messages"`
	}
	require.NoError(t, json.Unmarshal(openai.CodexGPT61TerraMetadata, &descriptor))
	require.NotEmpty(t, descriptor.ModelMessages.InstructionsTemplate)
	for _, model := range []string{"gpt-6.1-terra", "openai/GPT-6.1-Terra-high", "gpt-6.1-terra-openai-compact"} {
		require.Equal(t, descriptor.ModelMessages.InstructionsTemplate, openai.CodexBaseInstructionsForModel(model), model)
	}
	require.NotEqual(t, openai.CodexBaseInstructionsForModel("gpt-5.5"), openai.CodexBaseInstructionsForModel("gpt-6.1-terra"))
	require.NotEqual(t, openai.CodexBaseInstructionsForModel("gpt-6.1-sol"), openai.CodexBaseInstructionsForModel("gpt-6.1-terra"))
}

func TestGPT61TerraCodexCapabilitiesFollowRoute(t *testing.T) {
	for _, tc := range []struct {
		accountType, base string
		toolMode, lite    string
	}{
		{AccountTypeAPIKey, "https://api.openai.com", "null", "false"},
		{AccountTypeOAuth, "https://chatgpt.com", `"code_mode_only"`, "true"},
	} {
		account := &Account{Platform: PlatformOpenAI, Type: tc.accountType, Credentials: map[string]any{"base_url": tc.base}}
		caps := accountCodexToolCapabilities(account, "gpt-6.1-terra")
		require.JSONEq(t, tc.toolMode, string(caps["tool_mode"]), tc.accountType)
		require.JSONEq(t, tc.lite, string(caps["use_responses_lite"]), tc.accountType)
		require.JSONEq(t, "true", string(caps["supports_search_tool"]), tc.accountType)
		require.False(t, bytes.Contains(caps["service_tiers"], []byte("ultrafast")), tc.accountType)
	}

	apiKey := &Account{Platform: PlatformOpenAI, Type: AccountTypeAPIKey, Credentials: map[string]any{"base_url": "https://api.openai.com"}}
	body, err := completeAPIKeyCodexModelsManifestMetadata([]byte(`{"models":[{"slug":"gpt-6.1-terra"}]}`), true, apiKey)
	require.NoError(t, err)
	models := decodeCodexManifestModels(t, body)
	require.Len(t, models, 1)
	require.Equal(t, false, models[0]["use_responses_lite"])
	require.Equal(t, "low", models[0]["default_reasoning_level"])
	require.EqualValues(t, 500000, models[0]["context_window"])

	adjusted, err := adjustAPIKeyCodexModelsManifest([]byte(`{"models":[{"slug":"gpt-6.1-terra","use_responses_lite":true}]}`), apiKey)
	require.NoError(t, err)
	require.JSONEq(t, `{"models":[{"slug":"gpt-6.1-terra","use_responses_lite":false}]}`, string(adjusted))
}

func TestGPT61TerraCompatPromptCacheKey(t *testing.T) {
	for _, model := range []string{"gpt-6.1-terra", "openai/GPT-6.1-Terra-max", "gpt-6.1-terra-none"} {
		require.True(t, shouldAutoInjectPromptCacheKeyForCompat(model), model)
	}
	require.False(t, shouldAutoInjectPromptCacheKeyForCompat("gpt-6.1-terra-preview"))
	req := &apicompat.ChatCompletionsRequest{Model: "gpt-6.1-terra", Messages: []apicompat.ChatMessage{{Role: "user", Content: json.RawMessage(`"hi"`)}}}
	require.NotEqual(t, deriveCompatPromptCacheKey(req, "gpt-6.1-terra"), deriveCompatPromptCacheKey(req, "gpt-6.1-sol"))
}
