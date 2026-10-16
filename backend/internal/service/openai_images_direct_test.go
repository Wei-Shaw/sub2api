package service

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

func directImagesTestAccount() *Account {
	return &Account{ID: 35, Platform: PlatformOpenAI, Type: AccountTypeOAuth,
		Credentials: map[string]any{"access_token": "test-token", "chatgpt_account_id": "test-account"}}
}

func TestCodexDirectImagesRouting(t *testing.T) {
	for _, model := range []string{"gpt-image-1.5", "gpt-image-2", "gpt-image-2.5-flare", "gpt-image-2.5-sunburst", "gpt-image-2.5-flare-2026-09-08", "gpt-image-2.5-sunburst-2026-09-08"} {
		t.Run(model, func(t *testing.T) {
			body := []byte(fmt.Sprintf(`{"model":%q,"prompt":"  原样保留 prompt  ","quality":"max","size":"auto","response_format":"url","extra":{"preserve":true}}`, model))
			c, rec := newOpenAIImagesTestContext(t, body)
			upstream := &httpUpstreamRecorder{resp: openAIImagesJSONResponse()}
			svc := newOpenAIImagesTestService(upstream)
			parsed, err := svc.ParseOpenAIImagesRequest(c, body)
			require.NoError(t, err)
			ctx, cancel := context.WithCancel(context.Background())
			cancel()
			result, err := svc.ForwardImages(ctx, c, directImagesTestAccount(), body, parsed, "")
			require.NoError(t, err)
			require.Equal(t, 1, result.ImageCount)
			require.Equal(t, 20, result.Usage.ImageOutputTokens)
			require.Equal(t, model, result.UpstreamModel)
			require.Equal(t, "/backend-api/codex/images/generations", result.UpstreamEndpoint)
			require.Equal(t, "https://chatgpt.com/backend-api/codex/images/generations", upstream.lastReq.URL.String())
			require.NoError(t, upstream.lastReq.Context().Err())
			require.Equal(t, "Bearer test-token", upstream.lastReq.Header.Get("Authorization"))
			require.Equal(t, "application/json", upstream.lastReq.Header.Get("Accept"))
			require.Empty(t, upstream.lastReq.Header.Get("OpenAI-Beta"))
			require.NotEmpty(t, upstream.lastReq.Header.Get("Originator"))
			require.NotEmpty(t, upstream.lastReq.Header.Get("User-Agent"))
			require.Equal(t, model, gjson.GetBytes(upstream.lastBody, "model").String())
			require.Equal(t, "  原样保留 prompt  ", gjson.GetBytes(upstream.lastBody, "prompt").String())
			require.False(t, gjson.GetBytes(upstream.lastBody, "extra").Exists())
			for _, key := range []string{"tools", "reasoning", "instructions", "response_format", "stream"} {
				require.False(t, gjson.GetBytes(upstream.lastBody, key).Exists(), key)
			}
			require.Equal(t, "data:image/png;base64,aGVsbG8=", gjson.GetBytes(rec.Body.Bytes(), "data.0.url").String())
		})
	}
}

func TestCodexDirectImagesMappingBeforeRouting(t *testing.T) {
	for _, accountType := range []string{AccountTypeOAuth, AccountTypeSetupToken} {
		t.Run(accountType, func(t *testing.T) {
			body := []byte(`{"model":"gpt-image-1","prompt":"draw"}`)
			c, _ := newOpenAIImagesTestContext(t, body)
			upstream := &httpUpstreamRecorder{resp: openAIImagesJSONResponse()}
			svc := newOpenAIImagesTestService(upstream)
			parsed, err := svc.ParseOpenAIImagesRequest(c, body)
			require.NoError(t, err)
			account := directImagesTestAccount()
			account.Type = accountType
			account.Credentials["model_mapping"] = map[string]any{"gpt-image-2": "gpt-image-2.5-flare"}
			result, err := svc.ForwardImages(context.Background(), c, account, body, parsed, "gpt-image-2")
			require.NoError(t, err)
			require.Equal(t, "gpt-image-2.5-flare", result.UpstreamModel)
			require.Equal(t, "gpt-image-2", result.Model)
			require.Equal(t, "gpt-image-2.5-flare", gjson.GetBytes(upstream.lastBody, "model").String())
		})
	}
	for _, model := range []string{"gpt-image-1", "gpt-image-future"} {
		body, target, err := buildOpenAIImagesOAuthPayload(&OpenAIImagesRequest{Prompt: "draw"}, model)
		require.NoError(t, err)
		require.Equal(t, chatgptCodexURL, target)
		require.Equal(t, "gpt-5.6-luna", gjson.GetBytes(body, "model").String())
		require.Equal(t, model, gjson.GetBytes(body, "tools.0.model").String())
	}
}

func TestCodexDirectImagesHTTPErrorFallbacksOnlyWhenEndpointUnavailable(t *testing.T) {
	for _, status := range []int{400, 401, 403, 429, 500, 502, 503, 404, 405} {
		t.Run(fmt.Sprint(status), func(t *testing.T) {
			calls := 0
			upstream := &codexModelsHTTPUpstreamStub{do: func(req *http.Request, _ string, _ int64, _ int) (*http.Response, error) {
				calls++
				if calls == 1 {
					require.Equal(t, "/backend-api/codex/images/generations", req.URL.Path)
					return &http.Response{StatusCode: status, Header: http.Header{}, Body: io.NopCloser(strings.NewReader(`{"error":{"type":"server_error","message":"image request rejected"}}`))}, nil
				}
				require.Contains(t, req.URL.Path, "/backend-api/codex/responses")
				return &http.Response{StatusCode: 200, Header: http.Header{"Content-Type": {"text/event-stream"}}, Body: io.NopCloser(strings.NewReader("data: {\"type\":\"response.completed\",\"response\":{\"output\":[{\"type\":\"image_generation_call\",\"result\":\"aGVsbG8=\"}]}}\n\n"))}, nil
			}}
			body := []byte(`{"model":"gpt-image-2.5-sunburst","prompt":"draw"}`)
			c, _ := newOpenAIImagesTestContext(t, body)
			svc := newOpenAIImagesTestService(upstream)
			parsed, err := svc.ParseOpenAIImagesRequest(c, body)
			require.NoError(t, err)
			result, err := svc.ForwardImages(context.Background(), c, directImagesTestAccount(), body, parsed, "")
			if status == http.StatusNotFound || status == http.StatusMethodNotAllowed {
				require.NoError(t, err)
				require.NotNil(t, result)
				require.Equal(t, 2, calls)
			} else {
				require.Error(t, err)
				require.Equal(t, 1, calls)
			}
		})
	}
}

// The native edits endpoint ignores stream=true and answers with one JSON
// document; it must reach the client as the stream it asked for.
func TestCodexDirectImagesStreamConvertsPlainJSON(t *testing.T) {
	for _, test := range []struct {
		name, path, body, wantEvent, wantEndpoint string
	}{
		{"generations", "/v1/images/generations", `{"model":"gpt-image-2","prompt":"draw","stream":true}`, "image_generation.completed", "/backend-api/codex/images/generations"},
		{"edits", "/v1/images/edits", `{"model":"gpt-image-2","prompt":"edit","images":[{"image_url":"data:image/png;base64,AA=="}],"stream":true,"response_format":"url"}`, "image_edit.completed", "/backend-api/codex/images/edits"},
	} {
		t.Run(test.name, func(t *testing.T) {
			c, rec := newOpenAIImagesTestContext(t, []byte(test.body))
			c.Request.URL.Path = test.path
			// The live endpoint pretty-prints its JSON; every SSE data payload
			// must still fit on a single line.
			resp := openAIImagesJSONResponse()
			resp.Body = io.NopCloser(strings.NewReader("{\n  \"created\": 1710000000,\n  \"data\": [\n    {\n      \"b64_json\": \"aGVsbG8=\"\n    }\n  ],\n  \"usage\": {\n    \"input_tokens\": 10,\n    \"output_tokens\": 20,\n    \"total_tokens\": 30\n  }\n}\n"))
			upstream := &httpUpstreamRecorder{resp: resp}
			svc := newOpenAIImagesTestService(upstream)
			parsed, err := svc.ParseOpenAIImagesRequest(c, []byte(test.body))
			require.NoError(t, err)
			result, err := svc.ForwardImages(context.Background(), c, directImagesTestAccount(), []byte(test.body), parsed, "")
			require.NoError(t, err)
			require.NotNil(t, result)
			require.Equal(t, 1, result.ImageCount)
			require.Equal(t, 20, result.Usage.ImageOutputTokens)
			require.NotNil(t, result.FirstTokenMs)
			require.Equal(t, test.wantEndpoint, upstream.lastReq.URL.Path)

			require.Equal(t, http.StatusOK, rec.Code)
			require.Contains(t, rec.Header().Get("Content-Type"), "text/event-stream")
			out := rec.Body.String()
			require.Contains(t, out, "event: "+test.wantEvent+"\n")
			for _, line := range strings.Split(strings.TrimRight(out, "\n"), "\n") {
				require.True(t, line == "" || strings.HasPrefix(line, "event: ") || strings.HasPrefix(line, "data: "), "SSE line split mid-payload: %q", line)
			}
			dataLine := out[strings.Index(out, "data: ")+len("data: "):]
			dataLine = dataLine[:strings.Index(dataLine, "\n")]
			require.Equal(t, test.wantEvent, gjson.Get(dataLine, "type").String())
			require.Equal(t, "aGVsbG8=", gjson.Get(dataLine, "b64_json").String())
			require.Equal(t, "gpt-image-2", gjson.Get(dataLine, "model").String())
			require.Equal(t, int64(1710000000), gjson.Get(dataLine, "created_at").Int())
			require.Equal(t, int64(20), gjson.Get(dataLine, "usage.output_tokens").Int())
			if parsed.ResponseFormat == "url" {
				require.Equal(t, "data:image/png;base64,aGVsbG8=", gjson.Get(dataLine, "url").String())
			}
		})
	}
}

// A JSON error body must stay retryable: nothing may reach the client before
// the failover decision is made.
func TestCodexDirectImagesStreamPlainJSONErrorWritesNothing(t *testing.T) {
	for _, test := range []struct {
		name, response string
		limit          int64
		wantFailover   bool
	}{
		{"empty data", `{"data":[]}`, 0, true},
		{"upstream error with image", `{"error":{"type":"server_error","message":"temporary failure"},"data":[{"b64_json":"aGVsbG8="}]}`, 0, true},
		{"malformed after image", `{"data":[{"b64_json":"aGVsbG8="}],`, 0, false},
		{"read error after image", `{"data":[{"b64_json":"aGVsbG8="}]}`, 0, true},
		{"oversized", `{"data":[{"b64_json":"aGVsbG8="}]}`, 8, false},
	} {
		t.Run(test.name, func(t *testing.T) {
			body := []byte(`{"model":"gpt-image-2","prompt":"draw","stream":true}`)
			c, rec := newOpenAIImagesTestContext(t, body)
			resp := openAIImagesJSONResponse()
			resp.Body = io.NopCloser(strings.NewReader(test.response))
			if test.name == "read error after image" {
				resp.Body = io.NopCloser(io.MultiReader(strings.NewReader(test.response), &openAIImagesReadErrorBody{err: io.ErrUnexpectedEOF}))
			}
			upstream := &httpUpstreamRecorder{resp: resp}
			svc := newOpenAIImagesTestService(upstream)
			svc.cfg.Gateway.UpstreamResponseReadMaxBytes = test.limit
			parsed, err := svc.ParseOpenAIImagesRequest(c, body)
			require.NoError(t, err)
			result, err := svc.ForwardImages(context.Background(), c, directImagesTestAccount(), body, parsed, "")
			require.Error(t, err)
			require.Nil(t, result)
			require.Empty(t, rec.Body.String())
			require.False(t, c.Writer.Written(), "even headers must remain uncommitted")
			require.Equal(t, -1, c.Writer.Size())
			require.False(t, rec.Flushed)
			var failover *UpstreamFailoverError
			if test.wantFailover {
				require.ErrorAs(t, err, &failover)
				require.Equal(t, http.StatusBadGateway, failover.StatusCode)
				// Reuse the same client writer, as the handler does on failover.
				upstream.resp = openAIImagesJSONResponse()
				result, err = svc.ForwardImages(context.Background(), c, directImagesTestAccount(), body, parsed, "")
				require.NoError(t, err)
				require.Equal(t, 1, result.ImageCount)
				require.Equal(t, 1, strings.Count(rec.Body.String(), "event: image_generation.completed\n"))
			} else {
				require.False(t, errors.As(err, &failover))
			}
		})
	}
}

func TestCodexDirectImagesStreamMultipleOutputs(t *testing.T) {
	body := []byte(`{"model":"gpt-image-2","prompt":"draw","stream":true,"n":2}`)
	c, rec := newOpenAIImagesTestContext(t, body)
	resp := openAIImagesJSONResponse()
	resp.Header.Set("Content-Length", "999")
	resp.Body = io.NopCloser(strings.NewReader(`{"created":1710000000,"data":[{"b64_json":"aGVsbG8="},{"b64_json":"d29ybGQ="}],"usage":{"output_tokens":20}}`))
	svc := newOpenAIImagesTestService(&httpUpstreamRecorder{resp: resp})
	parsed, err := svc.ParseOpenAIImagesRequest(c, body)
	require.NoError(t, err)
	result, err := svc.ForwardImages(context.Background(), c, directImagesTestAccount(), body, parsed, "")
	require.NoError(t, err)
	require.Equal(t, 2, result.ImageCount)
	require.Equal(t, 20, result.Usage.ImageOutputTokens)
	require.Empty(t, rec.Header().Get("Content-Length"))
	frames := strings.Split(strings.TrimSuffix(rec.Body.String(), "\n\n"), "\n\n")
	require.Len(t, frames, 2)
	for i, frame := range frames {
		lines := strings.Split(frame, "\n")
		require.Len(t, lines, 2)
		require.Equal(t, "event: image_generation.completed", lines[0])
		require.True(t, strings.HasPrefix(lines[1], "data: "))
		payload := strings.TrimPrefix(lines[1], "data: ")
		require.True(t, gjson.Valid(payload))
		require.Equal(t, "image_generation.completed", gjson.Get(payload, "type").String())
		require.Equal(t, []string{"aGVsbG8=", "d29ybGQ="}[i], gjson.Get(payload, "b64_json").String())
	}
}

func TestCodexDirectImagesMultipleOutputs(t *testing.T) {
	for _, stream := range []bool{false, true} {
		t.Run(fmt.Sprint(stream), func(t *testing.T) {
			body := []byte(fmt.Sprintf(`{"model":"gpt-image-2.5-flare","prompt":"draw","n":2,"stream":%t}`, stream))
			c, _ := newOpenAIImagesTestContext(t, body)
			response := `{"data":[{"b64_json":"AA=="},{"b64_json":"AQ=="}],"usage":{"input_tokens":10,"output_tokens":40}}`
			if stream {
				response = "data: {\"type\":\"image_generation.completed\",\"b64_json\":\"AA==\"}\n\ndata: {\"type\":\"image_generation.completed\",\"b64_json\":\"AQ==\",\"usage\":{\"input_tokens\":10,\"output_tokens\":40}}\n\n"
			}
			upstream := &httpUpstreamRecorder{resp: &http.Response{StatusCode: 200, Header: http.Header{}, Body: io.NopCloser(strings.NewReader(response))}}
			svc := newOpenAIImagesTestService(upstream)
			parsed, err := svc.ParseOpenAIImagesRequest(c, body)
			require.NoError(t, err)
			result, err := svc.ForwardImages(context.Background(), c, directImagesTestAccount(), body, parsed, "")
			require.NoError(t, err)
			require.Equal(t, 2, result.ImageCount)
			require.Equal(t, 40, result.Usage.ImageOutputTokens)
		})
	}
}

func TestCodexDirectImagesStreaming(t *testing.T) {
	for _, test := range []struct {
		name, events          string
		wantCount             int
		wantError, disconnect bool
	}{
		{"completed", "complete", 1, false, false},
		{"duplicate", "complete,complete", 1, false, false},
		{"disconnect", "partial,complete", 1, false, true},
		{"truncated", "partial", 0, true, false},
		{"partial_requested_multiple", "complete", 1, false, false},
		{"error", "error", 0, true, false},
		{"partial_success", "complete,error", 1, true, false},
	} {
		t.Run(test.name, func(t *testing.T) {
			body := []byte(`{"model":"gpt-image-2","prompt":"edit","images":[{"image_url":"data:image/png;base64,AA=="}],"stream":true,"response_format":"url"}`)
			if test.name == "partial_requested_multiple" {
				body = []byte(`{"model":"gpt-image-2","prompt":"edit","n":2,"images":[{"image_url":"data:image/png;base64,AA=="}],"stream":true,"response_format":"url"}`)
			}
			c, rec := newOpenAIImagesTestContext(t, body)
			c.Request.URL.Path = "/v1/images/edits"
			if test.disconnect {
				c.Writer = &failingOpenAIImageWriter{ResponseWriter: c.Writer, failAfter: 1}
			}
			frames := map[string]string{
				"partial":  `{"type":"image_generation.partial_image","b64_json":"AA==","partial_image_index":0}`,
				"complete": `{"type":"image_generation.completed","model":"gpt-image-2-codex","size":"1024x1024","b64_json":"aGVsbG8=","output_format":"webp","usage":{"input_tokens":10,"output_tokens":20}}`,
				"error":    `{"type":"error","error":{"type":"server_error","message":"overloaded"}}`,
			}
			var stream strings.Builder
			for _, event := range strings.Split(test.events, ",") {
				fmt.Fprintf(&stream, "data: %s\n\n", frames[event])
			}
			upstream := &httpUpstreamRecorder{resp: &http.Response{StatusCode: 200, Header: http.Header{"Content-Type": {"text/event-stream"}}, Body: io.NopCloser(strings.NewReader(stream.String()))}}
			svc := newOpenAIImagesTestService(upstream)
			parsed, err := svc.ParseOpenAIImagesRequest(c, body)
			require.NoError(t, err)
			result, err := svc.ForwardImages(context.Background(), c, directImagesTestAccount(), body, parsed, "")
			if test.wantError {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
			}
			if test.wantCount > 0 {
				require.NotNil(t, result)
				require.Equal(t, test.wantCount, result.ImageCount)
				require.Equal(t, 20, result.Usage.ImageOutputTokens)
				if !test.wantError {
					require.Equal(t, "gpt-image-2-codex", result.UpstreamResponseModel)
				}
			}
			require.Equal(t, "/backend-api/codex/images/edits", upstream.lastReq.URL.Path)
			require.Equal(t, "text/event-stream", upstream.lastReq.Header.Get("Accept"))
			if !test.disconnect && test.wantCount > 0 {
				require.Contains(t, rec.Body.String(), "event: image_edit.completed")
				require.Contains(t, rec.Body.String(), "data:image/webp;base64,aGVsbG8=")
			}
		})
	}
}

func TestCodexDirectImagesEmptyResponseFails(t *testing.T) {
	for _, body := range []string{`{"data":[]}`, `{"data":[{"b64_json":""}]}`, `{}`, `{"error":{"message":"overloaded","type":"server_error"}}`, `not json`} {
		_, err := parseCodexDirectImagesResponse([]byte(body))
		require.Error(t, err)
	}
}

func TestCodexDirectImagesAccountTestAndWhitelist(t *testing.T) {
	body := []byte(`{"model":"gpt-image-2.5-sunburst","prompt":"draw"}`)
	c, rec := newOpenAIImagesTestContext(t, body)
	upstream := &httpUpstreamRecorder{resp: openAIImagesJSONResponse()}
	svc := &AccountTestService{httpUpstream: upstream}
	require.NoError(t, svc.testOpenAIImageOAuth(c, context.Background(), directImagesTestAccount(), "gpt-image-2.5-sunburst", "draw"))
	require.Equal(t, "/backend-api/codex/images/generations", upstream.lastReq.URL.Path)
	require.Contains(t, rec.Body.String(), `"success":true`)
	newCodexModelsOAuthCacheServer(t, `{"models":[{"slug":"gpt-5.6-luna"}]}`)
	svc.openaiGatewayService = &OpenAIGatewayService{}
	account := newCodexModelsTestAccount()
	account.Credentials["model_mapping"] = map[string]any{"gpt-image-2.5-flare": "gpt-image-2.5-flare"}
	models, err := svc.FetchOpenAIAccountModels(context.Background(), account)
	require.NoError(t, err)
	for _, model := range models {
		require.NotEqual(t, "gpt-image-2.5-sunburst", model.ID)
	}
}
