package service

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

func TestOpenAIGatewayServiceForward_RejectsDisabledImageGenerationIntents(t *testing.T) {
	gin.SetMode(gin.TestMode)

	tests := []struct {
		name string
		body []byte
REDACTED{
		{
			name: "image model",
			body: []byte(`{"model":"gpt-image-2","input":"draw"REDACTED`),
	REDACTED,
		{
			name: "image tool",
			body: []byte(`{"model":"gpt-5.4","input":"draw","tools":[{"type":"image_generation"REDACTED]REDACTED`),
	REDACTED,
		{
			name: "image tool choice",
			body: []byte(`{"model":"gpt-5.4","input":"draw","tool_choice":{"type":"image_generation"REDACTEDREDACTED`),
	REDACTED,
REDACTED

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			upstream := &httpUpstreamRecorder{REDACTED
			svc := newOpenAIImageGenerationControlTestService(upstream)
			c, recorder := newOpenAIImageGenerationControlTestContext(false, "unit-test-agent/1.0")
			account := newOpenAIImageGenerationControlTestAccount()

			result, err := svc.Forward(context.Background(), c, account, tt.body)

		REDACTED
			require.Nil(t, result)
			require.Equal(t, http.StatusForbidden, recorder.Code)
			require.Equal(t, "permission_error", gjson.GetBytes(recorder.Body.Bytes(), "error.type").String())
			require.Nil(t, upstream.lastReq, "disabled image request must not reach upstream")
	REDACTED)
REDACTED
REDACTED

func TestOpenAIGatewayServiceForward_DisabledGroupAllowsTextOnlyResponses(t *testing.T) {
	gin.SetMode(gin.TestMode)

	upstream := &httpUpstreamRecorder{
		resp: &http.Response{
			StatusCode: http.StatusOK,
			Header:     http.Header{"Content-Type": []string{"application/json"REDACTEDREDACTED,
			Body:       io.NopCloser(strings.NewReader(`{"id":"resp_text","model":"gpt-5.4","usage":{"input_tokens":3,"output_tokens":2REDACTEDREDACTED`)),
	REDACTED,
REDACTED
	svc := newOpenAIImageGenerationControlTestService(upstream)
	c, recorder := newOpenAIImageGenerationControlTestContext(false, "unit-test-agent/1.0")
	account := newOpenAIImageGenerationControlTestAccount()

	result, err := svc.Forward(context.Background(), c, account, []byte(`{"model":"gpt-5.4","input":"write code","stream":falseREDACTED`))

REDACTED
	require.NotNil(t, result)
	require.Equal(t, http.StatusOK, recorder.Code)
	require.Equal(t, 3, result.Usage.InputTokens)
	require.Equal(t, 2, result.Usage.OutputTokens)
	require.Equal(t, 0, result.ImageCount)
	require.NotNil(t, upstream.lastReq)
REDACTED

func TestOpenAIGatewayServiceForward_CodexImageInjectionRespectsGroupCapability(t *testing.T) {
	gin.SetMode(gin.TestMode)

	tests := []struct {
		name         string
		allowImages  bool
		wantInjected bool
REDACTED{
		{name: "disabled group skips injection", allowImages: false, wantInjected: falseREDACTED,
		{name: "enabled group injects image tool", allowImages: true, wantInjected: trueREDACTED,
REDACTED

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			upstream := &httpUpstreamRecorder{
				resp: &http.Response{
					StatusCode: http.StatusOK,
					Header:     http.Header{"Content-Type": []string{"application/json"REDACTEDREDACTED,
					Body:       io.NopCloser(strings.NewReader(`{"id":"resp_codex","model":"gpt-5.4","usage":{"input_tokens":1,"output_tokens":1REDACTEDREDACTED`)),
			REDACTED,
		REDACTED
			svc := newOpenAIImageGenerationControlTestService(upstream)
			c, _ := newOpenAIImageGenerationControlTestContext(tt.allowImages, "codex_cli_rs/0.98.0")
			account := newOpenAIImageGenerationControlTestAccount()

			result, err := svc.Forward(context.Background(), c, account, []byte(`{"model":"gpt-5.4","input":"write code","stream":falseREDACTED`))

		REDACTED
			require.NotNil(t, result)
			require.NotNil(t, upstream.lastReq)
			hasImageTool := gjson.GetBytes(upstream.lastBody, `tools.#(type=="image_generation")`).Exists()
			require.Equal(t, tt.wantInjected, hasImageTool)
			instructions := gjson.GetBytes(upstream.lastBody, "instructions").String()
			require.Equal(t, tt.wantInjected, strings.Contains(instructions, "image_generation"))
	REDACTED)
REDACTED
REDACTED

func TestOpenAIGatewayServiceHandleResponsesImageOutputs_NonStreaming(t *testing.T) {
	gin.SetMode(gin.TestMode)

	svc := newOpenAIImageGenerationControlTestService(&httpUpstreamRecorder{REDACTED)
	c, _ := newOpenAIImageGenerationControlTestContext(true, "unit-test-agent/1.0")
	resp := &http.Response{
		StatusCode: http.StatusOK,
		Header:     http.Header{"Content-Type": []string{"application/json"REDACTEDREDACTED,
		Body: io.NopCloser(strings.NewReader(`{
			"id":"resp_image_json",
			"model":"gpt-5.4",
			"output":[{"id":"ig_json_1","type":"image_generation_call","result":"final-image"REDACTED],
			"usage":{"input_tokens":7,"output_tokens":3,"output_tokens_details":{"image_tokens":2REDACTEDREDACTED
	REDACTED`)),
REDACTED

	result, err := svc.handleNonStreamingResponse(context.Background(), resp, c, &Account{ID: 1, Type: AccountTypeAPIKeyREDACTED, "gpt-5.4", "gpt-5.4")

REDACTED
	require.NotNil(t, result)
	require.Equal(t, 1, result.imageCount)
	require.NotNil(t, result.usage)
	require.Equal(t, 7, result.usage.InputTokens)
	require.Equal(t, 3, result.usage.OutputTokens)
	require.Equal(t, 2, result.usage.ImageOutputTokens)
REDACTED

func TestOpenAIGatewayServiceHandleResponsesImageOutputs_Streaming(t *testing.T) {
	gin.SetMode(gin.TestMode)

	svc := newOpenAIImageGenerationControlTestService(&httpUpstreamRecorder{REDACTED)
	c, _ := newOpenAIImageGenerationControlTestContext(true, "unit-test-agent/1.0")
	resp := &http.Response{
		StatusCode: http.StatusOK,
		Header:     http.Header{"Content-Type": []string{"text/event-stream"REDACTEDREDACTED,
		Body: io.NopCloser(strings.NewReader(
			"data: {\"type\":\"response.output_item.done\",\"item\":{\"id\":\"ig_stream_1\",\"type\":\"image_generation_call\",\"result\":\"final-image\"REDACTEDREDACTED\n\n" +
				"data: {\"type\":\"response.completed\",\"response\":{\"id\":\"resp_image_stream\",\"model\":\"gpt-5.5\",\"output\":[{\"id\":\"ig_stream_1\",\"type\":\"image_generation_call\",\"result\":\"final-image\"REDACTED],\"usage\":{\"input_tokens\":11,\"output_tokens\":5,\"output_tokens_details\":{\"image_tokens\":4REDACTEDREDACTEDREDACTEDREDACTED\n\n",
		)),
REDACTED

	result, err := svc.handleStreamingResponse(context.Background(), resp, c, &Account{ID: 1REDACTED, time.Now(), "gpt-5.5", "gpt-5.5")

REDACTED
	require.NotNil(t, result)
	require.Equal(t, 1, result.imageCount)
	require.NotNil(t, result.usage)
	require.Equal(t, 11, result.usage.InputTokens)
	require.Equal(t, 5, result.usage.OutputTokens)
	require.Equal(t, 4, result.usage.ImageOutputTokens)
REDACTED

func newOpenAIImageGenerationControlTestService(upstream *httpUpstreamRecorder) *OpenAIGatewayService {
	cfg := &config.Config{REDACTED
	return &OpenAIGatewayService{
		cfg:              cfg,
		httpUpstream:     upstream,
		cache:            &stubGatewayCache{REDACTED,
		openaiWSResolver: NewOpenAIWSProtocolResolver(cfg),
		toolCorrector:    NewCodexToolCorrector(),
REDACTED
REDACTED

func newOpenAIImageGenerationControlTestContext(allowImages bool, userAgent string) (*gin.Context, *httptest.ResponseRecorder) {
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodPost, "/openai/v1/responses", nil)
	c.Request.Header.Set("User-Agent", userAgent)
	groupID := int64(4242)
	c.Set("api_key", &APIKey{
		ID:      2424,
		GroupID: &groupID,
		Group: &Group{
			ID:                   groupID,
			AllowImageGeneration: allowImages,
			RateMultiplier:       1,
			ImageRateMultiplier:  1,
	REDACTED,
REDACTED)
	return c, recorder
REDACTED

func newOpenAIImageGenerationControlTestAccount() *Account {
REDACTED
		ID:          5151,
		Name:        "openai-image-controls",
		Platform:    PlatformOpenAI,
		Type:        AccountTypeAPIKey,
		Status:      StatusActive,
		Schedulable: true,
		Concurrency: 1,
REDACTED
			"api_key": "sk-test",
	REDACTED,
REDACTED
REDACTED
