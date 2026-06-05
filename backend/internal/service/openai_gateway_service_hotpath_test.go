package service

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

func TestOpenAIRequestView_ExtractsRawScalars(t *testing.T) {
	view := newOpenAIRequestView([]byte(`{"model":" gpt-5 ","stream":true,"prompt_cache_key":" ses-1 ","previous_response_id":" resp-1 ","service_tier":" fast ","reasoning":{"effort":" medium "REDACTEDREDACTED`))

	require.Equal(t, "gpt-5", view.Model)
	require.True(t, view.Stream)
	require.Equal(t, "ses-1", view.PromptCacheKey)
	require.Equal(t, "resp-1", view.PreviousResponseID)
	require.Equal(t, "fast", view.ServiceTier)
	require.Equal(t, "medium", view.ReasoningEffort)
REDACTED

func TestOpenAIRequestView_DecodeKeepsFullMapBehavior(t *testing.T) {
	view := newOpenAIRequestView([]byte(`{"model":"gpt-5","stream":true,"input":[{"type":"message","content":"hi"REDACTED]REDACTED`))

	reqBody, err := view.Decode(nil)
REDACTED
	require.Equal(t, "gpt-5", reqBody["model"])
	require.IsType(t, []any{REDACTED, reqBody["input"])
REDACTED

func TestOpenAIRequestView_ApplyPatches(t *testing.T) {
	view := newOpenAIRequestView([]byte(`{"model":"gpt-5","previous_response_id":"resp_1","reasoning":{"effort":"minimal"REDACTED,"input":[{"type":"message","content":"hi"REDACTED]REDACTED`))
	view.MarkPatchSet("model", "gpt-5.1")
	view.MarkPatchDelete("previous_response_id")
	view.MarkPatchSet("reasoning.effort", "none")

	patched, err := view.ApplyPatches()
REDACTED
	require.JSONEq(t, `{"model":"gpt-5.1","reasoning":{"effort":"none"REDACTED,"input":[{"type":"message","content":"hi"REDACTED]REDACTED`, string(patched))
REDACTED

func TestOpenAIRequestView_RejectsEscapedPatchPath(t *testing.T) {
	view := newOpenAIRequestView([]byte(`{"metadata":{"user.id":"old"REDACTEDREDACTED`))
	view.MarkPatchSet(`metadata.user\.id`, "new")

	require.False(t, view.HasPatches())
	_, err := view.ApplyPatches()
REDACTED
REDACTED

func TestOpenAIRequestView_ApplyPatchesDisabled(t *testing.T) {
	view := newOpenAIRequestView([]byte(`{"model":"gpt-5"REDACTED`))
	view.MarkPatchSet("model", "gpt-5.1")
	view.DisablePatches()

	_, err := view.ApplyPatches()
REDACTED
REDACTED

func TestOpenAIRequestView_HasPatches(t *testing.T) {
	view := newOpenAIRequestView([]byte(`{"model":"gpt-5"REDACTED`))
	require.False(t, view.HasPatches())

	view.MarkPatchSet("model", "gpt-5.1")
	require.True(t, view.HasPatches())

	view.DisablePatches()
	require.False(t, view.HasPatches())
REDACTED

func TestOpenAIGatewayService_Forward_HTTPPatchPathKeepsLargeInputRaw(t *testing.T) {
	gin.SetMode(gin.TestMode)
	upstream := &httpUpstreamRecorder{
		resp: &http.Response{
			StatusCode: http.StatusOK,
			Header:     http.Header{"Content-Type": []string{"application/json"REDACTEDREDACTED,
			Body: io.NopCloser(strings.NewReader(
				`{"usage":{"input_tokens":1,"output_tokens":2,"input_tokens_details":{"cached_tokens":0REDACTEDREDACTEDREDACTED`,
			)),
	REDACTED,
REDACTED
	cfg := &config.Config{REDACTED
	cfg.Security.URLAllowlist.Enabled = false
	svc := &OpenAIGatewayService{cfg: cfg, httpUpstream: upstreamREDACTED
	account := &Account{
		ID:          1,
		Name:        "openai-apikey",
		Platform:    PlatformOpenAI,
		Type:        AccountTypeAPIKey,
		Concurrency: 1,
REDACTED
			"api_key":  "sk-test",
			"base_url": "https://example.com",
	REDACTED,
		Extra: map[string]any{"use_responses_api": trueREDACTED,
REDACTED
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPost, "/openai/v1/responses", nil)
	SetOpenAIClientTransport(c, OpenAIClientTransportHTTP)

	body := []byte(`{"model":"gpt-5","stream":false,"reasoning":{"effort":"minimal"REDACTED,"input":[{"type":"message","content":[{"type":"input_text","text":"hi","nonce":9007199254740993REDACTED]REDACTED]REDACTED`)
	result, err := svc.Forward(context.Background(), c, account, body)
REDACTED
	require.NotNil(t, result)
	require.NotNil(t, upstream.lastReq)
	// 合成路径默认 instructions 现填入真实 Codex base prompt（openai.DefaultInstructions）。
	encodedInstr, _ := json.Marshal(defaultCodexSynthInstructions())
	expectedBody := fmt.Sprintf(`{"model":"gpt-5","stream":false,"reasoning":{"effort":"none"REDACTED,"instructions":%s,"input":[{"type":"message","content":[{"type":"input_text","text":"hi","nonce":9007199254740993REDACTED]REDACTED]REDACTED`, string(encodedInstr))
	require.JSONEq(t, expectedBody, string(upstream.lastBody))
	require.Equal(t, "9007199254740993", gjson.GetBytes(upstream.lastBody, "input.0.content.0.nonce").Raw)
REDACTED

func TestOpenAIGatewayService_Forward_DecodedMutationKeepsLaterFieldDeletes(t *testing.T) {
	gin.SetMode(gin.TestMode)
	upstream := &httpUpstreamRecorder{
		resp: &http.Response{
			StatusCode: http.StatusOK,
			Header:     http.Header{"Content-Type": []string{"application/json"REDACTEDREDACTED,
			Body:       io.NopCloser(strings.NewReader(`{"usage":{"input_tokens":1,"output_tokens":2REDACTEDREDACTED`)),
	REDACTED,
REDACTED
	cfg := &config.Config{REDACTED
	cfg.Security.URLAllowlist.Enabled = false
	svc := &OpenAIGatewayService{cfg: cfg, httpUpstream: upstreamREDACTED
	account := &Account{
		ID:          2,
		Name:        "openai-apikey",
		Platform:    PlatformOpenAI,
		Type:        AccountTypeAPIKey,
		Concurrency: 1,
REDACTED
			"api_key":  "sk-test",
			"base_url": "https://example.com",
	REDACTED,
		Extra: map[string]any{"use_responses_api": trueREDACTED,
REDACTED
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPost, "/openai/v1/responses", nil)
	SetOpenAIClientTransport(c, OpenAIClientTransportHTTP)

	body := []byte(`{"model":"gpt-5.4","stream":false,"max_completion_tokens":12,"tools":[{"type":"image_generation","format":"png"REDACTED],"input":[{"type":"message","content":"draw"REDACTED]REDACTED`)
	result, err := svc.Forward(context.Background(), c, account, body)
REDACTED
	require.NotNil(t, result)
	require.False(t, gjson.GetBytes(upstream.lastBody, "max_completion_tokens").Exists())
	require.False(t, gjson.GetBytes(upstream.lastBody, "tools.0.format").Exists())
	require.Equal(t, "png", gjson.GetBytes(upstream.lastBody, "tools.0.output_format").String())
REDACTED

func TestOpenAIGatewayService_Forward_MappedImageModelUsesImageGate(t *testing.T) {
	gin.SetMode(gin.TestMode)
	upstream := &httpUpstreamRecorder{
		resp: &http.Response{
			StatusCode: http.StatusOK,
			Header:     http.Header{"Content-Type": []string{"application/json"REDACTEDREDACTED,
			Body:       io.NopCloser(strings.NewReader(`{"usage":{"input_tokens":1,"output_tokens":2REDACTEDREDACTED`)),
	REDACTED,
REDACTED
	cfg := &config.Config{REDACTED
	cfg.Security.URLAllowlist.Enabled = false
	svc := &OpenAIGatewayService{cfg: cfg, httpUpstream: upstreamREDACTED
	account := &Account{
		ID:          3,
		Name:        "openai-apikey",
		Platform:    PlatformOpenAI,
		Type:        AccountTypeAPIKey,
		Concurrency: 1,
REDACTED
			"api_key":       "sk-test",
			"base_url":      "https://example.com",
			"model_mapping": map[string]any{"draw-alias": "gpt-image-2"REDACTED,
	REDACTED,
		Extra: map[string]any{"use_responses_api": trueREDACTED,
REDACTED
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPost, "/openai/v1/responses", nil)
	c.Set("api_key", &APIKey{Group: &Group{AllowImageGeneration: falseREDACTEDREDACTED)
	SetOpenAIClientTransport(c, OpenAIClientTransportHTTP)

	body := []byte(`{"model":"draw-alias","stream":false,"input":"draw"REDACTED`)
	result, err := svc.Forward(context.Background(), c, account, body)
REDACTED
	require.Nil(t, result)
	require.Nil(t, upstream.lastReq)
	require.Equal(t, http.StatusForbidden, rec.Code)
REDACTED

func TestOpenAIGatewayService_Forward_TextDataImageDoesNotForceMapMarshal(t *testing.T) {
	gin.SetMode(gin.TestMode)
	upstream := &httpUpstreamRecorder{
		resp: &http.Response{
			StatusCode: http.StatusOK,
			Header:     http.Header{"Content-Type": []string{"application/json"REDACTEDREDACTED,
			Body:       io.NopCloser(strings.NewReader(`{"usage":{"input_tokens":1,"output_tokens":2REDACTEDREDACTED`)),
	REDACTED,
REDACTED
	cfg := &config.Config{REDACTED
	cfg.Security.URLAllowlist.Enabled = false
	svc := &OpenAIGatewayService{cfg: cfg, httpUpstream: upstreamREDACTED
	account := &Account{
		ID:          4,
		Name:        "openai-apikey",
		Platform:    PlatformOpenAI,
		Type:        AccountTypeAPIKey,
		Concurrency: 1,
REDACTED
			"api_key":  "sk-test",
			"base_url": "https://example.com",
	REDACTED,
		Extra: map[string]any{"use_responses_api": trueREDACTED,
REDACTED
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPost, "/openai/v1/responses", nil)
	SetOpenAIClientTransport(c, OpenAIClientTransportHTTP)

	body := []byte(`{"model":"gpt-5","stream":false,"input":[{"type":"message","content":[{"type":"input_text","text":"literal data:image/png;base64, only","nonce":1e1000000REDACTED]REDACTED]REDACTED`)
	result, err := svc.Forward(context.Background(), c, account, body)
REDACTED
	require.NotNil(t, result)
	require.Equal(t, "1e1000000", gjson.GetBytes(upstream.lastBody, "input.0.content.0.nonce").Raw)
REDACTED

func TestOpenAIGatewayService_Forward_ImageToolBillingDoesNotForceFullDecode(t *testing.T) {
	gin.SetMode(gin.TestMode)
	upstream := &httpUpstreamRecorder{
		resp: &http.Response{
			StatusCode: http.StatusOK,
			Header:     http.Header{"Content-Type": []string{"application/json"REDACTEDREDACTED,
			Body: io.NopCloser(strings.NewReader(
				`{"output":[{"id":"ig_1","type":"image_generation_call","result":"final-image"REDACTED],"usage":{"input_tokens":1,"output_tokens":2REDACTEDREDACTED`,
			)),
	REDACTED,
REDACTED
	cfg := &config.Config{REDACTED
	cfg.Security.URLAllowlist.Enabled = false
	svc := &OpenAIGatewayService{cfg: cfg, httpUpstream: upstreamREDACTED
	account := &Account{
		ID:          9,
		Name:        "openai-apikey",
		Platform:    PlatformOpenAI,
		Type:        AccountTypeAPIKey,
		Concurrency: 1,
REDACTED
			"api_key":  "sk-test",
			"base_url": "https://example.com",
	REDACTED,
		Extra: map[string]any{"use_responses_api": trueREDACTED,
REDACTED
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPost, "/openai/v1/responses", nil)
	SetOpenAIClientTransport(c, OpenAIClientTransportHTTP)

	body := []byte(`{"model":"gpt-5","stream":false,"tools":[{"type":"image_generation","model":"gpt-image-2","size":"2048x1152"REDACTED],"input":[{"type":"message","content":[{"type":"input_text","text":"draw","nonce":1e1000000REDACTED]REDACTED]REDACTED`)
	result, err := svc.Forward(context.Background(), c, account, body)
REDACTED
	require.NotNil(t, result)
	require.Equal(t, "1e1000000", gjson.GetBytes(upstream.lastBody, "input.0.content.0.nonce").Raw)
	require.Equal(t, 1, result.ImageCount)
	require.Equal(t, "2K", result.ImageSize)
	require.Equal(t, "gpt-image-2", result.BillingModel)
REDACTED

func TestOpenAIGatewayService_Forward_ImageToolWithImageOnlyModelIsNormalized(t *testing.T) {
	gin.SetMode(gin.TestMode)
	upstream := &httpUpstreamRecorder{
		resp: &http.Response{
			StatusCode: http.StatusOK,
			Header:     http.Header{"Content-Type": []string{"application/json"REDACTEDREDACTED,
			Body:       io.NopCloser(strings.NewReader(`{"usage":{"input_tokens":1,"output_tokens":2REDACTEDREDACTED`)),
	REDACTED,
REDACTED
	cfg := &config.Config{REDACTED
	cfg.Security.URLAllowlist.Enabled = false
	svc := &OpenAIGatewayService{cfg: cfg, httpUpstream: upstreamREDACTED
	account := &Account{
		ID:          11,
		Name:        "openai-apikey",
		Platform:    PlatformOpenAI,
		Type:        AccountTypeAPIKey,
		Concurrency: 1,
REDACTED
			"api_key":  "sk-test",
			"base_url": "https://example.com",
	REDACTED,
		Extra: map[string]any{"use_responses_api": trueREDACTED,
REDACTED
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPost, "/openai/v1/responses", nil)
	SetOpenAIClientTransport(c, OpenAIClientTransportHTTP)

	body := []byte(`{"model":"gpt-image-2","stream":false,"tools":[{"type":"image_generation","model":"gpt-image-2"REDACTED],"input":"draw"REDACTED`)
	result, err := svc.Forward(context.Background(), c, account, body)
REDACTED
	require.NotNil(t, result)
	require.Equal(t, openAIImagesResponsesMainModel, gjson.GetBytes(upstream.lastBody, "model").String())
REDACTED

func TestOpenAIGatewayService_Forward_HTTPRetryRecoveryDoesNotDecodeBeforeError(t *testing.T) {
	gin.SetMode(gin.TestMode)
	upstream := &httpUpstreamRecorder{
		responses: []*http.Response{
			{
				StatusCode: http.StatusBadRequest,
				Header:     http.Header{"Content-Type": []string{"application/json"REDACTEDREDACTED,
				Body:       io.NopCloser(strings.NewReader(`{"error":{"code":"invalid_encrypted_content","type":"invalid_request_error","message":"bad encrypted content"REDACTEDREDACTED`)),
		REDACTED,
			{
				StatusCode: http.StatusOK,
				Header:     http.Header{"Content-Type": []string{"application/json"REDACTEDREDACTED,
				Body:       io.NopCloser(strings.NewReader(`{"usage":{"input_tokens":1,"output_tokens":2REDACTEDREDACTED`)),
		REDACTED,
	REDACTED,
REDACTED
	cfg := &config.Config{REDACTED
	cfg.Security.URLAllowlist.Enabled = false
	svc := &OpenAIGatewayService{cfg: cfg, httpUpstream: upstreamREDACTED
	account := &Account{
		ID:          10,
		Name:        "openai-apikey",
		Platform:    PlatformOpenAI,
		Type:        AccountTypeAPIKey,
		Concurrency: 1,
REDACTED
			"api_key":  "sk-test",
			"base_url": "https://example.com",
	REDACTED,
		Extra: map[string]any{"use_responses_api": trueREDACTED,
REDACTED
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPost, "/openai/v1/responses", nil)
	SetOpenAIClientTransport(c, OpenAIClientTransportHTTP)

	body := []byte(`{"model":"gpt-5","stream":false,"input":[{"type":"reasoning","encrypted_content":"gAAA","summary":[{"type":"summary_text","text":"keep me"REDACTED]REDACTED,{"type":"message","content":[{"type":"input_text","text":"hi","nonce":9007199254740993REDACTED]REDACTED]REDACTED`)
	result, err := svc.Forward(context.Background(), c, account, body)
REDACTED
	require.NotNil(t, result)
	require.Len(t, upstream.bodies, 2)
	require.Equal(t, "gAAA", gjson.GetBytes(upstream.bodies[0], "input.0.encrypted_content").String())
	require.Equal(t, "9007199254740993", gjson.GetBytes(upstream.bodies[0], "input.1.content.0.nonce").Raw)
	require.False(t, gjson.GetBytes(upstream.bodies[1], "input.0.encrypted_content").Exists())
	require.Equal(t, "summary_text", gjson.GetBytes(upstream.bodies[1], "input.0.summary.0.type").String())
REDACTED

func TestOpenAIGatewayService_Forward_CodexSparkRejectsEscapedInputImage(t *testing.T) {
	gin.SetMode(gin.TestMode)
	upstream := &httpUpstreamRecorder{
		resp: &http.Response{
			StatusCode: http.StatusOK,
			Header:     http.Header{"Content-Type": []string{"application/json"REDACTEDREDACTED,
			Body:       io.NopCloser(strings.NewReader(`{"usage":{"input_tokens":1,"output_tokens":2REDACTEDREDACTED`)),
	REDACTED,
REDACTED
	cfg := &config.Config{REDACTED
	cfg.Security.URLAllowlist.Enabled = false
	svc := &OpenAIGatewayService{cfg: cfg, httpUpstream: upstreamREDACTED
	account := &Account{
		ID:          5,
		Name:        "openai-apikey",
		Platform:    PlatformOpenAI,
		Type:        AccountTypeAPIKey,
		Concurrency: 1,
REDACTED
			"api_key":  "sk-test",
			"base_url": "https://example.com",
	REDACTED,
		Extra: map[string]any{"use_responses_api": trueREDACTED,
REDACTED
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPost, "/openai/v1/responses", nil)
	SetOpenAIClientTransport(c, OpenAIClientTransportHTTP)

	body := []byte(`{"model":"gpt-5.3-codex-spark","stream":false,"input":[{"type":"input_` + "\\u0069" + `mage","file_id":"file_1"REDACTED]REDACTED`)
	result, err := svc.Forward(context.Background(), c, account, body)
REDACTED
	require.Nil(t, result)
	require.Nil(t, upstream.lastReq)
	require.Equal(t, http.StatusBadRequest, rec.Code)
REDACTED

func TestOpenAIGatewayService_Forward_CodexBridgeInjectionSetsImageBilling(t *testing.T) {
	gin.SetMode(gin.TestMode)
	upstream := &httpUpstreamRecorder{
		resp: &http.Response{
			StatusCode: http.StatusOK,
			Header:     http.Header{"Content-Type": []string{"application/json"REDACTEDREDACTED,
			Body: io.NopCloser(strings.NewReader(
				`{"output":[{"id":"ig_1","type":"image_generation_call","result":"final-image","size":"1024x1024"REDACTED],"usage":{"input_tokens":1,"output_tokens":2REDACTEDREDACTED`,
			)),
	REDACTED,
REDACTED
	cfg := &config.Config{REDACTED
	cfg.Security.URLAllowlist.Enabled = false
	cfg.Gateway.ForceCodexCLI = true
	cfg.Gateway.CodexImageGenerationBridgeEnabled = true
	svc := &OpenAIGatewayService{cfg: cfg, httpUpstream: upstreamREDACTED
	account := &Account{
		ID:          7,
		Name:        "openai-apikey",
		Platform:    PlatformOpenAI,
		Type:        AccountTypeAPIKey,
		Concurrency: 1,
REDACTED
			"api_key":  "sk-test",
			"base_url": "https://example.com",
	REDACTED,
		Extra: map[string]any{"use_responses_api": trueREDACTED,
REDACTED
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPost, "/openai/v1/responses", nil)
	c.Set("api_key", &APIKey{Group: &Group{AllowImageGeneration: trueREDACTEDREDACTED)
	SetOpenAIClientTransport(c, OpenAIClientTransportHTTP)

	body := []byte(`{"model":"gpt-5","stream":false,"input":"draw if needed"REDACTED`)
	result, err := svc.Forward(context.Background(), c, account, body)
REDACTED
	require.NotNil(t, result)
	require.Equal(t, 1, result.ImageCount)
	require.Equal(t, "2K", result.ImageSize)
	require.Equal(t, "gpt-image-2", result.BillingModel)
REDACTED

func TestOpenAIGatewayService_Forward_HTTPDeletesPreviousResponseIDWhenPresent(t *testing.T) {
	gin.SetMode(gin.TestMode)
	cfg := &config.Config{REDACTED
	cfg.Security.URLAllowlist.Enabled = false
	account := &Account{
		ID:          8,
		Name:        "openai-apikey",
		Platform:    PlatformOpenAI,
		Type:        AccountTypeAPIKey,
		Concurrency: 1,
REDACTED
			"api_key":  "sk-test",
			"base_url": "https://example.com",
	REDACTED,
		Extra: map[string]any{"use_responses_api": trueREDACTED,
REDACTED

	for _, body := range [][]byte{
		[]byte(`{"model":"gpt-5","stream":false,"previous_response_id":"","input":"hi"REDACTED`),
		[]byte(`{"model":"gpt-5","stream":false,"previous_response_id":null,"input":"hi"REDACTED`),
REDACTED {
		upstream := &httpUpstreamRecorder{
			resp: &http.Response{
				StatusCode: http.StatusOK,
				Header:     http.Header{"Content-Type": []string{"application/json"REDACTEDREDACTED,
				Body:       io.NopCloser(strings.NewReader(`{"usage":{"input_tokens":1,"output_tokens":2REDACTEDREDACTED`)),
		REDACTED,
	REDACTED
		svc := &OpenAIGatewayService{cfg: cfg, httpUpstream: upstreamREDACTED
		rec := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(rec)
		c.Request = httptest.NewRequest(http.MethodPost, "/openai/v1/responses", nil)
		SetOpenAIClientTransport(c, OpenAIClientTransportHTTP)

		result, err := svc.Forward(context.Background(), c, account, body)
	REDACTED
		require.NotNil(t, result)
		require.False(t, gjson.GetBytes(upstream.lastBody, "previous_response_id").Exists())
REDACTED
REDACTED

func TestOpenAIRequestBodyMayContainEmptyBase64InputImageSeesEscapedJSON(t *testing.T) {
	body := []byte(`{"input":[{"type":"message","content":[{"type":"input_image","image_` + "\\u0075" + `rl":"data:image/png;base64` + "\\u002c" + `   "REDACTED]REDACTED]REDACTED`)

	require.True(t, openAIRequestBodyMayContainEmptyBase64InputImage(body))
REDACTED

func TestOpenAIRequestBodyMayContainEmptyBase64InputImageSeesEscapedImageType(t *testing.T) {
	body := []byte(`{"input":[{"type":"message","content":[{"type":"input_` + "\\u0069" + `mage","image_url":"data:image/png;base64,   "REDACTED]REDACTED]REDACTED`)

	require.True(t, openAIRequestBodyMayContainEmptyBase64InputImage(body))
REDACTED

func TestOpenAIRequestBodyMayContainEmptyBase64InputImageSeesEscapedInputPrefix(t *testing.T) {
	body := []byte(`{"input":[{"type":"message","content":[{"type":"inp` + "\\u0075" + `t_image","image_url":"data:image/png;base64,   "REDACTED]REDACTED]REDACTED`)

	require.True(t, openAIRequestBodyMayContainEmptyBase64InputImage(body))
REDACTED

func TestOpenAIGatewayService_Forward_ImageOnlyModelKeepsSupportedVerbosity(t *testing.T) {
	gin.SetMode(gin.TestMode)
	upstream := &httpUpstreamRecorder{
		resp: &http.Response{
			StatusCode: http.StatusOK,
			Header:     http.Header{"Content-Type": []string{"application/json"REDACTEDREDACTED,
			Body:       io.NopCloser(strings.NewReader(`{"usage":{"input_tokens":1,"output_tokens":2REDACTEDREDACTED`)),
	REDACTED,
REDACTED
	cfg := &config.Config{REDACTED
	cfg.Security.URLAllowlist.Enabled = false
	svc := &OpenAIGatewayService{cfg: cfg, httpUpstream: upstreamREDACTED
	account := &Account{
		ID:          6,
		Name:        "openai-apikey",
		Platform:    PlatformOpenAI,
		Type:        AccountTypeAPIKey,
		Concurrency: 1,
REDACTED
			"api_key":  "sk-test",
			"base_url": "https://example.com",
	REDACTED,
		Extra: map[string]any{"use_responses_api": trueREDACTED,
REDACTED
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPost, "/openai/v1/responses", nil)
	SetOpenAIClientTransport(c, OpenAIClientTransportHTTP)

	body := []byte(`{"model":"gpt-image-2","stream":false,"text":{"verbosity":"low"REDACTED,"input":"draw"REDACTED`)
	result, err := svc.Forward(context.Background(), c, account, body)
REDACTED
	require.NotNil(t, result)
	require.Equal(t, "low", gjson.GetBytes(upstream.lastBody, "text.verbosity").String())
	require.Equal(t, openAIImagesResponsesMainModel, gjson.GetBytes(upstream.lastBody, "model").String())
REDACTED

func TestExtractOpenAIRequestMetaFromBody(t *testing.T) {
	tests := []struct {
		name          string
		body          []byte
		wantModel     string
		wantStream    bool
		wantPromptKey string
REDACTED{
		{
			name:          "完整字段",
			body:          []byte(`{"model":"gpt-5","stream":true,"prompt_cache_key":" ses-1 "REDACTED`),
			wantModel:     "gpt-5",
			wantStream:    true,
			wantPromptKey: "ses-1",
	REDACTED,
		{
			name:          "缺失可选字段",
			body:          []byte(`{"model":"gpt-4"REDACTED`),
			wantModel:     "gpt-4",
			wantStream:    false,
			wantPromptKey: "",
	REDACTED,
		{
			name:          "空请求体",
			body:          nil,
			wantModel:     "",
			wantStream:    false,
			wantPromptKey: "",
	REDACTED,
REDACTED

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			model, stream, promptKey := extractOpenAIRequestMetaFromBody(tt.body)
			require.Equal(t, tt.wantModel, model)
			require.Equal(t, tt.wantStream, stream)
			require.Equal(t, tt.wantPromptKey, promptKey)
	REDACTED)
REDACTED
REDACTED

func TestExtractOpenAIReasoningEffortFromBody(t *testing.T) {
	tests := []struct {
		name      string
		body      []byte
		model     string
		wantNil   bool
		wantValue string
REDACTED{
		{
			name:      "优先读取 reasoning.effort",
			body:      []byte(`{"reasoning":{"effort":"medium"REDACTEDREDACTED`),
			model:     "gpt-5-high",
			wantNil:   false,
			wantValue: "medium",
	REDACTED,
		{
			name:      "兼容 reasoning_effort",
			body:      []byte(`{"reasoning_effort":"x-high"REDACTED`),
			model:     "",
			wantNil:   false,
			wantValue: "xhigh",
	REDACTED,
		{
			name:    "minimal 归一化为空",
			body:    []byte(`{"reasoning":{"effort":"minimal"REDACTEDREDACTED`),
			model:   "gpt-5-high",
			wantNil: true,
	REDACTED,
		{
			name:      "缺失字段时从模型后缀推导",
			body:      []byte(`{"input":"hi"REDACTED`),
			model:     "gpt-5-high",
			wantNil:   false,
			wantValue: "high",
	REDACTED,
		{
			name:    "未知后缀不返回",
			body:    []byte(`{"input":"hi"REDACTED`),
			model:   "gpt-5-unknown",
			wantNil: true,
	REDACTED,
REDACTED

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := extractOpenAIReasoningEffortFromBody(tt.body, tt.model)
			if tt.wantNil {
				require.Nil(t, got)
				return
		REDACTED
			require.NotNil(t, got)
			require.Equal(t, tt.wantValue, *got)
	REDACTED)
REDACTED
REDACTED

func TestGetOpenAIRequestBodyMap_ParseError(t *testing.T) {
	_, err := getOpenAIRequestBodyMap(nil, []byte(`{invalid-json`))
REDACTED
	require.Contains(t, err.Error(), "parse request")
REDACTED

func TestGetOpenAIRequestBodyMap_DoesNotWriteContextCache(t *testing.T) {
	gin.SetMode(gin.TestMode)
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)

	got, err := getOpenAIRequestBodyMap(c, []byte(`{"model":"gpt-5","stream":trueREDACTED`))
REDACTED
	require.Equal(t, "gpt-5", got["model"])
	require.Empty(t, c.Keys)
REDACTED

func TestSanitizeEmptyBase64InputImagesInOpenAIRequestBodyMap(t *testing.T) {
	var reqBody map[string]any
	require.NoError(t, json.Unmarshal([]byte(`{
		"model":"gpt-5.4",
		"input":[
			{"role":"user","content":[
				{"type":"input_text","text":"Describe this"REDACTED,
				{"type":"input_image","image_url":"data:image/png;base64,   "REDACTED,
				{"type":"input_image","image_url":"data:image/png;base64,abc123"REDACTED
			]REDACTED,
			{"role":"user","content":[
				{"type":"input_image","image_url":"data:image/png;base64,"REDACTED
			]REDACTED,
			{"type":"input_image","image_url":"data:image/png;base64,"REDACTED,
			{"type":"input_image","image_url":"data:image/png;base64,top-level-valid"REDACTED
		]
REDACTED`), &reqBody))

	require.True(t, sanitizeEmptyBase64InputImagesInOpenAIRequestBodyMap(reqBody))

	normalized, err := json.Marshal(reqBody)
REDACTED
	require.JSONEq(t, `{
		"model":"gpt-5.4",
		"input":[
			{"role":"user","content":[
				{"type":"input_text","text":"Describe this"REDACTED,
				{"type":"input_image","image_url":"data:image/png;base64,abc123"REDACTED
			]REDACTED,
			{"type":"input_image","image_url":"data:image/png;base64,top-level-valid"REDACTED
		]
REDACTED`, string(normalized))
REDACTED

func TestSanitizeEmptyBase64InputImagesInOpenAIBody(t *testing.T) {
	body, changed, err := sanitizeEmptyBase64InputImagesInOpenAIBody([]byte(`{
		"model":"gpt-5.4",
		"stream":true,
		"input":[
			{"role":"user","content":[
				{"type":"input_text","text":"Describe this"REDACTED,
				{"type":"input_image","image_url":"data:image/png;base64,"REDACTED
			]REDACTED
		]
REDACTED`))
REDACTED
	require.True(t, changed)
	require.JSONEq(t, `{
		"model":"gpt-5.4",
		"stream":true,
		"input":[
			{"role":"user","content":[
				{"type":"input_text","text":"Describe this"REDACTED
			]REDACTED
		]
REDACTED`, string(body))
REDACTED
