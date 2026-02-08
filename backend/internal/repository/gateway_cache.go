package repository

import (
	"context"
	"fmt"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/redis/go-redis/v9"
)

const stickySessionPrefix = "sticky_session:"

// 模型负载统计相关常量
const (
	modelLoadKeyPrefix     = "ag:model_load:"      // 模型调用次数 key 前缀
	modelLastUsedKeyPrefix = "ag:model_last_used:" // 模型最后调度时间 key 前缀
	modelLoadTTL           = 24 * time.Hour        // 调用次数 TTL（24 小时无调用后清零）
	modelLastUsedTTL       = 24 * time.Hour        // 最后调度时间 TTL
)

type gatewayCache struct {
	rdb *redis.Client
REDACTED

func NewGatewayCache(rdb *redis.Client) service.GatewayCache {
	return &gatewayCache{rdb: rdbREDACTED
REDACTED

// buildSessionKey 构建 session key，包含 groupID 实现分组隔离
// 格式: sticky_session:{groupIDREDACTED:{sessionHashREDACTED
func buildSessionKey(groupID int64, sessionHash string) string {
	return fmt.Sprintf("%s%d:%s", stickySessionPrefix, groupID, sessionHash)
REDACTED

func (c *gatewayCache) GetSessionAccountID(ctx context.Context, groupID int64, sessionHash string) (int64, error) {
	key := buildSessionKey(groupID, sessionHash)
	return c.rdb.Get(ctx, key).Int64()
REDACTED

func (c *gatewayCache) SetSessionAccountID(ctx context.Context, groupID int64, sessionHash string, accountID int64, ttl time.Duration) error {
	key := buildSessionKey(groupID, sessionHash)
	return c.rdb.Set(ctx, key, accountID, ttl).Err()
REDACTED

func (c *gatewayCache) RefreshSessionTTL(ctx context.Context, groupID int64, sessionHash string, ttl time.Duration) error {
	key := buildSessionKey(groupID, sessionHash)
	return c.rdb.Expire(ctx, key, ttl).Err()
REDACTED

// DeleteSessionAccountID 删除粘性会话与账号的绑定关系。
// 当检测到绑定的账号不可用（如状态错误、禁用、不可调度等）时调用，
// 以便下次请求能够重新选择可用账号。
//
// DeleteSessionAccountID removes the sticky session binding for the given session.
// Called when the bound account becomes unavailable (e.g., error status, disabled,
// or unschedulable), allowing subsequent requests to select a new available account.
func (c *gatewayCache) DeleteSessionAccountID(ctx context.Context, groupID int64, sessionHash string) error {
	key := buildSessionKey(groupID, sessionHash)
	return c.rdb.Del(ctx, key).Err()
REDACTED

// ============ Antigravity 模型负载统计方法 ============

// modelLoadKey 构建模型调用次数 key
// 格式: ag:model_load:{accountIDREDACTED:{modelREDACTED
func modelLoadKey(accountID int64, model string) string {
	return fmt.Sprintf("%s%d:%s", modelLoadKeyPrefix, accountID, model)
REDACTED

// modelLastUsedKey 构建模型最后调度时间 key
// 格式: ag:model_last_used:{accountIDREDACTED:{modelREDACTED
func modelLastUsedKey(accountID int64, model string) string {
	return fmt.Sprintf("%s%d:%s", modelLastUsedKeyPrefix, accountID, model)
REDACTED

// IncrModelCallCount 增加模型调用次数并更新最后调度时间
// 返回更新后的调用次数
func (c *gatewayCache) IncrModelCallCount(ctx context.Context, accountID int64, model string) (int64, error) {
	loadKey := modelLoadKey(accountID, model)
	lastUsedKey := modelLastUsedKey(accountID, model)

	pipe := c.rdb.Pipeline()
	incrCmd := pipe.Incr(ctx, loadKey)
	pipe.Expire(ctx, loadKey, modelLoadTTL) // 每次调用刷新 TTL
	pipe.Set(ctx, lastUsedKey, time.Now().Unix(), modelLastUsedTTL)
	if _, err := pipe.Exec(ctx); err != nil {
		return 0, err
REDACTED
	return incrCmd.Val(), nil
REDACTED

// GetModelLoadBatch 批量获取账号的模型负载信息
func (c *gatewayCache) GetModelLoadBatch(ctx context.Context, accountIDs []int64, model string) (map[int64]*service.ModelLoadInfo, error) {
	if len(accountIDs) == 0 {
		return make(map[int64]*service.ModelLoadInfo), nil
REDACTED

	loadCmds, lastUsedCmds := c.pipelineModelLoadGet(ctx, accountIDs, model)
	return c.parseModelLoadResults(accountIDs, loadCmds, lastUsedCmds), nil
REDACTED

// pipelineModelLoadGet 批量获取模型负载的 Pipeline 操作
func (c *gatewayCache) pipelineModelLoadGet(
	ctx context.Context,
	accountIDs []int64,
	model string,
) (map[int64]*redis.StringCmd, map[int64]*redis.StringCmd) {
	pipe := c.rdb.Pipeline()
	loadCmds := make(map[int64]*redis.StringCmd, len(accountIDs))
	lastUsedCmds := make(map[int64]*redis.StringCmd, len(accountIDs))

	for _, id := range accountIDs {
		loadCmds[id] = pipe.Get(ctx, modelLoadKey(id, model))
		lastUsedCmds[id] = pipe.Get(ctx, modelLastUsedKey(id, model))
REDACTED
	_, _ = pipe.Exec(ctx) // 忽略错误，key 不存在是正常的
	return loadCmds, lastUsedCmds
REDACTED

// parseModelLoadResults 解析 Pipeline 结果
func (c *gatewayCache) parseModelLoadResults(
	accountIDs []int64,
	loadCmds map[int64]*redis.StringCmd,
	lastUsedCmds map[int64]*redis.StringCmd,
) map[int64]*service.ModelLoadInfo {
	result := make(map[int64]*service.ModelLoadInfo, len(accountIDs))
	for _, id := range accountIDs {
		result[id] = &service.ModelLoadInfo{
			CallCount:  getInt64OrZero(loadCmds[id]),
			LastUsedAt: getTimeOrZero(lastUsedCmds[id]),
	REDACTED
REDACTED
	return result
REDACTED

// getInt64OrZero 从 StringCmd 获取 int64 值，失败返回 0
func getInt64OrZero(cmd *redis.StringCmd) int64 {
	val, _ := cmd.Int64()
	return val
REDACTED

// getTimeOrZero 从 StringCmd 获取 time.Time，失败返回零值
func getTimeOrZero(cmd *redis.StringCmd) time.Time {
	val, err := cmd.Int64()
	if err != nil {
		return time.Time{REDACTED
REDACTED
	return time.Unix(val, 0)
REDACTED
