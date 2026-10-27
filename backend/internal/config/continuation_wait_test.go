package config

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestContinuationMaxWaitingConfig(t *testing.T) {
	for _, tc := range []struct {
		name, env string
		fileValue int
		want      int
	}{
		{name: "default", want: 100},
		{name: "yaml", fileValue: 7, want: 7},
		{name: "environment overrides yaml", fileValue: 7, env: "150", want: 150},
		{name: "zero", env: "0"},
		{name: "negative", env: "-1"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			resetViperWithJWTSecret(t)
			t.Setenv("GATEWAY_SCHEDULING_CONTINUATION_MAX_WAITING", tc.env)
			if tc.fileValue != 0 {
				path := filepath.Join(t.TempDir(), "config.yaml")
				body := fmt.Sprintf("gateway:\n  scheduling:\n    continuation_max_waiting: %d\n", tc.fileValue)
				require.NoError(t, os.WriteFile(path, []byte(body), 0o600))
				t.Setenv("CONFIG_FILE", path)
			}
			cfg, err := Load()
			if tc.want == 0 {
				require.ErrorContains(t, err, "gateway.scheduling.continuation_max_waiting must be positive")
				return
			}
			require.NoError(t, err)
			require.Equal(t, tc.want, cfg.Gateway.Scheduling.ContinuationMaxWaiting)
			require.Equal(t, 3, cfg.Gateway.Scheduling.StickySessionMaxWaiting)
		})
	}
}
