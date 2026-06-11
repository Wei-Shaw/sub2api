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
		name          string
		allowImages   bool
		bridgeEnabled bool
		wantInjected  bool
REDACTED{
		{name: "disabled group skips injection", allowImages: false, bridgeEnabled: true, wantInjected: falseREDACTED,
		{name: "enabled group skips injection by default", allowImages: true, bridgeEnabled: false, wantInjected: falseREDACTED,
		{name: "enabled group injects image tool when bridge enabled", allowImages: true, bridgeEnabled: true, wantInjected: trueREDACTED,
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
			svc.cfg.Gateway.CodexImageGenerationBridgeEnabled = tt.bridgeEnabled
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

func TestOpenAIGatewayServiceForward_ExplicitImageToolWorksWithBridgeDisabled(t *testing.T) {
	gin.SetMode(gin.TestMode)

	upstream := &httpUpstreamRecorder{
		resp: &http.Response{
			StatusCode: http.StatusOK,
			Header:     http.Header{"Content-Type": []string{"application/json"REDACTEDREDACTED,
			Body:       io.NopCloser(strings.NewReader(`{"id":"resp_explicit_image","model":"gpt-5.4","usage":{"input_tokens":2,"output_tokens":1REDACTEDREDACTED`)),
	REDACTED,
REDACTED
	svc := newOpenAIImageGenerationControlTestService(upstream)
	c, _ := newOpenAIImageGenerationControlTestContext(true, "codex_cli_rs/0.98.0")
	account := newOpenAIImageGenerationControlTestAccount()
	body := []byte(`{"model":"gpt-5.4","input":"draw","stream":false,"tools":[{"type":"image_generation","format":"jpeg"REDACTED]REDACTED`)

	result, err := svc.Forward(context.Background(), c, account, body)

REDACTED
	require.NotNil(t, result)
	require.NotNil(t, upstream.lastReq)
	require.True(t, gjson.GetBytes(upstream.lastBody, `tools.#(type=="image_generation")`).Exists())
	require.Equal(t, "jpeg", gjson.GetBytes(upstream.lastBody, `tools.#(type=="image_generation").output_format`).String())
	require.False(t, gjson.GetBytes(upstream.lastBody, `tools.#(type=="image_generation").format`).Exists())
	instructions := gjson.GetBytes(upstream.lastBody, "instructions").String()
	require.NotContains(t, instructions, "image_generation")
REDACTED

func TestOpenAIGatewayServiceForward_ChannelBridgeOverrideEnablesCodexInjection(t *testing.T) {
	gin.SetMode(gin.TestMode)

	upstream := &httpUpstreamRecorder{
		resp: &http.Response{
			StatusCode: http.StatusOK,
			Header:     http.Header{"Content-Type": []string{"application/json"REDACTEDREDACTED,
			Body:       io.NopCloser(strings.NewReader(`{"id":"resp_channel_bridge","model":"gpt-5.4","usage":{"input_tokens":1,"output_tokens":1REDACTEDREDACTED`)),
	REDACTED,
REDACTED
	svc := newOpenAIImageGenerationControlTestService(upstream)
	groupID := int64(4242)
	svc.channelService = newOpenAIImageGenerationControlChannelService(groupID, &Channel{
		ID:     9001,
		Status: StatusActive,
		FeaturesConfig: map[string]any{
			featureKeyCodexImageGenerationBridge: map[string]any{PlatformOpenAI: trueREDACTED,
	REDACTED,
REDACTED)
	c, _ := newOpenAIImageGenerationControlTestContext(true, "codex_cli_rs/0.98.0")
	account := newOpenAIImageGenerationControlTestAccount()

	result, err := svc.Forward(context.Background(), c, account, []byte(`{"model":"gpt-5.4","input":"write code","stream":falseREDACTED`))

REDACTED
	require.NotNil(t, result)
	require.NotNil(t, upstream.lastReq)
	require.True(t, gjson.GetBytes(upstream.lastBody, `tools.#(type=="image_generation")`).Exists())
	instructions := gjson.GetBytes(upstream.lastBody, "instructions").String()
	require.Contains(t, instructions, "image_generation")
REDACTED

func TestOpenAIGatewayService_CodexImageGenerationBridgeOverridePrecedence(t *testing.T) {
	groupID := int64(4242)

	tests := []struct {
		name    string
		global  bool
		channel *Channel
		account *Account
		want    bool
REDACTED{
		{
			name:   "global default enables bridge",
			global: true,
			account: &Account{
				Platform: PlatformOpenAI,
		REDACTED,
			want: true,
	REDACTED,
		{
			name:   "channel true overrides disabled global",
			global: false,
			channel: &Channel{ID: 1, Status: StatusActive, FeaturesConfig: map[string]any{
				featureKeyCodexImageGenerationBridge: map[string]any{PlatformOpenAI: trueREDACTED,
	REDACTED
			account: &Account{Platform: PlatformOpenAIREDACTED,
			want:    true,
	REDACTED,
		{
			name:   "channel false overrides enabled global",
			global: true,
			channel: &Channel{ID: 1, Status: StatusActive, FeaturesConfig: map[string]any{
				featureKeyCodexImageGenerationBridge: map[string]any{PlatformOpenAI: falseREDACTED,
	REDACTED
			account: &Account{Platform: PlatformOpenAIREDACTED,
			want:    false,
	REDACTED,
		{
			name:   "account false overrides channel and global true",
			global: true,
			channel: &Channel{ID: 1, Status: StatusActive, FeaturesConfig: map[string]any{
				featureKeyCodexImageGenerationBridge: map[string]any{PlatformOpenAI: trueREDACTED,
	REDACTED
			account: &Account{
				Platform: PlatformOpenAI,
				Extra:    map[string]any{featureKeyCodexImageGenerationBridge: falseREDACTED,
		REDACTED,
			want: false,
	REDACTED,
		{
			name:   "nested account true overrides channel false",
			global: false,
			channel: &Channel{ID: 1, Status: StatusActive, FeaturesConfig: map[string]any{
				featureKeyCodexImageGenerationBridge: map[string]any{PlatformOpenAI: falseREDACTED,
	REDACTED
			account: &Account{
				Platform: PlatformOpenAI,
				Extra: map[string]any{
					PlatformOpenAI: map[string]any{"codex_image_generation_bridge_enabled": trueREDACTED,
			REDACTED,
		REDACTED,
			want: true,
	REDACTED,
		{
			name:   "non openai account extra is ignored",
			global: false,
			account: &Account{
				Platform: PlatformAnthropic,
				Extra:    map[string]any{featureKeyCodexImageGenerationBridge: trueREDACTED,
		REDACTED,
			want: false,
	REDACTED,
REDACTED

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			svc := newOpenAIImageGenerationControlTestService(&httpUpstreamRecorder{REDACTED)
			svc.cfg.Gateway.CodexImageGenerationBridgeEnabled = tt.global
			if tt.channel != nil {
				svc.channelService = newOpenAIImageGenerationControlChannelService(groupID, tt.channel)
		REDACTED
			apiKey := &APIKey{GroupID: &groupIDREDACTED

			got := svc.isCodexImageGenerationBridgeEnabled(context.Background(), tt.account, apiKey)

			require.Equal(t, tt.want, got)
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

// TestHandleStreamingResponse_CyberPolicyCapturesRealUpstreamTokens 锁定流式
// /v1/responses 命中 cyber_policy 的计费正确性：response.failed 自带的真实 usage
// 必须在打 cyber 标记前被解析进 mark；否则计费走 mark.UpstreamInTok 会按 0 token
// 漏记真实用量（该路径返回错误，handler 仅经 RecordCyberPolicyUsageLog 计费）。
func TestHandleStreamingResponse_CyberPolicyCapturesRealUpstreamTokens(t *testing.T) {
	gin.SetMode(gin.TestMode)

	svc := newOpenAIImageGenerationControlTestService(&httpUpstreamRecorder{REDACTED)
	c, _ := newOpenAIImageGenerationControlTestContext(false, "unit-test-agent/1.0")
	resp := &http.Response{
		StatusCode: http.StatusOK,
		Header:     http.Header{"Content-Type": []string{"text/event-stream"REDACTEDREDACTED,
		Body: io.NopCloser(strings.NewReader(
			"data: {\"type\":\"response.created\",\"response\":{\"id\":\"resp_cyber\"REDACTEDREDACTED\n\n" +
				"data: {\"type\":\"response.failed\",\"response\":{\"id\":\"resp_cyber\",\"error\":{\"code\":\"cyber_policy\",\"message\":\"blocked by network policy\"REDACTED,\"usage\":{\"input_tokens\":1234,\"output_tokens\":7REDACTEDREDACTEDREDACTED\n\n",
		)),
REDACTED

	_, err := svc.handleStreamingResponse(context.Background(), resp, c, &Account{ID: 1REDACTED, time.Now(), "gpt-5.5", "gpt-5.5")
	require.Error(t, err, "cyber 命中的流式响应应返回错误（sawFailedEvent）")

	mark := GetOpsCyberPolicy(c)
	require.NotNil(t, mark, "必须打上 cyber 标记")
	require.Equal(t, "cyber_policy", mark.Code)
	require.Equal(t, 1234, mark.UpstreamInTok, "必须捕获 response.failed 自带真实 input token，而非解析前的 0")
	require.Equal(t, 7, mark.UpstreamOutTok)
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

func newOpenAIImageGenerationControlChannelService(groupID int64, ch *Channel) *ChannelService {
	svc := &ChannelService{REDACTED
	cache := newEmptyChannelCache()
	if ch != nil {
		cache.channelByGroupID[groupID] = ch
		cache.byID[ch.ID] = ch
REDACTED
	cache.loadedAt = time.Now()
	svc.cache.Store(cache)
	return svc
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
