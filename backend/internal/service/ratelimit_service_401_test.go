//go:build unit

package service

import (
	"context"
	"errors"
	"net/http"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/stretchr/testify/require"
)

type rateLimitAccountRepoStub struct {
	mockAccountRepoForGemini
	tempCalls     int
	tempUntil     time.Time
	tempReason    string
	setErrorCalls int
REDACTED

func (r *rateLimitAccountRepoStub) SetTempUnschedulable(ctx context.Context, id int64, until time.Time, reason string) error {
	r.tempCalls++
	r.tempUntil = until
	r.tempReason = reason
	return nil
REDACTED

func (r *rateLimitAccountRepoStub) SetError(ctx context.Context, id int64, errorMsg string) error {
	r.setErrorCalls++
	return nil
REDACTED

type tokenCacheInvalidatorRecorder struct {
	accounts []*Account
	err      error
REDACTED

func (r *tokenCacheInvalidatorRecorder) InvalidateToken(ctx context.Context, account *Account) error {
	r.accounts = append(r.accounts, account)
	return r.err
REDACTED

func TestRateLimitService_HandleUpstreamError_OAuth401TempUnschedulable(t *testing.T) {
	tests := []struct {
		name     string
		platform string
REDACTED{
		{name: "gemini", platform: PlatformGeminiREDACTED,
		{name: "antigravity", platform: PlatformAntigravityREDACTED,
REDACTED

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repo := &rateLimitAccountRepoStub{REDACTED
			invalidator := &tokenCacheInvalidatorRecorder{REDACTED
			service := NewRateLimitService(repo, nil, &config.Config{REDACTED, nil, nil)
			service.SetTokenCacheInvalidator(invalidator)
			account := &Account{
				ID:       100,
				Platform: tt.platform,
				Type:     AccountTypeOAuth,
		REDACTED

			start := time.Now()
			shouldDisable := service.HandleUpstreamError(context.Background(), account, 401, http.Header{REDACTED, []byte("unauthorized"))

			require.True(t, shouldDisable)
			require.Equal(t, 1, repo.tempCalls)
			require.Equal(t, 0, repo.setErrorCalls)
			require.Len(t, invalidator.accounts, 1)
			require.WithinDuration(t, start.Add(5*time.Minute), repo.tempUntil, 10*time.Second)
			require.NotEmpty(t, repo.tempReason)
	REDACTED)
REDACTED
REDACTED

func TestRateLimitService_HandleUpstreamError_OAuth401InvalidatorError(t *testing.T) {
	repo := &rateLimitAccountRepoStub{REDACTED
	invalidator := &tokenCacheInvalidatorRecorder{err: errors.New("boom")REDACTED
	service := NewRateLimitService(repo, nil, &config.Config{REDACTED, nil, nil)
	service.SetTokenCacheInvalidator(invalidator)
	account := &Account{
		ID:       101,
REDACTED
		Type:     AccountTypeOAuth,
REDACTED

	shouldDisable := service.HandleUpstreamError(context.Background(), account, 401, http.Header{REDACTED, []byte("unauthorized"))

	require.True(t, shouldDisable)
	require.Equal(t, 1, repo.tempCalls)
	require.Equal(t, 0, repo.setErrorCalls)
	require.Len(t, invalidator.accounts, 1)
REDACTED

func TestRateLimitService_HandleUpstreamError_OAuth401CustomRule(t *testing.T) {
	repo := &rateLimitAccountRepoStub{REDACTED
	invalidator := &tokenCacheInvalidatorRecorder{REDACTED
	service := NewRateLimitService(repo, nil, &config.Config{REDACTED, nil, nil)
	service.SetTokenCacheInvalidator(invalidator)
	account := &Account{
		ID:       103,
REDACTED
		Type:     AccountTypeOAuth,
REDACTED
			"temp_unschedulable_enabled": true,
			"temp_unschedulable_rules": []any{
				map[string]any{
					"error_code":       401,
					"keywords":         []any{"unauthorized"REDACTED,
					"duration_minutes": 30,
					"description":      "custom rule",
			REDACTED,
		REDACTED,
	REDACTED,
REDACTED

	start := time.Now()
	shouldDisable := service.HandleUpstreamError(context.Background(), account, 401, http.Header{REDACTED, []byte("unauthorized"))

	require.True(t, shouldDisable)
	require.Equal(t, 1, repo.tempCalls)
	require.Equal(t, 0, repo.setErrorCalls)
	require.Len(t, invalidator.accounts, 1)
	require.WithinDuration(t, start.Add(30*time.Minute), repo.tempUntil, 10*time.Second)
REDACTED

func TestRateLimitService_HandleUpstreamError_NonOAuth401(t *testing.T) {
	repo := &rateLimitAccountRepoStub{REDACTED
	invalidator := &tokenCacheInvalidatorRecorder{REDACTED
	service := NewRateLimitService(repo, nil, &config.Config{REDACTED, nil, nil)
	service.SetTokenCacheInvalidator(invalidator)
	account := &Account{
		ID:       102,
		Platform: PlatformOpenAI,
		Type:     AccountTypeAPIKey,
REDACTED

	shouldDisable := service.HandleUpstreamError(context.Background(), account, 401, http.Header{REDACTED, []byte("unauthorized"))

	require.True(t, shouldDisable)
	require.Equal(t, 0, repo.tempCalls)
	require.Equal(t, 1, repo.setErrorCalls)
	require.Empty(t, invalidator.accounts)
REDACTED

// TestRateLimitService_HandleOAuth401_NilAccount 测试 account 为 nil 的情况
func TestRateLimitService_HandleOAuth401_NilAccount(t *testing.T) {
	repo := &rateLimitAccountRepoStub{REDACTED
	service := NewRateLimitService(repo, nil, &config.Config{REDACTED, nil, nil)

	result := service.handleOAuth401TempUnschedulable(context.Background(), nil, "error")

	require.False(t, result)
	require.Equal(t, 0, repo.tempCalls)
REDACTED

// TestRateLimitService_HandleOAuth401_NilInvalidator 测试 tokenCacheInvalidator 为 nil 的情况
func TestRateLimitService_HandleOAuth401_NilInvalidator(t *testing.T) {
	repo := &rateLimitAccountRepoStub{REDACTED
	service := NewRateLimitService(repo, nil, &config.Config{REDACTED, nil, nil)
	// 不设置 tokenCacheInvalidator
	account := &Account{
		ID:       200,
REDACTED
		Type:     AccountTypeOAuth,
REDACTED

	result := service.handleOAuth401TempUnschedulable(context.Background(), account, "error")

	require.True(t, result)
	require.Equal(t, 1, repo.tempCalls)
REDACTED

// TestRateLimitService_HandleOAuth401_SetTempUnschedulableFailed 测试 SetTempUnschedulable 失败的情况
func TestRateLimitService_HandleOAuth401_SetTempUnschedulableFailed(t *testing.T) {
	repo := &rateLimitAccountRepoStubWithError{
		setTempErr: errors.New("db error"),
REDACTED
	invalidator := &tokenCacheInvalidatorRecorder{REDACTED
	service := NewRateLimitService(repo, nil, &config.Config{REDACTED, nil, nil)
	service.SetTokenCacheInvalidator(invalidator)
	account := &Account{
		ID:       201,
REDACTED
		Type:     AccountTypeOAuth,
REDACTED

	result := service.handleOAuth401TempUnschedulable(context.Background(), account, "error")

	require.False(t, result) // 失败应返回 false
	require.Len(t, invalidator.accounts, 1) // 但 invalidator 仍然被调用
REDACTED

// rateLimitAccountRepoStubWithError 支持返回错误的 stub
type rateLimitAccountRepoStubWithError struct {
	mockAccountRepoForGemini
	setTempErr    error
	setErrorCalls int
REDACTED

func (r *rateLimitAccountRepoStubWithError) SetTempUnschedulable(ctx context.Context, id int64, until time.Time, reason string) error {
	return r.setTempErr
REDACTED

func (r *rateLimitAccountRepoStubWithError) SetError(ctx context.Context, id int64, errorMsg string) error {
	r.setErrorCalls++
	return nil
REDACTED

// TestRateLimitService_HandleOAuth401_WithTempUnschedCache 测试 tempUnschedCache 存在的情况
func TestRateLimitService_HandleOAuth401_WithTempUnschedCache(t *testing.T) {
	repo := &rateLimitAccountRepoStub{REDACTED
	invalidator := &tokenCacheInvalidatorRecorder{REDACTED
	tempCache := &tempUnschedCacheStub{REDACTED
	service := NewRateLimitService(repo, nil, &config.Config{REDACTED, nil, tempCache)
	service.SetTokenCacheInvalidator(invalidator)
	account := &Account{
		ID:       202,
REDACTED
		Type:     AccountTypeOAuth,
REDACTED

	result := service.handleOAuth401TempUnschedulable(context.Background(), account, "error")

	require.True(t, result)
	require.Equal(t, 1, repo.tempCalls)
	require.Equal(t, 1, tempCache.setCalls)
REDACTED

// TestRateLimitService_HandleOAuth401_TempUnschedCacheError 测试 tempUnschedCache 设置失败的情况
func TestRateLimitService_HandleOAuth401_TempUnschedCacheError(t *testing.T) {
	repo := &rateLimitAccountRepoStub{REDACTED
	invalidator := &tokenCacheInvalidatorRecorder{REDACTED
	tempCache := &tempUnschedCacheStub{setErr: errors.New("cache error")REDACTED
	service := NewRateLimitService(repo, nil, &config.Config{REDACTED, nil, tempCache)
	service.SetTokenCacheInvalidator(invalidator)
	account := &Account{
		ID:       203,
REDACTED
		Type:     AccountTypeOAuth,
REDACTED

	result := service.handleOAuth401TempUnschedulable(context.Background(), account, "error")

	require.True(t, result) // 缓存错误不影响主流程
	require.Equal(t, 1, repo.tempCalls)
REDACTED

// tempUnschedCacheStub 用于测试的 TempUnschedCache stub
type tempUnschedCacheStub struct {
	setCalls int
	setErr   error
REDACTED

func (c *tempUnschedCacheStub) GetTempUnsched(ctx context.Context, accountID int64) (*TempUnschedState, error) {
	return nil, nil
REDACTED

func (c *tempUnschedCacheStub) SetTempUnsched(ctx context.Context, accountID int64, state *TempUnschedState) error {
	c.setCalls++
	return c.setErr
REDACTED

func (c *tempUnschedCacheStub) DeleteTempUnsched(ctx context.Context, accountID int64) error {
	return nil
REDACTED

// TestRateLimitService_OAuth401Cooldown 测试 oauth401Cooldown 函数
func TestRateLimitService_OAuth401Cooldown(t *testing.T) {
	tests := []struct {
		name     string
		cfg      *config.Config
		expected time.Duration
REDACTED{
		{
			name:     "default_when_config_zero",
			cfg:      &config.Config{RateLimit: config.RateLimitConfig{OAuth401CooldownMinutes: 0REDACTEDREDACTED,
			expected: 5 * time.Minute,
	REDACTED,
		{
			name:     "custom_cooldown_10_minutes",
			cfg:      &config.Config{RateLimit: config.RateLimitConfig{OAuth401CooldownMinutes: 10REDACTEDREDACTED,
			expected: 10 * time.Minute,
	REDACTED,
		{
			name:     "custom_cooldown_1_minute",
			cfg:      &config.Config{RateLimit: config.RateLimitConfig{OAuth401CooldownMinutes: 1REDACTEDREDACTED,
			expected: 1 * time.Minute,
	REDACTED,
		{
			name:     "negative_value_uses_default",
			cfg:      &config.Config{RateLimit: config.RateLimitConfig{OAuth401CooldownMinutes: -5REDACTEDREDACTED,
			expected: 5 * time.Minute,
	REDACTED,
REDACTED

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			service := NewRateLimitService(nil, nil, tt.cfg, nil, nil)
			result := service.oauth401Cooldown()
			require.Equal(t, tt.expected, result)
	REDACTED)
REDACTED
REDACTED

// TestRateLimitService_OAuth401Cooldown_NilConfig 测试 cfg 为 nil 的情况
func TestRateLimitService_OAuth401Cooldown_NilConfig(t *testing.T) {
	service := &RateLimitService{cfg: nilREDACTED
	result := service.oauth401Cooldown()
	require.Equal(t, 5*time.Minute, result)
REDACTED

// TestRateLimitService_HandleOAuth401_WithCustomCooldown 测试自定义 cooldown 配置
func TestRateLimitService_HandleOAuth401_WithCustomCooldown(t *testing.T) {
	repo := &rateLimitAccountRepoStub{REDACTED
	cfg := &config.Config{
		RateLimit: config.RateLimitConfig{
			OAuth401CooldownMinutes: 15,
	REDACTED,
REDACTED
	service := NewRateLimitService(repo, nil, cfg, nil, nil)
	account := &Account{
		ID:       204,
		Platform: PlatformAntigravity,
		Type:     AccountTypeOAuth,
REDACTED

	start := time.Now()
	result := service.handleOAuth401TempUnschedulable(context.Background(), account, "error")

	require.True(t, result)
	require.WithinDuration(t, start.Add(15*time.Minute), repo.tempUntil, 10*time.Second)
REDACTED

// TestRateLimitService_HandleOAuth401_EmptyUpstreamMsg 测试 upstreamMsg 为空的情况
func TestRateLimitService_HandleOAuth401_EmptyUpstreamMsg(t *testing.T) {
	repo := &rateLimitAccountRepoStub{REDACTED
	service := NewRateLimitService(repo, nil, &config.Config{REDACTED, nil, nil)
	account := &Account{
		ID:       205,
REDACTED
		Type:     AccountTypeOAuth,
REDACTED

	result := service.handleOAuth401TempUnschedulable(context.Background(), account, "")

	require.True(t, result)
	require.Contains(t, repo.tempReason, "Authentication failed (401)")
REDACTED
