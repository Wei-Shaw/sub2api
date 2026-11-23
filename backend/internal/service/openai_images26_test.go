package service

import (
	"bytes"
	"context"
	"encoding/base64"
	"fmt"
	"image"
	"image/gif"
	"io"
	"mime/multipart"
	"net/http"
	"net/textproto"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

func TestGPTImage26ComplexityAcrossImageEndpoints(t *testing.T) {
	for _, model := range []string{"gpt-image-2.6-flare", "gpt-image-2.6-sunburst"} {
		for _, accountType := range []string{AccountTypeOAuth, AccountTypeSetupToken} {
			for _, format := range []string{"generation", "json-edit", "multipart-edit"} {
				for _, complexity := range []string{"", "low", "medium", "high"} {
					t.Run(strings.Join([]string{model, accountType, format, complexity}, "/"), func(t *testing.T) {
						body := []byte(fmt.Sprintf(`{"model":%q,"prompt":"draw","quality":"medium"`, model))
						if complexity != "" {
							body = append(body, []byte(fmt.Sprintf(`,"complexity":%q`, complexity))...)
						}
						if format == "json-edit" {
							body = append(body, []byte(`,"images":[{"image_url":"data:image/png;base64,AA=="}]`)...)
						}
						body = append(body, '}')
						contentType := "application/json"
						if format == "multipart-edit" {
							var buf bytes.Buffer
							writer := multipart.NewWriter(&buf)
							for key, value := range map[string]string{"model": model, "prompt": "draw", "quality": "medium"} {
								require.NoError(t, writer.WriteField(key, value))
							}
							if complexity != "" {
								require.NoError(t, writer.WriteField("complexity", complexity))
							}
							part, err := writer.CreateFormFile("image", "input.png")
							require.NoError(t, err)
							_, err = part.Write([]byte("image-bytes"))
							require.NoError(t, err)
							require.NoError(t, writer.Close())
							body, contentType = buf.Bytes(), writer.FormDataContentType()
						}
						c, _ := newOpenAIImagesTestContext(t, body)
						c.Request.Header.Set("Content-Type", contentType)
						endpoint := "/backend-api/codex/images/generations"
						if format != "generation" {
							c.Request.URL.Path = openAIImagesEditsEndpoint
							endpoint = "/backend-api/codex/images/edits"
						}
						upstream := &httpUpstreamRecorder{resp: openAIImagesJSONResponse()}
						svc := newOpenAIImagesTestService(upstream)
						parsed, err := svc.ParseOpenAIImagesRequest(c, body)
						require.NoError(t, err)
						account := directImagesTestAccount()
						account.Type = accountType
						result, err := svc.ForwardImages(context.Background(), c, account, body, parsed, "")
						require.NoError(t, err)
						require.Equal(t, endpoint, result.UpstreamEndpoint)
						require.Equal(t, model, result.UpstreamModel)
						require.Equal(t, "medium", gjson.GetBytes(upstream.lastBody, "quality").String())
						require.Equal(t, complexity, gjson.GetBytes(upstream.lastBody, "complexity").String())
						require.Equal(t, complexity != "", gjson.GetBytes(upstream.lastBody, "complexity").Exists())
						require.False(t, gjson.GetBytes(upstream.lastBody, "reasoning").Exists())
						if format != "generation" {
							require.Len(t, gjson.GetBytes(upstream.lastBody, "images").Array(), 1)
						}
					})
				}
			}
		}
	}
}

func TestGPTImage26DoesNotFallbackToResponses(t *testing.T) {
	for _, model := range []string{"gpt-image-2.6-flare", "gpt-image-2.6-sunburst"} {
		for _, status := range []int{400, 404, 405, 429} {
			t.Run(fmt.Sprintf("%s/%d", model, status), func(t *testing.T) {
				calls := 0
				upstream := &codexModelsHTTPUpstreamStub{do: func(req *http.Request, _ string, _ int64, _ int) (*http.Response, error) {
					calls++
					require.Equal(t, "/backend-api/codex/images/generations", req.URL.Path)
					return &http.Response{StatusCode: status, Header: http.Header{}, Body: io.NopCloser(strings.NewReader(`{"error":{"type":"invalid_request_error","message":"unsupported image option"}}`))}, nil
				}}
				body := []byte(fmt.Sprintf(`{"model":%q,"prompt":"draw","complexity":"low"}`, model))
				c, rec := newOpenAIImagesTestContext(t, body)
				svc := newOpenAIImagesTestService(upstream)
				parsed, err := svc.ParseOpenAIImagesRequest(c, body)
				require.NoError(t, err)
				result, err := svc.ForwardImages(context.Background(), c, directImagesTestAccount(), body, parsed, "")
				require.Error(t, err)
				require.Nil(t, result)
				require.Equal(t, 1, calls)
				if status == 400 {
					require.Equal(t, 400, rec.Code)
				}
			})
		}
	}
}

func TestOpenAIImagesComplexityLegacyBridgeAndAPIKey(t *testing.T) {
	body := []byte(`{"model":"gpt-image-1","prompt":"draw","complexity":"low","quality":"high","custom_option":{"keep":true}}`)
	for _, apiKey := range []bool{false, true} {
		t.Run(fmt.Sprint(apiKey), func(t *testing.T) {
			c, _ := newOpenAIImagesTestContext(t, body)
			upstream := &httpUpstreamRecorder{resp: openAIImagesJSONResponse()}
			svc := newOpenAIImagesTestService(upstream)
			parsed, err := svc.ParseOpenAIImagesRequest(c, body)
			require.NoError(t, err)
			if apiKey {
				_, err = svc.ForwardImages(context.Background(), c, newOpenAIImagesAPIKeyAccount(), body, parsed, "")
				require.NoError(t, err)
				require.JSONEq(t, string(body), string(upstream.lastBody))
			} else {
				payload, target, err := buildOpenAIImagesOAuthPayload(parsed, parsed.Model)
				require.NoError(t, err)
				require.Equal(t, chatgptCodexURL, target)
				require.Equal(t, "low", gjson.GetBytes(payload, "tools.0.complexity").String())
				require.Equal(t, "medium", gjson.GetBytes(payload, "reasoning.effort").String())
			}
		})
	}
	c, _ := newOpenAIImagesTestContext(t, []byte(`{"prompt":"draw","complexity":"low"}`))
	parsed, err := newOpenAIImagesTestService(nil).ParseOpenAIImagesRequest(c, []byte(`{"prompt":"draw","complexity":"low"}`))
	require.NoError(t, err)
	require.Equal(t, OpenAIImagesCapabilityNative, parsed.RequiredCapability)
}

func TestGPTImage26OutputFormats(t *testing.T) {
	for _, format := range []string{"gif", "svg"} {
		t.Run(format, func(t *testing.T) {
			body := []byte(fmt.Sprintf(`{"model":"gpt-image-2.6-flare","prompt":"draw","output_format":%q,"response_format":"url"}`, format))
			c, rec := newOpenAIImagesTestContext(t, body)
			response := fmt.Sprintf(`{"output_format":%q,"data":[{"b64_json":"AA=="}],"usage":{"input_tokens":10,"output_tokens":20}}`, format)
			upstream := &httpUpstreamRecorder{resp: &http.Response{StatusCode: 200, Header: http.Header{}, Body: io.NopCloser(strings.NewReader(response))}}
			svc := newOpenAIImagesTestService(upstream)
			parsed, err := svc.ParseOpenAIImagesRequest(c, body)
			require.NoError(t, err)
			result, err := svc.ForwardImages(context.Background(), c, directImagesTestAccount(), body, parsed, "")
			require.NoError(t, err)
			require.Equal(t, 1, result.ImageCount)
			mime := "image/gif"
			if format == "svg" {
				mime = "image/svg+xml"
			}
			require.Equal(t, "data:"+mime+";base64,AA==", gjson.GetBytes(rec.Body.Bytes(), "data.0.url").String())
		})
	}
	var buf bytes.Buffer
	require.NoError(t, gif.Encode(&buf, image.NewGray(image.Rect(0, 0, 64, 32)), nil))
	require.Equal(t, "64x32", detectOpenAIImageResultSize("data:image/gif;base64,"+base64.StdEncoding.EncodeToString(buf.Bytes())))
}

func TestGPTImage26NativeOptions(t *testing.T) {
	for _, options := range []string{
		`"output_format":"gif","frame_count":2,"frame_duration_ms":100`,
		`"output_format":"svg","svg":{"model":"gpt-6-astra","reasoning":{"effort":"high"},"ignored":true}`,
	} {
		body := []byte(`{"model":"gpt-image-2.6-sunburst","prompt":"draw",` + options + `}`)
		c, _ := newOpenAIImagesTestContext(t, body)
		parsed, err := newOpenAIImagesTestService(nil).ParseOpenAIImagesRequest(c, body)
		require.NoError(t, err)
		payload, _, err := buildOpenAIImagesOAuthPayload(parsed, parsed.Model)
		require.NoError(t, err)
		if parsed.OutputFormat == "gif" {
			require.EqualValues(t, 2, gjson.GetBytes(payload, "frame_count").Int())
			require.EqualValues(t, 100, gjson.GetBytes(payload, "frame_duration_ms").Int())
		} else {
			require.Equal(t, "gpt-6-astra", gjson.GetBytes(payload, "svg.model").String())
			require.Equal(t, "high", gjson.GetBytes(payload, "svg.reasoning.effort").String())
			require.False(t, gjson.GetBytes(payload, "svg.ignored").Exists())
		}
		_, err = buildOpenAIImagesResponsesRequest(parsed, parsed.Model)
		require.ErrorContains(t, err, "native Images endpoint", "native options must not disappear during a legacy fallback")
		upstream := &httpUpstreamRecorder{resp: openAIImagesJSONResponse()}
		_, err = newOpenAIImagesTestService(upstream).ForwardImages(context.Background(), c, newOpenAIImagesAPIKeyAccount(), body, parsed, "")
		require.NoError(t, err)
		require.JSONEq(t, string(body), string(upstream.lastBody), "API-key requests must retain the original options")
	}
	for _, multipartInput := range []bool{false, true} {
		t.Run(fmt.Sprintf("markup/multipart=%t", multipartInput), func(t *testing.T) {
			body := []byte(`{"model":"gpt-image-2.6-flare","prompt":"edit","complexity":"low","images":[{"image_url":"data:image/png;base64,aW1hZ2U="}],"markup":{"image_url":"data:image/png;base64,bWFya3Vw"}}`)
			contentType := "application/json"
			if multipartInput {
				var buf bytes.Buffer
				writer := multipart.NewWriter(&buf)
				for key, value := range map[string]string{"model": "gpt-image-2.6-flare", "prompt": "edit", "complexity": "low", "frame_count": "2", "frame_duration_ms": "100"} {
					require.NoError(t, writer.WriteField(key, value))
				}
				for _, name := range []string{"image", "markup"} {
					header := textproto.MIMEHeader{}
					header.Set("Content-Disposition", fmt.Sprintf(`form-data; name=%q; filename="input.png"`, name))
					header.Set("Content-Type", "image/png")
					part, err := writer.CreatePart(header)
					require.NoError(t, err)
					_, err = part.Write([]byte(name))
					require.NoError(t, err)
				}
				require.NoError(t, writer.Close())
				body, contentType = buf.Bytes(), writer.FormDataContentType()
			}
			c, _ := newOpenAIImagesTestContext(t, body)
			c.Request.URL.Path = openAIImagesEditsEndpoint
			c.Request.Header.Set("Content-Type", contentType)
			parsed, err := newOpenAIImagesTestService(nil).ParseOpenAIImagesRequest(c, body)
			require.NoError(t, err)
			require.Equal(t, OpenAIImagesCapabilityNative, parsed.RequiredCapability)
			payload, target, err := buildOpenAIImagesOAuthPayload(parsed, parsed.Model)
			require.NoError(t, err)
			require.Equal(t, "https://chatgpt.com/backend-api/codex/images/edits", target)
			require.Equal(t, "data:image/png;base64,bWFya3Vw", gjson.GetBytes(payload, "markup.image_url").String())
			require.Equal(t, "low", gjson.GetBytes(payload, "complexity").String())
			require.Len(t, gjson.GetBytes(parsed.ModerationBody(), "images").Array(), 2)
			_, err = buildOpenAIImagesResponsesRequest(parsed, parsed.Model)
			require.ErrorContains(t, err, "native Images endpoint")
		})
	}
}

func TestGPTImage26StreamedJSONEdits(t *testing.T) {
	for _, model := range []string{"gpt-image-2.6-flare", "gpt-image-2.6-sunburst"} {
		for _, disconnect := range []bool{false, true} {
			t.Run(fmt.Sprintf("%s/disconnect=%t", model, disconnect), func(t *testing.T) {
				body := []byte(fmt.Sprintf(`{"model":%q,"prompt":"edit","images":[{"image_url":"data:image/png;base64,AA=="}],"n":2,"stream":true,"response_format":"url"}`, model))
				c, rec := newOpenAIImagesTestContext(t, body)
				c.Request.URL.Path = openAIImagesEditsEndpoint
				if disconnect {
					c.Writer = &failingOpenAIImageWriter{ResponseWriter: c.Writer, failAfter: 1}
				}
				response := `{"created":1710000000,"size":"1024x1024","output_format":"png","data":[{"b64_json":"AA=="},{"b64_json":"AQ=="}],"usage":{"input_tokens":100,"input_tokens_details":{"image_tokens":80},"output_tokens":40}}`
				upstream := &httpUpstreamRecorder{resp: &http.Response{StatusCode: 200, Header: http.Header{"Content-Type": {"application/json"}}, Body: io.NopCloser(strings.NewReader(response))}}
				svc := newOpenAIImagesTestService(upstream)
				parsed, err := svc.ParseOpenAIImagesRequest(c, body)
				require.NoError(t, err)
				result, err := svc.ForwardImages(context.Background(), c, directImagesTestAccount(), body, parsed, "")
				require.NoError(t, err)
				require.Equal(t, 2, result.ImageCount)
				require.Equal(t, 40, result.Usage.ImageOutputTokens)
				require.Equal(t, 80, result.Usage.ImageInputTokens)
				require.Len(t, upstream.requests, 1)
				if !disconnect {
					require.Equal(t, "text/event-stream", rec.Header().Get("Content-Type"))
					require.Equal(t, 2, strings.Count(rec.Body.String(), "event: image_edit.completed"))
					require.Equal(t, 1, strings.Count(rec.Body.String(), `"usage"`))
					require.Contains(t, rec.Body.String(), "data:image/png;base64,AQ==")
				}
			})
		}
	}
	for _, response := range []string{`{}`, `{"data":[]}`, `{"data":[{"b64_json":""}]}`, `{"error":{"type":"invalid_request_error","message":"bad image"}}`, `not json`} {
		t.Run(response, func(t *testing.T) {
			body := []byte(`{"model":"gpt-image-2.6-flare","prompt":"draw","stream":true}`)
			c, rec := newOpenAIImagesTestContext(t, body)
			upstream := &httpUpstreamRecorder{resp: &http.Response{StatusCode: 200, Header: http.Header{"Content-Type": {"application/json"}}, Body: io.NopCloser(strings.NewReader(response))}}
			svc := newOpenAIImagesTestService(upstream)
			parsed, err := svc.ParseOpenAIImagesRequest(c, body)
			require.NoError(t, err)
			result, err := svc.ForwardImages(context.Background(), c, directImagesTestAccount(), body, parsed, "")
			require.Error(t, err)
			require.Nil(t, result)
			require.NotContains(t, rec.Body.String(), "event: image_generation.completed")
		})
	}
}

func TestGPTImage26InvalidNativeOptions(t *testing.T) {
	for _, options := range []string{
		`"frame_count":"2"`, `"frame_count":2.5`, `"frame_duration_ms":true`,
		`"svg":[]`, `"svg":{"reasoning":"high"}`,
		`"markup":[]`, `"markup":{"file_id":"file-1"}`, `"markup":{}`,
	} {
		t.Run(options, func(t *testing.T) {
			body := []byte(`{"model":"gpt-image-2.6-flare","prompt":"edit","images":[{"image_url":"data:image/png;base64,AA=="}],` + options + `}`)
			c, _ := newOpenAIImagesTestContext(t, body)
			c.Request.URL.Path = openAIImagesEditsEndpoint
			_, err := newOpenAIImagesTestService(nil).ParseOpenAIImagesRequest(c, body)
			require.Error(t, err)
		})
	}
}

func TestGPTImage26BillingWithoutDynamicPricing(t *testing.T) {
	svc := &BillingService{}
	for _, model := range []string{"gpt-image-2.6-flare", "gpt-image-2.6-sunburst"} {
		p, err := svc.GetModelPricing(model)
		require.NoError(t, err)
		require.Equal(t, 5e-6, p.InputPricePerToken)
		require.Equal(t, 1.25e-6, p.CacheReadPricePerToken)
		require.Equal(t, 8e-6, p.ImageInputPricePerToken)
		require.Equal(t, 2e-6, p.ImageCacheReadPricePerToken)
		require.Equal(t, 30e-6, p.ImageOutputPricePerToken)
		require.Zero(t, p.OutputPricePerToken)
	}
}
