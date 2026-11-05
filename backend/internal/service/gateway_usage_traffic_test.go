//go:build unit

package service

import (
	"context"
	"errors"
	"io"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/traffic"
	"github.com/stretchr/testify/require"
)

func completeUsageTrafficBody(c *traffic.Collector, direction traffic.Direction, original, actual int, compressed bool) {
	counter := c.Begin(direction, compressed)
	counter.AddOriginal(original)
	counter.AddActual(actual)
	counter.CompleteOriginal()
	counter.CompleteActual()
}

func TestWriteUsageTraffic_ConsumedResponseSurvivesFinalize(t *testing.T) {
	for _, compressed := range []bool{false, true} {
		for _, alreadyFinalized := range []bool{false, true} {
			collector := traffic.NewCollector()
			ctx := traffic.WithCollector(context.Background(), collector)
			counter := collector.Begin(traffic.UpstreamResponse, compressed)
			reader := strings.NewReader("received-prefix-unread-tail")
			body := traffic.ObserveResponseBody(io.NopCloser(reader), counter, !compressed, -1)
			repo := &openAIRecordUsageLogRepoStub{}
			log := &UsageLog{RequestID: "sse-consumed-prefix"}
			if !alreadyFinalized {
				writeUsageLogBestEffort(ctx, repo, log, "test")
				require.Zero(t, repo.calls)
			}
			consumed := len("received-prefix")
			_, err := io.ReadFull(body, make([]byte, consumed))
			require.NoError(t, err)
			require.NoError(t, body.Close())
			require.Equal(t, len("-unread-tail"), reader.Len(), "accounting must not drain for persistence")
			collector.Finalize()
			if alreadyFinalized {
				writeUsageLogBestEffort(ctx, repo, log, "test")
			}
			require.Equal(t, 1, repo.calls)
			measured := repo.lastLog.Traffic.Upstream.Response
			require.NotNil(t, measured.ActualBytes)
			require.EqualValues(t, consumed, *measured.ActualBytes)
			require.Equal(t, compressed, measured.Compressed)
			if compressed {
				require.Nil(t, measured.OriginalBytes)
				require.Nil(t, measured.Ratio())
			} else {
				require.NotNil(t, measured.OriginalBytes)
				require.EqualValues(t, consumed, *measured.OriginalBytes)
			}
		}
	}
}

func TestRecordUsageTraffic_BillingCompletesBeforeFinalize(t *testing.T) {
	for _, openAI := range []bool{false, true} {
		for _, billingFails := range []bool{false, true} {
			name := "claude"
			if openAI {
				name = "openai"
			}
			if billingFails {
				name += "/billing_failure"
			}
			t.Run(name, func(t *testing.T) {
				usageRepo := &openAIRecordUsageLogRepoStub{}
				billingRepo := &openAIRecordUsageBillingRepoStub{result: &UsageBillingApplyResult{Applied: true}}
				if billingFails {
					billingRepo.err = errors.New("billing failed")
				}
				collector := traffic.NewCollector()
				completeUsageTrafficBody(collector, traffic.UpstreamRequest, 100, 60, true) // failed attempt
				completeUsageTrafficBody(collector, traffic.UpstreamResponse, 40, 20, true)
				completeUsageTrafficBody(collector, traffic.UpstreamRequest, 120, 70, true) // successful retry
				completeUsageTrafficBody(collector, traffic.UpstreamResponse, 250, 90, true)
				response := collector.Begin(traffic.DownstreamResponse, true)
				response.AddOriginal(200)
				response.AddActual(35)
				ctx, cancel := context.WithCancel(traffic.WithCollector(context.Background(), collector))
				cancel() // client disconnect must not cancel billing or finalized persistence

				done := make(chan error, 1)
				go func() {
					if openAI {
						svc := newOpenAIRecordUsageServiceWithBillingRepoForTest(usageRepo, billingRepo, &openAIRecordUsageUserRepoStub{}, &openAIRecordUsageSubRepoStub{}, nil)
						done <- svc.RecordUsage(ctx, &OpenAIRecordUsageInput{
							Result: &OpenAIForwardResult{RequestID: "traffic-openai", Model: "gpt-4o", Usage: OpenAIUsage{InputTokens: 20, OutputTokens: 10}},
							APIKey: &APIKey{ID: 1}, User: &User{ID: 2}, Account: &Account{ID: 3},
						})
					} else {
						svc := newGatewayRecordUsageServiceWithBillingRepoForTest(usageRepo, billingRepo, &openAIRecordUsageUserRepoStub{}, &openAIRecordUsageSubRepoStub{})
						done <- svc.RecordUsage(ctx, &RecordUsageInput{
							Result: &ForwardResult{RequestID: "traffic-claude", Model: "claude-sonnet-4", Usage: ClaudeUsage{InputTokens: 20, OutputTokens: 10}},
							APIKey: &APIKey{ID: 1}, User: &User{ID: 2}, Account: &Account{ID: 3},
						})
					}
				}()
				select {
				case err := <-done:
					if billingFails {
						require.ErrorIs(t, err, billingRepo.err)
					} else {
						require.NoError(t, err)
					}
				case <-time.After(5 * time.Second):
					t.Fatal("billing must not wait for HTTP finalize")
				}
				require.Equal(t, 1, billingRepo.calls)
				require.Zero(t, usageRepo.calls, "do not persist an unfinished compressed response")

				response.AddActual(8) // compressor Close trailer arrives after the usage task
				response.CompleteOriginal()
				response.CompleteActual()
				collector.Finalize()
				collector.Finalize()
				require.Equal(t, 1, usageRepo.calls)
				require.NoError(t, usageRepo.lastCtxErr)
				log := usageRepo.lastLog
				require.NotNil(t, log.Traffic)
				require.EqualValues(t, 220, *log.Traffic.Upstream.Request.OriginalBytes)
				require.EqualValues(t, 130, *log.Traffic.Upstream.Request.ActualBytes)
				require.EqualValues(t, 290, *log.Traffic.Upstream.Response.OriginalBytes)
				require.EqualValues(t, 110, *log.Traffic.Upstream.Response.ActualBytes)
				require.EqualValues(t, 43, *log.Traffic.Downstream.Response.ActualBytes)
				if billingFails {
					require.Zero(t, log.ActualCost)
				}
			})
		}
	}
}

func TestWriteUsageTraffic_FinalizedAndLegacyPaths(t *testing.T) {
	for _, finalized := range []bool{false, true} {
		t.Run(map[bool]string{false: "no_collector", true: "already_finalized"}[finalized], func(t *testing.T) {
			repo := &openAIRecordUsageLogRepoStub{}
			ctx := context.Background()
			if finalized {
				collector := traffic.NewCollector()
				completeUsageTrafficBody(collector, traffic.DownstreamResponse, 0, 0, false)
				collector.Finalize()
				ctx = traffic.WithCollector(ctx, collector)
			}
			writeUsageLogBestEffort(ctx, repo, &UsageLog{RequestID: "immediate"}, "test")
			require.Equal(t, 1, repo.calls)
			if finalized {
				require.EqualValues(t, 0, *repo.lastLog.Traffic.Downstream.Response.ActualBytes)
			} else {
				require.Nil(t, repo.lastLog.Traffic)
			}
		})
	}
}

func TestWriteUsageTraffic_FinalizedPersistencePreservesFallbackAndFrozenFields(t *testing.T) {
	collector := traffic.NewCollector()
	ctx, cancel := context.WithCancel(traffic.WithCollector(context.Background(), collector))
	repo := &openAIRecordUsageBestEffortLogRepoStub{bestEffortErr: MarkUsageLogCreateDropped(errors.New("queue full"))}
	model, duration := "original", 23
	log := &UsageLog{RequestID: "frozen", UpstreamModel: &model, DurationMs: &duration, ImageSizeBreakdown: map[string]int{"large": 1}}
	writeUsageLogBestEffort(ctx, repo, log, "test")
	require.Zero(t, repo.bestEffortCalls)
	log.RequestID, model, duration = "changed", "changed", 99
	log.ImageSizeBreakdown["large"] = 7
	cancel()
	completeUsageTrafficBody(collector, traffic.DownstreamResponse, 10, 5, true)
	collector.Finalize()
	require.Equal(t, 1, repo.bestEffortCalls)
	require.Equal(t, 1, repo.createCalls)
	require.NoError(t, repo.lastCtxErr)
	require.Equal(t, "frozen", repo.lastLog.RequestID)
	require.Equal(t, "original", *repo.lastLog.UpstreamModel)
	require.Equal(t, 23, *repo.lastLog.DurationMs)
	require.Equal(t, map[string]int{"large": 1}, repo.lastLog.ImageSizeBreakdown)
	require.EqualValues(t, 5, *repo.lastLog.Traffic.Downstream.Response.ActualBytes)
	require.Nil(t, log.Traffic, "callback must not mutate the caller's log")
}

func TestWriteUsageTraffic_PartialFailedAttemptRemainsUnknown(t *testing.T) {
	collector := traffic.NewCollector()
	partial := collector.Begin(traffic.UpstreamResponse, true)
	partial.AddActual(11)
	partial.FailOriginal()
	partial.FailActual()
	completeUsageTrafficBody(collector, traffic.UpstreamResponse, 60, 20, true)
	repo := &openAIRecordUsageLogRepoStub{}
	writeUsageLogBestEffort(traffic.WithCollector(context.Background(), collector), repo, &UsageLog{}, "test")
	collector.Finalize()
	require.NotNil(t, repo.lastLog.Traffic.Upstream.Response)
	require.Nil(t, repo.lastLog.Traffic.Upstream.Response.OriginalBytes)
	require.Nil(t, repo.lastLog.Traffic.Upstream.Response.ActualBytes, "do not silently count only the successful retry")
}

func TestWriteUsageTraffic_WebSocketTurnsStayUnknownAndImmediate(t *testing.T) {
	collector := traffic.NewCollector()
	completeUsageTrafficBody(collector, traffic.DownstreamResponse, 123, 123, false)
	// Legacy/direct WS callers have no collector; handshake counts from an
	// unrelated collector must never be reused on these records.
	ctx := context.Background()
	repo := &openAIRecordUsageLogRepoStub{}
	for _, log := range []*UsageLog{
		{RequestID: "turn-1", OpenAIWSMode: true},
		{RequestID: "turn-2", OpenAIWSMode: true, Traffic: collector.Snapshot()},
		{RequestID: "turn-3", RequestType: RequestTypeWSV2},
		{RequestID: "live-turn", RequestType: RequestTypeLive},
	} {
		before := repo.calls
		writeUsageLogBestEffort(ctx, repo, log, "test")
		require.Equal(t, before+1, repo.calls, "WS logs cannot wait for session shutdown")
		require.Nil(t, repo.lastLog.Traffic)
	}
	collector.Finalize()
	require.Equal(t, 4, repo.calls, "finalize must not duplicate per-turn records")
}

func TestWriteUsageTraffic_HTTPIngressUpstreamWSKeepsDownstream(t *testing.T) {
	collector := traffic.NewCollector()
	completeUsageTrafficBody(collector, traffic.DownstreamRequest, 100, 100, false)
	completeUsageTrafficBody(collector, traffic.DownstreamResponse, 200, 60, true)
	// Even if a prior HTTP attempt was measured, a final WS response has no
	// complete upstream HTTP entity-body representation for this request.
	completeUsageTrafficBody(collector, traffic.UpstreamRequest, 80, 80, false)
	repo := &openAIRecordUsageLogRepoStub{}
	writeUsageLogBestEffort(traffic.WithCollector(context.Background(), collector), repo, &UsageLog{OpenAIWSMode: true}, "test")
	require.Zero(t, repo.calls)
	collector.Finalize()
	require.Equal(t, 1, repo.calls)
	require.NotNil(t, repo.lastLog.Traffic)
	require.Nil(t, repo.lastLog.Traffic.Upstream)
	require.EqualValues(t, 100, *repo.lastLog.Traffic.Downstream.Request.ActualBytes)
	require.EqualValues(t, 60, *repo.lastLog.Traffic.Downstream.Response.ActualBytes)
}

func TestWriteUsageTraffic_ExcludedWebSocketCollectorWritesEveryTurnImmediately(t *testing.T) {
	collector := traffic.NewCollector()
	completeUsageTrafficBody(collector, traffic.DownstreamRequest, 0, 0, false)
	collector.Exclude() // confirmed successful Hijack, not an untrusted Upgrade header
	ctx := traffic.WithCollector(context.Background(), collector)
	repo := &openAIRecordUsageLogRepoStub{}
	for _, log := range []*UsageLog{
		{RequestID: "ws-turn-1", OpenAIWSMode: true},
		{RequestID: "ws-turn-2", RequestType: RequestTypeWSV2},
		{RequestID: "ws-cyber", RequestType: RequestTypeCyberBlocked},
	} {
		before := repo.calls
		writeUsageLogBestEffort(ctx, repo, log, "test")
		require.Equal(t, before+1, repo.calls, "confirmed WS must not wait for the whole session to finalize")
		require.Nil(t, repo.lastLog.Traffic)
	}
	collector.Finalize()
	require.Equal(t, 3, repo.calls)
}

type usageTrafficFuncRepo struct {
	UsageLogRepository
	create func(context.Context, *UsageLog) (bool, error)
}

func (r *usageTrafficFuncRepo) Create(ctx context.Context, log *UsageLog) (bool, error) {
	return r.create(ctx, log)
}

func TestWriteUsageTraffic_HealthyPoolKeepsFinalizeOffDatabaseAndDrains(t *testing.T) {
	pool := newTrafficPersistenceTestPool(t, "drop", 8)
	collector := traffic.NewCollector()
	completeUsageTrafficBody(collector, traffic.DownstreamResponse, 100, 40, true)
	ctx, cancel := context.WithCancel(WithUsageRecordWorkerPool(traffic.WithCollector(context.Background(), collector), pool))
	entered, release := make(chan struct{}), make(chan struct{})
	releaseWriter := sync.OnceFunc(func() { close(release) })
	defer releaseWriter()
	written := make(chan *UsageLog, 1)
	repo := &usageTrafficFuncRepo{create: func(ctx context.Context, log *UsageLog) (bool, error) {
		close(entered)
		<-release
		if err := ctx.Err(); err != nil {
			return false, err
		}
		written <- log
		return true, nil
	}}
	writeUsageLogBestEffort(ctx, repo, &UsageLog{RequestID: "offload"}, "test")
	cancel()
	finalized := make(chan struct{})
	go func() { collector.Finalize(); close(finalized) }()
	select {
	case <-finalized:
	case <-time.After(time.Second):
		t.Fatal("Finalize must return while a healthy pool's database task is blocked")
	}
	select {
	case <-entered:
	case <-time.After(time.Second):
		t.Fatal("persistence task was not dispatched")
	}
	stopped := make(chan struct{})
	go func() { pool.Stop(); close(stopped) }()
	select {
	case <-stopped:
		t.Fatal("Stop must drain accepted persistence work")
	default:
	}
	releaseWriter()
	select {
	case log := <-written:
		require.EqualValues(t, 40, *log.Traffic.Downstream.Response.ActualBytes)
	case <-time.After(time.Second):
		t.Fatal("persistence was lost after cancellation")
	}
	select {
	case <-stopped:
	case <-time.After(time.Second):
		t.Fatal("pool did not finish draining")
	}
}

func TestWriteUsageTraffic_AlreadyFinalizedDoesNotRequeue(t *testing.T) {
	pool := newTrafficPersistenceTestPool(t, "drop", 1)
	collector := traffic.NewCollector()
	collector.Finalize()
	repo := &openAIRecordUsageLogRepoStub{}
	ctx := WithUsageRecordWorkerPool(traffic.WithCollector(context.Background(), collector), pool)
	writeUsageLogBestEffort(ctx, repo, &UsageLog{}, "test")
	require.Equal(t, 1, repo.calls)
	require.Zero(t, pool.Stats().SubmittedTasks)
}

func TestWriteUsageTraffic_FullOrStoppedPoolPreservesSynchronousWrite(t *testing.T) {
	for _, state := range []string{"full", "stopped"} {
		t.Run(state, func(t *testing.T) {
			pool := newTrafficPersistenceTestPool(t, "drop", 1)
			if state == "stopped" {
				pool.Stop()
			} else {
				started, release := make(chan struct{}), make(chan struct{})
				defer close(release)
				pool.Submit(func(context.Context) { close(started); <-release })
				<-started
				require.Equal(t, UsageRecordSubmitModeEnqueued, pool.Submit(func(context.Context) {}))
			}
			collector := traffic.NewCollector()
			repo := &openAIRecordUsageLogRepoStub{}
			ctx := WithUsageRecordWorkerPool(traffic.WithCollector(context.Background(), collector), pool)
			writeUsageLogBestEffort(ctx, repo, &UsageLog{}, "test")
			collector.Finalize()
			require.Equal(t, 1, repo.calls)
			require.EqualValues(t, 1, pool.Stats().SyncFallbackTasks)
		})
	}
}

func TestWriteUsageTraffic_FinalizeRegistrationRaceWritesExactlyOnce(t *testing.T) {
	pool := newTrafficPersistenceTestPool(t, "drop", 64)
	var writes atomic.Int64
	repo := &usageTrafficFuncRepo{create: func(context.Context, *UsageLog) (bool, error) { writes.Add(1); return true, nil }}
	for range 64 {
		collector := traffic.NewCollector()
		ctx := WithUsageRecordWorkerPool(traffic.WithCollector(context.Background(), collector), pool)
		var racing sync.WaitGroup
		racing.Add(2)
		go func() { defer racing.Done(); writeUsageLogBestEffort(ctx, repo, &UsageLog{}, "test") }()
		go func() { defer racing.Done(); collector.Finalize() }()
		racing.Wait()
		collector.Finalize()
	}
	pool.Stop()
	require.EqualValues(t, 64, writes.Load())
}
