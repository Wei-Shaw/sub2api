//go:build unit

package plugin

import (
	"context"
	"database/sql/driver"
	"errors"
	"log/slog"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
)

// mkdirAll wraps os.MkdirAll with the standard 0o755 mode used by test
// scaffolding throughout this package.
func mkdirAll(path string) error {
	return os.MkdirAll(path, 0o755)
}

// writeExecutable creates an empty file at path with mode 0o755 so the
// isBuiltinPlugin filesystem check (st.IsDir()==false) accepts it as a
// plugin binary.
func writeExecutable(path string) error {
	return os.WriteFile(path, []byte{}, 0o755)
}

// newRepoTestManager builds a minimal PluginManager wired only to a sqlmock-
// backed repository. It deliberately skips Start / SDK gRPC / router so the
// tests can exercise Uninstall / Install lifecycle paths that do NOT touch a
// running plugin process. The few manager fields the lifecycle methods read
// (logger, plugins map, mu, frontendCacheInvalidator) are zero-initialised
// to safe defaults.
//
// Returned cleanup must be invoked to close the sqlmock DB.
func newRepoTestManager(t *testing.T) (*PluginManager, sqlmock.Sqlmock, func()) {
	t.Helper()
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherEqual))
	if err != nil {
		t.Fatalf("sqlmock.New: %v", err)
	}
	m := &PluginManager{
		plugins: make(map[string]*PluginInstance),
		repo:    NewPluginRepository(db),
		db:      db,
		logger:  slog.Default(),
	}
	cleanup := func() { _ = db.Close() }
	return m, mock, cleanup
}

// activeRow returns the column values that scanPluginRow expects for an
// active (uninstalled_at IS NULL) plugin record.
func activeRow(name string, enabled bool) []driver.Value {
	return []driver.Value{
		name,         // name
		"",           // display_name
		"",           // description
		"",           // version
		enabled,      // enabled
		int64(0),     // sort_order
		[]byte("{}"), // config
		time.Now(),   // installed_at
		time.Now(),   // updated_at
		nil,          // uninstalled_at (NULL)
	}
}

// uninstalledRow mirrors activeRow but with a non-null uninstalled_at.
func uninstalledRow(name string) []driver.Value {
	return []driver.Value{
		name, "", "", "", false, int64(0), []byte("{}"),
		time.Now(), time.Now(), time.Now(), // uninstalled_at NOT NULL
	}
}

// pluginColumns is the SELECT projection scanPluginRow expects, in order.
var pluginColumns = []string{
	"name", "display_name", "description", "version",
	"enabled", "sort_order", "config",
	"installed_at", "updated_at", "uninstalled_at",
}

// queryGetIncludingUninstalled is the SQL emitted by repo.GetIncludingUninstalled
// — sqlmock with QueryMatcherEqual requires the literal string match.
const queryGetIncludingUninstalled = `
		SELECT name, display_name, description, version, enabled, sort_order, config, installed_at, updated_at, uninstalled_at
		FROM plugins WHERE name = $1
	`

// queryGetActive only matches active rows (uninstalled_at IS NULL).
const queryGetActive = `
		SELECT name, display_name, description, version, enabled, sort_order, config, installed_at, updated_at, uninstalled_at
		FROM plugins WHERE name = $1
	 AND uninstalled_at IS NULL`

// queryListActive matches repo.List (active only).
const queryListActive = `
		SELECT name, display_name, description, version, enabled, sort_order, config, installed_at, updated_at, uninstalled_at
		FROM plugins
	 WHERE uninstalled_at IS NULL ORDER BY sort_order ASC, name ASC`

// queryListAll matches repo.ListAll.
const queryListAll = `
		SELECT name, display_name, description, version, enabled, sort_order, config, installed_at, updated_at, uninstalled_at
		FROM plugins
	 ORDER BY sort_order ASC, name ASC`

// queryMarkUninstalled / queryMarkInstalled / queryExists match the literal
// SQL emitted by their repository methods.
const queryMarkUninstalled = `
		UPDATE plugins
		SET uninstalled_at = NOW(),
		    enabled        = FALSE,
		    updated_at     = NOW()
		WHERE name = $1 AND uninstalled_at IS NULL
	`

const queryMarkInstalled = `
		UPDATE plugins
		SET uninstalled_at = NULL,
		    updated_at     = NOW()
		WHERE name = $1
	`

const queryExists = `SELECT 1 FROM plugins WHERE name = $1`

func TestUninstallPreservesDataAndDropsInstance(t *testing.T) {
	t.Parallel()
	m, mock, cleanup := newRepoTestManager(t)
	defer cleanup()

	const name = "demo"

	// 模拟 instance 在 map 中存在但未运行 (stopInstance 在没有 cmd 时 early return),
	// 用以验证 Uninstall 后内存条目被清掉。
	m.plugins[name] = &PluginInstance{
		Name:  name,
		State: StateRegistered,
	}

	// GetIncludingUninstalled — 返回 active 行
	mock.ExpectQuery(queryGetIncludingUninstalled).
		WithArgs(name).
		WillReturnRows(sqlmock.NewRows(pluginColumns).AddRow(activeRow(name, false)...))

	// MarkUninstalled
	mock.ExpectExec(queryMarkUninstalled).
		WithArgs(name).
		WillReturnResult(sqlmock.NewResult(0, 1))

	if err := m.Uninstall(context.Background(), name); err != nil {
		t.Fatalf("Uninstall: %v", err)
	}

	// instance 应已从 manager.plugins 删除
	m.mu.RLock()
	_, exists := m.plugins[name]
	m.mu.RUnlock()
	if exists {
		t.Fatalf("expected manager.plugins to no longer contain %q after Uninstall", name)
	}

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("sqlmock expectations: %v", err)
	}
}

func TestUninstallIdempotentOnAlreadyUninstalled(t *testing.T) {
	t.Parallel()
	m, mock, cleanup := newRepoTestManager(t)
	defer cleanup()

	const name = "demo"

	// GetIncludingUninstalled — 返回 already-uninstalled 行 (uninstalled_at NOT NULL)
	mock.ExpectQuery(queryGetIncludingUninstalled).
		WithArgs(name).
		WillReturnRows(sqlmock.NewRows(pluginColumns).AddRow(uninstalledRow(name)...))

	if err := m.Uninstall(context.Background(), name); err != nil {
		t.Fatalf("Uninstall (idempotent path) returned error: %v", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("sqlmock expectations: %v", err)
	}
}

func TestUninstallReturnsNotFoundForMissingRow(t *testing.T) {
	t.Parallel()
	m, mock, cleanup := newRepoTestManager(t)
	defer cleanup()

	const name = "ghost"

	mock.ExpectQuery(queryGetIncludingUninstalled).
		WithArgs(name).
		WillReturnRows(sqlmock.NewRows(pluginColumns)) // empty -> sql.ErrNoRows -> ErrPluginNotFound

	err := m.Uninstall(context.Background(), name)
	if !errors.Is(err, ErrPluginNotFound) {
		t.Fatalf("expected ErrPluginNotFound, got: %v", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("sqlmock expectations: %v", err)
	}
}

func TestUninstallRejectsBuiltinPlugins(t *testing.T) {
	t.Parallel()
	m, _, cleanup := newRepoTestManager(t)
	defer cleanup()

	// Stand up a fake builtin dir with the expected layout: <BuiltinDir>/<name>/<name>(.exe).
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

	err := m.Uninstall(context.Background(), name)
	if !errors.Is(err, ErrPluginIsBuiltin) {
		t.Fatalf("expected ErrPluginIsBuiltin, got: %v", err)
	}
	// 未到 DB 阶段, sqlmock 不应有任何 expectation 触发。
}

func TestInstallRestoresSoftUninstalled(t *testing.T) {
	t.Parallel()
	m, mock, cleanup := newRepoTestManager(t)
	defer cleanup()

	const name = "demo"

	// GetIncludingUninstalled — 返回 uninstalled 行
	mock.ExpectQuery(queryGetIncludingUninstalled).
		WithArgs(name).
		WillReturnRows(sqlmock.NewRows(pluginColumns).AddRow(uninstalledRow(name)...))

	// MarkInstalled
	mock.ExpectExec(queryMarkInstalled).
		WithArgs(name).
		WillReturnResult(sqlmock.NewResult(0, 1))

	if err := m.Install(context.Background(), name); err != nil {
		t.Fatalf("Install: %v", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("sqlmock expectations: %v", err)
	}
}

func TestInstallIdempotentOnActivePlugin(t *testing.T) {
	t.Parallel()
	m, mock, cleanup := newRepoTestManager(t)
	defer cleanup()

	const name = "demo"

	// GetIncludingUninstalled — 返回 active 行 (uninstalled_at IS NULL)
	mock.ExpectQuery(queryGetIncludingUninstalled).
		WithArgs(name).
		WillReturnRows(sqlmock.NewRows(pluginColumns).AddRow(activeRow(name, true)...))

	// 不应触发 MarkInstalled
	if err := m.Install(context.Background(), name); err != nil {
		t.Fatalf("Install (idempotent path) returned error: %v", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("sqlmock expectations: %v", err)
	}
}

func TestListPluginsExtFiltersUninstalled(t *testing.T) {
	t.Parallel()
	m, mock, cleanup := newRepoTestManager(t)
	defer cleanup()

	// 默认 (include=false) 只返回 active 行
	mock.ExpectQuery(queryListActive).
		WillReturnRows(sqlmock.NewRows(pluginColumns).
			AddRow(activeRow("alpha", true)...))

	infosActive, err := m.ListPluginsExt(context.Background(), false)
	if err != nil {
		t.Fatalf("ListPluginsExt(false): %v", err)
	}
	if len(infosActive) != 1 || infosActive[0].Name != "alpha" {
		t.Fatalf("active list: %+v", infosActive)
	}
	if infosActive[0].UninstalledAt != nil {
		t.Fatalf("active row should not carry uninstalled_at, got %v", infosActive[0].UninstalledAt)
	}

	// include=true 时返回全部
	mock.ExpectQuery(queryListAll).
		WillReturnRows(sqlmock.NewRows(pluginColumns).
			AddRow(activeRow("alpha", true)...).
			AddRow(uninstalledRow("beta")...))

	infosAll, err := m.ListPluginsExt(context.Background(), true)
	if err != nil {
		t.Fatalf("ListPluginsExt(true): %v", err)
	}
	if len(infosAll) != 2 {
		t.Fatalf("expected 2 rows, got: %+v", infosAll)
	}
	var foundUninstalled bool
	for _, info := range infosAll {
		if info.Name == "beta" {
			if info.UninstalledAt == nil {
				t.Fatalf("beta should carry uninstalled_at != nil")
			}
			foundUninstalled = true
		}
	}
	if !foundUninstalled {
		t.Fatalf("uninstalled row not surfaced in include=true list")
	}

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("sqlmock expectations: %v", err)
	}
}

// TestUninstallInvalidatesFrontendCache verifies the frontend cache callback
// fires exactly once per Uninstall — the SSR HTML must drop the plugin's
// sidebar / route entries on the next render.
func TestUninstallInvalidatesFrontendCache(t *testing.T) {
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
		WillReturnRows(sqlmock.NewRows(pluginColumns).AddRow(activeRow(name, false)...))
	mock.ExpectExec(queryMarkUninstalled).
		WithArgs(name).
		WillReturnResult(sqlmock.NewResult(0, 1))

	if err := m.Uninstall(context.Background(), name); err != nil {
		t.Fatalf("Uninstall: %v", err)
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
