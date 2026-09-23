package service

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	coderws "github.com/coder/websocket"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

func TestCodexTicketWSUsesLiveOptInIndependentOfGlobalSwitch(t *testing.T) {
	account := ticketTestAccount(41)
	live := *account
	live.Extra = map[string]any{}
	repo := &codexTicketRefreshRepo{accounts: []Account{live}}
	svc := ticketTestService(t, config.OpenAICodexTicketConfig{Enabled: false}, nil)
	svc.accountRepo = repo
	require.False(t, svc.shouldBridgeOpenAICodexTicketAccount(context.Background(), account), "persisted opt-out overrides a stale enabled snapshot")
	require.NoError(t, svc.checkOpenAICodexTicketNativeTurn(context.Background(), account))
	require.NoError(t, repo.UpdateExtra(context.Background(), account.ID, account.Extra))
	stale := live
	require.True(t, svc.shouldBridgeOpenAICodexTicketAccount(context.Background(), &stale), "global off must not create a native session that survives re-enable")
	var closeErr *OpenAIWSClientCloseError
	require.ErrorAs(t, svc.checkOpenAICodexTicketNativeTurn(context.Background(), &stale), &closeErr)
	require.Equal(t, coderws.StatusTryAgainLater, closeErr.statusCode)
	stale.Type = AccountTypeAPIKey
	require.False(t, svc.shouldBridgeOpenAICodexTicketAccount(context.Background(), &stale))
}

func TestCodexTicketWSHTTPBridgeRechecksEveryTurn(t *testing.T) {
	for _, change := range []string{"refresh", "expire", "account_off", "global_off"} {
		t.Run(change, func(t *testing.T) {
			gin.SetMode(gin.TestMode)
			ctx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
			defer cancel()
			account := ticketTestAccount(41)
			account.Extra["responses_websockets_v2_enabled"] = true
			account.Extra["responses_websockets_v2_mode"] = "passthrough"
			ticket := verifiedTestTicket(account, 292)
			account.Extra[openAICodexTicketExtraKey(ticket.Model)] = ticket
			repo := &codexTicketRefreshRepo{accounts: []Account{*account}}
			requestStates := make(chan string, 4)
			upstream := &codexTicketFuncUpstream{do: func(req *http.Request) (*http.Response, error) {
				requestStates <- req.Header.Get(openAICodexTurnStateHeader)
				return &http.Response{StatusCode: http.StatusOK, Header: http.Header{"Content-Type": []string{"text/event-stream"}}, Body: io.NopCloser(strings.NewReader("data: " + fmt.Sprintf(`{"type":"response.completed","response":{"id":"resp_ticket","model":%q,"status":"completed","usage":{"input_tokens":1,"output_tokens":1}}}`, ticket.Model) + "\n\n"))}, nil
			}}
			svc := ticketTestService(t, config.OpenAICodexTicketConfig{Enabled: true}, upstream)
			svc.accountRepo = repo
			svc.settingService = NewSettingService(&codexTicketSettingRepo{codexPolicyMigrationRepoStub: &codexPolicyMigrationRepoStub{values: map[string]string{SettingKeyOpenAICodexTicketEnabled: "true"}}}, svc.cfg)
			svc.settingService.openAICodexTicketEnabledCache.Store(&cachedOpenAICodexTicketEnabled{value: true, expiresAt: time.Now().Add(time.Hour).UnixNano()})
			svc.cache = &stubGatewayCache{}
			svc.cfg.Gateway.MaxLineSize = defaultMaxLineSize
			svc.cfg.Gateway.OpenAIWS.Enabled = true
			svc.cfg.Gateway.OpenAIWS.OAuthEnabled = true
			svc.cfg.Gateway.OpenAIWS.ResponsesWebsocketsV2 = true
			svc.cfg.Gateway.OpenAIWS.ModeRouterV2Enabled = true
			svc.cfg.Gateway.OpenAIWS.IngressModeDefault = OpenAIWSIngressModePassthrough
			// No native WS dialer/pool is supplied: all turns must take HTTP.
			server, serverErr := startPassthroughHookRecordingServer(t, ctx, svc, account, nil)
			defer server.Close()
			client, _, err := coderws.Dial(ctx, "ws"+strings.TrimPrefix(server.URL, "http"), nil)
			require.NoError(t, err)
			defer func() { _ = client.CloseNow() }()
			writeTurn := func() {
				t.Helper()
				require.NoError(t, client.Write(ctx, coderws.MessageText, []byte(`{"type":"response.create","model":"gpt-6-astra","input":"ping"}`)))
			}
			readCompleted := func() {
				t.Helper()
				_, payload, readErr := client.Read(ctx)
				require.NoError(t, readErr)
				require.Equal(t, "response.completed", gjson.GetBytes(payload, "type").String())
			}
			writeTurn()
			readCompleted()
			require.Equal(t, ticket.State, <-requestStates)
			wantState := ""
			switch change {
			case "refresh", "expire":
				next := *ticket
				next.CapturedAt = next.CapturedAt.Add(time.Millisecond)
				next.ExpiresAt = next.CapturedAt.Add(time.Hour)
				next.State = openAICodexTicketStatePrefix + strings.Repeat("C", next.Length-len(openAICodexTicketStatePrefix))
				if change == "expire" {
					next.CapturedAt = time.Now().Add(-2 * time.Hour)
					next.ExpiresAt = time.Now().Add(-time.Hour)
					svc.openaiCodexTickets.Delete(openAICodexTicketKey(account.ID, ticket.Model))
				}
				require.NoError(t, repo.UpdateExtra(ctx, account.ID, map[string]any{openAICodexTicketExtraKey(next.Model): &next}))
				wantState = next.State
			case "account_off":
				ac := codexAccountTicketConfigOf(account)
				ac.Enabled = false
				require.NoError(t, repo.UpdateExtra(ctx, account.ID, map[string]any{codexAccountTicketConfigKey: ac}))
			case "global_off":
				svc.settingService.openAICodexTicketEnabledCache.Store(&cachedOpenAICodexTicketEnabled{value: false, expiresAt: time.Now().Add(time.Hour).UnixNano()})
			}
			writeTurn()
			if change == "expire" {
				select {
				case proxyErr := <-serverErr:
					require.ErrorIs(t, proxyErr, ErrOpenAICodexTicketUnavailable)
				case <-ctx.Done():
					t.Fatal("expired ticket did not stop the next turn")
				}
				require.Empty(t, requestStates, "expired ticket must not issue an upstream request")
				return
			}
			readCompleted()
			require.Equal(t, wantState, <-requestStates)
			require.NoError(t, client.Close(coderws.StatusNormalClosure, "done"))
			select {
			case proxyErr := <-serverErr:
				require.NoError(t, proxyErr)
			case <-ctx.Done():
				t.Fatal("bridge did not stop")
			}
		})
	}
}

func TestCodexTicketLegacyPassthroughClosesOnAccountOptIn(t *testing.T) {
	gin.SetMode(gin.TestMode)
	ctx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
	defer cancel()
	upstream := newStagedPassthroughConn()
	upstream.Send(`{"type":"response.completed","response":{"id":"resp_before_optin","model":"gpt-5.1","usage":{"input_tokens":1,"output_tokens":1}}}`)
	account := passthroughLifecycleAccount()
	account.Type = AccountTypeOAuth
	account.Credentials["access_token"] = "test-token"
	account.Extra["openai_oauth_responses_websockets_v2_mode"] = OpenAIWSIngressModePassthrough
	repo := &codexTicketRefreshRepo{accounts: []Account{*account}}
	svc := newPassthroughLifecycleService(passthroughLifecycleConfig(), upstream)
	svc.cfg.Gateway.OpenAIWS.OAuthEnabled = true
	svc.accountRepo = repo
	server, serverErr := startPassthroughHookRecordingServer(t, ctx, svc, account, nil)
	defer server.Close()
	client := dialPassthroughLifecycleClient(t, server)
	defer func() { _ = client.CloseNow() }()
	requirePassthroughUpstreamWrite(t, upstream, time.Second)
	_, err := readPassthroughLifecycleFrame(t, client, 3*time.Second)
	require.NoError(t, err)
	require.NoError(t, repo.UpdateExtra(ctx, account.ID, ticketTestAccount(account.ID).Extra))
	require.NoError(t, client.Write(ctx, coderws.MessageText, []byte(`{"type":"response.create","model":"gpt-5.1"}`)))
	select {
	case proxyErr := <-serverErr:
		var closeErr *OpenAIWSClientCloseError
		require.ErrorAs(t, proxyErr, &closeErr)
		require.Equal(t, coderws.StatusTryAgainLater, closeErr.statusCode)
	case <-ctx.Done():
		t.Fatal("legacy passthrough did not request reconnect")
	}
	require.Empty(t, upstream.writes, "post-optin turn must not reach the old native connection")
}
