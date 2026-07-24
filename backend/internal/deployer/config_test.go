package deployer

import (
	"testing"
	"time"
)

func TestConfigRejectsNegativeDeploymentPhaseDurations(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		mutate func(*Config)
	}{
		{
			name: "stabilize duration",
			mutate: func(cfg *Config) {
				cfg.StabilizeDuration = Duration{Duration: -time.Second}
			},
		},
		{
			name: "drain duration",
			mutate: func(cfg *Config) {
				cfg.DrainDuration = Duration{Duration: -time.Second}
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			cfg := testConfig(t, 19081)
			test.mutate(&cfg)
			if err := cfg.validate(); err == nil {
				t.Fatal("negative deployment duration unexpectedly passed validation")
			}
		})
	}
}

func TestConfigRejectsDrainQuietPeriodAtOrAboveTimeout(t *testing.T) {
	cfg := testConfig(t, 19081)
	cfg.DrainDuration = Duration{Duration: time.Minute}
	cfg.DrainTimeout = Duration{Duration: time.Minute}
	if err := cfg.validate(); err == nil {
		t.Fatal("drain quiet period equal to timeout unexpectedly passed validation")
	}
}
