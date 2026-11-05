package handler

import (
	"bytes"
	"compress/gzip"
	"compress/zlib"
	"context"
	"go/ast"
	"go/parser"
	"go/token"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/pkg/traffic"
	"github.com/Wei-Shaw/sub2api/internal/server/middleware"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/andybalholm/brotli"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestUsageRecordTraffic_AfterCompressorClose(t *testing.T) {
	for _, encoding := range []string{"identity", "gzip", "deflate", "br"} {
		for _, stream := range []bool{false, true} {
			for _, withPool := range []bool{false, true} {
				name := encoding
				if stream {
					name += "/sse"
				}
				if withPool {
					name += "/async"
				} else {
					name += "/sync_fallback"
				}
				t.Run(name, func(t *testing.T) {
					var pool *service.UsageRecordWorkerPool
					if withPool {
						pool = newUsageRecordTestPool(t)
					}
					h := &GatewayHandler{usageRecordWorkerPool: pool}
					payload := strings.Repeat("data: {\"message\":\"hello traffic\"}\n\n", 20)
					seen := make(chan *traffic.Usage, 1)
					registered := make(chan struct{})
					requestCtx, cancel := context.WithCancel(context.Background())
					defer cancel()
					router := gin.New()
					router.Use(middleware.TrafficCapture(), middleware.ResponseCompress())
					router.GET("/usage", func(c *gin.Context) {
						if stream {
							c.Header("Content-Type", "text/event-stream")
						} else {
							c.Header("Content-Type", "application/json")
						}
						_, err := c.Writer.WriteString(payload)
						require.NoError(t, err)
						if stream {
							c.Writer.Flush()
						}
						h.submitUsageRecordTask(c.Request.Context(), func(ctx context.Context) {
							traffic.AfterFinalize(ctx, func(snapshot *traffic.Usage) { seen <- snapshot })
							close(registered)
						})
						select {
						case <-registered:
						case <-time.After(5 * time.Second):
							t.Fatal("usage task blocked waiting for the handler to finalize")
						}
						select {
						case <-seen:
							t.Fatal("traffic escaped before response compressor Close")
						default:
						}
						cancel() // client cancellation must not discard the completion
					})
					recorder := httptest.NewRecorder()
					request := httptest.NewRequest(http.MethodGet, "/usage", nil).WithContext(requestCtx)
					request.Header.Set("Accept-Encoding", encoding)
					router.ServeHTTP(recorder, request)
					var captured *traffic.Usage
					select {
					case captured = <-seen:
					case <-time.After(5 * time.Second):
						t.Fatal("missing finalized traffic")
					}
					require.NotNil(t, captured)
					require.NotNil(t, captured.Downstream.Response)
					response := captured.Downstream.Response
					require.EqualValues(t, len(payload), *response.OriginalBytes)
					require.EqualValues(t, recorder.Body.Len(), *response.ActualBytes, "Close trailer must be included")
					require.Equal(t, encoding != "identity", response.Compressed)
					var reader io.Reader = bytes.NewReader(recorder.Body.Bytes())
					switch encoding {
					case "gzip":
						r, err := gzip.NewReader(reader)
						require.NoError(t, err)
						defer func() { require.NoError(t, r.Close()) }()
						reader = r
					case "deflate":
						r, err := zlib.NewReader(reader)
						require.NoError(t, err)
						defer func() { require.NoError(t, r.Close()) }()
						reader = r
					case "br":
						reader = brotli.NewReader(reader)
					}
					decoded, err := io.ReadAll(reader)
					require.NoError(t, err, "compressed stream must include a valid trailer")
					require.Equal(t, payload, string(decoded))
				})
			}
		}
	}
}

func TestUsageRecordTraffic_OpenAIWorkerRetainsCollectorNotGinContext(t *testing.T) {
	pool := newUsageRecordTestPool(t)
	h := &OpenAIGatewayHandler{usageRecordWorkerPool: pool}
	block := make(chan struct{})
	pool.Submit(func(context.Context) { <-block })
	collector := traffic.NewCollector()
	requestCtx, cancel := context.WithCancel(traffic.WithCollector(context.Background(), collector))
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", nil).WithContext(requestCtx)
	seen := make(chan *traffic.Collector, 1)
	h.submitOpenAIUsageRecordTask(c.Request.Context(), &service.OpenAIForwardResult{}, func(ctx context.Context) {
		seen <- traffic.FromContext(ctx)
	})
	// Simulate the pooled gin.Context being reused before the queued task starts.
	c.Request = httptest.NewRequest(http.MethodPost, "/other-request", nil)
	cancel()
	close(block)
	select {
	case captured := <-seen:
		require.Same(t, collector, captured)
	case <-time.After(5 * time.Second):
		t.Fatal("usage task not executed")
	}
}

func TestUsageRecordTraffic_AsyncImageDoesNotReuseSubmissionCollector(t *testing.T) {
	type requestValueKey struct{}
	allowJob := make(chan struct{})
	type jobResult struct {
		collector *traffic.Collector
		value     any
		err       error
	}
	jobDone := make(chan jobResult, 1)
	submissionTraffic := make(chan *traffic.Usage, 1)
	var stopJob context.CancelFunc
	router := gin.New()
	router.Use(middleware.TrafficCapture(), middleware.ResponseCompress())
	router.POST("/images/async", func(c *gin.Context) {
		traffic.AfterFinalize(c.Request.Context(), func(snapshot *traffic.Usage) { submissionTraffic <- snapshot })
		job, _, cancel := newAsyncImageContext(c, []byte(`{"prompt":"image"}`), time.Minute)
		stopJob = cancel
		go func() {
			<-allowJob // deterministically start only after the submission's 202 finalized
			collector := traffic.FromContext(job.Request.Context())
			counter := collector.Begin(traffic.UpstreamRequest, false)
			counter.AddActual(999)
			counter.CompleteActual()
			job.Writer.WriteHeader(http.StatusOK)
			_, err := job.Writer.WriteString(`{"data":[{"url":"image"}]}`)
			jobDone <- jobResult{collector: collector, value: job.Request.Context().Value(requestValueKey{}), err: err}
		}()
		c.Data(http.StatusAccepted, "application/json", []byte(`{"task_id":"one"}`))
	})
	requestCtx, cancelRequest := context.WithCancel(context.WithValue(context.Background(), requestValueKey{}, "preserved"))
	request := httptest.NewRequest(http.MethodPost, "/images/async", nil).WithContext(requestCtx)
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, request)
	defer stopJob()
	cancelRequest()
	frozen := <-submissionTraffic
	require.Equal(t, http.StatusAccepted, recorder.Code)
	require.EqualValues(t, recorder.Body.Len(), *frozen.Downstream.Response.ActualBytes)
	require.Nil(t, frozen.Upstream)
	close(allowJob)
	select {
	case result := <-jobDone:
		require.Nil(t, result.collector)
		require.Equal(t, "preserved", result.value)
		require.NoError(t, result.err)
	case <-time.After(5 * time.Second):
		t.Fatal("background image job did not finish")
	}
	require.Nil(t, frozen.Upstream, "background job must not pollute the 202 ledger")
}

type trafficHandlerUsageRepo struct {
	service.UsageLogRepository
	create func(context.Context, *service.UsageLog) (bool, error)
}

func (r *trafficHandlerUsageRepo) Create(ctx context.Context, log *service.UsageLog) (bool, error) {
	return r.create(ctx, log)
}

func TestUsageRecordTraffic_HealthyPoolServeHTTPDoesNotWaitForDatabase(t *testing.T) {
	pool := newUsageRecordTestPool(t)
	started, release := make(chan struct{}), make(chan struct{})
	releaseWriter := sync.OnceFunc(func() { close(release) })
	defer releaseWriter()
	written := make(chan *service.UsageLog, 1)
	repo := &trafficHandlerUsageRepo{create: func(ctx context.Context, log *service.UsageLog) (bool, error) {
		close(started)
		<-release
		if err := ctx.Err(); err != nil {
			return false, err
		}
		written <- log
		return true, nil
	}}
	cfg := &config.Config{RunMode: config.RunModeSimple}
	svc := service.NewGatewayService(
		nil, nil, repo, nil, nil, nil, nil, nil, cfg,
		nil, nil, service.NewBillingService(cfg, nil), nil, &service.BillingCacheService{},
		nil, nil, &service.DeferredService{}, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil,
	)
	h := &GatewayHandler{gatewayService: svc, usageRecordWorkerPool: pool}
	taskErr := make(chan error, 1)
	requestCtx, cancel := context.WithCancel(context.Background())
	defer cancel()
	router := gin.New()
	router.Use(middleware.TrafficCapture(), middleware.ResponseCompress())
	router.GET("/usage", func(c *gin.Context) {
		c.Data(http.StatusOK, "application/json", []byte(`{"result":"ok"}`))
		registered := make(chan struct{})
		h.submitUsageRecordTask(c.Request.Context(), func(ctx context.Context) {
			taskErr <- svc.RecordUsage(ctx, &service.RecordUsageInput{
				Result: &service.ForwardResult{RequestID: "http-persistence", Model: "claude-sonnet-4", Usage: service.ClaudeUsage{InputTokens: 2}},
				APIKey: &service.APIKey{ID: 1}, User: &service.User{ID: 2}, Account: &service.Account{ID: 3},
			})
			close(registered)
		})
		<-registered // the billing task definitely finishes before compressor Close
		cancel()
	})
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/usage", nil).WithContext(requestCtx)
	request.Header.Set("Accept-Encoding", "gzip")
	returned := make(chan struct{})
	go func() { router.ServeHTTP(recorder, request); close(returned) }()
	select {
	case <-returned:
	case <-time.After(time.Second):
		t.Fatal("healthy-pool response waited for a blocked database write")
	}
	require.NoError(t, <-taskErr)
	select {
	case <-started:
	case <-time.After(time.Second):
		t.Fatal("database task not dispatched")
	}
	releaseWriter()
	select {
	case log := <-written:
		require.EqualValues(t, recorder.Body.Len(), *log.Traffic.Downstream.Response.ActualBytes)
	case <-time.After(time.Second):
		t.Fatal("finalized usage log was lost")
	}
}

func TestUsageRecordTraffic_AllSubmitPathsCarryPersistencePool(t *testing.T) {
	pool := newUsageRecordTestPool(t)
	gateway := &GatewayHandler{usageRecordWorkerPool: pool}
	openAI := &OpenAIGatewayHandler{usageRecordWorkerPool: pool}
	for name, submit := range map[string]func(context.Context, service.UsageRecordTask){
		"gateway":           gateway.submitUsageRecordTask,
		"gateway_mandatory": gateway.submitMandatoryUsageRecordTask,
		"openai":            openAI.submitUsageRecordTask,
		"openai_mandatory":  openAI.submitMandatoryUsageRecordTask,
	} {
		t.Run(name, func(t *testing.T) {
			seen := make(chan *service.UsageRecordWorkerPool, 1)
			submit(context.Background(), func(ctx context.Context) { seen <- service.UsageRecordWorkerPoolFromContext(ctx) })
			select {
			case actual := <-seen:
				require.Same(t, pool, actual)
			case <-time.After(time.Second):
				t.Fatal("usage task not executed")
			}
		})
	}
}

// Guard all production usage submission sites, including newly added provider
// routes, against retaining the pooled request context in the asynchronous task.
func TestUsageRecordTraffic_TaskClosuresDoNotCaptureGin(t *testing.T) {
	_, filename, _, ok := runtime.Caller(0)
	require.True(t, ok)
	dir := filepath.Dir(filename)
	entries, err := os.ReadDir(dir)
	require.NoError(t, err)
	count := 0
	for _, entry := range entries {
		if !strings.HasSuffix(entry.Name(), ".go") || strings.HasSuffix(entry.Name(), "_test.go") {
			continue
		}
		fset := token.NewFileSet()
		file, err := parser.ParseFile(fset, filepath.Join(dir, entry.Name()), nil, 0)
		require.NoError(t, err)
		ast.Inspect(file, func(node ast.Node) bool {
			call, ok := node.(*ast.CallExpr)
			if !ok {
				return true
			}
			selector, ok := call.Fun.(*ast.SelectorExpr)
			if !ok || !strings.HasPrefix(selector.Sel.Name, "submit") || !strings.HasSuffix(selector.Sel.Name, "UsageRecordTask") {
				return true
			}
			for _, arg := range call.Args {
				task, ok := arg.(*ast.FuncLit)
				if !ok {
					continue
				}
				count++
				ast.Inspect(task.Body, func(n ast.Node) bool {
					if ident, ok := n.(*ast.Ident); ok && ident.Name == "c" {
						t.Errorf("%s: usage task captures gin.Context; snapshot inputs before submitting", fset.Position(ident.Pos()))
					}
					return true
				})
			}
			return true
		})
	}
	require.GreaterOrEqual(t, count, 17, "guard must inspect every provider's usage submission")
}
