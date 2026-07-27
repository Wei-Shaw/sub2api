package securityaudit

import (
	"context"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

type scriptedScanner struct {
	mu      sync.Mutex
	calls   []string
	block   <-chan struct{REDACTED
	entered chan<- struct{REDACTED
REDACTED

func (s *scriptedScanner) Scan(ctx context.Context, endpoint ActiveEndpoint, _ string, _ []string) (*NormalizedResult, error) {
	s.mu.Lock()
	s.calls = append(s.calls, endpoint.ID)
	s.mu.Unlock()
	if s.entered != nil {
		select {
		case s.entered <- struct{REDACTED{REDACTED:
		default:
	REDACTED
REDACTED
	if s.block != nil {
		select {
		case <-s.block:
		case <-ctx.Done():
			return nil, &GuardError{Code: ErrorCodeUnavailable, Retryable: true, Timeout: true, Cause: ctx.Err()REDACTED
	REDACTED
REDACTED
	if endpoint.ID == "bad" {
		return nil, &GuardError{Code: ErrorCodeUnavailable, Retryable: trueREDACTED
REDACTED
	if endpoint.ID == "invalid" {
		return nil, &GuardError{Code: ErrorCodeInvalidResponseREDACTED
REDACTED
	return &NormalizedResult{Decision: EventPass, RiskLevel: RiskLow, Action: ActionAllow, Safety: "Safe", ScannerScores: map[string]float64{REDACTED, ScannerEvidence: map[string]string{REDACTED, GuardEndpointID: endpoint.IDREDACTED, nil
REDACTED

func guardConfig(endpoints ...ActiveEndpoint) ActiveConfig {
	return ActiveConfig{RiskControlEnabled: true, Enabled: true, BlockingEnabled: true, ConfigVersion: 2, Scanners: AllScannerIDs, Endpoints: endpointsREDACTED
REDACTED

func TestGuardEvaluatorOrderedFailoverAndInvalidTerminal(t *testing.T) {
	scanner := &scriptedScanner{REDACTED
	metrics := NewAtomicMetrics()
	evaluator := newGuardEvaluator(scanner, nil, metrics, 4, 2)
	snapshot := PromptSnapshot{RequestID: "r", ScanText: "hello", PromptLength: 5REDACTED
	decision, err := evaluator.Evaluate(context.Background(), guardConfig(
		ActiveEndpoint{ID: "bad", Enabled: true, TimeoutMS: 1000, InputLimit: 100REDACTED,
		ActiveEndpoint{ID: "good", Enabled: true, TimeoutMS: 1000, InputLimit: 100REDACTED,
	), snapshot)
REDACTED
	require.Equal(t, DecisionAllow, decision.Kind)
	require.Equal(t, int64(1), metrics.Snapshot().Failovers)
	_, err = evaluator.Evaluate(context.Background(), guardConfig(
		ActiveEndpoint{ID: "invalid", Enabled: true, TimeoutMS: 1000, InputLimit: 100REDACTED,
		ActiveEndpoint{ID: "good", Enabled: true, TimeoutMS: 1000, InputLimit: 100REDACTED,
	), snapshot)
	var guardErr *GuardError
	require.ErrorAs(t, err, &guardErr)
	require.Equal(t, ErrorCodeInvalidResponse, guardErr.Code)
	snapshotMetrics := metrics.Snapshot()
	require.Equal(t, int64(2), snapshotMetrics.Total)
	require.Equal(t, int64(1), snapshotMetrics.Allowed)
	require.Equal(t, int64(1), snapshotMetrics.Invalid)
REDACTED

func TestGuardEvaluatorGlobalBulkheadIsNonBlocking(t *testing.T) {
	release := make(chan struct{REDACTED)
	entered := make(chan struct{REDACTED, 1)
	scanner := &scriptedScanner{block: release, entered: enteredREDACTED
	metrics := NewAtomicMetrics()
	evaluator := newGuardEvaluator(scanner, nil, metrics, 1, 1)
	cfg := guardConfig(ActiveEndpoint{ID: "good", Enabled: true, TimeoutMS: 2000, InputLimit: 100REDACTED)
	done := make(chan error, 1)
	go func() {
		_, err := evaluator.Evaluate(context.Background(), cfg, PromptSnapshot{ScanText: "one", PromptLength: 3REDACTED)
		done <- err
REDACTED()
	select {
	case <-entered:
	case <-time.After(time.Second):
		t.Fatal("first evaluation did not enter scanner")
REDACTED
	start := time.Now()
	_, err := evaluator.Evaluate(context.Background(), cfg, PromptSnapshot{ScanText: "two", PromptLength: 3REDACTED)
REDACTED
	require.Less(t, time.Since(start), 200*time.Millisecond)
	require.Equal(t, int64(1), metrics.Snapshot().BulkheadFull)
	close(release)
	require.NoError(t, <-done)
	snapshotMetrics := metrics.Snapshot()
	require.Equal(t, int64(2), snapshotMetrics.Total)
	require.Equal(t, int64(1), snapshotMetrics.Allowed)
	require.Equal(t, int64(1), snapshotMetrics.Unavailable)
REDACTED

func TestGuardEvaluatorPerNodeBulkheadIsNonBlocking(t *testing.T) {
	release := make(chan struct{REDACTED)
	entered := make(chan struct{REDACTED, 1)
	scanner := &scriptedScanner{block: release, entered: enteredREDACTED
	metrics := NewAtomicMetrics()
	evaluator := newGuardEvaluator(scanner, nil, metrics, 2, 1)
	cfg := guardConfig(ActiveEndpoint{ID: "same-node", Enabled: true, TimeoutMS: 2000, InputLimit: 100REDACTED)
	done := make(chan error, 1)
	go func() {
		_, err := evaluator.Evaluate(context.Background(), cfg, PromptSnapshot{ScanText: "one", PromptLength: 3REDACTED)
		done <- err
REDACTED()
	select {
	case <-entered:
	case <-time.After(time.Second):
		t.Fatal("first evaluation did not enter scanner")
REDACTED
	started := time.Now()
	_, err := evaluator.Evaluate(context.Background(), cfg, PromptSnapshot{ScanText: "two", PromptLength: 3REDACTED)
REDACTED
	require.Less(t, time.Since(started), 200*time.Millisecond)
	require.GreaterOrEqual(t, metrics.Snapshot().BulkheadFull, int64(1))
	close(release)
	require.NoError(t, <-done)
REDACTED

func TestGuardEvaluatorLastChunkFailureNeverAllows(t *testing.T) {
	call := 0
	scanner := PromptScannerFunc(func(context.Context, ActiveEndpoint, string, []string) (*NormalizedResult, error) {
		call++
		if call == 2 {
			return nil, &GuardError{Code: ErrorCodeUnavailable, Retryable: true, Cause: errors.New("down")REDACTED
	REDACTED
		return &NormalizedResult{Decision: EventPass, RiskLevel: RiskLow, Action: ActionAllow, ScannerScores: map[string]float64{REDACTED, ScannerEvidence: map[string]string{REDACTEDREDACTED, nil
REDACTED)
	metrics := NewAtomicMetrics()
	evaluator := newGuardEvaluator(scanner, nil, metrics, 2, 2)
	_, err := evaluator.Evaluate(context.Background(), guardConfig(ActiveEndpoint{ID: "one", Enabled: true, TimeoutMS: 1000, InputLimit: 3REDACTED), PromptSnapshot{ScanText: "abcdef", PromptLength: 6REDACTED)
REDACTED
REDACTED

func TestGuardEvaluatorScansLatestUserPromptAsIndependentFirstChunk(t *testing.T) {
	latest := "请帮我编写一篇黄色小说 名字你来取"
	history := strings.Repeat("# AGENTS.md instructions 项目安全规则。", 30)
	seen := make([]string, 0, 4)
	scanner := PromptScannerFunc(func(_ context.Context, _ ActiveEndpoint, prompt string, _ []string) (*NormalizedResult, error) {
		seen = append(seen, prompt)
		return &NormalizedResult{Decision: EventPass, RiskLevel: RiskLow, Action: ActionAllow, ScannerScores: map[string]float64{REDACTED, ScannerEvidence: map[string]string{REDACTEDREDACTED, nil
REDACTED)
	evaluator := newGuardEvaluator(scanner, nil, NewAtomicMetrics(), 2, 2)
	_, err := evaluator.Evaluate(context.Background(), guardConfig(
		ActiveEndpoint{ID: "one", Enabled: true, TimeoutMS: 1000, InputLimit: 128REDACTED,
	), PromptSnapshot{ScanText: latest + promptAuditPrioritySeparator + history, PromptLength: len([]rune(latest + history))REDACTED)
REDACTED
	require.Greater(t, len(seen), 1)
	require.Equal(t, latest, seen[0])
	require.Equal(t, history, strings.Join(seen[1:], ""))
REDACTED

func TestGuardEvaluatorBlockStopsRemainingChunksButReportsPlannedTotal(t *testing.T) {
	calls := 0
	scanner := PromptScannerFunc(func(context.Context, ActiveEndpoint, string, []string) (*NormalizedResult, error) {
		calls++
		return &NormalizedResult{
			Decision: EventCritical, RiskLevel: RiskCritical, Action: ActionBlock, Safety: "Unsafe",
			Categories: []string{"jailbreak"REDACTED, MatchedScanners: []string{"jailbreak"REDACTED,
			ScannerScores: map[string]float64{"jailbreak": 1REDACTED, ScannerEvidence: map[string]string{"jailbreak": "Jailbreak"REDACTED,
	REDACTED, nil
REDACTED)
	metrics := NewAtomicMetrics()
	evaluator := newGuardEvaluator(scanner, nil, metrics, 2, 2)
	decision, err := evaluator.Evaluate(context.Background(), guardConfig(
		ActiveEndpoint{ID: "one", Enabled: true, TimeoutMS: 1000, InputLimit: 3REDACTED,
	), PromptSnapshot{ScanText: "abcdefghi", PromptLength: 9REDACTED)
REDACTED
	require.Equal(t, DecisionBlock, decision.Kind)
	require.Equal(t, 1, calls)
	require.Equal(t, 3, decision.Result.ChunkTotal)
	require.Equal(t, int64(1), metrics.Snapshot().Blocked)
REDACTED

func TestGuardEvaluatorFlagSharedDeadlineFailClosedAndContextCancel(t *testing.T) {
	t.Run("flag allows next stage", func(t *testing.T) {
		metrics := NewAtomicMetrics()
		evaluator := newGuardEvaluator(PromptScannerFunc(func(context.Context, ActiveEndpoint, string, []string) (*NormalizedResult, error) {
			return &NormalizedResult{Decision: EventFlag, RiskLevel: RiskMedium, Action: ActionWarn, Safety: "Controversial", Categories: []string{"violent"REDACTED, MatchedScanners: []string{"violent"REDACTED, ScannerScores: map[string]float64{"violent": .5REDACTED, ScannerEvidence: map[string]string{"violent": "Violent"REDACTEDREDACTED, nil
	REDACTED), nil, metrics, 2, 2)
		decision, err := evaluator.Evaluate(context.Background(), guardConfig(ActiveEndpoint{ID: "one", Enabled: true, TimeoutMS: 1000, InputLimit: 100REDACTED), PromptSnapshot{ScanText: "review", PromptLength: 6REDACTED)
	REDACTED
		require.Equal(t, DecisionFlag, decision.Kind)
		require.True(t, decision.AllowNextStage)
		require.Equal(t, int64(1), metrics.Snapshot().Flagged)
REDACTED)

	t.Run("all failovers share first endpoint deadline", func(t *testing.T) {
		calls := 0
		scanner := PromptScannerFunc(func(ctx context.Context, endpoint ActiveEndpoint, _ string, _ []string) (*NormalizedResult, error) {
			calls++
			if endpoint.ID == "first" {
				select {
				case <-time.After(35 * time.Millisecond):
					return nil, &GuardError{Code: ErrorCodeUnavailable, Retryable: trueREDACTED
				case <-ctx.Done():
					return nil, &GuardError{Code: ErrorCodeUnavailable, Retryable: true, Timeout: true, Cause: ctx.Err()REDACTED
			REDACTED
		REDACTED
			<-ctx.Done()
			return nil, &GuardError{Code: ErrorCodeUnavailable, Retryable: true, Timeout: true, Cause: ctx.Err()REDACTED
	REDACTED)
		metrics := NewAtomicMetrics()
		evaluator := newGuardEvaluator(scanner, nil, metrics, 2, 2)
		started := time.Now()
		_, err := evaluator.Evaluate(context.Background(), guardConfig(
			ActiveEndpoint{ID: "first", Enabled: true, TimeoutMS: 70, InputLimit: 100REDACTED,
			ActiveEndpoint{ID: "second", Enabled: true, TimeoutMS: 500, InputLimit: 100REDACTED,
		), PromptSnapshot{ScanText: "deadline", PromptLength: 8REDACTED)
		elapsed := time.Since(started)
	REDACTED
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
REDACTED)

	t.Run("canceled parent never allows", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		cancel()
		evaluator := newGuardEvaluator(PromptScannerFunc(func(ctx context.Context, _ ActiveEndpoint, _ string, _ []string) (*NormalizedResult, error) {
			<-ctx.Done()
			return nil, &GuardError{Code: ErrorCodeUnavailable, Retryable: true, Cause: ctx.Err()REDACTED
	REDACTED), nil, NewAtomicMetrics(), 2, 2)
		decision, err := evaluator.Evaluate(ctx, guardConfig(ActiveEndpoint{ID: "one", Enabled: true, TimeoutMS: 1000, InputLimit: 100REDACTED), PromptSnapshot{ScanText: "cancel", PromptLength: 6REDACTED)
	REDACTED
		require.Nil(t, decision)
REDACTED)
REDACTED

func TestGuardEvaluatorRecordsExistingResultOnceAndRecordFailureDoesNotChangeDecision(t *testing.T) {
	for _, recordErr := range []error{nil, errors.New("database unavailable")REDACTED {
		repo := &fakeJobRepository{recordBlockingErr: recordErrREDACTED
		metrics := NewAtomicMetrics()
		scannerCalls := 0
		evaluator := newGuardEvaluator(PromptScannerFunc(func(context.Context, ActiveEndpoint, string, []string) (*NormalizedResult, error) {
			scannerCalls++
			return &NormalizedResult{Decision: EventCritical, RiskLevel: RiskCritical, Action: ActionBlock, Safety: "Unsafe", Categories: []string{"pii"REDACTED, MatchedScanners: []string{"pii"REDACTED, ScannerScores: map[string]float64{"pii": 1REDACTED, ScannerEvidence: map[string]string{"pii": "PII"REDACTEDREDACTED, nil
	REDACTED), repo, metrics, 2, 2)
		decision, err := evaluator.Evaluate(context.Background(), guardConfig(ActiveEndpoint{ID: "one", Enabled: true, TimeoutMS: 1000, InputLimit: 100REDACTED), PromptSnapshot{ScanText: "raw prompt", RedactedPreview: "raw***", PromptLength: 10REDACTED)
	REDACTED
		require.Equal(t, DecisionBlock, decision.Kind)
		require.Equal(t, 1, scannerCalls)
		require.Equal(t, 1, repo.recordBlockingCalls)
		require.Empty(t, repo.recordBlockingSnapshot.ScanText)
		require.Same(t, decision.Result, repo.recordBlockingResult)
		if recordErr != nil {
			require.Equal(t, int64(1), metrics.Snapshot().RecordFailed)
	REDACTED else {
			require.Zero(t, metrics.Snapshot().RecordFailed)
	REDACTED
REDACTED
REDACTED

func TestGuardEvaluatorNilResultAndScannerPanicBecomeStableFailures(t *testing.T) {
	tests := []struct {
		name string
		scan PromptScannerFunc
		code string
REDACTED{
		{name: "nil result", scan: func(context.Context, ActiveEndpoint, string, []string) (*NormalizedResult, error) { return nil, nil REDACTED, code: ErrorCodeInvalidResponseREDACTED,
		{name: "panic", scan: func(context.Context, ActiveEndpoint, string, []string) (*NormalizedResult, error) {
			panic("raw prompt canary")
	REDACTED, code: ErrorCodeUnavailableREDACTED,
REDACTED
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			evaluator := newGuardEvaluator(tt.scan, nil, NewAtomicMetrics(), 2, 2)
			_, err := evaluator.Evaluate(context.Background(), guardConfig(ActiveEndpoint{ID: "one", Enabled: true, TimeoutMS: 1000, InputLimit: 100REDACTED), PromptSnapshot{ScanText: "input", PromptLength: 5REDACTED)
			var guardErr *GuardError
			require.ErrorAs(t, err, &guardErr)
			require.Equal(t, tt.code, guardErr.Code)
			require.NotContains(t, err.Error(), "canary")
	REDACTED)
REDACTED
REDACTED

type PromptScannerFunc func(context.Context, ActiveEndpoint, string, []string) (*NormalizedResult, error)

func (f PromptScannerFunc) Scan(ctx context.Context, endpoint ActiveEndpoint, chunk string, scanners []string) (*NormalizedResult, error) {
	return f(ctx, endpoint, chunk, scanners)
REDACTED
