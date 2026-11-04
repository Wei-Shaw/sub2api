//go:build unit

package service

import (
	"encoding/base64"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/pkg/tlsfingerprint"
	"github.com/gin-gonic/gin"
	"github.com/tidwall/gjson"

	"github.com/stretchr/testify/require"
)

func TestChatGPTWebPromptWithCanvasSize(t *testing.T) {
	prompt := chatGPTWebPromptWithCanvasSize("draw a test chart", "2048x1152")
	require.Contains(t, prompt, "2048×1152")
	require.Contains(t, prompt, "横向画幅")
	require.Contains(t, prompt, "2K 分辨率")
	require.Equal(t, "draw a test chart", chatGPTWebPromptWithCanvasSize("draw a test chart", "auto"))
	require.Equal(t, "draw a test chart", chatGPTWebPromptWithCanvasSize("draw a test chart", "invalid"))
}

func TestNormalizeOpenAIImageBase64RestoresRequiredPadding(t *testing.T) {
	require.Equal(t, "aGVsbG8=", normalizeOpenAIImageBase64("aGVsbG8="))
	require.Equal(t, "aGVsbG8=", normalizeOpenAIImageBase64("aGVsbG8"))
}

// 合成的非流式体必须能被既有 Responses 解析器当成真实上游输出读取。
func TestBuildChatGPTWebImagesCompletedBodyIsParsedByExistingPipeline(t *testing.T) {
	images := [][]byte{[]byte("first-image-bytes"), []byte("second-image-bytes")}
	body := buildChatGPTWebImagesCompletedBody("gpt-image-2", images)

	results, createdAt, _, _, _, err := collectOpenAIImagesFromResponsesBody(body)
	require.NoError(t, err)
	require.Greater(t, createdAt, int64(0))
	require.Len(t, results, 2)

	decoded, err := base64.StdEncoding.DecodeString(strings.TrimSpace(results[0].Result))
	require.NoError(t, err)
	require.Equal(t, "first-image-bytes", string(decoded))

	decoded, err = base64.StdEncoding.DecodeString(strings.TrimSpace(results[1].Result))
	require.NoError(t, err)
	require.Equal(t, "second-image-bytes", string(decoded))
}

// 流式体的事件顺序与上游一致：created -> output_item.done -> completed。
func TestBuildChatGPTWebImagesStreamBodyCarriesImageAndCompletion(t *testing.T) {
	body := string(buildChatGPTWebImagesStreamBody("gpt-image-2", [][]byte{[]byte("stream-image")}))
	require.Contains(t, body, "event: response.created")
	require.Contains(t, body, "event: response.output_item.done")
	require.Contains(t, body, "event: response.completed")

	results, createdAt, _, _, _, err := collectOpenAIImagesFromResponsesBody([]byte(body))
	require.NoError(t, err)
	require.Greater(t, createdAt, int64(0))
	require.Len(t, results, 1)
	decoded, err := base64.StdEncoding.DecodeString(strings.TrimSpace(results[0].Result))
	require.NoError(t, err)
	require.Equal(t, "stream-image", string(decoded))
}

func TestBuildChatGPTWebImagesStreamBodyPreservesSignedURL(t *testing.T) {
	const signedURL = "https://fixture.example/image.png?sig=fixture"
	body := buildChatGPTWebImagesStreamBodyFromResults("gpt-image-2", []string{signedURL})
	results, _, _, _, _, err := collectOpenAIImagesFromResponsesBody(body)
	require.NoError(t, err)
	require.Len(t, results, 1)
	require.Equal(t, signedURL, results[0].Result)

	payload := buildOpenAIImagesStreamCompletedPayload(
		"image_generation.completed",
		results[0],
		"url",
		0,
		nil,
	)
	require.Equal(t, signedURL, gjson.GetBytes(payload, "url").String())
	require.False(t, gjson.GetBytes(payload, "b64_json").Exists())
}

// 指针收集必须忽略用户输入附件，只接受工具产出的图片。
func TestChatGPTWebCollectImagePointersIgnoresInputAttachments(t *testing.T) {
	payload := map[string]any{
		"conversation_id": "conv-1",
		"message": map[string]any{
			"author": map[string]any{"role": "user"},
			"content": map[string]any{
				"content_type": "multimodal_text",
				"parts":        []any{map[string]any{"asset_pointer": "sediment://input_file"}},
			},
		},
	}
	conversationID := ""
	pointers := []string{}
	chatGPTWebCollectImagePointers(payload, &conversationID, &pointers)
	require.Equal(t, "conv-1", conversationID)
	require.Empty(t, pointers)

	payload["message"] = map[string]any{
		"author":   map[string]any{"role": "tool"},
		"metadata": map[string]any{"async_task_type": "image_gen"},
		"content": map[string]any{
			"content_type": "multimodal_text",
			"parts": []any{
				map[string]any{"asset_pointer": "file-service://file_result"},
				map[string]any{"asset_pointer": "file-service://file_result"},
			},
		},
	}
	chatGPTWebCollectImagePointers(payload, &conversationID, &pointers)
	require.Equal(t, []string{"file-service://file_result"}, pointers)

	payload["message"] = map[string]any{
		"author": map[string]any{"role": "assistant"},
		"content": map[string]any{
			"content_type": "multimodal_text",
			"parts":        []any{map[string]any{"asset_pointer": "file-service://assistant_result"}},
		},
	}
	chatGPTWebCollectImagePointers(payload, &conversationID, &pointers)
	require.Equal(t, []string{"file-service://file_result", "file-service://assistant_result"}, pointers)
}

type chatGPTWebNoopTransport struct{}

func (chatGPTWebNoopTransport) Do(*http.Request, string, int64, int) (*http.Response, error) {
	return nil, errors.New("unexpected upstream call")
}

func (chatGPTWebNoopTransport) DoWithTLS(*http.Request, string, int64, int, *tlsfingerprint.Profile) (*http.Response, error) {
	return nil, errors.New("unexpected upstream call")
}

// n 超出通道上限必须是 400 客户端错误，而不是把一个"网页端做不到"的请求发出去。
func TestChatGPTWebImagesRejectsExcessiveN(t *testing.T) {
	service := &OpenAIGatewayService{httpUpstream: chatGPTWebNoopTransport{}, cfg: &config.Config{}}
	account := &Account{
		ID:          1,
		Name:        "chatgpt-web-n-test",
		Platform:    PlatformOpenAI,
		Type:        AccountTypeOAuth,
		Credentials: map[string]any{"access_token": "dummy"},
		Extra:       map[string]any{featureKeyChatGPTWebImageGeneration: true},
	}
	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	context, _ := gin.CreateTestContext(recorder)
	context.Request = httptest.NewRequest(http.MethodPost, "/v1/images/generations", nil)

	parsed := &OpenAIImagesRequest{
		Endpoint: "/v1/images/generations",
		Model:    "gpt-image-2",
		Prompt:   "anything",
		N:        chatGPTWebMaxImagesPerRequest + 1,
	}
	_, err := service.forwardOpenAIImagesChatGPTWeb(context.Request.Context(), context, account, parsed, "")
	require.Error(t, err)
	var upstreamErr *OpenAIImagesUpstreamError
	require.ErrorAs(t, err, &upstreamErr)
	require.Equal(t, http.StatusBadRequest, upstreamErr.StatusCode)
	require.Equal(t, "n", upstreamErr.Param)
}

func TestClassifyChatGPTWebFailure(t *testing.T) {
	require.Equal(t, chatGPTWebFailureAuth, classifyChatGPTWebFailure(401, false))
	require.Equal(t, chatGPTWebFailureAuth, classifyChatGPTWebFailure(403, false))
	require.Equal(t, chatGPTWebFailureQuota, classifyChatGPTWebFailure(429, false))
	require.Equal(t, chatGPTWebFailureQuota, classifyChatGPTWebFailure(402, false))
	require.Equal(t, chatGPTWebFailureTransient, classifyChatGPTWebFailure(500, false))
	require.Equal(t, chatGPTWebFailureTransient, classifyChatGPTWebFailure(503, false))
	require.Equal(t, chatGPTWebFailureNone, classifyChatGPTWebFailure(400, false))
	// Cloudflare 拦截页优先归因：换号解决不了，也不能当额度问题去冷却账号。
	require.Equal(t, chatGPTWebFailureChallenge, classifyChatGPTWebFailure(403, true))
	require.Equal(t, chatGPTWebFailureChallenge, classifyChatGPTWebFailure(429, true))
}

func TestChatGPTWebUpstreamErrorClientStatus(t *testing.T) {
	require.Equal(t, 429, (&chatGPTWebUpstreamError{StatusCode: 429}).clientStatusCode())
	require.Equal(t, 429, (&chatGPTWebUpstreamError{StatusCode: 402}).clientStatusCode())
	require.Equal(t, 502, (&chatGPTWebUpstreamError{StatusCode: 401}).clientStatusCode())
	require.Equal(t, 502, (&chatGPTWebUpstreamError{StatusCode: 403}).clientStatusCode())
	require.Equal(t, 502, (&chatGPTWebUpstreamError{StatusCode: 500}).clientStatusCode())
	require.Equal(t, 502, (&chatGPTWebUpstreamError{StatusCode: 0}).clientStatusCode())
	// 拦截页即使带 429 也不能告诉客户端"限流了"。
	require.Equal(t, 502, (&chatGPTWebUpstreamError{StatusCode: 429, Challenge: true}).clientStatusCode())
}

func TestIsChatGPTWebChallengePage(t *testing.T) {
	require.True(t, isChatGPTWebChallengePage(&http.Response{Header: http.Header{"Cf-Mitigated": []string{"challenge"}}}, []byte("{}")))
	require.True(t, isChatGPTWebChallengePage(&http.Response{Header: http.Header{}}, []byte("<html><head></head></html>")))
	require.False(t, isChatGPTWebChallengePage(&http.Response{Header: http.Header{}}, []byte(`{"detail":"Not Found"}`)))
}

func TestChatGPTWebImageModelSettings(t *testing.T) {
	require.Equal(t, "auto", chatGPTWebImageModelSettings("gpt-image-2", ""))
	require.Equal(t, "auto", chatGPTWebImageModelSettings("", ""))
	require.Equal(t, "gpt-5-3", chatGPTWebImageModelSettings("gpt-image-2", "gpt-5-3"))
	require.Equal(t, "auto", chatGPTWebImageModelSettings("dall-e-3", "gpt-5-3"))
}

// 未开启开关的账号必须保持原有 OAuth 路径。
func TestChatGPTWebImageGenerationEnabledDefaultsOff(t *testing.T) {
	account := &Account{Platform: PlatformOpenAI, Type: AccountTypeOAuth}
	require.False(t, account.ChatGPTWebImageGenerationEnabled())

	account.Extra = map[string]any{featureKeyChatGPTWebImageGeneration: true}
	require.True(t, account.ChatGPTWebImageGenerationEnabled())

	account.Extra = map[string]any{PlatformOpenAI: map[string]any{featureKeyChatGPTWebImageGeneration: true}}
	require.True(t, account.ChatGPTWebImageGenerationEnabled())
}

// web 类型账号必须只服务图片端点：否则文本流量会把它打到 429，
// 从而把网页生图额度一起锁死（这正是新增该类型的理由）。
func TestWebAccountCapabilityIsImagesOnly(t *testing.T) {
	account := &Account{Platform: PlatformOpenAI, Type: AccountTypeWebImage}
	require.True(t, account.SupportsOpenAIEndpointCapability(""))
	require.False(t, account.SupportsOpenAIEndpointCapability(OpenAIEndpointCapabilityChatCompletions))
	require.False(t, account.SupportsOpenAIEndpointCapability(OpenAIEndpointCapabilityResponses))
	require.False(t, account.SupportsOpenAIEndpointCapability(OpenAIEndpointCapabilityEmbeddings))
	require.False(t, account.SupportsOpenAIEndpointCapability(OpenAIEndpointCapabilityAlphaSearch))

	oauth := &Account{Platform: PlatformOpenAI, Type: AccountTypeOAuth}
	require.True(t, oauth.SupportsOpenAIEndpointCapability(OpenAIEndpointCapabilityChatCompletions))
	require.True(t, oauth.SupportsOpenAIEndpointCapability(""))
}

// web 类型账号直接走网页生图通道，无需再配 extra 开关。
func TestWebAccountRoutesToChatGPTWebChannel(t *testing.T) {
	service := &OpenAIGatewayService{httpUpstream: chatGPTWebNoopTransport{}, cfg: &config.Config{}}
	account := &Account{
		ID:          9002,
		Name:        "web-type",
		Platform:    PlatformOpenAI,
		Type:        AccountTypeWebImage,
		Credentials: map[string]any{"access_token": "dummy"},
	}
	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	context, _ := gin.CreateTestContext(recorder)
	context.Request = httptest.NewRequest(http.MethodPost, "/v1/images/generations", nil)

	// 用超限的 n 做探针：只有网页通道会返回这个 400。
	parsed := &OpenAIImagesRequest{
		Endpoint: "/v1/images/generations",
		Model:    "gpt-image-2",
		Prompt:   "probe",
		N:        chatGPTWebMaxImagesPerRequest + 1,
	}
	_, err := service.ForwardImages(context.Request.Context(), context, account, nil, parsed, "")
	require.Error(t, err)
	var upstreamErr *OpenAIImagesUpstreamError
	require.ErrorAs(t, err, &upstreamErr)
	require.Equal(t, http.StatusBadRequest, upstreamErr.StatusCode)
}

// 分派判定的唯一来源：网关转发与账号测试都必须落在这里。
func TestUsesChatGPTWebImageChannel(t *testing.T) {
	require.True(t, (&Account{Platform: PlatformOpenAI, Type: AccountTypeWebImage}).UsesChatGPTWebImageChannel())

	oauthOn := &Account{Platform: PlatformOpenAI, Type: AccountTypeOAuth, Extra: map[string]any{featureKeyChatGPTWebImageGeneration: true}}
	require.True(t, oauthOn.UsesChatGPTWebImageChannel())
	require.False(t, (&Account{Platform: PlatformOpenAI, Type: AccountTypeOAuth}).UsesChatGPTWebImageChannel())

	setupOn := &Account{Platform: PlatformOpenAI, Type: AccountTypeSetupToken, Extra: map[string]any{featureKeyChatGPTWebImageGeneration: true}}
	require.True(t, setupOn.UsesChatGPTWebImageChannel())

	require.False(t, (&Account{Platform: PlatformOpenAI, Type: AccountTypeAPIKey}).UsesChatGPTWebImageChannel())
	require.False(t, (&Account{Platform: PlatformGrok, Type: AccountTypeWebImage}).UsesChatGPTWebImageChannel())
	require.False(t, (*Account)(nil).UsesChatGPTWebImageChannel())
}

func TestChatGPTWebDeviceUsesImmutableAccountID(t *testing.T) {
	account := &Account{ID: 41, Name: "before-rename", Platform: PlatformOpenAI, Type: AccountTypeWebImage}
	before := account.ChatGPTWebDevice()
	account.Name = "after-rename"
	after := account.ChatGPTWebDevice()
	require.Equal(t, before.DeviceID, after.DeviceID)
	require.Equal(t, before.SessionID, after.SessionID)

	sameName := &Account{ID: 42, Name: account.Name, Platform: PlatformOpenAI, Type: AccountTypeWebImage}
	other := sameName.ChatGPTWebDevice()
	require.NotEqual(t, after.DeviceID, other.DeviceID)
	require.NotEqual(t, after.SessionID, other.SessionID)
}
