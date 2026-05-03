//go:build unit

package service

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestBuildOpenAIChatCompletionsURL(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		base string
		want string
REDACTED{
		// 已是 /chat/completions：原样返回
		{"already chat/completions", "https://api.openai.com/v1/chat/completions", "https://api.openai.com/v1/chat/completions"REDACTED,
		// 以 /v1 结尾：追加 /chat/completions
		{"bare /v1", "https://api.openai.com/v1", "https://api.openai.com/v1/chat/completions"REDACTED,
		// 其他情况：追加 /v1/chat/completions
		{"bare domain", "https://api.openai.com", "https://api.openai.com/v1/chat/completions"REDACTED,
		{"domain with trailing slash", "https://api.openai.com/", "https://api.openai.com/v1/chat/completions"REDACTED,
		// 第三方上游常见形式
		{"third-party bare domain", "https://api.deepseek.com", "https://api.deepseek.com/v1/chat/completions"REDACTED,
		{"third-party with path prefix", "https://api.gptgod.online/api", "https://api.gptgod.online/api/v1/chat/completions"REDACTED,
		// 带空白字符
		{"whitespace trimmed", "  https://api.openai.com/v1  ", "https://api.openai.com/v1/chat/completions"REDACTED,
REDACTED

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := buildOpenAIChatCompletionsURL(tt.base)
			require.Equal(t, tt.want, got)
	REDACTED)
REDACTED
REDACTED

// TestBuildOpenAIResponsesURL_ProbeURL 锁定 probe/测试端点使用的 URL 构建逻辑，
// 确保 buildOpenAIResponsesURL 对标准 OpenAI base_url 格式均拼出 `/v1/responses`。
func TestBuildOpenAIResponsesURL_ProbeURL(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		base string
		want string
REDACTED{
		{"bare domain", "https://api.openai.com", "https://api.openai.com/v1/responses"REDACTED,
		{"domain trailing slash", "https://api.openai.com/", "https://api.openai.com/v1/responses"REDACTED,
		{"bare /v1", "https://api.openai.com/v1", "https://api.openai.com/v1/responses"REDACTED,
		{"already /responses", "https://api.openai.com/v1/responses", "https://api.openai.com/v1/responses"REDACTED,
		{"third-party bare domain", "https://api.deepseek.com", "https://api.deepseek.com/v1/responses"REDACTED,
		{"only domain, no scheme", "api.gptgod.online", "api.gptgod.online/v1/responses"REDACTED,
REDACTED

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := buildOpenAIResponsesURL(tt.base)
			require.Equal(t, tt.want, got)
	REDACTED)
REDACTED
REDACTED
