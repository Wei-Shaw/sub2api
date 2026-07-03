package repository

import (
	"context"
	"sync"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/redis/go-redis/v9"
)

const (
	defaultBatchImageDownloadActivePrefix = "batch_image:download:active:"
	defaultBatchImageDownloadActiveTTL    = 10 * time.Minute
	defaultBatchImageDownloadConcurrency  = 2
)

var batchImageDownloadAcquireScript = redis.NewScript(`
local current = tonumber(redis.call("GET", KEYS[1]) or "0")
local max = tonumber(ARGV[1])
if current >= max then
  return 0
end
redis.call("INCR", KEYS[1])
redis.call("EXPIRE", KEYS[1], ARGV[2])
return 1
`)

var batchImageDownloadReleaseScript = redis.NewScript(`
local current = tonumber(redis.call("GET", KEYS[1]) or "0")
if current <= 1 then
  redis.call("DEL", KEYS[1])
  return 0
end
return redis.call("DECR", KEYS[1])
`)

type batchImageDownloadLimiter struct {
	rdb          *redis.Client
	activePrefix string
	maxActive    int
	ttl          time.Duration
REDACTED

func NewBatchImageDownloadLimiter(rdb *redis.Client, cfg *config.Config) service.BatchImageDownloadLimiter {
	maxActive := defaultBatchImageDownloadConcurrency
	ttl := defaultBatchImageDownloadActiveTTL
	if cfg != nil {
		if cfg.BatchImage.MaxDownloadConcurrencyPerUser > 0 {
			maxActive = cfg.BatchImage.MaxDownloadConcurrencyPerUser
	REDACTED
		if cfg.BatchImage.MaxDownloadDurationSeconds > 0 {
			ttl = time.Duration(cfg.BatchImage.MaxDownloadDurationSeconds) * time.Second
	REDACTED
REDACTED
	return &batchImageDownloadLimiter{
		rdb:          rdb,
		activePrefix: defaultBatchImageDownloadActivePrefix,
		maxActive:    maxActive,
		ttl:          ttl,
REDACTED
REDACTED

func newBatchImageDownloadLimiterForTest(rdb *redis.Client, maxActive int, ttl time.Duration) *batchImageDownloadLimiter {
	if maxActive <= 0 {
		maxActive = defaultBatchImageDownloadConcurrency
REDACTED
	if ttl <= 0 {
		ttl = defaultBatchImageDownloadActiveTTL
REDACTED
	return &batchImageDownloadLimiter{rdb: rdb, activePrefix: defaultBatchImageDownloadActivePrefix, maxActive: maxActive, ttl: ttlREDACTED
REDACTED

func (l *batchImageDownloadLimiter) Acquire(ctx context.Context, userID string, kind string) (service.BatchImageDownloadPermit, error) {
	if l == nil || l.rdb == nil {
		return nil, service.ErrBatchImageDownloadLimited
REDACTED
	key := l.activeKey(userID)
	ok, err := batchImageDownloadAcquireScript.Run(ctx, l.rdb, []string{keyREDACTED, l.maxActive, int(l.ttl.Seconds())).Int()
	if err != nil {
		return nil, err
REDACTED
	if ok != 1 {
		return nil, service.ErrBatchImageDownloadLimited
REDACTED
	return &batchImageDownloadPermit{rdb: l.rdb, key: keyREDACTED, nil
REDACTED

func (l *batchImageDownloadLimiter) activeKey(userID string) string {
	return l.activePrefix + userID
REDACTED

type batchImageDownloadPermit struct {
	rdb  *redis.Client
	key  string
	once sync.Once
	err  error
REDACTED

func (p *batchImageDownloadPermit) Release(ctx context.Context) error {
	if p == nil || p.rdb == nil || p.key == "" {
		return nil
REDACTED
	p.once.Do(func() {
		_, p.err = batchImageDownloadReleaseScript.Run(ctx, p.rdb, []string{p.keyREDACTED).Result()
REDACTED)
	return p.err
REDACTED

var _ service.BatchImageDownloadLimiter = (*batchImageDownloadLimiter)(nil)
var _ service.BatchImageDownloadPermit = (*batchImageDownloadPermit)(nil)
