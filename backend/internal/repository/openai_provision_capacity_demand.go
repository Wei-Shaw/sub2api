package repository

import (
	"context"
	"fmt"
	"strconv"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/redis/go-redis/v9"
)

const openAIProvisionCapacityDemandKey = "openai:provision:capacity-denied-users"

type openAIProvisionCapacityDemandStore struct {
	rdb *redis.Client
}

func NewOpenAIProvisionCapacityDemandStore(rdb *redis.Client) service.OpenAIProvisionCapacityDemandStore {
	return &openAIProvisionCapacityDemandStore{rdb: rdb}
}

func (s *openAIProvisionCapacityDemandStore) RecordOpenAICapacityDenied(ctx context.Context, userID int64) error {
	if s == nil || s.rdb == nil {
		return nil
	}
	now := time.Now().UTC()
	cutoff := now.Add(-service.OpenAIProvisionCapacitySignalWindow)
	member := strconv.FormatInt(userID, 10)
	pipe := s.rdb.TxPipeline()
	pipe.ZRemRangeByScore(ctx, openAIProvisionCapacityDemandKey, "-inf", strconv.FormatInt(cutoff.UnixMilli(), 10))
	pipe.ZAdd(ctx, openAIProvisionCapacityDemandKey, redis.Z{Score: float64(now.UnixMilli()), Member: member})
	pipe.Expire(ctx, openAIProvisionCapacityDemandKey, service.OpenAIProvisionCapacitySignalWindow+time.Minute)
	if _, err := pipe.Exec(ctx); err != nil {
		return fmt.Errorf("record openai capacity denial: %w", err)
	}
	return nil
}

func (s *openAIProvisionCapacityDemandStore) GetOpenAICapacityDeniedUsers(ctx context.Context, now time.Time) (int64, error) {
	if s == nil || s.rdb == nil {
		return 0, nil
	}
	cutoff := now.Add(-service.OpenAIProvisionCapacitySignalWindow)
	pipe := s.rdb.TxPipeline()
	pipe.ZRemRangeByScore(ctx, openAIProvisionCapacityDemandKey, "-inf", strconv.FormatInt(cutoff.UnixMilli(), 10))
	count := pipe.ZCard(ctx, openAIProvisionCapacityDemandKey)
	if _, err := pipe.Exec(ctx); err != nil {
		return 0, fmt.Errorf("read openai capacity denial demand: %w", err)
	}
	return count.Val(), nil
}
