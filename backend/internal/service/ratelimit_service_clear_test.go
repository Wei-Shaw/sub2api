//go:build unit

package service

import (
	"context"
	"errors"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/stretchr/testify/require"
)

type rateLimitClearRepoStub struct {
	mockAccountRepoForGemini
	clearRateLimitCalls       int
	clearAntigravityCalls     int
	clearModelRateLimitCalls  int
	clearTempUnschedCalls     int
	clearRateLimitErr         error
	clearAntigravityErr       error
	clearModelRateLimitErr    error
	clearTempUnschedulableErr error
REDACTED

func (r *rateLimitClearRepoStub) ClearRateLimit(ctx context.Context, id int64) error {
	r.clearRateLimitCalls++
	return r.clearRateLimitErr
REDACTED

func (r *rateLimitClearRepoStub) ClearAntigravityQuotaScopes(ctx context.Context, id int64) error {
	r.clearAntigravityCalls++
	return r.clearAntigravityErr
REDACTED

func (r *rateLimitClearRepoStub) ClearModelRateLimits(ctx context.Context, id int64) error {
	r.clearModelRateLimitCalls++
	return r.clearModelRateLimitErr
REDACTED

func (r *rateLimitClearRepoStub) ClearTempUnschedulable(ctx context.Context, id int64) error {
	r.clearTempUnschedCalls++
	return r.clearTempUnschedulableErr
REDACTED

type tempUnschedCacheRecorder struct {
	deletedIDs []int64
	deleteErr  error
REDACTED

func (c *tempUnschedCacheRecorder) SetTempUnsched(ctx context.Context, accountID int64, state *TempUnschedState) error {
	return nil
REDACTED

func (c *tempUnschedCacheRecorder) GetTempUnsched(ctx context.Context, accountID int64) (*TempUnschedState, error) {
	return nil, nil
REDACTED

func (c *tempUnschedCacheRecorder) DeleteTempUnsched(ctx context.Context, accountID int64) error {
	c.deletedIDs = append(c.deletedIDs, accountID)
	return c.deleteErr
REDACTED

func TestRateLimitService_ClearRateLimit_AlsoClearsTempUnschedulable(t *testing.T) {
	repo := &rateLimitClearRepoStub{REDACTED
	cache := &tempUnschedCacheRecorder{REDACTED
	svc := NewRateLimitService(repo, nil, &config.Config{REDACTED, nil, cache)

	err := svc.ClearRateLimit(context.Background(), 42)
REDACTED

	require.Equal(t, 1, repo.clearRateLimitCalls)
	require.Equal(t, 1, repo.clearAntigravityCalls)
	require.Equal(t, 1, repo.clearModelRateLimitCalls)
	require.Equal(t, 1, repo.clearTempUnschedCalls)
	require.Equal(t, []int64{42REDACTED, cache.deletedIDs)
REDACTED

func TestRateLimitService_ClearRateLimit_ClearTempUnschedulableFailed(t *testing.T) {
	repo := &rateLimitClearRepoStub{
		clearTempUnschedulableErr: errors.New("clear temp unsched failed"),
REDACTED
	cache := &tempUnschedCacheRecorder{REDACTED
	svc := NewRateLimitService(repo, nil, &config.Config{REDACTED, nil, cache)

	err := svc.ClearRateLimit(context.Background(), 7)
REDACTED

	require.Equal(t, 1, repo.clearTempUnschedCalls)
	require.Empty(t, cache.deletedIDs)
REDACTED

func TestRateLimitService_ClearRateLimit_ClearRateLimitFailed(t *testing.T) {
	repo := &rateLimitClearRepoStub{
		clearRateLimitErr: errors.New("clear rate limit failed"),
REDACTED
	cache := &tempUnschedCacheRecorder{REDACTED
	svc := NewRateLimitService(repo, nil, &config.Config{REDACTED, nil, cache)

	err := svc.ClearRateLimit(context.Background(), 11)
REDACTED

	require.Equal(t, 1, repo.clearRateLimitCalls)
	require.Equal(t, 0, repo.clearAntigravityCalls)
	require.Equal(t, 0, repo.clearModelRateLimitCalls)
	require.Equal(t, 0, repo.clearTempUnschedCalls)
	require.Empty(t, cache.deletedIDs)
REDACTED

func TestRateLimitService_ClearRateLimit_ClearAntigravityFailed(t *testing.T) {
	repo := &rateLimitClearRepoStub{
		clearAntigravityErr: errors.New("clear antigravity failed"),
REDACTED
	cache := &tempUnschedCacheRecorder{REDACTED
	svc := NewRateLimitService(repo, nil, &config.Config{REDACTED, nil, cache)

	err := svc.ClearRateLimit(context.Background(), 12)
REDACTED

	require.Equal(t, 1, repo.clearRateLimitCalls)
	require.Equal(t, 1, repo.clearAntigravityCalls)
	require.Equal(t, 0, repo.clearModelRateLimitCalls)
	require.Equal(t, 0, repo.clearTempUnschedCalls)
	require.Empty(t, cache.deletedIDs)
REDACTED

func TestRateLimitService_ClearRateLimit_ClearModelRateLimitsFailed(t *testing.T) {
	repo := &rateLimitClearRepoStub{
		clearModelRateLimitErr: errors.New("clear model rate limits failed"),
REDACTED
	cache := &tempUnschedCacheRecorder{REDACTED
	svc := NewRateLimitService(repo, nil, &config.Config{REDACTED, nil, cache)

	err := svc.ClearRateLimit(context.Background(), 13)
REDACTED

	require.Equal(t, 1, repo.clearRateLimitCalls)
	require.Equal(t, 1, repo.clearAntigravityCalls)
	require.Equal(t, 1, repo.clearModelRateLimitCalls)
	require.Equal(t, 0, repo.clearTempUnschedCalls)
	require.Empty(t, cache.deletedIDs)
REDACTED

func TestRateLimitService_ClearRateLimit_CacheDeleteFailedShouldNotFail(t *testing.T) {
	repo := &rateLimitClearRepoStub{REDACTED
	cache := &tempUnschedCacheRecorder{
		deleteErr: errors.New("cache delete failed"),
REDACTED
	svc := NewRateLimitService(repo, nil, &config.Config{REDACTED, nil, cache)

	err := svc.ClearRateLimit(context.Background(), 14)
REDACTED

	require.Equal(t, 1, repo.clearRateLimitCalls)
	require.Equal(t, 1, repo.clearAntigravityCalls)
	require.Equal(t, 1, repo.clearModelRateLimitCalls)
	require.Equal(t, 1, repo.clearTempUnschedCalls)
	require.Equal(t, []int64{14REDACTED, cache.deletedIDs)
REDACTED

func TestRateLimitService_ClearRateLimit_WithoutTempUnschedCache(t *testing.T) {
	repo := &rateLimitClearRepoStub{REDACTED
	svc := NewRateLimitService(repo, nil, &config.Config{REDACTED, nil, nil)

	err := svc.ClearRateLimit(context.Background(), 15)
REDACTED

	require.Equal(t, 1, repo.clearRateLimitCalls)
	require.Equal(t, 1, repo.clearAntigravityCalls)
	require.Equal(t, 1, repo.clearModelRateLimitCalls)
	require.Equal(t, 1, repo.clearTempUnschedCalls)
REDACTED
