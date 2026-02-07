//go:build integration

package repository

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"
)

// ============ Gateway Cache 模型负载统计集成测试 ============

type GatewayCacheModelLoadSuite struct {
	suite.Suite
REDACTED

func TestGatewayCacheModelLoadSuite(t *testing.T) {
	suite.Run(t, new(GatewayCacheModelLoadSuite))
REDACTED

func (s *GatewayCacheModelLoadSuite) TestIncrModelCallCount_Basic() {
	t := s.T()
	rdb := testRedis(t)
	cache := &gatewayCache{rdb: rdbREDACTED
	ctx := context.Background()

	accountID := int64(123)
	model := "claude-sonnet-4-20250514"

	// 首次调用应返回 1
	count1, err := cache.IncrModelCallCount(ctx, accountID, model)
REDACTED
	require.Equal(t, int64(1), count1)

	// 第二次调用应返回 2
	count2, err := cache.IncrModelCallCount(ctx, accountID, model)
REDACTED
	require.Equal(t, int64(2), count2)

	// 第三次调用应返回 3
	count3, err := cache.IncrModelCallCount(ctx, accountID, model)
REDACTED
	require.Equal(t, int64(3), count3)
REDACTED

func (s *GatewayCacheModelLoadSuite) TestIncrModelCallCount_DifferentModels() {
	t := s.T()
	rdb := testRedis(t)
	cache := &gatewayCache{rdb: rdbREDACTED
	ctx := context.Background()

	accountID := int64(456)
	model1 := "claude-sonnet-4-20250514"
	model2 := "claude-opus-4-5-20251101"

	// 不同模型应该独立计数
	count1, err := cache.IncrModelCallCount(ctx, accountID, model1)
REDACTED
	require.Equal(t, int64(1), count1)

	count2, err := cache.IncrModelCallCount(ctx, accountID, model2)
REDACTED
	require.Equal(t, int64(1), count2)

	count1Again, err := cache.IncrModelCallCount(ctx, accountID, model1)
REDACTED
	require.Equal(t, int64(2), count1Again)
REDACTED

func (s *GatewayCacheModelLoadSuite) TestIncrModelCallCount_DifferentAccounts() {
	t := s.T()
	rdb := testRedis(t)
	cache := &gatewayCache{rdb: rdbREDACTED
	ctx := context.Background()

	account1 := int64(111)
	account2 := int64(222)
	model := "gemini-2.5-pro"

	// 不同账号应该独立计数
	count1, err := cache.IncrModelCallCount(ctx, account1, model)
REDACTED
	require.Equal(t, int64(1), count1)

	count2, err := cache.IncrModelCallCount(ctx, account2, model)
REDACTED
	require.Equal(t, int64(1), count2)
REDACTED

func (s *GatewayCacheModelLoadSuite) TestGetModelLoadBatch_Empty() {
	t := s.T()
	rdb := testRedis(t)
	cache := &gatewayCache{rdb: rdbREDACTED
	ctx := context.Background()

	result, err := cache.GetModelLoadBatch(ctx, []int64{REDACTED, "any-model")
REDACTED
	require.NotNil(t, result)
	require.Empty(t, result)
REDACTED

func (s *GatewayCacheModelLoadSuite) TestGetModelLoadBatch_NonExistent() {
	t := s.T()
	rdb := testRedis(t)
	cache := &gatewayCache{rdb: rdbREDACTED
	ctx := context.Background()

	// 查询不存在的账号应返回零值
	result, err := cache.GetModelLoadBatch(ctx, []int64{9999, 9998REDACTED, "claude-sonnet-4-20250514")
REDACTED
	require.Len(t, result, 2)

	require.Equal(t, int64(0), result[9999].CallCount)
	require.True(t, result[9999].LastUsedAt.IsZero())
	require.Equal(t, int64(0), result[9998].CallCount)
	require.True(t, result[9998].LastUsedAt.IsZero())
REDACTED

func (s *GatewayCacheModelLoadSuite) TestGetModelLoadBatch_AfterIncrement() {
	t := s.T()
	rdb := testRedis(t)
	cache := &gatewayCache{rdb: rdbREDACTED
	ctx := context.Background()

	accountID := int64(789)
	model := "claude-sonnet-4-20250514"

	// 先增加调用次数
	beforeIncr := time.Now()
	_, err := cache.IncrModelCallCount(ctx, accountID, model)
REDACTED
	_, err = cache.IncrModelCallCount(ctx, accountID, model)
REDACTED
	_, err = cache.IncrModelCallCount(ctx, accountID, model)
REDACTED
	afterIncr := time.Now()

	// 获取负载信息
	result, err := cache.GetModelLoadBatch(ctx, []int64{accountIDREDACTED, model)
REDACTED
	require.Len(t, result, 1)

	loadInfo := result[accountID]
	require.NotNil(t, loadInfo)
	require.Equal(t, int64(3), loadInfo.CallCount)
	require.False(t, loadInfo.LastUsedAt.IsZero())
	// LastUsedAt 应该在 beforeIncr 和 afterIncr 之间
	require.True(t, loadInfo.LastUsedAt.After(beforeIncr.Add(-time.Second)) || loadInfo.LastUsedAt.Equal(beforeIncr))
	require.True(t, loadInfo.LastUsedAt.Before(afterIncr.Add(time.Second)) || loadInfo.LastUsedAt.Equal(afterIncr))
REDACTED

func (s *GatewayCacheModelLoadSuite) TestGetModelLoadBatch_MultipleAccounts() {
	t := s.T()
	rdb := testRedis(t)
	cache := &gatewayCache{rdb: rdbREDACTED
	ctx := context.Background()

	model := "claude-opus-4-5-20251101"
	account1 := int64(1001)
	account2 := int64(1002)
	account3 := int64(1003) // 不调用

	// account1 调用 2 次
	_, err := cache.IncrModelCallCount(ctx, account1, model)
REDACTED
	_, err = cache.IncrModelCallCount(ctx, account1, model)
REDACTED

	// account2 调用 5 次
	for i := 0; i < 5; i++ {
		_, err = cache.IncrModelCallCount(ctx, account2, model)
	REDACTED
REDACTED

	// 批量获取
	result, err := cache.GetModelLoadBatch(ctx, []int64{account1, account2, account3REDACTED, model)
REDACTED
	require.Len(t, result, 3)

	require.Equal(t, int64(2), result[account1].CallCount)
	require.False(t, result[account1].LastUsedAt.IsZero())

	require.Equal(t, int64(5), result[account2].CallCount)
	require.False(t, result[account2].LastUsedAt.IsZero())

	require.Equal(t, int64(0), result[account3].CallCount)
	require.True(t, result[account3].LastUsedAt.IsZero())
REDACTED

func (s *GatewayCacheModelLoadSuite) TestGetModelLoadBatch_ModelIsolation() {
	t := s.T()
	rdb := testRedis(t)
	cache := &gatewayCache{rdb: rdbREDACTED
	ctx := context.Background()

	accountID := int64(2001)
	model1 := "claude-sonnet-4-20250514"
	model2 := "gemini-2.5-pro"

	// 对 model1 调用 3 次
	for i := 0; i < 3; i++ {
		_, err := cache.IncrModelCallCount(ctx, accountID, model1)
	REDACTED
REDACTED

	// 获取 model1 的负载
	result1, err := cache.GetModelLoadBatch(ctx, []int64{accountIDREDACTED, model1)
REDACTED
	require.Equal(t, int64(3), result1[accountID].CallCount)

	// 获取 model2 的负载（应该为 0）
	result2, err := cache.GetModelLoadBatch(ctx, []int64{accountIDREDACTED, model2)
REDACTED
	require.Equal(t, int64(0), result2[accountID].CallCount)
REDACTED

// ============ 辅助函数测试 ============

func (s *GatewayCacheModelLoadSuite) TestModelLoadKey_Format() {
	t := s.T()

	key := modelLoadKey(123, "claude-sonnet-4")
	require.Equal(t, "ag:model_load:123:claude-sonnet-4", key)
REDACTED

func (s *GatewayCacheModelLoadSuite) TestModelLastUsedKey_Format() {
	t := s.T()

	key := modelLastUsedKey(456, "gemini-2.5-pro")
	require.Equal(t, "ag:model_last_used:456:gemini-2.5-pro", key)
REDACTED
