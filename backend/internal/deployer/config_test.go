package deployer

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestConfigDefaultsManagedBackupSettings(t *testing.T) {
	cfg := testConfig(t, 19080)
	cfg.BackupRootPath = ""
	cfg.BackupDatabaseService = ""
	cfg.BackupApplicationConfigPath = ""
	cfg.BackupDockerConfigPath = ""
	cfg.BackupDeployerBinaryPath = ""
	cfg.BackupTimeout = Duration{}
	cfg.LoadedFrom = filepath.Join(filepath.Dir(cfg.StatePath), "config.json")
	cfg.applyDefaults()

	if cfg.BackupRootPath != filepath.Join(filepath.Dir(cfg.StatePath), "backups") {
		t.Fatalf("backup root=%q", cfg.BackupRootPath)
	}
	if cfg.BackupDatabaseService != "postgres" {
		t.Fatalf("database service=%q", cfg.BackupDatabaseService)
	}
	if cfg.BackupApplicationConfigPath != filepath.Join(cfg.ComposeWorkDir, "data", "config.yaml") {
		t.Fatalf("application config=%q", cfg.BackupApplicationConfigPath)
	}
	if cfg.BackupDockerConfigPath != filepath.Join(filepath.Dir(cfg.LoadedFrom), "docker", "config.json") {
		t.Fatalf("docker config=%q", cfg.BackupDockerConfigPath)
	}
	if !filepath.IsAbs(cfg.BackupDeployerBinaryPath) {
		t.Fatalf("deployer binary=%q", cfg.BackupDeployerBinaryPath)
	}
	if cfg.BackupTimeout.Duration != 30*time.Minute {
		t.Fatalf("backup timeout=%s", cfg.BackupTimeout.Duration)
	}
	if _, err := os.Stat(cfg.BackupDeployerBinaryPath); err != nil {
		t.Fatalf("default deployer binary is not readable: %v", err)
	}
}

func TestConfigRejectsInvalidManagedBackupSettings(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(*Config)
		want   string
	}{
		{name: "relative root", mutate: func(cfg *Config) { cfg.BackupRootPath = "backups" }, want: "backup_root_path"},
		{name: "unsafe service", mutate: func(cfg *Config) { cfg.BackupDatabaseService = "postgres;rm" }, want: "backup_database_service"},
		{name: "relative application config", mutate: func(cfg *Config) { cfg.BackupApplicationConfigPath = "data/config.yaml" }, want: "backup_application_config_path"},
		{name: "relative docker config", mutate: func(cfg *Config) { cfg.BackupDockerConfigPath = "docker/config.json" }, want: "backup_docker_config_path"},
		{name: "relative binary", mutate: func(cfg *Config) { cfg.BackupDeployerBinaryPath = "sub2api-deployer" }, want: "backup_deployer_binary_path"},
		{name: "short timeout", mutate: func(cfg *Config) { cfg.BackupTimeout = Duration{Duration: 59 * time.Second} }, want: "backup_timeout"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			cfg := testConfig(t, 19080)
			test.mutate(&cfg)
			if err := cfg.validate(); err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("validation error=%v", err)
			}
		})
	}
}

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

func TestConfigRequiresCompleteControlPlaneUpgradeConfiguration(t *testing.T) {
	cfg := testConfig(t, 19081)
	cfg.HealthTimeout = Duration{Duration: 12 * time.Minute}
	cfg.ControlPlaneUpgradePath = filepath.Join(filepath.Dir(cfg.StatePath), "upgrade.json")
	cfg.ControlPlaneUpgradeCommand = nil
	if err := cfg.validate(); err == nil {
		t.Fatal("upgrade request path without command unexpectedly passed validation")
	}

	cfg.ControlPlaneUpgradeCommand = []string{"/bin/systemctl", "start", "--no-block", "sub2api-deployer-upgrade.service"}
	if err := cfg.validate(); err != nil {
		t.Fatalf("complete control-plane upgrade configuration was rejected: %v", err)
	}
}

func TestConfigRequiresControlPlaneStateInOneDirectory(t *testing.T) {
	cfg := testConfig(t, 19081)
	cfg.HealthTimeout = Duration{Duration: 12 * time.Minute}
	cfg.ControlPlaneUpgradePath = filepath.Join(t.TempDir(), "upgrade.json")
	if err := cfg.validate(); err == nil || err.Error() != "control_plane_upgrade_path must use the same directory as state_path" {
		t.Fatalf("split control-plane state directories were not rejected: %v", err)
	}
}
