package service

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/coder/websocket"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

func TestBuildOpenAIWSCurrentTurnRetryPayloadRejectsOrphanToolOutput(t *testing.T) {
	payload := []byte(`{"type":"response.create","model":"mapped-model","previous_response_id":"resp_old"}`)
	fullInput := []json.RawMessage{
		json.RawMessage(`{"type":"function_call_output","call_id":"missing_call","output":"done"}`),
	}

	retryPayload, retrySafe, err := buildOpenAIWSCurrentTurnRetryPayload(payload, fullInput, true, "gpt-5.6-sol")

	require.NoError(t, err)
	require.False(t, retrySafe)
	require.Nil(t, retryPayload)
}

func TestProxyOpenAIWSHTTPBridgeTurnLaterTurn429FailsOverBeforeClientWrite(t *testing.T) {
	gin.SetMode(gin.TestMode)

	upstream := &httpUpstreamRecorder{resp: &http.Response{
		StatusCode: http.StatusTooManyRequests,
		Header:     http.Header{"Retry-After": []string{"60"}},
		Body:       io.NopCloser(strings.NewReader(`{"error":{"type":"usage_limit_reached","message":"The usage limit has been reached"}}`)),
	}}
	svc := &OpenAIGatewayService{cfg: &config.Config{}, httpUpstream: upstream}
	account := &Account{ID: 129, Platform: PlatformOpenAI, Type: AccountTypeOAuth, Concurrency: 1}
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodGet, "/v1/responses", nil)
	payload := []byte(`{"type":"response.create","model":"gpt-5.6-sol","previous_response_id":"resp_old","input":[{"role":"user","content":"continue"}]}`)
	writes := 0

	result, err := svc.proxyOpenAIWSHTTPBridgeTurn(
		context.Background(), c, account, "access-token", payload, len(payload),
		"gpt-5.6-sol", "", "", "", "", 281,
		func([]byte) error {
			writes++
			return nil
		},
	)

	require.Nil(t, result)
	var failoverErr *UpstreamFailoverError
	require.ErrorAs(t, err, &failoverErr)
	require.Equal(t, http.StatusTooManyRequests, failoverErr.StatusCode)
	require.Zero(t, writes)
}

func TestProxyOpenAIWSHTTPBridgeTurnLaterTurnDoesNotFailOverAfterDownstreamOutput(t *testing.T) {
	gin.SetMode(gin.TestMode)

	upstream := &httpUpstreamRecorder{resp: &http.Response{
		StatusCode: http.StatusOK,
		Header:     make(http.Header),
		Body: io.NopCloser(strings.NewReader(
			"data: {\"type\":\"response.output_text.delta\",\"delta\":\"partial\"}\n\n" +
				"data: {\"type\":\"error\",\"error\":{\"type\":\"rate_limit_error\",\"message\":\"limited\"}}\n\n",
		)),
	}}
	svc := &OpenAIGatewayService{cfg: &config.Config{}, httpUpstream: upstream}
	account := &Account{ID: 10, Platform: PlatformOpenAI, Type: AccountTypeAPIKey, Concurrency: 1}
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodGet, "/v1/responses", nil)
	payload := []byte(`{"type":"response.create","model":"gpt-5","input":"hi"}`)
	var writes [][]byte

	result, err := svc.proxyOpenAIWSHTTPBridgeTurn(
		context.Background(), c, account, "sk-test", payload, len(payload),
		"gpt-5", "", "", "", "", 281,
		func(message []byte) error {
			writes = append(writes, append([]byte(nil), message...))
			return nil
		},
	)

	require.NotNil(t, result)
	require.Error(t, err)
	var failoverErr *UpstreamFailoverError
	require.False(t, errors.As(err, &failoverErr))
	require.Len(t, writes, 2)
	require.Equal(t, "response.output_text.delta", gjson.GetBytes(writes[0], "type").String())
	// P0.1: after downstream output, retain the upstream error and synthesize
	// the official Responses terminal so Codex clients do not see a bare error
	// followed by a closed stream.
	require.Equal(t, "response.failed", gjson.GetBytes(writes[1], "type").String())
}

func TestOpenAIWSHTTPBridgeLaterTurn429RetriesCurrentTurnOnReplacementAccount(t *testing.T) {
	gin.SetMode(gin.TestMode)

	cfg := &config.Config{}
	cfg.Security.URLAllowlist.Enabled = false
	cfg.Security.URLAllowlist.AllowInsecureHTTP = true
	cfg.Gateway.OpenAIWS.Enabled = true
	cfg.Gateway.OpenAIWS.OAuthEnabled = true
	cfg.Gateway.OpenAIWS.ResponsesWebsocketsV2 = true
	cfg.Gateway.OpenAIWS.ModeRouterV2Enabled = true
	cfg.Gateway.OpenAIWS.ReadTimeoutSeconds = 3
	cfg.Gateway.OpenAIWS.WriteTimeoutSeconds = 3

	upstream := &httpUpstreamRecorder{responses: []*http.Response{
		{
			StatusCode: http.StatusOK,
			Header: http.Header{
				"Content-Type":          []string{"text/event-stream"},
				openAIWSTurnStateHeader: []string{"old-account-state"},
			},
			Body: io.NopCloser(strings.NewReader(
				"data: {\"type\":\"response.completed\",\"response\":{\"id\":\"resp_first\",\"output\":[{\"id\":\"msg_1\",\"type\":\"message\",\"role\":\"assistant\",\"content\":[{\"type\":\"output_text\",\"text\":\"first-ok\"}]},{\"id\":\"fc_1\",\"type\":\"function_call\",\"call_id\":\"call_1\",\"name\":\"inspect\",\"arguments\":\"{}\"}],\"usage\":{\"input_tokens\":1,\"output_tokens\":1}}}\n\n",
			)),
		},
		{
			StatusCode: http.StatusTooManyRequests,
			Header:     http.Header{"Retry-After": []string{"60"}},
			Body:       io.NopCloser(strings.NewReader(`{"error":{"type":"usage_limit_reached","message":"The usage limit has been reached"}}`)),
		},
		{
			StatusCode: http.StatusOK,
			Header:     http.Header{"Content-Type": []string{"text/event-stream"}},
			Body: io.NopCloser(strings.NewReader(
				"data: {\"type\":\"response.completed\",\"response\":{\"id\":\"resp_second\",\"output\":[{\"id\":\"msg_2\",\"type\":\"message\",\"role\":\"assistant\",\"content\":[{\"type\":\"output_text\",\"text\":\"second-ok\"}]}],\"usage\":{\"input_tokens\":4,\"output_tokens\":1}}}\n\n",
			)),
		},
	}}
	profileCache := &codexProfileGatewayCache{values: map[string]int64{}}
	svc := &OpenAIGatewayService{
		cfg:              cfg,
		httpUpstream:     upstream,
		cache:            profileCache,
		openaiWSResolver: NewOpenAIWSProtocolResolver(cfg),
		toolCorrector:    NewCodexToolCorrector(),
	}
	groupID := int64(4202)
	account := codexProfileTestAccount(t, 129, CodexOSWindows, CodexSurfaceCLI, CodexArchX8664, false)
	account.Name = "limited"
	account.Concurrency = 1
	account.CodexIdentityPolicy.SessionPolicy = CodexSessionPolicySpec{
		Mode: CodexSessionDeviceShared, MaxActiveConversationsPerSlot: 1, DisableCrossKeyContinuation: true,
	}
	account.Credentials["chatgpt_account_id"] = "account-a"
	account.Credentials["chatgpt_user_id"] = "user-a"
	account.Extra["openai_oauth_responses_websockets_v2_mode"] = OpenAIWSIngressModeHTTPBridge
	nextAccount := codexProfileTestAccount(t, 130, CodexOSWindows, CodexSurfaceCLI, CodexArchX8664, false)
	nextAccount.Name = "replacement"
	nextAccount.Concurrency = 1
	nextAccount.CodexIdentityPolicy.SessionPolicy = CodexSessionPolicySpec{
		Mode: CodexSessionDeviceShared, MaxActiveConversationsPerSlot: 1, DisableCrossKeyContinuation: true,
	}
	nextAccount.Credentials["chatgpt_account_id"] = "account-b"
	nextAccount.Credentials["chatgpt_user_id"] = "user-b"
	nextAccount.Extra[codexFingerprintSeedExtraKey] = "22222222-2222-4222-8222-222222222222"
	nextAccount.Extra["openai_oauth_responses_websockets_v2_mode"] = OpenAIWSIngressModeHTTPBridge
	bindingRepo := &codexProfileGatewayAccountRepo{
		accounts: map[int64]*Account{account.ID: account, nextAccount.ID: nextAccount},
		resolvedSlots: map[int64]*CodexResolvedDeviceSlot{
			account.ID: {
				AccountID: account.ID, SlotID: 12901, ProfileID: 12900,
				OSClass: CodexOSWindows, CanonicalSurface: CodexSurfaceCLI,
				Architecture: CodexArchX8664, CatalogVersion: 1,
				SlotIndex: 0, Epoch: 4, State: "active", PolicyVersion: 1,
			},
			nextAccount.ID: {
				AccountID: nextAccount.ID, SlotID: 13001, ProfileID: 13000,
				OSClass: CodexOSWindows, CanonicalSurface: CodexSurfaceCLI,
				Architecture: CodexArchX8664, CatalogVersion: 1,
				SlotIndex: 0, Epoch: 4, State: "active", PolicyVersion: 1,
			},
		},
	}
	svc.accountRepo = bindingRepo
	deviceLeaseCache := &codexDeviceLeaseCache{}
	svc.concurrencyService = NewConcurrencyService(deviceLeaseCache)

	serverErrCh := make(chan error, 1)
	failoverCh := make(chan []byte, 1)
	sessionHashCh := make(chan string, 1)
	identityCh := make(chan [4]string, 1)
	wsServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := websocket.Accept(w, r, nil)
		if err != nil {
			serverErrCh <- err
			return
		}
		defer func() { _ = conn.CloseNow() }()

		rec := httptest.NewRecorder()
		ginCtx, _ := gin.CreateTestContext(rec)
		requestCtx := WithHTTPUpstreamIsolationScope(r.Context(), 7, 101)
		ginCtx.Request = r.Clone(requestCtx)
		ginCtx.Request.URL.Path = "/v1/responses"
		ginCtx.Request.Header.Set("User-Agent", "codex_cli_rs/0.146.0 (Windows 11; arm64) WindowsTerminal")
		ginCtx.Request.Header.Set("originator", "codex_cli_rs")
		ginCtx.Set("api_key", &APIKey{ID: 101, GroupID: &groupID, User: &User{ID: 7}})
		readCtx, cancel := context.WithTimeout(requestCtx, 3*time.Second)
		_, firstMessage, readErr := conn.Read(readCtx)
		cancel()
		if readErr != nil {
			serverErrCh <- readErr
			return
		}
		profileSessionHash := svc.GenerateSessionHashWithFallback(ginCtx, firstMessage, "profile-429")
		if profileSessionHash == "" {
			serverErrCh <- errors.New("missing profile session hash")
			return
		}
		sessionHashCh <- profileSessionHash
		preparedAccount, prepareErr := svc.PrepareCodexProfileAttempt(requestCtx, ginCtx, account, firstMessage)
		if prepareErr != nil {
			serverErrCh <- prepareErr
			return
		}
		firstPlan := stagedCodexIdentityAttemptPlan(ginCtx, preparedAccount)
		if _, bindErr := svc.setCodexProfileAffinityAccountID(ginCtx.Request.Context(), &groupID, profileSessionHash, preparedAccount.ID); bindErr != nil {
			serverErrCh <- bindErr
			return
		}
		proxyErr := svc.ProxyResponsesWebSocketFromClient(requestCtx, ginCtx, conn, preparedAccount, "access-token-a", firstMessage, nil)
		svc.ReleaseCodexProfileAttempt(ginCtx, preparedAccount)
		var failoverErr *UpstreamFailoverError
		if !errors.As(proxyErr, &failoverErr) {
			serverErrCh <- proxyErr
			return
		}
		retryPayload, retryCurrentTurn := OpenAIWSCurrentTurnRetryPayload(proxyErr)
		if !retryCurrentTurn || len(retryPayload) == 0 {
			serverErrCh <- errors.New("missing current-turn retry payload")
			return
		}
		retryPayload, prepareErr = svc.RestoreCodexProfileRetryPayload(ginCtx, preparedAccount, retryPayload)
		if prepareErr != nil {
			serverErrCh <- prepareErr
			return
		}
		failoverCh <- retryPayload
		preparedNext, prepareErr := svc.PrepareCodexProfileAttempt(requestCtx, ginCtx, nextAccount, retryPayload)
		if prepareErr != nil {
			serverErrCh <- prepareErr
			return
		}
		nextPlan := stagedCodexIdentityAttemptPlan(ginCtx, preparedNext)
		identityCh <- [4]string{
			firstPlan.UpstreamValue(CodexIdentityInstallation),
			firstPlan.UpstreamValue(CodexIdentitySession),
			nextPlan.UpstreamValue(CodexIdentityInstallation),
			nextPlan.UpstreamValue(CodexIdentitySession),
		}
		if _, bindErr := svc.setCodexProfileAffinityAccountID(ginCtx.Request.Context(), &groupID, profileSessionHash, preparedNext.ID); bindErr != nil {
			serverErrCh <- bindErr
			return
		}
		serverErrCh <- svc.ProxyResponsesWebSocketFromClient(
			requestCtx, ginCtx, conn, preparedNext, "access-token-b", retryPayload, nil,
		)
		svc.ReleaseCodexProfileAttempt(ginCtx, preparedNext)
	}))
	defer wsServer.Close()

	dialCtx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	clientConn, _, err := websocket.Dial(dialCtx, "ws"+strings.TrimPrefix(wsServer.URL, "http"), nil)
	cancel()
	require.NoError(t, err)
	defer func() { _ = clientConn.CloseNow() }()

	writeCtx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	err = clientConn.Write(writeCtx, websocket.MessageText, []byte(`{"type":"response.create","model":"gpt-5.6-sol","input":[{"role":"user","content":"first"}]}`))
	cancel()
	require.NoError(t, err)

	readCtx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	_, completed, err := clientConn.Read(readCtx)
	cancel()
	if err != nil {
		select {
		case serverErr := <-serverErrCh:
			t.Fatalf("first client read failed: %v (server: %v)", err, serverErr)
		default:
			require.NoError(t, err)
		}
	}
	require.Equal(t, "response.completed", gjson.GetBytes(completed, "type").String())

	writeCtx, cancel = context.WithTimeout(context.Background(), 3*time.Second)
	err = clientConn.Write(writeCtx, websocket.MessageText, []byte(`{"type":"response.create","model":"gpt-5.6-sol","previous_response_id":"resp_first","prompt_cache_key":"client-session","client_metadata":{"session_id":"client-session","thread_id":"client-thread"},"input":[{"type":"function_call_output","call_id":"call_1","output":"second"}]}`))
	cancel()
	require.NoError(t, err)

	readCtx, cancel = context.WithTimeout(context.Background(), 3*time.Second)
	_, retriedCompleted, err := clientConn.Read(readCtx)
	cancel()
	require.NoError(t, err)
	require.Equal(t, "response.completed", gjson.GetBytes(retriedCompleted, "type").String())
	require.Equal(t, "resp_second", gjson.GetBytes(retriedCompleted, "response.id").String())
	_ = clientConn.Close(websocket.StatusNormalClosure, "done")

	select {
	case retryPayload := <-failoverCh:
		require.NotEmpty(t, retryPayload)
		require.False(t, gjson.GetBytes(retryPayload, "previous_response_id").Exists())
		require.Equal(t, "gpt-5.6-sol", gjson.GetBytes(retryPayload, "model").String())
		input := gjson.GetBytes(retryPayload, "input")
		require.True(t, input.IsArray())
		require.Len(t, input.Array(), 4)
		require.Contains(t, input.Raw, "first")
		require.Contains(t, input.Raw, "first-ok")
		require.Contains(t, input.Raw, "second")
		require.Equal(t, 1, strings.Count(input.Raw, `"id":"fc_1"`))
		require.Equal(t, 2, strings.Count(input.Raw, `"call_id":"call_1"`))
		require.Equal(t, "client-session", gjson.GetBytes(retryPayload, "client_metadata.session_id").String())
		require.Equal(t, "client-thread", gjson.GetBytes(retryPayload, "client_metadata.thread_id").String())
	case <-time.After(5 * time.Second):
		t.Fatal("timed out waiting for current-turn failover")
	}

	select {
	case proxyErr := <-serverErrCh:
		require.NoError(t, proxyErr)
	case <-time.After(5 * time.Second):
		t.Fatal("timed out waiting for replacement-account completion")
	}
	require.Len(t, upstream.bodies, 3)
	require.Contains(t, string(upstream.bodies[0]), "first")
	require.NotContains(t, string(upstream.bodies[2]), "previous_response_id")
	require.Contains(t, string(upstream.bodies[2]), "second")
	identities := <-identityCh
	require.NotEqual(t, identities[0], identities[2])
	require.NotEqual(t, identities[1], identities[3])
	require.Equal(t, identities[1], gjson.GetBytes(upstream.bodies[1], "client_metadata.session_id").String())
	require.Equal(t, identities[2], gjson.GetBytes(upstream.bodies[2], "client_metadata.installation_id").String())
	require.Equal(t, identities[3], gjson.GetBytes(upstream.bodies[2], "client_metadata.session_id").String())
	require.NotContains(t, string(upstream.bodies[2]), identities[0], "retry payload must not treat the old account alias as client identity")
	require.Empty(t, upstream.requests[2].Header.Get(openAIWSTurnStateHeader))

	require.Equal(t, [][2]int64{{129, 130}}, bindingRepo.rebinds, "429 failover must atomically move the API-key/OS binding")
	profileSessionHash := <-sessionHashCh
	affinityCtx := codexProfileTestContext(7, 101, CodexClientProfile{
		OSClass: CodexOSWindows, Surface: CodexSurfaceCLI, Architecture: CodexArchARM64,
	}, profileSessionHash)
	affinityAccountID, handled, err := svc.getCodexProfileAffinityAccountID(affinityCtx, &groupID, profileSessionHash)
	require.NoError(t, err)
	require.True(t, handled)
	require.Equal(t, int64(130), affinityAccountID, "affinity must remain on the replacement account for its TTL")
	deviceLeaseCache.mu.Lock()
	require.Empty(t, deviceLeaseCache.owners, "both device_shared attempt leases must be released across 429 failover")
	deviceLeaseCache.mu.Unlock()
}
