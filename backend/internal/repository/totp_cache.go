package repository

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/redis/go-redis/v9"

	"github.com/Wei-Shaw/sub2api/internal/service"
)

const (
	totpSetupKeyPrefix    = "totp:setup:"
	totpLoginKeyPrefix    = "totp:login:"
	totpAttemptsKeyPrefix = "totp:attempts:"
	totpStepUpKeyPrefix   = "totp:stepup:"
	totpAttemptsTTL       = 15 * time.Minute
)

// TotpCache implements service.TotpCache using Redis
type TotpCache struct {
	rdb *redis.Client
REDACTED

// NewTotpCache creates a new TOTP cache
func NewTotpCache(rdb *redis.Client) service.TotpCache {
	return &TotpCache{rdb: rdbREDACTED
REDACTED

// GetSetupSession retrieves a TOTP setup session
func (c *TotpCache) GetSetupSession(ctx context.Context, userID int64) (*service.TotpSetupSession, error) {
	key := fmt.Sprintf("%s%d", totpSetupKeyPrefix, userID)
	data, err := c.rdb.Get(ctx, key).Bytes()
	if err != nil {
		if err == redis.Nil {
			return nil, nil
	REDACTED
		return nil, fmt.Errorf("get setup session: %w", err)
REDACTED

	var session service.TotpSetupSession
	if err := json.Unmarshal(data, &session); err != nil {
		return nil, fmt.Errorf("unmarshal setup session: %w", err)
REDACTED

	return &session, nil
REDACTED

// SetSetupSession stores a TOTP setup session
func (c *TotpCache) SetSetupSession(ctx context.Context, userID int64, session *service.TotpSetupSession, ttl time.Duration) error {
	key := fmt.Sprintf("%s%d", totpSetupKeyPrefix, userID)
	data, err := json.Marshal(session)
	if err != nil {
		return fmt.Errorf("marshal setup session: %w", err)
REDACTED

	if err := c.rdb.Set(ctx, key, data, ttl).Err(); err != nil {
		return fmt.Errorf("set setup session: %w", err)
REDACTED

	return nil
REDACTED

// DeleteSetupSession deletes a TOTP setup session
func (c *TotpCache) DeleteSetupSession(ctx context.Context, userID int64) error {
	key := fmt.Sprintf("%s%d", totpSetupKeyPrefix, userID)
	return c.rdb.Del(ctx, key).Err()
REDACTED

// GetLoginSession retrieves a TOTP login session
func (c *TotpCache) GetLoginSession(ctx context.Context, tempToken string) (*service.TotpLoginSession, error) {
	key := totpLoginKeyPrefix + tempToken
	data, err := c.rdb.Get(ctx, key).Bytes()
	if err != nil {
		if err == redis.Nil {
			return nil, nil
	REDACTED
		return nil, fmt.Errorf("get login session: %w", err)
REDACTED

	var session service.TotpLoginSession
	if err := json.Unmarshal(data, &session); err != nil {
		return nil, fmt.Errorf("unmarshal login session: %w", err)
REDACTED

	return &session, nil
REDACTED

// SetLoginSession stores a TOTP login session
func (c *TotpCache) SetLoginSession(ctx context.Context, tempToken string, session *service.TotpLoginSession, ttl time.Duration) error {
	key := totpLoginKeyPrefix + tempToken
	data, err := json.Marshal(session)
	if err != nil {
		return fmt.Errorf("marshal login session: %w", err)
REDACTED

	if err := c.rdb.Set(ctx, key, data, ttl).Err(); err != nil {
		return fmt.Errorf("set login session: %w", err)
REDACTED

	return nil
REDACTED

// DeleteLoginSession deletes a TOTP login session
func (c *TotpCache) DeleteLoginSession(ctx context.Context, tempToken string) error {
	key := totpLoginKeyPrefix + tempToken
	return c.rdb.Del(ctx, key).Err()
REDACTED

// IncrementVerifyAttempts increments the verify attempt counter
func (c *TotpCache) IncrementVerifyAttempts(ctx context.Context, userID int64) (int, error) {
	key := fmt.Sprintf("%s%d", totpAttemptsKeyPrefix, userID)

	// Use pipeline for atomic increment and set TTL
	pipe := c.rdb.Pipeline()
	incrCmd := pipe.Incr(ctx, key)
	pipe.Expire(ctx, key, totpAttemptsTTL)

	if _, err := pipe.Exec(ctx); err != nil {
		return 0, fmt.Errorf("increment verify attempts: %w", err)
REDACTED

	count, err := incrCmd.Result()
	if err != nil {
		return 0, fmt.Errorf("get increment result: %w", err)
REDACTED

	return int(count), nil
REDACTED

// GetVerifyAttempts gets the current verify attempt count
func (c *TotpCache) GetVerifyAttempts(ctx context.Context, userID int64) (int, error) {
	key := fmt.Sprintf("%s%d", totpAttemptsKeyPrefix, userID)
	count, err := c.rdb.Get(ctx, key).Int()
	if err != nil {
		if err == redis.Nil {
			return 0, nil
	REDACTED
		return 0, fmt.Errorf("get verify attempts: %w", err)
REDACTED
	return count, nil
REDACTED

// ClearVerifyAttempts clears the verify attempt counter
func (c *TotpCache) ClearVerifyAttempts(ctx context.Context, userID int64) error {
	key := fmt.Sprintf("%s%d", totpAttemptsKeyPrefix, userID)
	return c.rdb.Del(ctx, key).Err()
REDACTED

func totpStepUpKey(userID int64, sessionKey string) string {
	return fmt.Sprintf("%s%d:%s", totpStepUpKeyPrefix, userID, sessionKey)
REDACTED

// SetStepUpGrant 记录一次 step-up 验证通过（sudo 窗口），绑定用户+会话。
func (c *TotpCache) SetStepUpGrant(ctx context.Context, userID int64, sessionKey string, ttl time.Duration) error {
	return c.rdb.Set(ctx, totpStepUpKey(userID, sessionKey), "1", ttl).Err()
REDACTED

// HasStepUpGrant 检查 step-up 授权是否仍在有效期内。
func (c *TotpCache) HasStepUpGrant(ctx context.Context, userID int64, sessionKey string) (bool, error) {
	_, err := c.rdb.Get(ctx, totpStepUpKey(userID, sessionKey)).Result()
	if err != nil {
		if err == redis.Nil {
			return false, nil
	REDACTED
		return false, fmt.Errorf("get step-up grant: %w", err)
REDACTED
	return true, nil
REDACTED
