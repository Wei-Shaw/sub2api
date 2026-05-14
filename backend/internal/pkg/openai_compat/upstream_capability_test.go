package openai_compat

import "testing"

func TestResolveResponsesSupport(t *testing.T) {
	tests := []struct {
		name  string
		extra map[string]any
		want  AccountResponsesSupport
REDACTED{
		{"nil extra", nil, ResponsesSupportUnknownREDACTED,
		{"empty extra", map[string]any{REDACTED, ResponsesSupportUnknownREDACTED,
		{"key missing", map[string]any{"other": "value"REDACTED, ResponsesSupportUnknownREDACTED,
		{"value true", map[string]any{ExtraKeyResponsesSupported: trueREDACTED, ResponsesSupportYesREDACTED,
		{"value false", map[string]any{ExtraKeyResponsesSupported: falseREDACTED, ResponsesSupportNoREDACTED,
		{"value wrong type string", map[string]any{ExtraKeyResponsesSupported: "true"REDACTED, ResponsesSupportUnknownREDACTED,
		{"value wrong type number", map[string]any{ExtraKeyResponsesSupported: 1REDACTED, ResponsesSupportUnknownREDACTED,
		{"value nil", map[string]any{ExtraKeyResponsesSupported: nilREDACTED, ResponsesSupportUnknownREDACTED,
		{"force responses", map[string]any{ExtraKeyResponsesMode: string(ResponsesSupportModeForceResponses)REDACTED, ResponsesSupportYesREDACTED,
		{"force chat completions", map[string]any{ExtraKeyResponsesMode: string(ResponsesSupportModeForceChatCompletions)REDACTED, ResponsesSupportNoREDACTED,
		{"auto follows probe", map[string]any{ExtraKeyResponsesMode: string(ResponsesSupportModeAuto), ExtraKeyResponsesSupported: falseREDACTED, ResponsesSupportNoREDACTED,
		{"invalid mode follows probe", map[string]any{ExtraKeyResponsesMode: "bogus", ExtraKeyResponsesSupported: trueREDACTED, ResponsesSupportYesREDACTED,
		{"force responses overrides probe false", map[string]any{ExtraKeyResponsesMode: string(ResponsesSupportModeForceResponses), ExtraKeyResponsesSupported: falseREDACTED, ResponsesSupportYesREDACTED,
		{"force chat completions overrides probe true", map[string]any{ExtraKeyResponsesMode: string(ResponsesSupportModeForceChatCompletions), ExtraKeyResponsesSupported: trueREDACTED, ResponsesSupportNoREDACTED,
REDACTED

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := ResolveResponsesSupport(tc.extra)
			if got != tc.want {
				t.Errorf("ResolveResponsesSupport(%v) = %v, want %v", tc.extra, got, tc.want)
		REDACTED
	REDACTED)
REDACTED
REDACTED

func TestShouldUseResponsesAPI(t *testing.T) {
	tests := []struct {
		name  string
		extra map[string]any
		want  bool
REDACTED{
		// 关键不变量：未探测必须返回 true（保留旧行为）
		{"unknown defaults to true (preserve old behavior)", nil, trueREDACTED,
		{"unknown empty defaults to true", map[string]any{REDACTED, trueREDACTED,
		{"unknown wrong type defaults to true", map[string]any{ExtraKeyResponsesSupported: "yes"REDACTED, trueREDACTED,

		// 已探测：标记决定
		{"explicitly supported", map[string]any{ExtraKeyResponsesSupported: trueREDACTED, trueREDACTED,
		{"explicitly unsupported", map[string]any{ExtraKeyResponsesSupported: falseREDACTED, falseREDACTED,

		// 手动覆盖：覆盖自动探测结果
		{"force responses overrides unsupported probe", map[string]any{ExtraKeyResponsesMode: string(ResponsesSupportModeForceResponses), ExtraKeyResponsesSupported: falseREDACTED, trueREDACTED,
		{"force chat completions overrides supported probe", map[string]any{ExtraKeyResponsesMode: string(ResponsesSupportModeForceChatCompletions), ExtraKeyResponsesSupported: trueREDACTED, falseREDACTED,
REDACTED

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := ShouldUseResponsesAPI(tc.extra)
			if got != tc.want {
				t.Errorf("ShouldUseResponsesAPI(%v) = %v, want %v", tc.extra, got, tc.want)
		REDACTED
	REDACTED)
REDACTED
REDACTED

func TestNormalizeResponsesSupportMode(t *testing.T) {
	tests := []struct {
		name string
		mode string
		want ResponsesSupportMode
REDACTED{
		{"empty", "", ResponsesSupportModeAutoREDACTED,
		{"auto", "auto", ResponsesSupportModeAutoREDACTED,
		{"force responses", "force_responses", ResponsesSupportModeForceResponsesREDACTED,
		{"force chat completions", "force_chat_completions", ResponsesSupportModeForceChatCompletionsREDACTED,
		{"invalid", "enabled", ResponsesSupportModeAutoREDACTED,
REDACTED

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := NormalizeResponsesSupportMode(tc.mode)
			if got != tc.want {
				t.Errorf("NormalizeResponsesSupportMode(%q) = %q, want %q", tc.mode, got, tc.want)
		REDACTED
	REDACTED)
REDACTED
REDACTED
