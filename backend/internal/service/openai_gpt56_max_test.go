package service

import (
	"context"
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

func TestNormalizeOpenAIReasoningEffortForGPT56(t *testing.T) {
	tests := []struct {
		name  string
		raw   string
		model string
		want  string
REDACTED{
		{name: "Sol 保留 max", raw: "max", model: "gpt-5.6-sol", want: "max"REDACTED,
		{name: "Terra 保留 max", raw: "max", model: "openai/gpt-5.6-terra", want: "max"REDACTED,
		{name: "Luna 后缀保留 max", raw: "max", model: "gpt-5.6-luna-2026-07-09", want: "max"REDACTED,
		{name: "其他模型沿用 xhigh", raw: "max", model: "deepseek-v4-pro", want: "xhigh"REDACTED,
REDACTED

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			require.Equal(t, tt.want, normalizeOpenAIReasoningEffortForModel(tt.raw, tt.model))
	REDACTED)
REDACTED
REDACTED

func TestNormalizeOpenAICodexCompactReasoningEffortDowngradesMax(t *testing.T) {
	body := []byte(`{"model":"gpt-5.6-sol","input":"compact me","reasoning":{"effort":"max","summary":"auto"REDACTEDREDACTED`)

	normalized, changed, err := normalizeOpenAICodexCompactReasoningEffort(body, "gpt-5.6-sol")

REDACTED
	require.True(t, changed)
	require.Equal(t, "gpt-5.6-sol", gjson.GetBytes(normalized, "model").String())
	require.Equal(t, "xhigh", gjson.GetBytes(normalized, "reasoning.effort").String())
	require.Equal(t, "auto", gjson.GetBytes(normalized, "reasoning.summary").String())
REDACTED

func TestNormalizeOpenAICodexCompactReasoningEffortForAccountScopesCompatibility(t *testing.T) {
	gin.SetMode(gin.TestMode)
	body := []byte(`{"model":"gpt-5.6-sol","input":"compact me","reasoning":{"effort":"max"REDACTEDREDACTED`)

	tests := []struct {
		name    string
		path    string
		account *Account
		changed bool
		want    string
REDACTED{
		{
			name:    "OpenAI OAuth compact 降级",
			path:    "/openai/v1/responses/compact",
			account: &Account{Platform: PlatformOpenAI, Type: AccountTypeOAuthREDACTED,
			changed: true,
			want:    "xhigh",
	REDACTED,
		{
			name:    "OpenAI OAuth 普通请求保留",
			path:    "/openai/v1/responses",
			account: &Account{Platform: PlatformOpenAI, Type: AccountTypeOAuthREDACTED,
			want:    "max",
	REDACTED,
		{
			name:    "OpenAI API Key compact 保留",
			path:    "/openai/v1/responses/compact",
			account: &Account{Platform: PlatformOpenAI, Type: AccountTypeAPIKeyREDACTED,
			want:    "max",
	REDACTED,
		{
			name:    "Grok OAuth compact 保留",
			path:    "/openai/v1/responses/compact",
			account: &Account{Platform: PlatformGrok, Type: AccountTypeOAuthREDACTED,
			want:    "max",
	REDACTED,
REDACTED

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rec := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(rec)
			c.Request = httptest.NewRequest(http.MethodPost, tt.path, nil)

			normalized, changed, err := normalizeOpenAICodexCompactReasoningEffortForAccount(c, tt.account, body)

		REDACTED
			require.Equal(t, tt.changed, changed)
			require.Equal(t, tt.want, gjson.GetBytes(normalized, "reasoning.effort").String())
	REDACTED)
REDACTED
REDACTED

func TestOpenAIGatewayServiceForwardPreservesGPT56MaxEffort(t *testing.T) {
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
	SetOpenAIClientTransport(c, OpenAIClientTransportHTTP)

	body := []byte(`{"model":"gpt-5.6-sol","stream":false,"reasoning":{"effort":"max"REDACTED,"input":"hello"REDACTED`)
	result, err := svc.Forward(context.Background(), c, account, body)

REDACTED
	require.NotNil(t, result)
	require.Equal(t, "max", gjson.GetBytes(upstream.lastBody, "reasoning.effort").String())
	require.NotNil(t, result.ReasoningEffort)
	require.Equal(t, "max", *result.ReasoningEffort)
REDACTED

func TestOpenAIGatewayServiceForwardPreservesMappedGPT56MaxEffort(t *testing.T) {
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
		ID:          9,
		Name:        "openai-apikey-mapped",
		Platform:    PlatformOpenAI,
		Type:        AccountTypeAPIKey,
		Concurrency: 1,
REDACTED
			"api_key":  "sk-test",
			"base_url": "https://example.com",
			"model_mapping": map[string]any{
				"sol": "gpt-5.6-sol",
		REDACTED,
	REDACTED,
		Extra: map[string]any{"use_responses_api": trueREDACTED,
REDACTED
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPost, "/openai/v1/responses", nil)
	SetOpenAIClientTransport(c, OpenAIClientTransportHTTP)

	body := []byte(`{"model":"sol","stream":false,"reasoning":{"effort":"max"REDACTED,"input":"hello"REDACTED`)
	result, err := svc.Forward(context.Background(), c, account, body)

REDACTED
	require.NotNil(t, result)
	require.Equal(t, "gpt-5.6-sol", gjson.GetBytes(upstream.lastBody, "model").String())
	require.Equal(t, "max", gjson.GetBytes(upstream.lastBody, "reasoning.effort").String())
	require.NotNil(t, result.ReasoningEffort)
	require.Equal(t, "max", *result.ReasoningEffort)
REDACTED

func TestOpenAIGatewayServiceForwardOAuthCompactDowngradesMaxEffort(t *testing.T) {
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
		ID:          8,
		Name:        "openai-oauth",
		Platform:    PlatformOpenAI,
		Type:        AccountTypeOAuth,
		Concurrency: 1,
REDACTED
			"access_token":       "oauth-token",
			"chatgpt_account_id": "chatgpt-acc",
	REDACTED,
		Status:      StatusActive,
		Schedulable: true,
REDACTED
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPost, "/openai/v1/responses/compact", nil)
	SetOpenAIClientTransport(c, OpenAIClientTransportHTTP)

	body := []byte(`{"model":"gpt-5.6-sol","instructions":"compact-test","input":"hello","reasoning":{"effort":"max"REDACTEDREDACTED`)
	result, err := svc.Forward(context.Background(), c, account, body)

REDACTED
	require.NotNil(t, result)
	require.NotNil(t, upstream.lastReq)
	require.Equal(t, chatgptCodexURL+"/compact", upstream.lastReq.URL.String())
	require.Equal(t, "xhigh", gjson.GetBytes(upstream.lastBody, "reasoning.effort").String())
	require.NotNil(t, result.ReasoningEffort)
	require.Equal(t, "xhigh", *result.ReasoningEffort)
REDACTED

func TestOpenAIGatewayServiceForwardOAuthResponsesPreservesMaxEffort(t *testing.T) {
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
		ID:          10,
		Name:        "openai-oauth-responses",
		Platform:    PlatformOpenAI,
		Type:        AccountTypeOAuth,
		Concurrency: 1,
REDACTED
			"access_token":       "oauth-token",
			"chatgpt_account_id": "chatgpt-acc",
	REDACTED,
		Status:      StatusActive,
		Schedulable: true,
REDACTED
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPost, "/openai/v1/responses", nil)
	SetOpenAIClientTransport(c, OpenAIClientTransportHTTP)

	body := []byte(`{"model":"gpt-5.6-sol","instructions":"response-test","input":"hello","reasoning":{"effort":"max"REDACTEDREDACTED`)
	result, err := svc.Forward(context.Background(), c, account, body)

REDACTED
	require.NotNil(t, result)
	require.NotNil(t, upstream.lastReq)
	require.Equal(t, chatgptCodexURL, upstream.lastReq.URL.String())
	require.Equal(t, "max", gjson.GetBytes(upstream.lastBody, "reasoning.effort").String())
	require.NotNil(t, result.ReasoningEffort)
	require.Equal(t, "max", *result.ReasoningEffort)
REDACTED
