package service

import (
	"context"
	"errors"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
	"unicode/utf8"

	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
	"github.com/stretchr/testify/require"
)

type inMemoryIdempotencyRepo struct {
	mu     sync.Mutex
	nextID int64
	data   map[string]*IdempotencyRecord
REDACTED

func newInMemoryIdempotencyRepo() *inMemoryIdempotencyRepo {
	return &inMemoryIdempotencyRepo{
		nextID: 1,
		data:   make(map[string]*IdempotencyRecord),
REDACTED
REDACTED

func (r *inMemoryIdempotencyRepo) key(scope, hash string) string {
	return scope + "|" + hash
REDACTED

func cloneRecord(in *IdempotencyRecord) *IdempotencyRecord {
	if in == nil {
		return nil
REDACTED
	out := *in
	if in.ResponseStatus != nil {
		v := *in.ResponseStatus
		out.ResponseStatus = &v
REDACTED
	if in.ResponseBody != nil {
		v := *in.ResponseBody
		out.ResponseBody = &v
REDACTED
	if in.ErrorReason != nil {
		v := *in.ErrorReason
		out.ErrorReason = &v
REDACTED
	if in.LockedUntil != nil {
		v := *in.LockedUntil
		out.LockedUntil = &v
REDACTED
	return &out
REDACTED

func (r *inMemoryIdempotencyRepo) CreateProcessing(_ context.Context, record *IdempotencyRecord) (bool, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	k := r.key(record.Scope, record.IdempotencyKeyHash)
	if _, ok := r.data[k]; ok {
		return false, nil
REDACTED
	rec := cloneRecord(record)
	rec.ID = r.nextID
	rec.CreatedAt = time.Now()
	rec.UpdatedAt = rec.CreatedAt
	r.nextID++
	r.data[k] = rec
	record.ID = rec.ID
	record.CreatedAt = rec.CreatedAt
	record.UpdatedAt = rec.UpdatedAt
	return true, nil
REDACTED

func (r *inMemoryIdempotencyRepo) GetByScopeAndKeyHash(_ context.Context, scope, keyHash string) (*IdempotencyRecord, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	return cloneRecord(r.data[r.key(scope, keyHash)]), nil
REDACTED

func (r *inMemoryIdempotencyRepo) TryReclaim(_ context.Context, id int64, fromStatus string, now, newLockedUntil, newExpiresAt time.Time) (bool, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	for _, rec := range r.data {
		if rec.ID != id {
			continue
	REDACTED
		if rec.Status != fromStatus {
			return false, nil
	REDACTED
		if rec.LockedUntil != nil && rec.LockedUntil.After(now) {
			return false, nil
	REDACTED
		rec.Status = IdempotencyStatusProcessing
		rec.LockedUntil = &newLockedUntil
		rec.ExpiresAt = newExpiresAt
		rec.ErrorReason = nil
		rec.UpdatedAt = time.Now()
		return true, nil
REDACTED
	return false, nil
REDACTED

func (r *inMemoryIdempotencyRepo) ExtendProcessingLock(_ context.Context, id int64, requestFingerprint string, newLockedUntil, newExpiresAt time.Time) (bool, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	for _, rec := range r.data {
		if rec.ID != id {
			continue
	REDACTED
		if rec.Status != IdempotencyStatusProcessing || rec.RequestFingerprint != requestFingerprint {
			return false, nil
	REDACTED
		rec.LockedUntil = &newLockedUntil
		rec.ExpiresAt = newExpiresAt
		rec.UpdatedAt = time.Now()
		return true, nil
REDACTED
	return false, nil
REDACTED

func (r *inMemoryIdempotencyRepo) MarkSucceeded(_ context.Context, id int64, responseStatus int, responseBody string, expiresAt time.Time) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	for _, rec := range r.data {
		if rec.ID != id {
			continue
	REDACTED
		rec.Status = IdempotencyStatusSucceeded
		rec.LockedUntil = nil
		rec.ExpiresAt = expiresAt
		rec.UpdatedAt = time.Now()
		rec.ErrorReason = nil
		rec.ResponseStatus = &responseStatus
		rec.ResponseBody = &responseBody
		return nil
REDACTED
	return errors.New("record not found")
REDACTED

func (r *inMemoryIdempotencyRepo) MarkFailedRetryable(_ context.Context, id int64, errorReason string, lockedUntil, expiresAt time.Time) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	for _, rec := range r.data {
		if rec.ID != id {
			continue
	REDACTED
		rec.Status = IdempotencyStatusFailedRetryable
		rec.LockedUntil = &lockedUntil
		rec.ExpiresAt = expiresAt
		rec.UpdatedAt = time.Now()
		rec.ErrorReason = &errorReason
		return nil
REDACTED
	return errors.New("record not found")
REDACTED

func (r *inMemoryIdempotencyRepo) DeleteExpired(_ context.Context, now time.Time, _ int) (int64, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	var deleted int64
	for k, rec := range r.data {
		if !rec.ExpiresAt.After(now) {
			delete(r.data, k)
			deleted++
	REDACTED
REDACTED
	return deleted, nil
REDACTED

func TestIdempotencyCoordinator_RequireKey(t *testing.T) {
	resetIdempotencyMetricsForTest()
	repo := newInMemoryIdempotencyRepo()
	cfg := DefaultIdempotencyConfig()
	cfg.ObserveOnly = false
	coordinator := NewIdempotencyCoordinator(repo, cfg)

	_, err := coordinator.Execute(context.Background(), IdempotencyExecuteOptions{
		Scope:      "test.scope",
		Method:     "POST",
		Route:      "/test",
		ActorScope: "admin:1",
		RequireKey: true,
		Payload:    map[string]any{"a": 1REDACTED,
REDACTED, func(ctx context.Context) (any, error) {
		return map[string]any{"ok": trueREDACTED, nil
REDACTED)
REDACTED
	require.Equal(t, infraerrors.Code(err), infraerrors.Code(ErrIdempotencyKeyRequired))
REDACTED

func TestIdempotencyCoordinator_ReplaySucceededResult(t *testing.T) {
	resetIdempotencyMetricsForTest()
	repo := newInMemoryIdempotencyRepo()
	cfg := DefaultIdempotencyConfig()
	coordinator := NewIdempotencyCoordinator(repo, cfg)

	execCount := 0
	exec := func(ctx context.Context) (any, error) {
		execCount++
		return map[string]any{"count": execCountREDACTED, nil
REDACTED

	opts := IdempotencyExecuteOptions{
		Scope:          "test.scope",
		Method:         "POST",
		Route:          "/test",
		ActorScope:     "user:1",
		RequireKey:     true,
		IdempotencyKey: "case-1",
		Payload:        map[string]any{"a": 1REDACTED,
REDACTED

	first, err := coordinator.Execute(context.Background(), opts, exec)
REDACTED
	require.False(t, first.Replayed)

	second, err := coordinator.Execute(context.Background(), opts, exec)
REDACTED
	require.True(t, second.Replayed)
	require.Equal(t, 1, execCount, "second request should replay without executing business logic")

	metrics := GetIdempotencyMetricsSnapshot()
	require.Equal(t, uint64(1), metrics.ClaimTotal)
	require.Equal(t, uint64(1), metrics.ReplayTotal)
REDACTED

func TestIdempotencyCoordinator_ReclaimExpiredSucceededRecord(t *testing.T) {
	resetIdempotencyMetricsForTest()
	repo := newInMemoryIdempotencyRepo()
	coordinator := NewIdempotencyCoordinator(repo, DefaultIdempotencyConfig())

	opts := IdempotencyExecuteOptions{
		Scope:          "test.scope.expired",
		Method:         "POST",
		Route:          "/test/expired",
		ActorScope:     "user:99",
		RequireKey:     true,
		IdempotencyKey: "expired-case",
		Payload:        map[string]any{"k": "v"REDACTED,
REDACTED

	execCount := 0
	exec := func(ctx context.Context) (any, error) {
		execCount++
		return map[string]any{"count": execCountREDACTED, nil
REDACTED

	first, err := coordinator.Execute(context.Background(), opts, exec)
REDACTED
	require.NotNil(t, first)
	require.False(t, first.Replayed)
	require.Equal(t, 1, execCount)

	keyHash := HashIdempotencyKey(opts.IdempotencyKey)
	repo.mu.Lock()
	existing := repo.data[repo.key(opts.Scope, keyHash)]
	require.NotNil(t, existing)
	existing.ExpiresAt = time.Now().Add(-time.Second)
	repo.mu.Unlock()

	second, err := coordinator.Execute(context.Background(), opts, exec)
REDACTED
	require.NotNil(t, second)
	require.False(t, second.Replayed, "expired record should be reclaimed and execute business logic again")
	require.Equal(t, 2, execCount)

	third, err := coordinator.Execute(context.Background(), opts, exec)
REDACTED
	require.NotNil(t, third)
	require.True(t, third.Replayed)
	payload, ok := third.Data.(map[string]any)
	require.True(t, ok)
	require.Equal(t, float64(2), payload["count"])

	metrics := GetIdempotencyMetricsSnapshot()
	require.GreaterOrEqual(t, metrics.ClaimTotal, uint64(2))
	require.GreaterOrEqual(t, metrics.ReplayTotal, uint64(1))
REDACTED

func TestIdempotencyCoordinator_SameKeyDifferentPayloadConflict(t *testing.T) {
	resetIdempotencyMetricsForTest()
	repo := newInMemoryIdempotencyRepo()
	cfg := DefaultIdempotencyConfig()
	coordinator := NewIdempotencyCoordinator(repo, cfg)

	_, err := coordinator.Execute(context.Background(), IdempotencyExecuteOptions{
		Scope:          "test.scope",
		Method:         "POST",
		Route:          "/test",
		ActorScope:     "user:1",
		RequireKey:     true,
		IdempotencyKey: "case-2",
		Payload:        map[string]any{"a": 1REDACTED,
REDACTED, func(ctx context.Context) (any, error) {
		return map[string]any{"ok": trueREDACTED, nil
REDACTED)
REDACTED

	_, err = coordinator.Execute(context.Background(), IdempotencyExecuteOptions{
		Scope:          "test.scope",
		Method:         "POST",
		Route:          "/test",
		ActorScope:     "user:1",
		RequireKey:     true,
		IdempotencyKey: "case-2",
		Payload:        map[string]any{"a": 2REDACTED,
REDACTED, func(ctx context.Context) (any, error) {
		return map[string]any{"ok": trueREDACTED, nil
REDACTED)
REDACTED
	require.Equal(t, infraerrors.Code(err), infraerrors.Code(ErrIdempotencyKeyConflict))

	metrics := GetIdempotencyMetricsSnapshot()
	require.Equal(t, uint64(1), metrics.ConflictTotal)
REDACTED

func TestIdempotencyCoordinator_BackoffAfterRetryableFailure(t *testing.T) {
	resetIdempotencyMetricsForTest()
	repo := newInMemoryIdempotencyRepo()
	cfg := DefaultIdempotencyConfig()
	cfg.FailedRetryBackoff = 2 * time.Second
	coordinator := NewIdempotencyCoordinator(repo, cfg)

	opts := IdempotencyExecuteOptions{
		Scope:          "test.scope",
		Method:         "POST",
		Route:          "/test",
		ActorScope:     "user:1",
		RequireKey:     true,
		IdempotencyKey: "case-3",
		Payload:        map[string]any{"a": 1REDACTED,
REDACTED

	_, err := coordinator.Execute(context.Background(), opts, func(ctx context.Context) (any, error) {
		return nil, infraerrors.InternalServer("UPSTREAM_ERROR", "upstream error")
REDACTED)
REDACTED

	_, err = coordinator.Execute(context.Background(), opts, func(ctx context.Context) (any, error) {
		return map[string]any{"ok": trueREDACTED, nil
REDACTED)
REDACTED
	require.Equal(t, infraerrors.Code(err), infraerrors.Code(ErrIdempotencyRetryBackoff))
	require.Greater(t, RetryAfterSecondsFromError(err), 0)

	metrics := GetIdempotencyMetricsSnapshot()
	require.GreaterOrEqual(t, metrics.RetryBackoffTotal, uint64(2))
	require.GreaterOrEqual(t, metrics.ConflictTotal, uint64(1))
	require.GreaterOrEqual(t, metrics.ProcessingDurationCount, uint64(1))
REDACTED

func TestIdempotencyCoordinator_ConcurrentSameKeySingleSideEffect(t *testing.T) {
	resetIdempotencyMetricsForTest()
	repo := newInMemoryIdempotencyRepo()
	cfg := DefaultIdempotencyConfig()
	cfg.ProcessingTimeout = 2 * time.Second
	coordinator := NewIdempotencyCoordinator(repo, cfg)

	opts := IdempotencyExecuteOptions{
		Scope:          "test.scope.concurrent",
		Method:         "POST",
		Route:          "/test/concurrent",
		ActorScope:     "user:7",
		RequireKey:     true,
		IdempotencyKey: "concurrent-case",
		Payload:        map[string]any{"v": 1REDACTED,
REDACTED

	var execCount int32
	var wg sync.WaitGroup
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, _ = coordinator.Execute(context.Background(), opts, func(ctx context.Context) (any, error) {
				atomic.AddInt32(&execCount, 1)
				time.Sleep(80 * time.Millisecond)
				return map[string]any{"ok": trueREDACTED, nil
		REDACTED)
	REDACTED()
REDACTED
	wg.Wait()

	replayed, err := coordinator.Execute(context.Background(), opts, func(ctx context.Context) (any, error) {
		atomic.AddInt32(&execCount, 1)
		return map[string]any{"ok": trueREDACTED, nil
REDACTED)
REDACTED
	require.True(t, replayed.Replayed)
	require.Equal(t, int32(1), atomic.LoadInt32(&execCount), "concurrent same-key requests should execute business side-effect once")

	metrics := GetIdempotencyMetricsSnapshot()
	require.Equal(t, uint64(1), metrics.ClaimTotal)
	require.Equal(t, uint64(1), metrics.ReplayTotal)
	require.GreaterOrEqual(t, metrics.ConflictTotal, uint64(1))
REDACTED

type failingIdempotencyRepo struct{REDACTED

func (failingIdempotencyRepo) CreateProcessing(context.Context, *IdempotencyRecord) (bool, error) {
	return false, errors.New("store unavailable")
REDACTED
func (failingIdempotencyRepo) GetByScopeAndKeyHash(context.Context, string, string) (*IdempotencyRecord, error) {
	return nil, errors.New("store unavailable")
REDACTED
func (failingIdempotencyRepo) TryReclaim(context.Context, int64, string, time.Time, time.Time, time.Time) (bool, error) {
	return false, errors.New("store unavailable")
REDACTED
func (failingIdempotencyRepo) ExtendProcessingLock(context.Context, int64, string, time.Time, time.Time) (bool, error) {
	return false, errors.New("store unavailable")
REDACTED
func (failingIdempotencyRepo) MarkSucceeded(context.Context, int64, int, string, time.Time) error {
	return errors.New("store unavailable")
REDACTED
func (failingIdempotencyRepo) MarkFailedRetryable(context.Context, int64, string, time.Time, time.Time) error {
	return errors.New("store unavailable")
REDACTED
func (failingIdempotencyRepo) DeleteExpired(context.Context, time.Time, int) (int64, error) {
	return 0, errors.New("store unavailable")
REDACTED

func TestIdempotencyCoordinator_StoreUnavailableMetrics(t *testing.T) {
	resetIdempotencyMetricsForTest()
	coordinator := NewIdempotencyCoordinator(failingIdempotencyRepo{REDACTED, DefaultIdempotencyConfig())

	_, err := coordinator.Execute(context.Background(), IdempotencyExecuteOptions{
		Scope:          "test.scope.unavailable",
		Method:         "POST",
		Route:          "/test/unavailable",
		ActorScope:     "admin:1",
		RequireKey:     true,
		IdempotencyKey: "case-unavailable",
		Payload:        map[string]any{"v": 1REDACTED,
REDACTED, func(ctx context.Context) (any, error) {
		return map[string]any{"ok": trueREDACTED, nil
REDACTED)
REDACTED
	require.Equal(t, infraerrors.Code(ErrIdempotencyStoreUnavail), infraerrors.Code(err))
	require.GreaterOrEqual(t, GetIdempotencyMetricsSnapshot().StoreUnavailableTotal, uint64(1))
REDACTED

type utf8RejectingIdempotencyRepo struct {
	inMemoryIdempotencyRepo
REDACTED

func newUTF8RejectingIdempotencyRepo() *utf8RejectingIdempotencyRepo {
	return &utf8RejectingIdempotencyRepo{inMemoryIdempotencyRepo: *newInMemoryIdempotencyRepo()REDACTED
REDACTED

func (r *utf8RejectingIdempotencyRepo) MarkSucceeded(ctx context.Context, id int64, responseStatus int, responseBody string, expiresAt time.Time) error {
	if !utf8.ValidString(responseBody) {
		return errors.New(`pq: invalid byte sequence for encoding "UTF8": 0xe8 0xb4 0x2e`)
REDACTED
	return r.inMemoryIdempotencyRepo.MarkSucceeded(ctx, id, responseStatus, responseBody, expiresAt)
REDACTED

func TestIdempotencyCoordinator_TruncatedStoredResponseRemainsUTF8(t *testing.T) {
	repo := newUTF8RejectingIdempotencyRepo()
	cfg := DefaultIdempotencyConfig()
	cfg.MaxStoredResponseLen = len(`{"message":"`) + 2
	coordinator := NewIdempotencyCoordinator(repo, cfg)

	opts := IdempotencyExecuteOptions{
		Scope:          "test.scope.truncate_utf8",
		Method:         "POST",
		Route:          "/api/v1/accounts/import/codex-session",
		ActorScope:     "admin:1",
		RequireKey:     true,
		IdempotencyKey: "truncate-utf8",
		Payload:        map[string]any{"content": "codex-session"REDACTED,
REDACTED

	result, err := coordinator.Execute(context.Background(), opts, func(ctx context.Context) (any, error) {
		return map[string]any{"message": strings.Repeat("\u8d26", 8)REDACTED, nil
REDACTED)
REDACTED
	require.NotNil(t, result)

	stored, err := repo.GetByScopeAndKeyHash(context.Background(), opts.Scope, HashIdempotencyKey(opts.IdempotencyKey))
REDACTED
	require.NotNil(t, stored)
	require.NotNil(t, stored.ResponseBody)
	require.True(t, utf8.ValidString(*stored.ResponseBody))
	require.Contains(t, *stored.ResponseBody, "...(truncated)")
REDACTED

func TestDefaultIdempotencyCoordinatorAndTTLs(t *testing.T) {
	SetDefaultIdempotencyCoordinator(nil)
	require.Nil(t, DefaultIdempotencyCoordinator())
	require.Equal(t, DefaultIdempotencyConfig().DefaultTTL, DefaultWriteIdempotencyTTL())
	require.Equal(t, DefaultIdempotencyConfig().SystemOperationTTL, DefaultSystemOperationIdempotencyTTL())

	coordinator := NewIdempotencyCoordinator(newInMemoryIdempotencyRepo(), IdempotencyConfig{
		DefaultTTL:         2 * time.Hour,
		SystemOperationTTL: 15 * time.Minute,
		ProcessingTimeout:  10 * time.Second,
		FailedRetryBackoff: 3 * time.Second,
		ObserveOnly:        false,
REDACTED)
	SetDefaultIdempotencyCoordinator(coordinator)
	t.Cleanup(func() {
		SetDefaultIdempotencyCoordinator(nil)
REDACTED)

	require.Same(t, coordinator, DefaultIdempotencyCoordinator())
	require.Equal(t, 2*time.Hour, DefaultWriteIdempotencyTTL())
	require.Equal(t, 15*time.Minute, DefaultSystemOperationIdempotencyTTL())
REDACTED

func TestNormalizeIdempotencyKeyAndFingerprint(t *testing.T) {
	key, err := NormalizeIdempotencyKey("  abc-123  ")
REDACTED
	require.Equal(t, "abc-123", key)

	key, err = NormalizeIdempotencyKey("")
REDACTED
	require.Equal(t, "", key)

	_, err = NormalizeIdempotencyKey(string(make([]byte, 129)))
REDACTED

	_, err = NormalizeIdempotencyKey("bad\nkey")
REDACTED

	fp1, err := BuildIdempotencyFingerprint("", "", "", map[string]any{"a": 1REDACTED)
REDACTED
	require.NotEmpty(t, fp1)
	fp2, err := BuildIdempotencyFingerprint("POST", "/", "anonymous", map[string]any{"a": 1REDACTED)
REDACTED
	require.Equal(t, fp1, fp2)

	_, err = BuildIdempotencyFingerprint("POST", "/x", "u:1", map[string]any{"bad": make(chan int)REDACTED)
REDACTED
	require.Equal(t, infraerrors.Code(ErrIdempotencyInvalidPayload), infraerrors.Code(err))
REDACTED

func TestRetryAfterSecondsFromErrorBranches(t *testing.T) {
	require.Equal(t, 0, RetryAfterSecondsFromError(nil))
	require.Equal(t, 0, RetryAfterSecondsFromError(errors.New("plain")))

	err := ErrIdempotencyInProgress.WithMetadata(map[string]string{"retry_after": "12"REDACTED)
	require.Equal(t, 12, RetryAfterSecondsFromError(err))

	err = ErrIdempotencyInProgress.WithMetadata(map[string]string{"retry_after": "bad"REDACTED)
	require.Equal(t, 0, RetryAfterSecondsFromError(err))
REDACTED

func TestIdempotencyCoordinator_ExecuteNilExecutorAndNoKeyPassThrough(t *testing.T) {
	repo := newInMemoryIdempotencyRepo()
	coordinator := NewIdempotencyCoordinator(repo, DefaultIdempotencyConfig())

	_, err := coordinator.Execute(context.Background(), IdempotencyExecuteOptions{
		Scope:          "scope",
		IdempotencyKey: "k",
		Payload:        map[string]any{"a": 1REDACTED,
REDACTED, nil)
REDACTED
	require.Equal(t, "IDEMPOTENCY_EXECUTOR_NIL", infraerrors.Reason(err))

	called := 0
	result, err := coordinator.Execute(context.Background(), IdempotencyExecuteOptions{
		Scope:      "scope",
		RequireKey: true,
		Payload:    map[string]any{"a": 1REDACTED,
REDACTED, func(ctx context.Context) (any, error) {
		called++
		return map[string]any{"ok": trueREDACTED, nil
REDACTED)
REDACTED
	require.Equal(t, 1, called)
	require.NotNil(t, result)
	require.False(t, result.Replayed)
REDACTED

type noIDOwnerRepo struct{REDACTED

func (noIDOwnerRepo) CreateProcessing(context.Context, *IdempotencyRecord) (bool, error) {
	return true, nil
REDACTED
func (noIDOwnerRepo) GetByScopeAndKeyHash(context.Context, string, string) (*IdempotencyRecord, error) {
	return nil, nil
REDACTED
func (noIDOwnerRepo) TryReclaim(context.Context, int64, string, time.Time, time.Time, time.Time) (bool, error) {
	return false, nil
REDACTED
func (noIDOwnerRepo) ExtendProcessingLock(context.Context, int64, string, time.Time, time.Time) (bool, error) {
	return false, nil
REDACTED
func (noIDOwnerRepo) MarkSucceeded(context.Context, int64, int, string, time.Time) error { return nil REDACTED
func (noIDOwnerRepo) MarkFailedRetryable(context.Context, int64, string, time.Time, time.Time) error {
	return nil
REDACTED
func (noIDOwnerRepo) DeleteExpired(context.Context, time.Time, int) (int64, error) { return 0, nil REDACTED

func TestIdempotencyCoordinator_RepoNilScopeRequiredAndRecordIDMissing(t *testing.T) {
	cfg := DefaultIdempotencyConfig()
	coordinator := NewIdempotencyCoordinator(nil, cfg)

	_, err := coordinator.Execute(context.Background(), IdempotencyExecuteOptions{
		Scope:          "scope",
		IdempotencyKey: "k",
		Payload:        map[string]any{"a": 1REDACTED,
REDACTED, func(ctx context.Context) (any, error) {
		return map[string]any{"ok": trueREDACTED, nil
REDACTED)
REDACTED
	require.Equal(t, infraerrors.Code(ErrIdempotencyStoreUnavail), infraerrors.Code(err))

	coordinator = NewIdempotencyCoordinator(newInMemoryIdempotencyRepo(), cfg)
	_, err = coordinator.Execute(context.Background(), IdempotencyExecuteOptions{
		IdempotencyKey: "k2",
		Payload:        map[string]any{"a": 1REDACTED,
REDACTED, func(ctx context.Context) (any, error) {
		return map[string]any{"ok": trueREDACTED, nil
REDACTED)
REDACTED
	require.Equal(t, "IDEMPOTENCY_SCOPE_REQUIRED", infraerrors.Reason(err))

	coordinator = NewIdempotencyCoordinator(noIDOwnerRepo{REDACTED, cfg)
	_, err = coordinator.Execute(context.Background(), IdempotencyExecuteOptions{
		Scope:          "scope-no-id",
		IdempotencyKey: "k3",
		Payload:        map[string]any{"a": 1REDACTED,
REDACTED, func(ctx context.Context) (any, error) {
		return map[string]any{"ok": trueREDACTED, nil
REDACTED)
REDACTED
	require.Equal(t, infraerrors.Code(ErrIdempotencyStoreUnavail), infraerrors.Code(err))
REDACTED

type conflictBranchRepo struct {
	existing      *IdempotencyRecord
	tryReclaimErr error
	tryReclaimOK  bool
REDACTED

func (r *conflictBranchRepo) CreateProcessing(context.Context, *IdempotencyRecord) (bool, error) {
	return false, nil
REDACTED
func (r *conflictBranchRepo) GetByScopeAndKeyHash(context.Context, string, string) (*IdempotencyRecord, error) {
	return cloneRecord(r.existing), nil
REDACTED
func (r *conflictBranchRepo) TryReclaim(context.Context, int64, string, time.Time, time.Time, time.Time) (bool, error) {
	if r.tryReclaimErr != nil {
		return false, r.tryReclaimErr
REDACTED
	return r.tryReclaimOK, nil
REDACTED
func (r *conflictBranchRepo) ExtendProcessingLock(context.Context, int64, string, time.Time, time.Time) (bool, error) {
	return false, nil
REDACTED
func (r *conflictBranchRepo) MarkSucceeded(context.Context, int64, int, string, time.Time) error {
	return nil
REDACTED
func (r *conflictBranchRepo) MarkFailedRetryable(context.Context, int64, string, time.Time, time.Time) error {
	return nil
REDACTED
func (r *conflictBranchRepo) DeleteExpired(context.Context, time.Time, int) (int64, error) {
	return 0, nil
REDACTED

func TestIdempotencyCoordinator_ConflictBranchesAndDecodeError(t *testing.T) {
	now := time.Now()
	fp, err := BuildIdempotencyFingerprint("POST", "/x", "u:1", map[string]any{"a": 1REDACTED)
REDACTED
	badBody := "{bad-json"
	repo := &conflictBranchRepo{
		existing: &IdempotencyRecord{
			ID:                 1,
			Scope:              "scope",
			IdempotencyKeyHash: HashIdempotencyKey("k"),
			RequestFingerprint: fp,
			Status:             IdempotencyStatusSucceeded,
			ResponseBody:       &badBody,
			ExpiresAt:          now.Add(time.Hour),
	REDACTED,
REDACTED
	coordinator := NewIdempotencyCoordinator(repo, DefaultIdempotencyConfig())
	_, err = coordinator.Execute(context.Background(), IdempotencyExecuteOptions{
		Scope:          "scope",
		IdempotencyKey: "k",
		Method:         "POST",
		Route:          "/x",
		ActorScope:     "u:1",
		Payload:        map[string]any{"a": 1REDACTED,
REDACTED, func(ctx context.Context) (any, error) {
		return map[string]any{"ok": trueREDACTED, nil
REDACTED)
REDACTED
	require.Equal(t, infraerrors.Code(ErrIdempotencyStoreUnavail), infraerrors.Code(err))

	repo.existing = &IdempotencyRecord{
		ID:                 2,
		Scope:              "scope",
		IdempotencyKeyHash: HashIdempotencyKey("k"),
		RequestFingerprint: fp,
		Status:             "unknown",
		ExpiresAt:          now.Add(time.Hour),
REDACTED
	_, err = coordinator.Execute(context.Background(), IdempotencyExecuteOptions{
		Scope:          "scope",
		IdempotencyKey: "k",
		Method:         "POST",
		Route:          "/x",
		ActorScope:     "u:1",
		Payload:        map[string]any{"a": 1REDACTED,
REDACTED, func(ctx context.Context) (any, error) {
		return map[string]any{"ok": trueREDACTED, nil
REDACTED)
REDACTED
	require.Equal(t, infraerrors.Code(ErrIdempotencyKeyConflict), infraerrors.Code(err))

	repo.existing = &IdempotencyRecord{
		ID:                 3,
		Scope:              "scope",
		IdempotencyKeyHash: HashIdempotencyKey("k"),
		RequestFingerprint: fp,
		Status:             IdempotencyStatusFailedRetryable,
		LockedUntil:        ptrTime(now.Add(-time.Second)),
		ExpiresAt:          now.Add(time.Hour),
REDACTED
	repo.tryReclaimErr = errors.New("reclaim down")
	_, err = coordinator.Execute(context.Background(), IdempotencyExecuteOptions{
		Scope:          "scope",
		IdempotencyKey: "k",
		Method:         "POST",
		Route:          "/x",
		ActorScope:     "u:1",
		Payload:        map[string]any{"a": 1REDACTED,
REDACTED, func(ctx context.Context) (any, error) {
		return map[string]any{"ok": trueREDACTED, nil
REDACTED)
REDACTED
	require.Equal(t, infraerrors.Code(ErrIdempotencyStoreUnavail), infraerrors.Code(err))

	repo.tryReclaimErr = nil
	repo.tryReclaimOK = false
	_, err = coordinator.Execute(context.Background(), IdempotencyExecuteOptions{
		Scope:          "scope",
		IdempotencyKey: "k",
		Method:         "POST",
		Route:          "/x",
		ActorScope:     "u:1",
		Payload:        map[string]any{"a": 1REDACTED,
REDACTED, func(ctx context.Context) (any, error) {
		return map[string]any{"ok": trueREDACTED, nil
REDACTED)
REDACTED
	require.Equal(t, infraerrors.Code(ErrIdempotencyInProgress), infraerrors.Code(err))
REDACTED

type markBehaviorRepo struct {
	inMemoryIdempotencyRepo
	failMarkSucceeded bool
	failMarkFailed    bool
REDACTED

func (r *markBehaviorRepo) MarkSucceeded(ctx context.Context, id int64, responseStatus int, responseBody string, expiresAt time.Time) error {
	if r.failMarkSucceeded {
		return errors.New("mark succeeded failed")
REDACTED
	return r.inMemoryIdempotencyRepo.MarkSucceeded(ctx, id, responseStatus, responseBody, expiresAt)
REDACTED

func (r *markBehaviorRepo) MarkFailedRetryable(ctx context.Context, id int64, errorReason string, lockedUntil, expiresAt time.Time) error {
	if r.failMarkFailed {
		return errors.New("mark failed retryable failed")
REDACTED
	return r.inMemoryIdempotencyRepo.MarkFailedRetryable(ctx, id, errorReason, lockedUntil, expiresAt)
REDACTED

func TestIdempotencyCoordinator_MarkAndMarshalBranches(t *testing.T) {
	repo := &markBehaviorRepo{inMemoryIdempotencyRepo: *newInMemoryIdempotencyRepo()REDACTED
	coordinator := NewIdempotencyCoordinator(repo, DefaultIdempotencyConfig())

	repo.failMarkSucceeded = true
	_, err := coordinator.Execute(context.Background(), IdempotencyExecuteOptions{
		Scope:          "scope-success",
		IdempotencyKey: "k1",
		Method:         "POST",
		Route:          "/ok",
		ActorScope:     "u:1",
		Payload:        map[string]any{"a": 1REDACTED,
REDACTED, func(ctx context.Context) (any, error) {
		return map[string]any{"ok": trueREDACTED, nil
REDACTED)
REDACTED
	require.Equal(t, infraerrors.Code(ErrIdempotencyStoreUnavail), infraerrors.Code(err))

	repo.failMarkSucceeded = false
	_, err = coordinator.Execute(context.Background(), IdempotencyExecuteOptions{
		Scope:          "scope-marshal",
		IdempotencyKey: "k2",
		Method:         "POST",
		Route:          "/bad",
		ActorScope:     "u:1",
		Payload:        map[string]any{"a": 1REDACTED,
REDACTED, func(ctx context.Context) (any, error) {
		return map[string]any{"bad": make(chan int)REDACTED, nil
REDACTED)
REDACTED
	require.Equal(t, infraerrors.Code(ErrIdempotencyStoreUnavail), infraerrors.Code(err))

	repo.failMarkFailed = true
	_, err = coordinator.Execute(context.Background(), IdempotencyExecuteOptions{
		Scope:          "scope-fail",
		IdempotencyKey: "k3",
		Method:         "POST",
		Route:          "/fail",
		ActorScope:     "u:1",
		Payload:        map[string]any{"a": 1REDACTED,
REDACTED, func(ctx context.Context) (any, error) {
		return nil, errors.New("plain failure")
REDACTED)
REDACTED
	require.Equal(t, "plain failure", err.Error())
REDACTED

func TestIdempotencyCoordinator_HelperBranches(t *testing.T) {
	c := NewIdempotencyCoordinator(newInMemoryIdempotencyRepo(), IdempotencyConfig{
		DefaultTTL:           time.Hour,
		SystemOperationTTL:   time.Hour,
		ProcessingTimeout:    time.Second,
		FailedRetryBackoff:   time.Second,
		MaxStoredResponseLen: 12,
		ObserveOnly:          false,
REDACTED)

	// conflictWithRetryAfter without locked_until should return base error.
	base := ErrIdempotencyInProgress
	err := c.conflictWithRetryAfter(base, nil, time.Now())
	require.Equal(t, infraerrors.Code(base), infraerrors.Code(err))

	// marshalStoredResponse should truncate.
	body, err := c.marshalStoredResponse(map[string]any{"long": "abcdefghijklmnopqrstuvwxyz"REDACTED)
REDACTED
	require.Contains(t, body, "...(truncated)")

	// decodeStoredResponse empty and invalid json.
	out, err := c.decodeStoredResponse(nil)
REDACTED
	_, ok := out.(map[string]any)
	require.True(t, ok)

	invalid := "{invalid"
	_, err = c.decodeStoredResponse(&invalid)
REDACTED
REDACTED
