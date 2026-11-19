package securityaudit

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

type scriptedScanner struct {
	mu      sync.Mutex
	calls   []string
	block   <-chan struct{}
	entered chan<- struct{}
}

func (s *scriptedScanner) Scan(ctx context.Context, endpoint ActiveEndpoint, _ string, _ []string) (*NormalizedResult, error) {
	s.mu.Lock()
	s.calls = append(s.calls, endpoint.ID)
	s.mu.Unlock()
	if s.entered != nil {
		select {
		case s.entered <- struct{}{}:
		default:
		}
	}
	if s.block != nil {
		select {
		case <-s.block:
		case <-ctx.Done():
			return nil, &GuardError{Code: ErrorCodeUnavailable, Retryable: true, Timeout: true, Cause: ctx.Err()}
		}
	}
	if endpoint.ID == "bad" {
		return nil, &GuardError{Code: ErrorCodeUnavailable, Retryable: true}
	}
	if endpoint.ID == "invalid" {
		return nil, &GuardError{Code: ErrorCodeInvalidResponse}
	}
	return &NormalizedResult{Decision: EventPass, RiskLevel: RiskLow, Action: ActionAllow, Safety: "Safe", ScannerScores: map[string]float64{}, ScannerEvidence: map[string]string{}, GuardEndpointID: endpoint.ID}, nil
}

func guardConfig(endpoints ...ActiveEndpoint) ActiveConfig {
	return ActiveConfig{RiskControlEnabled: true, Enabled: true, BlockingEnabled: true, ConfigVersion: 2, Scanners: AllScannerIDs, Endpoints: endpoints}
}

func TestGuardEvaluatorOrderedFailoverAndInvalidTerminal(t *testing.T) {
	scanner := &scriptedScanner{}
	metrics := NewAtomicMetrics()
	evaluator := newGuardEvaluator(scanner, nil, metrics, 4, 2)
	snapshot := PromptSnapshot{RequestID: "r", ScanText: "hello", PromptLength: 5}
	decision, err := evaluator.Evaluate(context.Background(), guardConfig(
		ActiveEndpoint{ID: "bad", Enabled: true, TimeoutMS: 1000, InputLimit: 100},
		ActiveEndpoint{ID: "good", Enabled: true, TimeoutMS: 1000, InputLimit: 100},
	), snapshot)
	require.NoError(t, err)
	require.Equal(t, DecisionAllow, decision.Kind)
	require.Equal(t, int64(1), metrics.Snapshot().Failovers)
	// Changing the endpoints saves a new config version.
	invalidFirst := guardConfig(
		ActiveEndpoint{ID: "invalid", Enabled: true, TimeoutMS: 1000, InputLimit: 100},
		ActiveEndpoint{ID: "good", Enabled: true, TimeoutMS: 1000, InputLimit: 100},
	)
	invalidFirst.ConfigVersion++
	_, err = evaluator.Evaluate(context.Background(), invalidFirst, snapshot)
	var guardErr *GuardError
	require.ErrorAs(t, err, &guardErr)
	require.Equal(t, ErrorCodeInvalidResponse, guardErr.Code)
	snapshotMetrics := metrics.Snapshot()
	require.Equal(t, int64(2), snapshotMetrics.Total)
	require.Equal(t, int64(1), snapshotMetrics.Allowed)
	require.Equal(t, int64(1), snapshotMetrics.Invalid)
}

func TestGuardEvaluatorGlobalBulkheadIsNonBlocking(t *testing.T) {
	release := make(chan struct{})
	entered := make(chan struct{}, 1)
	scanner := &scriptedScanner{block: release, entered: entered}
	metrics := NewAtomicMetrics()
	evaluator := newGuardEvaluator(scanner, nil, metrics, 1, 1)
	cfg := guardConfig(ActiveEndpoint{ID: "good", Enabled: true, TimeoutMS: 2000, InputLimit: 100})
	done := make(chan error, 1)
	go func() {
		_, err := evaluator.Evaluate(context.Background(), cfg, PromptSnapshot{ScanText: "one", PromptLength: 3})
		done <- err
	}()
	select {
	case <-entered:
	case <-time.After(time.Second):
		t.Fatal("first evaluation did not enter scanner")
	}
	start := time.Now()
	_, err := evaluator.Evaluate(context.Background(), cfg, PromptSnapshot{ScanText: "two", PromptLength: 3})
	require.Error(t, err)
	require.Less(t, time.Since(start), 200*time.Millisecond)
	require.Equal(t, int64(1), metrics.Snapshot().BulkheadFull)
	close(release)
	require.NoError(t, <-done)
	snapshotMetrics := metrics.Snapshot()
	require.Equal(t, int64(2), snapshotMetrics.Total)
	require.Equal(t, int64(1), snapshotMetrics.Allowed)
	require.Equal(t, int64(1), snapshotMetrics.Unavailable)
}

func TestGuardEvaluatorPerNodeBulkheadIsNonBlocking(t *testing.T) {
	release := make(chan struct{})
	entered := make(chan struct{}, 1)
	scanner := &scriptedScanner{block: release, entered: entered}
	metrics := NewAtomicMetrics()
	evaluator := newGuardEvaluator(scanner, nil, metrics, 2, 1)
	cfg := guardConfig(ActiveEndpoint{ID: "same-node", Enabled: true, TimeoutMS: 2000, InputLimit: 100})
	done := make(chan error, 1)
	go func() {
		_, err := evaluator.Evaluate(context.Background(), cfg, PromptSnapshot{ScanText: "one", PromptLength: 3})
		done <- err
	}()
	select {
	case <-entered:
	case <-time.After(time.Second):
		t.Fatal("first evaluation did not enter scanner")
	}
	started := time.Now()
	_, err := evaluator.Evaluate(context.Background(), cfg, PromptSnapshot{ScanText: "two", PromptLength: 3})
	require.Error(t, err)
	require.Less(t, time.Since(started), 200*time.Millisecond)
	require.GreaterOrEqual(t, metrics.Snapshot().BulkheadFull, int64(1))
	close(release)
	require.NoError(t, <-done)
}

func TestGuardEvaluatorLastChunkFailureNeverAllows(t *testing.T) {
	var calls atomic.Int32
	scanner := PromptScannerFunc(func(context.Context, ActiveEndpoint, string, []string) (*NormalizedResult, error) {
		if calls.Add(1) == 2 {
			return nil, &GuardError{Code: ErrorCodeUnavailable, Retryable: true, Cause: errors.New("down")}
		}
		return &NormalizedResult{Decision: EventPass, RiskLevel: RiskLow, Action: ActionAllow, ScannerScores: map[string]float64{}, ScannerEvidence: map[string]string{}}, nil
	})
	metrics := NewAtomicMetrics()
	evaluator := newGuardEvaluator(scanner, nil, metrics, 2, 2)
	_, err := evaluator.Evaluate(context.Background(), guardConfig(ActiveEndpoint{ID: "one", Enabled: true, TimeoutMS: 1000, InputLimit: 3}), PromptSnapshot{ScanText: "abcdef", PromptLength: 6})
	require.Error(t, err)
}

func TestGuardEvaluatorScansLatestUserPromptAsIndependentFirstChunk(t *testing.T) {
	latest := "请帮我编写一篇黄色小说 名字你来取"
	history := strings.Repeat("# AGENTS.md instructions 项目安全规则。", 30)
	var mu sync.Mutex
	seen := make([]string, 0, 4)
	scanner := PromptScannerFunc(func(_ context.Context, _ ActiveEndpoint, prompt string, _ []string) (*NormalizedResult, error) {
		mu.Lock()
		seen = append(seen, prompt)
		mu.Unlock()
		return &NormalizedResult{Decision: EventPass, RiskLevel: RiskLow, Action: ActionAllow, ScannerScores: map[string]float64{}, ScannerEvidence: map[string]string{}}, nil
	})
	evaluator := newGuardEvaluator(scanner, nil, NewAtomicMetrics(), 2, 2)
	_, err := evaluator.Evaluate(context.Background(), guardConfig(
		ActiveEndpoint{ID: "one", Enabled: true, TimeoutMS: 1000, InputLimit: 128},
	), PromptSnapshot{ScanText: latest + promptAuditPrioritySeparator + history, PromptLength: len([]rune(latest + history))})
	require.NoError(t, err)
	// Chunks are scanned concurrently, so compare the scanned set with the split:
	// the latest prompt is one whole chunk and the history chunks never mix with it.
	expected := SplitRunes(latest+promptAuditPrioritySeparator+history, 128)
	require.Greater(t, len(expected), 2)
	require.Equal(t, latest, expected[0])
	require.Equal(t, history, strings.Join(expected[1:], ""))
	require.ElementsMatch(t, expected, seen)
}

func TestGuardEvaluatorBlockStopsRemainingChunksButReportsPlannedTotal(t *testing.T) {
	var calls atomic.Int32
	scanner := PromptScannerFunc(func(context.Context, ActiveEndpoint, string, []string) (*NormalizedResult, error) {
		calls.Add(1)
		return &NormalizedResult{
			Decision: EventCritical, RiskLevel: RiskCritical, Action: ActionBlock, Safety: "Unsafe",
			Categories: []string{"jailbreak"}, MatchedScanners: []string{"jailbreak"},
			ScannerScores: map[string]float64{"jailbreak": 1}, ScannerEvidence: map[string]string{"jailbreak": "Jailbreak"},
		}, nil
	})
	metrics := NewAtomicMetrics()
	evaluator := newGuardEvaluator(scanner, nil, metrics, 2, maxConcurrentChunks)
	// Three times the concurrency limit: every worker stops after its first Block,
	// so at most maxConcurrentChunks chunks are ever sent.
	text := strings.Repeat("abc", 3*maxConcurrentChunks)
	decision, err := evaluator.Evaluate(context.Background(), guardConfig(
		ActiveEndpoint{ID: "one", Enabled: true, TimeoutMS: 1000, InputLimit: 3},
	), PromptSnapshot{ScanText: text, PromptLength: len(text)})
	require.NoError(t, err)
	require.Equal(t, DecisionBlock, decision.Kind)
	require.LessOrEqual(t, int(calls.Load()), maxConcurrentChunks)
	require.Equal(t, 3*maxConcurrentChunks, decision.Result.ChunkTotal)
	require.Equal(t, int64(1), metrics.Snapshot().Blocked)
}

func TestGuardEvaluatorFlagSharedDeadlineFailClosedAndContextCancel(t *testing.T) {
	t.Run("flag allows next stage", func(t *testing.T) {
		metrics := NewAtomicMetrics()
		evaluator := newGuardEvaluator(PromptScannerFunc(func(context.Context, ActiveEndpoint, string, []string) (*NormalizedResult, error) {
			return &NormalizedResult{Decision: EventFlag, RiskLevel: RiskMedium, Action: ActionWarn, Safety: "Controversial", Categories: []string{"violent"}, MatchedScanners: []string{"violent"}, ScannerScores: map[string]float64{"violent": .5}, ScannerEvidence: map[string]string{"violent": "Violent"}}, nil
		}), nil, metrics, 2, 2)
		decision, err := evaluator.Evaluate(context.Background(), guardConfig(ActiveEndpoint{ID: "one", Enabled: true, TimeoutMS: 1000, InputLimit: 100}), PromptSnapshot{ScanText: "review", PromptLength: 6})
		require.NoError(t, err)
		require.Equal(t, DecisionFlag, decision.Kind)
		require.True(t, decision.AllowNextStage)
		require.Equal(t, int64(1), metrics.Snapshot().Flagged)
	})

	t.Run("all failovers share first endpoint deadline", func(t *testing.T) {
		calls := 0
		scanner := PromptScannerFunc(func(ctx context.Context, endpoint ActiveEndpoint, _ string, _ []string) (*NormalizedResult, error) {
			calls++
			if endpoint.ID == "first" {
				select {
				case <-time.After(35 * time.Millisecond):
					return nil, &GuardError{Code: ErrorCodeUnavailable, Retryable: true}
				case <-ctx.Done():
					return nil, &GuardError{Code: ErrorCodeUnavailable, Retryable: true, Timeout: true, Cause: ctx.Err()}
				}
			}
			<-ctx.Done()
			return nil, &GuardError{Code: ErrorCodeUnavailable, Retryable: true, Timeout: true, Cause: ctx.Err()}
		})
		metrics := NewAtomicMetrics()
		evaluator := newGuardEvaluator(scanner, nil, metrics, 2, 2)
		started := time.Now()
		_, err := evaluator.Evaluate(context.Background(), guardConfig(
			ActiveEndpoint{ID: "first", Enabled: true, TimeoutMS: 70, InputLimit: 100},
			ActiveEndpoint{ID: "second", Enabled: true, TimeoutMS: 500, InputLimit: 100},
		), PromptSnapshot{ScanText: "deadline", PromptLength: 8})
		elapsed := time.Since(started)
		require.Error(t, err)
		require.Equal(t, 2, calls)
		// The bound only has to prove the failover shared the first endpoint's
		// 70ms deadline instead of taking the second endpoint's own 500ms one.
		// An unshared deadline lands at ~535ms, so 350ms still fails loudly
		// while leaving room for scheduler delay on a busy CI machine. A
		// tighter bound made this test flaky, not stricter.
		require.Less(t, elapsed, 350*time.Millisecond)
		require.GreaterOrEqual(t, elapsed, 50*time.Millisecond)
		require.Equal(t, int64(1), metrics.Snapshot().Failovers)
		require.Equal(t, int64(1), metrics.Snapshot().Timeouts)
	})

	t.Run("canceled parent never allows", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		cancel()
		evaluator := newGuardEvaluator(PromptScannerFunc(func(ctx context.Context, _ ActiveEndpoint, _ string, _ []string) (*NormalizedResult, error) {
			<-ctx.Done()
			return nil, &GuardError{Code: ErrorCodeUnavailable, Retryable: true, Cause: ctx.Err()}
		}), nil, NewAtomicMetrics(), 2, 2)
		decision, err := evaluator.Evaluate(ctx, guardConfig(ActiveEndpoint{ID: "one", Enabled: true, TimeoutMS: 1000, InputLimit: 100}), PromptSnapshot{ScanText: "cancel", PromptLength: 6})
		require.Error(t, err)
		require.Nil(t, decision)
	})
}

func TestGuardEvaluatorRecordsExistingResultOnceAndRecordFailureDoesNotChangeDecision(t *testing.T) {
	for _, recordErr := range []error{nil, errors.New("database unavailable")} {
		repo := &fakeJobRepository{recordBlockingErr: recordErr}
		metrics := NewAtomicMetrics()
		scannerCalls := 0
		evaluator := newGuardEvaluator(PromptScannerFunc(func(context.Context, ActiveEndpoint, string, []string) (*NormalizedResult, error) {
			scannerCalls++
			return &NormalizedResult{Decision: EventCritical, RiskLevel: RiskCritical, Action: ActionBlock, Safety: "Unsafe", Categories: []string{"pii"}, MatchedScanners: []string{"pii"}, ScannerScores: map[string]float64{"pii": 1}, ScannerEvidence: map[string]string{"pii": "PII"}}, nil
		}), repo, metrics, 2, 2)
		decision, err := evaluator.Evaluate(context.Background(), guardConfig(ActiveEndpoint{ID: "one", Enabled: true, TimeoutMS: 1000, InputLimit: 100}), PromptSnapshot{ScanText: "raw prompt", RedactedPreview: "raw***", PromptLength: 10})
		require.NoError(t, err)
		require.Equal(t, DecisionBlock, decision.Kind)
		require.Equal(t, 1, scannerCalls)
		require.Equal(t, 1, repo.recordBlockingCalls)
		require.Empty(t, repo.recordBlockingSnapshot.ScanText)
		require.Same(t, decision.Result, repo.recordBlockingResult)
		if recordErr != nil {
			require.Equal(t, int64(1), metrics.Snapshot().RecordFailed)
		} else {
			require.Zero(t, metrics.Snapshot().RecordFailed)
		}
	}
}

func TestGuardEvaluatorNilResultAndScannerPanicBecomeStableFailures(t *testing.T) {
	tests := []struct {
		name string
		scan PromptScannerFunc
		code string
	}{
		{name: "nil result", scan: func(context.Context, ActiveEndpoint, string, []string) (*NormalizedResult, error) { return nil, nil }, code: ErrorCodeInvalidResponse},
		{name: "panic", scan: func(context.Context, ActiveEndpoint, string, []string) (*NormalizedResult, error) {
			panic("raw prompt canary")
		}, code: ErrorCodeUnavailable},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			evaluator := newGuardEvaluator(tt.scan, nil, NewAtomicMetrics(), 2, 2)
			_, err := evaluator.Evaluate(context.Background(), guardConfig(ActiveEndpoint{ID: "one", Enabled: true, TimeoutMS: 1000, InputLimit: 100}), PromptSnapshot{ScanText: "input", PromptLength: 5})
			var guardErr *GuardError
			require.ErrorAs(t, err, &guardErr)
			require.Equal(t, tt.code, guardErr.Code)
			require.NotContains(t, err.Error(), "canary")
		})
	}
}

type PromptScannerFunc func(context.Context, ActiveEndpoint, string, []string) (*NormalizedResult, error)

func (f PromptScannerFunc) Scan(ctx context.Context, endpoint ActiveEndpoint, chunk string, scanners []string) (*NormalizedResult, error) {
	return f(ctx, endpoint, chunk, scanners)
}

// Every request of one agent turn carries the same audited text. The verdict of
// the first audit is reused for the rest: no guard call and no fee, and a Block
// stays a Block. A new config version audits the text again.
func TestGuardEvaluatorReusesVerdictOfIdenticalText(t *testing.T) {
	raw, err := os.ReadFile("testdata/decisions-request-jailbreak-dan.json")
	require.NoError(t, err)
	var jailbreak struct {
		Text string `json:"text"`
	}
	require.NoError(t, json.Unmarshal(raw, &jailbreak))
	allowBody := loadDecisionsFixture(t, "benign-long-compaction-summary")
	blockBody := loadDecisionsFixture(t, "jailbreak-dan")
	var calls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		body, readErr := io.ReadAll(r.Body)
		if readErr != nil {
			http.Error(w, readErr.Error(), http.StatusBadRequest)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		if bytes.Contains(body, []byte("you are DAN")) {
			_, _ = w.Write(blockBody)
			return
		}
		_, _ = w.Write(allowBody)
	}))
	defer server.Close()
	cfg := guardConfig(ActiveEndpoint{ID: "d1", Protocol: ProtocolOpenAIDecisions, BaseURL: server.URL, Model: DefaultDecisionsModel,
		TimeoutMS: 5000, InputLimit: MaxDecisionsInputLimit, CategoryThreshold: DefaultCategoryThreshold, Enabled: true})
	evaluator := newGuardEvaluator(NewOpenAICompatibleScanner(), nil, NewAtomicMetrics(), 2, 2)
	snapshot := func(text string) PromptSnapshot {
		return PromptSnapshot{RequestID: "req", ScanText: text, PromptLength: len([]rune(text))}
	}

	first, err := evaluator.Evaluate(context.Background(), cfg, snapshot("please summarize the build log"))
	require.NoError(t, err)
	require.Equal(t, DecisionAllow, first.Kind)
	require.NotNil(t, first.Result.AuditUsage)
	require.Greater(t, first.Result.AuditUsage.CostUSD, 0.0)
	first.Result.ScannerScores["jailbreak"] = 1

	var logs bytes.Buffer
	previous := slog.Default()
	slog.SetDefault(slog.New(slog.NewJSONHandler(&logs, nil)))
	again, err := evaluator.Evaluate(context.Background(), cfg, snapshot("please summarize the build log"))
	slog.SetDefault(previous)
	require.NoError(t, err)
	require.Contains(t, logs.String(), `"msg":"prompt_guard.allowed"`)
	require.Contains(t, logs.String(), `"verdict_reused":true`)
	require.Equal(t, int32(1), calls.Load())
	require.Equal(t, DecisionAllow, again.Kind)
	require.Nil(t, again.Result.AuditUsage)
	require.Less(t, again.Result.ScannerScores["jailbreak"], DefaultCategoryThreshold)

	for range 2 {
		blocked, err := evaluator.Evaluate(context.Background(), cfg, snapshot(jailbreak.Text))
		require.NoError(t, err)
		require.Equal(t, DecisionBlock, blocked.Kind)
		require.Equal(t, ErrorCodeBlocked, blocked.ErrorCode)
		require.Contains(t, blocked.Result.MatchedScanners, "jailbreak")
	}
	require.Equal(t, int32(2), calls.Load())

	cfg.ConfigVersion++
	_, err = evaluator.Evaluate(context.Background(), cfg, snapshot("please summarize the build log"))
	require.NoError(t, err)
	require.Equal(t, int32(3), calls.Load())
}

func TestVerdictCacheHoldsItsConfiguredEntries(t *testing.T) {
	cache := newVerdictCache()
	for i := range 2000 {
		cache.put(fmt.Sprintf("key-%d", i), &NormalizedResult{Action: ActionAllow})
	}
	held := 0
	for i := range 2000 {
		if _, ok := cache.get(fmt.Sprintf("key-%d", i)); ok {
			held++
		}
	}
	require.Equal(t, 2000, held)
}
