package repository

import (
	"os"
	"strings"
	"testing"
)

func TestInitEntUsesCanonicalMigrationEntrypoint(t *testing.T) {
	content, err := os.ReadFile("ent.go")
	if err != nil {
		t.Fatalf("read ent.go: %v", err)
	}
	source := string(content)
	if !strings.Contains(source, "github.com/Wei-Shaw/sub2api/migrations") {
		t.Fatalf("InitEnt must keep migrations import visible so startup explicitly passes upstream and local migration sources")
	}
	if !strings.Contains(source, "ApplyMigrationsFromFS(migrationCtx, drv.DB(), migrations.UpstreamFS, migrations.LocalFS)") {
		t.Fatalf("InitEnt must run both upstream and local migrations on startup")
	}
	if strings.Contains(source, "applyMigrationsFS(migrationCtx, drv.DB(), migrations.FS)") || strings.Contains(source, "ApplyMigrations(migrationCtx, drv.DB())") {
		t.Fatalf("InitEnt must not hide or bypass local migrations through a single-source helper")
	}
}
