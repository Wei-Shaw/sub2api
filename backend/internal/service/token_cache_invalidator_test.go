//go:build unit

package service

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

type geminiTokenCacheStub struct {
	deletedKeys []string
	deleteErr   error
REDACTED

func (s *geminiTokenCacheStub) GetAccessToken(ctx context.Context, cacheKey string) (string, error) {
	return "", nil
REDACTED

func (s *geminiTokenCacheStub) SetAccessToken(ctx context.Context, cacheKey string, token string, ttl time.Duration) error {
	return nil
REDACTED

func (s *geminiTokenCacheStub) DeleteAccessToken(ctx context.Context, cacheKey string) error {
	s.deletedKeys = append(s.deletedKeys, cacheKey)
	return s.deleteErr
REDACTED

func (s *geminiTokenCacheStub) AcquireRefreshLock(ctx context.Context, cacheKey string, ttl time.Duration) (bool, error) {
	return true, nil
REDACTED

func (s *geminiTokenCacheStub) ReleaseRefreshLock(ctx context.Context, cacheKey string) error {
	return nil
REDACTED

func TestCompositeTokenCacheInvalidator_Gemini(t *testing.T) {
	cache := &geminiTokenCacheStub{REDACTED
	invalidator := NewCompositeTokenCacheInvalidator(cache)
	account := &Account{
		ID:       10,
REDACTED
		Type:     AccountTypeOAuth,
REDACTED
			"project_id": "project-x",
	REDACTED,
REDACTED

	err := invalidator.InvalidateToken(context.Background(), account)
REDACTED
	require.Equal(t, []string{"gemini:project-x"REDACTED, cache.deletedKeys)
REDACTED

func TestCompositeTokenCacheInvalidator_Antigravity(t *testing.T) {
	cache := &geminiTokenCacheStub{REDACTED
	invalidator := NewCompositeTokenCacheInvalidator(cache)
	account := &Account{
		ID:       99,
		Platform: PlatformAntigravity,
		Type:     AccountTypeOAuth,
REDACTED
			"project_id": "ag-project",
	REDACTED,
REDACTED

	err := invalidator.InvalidateToken(context.Background(), account)
REDACTED
	require.Equal(t, []string{"ag:ag-project"REDACTED, cache.deletedKeys)
REDACTED

func TestCompositeTokenCacheInvalidator_OpenAI(t *testing.T) {
	cache := &geminiTokenCacheStub{REDACTED
	invalidator := NewCompositeTokenCacheInvalidator(cache)
	account := &Account{
		ID:       500,
		Platform: PlatformOpenAI,
		Type:     AccountTypeOAuth,
REDACTED
			"access_token": "openai-token",
	REDACTED,
REDACTED

	err := invalidator.InvalidateToken(context.Background(), account)
REDACTED
	require.Equal(t, []string{"openai:account:500"REDACTED, cache.deletedKeys)
REDACTED

func TestCompositeTokenCacheInvalidator_Claude(t *testing.T) {
	cache := &geminiTokenCacheStub{REDACTED
	invalidator := NewCompositeTokenCacheInvalidator(cache)
	account := &Account{
		ID:       600,
		Platform: PlatformAnthropic,
		Type:     AccountTypeOAuth,
REDACTED
			"access_token": "claude-token",
	REDACTED,
REDACTED

	err := invalidator.InvalidateToken(context.Background(), account)
REDACTED
	require.Equal(t, []string{"claude:account:600"REDACTED, cache.deletedKeys)
REDACTED

func TestCompositeTokenCacheInvalidator_SkipNonOAuth(t *testing.T) {
	cache := &geminiTokenCacheStub{REDACTED
	invalidator := NewCompositeTokenCacheInvalidator(cache)

	tests := []struct {
		name     string
		account  *Account
REDACTED{
		{
			name: "gemini_api_key",
			account: &Account{
				ID:       1,
		REDACTED
				Type:     AccountTypeAPIKey,
		REDACTED,
	REDACTED,
		{
			name: "openai_api_key",
			account: &Account{
				ID:       2,
				Platform: PlatformOpenAI,
				Type:     AccountTypeAPIKey,
		REDACTED,
	REDACTED,
		{
			name: "claude_api_key",
			account: &Account{
				ID:       3,
				Platform: PlatformAnthropic,
				Type:     AccountTypeAPIKey,
		REDACTED,
	REDACTED,
		{
			name: "claude_setup_token",
			account: &Account{
				ID:       4,
				Platform: PlatformAnthropic,
				Type:     AccountTypeSetupToken,
		REDACTED,
	REDACTED,
REDACTED

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cache.deletedKeys = nil
			err := invalidator.InvalidateToken(context.Background(), tt.account)
		REDACTED
			require.Empty(t, cache.deletedKeys)
	REDACTED)
REDACTED
REDACTED

func TestCompositeTokenCacheInvalidator_SkipUnsupportedPlatform(t *testing.T) {
	cache := &geminiTokenCacheStub{REDACTED
	invalidator := NewCompositeTokenCacheInvalidator(cache)
	account := &Account{
		ID:       100,
		Platform: "unknown-platform",
		Type:     AccountTypeOAuth,
REDACTED

	err := invalidator.InvalidateToken(context.Background(), account)
REDACTED
	require.Empty(t, cache.deletedKeys)
REDACTED

func TestCompositeTokenCacheInvalidator_NilCache(t *testing.T) {
	invalidator := NewCompositeTokenCacheInvalidator(nil)
	account := &Account{
		ID:       2,
REDACTED
		Type:     AccountTypeOAuth,
REDACTED

	err := invalidator.InvalidateToken(context.Background(), account)
REDACTED
REDACTED

func TestCompositeTokenCacheInvalidator_NilAccount(t *testing.T) {
	cache := &geminiTokenCacheStub{REDACTED
	invalidator := NewCompositeTokenCacheInvalidator(cache)

	err := invalidator.InvalidateToken(context.Background(), nil)
REDACTED
	require.Empty(t, cache.deletedKeys)
REDACTED

func TestCompositeTokenCacheInvalidator_NilInvalidator(t *testing.T) {
	var invalidator *CompositeTokenCacheInvalidator
	account := &Account{
		ID:       5,
REDACTED
		Type:     AccountTypeOAuth,
REDACTED

	err := invalidator.InvalidateToken(context.Background(), account)
REDACTED
REDACTED

func TestCompositeTokenCacheInvalidator_DeleteError(t *testing.T) {
	expectedErr := errors.New("redis connection failed")
	cache := &geminiTokenCacheStub{deleteErr: expectedErrREDACTED
	invalidator := NewCompositeTokenCacheInvalidator(cache)

	tests := []struct {
		name     string
		account  *Account
REDACTED{
		{
			name: "openai_delete_error",
			account: &Account{
				ID:       700,
				Platform: PlatformOpenAI,
				Type:     AccountTypeOAuth,
		REDACTED,
	REDACTED,
		{
			name: "claude_delete_error",
			account: &Account{
				ID:       800,
				Platform: PlatformAnthropic,
				Type:     AccountTypeOAuth,
		REDACTED,
	REDACTED,
REDACTED

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := invalidator.InvalidateToken(context.Background(), tt.account)
		REDACTED
			require.Equal(t, expectedErr, err)
	REDACTED)
REDACTED
REDACTED

func TestCompositeTokenCacheInvalidator_AllPlatformsIntegration(t *testing.T) {
	// 测试所有平台的缓存键生成和删除
	cache := &geminiTokenCacheStub{REDACTED
	invalidator := NewCompositeTokenCacheInvalidator(cache)

	accounts := []*Account{
		{ID: 1, Platform: PlatformGemini, Type: AccountTypeOAuth, Credentials: map[string]any{"project_id": "gemini-proj"REDACTEDREDACTED,
		{ID: 2, Platform: PlatformAntigravity, Type: AccountTypeOAuth, Credentials: map[string]any{"project_id": "ag-proj"REDACTEDREDACTED,
		{ID: 3, Platform: PlatformOpenAI, Type: AccountTypeOAuthREDACTED,
		{ID: 4, Platform: PlatformAnthropic, Type: AccountTypeOAuthREDACTED,
REDACTED

	expectedKeys := []string{
		"gemini:gemini-proj",
		"ag:ag-proj",
		"openai:account:3",
		"claude:account:4",
REDACTED

	for _, acc := range accounts {
		err := invalidator.InvalidateToken(context.Background(), acc)
	REDACTED
REDACTED

	require.Equal(t, expectedKeys, cache.deletedKeys)
REDACTED
