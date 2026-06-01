//go:build !unit

package service

import (
	"context"
	"time"
)

type defaultRateLimitCall struct {
	accountID int64
	resetAt   time.Time
REDACTED

type defaultModelRateLimitCall struct {
	accountID int64
	modelKey  string
	resetAt   time.Time
REDACTED

type defaultExtraUpdateCall struct {
	accountID int64
	updates   map[string]any
REDACTED

type stubAntigravityAccountRepo struct {
	AccountRepository
	rateCalls           []defaultRateLimitCall
	modelRateLimitCalls []defaultModelRateLimitCall
	extraUpdateCalls    []defaultExtraUpdateCall
REDACTED

func (s *stubAntigravityAccountRepo) SetRateLimited(_ context.Context, id int64, resetAt time.Time) error {
	s.rateCalls = append(s.rateCalls, defaultRateLimitCall{accountID: id, resetAt: resetAtREDACTED)
	return nil
REDACTED

func (s *stubAntigravityAccountRepo) SetModelRateLimit(_ context.Context, id int64, modelKey string, resetAt time.Time, _ ...string) error {
	s.modelRateLimitCalls = append(s.modelRateLimitCalls, defaultModelRateLimitCall{accountID: id, modelKey: modelKey, resetAt: resetAtREDACTED)
	return nil
REDACTED

func (s *stubAntigravityAccountRepo) UpdateExtra(_ context.Context, id int64, updates map[string]any) error {
	s.extraUpdateCalls = append(s.extraUpdateCalls, defaultExtraUpdateCall{accountID: id, updates: updatesREDACTED)
	return nil
REDACTED

type defaultDeleteSessionCall struct {
	groupID     int64
	sessionHash string
REDACTED

type stubSmartRetryCache struct {
	GatewayCache
	deleteCalls []defaultDeleteSessionCall
REDACTED

func (c *stubSmartRetryCache) DeleteSessionAccountID(_ context.Context, groupID int64, sessionHash string) error {
	c.deleteCalls = append(c.deleteCalls, defaultDeleteSessionCall{groupID: groupID, sessionHash: sessionHashREDACTED)
	return nil
REDACTED
