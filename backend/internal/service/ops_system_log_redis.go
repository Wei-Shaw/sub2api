package service

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	"github.com/redis/go-redis/v9"
)

const (
	redisSystemLogKey   = "ops:system_logs:recent:v1"
	redisSystemLogIDKey = "ops:system_logs:recent:id:v1"
	redisSystemLogLimit = 500
)

func (s *OpsSystemLogSink) writeRedisSystemLogs(ctx context.Context, inputs []*OpsInsertSystemLogInput) (int, error) {
	if s == nil || s.redisClient == nil {
		return 0, fmt.Errorf("redis system log store is not configured")
	}
	validInputs := make([]*OpsInsertSystemLogInput, 0, len(inputs))
	for _, input := range inputs {
		if input != nil {
			validInputs = append(validInputs, input)
		}
	}
	if len(validInputs) == 0 {
		return 0, nil
	}
	lastID, err := s.redisClient.IncrBy(ctx, redisSystemLogIDKey, int64(len(validInputs))).Result()
	if err != nil {
		return 0, err
	}
	firstID := lastID - int64(len(validInputs)) + 1
	values := make([]any, 0, len(validInputs))
	for index, input := range validInputs {
		item := &OpsSystemLog{
			ID:              firstID + int64(index),
			CreatedAt:       input.CreatedAt.UTC(),
			Host:            input.Host,
			Level:           input.Level,
			Component:       input.Component,
			Message:         input.Message,
			RequestID:       input.RequestID,
			ClientRequestID: input.ClientRequestID,
			UserID:          input.UserID,
			APIKeyID:        input.APIKeyID,
			AccountID:       input.AccountID,
			Platform:        input.Platform,
			Model:           input.Model,
		}
		if extra := strings.TrimSpace(input.ExtraJSON); extra != "" && extra != "{}" && extra != "null" {
			_ = json.Unmarshal([]byte(extra), &item.Extra)
		}
		raw, err := json.Marshal(item)
		if err != nil {
			return 0, err
		}
		values = append(values, raw)
	}
	_, err = s.redisClient.TxPipelined(ctx, func(pipe redis.Pipeliner) error {
		pipe.LPush(ctx, redisSystemLogKey, values...)
		pipe.LTrim(ctx, redisSystemLogKey, 0, redisSystemLogLimit-1)
		return nil
	})
	if err != nil {
		return 0, err
	}
	return len(values), nil
}

func (s *OpsSystemLogSink) ListRedisSystemLogs(ctx context.Context, filter *OpsSystemLogFilter) (*OpsSystemLogList, error) {
	if s == nil || s.redisClient == nil {
		return nil, fmt.Errorf("redis system log store is not configured")
	}
	if filter == nil {
		filter = &OpsSystemLogFilter{}
	}
	rawItems, err := s.redisClient.LRange(ctx, redisSystemLogKey, 0, redisSystemLogLimit-1).Result()
	if err != nil {
		return nil, err
	}

	matched := make([]*OpsSystemLog, 0, len(rawItems))
	for _, raw := range rawItems {
		item := &OpsSystemLog{}
		if err := json.Unmarshal([]byte(raw), item); err != nil {
			continue
		}
		if matchesOpsSystemLog(item, filter) {
			matched = append(matched, item)
		}
	}
	sort.SliceStable(matched, func(i, j int) bool {
		if matched[i].CreatedAt.Equal(matched[j].CreatedAt) {
			return matched[i].ID > matched[j].ID
		}
		return matched[i].CreatedAt.After(matched[j].CreatedAt)
	})

	page := filter.Page
	if page <= 0 {
		page = 1
	}
	pageSize := filter.PageSize
	if pageSize <= 0 {
		pageSize = 50
	}
	if pageSize > 200 {
		pageSize = 200
	}
	start := (page - 1) * pageSize
	if start > len(matched) {
		start = len(matched)
	}
	end := start + pageSize
	if end > len(matched) {
		end = len(matched)
	}
	return &OpsSystemLogList{
		Logs:     matched[start:end],
		Total:    len(matched),
		Page:     page,
		PageSize: pageSize,
	}, nil
}

func (s *OpsSystemLogSink) DeleteRedisSystemLogs(ctx context.Context, filter *OpsSystemLogCleanupFilter) (int64, error) {
	if s == nil || s.redisClient == nil {
		return 0, fmt.Errorf("redis system log store is not configured")
	}
	hasConstraint := hasOpsSystemLogCleanupConstraint(filter)
	if !hasConstraint && (filter == nil || !filter.ClearAll) {
		return 0, fmt.Errorf("cleanup requires at least one filter condition")
	}
	if hasConstraint && filter.ClearAll {
		return 0, fmt.Errorf("clear_all cannot be combined with filter conditions")
	}
	rawItems, err := s.redisClient.LRange(ctx, redisSystemLogKey, 0, redisSystemLogLimit-1).Result()
	if err != nil {
		return 0, err
	}
	listFilter := opsSystemLogCleanupListFilter(filter)
	toDelete := make([]string, 0)
	for _, raw := range rawItems {
		item := &OpsSystemLog{}
		if err := json.Unmarshal([]byte(raw), item); err != nil {
			continue
		}
		if matchesOpsSystemLog(item, listFilter) {
			toDelete = append(toDelete, raw)
		}
	}
	if len(toDelete) == 0 {
		return 0, nil
	}

	commands := make([]*redis.IntCmd, 0, len(toDelete))
	_, err = s.redisClient.TxPipelined(ctx, func(pipe redis.Pipeliner) error {
		for _, raw := range toDelete {
			commands = append(commands, pipe.LRem(ctx, redisSystemLogKey, 1, raw))
		}
		return nil
	})
	if err != nil {
		return 0, err
	}
	var deleted int64
	for _, command := range commands {
		deleted += command.Val()
	}
	return deleted, nil
}

func matchesOpsSystemLog(item *OpsSystemLog, filter *OpsSystemLogFilter) bool {
	if item == nil || filter == nil {
		return item != nil
	}
	if filter.StartTime != nil && !filter.StartTime.IsZero() && item.CreatedAt.Before(filter.StartTime.UTC()) {
		return false
	}
	if filter.EndTime != nil && !filter.EndTime.IsZero() && !item.CreatedAt.Before(filter.EndTime.UTC()) {
		return false
	}
	if v := strings.TrimSpace(filter.Host); v != "" && item.Host != v {
		return false
	}
	if v := strings.ToLower(strings.TrimSpace(filter.Level)); v != "" && strings.ToLower(item.Level) != v {
		return false
	}
	if v := strings.TrimSpace(filter.Component); v != "" && item.Component != v {
		return false
	}
	if v := strings.TrimSpace(filter.RequestID); v != "" && item.RequestID != v {
		return false
	}
	if v := strings.TrimSpace(filter.ClientRequestID); v != "" && item.ClientRequestID != v {
		return false
	}
	if filter.UserID != nil && *filter.UserID > 0 && (item.UserID == nil || *item.UserID != *filter.UserID) {
		return false
	}
	if filter.APIKeyID != nil && *filter.APIKeyID > 0 && (item.APIKeyID == nil || *item.APIKeyID != *filter.APIKeyID) {
		return false
	}
	if filter.AccountID != nil && *filter.AccountID > 0 && (item.AccountID == nil || *item.AccountID != *filter.AccountID) {
		return false
	}
	if v := strings.TrimSpace(filter.Platform); v != "" && item.Platform != v {
		return false
	}
	if v := strings.TrimSpace(filter.Model); v != "" && item.Model != v {
		return false
	}
	if query := strings.ToLower(strings.TrimSpace(filter.Query)); query != "" {
		extra, _ := json.Marshal(item.Extra)
		haystack := strings.ToLower(strings.Join([]string{
			item.Message,
			item.RequestID,
			item.ClientRequestID,
			string(extra),
		}, "\n"))
		if !strings.Contains(haystack, query) {
			return false
		}
	}
	return true
}

func opsSystemLogCleanupListFilter(filter *OpsSystemLogCleanupFilter) *OpsSystemLogFilter {
	if filter == nil {
		return &OpsSystemLogFilter{}
	}
	return &OpsSystemLogFilter{
		StartTime:       filter.StartTime,
		EndTime:         filter.EndTime,
		Host:            filter.Host,
		Level:           filter.Level,
		Component:       filter.Component,
		RequestID:       filter.RequestID,
		ClientRequestID: filter.ClientRequestID,
		UserID:          filter.UserID,
		APIKeyID:        filter.APIKeyID,
		AccountID:       filter.AccountID,
		Platform:        filter.Platform,
		Model:           filter.Model,
		Query:           filter.Query,
	}
}

func hasOpsSystemLogCleanupConstraint(filter *OpsSystemLogCleanupFilter) bool {
	if filter == nil {
		return false
	}
	if (filter.StartTime != nil && !filter.StartTime.IsZero()) || (filter.EndTime != nil && !filter.EndTime.IsZero()) {
		return true
	}
	if filter.UserID != nil && *filter.UserID > 0 {
		return true
	}
	if filter.APIKeyID != nil && *filter.APIKeyID > 0 {
		return true
	}
	if filter.AccountID != nil && *filter.AccountID > 0 {
		return true
	}
	return strings.TrimSpace(filter.Host) != "" ||
		strings.TrimSpace(filter.Level) != "" ||
		strings.TrimSpace(filter.Component) != "" ||
		strings.TrimSpace(filter.RequestID) != "" ||
		strings.TrimSpace(filter.ClientRequestID) != "" ||
		strings.TrimSpace(filter.Platform) != "" ||
		strings.TrimSpace(filter.Model) != "" ||
		strings.TrimSpace(filter.Query) != ""
}
