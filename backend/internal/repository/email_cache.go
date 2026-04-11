package repository

import (
	"context"
	"encoding/json"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/redis/go-redis/v9"
)

const (
	verifyCodeKeyPrefix          = "verify_code:"
	notifyVerifyKeyPrefix        = "notify_verify:"
	passwordResetKeyPrefix       = "password_reset:"
	passwordResetSentAtKeyPrefix = "password_reset_sent:"
)

// verifyCodeKey generates the Redis key for email verification code.
func verifyCodeKey(email string) string {
	return verifyCodeKeyPrefix + email
REDACTED

// notifyVerifyKey generates the Redis key for notify email verification code.
func notifyVerifyKey(email string) string {
	return notifyVerifyKeyPrefix + email
REDACTED

// passwordResetKey generates the Redis key for password reset token.
func passwordResetKey(email string) string {
	return passwordResetKeyPrefix + email
REDACTED

// passwordResetSentAtKey generates the Redis key for password reset email sent timestamp.
func passwordResetSentAtKey(email string) string {
	return passwordResetSentAtKeyPrefix + email
REDACTED

type emailCache struct {
	rdb *redis.Client
REDACTED

func NewEmailCache(rdb *redis.Client) service.EmailCache {
	return &emailCache{rdb: rdbREDACTED
REDACTED

func (c *emailCache) GetVerificationCode(ctx context.Context, email string) (*service.VerificationCodeData, error) {
	key := verifyCodeKey(email)
	val, err := c.rdb.Get(ctx, key).Result()
	if err != nil {
		return nil, err
REDACTED
	var data service.VerificationCodeData
	if err := json.Unmarshal([]byte(val), &data); err != nil {
		return nil, err
REDACTED
	return &data, nil
REDACTED

func (c *emailCache) SetVerificationCode(ctx context.Context, email string, data *service.VerificationCodeData, ttl time.Duration) error {
	key := verifyCodeKey(email)
	val, err := json.Marshal(data)
	if err != nil {
		return err
REDACTED
	return c.rdb.Set(ctx, key, val, ttl).Err()
REDACTED

func (c *emailCache) DeleteVerificationCode(ctx context.Context, email string) error {
	key := verifyCodeKey(email)
	return c.rdb.Del(ctx, key).Err()
REDACTED

// Password reset token methods

func (c *emailCache) GetPasswordResetToken(ctx context.Context, email string) (*service.PasswordResetTokenData, error) {
	key := passwordResetKey(email)
	val, err := c.rdb.Get(ctx, key).Result()
	if err != nil {
		return nil, err
REDACTED
	var data service.PasswordResetTokenData
	if err := json.Unmarshal([]byte(val), &data); err != nil {
		return nil, err
REDACTED
	return &data, nil
REDACTED

func (c *emailCache) SetPasswordResetToken(ctx context.Context, email string, data *service.PasswordResetTokenData, ttl time.Duration) error {
	key := passwordResetKey(email)
	val, err := json.Marshal(data)
	if err != nil {
		return err
REDACTED
	return c.rdb.Set(ctx, key, val, ttl).Err()
REDACTED

func (c *emailCache) DeletePasswordResetToken(ctx context.Context, email string) error {
	key := passwordResetKey(email)
	return c.rdb.Del(ctx, key).Err()
REDACTED

// Password reset email cooldown methods

func (c *emailCache) IsPasswordResetEmailInCooldown(ctx context.Context, email string) bool {
	key := passwordResetSentAtKey(email)
	exists, err := c.rdb.Exists(ctx, key).Result()
	return err == nil && exists > 0
REDACTED

func (c *emailCache) SetPasswordResetEmailCooldown(ctx context.Context, email string, ttl time.Duration) error {
	key := passwordResetSentAtKey(email)
	return c.rdb.Set(ctx, key, "1", ttl).Err()
REDACTED

// Notify email verification code methods

func (c *emailCache) GetNotifyVerifyCode(ctx context.Context, email string) (*service.VerificationCodeData, error) {
	key := notifyVerifyKey(email)
	val, err := c.rdb.Get(ctx, key).Result()
	if err != nil {
		return nil, err
REDACTED
	var data service.VerificationCodeData
	if err := json.Unmarshal([]byte(val), &data); err != nil {
		return nil, err
REDACTED
	return &data, nil
REDACTED

func (c *emailCache) SetNotifyVerifyCode(ctx context.Context, email string, data *service.VerificationCodeData, ttl time.Duration) error {
	key := notifyVerifyKey(email)
	val, err := json.Marshal(data)
	if err != nil {
		return err
REDACTED
	return c.rdb.Set(ctx, key, val, ttl).Err()
REDACTED

func (c *emailCache) DeleteNotifyVerifyCode(ctx context.Context, email string) error {
	key := notifyVerifyKey(email)
	return c.rdb.Del(ctx, key).Err()
REDACTED
