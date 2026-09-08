package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestMustLoadLoadsSharedDotEnvFromParentDir(t *testing.T) {
	unsetEnv(t, "PLUGIN_SERVER_PORT")

	tempDir := t.TempDir()
	writeFile(t, filepath.Join(tempDir, ".env"), "PLUGIN_SERVER_PORT=18091\n")
	if err := os.MkdirAll(filepath.Join(tempDir, "plugin-service"), 0o755); err != nil {
		t.Fatal(err)
	}

	cwd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = os.Chdir(cwd) }()
	if err := os.Chdir(filepath.Join(tempDir, "plugin-service")); err != nil {
		t.Fatal(err)
	}

	cfg := MustLoad()
	if cfg.ListenAddr != ":18091" {
		t.Fatalf("listen addr = %q, want %q", cfg.ListenAddr, ":18091")
	}
}

func TestMustLoadIgnoresPluginServiceLocalDotEnv(t *testing.T) {
	unsetEnv(t, "PLUGIN_SERVER_PORT")

	tempDir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(tempDir, "plugin-service"), 0o755); err != nil {
		t.Fatal(err)
	}
	writeFile(t, filepath.Join(tempDir, ".env"), "PLUGIN_SERVER_PORT=18091\n")
	writeFile(t, filepath.Join(tempDir, "plugin-service", ".env"), "PLUGIN_SERVER_PORT=19091\n")

	cwd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = os.Chdir(cwd) }()
	if err := os.Chdir(filepath.Join(tempDir, "plugin-service")); err != nil {
		t.Fatal(err)
	}

	cfg := MustLoad()
	if cfg.ListenAddr != ":18091" {
		t.Fatalf("listen addr = %q, want %q", cfg.ListenAddr, ":18091")
	}
}

func TestMustLoadPrefersProcessEnvOverDotEnv(t *testing.T) {
	unsetEnv(t, "PLUGIN_SERVER_PORT")

	tempDir := t.TempDir()
	writeFile(t, filepath.Join(tempDir, ".env"), "PLUGIN_SERVER_PORT=18091\n")
	if err := os.MkdirAll(filepath.Join(tempDir, "plugin-service"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Setenv("PLUGIN_SERVER_PORT", "17091"); err != nil {
		t.Fatal(err)
	}

	cwd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = os.Chdir(cwd) }()
	if err := os.Chdir(filepath.Join(tempDir, "plugin-service")); err != nil {
		t.Fatal(err)
	}

	cfg := MustLoad()
	if cfg.ListenAddr != ":17091" {
		t.Fatalf("listen addr = %q, want %q", cfg.ListenAddr, ":17091")
	}
}

func TestMustLoadUsesDefaultPortWhenUnset(t *testing.T) {
	unsetEnv(t, "PLUGIN_SERVER_PORT", "DATABASE_URL", "DATABASE_HOST", "DATABASE_PORT", "DATABASE_USER", "DATABASE_PASSWORD", "DATABASE_DBNAME", "DATABASE_SSLMODE", "MINIO_ENDPOINT", "MINIO_ACCESS_KEY", "MINIO_SECRET_KEY", "MINIO_BUCKET", "MINIO_USE_SSL")

	cfg := MustLoad()
	if cfg.ListenAddr != ":8091" {
		t.Fatalf("listen addr = %q, want default %q", cfg.ListenAddr, ":8091")
	}
	if cfg.Database.Enabled {
		t.Fatal("database should be disabled when shared database env is unset")
	}
}

func TestMustLoadEnablesMinIOWhenFullyConfigured(t *testing.T) {
	unsetEnv(t, "MINIO_ENDPOINT", "MINIO_ACCESS_KEY", "MINIO_SECRET_KEY", "MINIO_BUCKET", "MINIO_USE_SSL")
	t.Setenv("MINIO_ENDPOINT", "minio:9000")
	t.Setenv("MINIO_ACCESS_KEY", "plugin")
	t.Setenv("MINIO_SECRET_KEY", "plugin-secret")
	t.Setenv("MINIO_BUCKET", "plugin-media")
	t.Setenv("MINIO_USE_SSL", "true")

	cfg := MustLoad()
	if !cfg.MinIO.Enabled || cfg.MinIO.Endpoint != "minio:9000" || cfg.MinIO.Bucket != "plugin-media" || !cfg.MinIO.UseSSL {
		t.Fatalf("minio config = %#v", cfg.MinIO)
	}
}

func TestMustLoadUsesMinIORootCredentialsForSourceRun(t *testing.T) {
	unsetEnv(t, "MINIO_ENDPOINT", "MINIO_ACCESS_KEY", "MINIO_SECRET_KEY", "MINIO_ROOT_USER", "MINIO_ROOT_PASSWORD", "MINIO_BUCKET", "MINIO_USE_SSL")
	t.Setenv("MINIO_ENDPOINT", "127.0.0.1:9000")
	t.Setenv("MINIO_ROOT_USER", "plugin-root")
	t.Setenv("MINIO_ROOT_PASSWORD", "plugin-root-secret")
	t.Setenv("MINIO_BUCKET", "plugin-media")

	cfg := MustLoad()
	if !cfg.MinIO.Enabled || cfg.MinIO.AccessKey != "plugin-root" || cfg.MinIO.SecretKey != "plugin-root-secret" {
		t.Fatalf("minio config = %#v", cfg.MinIO)
	}
}

func TestMustLoadRejectsPartialMinIOConfiguration(t *testing.T) {
	unsetEnv(t, "MINIO_ENDPOINT", "MINIO_ACCESS_KEY", "MINIO_SECRET_KEY", "MINIO_BUCKET", "MINIO_USE_SSL")
	t.Setenv("MINIO_ENDPOINT", "minio:9000")

	defer func() {
		if recover() == nil {
			t.Fatal("MustLoad() did not reject partial MinIO configuration")
		}
	}()
	_ = MustLoad()
}

func TestMustLoadIgnoresLegacyPluginServiceEnvNames(t *testing.T) {
	unsetEnv(t, "PLUGIN_SERVER_PORT", "PLUGIN_SERVICE_LISTEN_ADDR")

	if err := os.Setenv("PLUGIN_SERVICE_LISTEN_ADDR", ":19091"); err != nil {
		t.Fatal(err)
	}

	cfg := MustLoad()
	if cfg.ListenAddr != ":8091" {
		t.Fatalf("listen addr = %q, want default %q", cfg.ListenAddr, ":8091")
	}
}

func TestMustLoadUsesSharedDatabaseURL(t *testing.T) {
	unsetEnv(t, "DATABASE_URL", "DATABASE_HOST", "DATABASE_PORT", "DATABASE_USER", "DATABASE_PASSWORD", "DATABASE_DBNAME", "DATABASE_SSLMODE")

	if err := os.Setenv("DATABASE_URL", "postgres://sub2api:secret@postgres:5432/sub2api?sslmode=disable"); err != nil {
		t.Fatal(err)
	}

	cfg := MustLoad()
	if !cfg.Database.Enabled {
		t.Fatal("database should be enabled when DATABASE_URL is set")
	}
	if cfg.Database.DSN() != "postgres://sub2api:secret@postgres:5432/sub2api?sslmode=disable" {
		t.Fatalf("database dsn = %q", cfg.Database.DSN())
	}
}

func TestMustLoadUsesSharedDatabaseFields(t *testing.T) {
	unsetEnv(t, "DATABASE_URL", "DATABASE_HOST", "DATABASE_PORT", "DATABASE_USER", "DATABASE_PASSWORD", "DATABASE_DBNAME", "DATABASE_SSLMODE")

	for key, value := range map[string]string{
		"DATABASE_HOST":     "postgres",
		"DATABASE_PORT":     "5433",
		"DATABASE_USER":     "sub2api",
		"DATABASE_PASSWORD": "secret",
		"DATABASE_DBNAME":   "sub2api",
		"DATABASE_SSLMODE":  "disable",
	} {
		if err := os.Setenv(key, value); err != nil {
			t.Fatal(err)
		}
	}

	cfg := MustLoad()
	if !cfg.Database.Enabled {
		t.Fatal("database should be enabled when DATABASE_HOST is set")
	}
	want := "host=postgres port=5433 user=sub2api password=secret dbname=sub2api sslmode=disable"
	if cfg.Database.DSN() != want {
		t.Fatalf("database dsn = %q, want %q", cfg.Database.DSN(), want)
	}
}

func TestMustLoadFallsBackToDeployDotEnv(t *testing.T) {
	unsetEnv(t, "PLUGIN_SERVER_PORT", "DATABASE_URL", "DATABASE_HOST", "DATABASE_PORT", "DATABASE_USER", "DATABASE_PASSWORD", "DATABASE_DBNAME", "DATABASE_SSLMODE")

	tempDir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(tempDir, "plugin-service"), 0o755); err != nil {
		t.Fatal(err)
	}
	writeFile(t, filepath.Join(tempDir, "deploy", ".env"), "DATABASE_HOST=deploy-postgres\nDATABASE_PORT=5439\nDATABASE_USER=deploy-user\nDATABASE_PASSWORD=deploy-pass\nDATABASE_DBNAME=deploy-db\nDATABASE_SSLMODE=disable\n")

	cwd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = os.Chdir(cwd) }()
	if err := os.Chdir(filepath.Join(tempDir, "plugin-service")); err != nil {
		t.Fatal(err)
	}

	cfg := MustLoad()
	if !cfg.Database.Enabled {
		t.Fatal("database should be enabled from deploy/.env")
	}
	want := "host=deploy-postgres port=5439 user=deploy-user password=deploy-pass dbname=deploy-db sslmode=disable"
	if cfg.Database.DSN() != want {
		t.Fatalf("database dsn = %q, want %q", cfg.Database.DSN(), want)
	}
}

func TestMustLoadRootDotEnvOverridesDeployDotEnv(t *testing.T) {
	unsetEnv(t, "PLUGIN_SERVER_PORT", "DATABASE_URL", "DATABASE_HOST", "DATABASE_PORT", "DATABASE_USER", "DATABASE_PASSWORD", "DATABASE_DBNAME", "DATABASE_SSLMODE")

	tempDir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(tempDir, "plugin-service"), 0o755); err != nil {
		t.Fatal(err)
	}
	writeFile(t, filepath.Join(tempDir, ".env"), "DATABASE_HOST=root-postgres\nDATABASE_PORT=5438\nDATABASE_USER=root-user\nDATABASE_PASSWORD=root-pass\nDATABASE_DBNAME=root-db\nDATABASE_SSLMODE=disable\n")
	writeFile(t, filepath.Join(tempDir, "deploy", ".env"), "DATABASE_HOST=deploy-postgres\nDATABASE_PORT=5439\nDATABASE_USER=deploy-user\nDATABASE_PASSWORD=deploy-pass\nDATABASE_DBNAME=deploy-db\nDATABASE_SSLMODE=disable\n")

	cwd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = os.Chdir(cwd) }()
	if err := os.Chdir(filepath.Join(tempDir, "plugin-service")); err != nil {
		t.Fatal(err)
	}

	cfg := MustLoad()
	want := "host=root-postgres port=5438 user=root-user password=root-pass dbname=root-db sslmode=disable"
	if cfg.Database.DSN() != want {
		t.Fatalf("database dsn = %q, want %q", cfg.Database.DSN(), want)
	}
}

func TestMustLoadFallsBackToPostgresEnvNames(t *testing.T) {
	unsetEnv(t, "DATABASE_URL", "DATABASE_HOST", "DATABASE_PORT", "DATABASE_USER", "DATABASE_PASSWORD", "DATABASE_DBNAME", "DATABASE_SSLMODE", "POSTGRES_HOST", "POSTGRES_PORT", "POSTGRES_USER", "POSTGRES_PASSWORD", "POSTGRES_DB")

	for key, value := range map[string]string{
		"POSTGRES_USER":     "sub2api",
		"POSTGRES_PASSWORD": "secret",
		"POSTGRES_DB":       "sub2api",
	} {
		if err := os.Setenv(key, value); err != nil {
			t.Fatal(err)
		}
	}

	cfg := MustLoad()
	if !cfg.Database.Enabled {
		t.Fatal("database should be enabled from POSTGRES_* env")
	}
	want := "host=127.0.0.1 port=5432 user=sub2api password=secret dbname=sub2api sslmode=disable"
	if cfg.Database.DSN() != want {
		t.Fatalf("database dsn = %q, want %q", cfg.Database.DSN(), want)
	}
}

func unsetEnv(t *testing.T, keys ...string) {
	t.Helper()
	for _, key := range keys {
		value, exists := os.LookupEnv(key)
		if exists {
			if err := os.Unsetenv(key); err != nil {
				t.Fatal(err)
			}
		}
		t.Cleanup(func() {
			if exists {
				_ = os.Setenv(key, value)
				return
			}
			_ = os.Unsetenv(key)
		})
	}
}

func writeFile(t *testing.T, path string, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}
