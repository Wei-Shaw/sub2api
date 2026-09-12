package config

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestLoadOpenAIAPIKeyAvailability(t *testing.T) {
	for _, tc := range []struct {
		name, env string
		want      bool
	}{
		{name: "disabled by default"},
		{name: "enabled by environment", env: "true", want: true},
		{name: "explicitly disabled", env: "false"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			resetViperWithJWTSecret(t)
			t.Setenv("GATEWAY_OPENAI_APIKEY_AVAILABILITY_ENABLED", tc.env)
			cfg, err := Load()
			require.NoError(t, err)
			require.Equal(t, tc.want, cfg.Gateway.OpenAIAPIKeyAvailabilityEnabled)
		})
	}
}
