//go:build unit

package service

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/stretchr/testify/require"
)

type bmRepoStub struct {
	getValueFn func(ctx context.Context, key string) (string, error)
	calls      int
REDACTED

func (s *bmRepoStub) Get(ctx context.Context, key string) (*Setting, error) {
	panic("unexpected Get call")
REDACTED

func (s *bmRepoStub) GetValue(ctx context.Context, key string) (string, error) {
	s.calls++
	if s.getValueFn == nil {
		panic("unexpected GetValue call")
REDACTED
	return s.getValueFn(ctx, key)
REDACTED

func (s *bmRepoStub) Set(ctx context.Context, key, value string) error {
	panic("unexpected Set call")
REDACTED

func (s *bmRepoStub) GetMultiple(ctx context.Context, keys []string) (map[string]string, error) {
	panic("unexpected GetMultiple call")
REDACTED

func (s *bmRepoStub) SetMultiple(ctx context.Context, settings map[string]string) error {
	panic("unexpected SetMultiple call")
REDACTED

func (s *bmRepoStub) GetAll(ctx context.Context) (map[string]string, error) {
	panic("unexpected GetAll call")
REDACTED

func (s *bmRepoStub) Delete(ctx context.Context, key string) error {
	panic("unexpected Delete call")
REDACTED

type bmUpdateRepoStub struct {
	updates    map[string]string
	getValueFn func(ctx context.Context, key string) (string, error)
REDACTED

func (s *bmUpdateRepoStub) Get(ctx context.Context, key string) (*Setting, error) {
	panic("unexpected Get call")
REDACTED

func (s *bmUpdateRepoStub) GetValue(ctx context.Context, key string) (string, error) {
	if s.getValueFn == nil {
		panic("unexpected GetValue call")
REDACTED
	return s.getValueFn(ctx, key)
REDACTED

func (s *bmUpdateRepoStub) Set(ctx context.Context, key, value string) error {
	panic("unexpected Set call")
REDACTED

func (s *bmUpdateRepoStub) GetMultiple(ctx context.Context, keys []string) (map[string]string, error) {
	panic("unexpected GetMultiple call")
REDACTED

func (s *bmUpdateRepoStub) SetMultiple(ctx context.Context, settings map[string]string) error {
	s.updates = make(map[string]string, len(settings))
	for k, v := range settings {
		s.updates[k] = v
REDACTED
	return nil
REDACTED

func (s *bmUpdateRepoStub) GetAll(ctx context.Context) (map[string]string, error) {
	panic("unexpected GetAll call")
REDACTED

func (s *bmUpdateRepoStub) Delete(ctx context.Context, key string) error {
	panic("unexpected Delete call")
REDACTED

func resetBackendModeTestCache(t *testing.T) {
REDACTED

	backendModeCache.Store((*cachedBackendMode)(nil))
	t.Cleanup(func() {
		backendModeCache.Store((*cachedBackendMode)(nil))
REDACTED)
REDACTED

func TestIsBackendModeEnabled_ReturnsTrue(t *testing.T) {
	resetBackendModeTestCache(t)

	repo := &bmRepoStub{
		getValueFn: func(ctx context.Context, key string) (string, error) {
			require.Equal(t, SettingKeyBackendModeEnabled, key)
			return "true", nil
	REDACTED,
REDACTED
	svc := NewSettingService(repo, &config.Config{REDACTED)

	require.True(t, svc.IsBackendModeEnabled(context.Background()))
	require.Equal(t, 1, repo.calls)
REDACTED

func TestIsBackendModeEnabled_ReturnsFalse(t *testing.T) {
	resetBackendModeTestCache(t)

	repo := &bmRepoStub{
		getValueFn: func(ctx context.Context, key string) (string, error) {
			require.Equal(t, SettingKeyBackendModeEnabled, key)
			return "false", nil
	REDACTED,
REDACTED
	svc := NewSettingService(repo, &config.Config{REDACTED)

	require.False(t, svc.IsBackendModeEnabled(context.Background()))
	require.Equal(t, 1, repo.calls)
REDACTED

func TestIsBackendModeEnabled_ReturnsFalseOnNotFound(t *testing.T) {
	resetBackendModeTestCache(t)

	repo := &bmRepoStub{
		getValueFn: func(ctx context.Context, key string) (string, error) {
			require.Equal(t, SettingKeyBackendModeEnabled, key)
			return "", ErrSettingNotFound
	REDACTED,
REDACTED
	svc := NewSettingService(repo, &config.Config{REDACTED)

	require.False(t, svc.IsBackendModeEnabled(context.Background()))
	require.Equal(t, 1, repo.calls)
REDACTED

func TestIsBackendModeEnabled_ReturnsFalseOnDBError(t *testing.T) {
	resetBackendModeTestCache(t)

	repo := &bmRepoStub{
		getValueFn: func(ctx context.Context, key string) (string, error) {
			require.Equal(t, SettingKeyBackendModeEnabled, key)
			return "", errors.New("db down")
	REDACTED,
REDACTED
	svc := NewSettingService(repo, &config.Config{REDACTED)

	require.False(t, svc.IsBackendModeEnabled(context.Background()))
	require.Equal(t, 1, repo.calls)
REDACTED

func TestIsBackendModeEnabled_CachesResult(t *testing.T) {
	resetBackendModeTestCache(t)

	repo := &bmRepoStub{
		getValueFn: func(ctx context.Context, key string) (string, error) {
			require.Equal(t, SettingKeyBackendModeEnabled, key)
			return "true", nil
	REDACTED,
REDACTED
	svc := NewSettingService(repo, &config.Config{REDACTED)

	require.True(t, svc.IsBackendModeEnabled(context.Background()))
	require.True(t, svc.IsBackendModeEnabled(context.Background()))
	require.Equal(t, 1, repo.calls)
REDACTED

func TestUpdateSettings_InvalidatesBackendModeCache(t *testing.T) {
	resetBackendModeTestCache(t)

	backendModeCache.Store(&cachedBackendMode{
		value:     true,
		expiresAt: time.Now().Add(backendModeCacheTTL).UnixNano(),
REDACTED)

	repo := &bmUpdateRepoStub{
		getValueFn: func(ctx context.Context, key string) (string, error) {
			require.Equal(t, SettingKeyBackendModeEnabled, key)
			return "true", nil
	REDACTED,
REDACTED
	svc := NewSettingService(repo, &config.Config{REDACTED)

	err := svc.UpdateSettings(context.Background(), &SystemSettings{
		BackendModeEnabled: false,
REDACTED)
REDACTED
	require.Equal(t, "false", repo.updates[SettingKeyBackendModeEnabled])
	require.False(t, svc.IsBackendModeEnabled(context.Background()))
REDACTED
