package handler

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
	"github.com/Wei-Shaw/sub2api/internal/server/middleware"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/Wei-Shaw/sub2api/internal/testutil"
	coderws "github.com/coder/websocket"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

type wsKeyQueueAdmission struct {
	release  func()
	acquired bool
	err      error
}

func startHandlerKeyQueueConn(t *testing.T) (*coderws.Conn, *coderws.Conn) {
	t.Helper()
	gin.SetMode(gin.TestMode)
	connCh := make(chan *coderws.Conn, 1)
	stop := make(chan struct{})
	var stopOnce sync.Once
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := coderws.Accept(w, r, nil)
		if err != nil {
			return
		}
		connCh <- conn
		<-stop
		_ = conn.CloseNow()
	}))
	client, _, err := coderws.Dial(context.Background(), "ws"+strings.TrimPrefix(server.URL, "http"), nil)
	require.NoError(t, err)
	serverConn := <-connCh
	t.Cleanup(func() {
		stopOnce.Do(func() { close(stop) })
		_ = client.CloseNow()
		server.Close()
	})
	return client, serverConn
}

func runHandlerKeyQueueAdmission(t *testing.T, helper *ConcurrencyHelper, ctx context.Context, serverConn *coderws.Conn, apiKeyID int64, keyLimit int) <-chan wsKeyQueueAdmission {
	t.Helper()
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	service.EnsureOpenAIWSIngressReader(c, serverConn)
	resultCh := make(chan wsKeyQueueAdmission, 1)
	go func() {
		release, acquired, err := admitOpenAIWSTurn(helper, ctx, c, 202, 3, apiKeyID, keyLimit)
		resultCh <- wsKeyQueueAdmission{release: release, acquired: acquired, err: err}
	}()
	return resultCh
}

func requireQueueWaiting(t *testing.T, cache service.APIKeySlotQueueCache, apiKeyID int64, want int) {
	t.Helper()
	require.Eventually(t, func() bool {
		_, waiting, err := cache.GetAPIKeyQueueStats(context.Background(), apiKeyID)
		return err == nil && waiting == want
	}, 3*time.Second, 10*time.Millisecond)
}

func waitHandlerKeyQueueAdmission(t *testing.T, resultCh <-chan wsKeyQueueAdmission) wsKeyQueueAdmission {
	t.Helper()
	select {
	case got := <-resultCh:
		return got
	case <-time.After(3 * time.Second):
		t.Fatal("key queue admission did not finish promptly")
		return wsKeyQueueAdmission{}
	}
}

// TestOpenAIWSFirstTurnKeyWaitConsumesPendingCancel uses the real Redis queue
// and the handler's first-turn admission: while the turn waits with no ingress
// mode chosen yet, a response.cancel for the pending request must stop the wait
// promptly with an explicit normal close, release the queue ticket, and never
// take the user slot or reach an upstream account.
func TestOpenAIWSFirstTurnKeyWaitConsumesPendingCancel(t *testing.T) {
	helper, rawCache := newAPIKeyAdmissionHelper(t)
	helper.concurrencyService.SetAPIKeyQueuePolicy(service.APIKeyQueuePolicy{MaxWaiting: 4, Timeout: 30 * time.Second})
	queueCache, ok := rawCache.(service.APIKeySlotQueueCache)
	require.True(t, ok)

	holderCtx, cancelHolder := service.WithAPIKeyAdmissionOwner(context.Background())
	defer cancelHolder()
	holder, err := helper.concurrencyService.ReserveAPIKeySlotWithWait(holderCtx, 111, 1)
	require.NoError(t, err)
	defer holder.Release()

	client, serverConn := startHandlerKeyQueueConn(t)
	ctx, cancelOwner := service.WithAPIKeyAdmissionOwner(context.Background())
	defer cancelOwner()
	resultCh := runHandlerKeyQueueAdmission(t, helper, ctx, serverConn, 111, 1)
	requireQueueWaiting(t, queueCache, 111, 1)

	begin := time.Now()
	require.NoError(t, client.Write(context.Background(), coderws.MessageText, []byte(`{"type":"response.cancel"}`)))
	got := waitHandlerKeyQueueAdmission(t, resultCh)
	require.Less(t, time.Since(begin), 3*time.Second)
	require.Nil(t, got.release)
	require.False(t, got.acquired)
	require.False(t, service.IsOpenAIWSClientGoneError(got.err))
	var closeErr *service.OpenAIWSClientCloseError
	require.ErrorAs(t, got.err, &closeErr)
	require.Equal(t, coderws.StatusNormalClosure, closeErr.StatusCode())
	require.Contains(t, closeErr.Reason(), "canceled")
	requireQueueWaiting(t, queueCache, 111, 0)

	userCount, err := rawCache.GetUserConcurrency(context.Background(), 202)
	require.NoError(t, err)
	require.Zero(t, userCount, "a canceled pending first turn must not hold a user slot")
}

// TestOpenAIWSFirstTurnKeyWaitKeepsOldCancelTarget pins target matching with
// the real queue: a cancel carrying an older response_id is preserved, so the
// waiting turn is still admitted only by an actual key release.
func TestOpenAIWSFirstTurnKeyWaitKeepsOldCancelTarget(t *testing.T) {
	helper, rawCache := newAPIKeyAdmissionHelper(t)
	helper.concurrencyService.SetAPIKeyQueuePolicy(service.APIKeyQueuePolicy{MaxWaiting: 4, Timeout: 10 * time.Second})
	queueCache, ok := rawCache.(service.APIKeySlotQueueCache)
	require.True(t, ok)

	holderCtx, cancelHolder := service.WithAPIKeyAdmissionOwner(context.Background())
	defer cancelHolder()
	holder, err := helper.concurrencyService.ReserveAPIKeySlotWithWait(holderCtx, 112, 1)
	require.NoError(t, err)

	client, serverConn := startHandlerKeyQueueConn(t)
	defer func() { _ = client.CloseNow() }()
	ctx, cancelOwner := service.WithAPIKeyAdmissionOwner(context.Background())
	defer cancelOwner()
	resultCh := runHandlerKeyQueueAdmission(t, helper, ctx, serverConn, 112, 1)
	requireQueueWaiting(t, queueCache, 112, 1)

	require.NoError(t, client.Write(context.Background(), coderws.MessageText, []byte(`{"type":"response.cancel","response_id":"resp_old"}`)))
	select {
	case got := <-resultCh:
		t.Fatalf("old response_id canceled the pending turn: %+v", got)
	case <-time.After(300 * time.Millisecond):
	}
	requireQueueWaiting(t, queueCache, 112, 1)

	holder.Release()
	got := waitHandlerKeyQueueAdmission(t, resultCh)
	require.NoError(t, got.err)
	require.True(t, got.acquired)
	require.NotNil(t, got.release)
	got.release()
}

// TestOpenAIWSTurnPricingFreezesAfterBlockedKeyGrant pins the 6.2.1 ordering on
// the real queue: while the key wait blocks the turn, no pricing snapshot
// exists; the freeze happens only after the key grant and the user/account
// slots are held, so the wait is never billed at the pre-wait instant.
func TestOpenAIWSTurnPricingFreezesAfterBlockedKeyGrant(t *testing.T) {
	helper, rawCache := newAPIKeyAdmissionHelper(t)
	helper.concurrencyService.SetAPIKeyQueuePolicy(service.APIKeyQueuePolicy{MaxWaiting: 4, Timeout: 30 * time.Second})
	queueCache, ok := rawCache.(service.APIKeySlotQueueCache)
	require.True(t, ok)

	holderCtx, cancelHolder := service.WithAPIKeyAdmissionOwner(context.Background())
	defer cancelHolder()
	holder, err := helper.concurrencyService.ReserveAPIKeySlotWithWait(holderCtx, 77, 1)
	require.NoError(t, err)

	_, serverConn := startHandlerKeyQueueConn(t)
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	service.EnsureOpenAIWSIngressReader(c, serverConn)
	ctx, cancelOwner := service.WithAPIKeyAdmissionOwner(context.Background())
	defer cancelOwner()

	type pricingAdmissionResult struct {
		userRelease    func()
		accountRelease func()
		err            error
	}
	resultCh := make(chan pricingAdmissionResult, 1)
	var pricing openAIWSTurnPricing
	h := &OpenAIGatewayHandler{concurrencyHelper: helper}
	account := &service.Account{ID: 1, Concurrency: 3}
	go func() {
		userRelease, accountRelease, err := h.admitOpenAIWSTurnForPricing(
			ctx, c, 202, 3, &service.APIKey{ID: 77, ConcurrencyLimit: 1}, account, 3, 2, nil, &pricing,
		)
		resultCh <- pricingAdmissionResult{userRelease: userRelease, accountRelease: accountRelease, err: err}
	}()
	requireQueueWaiting(t, queueCache, 77, 1)
	require.True(t, pricing.currentOr(time.Time{}).IsZero(), "pricing must not be frozen while the key wait is pending")

	grantAt := time.Now()
	holder.Release()
	var got pricingAdmissionResult
	select {
	case got = <-resultCh:
	case <-time.After(5 * time.Second):
		t.Fatal("released key slot did not admit the waiting turn")
	}
	require.NoError(t, got.err)
	require.NotNil(t, got.userRelease)
	require.NotNil(t, got.accountRelease)

	frozen := pricing.currentOr(time.Time{})
	require.False(t, frozen.IsZero(), "granted admission must freeze the turn pricing")
	require.False(t, frozen.Before(grantAt), "pricing must be frozen no earlier than the key grant")
	got.userRelease()
	got.accountRelease()
}

// TestOpenAIWSFirstTurnKeyWaitStopsOnClientClose covers W01 with the real
// Redis-backed queue: the wait ticket is cleaned and no user slot is taken when
// the peer leaves before the key slot is admitted.
func TestOpenAIWSFirstTurnKeyWaitStopsOnClientClose(t *testing.T) {
	helper, rawCache := newAPIKeyAdmissionHelper(t)
	helper.concurrencyService.SetAPIKeyQueuePolicy(service.APIKeyQueuePolicy{MaxWaiting: 4, Timeout: 30 * time.Second})
	queueCache, ok := rawCache.(service.APIKeySlotQueueCache)
	require.True(t, ok)

	holderCtx, cancelHolder := service.WithAPIKeyAdmissionOwner(context.Background())
	defer cancelHolder()
	holder, err := helper.concurrencyService.ReserveAPIKeySlotWithWait(holderCtx, 77, 1)
	require.NoError(t, err)
	defer holder.Release()

	client, serverConn := startHandlerKeyQueueConn(t)
	ctx, cancelOwner := service.WithAPIKeyAdmissionOwner(context.Background())
	defer cancelOwner()
	resultCh := runHandlerKeyQueueAdmission(t, helper, ctx, serverConn, 77, 1)
	requireQueueWaiting(t, queueCache, 77, 1)

	begin := time.Now()
	require.NoError(t, client.CloseNow())
	select {
	case got := <-resultCh:
		if got.release != nil {
			got.release()
		}
		require.False(t, got.acquired)
		require.True(t, service.IsOpenAIWSClientGoneError(got.err), "got %v", got.err)
	case <-time.After(3 * time.Second):
		t.Fatal("client close did not stop the first-turn key wait")
	}
	require.Less(t, time.Since(begin), 3*time.Second)
	requireQueueWaiting(t, queueCache, 77, 0)

	userCount, err := rawCache.GetUserConcurrency(context.Background(), 202)
	require.NoError(t, err)
	require.Zero(t, userCount, "a departed client must not hold a user slot")
}

// TestOpenAIWSKeyWaitAcquiresAfterExternalRelease covers the normal wait path:
// an externally released key slot lets the turn proceed to the immediate user
// slot, and the combined release returns both counters to zero.
func TestOpenAIWSKeyWaitAcquiresAfterExternalRelease(t *testing.T) {
	helper, rawCache := newAPIKeyAdmissionHelper(t)
	helper.concurrencyService.SetAPIKeyQueuePolicy(service.APIKeyQueuePolicy{MaxWaiting: 4, Timeout: 10 * time.Second})
	queueCache, ok := rawCache.(service.APIKeySlotQueueCache)
	require.True(t, ok)

	holderCtx, cancelHolder := service.WithAPIKeyAdmissionOwner(context.Background())
	defer cancelHolder()
	holder, err := helper.concurrencyService.ReserveAPIKeySlotWithWait(holderCtx, 88, 1)
	require.NoError(t, err)

	client, serverConn := startHandlerKeyQueueConn(t)
	defer func() { _ = client.CloseNow() }()
	ctx, cancelOwner := service.WithAPIKeyAdmissionOwner(context.Background())
	defer cancelOwner()
	resultCh := runHandlerKeyQueueAdmission(t, helper, ctx, serverConn, 88, 1)
	requireQueueWaiting(t, queueCache, 88, 1)

	holder.Release()
	var got wsKeyQueueAdmission
	select {
	case got = <-resultCh:
	case <-time.After(5 * time.Second):
		t.Fatal("released key slot did not admit the waiting turn")
	}
	require.NoError(t, got.err)
	require.True(t, got.acquired)
	require.NotNil(t, got.release)
	active, waiting, err := queueCache.GetAPIKeyQueueStats(context.Background(), 88)
	require.NoError(t, err)
	require.Equal(t, 1, active)
	require.Zero(t, waiting)
	userCount, err := rawCache.GetUserConcurrency(context.Background(), 202)
	require.NoError(t, err)
	require.Equal(t, 1, userCount)

	got.release()
	got.release() // single-shot: a duplicate release must not double-decrement
	active, waiting, err = queueCache.GetAPIKeyQueueStats(context.Background(), 88)
	require.NoError(t, err)
	require.Zero(t, active)
	require.Zero(t, waiting)
	userCount, err = rawCache.GetUserConcurrency(context.Background(), 202)
	require.NoError(t, err)
	require.Zero(t, userCount)
}

// TestOpenAIWSKeyQueueAdmissionUsesCurrentTurnPermissions drives the real
// Redis-backed worker admission twice with different per-turn permission
// snapshots: each wait must revalidate against its own turn's models, and a
// previous turn's snapshot must never leak into the next wait (run with -race).
func TestOpenAIWSKeyQueueAdmissionUsesCurrentTurnPermissions(t *testing.T) {
	helper, rawCache := newAPIKeyAdmissionHelper(t)
	helper.concurrencyService.SetAPIKeyQueuePolicy(service.APIKeyQueuePolicy{MaxWaiting: 4, Timeout: 10 * time.Second})
	queueCache, ok := rawCache.(service.APIKeySlotQueueCache)
	require.True(t, ok)

	var expectedMu sync.Mutex
	expected := "model-a"
	var observedMu sync.Mutex
	var observed [][]string
	baseCtx, cancelOwner := service.WithAPIKeyAdmissionOwner(context.Background())
	defer cancelOwner()
	revalidateCtx := service.WithAPIKeyQueueAuthRevalidator(baseCtx, func(ctx context.Context) (int, error) {
		permissions := service.APIKeyQueueRequestPermissionsFromContext(ctx)
		observedMu.Lock()
		observed = append(observed, append([]string(nil), permissions.Models...))
		observedMu.Unlock()
		expectedMu.Lock()
		want := expected
		expectedMu.Unlock()
		if len(permissions.Models) != 1 || permissions.Models[0] != want {
			return 0, service.NewAPIKeyQueueAuthRejected(
				infraerrors.Forbidden("MODEL_NOT_ALLOWED", "turn permissions leaked"),
			)
		}
		return 1, nil
	})

	admitTurn := func(t *testing.T, apiKeyID int64, model string) wsKeyQueueAdmission {
		t.Helper()
		expectedMu.Lock()
		expected = model
		expectedMu.Unlock()

		holderCtx, cancelHolder := service.WithAPIKeyAdmissionOwner(context.Background())
		defer cancelHolder()
		holder, err := helper.concurrencyService.ReserveAPIKeySlotWithWait(holderCtx, apiKeyID, 1)
		require.NoError(t, err)

		client, serverConn := startHandlerKeyQueueConn(t)
		defer func() { _ = client.CloseNow() }()
		turnCtx := service.WithAPIKeyQueueRequestPermissions(revalidateCtx, service.APIKeyQueueRequestPermissions{Models: []string{model}})
		resultCh := runHandlerKeyQueueAdmission(t, helper, turnCtx, serverConn, apiKeyID, 1)
		requireQueueWaiting(t, queueCache, apiKeyID, 1)
		holder.Release()
		got := waitHandlerKeyQueueAdmission(t, resultCh)
		require.NoError(t, got.err)
		require.True(t, got.acquired)
		require.NotNil(t, got.release)
		got.release()
		return got
	}

	admitTurn(t, 333, "model-a")
	observedMu.Lock()
	firstTurnObserved := append([][]string(nil), observed...)
	observedMu.Unlock()
	require.NotEmpty(t, firstTurnObserved)
	for _, models := range firstTurnObserved {
		require.Equal(t, []string{"model-a"}, models)
	}

	admitTurn(t, 333, "model-b")
	observedMu.Lock()
	secondTurnObserved := append([][]string(nil), observed[len(firstTurnObserved):]...)
	observedMu.Unlock()
	require.NotEmpty(t, secondTurnObserved)
	for _, models := range secondTurnObserved {
		require.Equal(t, []string{"model-b"}, models, "the second wait must not see the first turn's snapshot")
	}
}

func TestOpenAIWSQueueTurnPermissionsPerTurnScope(t *testing.T) {
	// The actual/effective model is prepended to the frame's own candidates, so
	// a frame that already carries it yields a duplicate. Duplicates are
	// harmless for the allowlist predicates and preserve the existing
	// turn>1 candidate order.
	text := openAIWSQueueTurnPermissions("gpt-5.4", []byte(`{"type":"response.create","model":"gpt-5.4","input":"hi"}`))
	require.Equal(t, []string{"gpt-5.4", "gpt-5.4"}, text.Models)
	require.Zero(t, text.Capabilities&service.APIKeyQueueCapabilityImageGeneration)

	image := openAIWSQueueTurnPermissions("gpt-5.4", []byte(`{"type":"response.create","model":"gpt-5.4","tools":[{"type":"image_generation"}],"input":"draw"}`))
	require.Equal(t, []string{"gpt-5.4", "gpt-5.4"}, image.Models)
	require.NotZero(t, image.Capabilities&service.APIKeyQueueCapabilityImageGeneration, "native image tool marks the image capability")

	// Duplicate and case-variant model keys stay in the candidate set, and each
	// turn's context keeps its own snapshot after the next turn is installed.
	duplicates := openAIWSQueueTurnPermissions("gpt-5.4", []byte(`{"model":"gpt-5.4","Model":"gpt-4"}`))
	require.Equal(t, []string{"gpt-5.4", "gpt-5.4", "gpt-4"}, duplicates.Models)

	ctxFirst := service.WithAPIKeyQueueRequestPermissions(context.Background(), text)
	ctxSecond := service.WithAPIKeyQueueRequestPermissions(ctxFirst, image)
	require.Zero(t, service.APIKeyQueueRequestPermissionsFromContext(ctxFirst).Capabilities&service.APIKeyQueueCapabilityImageGeneration)
	require.NotZero(t, service.APIKeyQueueRequestPermissionsFromContext(ctxSecond).Capabilities&service.APIKeyQueueCapabilityImageGeneration)
}

// TestOpenAIWSFirstTurnKeyWaitPreservesLeaseCause keeps the typed retryable
// close (1013) and the original ingress lease cause for control cancellation.
func TestOpenAIWSFirstTurnKeyWaitPreservesLeaseCause(t *testing.T) {
	helper, rawCache := newAPIKeyAdmissionHelper(t)
	helper.concurrencyService.SetAPIKeyQueuePolicy(service.APIKeyQueuePolicy{MaxWaiting: 4, Timeout: 30 * time.Second})
	queueCache, ok := rawCache.(service.APIKeySlotQueueCache)
	require.True(t, ok)

	holderCtx, cancelHolder := service.WithAPIKeyAdmissionOwner(context.Background())
	defer cancelHolder()
	holder, err := helper.concurrencyService.ReserveAPIKeySlotWithWait(holderCtx, 99, 1)
	require.NoError(t, err)
	defer holder.Release()

	client, serverConn := startHandlerKeyQueueConn(t)
	baseCtx, cancelOwner := service.WithAPIKeyAdmissionOwner(context.Background())
	defer cancelOwner()
	ctx, cancelCause := context.WithCancelCause(baseCtx)
	defer cancelCause(context.Canceled)
	resultCh := runHandlerKeyQueueAdmission(t, helper, ctx, serverConn, 99, 1)
	requireQueueWaiting(t, queueCache, 99, 1)

	cancelCause(service.ErrOpenAIWSIngressLeaseLost)
	var got wsKeyQueueAdmission
	select {
	case got = <-resultCh:
	case <-time.After(3 * time.Second):
		t.Fatal("lease loss did not stop the key wait")
	}
	require.Nil(t, got.release)
	require.False(t, got.acquired)
	require.False(t, service.IsOpenAIWSClientGoneError(got.err))
	require.ErrorIs(t, got.err, service.ErrOpenAIWSIngressLeaseLost)
	var closeErr *service.OpenAIWSClientCloseError
	require.ErrorAs(t, got.err, &closeErr)
	require.Equal(t, coderws.StatusTryAgainLater, closeErr.StatusCode())
	require.Contains(t, closeErr.Reason(), "lease lost")
	requireQueueWaiting(t, queueCache, 99, 0)

	// The handler's first-turn close path must put the typed 1013 on the wire.
	closed := make(chan struct{})
	go func() {
		defer close(closed)
		closeOpenAIWSAdmissionError(serverConn, nil, "test_lease_lost", got.err)
	}()
	readCtx, stopRead := context.WithTimeout(context.Background(), 3*time.Second)
	defer stopRead()
	_, _, readErr := client.Read(readCtx)
	require.Equal(t, coderws.StatusTryAgainLater, coderws.CloseStatus(readErr))
	select {
	case <-closed:
	case <-time.After(3 * time.Second):
		t.Fatal("lease lost close did not complete")
	}
}

// TestOpenAIResponsesWebSocketFirstTurnKeyWaitEndsWithoutUpstream drives the
// real ResponsesWebSocket handler with the real Redis key queue: while the
// first turn waits on an occupied key, a pending response.cancel must close the
// client with the explicit normal reason and a peer disconnect must clean up
// silently. Both must release the ticket without selecting or dialing an
// upstream account.
func TestOpenAIResponsesWebSocketFirstTurnKeyWaitEndsWithoutUpstream(t *testing.T) {
	gin.SetMode(gin.TestMode)
	for _, tc := range []struct {
		name        string
		end         func(*coderws.Conn) error
		expectClose bool
	}{
		{
			name: "pending response.cancel",
			end: func(client *coderws.Conn) error {
				return client.Write(context.Background(), coderws.MessageText, []byte(`{"type":"response.cancel"}`))
			},
			expectClose: true,
		},
		{
			name: "peer disconnect",
			end: func(client *coderws.Conn) error {
				return client.CloseNow()
			},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			redisCache := testutil.NewRedisConcurrencyCache(t)
			queueCache, ok := redisCache.(service.APIKeySlotQueueCache)
			require.True(t, ok)
			helper := NewConcurrencyHelper(service.NewConcurrencyService(redisCache), SSEPingFormatNone, time.Millisecond)
			helper.concurrencyService.SetAPIKeyQueuePolicy(service.APIKeyQueuePolicy{MaxWaiting: 4, Timeout: 30 * time.Second})

			holderCtx, cancelHolder := service.WithAPIKeyAdmissionOwner(context.Background())
			defer cancelHolder()
			holder, err := helper.concurrencyService.ReserveAPIKeySlotWithWait(holderCtx, 1801, 1)
			require.NoError(t, err)
			defer holder.Release()

			var upstreamRequests atomic.Int32
			upstreamServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				upstreamRequests.Add(1)
				conn, err := coderws.Accept(w, r, nil)
				if err != nil {
					return
				}
				defer func() { _ = conn.CloseNow() }()
				_, _, _ = conn.Read(r.Context())
			}))
			defer upstreamServer.Close()

			groupID := int64(4201)
			account := service.Account{
				ID: 9901, Name: "openai-ws-key-cancel", Platform: service.PlatformOpenAI,
				Type: service.AccountTypeAPIKey, Status: service.StatusActive, Schedulable: true, Concurrency: 1,
				Credentials: map[string]any{"api_key": "sk-test", "base_url": upstreamServer.URL},
				Extra: map[string]any{
					"openai_apikey_responses_websockets_v2_enabled": true,
					"openai_apikey_responses_websockets_v2_mode":    service.OpenAIWSIngressModePassthrough,
				},
			}
			cfg := &config.Config{}
			cfg.RunMode = config.RunModeSimple
			cfg.Default.RateMultiplier = 1
			cfg.Security.URLAllowlist.Enabled = false
			cfg.Security.URLAllowlist.AllowInsecureHTTP = true
			cfg.Gateway.OpenAIWS.Enabled = true
			cfg.Gateway.OpenAIWS.APIKeyEnabled = true
			cfg.Gateway.OpenAIWS.ResponsesWebsocketsV2 = true
			cfg.Gateway.OpenAIWS.ModeRouterV2Enabled = true
			cfg.Gateway.OpenAIWS.DialTimeoutSeconds = 3
			cfg.Gateway.OpenAIWS.ReadTimeoutSeconds = 3
			cfg.Gateway.OpenAIWS.WriteTimeoutSeconds = 3

			billingCacheSvc := service.NewBillingCacheService(nil, nil, nil, nil, nil, nil, cfg, nil)
			gatewaySvc := service.NewOpenAIGatewayService(
				&openAIWSUsageHandlerAccountRepoStub{account: account},
				&openAIWSUsageHandlerUsageLogRepoStub{},
				nil, nil, nil, nil, nil,
				cfg,
				nil, nil,
				service.NewBillingService(cfg, nil),
				nil,
				billingCacheSvc,
				nil,
				&service.DeferredService{},
				nil, nil, nil, nil, nil, nil, nil,
			)
			h := &OpenAIGatewayHandler{
				gatewayService:      gatewaySvc,
				billingCacheService: billingCacheSvc,
				apiKeyService:       &service.APIKeyService{},
				concurrencyHelper:   helper,
			}

			apiKey := &service.APIKey{ID: 1801, GroupID: &groupID, ConcurrencyLimit: 1, User: &service.User{ID: 1701, Status: service.StatusActive}}
			ownerCtx, cancelOwner := service.WithAPIKeyAdmissionOwner(context.Background())
			defer cancelOwner()
			router := gin.New()
			router.Use(func(c *gin.Context) {
				c.Request = c.Request.WithContext(ownerCtx)
				c.Set(string(middleware.ContextKeyAPIKey), apiKey)
				c.Set(string(middleware.ContextKeyUser), middleware.AuthSubject{UserID: apiKey.User.ID, Concurrency: 1})
				c.Next()
			})
			router.GET("/openai/v1/responses", h.ResponsesWebSocket)
			handlerServer := httptest.NewServer(router)
			defer handlerServer.Close()

			client, _, err := coderws.Dial(context.Background(), "ws"+strings.TrimPrefix(handlerServer.URL, "http")+"/openai/v1/responses", nil)
			require.NoError(t, err)
			defer func() { _ = client.CloseNow() }()

			require.NoError(t, client.Write(context.Background(), coderws.MessageText, []byte(`{"type":"response.create","model":"gpt-5.4","input":"hi"}`)))
			requireQueueWaiting(t, queueCache, 1801, 1)

			begin := time.Now()
			require.NoError(t, tc.end(client))
			if tc.expectClose {
				readCtx, cancelRead := context.WithTimeout(context.Background(), 3*time.Second)
				_, _, readErr := client.Read(readCtx)
				cancelRead()
				var closeErr coderws.CloseError
				require.ErrorAs(t, readErr, &closeErr)
				require.Equal(t, coderws.StatusNormalClosure, closeErr.Code)
				require.Contains(t, closeErr.Reason, "canceled")
			}
			requireQueueWaiting(t, queueCache, 1801, 0)
			require.Less(t, time.Since(begin), 3*time.Second)
			require.Zero(t, upstreamRequests.Load(), "an unadmitted first turn must not reach the upstream account")
			userCount, err := redisCache.GetUserConcurrency(context.Background(), 1701)
			require.NoError(t, err)
			require.Zero(t, userCount, "an unadmitted first turn must not hold a user slot")
		})
	}
}

// TestOpenAIResponsesWebSocketFirstTurnCarriesPermissionsToWorker pins the
// first-turn ctx wiring: the immutable per-turn permissions must be installed
// before the handler's first key admission so the waiting worker revalidates
// against the actual first-frame model and image intent, not an empty snapshot.
func TestOpenAIResponsesWebSocketFirstTurnCarriesPermissionsToWorker(t *testing.T) {
	gin.SetMode(gin.TestMode)
	redisCache := testutil.NewRedisConcurrencyCache(t)
	queueCache, ok := redisCache.(service.APIKeySlotQueueCache)
	require.True(t, ok)
	helper := NewConcurrencyHelper(service.NewConcurrencyService(redisCache), SSEPingFormatNone, time.Millisecond)
	helper.concurrencyService.SetAPIKeyQueuePolicy(service.APIKeyQueuePolicy{MaxWaiting: 4, Timeout: 30 * time.Second})

	const apiKeyID, userID = int64(1802), int64(1702)
	holderCtx, cancelHolder := service.WithAPIKeyAdmissionOwner(context.Background())
	defer cancelHolder()
	holder, err := helper.concurrencyService.ReserveAPIKeySlotWithWait(holderCtx, apiKeyID, 1)
	require.NoError(t, err)
	defer holder.Release()

	upstreamServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, acceptErr := coderws.Accept(w, r, nil)
		if acceptErr != nil {
			return
		}
		defer func() { _ = conn.CloseNow() }()
		_, _, _ = conn.Read(r.Context())
	}))
	defer upstreamServer.Close()

	groupID := int64(4202)
	account := service.Account{
		ID: 9902, Name: "openai-ws-first-turn-permissions", Platform: service.PlatformOpenAI,
		Type: service.AccountTypeAPIKey, Status: service.StatusActive, Schedulable: true, Concurrency: 1,
		Credentials: map[string]any{"api_key": "sk-test", "base_url": upstreamServer.URL},
		Extra: map[string]any{
			"openai_apikey_responses_websockets_v2_enabled": true,
			"openai_apikey_responses_websockets_v2_mode":    service.OpenAIWSIngressModePassthrough,
		},
	}
	cfg := &config.Config{}
	cfg.RunMode = config.RunModeSimple
	cfg.Default.RateMultiplier = 1
	cfg.Security.URLAllowlist.Enabled = false
	cfg.Security.URLAllowlist.AllowInsecureHTTP = true
	cfg.Gateway.OpenAIWS.Enabled = true
	cfg.Gateway.OpenAIWS.APIKeyEnabled = true
	cfg.Gateway.OpenAIWS.ResponsesWebsocketsV2 = true
	cfg.Gateway.OpenAIWS.ModeRouterV2Enabled = true
	cfg.Gateway.OpenAIWS.DialTimeoutSeconds = 3
	cfg.Gateway.OpenAIWS.ReadTimeoutSeconds = 3
	cfg.Gateway.OpenAIWS.WriteTimeoutSeconds = 3

	billingCacheSvc := service.NewBillingCacheService(nil, nil, nil, nil, nil, nil, cfg, nil)
	gatewaySvc := service.NewOpenAIGatewayService(
		&openAIWSUsageHandlerAccountRepoStub{account: account},
		&openAIWSUsageHandlerUsageLogRepoStub{},
		nil, nil, nil, nil, nil,
		cfg,
		nil, nil,
		service.NewBillingService(cfg, nil),
		nil,
		billingCacheSvc,
		nil,
		&service.DeferredService{},
		nil, nil, nil, nil, nil, nil, nil,
	)
	h := &OpenAIGatewayHandler{
		gatewayService:      gatewaySvc,
		billingCacheService: billingCacheSvc,
		apiKeyService:       &service.APIKeyService{},
		concurrencyHelper:   helper,
	}

	apiKey := &service.APIKey{ID: apiKeyID, GroupID: &groupID, ConcurrencyLimit: 1, User: &service.User{ID: userID, Status: service.StatusActive}}
	ownerCtx, cancelOwner := service.WithAPIKeyAdmissionOwner(context.Background())
	defer cancelOwner()
	var observedMu sync.Mutex
	var observed []service.APIKeyQueueRequestPermissions
	ownerCtx = service.WithAPIKeyQueueAuthRevalidator(ownerCtx, func(ctx context.Context) (int, error) {
		permissions := service.APIKeyQueueRequestPermissionsFromContext(ctx)
		observedMu.Lock()
		observed = append(observed, permissions)
		observedMu.Unlock()
		if len(permissions.Models) == 0 {
			return 0, service.NewAPIKeyQueueAuthRejected(infraerrors.Forbidden("MODEL_NOT_ALLOWED", "first turn lost its permissions"))
		}
		return 1, nil
	})

	router := gin.New()
	router.Use(func(c *gin.Context) {
		c.Request = c.Request.WithContext(ownerCtx)
		c.Set(string(middleware.ContextKeyAPIKey), apiKey)
		c.Set(string(middleware.ContextKeyUser), middleware.AuthSubject{UserID: apiKey.User.ID, Concurrency: 1})
		c.Next()
	})
	router.GET("/openai/v1/responses", h.ResponsesWebSocket)
	handlerServer := httptest.NewServer(router)
	defer handlerServer.Close()

	client, _, err := coderws.Dial(context.Background(), "ws"+strings.TrimPrefix(handlerServer.URL, "http")+"/openai/v1/responses", nil)
	require.NoError(t, err)
	defer func() { _ = client.CloseNow() }()
	require.NoError(t, client.Write(context.Background(), coderws.MessageText, []byte(`{"type":"response.create","model":"gpt-5.4","tools":[{"type":"image_generation"}],"input":"draw"}`)))
	requireQueueWaiting(t, queueCache, apiKeyID, 1)

	observedMu.Lock()
	snapshot := append([]service.APIKeyQueueRequestPermissions(nil), observed...)
	observedMu.Unlock()
	require.NotEmpty(t, snapshot, "the first admission must revalidate before entering the queue")
	for _, permissions := range snapshot {
		require.Contains(t, permissions.Models, "gpt-5.4", "the worker must see the first frame's model")
		require.NotZero(t, permissions.Capabilities&service.APIKeyQueueCapabilityImageGeneration, "the worker must see the first frame's image intent")
	}

	holder.Release()
	requireQueueWaiting(t, queueCache, apiKeyID, 0)
}
