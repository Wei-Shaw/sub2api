package service

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"time"

	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
	"github.com/stretchr/testify/require"
)

func TestSystemOperationLockService_AcquireBusyAndRelease(t *testing.T) {
	repo := newInMemoryIdempotencyRepo()
	svc := NewSystemOperationLockService(repo, IdempotencyConfig{
		SystemOperationTTL: 10 * time.Second,
		ProcessingTimeout:  2 * time.Second,
REDACTED)

	lock1, err := svc.Acquire(context.Background(), "op-1")
REDACTED
	require.NotNil(t, lock1)

	_, err = svc.Acquire(context.Background(), "op-2")
REDACTED
	require.Equal(t, infraerrors.Code(ErrSystemOperationBusy), infraerrors.Code(err))
	appErr := infraerrors.FromError(err)
	require.Equal(t, "op-1", appErr.Metadata["operation_id"])
	require.NotEmpty(t, appErr.Metadata["retry_after"])

	require.NoError(t, svc.Release(context.Background(), lock1, true, ""))

	lock2, err := svc.Acquire(context.Background(), "op-2")
REDACTED
	require.NotNil(t, lock2)
	require.NoError(t, svc.Release(context.Background(), lock2, true, ""))
REDACTED

func TestSystemOperationLockService_RenewLease(t *testing.T) {
	repo := newInMemoryIdempotencyRepo()
	svc := NewSystemOperationLockService(repo, IdempotencyConfig{
		SystemOperationTTL: 5 * time.Second,
		ProcessingTimeout:  1200 * time.Millisecond,
REDACTED)

	lock, err := svc.Acquire(context.Background(), "op-renew")
REDACTED
	require.NotNil(t, lock)
	defer func() {
		_ = svc.Release(context.Background(), lock, true, "")
REDACTED()

	keyHash := HashIdempotencyKey(systemOperationLockKey)
	initial, _ := repo.GetByScopeAndKeyHash(context.Background(), systemOperationLockScope, keyHash)
	require.NotNil(t, initial)
	require.NotNil(t, initial.LockedUntil)
	initialLockedUntil := *initial.LockedUntil

	time.Sleep(1500 * time.Millisecond)

	updated, _ := repo.GetByScopeAndKeyHash(context.Background(), systemOperationLockScope, keyHash)
	require.NotNil(t, updated)
	require.NotNil(t, updated.LockedUntil)
	require.True(t, updated.LockedUntil.After(initialLockedUntil), "locked_until should be renewed while lock is held")
REDACTED

type flakySystemLockRenewRepo struct {
	*inMemoryIdempotencyRepo
	extendCalls int32
REDACTED

func (r *flakySystemLockRenewRepo) ExtendProcessingLock(ctx context.Context, id int64, requestFingerprint string, newLockedUntil, newExpiresAt time.Time) (bool, error) {
	call := atomic.AddInt32(&r.extendCalls, 1)
	if call == 1 {
		return false, errors.New("transient extend failure")
REDACTED
	return r.inMemoryIdempotencyRepo.ExtendProcessingLock(ctx, id, requestFingerprint, newLockedUntil, newExpiresAt)
REDACTED

func TestSystemOperationLockService_RenewLeaseContinuesAfterTransientFailure(t *testing.T) {
	repo := &flakySystemLockRenewRepo{inMemoryIdempotencyRepo: newInMemoryIdempotencyRepo()REDACTED
	svc := NewSystemOperationLockService(repo, IdempotencyConfig{
		SystemOperationTTL: 5 * time.Second,
		ProcessingTimeout:  2400 * time.Millisecond,
REDACTED)

	lock, err := svc.Acquire(context.Background(), "op-renew-transient")
REDACTED
	require.NotNil(t, lock)
	defer func() {
		_ = svc.Release(context.Background(), lock, true, "")
REDACTED()

	keyHash := HashIdempotencyKey(systemOperationLockKey)
	initial, _ := repo.GetByScopeAndKeyHash(context.Background(), systemOperationLockScope, keyHash)
	require.NotNil(t, initial)
	require.NotNil(t, initial.LockedUntil)
	initialLockedUntil := *initial.LockedUntil

	// 首次续租失败后，下一轮应继续尝试并成功更新锁过期时间。
	require.Eventually(t, func() bool {
		updated, _ := repo.GetByScopeAndKeyHash(context.Background(), systemOperationLockScope, keyHash)
		if updated == nil || updated.LockedUntil == nil {
			return false
	REDACTED
		return atomic.LoadInt32(&repo.extendCalls) >= 2 && updated.LockedUntil.After(initialLockedUntil)
REDACTED, 4*time.Second, 100*time.Millisecond, "renew loop should continue after transient error")
REDACTED

func TestSystemOperationLockService_SameOperationIDRetryWhileRunning(t *testing.T) {
	repo := newInMemoryIdempotencyRepo()
	svc := NewSystemOperationLockService(repo, IdempotencyConfig{
		SystemOperationTTL: 10 * time.Second,
		ProcessingTimeout:  2 * time.Second,
REDACTED)

	lock1, err := svc.Acquire(context.Background(), "op-same")
REDACTED
	require.NotNil(t, lock1)

	_, err = svc.Acquire(context.Background(), "op-same")
REDACTED
	require.Equal(t, infraerrors.Code(ErrSystemOperationBusy), infraerrors.Code(err))
	appErr := infraerrors.FromError(err)
	require.Equal(t, "op-same", appErr.Metadata["operation_id"])

	require.NoError(t, svc.Release(context.Background(), lock1, true, ""))

	lock2, err := svc.Acquire(context.Background(), "op-same")
REDACTED
	require.NotNil(t, lock2)
	require.NoError(t, svc.Release(context.Background(), lock2, true, ""))
REDACTED

func TestSystemOperationLockService_RecoverAfterLeaseExpired(t *testing.T) {
	repo := newInMemoryIdempotencyRepo()
	svc := NewSystemOperationLockService(repo, IdempotencyConfig{
		SystemOperationTTL: 5 * time.Second,
		ProcessingTimeout:  300 * time.Millisecond,
REDACTED)

	lock1, err := svc.Acquire(context.Background(), "op-crashed")
REDACTED
	require.NotNil(t, lock1)

	// 模拟实例异常：停止续租，不调用 Release。
	lock1.stopOnce.Do(func() {
		close(lock1.stopCh)
REDACTED)

	time.Sleep(450 * time.Millisecond)

	lock2, err := svc.Acquire(context.Background(), "op-recovered")
	require.NoError(t, err, "expired lease should allow a new operation to reclaim lock")
	require.NotNil(t, lock2)
	require.NoError(t, svc.Release(context.Background(), lock2, true, ""))
REDACTED

type systemLockRepoStub struct {
	createOwner bool
	createErr   error
	existing    *IdempotencyRecord
	getErr      error
	reclaimOK   bool
	reclaimErr  error
	markSuccErr error
	markFailErr error
REDACTED

func (s *systemLockRepoStub) CreateProcessing(context.Context, *IdempotencyRecord) (bool, error) {
	if s.createErr != nil {
		return false, s.createErr
REDACTED
	return s.createOwner, nil
REDACTED

func (s *systemLockRepoStub) GetByScopeAndKeyHash(context.Context, string, string) (*IdempotencyRecord, error) {
	if s.getErr != nil {
		return nil, s.getErr
REDACTED
	return cloneRecord(s.existing), nil
REDACTED

func (s *systemLockRepoStub) TryReclaim(context.Context, int64, string, time.Time, time.Time, time.Time) (bool, error) {
	if s.reclaimErr != nil {
		return false, s.reclaimErr
REDACTED
	return s.reclaimOK, nil
REDACTED

func (s *systemLockRepoStub) ExtendProcessingLock(context.Context, int64, string, time.Time, time.Time) (bool, error) {
	return true, nil
REDACTED

func (s *systemLockRepoStub) MarkSucceeded(context.Context, int64, int, string, time.Time) error {
	return s.markSuccErr
REDACTED

func (s *systemLockRepoStub) MarkFailedRetryable(context.Context, int64, string, time.Time, time.Time) error {
	return s.markFailErr
REDACTED

func (s *systemLockRepoStub) DeleteExpired(context.Context, time.Time, int) (int64, error) {
	return 0, nil
REDACTED

func TestSystemOperationLockService_InputAndStoreErrorBranches(t *testing.T) {
	var nilSvc *SystemOperationLockService
	_, err := nilSvc.Acquire(context.Background(), "x")
REDACTED
	require.Equal(t, infraerrors.Code(ErrIdempotencyStoreUnavail), infraerrors.Code(err))

	svc := &SystemOperationLockService{repo: nilREDACTED
	_, err = svc.Acquire(context.Background(), "x")
REDACTED
	require.Equal(t, infraerrors.Code(ErrIdempotencyStoreUnavail), infraerrors.Code(err))

	svc = NewSystemOperationLockService(newInMemoryIdempotencyRepo(), IdempotencyConfig{
		SystemOperationTTL: 10 * time.Second,
		ProcessingTimeout:  2 * time.Second,
REDACTED)
	_, err = svc.Acquire(context.Background(), "")
REDACTED
	require.Equal(t, "SYSTEM_OPERATION_ID_REQUIRED", infraerrors.Reason(err))

	badStore := &systemLockRepoStub{createErr: errors.New("db down")REDACTED
	svc = NewSystemOperationLockService(badStore, IdempotencyConfig{
		SystemOperationTTL: 10 * time.Second,
		ProcessingTimeout:  2 * time.Second,
REDACTED)
	_, err = svc.Acquire(context.Background(), "x")
REDACTED
	require.Equal(t, infraerrors.Code(ErrIdempotencyStoreUnavail), infraerrors.Code(err))
REDACTED

func TestSystemOperationLockService_ExistingNilAndReclaimBranches(t *testing.T) {
	now := time.Now()
	repo := &systemLockRepoStub{
		createOwner: false,
REDACTED
	svc := NewSystemOperationLockService(repo, IdempotencyConfig{
		SystemOperationTTL: 10 * time.Second,
		ProcessingTimeout:  2 * time.Second,
REDACTED)

	_, err := svc.Acquire(context.Background(), "op")
REDACTED
	require.Equal(t, infraerrors.Code(ErrIdempotencyStoreUnavail), infraerrors.Code(err))

	repo.existing = &IdempotencyRecord{
		ID:                 1,
		Scope:              systemOperationLockScope,
		IdempotencyKeyHash: HashIdempotencyKey(systemOperationLockKey),
		RequestFingerprint: "other-op",
		Status:             IdempotencyStatusFailedRetryable,
		LockedUntil:        ptrTime(now.Add(-time.Second)),
		ExpiresAt:          now.Add(time.Hour),
REDACTED
	repo.reclaimErr = errors.New("reclaim failed")
	_, err = svc.Acquire(context.Background(), "op")
REDACTED
	require.Equal(t, infraerrors.Code(ErrIdempotencyStoreUnavail), infraerrors.Code(err))

	repo.reclaimErr = nil
	repo.reclaimOK = false
	_, err = svc.Acquire(context.Background(), "op")
REDACTED
	require.Equal(t, infraerrors.Code(ErrSystemOperationBusy), infraerrors.Code(err))
REDACTED

func TestSystemOperationLockService_ReleaseBranchesAndOperationID(t *testing.T) {
	require.Equal(t, "", (*SystemOperationLock)(nil).OperationID())

	svc := NewSystemOperationLockService(newInMemoryIdempotencyRepo(), IdempotencyConfig{
		SystemOperationTTL: 10 * time.Second,
		ProcessingTimeout:  2 * time.Second,
REDACTED)
	lock, err := svc.Acquire(context.Background(), "op")
REDACTED
	require.NotNil(t, lock)

	require.NoError(t, svc.Release(context.Background(), lock, false, ""))
	require.NoError(t, svc.Release(context.Background(), lock, true, ""))

	repo := &systemLockRepoStub{
		createOwner: true,
		markSuccErr: errors.New("mark succeeded failed"),
		markFailErr: errors.New("mark failed failed"),
REDACTED
	svc = NewSystemOperationLockService(repo, IdempotencyConfig{
		SystemOperationTTL: 10 * time.Second,
		ProcessingTimeout:  2 * time.Second,
REDACTED)
	lock = &SystemOperationLock{recordID: 1, operationID: "op2", stopCh: make(chan struct{REDACTED)REDACTED
	require.Error(t, svc.Release(context.Background(), lock, true, ""))
	lock = &SystemOperationLock{recordID: 1, operationID: "op3", stopCh: make(chan struct{REDACTED)REDACTED
	require.Error(t, svc.Release(context.Background(), lock, false, "BAD"))

	var nilLockSvc *SystemOperationLockService
	require.NoError(t, nilLockSvc.Release(context.Background(), nil, true, ""))

	err = svc.busyError("", nil, time.Now())
	require.Equal(t, infraerrors.Code(ErrSystemOperationBusy), infraerrors.Code(err))
REDACTED
