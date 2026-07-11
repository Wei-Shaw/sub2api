package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

var deliveryConfigEnvNames = []string{
	"VIDEO_GATEWAY_ENCRYPTION_KEY",
	"VIDEO_GATEWAY_WORKER_ENABLED",
	"VIDEO_GATEWAY_POLL_INTERVAL_SECONDS",
	"VIDEO_GATEWAY_TASK_TIMEOUT_MINUTES",
	"VIDEO_GATEWAY_WORKER_BATCH_SIZE",
	"VIDEO_GATEWAY_MAX_POLL_ATTEMPTS",
	"VIDEO_GATEWAY_COST_PER_SECOND",
	"VIDEO_GATEWAY_PER_CALL_BUDGET",
	"RELIABILITY_CORE_VIDEO_ENABLED",
	"RELIABILITY_CORE_RESERVATION_TTL_HOURS",
	"RELIABILITY_CORE_RESERVATION_REAP_INTERVAL_SECONDS",
	"RELIABILITY_CORE_OUTBOX_POLL_INTERVAL_SECONDS",
	"RELIABILITY_CORE_OUTBOX_CLAIM_BATCH_SIZE",
	"RELIABILITY_CORE_OUTBOX_LEASE_SECONDS",
	"RELIABILITY_CORE_OUTBOX_MAX_ATTEMPTS",
	"RELIABILITY_CORE_OUTBOX_RETRY_BACKOFF_SECONDS",
}

func TestDeliveryConfigContractCoversMainlineComposeFiles(t *testing.T) {
	repoRoot := filepath.Clean(filepath.Join("..", "..", ".."))
	for _, relative := range []string{
		"deploy/docker-compose.yml",
		"deploy/docker-compose.local.yml",
		"deploy/docker-compose.dev.yml",
	} {
		relative := relative
		t.Run(filepath.Base(relative), func(t *testing.T) {
			content := readDeliveryContractFile(t, filepath.Join(repoRoot, filepath.FromSlash(relative)))
			for _, envName := range deliveryConfigEnvNames {
				if !strings.Contains(content, envName) {
					t.Errorf("%s does not pass through %s", relative, envName)
				}
			}
		})
	}
}

func TestDeliveryConfigContractUsesNamedVolumesForWindowsCompose(t *testing.T) {
	repoRoot := filepath.Clean(filepath.Join("..", "..", ".."))
	content := readDeliveryContractFile(t, filepath.Join(repoRoot, "deploy", "docker-compose.yml"))
	for _, mount := range []string{
		"sub2api_data:/app/data",
		"postgres_data:/var/lib/postgresql/data",
		"redis_data:/data",
	} {
		if !strings.Contains(content, mount) {
			t.Errorf("Windows-recommended compose is missing named-volume mount %q", mount)
		}
	}
}

func TestDeliveryConfigContractCoversOperatorExamples(t *testing.T) {
	repoRoot := filepath.Clean(filepath.Join("..", "..", ".."))
	envExample := readDeliveryContractFile(t, filepath.Join(repoRoot, "deploy", ".env.example"))
	for _, envName := range deliveryConfigEnvNames {
		if !strings.Contains(envExample, envName+"=") {
			t.Errorf("deploy/.env.example does not document %s", envName)
		}
	}

	yamlExample := readDeliveryContractFile(t, filepath.Join(repoRoot, "deploy", "config.example.yaml"))
	for _, fragment := range []string{
		"video_gateway:",
		"worker_enabled: true",
		"reliability_core:",
		"video_enabled: false",
		"retry_backoff_seconds: [5, 10, 20, 40, 80, 160, 300]",
	} {
		if !strings.Contains(yamlExample, fragment) {
			t.Errorf("deploy/config.example.yaml does not document %q", fragment)
		}
	}
}

func readDeliveryContractFile(t *testing.T, path string) string {
	t.Helper()
	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	return string(content)
}
