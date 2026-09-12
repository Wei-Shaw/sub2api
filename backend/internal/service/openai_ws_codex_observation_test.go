package service

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	coderws "github.com/coder/websocket"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

type quotaWSConn struct{ *stagedPassthroughConn }

func (c *quotaWSConn) WriteJSON(ctx context.Context, value any) error {
	payload, err := json.Marshal(value)
	if err != nil {
		return err
	}
	return c.WriteFrame(ctx, coderws.MessageText, payload)
}

type quotaWSDialer struct {
	conn    openAIWSClientConn
	headers http.Header
	dials   atomic.Int32
}

func (d *quotaWSDialer) Dial(context.Context, string, http.Header, string) (openAIWSClientConn, int, http.Header, error) {
	d.dials.Add(1)
	return d.conn, http.StatusSwitchingProtocols, d.headers.Clone(), nil
}

func newWSQuotaFixture(t *testing.T, mode string) (*OpenAIGatewayService, *Account, *quotaWSConn, *quotaWSDialer, *snapshotUpdateAccountRepo) {
	t.Helper()
	cfg := passthroughLifecycleConfig()
	cfg.Gateway.OpenAIWS.OAuthEnabled = true
	cfg.Gateway.OpenAIWS.MaxConnsPerAccount = 1
	cfg.Gateway.OpenAIWS.MinIdlePerAccount = 0
	cfg.Gateway.OpenAIWS.MaxIdlePerAccount = 1
	cfg.Gateway.OpenAIWS.ReadTimeoutSeconds = 10
	cfg.Gateway.OpenAIWS.IngressInterTurnIdleTimeoutSeconds = 10
	cfg.Gateway.OpenAIFirstOutputTimeoutSeconds = 10
	upstream := &quotaWSConn{newStagedPassthroughConn()}
	headers := codexObservationTestHeaders()
	headers.Set(openAIWSTurnStateHeader, "preserved-turn-state")
	dialer := &quotaWSDialer{conn: upstream, headers: headers}
	repo := &snapshotUpdateAccountRepo{updateExtraCalls: make(chan map[string]any, 8)}
	svc := &OpenAIGatewayService{
		cfg: cfg, accountRepo: repo, httpUpstream: &httpUpstreamRecorder{},
		cache: &stubGatewayCache{}, openaiWSResolver: NewOpenAIWSProtocolResolver(cfg),
		toolCorrector: NewCodexToolCorrector(), openaiWSPassthroughDialer: dialer,
	}
	pool := svc.getOpenAIWSConnPool()
	pool.setClientDialerForTest(dialer)
	t.Cleanup(svc.codexObservationGate.close)
	t.Cleanup(pool.Close)
	t.Cleanup(func() { _ = upstream.Close() })
	account := &Account{
		ID: 905, Platform: PlatformOpenAI, Type: AccountTypeOAuth,
		Status: StatusActive, Schedulable: true, Concurrency: 1,
		Credentials: map[string]any{"access_token": "oauth-token", "chatgpt_account_id": "account-id"},
		Extra:       map[string]any{"responses_websockets_v2_enabled": true, "openai_oauth_responses_websockets_v2_mode": mode},
	}
	return svc, account, upstream, dialer, repo
}

func requireWSHandshakeObservation(t *testing.T, repo *snapshotUpdateAccountRepo) map[string]any {
	t.Helper()
	select {
	case updates := <-repo.updateExtraCalls:
		observedAt, err := time.Parse(time.RFC3339Nano, updates["codex_usage_updated_at"].(string))
		require.NoError(t, err)
		require.Equal(t, observedAt.Add(600*time.Second).UTC().Format(time.RFC3339), updates["codex_5h_reset_at"])
		return updates
	case <-time.After(3 * time.Second):
		t.Fatal("quota must be captured before the first upstream turn completes")
		return nil
	}
}

func requireNoRepeatedWSQuota(t *testing.T, svc *OpenAIGatewayService, repo *snapshotUpdateAccountRepo) {
	t.Helper()
	waitCodexObservationWrites(t, svc)
	select {
	case updates := <-repo.updateExtraCalls:
		t.Fatalf("reused handshake must not become a new observation: %v", updates)
	default:
	}
}

func TestNativeWSQuotaCapturedDuringPrewarmOnlyOnce(t *testing.T) {
	svc, account, _, dialer, repo := newWSQuotaFixture(t, OpenAIWSIngressModeCtxPool)
	pool := svc.getOpenAIWSConnPool()
	pool.cfg.Gateway.OpenAIWS.MinIdlePerAccount = 1
	req := openAIWSAcquireRequest{
		Account: account, WSURL: "wss://example.test/responses", OnHandshake: svc.captureOpenAIWSHandshakeQuota,
	}
	ap := pool.getOrCreateAccountPool(account.ID)
	ap.mu.Lock()
	ap.lastAcquire = cloneOpenAIWSAcquireRequestPtr(&req)
	ap.mu.Unlock()
	pool.ensureTargetIdleAsync(account.ID)
	requireWSHandshakeObservation(t, repo)

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	for range 2 {
		lease, err := pool.Acquire(ctx, req)
		require.NoError(t, err)
		require.True(t, lease.Reused())
		require.Equal(t, "preserved-turn-state", lease.HandshakeHeader(openAIWSTurnStateHeader))
		lease.Release()
	}
	require.EqualValues(t, 1, dialer.dials.Load())
	requireNoRepeatedWSQuota(t, svc, repo)
}

func TestNativeWSV2QuotaCapturedBeforeTurnAndNotOnReuse(t *testing.T) {
	gin.SetMode(gin.TestMode)
	svc, account, upstream, dialer, repo := newWSQuotaFixture(t, OpenAIWSIngressModeCtxPool)
	type forwardOutcome struct {
		result *OpenAIForwardResult
		err    error
	}
	for turn := 1; turn <= 2; turn++ {
		c, _ := gin.CreateTestContext(httptest.NewRecorder())
		c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", nil)
		c.Request.Header.Set("User-Agent", "codex_cli_rs/0.98.0")
		c.Request.Header.Set("session_id", "quota-session")
		finished := make(chan forwardOutcome, 1)
		go func() {
			result, err := svc.Forward(context.Background(), c, account, []byte(`{"model":"gpt-5.1","stream":true,"input":"hi"}`))
			finished <- forwardOutcome{result, err}
		}()
		requirePassthroughUpstreamWrite(t, upstream.stagedPassthroughConn, 3*time.Second)
		if turn == 1 {
			requireWSHandshakeObservation(t, repo)
		}
		upstream.Send(fmt.Sprintf(`{"type":"response.completed","response":{"id":"resp_quota_%d","model":"gpt-5.1","usage":{"input_tokens":1,"output_tokens":1}}}`, turn))
		select {
		case outcome := <-finished:
			require.NoError(t, outcome.err)
			require.NotNil(t, outcome.result)
			require.True(t, outcome.result.CodexUsageCaptured)
			require.Equal(t, "preserved-turn-state", outcome.result.ResponseHeaders.Get(openAIWSTurnStateHeader))
		case <-time.After(3 * time.Second):
			t.Fatal("WS forward did not complete")
		}
	}
	require.EqualValues(t, 1, dialer.dials.Load())
	requireNoRepeatedWSQuota(t, svc, repo)
}

func TestNativeWSIngressQuotaCapturedOnceAcrossTurns(t *testing.T) {
	gin.SetMode(gin.TestMode)
	for _, mode := range []string{OpenAIWSIngressModeCtxPool, OpenAIWSIngressModePassthrough} {
		t.Run(mode, func(t *testing.T) {
			svc, account, upstream, dialer, repo := newWSQuotaFixture(t, mode)
			controlCtx, cancelControl := context.WithCancel(context.Background())
			defer cancelControl()
			results := make(chan *OpenAIForwardResult, 2)
			server, serverErr := startPassthroughLifecycleServerWithHooks(t, controlCtx, svc, account, func(*gin.Context) *OpenAIWSIngressHooks {
				return &OpenAIWSIngressHooks{AfterTurn: func(_ int, result *OpenAIForwardResult, _ error) { results <- result }}
			})
			defer server.Close()
			client := dialPassthroughLifecycleClient(t, server)
			defer func() { _ = client.CloseNow() }()
			for turn := 1; turn <= 2; turn++ {
				if turn > 1 {
					writeCtx, cancel := context.WithTimeout(controlCtx, 3*time.Second)
					err := client.Write(writeCtx, coderws.MessageText, []byte(`{"type":"response.create","model":"gpt-5.1","stream":true,"input":"hi"}`))
					cancel()
					require.NoError(t, err)
				}
				requirePassthroughUpstreamWrite(t, upstream.stagedPassthroughConn, 3*time.Second)
				if turn == 1 {
					requireWSHandshakeObservation(t, repo)
				}
				upstream.Send(fmt.Sprintf(`{"type":"response.completed","response":{"id":"resp_quota_ingress_%d","model":"gpt-5.1","usage":{"input_tokens":1,"output_tokens":1}}}`, turn))
				_, err := readPassthroughLifecycleFrame(t, client, 3*time.Second)
				require.NoError(t, err)
				select {
				case result := <-results:
					require.NotNil(t, result)
					require.True(t, result.CodexUsageCaptured)
					require.Equal(t, "preserved-turn-state", result.ResponseHeaders.Get(openAIWSTurnStateHeader))
				case <-time.After(3 * time.Second):
					t.Fatal("missing WS turn callback")
				}
			}
			require.EqualValues(t, 1, dialer.dials.Load())
			requireNoRepeatedWSQuota(t, svc, repo)
			_ = client.CloseNow()
			cancelControl()
			select {
			case <-serverErr:
			case <-time.After(3 * time.Second):
				t.Fatal("WS ingress did not stop")
			}
		})
	}
}

func TestNativeWSQuotaExcludesShadowAndAPIKeyAccounts(t *testing.T) {
	svc, account, _, _, repo := newWSQuotaFixture(t, OpenAIWSIngressModeCtxPool)
	parentID := int64(123)
	account.ParentAccountID = &parentID
	svc.captureOpenAIWSHandshakeQuota(context.Background(), account, codexObservationTestHeaders())
	account.ParentAccountID = nil
	account.Type = AccountTypeAPIKey
	svc.captureOpenAIWSHandshakeQuota(context.Background(), account, codexObservationTestHeaders())
	svc.captureOpenAIWSHandshakeQuota(context.Background(), nil, codexObservationTestHeaders())
	requireNoRepeatedWSQuota(t, svc, repo)
}
