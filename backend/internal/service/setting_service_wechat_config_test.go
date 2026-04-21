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
	require.Equal(t, "wx-db-app", got.AppID)
	require.Equal(t, "wx-db-secret", got.AppSecret)
	require.True(t, got.OpenEnabled)
	require.True(t, got.MPEnabled)
	require.Equal(t, "mp", got.Mode)
	require.Equal(t, "snsapi_base", got.Scopes)
	require.Equal(t, "https://api.example.com/api/v1/auth/oauth/wechat/callback", got.RedirectURL)
	require.Equal(t, "/auth/wechat/callback", got.FrontendRedirectURL)
REDACTED
