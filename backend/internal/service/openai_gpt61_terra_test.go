//go:build unit

package service

import (
	"context"
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
		require.NoError(t, validateGPT61SolCompatRequest([]byte(`{"reasoning":{"effort":"`+effort+`"}}`), model))
	}
	for _, model := range []string{"gpt-6.1-terra", "gpt-6.1-terra-openai-compact", "openai/gpt_6.1_terra"} {
		require.Equal(t, "gpt-6.1-terra", normalizeKnownOpenAICodexModel(model))
	}
	for _, model := range []string{"gpt-6.1-sol", "gpt-5.6-terra", "gpt-6.1-terra-pro", "gpt-6.1-terra-ultra", "gpt-6.1-terra-minimal"} {
		require.False(t, openai.IsGPT61TerraModelSpelling(model))
	}
	descriptor := newConfiguredCodexModelDescriptor("gpt-6.1-terra")
	require.Equal(t, "medium", *descriptor.DefaultReasoningLevel)
	var efforts []string
	for _, level := range descriptor.SupportedReasoningLevels {
		efforts = append(efforts, level.Effort)
	}
	require.Equal(t, []string{"none", "low", "medium", "high", "xhigh", "max"}, efforts)
	require.Empty(t, descriptor.officialMetadata)
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
