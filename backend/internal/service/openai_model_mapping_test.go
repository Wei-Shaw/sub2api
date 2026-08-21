package service

import "testing"

func TestResolveOpenAIForwardModel(t *testing.T) {
	tests := []struct {
		name                        string
		account                     *Account
		requestedModel              string
		messagesDispatchMappedModel string
		expectedModel               string
REDACTED{
		{
			name: "uses messages dispatch model for known claude family",
			account: &Account{
		REDACTEDREDACTED,
		REDACTED,
			requestedModel:              "claude-opus-4-6",
			messagesDispatchMappedModel: "gpt-4o-mini",
			expectedModel:               "gpt-4o-mini",
	REDACTED,
		{
			name: "uses exact messages dispatch model for unknown claude family",
			account: &Account{
		REDACTEDREDACTED,
		REDACTED,
			requestedModel:              "claude-fable-5",
			messagesDispatchMappedModel: " gpt-5.6-sol ",
			expectedModel:               "gpt-5.6-sol",
	REDACTED,
		{
			name:                        "nil account uses messages dispatch model",
			requestedModel:              "claude-fable-5",
			messagesDispatchMappedModel: "gpt-5.6-sol",
			expectedModel:               "gpt-5.6-sol",
	REDACTED,
		{
			name:           "nil account without messages dispatch keeps requested model",
			requestedModel: "claude-fable-5",
			expectedModel:  "claude-fable-5",
	REDACTED,
		{
			name: "ordinary unknown gpt model has no messages dispatch fallback",
			account: &Account{
		REDACTEDREDACTED,
		REDACTED,
			requestedModel: "gpt6",
			expectedModel:  "gpt6",
	REDACTED,
		{
			name: "account exact mapping overrides messages dispatch model",
			account: &Account{
		REDACTED
					"model_mapping": map[string]any{
						"claude-fable-5": "gpt-5.5",
				REDACTED,
			REDACTED,
		REDACTED,
			requestedModel:              "claude-fable-5",
			messagesDispatchMappedModel: "gpt-5.6-sol",
			expectedModel:               "gpt-5.5",
	REDACTED,
		{
			name: "account wildcard mapping overrides messages dispatch model",
			account: &Account{
		REDACTED
					"model_mapping": map[string]any{
						"claude-*": "gpt-5.4",
				REDACTED,
			REDACTED,
		REDACTED,
			requestedModel:              "claude-fable-5",
			messagesDispatchMappedModel: "gpt-5.6-sol",
			expectedModel:               "gpt-5.4",
	REDACTED,
		{
			name: "account passthrough mapping overrides messages dispatch model",
			account: &Account{
		REDACTED
					"model_mapping": map[string]any{
						"claude-fable-5": "claude-fable-5",
				REDACTED,
			REDACTED,
		REDACTED,
			requestedModel:              "claude-fable-5",
			messagesDispatchMappedModel: "gpt-5.6-sol",
			expectedModel:               "claude-fable-5",
	REDACTED,
		{
			name: "ordinary codex spark request keeps requested model",
			account: &Account{
		REDACTEDREDACTED,
		REDACTED,
			requestedModel: "gpt-5.3-codex-spark",
			expectedModel:  "gpt-5.3-codex-spark",
	REDACTED,
		{
			name: "ordinary gpt-5.5 request keeps requested model",
			account: &Account{
		REDACTEDREDACTED,
		REDACTED,
			requestedModel: "gpt-5.5",
			expectedModel:  "gpt-5.5",
	REDACTED,
		{
			name: "ordinary gpt-5.5-pro request keeps requested model",
			account: &Account{
		REDACTEDREDACTED,
		REDACTED,
			requestedModel: "gpt-5.5-pro",
			expectedModel:  "gpt-5.5-pro",
	REDACTED,
		{
			name: "ordinary compact-spelled gpt5.5 request keeps requested model",
			account: &Account{
		REDACTEDREDACTED,
		REDACTED,
			requestedModel: "gpt5.5",
			expectedModel:  "gpt5.5",
	REDACTED,
		{
			name: "ordinary namespaced gpt-5.5 request keeps requested model",
			account: &Account{
		REDACTEDREDACTED,
		REDACTED,
			requestedModel: "openai/gpt-5.5",
			expectedModel:  "openai/gpt-5.5",
	REDACTED,
		{
			name: "ordinary compact gpt-5.5 request keeps requested model",
			account: &Account{
		REDACTEDREDACTED,
		REDACTED,
			requestedModel: "gpt-5.5-openai-compact",
			expectedModel:  "gpt-5.5-openai-compact",
	REDACTED,
		{
			name: "whitespace-only messages dispatch model is ignored",
			account: &Account{
		REDACTEDREDACTED,
		REDACTED,
			requestedModel:              "gpt-5.5",
			messagesDispatchMappedModel: "  ",
			expectedModel:               "gpt-5.5",
	REDACTED,
REDACTED

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := resolveOpenAIForwardModel(tt.account, tt.requestedModel, tt.messagesDispatchMappedModel); got != tt.expectedModel {
				t.Fatalf("resolveOpenAIForwardModel(...) = %q, want %q", got, tt.expectedModel)
		REDACTED
	REDACTED)
REDACTED
REDACTED

func TestResolveOpenAICompactForwardModel(t *testing.T) {
	tests := []struct {
		name          string
		account       *Account
		model         string
		expectedModel string
REDACTED{
		{
			name:          "nil account keeps original model",
			account:       nil,
			model:         "gpt-5.4",
			expectedModel: "gpt-5.4",
	REDACTED,
		{
			name: "missing compact mapping keeps original model",
			account: &Account{
		REDACTEDREDACTED,
		REDACTED,
			model:         "gpt-5.4",
			expectedModel: "gpt-5.4",
	REDACTED,
		{
			name: "exact compact mapping overrides model",
			account: &Account{
		REDACTED
					"compact_model_mapping": map[string]any{
						"gpt-5.4": "gpt-5.4-openai-compact",
				REDACTED,
			REDACTED,
		REDACTED,
			model:         "gpt-5.4",
			expectedModel: "gpt-5.4-openai-compact",
	REDACTED,
		{
			name: "wildcard compact mapping overrides model",
			account: &Account{
		REDACTED
					"compact_model_mapping": map[string]any{
						"gpt-5.*": "gpt-5-openai-compact",
				REDACTED,
			REDACTED,
		REDACTED,
			model:         "gpt-5.4",
			expectedModel: "gpt-5-openai-compact",
	REDACTED,
		{
			name: "passthrough compact mapping remains unchanged",
			account: &Account{
		REDACTED
					"compact_model_mapping": map[string]any{
						"gpt-5.4": "gpt-5.4",
				REDACTED,
			REDACTED,
		REDACTED,
			model:         "gpt-5.4",
			expectedModel: "gpt-5.4",
	REDACTED,
REDACTED

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := resolveOpenAICompactForwardModel(tt.account, tt.model); got != tt.expectedModel {
				t.Fatalf("resolveOpenAICompactForwardModel(...) = %q, want %q", got, tt.expectedModel)
		REDACTED
	REDACTED)
REDACTED
REDACTED

func TestResolveOpenAIForwardMappedModels_CompactMappingPrecedence(t *testing.T) {
	conflictingMappings := map[string]any{
		"model_mapping":         map[string]any{"gpt-5.5": "gpt-5.4"REDACTED,
		"compact_model_mapping": map[string]any{"gpt-5.5": "gpt-5.5-openai-compact"REDACTED,
REDACTED
	mappedOnlyCompact := map[string]any{
		"model_mapping":         map[string]any{"gpt-5.5": "gpt-5.4"REDACTED,
		"compact_model_mapping": map[string]any{"gpt-5.4": "gpt-5.4-openai-compact"REDACTED,
REDACTED
	tests := []struct {
		name           string
		account        *Account
		requireCompact bool
		wantBilling    string
		wantUpstream   string
REDACTED{
		{
			name: "compact uses client-visible model before ordinary mapping",
			account: &Account{Platform: PlatformOpenAI, Type: AccountTypeOAuth,
				Credentials: conflictingMappingsREDACTED,
			requireCompact: true,
			wantBilling:    "gpt-5.4",
			wantUpstream:   "gpt-5.5-openai-compact",
	REDACTED,
		{
			name: "non-compact uses ordinary mapping",
			account: &Account{Platform: PlatformOpenAI, Type: AccountTypeOAuth,
				Credentials: conflictingMappingsREDACTED,
			wantBilling:  "gpt-5.4",
			wantUpstream: "gpt-5.4",
	REDACTED,
		{
			name: "compact falls back to ordinary mapped model",
			account: &Account{Platform: PlatformOpenAI, Type: AccountTypeOAuth,
				Credentials: mappedOnlyCompactREDACTED,
			requireCompact: true,
			wantBilling:    "gpt-5.4",
			wantUpstream:   "gpt-5.4-openai-compact",
	REDACTED,
		{
			name: "passthrough ignores ordinary mapping",
			account: &Account{Platform: PlatformOpenAI, Type: AccountTypeOAuth,
				Credentials: conflictingMappings, Extra: map[string]any{"openai_passthrough": trueREDACTEDREDACTED,
			requireCompact: true,
			wantBilling:    "gpt-5.5",
			wantUpstream:   "gpt-5.5-openai-compact",
	REDACTED,
		{
			name: "raw chat fallback never applies compact mapping",
			account: &Account{Platform: PlatformOpenAI, Type: AccountTypeAPIKey,
				Credentials: conflictingMappings, Extra: map[string]any{"openai_responses_supported": falseREDACTEDREDACTED,
			requireCompact: true,
			wantBilling:    "gpt-5.4",
			wantUpstream:   "gpt-5.4",
	REDACTED,
REDACTED

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			billing, upstream := resolveOpenAIForwardMappedModels(tt.account, "gpt-5.5", tt.requireCompact)
			if billing != tt.wantBilling {
				t.Fatalf("billing model = %q, want %q", billing, tt.wantBilling)
		REDACTED
			if upstream != tt.wantUpstream {
				t.Fatalf("upstream model = %q, want %q", upstream, tt.wantUpstream)
		REDACTED
			if scheduler := resolveOpenAIAccountUpstreamModelForRequest(tt.account, "gpt-5.5", tt.requireCompact); scheduler != upstream {
				t.Fatalf("scheduler model %q disagrees with Forward model %q", scheduler, upstream)
		REDACTED
	REDACTED)
REDACTED
REDACTED

func TestCanonicalOpenAIAccountSchedulingModelMatchesForwardSemantics(t *testing.T) {
	tests := []struct {
		name    string
		account *Account
		model   string
		want    string
REDACTED{
		{
			name:    "OpenAI OAuth applies Codex alias normalization",
			account: &Account{Platform: PlatformOpenAI, Type: AccountTypeOAuthREDACTED,
			model:   "gpt-5.6",
			want:    "gpt-5.6-sol",
	REDACTED,
		{
			name: "OpenAI passthrough ignores ordinary account mapping",
			account: &Account{Platform: PlatformOpenAI, Type: AccountTypeOAuth,
		REDACTED"model_mapping": map[string]any{"public": "private"REDACTEDREDACTED,
				Extra:       map[string]any{"openai_passthrough": trueREDACTEDREDACTED,
			model: "public",
			want:  "public",
	REDACTED,
		{
			name:    "Grok OAuth does not inherit OpenAI Codex aliases",
			account: &Account{Platform: PlatformGrok, Type: AccountTypeOAuthREDACTED,
			model:   "gpt-5.6",
			want:    "gpt-5.6",
	REDACTED,
REDACTED

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := canonicalOpenAIAccountSchedulingModel(tt.account, tt.model); got != tt.want {
				t.Fatalf("canonical scheduling model = %q, want %q", got, tt.want)
		REDACTED
	REDACTED)
REDACTED
REDACTED

func TestResolveOpenAIErrorSchedulingModelPrefersActualUpstreamModel(t *testing.T) {
	if got := resolveOpenAIErrorSchedulingModel("gpt-5.4", "gpt-5.5-openai-compact"); got != "gpt-5.5-openai-compact" {
		t.Fatalf("error scheduling model = %q, want compact upstream model", got)
REDACTED
	if got := resolveOpenAIErrorSchedulingModel("gpt-5.4", ""); got != "gpt-5.4" {
		t.Fatalf("empty upstream fallback = %q, want billing model", got)
REDACTED
REDACTED

func TestNormalizeCodexModel(t *testing.T) {
	cases := map[string]string{
		"gpt-5.3-codex-spark":       "gpt-5.3-codex-spark",
		"gpt-5.3-codex-spark-high":  "gpt-5.3-codex-spark",
		"gpt-5.3-codex-spark-xhigh": "gpt-5.3-codex-spark",
		"gpt-5.3":                   "gpt-5.3-codex",
		"gpt-image-2":               "gpt-image-2",
		"gpt-5.4-nano":              "gpt-5.4-nano",
		"gpt-5.4-nano-high":         "gpt-5.4-nano",
		"gpt6":                      "gpt6",
		"claude-opus-4-6":           "claude-opus-4-6",
REDACTED

	for input, expected := range cases {
		if got := normalizeCodexModel(input); got != expected {
			t.Fatalf("normalizeCodexModel(%q) = %q, want %q", input, got, expected)
	REDACTED
REDACTED
REDACTED

func TestNormalizeOpenAIModelForUpstream(t *testing.T) {
	tests := []struct {
		name    string
		account *Account
		model   string
		want    string
REDACTED{
		{
			name:    "oauth routes bare GPT-5.6 alias to Sol",
			account: &Account{Type: AccountTypeOAuthREDACTED,
			model:   "gpt-5.6",
			want:    "gpt-5.6-sol",
	REDACTED,
		{
			name:    "oauth routes provider-prefixed GPT-5.6 alias to Sol",
			account: &Account{Type: AccountTypeOAuthREDACTED,
			model:   "openai/gpt-5.6",
			want:    "gpt-5.6-sol",
	REDACTED,
		{
			name:    "oauth preserves unknown non codex model",
			account: &Account{Type: AccountTypeOAuthREDACTED,
			model:   "gemini-3-flash-preview",
			want:    "gemini-3-flash-preview",
	REDACTED,
		{
			name:    "oauth preserves invalid gpt model",
			account: &Account{Type: AccountTypeOAuthREDACTED,
			model:   "gpt6",
			want:    "gpt6",
	REDACTED,
		{
			name:    "oauth normalizes known codex alias",
			account: &Account{Type: AccountTypeOAuthREDACTED,
			model:   "gpt-5.4-high",
			want:    "gpt-5.4",
	REDACTED,
		{
			name:    "oauth preserves GPT-5.5 Pro model",
			account: &Account{Type: AccountTypeOAuthREDACTED,
			model:   "openai/gpt-5.5-pro",
			want:    "gpt-5.5-pro",
	REDACTED,
		{
			name:    "oauth preserves codex auto review model",
			account: &Account{Type: AccountTypeOAuthREDACTED,
			model:   "codex-auto-review",
			want:    "codex-auto-review",
	REDACTED,
		{
			name:    "apikey preserves official bare GPT-5.6 alias",
			account: &Account{Type: AccountTypeAPIKeyREDACTED,
			model:   "gpt-5.6",
			want:    "gpt-5.6",
	REDACTED,
		{
			name:    "apikey preserves custom compatible model",
			account: &Account{Type: AccountTypeAPIKeyREDACTED,
			model:   "gemini-3-flash-preview",
			want:    "gemini-3-flash-preview",
	REDACTED,
		{
			name:    "apikey preserves official non codex model",
			account: &Account{Type: AccountTypeAPIKeyREDACTED,
			model:   "gpt-4.1",
			want:    "gpt-4.1",
	REDACTED,
REDACTED

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := normalizeOpenAIModelForUpstream(tt.account, tt.model); got != tt.want {
				t.Fatalf("normalizeOpenAIModelForUpstream(...) = %q, want %q", got, tt.want)
		REDACTED
	REDACTED)
REDACTED
REDACTED

func TestUsageBillingModelCandidatesPreserveCodexAutoReviewModel(t *testing.T) {
	candidates := usageBillingModelCandidates("codex-auto-review")

	expected := []string{"codex-auto-review"REDACTED
	if len(candidates) != len(expected) {
		t.Fatalf("usageBillingModelCandidates(codex-auto-review) = %#v, want %#v", candidates, expected)
REDACTED
	for i := range expected {
		if candidates[i] != expected[i] {
			t.Fatalf("usageBillingModelCandidates(codex-auto-review) = %#v, want %#v", candidates, expected)
	REDACTED
REDACTED
REDACTED

func TestUsageBillingModelCandidatesPreserveGPT55ProModel(t *testing.T) {
	candidates := usageBillingModelCandidates("openai/gpt-5.5-pro")

	expected := []string{"openai/gpt-5.5-pro", "gpt-5.5-pro"REDACTED
	if len(candidates) != len(expected) {
		t.Fatalf("usageBillingModelCandidates(openai/gpt-5.5-pro) = %#v, want %#v", candidates, expected)
REDACTED
	for i := range expected {
		if candidates[i] != expected[i] {
			t.Fatalf("usageBillingModelCandidates(openai/gpt-5.5-pro) = %#v, want %#v", candidates, expected)
	REDACTED
REDACTED
REDACTED
