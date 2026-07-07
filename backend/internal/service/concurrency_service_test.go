//go:build unit

package service

import (
	"context"
	"errors"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// stubConcurrencyCacheForTest 用于并发服务单元测试的缓存桩
type stubConcurrencyCacheForTest struct {
	acquireResult        bool
	acquireErr           error
	releaseErr           error
	concurrency          int
	concurrencyErr       error
	waitAllowed          bool
	waitErr              error
	waitCount            int
	waitCountErr         error
	loadBatch            map[int64]*AccountLoadInfo
	loadBatchErr         error
	usersLoadBatch       map[int64]*UserLoadInfo
	usersLoadErr         error
	cleanupErr           error
	apiKeyTrackErr       error
	apiKeyReleaseErr     error
	apiKeyConcurrency    map[int64]int
	apiKeyConcurrencyErr error

	// 记录调用
	releasedAccountIDs       []int64
	releasedRequestIDs       []string
	loadBatchCalls           atomic.Int64
	trackedAPIKeyIDs         []int64
	trackedAPIKeyRequestIDs  []string
	releasedAPIKeyIDs        []int64
	releasedAPIKeyRequestIDs []string
REDACTED

var _ ConcurrencyCache = (*stubConcurrencyCacheForTest)(nil)

func (c *stubConcurrencyCacheForTest) AcquireAccountSlot(_ context.Context, _ int64, _ int, _ string) (bool, error) {
	return c.acquireResult, c.acquireErr
REDACTED
func (c *stubConcurrencyCacheForTest) ReleaseAccountSlot(_ context.Context, accountID int64, requestID string) error {
	c.releasedAccountIDs = append(c.releasedAccountIDs, accountID)
	c.releasedRequestIDs = append(c.releasedRequestIDs, requestID)
	return c.releaseErr
REDACTED
func (c *stubConcurrencyCacheForTest) GetAccountConcurrency(_ context.Context, _ int64) (int, error) {
	return c.concurrency, c.concurrencyErr
REDACTED
func (c *stubConcurrencyCacheForTest) GetAccountConcurrencyBatch(_ context.Context, accountIDs []int64) (map[int64]int, error) {
	result := make(map[int64]int, len(accountIDs))
	for _, accountID := range accountIDs {
		if c.concurrencyErr != nil {
			return nil, c.concurrencyErr
	REDACTED
		result[accountID] = c.concurrency
REDACTED
	return result, nil
REDACTED
func (c *stubConcurrencyCacheForTest) IncrementAccountWaitCount(_ context.Context, _ int64, _ int) (bool, error) {
	return c.waitAllowed, c.waitErr
REDACTED
func (c *stubConcurrencyCacheForTest) DecrementAccountWaitCount(_ context.Context, _ int64) error {
	return nil
REDACTED
func (c *stubConcurrencyCacheForTest) GetAccountWaitingCount(_ context.Context, _ int64) (int, error) {
	return c.waitCount, c.waitCountErr
REDACTED
func (c *stubConcurrencyCacheForTest) AcquireUserSlot(_ context.Context, _ int64, _ int, _ string) (bool, error) {
	return c.acquireResult, c.acquireErr
REDACTED
func (c *stubConcurrencyCacheForTest) ReleaseUserSlot(_ context.Context, _ int64, _ string) error {
	return c.releaseErr
REDACTED
func (c *stubConcurrencyCacheForTest) GetUserConcurrency(_ context.Context, _ int64) (int, error) {
	return c.concurrency, c.concurrencyErr
REDACTED
func (c *stubConcurrencyCacheForTest) TrackAPIKeySlot(_ context.Context, apiKeyID int64, requestID string) error {
	c.trackedAPIKeyIDs = append(c.trackedAPIKeyIDs, apiKeyID)
	c.trackedAPIKeyRequestIDs = append(c.trackedAPIKeyRequestIDs, requestID)
	return c.apiKeyTrackErr
REDACTED
func (c *stubConcurrencyCacheForTest) ReleaseAPIKeySlot(_ context.Context, apiKeyID int64, requestID string) error {
	c.releasedAPIKeyIDs = append(c.releasedAPIKeyIDs, apiKeyID)
	c.releasedAPIKeyRequestIDs = append(c.releasedAPIKeyRequestIDs, requestID)
	return c.apiKeyReleaseErr
REDACTED
func (c *stubConcurrencyCacheForTest) GetAPIKeyConcurrencyBatch(_ context.Context, apiKeyIDs []int64) (map[int64]int, error) {
	if c.apiKeyConcurrencyErr != nil {
		return nil, c.apiKeyConcurrencyErr
REDACTED
	result := make(map[int64]int, len(apiKeyIDs))
	for _, apiKeyID := range apiKeyIDs {
		result[apiKeyID] = c.apiKeyConcurrency[apiKeyID]
REDACTED
	return result, nil
REDACTED
func (c *stubConcurrencyCacheForTest) IncrementWaitCount(_ context.Context, _ int64, _ int) (bool, error) {
	return c.waitAllowed, c.waitErr
REDACTED
func (c *stubConcurrencyCacheForTest) DecrementWaitCount(_ context.Context, _ int64) error {
	return nil
REDACTED
func (c *stubConcurrencyCacheForTest) GetAccountsLoadBatch(_ context.Context, _ []AccountWithConcurrency) (map[int64]*AccountLoadInfo, error) {
	c.loadBatchCalls.Add(1)
	return c.loadBatch, c.loadBatchErr
REDACTED
func (c *stubConcurrencyCacheForTest) GetUsersLoadBatch(_ context.Context, _ []UserWithConcurrency) (map[int64]*UserLoadInfo, error) {
	return c.usersLoadBatch, c.usersLoadErr
REDACTED
func (c *stubConcurrencyCacheForTest) CleanupExpiredAccountSlots(_ context.Context, _ int64) error {
	return c.cleanupErr
REDACTED

func (c *stubConcurrencyCacheForTest) CleanupExpiredAccountSlotKeys(_ context.Context) error {
	return c.cleanupErr
REDACTED

func (c *stubConcurrencyCacheForTest) CleanupStaleProcessSlots(_ context.Context, _ string) error {
	return c.cleanupErr
REDACTED

type trackingConcurrencyCache struct {
	stubConcurrencyCacheForTest
	cleanupPrefix string
REDACTED

func (c *trackingConcurrencyCache) CleanupStaleProcessSlots(_ context.Context, prefix string) error {
	c.cleanupPrefix = prefix
	return c.cleanupErr
REDACTED

func TestCleanupStaleProcessSlots_NilCache(t *testing.T) {
	svc := &ConcurrencyService{cache: nilREDACTED
	require.NoError(t, svc.CleanupStaleProcessSlots(context.Background()))
REDACTED

func TestCleanupStaleProcessSlots_DelegatesPrefix(t *testing.T) {
	cache := &trackingConcurrencyCache{REDACTED
	svc := NewConcurrencyService(cache)
	require.NoError(t, svc.CleanupStaleProcessSlots(context.Background()))
	require.Equal(t, RequestIDPrefix(), cache.cleanupPrefix)
REDACTED

func TestAcquireAccountSlot_Success(t *testing.T) {
	cache := &stubConcurrencyCacheForTest{acquireResult: trueREDACTED
	svc := NewConcurrencyService(cache)

	result, err := svc.AcquireAccountSlot(context.Background(), 1, 5)
REDACTED
	require.True(t, result.Acquired)
	require.NotNil(t, result.ReleaseFunc)
REDACTED

func TestAcquireAccountSlot_Failure(t *testing.T) {
	cache := &stubConcurrencyCacheForTest{acquireResult: falseREDACTED
	svc := NewConcurrencyService(cache)

	result, err := svc.AcquireAccountSlot(context.Background(), 1, 5)
REDACTED
	require.False(t, result.Acquired)
	require.Nil(t, result.ReleaseFunc)
REDACTED

func TestAcquireAccountSlot_UnlimitedConcurrency(t *testing.T) {
	svc := NewConcurrencyService(&stubConcurrencyCacheForTest{REDACTED)

	for _, maxConcurrency := range []int{0, -1REDACTED {
		result, err := svc.AcquireAccountSlot(context.Background(), 1, maxConcurrency)
	REDACTED
		require.True(t, result.Acquired, "maxConcurrency=%d 应无限制通过", maxConcurrency)
		require.NotNil(t, result.ReleaseFunc, "ReleaseFunc 应为 no-op 函数")
REDACTED
REDACTED

func TestAcquireAccountSlot_CacheError(t *testing.T) {
	cache := &stubConcurrencyCacheForTest{acquireErr: errors.New("redis down")REDACTED
	svc := NewConcurrencyService(cache)

	result, err := svc.AcquireAccountSlot(context.Background(), 1, 5)
REDACTED
	require.Nil(t, result)
REDACTED

func TestAcquireAccountSlot_ReleaseDecrements(t *testing.T) {
	cache := &stubConcurrencyCacheForTest{acquireResult: trueREDACTED
	svc := NewConcurrencyService(cache)

	result, err := svc.AcquireAccountSlot(context.Background(), 42, 5)
REDACTED
	require.True(t, result.Acquired)

	// 调用 ReleaseFunc 应释放槽位
	result.ReleaseFunc()

	require.Len(t, cache.releasedAccountIDs, 1)
	require.Equal(t, int64(42), cache.releasedAccountIDs[0])
	require.Len(t, cache.releasedRequestIDs, 1)
	require.NotEmpty(t, cache.releasedRequestIDs[0], "requestID 不应为空")
REDACTED

func TestAcquireUserSlot_IndependentFromAccount(t *testing.T) {
	cache := &stubConcurrencyCacheForTest{acquireResult: trueREDACTED
	svc := NewConcurrencyService(cache)

	// 用户槽位获取应独立于账户槽位
	result, err := svc.AcquireUserSlot(context.Background(), 100, 3)
REDACTED
	require.True(t, result.Acquired)
	require.NotNil(t, result.ReleaseFunc)
REDACTED

func TestAcquireUserSlot_UnlimitedConcurrency(t *testing.T) {
	svc := NewConcurrencyService(&stubConcurrencyCacheForTest{REDACTED)

	result, err := svc.AcquireUserSlot(context.Background(), 1, 0)
REDACTED
	require.True(t, result.Acquired)
REDACTED

func TestTrackAPIKeySlot_ReleaseDecrements(t *testing.T) {
	cache := &stubConcurrencyCacheForTest{REDACTED
	svc := NewConcurrencyService(cache)

	release := svc.TrackAPIKeySlot(context.Background(), 88)
	require.NotNil(t, release)
	require.Equal(t, []int64{88REDACTED, cache.trackedAPIKeyIDs)
	require.Len(t, cache.trackedAPIKeyRequestIDs, 1)
	require.NotEmpty(t, cache.trackedAPIKeyRequestIDs[0])

	release()

	require.Equal(t, []int64{88REDACTED, cache.releasedAPIKeyIDs)
	require.Equal(t, cache.trackedAPIKeyRequestIDs, cache.releasedAPIKeyRequestIDs)
REDACTED

func TestTrackAPIKeySlot_FailOpen(t *testing.T) {
	cache := &stubConcurrencyCacheForTest{apiKeyTrackErr: errors.New("redis down")REDACTED
	svc := NewConcurrencyService(cache)

	release := svc.TrackAPIKeySlot(context.Background(), 88)
	require.NotNil(t, release)
	require.Equal(t, []int64{88REDACTED, cache.trackedAPIKeyIDs)

	require.NotPanics(t, release)
	require.Empty(t, cache.releasedAPIKeyIDs)
REDACTED

func TestGetAPIKeyConcurrencyBatch_Fallbacks(t *testing.T) {
	t.Run("nil cache returns zeroes", func(t *testing.T) {
		svc := &ConcurrencyService{cache: nilREDACTED

		counts, err := svc.GetAPIKeyConcurrencyBatch(context.Background(), []int64{1, 2REDACTED)
	REDACTED
		require.Equal(t, map[int64]int{1: 0, 2: 0REDACTED, counts)
REDACTED)

	t.Run("redis error returns zeroes", func(t *testing.T) {
		cache := &stubConcurrencyCacheForTest{apiKeyConcurrencyErr: errors.New("redis down")REDACTED
		svc := NewConcurrencyService(cache)

		counts, err := svc.GetAPIKeyConcurrencyBatch(context.Background(), []int64{1, 2REDACTED)
	REDACTED
		require.Equal(t, map[int64]int{1: 0, 2: 0REDACTED, counts)
REDACTED)

	t.Run("success returns counts", func(t *testing.T) {
		cache := &stubConcurrencyCacheForTest{apiKeyConcurrency: map[int64]int{1: 3, 2: 0REDACTEDREDACTED
		svc := NewConcurrencyService(cache)

		counts, err := svc.GetAPIKeyConcurrencyBatch(context.Background(), []int64{1, 2REDACTED)
	REDACTED
		require.Equal(t, map[int64]int{1: 3, 2: 0REDACTED, counts)
REDACTED)
REDACTED

func TestGenerateRequestID_UsesStablePrefixAndMonotonicCounter(t *testing.T) {
	id1 := generateRequestID()
	id2 := generateRequestID()
	require.NotEmpty(t, id1)
	require.NotEmpty(t, id2)

	p1 := strings.Split(id1, "-")
	p2 := strings.Split(id2, "-")
	require.Len(t, p1, 2)
	require.Len(t, p2, 2)
	require.Equal(t, p1[0], p2[0], "同一进程前缀应保持一致")

	n1, err := strconv.ParseUint(p1[1], 36, 64)
REDACTED
	n2, err := strconv.ParseUint(p2[1], 36, 64)
REDACTED
	require.Equal(t, n1+1, n2, "计数器应单调递增")
REDACTED

func TestGetAccountsLoadBatch_ReturnsCorrectData(t *testing.T) {
	expected := map[int64]*AccountLoadInfo{
		1: {AccountID: 1, CurrentConcurrency: 3, WaitingCount: 0, LoadRate: 60REDACTED,
		2: {AccountID: 2, CurrentConcurrency: 5, WaitingCount: 2, LoadRate: 100REDACTED,
REDACTED
	cache := &stubConcurrencyCacheForTest{loadBatch: expectedREDACTED
	svc := NewConcurrencyService(cache)

	accounts := []AccountWithConcurrency{
		{ID: 1, MaxConcurrency: 5REDACTED,
		{ID: 2, MaxConcurrency: 5REDACTED,
REDACTED
	result, err := svc.GetAccountsLoadBatch(context.Background(), accounts)
REDACTED
	require.Equal(t, expected, result)
REDACTED

func TestGetAccountsLoadBatch_NilCache(t *testing.T) {
	svc := &ConcurrencyService{cache: nilREDACTED

	result, err := svc.GetAccountsLoadBatch(context.Background(), nil)
REDACTED
	require.Empty(t, result)
REDACTED

func TestGetAccountsLoadBatch_UsesShortTTLCache(t *testing.T) {
	cache := &stubConcurrencyCacheForTest{
		loadBatch: map[int64]*AccountLoadInfo{
			1: {AccountID: 1, CurrentConcurrency: 1, LoadRate: 20REDACTED,
	REDACTED,
REDACTED
	svc := NewConcurrencyService(cache)
	svc.SetAccountLoadBatchCacheTTL(time.Second)

	accounts := []AccountWithConcurrency{{ID: 1, MaxConcurrency: 5REDACTEDREDACTED
	first, err := svc.GetAccountsLoadBatch(context.Background(), accounts)
REDACTED
	require.Equal(t, 1, first[int64(1)].CurrentConcurrency)

	cache.loadBatch[1] = &AccountLoadInfo{AccountID: 1, CurrentConcurrency: 4, LoadRate: 80REDACTED
	second, err := svc.GetAccountsLoadBatch(context.Background(), accounts)
REDACTED
	require.Equal(t, 1, second[int64(1)].CurrentConcurrency)
	require.Equal(t, int64(1), cache.loadBatchCalls.Load())
REDACTED

func TestGetAccountsLoadBatchFresh_BypassesShortTTLCache(t *testing.T) {
	cache := &stubConcurrencyCacheForTest{
		loadBatch: map[int64]*AccountLoadInfo{
			1: {AccountID: 1, CurrentConcurrency: 1, LoadRate: 20REDACTED,
	REDACTED,
REDACTED
	svc := NewConcurrencyService(cache)
	svc.SetAccountLoadBatchCacheTTL(time.Second)

	accounts := []AccountWithConcurrency{{ID: 1, MaxConcurrency: 5REDACTEDREDACTED
	_, err := svc.GetAccountsLoadBatch(context.Background(), accounts)
REDACTED

	cache.loadBatch[1] = &AccountLoadInfo{AccountID: 1, CurrentConcurrency: 4, LoadRate: 80REDACTED
	fresh, err := svc.GetAccountsLoadBatchFresh(context.Background(), accounts)
REDACTED
	require.Equal(t, 4, fresh[int64(1)].CurrentConcurrency)
	require.Equal(t, int64(2), cache.loadBatchCalls.Load())
REDACTED

func TestIncrementWaitCount_Success(t *testing.T) {
	cache := &stubConcurrencyCacheForTest{waitAllowed: trueREDACTED
	svc := NewConcurrencyService(cache)

	allowed, err := svc.IncrementWaitCount(context.Background(), 1, 25)
REDACTED
	require.True(t, allowed)
REDACTED

func TestIncrementWaitCount_QueueFull(t *testing.T) {
	cache := &stubConcurrencyCacheForTest{waitAllowed: falseREDACTED
	svc := NewConcurrencyService(cache)

	allowed, err := svc.IncrementWaitCount(context.Background(), 1, 25)
REDACTED
	require.False(t, allowed)
REDACTED

func TestIncrementWaitCount_FailOpen(t *testing.T) {
	// Redis 错误时应 fail-open（允许请求通过）
	cache := &stubConcurrencyCacheForTest{waitErr: errors.New("redis timeout")REDACTED
	svc := NewConcurrencyService(cache)

	allowed, err := svc.IncrementWaitCount(context.Background(), 1, 25)
	require.NoError(t, err, "Redis 错误不应传播")
	require.True(t, allowed, "Redis 错误时应 fail-open")
REDACTED

func TestIncrementWaitCount_NilCache(t *testing.T) {
	svc := &ConcurrencyService{cache: nilREDACTED

	allowed, err := svc.IncrementWaitCount(context.Background(), 1, 25)
REDACTED
	require.True(t, allowed, "nil cache 应 fail-open")
REDACTED

func TestCalculateMaxWait(t *testing.T) {
	tests := []struct {
		concurrency int
		expected    int
REDACTED{
		{5, 25REDACTED,  // 5 + 20
		{1, 21REDACTED,  // 1 + 20
		{0, 21REDACTED,  // min(1) + 20
		{-1, 21REDACTED, // min(1) + 20
		{10, 30REDACTED, // 10 + 20
REDACTED
	for _, tt := range tests {
		result := CalculateMaxWait(tt.concurrency)
		require.Equal(t, tt.expected, result, "CalculateMaxWait(%d)", tt.concurrency)
REDACTED
REDACTED

func TestGetAccountWaitingCount(t *testing.T) {
	cache := &stubConcurrencyCacheForTest{waitCount: 5REDACTED
	svc := NewConcurrencyService(cache)

	count, err := svc.GetAccountWaitingCount(context.Background(), 1)
REDACTED
	require.Equal(t, 5, count)
REDACTED

func TestGetAccountWaitingCount_NilCache(t *testing.T) {
	svc := &ConcurrencyService{cache: nilREDACTED

	count, err := svc.GetAccountWaitingCount(context.Background(), 1)
REDACTED
	require.Equal(t, 0, count)
REDACTED

func TestGetAccountConcurrencyBatch(t *testing.T) {
	cache := &stubConcurrencyCacheForTest{concurrency: 3REDACTED
	svc := NewConcurrencyService(cache)

	result, err := svc.GetAccountConcurrencyBatch(context.Background(), []int64{1, 2, 3REDACTED)
REDACTED
	require.Len(t, result, 3)
	for _, id := range []int64{1, 2, 3REDACTED {
		require.Equal(t, 3, result[id])
REDACTED
REDACTED

func TestIncrementAccountWaitCount_FailOpen(t *testing.T) {
	cache := &stubConcurrencyCacheForTest{waitErr: errors.New("redis error")REDACTED
	svc := NewConcurrencyService(cache)

	allowed, err := svc.IncrementAccountWaitCount(context.Background(), 1, 10)
	require.NoError(t, err, "Redis 错误不应传播")
	require.True(t, allowed, "Redis 错误时应 fail-open")
REDACTED

func TestIncrementAccountWaitCount_NilCache(t *testing.T) {
	svc := &ConcurrencyService{cache: nilREDACTED

	allowed, err := svc.IncrementAccountWaitCount(context.Background(), 1, 10)
REDACTED
	require.True(t, allowed)
REDACTED
