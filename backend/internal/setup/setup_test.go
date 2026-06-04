package setup

import (
	"os"
	"strings"
	"testing"
)

func TestDecideAdminBootstrap(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		totalUsers int64
		adminUsers int64
		should     bool
		reason     string
	}{
		{
			name:       "empty database should create admin",
			totalUsers: 0,
			adminUsers: 0,
			should:     true,
			reason:     adminBootstrapReasonEmptyDatabase,
		},
		{
			name:       "admin exists should skip",
			totalUsers: 10,
			adminUsers: 1,
			should:     false,
			reason:     adminBootstrapReasonAdminExists,
		},
		{
			name:       "users exist without admin should skip",
			totalUsers: 5,
			adminUsers: 0,
			should:     false,
			reason:     adminBootstrapReasonUsersExistWithoutAdmin,
		},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got := decideAdminBootstrap(tc.totalUsers, tc.adminUsers)
			if got.shouldCreate != tc.should {
				t.Fatalf("shouldCreate=%v, want %v", got.shouldCreate, tc.should)
			}
			if got.reason != tc.reason {
				t.Fatalf("reason=%q, want %q", got.reason, tc.reason)
			}
		})
	}
}

func TestSetupDefaultAdminConcurrency(t *testing.T) {
	t.Run("simple mode admin uses higher concurrency", func(t *testing.T) {
		t.Setenv("RUN_MODE", "simple")
		if got := setupDefaultAdminConcurrency(); got != simpleModeAdminConcurrency {
			t.Fatalf("setupDefaultAdminConcurrency()=%d, want %d", got, simpleModeAdminConcurrency)
		}
	})

	t.Run("standard mode keeps existing default", func(t *testing.T) {
		t.Setenv("RUN_MODE", "standard")
		if got := setupDefaultAdminConcurrency(); got != defaultUserConcurrency {
			t.Fatalf("setupDefaultAdminConcurrency()=%d, want %d", got, defaultUserConcurrency)
		}
	})
}

func TestWriteConfigFileKeepsDefaultUserConcurrency(t *testing.T) {
	t.Setenv("RUN_MODE", "simple")
	t.Setenv("DATA_DIR", t.TempDir())

	if err := writeConfigFile(&SetupConfig{}); err != nil {
		t.Fatalf("writeConfigFile() error = %v", err)
	}

	data, err := os.ReadFile(GetConfigFilePath())
	if err != nil {
		t.Fatalf("ReadFile() error = %v", err)
	}

	if !strings.Contains(string(data), "user_concurrency: 5") {
		t.Fatalf("config missing default user concurrency, got:\n%s", string(data))
	}
}

func TestNeedsSetupIgnoresExistingConfigWithoutInstallLock(t *testing.T) {
	t.Setenv("DATA_DIR", t.TempDir())

	if err := os.WriteFile(GetConfigFilePath(), []byte("server:\n  port: 8080\n"), 0o600); err != nil {
		t.Fatalf("WriteFile(config) error = %v", err)
	}

	if !NeedsSetup() {
		t.Fatalf("NeedsSetup() = false, want true when only config exists")
	}
}

func TestNeedsSetupFalseWhenInstallLockExists(t *testing.T) {
	t.Setenv("DATA_DIR", t.TempDir())

	if err := os.WriteFile(GetInstallLockPath(), []byte("installed_at=2026-04-15T00:00:00Z\n"), 0o400); err != nil {
		t.Fatalf("WriteFile(lock) error = %v", err)
	}

	if NeedsSetup() {
		t.Fatalf("NeedsSetup() = true, want false when install lock exists")
	}
}

func TestResolveInstallServerConfigUsesBackendBootstrapPort(t *testing.T) {
	t.Setenv("SERVER_HOST", "127.0.0.1")
	t.Setenv("SERVER_PORT", "9090")

	got := resolveInstallServerConfig(ServerConfig{
		Host: "0.0.0.0",
		Port: 443,
		Mode: "release",
	})

	if got.Host != "127.0.0.1" {
		t.Fatalf("Host = %q, want %q", got.Host, "127.0.0.1")
	}
	if got.Port != 9090 {
		t.Fatalf("Port = %d, want %d", got.Port, 9090)
	}
	if got.Mode != "release" {
		t.Fatalf("Mode = %q, want %q", got.Mode, "release")
	}
}
