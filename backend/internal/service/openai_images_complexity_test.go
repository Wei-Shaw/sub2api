package service

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

func complexityResponsesTestResponse() *http.Response {
	return &http.Response{
		StatusCode: http.StatusOK,
		Header:     http.Header{"Content-Type": {"text/event-stream"}},
		Body:       io.NopCloser(strings.NewReader("data: {\"type\":\"response.completed\",\"response\":{\"output\":[{\"type\":\"image_generation_call\",\"result\":\"aGVsbG8=\"}]}}\n\n")),
	}
}

func TestOpenAIImagesComplexityForwarding(t *testing.T) {
	for _, test := range []struct {
		name, model, mappedModel, complexity, accountType string
		direct                                            bool
	}{
		{name: "flare25_high", model: "gpt-image-2.5-flare", complexity: "high", direct: true},
		{name: "sunburst25_snapshot_medium", model: "gpt-image-2.5-sunburst-2026-09-08", complexity: "medium", direct: true},
		{name: "flare26_low", model: "gpt-image-2.6-flare", complexity: "low"},
		{name: "sunburst26_medium", model: "gpt-image-2.6-sunburst", complexity: "medium"},
		{name: "mapped_model", model: "gpt-image-2", mappedModel: "gpt-image-2.6-flare", complexity: "high"},
		{name: "setup_token", model: "gpt-image-2.6-flare", complexity: "low", accountType: AccountTypeSetupToken},
		{name: "direct_omitted", model: "gpt-image-2.5-flare", direct: true},
		{name: "responses_omitted", model: "gpt-image-2.6-flare"},
	} {
		t.Run(test.name, func(t *testing.T) {
			for _, edits := range []bool{false, true} {
				t.Run(fmt.Sprintf("edits=%t", edits), func(t *testing.T) {
					payload := map[string]any{"model": test.model, "prompt": "draw a red cup", "quality": "medium"}
					if test.complexity != "" {
						payload["complexity"] = test.complexity
					}
					if edits {
						payload["images"] = []map[string]string{{"image_url": "data:image/png;base64,AA=="}}
					}
					body, err := json.Marshal(payload)
					require.NoError(t, err)
					c, _ := newOpenAIImagesTestContext(t, body)
					if edits {
						c.Request.URL.Path = openAIImagesEditsEndpoint
					}
					upstream := &httpUpstreamRecorder{resp: complexityResponsesTestResponse()}
					prefix := "tools.0."
					endpoint := "/backend-api/codex/responses"
					if test.direct {
						upstream.resp = openAIImagesJSONResponse()
						prefix = ""
						endpoint = "/backend-api/codex/images/generations"
						if edits {
							endpoint = "/backend-api/codex/images/edits"
						}
					}
					svc := newOpenAIImagesTestService(upstream)
					parsed, err := svc.ParseOpenAIImagesRequest(c, body)
					require.NoError(t, err)
					account := directImagesTestAccount()
					if test.accountType != "" {
						account.Type = test.accountType
					}
					if test.mappedModel != "" {
						account.Credentials["model_mapping"] = map[string]any{test.model: test.mappedModel}
					}
					result, err := svc.ForwardImages(context.Background(), c, account, body, parsed, "")
					require.NoError(t, err)
					require.Equal(t, 1, result.ImageCount)
					require.Equal(t, endpoint, upstream.lastReq.URL.Path)
					require.Equal(t, test.complexity, gjson.GetBytes(upstream.lastBody, prefix+"complexity").String())
					require.Equal(t, test.complexity != "", gjson.GetBytes(upstream.lastBody, prefix+"complexity").Exists())
					require.Equal(t, "medium", gjson.GetBytes(upstream.lastBody, prefix+"quality").String())
					if !test.direct {
						require.Equal(t, "medium", gjson.GetBytes(upstream.lastBody, "reasoning.effort").String())
					}
				})
			}
		})
	}
}

func TestOpenAIImagesComplexityMultipartEdit(t *testing.T) {
	for _, model := range []string{"gpt-image-2.5-flare", "gpt-image-2.6-flare"} {
		t.Run(model, func(t *testing.T) {
			var body bytes.Buffer
			w := multipart.NewWriter(&body)
			for key, value := range map[string]string{"model": model, "prompt": "replace background", "quality": "medium", "complexity": "low"} {
				require.NoError(t, w.WriteField(key, value))
			}
			part, err := w.CreateFormFile("image", "source.png")
			require.NoError(t, err)
			_, err = part.Write([]byte("fake-image-bytes"))
			require.NoError(t, err)
			require.NoError(t, w.Close())
			c, _ := newOpenAIImagesTestContext(t, body.Bytes())
			c.Request.URL.Path = openAIImagesEditsEndpoint
			c.Request.Header.Set("Content-Type", w.FormDataContentType())
			upstream := &httpUpstreamRecorder{resp: complexityResponsesTestResponse()}
			prefix := "tools.0."
			if model == "gpt-image-2.5-flare" {
				upstream.resp = openAIImagesJSONResponse()
				prefix = ""
			}
			svc := newOpenAIImagesTestService(upstream)
			parsed, err := svc.ParseOpenAIImagesRequest(c, body.Bytes())
			require.NoError(t, err)
			_, err = svc.ForwardImages(context.Background(), c, directImagesTestAccount(), body.Bytes(), parsed, "")
			require.NoError(t, err)
			require.Equal(t, "low", gjson.GetBytes(upstream.lastBody, prefix+"complexity").String())
			require.Equal(t, "medium", gjson.GetBytes(upstream.lastBody, prefix+"quality").String())
		})
	}
}

func TestOpenAIImagesComplexityRequiresNativeCapability(t *testing.T) {
	for _, multipartRequest := range []bool{false, true} {
		t.Run(fmt.Sprint(multipartRequest), func(t *testing.T) {
			body := []byte(`{"prompt":"draw a red cup","complexity":"low"}`)
			contentType := "application/json"
			if multipartRequest {
				var buf bytes.Buffer
				w := multipart.NewWriter(&buf)
				require.NoError(t, w.WriteField("prompt", "draw a red cup"))
				require.NoError(t, w.WriteField("complexity", "low"))
				require.NoError(t, w.Close())
				body = buf.Bytes()
				contentType = w.FormDataContentType()
			}
			c, _ := newOpenAIImagesTestContext(t, body)
			c.Request.Header.Set("Content-Type", contentType)
			parsed, err := (&OpenAIGatewayService{}).ParseOpenAIImagesRequest(c, body)
			require.NoError(t, err)
			require.True(t, parsed.HasNativeOptions)
			require.Equal(t, OpenAIImagesCapabilityNative, parsed.RequiredCapability)
		})
	}
	require.True(t, isOpenAINativeImageOption("complexity"))
}

func TestOpenAIImagesComplexityResponsesFallback(t *testing.T) {
	for _, status := range []int{http.StatusNotFound, http.StatusMethodNotAllowed} {
		t.Run(fmt.Sprint(status), func(t *testing.T) {
			body := []byte(`{"model":"gpt-image-2.5-flare","prompt":"draw a red cup","quality":"medium","complexity":"high"}`)
			c, _ := newOpenAIImagesTestContext(t, body)
			upstream := &httpUpstreamRecorder{responses: []*http.Response{
				{StatusCode: status, Header: http.Header{}, Body: io.NopCloser(strings.NewReader(`{"error":{"message":"endpoint unavailable"}}`))},
				complexityResponsesTestResponse(),
			}}
			svc := newOpenAIImagesTestService(upstream)
			parsed, err := svc.ParseOpenAIImagesRequest(c, body)
			require.NoError(t, err)
			result, err := svc.ForwardImages(context.Background(), c, directImagesTestAccount(), body, parsed, "")
			require.NoError(t, err)
			require.Equal(t, 1, result.ImageCount)
			require.Len(t, upstream.bodies, 2)
			require.Equal(t, "high", gjson.GetBytes(upstream.bodies[0], "complexity").String())
			require.Equal(t, "/backend-api/codex/responses", upstream.lastReq.URL.Path)
			require.Equal(t, "high", gjson.GetBytes(upstream.lastBody, "tools.0.complexity").String())
			require.Equal(t, "medium", gjson.GetBytes(upstream.lastBody, "tools.0.quality").String())
		})
	}
}

func TestOpenAIImagesComplexityAPIKeyPassthrough(t *testing.T) {
	for _, extra := range []string{"", `,"complexity":"low"`} {
		t.Run(extra, func(t *testing.T) {
			body := []byte(`{"model":"gpt-image-2.6-flare","prompt":"draw a red cup","quality":"medium"` + extra + `}`)
			c, _ := newOpenAIImagesTestContext(t, body)
			upstream := &httpUpstreamRecorder{resp: openAIImagesJSONResponse()}
			svc := newOpenAIImagesTestService(upstream)
			parsed, err := svc.ParseOpenAIImagesRequest(c, body)
			require.NoError(t, err)
			_, err = svc.ForwardImages(context.Background(), c, newOpenAIImagesAPIKeyAccount(), body, parsed, "")
			require.NoError(t, err)
			require.Equal(t, gjson.GetBytes(body, "complexity").Exists(), gjson.GetBytes(upstream.lastBody, "complexity").Exists())
			require.Equal(t, gjson.GetBytes(body, "complexity").String(), gjson.GetBytes(upstream.lastBody, "complexity").String())
			require.Equal(t, "medium", gjson.GetBytes(upstream.lastBody, "quality").String())
		})
	}
}

func TestOpenAIImagesComplexityImageOnlyResponsesModel(t *testing.T) {
	for _, toolComplexity := range []string{"", "high"} {
		t.Run(toolComplexity, func(t *testing.T) {
			tool := map[string]any{"type": "image_generation"}
			if toolComplexity != "" {
				tool["complexity"] = toolComplexity
			}
			body := map[string]any{
				"model": "gpt-image-2.6-flare", "input": "draw a red cup", "complexity": "low",
				"tools": []any{tool},
			}
			require.True(t, normalizeOpenAIResponsesImageOnlyModel(body))
			require.NotContains(t, body, "complexity")
			want := toolComplexity
			if want == "" {
				want = "low"
			}
			require.Equal(t, want, tool["complexity"])
		})
	}
}
