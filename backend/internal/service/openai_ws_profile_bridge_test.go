package service

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	coderws "github.com/coder/websocket"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

func TestCodexProfileWSForcesHTTPBridgeAndRestoresStructuredIdentity(t *testing.T) {
	gin.SetMode(gin.TestMode)

	cfg := &config.Config{}
	cfg.Security.URLAllowlist.Enabled = false
	cfg.Security.URLAllowlist.AllowInsecureHTTP = true
	cfg.Gateway.OpenAIWS.Enabled = true
	cfg.Gateway.OpenAIWS.OAuthEnabled = true
	cfg.Gateway.OpenAIWS.ResponsesWebsocketsV2 = true
	cfg.Gateway.OpenAIWS.ModeRouterV2Enabled = false
	cfg.Gateway.OpenAIWS.IngressModeDefault = OpenAIWSIngressModeCtxPool
	cfg.Gateway.OpenAIWS.ReadTimeoutSeconds = 3
	cfg.Gateway.OpenAIWS.WriteTimeoutSeconds = 3

	account := codexProfileTestAccount(t, 391, CodexOSWindows, CodexSurfaceDesktop, CodexArchX8664, false)
	repo := &codexProfileGatewayAccountRepo{
		accounts: map[int64]*Account{account.ID: account},
		resolvedSlots: map[int64]*CodexResolvedDeviceSlot{
			account.ID: {
				AccountID: account.ID, SlotID: 39101, ProfileID: 39100,
				OSClass: CodexOSWindows, CanonicalSurface: CodexSurfaceDesktop,
				Architecture: CodexArchX8664, CatalogVersion: 1,
				SlotIndex: 0, Epoch: 4, State: "active", PolicyVersion: 1,
			},
		},
	}

	upstream := &codexProfileEchoUpstream{stream: true}
	captureDialer := &openAIWSCaptureDialer{conn: &openAIWSCaptureConn{}}
	pool := newOpenAIWSConnPool(cfg)
	pool.setClientDialerForTest(captureDialer)
	svc := &OpenAIGatewayService{
		accountRepo:      repo,
		cache:            &codexProfileGatewayCache{values: map[string]int64{}},
		cfg:              cfg,
		httpUpstream:     upstream,
		openaiWSResolver: NewOpenAIWSProtocolResolver(cfg),
		toolCorrector:    NewCodexToolCorrector(),
		openaiWSPool:     pool,
	}
	require.True(t, svc.isOpenAIAccountTransportCompatible(account, OpenAIUpstreamTransportResponsesWebsocketV2Ingress))

	serverErr := make(chan error, 1)
	wsServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		client, err := coderws.Accept(w, r, nil)
		if err != nil {
			serverErr <- err
			return
		}
		defer func() { _ = client.CloseNow() }()

		readCtx, cancel := context.WithTimeout(r.Context(), 3*time.Second)
		_, firstMessage, err := client.Read(readCtx)
		cancel()
		if err != nil {
			serverErr <- err
			return
		}

		ginCtx := newCodexProfileGatewayContext(t, 7, 101, firstMessage)
		_ = svc.GenerateSessionHashWithFallback(ginCtx, firstMessage, "profile-bridge")
		prepared, err := svc.PrepareCodexProfileAttempt(ginCtx.Request.Context(), ginCtx, account, firstMessage)
		if err != nil {
			serverErr <- err
			return
		}
		err = svc.ProxyResponsesWebSocketFromClient(
			ginCtx.Request.Context(), ginCtx, client, prepared, "test-token", firstMessage, nil,
		)
		svc.ReleaseCodexProfileAttempt(ginCtx, prepared)
		serverErr <- err
	}))
	defer wsServer.Close()

	dialCtx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	client, _, err := coderws.Dial(dialCtx, "ws"+strings.TrimPrefix(wsServer.URL, "http"), nil)
	cancel()
	require.NoError(t, err)
	defer func() { _ = client.CloseNow() }()

	firstMessage := []byte(`{"type":"response.create","model":"gpt-5.6-sol","client_metadata":{"session_id":"client-session","os":"windows","arch":"arm64","surface":"desktop"}}`)
	writeCtx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	require.NoError(t, client.Write(writeCtx, coderws.MessageText, firstMessage))
	cancel()

	readCtx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	_, created, err := client.Read(readCtx)
	cancel()
	require.NoError(t, err)
	require.Equal(t, "response.created", gjson.GetBytes(created, "type").String())
	require.Equal(t, "client-session", gjson.GetBytes(created, "response.client_metadata.session_id").String())

	readCtx, cancel = context.WithTimeout(context.Background(), 3*time.Second)
	_, delta, err := client.Read(readCtx)
	cancel()
	require.NoError(t, err)

	upstreamSession := gjson.GetBytes(upstream.requestBody, "client_metadata.session_id").String()
	require.NotEmpty(t, upstreamSession)
	require.NotEqual(t, "client-session", upstreamSession)
	require.Equal(t, upstreamSession, gjson.GetBytes(delta, "delta").String(), "ordinary output text must not be identity-restored")
	require.Zero(t, captureDialer.DialCount(), "Profile mode must not dial an upstream native WebSocket")

	_ = client.Close(coderws.StatusNormalClosure, "done")
	select {
	case proxyErr := <-serverErr:
		var closeErr *OpenAIWSClientCloseError
		if proxyErr != nil && (!errors.As(proxyErr, &closeErr) || closeErr.StatusCode() != coderws.StatusNormalClosure) {
			require.NoError(t, proxyErr)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("timed out waiting for Profile HTTP Bridge session to close")
	}
}

func TestCodexProfileWSRejectsContinuationOwnedByAnotherAPIKey(t *testing.T) {
	svc := &OpenAIGatewayService{cfg: &config.Config{}, openaiWSStateStore: NewOpenAIWSStateStore(nil)}
	groupID := int64(3)
	require.NoError(t, svc.BindOpenAIHTTPResponseOwner(context.Background(), groupID, "resp_key_a", 7, 101))

	owned, err := svc.ValidateOpenAIHTTPResponseOwnerForAPIKey(context.Background(), groupID, "resp_key_a", 7, 202)
	require.NoError(t, err)
	require.False(t, owned)
	owned, err = svc.ValidateOpenAIHTTPResponseOwnerForAPIKey(context.Background(), groupID, "resp_key_a", 7, 101)
	require.NoError(t, err)
	require.True(t, owned)
}

func TestOpenAIWSHTTPBridgeForceKeepsModeOffNativeWithPluginRoute(t *testing.T) {
	manager := &PluginManager{}
	manager.route.Store(&pluginRoute{pluginID: 1, rolloutPercent: 100})
	svc := &OpenAIGatewayService{pluginManager: manager}

	off := &Account{
		ID: 1, Platform: PlatformOpenAI, Type: AccountTypeOAuth,
		CodexIdentityPolicy: CodexIdentityPolicySpec{Mode: CodexIdentityPolicyOff},
	}
	require.True(t, manager.ShouldRouteOpenAIOAuth(off), "HTTP requests should still use the selected Transport plugin")
	require.False(t, svc.shouldForceOpenAIWSHTTPBridge(off), "mode=off must retain the official native WebSocket path")

	profile := *off
	profile.CodexIdentityPolicy.Mode = CodexIdentityPolicyOSProfileDevicePool
	require.True(t, svc.shouldForceOpenAIWSHTTPBridge(&profile), "Profile mode must use the host HTTP Bridge")
}
