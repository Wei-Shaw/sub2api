package handler

import (
	"context"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/service"
	coderws "github.com/coder/websocket"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

func TestOpenAIWSCodexCLIOnlyRejectsUnofficialClient(t *testing.T) {
	gin.SetMode(gin.TestMode)
	repo := &openAIWSTurnBudgetAccountRepo{accounts: []service.Account{{
		ID: 901, Name: "ws-codex-cli-only", Platform: service.PlatformOpenAI,
		Type: service.AccountTypeOAuth, Status: service.StatusActive, Schedulable: true, Concurrency: 1, Priority: 1,
		Credentials: map[string]any{"access_token": "oauth-token"},
		Extra: map[string]any{
			"codex_cli_only": true,
			"openai_oauth_responses_websockets_v2_enabled": true,
			"openai_oauth_responses_websockets_v2_mode":    service.OpenAIWSIngressModeCtxPool,
		},
	}}}
	conn, finish := newOpenAIWSHandshakeTestClient(t, repo, 1, func(cfg *config.Config) {
		cfg.Gateway.OpenAIWS.OAuthEnabled = true
	})
	ctx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
	defer cancel()
	require.NoError(t, conn.Write(ctx, coderws.MessageText, []byte(`{"type":"response.create","model":"gpt-5.1","input":"hello"}`)))

	_, payload, err := conn.Read(ctx)
	require.NoError(t, err)
	require.Equal(t, "error", gjson.GetBytes(payload, "type").String())
	require.Equal(t, "forbidden_error", gjson.GetBytes(payload, "error.type").String())
	require.Equal(t, service.CodexOfficialClientsOnlyMessage, gjson.GetBytes(payload, "error.message").String())

	_, _, err = conn.Read(ctx)
	var closeErr coderws.CloseError
	require.ErrorAs(t, err, &closeErr)
	require.Equal(t, coderws.StatusPolicyViolation, closeErr.Code)
	require.Equal(t, service.CodexOfficialClientsOnlyMessage, closeErr.Reason)
	finish()
}

// codex_cli_only 是账号设置，拒绝与 HTTP 路径一致计入账号调度失败。
func TestShouldReportOpenAIWSProxyAccountFailureIncludesCodexCLIOnlyRejection(t *testing.T) {
	err := service.NewOpenAIWSClientCloseError(coderws.StatusPolicyViolation, service.CodexOfficialClientsOnlyMessage, service.ErrCodexClientRestricted)
	require.True(t, shouldReportOpenAIWSProxyAccountFailure(err))
}
