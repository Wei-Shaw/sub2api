package service

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
)

type opsMetricsProjectionRepo struct {
	AccountRepository
	accounts        []Account
	accountLoads    []AccountWithConcurrency
	listCalls       int
	projectionCalls int
REDACTED

func (r *opsMetricsProjectionRepo) ListSchedulable(context.Context) ([]Account, error) {
	r.listCalls++
	return r.accounts, nil
REDACTED

func (r *opsMetricsProjectionRepo) ListSchedulableAccountLoads(context.Context) ([]AccountWithConcurrency, error) {
	r.projectionCalls++
	return r.accountLoads, nil
REDACTED

type opsMetricsFallbackRepo struct {
	AccountRepository
	accounts  []Account
	listCalls int
REDACTED

func (r *opsMetricsFallbackRepo) ListSchedulable(context.Context) ([]Account, error) {
	r.listCalls++
	return r.accounts, nil
REDACTED

type opsMetricsLoadCache struct {
	ConcurrencyCache
	loads map[int64]*AccountLoadInfo
	got   []AccountWithConcurrency
REDACTED

func (c *opsMetricsLoadCache) GetAccountsLoadBatch(_ context.Context, accounts []AccountWithConcurrency) (map[int64]*AccountLoadInfo, error) {
	c.got = accounts
	return c.loads, nil
REDACTED

func TestCollectConcurrencyQueueDepthUsesProjectionAndPreservesFallbackResult(t *testing.T) {
	loadFactor := 7
	accounts := []Account{
		{ID: 11, Concurrency: 2, LoadFactor: &loadFactorREDACTED,
		{ID: 12, Concurrency: 3REDACTED,
		{ID: 13REDACTED,
REDACTED
	accountLoads := []AccountWithConcurrency{
		{ID: 11, MaxConcurrency: 7REDACTED,
		{ID: 12, MaxConcurrency: 3REDACTED,
		{ID: 13, MaxConcurrency: 1REDACTED,
REDACTED
	loads := map[int64]*AccountLoadInfo{
		11: {AccountID: 11, WaitingCount: 2REDACTED,
		12: {AccountID: 12, WaitingCount: 3REDACTED,
		13: {AccountID: 13, WaitingCount: 0REDACTED,
REDACTED

	projectionRepo := &opsMetricsProjectionRepo{accounts: accounts, accountLoads: accountLoadsREDACTED
	projectionCache := &opsMetricsLoadCache{loads: loadsREDACTED
	projectionConcurrency := NewConcurrencyService(projectionCache)
	projectionConcurrency.SetAccountLoadBatchCacheTTL(0)
	projectionCollector := &OpsMetricsCollector{
		accountRepo:        projectionRepo,
		concurrencyService: projectionConcurrency,
REDACTED

	fallbackRepo := &opsMetricsFallbackRepo{accounts: accountsREDACTED
	fallbackCache := &opsMetricsLoadCache{loads: loadsREDACTED
	fallbackConcurrency := NewConcurrencyService(fallbackCache)
	fallbackConcurrency.SetAccountLoadBatchCacheTTL(0)
	fallbackCollector := &OpsMetricsCollector{
		accountRepo:        fallbackRepo,
		concurrencyService: fallbackConcurrency,
REDACTED

	projectionDepth := projectionCollector.collectConcurrencyQueueDepth(context.Background())
	fallbackDepth := fallbackCollector.collectConcurrencyQueueDepth(context.Background())

	require.NotNil(t, projectionDepth)
	require.NotNil(t, fallbackDepth)
	require.Equal(t, 5, *projectionDepth)
	require.Equal(t, *fallbackDepth, *projectionDepth)
	require.Equal(t, 1, projectionRepo.projectionCalls)
	require.Zero(t, projectionRepo.listCalls)
	require.Equal(t, 1, fallbackRepo.listCalls)
	require.Equal(t, accountLoads, projectionCache.got)
	require.Equal(t, fallbackCache.got, projectionCache.got)
REDACTED

func BenchmarkOpsMetricsCollectorCollectConcurrencyQueueDepth(b *testing.B) {
	const accountCount = 1000
	loadFactor := 8
	accounts := make([]Account, accountCount)
	accountLoads := make([]AccountWithConcurrency, accountCount)
	for i := range accountCount {
		id := int64(i + 1)
		accounts[i] = Account{
			ID:          id,
			Concurrency: 4,
			LoadFactor:  &loadFactor,
	REDACTED
		accountLoads[i] = AccountWithConcurrency{ID: id, MaxConcurrency: loadFactorREDACTED
REDACTED

	repo := &opsMetricsProjectionRepo{accounts: accounts, accountLoads: accountLoadsREDACTED
	cache := &opsMetricsLoadCache{loads: map[int64]*AccountLoadInfo{REDACTEDREDACTED
	concurrency := NewConcurrencyService(cache)
	concurrency.SetAccountLoadBatchCacheTTL(0)
	collector := &OpsMetricsCollector{accountRepo: repo, concurrencyService: concurrencyREDACTED

	b.ReportAllocs()
	b.ResetTimer()
	for range b.N {
		if depth := collector.collectConcurrencyQueueDepth(context.Background()); depth == nil || *depth != 0 {
			b.Fatalf("unexpected queue depth: %v", depth)
	REDACTED
REDACTED
REDACTED
