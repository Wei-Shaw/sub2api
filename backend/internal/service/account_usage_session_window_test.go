package service

import (
	"context"
	"sync"
	"testing"
	"time"
)

// sessionWindowSyncRepo 记录 syncActiveToPassive 触发的所有写操作。
type sessionWindowSyncRepo struct {
	AccountRepository

	mu                sync.Mutex
	extraUpdates      []map[string]any
	sessionWindowEnds []sessionWindowEndCall
REDACTED

type sessionWindowEndCall struct {
	AccountID int64
	End       time.Time
REDACTED

func (r *sessionWindowSyncRepo) UpdateExtra(_ context.Context, _ int64, updates map[string]any) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	copied := make(map[string]any, len(updates))
	for k, v := range updates {
		copied[k] = v
REDACTED
	r.extraUpdates = append(r.extraUpdates, copied)
	return nil
REDACTED

func (r *sessionWindowSyncRepo) UpdateSessionWindowEnd(_ context.Context, id int64, end time.Time) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.sessionWindowEnds = append(r.sessionWindowEnds, sessionWindowEndCall{AccountID: id, End: endREDACTED)
	return nil
REDACTED

func TestEstimateSetupTokenUsage_ExpiredWindowZeroes(t *testing.T) {
	t.Parallel()

	past := time.Now().Add(-2 * time.Hour)
	svc := &AccountUsageService{REDACTED
	info := svc.estimateSetupTokenUsage(&Account{
		SessionWindowEnd: &past,
		Extra: map[string]any{
			"session_window_utilization": 0.53,
	REDACTED,
REDACTED)

	if info.FiveHour == nil {
		t.Fatal("expected non-nil FiveHour info")
REDACTED
	if info.FiveHour.Utilization != 0 {
		t.Fatalf("expected Utilization=0 for expired window, got %v", info.FiveHour.Utilization)
REDACTED
	if info.FiveHour.ResetsAt != nil {
		t.Fatalf("expected ResetsAt=nil for expired window, got %v", info.FiveHour.ResetsAt)
REDACTED
	if info.FiveHour.RemainingSeconds != 0 {
		t.Fatalf("expected RemainingSeconds=0 for expired window, got %v", info.FiveHour.RemainingSeconds)
REDACTED
REDACTED

func TestEstimateSetupTokenUsage_ActiveWindowPreservesUtilization(t *testing.T) {
	t.Parallel()

	future := time.Now().Add(3 * time.Hour)
	svc := &AccountUsageService{REDACTED
	info := svc.estimateSetupTokenUsage(&Account{
		SessionWindowEnd: &future,
		Extra: map[string]any{
			"session_window_utilization": 0.53,
	REDACTED,
REDACTED)

	if info.FiveHour == nil {
		t.Fatal("expected non-nil FiveHour info")
REDACTED
	if info.FiveHour.Utilization != 53 {
		t.Fatalf("expected Utilization=53, got %v", info.FiveHour.Utilization)
REDACTED
	if info.FiveHour.ResetsAt == nil || !info.FiveHour.ResetsAt.Equal(future) {
		t.Fatalf("expected ResetsAt=%v, got %v", future, info.FiveHour.ResetsAt)
REDACTED
	if info.FiveHour.RemainingSeconds <= 0 {
		t.Fatalf("expected positive RemainingSeconds, got %v", info.FiveHour.RemainingSeconds)
REDACTED
REDACTED

func TestSyncActiveToPassive_WritesFiveHourSessionWindowEnd(t *testing.T) {
	t.Parallel()

	repo := &sessionWindowSyncRepo{REDACTED
	svc := &AccountUsageService{accountRepo: repoREDACTED
	resetsAt := time.Now().Add(3 * time.Hour).UTC().Truncate(time.Second)
	svc.syncActiveToPassive(context.Background(), 42, &UsageInfo{
		FiveHour: &UsageProgress{
			Utilization: 53,
			ResetsAt:    &resetsAt,
	REDACTED,
REDACTED)

	repo.mu.Lock()
	defer repo.mu.Unlock()
	if len(repo.sessionWindowEnds) != 1 {
		t.Fatalf("expected 1 UpdateSessionWindowEnd call, got %d", len(repo.sessionWindowEnds))
REDACTED
	call := repo.sessionWindowEnds[0]
	if call.AccountID != 42 {
		t.Fatalf("expected AccountID=42, got %d", call.AccountID)
REDACTED
	if !call.End.Equal(resetsAt) {
		t.Fatalf("expected End=%v, got %v", resetsAt, call.End)
REDACTED
REDACTED

func TestSyncActiveToPassive_SkipsSessionWindowEndWhenResetMissing(t *testing.T) {
	t.Parallel()

	repo := &sessionWindowSyncRepo{REDACTED
	svc := &AccountUsageService{accountRepo: repoREDACTED
	svc.syncActiveToPassive(context.Background(), 99, &UsageInfo{
		FiveHour: &UsageProgress{Utilization: 10REDACTED,
REDACTED)

	repo.mu.Lock()
	defer repo.mu.Unlock()
	if len(repo.sessionWindowEnds) != 0 {
		t.Fatalf("expected no UpdateSessionWindowEnd calls when ResetsAt is nil, got %d", len(repo.sessionWindowEnds))
REDACTED
REDACTED
