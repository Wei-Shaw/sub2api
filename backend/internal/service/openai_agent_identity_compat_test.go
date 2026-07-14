package service

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestAccountTestServiceOpenAICompactAgentIdentityUsesFreshAssertion(t *testing.T) {
	gin.SetMode(gin.TestMode)
	key, privateKey := newTestAgentIdentityKey(t)
	account := Account{
		ID:          21,
		Name:        "agent-identity",
		Platform:    PlatformOpenAI,
		Type:        AccountTypeOAuth,
		Status:      StatusActive,
		Schedulable: true,
		Concurrency: 1,
REDACTED
			"auth_mode":                  OpenAIAuthModeAgentIdentity,
			"agent_runtime_id":           key.runtimeID,
			"agent_private_key":          privateKey,
			"task_id":                    key.taskID,
			"chatgpt_account_id":         "account-agent-test",
			"chatgpt_account_is_fedramp": true,
	REDACTED,
REDACTED
	repo := &snapshotUpdateAccountRepo{stubOpenAIAccountRepo: stubOpenAIAccountRepo{accounts: []Account{accountREDACTEDREDACTEDREDACTED
	upstream := &httpUpstreamRecorder{resp: &http.Response{
		StatusCode: http.StatusOK,
		Header:     http.Header{"Content-Type": []string{"application/json"REDACTEDREDACTED,
		Body:       io.NopCloser(strings.NewReader(`{"id":"compact-agent","status":"completed"REDACTED`)),
REDACTEDREDACTED
	svc := &AccountTestService{accountRepo: repo, httpUpstream: upstreamREDACTED

	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPost, "/api/v1/admin/accounts/21/test", bytes.NewReader(nil))

	require.NoError(t, svc.TestAccountConnection(c, account.ID, "gpt-5.4", "", AccountTestModeCompact))
	require.Equal(t, "AgentAssertion", strings.SplitN(upstream.lastReq.Header.Get("Authorization"), " ", 2)[0])
	require.Equal(t, "account-agent-test", upstream.lastReq.Header.Get("chatgpt-account-id"))
	require.Equal(t, "true", upstream.lastReq.Header.Get("x-openai-fedramp"))
	require.NotContains(t, upstream.lastReq.Header.Get("Authorization"), privateKey)
REDACTED

func TestAccountTestServiceOpenAICompactAgentIdentityRecoversInvalidTaskOnce(t *testing.T) {
	gin.SetMode(gin.TestMode)
	key, privateKey := newTestAgentIdentityKey(t)
	account := &Account{
		ID:          22,
		Name:        "agent-identity-recovery",
		Platform:    PlatformOpenAI,
		Type:        AccountTypeOAuth,
		Status:      StatusActive,
		Schedulable: true,
		Concurrency: 1,
REDACTED
			"auth_mode":          OpenAIAuthModeAgentIdentity,
			"agent_runtime_id":   key.runtimeID,
			"agent_private_key":  privateKey,
			"task_id":            "task-compact-old",
			"chatgpt_account_id": "account-agent-compact-recovery",
	REDACTED,
REDACTED
	repo := &accountTestAgentIdentityRepo{account: accountREDACTED
	registerCalls := 0
	registerServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		registerCalls++
		_, _ = io.WriteString(w, `{"task_id":"task-compact-new"REDACTED`)
REDACTED))
	defer registerServer.Close()
	oldBase := openAIAgentIdentityAuthAPIBaseURL
	openAIAgentIdentityAuthAPIBaseURL = registerServer.URL
	t.Cleanup(func() { openAIAgentIdentityAuthAPIBaseURL = oldBase REDACTED)

	upstream := &httpUpstreamRecorder{responses: []*http.Response{
		{StatusCode: http.StatusUnauthorized, Header: http.Header{"Content-Type": []string{"application/json"REDACTEDREDACTED, Body: io.NopCloser(strings.NewReader(`{"error":{"code":"invalid_task_id"REDACTEDREDACTED`))REDACTED,
		{StatusCode: http.StatusOK, Header: http.Header{"Content-Type": []string{"application/json"REDACTEDREDACTED, Body: io.NopCloser(strings.NewReader(`{"id":"compact-agent","status":"completed"REDACTED`))REDACTED,
REDACTEDREDACTED
	svc := &AccountTestService{accountRepo: repo, httpUpstream: upstreamREDACTED
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPost, "/api/v1/admin/accounts/22/test", bytes.NewReader(nil))

	require.NoError(t, svc.TestAccountConnection(c, account.ID, "gpt-5.4", "", AccountTestModeCompact))
	require.Equal(t, 1, registerCalls)
	require.Len(t, upstream.requests, 2)
	require.Equal(t, "task-compact-new", account.GetCredential("task_id"))
	require.Equal(t, 0, repo.setErrorCalls)
REDACTED

func TestOpenAIAgentIdentityPassthroughKeepsSessionAndPromptCacheHeaders(t *testing.T) {
	gin.SetMode(gin.TestMode)
	key, privateKey := newTestAgentIdentityKey(t)
	account := &Account{
		ID:       24,
		Platform: PlatformOpenAI,
		Type:     AccountTypeOAuth,
REDACTED
			"auth_mode":          OpenAIAuthModeAgentIdentity,
			"agent_runtime_id":   key.runtimeID,
			"agent_private_key":  privateKey,
			"task_id":            key.taskID,
			"chatgpt_account_id": "account-agent-passthrough",
	REDACTED,
REDACTED
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	body := []byte(`{"model":"gpt-5.4","instructions":"Reply OK","input":[],"stream":true,"prompt_cache_key":"cache-agent"REDACTED`)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", bytes.NewReader(body))
	c.Request.Header.Set("session_id", "client-session")
	c.Request.Header.Set("conversation_id", "client-conversation")
	c.Request.Header.Set("Authorization", "Bearer inbound-must-not-forward")

	svc := &OpenAIGatewayService{REDACTED
	req, err := svc.buildUpstreamRequestOpenAIPassthrough(context.Background(), c, account, body, "")
REDACTED
	require.Equal(t, "AgentAssertion", strings.SplitN(req.Header.Get("Authorization"), " ", 2)[0])
	require.Equal(t, "account-agent-passthrough", req.Header.Get("chatgpt-account-id"))
	require.NotEqual(t, "client-session", req.Header.Get("session_id"))
	require.NotEqual(t, "client-conversation", req.Header.Get("conversation_id"))
	require.Equal(t, isolateOpenAISessionID(0, "cache-agent"), req.Header.Get("session_id"))
REDACTED

func TestOpenAIAgentIdentityErrorRedactionDoesNotLeakCredentialValues(t *testing.T) {
	key, privateKey := newTestAgentIdentityKey(t)
	account := &Account{
		ID:       25,
		Platform: PlatformOpenAI,
		Type:     AccountTypeOAuth,
REDACTED
			"auth_mode":         OpenAIAuthModeAgentIdentity,
			"agent_runtime_id":  key.runtimeID,
			"agent_private_key": privateKey,
			"task_id":           key.taskID,
			"access_token":      key.runtimeID + "-oauth-value",
	REDACTED,
REDACTED
	svc := &OpenAIGatewayService{REDACTED
	oauthValue := account.GetCredential("access_token")
	redacted := svc.redactAgentIdentitySensitiveBody(context.Background(), account, []byte(`{"message":"runtime-test task-test `+oauthValue+` AgentAssertion abc123"REDACTED`))
	require.NotContains(t, string(redacted), key.runtimeID)
	require.NotContains(t, string(redacted), key.taskID)
	require.NotContains(t, string(redacted), oauthValue)
	require.NotContains(t, string(redacted), "AgentAssertion abc123")
	require.Contains(t, string(redacted), "[redacted]")
REDACTED

func TestOpenAIAuthenticationHeadersPreserveOAuthPATAndAPIKeyBearerModes(t *testing.T) {
	svc := &OpenAIGatewayService{REDACTED
	tests := []struct {
		name    string
		account *Account
		token   string
REDACTED{
		{name: "oauth", account: &Account{Platform: PlatformOpenAI, Type: AccountTypeOAuthREDACTED, token: "oauth-runtime-token"REDACTED,
		{name: "personal access token", account: &Account{Platform: PlatformOpenAI, Type: AccountTypeOAuth, Credentials: map[string]any{"auth_mode": OpenAIAuthModePersonalAccessTokenREDACTEDREDACTED, token: "pat-runtime-token"REDACTED,
		{name: "api key", account: &Account{Platform: PlatformOpenAI, Type: AccountTypeAPIKeyREDACTED, token: "api-key-runtime-token"REDACTED,
REDACTED
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			headers, err := svc.buildOpenAIAuthenticationHeaders(context.Background(), tt.account, tt.token)
		REDACTED
			require.Equal(t, "Bearer "+tt.token, headers.Get("Authorization"))
	REDACTED)
REDACTED
REDACTED

func TestOpenAIWSAgentIdentityRecoveryRequiresTaskInvalidBody(t *testing.T) {
	require.False(t, isAgentIdentityTaskInvalidWSDialError(&openAIWSDialError{
		StatusCode:   http.StatusUnauthorized,
		ResponseBody: []byte(`{"error":{"code":"invalid_signature"REDACTEDREDACTED`),
REDACTED))
	require.True(t, isAgentIdentityTaskInvalidWSDialError(&openAIWSDialError{
		StatusCode:   http.StatusUnauthorized,
		ResponseBody: []byte(`{"error":{"code":"invalid_task_id"REDACTEDREDACTED`),
REDACTED))
REDACTED

func TestValidateOpenAIWSBearerTokenAllowsAgentIdentityWithoutStoredToken(t *testing.T) {
	t.Run("Given Agent Identity When a WS path receives no bearer token Then dial-time assertion auth is allowed", func(t *testing.T) {
		account := &Account{
			Platform: PlatformOpenAI,
			Type:     AccountTypeOAuth,
	REDACTED
				"auth_mode": OpenAIAuthModeAgentIdentity,
		REDACTED,
	REDACTED

		require.NoError(t, validateOpenAIWSBearerToken(account, ""))
REDACTED)

	t.Run("Given bearer credentials When a WS path receives no token Then the request is rejected", func(t *testing.T) {
		accounts := []*Account{
			{Platform: PlatformOpenAI, Type: AccountTypeOAuthREDACTED,
			{Platform: PlatformOpenAI, Type: AccountTypeOAuth, Credentials: map[string]any{"auth_mode": OpenAIAuthModePersonalAccessTokenREDACTEDREDACTED,
			{Platform: PlatformOpenAI, Type: AccountTypeAPIKeyREDACTED,
	REDACTED

		for _, account := range accounts {
			require.EqualError(t, validateOpenAIWSBearerToken(account, ""), "token is empty")
	REDACTED
REDACTED)
REDACTED

func TestOpenAIWSConnPoolHeadersFactoryRunsAtDialAndStalePrewarmIsDiscarded(t *testing.T) {
	cfg := &config.Config{REDACTED
	cfg.Gateway.OpenAIWS.MaxConnsPerAccount = 1
	pool := newOpenAIWSConnPool(cfg)
	defer pool.Close()
	pool.setClientDialerForTest(&openAIWSFakeDialer{REDACTED)

	accountID := int64(22)
	ap := pool.getOrCreateAccountPool(accountID)
	factoryCalls := 0
	latestHeader := ""
	req := openAIWSAcquireRequest{
		Account: &Account{ID: accountID, Platform: PlatformOpenAI, Type: AccountTypeOAuthREDACTED,
		WSURL:   "wss://example.com/v1/responses",
		HeadersFactory: func(_ context.Context, headers http.Header) (http.Header, error) {
			factoryCalls++
			latestHeader = "AgentAssertion dial-" + string(rune('0'+factoryCalls))
			if headers == nil {
				headers = make(http.Header)
		REDACTED
			headers.Set("Authorization", latestHeader)
			return headers, nil
	REDACTED,
REDACTED
	ap.mu.Lock()
	ap.lastAcquire = &req
	generation := ap.generation
	ap.mu.Unlock()

	pool.prewarmConns(accountID, req, 1, generation)
	require.Equal(t, 1, factoryCalls, "prewarm must generate authorization inside the actual dial")
	require.Equal(t, "AgentAssertion dial-1", latestHeader)

	pool.ClearAccount(accountID)
	ap.mu.Lock()
	require.Empty(t, ap.conns, "credential recovery must remove pooled connections")
	require.Nil(t, ap.lastAcquire, "credential recovery must discard delayed acquire state")
	require.Equal(t, generation+1, ap.generation)
	ap.mu.Unlock()

	// A prewarm captured before ClearAccount must not be admitted after recovery.
	pool.prewarmConns(accountID, req, 1, generation)
	ap.mu.Lock()
	require.Empty(t, ap.conns)
	ap.mu.Unlock()
REDACTED

func TestOpenAIAgentIdentityTaskInvalidRetriesExactlyOnce(t *testing.T) {
	gin.SetMode(gin.TestMode)
	key, privateKey := newTestAgentIdentityKey(t)
	account := &Account{
		ID:          23,
		Name:        "agent-identity",
		Platform:    PlatformOpenAI,
		Type:        AccountTypeOAuth,
		Status:      StatusActive,
		Schedulable: true,
		Concurrency: 1,
REDACTED
			"auth_mode":          OpenAIAuthModeAgentIdentity,
			"agent_runtime_id":   key.runtimeID,
			"agent_private_key":  privateKey,
			"task_id":            "task-old",
			"chatgpt_account_id": "account-agent-retry",
	REDACTED,
REDACTED
	repo := &agentIdentityForwardRepo{account: accountREDACTED
	registerCalls := 0
	registerServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		registerCalls++
		_, _ = io.WriteString(w, `{"task_id":"task-new"REDACTED`)
REDACTED))
	defer registerServer.Close()
	oldBase := openAIAgentIdentityAuthAPIBaseURL
	openAIAgentIdentityAuthAPIBaseURL = registerServer.URL
	t.Cleanup(func() { openAIAgentIdentityAuthAPIBaseURL = oldBase REDACTED)

	successBody := `{"id":"resp-agent-retry","object":"response","model":"gpt-5.4","output":[],"usage":{"input_tokens":1,"output_tokens":1,"total_tokens":2REDACTEDREDACTED`
	upstream := &httpUpstreamRecorder{responses: []*http.Response{
		{StatusCode: http.StatusUnauthorized, Header: http.Header{"Content-Type": []string{"application/json"REDACTEDREDACTED, Body: io.NopCloser(strings.NewReader(`{"error":{"code":"invalid_task_id"REDACTEDREDACTED`))REDACTED,
		{StatusCode: http.StatusOK, Header: http.Header{"Content-Type": []string{"application/json"REDACTEDREDACTED, Body: io.NopCloser(strings.NewReader(successBody))REDACTED,
REDACTEDREDACTED
	require.True(t, isAgentIdentityTaskInvalidHTTPResponse(http.StatusUnauthorized, []byte(`{"error":{"code":"invalid_task_id"REDACTEDREDACTED`)))
	svc := &OpenAIGatewayService{cfg: &config.Config{REDACTED, accountRepo: repo, httpUpstream: upstreamREDACTED
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", strings.NewReader(`{"model":"gpt-5.4","instructions":"Reply OK","input":[],"stream":falseREDACTED`))

	_, err := svc.Forward(context.Background(), c, account, []byte(`{"model":"gpt-5.4","instructions":"Reply OK","input":[],"stream":falseREDACTED`))
REDACTED
	require.Equal(t, 1, registerCalls)
	require.Len(t, upstream.requests, 2)
	require.NotEqual(t, upstream.requests[0].Header.Get("Authorization"), upstream.requests[1].Header.Get("Authorization"))
	require.Equal(t, "task-new", decodeAgentAssertionTask(t, upstream.requests[1].Header.Get("Authorization")))

	// Two consecutive invalid responses still produce only one retry for this
	// request; the recovery path must not loop indefinitely.
	upstream.responses = []*http.Response{
		{StatusCode: http.StatusUnauthorized, Header: http.Header{"Content-Type": []string{"application/json"REDACTEDREDACTED, Body: io.NopCloser(strings.NewReader(`{"error":{"code":"invalid_task_id"REDACTEDREDACTED`))REDACTED,
		{StatusCode: http.StatusUnauthorized, Header: http.Header{"Content-Type": []string{"application/json"REDACTEDREDACTED, Body: io.NopCloser(strings.NewReader(`{"error":{"code":"invalid_task_id"REDACTEDREDACTED`))REDACTED,
REDACTED
	rec2 := httptest.NewRecorder()
	c2, _ := gin.CreateTestContext(rec2)
	c2.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", strings.NewReader(`{"model":"gpt-5.4","instructions":"Reply OK","input":[],"stream":falseREDACTED`))
	_, err = svc.Forward(context.Background(), c2, account, []byte(`{"model":"gpt-5.4","instructions":"Reply OK","input":[],"stream":falseREDACTED`))
REDACTED
	require.Equal(t, 2, registerCalls)
	require.Len(t, upstream.requests, 4)

	// Passthrough uses the same one-shot task recovery contract.
	account.Extra = map[string]any{"openai_passthrough": trueREDACTED
	account.Credentials["task_id"] = "task-old-passthrough"
	upstream.responses = []*http.Response{
		{StatusCode: http.StatusUnauthorized, Header: http.Header{"Content-Type": []string{"application/json"REDACTEDREDACTED, Body: io.NopCloser(strings.NewReader(`{"error":{"code":"invalid_task_id"REDACTEDREDACTED`))REDACTED,
		{StatusCode: http.StatusOK, Header: http.Header{"Content-Type": []string{"text/event-stream"REDACTEDREDACTED, Body: io.NopCloser(strings.NewReader("data: {\"type\":\"response.completed\",\"response\":{\"usage\":{\"input_tokens\":1,\"output_tokens\":1,\"total_tokens\":2REDACTEDREDACTEDREDACTED\n\ndata: [DONE]\n\n"))REDACTED,
REDACTED
	rec3 := httptest.NewRecorder()
	c3, _ := gin.CreateTestContext(rec3)
	c3.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", strings.NewReader(`{"model":"gpt-5.4","instructions":"Reply OK","input":[],"stream":falseREDACTED`))
	_, err = svc.Forward(context.Background(), c3, account, []byte(`{"model":"gpt-5.4","instructions":"Reply OK","input":[],"stream":falseREDACTED`))
REDACTED
	require.Equal(t, 3, registerCalls)
	require.Len(t, upstream.requests, 6)
REDACTED

func TestOpenAIAgentIdentityCompatRoutesRecoverInvalidTaskOnce(t *testing.T) {
	gin.SetMode(gin.TestMode)
	tests := []struct {
		name string
		path string
		body []byte
		call func(*OpenAIGatewayService, context.Context, *gin.Context, *Account, []byte) (*OpenAIForwardResult, error)
REDACTED{
		{
			name: "chat completions",
			path: "/v1/chat/completions",
			body: []byte(`{"model":"gpt-5.4","stream":false,"messages":[{"role":"user","content":"hi"REDACTED]REDACTED`),
			call: func(s *OpenAIGatewayService, ctx context.Context, c *gin.Context, account *Account, body []byte) (*OpenAIForwardResult, error) {
				return s.ForwardAsChatCompletions(ctx, c, account, body, "", "gpt-5.4")
		REDACTED,
	REDACTED,
		{
			name: "anthropic messages",
			path: "/v1/messages",
			body: []byte(`{"model":"gpt-5.4","stream":false,"max_tokens":32,"messages":[{"role":"user","content":"hi"REDACTED]REDACTED`),
			call: func(s *OpenAIGatewayService, ctx context.Context, c *gin.Context, account *Account, body []byte) (*OpenAIForwardResult, error) {
				return s.ForwardAsAnthropic(ctx, c, account, body, "", "gpt-5.4")
		REDACTED,
	REDACTED,
REDACTED

	for index, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			key, privateKey := newTestAgentIdentityKey(t)
			account := &Account{
				ID:          int64(40 + index),
				Name:        "agent-identity-compat",
				Platform:    PlatformOpenAI,
				Type:        AccountTypeOAuth,
				Status:      StatusActive,
				Schedulable: true,
				Concurrency: 1,
		REDACTED
					"auth_mode":          OpenAIAuthModeAgentIdentity,
					"agent_runtime_id":   key.runtimeID,
					"agent_private_key":  privateKey,
					"task_id":            "task-compat-old",
					"chatgpt_account_id": "account-compat-recovery",
			REDACTED,
		REDACTED
			repo := &agentIdentityForwardRepo{account: accountREDACTED
			registerCalls := 0
			registerServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				registerCalls++
				_, _ = io.WriteString(w, `{"task_id":"task-compat-new"REDACTED`)
		REDACTED))
			defer registerServer.Close()
			oldBase := openAIAgentIdentityAuthAPIBaseURL
			openAIAgentIdentityAuthAPIBaseURL = registerServer.URL
			t.Cleanup(func() { openAIAgentIdentityAuthAPIBaseURL = oldBase REDACTED)

			upstream := &httpUpstreamRecorder{responses: []*http.Response{
				{StatusCode: http.StatusUnauthorized, Header: http.Header{"Content-Type": []string{"application/json"REDACTEDREDACTED, Body: io.NopCloser(strings.NewReader(`{"error":{"code":"invalid_task_id"REDACTEDREDACTED`))REDACTED,
				{StatusCode: http.StatusUnauthorized, Header: http.Header{"Content-Type": []string{"application/json"REDACTEDREDACTED, Body: io.NopCloser(strings.NewReader(`{"error":{"code":"invalid_task_id"REDACTEDREDACTED`))REDACTED,
		REDACTEDREDACTED
			svc := &OpenAIGatewayService{cfg: &config.Config{REDACTED, accountRepo: repo, httpUpstream: upstreamREDACTED
			rec := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(rec)
			c.Request = httptest.NewRequest(http.MethodPost, tt.path, bytes.NewReader(tt.body))

			_, err := tt.call(svc, context.Background(), c, account, tt.body)
		REDACTED
			require.Equal(t, 1, registerCalls)
			require.Len(t, upstream.requests, 2)
			require.Equal(t, "task-compat-new", account.GetCredential("task_id"))
	REDACTED)
REDACTED
REDACTED

func decodeAgentAssertionTask(t *testing.T, header string) string {
REDACTED
	encoded := strings.TrimPrefix(header, "AgentAssertion ")
	decoded, err := base64.RawURLEncoding.DecodeString(encoded)
REDACTED
	var envelope struct {
		TaskID string `json:"task_id"`
REDACTED
	require.NoError(t, json.Unmarshal(decoded, &envelope))
	return envelope.TaskID
REDACTED

type agentIdentityForwardRepo struct {
	AccountRepository
	account *Account
REDACTED

type accountTestAgentIdentityRepo struct {
	AccountRepository
	account       *Account
	setErrorCalls int
REDACTED

func (r *accountTestAgentIdentityRepo) GetByID(_ context.Context, _ int64) (*Account, error) {
	return r.account, nil
REDACTED

func (r *accountTestAgentIdentityRepo) UpdateCredentials(_ context.Context, _ int64, credentials map[string]any) error {
	r.account.Credentials = credentials
	return nil
REDACTED

func (r *accountTestAgentIdentityRepo) UpdateExtra(_ context.Context, _ int64, _ map[string]any) error {
	return nil
REDACTED

func (r *accountTestAgentIdentityRepo) SetError(_ context.Context, _ int64, _ string) error {
	r.setErrorCalls++
	return nil
REDACTED

func (r *agentIdentityForwardRepo) GetByID(_ context.Context, _ int64) (*Account, error) {
	return r.account, nil
REDACTED

func (r *agentIdentityForwardRepo) UpdateCredentials(_ context.Context, _ int64, credentials map[string]any) error {
	r.account.Credentials = credentials
	return nil
REDACTED
