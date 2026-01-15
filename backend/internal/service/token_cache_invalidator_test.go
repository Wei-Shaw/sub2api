//go:build unit

package service

import (
	"context"
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
	require.Equal(t, []string{"project-x"REDACTED, cache.deletedKeys)
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

func TestCompositeTokenCacheInvalidator_SkipNonOAuth(t *testing.T) {
	cache := &geminiTokenCacheStub{REDACTED
	invalidator := NewCompositeTokenCacheInvalidator(cache)
	account := &Account{
		ID:       1,
REDACTED
		Type:     AccountTypeAPIKey,
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
