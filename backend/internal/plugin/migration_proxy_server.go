package plugin

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"errors"
	"fmt"
	"log/slog"
	"strings"

	pluginsdk "github.com/Wei-Shaw/sub2api/plugin-sdk/proto/pluginsdk"
)

// fetchAndRunPluginMigrations drives the host side of the W1 migration flow.
//
// The plugin declares the SQL migration files it ships in
// ManifestResponse.migrations (each entry pinning a sha256 of the SQL body).
// The host pulls each body via PluginLifecycle.GetMigration, re-verifies the
// checksum, and then hands the bodies off to RunPluginMigrations which owns
// the advisory lock + plugin_migrations bookkeeping.
//
// Returning an error blocks plugin startup (V5-CURATE Q3). Operators can opt
// out of the run by setting the plugin record's `skip_migration` config to
// "true" — see runPluginMigrationsForInstance below for the entry point.
//
// fetchAndRunPluginMigrations is intentionally tolerant of legacy plugin
// binaries that only populate the deprecated migration_files field: when the
// new `migrations` slice is empty there is nothing the host can fetch, so we
// just log and return nil. The legacy slice is informational only — the host
// has no SQL body to apply for those entries.
func fetchAndRunPluginMigrations(
	ctx context.Context,
	db *sql.DB,
	lifecycle pluginsdk.PluginLifecycleClient,
	manifest *pluginsdk.ManifestResponse,
	pluginName string,
	logger *slog.Logger,
) error {
	if db == nil {
		return errors.New("nil sql db")
	}
	if lifecycle == nil {
		return errors.New("nil lifecycle client")
	}
	if manifest == nil {
		return nil
	}
	decls := manifest.GetMigrations()
	if len(decls) == 0 {
		// Old plugin: only `migration_files` populated. We have no way to
		// fetch the SQL body, so leave the legacy log behaviour for the
		// caller and return.
		return nil
	}
	files, err := buildPluginMigrationFiles(ctx, lifecycle, decls, pluginName)
	if err != nil {
		return err
	}
	if logger != nil {
		logger.Info("running plugin migrations",
			"plugin", pluginName,
			"count", len(files),
		)
	}
	if err := RunPluginMigrations(ctx, db, pluginName, files); err != nil {
		return fmt.Errorf("apply plugin migrations: %w", err)
	}
	return nil
}

// buildPluginMigrationFiles drives the per-migration fetch+verify dance:
// for each declaration in the manifest it pulls the up SQL body (always)
// and the optional down SQL body, re-hashes both, and assembles a
// MigrationFile slice in the order RunPluginMigrations expects. Any
// failure short-circuits the run — partial caching is intentional (the
// plugin migration table tracks each filename independently).
func buildPluginMigrationFiles(
	ctx context.Context,
	lifecycle pluginsdk.PluginLifecycleClient,
	decls []*pluginsdk.MigrationDecl,
	pluginName string,
) ([]MigrationFile, error) {
	files := make([]MigrationFile, 0, len(decls))
	for _, decl := range decls {
		file, err := fetchOnePluginMigration(ctx, lifecycle, decl, pluginName)
		if err != nil {
			return nil, err
		}
		files = append(files, file)
	}
	return files, nil
}

// fetchOnePluginMigration handles a single MigrationDecl: validates the
// up declaration, pulls + verifies the up SQL body, then optionally
// pulls + verifies the matching down body. Returns a fully populated
// MigrationFile on success.
func fetchOnePluginMigration(
	ctx context.Context,
	lifecycle pluginsdk.PluginLifecycleClient,
	decl *pluginsdk.MigrationDecl,
	pluginName string,
) (MigrationFile, error) {
	filename := decl.GetFilename()
	if filename == "" {
		return MigrationFile{}, fmt.Errorf("plugin %s declared migration with empty filename", pluginName)
	}
	body, err := fetchAndVerifyUpMigration(ctx, lifecycle, decl, pluginName, filename)
	if err != nil {
		return MigrationFile{}, err
	}
	downFilename, downBody, downChecksum, err := fetchAndVerifyDownMigration(ctx, lifecycle, decl, pluginName, filename)
	if err != nil {
		return MigrationFile{}, err
	}
	return MigrationFile{
		Filename:           filename,
		Content:            body,
		NonTransactional:   decl.GetNonTransactional(),
		DownFilename:       downFilename,
		DownChecksumSha256: downChecksum,
		DownSQL:            downBody,
	}, nil
}

// fetchAndVerifyUpMigration pulls the up SQL body via PluginLifecycle,
// verifies its sha256 against both the manifest pin (authoritative) and
// the SDK-reported checksum (defence in depth: a disagreement means the
// SDK and host built the hex from different bytes — plugin bug).
func fetchAndVerifyUpMigration(
	ctx context.Context,
	lifecycle pluginsdk.PluginLifecycleClient,
	decl *pluginsdk.MigrationDecl,
	pluginName, filename string,
) ([]byte, error) {
	expected := strings.ToLower(strings.TrimSpace(decl.GetChecksumSha256()))
	if expected == "" {
		return nil, fmt.Errorf("plugin %s migration %s: empty checksum_sha256 in manifest", pluginName, filename)
	}
	resp, err := lifecycle.GetMigration(ctx, &pluginsdk.GetMigrationRequest{Filename: filename})
	if err != nil {
		return nil, fmt.Errorf("fetch plugin migration %s/%s: %w", pluginName, filename, err)
	}
	body := resp.GetSql()
	if len(body) == 0 {
		return nil, fmt.Errorf("plugin %s migration %s: empty SQL body returned by GetMigration", pluginName, filename)
	}
	actual := sha256.Sum256(body)
	actualHex := hex.EncodeToString(actual[:])
	if !strings.EqualFold(actualHex, expected) {
		return nil, fmt.Errorf(
			"plugin %s migration %s: checksum mismatch (manifest=%s body=%s) — refusing to apply",
			pluginName, filename, expected, actualHex,
		)
	}
	if sdkSum := strings.ToLower(strings.TrimSpace(resp.GetChecksumSha256())); sdkSum != "" && sdkSum != actualHex {
		return nil, fmt.Errorf(
			"plugin %s migration %s: SDK-reported checksum %s does not match body sha256 %s",
			pluginName, filename, sdkSum, actualHex,
		)
	}
	return body, nil
}

// fetchAndVerifyDownMigration pulls the optional down SQL body. Returns
// ("", nil, "", nil) when the manifest does not declare a down filename.
// When declared, the down checksum pin is mandatory and the body is
// re-hashed before being cached for later rollback use.
func fetchAndVerifyDownMigration(
	ctx context.Context,
	lifecycle pluginsdk.PluginLifecycleClient,
	decl *pluginsdk.MigrationDecl,
	pluginName, upFilename string,
) (downFilename string, downBody []byte, downChecksum string, err error) {
	downFilename = strings.TrimSpace(decl.GetDownFilename())
	if downFilename == "" {
		return "", nil, "", nil
	}
	downExpected := strings.ToLower(strings.TrimSpace(decl.GetDownChecksumSha256()))
	if downExpected == "" {
		return "", nil, "", fmt.Errorf("plugin %s migration %s: down_filename %q declared without down_checksum_sha256",
			pluginName, upFilename, downFilename)
	}
	downResp, err := lifecycle.GetMigration(ctx, &pluginsdk.GetMigrationRequest{Filename: downFilename})
	if err != nil {
		return "", nil, "", fmt.Errorf("fetch plugin down migration %s/%s: %w", pluginName, downFilename, err)
	}
	downBody = downResp.GetSql()
	if len(downBody) == 0 {
		return "", nil, "", fmt.Errorf("plugin %s down migration %s: empty SQL body returned by GetMigration",
			pluginName, downFilename)
	}
	downActual := sha256.Sum256(downBody)
	downActualHex := hex.EncodeToString(downActual[:])
	if !strings.EqualFold(downActualHex, downExpected) {
		return "", nil, "", fmt.Errorf(
			"plugin %s down migration %s: checksum mismatch (manifest=%s body=%s) — refusing to cache",
			pluginName, downFilename, downExpected, downActualHex,
		)
	}
	return downFilename, downBody, downActualHex, nil
}

// pluginConfigSkipMigrationKey is read from PluginRecord.Config to let an
// operator bypass migration execution for a specific plugin without
// rebuilding it. Value comparison is strict-equal to "true" so accidental
// truthy strings ("yes", "1", etc.) do not silently skip.
const pluginConfigSkipMigrationKey = "skip_migration"

// shouldSkipPluginMigrations returns true when the operator has explicitly
// set the `skip_migration` config key to "true" on the plugin record. This
// is the V5-CURATE escape hatch for unblocking a plugin whose declared
// migrations are broken in production while a real fix is being prepared.
func shouldSkipPluginMigrations(pluginConfig map[string]string) bool {
	if len(pluginConfig) == 0 {
		return false
	}
	return strings.EqualFold(pluginConfig[pluginConfigSkipMigrationKey], "true")
}
