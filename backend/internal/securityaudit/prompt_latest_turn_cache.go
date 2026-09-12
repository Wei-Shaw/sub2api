package securityaudit

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/redis/go-redis/v9"
)

const (
	LatestTurnDecisionCacheTTL  = 5 * time.Minute
	latestTurnDecisionKeyPrefix = "sub2api:prompt_guard:latest_turn:"
)

type LatestTurnDecisionCache interface {
	GetLatestTurnDecision(ctx context.Context, key string) (*PromptDecision, error)
	SetLatestTurnDecision(ctx context.Context, key string, decision *PromptDecision, ttl time.Duration) error
}

func cacheableLatestTurnDecision(decision *PromptDecision) bool {
	return decision != nil && decision.AllowNextStage && (decision.Kind == DecisionAllow || decision.Kind == DecisionFlag)
}

func latestTurnDecisionCacheKey(configVersion int64, promptHash string) string {
	promptHash = strings.TrimSpace(promptHash)
	if promptHash == "" {
		return ""
	}
	return latestTurnDecisionKeyPrefix + strconv.FormatInt(configVersion, 10) + ":" + promptHash
}

func (s *RedisPayloadStore) GetLatestTurnDecision(ctx context.Context, key string) (*PromptDecision, error) {
	if s == nil || s.client == nil {
		return nil, fmt.Errorf("latest turn decision cache unavailable")
	}
	key = strings.TrimSpace(key)
	if key == "" {
		return nil, nil
	}
	raw, err := s.client.Get(ctx, key).Result()
	if err == redis.Nil {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var decision PromptDecision
	if err := json.Unmarshal([]byte(raw), &decision); err != nil || !cacheableLatestTurnDecision(&decision) {
		_ = s.client.Del(ctx, key).Err()
		if err == nil {
			err = fmt.Errorf("cached decision does not allow next stage")
		}
		return nil, err
	}
	return &decision, nil
}

func (s *RedisPayloadStore) SetLatestTurnDecision(ctx context.Context, key string, decision *PromptDecision, ttl time.Duration) error {
	if s == nil || s.client == nil {
		return fmt.Errorf("latest turn decision cache unavailable")
	}
	key = strings.TrimSpace(key)
	if key == "" || !cacheableLatestTurnDecision(decision) {
		return nil
	}
	if ttl <= 0 || ttl > LatestTurnDecisionCacheTTL {
		ttl = LatestTurnDecisionCacheTTL
	}
	payload, err := json.Marshal(decision)
	if err != nil {
		return err
	}
	return s.client.Set(ctx, key, payload, ttl).Err()
}
