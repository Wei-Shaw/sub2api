package service

import (
	"context"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/stretchr/testify/require"
)

type idempotencyCleanupRepoStub struct {
	deleteCalls int
	lastLimit   int
	deleteErr   error
REDACTED

func (r *idempotencyCleanupRepoStub) CreateProcessing(context.Context, *IdempotencyRecord) (bool, error) {
	return false, nil
REDACTED
func (r *idempotencyCleanupRepoStub) GetByScopeAndKeyHash(context.Context, string, string) (*IdempotencyRecord, error) {
	return nil, nil
REDACTED
func (r *idempotencyCleanupRepoStub) TryReclaim(context.Context, int64, string, time.Time, time.Time, time.Time) (bool, error) {
	return false, nil
REDACTED
func (r *idempotencyCleanupRepoStub) ExtendProcessingLock(context.Context, int64, string, time.Time, time.Time) (bool, error) {
	return false, nil
REDACTED
func (r *idempotencyCleanupRepoStub) MarkSucceeded(context.Context, int64, int, string, time.Time) error {
	return nil
REDACTED
func (r *idempotencyCleanupRepoStub) MarkFailedRetryable(context.Context, int64, string, time.Time, time.Time) error {
	return nil
REDACTED
func (r *idempotencyCleanupRepoStub) DeleteExpired(_ context.Context, _ time.Time, limit int) (int64, error) {
	r.deleteCalls++
	r.lastLimit = limit
	if r.deleteErr != nil {
		return 0, r.deleteErr
REDACTED
	return 1, nil
REDACTED

func TestNewIdempotencyCleanupService_UsesConfig(t *testing.T) {
	repo := &idempotencyCleanupRepoStub{REDACTED
	cfg := &config.Config{
		Idempotency: config.IdempotencyConfig{
			CleanupIntervalSeconds: 7,
			CleanupBatchSize:       321,
	REDACTED,
REDACTED
	svc := NewIdempotencyCleanupService(repo, cfg)
	require.Equal(t, 7*time.Second, svc.interval)
	require.Equal(t, 321, svc.batch)
REDACTED

func TestIdempotencyCleanupService_CleanupOnce(t *testing.T) {
	repo := &idempotencyCleanupRepoStub{REDACTED
	svc := NewIdempotencyCleanupService(repo, &config.Config{
		Idempotency: config.IdempotencyConfig{
			CleanupBatchSize: 99,
	REDACTED,
REDACTED)

	svc.cleanupOnce()
	require.Equal(t, 1, repo.deleteCalls)
	require.Equal(t, 99, repo.lastLimit)
REDACTED
