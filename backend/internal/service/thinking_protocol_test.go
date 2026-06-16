package service

import "testing"

func TestResolveThinkingProtocol(t *testing.T) {
	tests := []struct {
		name    string
		modelID string
		want    ThinkingProtocol
REDACTED{
		// Anthropic 官方
		{"claude-sonnet-4-5", "claude-sonnet-4-5", ThinkingProtocolAnthropicStrictREDACTED,
		{"claude-opus-4-5", "claude-opus-4-5-20251101", ThinkingProtocolAnthropicStrictREDACTED,
		{"claude-haiku full id", "REDACTED", ThinkingProtocolAnthropicStrictREDACTED,
		{"opus short", "opus-4-5", ThinkingProtocolAnthropicStrictREDACTED,
		{"sonnet short", "sonnet-4-5", ThinkingProtocolAnthropicStrictREDACTED,
		{"haiku short", "haiku-4-5", ThinkingProtocolAnthropicStrictREDACTED,
		{"upper case Claude", "Claude-Sonnet-4-5", ThinkingProtocolAnthropicStrictREDACTED,

		// 第三方兼容上游
		{"deepseek-v4-pro", "deepseek-v4-pro", ThinkingProtocolPassbackRequiredREDACTED,
		{"deepseek-r2-thinking", "deepseek-r2-thinking", ThinkingProtocolPassbackRequiredREDACTED,
		{"kimi-coding", "kimi-coding-v2", ThinkingProtocolPassbackRequiredREDACTED,
		{"kimi-k2-thinking", "kimi-k2-thinking", ThinkingProtocolPassbackRequiredREDACTED,
		{"moonshot-v1", "moonshot-v1-32k", ThinkingProtocolPassbackRequiredREDACTED,
		{"glm-5.1", "glm-5.1", ThinkingProtocolPassbackRequiredREDACTED,
		{"qwen-2 thinking variant", "qwen-2-72b-thinking", ThinkingProtocolPassbackRequiredREDACTED,
		{"qwen3 thinking (real Alibaba naming)", "qwen3-235b-a22b-thinking-2507", ThinkingProtocolPassbackRequiredREDACTED,
		{"qwen3-next thinking", "qwen3-next-80b-a3b-thinking", ThinkingProtocolPassbackRequiredREDACTED,
		{"upper case Deepseek", "DeepSeek-V4-Pro", ThinkingProtocolPassbackRequiredREDACTED,

		// MiniMax M 系列（Anthropic 兼容端点要求 thinking round-trip）
		{"MiniMax-M2 (case-sensitive original)", "MiniMax-M2", ThinkingProtocolPassbackRequiredREDACTED,
		{"MiniMax-M2.1", "MiniMax-M2.1", ThinkingProtocolPassbackRequiredREDACTED,
		{"MiniMax-M2.5", "MiniMax-M2.5", ThinkingProtocolPassbackRequiredREDACTED,
		{"MiniMax-M2.7", "MiniMax-M2.7", ThinkingProtocolPassbackRequiredREDACTED,
		{"MiniMax-M2.7-highspeed", "MiniMax-M2.7-highspeed", ThinkingProtocolPassbackRequiredREDACTED,
		{"minimax-m2 lowercase", "minimax-m2", ThinkingProtocolPassbackRequiredREDACTED,

		// 未知 / 保守
		{"empty", "", ThinkingProtocolUnknownREDACTED,
		{"gpt-5", "gpt-5.1", ThinkingProtocolUnknownREDACTED,
		{"gemini", "gemini-3-pro-preview", ThinkingProtocolUnknownREDACTED,
		{"qwen3 non-thinking", "qwen3-32b", ThinkingProtocolUnknownREDACTED,
		{"qwen2 non-thinking", "qwen-2-72b", ThinkingProtocolUnknownREDACTED,
		{"random vendor", "yi-large", ThinkingProtocolUnknownREDACTED,
		// MiniMax 非 M 系列（如 abab、speech 等其他产品线）—— unknown
		{"minimax abab non-M", "abab6.5-chat", ThinkingProtocolUnknownREDACTED,
		// Doubao 走 OpenAI 协议，不属于本网关 Anthropic 路径——归 unknown
		{"doubao goes via openai", "doubao-1-5-thinking-vision-pro-250428", ThinkingProtocolUnknownREDACTED,
		// Hunyuan T1 未暴露 Anthropic 端点——归 unknown
		{"hunyuan t1 no anthropic endpoint", "hunyuan-t1", ThinkingProtocolUnknownREDACTED,
		{"hy-t1 short alias", "hy-t1", ThinkingProtocolUnknownREDACTED,
		// claude-something 但不是 anthropic 官方命名风格——也归 strict（前缀匹配优先）
		{"weird claude prefix", "claude-experimental-fork", ThinkingProtocolAnthropicStrictREDACTED,
REDACTED
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ResolveThinkingProtocol(tt.modelID)
			if got != tt.want {
				t.Errorf("ResolveThinkingProtocol(%q) = %v, want %v", tt.modelID, got, tt.want)
		REDACTED
	REDACTED)
REDACTED
REDACTED

func TestShouldPreFilterThinkingBlocks(t *testing.T) {
	tests := []struct {
		modelID string
		want    bool
REDACTED{
		{"claude-sonnet-4-5", trueREDACTED,
		{"deepseek-v4-pro", falseREDACTED,
		{"kimi-coding", falseREDACTED,
		{"glm-5.1", falseREDACTED,
		{"gpt-5.1", falseREDACTED,
		{"", falseREDACTED,
REDACTED
	for _, tt := range tests {
		t.Run(tt.modelID, func(t *testing.T) {
			if got := ShouldPreFilterThinkingBlocks(tt.modelID); got != tt.want {
				t.Errorf("ShouldPreFilterThinkingBlocks(%q) = %v, want %v", tt.modelID, got, tt.want)
		REDACTED
	REDACTED)
REDACTED
REDACTED

func TestShouldRectifyThinkingSignatureError(t *testing.T) {
	if !ShouldRectifyThinkingSignatureError("claude-sonnet-4-5") {
		t.Error("anthropic-strict should rectify signature error")
REDACTED
	if ShouldRectifyThinkingSignatureError("deepseek-v4-pro") {
		t.Error("passback-required must NOT rectify (would break protocol contract)")
REDACTED
	if ShouldRectifyThinkingSignatureError("gpt-5.1") {
		t.Error("unknown should NOT rectify (conservative default)")
REDACTED
	if ShouldRectifyThinkingSignatureError("") {
		t.Error("empty model id should NOT rectify")
REDACTED
REDACTED

// ShouldApplyRetryFilters 与 ShouldPreFilterThinkingBlocks 必须语义一致：
// 仅 anthropic-strict 走变形，避免预过滤跳过但 retry 路径反而剥离的语义裂缝。
func TestShouldApplyRetryFiltersMirrorsPreFilter(t *testing.T) {
	models := []string{
		"claude-sonnet-4-5", "claude-opus-4-5-20251101", "haiku-4-5",
		"deepseek-v4-pro", "kimi-coding", "glm-5.1",
		"qwen3-235b-a22b-thinking-2507", "qwen3-32b",
		"gpt-5.1", "gemini-3-pro-preview", "yi-large", "",
REDACTED
	for _, m := range models {
		t.Run(m, func(t *testing.T) {
			if got := ShouldApplyRetryFilters(m); got != ShouldPreFilterThinkingBlocks(m) {
				t.Errorf("ShouldApplyRetryFilters(%q)=%v but ShouldPreFilterThinkingBlocks=%v — must match",
					m, got, ShouldPreFilterThinkingBlocks(m))
		REDACTED
	REDACTED)
REDACTED
REDACTED
