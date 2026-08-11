package service

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"

	"github.com/Wei-Shaw/sub2api/internal/config"
)

func newOpenAIImagesTestContext(t *testing.T, body []byte) (*gin.Context, *httptest.ResponseRecorder) {
REDACTED
	gin.SetMode(gin.TestMode)
	req := httptest.NewRequest(http.MethodPost, "/v1/images/generations", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = req
	return c, rec
REDACTED

func newOpenAIImagesTestService(upstream HTTPUpstream) *OpenAIGatewayService {
	return &OpenAIGatewayService{
		httpUpstream: upstream,
		cfg: &config.Config{
			Security: config.SecurityConfig{
				URLAllowlist: config.URLAllowlistConfig{Enabled: falseREDACTED,
		REDACTED,
	REDACTED,
REDACTED
REDACTED

func newOpenAIImagesAPIKeyAccount() *Account {
REDACTED
		ID:       31,
		Name:     "openai-apikey-images",
		Platform: PlatformOpenAI,
		Type:     AccountTypeAPIKey,
REDACTED
			"api_key":  "sk-test",
			"base_url": "https://api.openai.com/v1",
	REDACTED,
REDACTED
REDACTED

func openAIImagesJSONResponse() *http.Response {
	return &http.Response{
		StatusCode: http.StatusOK,
		Header: http.Header{
			"Content-Type": []string{"application/json"REDACTED,
			"X-Request-Id": []string{"req_img_ctx"REDACTED,
	REDACTED,
		Body: io.NopCloser(strings.NewReader(
			`{"created":1710000000,"data":[{"b64_json":"aGVsbG8="REDACTED],"usage":{"input_tokens":10,"output_tokens":20,"total_tokens":30REDACTEDREDACTED`,
		)),
REDACTED
REDACTED

// issue #5411：生图是长耗时、上游侧已经产生实际成本的操作。客户端中途断开时，
// 如果连带取消上游请求，就会出现「上游已出图并计费、网关记 502 context canceled、
// 用户不扣费」。非流式路径以前走 detachStreamUpstreamContext(ctx, false)，
// 该函数在非流式时原样返回请求 context，因此不脱钩。
func TestForwardOpenAIImagesAPIKey_NonStreamDetachesUpstreamContext(t *testing.T) {
	body := []byte(`{"model":"gpt-image-2","prompt":"draw a cat","response_format":"b64_json"REDACTED`)
	c, _ := newOpenAIImagesTestContext(t, body)

	recorder := &httpUpstreamRecorder{resp: openAIImagesJSONResponse()REDACTED
	svc := newOpenAIImagesTestService(recorder)

	parsed, err := svc.ParseOpenAIImagesRequest(c, body)
REDACTED
	require.False(t, parsed.Stream, "本用例覆盖非流式生图")

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // 客户端已断开

	result, err := svc.ForwardImages(ctx, c, newOpenAIImagesAPIKeyAccount(), body, parsed, "")

	require.NoError(t, err, "客户端断开不应把已在出图的上游调用打断成 context canceled")
	require.NotNil(t, result)
	require.Equal(t, 1, result.ImageCount, "图片已产出，必须带回结果供计费")

	require.NotNil(t, recorder.lastReq)
	require.NoError(t, recorder.lastReq.Context().Err(),
		"交给上游的请求 context 必须已脱钩，不随客户端断开取消")
REDACTED

// 流式路径本来就脱钩，这条守卫防止对齐时把它改坏。
func TestForwardOpenAIImagesAPIKey_StreamKeepsDetachedUpstreamContext(t *testing.T) {
	body := []byte(`{"model":"gpt-image-2","prompt":"draw a cat","stream":true,"response_format":"b64_json"REDACTED`)
	c, _ := newOpenAIImagesTestContext(t, body)

	recorder := &httpUpstreamRecorder{resp: &http.Response{
		StatusCode: http.StatusOK,
		Header: http.Header{
			"Content-Type": []string{"text/event-stream"REDACTED,
			"X-Request-Id": []string{"req_img_ctx_stream"REDACTED,
	REDACTED,
		Body: io.NopCloser(strings.NewReader(
			"data: {\"type\":\"response.created\",\"response\":{\"created_at\":1710000000REDACTEDREDACTED\n\n" +
				"data: {\"type\":\"response.image_generation_call.completed\",\"result\":\"aGVsbG8=\"REDACTED\n\n" +
				"data: {\"type\":\"response.completed\",\"response\":{\"usage\":{\"input_tokens\":10,\"output_tokens\":20REDACTEDREDACTEDREDACTED\n\n",
		)),
REDACTEDREDACTED
	svc := newOpenAIImagesTestService(recorder)

	parsed, err := svc.ParseOpenAIImagesRequest(c, body)
REDACTED
	require.True(t, parsed.Stream)

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, _ = svc.ForwardImages(ctx, c, newOpenAIImagesAPIKeyAccount(), body, parsed, "")

	require.NotNil(t, recorder.lastReq)
	require.NoError(t, recorder.lastReq.Context().Err(),
		"流式路径原本就脱钩，不能被改回随客户端取消")
REDACTED

// 两个 detach 辅助函数的语义差异是本次修复的根据，锁死它们防止被悄悄改动。
func TestDetachUpstreamContextSemantics(t *testing.T) {
	t.Run("detachUpstreamContext_always_detaches", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		cancel()
		detached, release := detachUpstreamContext(ctx)
		defer release()
		require.NoError(t, detached.Err())
REDACTED)

	t.Run("detachStreamUpstreamContext_keeps_cancel_when_not_streaming", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		cancel()
		same, release := detachStreamUpstreamContext(ctx, false)
		defer release()
		require.ErrorIs(t, same.Err(), context.Canceled,
			"非流式时该函数原样返回请求 context —— 生图路径不能用它")
REDACTED)

	t.Run("detachStreamUpstreamContext_detaches_when_streaming", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		cancel()
		detached, release := detachStreamUpstreamContext(ctx, true)
		defer release()
		require.NoError(t, detached.Err())
REDACTED)
REDACTED
