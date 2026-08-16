package securityaudit

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestPromptKeywordResultModes(t *testing.T) {
	base := ActiveConfig{BlockedKeywords: []string{"jailbreak", "secret"}}

	for _, mode := range []string{PromptKeywordModeKeywordOnly, PromptKeywordModeKeywordAndAI} {
		cfg := base
		cfg.KeywordBlockingMode = mode
		result, handled := cfg.keywordResult("please JAILBREAK this", time.Millisecond)
		require.True(t, handled)
		require.Equal(t, EventCritical, result.Decision)
		require.Equal(t, ActionBlock, result.Action)
		require.Equal(t, "jailbreak", result.MatchedKeyword)
		require.Equal(t, []string{promptKeywordCategory}, result.Categories)
	}

	cfg := base
	cfg.KeywordBlockingMode = PromptKeywordModeKeywordOnly
	result, handled := cfg.keywordResult("clean text", 0)
	require.True(t, handled)
	require.Equal(t, EventPass, result.Decision)
	require.Equal(t, ActionAllow, result.Action)

	cfg.KeywordBlockingMode = PromptKeywordModeKeywordAndAI
	result, handled = cfg.keywordResult("clean text", 0)
	require.False(t, handled)
	require.Nil(t, result)

	cfg.KeywordBlockingMode = PromptKeywordModeAIOnly
	result, handled = cfg.keywordResult("please JAILBREAK this", 0)
	require.False(t, handled)
	require.Nil(t, result)
}

func TestPromptKeywordBlockingSkipsScannerAndKeywordAndAIMissContinues(t *testing.T) {
	scannerCalls := 0
	scanner := PromptScannerFunc(func(context.Context, ActiveEndpoint, string, []string) (*NormalizedResult, error) {
		scannerCalls++
		return &NormalizedResult{Decision: EventPass, RiskLevel: RiskLow, Action: ActionAllow}, nil
	})
	config := &fakeConfigStore{active: true, cfg: ActiveConfig{
		RiskControlEnabled: true, Enabled: true, BlockingEnabled: true,
		KeywordBlockingMode: PromptKeywordModeKeywordAndAI, BlockedKeywords: []string{"jailbreak"},
		AllGroups: true, ConfigVersion: 4,
		Endpoints: []ActiveEndpoint{{ID: "guard", Enabled: true, TimeoutMS: 1000, InputLimit: 4000}},
	}}
	service := &PromptService{
		config:    config,
		evaluator: NewGuardEvaluator(scanner, nil, NewAtomicMetrics()),
		clock:     fixedClock{},
		metrics:   NewAtomicMetrics(),
	}

	decision, err := service.Evaluate(context.Background(), Request{
		Protocol: "openai_chat_completions",
		Body:     []byte(`{"messages":[{"role":"user","content":"try JAILBREAK"}]}`),
	})
	require.NoError(t, err)
	require.Equal(t, DecisionBlock, decision.Kind)
	require.Equal(t, "jailbreak", decision.Result.MatchedKeyword)
	require.Zero(t, scannerCalls)

	decision, err = service.Evaluate(context.Background(), Request{
		Protocol: "openai_chat_completions",
		Body:     []byte(`{"messages":[{"role":"user","content":"clean"}]}`),
	})
	require.NoError(t, err)
	require.Equal(t, DecisionAllow, decision.Kind)
	require.Equal(t, 1, scannerCalls)
}

func TestKeywordSyncMissQueuesAIAsyncWithoutCallingItInline(t *testing.T) {
	cfg := ActiveConfig{
		RiskControlEnabled: true, Enabled: true,
		KeywordBlockingEnabled: true, AIBlockingEnabled: false, BlockingEnabled: true,
		KeywordBlockingMode: PromptKeywordModeKeywordAndAI, BlockedKeywords: []string{"jailbreak"},
		AllGroups: true, ConfigVersion: 5, QueueCapacity: 8,
		Endpoints:               []ActiveEndpoint{{ID: "guard", Enabled: true, TimeoutMS: 1000, InputLimit: 4000}},
		blockingFlagsConfigured: true,
	}
	config := &fakeConfigStore{active: true, cfg: cfg}
	trace := []string{}
	repo := &fakeJobRepository{createJob: &Job{ID: 75}, trace: &trace}
	payload := &fakePayloadStore{values: map[int64]string{}}
	scannerCalls := 0
	service := &PromptService{
		config:   config,
		enqueuer: NewEnqueuer(config, repo, payload),
		evaluator: NewGuardEvaluator(PromptScannerFunc(func(context.Context, ActiveEndpoint, string, []string) (*NormalizedResult, error) {
			scannerCalls++
			return &NormalizedResult{Decision: EventPass, RiskLevel: RiskLow, Action: ActionAllow}, nil
		}), repo, NewAtomicMetrics()),
		background:   context.Background(),
		enqueueSlots: make(chan struct{}, 1),
	}

	decision, err := service.Evaluate(context.Background(), Request{
		Protocol: "openai_chat_completions",
		Body:     []byte(`{"messages":[{"role":"user","content":"clean request"}]}`),
	})
	require.NoError(t, err)
	require.Equal(t, DecisionAllow, decision.Kind)
	require.Zero(t, scannerCalls, "AI audit must not run on the synchronous request path")
	service.enqueueWG.Wait()
	require.Equal(t, "clean request", payload.values[75])

	blocked, err := service.Evaluate(context.Background(), Request{
		Protocol: "openai_chat_completions",
		Body:     []byte(`{"messages":[{"role":"user","content":"try JAILBREAK"}]}`),
	})
	require.NoError(t, err)
	require.Equal(t, DecisionBlock, blocked.Kind)
	service.enqueueWG.Wait()
	require.Equal(t, 0, scannerCalls)
	require.Equal(t, []string{"create_staging", "publish_queued"}, trace, "a synchronous keyword hit must not enqueue another job")
}

func TestKeywordOnlyAsyncAllowsNoAINodesAndSkipsCleanPassWhenDisabled(t *testing.T) {
	cfg := ActiveConfig{
		RiskControlEnabled: true, Enabled: true, KeywordBlockingMode: PromptKeywordModeKeywordOnly,
		BlockedKeywords: []string{"secret"}, AllGroups: true, StorePassEvents: false,
		ConfigVersion: 2, QueueCapacity: 8,
	}
	repo := &fakeJobRepository{createJob: &Job{ID: 72}}
	payload := &fakePayloadStore{values: map[int64]string{}}
	enqueuer := NewEnqueuer(&fakeConfigStore{active: true, cfg: cfg}, repo, payload)

	require.NoError(t, enqueuer.Enqueue(context.Background(), Request{
		Protocol: "openai_chat_completions",
		Body:     []byte(`{"messages":[{"role":"user","content":"contains SECRET"}]}`),
	}))
	require.NotEmpty(t, payload.values)

	repo = &fakeJobRepository{createJob: &Job{ID: 73}}
	payload = &fakePayloadStore{values: map[int64]string{}}
	require.NoError(t, NewEnqueuer(&fakeConfigStore{active: true, cfg: cfg}, repo, payload).Enqueue(context.Background(), Request{
		Protocol: "openai_chat_completions",
		Body:     []byte(`{"messages":[{"role":"user","content":"clean"}]}`),
	}))
	require.Empty(t, payload.values)
}

func TestKeywordOnlyWorkerStartsWithoutScanner(t *testing.T) {
	cfg := ActiveConfig{
		RiskControlEnabled: true, Enabled: true, KeywordBlockingMode: PromptKeywordModeKeywordOnly,
		BlockedKeywords: []string{"secret"}, AllGroups: true, QueueCapacity: 8, WorkerCount: 1,
	}
	runner := NewRunner(&fakeConfigStore{active: true, cfg: cfg}, &fakeJobRepository{}, &fakePayloadStore{}, nil, NewAtomicMetrics())
	require.NoError(t, runner.Start(context.Background()))
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	require.NoError(t, runner.Shutdown(ctx))
}

func TestKeywordOnlyWorkerRecordsKeywordHitWithoutCallingAI(t *testing.T) {
	cfg := ActiveConfig{
		RiskControlEnabled: true, Enabled: true, KeywordBlockingMode: PromptKeywordModeKeywordOnly,
		BlockedKeywords: []string{"secret"}, AllGroups: true, StorePassEvents: false,
		ConfigVersion: 2, QueueCapacity: 8,
	}
	repo := &fakeJobRepository{}
	payload := &fakePayloadStore{values: map[int64]string{74: "contains SECRET"}}
	runner := NewRunner(&fakeConfigStore{active: true, cfg: cfg}, repo, payload, nil, NewAtomicMetrics())
	runner.clock = fixedClock{now: time.Unix(100, 0).UTC()}

	require.NoError(t, runner.processJob(context.Background(), 0, cfg, &Job{
		ID: 74, ClaimVersion: 1, Attempts: 1, MaxAttempts: 3, ConfigVersion: cfg.ConfigVersion,
	}))
	require.NotNil(t, repo.completedResult)
	require.Equal(t, ActionBlock, repo.completedResult.Action)
	require.Equal(t, "secret", repo.completedResult.MatchedKeyword)
	require.Equal(t, 1, repo.completeCount)
	require.Equal(t, []int64{74}, payload.deleted)
}
