package service

import (
	"bytes"
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

func TestAccountTestService_TestAccountConnection_OpenAICompactOAuthSuccessPersistsSupport(t *testing.T) {
	gin.SetMode(gin.TestMode)

	updateCalls := make(chan map[string]any, 1)
	account := Account{
		ID:          1,
		Name:        "openai-oauth",
		Platform:    PlatformOpenAI,
		Type:        AccountTypeOAuth,
		Status:      StatusActive,
		Schedulable: true,
		Concurrency: 1,
REDACTED
			"access_token":               "oauth-token",
			"chatgpt_account_id":         "chatgpt-acc",
			"chatgpt_account_is_fedramp": true,
	REDACTED,
REDACTED
	repo := &snapshotUpdateAccountRepo{
		stubOpenAIAccountRepo: stubOpenAIAccountRepo{accounts: []Account{accountREDACTEDREDACTED,
		updateExtraCalls:      updateCalls,
REDACTED
	upstream := &httpUpstreamRecorder{resp: &http.Response{
		StatusCode: http.StatusOK,
		Header:     http.Header{"Content-Type": []string{"application/json"REDACTED, "x-request-id": []string{"rid-probe"REDACTEDREDACTED,
		Body:       io.NopCloser(strings.NewReader(`{"id":"cmp_probe","status":"completed"REDACTED`)),
REDACTEDREDACTED
	svc := &AccountTestService{
		accountRepo:  repo,
		httpUpstream: upstream,
REDACTED

	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPost, "/api/v1/admin/accounts/1/test", bytes.NewReader(nil))

	err := svc.TestAccountConnection(c, account.ID, "gpt-5.4", "", AccountTestModeCompact)
REDACTED

	require.Equal(t, chatgptCodexAPIURL+"/compact", upstream.lastReq.URL.String())
	require.Equal(t, "chatgpt.com", upstream.lastReq.Host)
	require.Equal(t, "application/json", upstream.lastReq.Header.Get("Accept"))
	require.Equal(t, codexCLIVersion, upstream.lastReq.Header.Get("Version"))
	require.NotEmpty(t, upstream.lastReq.Header.Get("Session_Id"))
	require.Equal(t, HTTPUpstreamProfileOpenAI, HTTPUpstreamProfileFromContext(upstream.lastReq.Context()))
	require.Equal(t, codexCLIUserAgent, upstream.lastReq.Header.Get("User-Agent"))
	require.Equal(t, "chatgpt-acc", upstream.lastReq.Header.Get("chatgpt-account-id"))
	require.Equal(t, "true", upstream.lastReq.Header.Get("x-openai-fedramp"))
	require.Equal(t, "gpt-5.4", gjson.GetBytes(upstream.lastBody, "model").String())

	updates := <-updateCalls
	require.Equal(t, true, updates["openai_compact_supported"])
	require.Equal(t, http.StatusOK, updates["openai_compact_last_status"])
	require.Contains(t, rec.Body.String(), `"type":"test_complete"`)
REDACTED

func TestAccountTestService_TestAccountConnection_OpenAICompactOAuth404MarksUnsupported(t *testing.T) {
	gin.SetMode(gin.TestMode)

	updateCalls := make(chan map[string]any, 1)
	account := Account{
		ID:          2,
		Name:        "openai-oauth",
		Platform:    PlatformOpenAI,
		Type:        AccountTypeOAuth,
		Status:      StatusActive,
		Schedulable: true,
		Concurrency: 1,
REDACTED
			"access_token":       "oauth-token",
			"chatgpt_account_id": "chatgpt-acc",
	REDACTED,
REDACTED
	repo := &snapshotUpdateAccountRepo{
		stubOpenAIAccountRepo: stubOpenAIAccountRepo{accounts: []Account{accountREDACTEDREDACTED,
		updateExtraCalls:      updateCalls,
REDACTED
	upstream := &httpUpstreamRecorder{resp: &http.Response{
		StatusCode: http.StatusNotFound,
		Header:     http.Header{"Content-Type": []string{"application/json"REDACTEDREDACTED,
		Body:       io.NopCloser(strings.NewReader(`404 page not found`)),
REDACTEDREDACTED
	svc := &AccountTestService{
		accountRepo:  repo,
		httpUpstream: upstream,
REDACTED

	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPost, "/api/v1/admin/accounts/2/test", bytes.NewReader(nil))

	err := svc.TestAccountConnection(c, account.ID, "gpt-5.4", "", AccountTestModeCompact)
REDACTED

	updates := <-updateCalls
	require.Equal(t, false, updates["openai_compact_supported"])
	require.Equal(t, http.StatusNotFound, updates["openai_compact_last_status"])
	require.Contains(t, rec.Body.String(), `"type":"error"`)
REDACTED

func TestAccountTestService_TestAccountConnection_OpenAICompactAPIKeyUsesCompactPath(t *testing.T) {
	gin.SetMode(gin.TestMode)

	updateCalls := make(chan map[string]any, 1)
	account := Account{
		ID:          3,
		Name:        "openai-apikey",
		Platform:    PlatformOpenAI,
		Type:        AccountTypeAPIKey,
		Status:      StatusActive,
		Schedulable: true,
		Concurrency: 1,
REDACTED
			"api_key":               "sk-test",
			"base_url":              "https://example.com/v1",
			"compact_model_mapping": map[string]any{"gpt-5.4": "gpt-5.4-openai-compact"REDACTED,
	REDACTED,
REDACTED
	repo := &snapshotUpdateAccountRepo{
		stubOpenAIAccountRepo: stubOpenAIAccountRepo{accounts: []Account{accountREDACTEDREDACTED,
		updateExtraCalls:      updateCalls,
REDACTED
	upstream := &httpUpstreamRecorder{resp: &http.Response{
		StatusCode: http.StatusOK,
		Header:     http.Header{"Content-Type": []string{"application/json"REDACTEDREDACTED,
		Body:       io.NopCloser(strings.NewReader(`{"id":"cmp_probe_apikey","status":"completed"REDACTED`)),
REDACTEDREDACTED
	svc := &AccountTestService{
		accountRepo:  repo,
		httpUpstream: upstream,
		cfg:          &config.Config{Security: config.SecurityConfig{URLAllowlist: config.URLAllowlistConfig{Enabled: falseREDACTEDREDACTEDREDACTED,
REDACTED

	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPost, "/api/v1/admin/accounts/3/test", bytes.NewReader(nil))

	err := svc.TestAccountConnection(c, account.ID, "gpt-5.4", "", AccountTestModeCompact)
REDACTED

	require.Equal(t, "https://example.com/v1/responses/compact", upstream.lastReq.URL.String())
	require.Equal(t, "gpt-5.4-openai-compact", gjson.GetBytes(upstream.lastBody, "model").String())
	updates := <-updateCalls
	require.Equal(t, true, updates["openai_compact_supported"])
REDACTED

func TestAccountTestService_TestAccountConnection_OpenAICompactAPIKeyDefaultBaseURLUsesV1Path(t *testing.T) {
	gin.SetMode(gin.TestMode)

	updateCalls := make(chan map[string]any, 1)
	account := Account{
		ID:          4,
		Name:        "openai-apikey-default",
		Platform:    PlatformOpenAI,
		Type:        AccountTypeAPIKey,
		Status:      StatusActive,
		Schedulable: true,
		Concurrency: 1,
REDACTED
			"api_key": "sk-test",
	REDACTED,
REDACTED
	repo := &snapshotUpdateAccountRepo{
		stubOpenAIAccountRepo: stubOpenAIAccountRepo{accounts: []Account{accountREDACTEDREDACTED,
		updateExtraCalls:      updateCalls,
REDACTED
	upstream := &httpUpstreamRecorder{resp: &http.Response{
		StatusCode: http.StatusOK,
		Header:     http.Header{"Content-Type": []string{"application/json"REDACTEDREDACTED,
		Body:       io.NopCloser(strings.NewReader(`{"id":"cmp_probe_apikey_default","status":"completed"REDACTED`)),
REDACTEDREDACTED
	svc := &AccountTestService{
		accountRepo:  repo,
		httpUpstream: upstream,
		cfg:          &config.Config{Security: config.SecurityConfig{URLAllowlist: config.URLAllowlistConfig{Enabled: falseREDACTEDREDACTEDREDACTED,
REDACTED

	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPost, "/api/v1/admin/accounts/4/test", bytes.NewReader(nil))

	err := svc.TestAccountConnection(c, account.ID, "gpt-5.4", "", AccountTestModeCompact)
REDACTED
	require.Equal(t, "https://api.openai.com/v1/responses/compact", upstream.lastReq.URL.String())
	<-updateCalls
REDACTED
