package config

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestLoadClientGeoPrivacy(t *testing.T) {
	for _, value := range []string{"", "true", "false"} {
		t.Run("env="+value, func(t *testing.T) {
			resetViperWithJWTSecret(t)
			t.Setenv("GATEWAY_REDACT_CLIENT_GEO_METADATA", value)
			cfg, err := Load()
			require.NoError(t, err)
			require.Equal(t, value == "true", cfg.Gateway.RedactClientGeoMetadata)
		})
	}
}
