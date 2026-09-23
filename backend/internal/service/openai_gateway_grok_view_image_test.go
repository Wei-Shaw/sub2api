//go:build unit

package service

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/xai"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

func TestGrokModelNeedsImageTextBridge(t *testing.T) {
	t.Parallel()

	require.True(t, grokModelNeedsImageTextBridge("grok-composer-2.5-fast"))
	require.True(t, grokModelNeedsImageTextBridge("grok-4.6-build"))
	require.True(t, grokModelNeedsImageTextBridge("grok/grok-4.6-build"))
	require.False(t, grokModelNeedsImageTextBridge("grok-build-0.1"))
	require.False(t, grokModelNeedsImageTextBridge(grokComposerImageBridgeVisionModel))
	require.False(t, grokModelNeedsImageTextBridge("grok-4.6"))
	require.False(t, grokModelNeedsImageTextBridge("grok-4.5"))
	require.False(t, grokModelNeedsImageTextBridge(""))
}

func TestShouldBridgeGrokComposerImageInputs_ResponsesBuildModel(t *testing.T) {
	t.Parallel()

	body := []byte(`{
		"model":"grok-4.6-build",
		"input":[{"type":"message","role":"user","content":[
			{"type":"input_text","text":"what is this"},
			{"type":"input_image","image_url":"data:image/png;base64,QUJD"}
		]}]
	}`)
	require.True(t, shouldBridgeGrokComposerImageInputs(body))

	visionProbe := []byte(`{
		"model":"grok-build-0.1",
		"input":[{"type":"message","role":"user","content":[
			{"type":"input_image","image_url":"data:image/png;base64,QUJD"}
		]}]
	}`)
	require.False(t, shouldBridgeGrokComposerImageInputs(visionProbe))
}

func TestRewriteGrokResponsesInputImagesAsText(t *testing.T) {
	t.Parallel()

	req := map[string]any{
		"model": "grok-4.6-build",
		"input": []any{
			map[string]any{
				"type": "message",
				"role": "user",
				"content": []any{
					map[string]any{"type": "input_text", "text": "What is shown?"},
					map[string]any{"type": "input_image", "image_url": "data:image/png;base64,QUJD"},
				},
			},
		},
	}
	require.Equal(t, []string{"data:image/png;base64,QUJD"}, collectGrokComposerImageURLs(req))
	require.True(t, rewriteGrokComposerImagesAsText(req, []string{"A red pixel."}))
	content := req["input"].([]any)[0].(map[string]any)["content"].([]any)
	require.Len(t, content, 2)
	require.Equal(t, "input_text", content[0].(map[string]any)["type"])
	require.Equal(t, "What is shown?", content[0].(map[string]any)["text"])
	require.Equal(t, "input_text", content[1].(map[string]any)["type"])
	require.Contains(t, content[1].(map[string]any)["text"], "Image 1 description: A red pixel.")
}

func TestAliasGrokReservedClientToolNamesRewritesViewImage(t *testing.T) {
	t.Parallel()

	body := []byte(`{
		"model":"grok-4.6",
		"tools":[{"type":"function","name":"view_image","parameters":{"type":"object","properties":{"path":{"type":"string"}},"required":["path"]}}],
		"tool_choice":{"type":"function","name":"view_image"},
		"input":[
			{"type":"function_call","call_id":"call_1","name":"view_image","arguments":"{\"path\":\"/tmp/a.png\"}"},
			{"type":"function_call_output","call_id":"call_1","output":"ok"}
		]
	}`)
	aliased, reverse, err := aliasGrokReservedClientToolNamesBody(body)
	require.NoError(t, err)
	require.Equal(t, map[string]string{"client_view_image": "view_image"}, reverse)
	require.Equal(t, "client_view_image", gjson.GetBytes(aliased, "tools.0.name").String())
	require.Equal(t, "client_view_image", gjson.GetBytes(aliased, "tool_choice.name").String())
	require.Equal(t, "client_view_image", gjson.GetBytes(aliased, "input.0.name").String())
	require.Equal(t, "function_call_output", gjson.GetBytes(aliased, "input.1.type").String())
}

func TestAliasGrokReservedClientToolNamesSkipsWhenAliasOccupied(t *testing.T) {
	t.Parallel()

	body := []byte(`{
		"tools":[
			{"type":"function","name":"view_image","parameters":{"type":"object"}},
			{"type":"function","name":"client_view_image","parameters":{"type":"object"}}
		]
	}`)
	aliased, reverse, err := aliasGrokReservedClientToolNamesBody(body)
	require.NoError(t, err)
	require.Empty(t, reverse)
	require.Equal(t, "view_image", gjson.GetBytes(aliased, "tools.0.name").String())
}

func TestForwardGrokResponsesAliasesViewImageAndRestoresClientName(t *testing.T) {
	gin.SetMode(gin.TestMode)

	body := []byte(`{
		"model":"grok-4.6","stream":false,
		"tools":[{"type":"function","name":"view_image","parameters":{"type":"object","properties":{"path":{"type":"string"}},"required":["path"]}}],
		"input":[{"type":"message","role":"user","content":[{"type":"input_text","text":"Inspect /tmp/a.png"}]}]
	}`)
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", bytes.NewReader(body))
	c.Request.Header.Set("Content-Type", "application/json")
	c.Set("api_key", &APIKey{ID: 8101})

	account := grokProtocolAPIKeyAccount(8101)
	repo := &grokQuotaAccountRepo{mockAccountRepoForPlatform: &mockAccountRepoForPlatform{
		accountsByID: map[int64]*Account{account.ID: account},
	}}
	upstream := &httpUpstreamRecorder{resp: &http.Response{
		StatusCode: http.StatusOK,
		Header:     http.Header{"Content-Type": []string{"application/json"}},
		Body: io.NopCloser(strings.NewReader(`{
			"id":"resp_view","object":"response","model":"grok-4.6","status":"completed",
			"output":[{"type":"function_call","id":"fc_1","call_id":"call_1","name":"client_view_image","arguments":"{\"path\":\"/tmp/a.png\"}"}],
			"usage":{"input_tokens":4,"output_tokens":2,"total_tokens":6}
		}`)),
	}}
	svc := &OpenAIGatewayService{
		httpUpstream:      upstream,
		grokTokenProvider: NewGrokTokenProvider(repo, nil),
		accountRepo:       repo,
	}

	result, err := svc.forwardGrokResponses(context.Background(), c, account, body, "grok-4.6", false, time.Now())
	require.NoError(t, err)
	require.NotNil(t, result)
	require.Equal(t, "client_view_image", gjson.GetBytes(upstream.lastBody, "tools.0.name").String())
	require.Equal(t, "view_image", gjson.GetBytes(recorder.Body.Bytes(), "output.0.name").String())
	require.Equal(t, "function_call", gjson.GetBytes(recorder.Body.Bytes(), "output.0.type").String())
}

func TestForwardGrokResponsesBridgesBuildModelInputImages(t *testing.T) {
	gin.SetMode(gin.TestMode)

	body := []byte(`{
		"model":"grok-4.6-build","stream":false,
		"input":[{"type":"message","role":"user","content":[
			{"type":"input_text","text":"What is shown?"},
			{"type":"input_image","image_url":"data:image/png;base64,QUJD"}
		]}]
	}`)
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", bytes.NewReader(body))
	c.Request.Header.Set("Content-Type", "application/json")
	c.Set("api_key", &APIKey{ID: 8102})

	account := grokProtocolOAuthAccount(8102)
	account.Credentials["model_mapping"] = map[string]any{"grok-4.6-build": "grok-4.6-build"}
	repo := &grokQuotaAccountRepo{mockAccountRepoForPlatform: &mockAccountRepoForPlatform{
		accountsByID: map[int64]*Account{account.ID: account},
	}}
	upstream := &httpUpstreamRecorder{responses: []*http.Response{
		{
			StatusCode: http.StatusOK,
			Header:     http.Header{"Content-Type": []string{"application/json"}},
			Body:       io.NopCloser(strings.NewReader(`{"id":"resp_vision","object":"response","model":"grok-build-0.1","output":[{"type":"message","role":"assistant","content":[{"type":"output_text","text":"A tiny ABC diagram."}]}],"usage":{"input_tokens":8,"output_tokens":3,"total_tokens":11}}`)),
		},
		{
			StatusCode: http.StatusOK,
			Header:     http.Header{"Content-Type": []string{"application/json"}},
			Body:       io.NopCloser(strings.NewReader(`{"id":"resp_build","object":"response","model":"grok-4.6-build","status":"completed","output":[{"type":"message","role":"assistant","content":[{"type":"output_text","text":"It shows ABC."}]}],"usage":{"input_tokens":5,"output_tokens":4,"total_tokens":9}}`)),
		},
	}}
	svc := &OpenAIGatewayService{
		httpUpstream:      upstream,
		grokTokenProvider: NewGrokTokenProvider(repo, nil),
		accountRepo:       repo,
	}

	result, err := svc.forwardGrokResponses(context.Background(), c, account, body, "grok-4.6-build", false, time.Now())
	require.NoError(t, err)
	require.NotNil(t, result)
	require.Len(t, upstream.requests, 2)
	require.Equal(t, "grok-build-0.1", gjson.GetBytes(upstream.bodies[0], "model").String())
	require.Equal(t, "input_image", gjson.GetBytes(upstream.bodies[0], "input.0.content.1.type").String())
	require.Equal(t, xai.DefaultCLIBaseURL+"/responses", upstream.requests[1].URL.String())
	require.False(t, strings.Contains(string(upstream.bodies[1]), "input_image"))
	require.Contains(t, gjson.GetBytes(upstream.bodies[1], "input.0.content.1.text").String(), "Image 1 description: A tiny ABC diagram.")
	require.Equal(t, "It shows ABC.", gjson.Get(recorder.Body.String(), "output.0.content.0.text").String())
	require.Equal(t, 13, result.Usage.InputTokens)
	require.Equal(t, 7, result.Usage.OutputTokens)
}

func TestGrokComposerImageURLFromPartAcceptsResponsesInputImage(t *testing.T) {
	t.Parallel()

	require.Equal(t, "data:image/png;base64,QQ==", grokComposerImageURLFromPart(map[string]any{
		"type": "input_image", "image_url": "data:image/png;base64,QQ==",
	}))
	require.Equal(t, "https://example.com/a.png", grokComposerImageURLFromPart(map[string]any{
		"type": "image_url", "image_url": map[string]any{"url": "https://example.com/a.png"},
	}))
	require.Equal(t, "", grokComposerImageURLFromPart(map[string]any{
		"type": "input_text", "text": "hi",
	}))
}
