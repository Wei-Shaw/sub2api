# Gemini OpenAI-Compatible Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add a Gemini-only OpenAI-compatible `/v1beta/openai` surface covering models, chat completions with audio input, embeddings, image generations, explicit unsupported endpoints, and frontend key-usage guidance.

**Architecture:** Register a dedicated `/v1beta/openai` route group that uses Google-style API key extraction but enforces `gemini` group platform before dispatch. Chat completions reuses the existing `GatewayHandler.ChatCompletions` and `GeminiMessagesCompatService.ForwardAsChatCompletions` path; audio is preserved through the existing Chat Completions to Responses to Anthropic to Gemini bridge. Embeddings and image generation use new Gemini-native service methods that emit OpenAI-compatible responses while preserving sub2api account selection, billing, usage recording, and failover loops in `GatewayHandler`.

**Tech Stack:** Go, Gin, existing sub2api handler/service layers, Gemini native REST `generateContent`, `embedContent`, `batchEmbedContents`, Vue 3, Vitest.

---

## Reference Checks

- Google Gemini OpenAI compatibility documents `base_url="https://generativelanguage.googleapis.com/v1beta/openai/"`, `POST /v1beta/openai/chat/completions`, `POST /v1beta/openai/embeddings`, audio understanding through Chat Completions, `GET /v1beta/openai/models`, and video endpoints.
- Google Gemini native embeddings uses `POST /v1beta/{model=models/*}:embedContent` and `POST /v1beta/{model=models/*}:batchEmbedContents`.
- Google Gemini image generation uses `POST /v1beta/models/gemini-2.5-flash-image:generateContent` and returns image bytes in response parts `inlineData`.

## File Structure

- Modify `backend/internal/server/routes/gateway.go`
  - Add `/v1beta/openai` route group.
  - Add `requireGeminiOpenAICompatibleGroup` route middleware that rejects non-Gemini groups with OpenAI-style JSON before any `/v1beta/openai` handler runs, including chat completions.
  - Add route registration tests by inspecting Gin route metadata.
- Create `backend/internal/handler/gemini_openai_compatible_handler.go`
  - Model list and model retrieve handlers.
  - OpenAI-style unsupported endpoint response helper.
  - Gemini OpenAI embeddings and image handlers that reuse shared Gemini account-selection and usage-recording helpers.
- Create `backend/internal/handler/gemini_openai_compatible_handler_test.go`
  - Unit tests for model conversion, platform guard, unsupported errors, embeddings/image handler validation.
- Modify `backend/internal/pkg/apicompat/types.go`
  - Add `ChatInputAudio` and `ResponsesInputAudio`.
  - Add `InputAudio` to `ChatContentPart` and `ResponsesContentPart`.
- Modify `backend/internal/pkg/apicompat/chatcompletions_to_responses.go`
  - Preserve `input_audio` parts and validate data/format.
- Modify `backend/internal/pkg/apicompat/responses_to_anthropic_request.go`
  - Preserve internal `input_audio` parts as Anthropic-style content blocks with `source`.
- Add or modify `backend/internal/pkg/apicompat/chatcompletions_to_responses_test.go`
  - Coverage for preserving audio and rejecting invalid audio.
- Add or modify `backend/internal/pkg/apicompat/responses_to_anthropic_request_test.go`
  - Coverage for converting Responses audio into Anthropic blocks.
- Modify `backend/internal/service/gemini_messages_compat_service.go`
  - Convert audio blocks into Gemini `inlineData`.
- Modify `backend/internal/service/gemini_messages_compat_service_test.go`
  - Full-chain audio test through `ForwardAsChatCompletions`.
- Create `backend/internal/service/gemini_openai_embeddings.go`
  - Gemini-native embeddings request conversion, upstream call, OpenAI-compatible response conversion, usage extraction.
- Create `backend/internal/service/gemini_openai_embeddings_test.go`
  - Unit tests for single and batch embeddings conversion.
- Create `backend/internal/service/gemini_openai_images.go`
  - Gemini-native image-generation request conversion, upstream call, OpenAI-compatible response conversion, usage and image billing fields.
- Create `backend/internal/service/gemini_openai_images_test.go`
  - Unit tests for generated image response and validation failures.
- Modify `backend/internal/handler/endpoint.go` and `backend/internal/handler/endpoint_test.go`
  - Normalize `/v1beta/openai/embeddings` and `/v1beta/openai/images/generations` to the existing endpoint classes for ops logging.
- Modify `frontend/src/components/keys/UseKeyModal.vue`
  - Add Gemini OpenAI-compatible tab.
  - Correct native Gemini CLI base URL to `/v1beta`.
  - Add OpenAI SDK Python example and env var snippets for `/v1beta/openai`.
- Modify `frontend/src/components/keys/__tests__/UseKeyModal.spec.ts`
  - Test native Gemini CLI `/v1beta` and OpenAI-compatible Gemini `/v1beta/openai`.
- Modify `frontend/src/i18n/locales/zh.ts` and `frontend/src/i18n/locales/en.ts`
  - Add tab label and copy for OpenAI-compatible Gemini guidance.

---

### Task 1: Route Group, Platform Guard, Models, Unsupported Errors

**Files:**
- Modify: `backend/internal/server/routes/gateway.go`
- Create: `backend/internal/handler/gemini_openai_compatible_handler.go`
- Create: `backend/internal/handler/gemini_openai_compatible_handler_test.go`
- Test: `backend/internal/server/routes/gateway_test.go`

- [ ] **Step 1: Write route registration tests**

Add this test to `backend/internal/server/routes/gateway_test.go`:

```go
func TestGatewayRoutesGeminiOpenAICompatiblePathsAreRegistered(t *testing.T) {
	router := newGatewayRoutesTestRouter()

	routes := map[string]bool{}
	for _, route := range router.Routes() {
		routes[route.Method+" "+route.Path] = true
	}

	required := []string{
		http.MethodGet + " /v1beta/openai/models",
		http.MethodGet + " /v1beta/openai/models/:model",
		http.MethodPost + " /v1beta/openai/chat/completions",
		http.MethodPost + " /v1beta/openai/embeddings",
		http.MethodPost + " /v1beta/openai/images/generations",
		http.MethodPost + " /v1beta/openai/videos",
		http.MethodGet + " /v1beta/openai/videos/:id",
	}
	for _, key := range required {
		require.True(t, routes[key], "route %s should be registered", key)
	}
}
```

Also add a middleware unit test in the same file:

```go
func TestRequireGeminiOpenAICompatibleGroupRejectsNonGemini(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.Use(func(c *gin.Context) {
		groupID := int64(1)
		c.Set(string(servermiddleware.ContextKeyAPIKey), &service.APIKey{
			GroupID: &groupID,
			Group:   &service.Group{Platform: service.PlatformOpenAI},
		})
		c.Next()
	})
	router.Use(requireGeminiOpenAICompatibleGroup)
	router.POST("/v1beta/openai/chat/completions", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"ok": true})
	})

	req := httptest.NewRequest(http.MethodPost, "/v1beta/openai/chat/completions", strings.NewReader(`{"model":"gpt-test"}`))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	require.Equal(t, http.StatusBadRequest, w.Code)
	require.Contains(t, w.Body.String(), "requires a Gemini group")
}
```

- [ ] **Step 2: Write handler tests for model conversion and platform guard**

Create `backend/internal/handler/gemini_openai_compatible_handler_test.go`:

```go
package handler

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/pkg/gemini"
	servermiddleware "github.com/Wei-Shaw/sub2api/internal/server/middleware"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestGeminiOpenAICompatibleModelsUsesOpenAIShape(t *testing.T) {
	got := geminiModelsToOpenAIModelList(gemini.FallbackModelsList())

	require.Equal(t, "list", got.Object)
	require.NotEmpty(t, got.Data)
	require.Equal(t, "model", got.Data[0].Object)
	require.Equal(t, "google", got.Data[0].OwnedBy)
	require.NotContains(t, got.Data[0].ID, "models/")
}

func TestGeminiOpenAICompatibleRejectsNonGeminiGroup(t *testing.T) {
	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodGet, "/v1beta/openai/models", nil)
	groupID := int64(1)
	c.Set(string(servermiddleware.ContextKeyAPIKey), &service.APIKey{
		GroupID: &groupID,
		Group:   &service.Group{Platform: service.PlatformOpenAI},
	})

	ok := ensureGeminiOpenAICompatibleGroup(c)

	require.False(t, ok)
	require.Equal(t, http.StatusBadRequest, w.Code)
	require.Contains(t, w.Body.String(), "requires a Gemini group")
}

func TestGeminiOpenAICompatibleUnsupportedUsesOpenAIErrorShape(t *testing.T) {
	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1beta/openai/videos", nil)

	(&GatewayHandler{}).GeminiOpenAICompatibleUnsupported(c)

	require.Equal(t, http.StatusNotFound, w.Code)
	require.JSONEq(t, `{"error":{"type":"invalid_request_error","message":"Unsupported endpoint for Gemini OpenAI compatibility"}}`, w.Body.String())
}
```

- [ ] **Step 3: Run tests and verify they fail**

Run:

```bash
cd backend && go test ./internal/server/routes ./internal/handler -run 'GeminiOpenAICompatible|GatewayRoutesGeminiOpenAI' -count=1
```

Expected: FAIL with missing route paths and missing symbols such as `geminiModelsToOpenAIModelList`.

- [ ] **Step 4: Implement handlers and route group**

In `backend/internal/handler/gemini_openai_compatible_handler.go`, add:

```go
package handler

import (
	"net/http"
	"strings"

	"github.com/Wei-Shaw/sub2api/internal/pkg/gemini"
	servermiddleware "github.com/Wei-Shaw/sub2api/internal/server/middleware"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
)

type openAICompatModelList struct {
	Object string                    `json:"object"`
	Data   []openAICompatModelObject `json:"data"`
}

type openAICompatModelObject struct {
	ID      string `json:"id"`
	Object  string `json:"object"`
	Created int64  `json:"created"`
	OwnedBy string `json:"owned_by"`
}

func ensureGeminiOpenAICompatibleGroup(c *gin.Context) bool {
	apiKey, ok := servermiddleware.GetAPIKeyFromContext(c)
	if !ok || apiKey == nil || apiKey.Group == nil || apiKey.Group.Platform != service.PlatformGemini {
		geminiOpenAICompatError(c, http.StatusBadRequest, "invalid_request_error", "The /v1beta/openai compatibility endpoint requires a Gemini group")
		return false
	}
	return true
}

func geminiOpenAICompatError(c *gin.Context, status int, errType string, message string) {
	c.JSON(status, gin.H{
		"error": gin.H{
			"type":    errType,
			"message": message,
		},
	})
	c.Abort()
}

func geminiModelNameToOpenAIID(name string) string {
	name = strings.TrimSpace(name)
	name = strings.TrimPrefix(name, "models/")
	return name
}

func geminiModelsToOpenAIModelList(src gemini.ModelsListResponse) openAICompatModelList {
	out := openAICompatModelList{Object: "list", Data: make([]openAICompatModelObject, 0, len(src.Models))}
	for _, model := range src.Models {
		id := geminiModelNameToOpenAIID(model.Name)
		if id == "" {
			continue
		}
		out.Data = append(out.Data, openAICompatModelObject{
			ID:      id,
			Object:  "model",
			Created: 0,
			OwnedBy: "google",
		})
	}
	return out
}

func geminiModelToOpenAIModelObject(model string) openAICompatModelObject {
	return openAICompatModelObject{
		ID:      geminiModelNameToOpenAIID(model),
		Object:  "model",
		Created: 0,
		OwnedBy: "google",
	}
}

func (h *GatewayHandler) GeminiOpenAICompatibleModels(c *gin.Context) {
	if !ensureGeminiOpenAICompatibleGroup(c) {
		return
	}
	c.JSON(http.StatusOK, geminiModelsToOpenAIModelList(gemini.FallbackModelsList()))
}

func (h *GatewayHandler) GeminiOpenAICompatibleGetModel(c *gin.Context) {
	if !ensureGeminiOpenAICompatibleGroup(c) {
		return
	}
	model := strings.TrimSpace(c.Param("model"))
	if model == "" {
		geminiOpenAICompatError(c, http.StatusBadRequest, "invalid_request_error", "model is required")
		return
	}
	c.JSON(http.StatusOK, geminiModelToOpenAIModelObject(model))
}

func (h *GatewayHandler) GeminiOpenAICompatibleUnsupported(c *gin.Context) {
	geminiOpenAICompatError(c, http.StatusNotFound, "invalid_request_error", "Unsupported endpoint for Gemini OpenAI compatibility")
}
```

In `backend/internal/server/routes/gateway.go`, add this package-level helper near `getGroupPlatform`:

```go
func requireGeminiOpenAICompatibleGroup(c *gin.Context) {
	apiKey, ok := middleware.GetAPIKeyFromContext(c)
	if !ok || apiKey == nil || apiKey.Group == nil || apiKey.Group.Platform != service.PlatformGemini {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": gin.H{
				"type":    "invalid_request_error",
				"message": "The /v1beta/openai compatibility endpoint requires a Gemini group",
			},
		})
		c.Abort()
		return
	}
	c.Next()
}
```

Then add the group after the existing native `/v1beta` Gemini group:

```go

	geminiOpenAI := r.Group("/v1beta/openai")
	geminiOpenAI.Use(bodyLimit)
	geminiOpenAI.Use(clientRequestID)
	geminiOpenAI.Use(opsErrorLogger)
	geminiOpenAI.Use(endpointNorm)
	geminiOpenAI.Use(middleware.APIKeyAuthWithSubscriptionGoogle(apiKeyService, subscriptionService, cfg))
	geminiOpenAI.Use(requireGroupGoogle)
	geminiOpenAI.Use(requireGeminiOpenAICompatibleGroup)
	{
		geminiOpenAI.GET("/models", h.Gateway.GeminiOpenAICompatibleModels)
		geminiOpenAI.GET("/models/:model", h.Gateway.GeminiOpenAICompatibleGetModel)
		geminiOpenAI.POST("/chat/completions", h.Gateway.ChatCompletions)
		geminiOpenAI.POST("/embeddings", h.Gateway.GeminiOpenAICompatibleEmbeddings)
		geminiOpenAI.POST("/images/generations", h.Gateway.GeminiOpenAICompatibleImagesGenerations)
		geminiOpenAI.POST("/videos", h.Gateway.GeminiOpenAICompatibleUnsupported)
		geminiOpenAI.GET("/videos/:id", h.Gateway.GeminiOpenAICompatibleUnsupported)
	}
```

- [ ] **Step 5: Run tests and commit**

Run:

```bash
cd backend && go test ./internal/server/routes ./internal/handler -run 'GeminiOpenAICompatible|GatewayRoutesGeminiOpenAI' -count=1
```

Expected: PASS.

Commit:

```bash
git add backend/internal/server/routes/gateway.go backend/internal/server/routes/gateway_test.go backend/internal/handler/gemini_openai_compatible_handler.go backend/internal/handler/gemini_openai_compatible_handler_test.go
git commit -m "feat: add gemini openai compatibility routes"
```

---

### Task 2: Chat Completions `input_audio` Preservation

**Files:**
- Modify: `backend/internal/pkg/apicompat/types.go`
- Modify: `backend/internal/pkg/apicompat/chatcompletions_to_responses.go`
- Modify: `backend/internal/pkg/apicompat/responses_to_anthropic_request.go`
- Modify: `backend/internal/service/gemini_messages_compat_service.go`
- Test: `backend/internal/pkg/apicompat/*_test.go`
- Test: `backend/internal/service/gemini_messages_compat_service_test.go`

- [ ] **Step 1: Write failing apicompat tests**

Add tests that assert:

```go
func TestChatCompletionsToResponsesPreservesInputAudio(t *testing.T) {
	req := &ChatCompletionsRequest{
		Model: "gemini-3.5-flash",
		Messages: []ChatMessage{{
			Role:    "user",
			Content: json.RawMessage(`[{"type":"text","text":"Transcribe this."},{"type":"input_audio","input_audio":{"data":"UklGRg==","format":"wav"}}]`),
		}},
	}

	out, err := ChatCompletionsToResponses(req)
	require.NoError(t, err)

	var items []ResponsesInputItem
	require.NoError(t, json.Unmarshal(out.Input, &items))
	var parts []ResponsesContentPart
	require.NoError(t, json.Unmarshal(items[0].Content, &parts))
	require.Equal(t, "input_audio", parts[1].Type)
	require.Equal(t, "UklGRg==", parts[1].InputAudio.Data)
	require.Equal(t, "wav", parts[1].InputAudio.Format)
}

func TestChatCompletionsToResponsesRejectsUnsupportedInputAudioFormat(t *testing.T) {
	req := &ChatCompletionsRequest{
		Model: "gemini-3.5-flash",
		Messages: []ChatMessage{{
			Role:    "user",
			Content: json.RawMessage(`[{"type":"input_audio","input_audio":{"data":"abc","format":"bad"}}]`),
		}},
	}

	_, err := ChatCompletionsToResponses(req)
	require.ErrorContains(t, err, "unsupported input_audio format")
}
```

Add a Responses to Anthropic test:

```go
func TestResponsesToAnthropicRequestPreservesInputAudio(t *testing.T) {
	content, _ := json.Marshal([]ResponsesContentPart{{
		Type:       "input_audio",
		InputAudio: &ResponsesInputAudio{Data: "UklGRg==", Format: "wav"},
	}})
	input, _ := json.Marshal([]ResponsesInputItem{{Role: "user", Content: content}})

	out, err := ResponsesToAnthropicRequest(&ResponsesRequest{Model: "gemini-3.5-flash", Input: input})
	require.NoError(t, err)
	require.Len(t, out.Messages, 1)

	var blocks []AnthropicContentBlock
	require.NoError(t, json.Unmarshal(out.Messages[0].Content, &blocks))
	require.Equal(t, "input_audio", blocks[0].Type)
	require.NotNil(t, blocks[0].Source)
	require.Equal(t, "audio/wav", blocks[0].Source.MediaType)
	require.Equal(t, "UklGRg==", blocks[0].Source.Data)
}
```

- [ ] **Step 2: Write failing Gemini conversion tests**

Add to `backend/internal/service/gemini_messages_compat_service_test.go`:

```go
func TestConvertClaudeMessagesToGeminiGenerateContent_InputAudioToInlineData(t *testing.T) {
	claudeReq := map[string]any{
		"model":      "gemini-3.5-flash",
		"max_tokens": 128,
		"messages": []any{map[string]any{
			"role": "user",
			"content": []any{map[string]any{
				"type": "input_audio",
				"source": map[string]any{
					"type":       "base64",
					"media_type": "audio/wav",
					"data":       "UklGRg==",
				},
			}},
		}},
	}
	body, _ := json.Marshal(claudeReq)

	got, err := convertClaudeMessagesToGeminiGenerateContent(body)
	require.NoError(t, err)
	require.JSONEq(t, `{"contents":[{"role":"user","parts":[{"inlineData":{"mimeType":"audio/wav","data":"UklGRg=="}}]}],"generationConfig":{"maxOutputTokens":128}}`, string(got))
}
```

Add a full-chain test that sends Chat Completions `input_audio` through `ForwardAsChatCompletions` and inspects `httpStub.lastReq.Body` for `inlineData.mimeType == "audio/wav"`.

- [ ] **Step 3: Run tests and verify they fail**

Run:

```bash
cd backend && go test ./internal/pkg/apicompat ./internal/service -run 'InputAudio|Audio' -count=1
```

Expected: FAIL because audio fields and conversions do not exist.

- [ ] **Step 4: Implement audio types and conversion**

In `backend/internal/pkg/apicompat/types.go`, add:

```go
type ChatInputAudio struct {
	Data   string `json:"data"`
	Format string `json:"format"`
}

type ResponsesInputAudio struct {
	Data   string `json:"data"`
	Format string `json:"format"`
}
```

Extend `ChatContentPart`:

```go
	InputAudio *ChatInputAudio `json:"input_audio,omitempty"`
```

Extend `ResponsesContentPart`:

```go
	InputAudio *ResponsesInputAudio `json:"input_audio,omitempty"`
```

In `chatcompletions_to_responses.go`, change `marshalChatInputContent` and `convertChatContentPartsToResponses` so conversion can return validation errors:

```go
func OpenAIInputAudioFormatToMIMEType(format string) (string, bool) {
	switch strings.ToLower(strings.TrimSpace(format)) {
	case "wav":
		return "audio/wav", true
	case "mp3":
		return "audio/mpeg", true
	case "m4a":
		return "audio/mp4", true
	case "aac":
		return "audio/aac", true
	case "flac":
		return "audio/flac", true
	case "ogg":
		return "audio/ogg", true
	default:
		return "", false
	}
}
```

For `input_audio`, skip empty audio when other usable parts exist, return `input_audio.data is required` when the message has only empty audio, and return `unsupported input_audio format "<format>"` when the format is non-empty but unmapped.

In `responses_to_anthropic_request.go`, add:

```go
		case "input_audio":
			if p.InputAudio == nil {
				continue
			}
			data := strings.TrimSpace(p.InputAudio.Data)
			if data == "" {
				continue
			}
			mediaType, ok := OpenAIInputAudioFormatToMIMEType(p.InputAudio.Format)
			if !ok {
				return nil, fmt.Errorf("unsupported input_audio format %q", p.InputAudio.Format)
			}
			blocks = append(blocks, AnthropicContentBlock{
				Type: "input_audio",
				Source: &AnthropicImageSource{
					Type:      "base64",
					MediaType: mediaType,
					Data:      data,
				},
			})
```

In `convertClaudeMessagesToGeminiContents`, add an audio case alongside the image case:

```go
				case "input_audio", "audio":
					if src, ok := bm["source"].(map[string]any); ok {
						if srcType, _ := src["type"].(string); srcType == "base64" {
							mediaType, _ := src["media_type"].(string)
							data, _ := src["data"].(string)
							if strings.TrimSpace(mediaType) != "" && strings.TrimSpace(data) != "" {
								parts = append(parts, map[string]any{
									"inlineData": map[string]any{
										"mimeType": mediaType,
										"data":     data,
									},
								})
							}
						}
					}
```

- [ ] **Step 5: Run tests and commit**

Run:

```bash
cd backend && go test ./internal/pkg/apicompat ./internal/service -run 'InputAudio|Audio|GeminiForwardAsChatCompletions' -count=1
```

Expected: PASS.

Commit:

```bash
git add backend/internal/pkg/apicompat backend/internal/service/gemini_messages_compat_service.go backend/internal/service/gemini_messages_compat_service_test.go
git commit -m "feat: preserve gemini openai input audio"
```

---

### Task 3: Gemini OpenAI-Compatible Embeddings

**Files:**
- Create: `backend/internal/service/gemini_openai_embeddings.go`
- Create: `backend/internal/service/gemini_openai_embeddings_test.go`
- Modify: `backend/internal/handler/gemini_openai_compatible_handler.go`
- Test: `backend/internal/handler/gemini_openai_compatible_handler_test.go`

- [ ] **Step 1: Write service tests**

Create tests for:

```go
func TestGeminiForwardOpenAICompatibleEmbeddings_SingleInputUsesEmbedContent(t *testing.T)
func TestGeminiForwardOpenAICompatibleEmbeddings_BatchInputUsesBatchEmbedContents(t *testing.T)
func TestGeminiForwardOpenAICompatibleEmbeddings_RejectsTokenArrayInput(t *testing.T)
```

The single-input test should assert:
- upstream URL contains `/v1beta/models/gemini-embedding-2-preview:embedContent`
- upstream JSON contains `content.parts[0].text`
- client response contains `"object":"list"` and `data[0].object == "embedding"`
- returned `ForwardResult.Usage.InputTokens` equals upstream `usageMetadata.promptTokenCount`

- [ ] **Step 2: Run tests and verify they fail**

Run:

```bash
cd backend && go test ./internal/service -run 'OpenAICompatibleEmbeddings' -count=1
```

Expected: FAIL because `ForwardOpenAICompatibleEmbeddings` does not exist.

- [ ] **Step 3: Implement embeddings service**

Create `backend/internal/service/gemini_openai_embeddings.go` with these functions:

```go
type geminiOpenAIEmbeddingsRequest struct {
	Model string          `json:"model"`
	Input json.RawMessage `json:"input"`
}

func (s *GeminiMessagesCompatService) ForwardOpenAICompatibleEmbeddings(ctx context.Context, c *gin.Context, account *Account, body []byte) (*ForwardResult, error)
func parseGeminiOpenAIEmbeddingInputs(raw json.RawMessage) ([]string, error)
func buildGeminiEmbedContentRequest(input string) []byte
func buildGeminiBatchEmbedContentsRequest(model string, inputs []string) []byte
func geminiEmbeddingURL(baseURL string, model string, batch bool) string
func convertGeminiEmbeddingResponseToOpenAI(body []byte, model string, inputCount int) ([]byte, ClaudeUsage, error)
func writeGeminiOpenAIEmbeddingsError(c *gin.Context, statusCode int, errType string, message string)
```

Implementation rules:
- Accept `input` as a string or `[]string`.
- Reject token arrays and nested arrays with HTTP 400 `invalid_request_error`.
- For one input, call `:embedContent`.
- For multiple inputs, call `:batchEmbedContents`.
- Use `x-goog-api-key` for API key accounts and `Authorization: Bearer` for OAuth accounts.
- Use `account.GetGeminiBaseURL(geminicli.AIStudioBaseURL)`.
- Use `account.GetMappedModel` for API key and service account accounts.
- Convert `embedding.values` and `embeddings[].values` into OpenAI embeddings response data.
- Extract `usageMetadata.promptTokenCount` as input tokens when present.

- [ ] **Step 4: Add handler method and usage recording**

In `gemini_openai_compatible_handler.go`, add:

```go
func (h *GatewayHandler) GeminiOpenAICompatibleEmbeddings(c *gin.Context) {
	h.handleGeminiOpenAICompatibleUnary(c, geminiOpenAICompatibleUnaryOptions{
		Component:  "handler.gemini_openai.embeddings",
		Endpoint:   int16(service.RequestTypeSync),
		Forward: func(ctx context.Context, c *gin.Context, account *service.Account, body []byte) (*service.ForwardResult, error) {
			return h.geminiCompatService.ForwardOpenAICompatibleEmbeddings(ctx, c, account, body)
		},
	})
}
```

Add `handleGeminiOpenAICompatibleUnary` in the same file. It must:
- call `ensureGeminiOpenAICompatibleGroup`
- read and validate JSON body
- extract `model`
- set ops request and endpoint context
- resolve channel model mapping
- acquire user concurrency slot
- check billing eligibility
- select only Gemini accounts using `SelectAccountWithLoadAwareness`
- acquire account slot
- call the provided `Forward`
- record usage with `gatewayService.RecordUsage`
- fail over on `*service.UpstreamFailoverError`

- [ ] **Step 5: Run tests and commit**

Run:

```bash
cd backend && go test ./internal/service ./internal/handler -run 'OpenAICompatibleEmbeddings|GeminiOpenAICompatible' -count=1
```

Expected: PASS.

Commit:

```bash
git add backend/internal/service/gemini_openai_embeddings.go backend/internal/service/gemini_openai_embeddings_test.go backend/internal/handler/gemini_openai_compatible_handler.go backend/internal/handler/gemini_openai_compatible_handler_test.go
git commit -m "feat: add gemini openai embeddings"
```

---

### Task 4: Gemini OpenAI-Compatible Image Generations

**Files:**
- Create: `backend/internal/service/gemini_openai_images.go`
- Create: `backend/internal/service/gemini_openai_images_test.go`
- Modify: `backend/internal/handler/gemini_openai_compatible_handler.go`

- [ ] **Step 1: Write service tests**

Create tests for:

```go
func TestGeminiForwardOpenAICompatibleImagesGenerations_ReturnsB64JSON(t *testing.T)
func TestGeminiForwardOpenAICompatibleImagesGenerations_RejectsURLResponseFormat(t *testing.T)
func TestGeminiForwardOpenAICompatibleImagesGenerations_RejectsMultipleImages(t *testing.T)
func TestGeminiForwardOpenAICompatibleImagesGenerations_RequiresImageModel(t *testing.T)
```

The success test upstream response body:

```json
{
  "candidates": [
    {
      "content": {
        "parts": [
          {"text": "done"},
          {"inlineData": {"mimeType": "image/png", "data": "iVBORw0KGgo="}}
        ]
      }
    }
  ],
  "usageMetadata": {"promptTokenCount": 10, "candidatesTokenCount": 2}
}
```

Assert:
- upstream URL contains `/v1beta/models/gemini-2.5-flash-image:generateContent`
- upstream JSON contains `generationConfig.responseModalities` with `TEXT` and `IMAGE`
- downstream response contains `data[0].b64_json == "iVBORw0KGgo="`
- `ForwardResult.ImageCount == 1`

- [ ] **Step 2: Run tests and verify they fail**

Run:

```bash
cd backend && go test ./internal/service -run 'OpenAICompatibleImages' -count=1
```

Expected: FAIL because `ForwardOpenAICompatibleImagesGenerations` does not exist.

- [ ] **Step 3: Implement image generation service**

Create `backend/internal/service/gemini_openai_images.go` with these functions:

```go
type geminiOpenAIImageGenerationRequest struct {
	Model          string `json:"model"`
	Prompt         string `json:"prompt"`
	N              *int   `json:"n,omitempty"`
	Size           string `json:"size,omitempty"`
	ResponseFormat string `json:"response_format,omitempty"`
}

func (s *GeminiMessagesCompatService) ForwardOpenAICompatibleImagesGenerations(ctx context.Context, c *gin.Context, account *Account, body []byte) (*ForwardResult, error)
func parseGeminiOpenAIImageGenerationRequest(body []byte) (*geminiOpenAIImageGenerationRequest, error)
func buildGeminiImageGenerateContentRequest(prompt string, size string) []byte
func collectGeminiOpenAIImages(raw []byte) ([]string, string, ClaudeUsage, error)
func buildGeminiOpenAIImagesResponse(created int64, images []string, revisedPrompt string) []byte
func writeGeminiOpenAIImagesError(c *gin.Context, statusCode int, errType string, message string)
```

Implementation rules:
- Require non-empty `model` and `prompt`.
- Require `isImageGenerationModel(model)`.
- Allow `response_format` empty or `b64_json`.
- Reject `response_format == "url"` with HTTP 400 because sub2api has no URL storage path here.
- Allow `n` empty or `1`; reject `n > 1` with HTTP 400.
- Send native Gemini `generateContent` with `responseModalities: ["TEXT", "IMAGE"]`.
- Parse `inlineData.data` and `inline_data.data`.
- Return OpenAI Images response `{"created": <unix>, "data": [{"b64_json": "iVBORw0KGgo="}]}` in tests and the actual generated base64 value at runtime.
- Return a `ForwardResult` with `ImageCount`, `ImageSize`, `ImageInputSize`, and `Usage`.

- [ ] **Step 4: Add handler method**

In `gemini_openai_compatible_handler.go`, add:

```go
func (h *GatewayHandler) GeminiOpenAICompatibleImagesGenerations(c *gin.Context) {
	h.handleGeminiOpenAICompatibleUnary(c, geminiOpenAICompatibleUnaryOptions{
		Component: "handler.gemini_openai.images",
		Endpoint:  int16(service.RequestTypeSync),
		BeforeForward: func(c *gin.Context, apiKey *service.APIKey, subject servermiddleware.AuthSubject, model string, body []byte) bool {
			if !service.GroupAllowsImageGeneration(apiKey.Group) {
				geminiOpenAICompatError(c, http.StatusForbidden, "permission_error", service.ImageGenerationPermissionMessage())
				return false
			}
			reqLog := requestLogger(c, "handler.gemini_openai.images", zap.Int64("user_id", subject.UserID), zap.Int64("api_key_id", apiKey.ID), zap.Any("group_id", apiKey.GroupID), zap.String("model", model))
			if decision := h.checkContentModeration(c, reqLog, apiKey, subject, service.ContentModerationProtocolOpenAIImages, model, body); decision != nil && decision.Blocked {
				geminiOpenAICompatError(c, contentModerationStatus(decision), contentModerationErrorCode(decision), decision.Message)
				return false
			}
			return true
		},
		Forward: func(ctx context.Context, c *gin.Context, account *service.Account, body []byte) (*service.ForwardResult, error) {
			return h.geminiCompatService.ForwardOpenAICompatibleImagesGenerations(ctx, c, account, body)
		},
	})
}
```

- [ ] **Step 5: Run tests and commit**

Run:

```bash
cd backend && go test ./internal/service ./internal/handler -run 'OpenAICompatibleImages|GeminiOpenAICompatible' -count=1
```

Expected: PASS.

Commit:

```bash
git add backend/internal/service/gemini_openai_images.go backend/internal/service/gemini_openai_images_test.go backend/internal/handler/gemini_openai_compatible_handler.go backend/internal/handler/gemini_openai_compatible_handler_test.go
git commit -m "feat: add gemini openai image generations"
```

---

### Task 5: Endpoint Normalization and Ops Classification

**Files:**
- Modify: `backend/internal/handler/endpoint.go`
- Modify: `backend/internal/handler/endpoint_test.go`

- [ ] **Step 1: Write endpoint tests**

Add cases:

```go
{"/v1beta/openai/embeddings", EndpointEmbeddings},
{"/v1beta/openai/images/generations", EndpointImagesGenerations},
```

Also add platform normalization assertions that `service.PlatformGemini` keeps the same endpoint classes.

- [ ] **Step 2: Run tests and verify failure if normalization misses the new prefix**

Run:

```bash
cd backend && go test ./internal/handler -run 'Endpoint' -count=1
```

Expected: FAIL if current normalization does not classify new paths.

- [ ] **Step 3: Implement endpoint normalization**

Update `NormalizeInboundEndpoint` logic so path contains checks for `/embeddings` and `/images/generations` apply to `/v1beta/openai/*` as they already do for `/v1/*` and `/openai/v1/*`.

- [ ] **Step 4: Run tests and commit**

Run:

```bash
cd backend && go test ./internal/handler -run 'Endpoint' -count=1
```

Expected: PASS.

Commit:

```bash
git add backend/internal/handler/endpoint.go backend/internal/handler/endpoint_test.go
git commit -m "chore: classify gemini openai endpoints"
```

---

### Task 6: Frontend Use Key Modal

**Files:**
- Modify: `frontend/src/components/keys/UseKeyModal.vue`
- Modify: `frontend/src/components/keys/__tests__/UseKeyModal.spec.ts`
- Modify: `frontend/src/i18n/locales/zh.ts`
- Modify: `frontend/src/i18n/locales/en.ts`

- [ ] **Step 1: Write frontend tests**

Add tests:

```ts
it('renders native Gemini CLI config with v1beta base URL', () => {
  const wrapper = mount(UseKeyModal, {
    props: {
      show: true,
      apiKey: 'sk-gemini',
      baseUrl: 'https://example.com/v1',
      platform: 'gemini'
    },
    global: {
      stubs: {
        BaseDialog: { template: '<div><slot /><slot name="footer" /></div>' },
        Icon: { template: '<span />' }
      }
    }
  })

  const code = wrapper.find('pre code').text()
  expect(code).toContain('GOOGLE_GEMINI_BASE_URL="https://example.com/v1beta"')
  expect(code).toContain('GEMINI_API_KEY="sk-gemini"')
})

it('renders Gemini OpenAI-compatible config with v1beta openai base URL', async () => {
  const wrapper = mount(UseKeyModal, {
    props: {
      show: true,
      apiKey: 'sk-gemini',
      baseUrl: 'https://example.com/v1',
      platform: 'gemini'
    },
    global: {
      stubs: {
        BaseDialog: { template: '<div><slot /><slot name="footer" /></div>' },
        Icon: { template: '<span />' }
      }
    }
  })

  const tab = wrapper.findAll('button').find((button) =>
    button.text().includes('keys.useKeyModal.cliTabs.openaiCompatible')
  )
  expect(tab).toBeDefined()
  await tab!.trigger('click')
  await nextTick()

  const codeBlocks = wrapper.findAll('pre code').map((code) => code.text())
  expect(codeBlocks.join('\n')).toContain('OPENAI_BASE_URL="https://example.com/v1beta/openai"')
  expect(codeBlocks.join('\n')).toContain('base_url="https://example.com/v1beta/openai"')
})
```

- [ ] **Step 2: Run tests and verify they fail**

Run:

```bash
cd frontend && pnpm test:run src/components/keys/__tests__/UseKeyModal.spec.ts
```

Expected: FAIL because the tab and `/v1beta` base URL are not implemented.

- [ ] **Step 3: Implement UI changes**

In `UseKeyModal.vue`:
- Add Gemini tab:

```ts
{ id: 'openai-compatible', label: t('keys.useKeyModal.cliTabs.openaiCompatible'), icon: TerminalIcon }
```

- Compute:

```ts
const geminiOpenAIBase = `${baseRoot}/v1beta/openai`
```

- Change native Gemini current files to:

```ts
if (activeClientTab.value === 'openai-compatible') {
  return generateGeminiOpenAICompatibleFiles(geminiOpenAIBase, apiKey)
}
return [generateGeminiCliContent(geminiBase, apiKey)]
```

- Change Antigravity Gemini current files to:

```ts
return [generateGeminiCliContent(antigravityGeminiBase, apiKey)]
```

- Add:

```ts
function generateGeminiOpenAICompatibleFiles(baseUrl: string, apiKey: string): FileConfig[] {
  const model = 'gemini-3.5-flash'
  const shellPath = activeTab.value === 'cmd'
    ? 'Command Prompt'
    : activeTab.value === 'powershell'
      ? 'PowerShell'
      : 'Terminal'
  const shellContent = activeTab.value === 'cmd'
    ? `set OPENAI_BASE_URL=${baseUrl}
set OPENAI_API_KEY=${apiKey}
set OPENAI_MODEL=${model}`
    : activeTab.value === 'powershell'
      ? `$env:OPENAI_BASE_URL="${baseUrl}"
$env:OPENAI_API_KEY="${apiKey}"
$env:OPENAI_MODEL="${model}"`
      : `export OPENAI_BASE_URL="${baseUrl}"
export OPENAI_API_KEY="${apiKey}"
export OPENAI_MODEL="${model}"`

  const pythonContent = `from openai import OpenAI

client = OpenAI(
    api_key="${apiKey}",
    base_url="${baseUrl}",
)

response = client.chat.completions.create(
    model="${model}",
    messages=[{"role": "user", "content": "Hello"}],
)

print(response.choices[0].message.content)`

  return [
    { path: shellPath, content: shellContent },
    { path: 'openai_gemini.py', content: pythonContent, hint: t('keys.useKeyModal.gemini.openaiPythonHint') }
  ]
}
```

- [ ] **Step 4: Add i18n copy**

In both locale files add:

```ts
openaiCompatible: 'OpenAI Compatible'
```

In `zh.ts`, add `gemini.openaiPythonHint`:

```ts
openaiPythonHint: 'OpenAI SDK 示例，base_url 使用 Gemini OpenAI 兼容路径'
```

In `en.ts`, add:

```ts
openaiPythonHint: 'OpenAI SDK example using the Gemini OpenAI-compatible base URL'
```

- [ ] **Step 5: Run tests and commit**

Run:

```bash
cd frontend && pnpm test:run src/components/keys/__tests__/UseKeyModal.spec.ts
```

Expected: PASS.

Commit:

```bash
git add frontend/src/components/keys/UseKeyModal.vue frontend/src/components/keys/__tests__/UseKeyModal.spec.ts frontend/src/i18n/locales/zh.ts frontend/src/i18n/locales/en.ts
git commit -m "feat: show gemini openai compatible key usage"
```

---

### Task 7: Full Verification

**Files:**
- No implementation files unless tests expose a real defect.

- [ ] **Step 1: Run backend focused packages**

Run:

```bash
cd backend && go test ./internal/server/routes ./internal/handler ./internal/pkg/apicompat ./internal/service
```

Expected: PASS.

- [ ] **Step 2: Run frontend focused tests**

Run:

```bash
cd frontend && pnpm test:run src/components/keys/__tests__/UseKeyModal.spec.ts
```

Expected: PASS.

- [ ] **Step 3: Run formatting and diff checks**

Run:

```bash
gofmt -w backend/internal/server/routes/gateway.go backend/internal/handler/gemini_openai_compatible_handler.go backend/internal/pkg/apicompat/types.go backend/internal/pkg/apicompat/chatcompletions_to_responses.go backend/internal/pkg/apicompat/responses_to_anthropic_request.go backend/internal/service/gemini_messages_compat_service.go backend/internal/service/gemini_openai_embeddings.go backend/internal/service/gemini_openai_images.go
git diff --check
git status --short
```

Expected:
- `git diff --check` exits 0.
- `git status --short` shows only intentional files before the final commit, or clean after the task commits.

- [ ] **Step 4: Optional local smoke request**

If a local dev server is already running, issue:

```bash
curl -sS "$SUB2API_BASE_URL/v1beta/openai/models" \
  -H "Authorization: Bearer $SUB2API_GEMINI_KEY" | jq .
```

Expected: JSON object with `"object": "list"` and model IDs without `models/`.

- [ ] **Step 5: Final commit if verification caused follow-up fixes**

If verification required follow-up edits:

```bash
git add <changed-files>
git commit -m "fix: stabilize gemini openai compatibility"
```

If there were no follow-up edits, skip this commit.

---

## Self-Review

**Spec coverage:**
- `/v1beta/openai` Gemini-only routing: Task 1.
- OpenAI SDK chat completions base URL: Task 1 and Task 6.
- Audio `input_audio` to Gemini `inlineData`: Task 2.
- Models endpoint OpenAI shape: Task 1.
- Embeddings endpoint native Gemini conversion and OpenAI response: Task 3.
- Image generation endpoint native Gemini conversion and OpenAI response: Task 4.
- Explicit unsupported video endpoint: Task 1.
- Frontend Use Key modal with native Gemini plus OpenAI-compatible Gemini guidance: Task 6.

**Placeholder scan:** This plan contains concrete file paths, test names, commands, expected failures, expected passing states, and implementation contracts. It avoids deferred requirements.

**Type consistency:**
- `ChatInputAudio`, `ResponsesInputAudio`, `InputAudio`, and `OpenAIInputAudioFormatToMIMEType` are introduced before later tasks consume them.
- Route middleware and handler-level defensive group checks use the same error message and OpenAI-compatible error shape.
- Handler method names in routes match the handler task names.
- Service method names in handler tasks match service task names.
