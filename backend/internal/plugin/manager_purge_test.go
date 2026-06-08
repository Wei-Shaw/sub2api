//go:build unit

package plugin

import (
	"context"
	"database/sql/driver"
	"errors"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
)

// pluginMigrationDownColumns mirrors the SELECT projection emitted by
// revertCachedDownMigrationsTx — sqlmock with QueryMatcherEqual requires
// the column list to match exactly.
var pluginMigrationDownColumns = []string{"filename", "down_sql_cached"}

const queryRevertList = `SELECT filename, down_sql_cached
	FROM plugin_migrations
	WHERE plugin_name = $1
	ORDER BY filename DESC`

func TestPurgeRejectsActivePlugin(t *testing.T) {
	t.Parallel()
	m, mock, cleanup := newRepoTestManager(t)
	defer cleanup()

	const name = "demo"

	// GetIncludingUninstalled returns active row (uninstalled_at IS NULL).
	mock.ExpectQuery(queryGetIncludingUninstalled).
		WithArgs(name).
		WillReturnRows(sqlmock.NewRows(pluginColumns).AddRow(activeRow(name, false)...))

	err := m.Purge(context.Background(), name)
	if !errors.Is(err, ErrPluginNotSoftUninstalled) {
		t.Fatalf("expected ErrPluginNotSoftUninstalled, got: %v", err)
	}

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("sqlmock expectations: %v", err)
	}
}

func TestPurgeRejectsBuiltinPlugins(t *testing.T) {
	t.Parallel()
	m, _, cleanup := newRepoTestManager(t)
	defer cleanup()

	builtin := t.TempDir()
	const name = "builtin-demo"
	dir := filepath.Join(builtin, name)
	if err := mkdirAll(dir); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	binPath := filepath.Join(dir, pluginBinaryName(name))
	if err := writeExecutable(binPath); err != nil {
		t.Fatalf("write binary: %v", err)
	}
	m.cfg.BuiltinDir = builtin

	err := m.Purge(context.Background(), name)
	if !errors.Is(err, ErrPluginIsBuiltin) {
		t.Fatalf("expected ErrPluginIsBuiltin, got: %v", err)
	}
}

func TestPurgeReturnsNotFoundForMissingRow(t *testing.T) {
	t.Parallel()
	m, mock, cleanup := newRepoTestManager(t)
	defer cleanup()

	const name = "ghost"

	mock.ExpectQuery(queryGetIncludingUninstalled).
		WithArgs(name).
		WillReturnRows(sqlmock.NewRows(pluginColumns)) // empty -> ErrPluginNotFound

	err := m.Purge(context.Background(), name)
	if !errors.Is(err, ErrPluginNotFound) {
		t.Fatalf("expected ErrPluginNotFound, got: %v", err)
	}

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("sqlmock expectations: %v", err)
	}
}

// TestPurgeRevertsDownAndDeletesAll exercises the happy path: the soft-
// uninstalled plugin has two migrations, one with cached down SQL and one
// without. Purge runs the cached one in reverse order, then DELETEs the
// bookkeeping rows from all four tables in one transaction.
func TestPurgeRevertsDownAndDeletesAll(t *testing.T) {
	t.Parallel()
	m, mock, cleanup := newRepoTestManager(t)
	defer cleanup()

	const name = "demo"

	// Pre-populate the in-memory map so we can verify it's emptied after Purge.
	m.plugins[name] = &PluginInstance{Name: name, State: StateRegistered}

	mock.ExpectQuery(queryGetIncludingUninstalled).
		WithArgs(name).
		WillReturnRows(sqlmock.NewRows(pluginColumns).AddRow(uninstalledRow(name)...))

	mock.ExpectBegin()
	// Two migrations — one reversible, one irreversible (NULL down SQL).
	// ORDER BY filename DESC -> 002 first, then 001.
	mock.ExpectQuery(queryRevertList).
		WithArgs(name).
		WillReturnRows(sqlmock.NewRows(pluginMigrationDownColumns).
			AddRow("002_add_index.sql", "DROP INDEX IF EXISTS demo_idx;").
			AddRow("001_create_x.sql", nil),
		)
	mock.ExpectExec("DROP INDEX IF EXISTS demo_idx;").
		WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectExec("DELETE FROM plugin_migrations WHERE plugin_name = $1").
		WithArgs(name).
		WillReturnResult(sqlmock.NewResult(0, 2))
	mock.ExpectExec("DELETE FROM plugin_settings WHERE plugin_name = $1").
		WithArgs(name).
		WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectExec("DELETE FROM plugin_settings_schemas WHERE plugin_name = $1").
		WithArgs(name).
		WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectExec("DELETE FROM plugins WHERE name = $1").
		WithArgs(name).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()

	if err := m.Purge(context.Background(), name); err != nil {
		t.Fatalf("Purge: %v", err)
	}

	m.mu.RLock()
	_, exists := m.plugins[name]
	m.mu.RUnlock()
	if exists {
		t.Fatalf("expected manager.plugins to no longer contain %q after Purge", name)
	}

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("sqlmock expectations: %v", err)
	}
}

// TestPurgeRollsBackOnDownFailure verifies that a failing down migration
// aborts the entire Purge: the transaction is rolled back, the plugins
// row is NOT deleted, and the soft-uninstall state is preserved.
func TestPurgeRollsBackOnDownFailure(t *testing.T) {
	t.Parallel()
	m, mock, cleanup := newRepoTestManager(t)
	defer cleanup()

	const name = "demo"

	mock.ExpectQuery(queryGetIncludingUninstalled).
		WithArgs(name).
		WillReturnRows(sqlmock.NewRows(pluginColumns).AddRow(uninstalledRow(name)...))

	mock.ExpectBegin()
	mock.ExpectQuery(queryRevertList).
		WithArgs(name).
		WillReturnRows(sqlmock.NewRows(pluginMigrationDownColumns).
			AddRow("001_create_x.sql", "DROP TABLE oh_no;"),
		)
	mock.ExpectExec("DROP TABLE oh_no;").
		WillReturnError(errBoom)
	mock.ExpectRollback()

	err := m.Purge(context.Background(), name)
	if err == nil {
		t.Fatalf("expected error, got nil")
	}
	if !strings.Contains(err.Error(), "apply down migration") {
		t.Fatalf("error missing context about down failure: %v", err)
	}

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("sqlmock expectations: %v", err)
	}
}

// TestPurgeFiresFrontendCacheInvalidator confirms the SSR HTML cache is
// invalidated exactly once when a Purge succeeds — so the next render no
// longer references the deleted plugin's sidebar / route entries.
func TestPurgeFiresFrontendCacheInvalidator(t *testing.T) {
	t.Parallel()
	m, mock, cleanup := newRepoTestManager(t)
	defer cleanup()

	const name = "demo"
	var (
		callMu sync.Mutex
		calls  int
	)
	m.frontendCacheInvalidator = func() {
		callMu.Lock()
		defer callMu.Unlock()
		calls++
	}

	mock.ExpectQuery(queryGetIncludingUninstalled).
		WithArgs(name).
		WillReturnRows(sqlmock.NewRows(pluginColumns).AddRow(uninstalledRow(name)...))
	mock.ExpectBegin()
	mock.ExpectQuery(queryRevertList).
		WithArgs(name).
		WillReturnRows(sqlmock.NewRows(pluginMigrationDownColumns)) // no migrations
	mock.ExpectExec("DELETE FROM plugin_migrations WHERE plugin_name = $1").
		WithArgs(name).
		WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectExec("DELETE FROM plugin_settings WHERE plugin_name = $1").
		WithArgs(name).
		WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectExec("DELETE FROM plugin_settings_schemas WHERE plugin_name = $1").
		WithArgs(name).
		WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectExec("DELETE FROM plugins WHERE name = $1").
		WithArgs(name).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()

	if err := m.Purge(context.Background(), name); err != nil {
		t.Fatalf("Purge: %v", err)
	}

	callMu.Lock()
	defer callMu.Unlock()
	if calls != 1 {
		t.Fatalf("expected frontend cache invalidator to fire once, got %d", calls)
	}

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("sqlmock expectations: %v", err)
	}
}

// errBoom is a sentinel sqlmock error used to drive failure-path tests.
var errBoom = errBoomError("boom")

type errBoomError string

func (e errBoomError) Error() string { return string(e) }

// _ exists to keep the time import (used implicitly via uninstalledRow).
var _ = time.Time{}
var _ driver.Value = nil
