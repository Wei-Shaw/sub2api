package config

import (
	"strings"
	"testing"

	"github.com/spf13/viper"
	"github.com/stretchr/testify/require"
)

func TestValidateWebAuthnConfig(t *testing.T) {
	tests := []struct {
		name      string
		configure func(*Config)
		wantError string
REDACTED{
		{
			name: "valid production origin",
			configure: func(cfg *Config) {
				cfg.WebAuthn = WebAuthnConfig{
					Enabled:       true,
					RPDisplayName: "Sub2API",
					RPID:          "sub2api.example.com",
					RPOrigins:     []string{"https://sub2api.example.com"REDACTED,
			REDACTED
		REDACTED,
	REDACTED,
		{
			name: "valid localhost development origin",
			configure: func(cfg *Config) {
				cfg.WebAuthn = WebAuthnConfig{
					Enabled:       true,
					RPDisplayName: "Sub2API Dev",
					RPID:          "localhost",
					RPOrigins:     []string{"http://localhost:5173"REDACTED,
			REDACTED
		REDACTED,
	REDACTED,
		{
			name: "missing relying party id",
			configure: func(cfg *Config) {
				cfg.WebAuthn = WebAuthnConfig{
					Enabled:       true,
					RPDisplayName: "Sub2API",
					RPOrigins:     []string{"https://sub2api.example.com"REDACTED,
			REDACTED
		REDACTED,
			wantError: "webauthn.rp_id",
	REDACTED,
		{
			name: "relying party id contains scheme",
			configure: func(cfg *Config) {
				cfg.WebAuthn = WebAuthnConfig{
					Enabled:       true,
					RPDisplayName: "Sub2API",
					RPID:          "https://sub2api.example.com",
					RPOrigins:     []string{"https://sub2api.example.com"REDACTED,
			REDACTED
		REDACTED,
			wantError: "domain without scheme",
	REDACTED,
		{
			name: "non-local insecure origin",
			configure: func(cfg *Config) {
				cfg.WebAuthn = WebAuthnConfig{
					Enabled:       true,
					RPDisplayName: "Sub2API",
					RPID:          "sub2api.example.com",
					RPOrigins:     []string{"http://sub2api.example.com"REDACTED,
			REDACTED
		REDACTED,
			wantError: "must use HTTPS",
	REDACTED,
		{
			name: "origin outside relying party id",
			configure: func(cfg *Config) {
				cfg.WebAuthn = WebAuthnConfig{
					Enabled:       true,
					RPDisplayName: "Sub2API",
					RPID:          "example.com",
					RPOrigins:     []string{"https://example.net"REDACTED,
			REDACTED
		REDACTED,
			wantError: "not within relying party ID",
	REDACTED,
REDACTED

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			viper.Reset()
			t.Setenv("JWT_SECRET", strings.Repeat("x", 32))
			cfg, err := Load()
		REDACTED
			tt.configure(cfg)

			err = cfg.Validate()
			if tt.wantError == "" {
			REDACTED
		REDACTED else {
				require.ErrorContains(t, err, tt.wantError)
		REDACTED
	REDACTED)
REDACTED
REDACTED
