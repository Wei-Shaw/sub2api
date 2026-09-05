package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadDotEnvFromDir(t *testing.T) {
	t.Setenv("SERVER_PORT", "__unset__")
	if err := os.Unsetenv("SERVER_PORT"); err != nil {
		t.Fatalf("unset server port: %v", err)
	}
	if err := os.Unsetenv("SERVER_HOST"); err != nil {
		t.Fatalf("unset server host: %v", err)
	}

	root := t.TempDir()
	backendDir := filepath.Join(root, "backend")
	if err := os.MkdirAll(backendDir, 0o755); err != nil {
		t.Fatalf("mkdir backend dir: %v", err)
	}

	if err := os.WriteFile(filepath.Join(root, ".env"), []byte("SERVER_PORT=8080\nSERVER_HOST=127.0.0.1\n"), 0o644); err != nil {
		t.Fatalf("write root .env: %v", err)
	}
	if err := os.WriteFile(filepath.Join(backendDir, ".env"), []byte("SERVER_PORT=3000\n"), 0o644); err != nil {
		t.Fatalf("write backend .env: %v", err)
	}

	if err := loadDotEnvFromDir(backendDir); err != nil {
		t.Fatalf("loadDotEnvFromDir() error: %v", err)
	}

	if got := os.Getenv("SERVER_PORT"); got != "3000" {
		t.Fatalf("SERVER_PORT = %q, want %q", got, "3000")
	}
	if got := os.Getenv("SERVER_HOST"); got != "127.0.0.1" {
		t.Fatalf("SERVER_HOST = %q, want %q", got, "127.0.0.1")
	}
}

func TestLoadDotEnvDoesNotOverrideExistingEnv(t *testing.T) {
	t.Setenv("SERVER_PORT", "9090")

	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, ".env"), []byte("SERVER_PORT=8080\n"), 0o644); err != nil {
		t.Fatalf("write .env: %v", err)
	}

	if err := loadDotEnvFromDir(dir); err != nil {
		t.Fatalf("loadDotEnvFromDir() error: %v", err)
	}

	if got := os.Getenv("SERVER_PORT"); got != "9090" {
		t.Fatalf("SERVER_PORT = %q, want %q", got, "9090")
	}
}
