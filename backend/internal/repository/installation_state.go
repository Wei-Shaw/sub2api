package repository

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"errors"
	"fmt"
	"io/fs"
	"sort"
	"strings"

	"github.com/Wei-Shaw/sub2api/ent"
	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/migrations"
	"github.com/lib/pq"
)

const installationStateTableName = "installation_state"

var ErrBootstrapIncomplete = errors.New("bootstrap has not completed")

func BootstrapComplete(ctx context.Context, db *sql.DB) (bool, error) {
	if db == nil {
		return false, errors.New("nil sql db")
	}

	var complete bool
	err := db.QueryRowContext(ctx, "SELECT EXISTS (SELECT 1 FROM installation_state WHERE id = 1)").Scan(&complete)
	if err == nil {
		return complete, nil
	}
	if isUndefinedTableError(err) {
		return false, nil
	}
	return false, fmt.Errorf("read bootstrap state: %w", err)
}

func RequireBootstrapComplete(ctx context.Context, db *sql.DB) error {
	complete, err := BootstrapComplete(ctx, db)
	if err != nil {
		return err
	}
	if !complete {
		return ErrBootstrapIncomplete
	}
	return nil
}

func RequireMigrationsApplied(ctx context.Context, db *sql.DB) error {
	if db == nil {
		return errors.New("nil sql db")
	}
	return requireMigrationsApplied(ctx, db)
}

func BootstrapInstallation(ctx context.Context, db *sql.DB, client *ent.Client, cfg *config.Config) error {
	return BootstrapInstallationWithFinalizer(ctx, db, client, cfg, nil)
}

func BootstrapInstallationWithFinalizer(ctx context.Context, db *sql.DB, client *ent.Client, cfg *config.Config, finalize func() error) error {
	if db == nil {
		return errors.New("nil sql db")
	}
	if client == nil {
		return errors.New("nil ent client")
	}

	if err := RequireMigrationsApplied(ctx, db); err != nil {
		return err
	}

	lockConn, err := db.Conn(ctx)
	if err != nil {
		return fmt.Errorf("acquire bootstrap lock connection: %w", err)
	}
	defer func() { _ = lockConn.Close() }()
	if err := pgAdvisoryLockWithID(ctx, lockConn, bootstrapAdvisoryLockID); err != nil {
		return err
	}
	defer func() { _ = pgAdvisoryUnlockWithID(context.Background(), lockConn, bootstrapAdvisoryLockID) }()

	complete, err := BootstrapComplete(ctx, db)
	if err != nil {
		return err
	}
	if complete {
		return nil
	}

	if err := BootstrapEnt(ctx, client, cfg); err != nil {
		return err
	}
	if finalize != nil {
		if err := finalize(); err != nil {
			return err
		}
	}
	if _, err := lockConn.ExecContext(ctx, "INSERT INTO installation_state (id) VALUES (1)"); err != nil {
		return fmt.Errorf("record bootstrap completion: %w", err)
	}
	return nil
}

func requireMigrationsApplied(ctx context.Context, db *sql.DB) error {
	return requireMigrationsAppliedFS(ctx, db, migrations.FS)
}

func requireMigrationsAppliedFS(ctx context.Context, db *sql.DB, fsys fs.FS) error {
	files, err := migrationFilesWithChecksums(fsys)
	if err != nil {
		return fmt.Errorf("read migrations: %w", err)
	}
	for name, checksum := range files {
		var actual string
		err := db.QueryRowContext(ctx, "SELECT checksum FROM schema_migrations WHERE filename = $1", name).Scan(&actual)
		if err != nil {
			if errors.Is(err, sql.ErrNoRows) || isUndefinedTableError(err) {
				return fmt.Errorf("migration gate incomplete: %s", name)
			}
			return fmt.Errorf("read migration state for %s: %w", name, err)
		}
		if actual != checksum && !isMigrationChecksumCompatible(name, actual, checksum) {
			return fmt.Errorf("migration gate checksum mismatch for %s", name)
		}
	}
	return nil
}

func migrationFilesWithChecksums(fsys fs.FS) (map[string]string, error) {
	files, err := fs.Glob(fsys, "*.sql")
	if err != nil {
		return nil, err
	}
	sort.Strings(files)

	checksums := make(map[string]string, len(files))
	for _, name := range files {
		content, err := fs.ReadFile(fsys, name)
		if err != nil {
			return nil, err
		}
		trimmed := strings.TrimSpace(string(content))
		if trimmed == "" {
			continue
		}
		sum := sha256.Sum256([]byte(trimmed))
		checksums[name] = hex.EncodeToString(sum[:])
	}
	return checksums, nil
}

func isUndefinedTableError(err error) bool {
	var pqErr *pq.Error
	return errors.As(err, &pqErr) && pqErr.Code == "42P01"
}
