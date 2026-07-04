package repository

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/redis/go-redis/v9"
)

const responsesImageStatusKeyPrefix = "gen_img:"

type responsesImageStatusCache struct {
	rdb *redis.Client
}

func NewResponsesImageStatusStore(rdb *redis.Client) service.ResponsesImageStatusStore {
	return &responsesImageStatusCache{rdb: rdb}
}

func ResponsesImageStatusKey(requestID string) string {
	return responsesImageStatusKeyPrefix + strings.TrimSpace(requestID)
}

func (c *responsesImageStatusCache) GetResponsesImageStatus(ctx context.Context, requestID string) (*service.ResponsesImageStatus, error) {
	requestID = strings.TrimSpace(requestID)
	if requestID == "" {
		return nil, service.ErrResponsesImageStatusNotFound
	}
	payload, err := c.rdb.Get(ctx, ResponsesImageStatusKey(requestID)).Bytes()
	if errors.Is(err, redis.Nil) {
		return nil, service.ErrResponsesImageStatusNotFound
	}
	if err != nil {
		return nil, err
	}
	var status service.ResponsesImageStatus
	if err := json.Unmarshal(payload, &status); err != nil {
		return nil, err
	}
	if strings.TrimSpace(status.RequestID) == "" {
		status.RequestID = requestID
	}
	return &status, nil
}

func (c *responsesImageStatusCache) SetResponsesImageStatus(ctx context.Context, status *service.ResponsesImageStatus, ttl time.Duration) error {
	if status == nil || strings.TrimSpace(status.RequestID) == "" {
		return nil
	}
	payload, err := json.Marshal(status)
	if err != nil {
		return err
	}
	return c.rdb.Set(ctx, ResponsesImageStatusKey(status.RequestID), payload, ttl).Err()
}
