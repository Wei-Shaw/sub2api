package config

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestResponsesCompactionBodyLimitConfig(t *testing.T) {
	for _, tc := range []struct {
		value   string
		want    int64
		invalid bool
	}{
		{"0", 0, false},
		{"2097152", 2097152, false},
		{"-1", 0, true},
		{"268435457", 0, true},
	} {
		t.Run(tc.value, func(t *testing.T) {
			resetViperWithJWTSecret(t)
			t.Setenv("GATEWAY_RESPONSES_COMPACTION_BODY_LIMIT", tc.value)
			t.Setenv("GATEWAY_MAX_BODY_SIZE", "268435456")
			cfg, err := Load()
			if tc.invalid {
				require.ErrorContains(t, err, "responses_compaction_body_limit")
				return
			}
			require.NoError(t, err)
			require.Equal(t, tc.want, cfg.Gateway.ResponsesCompactionBodyLimit)
		})
	}
}
