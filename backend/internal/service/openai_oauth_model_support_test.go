//go:build unit

package service

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func newOpenAIOAuthAccountForModelTest() *Account {
REDACTED
		ID:       1,
		Platform: PlatformOpenAI,
		Type:     AccountTypeOAuth,
REDACTED
REDACTED

func TestIsModelSupported_OpenAIOAuthEmptyMapping_ServableModels(t *testing.T) {
	account := newOpenAIOAuthAccountForModelTest()

	servable := []string{
		"", // 空模型交由上层必填校验
		"gpt-5.4",
		"gpt-5.4-high", // 推理后缀变体
		"gpt-5.3-codex",
		"gpt-5.3-codex-xhigh",
		"gpt-5.1-codex-mini",
		"gpt-5",
		"codex-mini-latest",
		"gpt5.3codexspark",  // 别名拼写归一化
		"gpt-image-1",       // 图像生成模型
		"claude-sonnet-4-6", // /v1/messages 调度默认映射兜底
		"claude-3-opus-20240229",
REDACTED
	for _, model := range servable {
		require.True(t, account.IsModelSupported(model), "expected %q to be servable by empty-mapping OpenAI OAuth account", model)
REDACTED
REDACTED

func TestIsModelSupported_OpenAIOAuthEmptyMapping_RejectsForeignModels(t *testing.T) {
	account := newOpenAIOAuthAccountForModelTest()

	// Codex 上游必然以不可重试的 400 拒绝这些模型；调度阶段就应跳过该账号，
	// 让显式声明支持的 API Key 账号接手（#3662）。
	foreign := []string{
		"deepseek-v4",
		"deepseek-chat",
		"glm-4.7",
		"kimi-k2",
		"gemini-3.0-pro",
		"grok-4",
		"qwen3-max",
REDACTED
	for _, model := range foreign {
		require.False(t, account.IsModelSupported(model), "expected %q to be rejected by empty-mapping OpenAI OAuth account", model)
REDACTED
REDACTED

func TestIsModelSupported_OpenAIOAuthExplicitMappingUnchanged(t *testing.T) {
	account := newOpenAIOAuthAccountForModelTest()
	account.Credentials = map[string]any{
		"model_mapping": map[string]any{"deepseek-v4": "gpt-5.4"REDACTED,
REDACTED

	// 显式映射沿用原有语义：命中映射即支持，未命中即不支持。
	require.True(t, account.IsModelSupported("deepseek-v4"))
	require.False(t, account.IsModelSupported("glm-4.7"))
REDACTED

func TestIsModelSupported_OpenAIOAuthPassthroughAllowsAll(t *testing.T) {
	account := newOpenAIOAuthAccountForModelTest()
	account.Extra = map[string]any{"openai_passthrough": trueREDACTED

	// 透传模式仅替换认证，模型语义由上游决定，保持"允许所有"。
	require.True(t, account.IsModelSupported("deepseek-v4"))
REDACTED

func TestIsModelSupported_OpenAIAPIKeyEmptyMappingAllowsAll(t *testing.T) {
	account := &Account{
		ID:       2,
		Platform: PlatformOpenAI,
		Type:     AccountTypeAPIKey,
REDACTED

	// API Key 账号（第三方 OpenAI 兼容上游）可服务任意别名，语义不变。
	require.True(t, account.IsModelSupported("deepseek-v4"))
	require.True(t, account.IsModelSupported("gpt-5.4"))
REDACTED

func TestIsModelSupported_NonOpenAIPlatformsUnchanged(t *testing.T) {
	anthropic := &Account{ID: 3, Platform: PlatformAnthropic, Type: AccountTypeOAuthREDACTED
	require.True(t, anthropic.IsModelSupported("claude-sonnet-4-6"))
	require.True(t, anthropic.IsModelSupported("deepseek-v4"))
REDACTED

func TestIsOpenAIOAuthServableModel(t *testing.T) {
	require.True(t, isOpenAIOAuthServableModel("gpt-5.4-high"))
	require.True(t, isOpenAIOAuthServableModel("  gpt-5.3-codex  "))
	require.True(t, isOpenAIOAuthServableModel("claude-3-5-haiku-20241022"))
	require.False(t, isOpenAIOAuthServableModel("claude-unknown-family")) // 无 opus/sonnet/haiku 关键字，无默认映射兜底
	require.False(t, isOpenAIOAuthServableModel("deepseek-v4"))
REDACTED
