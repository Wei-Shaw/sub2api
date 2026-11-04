package service

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

func nativeImageTestAccount() *Account {
	return &Account{ID: 1, Platform: PlatformOpenAI, Type: AccountTypeOAuth, Credentials: map[string]any{"access_token": "test-token", "chatgpt_account_id": "test-account"}, Extra: map[string]any{OpenAIImageChannelExtraKey: "native"}}
}

func TestNativeImageForwardPreservesActualMetadataAndUsage(t *testing.T) {
	for _, stream := range []bool{false, true} {
		t.Run(fmt.Sprint(stream), func(t *testing.T) {
			body := []byte(fmt.Sprintf(`{"model":"gpt-image-2","prompt":"teapot","quality":"max","size":"1024x1024","stream":%t}`, stream))
			rec := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(rec)
			c.Request = httptest.NewRequest("POST", "/v1/images/generations", bytes.NewReader(body))
			c.Request.Header.Set("Content-Type", "application/json")
			native := fmt.Sprintf(`{"created":1234,"quality":"low","size":"1254x1254","output_format":"png","usage":{"input_tokens":61,"output_tokens":515,"total_tokens":576,"output_tokens_details":{"image_tokens":515}},"data":[{"b64_json":%q}]}`, encodeOpenAIImageTestPNG(t, 1254, 1254))
			upstream := &httpUpstreamRecorder{resp: &http.Response{StatusCode: 200, Header: http.Header{"X-Request-Id": {"native-test"}}, Body: io.NopCloser(strings.NewReader(native))}}
			svc := &OpenAIGatewayService{httpUpstream: upstream}
			parsed, err := svc.ParseOpenAIImagesRequest(c, body)
			require.NoError(t, err)
			result, err := svc.ForwardImages(context.Background(), c, nativeImageTestAccount(), body, parsed, "gpt-image-2.5-sunburst")
			require.NoError(t, err)
			require.Equal(t, "/backend-api/codex/images/generations", upstream.lastReq.URL.Path)
			require.Equal(t, "Bearer test-token", upstream.lastReq.Header.Get("Authorization"))
			require.Equal(t, "test-account", upstream.lastReq.Header.Get("Chatgpt-Account-Id"))
			require.Equal(t, "*/*", upstream.lastReq.Header.Get("Accept"))
			require.Equal(t, nativeImageCLIUserAgent, upstream.lastReq.Header.Get("User-Agent"))
			require.Equal(t, "codex_exec", upstream.lastReq.Header.Get("Originator"))
			require.Empty(t, upstream.lastReq.Header.Get("Version"))
			require.NotEmpty(t, upstream.lastReq.Header.Get("X-Codex-Image-Turn-Id"))
			require.Empty(t, upstream.lastReq.Header.Get("OpenAI-Beta"))
			require.Equal(t, "gpt-image-2.5-sunburst", gjson.GetBytes(upstream.lastBody, "model").String())
			require.Equal(t, "max", gjson.GetBytes(upstream.lastBody, "quality").String())
			require.Equal(t, "1024x1024", gjson.GetBytes(upstream.lastBody, "size").String())
			require.False(t, gjson.GetBytes(upstream.lastBody, "tools").Exists())
			require.False(t, gjson.GetBytes(upstream.lastBody, "instructions").Exists())
			require.Equal(t, 61, result.Usage.InputTokens)
			require.Equal(t, 515, result.Usage.ImageOutputTokens)
			require.Equal(t, []string{"1254x1254"}, result.ImageOutputSizes)
			output := rec.Body.String()
			if stream {
				event, ok := findOpenAIImageTestSSEEvent(parseOpenAIImageTestSSEEvents(output), "image_generation.completed")
				require.True(t, ok)
				output = event.Data
			}
			require.Equal(t, "low", gjson.Get(output, "quality").String())
			require.Equal(t, "1254x1254", gjson.Get(output, "size").String())
			require.Equal(t, int64(515), gjson.Get(output, "usage.output_tokens").Int())
			require.False(t, gjson.Get(output, "model").Exists(), "do not invent an upstream model identity")
		})
	}
}

func TestNativeImageStreamPreservesMultilineUpstreamUsage(t *testing.T) {
	const usageJSON = `{
  "input_tokens": 1553,
  "input_tokens_details": {"image_tokens": 1521, "text_tokens": 32, "cached_tokens": 0},
  "output_tokens": 915,
  "output_tokens_details": {"image_tokens": 915, "text_tokens": 0},
  "total_tokens": 2468,
  "future_details": {"value": null, "counts": [0, 1]}
}`
	for _, endpoint := range []string{openAIImagesGenerationsEndpoint, openAIImagesEditsEndpoint} {
		t.Run(endpoint, func(t *testing.T) {
			rec := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(rec)
			c.Request = httptest.NewRequest(http.MethodPost, endpoint, nil)
			body := fmt.Sprintf(`{"created":1234,"data":[{"b64_json":%q}],"quality":"medium","size":"32x32","output_format":"png","usage":%s}`, encodeOpenAIImageTestPNG(t, 32, 32), usageJSON)
			upstream := &httpUpstreamRecorder{resp: &http.Response{StatusCode: http.StatusOK, Header: http.Header{}, Body: io.NopCloser(strings.NewReader(body))}}
			svc := &OpenAIGatewayService{httpUpstream: upstream}
			parsed := &OpenAIImagesRequest{Endpoint: endpoint, Model: "gpt-image-2", Prompt: "make blue", Stream: true, N: 1}
			if parsed.IsEdits() {
				parsed.InputImageURLs = []string{"data:image/png;base64,dGVzdA=="}
			}
			result, err := svc.ForwardImages(context.Background(), c, nativeImageTestAccount(), nil, parsed, "")
			require.NoError(t, err)
			events := parseOpenAIImageTestSSEEvents(rec.Body.String())
			require.Len(t, events, 1)
			require.Equal(t, openAIImagesStreamPrefix(parsed)+".completed", events[0].Name)
			require.True(t, json.Valid([]byte(events[0].Data)), "each SSE data line must contain complete JSON")
			require.JSONEq(t, usageJSON, gjson.Get(events[0].Data, "usage").Raw)
			require.Equal(t, 1553, result.Usage.InputTokens)
			require.Equal(t, 1521, result.Usage.ImageInputTokens)
			require.Equal(t, 915, result.Usage.ImageOutputTokens)
		})
	}
}

func TestNativeImageEligibilityAndValidation(t *testing.T) {
	a := nativeImageTestAccount()
	require.NoError(t, validateOpenAIImageChannel(a))
	require.True(t, a.SupportsOpenAIEndpointCapability(""))
	require.False(t, a.SupportsOpenAIEndpointCapability(OpenAIEndpointCapabilityResponses))
	require.False(t, a.SupportsOpenAIEndpointCapability(OpenAIEndpointCapabilityChatCompletions))
	require.True(t, a.SupportsOpenAIImageCapability(OpenAIImagesCapabilityNative))
	a.Extra[featureKeyChatGPTWebImageGeneration] = true
	require.Error(t, validateOpenAIImageChannel(a))
	delete(a.Extra, featureKeyChatGPTWebImageGeneration)
	a.Type = AccountTypeAPIKey
	require.Error(t, validateOpenAIImageChannel(a))
	a.Extra[OpenAIImageChannelExtraKey] = []string{"native"}
	require.Error(t, validateOpenAIImageChannel(a))
}

func TestNativeImageUnsupportedOptionsAndEdits(t *testing.T) {
	for _, req := range []*OpenAIImagesRequest{{N: 2}, {HasMask: true}, {OutputFormat: "jpeg"}, {Moderation: "low"}} {
		_, err := buildOpenAINativeImagesBody(req, "gpt-image-2")
		require.Error(t, err)
	}
	req := &OpenAIImagesRequest{Endpoint: openAIImagesEditsEndpoint, Prompt: "make blue", Quality: "high", InputImageURLs: []string{"data:image/png;base64,dGVzdA=="}}
	body, err := buildOpenAINativeImagesBody(req, "gpt-image-2")
	require.NoError(t, err)
	require.Equal(t, req.InputImageURLs[0], gjson.GetBytes(body, "images.0.image_url").String())
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest("POST", "/v1/images/edits", nil)
	upstream, err := (&OpenAIGatewayService{}).buildOpenAINativeImagesRequest(context.Background(), c, nativeImageTestAccount(), req, "gpt-image-2", "test")
	require.NoError(t, err)
	require.Equal(t, "/backend-api/codex/images/edits", upstream.URL.Path)
	require.Equal(t, nativeImageCLIUserAgent, upstream.Header.Get("User-Agent"))
	require.Equal(t, "*/*", upstream.Header.Get("Accept"))
	require.Empty(t, upstream.Header.Get("Version"))
	require.NotEmpty(t, upstream.Header.Get("X-Codex-Image-Turn-Id"))
	_, _, _, _, err = decodeOpenAINativeImages([]byte(`{"data":[],"error":{"message":"no images"}}`))
	require.Error(t, err)
}

func TestNativeImageAccountTestUsesNativeEndpoint(t *testing.T) {
	upstream := &httpUpstreamRecorder{resp: &http.Response{StatusCode: 200, Header: http.Header{}, Body: io.NopCloser(strings.NewReader(`{"quality":"medium","size":"1254x1254","data":[{"b64_json":"dGVzdA=="}],"usage":{"output_tokens":915}}`))}}
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest("POST", "/test", nil)
	svc := &AccountTestService{httpUpstream: upstream}
	require.NoError(t, svc.testOpenAIImageNative(c, context.Background(), nativeImageTestAccount(), "gpt-image-2", "draw a circle"))
	require.Equal(t, "/backend-api/codex/images/generations", upstream.lastReq.URL.Path)
}

func TestNativeImageUnsupportedRequestIsClientErrorWithoutUpstreamCall(t *testing.T) {
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest("POST", "/v1/images/generations", nil)
	upstream := &httpUpstreamRecorder{}
	svc := &OpenAIGatewayService{httpUpstream: upstream}
	_, err := svc.ForwardImages(context.Background(), c, nativeImageTestAccount(), nil, &OpenAIImagesRequest{N: 2, Model: "gpt-image-2"}, "")
	require.Error(t, err)
	require.Equal(t, 400, rec.Code)
	require.Equal(t, "invalid_request_error", gjson.Get(rec.Body.String(), "error.type").String())
	require.Nil(t, upstream.lastReq)
}

func TestNativeImageCreateValidation(t *testing.T) {
	input := &CreateAccountInput{Platform: PlatformOpenAI, Type: AccountTypeOAuth, Credentials: map[string]any{"access_token": "test"}}
	a, err := buildAccountForCreate(input, map[string]any{OpenAIImageChannelExtraKey: "native"})
	require.NoError(t, err)
	require.True(t, a.UsesOpenAINativeImageChannel())
	input.Type = AccountTypeWebImage
	_, err = buildAccountForCreate(input, map[string]any{OpenAIImageChannelExtraKey: "native"})
	require.Error(t, err)
}
