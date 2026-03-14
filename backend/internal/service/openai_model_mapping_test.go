package service

import "testing"

func TestResolveOpenAIForwardModel(t *testing.T) {
	tests := []struct {
		name               string
		account            *Account
		requestedModel     string
		defaultMappedModel string
		expectedModel      string
REDACTED{
		{
			name: "falls back to group default when account has no mapping",
			account: &Account{
		REDACTEDREDACTED,
		REDACTED,
			requestedModel:     "gpt-5.4",
			defaultMappedModel: "gpt-4o-mini",
			expectedModel:      "gpt-4o-mini",
	REDACTED,
		{
			name: "preserves exact passthrough mapping instead of group default",
			account: &Account{
		REDACTED
					"model_mapping": map[string]any{
						"gpt-5.4": "gpt-5.4",
				REDACTED,
			REDACTED,
		REDACTED,
			requestedModel:     "gpt-5.4",
			defaultMappedModel: "gpt-4o-mini",
			expectedModel:      "gpt-5.4",
	REDACTED,
		{
			name: "preserves wildcard passthrough mapping instead of group default",
			account: &Account{
		REDACTED
					"model_mapping": map[string]any{
						"gpt-*": "gpt-5.4",
				REDACTED,
			REDACTED,
		REDACTED,
			requestedModel:     "gpt-5.4",
			defaultMappedModel: "gpt-4o-mini",
			expectedModel:      "gpt-5.4",
	REDACTED,
		{
			name: "uses account remap when explicit target differs",
			account: &Account{
		REDACTED
					"model_mapping": map[string]any{
						"gpt-5": "gpt-5.4",
				REDACTED,
			REDACTED,
		REDACTED,
			requestedModel:     "gpt-5",
			defaultMappedModel: "gpt-4o-mini",
			expectedModel:      "gpt-5.4",
	REDACTED,
REDACTED

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := resolveOpenAIForwardModel(tt.account, tt.requestedModel, tt.defaultMappedModel); got != tt.expectedModel {
				t.Fatalf("resolveOpenAIForwardModel(...) = %q, want %q", got, tt.expectedModel)
		REDACTED
	REDACTED)
REDACTED
REDACTED
