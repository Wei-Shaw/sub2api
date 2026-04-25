package service

import "testing"

func TestAccountGetOpenAICompactMode(t *testing.T) {
	tests := []struct {
		name    string
		account *Account
		want    string
REDACTED{
		{
			name: "nil account defaults to auto",
			want: OpenAICompactModeAuto,
	REDACTED,
		{
			name: "non openai account defaults to auto",
			account: &Account{
				Platform: PlatformAnthropic,
				Extra:    map[string]any{"openai_compact_mode": OpenAICompactModeForceOnREDACTED,
		REDACTED,
			want: OpenAICompactModeAuto,
	REDACTED,
		{
			name: "missing extra defaults to auto",
			account: &Account{
				Platform: PlatformOpenAI,
		REDACTED,
			want: OpenAICompactModeAuto,
	REDACTED,
		{
			name: "invalid mode falls back to auto",
			account: &Account{
				Platform: PlatformOpenAI,
				Extra:    map[string]any{"openai_compact_mode": "  invalid  "REDACTED,
		REDACTED,
			want: OpenAICompactModeAuto,
	REDACTED,
		{
			name: "force on is normalized",
			account: &Account{
				Platform: PlatformOpenAI,
				Extra:    map[string]any{"openai_compact_mode": " FORCE_ON "REDACTED,
		REDACTED,
			want: OpenAICompactModeForceOn,
	REDACTED,
		{
			name: "force off is normalized",
			account: &Account{
				Platform: PlatformOpenAI,
				Extra:    map[string]any{"openai_compact_mode": "force_off"REDACTED,
		REDACTED,
			want: OpenAICompactModeForceOff,
	REDACTED,
REDACTED

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.account.GetOpenAICompactMode(); got != tt.want {
				t.Fatalf("GetOpenAICompactMode() = %q, want %q", got, tt.want)
		REDACTED
	REDACTED)
REDACTED
REDACTED

func TestAccountOpenAICompactSupportKnown(t *testing.T) {
	tests := []struct {
		name          string
		account       *Account
		wantSupported bool
		wantKnown     bool
REDACTED{
		{
			name:          "nil account is unknown",
			wantSupported: false,
			wantKnown:     false,
	REDACTED,
		{
			name: "non openai account is unknown",
			account: &Account{
				Platform: PlatformAnthropic,
				Extra:    map[string]any{"openai_compact_supported": trueREDACTED,
		REDACTED,
			wantSupported: false,
			wantKnown:     false,
	REDACTED,
		{
			name: "force on overrides probe state",
			account: &Account{
				Platform: PlatformOpenAI,
				Extra: map[string]any{
					"openai_compact_mode":      OpenAICompactModeForceOn,
					"openai_compact_supported": false,
			REDACTED,
		REDACTED,
			wantSupported: true,
			wantKnown:     true,
	REDACTED,
		{
			name: "force off overrides probe state",
			account: &Account{
				Platform: PlatformOpenAI,
				Extra: map[string]any{
					"openai_compact_mode":      OpenAICompactModeForceOff,
					"openai_compact_supported": true,
			REDACTED,
		REDACTED,
			wantSupported: false,
			wantKnown:     true,
	REDACTED,
		{
			name: "auto true is known supported",
			account: &Account{
				Platform: PlatformOpenAI,
				Extra:    map[string]any{"openai_compact_supported": trueREDACTED,
		REDACTED,
			wantSupported: true,
			wantKnown:     true,
	REDACTED,
		{
			name: "auto false is known unsupported",
			account: &Account{
				Platform: PlatformOpenAI,
				Extra:    map[string]any{"openai_compact_supported": falseREDACTED,
		REDACTED,
			wantSupported: false,
			wantKnown:     true,
	REDACTED,
		{
			name: "auto without probe state remains unknown",
			account: &Account{
				Platform: PlatformOpenAI,
				Extra:    map[string]any{REDACTED,
		REDACTED,
			wantSupported: false,
			wantKnown:     false,
	REDACTED,
		{
			name: "invalid probe field remains unknown",
			account: &Account{
				Platform: PlatformOpenAI,
				Extra:    map[string]any{"openai_compact_supported": "true"REDACTED,
		REDACTED,
			wantSupported: false,
			wantKnown:     false,
	REDACTED,
REDACTED

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotSupported, gotKnown := tt.account.OpenAICompactSupportKnown()
			if gotSupported != tt.wantSupported || gotKnown != tt.wantKnown {
				t.Fatalf("OpenAICompactSupportKnown() = (%v, %v), want (%v, %v)", gotSupported, gotKnown, tt.wantSupported, tt.wantKnown)
		REDACTED
	REDACTED)
REDACTED
REDACTED

func TestAccountAllowsOpenAICompact(t *testing.T) {
	tests := []struct {
		name    string
		account *Account
		want    bool
REDACTED{
		{
			name: "nil account does not allow compact",
			want: false,
	REDACTED,
		{
			name: "non openai account does not allow compact",
			account: &Account{
				Platform: PlatformAnthropic,
		REDACTED,
			want: false,
	REDACTED,
		{
			name: "unknown openai account remains allowed",
			account: &Account{
				Platform: PlatformOpenAI,
				Extra:    map[string]any{REDACTED,
		REDACTED,
			want: true,
	REDACTED,
		{
			name: "supported openai account is allowed",
			account: &Account{
				Platform: PlatformOpenAI,
				Extra:    map[string]any{"openai_compact_supported": trueREDACTED,
		REDACTED,
			want: true,
	REDACTED,
		{
			name: "unsupported openai account is rejected",
			account: &Account{
				Platform: PlatformOpenAI,
				Extra:    map[string]any{"openai_compact_supported": falseREDACTED,
		REDACTED,
			want: false,
	REDACTED,
		{
			name: "force on is allowed",
			account: &Account{
				Platform: PlatformOpenAI,
				Extra:    map[string]any{"openai_compact_mode": OpenAICompactModeForceOnREDACTED,
		REDACTED,
			want: true,
	REDACTED,
		{
			name: "force off is rejected",
			account: &Account{
				Platform: PlatformOpenAI,
				Extra:    map[string]any{"openai_compact_mode": OpenAICompactModeForceOffREDACTED,
		REDACTED,
			want: false,
	REDACTED,
REDACTED

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.account.AllowsOpenAICompact(); got != tt.want {
				t.Fatalf("AllowsOpenAICompact() = %v, want %v", got, tt.want)
		REDACTED
	REDACTED)
REDACTED
REDACTED

func TestAccountGetCompactModelMapping(t *testing.T) {
	tests := []struct {
		name    string
		account *Account
		want    map[string]string
REDACTED{
		{
			name: "nil account returns nil",
			want: nil,
	REDACTED,
		{
			name: "missing credentials returns nil",
			account: &Account{
				Platform: PlatformOpenAI,
		REDACTED,
			want: nil,
	REDACTED,
		{
			name: "map any is converted",
			account: &Account{
		REDACTED
					"compact_model_mapping": map[string]any{
						"gpt-5.4": "gpt-5.4-openai-compact",
						"invalid": 1,
				REDACTED,
			REDACTED,
		REDACTED,
			want: map[string]string{
				"gpt-5.4": "gpt-5.4-openai-compact",
		REDACTED,
	REDACTED,
		{
			name: "map string string is copied",
			account: &Account{
		REDACTED
					"compact_model_mapping": map[string]string{
						"gpt-*": "compact-*",
				REDACTED,
			REDACTED,
		REDACTED,
			want: map[string]string{
				"gpt-*": "compact-*",
		REDACTED,
	REDACTED,
REDACTED

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.account.GetCompactModelMapping()
			if !equalStringMap(got, tt.want) {
				t.Fatalf("GetCompactModelMapping() = %#v, want %#v", got, tt.want)
		REDACTED
	REDACTED)
REDACTED
REDACTED

func TestAccountResolveCompactMappedModel(t *testing.T) {
	tests := []struct {
		name           string
		credentials    map[string]any
		requestedModel string
		expectedModel  string
		expectedMatch  bool
REDACTED{
		{
			name:           "no compact mapping reports unmatched",
			credentials:    nil,
			requestedModel: "gpt-5.4",
			expectedModel:  "gpt-5.4",
			expectedMatch:  false,
	REDACTED,
		{
			name: "exact compact mapping matches",
			credentials: map[string]any{
				"compact_model_mapping": map[string]any{
					"gpt-5.4": "gpt-5.4-openai-compact",
			REDACTED,
		REDACTED,
			requestedModel: "gpt-5.4",
			expectedModel:  "gpt-5.4-openai-compact",
			expectedMatch:  true,
	REDACTED,
		{
			name: "exact passthrough counts as match",
			credentials: map[string]any{
				"compact_model_mapping": map[string]any{
					"gpt-5.4": "gpt-5.4",
			REDACTED,
		REDACTED,
			requestedModel: "gpt-5.4",
			expectedModel:  "gpt-5.4",
			expectedMatch:  true,
	REDACTED,
		{
			name: "longest wildcard wins",
			credentials: map[string]any{
				"compact_model_mapping": map[string]any{
					"gpt-*":         "fallback-compact",
					"gpt-5.4*":      "gpt-5.4-openai-compact",
					"gpt-5.4-mini*": "gpt-5.4-mini-openai-compact",
			REDACTED,
		REDACTED,
			requestedModel: "gpt-5.4-mini",
			expectedModel:  "gpt-5.4-mini-openai-compact",
			expectedMatch:  true,
	REDACTED,
		{
			name: "missing compact mapping reports unmatched",
			credentials: map[string]any{
				"compact_model_mapping": map[string]any{
					"gpt-5.3": "gpt-5.3-openai-compact",
			REDACTED,
		REDACTED,
			requestedModel: "gpt-5.4",
			expectedModel:  "gpt-5.4",
			expectedMatch:  false,
	REDACTED,
REDACTED

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			account := &Account{
				Platform:    PlatformOpenAI,
				Credentials: tt.credentials,
		REDACTED
			gotModel, gotMatch := account.ResolveCompactMappedModel(tt.requestedModel)
			if gotModel != tt.expectedModel || gotMatch != tt.expectedMatch {
				t.Fatalf("ResolveCompactMappedModel(%q) = (%q, %v), want (%q, %v)", tt.requestedModel, gotModel, gotMatch, tt.expectedModel, tt.expectedMatch)
		REDACTED
	REDACTED)
REDACTED
REDACTED

func equalStringMap(left, right map[string]string) bool {
	if len(left) != len(right) {
		return false
REDACTED
	for key, want := range right {
		if got, ok := left[key]; !ok || got != want {
			return false
	REDACTED
REDACTED
	return true
REDACTED
