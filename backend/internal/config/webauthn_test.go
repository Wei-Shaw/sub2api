package config

import (
	"os"
	"path/filepath"
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
	}{
		{
			name: "valid production origin",
			configure: func(cfg *Config) {
				cfg.WebAuthn = WebAuthnConfig{
					Enabled:       true,
					RPDisplayName: "Sub2API",
					RPID:          "sub2api.example.com",
					RPOrigins:     []string{"https://sub2api.example.com"},
				}
			},
		},
		{
			name: "valid localhost development origin",
			configure: func(cfg *Config) {
				cfg.WebAuthn = WebAuthnConfig{
					Enabled:       true,
					RPDisplayName: "Sub2API Dev",
					RPID:          "localhost",
					RPOrigins:     []string{"http://localhost:5173"},
				}
			},
		},
		{
			name: "missing relying party id",
			configure: func(cfg *Config) {
				cfg.WebAuthn = WebAuthnConfig{
					Enabled:       true,
					RPDisplayName: "Sub2API",
					RPOrigins:     []string{"https://sub2api.example.com"},
				}
			},
			wantError: "webauthn.rp_id",
		},
		{
			name: "relying party id contains scheme",
			configure: func(cfg *Config) {
				cfg.WebAuthn = WebAuthnConfig{
					Enabled:       true,
					RPDisplayName: "Sub2API",
					RPID:          "https://sub2api.example.com",
					RPOrigins:     []string{"https://sub2api.example.com"},
				}
			},
			wantError: "domain without scheme",
		},
		{
			name: "non-local insecure origin",
			configure: func(cfg *Config) {
				cfg.WebAuthn = WebAuthnConfig{
					Enabled:       true,
					RPDisplayName: "Sub2API",
					RPID:          "sub2api.example.com",
					RPOrigins:     []string{"http://sub2api.example.com"},
				}
			},
			wantError: "must use HTTPS",
		},
		{
			name: "origin outside relying party id",
			configure: func(cfg *Config) {
				cfg.WebAuthn = WebAuthnConfig{
					Enabled:       true,
					RPDisplayName: "Sub2API",
					RPID:          "example.com",
					RPOrigins:     []string{"https://example.net"},
				}
			},
			wantError: "not within relying party ID",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			viper.Reset()
			t.Setenv("JWT_SECRET", strings.Repeat("x", 32))
			cfg, err := Load()
			require.NoError(t, err)
			tt.configure(cfg)

			err = cfg.Validate()
			if tt.wantError == "" {
				require.NoError(t, err)
			} else {
				require.ErrorContains(t, err, tt.wantError)
			}
		})
	}
}

func TestLoadWebAuthnConfigFromEnvironment(t *testing.T) {
	viper.Reset()
	t.Cleanup(viper.Reset)
	t.Setenv("JWT_SECRET", strings.Repeat("x", 32))
	t.Setenv("WEBAUTHN_ENABLED", "true")
	t.Setenv("WEBAUTHN_RP_DISPLAY_NAME", "TokenSupply")
	t.Setenv("WEBAUTHN_RP_ID", "tokensupply.net")
	t.Setenv("WEBAUTHN_RP_ORIGINS", " https://www.tokensupply.net,https://tokensupply.net ")

	cfg, err := Load()
	require.NoError(t, err)
	require.Equal(t, WebAuthnConfig{
		Enabled:       true,
		RPDisplayName: "TokenSupply",
		RPID:          "tokensupply.net",
		RPOrigins:     []string{"https://www.tokensupply.net", "https://tokensupply.net"},
	}, cfg.WebAuthn)
}

func TestLoadWebAuthnConfigRejectsEmptyEnvironmentOrigins(t *testing.T) {
	viper.Reset()
	t.Cleanup(viper.Reset)
	t.Setenv("JWT_SECRET", strings.Repeat("x", 32))
	t.Setenv("WEBAUTHN_ENABLED", "true")
	t.Setenv("WEBAUTHN_RP_ID", "tokensupply.net")
	t.Setenv("WEBAUTHN_RP_ORIGINS", " , ")

	_, err := Load()
	require.ErrorContains(t, err, "webauthn.rp_origins")
}

func TestLoadWebAuthnConfigFromFileWhenEnvironmentIsUnset(t *testing.T) {
	viper.Reset()
	t.Cleanup(viper.Reset)
	for _, name := range []string{
		"WEBAUTHN_ENABLED",
		"WEBAUTHN_RP_DISPLAY_NAME",
		"WEBAUTHN_RP_ID",
		"WEBAUTHN_RP_ORIGINS",
	} {
		value, existed := os.LookupEnv(name)
		require.NoError(t, os.Unsetenv(name))
		t.Cleanup(func() {
			if existed {
				_ = os.Setenv(name, value)
			} else {
				_ = os.Unsetenv(name)
			}
		})
	}

	configFile := filepath.Join(t.TempDir(), "config.yaml")
	require.NoError(t, os.WriteFile(configFile, []byte(`
webauthn:
  enabled: true
  rp_display_name: Existing Deployment
  rp_id: existing.example.com
  rp_origins:
    - https://existing.example.com
`), 0o600))
	t.Setenv("CONFIG_FILE", configFile)
	t.Setenv("JWT_SECRET", strings.Repeat("x", 32))

	cfg, err := Load()
	require.NoError(t, err)
	require.Equal(t, WebAuthnConfig{
		Enabled:       true,
		RPDisplayName: "Existing Deployment",
		RPID:          "existing.example.com",
		RPOrigins:     []string{"https://existing.example.com"},
	}, cfg.WebAuthn)
}
