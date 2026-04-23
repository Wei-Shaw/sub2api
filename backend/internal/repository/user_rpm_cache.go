package repository

import (
	"context"
	"fmt"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/redis/go-redis/v9"
)

// 用户/分组级 RPM 计数器 Redis 实现。
//
// 设计说明：
//   - key 形式：rpm:ug:{uidREDACTED:{gidREDACTED:{minuteREDACTED、rpm:u:{uidREDACTED:{minuteREDACTED
//   - 时间来源：rdb.Time()（Redis 服务端时间），避免多实例时钟漂移。
//   - 原子操作：TxPipeline (MULTI/EXEC) 执行 INCR+EXPIRE，兼容 Redis Cluster。
//   - TTL：120s，覆盖当前分钟窗口 + 少量冗余。
//   - 返回值语义：超限判断由调用方（billing_cache_service.checkRPM）与 RPMLimit 比较完成。
const (
	userGroupRPMKeyPrefix = "rpm:ug:"
	userRPMKeyPrefix      = "rpm:u:"

	userRPMKeyTTL = 120 * time.Second
)

type userRPMCacheImpl struct {
	rdb *redis.Client
REDACTED

// NewUserRPMCache 创建用户/分组级 RPM 计数器。
func NewUserRPMCache(rdb *redis.Client) service.UserRPMCache {
	return &userRPMCacheImpl{rdb: rdbREDACTED
REDACTED

// minuteTS 获取当前 Redis 服务端分钟时间戳。
func (c *userRPMCacheImpl) minuteTS(ctx context.Context) (int64, error) {
	t, err := c.rdb.Time(ctx).Result()
	if err != nil {
		return 0, fmt.Errorf("redis TIME: %w", err)
REDACTED
	return t.Unix() / 60, nil
REDACTED

// atomicIncr 原子 INCR+EXPIRE。
func (c *userRPMCacheImpl) atomicIncr(ctx context.Context, key string) (int, error) {
	pipe := c.rdb.TxPipeline()
	incr := pipe.Incr(ctx, key)
	pipe.Expire(ctx, key, userRPMKeyTTL)
	if _, err := pipe.Exec(ctx); err != nil {
		return 0, fmt.Errorf("user rpm increment: %w", err)
REDACTED
	return int(incr.Val()), nil
REDACTED

// IncrementUserGroupRPM 递增 (user, group) 分钟计数。
func (c *userRPMCacheImpl) IncrementUserGroupRPM(ctx context.Context, userID, groupID int64) (int, error) {
	minute, err := c.minuteTS(ctx)
	if err != nil {
		return 0, err
REDACTED
	key := fmt.Sprintf("%s%d:%d:%d", userGroupRPMKeyPrefix, userID, groupID, minute)
	return c.atomicIncr(ctx, key)
REDACTED

// IncrementUserRPM 递增用户分钟计数。
func (c *userRPMCacheImpl) IncrementUserRPM(ctx context.Context, userID int64) (int, error) {
	minute, err := c.minuteTS(ctx)
	if err != nil {
		return 0, err
REDACTED
	key := fmt.Sprintf("%s%d:%d", userRPMKeyPrefix, userID, minute)
	return c.atomicIncr(ctx, key)
REDACTED

// GetUserGroupRPM 获取 (user, group) 当前分钟已用 RPM（只读）。
func (c *userRPMCacheImpl) GetUserGroupRPM(ctx context.Context, userID, groupID int64) (int, error) {
	minute, err := c.minuteTS(ctx)
	if err != nil {
		return 0, err
REDACTED
	key := fmt.Sprintf("%s%d:%d:%d", userGroupRPMKeyPrefix, userID, groupID, minute)
	val, err := c.rdb.Get(ctx, key).Int()
	if err == redis.Nil {
		return 0, nil
REDACTED
	if err != nil {
		return 0, fmt.Errorf("user group rpm get: %w", err)
REDACTED
	return val, nil
REDACTED

// GetUserRPM 获取用户当前分钟已用 RPM（只读）。
func (c *userRPMCacheImpl) GetUserRPM(ctx context.Context, userID int64) (int, error) {
	minute, err := c.minuteTS(ctx)
	if err != nil {
		return 0, err
REDACTED
	key := fmt.Sprintf("%s%d:%d", userRPMKeyPrefix, userID, minute)
	val, err := c.rdb.Get(ctx, key).Int()
	if err == redis.Nil {
		return 0, nil
REDACTED
	if err != nil {
		return 0, fmt.Errorf("user rpm get: %w", err)
REDACTED
	return val, nil
REDACTED
