package securityaudit

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

type fixedClock struct{ now time.Time REDACTED

func (c fixedClock) Now() time.Time { return c.now REDACTED

type advancingClock struct {
	mu   sync.Mutex
	now  time.Time
	step time.Duration
REDACTED

func (c *advancingClock) Now() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.now = c.now.Add(c.step)
	return c.now
REDACTED

type fakeConfigStore struct {
	cfg    ActiveConfig
	active bool
REDACTED

func (s *fakeConfigStore) Start(context.Context) error    { return nil REDACTED
func (s *fakeConfigStore) Shutdown(context.Context) error { return nil REDACTED
func (s *fakeConfigStore) Active() (ActiveConfig, bool)   { return cloneActiveConfig(s.cfg), s.active REDACTED
func (s *fakeConfigStore) EffectiveMode() Mode {
	if s.BlockingActivationDegraded() {
		return ModeBlocking
REDACTED
	if !s.active {
		return ModeOff
REDACTED
	return s.cfg.EffectiveMode()
REDACTED
func (s *fakeConfigStore) BlockingActivationDegraded() bool { return false REDACTED
func (s *fakeConfigStore) Public() (PublicConfig, error)    { return PublicConfig{REDACTED, nil REDACTED
func (s *fakeConfigStore) Save(context.Context, UpdateConfigRequest, int64) (PublicConfig, error) {
	return PublicConfig{REDACTED, nil
REDACTED
func (s *fakeConfigStore) RuntimeState() (int64, int64, *time.Time, string) {
	return s.cfg.ConfigVersion, s.cfg.ConfigVersion, nil, ""
REDACTED
func (s *fakeConfigStore) Encrypt(value string) (string, error) { return value, nil REDACTED
func (s *fakeConfigStore) Decrypt(value string) (string, error) { return value, nil REDACTED

type fakeJobRepository struct {
	mu sync.Mutex

	trace       *[]string
	createJob   *Job
	createErr   error
	publishErr  error
	refreshErr  error
	completeErr error
	retryErr    error
	failErr     error

	createdSnapshot PromptSnapshot
	markedCode      string
	completedResult *NormalizedResult
	completedStore  bool
	completeCount   int
	eventCount      int
	retryAt         time.Time
	retryCode       string
	retried         int
	failedCode      string
	failed          int
	refreshes       int

	claimQueue []*Job

	recordBlockingCalls    int
	recordBlockingSnapshot PromptSnapshot
	recordBlockingResult   *NormalizedResult
	recordBlockingErr      error
REDACTED

func (r *fakeJobRepository) record(value string) {
	if r.trace != nil {
		*r.trace = append(*r.trace, value)
REDACTED
REDACTED

func (r *fakeJobRepository) CreateStagingWithCapacity(_ context.Context, snapshot PromptSnapshot, _ int64, _, _ int) (*Job, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.record("create_staging")
	r.createdSnapshot = snapshot
	if r.createErr != nil {
		return nil, r.createErr
REDACTED
	if r.createJob == nil {
		r.createJob = &Job{ID: 1, Snapshot: snapshotREDACTED
REDACTED
	return r.createJob, nil
REDACTED
func (r *fakeJobRepository) PublishQueued(context.Context, int64) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.record("publish_queued")
	return r.publishErr
REDACTED
func (r *fakeJobRepository) MarkStagingFailed(_ context.Context, _ int64, code, _ string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.record("mark_staging_failed")
	r.markedCode = code
	return nil
REDACTED
func (r *fakeJobRepository) ClaimNextJob(context.Context, time.Time) (*Job, bool, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if len(r.claimQueue) == 0 {
		return nil, false, nil
REDACTED
	job := r.claimQueue[0]
	r.claimQueue = r.claimQueue[1:]
	return job, true, nil
REDACTED
func (r *fakeJobRepository) RefreshLease(context.Context, int64, int64, time.Time) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.refreshes++
	return r.refreshErr
REDACTED
func (r *fakeJobRepository) Complete(_ context.Context, _ *Job, result *NormalizedResult, storePass bool) (*Event, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.completeCount++
	r.completedResult, r.completedStore = result, storePass
	if r.completeErr != nil {
		return nil, r.completeErr
REDACTED
	if result.Decision == EventPass && !storePass {
		return nil, nil
REDACTED
	r.eventCount++
	return &Event{ID: 99, Decision: result.DecisionREDACTED, nil
REDACTED
func (r *fakeJobRepository) Retry(_ context.Context, _, _ int64, next time.Time, code, _ string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.retried++
	r.retryAt, r.retryCode = next, code
	return r.retryErr
REDACTED
func (r *fakeJobRepository) Fail(_ context.Context, _, _ int64, code, _ string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.failed++
	r.failedCode = code
	return r.failErr
REDACTED
func (r *fakeJobRepository) ReclaimStale(context.Context, time.Time, time.Time, int) (int64, error) {
	return 0, nil
REDACTED
func (r *fakeJobRepository) QueueStats(context.Context) (QueueStats, error) { return QueueStats{REDACTED, nil REDACTED
func (r *fakeJobRepository) RecordBlocking(_ context.Context, snapshot PromptSnapshot, _ int64, result *NormalizedResult, _ bool) (*Event, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.recordBlockingCalls++
	r.recordBlockingSnapshot, r.recordBlockingResult = snapshot, result
	return nil, r.recordBlockingErr
REDACTED

type fakePayloadStore struct {
	mu sync.Mutex

	trace     *[]string
	values    map[int64]string
	setErr    error
	getErr    error
	deleteErr error
	pingErr   error
	setTTL    time.Duration
	deleted   []int64
REDACTED

func (s *fakePayloadStore) Set(_ context.Context, jobID int64, value string, ttl time.Duration) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.trace != nil {
		*s.trace = append(*s.trace, "payload_set")
REDACTED
	if s.setErr != nil {
		return s.setErr
REDACTED
	if s.values == nil {
		s.values = map[int64]string{REDACTED
REDACTED
	s.values[jobID], s.setTTL = value, ttl
	return nil
REDACTED
func (s *fakePayloadStore) Get(_ context.Context, jobID int64) (string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.getErr != nil {
		return "", s.getErr
REDACTED
	value, ok := s.values[jobID]
	if !ok {
		return "", errors.New("missing")
REDACTED
	return value, nil
REDACTED
func (s *fakePayloadStore) Delete(_ context.Context, jobID int64) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.trace != nil {
		*s.trace = append(*s.trace, "payload_delete")
REDACTED
	s.deleted = append(s.deleted, jobID)
	delete(s.values, jobID)
	return s.deleteErr
REDACTED
func (s *fakePayloadStore) Ping(context.Context) error { return s.pingErr REDACTED

func asyncConfig() ActiveConfig {
	return ActiveConfig{
		RiskControlEnabled: true, Enabled: true, BlockingEnabled: false, Strategy: "priority",
		WorkerCount: 1, QueueCapacity: 8, Scanners: []string{"pii"REDACTED, AllGroups: true, ConfigVersion: 7,
		Endpoints: []ActiveEndpoint{{ID: "guard", Enabled: true, TimeoutMS: 1000, InputLimit: 3REDACTEDREDACTED,
REDACTED
REDACTED

func asyncRequest() Request {
	return Request{RequestID: "request-async", Protocol: "openai_chat_completions", Body: []byte(`{"messages":[{"role":"user","content":"payload canary text"REDACTED]REDACTED`)REDACTED
REDACTED

func TestEnqueuerStagingPayloadPublishProtocolAndFailureCleanup(t *testing.T) {
	t.Run("success", func(t *testing.T) {
		trace := []string{REDACTED
		repo := &fakeJobRepository{trace: &trace, createJob: &Job{ID: 41REDACTEDREDACTED
		payload := &fakePayloadStore{trace: &trace, values: map[int64]string{REDACTEDREDACTED
		enqueuer := NewEnqueuer(&fakeConfigStore{cfg: asyncConfig(), active: trueREDACTED, repo, payload)
		require.NoError(t, enqueuer.Enqueue(context.Background(), asyncRequest()))
		require.Equal(t, []string{"create_staging", "payload_set", "publish_queued"REDACTED, trace)
		require.Empty(t, repo.createdSnapshot.ScanText)
		require.Equal(t, "payload canary text", payload.values[41])
		require.Equal(t, DefaultPayloadTTL, payload.setTTL)
REDACTED)

	t.Run("queue admission failures never touch payload", func(t *testing.T) {
		for _, createErr := range []error{ErrQueueFull, ErrQueueAdmissionBusy, errors.New("database down")REDACTED {
			trace := []string{REDACTED
			repo := &fakeJobRepository{trace: &trace, createErr: createErrREDACTED
			payload := &fakePayloadStore{trace: &trace, values: map[int64]string{REDACTEDREDACTED
			err := NewEnqueuer(&fakeConfigStore{cfg: asyncConfig(), active: trueREDACTED, repo, payload).Enqueue(context.Background(), asyncRequest())
			require.ErrorIs(t, err, createErr)
			require.Equal(t, []string{"create_staging"REDACTED, trace)
	REDACTED
REDACTED)

	t.Run("payload failure marks staging failed", func(t *testing.T) {
		trace := []string{REDACTED
		repo := &fakeJobRepository{trace: &trace, createJob: &Job{ID: 42REDACTEDREDACTED
		payload := &fakePayloadStore{trace: &trace, values: map[int64]string{REDACTED, setErr: errors.New("redis down")REDACTED
		err := NewEnqueuer(&fakeConfigStore{cfg: asyncConfig(), active: trueREDACTED, repo, payload).Enqueue(context.Background(), asyncRequest())
	REDACTED
		require.Equal(t, []string{"create_staging", "payload_set", "mark_staging_failed"REDACTED, trace)
		require.Equal(t, "payload_store_failed", repo.markedCode)
REDACTED)

	t.Run("publish failure removes payload and marks staging failed", func(t *testing.T) {
		trace := []string{REDACTED
		repo := &fakeJobRepository{trace: &trace, createJob: &Job{ID: 43REDACTED, publishErr: errors.New("publish down")REDACTED
		payload := &fakePayloadStore{trace: &trace, values: map[int64]string{REDACTEDREDACTED
		err := NewEnqueuer(&fakeConfigStore{cfg: asyncConfig(), active: trueREDACTED, repo, payload).Enqueue(context.Background(), asyncRequest())
	REDACTED
		require.Equal(t, []string{"create_staging", "payload_set", "publish_queued", "payload_delete", "mark_staging_failed"REDACTED, trace)
		require.Equal(t, "queue_publish_failed", repo.markedCode)
		require.NotContains(t, payload.values, int64(43))
REDACTED)
REDACTED

func TestEnqueuerSkipsOffOutOfScopeAndNoText(t *testing.T) {
	tests := []struct {
		name string
		cfg  ActiveConfig
		req  Request
REDACTED{
		{name: "off", cfg: ActiveConfig{REDACTED, req: asyncRequest()REDACTED,
		{name: "out of scope", cfg: func() ActiveConfig {
			cfg := asyncConfig()
			cfg.AllGroups = false
			cfg.GroupIDs = []int64{9REDACTED
			return cfg
	REDACTED(), req: asyncRequest()REDACTED,
		{name: "no user text", cfg: asyncConfig(), req: Request{Protocol: "openai_chat_completions", Body: []byte(`{"messages":[{"role":"function","content":"not audited"REDACTED]REDACTED`)REDACTEDREDACTED,
REDACTED
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repo := &fakeJobRepository{REDACTED
			err := NewEnqueuer(&fakeConfigStore{cfg: tt.cfg, active: trueREDACTED, repo, &fakePayloadStore{REDACTED).Enqueue(context.Background(), tt.req)
		REDACTED
			require.Zero(t, repo.createdSnapshot.MessageCount)
	REDACTED)
REDACTED
REDACTED

func TestEnqueuerRecordsAcceptedDroppedAndSkippedMetrics(t *testing.T) {
	t.Run("accepted increments enqueued", func(t *testing.T) {
		metrics := NewAtomicMetrics()
		repo := &fakeJobRepository{createJob: &Job{ID: 44REDACTEDREDACTED
		payload := &fakePayloadStore{values: map[int64]string{REDACTEDREDACTED

		require.NoError(t, NewEnqueuer(
			&fakeConfigStore{cfg: asyncConfig(), active: trueREDACTED,
			repo,
			payload,
			metrics,
		).Enqueue(context.Background(), asyncRequest()))

		require.Equal(t, AuditMetricsSnapshot{Enqueued: 1REDACTED, metrics.AuditSnapshot())
REDACTED)

	t.Run("queue full increments dropped", func(t *testing.T) {
		metrics := NewAtomicMetrics()
		repo := &fakeJobRepository{createErr: ErrQueueFullREDACTED

		err := NewEnqueuer(
			&fakeConfigStore{cfg: asyncConfig(), active: trueREDACTED,
			repo,
			&fakePayloadStore{REDACTED,
			metrics,
		).Enqueue(context.Background(), asyncRequest())

		require.ErrorIs(t, err, ErrQueueFull)
		require.Equal(t, AuditMetricsSnapshot{Dropped: 1REDACTED, metrics.AuditSnapshot())
REDACTED)

	t.Run("skipped request does not increment dropped", func(t *testing.T) {
		metrics := NewAtomicMetrics()

		require.NoError(t, NewEnqueuer(
			&fakeConfigStore{cfg: ActiveConfig{REDACTED, active: trueREDACTED,
			&fakeJobRepository{REDACTED,
			&fakePayloadStore{REDACTED,
			metrics,
		).Enqueue(context.Background(), asyncRequest()))

		require.Equal(t, AuditMetricsSnapshot{REDACTED, metrics.AuditSnapshot())
REDACTED)
REDACTED

func workerJob(attempts, maxAttempts int) *Job {
	return &Job{ID: 51, ClaimVersion: 3, Attempts: attempts, MaxAttempts: maxAttempts, ConfigVersion: 7,
		Snapshot: PromptSnapshot{RequestID: "worker-request", PromptLength: 6, RedactedPreview: "red***"REDACTEDREDACTED
REDACTED

func TestWorkerCompletesPassWithoutEventRefreshesEveryChunkAndDeletesPayload(t *testing.T) {
	repo := &fakeJobRepository{REDACTED
	payload := &fakePayloadStore{values: map[int64]string{51: "abcdef"REDACTEDREDACTED
	scannerCalls := 0
	scanner := PromptScannerFunc(func(_ context.Context, endpoint ActiveEndpoint, chunk string, _ []string) (*NormalizedResult, error) {
		scannerCalls++
		return &NormalizedResult{Decision: EventPass, RiskLevel: RiskLow, Action: ActionAllow, Safety: "Safe", Categories: []string{REDACTED, MatchedScanners: []string{REDACTED, ScannerScores: map[string]float64{REDACTED, ScannerEvidence: map[string]string{REDACTED, GuardEndpointID: endpoint.IDREDACTED, nil
REDACTED)
	metrics := NewAtomicMetrics()
	runner := NewRunner(&fakeConfigStore{cfg: asyncConfig(), active: trueREDACTED, repo, payload, scanner, metrics)
	runner.clock = fixedClock{now: time.Unix(100, 0).UTC()REDACTED
	require.NoError(t, runner.processJob(context.Background(), 0, asyncConfig(), workerJob(1, 3)))
	require.Equal(t, 2, scannerCalls)
	require.Equal(t, 2, repo.refreshes)
	require.NotNil(t, repo.completedResult)
	require.Equal(t, EventPass, repo.completedResult.Decision)
	require.False(t, repo.completedStore)
	require.Equal(t, []int64{51REDACTED, payload.deleted)
	require.Equal(t, int64(1), metrics.Snapshot().Total)
	require.Equal(t, int64(1), metrics.Snapshot().Allowed)
REDACTED

func TestWorkerRetryBackoffTerminalFailureAndFailover(t *testing.T) {
	now := time.Unix(200, 0).UTC()
	for _, tt := range []struct {
		name        string
		attempts    int
		maxAttempts int
		err         *GuardError
		wantRetry   bool
		wantBackoff time.Duration
REDACTED{
		{name: "first retry", attempts: 1, maxAttempts: 3, err: &GuardError{Code: ErrorCodeUnavailable, Retryable: trueREDACTED, wantRetry: true, wantBackoff: 5 * time.SecondREDACTED,
		{name: "second retry", attempts: 2, maxAttempts: 3, err: &GuardError{Code: ErrorCodeUnavailable, Retryable: trueREDACTED, wantRetry: true, wantBackoff: 30 * time.SecondREDACTED,
		{name: "third retry", attempts: 3, maxAttempts: 4, err: &GuardError{Code: ErrorCodeUnavailable, Retryable: trueREDACTED, wantRetry: true, wantBackoff: 2 * time.MinuteREDACTED,
		{name: "max attempts", attempts: 3, maxAttempts: 3, err: &GuardError{Code: ErrorCodeUnavailable, Retryable: trueREDACTEDREDACTED,
		{name: "invalid terminal", attempts: 1, maxAttempts: 3, err: &GuardError{Code: ErrorCodeInvalidResponse, Retryable: falseREDACTEDREDACTED,
REDACTED {
		t.Run(tt.name, func(t *testing.T) {
			repo := &fakeJobRepository{REDACTED
			payload := &fakePayloadStore{values: map[int64]string{51: "abc"REDACTEDREDACTED
			metrics := NewAtomicMetrics()
			runner := NewRunner(&fakeConfigStore{cfg: asyncConfig(), active: trueREDACTED, repo, payload, PromptScannerFunc(func(context.Context, ActiveEndpoint, string, []string) (*NormalizedResult, error) {
				return nil, tt.err
		REDACTED), metrics)
			runner.clock = fixedClock{now: nowREDACTED
			err := runner.processJob(context.Background(), 0, asyncConfig(), workerJob(tt.attempts, tt.maxAttempts))
		REDACTED
			if tt.wantRetry {
				require.Equal(t, 1, repo.retried)
				require.Equal(t, now.Add(tt.wantBackoff), repo.retryAt)
				require.Empty(t, payload.deleted)
		REDACTED else {
				require.Equal(t, 1, repo.failed)
				require.Equal(t, tt.err.Code, repo.failedCode)
				require.Equal(t, []int64{51REDACTED, payload.deleted)
		REDACTED
			snapshot := metrics.Snapshot()
			require.Equal(t, int64(1), snapshot.Total)
			if tt.err.Code == ErrorCodeInvalidResponse {
				require.Equal(t, int64(1), snapshot.Invalid)
		REDACTED else {
				require.Equal(t, int64(1), snapshot.Unavailable)
		REDACTED
	REDACTED)
REDACTED

	repo := &fakeJobRepository{REDACTED
	payload := &fakePayloadStore{values: map[int64]string{51: "abc"REDACTEDREDACTED
	metrics := NewAtomicMetrics()
	scanner := PromptScannerFunc(func(_ context.Context, endpoint ActiveEndpoint, _ string, _ []string) (*NormalizedResult, error) {
		if endpoint.ID == "first" {
			return nil, &GuardError{Code: ErrorCodeUnavailable, Retryable: trueREDACTED
	REDACTED
		return integrationResult(EventPass), nil
REDACTED)
	cfg := asyncConfig()
	cfg.Endpoints = []ActiveEndpoint{{ID: "first", Enabled: true, InputLimit: 10REDACTED, {ID: "second", Enabled: true, InputLimit: 10REDACTEDREDACTED
	runner := NewRunner(&fakeConfigStore{cfg: cfg, active: trueREDACTED, repo, payload, scanner, metrics)
	require.NoError(t, runner.processJob(context.Background(), 0, cfg, workerJob(1, 3)))
	require.Equal(t, int64(1), metrics.Snapshot().Failovers)
REDACTED

func TestWorkerPanicLeaseLossAndLifecycleAreContained(t *testing.T) {
	t.Run("panic", func(t *testing.T) {
		repo := &fakeJobRepository{REDACTED
		payload := &fakePayloadStore{values: map[int64]string{51: "abc"REDACTEDREDACTED
		runner := NewRunner(&fakeConfigStore{cfg: asyncConfig(), active: trueREDACTED, repo, payload, PromptScannerFunc(func(context.Context, ActiveEndpoint, string, []string) (*NormalizedResult, error) {
			panic("scanner panic canary")
	REDACTED), NewAtomicMetrics())
		require.NotPanics(t, func() { runner.processSafely(context.Background(), 0, asyncConfig(), workerJob(1, 3)) REDACTED)
		_, _, failed, _, _, code, message := runner.Snapshot()
		require.Equal(t, int64(1), failed)
		require.Equal(t, "worker_panic", code)
		require.NotContains(t, message, "canary")
		require.Equal(t, 1, repo.failed)
REDACTED)

	t.Run("lease loss", func(t *testing.T) {
		repo := &fakeJobRepository{refreshErr: ErrLeaseLostREDACTED
		payload := &fakePayloadStore{values: map[int64]string{51: "abc"REDACTEDREDACTED
		calls := 0
		runner := NewRunner(&fakeConfigStore{cfg: asyncConfig(), active: trueREDACTED, repo, payload, PromptScannerFunc(func(context.Context, ActiveEndpoint, string, []string) (*NormalizedResult, error) {
			calls++
			return integrationResult(EventPass), nil
	REDACTED), NewAtomicMetrics())
		require.ErrorIs(t, runner.processJob(context.Background(), 0, asyncConfig(), workerJob(1, 3)), ErrLeaseLost)
		require.Zero(t, calls)
		require.Zero(t, repo.retried)
		require.Zero(t, repo.failed)
REDACTED)

	t.Run("start and shutdown", func(t *testing.T) {
		cfg := asyncConfig()
		cfg.Enabled = false
		configStore := &fakeConfigStore{cfg: cfg, active: trueREDACTED
		repo := &fakeJobRepository{REDACTED
		payload := &fakePayloadStore{pingErr: errors.New("redis unavailable")REDACTED
		runner := NewRunner(configStore, repo, payload, PromptScannerFunc(func(context.Context, ActiveEndpoint, string, []string) (*NormalizedResult, error) {
			return integrationResult(EventPass), nil
	REDACTED), NewAtomicMetrics())
		require.NoError(t, runner.Start(context.Background()))
		require.NoError(t, runner.Start(context.Background()))
		_, _, _, _, _, code, _ := runner.Snapshot()
		require.Equal(t, "payload_store_unavailable", code)
		ctx, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()
		require.NoError(t, runner.Shutdown(ctx))
		require.NoError(t, runner.Shutdown(ctx))
REDACTED)

	t.Run("shutdown timeout is bounded", func(t *testing.T) {
		runner := &Runner{REDACTED
		release := make(chan struct{REDACTED)
		runner.wg.Add(1)
		go func() {
			defer runner.wg.Done()
			<-release
	REDACTED()
		ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
		defer cancel()
		require.ErrorIs(t, runner.Shutdown(ctx), context.DeadlineExceeded)
		close(release)
		ctx2, cancel2 := context.WithTimeout(context.Background(), time.Second)
		defer cancel2()
		require.NoError(t, runner.Shutdown(ctx2))
REDACTED)
REDACTED

func TestPromptAuditSyntheticAsyncBaseline(t *testing.T) {
	const totalRequests = 100
	cfg := asyncConfig()
	cfg.Endpoints[0].InputLimit = 256
	cfg.StorePassEvents = false
	repo := &fakeJobRepository{REDACTED
	payload := &fakePayloadStore{values: make(map[int64]string, totalRequests)REDACTED
	metrics := NewAtomicMetrics()
	knownBenignFindings := 0
	knownMaliciousBlocked := 0
	scanner := PromptScannerFunc(func(_ context.Context, endpoint ActiveEndpoint, chunk string, _ []string) (*NormalizedResult, error) {
		switch {
		case strings.HasPrefix(chunk, "benign"):
			return &NormalizedResult{Decision: EventPass, RiskLevel: RiskLow, Action: ActionAllow, Safety: "Safe", GuardEndpointID: endpoint.IDREDACTED, nil
		case strings.HasPrefix(chunk, "flag"):
			return &NormalizedResult{Decision: EventFlag, RiskLevel: RiskMedium, Action: ActionWarn, Safety: "Controversial", Categories: []string{"politically_sensitive_topics"REDACTED, GuardEndpointID: endpoint.IDREDACTED, nil
		case strings.HasPrefix(chunk, "block"):
			knownMaliciousBlocked++
			return &NormalizedResult{Decision: EventCritical, RiskLevel: RiskCritical, Action: ActionBlock, Safety: "Unsafe", Categories: []string{"jailbreak"REDACTED, GuardEndpointID: endpoint.IDREDACTED, nil
		case strings.HasPrefix(chunk, "invalid"):
			return nil, &GuardError{Code: ErrorCodeInvalidResponseREDACTED
		default:
			return nil, &GuardError{Code: ErrorCodeUnavailable, Retryable: true, Timeout: trueREDACTED
	REDACTED
REDACTED)
	runner := NewRunner(&fakeConfigStore{cfg: cfg, active: trueREDACTED, repo, payload, scanner, metrics)
	runner.clock = &advancingClock{now: time.Unix(1_000, 0).UTC(), step: time.MillisecondREDACTED

	for index := 1; index <= totalRequests; index++ {
		text := fmt.Sprintf("benign-%03d", index)
		switch {
		case index > 90 && index <= 95:
			text = fmt.Sprintf("flag-%03d", index)
		case index > 95 && index <= 98:
			text = fmt.Sprintf("block-%03d", index)
		case index == 99:
			text = "invalid-099"
		case index == 100:
			text = "timeout-100"
	REDACTED
		jobID := int64(index)
		payload.values[jobID] = text
		job := &Job{ID: jobID, ClaimVersion: 1, Attempts: 1, MaxAttempts: 1, ConfigVersion: cfg.ConfigVersion,
			Snapshot: PromptSnapshot{RequestID: fmt.Sprintf("baseline-%03d", index), PromptLength: len([]rune(text)), RedactedPreview: "synthetic"REDACTEDREDACTED
		err := runner.processJob(context.Background(), 0, cfg, job)
		if index <= 98 {
		REDACTED
	REDACTED else {
		REDACTED
	REDACTED
REDACTED

	snapshot := metrics.Snapshot()
	require.Equal(t, int64(totalRequests), snapshot.Total)
	require.Equal(t, int64(90), snapshot.Allowed)
	require.Equal(t, int64(5), snapshot.Flagged)
	require.Equal(t, int64(3), snapshot.Blocked)
	require.Equal(t, int64(1), snapshot.Invalid)
	require.Equal(t, int64(1), snapshot.Unavailable)
	require.Equal(t, int64(1), snapshot.Timeouts)
	require.Zero(t, knownBenignFindings)
	require.Equal(t, 3, knownMaliciousBlocked)
	require.Equal(t, 98, repo.completeCount)
	require.Equal(t, 8, repo.eventCount, "store_pass_events=false only grows events for flag/block fixtures")
	require.Positive(t, snapshot.LatencyP50MS)
	require.LessOrEqual(t, snapshot.LatencyP50MS, snapshot.LatencyP95MS)
	require.LessOrEqual(t, snapshot.LatencyP95MS, snapshot.LatencyP99MS)
	t.Logf("synthetic async baseline: p50=%dms p95=%dms p99=%dms failure_rate=2%% false_positive_rate=0%% event_growth=8/100", snapshot.LatencyP50MS, snapshot.LatencyP95MS, snapshot.LatencyP99MS)
REDACTED

func TestRequestCloneOwnsMutableInputs(t *testing.T) {
	groupID := int64(7)
	req := Request{Body: []byte("original"), GroupID: &groupIDREDACTED
	clone := req.Clone()
	clone.Body[0] = 'X'
	*clone.GroupID = 8
	require.Equal(t, []byte("original"), req.Body)
	require.Equal(t, int64(7), *req.GroupID)
	require.False(t, reflect.ValueOf(req.Body).Pointer() == reflect.ValueOf(clone.Body).Pointer())
REDACTED
