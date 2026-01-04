package geminicli

import (
	"strings"
	"testing"
)

func TestEffectiveOAuthConfig_GoogleOne(t *testing.T) {
	tests := []struct {
		name         string
		input        OAuthConfig
		oauthType    string
		wantClientID string
		wantScopes   string
		wantErr      bool
REDACTED{
		{
			name:         "Google One with built-in client (empty config)",
			input:        OAuthConfig{REDACTED,
			oauthType:    "google_one",
			wantClientID: GeminiCLIOAuthClientID,
			wantScopes:   DefaultCodeAssistScopes,
			wantErr:      false,
	REDACTED,
		{
			name: "Google One with custom client",
			input: OAuthConfig{
				ClientID:     "custom-client-id",
				ClientSecret: "custom-client-secret",
		REDACTED,
			oauthType:    "google_one",
			wantClientID: "custom-client-id",
			wantScopes:   DefaultGoogleOneScopes,
			wantErr:      false,
	REDACTED,
		{
			name: "Google One with built-in client and custom scopes (should filter restricted scopes)",
			input: OAuthConfig{
				Scopes: "https://www.googleapis.com/auth/cloud-platform https://www.googleapis.com/auth/generative-language.retriever https://www.googleapis.com/auth/drive.readonly",
		REDACTED,
			oauthType:    "google_one",
			wantClientID: GeminiCLIOAuthClientID,
			wantScopes:   "https://www.googleapis.com/auth/cloud-platform",
			wantErr:      false,
	REDACTED,
		{
			name: "Google One with built-in client and only restricted scopes (should fallback to default)",
			input: OAuthConfig{
				Scopes: "https://www.googleapis.com/auth/generative-language.retriever https://www.googleapis.com/auth/drive.readonly",
		REDACTED,
			oauthType:    "google_one",
			wantClientID: GeminiCLIOAuthClientID,
			wantScopes:   DefaultCodeAssistScopes,
			wantErr:      false,
	REDACTED,
		{
			name:         "Code Assist with built-in client",
			input:        OAuthConfig{REDACTED,
			oauthType:    "code_assist",
			wantClientID: GeminiCLIOAuthClientID,
			wantScopes:   DefaultCodeAssistScopes,
			wantErr:      false,
	REDACTED,
REDACTED

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := EffectiveOAuthConfig(tt.input, tt.oauthType)
			if (err != nil) != tt.wantErr {
				t.Errorf("EffectiveOAuthConfig() error = %v, wantErr %v", err, tt.wantErr)
				return
		REDACTED
			if err != nil {
				return
		REDACTED
			if got.ClientID != tt.wantClientID {
				t.Errorf("EffectiveOAuthConfig() ClientID = %v, want %v", got.ClientID, tt.wantClientID)
		REDACTED
			if got.Scopes != tt.wantScopes {
				t.Errorf("EffectiveOAuthConfig() Scopes = %v, want %v", got.Scopes, tt.wantScopes)
		REDACTED
	REDACTED)
REDACTED
REDACTED

func TestEffectiveOAuthConfig_ScopeFiltering(t *testing.T) {
	// Test that Google One with built-in client filters out restricted scopes
	cfg, err := EffectiveOAuthConfig(OAuthConfig{
		Scopes: "https://www.googleapis.com/auth/cloud-platform https://www.googleapis.com/auth/userinfo.email https://www.googleapis.com/auth/generative-language.retriever https://www.googleapis.com/auth/drive.readonly https://www.googleapis.com/auth/userinfo.profile",
REDACTED, "google_one")

	if err != nil {
		t.Fatalf("EffectiveOAuthConfig() error = %v", err)
REDACTED

	// Should only contain cloud-platform, userinfo.email, and userinfo.profile
	// Should NOT contain generative-language or drive scopes
	if strings.Contains(cfg.Scopes, "generative-language") {
		t.Errorf("Scopes should not contain generative-language when using built-in client, got: %v", cfg.Scopes)
REDACTED
	if strings.Contains(cfg.Scopes, "drive") {
		t.Errorf("Scopes should not contain drive when using built-in client, got: %v", cfg.Scopes)
REDACTED
	if !strings.Contains(cfg.Scopes, "cloud-platform") {
		t.Errorf("Scopes should contain cloud-platform, got: %v", cfg.Scopes)
REDACTED
	if !strings.Contains(cfg.Scopes, "userinfo.email") {
		t.Errorf("Scopes should contain userinfo.email, got: %v", cfg.Scopes)
REDACTED
	if !strings.Contains(cfg.Scopes, "userinfo.profile") {
		t.Errorf("Scopes should contain userinfo.profile, got: %v", cfg.Scopes)
REDACTED
REDACTED
