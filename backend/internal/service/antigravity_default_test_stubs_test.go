//go:build !unit

package service

import (
	"context"
	"time"
)

type defaultRateLimitCall struct {
	accountID int64
	resetAt   time.Time
}

type defaultModelRateLimitCall struct {
	accountID int64
	modelKey  string
	resetAt   time.Time
}

type defaultExtraUpdateCall struct {
	accountID int64
	updates   map[string]any
}

type stubAntigravityAccountRepo struct {
	AccountRepository
	rateCalls           []defaultRateLimitCall
	modelRateLimitCalls []defaultModelRateLimitCall
	extraUpdateCalls    []defaultExtraUpdateCall
}

func (s *stubAntigravityAccountRepo) SetRateLimited(_ context.Context, id int64, resetAt time.Time) error {
	s.rateCalls = append(s.rateCalls, defaultRateLimitCall{accountID: id, resetAt: resetAt})
	return nil
}

func (s *stubAntigravityAccountRepo) SetModelRateLimit(_ context.Context, id int64, modelKey string, resetAt time.Time, _ ...string) error {
	s.modelRateLimitCalls = append(s.modelRateLimitCalls, defaultModelRateLimitCall{accountID: id, modelKey: modelKey, resetAt: resetAt})
	return nil
}

func (s *stubAntigravityAccountRepo) UpdateExtra(_ context.Context, id int64, updates map[string]any) error {
	s.extraUpdateCalls = append(s.extraUpdateCalls, defaultExtraUpdateCall{accountID: id, updates: updates})
	return nil
}

type defaultDeleteSessionCall struct {
	groupID     int64
	sessionHash string
}

type stubSmartRetryCache struct {
	GatewayCache
	deleteCalls []defaultDeleteSessionCall
}

func (c *stubSmartRetryCache) DeleteSessionAccountID(_ context.Context, groupID int64, sessionHash string) error {
	c.deleteCalls = append(c.deleteCalls, defaultDeleteSessionCall{groupID: groupID, sessionHash: sessionHash})
	return nil
}
