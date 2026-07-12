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
	statuses, err := c.GetResponsesImageStatuses(ctx, []string{requestID})
	if err != nil {
		return nil, err
	}
	if status := statuses[requestID]; status != nil {
		return status, nil
	}
	return nil, service.ErrResponsesImageStatusNotFound
}

func (c *responsesImageStatusCache) GetResponsesImageStatuses(ctx context.Context, requestIDs []string) (map[string]*service.ResponsesImageStatus, error) {
	normalized := make([]string, 0, len(requestIDs))
	keys := make([]string, 0, len(requestIDs))
	seen := make(map[string]struct{}, len(requestIDs))
	for _, requestID := range requestIDs {
		requestID = strings.TrimSpace(requestID)
		if requestID == "" {
			continue
		}
		if _, ok := seen[requestID]; ok {
			continue
		}
		seen[requestID] = struct{}{}
		normalized = append(normalized, requestID)
		keys = append(keys, ResponsesImageStatusKey(requestID))
	}
	if len(keys) == 0 {
		return nil, service.ErrResponsesImageStatusNotFound
	}
	values, err := c.rdb.MGet(ctx, keys...).Result()
	if errors.Is(err, redis.Nil) {
		return nil, service.ErrResponsesImageStatusNotFound
	}
	if err != nil {
		return nil, err
	}
	statuses := make(map[string]*service.ResponsesImageStatus, len(values))
	for i, value := range values {
		if value == nil {
			continue
		}
		var payload []byte
		switch v := value.(type) {
		case string:
			payload = []byte(v)
		case []byte:
			payload = v
		default:
			continue
		}
		var status service.ResponsesImageStatus
		if err := json.Unmarshal(payload, &status); err != nil {
			return nil, err
		}
		requestID := normalized[i]
		if strings.TrimSpace(status.RequestID) == "" {
			status.RequestID = requestID
		}
		statuses[requestID] = &status
	}
	return statuses, nil
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
