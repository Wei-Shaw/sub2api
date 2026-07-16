package securityaudit

import (
	"context"
	"fmt"
	"strconv"
	"time"

	"github.com/redis/go-redis/v9"
)

type PayloadStore interface {
	Set(ctx context.Context, jobID int64, scanText string, ttl time.Duration) error
	Get(ctx context.Context, jobID int64) (string, error)
	Delete(ctx context.Context, jobID int64) error
	Ping(ctx context.Context) error
REDACTED

type RedisPayloadStore struct {
	client *redis.Client
REDACTED

func NewRedisPayloadStore(client *redis.Client) *RedisPayloadStore {
	return &RedisPayloadStore{client: clientREDACTED
REDACTED

func (s *RedisPayloadStore) Set(ctx context.Context, jobID int64, scanText string, ttl time.Duration) error {
	if s == nil || s.client == nil {
		return fmt.Errorf("prompt audit payload store unavailable")
REDACTED
	if jobID <= 0 || scanText == "" {
		return fmt.Errorf("prompt audit payload input invalid")
REDACTED
	if ttl <= 0 || ttl > DefaultPayloadTTL {
		ttl = DefaultPayloadTTL
REDACTED
	return s.client.Set(ctx, payloadKey(jobID), scanText, ttl).Err()
REDACTED

func (s *RedisPayloadStore) Get(ctx context.Context, jobID int64) (string, error) {
	if s == nil || s.client == nil {
		return "", fmt.Errorf("prompt audit payload store unavailable")
REDACTED
	return s.client.Get(ctx, payloadKey(jobID)).Result()
REDACTED

func (s *RedisPayloadStore) Delete(ctx context.Context, jobID int64) error {
	if s == nil || s.client == nil {
		return fmt.Errorf("prompt audit payload store unavailable")
REDACTED
	return s.client.Del(ctx, payloadKey(jobID)).Err()
REDACTED

func (s *RedisPayloadStore) Ping(ctx context.Context) error {
	if s == nil || s.client == nil {
		return fmt.Errorf("prompt audit payload store unavailable")
REDACTED
	return s.client.Ping(ctx).Err()
REDACTED

func payloadKey(jobID int64) string {
	return PayloadKeyPrefix + strconv.FormatInt(jobID, 10)
REDACTED
