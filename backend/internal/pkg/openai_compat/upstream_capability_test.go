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
