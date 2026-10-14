package service

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestAutoWorkerRequiresLocalOrigin(t *testing.T) {
	for _, origin := range []string{"http://127.0.0.1:8080", "http://localhost:8080", "http://[::1]:8080"} {
		require.True(t, validLocalWorkerOrigin(origin), origin)
	}
	for _, origin := range []string{"https://example.com:8080", "http://127.0.0.1:8080/api", "http://user@127.0.0.1:8080", "http://127.0.0.1:8080?token=abc", "http://127.0.0.1"} {
		require.False(t, validLocalWorkerOrigin(origin), origin)
	}
}

func TestAutoWorkerEnvironmentDoesNotInheritServerSecrets(t *testing.T) {
	t.Setenv("DATABASE_PASSWORD", "synthetic-db-secret")
	t.Setenv("ACCOUNT_VAULT_ENCRYPTION_KEY", "synthetic-vault-secret")
	t.Setenv("ADMIN_PASSWORD", "synthetic-admin-secret")
	t.Setenv("ACCOUNT_VAULT_BROWSER_PROXY", "http://127.0.0.1:7890")
	env := strings.Join(autoWorkerEnvironment(filepath.Join(t.TempDir(), "node.exe")), "\n")
	for _, secret := range []string{"synthetic-db-secret", "synthetic-vault-secret", "synthetic-admin-secret"} {
		require.NotContains(t, env, secret)
	}
	require.Contains(t, env, "PATH=")
	require.Contains(t, env, "ACCOUNT_VAULT_BROWSER_PROXY=http://127.0.0.1:7890")
}

func TestAutoWorkerPassesWindowsBrowserLocations(t *testing.T) {
	t.Setenv("PROGRAMFILES", `C:\Program Files`)
	t.Setenv("PROGRAMFILES(X86)", `C:\Program Files (x86)`)
	t.Setenv("HOMEDRIVE", `C:`)
	env := strings.Join(autoWorkerEnvironment(filepath.Join(t.TempDir(), "node.exe")), "\n")
	require.Contains(t, env, `PROGRAMFILES=C:\Program Files`)
	require.Contains(t, env, `PROGRAMFILES(X86)=C:\Program Files (x86)`)
	require.Contains(t, env, `HOMEDRIVE=C:`)
}

func TestAutoWorkerDisabledUnlessExplicitlyEnabled(t *testing.T) {
	old, wasSet := os.LookupEnv("ACCOUNT_VAULT_AUTO_WORKER")
	t.Cleanup(func() {
		if wasSet {
			_ = os.Setenv("ACCOUNT_VAULT_AUTO_WORKER", old)
		} else {
			_ = os.Unsetenv("ACCOUNT_VAULT_AUTO_WORKER")
		}
	})
	_ = os.Unsetenv("ACCOUNT_VAULT_AUTO_WORKER")
	require.Nil(t, newAccountVaultAutoWorker())
}

func TestAutoWorkerErrorSinkKeepsOnlySymbolicCode(t *testing.T) {
	sink := &autoWorkerErrorSink{}
	_, _ = sink.Write([]byte("provider URL with synthetic-secret-token\n助手停"))
	_, _ = sink.Write([]byte("止：BROWSER_START_FAILED_RUN_INSTALL_BROWSER。请查看 README\n"))
	require.Equal(t, "browser_start_failed_run_install_browser", sink.errorCode())
	_, _ = sink.Write([]byte("https://example.test/?token=synthetic-secret-token\n"))
	require.Equal(t, "browser_start_failed_run_install_browser", sink.errorCode())
}
