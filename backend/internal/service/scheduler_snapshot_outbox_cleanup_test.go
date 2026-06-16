package service

import (
	"context"
	"errors"
	"reflect"
	"testing"
	"time"
)

type outboxCleanupCache struct {
	watermark     int64
	setWatermarks []int64
	updateErr     error
REDACTED

func (c *outboxCleanupCache) GetSnapshot(ctx context.Context, bucket SchedulerBucket) ([]*Account, bool, error) {
	return nil, false, nil
REDACTED

func (c *outboxCleanupCache) SetSnapshot(ctx context.Context, bucket SchedulerBucket, accounts []Account) error {
	return nil
REDACTED

func (c *outboxCleanupCache) GetAccount(ctx context.Context, accountID int64) (*Account, error) {
	return nil, nil
REDACTED

func (c *outboxCleanupCache) SetAccount(ctx context.Context, account *Account) error {
	return nil
REDACTED

func (c *outboxCleanupCache) DeleteAccount(ctx context.Context, accountID int64) error {
	return nil
REDACTED

func (c *outboxCleanupCache) UpdateLastUsed(ctx context.Context, updates map[int64]time.Time) error {
	return c.updateErr
REDACTED

func (c *outboxCleanupCache) TryLockBucket(ctx context.Context, bucket SchedulerBucket, ttl time.Duration) (bool, error) {
	return true, nil
REDACTED

func (c *outboxCleanupCache) UnlockBucket(ctx context.Context, bucket SchedulerBucket) error {
	return nil
REDACTED

func (c *outboxCleanupCache) ListBuckets(ctx context.Context) ([]SchedulerBucket, error) {
	return nil, nil
REDACTED

func (c *outboxCleanupCache) GetOutboxWatermark(ctx context.Context) (int64, error) {
	return c.watermark, nil
REDACTED

func (c *outboxCleanupCache) SetOutboxWatermark(ctx context.Context, id int64) error {
	c.watermark = id
	c.setWatermarks = append(c.setWatermarks, id)
	return nil
REDACTED

type outboxCleanupDeleteCall struct {
	watermark int64
	limit     int
REDACTED

type outboxCleanupRepo struct {
	events       []SchedulerOutboxEvent
	rows         []int64
	lockAcquired bool
	lockAttempts int
	releaseCount int
	deleteCalls  []outboxCleanupDeleteCall
REDACTED

func (r *outboxCleanupRepo) ListAfterAndReleaseDedup(ctx context.Context, afterID int64, limit int) ([]SchedulerOutboxEvent, error) {
	events := make([]SchedulerOutboxEvent, 0, len(r.events))
	for _, event := range r.events {
		if event.ID <= afterID {
			continue
	REDACTED
		events = append(events, event)
		if limit > 0 && len(events) >= limit {
			break
	REDACTED
REDACTED
	return events, nil
REDACTED

func (r *outboxCleanupRepo) MaxID(ctx context.Context) (int64, error) {
	var maxID int64
	for _, id := range r.rows {
		if id > maxID {
			maxID = id
	REDACTED
REDACTED
	return maxID, nil
REDACTED

func (r *outboxCleanupRepo) DeleteConsumedUpTo(ctx context.Context, watermark int64, limit int) (int64, error) {
	r.deleteCalls = append(r.deleteCalls, outboxCleanupDeleteCall{
		watermark: watermark,
		limit:     limit,
REDACTED)
	if watermark <= 0 || limit <= 0 {
		return 0, nil
REDACTED

	deleted := int64(0)
	kept := make([]int64, 0, len(r.rows))
	for _, id := range r.rows {
		if id <= watermark && deleted < int64(limit) {
			deleted++
			continue
	REDACTED
		kept = append(kept, id)
REDACTED
	r.rows = kept
	return deleted, nil
REDACTED

func (r *outboxCleanupRepo) TryAcquireCleanupLock(ctx context.Context) (SchedulerOutboxCleanupLease, bool, error) {
	r.lockAttempts++
	if !r.lockAcquired {
		return nil, false, nil
REDACTED
	return outboxCleanupLease{release: func() {
		r.releaseCount++
REDACTEDREDACTED, true, nil
REDACTED

type outboxCleanupLease struct {
	release func()
REDACTED

func (l outboxCleanupLease) Release() {
	if l.release != nil {
		l.release()
REDACTED
REDACTED

func TestSchedulerSnapshotServicePollOutboxCleansConsumedRowsAfterWatermark(t *testing.T) {
	cache := &outboxCleanupCache{REDACTED
	repo := &outboxCleanupRepo{
		events: []SchedulerOutboxEvent{
			{ID: 10000, EventType: SchedulerOutboxEventAccountLastUsedREDACTED,
	REDACTED,
		rows:         int64Range(1, 10003),
		lockAcquired: true,
REDACTED
	svc := NewSchedulerSnapshotService(cache, repo, nil, nil, nil)

	svc.pollOutbox()

	if cache.watermark != 10000 {
		t.Fatalf("expected watermark 10000, got %d", cache.watermark)
REDACTED
	if !reflect.DeepEqual(cache.setWatermarks, []int64{10000REDACTED) {
		t.Fatalf("unexpected watermark writes: %#v", cache.setWatermarks)
REDACTED
	if !reflect.DeepEqual(repo.rows, []int64{10001, 10002, 10003REDACTED) {
		t.Fatalf("expected rows above watermark to remain, got %#v", repo.rows)
REDACTED
	if repo.lockAttempts != 1 || repo.releaseCount != 1 {
		t.Fatalf("expected one lock acquire/release, got acquire=%d release=%d", repo.lockAttempts, repo.releaseCount)
REDACTED
	if len(repo.deleteCalls) != 3 {
		t.Fatalf("expected cleanup to loop until a short batch, got %d calls", len(repo.deleteCalls))
REDACTED
	for _, call := range repo.deleteCalls {
		if call.watermark != 10000 || call.limit != schedulerOutboxCleanupBatch {
			t.Fatalf("unexpected cleanup call: %#v", call)
	REDACTED
REDACTED
REDACTED

func TestSchedulerSnapshotServicePollOutboxSkipsCleanupWhenLockUnavailable(t *testing.T) {
	cache := &outboxCleanupCache{REDACTED
	repo := &outboxCleanupRepo{
		events: []SchedulerOutboxEvent{
			{ID: 3, EventType: SchedulerOutboxEventAccountLastUsedREDACTED,
	REDACTED,
		rows:         []int64{1, 2, 3, 4REDACTED,
		lockAcquired: false,
REDACTED
	svc := NewSchedulerSnapshotService(cache, repo, nil, nil, nil)

	svc.pollOutbox()

	if cache.watermark != 3 {
		t.Fatalf("expected watermark 3, got %d", cache.watermark)
REDACTED
	if !reflect.DeepEqual(repo.rows, []int64{1, 2, 3, 4REDACTED) {
		t.Fatalf("expected cleanup to skip all rows, got %#v", repo.rows)
REDACTED
	if repo.lockAttempts != 1 {
		t.Fatalf("expected one lock attempt, got %d", repo.lockAttempts)
REDACTED
	if len(repo.deleteCalls) != 0 {
		t.Fatalf("expected no delete calls, got %#v", repo.deleteCalls)
REDACTED
	if repo.releaseCount != 0 {
		t.Fatalf("expected no release without lock, got %d", repo.releaseCount)
REDACTED
REDACTED

func TestSchedulerSnapshotServicePollOutboxDoesNotCleanupOnHandleFailure(t *testing.T) {
	cache := &outboxCleanupCache{
		updateErr: errors.New("cache update failed"),
REDACTED
	repo := &outboxCleanupRepo{
		events: []SchedulerOutboxEvent{
			{
				ID:        5,
				EventType: SchedulerOutboxEventAccountLastUsed,
				Payload: map[string]any{
					"last_used": map[string]any{"101": float64(123)REDACTED,
			REDACTED,
		REDACTED,
	REDACTED,
		rows:         []int64{1, 2, 3, 4, 5, 6REDACTED,
		lockAcquired: true,
REDACTED
	svc := NewSchedulerSnapshotService(cache, repo, nil, nil, nil)

	svc.pollOutbox()

	if len(cache.setWatermarks) != 0 {
		t.Fatalf("expected no watermark write on handle failure, got %#v", cache.setWatermarks)
REDACTED
	if repo.lockAttempts != 0 {
		t.Fatalf("expected cleanup lock not to be attempted, got %d", repo.lockAttempts)
REDACTED
	if len(repo.deleteCalls) != 0 {
		t.Fatalf("expected no delete calls, got %#v", repo.deleteCalls)
REDACTED
	if !reflect.DeepEqual(repo.rows, []int64{1, 2, 3, 4, 5, 6REDACTED) {
		t.Fatalf("expected rows unchanged, got %#v", repo.rows)
REDACTED
REDACTED

func TestSchedulerSnapshotServiceCleanupSkipsNonPositiveWatermark(t *testing.T) {
	repo := &outboxCleanupRepo{
		rows:         []int64{1, 2, 3REDACTED,
		lockAcquired: true,
REDACTED
	svc := NewSchedulerSnapshotService(&outboxCleanupCache{REDACTED, repo, nil, nil, nil)

	svc.cleanupConsumedOutbox(0)

	if repo.lockAttempts != 0 {
		t.Fatalf("expected no lock attempt for non-positive watermark, got %d", repo.lockAttempts)
REDACTED
	if len(repo.deleteCalls) != 0 {
		t.Fatalf("expected no delete calls, got %#v", repo.deleteCalls)
REDACTED
	if !reflect.DeepEqual(repo.rows, []int64{1, 2, 3REDACTED) {
		t.Fatalf("expected rows unchanged, got %#v", repo.rows)
REDACTED
REDACTED

func int64Range(start, end int64) []int64 {
	values := make([]int64, 0, end-start+1)
	for id := start; id <= end; id++ {
		values = append(values, id)
REDACTED
	return values
REDACTED
