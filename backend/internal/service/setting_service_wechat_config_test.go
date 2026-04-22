//go:build unit

package service

import (
	"context"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/stretchr/testify/require"
)

type settingWeChatRepoStub struct {
	values map[string]string
REDACTED

func (s *settingWeChatRepoStub) Get(context.Context, string) (*Setting, error) {
	panic("unexpected Get call")
REDACTED

func (s *settingWeChatRepoStub) GetValue(_ context.Context, key string) (string, error) {
	if value, ok := s.values[key]; ok {
		return value, nil
REDACTED
	return "", ErrSettingNotFound
REDACTED

func (s *settingWeChatRepoStub) Set(context.Context, string, string) error {
	panic("unexpected Set call")
REDACTED

func (s *settingWeChatRepoStub) GetMultiple(_ context.Context, keys []string) (map[string]string, error) {
	out := make(map[string]string, len(keys))
	for _, key := range keys {
		if value, ok := s.values[key]; ok {
			out[key] = value
	REDACTED
REDACTED
	return out, nil
REDACTED

func (s *settingWeChatRepoStub) SetMultiple(context.Context, map[string]string) error {
	panic("unexpected SetMultiple call")
REDACTED

func (s *settingWeChatRepoStub) GetAll(context.Context) (map[string]string, error) {
	panic("unexpected GetAll call")
REDACTED

func (s *settingWeChatRepoStub) Delete(context.Context, string) error {
	panic("unexpected Delete call")
REDACTED

func TestSettingService_GetWeChatConnectOAuthConfig_UsesDatabaseOverrides(t *testing.T) {
	repo := &settingWeChatRepoStub{
		values: map[string]string{
			SettingKeyWeChatConnectEnabled:             "true",
			SettingKeyWeChatConnectAppID:               "wx-db-app",
			SettingKeyWeChatConnectAppSecret:           "wx-db-secret",
			SettingKeyWeChatConnectMode:                "mp",
			SettingKeyWeChatConnectScopes:              "snsapi_base",
			SettingKeyWeChatConnectOpenEnabled:         "true",
			SettingKeyWeChatConnectMPEnabled:           "true",
			SettingKeyWeChatConnectRedirectURL:         "https://api.example.com/api/v1/auth/oauth/wechat/callback",
			SettingKeyWeChatConnectFrontendRedirectURL: "/auth/wechat/callback",
	REDACTED,
REDACTED
	svc := NewSettingService(repo, &config.Config{REDACTED)

	got, err := svc.GetWeChatConnectOAuthConfig(context.Background())
REDACTED
	require.True(t, got.Enabled)
	require.Equal(t, "wx-db-app", got.AppIDForMode("mp"))
	require.Equal(t, "wx-db-secret", got.AppSecretForMode("mp"))
	require.True(t, got.OpenEnabled)
	require.True(t, got.MPEnabled)
	require.Equal(t, "mp", got.Mode)
	require.Equal(t, "snsapi_base", got.Scopes)
	require.Equal(t, "https://api.example.com/api/v1/auth/oauth/wechat/callback", got.RedirectURL)
	require.Equal(t, "/auth/wechat/callback", got.FrontendRedirectURL)
REDACTED

func TestSettingService_GetWeChatConnectOAuthConfig_FallsBackToConfigWhenDatabaseEmpty(t *testing.T) {
	repo := &settingWeChatRepoStub{values: map[string]string{REDACTEDREDACTED
	svc := NewSettingService(repo, &config.Config{
		WeChat: config.WeChatConnectConfig{
			Enabled:             true,
			OpenEnabled:         true,
			MPEnabled:           true,
			Mode:                "open",
			OpenAppID:           "wx-open-config",
			OpenAppSecret:       "wx-open-secret",
			MPAppID:             "wx-mp-config",
			MPAppSecret:         "wx-mp-secret",
			FrontendRedirectURL: "/auth/wechat/config-callback",
	REDACTED,
REDACTED)

	got, err := svc.GetWeChatConnectOAuthConfig(context.Background())
REDACTED
	require.True(t, got.Enabled)
	require.True(t, got.OpenEnabled)
	require.True(t, got.MPEnabled)
	require.Equal(t, "wx-open-config", got.AppIDForMode("open"))
	require.Equal(t, "wx-open-secret", got.AppSecretForMode("open"))
	require.Equal(t, "wx-mp-config", got.AppIDForMode("mp"))
	require.Equal(t, "wx-mp-secret", got.AppSecretForMode("mp"))
	require.Equal(t, "/auth/wechat/config-callback", got.FrontendRedirectURL)
	require.Empty(t, got.RedirectURL)
REDACTED

func TestSettingService_GetWeChatConnectOAuthConfig_IgnoresSyntheticDisabledCapabilitiesFromMigration118(t *testing.T) {
	repo := &settingWeChatRepoStub{
		values: map[string]string{
			SettingKeyWeChatConnectOpenEnabled: "false",
			SettingKeyWeChatConnectMPEnabled:   "false",
	REDACTED,
REDACTED
	svc := NewSettingService(repo, &config.Config{
		WeChat: config.WeChatConnectConfig{
			Enabled:             true,
			OpenEnabled:         true,
			MPEnabled:           true,
			Mode:                "open",
			OpenAppID:           "wx-open-config",
			OpenAppSecret:       "wx-open-secret",
			MPAppID:             "wx-mp-config",
			MPAppSecret:         "wx-mp-secret",
			FrontendRedirectURL: "/auth/wechat/config-callback",
	REDACTED,
REDACTED)

	got, err := svc.GetWeChatConnectOAuthConfig(context.Background())
REDACTED
	require.True(t, got.Enabled)
	require.True(t, got.OpenEnabled)
	require.True(t, got.MPEnabled)
	require.Equal(t, "wx-open-config", got.AppIDForMode("open"))
	require.Equal(t, "wx-mp-config", got.AppIDForMode("mp"))
REDACTED

func TestSettingService_ParseSettings_FallsBackToConfigForWeChatAdminView(t *testing.T) {
	svc := NewSettingService(&settingWeChatRepoStub{values: map[string]string{REDACTEDREDACTED, &config.Config{
		WeChat: config.WeChatConnectConfig{
			Enabled:             true,
			OpenEnabled:         true,
			Mode:                "open",
			OpenAppID:           "wx-open-config",
			OpenAppSecret:       "wx-open-secret",
			FrontendRedirectURL: "/auth/wechat/config-callback",
	REDACTED,
REDACTED)

	got := svc.parseSettings(map[string]string{REDACTED)
	require.True(t, got.WeChatConnectEnabled)
	require.True(t, got.WeChatConnectOpenEnabled)
	require.Equal(t, "wx-open-config", got.WeChatConnectOpenAppID)
	require.True(t, got.WeChatConnectOpenAppSecretConfigured)
	require.Equal(t, "/auth/wechat/config-callback", got.WeChatConnectFrontendRedirectURL)
	require.Equal(t, "open", got.WeChatConnectMode)
	require.Equal(t, "snsapi_login", got.WeChatConnectScopes)
REDACTED
