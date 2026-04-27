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

	files := make([]MigrationFile, 0, len(decls))
	for _, decl := range decls {
		filename := decl.GetFilename()
		if filename == "" {
			return fmt.Errorf("plugin %s declared migration with empty filename", pluginName)
		}
		expected := strings.ToLower(strings.TrimSpace(decl.GetChecksumSha256()))
		if expected == "" {
			return fmt.Errorf("plugin %s migration %s: empty checksum_sha256 in manifest", pluginName, filename)
		}

		resp, err := lifecycle.GetMigration(ctx, &pluginsdk.GetMigrationRequest{Filename: filename})
		if err != nil {
			return fmt.Errorf("fetch plugin migration %s/%s: %w", pluginName, filename, err)
		}
		body := resp.GetSql()
		if len(body) == 0 {
			return fmt.Errorf("plugin %s migration %s: empty SQL body returned by GetMigration", pluginName, filename)
		}

		actual := sha256.Sum256(body)
		actualHex := hex.EncodeToString(actual[:])
		if !strings.EqualFold(actualHex, expected) {
			return fmt.Errorf(
				"plugin %s migration %s: checksum mismatch (manifest=%s body=%s) — refusing to apply",
				pluginName, filename, expected, actualHex,
			)
		}
		// Honour the SDK-side checksum if the plugin populated it, but the
		// manifest declaration is the authoritative pin: we already compared
		// against it above, so any disagreement here is a plugin bug.
		if sdkSum := strings.ToLower(strings.TrimSpace(resp.GetChecksumSha256())); sdkSum != "" && sdkSum != actualHex {
			return fmt.Errorf(
				"plugin %s migration %s: SDK-reported checksum %s does not match body sha256 %s",
				pluginName, filename, sdkSum, actualHex,
			)
		}

		files = append(files, MigrationFile{
			Filename:         filename,
			Content:          body,
			NonTransactional: decl.GetNonTransactional(),
		})
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
