package service

import (
	"context"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/stretchr/testify/require"
)

type allowClaudeCodeSettingRepoStub struct{ values map[string]string REDACTED

func (s *allowClaudeCodeSettingRepoStub) Get(ctx context.Context, key string) (*Setting, error) {
	panic("unused")
REDACTED
func (s *allowClaudeCodeSettingRepoStub) GetValue(ctx context.Context, key string) (string, error) {
	if v, ok := s.values[key]; ok {
		return v, nil
REDACTED
	return "", ErrSettingNotFound
REDACTED
func (s *allowClaudeCodeSettingRepoStub) Set(ctx context.Context, key, value string) error {
	panic("unused")
REDACTED
func (s *allowClaudeCodeSettingRepoStub) GetMultiple(ctx context.Context, keys []string) (map[string]string, error) {
	panic("unused")
REDACTED
func (s *allowClaudeCodeSettingRepoStub) SetMultiple(ctx context.Context, settings map[string]string) error {
	panic("unused")
REDACTED
func (s *allowClaudeCodeSettingRepoStub) GetAll(ctx context.Context) (map[string]string, error) {
	panic("unused")
REDACTED
func (s *allowClaudeCodeSettingRepoStub) Delete(ctx context.Context, key string) error {
	panic("unused")
REDACTED

func TestSettingService_IsOpenAIAllowClaudeCodeCodexPluginEnabled(t *testing.T) {
	t.Run("默认关闭（设置缺失）", func(t *testing.T) {
		svc := NewSettingService(&allowClaudeCodeSettingRepoStub{values: map[string]string{REDACTEDREDACTED, &config.Config{REDACTED)
		require.False(t, svc.IsOpenAIAllowClaudeCodeCodexPluginEnabled(context.Background()))
REDACTED)
	t.Run("值为 true 时开启", func(t *testing.T) {
		svc := NewSettingService(&allowClaudeCodeSettingRepoStub{values: map[string]string{
			SettingKeyOpenAIAllowClaudeCodeCodexPlugin: "true",
REDACTED &config.Config{REDACTED)
		require.True(t, svc.IsOpenAIAllowClaudeCodeCodexPluginEnabled(context.Background()))
REDACTED)
	t.Run("值非 true 时关闭", func(t *testing.T) {
		svc := NewSettingService(&allowClaudeCodeSettingRepoStub{values: map[string]string{
			SettingKeyOpenAIAllowClaudeCodeCodexPlugin: "false",
REDACTED &config.Config{REDACTED)
		require.False(t, svc.IsOpenAIAllowClaudeCodeCodexPluginEnabled(context.Background()))
REDACTED)
REDACTED
