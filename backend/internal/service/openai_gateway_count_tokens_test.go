package service

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/pkg/apicompat"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

type countTokensRuntimeStateRepo struct {
	AccountRepository
	tempUnschedCalls int
	setErrorCalls    int
REDACTED

func (r *countTokensRuntimeStateRepo) SetTempUnschedulable(_ context.Context, _ int64, _ time.Time, _ string) error {
	r.tempUnschedCalls++
	return nil
REDACTED

func (r *countTokensRuntimeStateRepo) SetError(_ context.Context, _ int64, _ string) error {
	r.setErrorCalls++
	return nil
REDACTED

func TestOpenAIGatewayService_ForwardCountTokensAsAnthropic_APIKeyUsesResponsesInputTokens(t *testing.T) {
	gin.SetMode(gin.TestMode)

	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	body := []byte(`{"model":"claude-sonnet-4-5","system":"You are helpful.","messages":[{"role":"user","content":"hello"REDACTED],"tools":[{"name":"lookup","input_schema":{"type":"object"REDACTEDREDACTED]REDACTED`)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/messages/count_tokens", bytes.NewReader(body))
	c.Request.Header.Set("Content-Type", "application/json")

	upstream := &httpUpstreamRecorder{resp: &http.Response{
		StatusCode: http.StatusOK,
		Header:     http.Header{"Content-Type": []string{"application/json"REDACTEDREDACTED,
		Body:       io.NopCloser(strings.NewReader(`{"object":"response.input_tokens","input_tokens":42REDACTED`)),
REDACTEDREDACTED

	svc := &OpenAIGatewayService{
		cfg: &config.Config{Security: config.SecurityConfig{URLAllowlist: config.URLAllowlistConfig{
			Enabled:           false,
			AllowInsecureHTTP: true,
	REDACTEDREDACTEDREDACTED,
		httpUpstream: upstream,
REDACTED
	account := &Account{
		ID:          101,
		Name:        "openai-apikey",
		Platform:    PlatformOpenAI,
		Type:        AccountTypeAPIKey,
		Concurrency: 1,
REDACTED
			"api_key":  "sk-test",
			"base_url": "http://upstream.example",
	REDACTED,
		Status:      StatusActive,
		Schedulable: true,
REDACTED

	err := svc.ForwardCountTokensAsAnthropic(context.Background(), c, account, body, "gpt-5.3-codex")
REDACTED
	require.Equal(t, http.StatusOK, rec.Code)
	require.JSONEq(t, `{"input_tokens":42REDACTED`, rec.Body.String())
	require.NotNil(t, upstream.lastReq)
	require.Equal(t, "http://upstream.example/v1/responses/input_tokens", upstream.lastReq.URL.String())
	require.Equal(t, "Bearer sk-test", upstream.lastReq.Header.Get("authorization"))
	require.Equal(t, "gpt-5.3-codex", gjson.GetBytes(upstream.lastBody, "model").String())
	require.True(t, gjson.GetBytes(upstream.lastBody, "input").Exists())
	require.False(t, gjson.GetBytes(upstream.lastBody, "messages").Exists())
REDACTED

func TestOpenAIGatewayService_ForwardCountTokensAsAnthropic_OAuthFallsBackWhenPlatformEndpointUnsupported(t *testing.T) {
	gin.SetMode(gin.TestMode)

	body := []byte(`{"model":"claude-opus-4-1","messages":[{"role":"user","content":"hello"REDACTED]REDACTED`)
	account := &Account{
		ID:          202,
		Name:        "openai-oauth",
		Platform:    PlatformOpenAI,
		Type:        AccountTypeOAuth,
		Concurrency: 1,
REDACTED
			"access_token":  "oauth-token",
			"refresh_token": "oauth-refresh-token",
	REDACTED,
		Status:      StatusActive,
		Schedulable: true,
REDACTED

	prepared, err := prepareOpenAIInputTokensCountRequest(body, account, "gpt-5.4")
REDACTED
	expectedEstimate, err := estimateOpenAIInputTokens(prepared.Request)
REDACTED

	cases := []struct {
		name       string
		statusCode int
		body       string
REDACTED{
		{
			name:       "401_missing_responses_write_scope",
			statusCode: http.StatusUnauthorized,
			body:       `{"error":{"type":"invalid_request_error","code":"missing_scope","message":"You have insufficient permissions for this operation. Missing scopes: api.responses.write."REDACTEDREDACTED`,
	REDACTED,
		{
			name:       "403_missing_responses_write_scope",
			statusCode: http.StatusForbidden,
			body:       `{"error":{"type":"invalid_request_error","code":"missing_scope","message":"Missing scopes: api.responses.write"REDACTEDREDACTED`,
	REDACTED,
		{
			name:       "404_input_tokens_unsupported",
			statusCode: http.StatusNotFound,
			body:       `{"error":{"type":"invalid_request_error","message":"The /v1/responses/input_tokens endpoint was not found"REDACTEDREDACTED`,
	REDACTED,
REDACTED

	for _, tt := range cases {
		t.Run(tt.name, func(t *testing.T) {
			rec := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(rec)
			c.Request = httptest.NewRequest(http.MethodPost, "/v1/messages/count_tokens", bytes.NewReader(body))
			c.Request.Header.Set("Content-Type", "application/json")
			c.Request.Header.Set("User-Agent", "Claude-Code/1.0")

			upstream := &httpUpstreamRecorder{resp: &http.Response{
				StatusCode: tt.statusCode,
				Header:     http.Header{"Content-Type": []string{"application/json"REDACTEDREDACTED,
				Body:       io.NopCloser(strings.NewReader(tt.body)),
		REDACTEDREDACTED
			repo := &countTokensRuntimeStateRepo{REDACTED
			svc := &OpenAIGatewayService{
				cfg:              &config.Config{REDACTED,
				httpUpstream:     upstream,
				rateLimitService: &RateLimitService{accountRepo: repo, cfg: &config.Config{REDACTEDREDACTED,
		REDACTED

			err := svc.ForwardCountTokensAsAnthropic(context.Background(), c, account, body, "gpt-5.4")
		REDACTED
			require.Equal(t, http.StatusOK, rec.Code)
			require.JSONEq(t, `{"input_tokens":`+strconv.Itoa(expectedEstimate)+`REDACTED`, rec.Body.String())
			require.NotNil(t, upstream.lastReq)
			require.Equal(t, "https://api.openai.com/v1/responses/input_tokens", upstream.lastReq.URL.String())
			require.Equal(t, "Bearer oauth-token", upstream.lastReq.Header.Get("authorization"))
			require.Empty(t, upstream.lastReq.Header.Get("Chatgpt-Account-Id"))
			require.Zero(t, repo.tempUnschedCalls, "OAuth input_tokens unsupported errors must not temp-unschedule the account")
			require.Zero(t, repo.setErrorCalls, "OAuth input_tokens unsupported errors must not mark the account error")
	REDACTED)
REDACTED
REDACTED

func TestOpenAIGatewayService_OpenAIOAuthInputTokensFallbackUsesMinimumWhenEstimateFails(t *testing.T) {
	gin.SetMode(gin.TestMode)

	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	prepared := &openAIInputTokensCountPrepared{
		Request: openAIInputTokensCountRequest{
			Model: "gpt-5",
			Input: json.RawMessage(`[`),
	REDACTED,
		UpstreamModel: "gpt-5",
REDACTED

	writeOpenAIOAuthInputTokensFallback(c, &Account{ID: 303REDACTED, prepared, http.StatusUnauthorized)

	require.Equal(t, http.StatusOK, rec.Code)
	require.JSONEq(t, `{"input_tokens":1REDACTED`, rec.Body.String())
REDACTED

func TestEstimateOpenAIInputTokens_RequestSamples(t *testing.T) {
	cases := []struct {
		name string
		req  openAIInputTokensCountRequest
		want int
REDACTED{
		{
			name: "simple text input",
			req: openAIInputTokensCountRequest{
				Model: "gpt-5",
				Input: json.RawMessage(`[{"role":"user","content":"hello world"REDACTED]`),
		REDACTED,
			want: 6,
	REDACTED,
		{
			name: "instructions plus tool schema",
			req: openAIInputTokensCountRequest{
				Model:        "gpt-5",
				Instructions: "You are helpful.",
				Input:        json.RawMessage(`[{"role":"user","content":"lookup weather in shanghai"REDACTED]`),
				Tools: []apicompat.ResponsesTool{
					{
						Type:        "function",
						Name:        "lookup_weather",
						Description: "Look up current weather",
						Parameters:  json.RawMessage(`{"type":"object","properties":{"city":{"type":"string"REDACTEDREDACTED,"required":["city"]REDACTED`),
				REDACTED,
			REDACTED,
		REDACTED,
			want: 50,
	REDACTED,
		{
			name: "input parts and tool output",
			req: openAIInputTokensCountRequest{
				Model: "gpt-4.1",
				Input: json.RawMessage(`[
					{"role":"user","content":[{"type":"input_text","text":"first line"REDACTED,{"type":"input_text","text":"second line"REDACTED]REDACTED,
					{"type":"function_call_output","call_id":"call_123","output":"{\"ok\":trueREDACTED"REDACTED
				]`),
		REDACTED,
			want: 24,
	REDACTED,
REDACTED

	for _, tt := range cases {
		t.Run(tt.name, func(t *testing.T) {
			got, err := estimateOpenAIInputTokens(tt.req)
		REDACTED
			require.Equal(t, tt.want, got)
	REDACTED)
REDACTED
REDACTED

func TestOpenAIInputTokensEncodingForModel(t *testing.T) {
	cases := []struct {
		model string
		want  string
REDACTED{
		{model: "gpt-5", want: "o200k_base"REDACTED,
		{model: "gpt-5.3-codex", want: "o200k_base"REDACTED,
		{model: "gpt-4o-mini", want: "o200k_base"REDACTED,
		{model: "gpt-4.1", want: "o200k_base"REDACTED,
		{model: "gpt-4-turbo", want: "cl100k_base"REDACTED,
		{model: "gpt-3.5-turbo", want: "cl100k_base"REDACTED,
REDACTED

	for _, tt := range cases {
		t.Run(tt.model, func(t *testing.T) {
			require.Equal(t, tt.want, string(openAIInputTokensEncodingForModel(tt.model)))
	REDACTED)
REDACTED
REDACTED

func TestEstimateOpenAIInputTokens_CompareWithOpenAIAPI(t *testing.T) {
	apiKey := strings.TrimSpace(os.Getenv("OPENAI_API_KEY"))
	if apiKey == "" {
		t.Skip("OPENAI_API_KEY not set")
REDACTED

	client := &http.Client{Timeout: 30 * time.SecondREDACTED
	cases := []struct {
		name               string
		anthropicBody      []byte
		defaultOpenAIModel string
REDACTED{
		{
			name:               "simple user text",
			defaultOpenAIModel: "gpt-5",
			anthropicBody:      []byte(`{"model":"claude-sonnet-4-5","messages":[{"role":"user","content":"hello world from sub2api"REDACTED]REDACTED`),
	REDACTED,
		{
			name:               "system plus tool",
			defaultOpenAIModel: "gpt-5",
			anthropicBody:      []byte(`{"model":"claude-sonnet-4-5","system":"You are helpful.","messages":[{"role":"user","content":"find weather in shanghai"REDACTED],"tools":[{"name":"lookup_weather","description":"Look up current weather","input_schema":{"type":"object","properties":{"city":{"type":"string"REDACTEDREDACTED,"required":["city"]REDACTEDREDACTED]REDACTED`),
	REDACTED,
		{
			name:               "multi turn text",
			defaultOpenAIModel: "gpt-4.1",
			anthropicBody:      []byte(`{"model":"claude-opus-4-1","messages":[{"role":"user","content":"summarize this repo"REDACTED,{"role":"assistant","content":"which repo?"REDACTED,{"role":"user","content":"sub2api"REDACTED]REDACTED`),
	REDACTED,
REDACTED

	account := &Account{Platform: PlatformOpenAI, Type: AccountTypeAPIKeyREDACTED
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			prepared, err := prepareOpenAIInputTokensCountRequest(tc.anthropicBody, account, tc.defaultOpenAIModel)
		REDACTED

			estimated, err := estimateOpenAIInputTokens(prepared.Request)
		REDACTED

			actual, err := callOpenAIInputTokensAPIForTest(client, apiKey, prepared.Request)
		REDACTED

			diff := estimated - actual
			if diff < 0 {
				diff = -diff
		REDACTED
			t.Logf("model=%s estimated=%d actual=%d diff=%d", prepared.Request.Model, estimated, actual, diff)
			require.LessOrEqual(t, diff, maxLocalInt(24, actual/4))
	REDACTED)
REDACTED
REDACTED

func callOpenAIInputTokensAPIForTest(client *http.Client, apiKey string, reqBody openAIInputTokensCountRequest) (int, error) {
	body, err := marshalOpenAIUpstreamJSON(reqBody)
	if err != nil {
		return 0, err
REDACTED
	req, err := http.NewRequest(http.MethodPost, openaiPlatformAPIInputTokensURL, bytes.NewReader(body))
	if err != nil {
		return 0, err
REDACTED
	req.Header.Set("authorization", "Bearer "+apiKey)
	req.Header.Set("content-type", "application/json")
	req.Header.Set("accept", "application/json")

	resp, err := client.Do(req)
	if err != nil {
		return 0, err
REDACTED
	defer func() { _ = resp.Body.Close() REDACTED()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return 0, err
REDACTED
	if resp.StatusCode >= 400 {
		return 0, fmt.Errorf("openai input_tokens api error: status=%d body=%s", resp.StatusCode, string(respBody))
REDACTED

	value := gjson.GetBytes(respBody, "input_tokens")
	if !value.Exists() {
		return 0, fmt.Errorf("openai input_tokens api missing input_tokens: %s", string(respBody))
REDACTED
	return int(value.Int()), nil
REDACTED

func maxLocalInt(a, b int) int {
	if a > b {
		return a
REDACTED
	return b
REDACTED
