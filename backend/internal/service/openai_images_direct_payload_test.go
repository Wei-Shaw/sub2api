package service

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"net/textproto"
	"os"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

func TestCodexDirectImagesMultipartEdit(t *testing.T) {
	var body bytes.Buffer
	w := multipart.NewWriter(&body)
	for key, value := range map[string]string{"model": "gpt-image-2.5-sunburst", "prompt": "编辑", "quality": "xhigh", "n": "2", "output_format": "webp", "output_compression": "75", "partial_images": "2", "input_fidelity": "high"} {
		require.NoError(t, w.WriteField(key, value))
	}
	for _, field := range []string{"image[]", "image[]", "mask"} {
		header := textproto.MIMEHeader{}
		header.Set("Content-Disposition", fmt.Sprintf(`form-data; name=%q; filename="image.png"`, field))
		header.Set("Content-Type", "image/png")
		part, err := w.CreatePart(header)
		require.NoError(t, err)
		_, err = part.Write([]byte("png-content"))
		require.NoError(t, err)
	}
	require.NoError(t, w.Close())
	c, _ := newOpenAIImagesTestContext(t, body.Bytes())
	c.Request.URL.Path = "/v1/images/edits"
	c.Request.Header.Set("Content-Type", w.FormDataContentType())
	svc := newOpenAIImagesTestService(nil)
	parsed, err := svc.ParseOpenAIImagesRequest(c, body.Bytes())
	require.NoError(t, err)
	upstreamBody, target, err := buildOpenAIImagesOAuthPayload(parsed, parsed.Model)
	require.NoError(t, err)
	require.Equal(t, "https://chatgpt.com/backend-api/codex/images/edits", target)
	require.Len(t, gjson.GetBytes(upstreamBody, "images").Array(), 2)
	require.Equal(t, "data:image/png;base64,cG5nLWNvbnRlbnQ=", gjson.GetBytes(upstreamBody, "mask.image_url").String())
	require.Equal(t, "xhigh", gjson.GetBytes(upstreamBody, "quality").String())
	require.Equal(t, "high", gjson.GetBytes(upstreamBody, "input_fidelity").String())
	require.EqualValues(t, 2, gjson.GetBytes(upstreamBody, "n").Int())
	require.EqualValues(t, 75, gjson.GetBytes(upstreamBody, "output_compression").Int())
	require.EqualValues(t, 2, gjson.GetBytes(upstreamBody, "partial_images").Int())
	require.False(t, gjson.GetBytes(upstreamBody, "tools").Exists())
}

func TestCodexDirectImagesPricingAndUsage(t *testing.T) {
	prices := &PricingService{pricingData: map[string]*LiteLLMModelPricing{"gpt-image-2": {InputCostPerToken: 1}}}
	for _, model := range []string{"gpt-image-2.5-flare", "gpt-image-2.5-sunburst", "gpt-image-2.5-flare-2026-09-08", "gpt-image-2.5-sunburst-2026-09-08"} {
		price := prices.matchOpenAIModel(model)
		require.Equal(t, 5e-6, price.InputCostPerToken)
		require.Equal(t, 8e-6, price.InputCostPerImageToken)
		require.Equal(t, 30e-6, price.OutputCostPerImageToken)
		require.Equal(t, 2e-6, price.CacheReadInputImageTokenCost)
	}
	body, err := os.ReadFile("../../resources/model-pricing/model_prices_and_context_window.json")
	require.NoError(t, err)
	var catalog map[string]LiteLLMModelPricing
	require.NoError(t, json.Unmarshal(body, &catalog))
	require.Equal(t, 2e-6, catalog["gpt-image-2.5-flare"].CacheReadInputImageTokenCost)
	usage, ok := codexDirectImagesUsage([]byte(`{"usage":{"input_tokens":100,"input_tokens_details":{"text_tokens":20,"image_tokens":80,"cached_tokens":50,"cached_tokens_details":{"text_tokens":10,"image_tokens":40}},"output_tokens":200}}`))
	require.True(t, ok)
	require.Equal(t, 40, usage.ImageCacheReadTokens)
	require.Equal(t, 200, usage.ImageOutputTokens)
	tokens := UsageTokens{InputTokens: usage.InputTokens - usage.CacheReadInputTokens, ImageInputTokens: usage.ImageInputTokens - usage.ImageCacheReadTokens, CacheReadTokens: usage.CacheReadInputTokens, ImageCacheReadTokens: usage.ImageCacheReadTokens, OutputTokens: usage.OutputTokens, ImageOutputTokens: usage.ImageOutputTokens}
	price := &ModelPricing{InputPricePerToken: 5e-6, ImageInputPricePerToken: 8e-6, CacheReadPricePerToken: 1.25e-6, ImageCacheReadPricePerToken: 2e-6, ImageOutputPricePerToken: 30e-6}
	cost := (&BillingService{}).computeTokenBreakdown(price, tokens, 1, "", false)
	require.InDelta(t, 10*5e-6, cost.InputCost, 1e-12)
	require.InDelta(t, 40*8e-6, cost.ImageInputCost, 1e-12)
	require.InDelta(t, 10*1.25e-6+40*2e-6, cost.CacheReadCost, 1e-12)
	require.InDelta(t, 200*30e-6, cost.ImageOutputCost, 1e-12)
	unknown, _ := codexDirectImagesUsage([]byte(`{"usage":{"input_tokens":100,"input_tokens_details":{"image_tokens":80,"cached_tokens":50},"output_tokens":200}}`))
	require.Zero(t, unknown.ImageCacheReadTokens, "不能根据总缓存量推测图片缓存占比")
}

func TestCodexImagesLunaErrorDoesNotCoolImageAccount(t *testing.T) {
	for _, stream := range []bool{false, true} {
		t.Run(fmt.Sprint(stream), func(t *testing.T) {
			body := []byte(fmt.Sprintf(`{"model":"gpt-image-1","prompt":"draw","stream":%t}`, stream))
			c, rec := newOpenAIImagesTestContext(t, body)
			upstream := &codexModelsHTTPUpstreamStub{do: func(req *http.Request, _ string, _ int64, _ int) (*http.Response, error) {
				require.Equal(t, "/backend-api/codex/responses", req.URL.Path)
				return &http.Response{StatusCode: 400, Header: http.Header{}, Body: io.NopCloser(strings.NewReader(`{"error":{"type":"invalid_request_error","message":"The 'gpt-5.6-luna' model is not supported when using Codex with a ChatGPT account."}}`))}, nil
			}}
			svc := newOpenAIImagesTestService(upstream)
			parsed, err := svc.ParseOpenAIImagesRequest(c, body)
			require.NoError(t, err)
			result, err := svc.ForwardImages(context.Background(), c, directImagesTestAccount(), body, parsed, "")
			require.Nil(t, result)
			var upstreamErr *OpenAIImagesUpstreamError
			require.ErrorAs(t, err, &upstreamErr)
			require.Contains(t, rec.Body.String(), "gpt-5.6-luna")
			var failover *UpstreamFailoverError
			require.NotErrorAs(t, err, &failover)
		})
	}
}

func TestCodexDirectImagesShadowCredentials(t *testing.T) {
	parent := directImagesTestAccount()
	parent.Status = StatusActive
	shadow := &Account{ID: 99, ParentAccountID: &parent.ID, Platform: PlatformOpenAI, Type: AccountTypeOAuth}
	body := []byte(`{"model":"gpt-image-2","prompt":"draw"}`)
	c, _ := newOpenAIImagesTestContext(t, body)
	upstream := &httpUpstreamRecorder{resp: openAIImagesJSONResponse()}
	svc := newOpenAIImagesTestService(upstream)
	svc.accountRepo = newStubCredRepo(parent)
	parsed, err := svc.ParseOpenAIImagesRequest(c, body)
	require.NoError(t, err)
	_, err = svc.ForwardImages(context.Background(), c, shadow, body, parsed, "")
	require.NoError(t, err)
	require.Equal(t, "Bearer test-token", upstream.lastReq.Header.Get("Authorization"))
	require.Equal(t, "test-account", upstream.lastReq.Header.Get("Chatgpt-Account-Id"))
	require.Equal(t, "/backend-api/codex/images/generations", upstream.lastReq.URL.Path)
}
