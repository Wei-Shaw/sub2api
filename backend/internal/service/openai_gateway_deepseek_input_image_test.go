package service

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

const deepSeekInputImageDataURI = "data:image/png;base64,AQID"

func deepSeekNativeResponsesAccount() *Account {
	return &Account{
		Platform: PlatformDeepseek,
		Type:     AccountTypeAPIKey,
		Credentials: map[string]any{
			"api_key":      "sk-test",
			"api_protocol": APIProtocolResponses,
		},
	}
}

func deepSeekUserImageBody(partJSON string) []byte {
	return []byte(`{"model":"deepseek-flash","store":true,"input":[{"type":"message","role":"user","content":[{"type":"input_text","text":"what is this?"},` + partJSON + `]}]}`)
}

func TestNormalizeDeepSeekResponsesRequestBodyAliasesInputImageURL(t *testing.T) {
	t.Parallel()

	native := deepSeekNativeResponsesAccount()
	mapped := openaiPlatformDeepSeekAccount()
	kimi := &Account{
		Platform: PlatformKimi, Type: AccountTypeAPIKey,
		Credentials: map[string]any{"api_protocol": APIProtocolResponses},
	}
	openai := &Account{Platform: PlatformOpenAI, Type: AccountTypeAPIKey}

	t.Run("openai_image_url_string_gets_url", func(t *testing.T) {
		body := deepSeekUserImageBody(`{"type":"input_image","image_url":"` + deepSeekInputImageDataURI + `"}`)
		got := normalizeDeepSeekResponsesRequestBody(mapped, body)
		require.Equal(t, "input_image", gjson.GetBytes(got, "input.0.content.1.type").String())
		require.Equal(t, deepSeekInputImageDataURI, gjson.GetBytes(got, "input.0.content.1.image_url").String())
		require.Equal(t, deepSeekInputImageDataURI, gjson.GetBytes(got, "input.0.content.1.url").String())
		require.Equal(t, "what is this?", gjson.GetBytes(got, "input.0.content.0.text").String())
	})

	t.Run("nested_image_url_object_flattened", func(t *testing.T) {
		body := deepSeekUserImageBody(`{"type":"input_image","image_url":{"url":"` + deepSeekInputImageDataURI + `"}}`)
		got := normalizeDeepSeekResponsesRequestBody(native, body)
		require.Equal(t, gjson.String, gjson.GetBytes(got, "input.0.content.1.image_url").Type)
		require.Equal(t, deepSeekInputImageDataURI, gjson.GetBytes(got, "input.0.content.1.image_url").String())
		require.Equal(t, deepSeekInputImageDataURI, gjson.GetBytes(got, "input.0.content.1.url").String())
	})

	t.Run("chat_completions_image_url_part_converted", func(t *testing.T) {
		body := deepSeekUserImageBody(`{"type":"image_url","image_url":{"url":"` + deepSeekInputImageDataURI + `"}}`)
		got := normalizeDeepSeekResponsesRequestBody(mapped, body)
		require.Equal(t, "input_image", gjson.GetBytes(got, "input.0.content.1.type").String())
		require.Equal(t, deepSeekInputImageDataURI, gjson.GetBytes(got, "input.0.content.1.image_url").String())
		require.Equal(t, deepSeekInputImageDataURI, gjson.GetBytes(got, "input.0.content.1.url").String())
	})

	t.Run("anthropic_source_becomes_data_uri", func(t *testing.T) {
		body := deepSeekUserImageBody(`{"type":"image","source":{"type":"base64","media_type":"image/png","data":"AQID"}}`)
		got := normalizeDeepSeekResponsesRequestBody(native, body)
		require.Equal(t, "input_image", gjson.GetBytes(got, "input.0.content.1.type").String())
		require.Equal(t, deepSeekInputImageDataURI, gjson.GetBytes(got, "input.0.content.1.image_url").String())
		require.Equal(t, deepSeekInputImageDataURI, gjson.GetBytes(got, "input.0.content.1.url").String())
	})

	t.Run("lifted_tool_output_image_gets_url", func(t *testing.T) {
		body := []byte(`{"model":"deepseek-flash","input":[{"type":"function_call","call_id":"call_image","name":"view_image","arguments":"{}"},{"type":"function_call_output","call_id":"call_image","output":[{"type":"input_image","image_url":"` + deepSeekInputImageDataURI + `"}]}]}`)
		got := normalizeDeepSeekResponsesRequestBody(mapped, body)
		require.Equal(t, gjson.String, gjson.GetBytes(got, "input.1.output").Type)
		require.Equal(t, "input_image", gjson.GetBytes(got, "input.2.content.1.type").String())
		require.Equal(t, deepSeekInputImageDataURI, gjson.GetBytes(got, "input.2.content.1.image_url").String())
		require.Equal(t, deepSeekInputImageDataURI, gjson.GetBytes(got, "input.2.content.1.url").String())
	})

	t.Run("file_id_only_does_not_gain_empty_url", func(t *testing.T) {
		body := deepSeekUserImageBody(`{"type":"input_image","file_id":"file-abc"}`)
		got := normalizeDeepSeekResponsesRequestBody(native, body)
		require.Equal(t, "file-abc", gjson.GetBytes(got, "input.0.content.1.file_id").String())
		require.False(t, gjson.GetBytes(got, "input.0.content.1.url").Exists())
		require.False(t, gjson.GetBytes(got, "input.0.content.1.image_url").Exists())
	})

	t.Run("kimi_does_not_alias_url", func(t *testing.T) {
		body := deepSeekUserImageBody(`{"type":"input_image","image_url":"` + deepSeekInputImageDataURI + `"}`)
		got := normalizeDeepSeekResponsesRequestBody(kimi, body)
		require.Equal(t, deepSeekInputImageDataURI, gjson.GetBytes(got, "input.0.content.1.image_url").String())
		require.False(t, gjson.GetBytes(got, "input.0.content.1.url").Exists())
	})

	t.Run("openai_host_unchanged", func(t *testing.T) {
		body := deepSeekUserImageBody(`{"type":"input_image","image_url":"` + deepSeekInputImageDataURI + `"}`)
		got := normalizeDeepSeekResponsesRequestBody(openai, body)
		require.Equal(t, string(body), string(got))
		require.False(t, gjson.GetBytes(got, "input.0.content.1.url").Exists())
	})
}

func deepSeekCompletedResponsesJSON(model string) string {
	return `{"id":"resp_ds_img","object":"response","model":"` + model + `","status":"completed","output":[{"type":"message","role":"assistant","content":[{"type":"output_text","text":"ok"}]}],"usage":{"input_tokens":5,"output_tokens":2,"total_tokens":7}}`
}

func TestForwardResponses_OpenAIMappedDeepSeekAliasesInputImageURL(t *testing.T) {
	body := deepSeekUserImageBody(`{"type":"input_image","image_url":"` + deepSeekInputImageDataURI + `"}`)
	c := newDeepSeekChatFallbackContext(t, body)
	upstream := &httpUpstreamRecorder{resp: &http.Response{
		StatusCode: http.StatusOK,
		Header:     http.Header{"Content-Type": []string{"application/json"}},
		Body:       io.NopCloser(bytes.NewReader([]byte(deepSeekCompletedResponsesJSON("deepseek-flash")))),
	}}
	svc := &OpenAIGatewayService{
		cfg:          deepSeekChatFallbackTestConfig(),
		httpUpstream: upstream,
	}

	result, err := svc.Forward(context.Background(), c, openaiPlatformDeepSeekAccount(), body)
	require.NoError(t, err)
	require.NotNil(t, result)
	require.Contains(t, upstream.lastReq.URL.String(), "/responses")
	require.Equal(t, deepSeekInputImageDataURI, gjson.GetBytes(upstream.lastBody, "input.0.content.1.image_url").String())
	require.Equal(t, deepSeekInputImageDataURI, gjson.GetBytes(upstream.lastBody, "input.0.content.1.url").String())
}

func TestForwardDeepSeekResponsesAliasesInputImageURL(t *testing.T) {
	gin.SetMode(gin.TestMode)
	body := deepSeekUserImageBody(`{"type":"input_image","image_url":"` + deepSeekInputImageDataURI + `"}`)
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", bytes.NewReader(body))

	upstream := &httpUpstreamRecorder{resp: &http.Response{
		StatusCode: http.StatusOK,
		Header:     http.Header{"Content-Type": []string{"application/json"}},
		Body:       io.NopCloser(bytes.NewReader([]byte(deepSeekCompletedResponsesJSON("deepseek-v4-pro")))),
	}}
	svc := &OpenAIGatewayService{cfg: deepSeekForwardTestConfig(), httpUpstream: upstream}

	result, err := svc.Forward(context.Background(), c, deepSeekForwardTestAccount(), body)
	require.NoError(t, err)
	require.NotNil(t, result)
	require.Equal(t, "http://deepseek.example/responses", upstream.lastReq.URL.String())
	require.Equal(t, deepSeekInputImageDataURI, gjson.GetBytes(upstream.lastBody, "input.0.content.1.image_url").String())
	require.Equal(t, deepSeekInputImageDataURI, gjson.GetBytes(upstream.lastBody, "input.0.content.1.url").String())
}

func TestBuildDeepSeekWSHTTPBridgeRequestAliasesInputImageURL(t *testing.T) {
	svc := &OpenAIGatewayService{cfg: deepSeekForwardTestConfig()}
	body := []byte(`{"type":"response.create","model":"deepseek-v4-pro","input":[{"type":"message","role":"user","content":[{"type":"input_image","image_url":"` + deepSeekInputImageDataURI + `"}]}]}`)

	_, prepared, err := svc.buildDeepSeekWSHTTPBridgeRequest(
		context.Background(),
		nil,
		deepSeekForwardTestAccount(),
		"sk-deepseek-test",
		body,
	)
	require.NoError(t, err)
	require.Equal(t, deepSeekInputImageDataURI, gjson.GetBytes(prepared, "input.0.content.0.image_url").String())
	require.Equal(t, deepSeekInputImageDataURI, gjson.GetBytes(prepared, "input.0.content.0.url").String())
	require.False(t, gjson.GetBytes(prepared, "type").Exists())
}
