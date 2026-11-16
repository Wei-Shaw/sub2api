//go:build unit

package config

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestLoadOpenAITransportErrorCooldown(t *testing.T) {
	for _, testCase := range []struct {
		name     string
		yaml     string
		env      string
		disabled bool
	}{
		{name: "default"},
		{name: "env_true", env: "true", disabled: true},
		{name: "env_false", env: "false"},
		{name: "yaml_true", yaml: "true", disabled: true},
		{name: "env_overrides_yaml", yaml: "true", env: "false"},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			resetViperWithJWTSecret(t)
			t.Chdir(t.TempDir())
			t.Setenv("GATEWAY_DISABLE_OPENAI_TRANSPORT_ERROR_COOLDOWN", testCase.env)
			if testCase.yaml != "" {
				configPath := filepath.Join(t.TempDir(), "config.yaml")
				require.NoError(t, os.WriteFile(configPath, []byte("gateway:\n  disable_openai_transport_error_cooldown: "+testCase.yaml+"\n"), 0600))
				t.Setenv("CONFIG_FILE", configPath)
			}
			cfg, err := Load()
			require.NoError(t, err)
			require.Equal(t, testCase.disabled, cfg.Gateway.DisableOpenAITransportErrorCooldown)
		})
	}
}
