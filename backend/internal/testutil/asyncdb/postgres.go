//go:build integration

// Package asyncdb supplies isolated PostgreSQL schemas for async API tests.
package asyncdb

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/migrations"
	"github.com/google/uuid"
	"github.com/lib/pq"
	"github.com/stretchr/testify/require"
	postgres "github.com/testcontainers/testcontainers-go/modules/postgres"
)

func Open(t *testing.T) *sql.DB {
	t.Helper()
	dsn := os.Getenv("OPENAI_ASYNC_TEST_DSN")
	if dsn == "" {
		// CI must exercise persistence even without a manually configured DSN.
		ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
		defer cancel()
		if err := exec.CommandContext(ctx, "docker", "info").Run(); err != nil {
			if os.Getenv("CI") != "" {
				t.Fatal("Docker is required for async database tests in CI")
			}
			t.Skip("Set OPENAI_ASYNC_TEST_DSN or start Docker for database tests")
		}
		image := os.Getenv("SUB2API_TEST_POSTGRES_IMAGE")
		if image == "" {
			image = "postgres:18.1-alpine3.23"
		}
		container, err := postgres.Run(ctx, image, postgres.WithDatabase("sub2api_test"), postgres.WithUsername("postgres"), postgres.WithPassword("postgres"), postgres.BasicWaitStrategies())
		require.NoError(t, err)
		t.Cleanup(func() {
			cleanupCtx, cleanupCancel := context.WithTimeout(context.Background(), 15*time.Second)
			defer cleanupCancel()
			require.NoError(t, container.Terminate(cleanupCtx))
		})
		host, err := container.Host(ctx)
		require.NoError(t, err)
		port, err := container.MappedPort(ctx, "5432/tcp")
		require.NoError(t, err)
		dsn = fmt.Sprintf("host=%s port=%d user=postgres password=postgres dbname=sub2api_test sslmode=disable", host, port.Int())
	}
	if strings.HasPrefix(dsn, "postgres://") || strings.HasPrefix(dsn, "postgresql://") {
		parsed, err := pq.ParseURL(dsn)
		require.NoError(t, err)
		dsn = parsed
	}
	admin, err := sql.Open("postgres", dsn)
	require.NoError(t, err)
	schema := "async_" + strings.ReplaceAll(uuid.NewString(), "-", "")
	_, err = admin.Exec(`CREATE SCHEMA ` + schema)
	require.NoError(t, err)
	db, err := sql.Open("postgres", dsn+" search_path="+schema)
	require.NoError(t, err)
	t.Cleanup(func() {
		_ = db.Close()
		_, dropErr := admin.Exec(`DROP SCHEMA ` + schema + ` CASCADE`)
		require.NoError(t, dropErr)
		_ = admin.Close()
	})
	_, err = db.Exec(`CREATE TABLE users(id BIGINT PRIMARY KEY);CREATE TABLE api_keys(id BIGINT PRIMARY KEY);CREATE TABLE groups(id BIGINT PRIMARY KEY);INSERT INTO users VALUES(1),(2);INSERT INTO api_keys VALUES(1);INSERT INTO groups VALUES(1)`)
	require.NoError(t, err)
	for _, name := range []string{"249_openai_files_batches.sql", "250_openai_batch_request_json.sql", "251_openai_batch_storage_reservations.sql"} {
		migration, readErr := migrations.FS.ReadFile(name)
		require.NoError(t, readErr)
		for attempt := 0; attempt < 2; attempt++ {
			_, err = db.Exec(string(migration))
			require.NoError(t, err, "forward migration must be safe to replay")
		}
	}
	return db
}
