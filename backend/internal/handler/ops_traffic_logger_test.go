package handler

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/ctxkey"
	"github.com/Wei-Shaw/sub2api/internal/pkg/traffic"
	"github.com/Wei-Shaw/sub2api/internal/server/middleware"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

type trafficHandlerRepo struct {
	service.OpsRepository
	mu     sync.Mutex
	events []*service.OpsTrafficEvent
}

func (r *trafficHandlerRepo) BatchInsertTrafficEvents(_ context.Context, events []*service.OpsTrafficEvent) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.events = append(r.events, events...)
	return nil
}
func newTrafficHandlerService(t *testing.T) (*service.OpsService, *trafficHandlerRepo) {
	t.Helper()
	repo := &trafficHandlerRepo{}
	ops := service.NewOpsService(repo, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil)
	t.Cleanup(ops.StopTrafficLedger)
	return ops, repo
}

type blockedTrafficHandlerRepo struct {
	service.OpsRepository
	started chan struct{}
	release chan struct{}
	once    sync.Once
}

func (r *blockedTrafficHandlerRepo) BatchInsertTrafficEvents(ctx context.Context, _ []*service.OpsTrafficEvent) error {
	r.once.Do(func() { close(r.started) })
	select {
	case <-r.release:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

func TestOpsTrafficLedgerSaturationDoesNotDelaySmallHTTPResponse(t *testing.T) {
	gin.SetMode(gin.TestMode)
	repo := &blockedTrafficHandlerRepo{started: make(chan struct{}), release: make(chan struct{})}
	ops := service.NewOpsService(repo, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil)
	var releaseOnce sync.Once
	release := func() { releaseOnce.Do(func() { close(repo.release) }) }
	t.Cleanup(func() { release(); ops.StopTrafficLedger() })
	event := &service.OpsTrafficEvent{Traffic: &traffic.Usage{}}
	require.NoError(t, ops.RecordTrafficEvent(context.Background(), event))
	select {
	case <-repo.started:
	case <-time.After(time.Second):
		t.Fatal("ledger writer did not enter blocked write")
	}
	// The sole writer is blocked with the first event. Fill every queue slot.
	for i := 0; i < 1024; i++ {
		require.NoError(t, ops.RecordTrafficEvent(context.Background(), event))
	}
	require.Equal(t, int64(1025), ops.GetTrafficLedgerHealth().Pending)
	router := gin.New()
	router.Use(middleware.TrafficCapture(), OpsErrorLoggerMiddleware(ops))
	router.GET("/v1/messages", func(c *gin.Context) { c.String(http.StatusOK, "ok") })
	server := httptest.NewServer(router)
	t.Cleanup(func() { release(); server.Close() })
	client := server.Client()
	client.Timeout = 200 * time.Millisecond
	started := time.Now()
	response, err := client.Get(server.URL + "/v1/messages")
	require.NoError(t, err, "HTTP completion must not wait for telemetry queue capacity")
	defer func() { require.NoError(t, response.Body.Close()) }()
	body, err := io.ReadAll(response.Body)
	require.NoError(t, err)
	require.Less(t, time.Since(started), 200*time.Millisecond)
	require.Equal(t, http.StatusOK, response.StatusCode)
	require.Equal(t, "ok", string(body))
	health := ops.GetTrafficLedgerHealth()
	require.Equal(t, int64(1), health.Failed)
	require.Equal(t, int64(1025), health.Pending)
	release()
	ops.StopTrafficLedger()
	health = ops.GetTrafficLedgerHealth()
	require.Equal(t, int64(1), health.Failed)
	require.Zero(t, health.Pending)
	require.Equal(t, int64(1025), health.Flushed)
}

func TestOpsTrafficLedgerHTTPOutcomesIndependentOfUsageAndErrorFilters(t *testing.T) {
	setupOpsErrorLogTestQueue(t, 16)
	gin.SetMode(gin.TestMode)
	for _, name := range []string{"success-no-usage", "auth-no-usage", "partial-with-usage-and-error", "retry", "body-limit"} {
		t.Run(name, func(t *testing.T) {
			ops, repo := newTrafficHandlerService(t)
			router := gin.New()
			router.Use(middleware.TrafficCapture(), middleware.RequestBodyLimit(4), OpsErrorLoggerMiddleware(ops))
			router.POST("/v1/messages", func(c *gin.Context) {
				switch name {
				case "auth-no-usage":
					middleware.MarkIngressRejected(c, middleware.IngressRejectInvalidAPIKey)
					c.AbortWithStatusJSON(401, gin.H{"error": "invalid_api_key"})
				case "partial-with-usage-and-error":
					c.Set(opsDedicatedErrorRecordedKey, true)
					c.Header("Content-Type", "text/event-stream")
					_, _ = c.Writer.WriteString("data: partial\n\n")
					c.Writer.Flush()
					_, _ = c.Writer.WriteString("event: error\ndata: {\"error\":{\"message\":\"failed\"}}\n\n")
				case "retry":
					collector := traffic.FromContext(c.Request.Context())
					for _, n := range []int{10, 20, 30} {
						counter := collector.Begin(traffic.UpstreamRequest, false)
						counter.AddOriginal(n)
						counter.AddActual(n)
						counter.CompleteOriginal()
						counter.CompleteActual()
					}
					c.String(200, "done")
				case "body-limit":
					_, err := io.ReadAll(c.Request.Body)
					require.Error(t, err)
					c.AbortWithStatus(413)
				default:
					c.String(200, "ok")
				}
			})
			request := httptest.NewRequest(http.MethodPost, "/v1/messages", strings.NewReader("oversized"))
			router.ServeHTTP(httptest.NewRecorder(), request)
			ops.StopTrafficLedger()
			require.Len(t, repo.events, 1)
			event := repo.events[0]
			require.NotNil(t, event.Traffic)
			require.Nil(t, event.GroupID)
			require.Nil(t, event.AccountID)
			require.Empty(t, event.Platform, "no resolved route must not be guessed from endpoint")
			if name == "retry" {
				require.Equal(t, int64(60), *event.Traffic.Upstream.Request.ActualBytes)
			}
		})
	}
}

func TestOpsTrafficLedgerFreezesGinMetadataBeforeFinalizeAndCountsOnce(t *testing.T) {
	ops, repo := newTrafficHandlerService(t)
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	collector := traffic.NewCollector()
	ctx, cancel := context.WithCancel(traffic.WithCollector(context.Background(), collector))
	ctx = context.WithValue(ctx, ctxkey.RequestID, "request-one")
	ctx = service.WithResolvedTargetPlatform(ctx, service.PlatformOpenAI)
	c.Request = httptest.NewRequest(http.MethodGet, "/v1/messages", nil).WithContext(ctx)
	group := int64(7)
	key := &service.APIKey{GroupID: &group, Group: &service.Group{Platform: service.PlatformComposite}}
	c.Set(string(middleware.ContextKeyAPIKey), key)
	c.Set(opsAccountIDKey, int64(9))
	registerOpsTrafficEvent(c, ops)
	registerOpsTrafficEvent(c, ops)
	// Simulate gin pool reuse and pointer mutation before the outer compressor
	// finalizes, then finalize after a cancelled client context.
	group = 999
	key.Group.Platform = "other"
	c.Keys = map[string]any{}
	c.Request = httptest.NewRequest(http.MethodGet, "/different", nil)
	cancel()
	collector.Finalize()
	collector.Finalize()
	ops.StopTrafficLedger()
	require.Len(t, repo.events, 1)
	event := repo.events[0]
	require.Equal(t, "request-one", event.RequestID)
	require.Equal(t, int64(7), *event.GroupID)
	require.Equal(t, int64(9), *event.AccountID)
	require.Equal(t, service.PlatformOpenAI, event.Platform)
}

func TestOpsTrafficLedgerWSAndNoCollectorDoNotFabricateHTTP(t *testing.T) {
	for _, websocket := range []bool{false, true} {
		ops, repo := newTrafficHandlerService(t)
		r := gin.New()
		if websocket {
			r.Use(middleware.TrafficCapture())
		}
		r.Use(OpsErrorLoggerMiddleware(ops))
		r.GET("/responses", func(c *gin.Context) {
			if websocket {
				// Simulate successful transport hijack, not an untrusted header.
				traffic.FromContext(c.Request.Context()).Exclude()
			}
			c.Status(200)
		})
		req := httptest.NewRequest(http.MethodGet, "/responses", nil)
		if websocket {
			req.Header.Set("Upgrade", "websocket")
		}
		r.ServeHTTP(httptest.NewRecorder(), req)
		ops.StopTrafficLedger()
		require.Empty(t, repo.events)
	}
}

func TestOpsTrafficLedgerFakeUpgradeAndFailedHandshakeAreHTTP(t *testing.T) {
	setupOpsErrorLogTestQueue(t, 8)
	for _, status := range []int{http.StatusOK, http.StatusBadRequest} {
		ops, repo := newTrafficHandlerService(t)
		r := gin.New()
		r.Use(middleware.TrafficCapture(), OpsErrorLoggerMiddleware(ops))
		r.GET("/responses", func(c *gin.Context) { c.String(status, "HTTP response") })
		req := httptest.NewRequest(http.MethodGet, "/responses", nil)
		req.Header.Set("Upgrade", "websocket")
		r.ServeHTTP(httptest.NewRecorder(), req)
		ops.StopTrafficLedger()
		require.Len(t, repo.events, 1)
	}
}

func TestOpsTrafficLedgerUsesFrozenCompletionForLateCallback(t *testing.T) {
	ops, repo := newTrafficHandlerService(t)
	collector := traffic.NewCollector()
	collector.Finalize()
	completed := collector.FinalizedAt()
	require.False(t, completed.IsZero())
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest(http.MethodGet, "/v1/messages", nil).WithContext(traffic.WithCollector(context.Background(), collector))
	beforeRegistration := time.Now()
	registerOpsTrafficEvent(c, ops)
	ops.StopTrafficLedger()
	require.Len(t, repo.events, 1)
	require.Equal(t, completed, repo.events[0].CreatedAt)
	require.False(t, repo.events[0].CreatedAt.After(beforeRegistration))
}

func TestOpsTrafficLedgerPanicBeforeWriteAndAfterSSE(t *testing.T) {
	gin.SetMode(gin.TestMode)
	for _, partial := range []bool{false, true} {
		ops, repo := newTrafficHandlerService(t)
		r := gin.New()
		r.Use(gin.CustomRecovery(func(c *gin.Context, _ any) { c.AbortWithStatus(500) }), middleware.TrafficCapture(), OpsErrorLoggerMiddleware(ops))
		r.GET("/v1/messages", func(c *gin.Context) {
			if partial {
				c.Header("Content-Type", "text/event-stream")
				c.String(200, "data: partial\n\n")
				c.Writer.Flush()
			}
			panic("test panic")
		})
		r.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "/v1/messages", nil))
		ops.StopTrafficLedger()
		require.Len(t, repo.events, 1)
		require.Nil(t, repo.events[0].Traffic.Downstream.Response.ActualBytes, "outer recovery writes after measurement; unknown, not a fake complete zero")
	}
}
