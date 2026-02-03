package repository

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/redis/go-redis/v9"
)

const (
	schedulerBucketSetKey       = "sched:buckets"
	schedulerOutboxWatermarkKey = "sched:outbox:watermark"
	schedulerAccountPrefix      = "sched:acc:"
	schedulerActivePrefix       = "sched:active:"
	schedulerReadyPrefix        = "sched:ready:"
	schedulerVersionPrefix      = "sched:ver:"
	schedulerSnapshotPrefix     = "sched:"
	schedulerLockPrefix         = "sched:lock:"
)

type schedulerCache struct {
	rdb *redis.Client
REDACTED

func NewSchedulerCache(rdb *redis.Client) service.SchedulerCache {
	return &schedulerCache{rdb: rdbREDACTED
REDACTED

func (c *schedulerCache) GetSnapshot(ctx context.Context, bucket service.SchedulerBucket) ([]*service.Account, bool, error) {
	readyKey := schedulerBucketKey(schedulerReadyPrefix, bucket)
	readyVal, err := c.rdb.Get(ctx, readyKey).Result()
	if err == redis.Nil {
		return nil, false, nil
REDACTED
	if err != nil {
		return nil, false, err
REDACTED
	if readyVal != "1" {
		return nil, false, nil
REDACTED

	activeKey := schedulerBucketKey(schedulerActivePrefix, bucket)
	activeVal, err := c.rdb.Get(ctx, activeKey).Result()
	if err == redis.Nil {
		return nil, false, nil
REDACTED
	if err != nil {
		return nil, false, err
REDACTED

	snapshotKey := schedulerSnapshotKey(bucket, activeVal)
	ids, err := c.rdb.ZRange(ctx, snapshotKey, 0, -1).Result()
	if err != nil {
		return nil, false, err
REDACTED
	if len(ids) == 0 {
		// 空快照视为缓存未命中，触发数据库回退查询
		// 这解决了新分组创建后立即绑定账号时的竞态条件问题
		return nil, false, nil
REDACTED

	keys := make([]string, 0, len(ids))
	for _, id := range ids {
		keys = append(keys, schedulerAccountKey(id))
REDACTED
	values, err := c.rdb.MGet(ctx, keys...).Result()
	if err != nil {
		return nil, false, err
REDACTED

	accounts := make([]*service.Account, 0, len(values))
	for _, val := range values {
		if val == nil {
			return nil, false, nil
	REDACTED
		account, err := decodeCachedAccount(val)
		if err != nil {
			return nil, false, err
	REDACTED
		accounts = append(accounts, account)
REDACTED

	return accounts, true, nil
REDACTED

func (c *schedulerCache) SetSnapshot(ctx context.Context, bucket service.SchedulerBucket, accounts []service.Account) error {
	activeKey := schedulerBucketKey(schedulerActivePrefix, bucket)
	oldActive, _ := c.rdb.Get(ctx, activeKey).Result()

	versionKey := schedulerBucketKey(schedulerVersionPrefix, bucket)
	version, err := c.rdb.Incr(ctx, versionKey).Result()
	if err != nil {
		return err
REDACTED

	versionStr := strconv.FormatInt(version, 10)
	snapshotKey := schedulerSnapshotKey(bucket, versionStr)

	pipe := c.rdb.Pipeline()
	for _, account := range accounts {
		payload, err := json.Marshal(account)
		if err != nil {
			return err
	REDACTED
		pipe.Set(ctx, schedulerAccountKey(strconv.FormatInt(account.ID, 10)), payload, 0)
REDACTED
	if len(accounts) > 0 {
		// 使用序号作为 score，保持数据库返回的排序语义。
		members := make([]redis.Z, 0, len(accounts))
		for idx, account := range accounts {
			members = append(members, redis.Z{
				Score:  float64(idx),
				Member: strconv.FormatInt(account.ID, 10),
		REDACTED)
	REDACTED
		pipe.ZAdd(ctx, snapshotKey, members...)
REDACTED else {
		pipe.Del(ctx, snapshotKey)
REDACTED
	pipe.Set(ctx, activeKey, versionStr, 0)
	pipe.Set(ctx, schedulerBucketKey(schedulerReadyPrefix, bucket), "1", 0)
	pipe.SAdd(ctx, schedulerBucketSetKey, bucket.String())
	if _, err := pipe.Exec(ctx); err != nil {
		return err
REDACTED

	if oldActive != "" && oldActive != versionStr {
		_ = c.rdb.Del(ctx, schedulerSnapshotKey(bucket, oldActive)).Err()
REDACTED

	return nil
REDACTED

func (c *schedulerCache) GetAccount(ctx context.Context, accountID int64) (*service.Account, error) {
	key := schedulerAccountKey(strconv.FormatInt(accountID, 10))
	val, err := c.rdb.Get(ctx, key).Result()
	if err == redis.Nil {
		return nil, nil
REDACTED
	if err != nil {
		return nil, err
REDACTED
	return decodeCachedAccount(val)
REDACTED

func (c *schedulerCache) SetAccount(ctx context.Context, account *service.Account) error {
	if account == nil || account.ID <= 0 {
		return nil
REDACTED
	payload, err := json.Marshal(account)
	if err != nil {
		return err
REDACTED
	key := schedulerAccountKey(strconv.FormatInt(account.ID, 10))
	return c.rdb.Set(ctx, key, payload, 0).Err()
REDACTED

func (c *schedulerCache) DeleteAccount(ctx context.Context, accountID int64) error {
	if accountID <= 0 {
		return nil
REDACTED
	key := schedulerAccountKey(strconv.FormatInt(accountID, 10))
	return c.rdb.Del(ctx, key).Err()
REDACTED

func (c *schedulerCache) UpdateLastUsed(ctx context.Context, updates map[int64]time.Time) error {
	if len(updates) == 0 {
		return nil
REDACTED

	keys := make([]string, 0, len(updates))
	ids := make([]int64, 0, len(updates))
	for id := range updates {
		keys = append(keys, schedulerAccountKey(strconv.FormatInt(id, 10)))
		ids = append(ids, id)
REDACTED

	values, err := c.rdb.MGet(ctx, keys...).Result()
	if err != nil {
		return err
REDACTED

	pipe := c.rdb.Pipeline()
	for i, val := range values {
		if val == nil {
			continue
	REDACTED
		account, err := decodeCachedAccount(val)
		if err != nil {
			return err
	REDACTED
		account.LastUsedAt = ptrTime(updates[ids[i]])
		updated, err := json.Marshal(account)
		if err != nil {
			return err
	REDACTED
		pipe.Set(ctx, keys[i], updated, 0)
REDACTED
	_, err = pipe.Exec(ctx)
	return err
REDACTED

func (c *schedulerCache) TryLockBucket(ctx context.Context, bucket service.SchedulerBucket, ttl time.Duration) (bool, error) {
	key := schedulerBucketKey(schedulerLockPrefix, bucket)
	return c.rdb.SetNX(ctx, key, time.Now().UnixNano(), ttl).Result()
REDACTED

func (c *schedulerCache) ListBuckets(ctx context.Context) ([]service.SchedulerBucket, error) {
	raw, err := c.rdb.SMembers(ctx, schedulerBucketSetKey).Result()
	if err != nil {
		return nil, err
REDACTED
	out := make([]service.SchedulerBucket, 0, len(raw))
	for _, entry := range raw {
		bucket, ok := service.ParseSchedulerBucket(entry)
		if !ok {
			continue
	REDACTED
		out = append(out, bucket)
REDACTED
	return out, nil
REDACTED

func (c *schedulerCache) GetOutboxWatermark(ctx context.Context) (int64, error) {
	val, err := c.rdb.Get(ctx, schedulerOutboxWatermarkKey).Result()
	if err == redis.Nil {
		return 0, nil
REDACTED
	if err != nil {
		return 0, err
REDACTED
	id, err := strconv.ParseInt(val, 10, 64)
	if err != nil {
		return 0, err
REDACTED
	return id, nil
REDACTED

func (c *schedulerCache) SetOutboxWatermark(ctx context.Context, id int64) error {
	return c.rdb.Set(ctx, schedulerOutboxWatermarkKey, strconv.FormatInt(id, 10), 0).Err()
REDACTED

func schedulerBucketKey(prefix string, bucket service.SchedulerBucket) string {
	return fmt.Sprintf("%s%d:%s:%s", prefix, bucket.GroupID, bucket.Platform, bucket.Mode)
REDACTED

func schedulerSnapshotKey(bucket service.SchedulerBucket, version string) string {
	return fmt.Sprintf("%s%d:%s:%s:v%s", schedulerSnapshotPrefix, bucket.GroupID, bucket.Platform, bucket.Mode, version)
REDACTED

func schedulerAccountKey(id string) string {
	return schedulerAccountPrefix + id
REDACTED

func ptrTime(t time.Time) *time.Time {
	return &t
REDACTED

func decodeCachedAccount(val any) (*service.Account, error) {
	var payload []byte
	switch raw := val.(type) {
	case string:
		payload = []byte(raw)
	case []byte:
		payload = raw
	default:
		return nil, fmt.Errorf("unexpected account cache type: %T", val)
REDACTED
	var account service.Account
	if err := json.Unmarshal(payload, &account); err != nil {
		return nil, err
REDACTED
	return &account, nil
REDACTED
