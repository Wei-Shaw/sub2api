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
	setErrorCalls          int
	tempCalls              int
	updateCredentialsCalls int
	lastCredentials        map[string]any
	lastErrorMsg           string
	lastTempReason         string
REDACTED

func (r *rateLimitAccountRepoStub) SetError(ctx context.Context, id int64, errorMsg string) error {
	r.setErrorCalls++
	r.lastErrorMsg = errorMsg
	return nil
REDACTED

func (r *rateLimitAccountRepoStub) SetTempUnschedulable(ctx context.Context, id int64, until time.Time, reason string) error {
	r.tempCalls++
	r.lastTempReason = reason
	return nil
REDACTED

func (r *rateLimitAccountRepoStub) UpdateCredentials(ctx context.Context, id int64, credentials map[string]any) error {
	r.updateCredentialsCalls++
	r.lastCredentials = cloneCredentials(credentials)
	return nil
REDACTED

type tokenCacheInvalidatorRecorder struct {
	accounts []*Account
	err      error
REDACTED

type openAI403CounterCacheStub struct {
	counts     []int64
	resetCalls []int64
	err        error
REDACTED

func (s *openAI403CounterCacheStub) IncrementOpenAI403Count(_ context.Context, _ int64, _ int) (int64, error) {
	if s.err != nil {
		return 0, s.err
REDACTED
	if len(s.counts) == 0 {
		return 1, nil
REDACTED
	count := s.counts[0]
	s.counts = s.counts[1:]
	return count, nil
REDACTED

func (s *openAI403CounterCacheStub) ResetOpenAI403Count(_ context.Context, accountID int64) error {
	s.resetCalls = append(s.resetCalls, accountID)
	return nil
REDACTED

func (r *tokenCacheInvalidatorRecorder) InvalidateToken(ctx context.Context, account *Account) error {
	r.accounts = append(r.accounts, account)
	return r.err
REDACTED

func TestRateLimitService_HandleUpstreamError_OAuth401SetsTempUnschedulable(t *testing.T) {
	t.Run("gemini", func(t *testing.T) {
		repo := &rateLimitAccountRepoStub{REDACTED
		invalidator := &tokenCacheInvalidatorRecorder{REDACTED
		service := NewRateLimitService(repo, nil, &config.Config{REDACTED, nil, nil)
		service.SetTokenCacheInvalidator(invalidator)
		account := &Account{
			ID:       100,
	REDACTED
			Type:     AccountTypeOAuth,
	REDACTED
				"refresh_token":              "rt-100",
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

		shouldDisable := service.HandleUpstreamError(context.Background(), account, 401, http.Header{REDACTED, []byte("unauthorized"))

		require.True(t, shouldDisable)
		require.Equal(t, 0, repo.setErrorCalls)
		require.Equal(t, 1, repo.tempCalls)
		require.Len(t, invalidator.accounts, 1)
REDACTED)

	t.Run("antigravity_401_uses_SetError", func(t *testing.T) {
		// Antigravity 401 由 applyErrorPolicy 的 temp_unschedulable_rules 控制，
		// HandleUpstreamError 中走 SetError 路径。
		repo := &rateLimitAccountRepoStub{REDACTED
		invalidator := &tokenCacheInvalidatorRecorder{REDACTED
		service := NewRateLimitService(repo, nil, &config.Config{REDACTED, nil, nil)
		service.SetTokenCacheInvalidator(invalidator)
		account := &Account{
			ID:       100,
			Platform: PlatformAntigravity,
			Type:     AccountTypeOAuth,
	REDACTED

		shouldDisable := service.HandleUpstreamError(context.Background(), account, 401, http.Header{REDACTED, []byte("unauthorized"))

		require.True(t, shouldDisable)
		require.Equal(t, 1, repo.setErrorCalls)
		require.Equal(t, 0, repo.tempCalls)
		require.Empty(t, invalidator.accounts)
REDACTED)
REDACTED

// TestRateLimitService_HandleUpstreamError_OAuth401InvalidatorError
// OpenAI OAuth 401 缓存失效出错时仍走 temp_unschedulable
func TestRateLimitService_HandleUpstreamError_OAuth401InvalidatorError(t *testing.T) {
	repo := &rateLimitAccountRepoStub{REDACTED
	invalidator := &tokenCacheInvalidatorRecorder{err: errors.New("boom")REDACTED
	service := NewRateLimitService(repo, nil, &config.Config{REDACTED, nil, nil)
	service.SetTokenCacheInvalidator(invalidator)
	account := &Account{
		ID:       101,
		Platform: PlatformOpenAI,
		Type:     AccountTypeOAuth,
REDACTED
			"refresh_token": "rt-101",
	REDACTED,
REDACTED

	shouldDisable := service.HandleUpstreamError(context.Background(), account, 401, http.Header{REDACTED, []byte("unauthorized"))

	require.True(t, shouldDisable)
	require.Equal(t, 0, repo.setErrorCalls)
	require.Equal(t, 1, repo.tempCalls)
	require.Equal(t, 1, repo.updateCredentialsCalls)
	require.Len(t, invalidator.accounts, 1)
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
	require.Equal(t, 1, repo.setErrorCalls)
	require.Empty(t, invalidator.accounts)
REDACTED

func TestRateLimitService_HandleUpstreamError_OAuth401UsesCredentialsUpdater(t *testing.T) {
	repo := &rateLimitAccountRepoStub{REDACTED
	service := NewRateLimitService(repo, nil, &config.Config{REDACTED, nil, nil)
	account := &Account{
		ID:       103,
		Platform: PlatformOpenAI,
		Type:     AccountTypeOAuth,
REDACTED
			"access_token":  "token",
			"refresh_token": "rt-103",
	REDACTED,
REDACTED

	shouldDisable := service.HandleUpstreamError(context.Background(), account, 401, http.Header{REDACTED, []byte("unauthorized"))

	require.True(t, shouldDisable)
	require.Equal(t, 1, repo.updateCredentialsCalls)
	require.NotEmpty(t, repo.lastCredentials["expires_at"])
REDACTED

// 缺少 refresh_token 的 OAuth 账号 401 应直接 SetError 永久禁用，
// 不再走 10 分钟冷却（冷却期内无人能刷新它，结束后还会被选中再 502 一次）。
func TestRateLimitService_HandleUpstreamError_OAuth401NoRefreshTokenSetsError(t *testing.T) {
	t.Run("openai_no_refresh_token", func(t *testing.T) {
		repo := &rateLimitAccountRepoStub{REDACTED
		invalidator := &tokenCacheInvalidatorRecorder{REDACTED
		service := NewRateLimitService(repo, nil, &config.Config{REDACTED, nil, nil)
		service.SetTokenCacheInvalidator(invalidator)
		account := &Account{
			ID:       2881,
			Platform: PlatformOpenAI,
			Type:     AccountTypeOAuth,
	REDACTED
				"access_token": "expired-at",
				// no refresh_token
		REDACTED,
	REDACTED

		shouldDisable := service.HandleUpstreamError(context.Background(), account, 401, http.Header{REDACTED, []byte("unauthorized"))

		require.True(t, shouldDisable)
		require.Equal(t, 1, repo.setErrorCalls, "AT-only OAuth 401 must SetError")
		require.Equal(t, 0, repo.tempCalls, "AT-only OAuth 401 must NOT temp-unschedule")
		require.Equal(t, 0, repo.updateCredentialsCalls, "no point forcing expires_at when refresh is impossible")
		require.Contains(t, repo.lastErrorMsg, "refresh_token missing")
		require.Len(t, invalidator.accounts, 1, "cache should still be invalidated")
REDACTED)

	t.Run("openai_blank_refresh_token_treated_as_missing", func(t *testing.T) {
		repo := &rateLimitAccountRepoStub{REDACTED
		service := NewRateLimitService(repo, nil, &config.Config{REDACTED, nil, nil)
		account := &Account{
			ID:       2882,
			Platform: PlatformOpenAI,
			Type:     AccountTypeOAuth,
	REDACTED
				"access_token":  "expired-at",
				"refresh_token": "   ",
		REDACTED,
	REDACTED

		shouldDisable := service.HandleUpstreamError(context.Background(), account, 401, http.Header{REDACTED, []byte("unauthorized"))

		require.True(t, shouldDisable)
		require.Equal(t, 1, repo.setErrorCalls)
		require.Equal(t, 0, repo.tempCalls)
REDACTED)
REDACTED
