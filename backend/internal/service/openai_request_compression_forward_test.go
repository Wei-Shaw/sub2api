package service

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/gin-gonic/gin"
	"github.com/klauspost/compress/zstd"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

func TestOpenAIGatewayService_ForwardRequestCompressionFallbackRebuildsPlainRequest(t *testing.T) {
	upstream := &httpUpstreamRecorder{responses: []*http.Response{
		newOpenAIRequestCompressionResponse(http.StatusUnsupportedMediaType, `{"error":{"code":"unsupported_content_encoding","message":"Content-Encoding zstd is not supported"}}`),
		newOpenAIRequestCompressionStreamingResponse("resp_fallback_ok"),
	}}
	svc, c, account, body := newOpenAIRequestCompressionForwardFixture(upstream)

	result, err := svc.Forward(context.Background(), c, account, body)

	require.NoError(t, err)
	require.NotNil(t, result)
	require.Len(t, upstream.requests, 2)
	require.Equal(t, "zstd", upstream.requests[0].Header.Get("Content-Encoding"))
	require.Empty(t, upstream.requests[1].Header.Get("Content-Encoding"))
	require.Equal(t, "application/json", upstream.requests[0].Header.Get("Content-Type"))
	require.Equal(t, upstream.bodies[1], decodeOpenAIRequestCompressionBody(t, upstream.bodies[0]))
	require.Equal(t, int64(len(upstream.bodies[0])), upstream.requests[0].ContentLength)
	require.Equal(t, int64(len(upstream.bodies[1])), upstream.requests[1].ContentLength)
	require.Nil(t, upstream.requests[0].GetBody)
	require.NotNil(t, upstream.requests[1].GetBody)
	require.True(t, gjson.GetBytes(upstream.bodies[1], "stream").Bool())
	require.False(t, gjson.GetBytes(upstream.bodies[1], "store").Bool())
}

func TestOpenAIGatewayService_ForwardRequestCompressionDoesNotFallbackOnBusiness400(t *testing.T) {
	upstream := &httpUpstreamRecorder{responses: []*http.Response{
		newOpenAIRequestCompressionResponse(http.StatusBadRequest, `{"error":{"code":"invalid_request_error","message":"Invalid tool schema","param":"tools"}}`),
	}}
	svc, c, account, body := newOpenAIRequestCompressionForwardFixture(upstream)

	result, err := svc.Forward(context.Background(), c, account, body)

	require.Error(t, err)
	require.Nil(t, result)
	require.Len(t, upstream.requests, 1)
	require.Equal(t, "zstd", upstream.requests[0].Header.Get("Content-Encoding"))
}

func TestOpenAIGatewayService_ForwardRequestCompressionFallbackCanBeDisabled(t *testing.T) {
	upstream := &httpUpstreamRecorder{responses: []*http.Response{
		newOpenAIRequestCompressionResponse(http.StatusUnsupportedMediaType, `{"error":{"code":"unsupported_content_encoding","message":"Content-Encoding zstd is not supported"}}`),
	}}
	svc, c, account, body := newOpenAIRequestCompressionForwardFixture(upstream)
	svc.cfg.Gateway.OpenAIRequestCompression.FallbackUncompressed = false

	result, err := svc.Forward(context.Background(), c, account, body)

	require.Error(t, err)
	require.Nil(t, result)
	require.Len(t, upstream.requests, 1)
	require.Equal(t, "zstd", upstream.requests[0].Header.Get("Content-Encoding"))
}

func TestOpenAIGatewayService_ForwardRequestCompressionRecompressesRewrittenBody(t *testing.T) {
	upstream := &httpUpstreamRecorder{responses: []*http.Response{
		newOpenAIRequestCompressionResponse(http.StatusBadRequest, `{"error":{"code":"invalid_encrypted_content","message":"bad encrypted content"}}`),
		newOpenAIRequestCompressionStreamingResponse("resp_rewritten_ok"),
	}}
	svc, c, account, _ := newOpenAIRequestCompressionForwardFixture(upstream)
	body := []byte(`{"model":"gpt-5.5","stream":true,"instructions":"test","input":[{"type":"reasoning","encrypted_content":"gAAA","summary":[{"type":"summary_text","text":"keep"}]},{"type":"message","role":"user","content":[{"type":"input_text","text":"hello"}]}]}`)

	result, err := svc.Forward(context.Background(), c, account, body)

	require.NoError(t, err)
	require.NotNil(t, result)
	require.Len(t, upstream.requests, 2)
	require.Equal(t, "zstd", upstream.requests[0].Header.Get("Content-Encoding"))
	require.Equal(t, "zstd", upstream.requests[1].Header.Get("Content-Encoding"))
	first := decodeOpenAIRequestCompressionBody(t, upstream.bodies[0])
	second := decodeOpenAIRequestCompressionBody(t, upstream.bodies[1])
	require.Equal(t, "gAAA", gjson.GetBytes(first, "input.0.encrypted_content").String())
	require.False(t, gjson.GetBytes(second, "input.0.encrypted_content").Exists())
	require.NotEqual(t, first, second)
}

func TestOpenAIGatewayService_ForwardRequestCompressionFallbackStaysPlainAfterBodyRewrite(t *testing.T) {
	upstream := &httpUpstreamRecorder{responses: []*http.Response{
		newOpenAIRequestCompressionResponse(http.StatusUnsupportedMediaType, `{"error":{"message":"unsupported Content-Encoding: zstd"}}`),
		newOpenAIRequestCompressionResponse(http.StatusBadRequest, `{"error":{"code":"invalid_encrypted_content","message":"bad encrypted content"}}`),
		newOpenAIRequestCompressionStreamingResponse("resp_plain_rewrite_ok"),
	}}
	svc, c, account, _ := newOpenAIRequestCompressionForwardFixture(upstream)
	body := []byte(`{"model":"gpt-5.5","stream":true,"instructions":"test","input":[{"type":"reasoning","encrypted_content":"gAAA","summary":[]},{"type":"message","role":"user","content":[{"type":"input_text","text":"hello"}]}]}`)

	result, err := svc.Forward(context.Background(), c, account, body)

	require.NoError(t, err)
	require.NotNil(t, result)
	require.Len(t, upstream.requests, 3)
	require.Equal(t, "zstd", upstream.requests[0].Header.Get("Content-Encoding"))
	require.Empty(t, upstream.requests[1].Header.Get("Content-Encoding"))
	require.Empty(t, upstream.requests[2].Header.Get("Content-Encoding"))
	require.Equal(t, "gAAA", gjson.GetBytes(upstream.bodies[1], "input.0.encrypted_content").String())
	require.False(t, gjson.GetBytes(upstream.bodies[2], "input.0.encrypted_content").Exists())
}

func TestOpenAIGatewayService_ForwardRequestCompressionFallbackScopeSurvivesForwardReentry(t *testing.T) {
	upstream := &httpUpstreamRecorder{responses: []*http.Response{
		newOpenAIRequestCompressionResponse(http.StatusUnsupportedMediaType, `{"error":{"message":"unsupported Content-Encoding: zstd"}}`),
		newOpenAIRequestCompressionResponse(http.StatusBadGateway, `{"error":{"message":"temporary upstream outage"}}`),
		newOpenAIRequestCompressionStreamingResponse("resp_reentry_ok"),
	}}
	svc, c, account, body := newOpenAIRequestCompressionForwardFixture(upstream)

	firstResult, firstErr := svc.Forward(context.Background(), c, account, body)
	require.Error(t, firstErr)
	require.Nil(t, firstResult)

	secondResult, secondErr := svc.Forward(context.Background(), c, account, body)
	require.NoError(t, secondErr)
	require.NotNil(t, secondResult)
	require.Len(t, upstream.requests, 3)
	require.Equal(t, "zstd", upstream.requests[0].Header.Get("Content-Encoding"))
	require.Empty(t, upstream.requests[1].Header.Get("Content-Encoding"))
	require.Empty(t, upstream.requests[2].Header.Get("Content-Encoding"))
}

func TestOpenAIGatewayService_ForwardRequestCompressionDoesNotFallbackAfterCancellation(t *testing.T) {
	baseUpstream := &httpUpstreamRecorder{responses: []*http.Response{
		newOpenAIRequestCompressionResponse(http.StatusUnsupportedMediaType, `{"error":{"message":"unsupported Content-Encoding: zstd"}}`),
	}}
	ctx, cancel := context.WithCancel(context.Background())
	upstream := &cancelingOpenAIRequestCompressionUpstream{httpUpstreamRecorder: baseUpstream, cancel: cancel}
	svc, c, account, body := newOpenAIRequestCompressionForwardFixture(upstream)

	result, err := svc.Forward(ctx, c, account, body)

	require.ErrorIs(t, err, context.Canceled)
	require.Nil(t, result)
	require.Len(t, baseUpstream.requests, 1)
}

type cancelingOpenAIRequestCompressionUpstream struct {
	*httpUpstreamRecorder
	cancel context.CancelFunc
}

func (u *cancelingOpenAIRequestCompressionUpstream) Do(req *http.Request, proxyURL string, accountID int64, accountConcurrency int) (*http.Response, error) {
	resp, err := u.httpUpstreamRecorder.Do(req, proxyURL, accountID, accountConcurrency)
	u.cancel()
	return resp, err
}

func newOpenAIRequestCompressionForwardFixture(upstream HTTPUpstream) (*OpenAIGatewayService, *gin.Context, *Account, []byte) {
	gin.SetMode(gin.TestMode)
	cfg := &config.Config{}
	cfg.Gateway.OpenAIRequestCompression.Enabled = true
	cfg.Gateway.OpenAIRequestCompression.FallbackUncompressed = true
	svc := &OpenAIGatewayService{cfg: cfg, httpUpstream: upstream}
	body := []byte(`{"model":"gpt-5.5","stream":true,"instructions":"test","input":[{"type":"message","role":"user","content":[{"type":"input_text","text":"hello hello hello hello hello hello hello hello"}]}]}`)
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", bytes.NewReader(body))
	c.Request.Header.Set("Content-Type", "application/json")
	SetOpenAIClientTransport(c, OpenAIClientTransportHTTP)
	account := &Account{
		ID:          8701,
		Name:        "openai-oauth-request-compression",
		Platform:    PlatformOpenAI,
		Type:        AccountTypeOAuth,
		Concurrency: 1,
		Credentials: map[string]any{
			"access_token":       "oauth-token",
			"chatgpt_account_id": "chatgpt-account",
		},
		Status:      StatusActive,
		Schedulable: true,
	}
	return svc, c, account, body
}

func newOpenAIRequestCompressionResponse(status int, body string) *http.Response {
	return &http.Response{
		StatusCode: status,
		Header:     http.Header{"Content-Type": []string{"application/json"}},
		Body:       io.NopCloser(strings.NewReader(body)),
	}
}

func newOpenAIRequestCompressionStreamingResponse(responseID string) *http.Response {
	return &http.Response{
		StatusCode: http.StatusOK,
		Header:     http.Header{"Content-Type": []string{"text/event-stream"}},
		Body: io.NopCloser(strings.NewReader(
			`data: {"type":"response.completed","response":{"id":"` + responseID + `","status":"completed","output":[],"usage":{"input_tokens":2,"output_tokens":1}}}` + "\n\n",
		)),
	}
}

func decodeOpenAIRequestCompressionBody(t *testing.T, body []byte) []byte {
	t.Helper()
	decoder, err := zstd.NewReader(nil)
	require.NoError(t, err)
	t.Cleanup(decoder.Close)
	decoded, err := decoder.DecodeAll(body, nil)
	require.NoError(t, err)
	return decoded
}

var _ HTTPUpstream = (*cancelingOpenAIRequestCompressionUpstream)(nil)
