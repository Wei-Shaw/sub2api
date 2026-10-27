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
)

func newCodexCLIOnlyIngressTestService(detector CodexClientRestrictionDetector) *OpenAIGatewayService {
	cfg := &config.Config{}
	cfg.Security.URLAllowlist.Enabled = false
	cfg.Security.URLAllowlist.AllowInsecureHTTP = true
	cfg.Gateway.OpenAIWS.Enabled = true
	cfg.Gateway.OpenAIWS.OAuthEnabled = true
	cfg.Gateway.OpenAIWS.ResponsesWebsocketsV2 = true
	cfg.Gateway.OpenAIWS.ModeRouterV2Enabled = true
	return &OpenAIGatewayService{
		cfg:              cfg,
		httpUpstream:     &httpUpstreamRecorder{},
		cache:            &stubGatewayCache{},
		openaiWSResolver: NewOpenAIWSProtocolResolver(cfg),
		toolCorrector:    NewCodexToolCorrector(),
		openaiWSPool:     newOpenAIWSConnPool(cfg),
		codexDetector:    detector,
	}
}

// newCodexCLIOnlyIngressTestAccount 的 WS 模式为 off：放行的请求走到模式判定即返回
// "websocket mode is disabled"，不会拨号上游，据此区分放行与拒绝。
func newCodexCLIOnlyIngressTestAccount(codexCLIOnly bool) *Account {
	return &Account{
		ID:          951,
		Name:        "openai-oauth-codex-cli-only",
		Platform:    PlatformOpenAI,
		Type:        AccountTypeOAuth,
		Status:      StatusActive,
		Schedulable: true,
		Concurrency: 1,
		Credentials: map[string]any{"access_token": "oauth-token"},
		Extra: map[string]any{
			"codex_cli_only": codexCLIOnly,
			"openai_oauth_responses_websockets_v2_mode": OpenAIWSIngressModeOff,
		},
	}
}

func runCodexCLIOnlyIngress(t *testing.T, svc *OpenAIGatewayService, account *Account, header http.Header) error {
	t.Helper()
	gin.SetMode(gin.TestMode)

	serverErrCh := make(chan error, 1)
	wsServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := coderws.Accept(w, r, nil)
		if err != nil {
			serverErrCh <- err
			return
		}
		defer func() {
			_ = conn.CloseNow()
		}()

		ginCtx, _ := gin.CreateTestContext(httptest.NewRecorder())
		ginCtx.Request = r.Clone(r.Context())

		readCtx, cancel := context.WithTimeout(r.Context(), 3*time.Second)
		_, firstMessage, readErr := conn.Read(readCtx)
		cancel()
		if readErr != nil {
			serverErrCh <- readErr
			return
		}
		serverErrCh <- svc.ProxyResponsesWebSocketFromClient(r.Context(), ginCtx, conn, account, "oauth-token", firstMessage, nil)
	}))
	defer wsServer.Close()

	dialCtx, cancelDial := context.WithTimeout(context.Background(), 3*time.Second)
	clientConn, _, err := coderws.Dial(dialCtx, "ws"+strings.TrimPrefix(wsServer.URL, "http"), &coderws.DialOptions{HTTPHeader: header})
	cancelDial()
	require.NoError(t, err)
	defer func() {
		_ = clientConn.CloseNow()
	}()

	writeCtx, cancelWrite := context.WithTimeout(context.Background(), 3*time.Second)
	err = clientConn.Write(writeCtx, coderws.MessageText, []byte(`{"type":"response.create","model":"gpt-5.1","stream":false}`))
	cancelWrite()
	require.NoError(t, err)

	select {
	case serverErr := <-serverErrCh:
		return serverErr
	case <-time.After(5 * time.Second):
		t.Fatal("等待 ingress websocket 结束超时")
		return nil
	}
}

func TestOpenAIGatewayService_ProxyResponsesWebSocketFromClient_CodexCLIOnly(t *testing.T) {
	officialHeader := func() http.Header {
		h := http.Header{}
		h.Set("User-Agent", "codex_cli_rs/0.153.4 (Windows 11; x86_64) WindowsTerminal")
		h.Set("originator", "codex_cli_rs")
		h.Set("x-codex-window-id", "window-1")
		return h
	}
	scriptHeader := func() http.Header {
		h := http.Header{}
		h.Set("User-Agent", "ctrl-probe/1")
		h.Set("session_id", "sess-1")
		return h
	}

	t.Run("非官方客户端被拒：1008 + 通用文案，可按哨兵错误识别", func(t *testing.T) {
		svc := newCodexCLIOnlyIngressTestService(nil)
		err := runCodexCLIOnlyIngress(t, svc, newCodexCLIOnlyIngressTestAccount(true), scriptHeader())

		var closeErr *OpenAIWSClientCloseError
		require.ErrorAs(t, err, &closeErr)
		require.Equal(t, coderws.StatusPolicyViolation, closeErr.StatusCode())
		require.Equal(t, CodexOfficialClientsOnlyMessage, closeErr.Reason())
		require.True(t, errors.Is(err, ErrCodexClientRestricted))
	})

	t.Run("版本越界：关闭原因使用带版本号的差异化文案", func(t *testing.T) {
		svc := newCodexCLIOnlyIngressTestService(&stubCodexRestrictionDetector{result: CodexClientRestrictionDetectionResult{
			Enabled:         true,
			Matched:         false,
			Reason:          CodexClientRestrictionReasonVersionTooLow,
			DetectedVersion: "0.39.0",
			MinCodexVersion: "0.42.0",
		}})
		err := runCodexCLIOnlyIngress(t, svc, newCodexCLIOnlyIngressTestAccount(true), officialHeader())

		var closeErr *OpenAIWSClientCloseError
		require.ErrorAs(t, err, &closeErr)
		require.Contains(t, closeErr.Reason(), "Your Codex version (0.39.0) is below the minimum required version (0.42.0)")
		require.True(t, errors.Is(err, ErrCodexClientRestricted))
	})

	t.Run("官方客户端放行：继续进入模式判定", func(t *testing.T) {
		svc := newCodexCLIOnlyIngressTestService(nil)
		err := runCodexCLIOnlyIngress(t, svc, newCodexCLIOnlyIngressTestAccount(true), officialHeader())

		var closeErr *OpenAIWSClientCloseError
		require.ErrorAs(t, err, &closeErr)
		require.Equal(t, "websocket mode is disabled for this account", closeErr.Reason())
		require.False(t, errors.Is(err, ErrCodexClientRestricted))
	})

	t.Run("账号未开 codex_cli_only：非官方客户端不受影响", func(t *testing.T) {
		svc := newCodexCLIOnlyIngressTestService(nil)
		err := runCodexCLIOnlyIngress(t, svc, newCodexCLIOnlyIngressTestAccount(false), scriptHeader())

		var closeErr *OpenAIWSClientCloseError
		require.ErrorAs(t, err, &closeErr)
		require.Equal(t, "websocket mode is disabled for this account", closeErr.Reason())
		require.False(t, errors.Is(err, ErrCodexClientRestricted))
	})
}
