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

func TestConfigDefaultsRouteConfirmationTimeout(t *testing.T) {
	cfg := testConfig(t, 19081)
	cfg.RouteConfirmationTimeout = Duration{}
	cfg.applyDefaults()
	if cfg.RouteConfirmationTimeout.Duration != 10*time.Second {
		t.Fatalf("route confirmation timeout=%s", cfg.RouteConfirmationTimeout.Duration)
	}
}

func TestConfigRejectsHealthTimeoutBelowMigrationBudget(t *testing.T) {
	cfg := testConfig(t, 19081)
	cfg.RouteConfirmationTimeout = Duration{Duration: time.Second}
	cfg.HealthTimeout = Duration{Duration: 9*time.Minute + 59*time.Second}
	if err := cfg.validate(); err == nil {
		t.Fatal("health timeout below the migration budget unexpectedly passed validation")
	}
}
