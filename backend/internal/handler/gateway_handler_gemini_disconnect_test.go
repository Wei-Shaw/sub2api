//go:build unit

package handler

import (
	"bytes"
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"testing/iotest"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/pkg/ctxkey"
	"github.com/Wei-Shaw/sub2api/internal/pkg/logger"
	"github.com/Wei-Shaw/sub2api/internal/pkg/tlsfingerprint"
	middleware "github.com/Wei-Shaw/sub2api/internal/server/middleware"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
	"go.uber.org/zap/zaptest/observer"
)

type geminiDisconnectHandlerUpstream struct {
	response           *http.Response
	responseForRequest func(*http.Request) *http.Response
	dispatched         chan *http.Request
}

func (s *geminiDisconnectHandlerUpstream) Do(req *http.Request, _ string, _ int64, _ int) (*http.Response, error) {
	if s.dispatched != nil {
		s.dispatched <- req
	}
	if s.responseForRequest != nil {
		return s.responseForRequest(req), nil
	}
	resp := *s.response
	return &resp, nil
}

func (s *geminiDisconnectHandlerUpstream) DoWithTLS(req *http.Request, proxyURL string, accountID int64, accountConcurrency int, _ *tlsfingerprint.Profile) (*http.Response, error) {
	return s.Do(req, proxyURL, accountID, accountConcurrency)
}

type geminiDisconnectUsageRepo struct {
	service.UsageLogRepository
	created chan *service.UsageLog
}

func (s *geminiDisconnectUsageRepo) Create(_ context.Context, log *service.UsageLog) (bool, error) {
	s.created <- log
	return true, nil
}

type geminiHandlerDelayedBody struct {
	ctx       context.Context
	release   <-chan []byte
	readStart chan struct{}
	reader    *bytes.Reader
	once      sync.Once
}

func (r *geminiHandlerDelayedBody) Read(p []byte) (int, error) {
	r.once.Do(func() { close(r.readStart) })
	if r.reader != nil {
		return r.reader.Read(p)
	}
	select {
	case <-r.ctx.Done():
		return 0, r.ctx.Err()
	case body := <-r.release:
		r.reader = bytes.NewReader(body)
		return r.reader.Read(p)
	}
}

func (*geminiHandlerDelayedBody) Close() error { return nil }

type geminiAccountSlotTrackingCache struct {
	fakeConcurrencyCache
	acquired chan struct{}
	released chan struct{}
	once     sync.Once
}

func (c *geminiAccountSlotTrackingCache) AcquireAccountSlot(context.Context, int64, int, string) (bool, error) {
	c.once.Do(func() { close(c.acquired) })
	return true, nil
}

func (c *geminiAccountSlotTrackingCache) ReleaseAccountSlot(context.Context, int64, string) error {
	select {
	case c.released <- struct{}{}:
	default:
	}
	return nil
}

func TestGatewayHandlerGeminiChatCompletions_RecordsPartialUsageExactlyOnceOnStreamError(t *testing.T) {
	gin.SetMode(gin.TestMode)
	groupID := int64(8801)
	group := &service.Group{
		ID: groupID, Hydrated: true, Platform: service.PlatformGemini,
		Status: service.StatusActive, RateMultiplier: 1,
	}
	account := &service.Account{
		ID: 8802, Platform: service.PlatformGemini, Type: service.AccountTypeAPIKey,
		Status: service.StatusActive, Schedulable: true, Concurrency: 1,
		Credentials:   map[string]any{"api_key": "test-key"},
		AccountGroups: []service.AccountGroup{{AccountID: 8802, GroupID: groupID}},
	}
	partial := `data: {"candidates":[{"content":{"parts":[{"text":"partial"}]}}],"usageMetadata":{"promptTokenCount":9,"cachedContentTokenCount":2,"candidatesTokenCount":4}}` + "\n\n"
	upstream := &geminiDisconnectHandlerUpstream{response: &http.Response{
		StatusCode: http.StatusOK,
		Header:     http.Header{"Content-Type": []string{"text/event-stream"}},
		Body:       io.NopCloser(io.MultiReader(bytes.NewBufferString(partial), iotest.ErrReader(errors.New("upstream reset")))),
	}}
	usageRepo := &geminiDisconnectUsageRepo{created: make(chan *service.UsageLog, 2)}
	cfg := &config.Config{RunMode: config.RunModeSimple}
	cfg.Default.RateMultiplier = 1
	schedulerSnapshot := service.NewSchedulerSnapshotService(&fakeSchedulerCache{accounts: []*service.Account{account}}, nil, nil, nil, nil)
	billingCacheService := service.NewBillingCacheService(nil, nil, nil, nil, nil, nil, cfg, nil)
	t.Cleanup(billingCacheService.Stop)
	deferredService := service.NewDeferredService(nil, nil, time.Minute)
	gatewayService := service.NewGatewayService(
		nil, &fakeGroupRepo{group: group}, usageRepo, nil, nil, nil, nil, nil, cfg,
		schedulerSnapshot, nil, service.NewBillingService(cfg, nil), nil, billingCacheService,
		nil, nil, deferredService, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil,
	)
	geminiService := service.NewGeminiMessagesCompatService(
		nil, nil, nil, nil, nil, nil, upstream, nil, cfg,
	)
	concurrencyService := service.NewConcurrencyService(&fakeConcurrencyCache{})
	h := &GatewayHandler{
		gatewayService:           gatewayService,
		geminiCompatService:      geminiService,
		billingCacheService:      billingCacheService,
		concurrencyHelper:        NewConcurrencyHelper(concurrencyService, SSEPingFormatClaude, 0),
		maxAccountSwitches:       1,
		maxAccountSwitchesGemini: 1,
		cfg:                      cfg,
	}
	apiKey := &service.APIKey{
		ID: 8803, UserID: 8804, GroupID: &groupID, Group: group, Status: service.StatusActive,
		User: &service.User{ID: 8804, Concurrency: 10, Balance: 100},
	}
	body := []byte(`{"model":"gemini-2.5-flash","stream":true,"messages":[{"role":"user","content":"hi"}]}`)
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	ctx := context.WithValue(context.Background(), ctxkey.Group, group)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", bytes.NewReader(body)).WithContext(ctx)
	c.Request.Header.Set("Content-Type", "application/json")
	c.Set(string(middleware.ContextKeyAPIKey), apiKey)
	c.Set(string(middleware.ContextKeyUser), middleware.AuthSubject{UserID: apiKey.UserID, Concurrency: 10})

	h.ChatCompletions(c)

	select {
	case log := <-usageRepo.created:
		require.Equal(t, 7, log.InputTokens)
		require.Equal(t, 4, log.OutputTokens)
	case <-time.After(time.Second):
		t.Fatal("partial Gemini usage was not recorded")
	}
	select {
	case <-usageRepo.created:
		t.Fatal("partial Gemini usage was recorded more than once")
	case <-time.After(50 * time.Millisecond):
	}
}

func TestGatewayHandlerGeminiChatCompletions_HoldsAccountSlotUntilDisconnectDrainFinishes(t *testing.T) {
	gin.SetMode(gin.TestMode)
	groupID := int64(8901)
	group := &service.Group{
		ID: groupID, Hydrated: true, Platform: service.PlatformGemini,
		Status: service.StatusActive, RateMultiplier: 1,
	}
	account := &service.Account{
		ID: 8902, Platform: service.PlatformGemini, Type: service.AccountTypeAPIKey,
		Status: service.StatusActive, Schedulable: true, Concurrency: 1,
		Credentials:   map[string]any{"api_key": "test-key"},
		AccountGroups: []service.AccountGroup{{AccountID: 8902, GroupID: groupID}},
	}
	releaseBody := make(chan []byte, 1)
	readStarted := make(chan struct{})
	upstream := &geminiDisconnectHandlerUpstream{
		dispatched: make(chan *http.Request, 1),
		responseForRequest: func(req *http.Request) *http.Response {
			return &http.Response{
				StatusCode: http.StatusOK,
				Header:     http.Header{"Content-Type": []string{"application/json"}},
				Body: &geminiHandlerDelayedBody{
					ctx: req.Context(), release: releaseBody, readStart: readStarted,
				},
			}
		},
	}
	usageRepo := &geminiDisconnectUsageRepo{created: make(chan *service.UsageLog, 2)}
	cfg := &config.Config{RunMode: config.RunModeSimple}
	cfg.Default.RateMultiplier = 1
	schedulerSnapshot := service.NewSchedulerSnapshotService(&fakeSchedulerCache{accounts: []*service.Account{account}}, nil, nil, nil, nil)
	concurrencyCache := &geminiAccountSlotTrackingCache{
		acquired: make(chan struct{}),
		released: make(chan struct{}, 1),
	}
	concurrencyService := service.NewConcurrencyService(concurrencyCache)
	billingCacheService := service.NewBillingCacheService(nil, nil, nil, nil, nil, nil, cfg, nil)
	t.Cleanup(billingCacheService.Stop)
	gatewayService := service.NewGatewayService(
		nil, &fakeGroupRepo{group: group}, usageRepo, nil, nil, nil, nil, nil, cfg,
		schedulerSnapshot, concurrencyService, service.NewBillingService(cfg, nil), nil, billingCacheService,
		nil, nil, service.NewDeferredService(nil, nil, time.Minute), nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil,
	)
	geminiService := service.NewGeminiMessagesCompatService(nil, nil, nil, nil, nil, nil, upstream, nil, cfg)
	h := &GatewayHandler{
		gatewayService:           gatewayService,
		geminiCompatService:      geminiService,
		billingCacheService:      billingCacheService,
		concurrencyHelper:        NewConcurrencyHelper(concurrencyService, SSEPingFormatClaude, 0),
		maxAccountSwitches:       1,
		maxAccountSwitchesGemini: 1,
		cfg:                      cfg,
	}
	apiKey := &service.APIKey{
		ID: 8903, UserID: 8904, GroupID: &groupID, Group: group, Status: service.StatusActive,
		User: &service.User{ID: 8904, Concurrency: 10, Balance: 100},
	}
	body := []byte(`{"model":"gemini-2.5-flash","stream":false,"messages":[{"role":"user","content":"hi"}]}`)
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	logCore, observedLogs := observer.New(zap.InfoLevel)
	requestCtx := logger.IntoContext(context.WithValue(context.Background(), ctxkey.Group, group), zap.New(logCore))
	requestCtx, cancel := context.WithCancel(requestCtx)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", bytes.NewReader(body)).WithContext(requestCtx)
	c.Request.Header.Set("Content-Type", "application/json")
	c.Set(string(middleware.ContextKeyAPIKey), apiKey)
	c.Set(string(middleware.ContextKeyUser), middleware.AuthSubject{UserID: apiKey.UserID, Concurrency: 10})

	handlerDone := make(chan struct{})
	go func() {
		h.ChatCompletions(c)
		close(handlerDone)
	}()
	select {
	case <-readStarted:
	case <-time.After(time.Second):
		t.Fatal("Gemini response body was not read")
	}
	cancel()
	select {
	case <-concurrencyCache.released:
		t.Fatal("account slot was released before the accepted Gemini attempt finished draining")
	case <-time.After(50 * time.Millisecond):
	}
	releaseBody <- []byte(`{"candidates":[{"content":{"parts":[{"text":"done"}]}}],"usageMetadata":{"promptTokenCount":6,"candidatesTokenCount":4}}`)

	select {
	case <-handlerDone:
	case <-time.After(time.Second):
		t.Fatal("handler did not finish after Gemini usage arrived")
	}
	select {
	case <-concurrencyCache.released:
	case <-time.After(time.Second):
		t.Fatal("account slot was not released after Gemini drain finished")
	}
	select {
	case log := <-usageRepo.created:
		require.Equal(t, 6, log.InputTokens)
		require.Equal(t, 4, log.OutputTokens)
	case <-time.After(time.Second):
		t.Fatal("drained Gemini usage was not recorded")
	}
	require.Empty(t, rec.Body.String())
	drainStartLogs := observedLogs.FilterMessage("gemini.chat_completions_disconnect_drain_started").All()
	require.Len(t, drainStartLogs, 1)
	require.Equal(t, int64(8902), drainStartLogs[0].ContextMap()["account_id"])
	drainFinishedLogs := observedLogs.FilterMessage("gateway.cc.gemini_disconnect_drain_finished").All()
	require.Len(t, drainFinishedLogs, 1)
	require.Equal(t, "terminal_usage", drainFinishedLogs[0].ContextMap()["outcome"])
	usageRecordedLogs := observedLogs.FilterMessage("gateway.cc.gemini_disconnect_usage_recorded").All()
	require.Len(t, usageRecordedLogs, 1)
	require.Equal(t, int64(6), usageRecordedLogs[0].ContextMap()["input_tokens"])
	require.Equal(t, int64(4), usageRecordedLogs[0].ContextMap()["output_tokens"])
	require.Equal(t, false, usageRecordedLogs[0].ContextMap()["partial_usage"])
}

func TestGatewayHandlerGeminiNative_HoldsSlotRecordsUsageAndLogsDisconnectDrain(t *testing.T) {
	gin.SetMode(gin.TestMode)
	groupID := int64(9901)
	group := &service.Group{
		ID: groupID, Hydrated: true, Platform: service.PlatformGemini,
		Status: service.StatusActive, RateMultiplier: 1,
	}
	account := &service.Account{
		ID: 9902, Platform: service.PlatformGemini, Type: service.AccountTypeAPIKey,
		Status: service.StatusActive, Schedulable: true, Concurrency: 1,
		Credentials:   map[string]any{"api_key": "test-key"},
		AccountGroups: []service.AccountGroup{{AccountID: 9902, GroupID: groupID}},
	}
	releaseBody := make(chan []byte, 1)
	readStarted := make(chan struct{})
	upstream := &geminiDisconnectHandlerUpstream{
		dispatched: make(chan *http.Request, 1),
		responseForRequest: func(req *http.Request) *http.Response {
			return &http.Response{
				StatusCode: http.StatusOK,
				Header:     http.Header{"Content-Type": []string{"application/json"}, "X-Request-Id": []string{"native-upstream-1"}},
				Body: &geminiHandlerDelayedBody{
					ctx: req.Context(), release: releaseBody, readStart: readStarted,
				},
			}
		},
	}
	usageRepo := &geminiDisconnectUsageRepo{created: make(chan *service.UsageLog, 2)}
	cfg := &config.Config{RunMode: config.RunModeSimple}
	cfg.Default.RateMultiplier = 1
	schedulerSnapshot := service.NewSchedulerSnapshotService(&fakeSchedulerCache{accounts: []*service.Account{account}}, nil, nil, nil, nil)
	concurrencyCache := &geminiAccountSlotTrackingCache{
		acquired: make(chan struct{}),
		released: make(chan struct{}, 1),
	}
	concurrencyService := service.NewConcurrencyService(concurrencyCache)
	billingCacheService := service.NewBillingCacheService(nil, nil, nil, nil, nil, nil, cfg, nil)
	t.Cleanup(billingCacheService.Stop)
	gatewayService := service.NewGatewayService(
		nil, &fakeGroupRepo{group: group}, usageRepo, nil, nil, nil, nil, nil, cfg,
		schedulerSnapshot, concurrencyService, service.NewBillingService(cfg, nil), nil, billingCacheService,
		nil, nil, service.NewDeferredService(nil, nil, time.Minute), nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil,
	)
	geminiService := service.NewGeminiMessagesCompatService(nil, nil, nil, nil, nil, nil, upstream, nil, cfg)
	h := &GatewayHandler{
		gatewayService:           gatewayService,
		geminiCompatService:      geminiService,
		billingCacheService:      billingCacheService,
		concurrencyHelper:        NewConcurrencyHelper(concurrencyService, SSEPingFormatClaude, 0),
		maxAccountSwitches:       1,
		maxAccountSwitchesGemini: 1,
		cfg:                      cfg,
	}
	apiKey := &service.APIKey{
		ID: 9903, UserID: 9904, GroupID: &groupID, Group: group, Status: service.StatusActive,
		User: &service.User{ID: 9904, Concurrency: 10, Balance: 100},
	}
	body := []byte(`{"contents":[]}`)
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Params = gin.Params{{Key: "modelAction", Value: "/gemini-2.5-flash:generateContent"}}
	logCore, observedLogs := observer.New(zap.InfoLevel)
	requestCtx := logger.IntoContext(context.WithValue(context.Background(), ctxkey.Group, group), zap.New(logCore))
	requestCtx, cancel := context.WithCancel(requestCtx)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1beta/models/gemini-2.5-flash:generateContent", bytes.NewReader(body)).WithContext(requestCtx)
	c.Request.Header.Set("Content-Type", "application/json")
	c.Set(string(middleware.ContextKeyAPIKey), apiKey)
	c.Set(string(middleware.ContextKeyUser), middleware.AuthSubject{UserID: apiKey.UserID, Concurrency: 10})

	handlerDone := make(chan struct{})
	go func() {
		h.GeminiV1BetaModels(c)
		close(handlerDone)
	}()
	select {
	case <-readStarted:
	case <-time.After(time.Second):
		t.Fatal("Gemini native response body was not read")
	}
	cancel()
	select {
	case <-concurrencyCache.released:
		t.Fatal("account slot was released before native Gemini drain finished")
	case <-time.After(50 * time.Millisecond):
	}
	releaseBody <- []byte(`{"candidates":[{"content":{"parts":[{"text":"done"}]}}],"usageMetadata":{"promptTokenCount":6,"candidatesTokenCount":4}}`)

	select {
	case <-handlerDone:
	case <-time.After(time.Second):
		t.Fatal("native Gemini handler did not finish after usage arrived")
	}
	select {
	case <-concurrencyCache.released:
	case <-time.After(time.Second):
		t.Fatal("account slot was not released after native Gemini drain finished")
	}
	select {
	case log := <-usageRepo.created:
		require.Equal(t, 6, log.InputTokens)
		require.Equal(t, 4, log.OutputTokens)
	case <-time.After(time.Second):
		t.Fatal("drained native Gemini usage was not recorded")
	}
	select {
	case <-usageRepo.created:
		t.Fatal("native Gemini usage was recorded more than once")
	case <-time.After(50 * time.Millisecond):
	}
	require.Empty(t, rec.Body.String())
	drainStartLogs := observedLogs.FilterMessage("gemini.native_disconnect_drain_started").All()
	require.Len(t, drainStartLogs, 1)
	require.Equal(t, int64(9902), drainStartLogs[0].ContextMap()["account_id"])
	drainFinishedLogs := observedLogs.FilterMessage("gemini.native_disconnect_drain_finished").All()
	require.Len(t, drainFinishedLogs, 1)
	require.Equal(t, "terminal_usage", drainFinishedLogs[0].ContextMap()["outcome"])
	usageRecordedLogs := observedLogs.FilterMessage("gemini.native_disconnect_usage_recorded").All()
	require.Len(t, usageRecordedLogs, 1)
	require.Equal(t, int64(6), usageRecordedLogs[0].ContextMap()["input_tokens"])
	require.Equal(t, int64(4), usageRecordedLogs[0].ContextMap()["output_tokens"])
	require.Equal(t, false, usageRecordedLogs[0].ContextMap()["partial_usage"])
}
