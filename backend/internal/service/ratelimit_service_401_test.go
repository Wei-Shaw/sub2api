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
	setErrorCalls int
	tempCalls     int
	lastErrorMsg  string
REDACTED

func (r *rateLimitAccountRepoStub) SetError(ctx context.Context, id int64, errorMsg string) error {
	r.setErrorCalls++
	r.lastErrorMsg = errorMsg
	return nil
REDACTED

func (r *rateLimitAccountRepoStub) SetTempUnschedulable(ctx context.Context, id int64, until time.Time, reason string) error {
	r.tempCalls++
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

func TestRateLimitService_HandleUpstreamError_OAuth401SetsTempUnschedulable(t *testing.T) {
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
	require.Equal(t, 0, repo.setErrorCalls)
	require.Equal(t, 1, repo.tempCalls)
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
