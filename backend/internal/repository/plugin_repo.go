package repository

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/service"
)

type pluginRepository struct {
	db *sql.DB
}

func NewPluginRepository(db *sql.DB) service.PluginRepository {
	return &pluginRepository{db: db}
}

func (r *pluginRepository) List(ctx context.Context) ([]*service.PluginInstallation, error) {
	rows, err := r.db.QueryContext(ctx, pluginSelectSQL+` ORDER BY installed_at DESC, id DESC`)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	plugins := make([]*service.PluginInstallation, 0)
	for rows.Next() {
		plugin, scanErr := scanPlugin(rows)
		if scanErr != nil {
			return nil, scanErr
		}
		plugins = append(plugins, plugin)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	for _, plugin := range plugins {
		bindings, bindingErr := r.listBindings(ctx, plugin.ID)
		if bindingErr != nil {
			return nil, bindingErr
		}
		plugin.Bindings = bindings
	}
	return plugins, nil
}

func (r *pluginRepository) GetByID(ctx context.Context, id int64) (*service.PluginInstallation, error) {
	plugin, err := scanPlugin(r.db.QueryRowContext(ctx, pluginSelectSQL+` WHERE id = $1`, id))
	if err != nil {
		return nil, err
	}
	plugin.Bindings, err = r.listBindings(ctx, plugin.ID)
	return plugin, err
}

func (r *pluginRepository) GetByKey(ctx context.Context, key string) (*service.PluginInstallation, error) {
	plugin, err := scanPlugin(r.db.QueryRowContext(ctx, pluginSelectSQL+` WHERE plugin_key = $1`, key))
	if err != nil {
		return nil, err
	}
	plugin.Bindings, err = r.listBindings(ctx, plugin.ID)
	return plugin, err
}

func (r *pluginRepository) Install(ctx context.Context, plugin *service.PluginInstallation, bindings []service.PluginBinding) (*service.PluginInstallation, error) {
	manifestJSON, err := json.Marshal(plugin.Manifest)
	if err != nil {
		return nil, fmt.Errorf("序列化插件清单: %w", err)
	}
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer func() { _ = tx.Rollback() }()
	row := tx.QueryRowContext(ctx, `
			INSERT INTO sub2api_plugin_installations (
				plugin_key, name, version, description, author, manifest, artifact_data,
				artifact_path, install_path, binary_path, binary_sha256,
				signature_status, state, last_error, installed_by, installed_at, updated_at
			) VALUES ($1, $2, $3, $4, $5, $6::jsonb, $7, $8, $9, $10, $11, $12, $13, '', $14, NOW(), NOW())
			ON CONFLICT (plugin_key) DO UPDATE SET
			name = EXCLUDED.name,
			version = EXCLUDED.version,
			description = EXCLUDED.description,
				author = EXCLUDED.author,
				manifest = EXCLUDED.manifest,
				artifact_data = EXCLUDED.artifact_data,
			artifact_path = EXCLUDED.artifact_path,
			install_path = EXCLUDED.install_path,
			binary_path = EXCLUDED.binary_path,
			binary_sha256 = EXCLUDED.binary_sha256,
			signature_status = EXCLUDED.signature_status,
			state = EXCLUDED.state,
			last_error = '',
			installed_by = EXCLUDED.installed_by,
			installed_at = NOW(),
			enabled_at = NULL,
			updated_at = NOW()
		WHERE sub2api_plugin_installations.state IN ('disabled', 'error', 'incompatible')
		  AND NOT EXISTS (
			SELECT 1 FROM sub2api_plugin_bindings b
			WHERE b.plugin_id = sub2api_plugin_installations.id AND b.enabled = TRUE
		  )
		RETURNING id
	`, plugin.PluginKey, plugin.Name, plugin.Version, plugin.Description, plugin.Author, manifestJSON, plugin.ArtifactData,
		plugin.ArtifactPath, plugin.InstallPath, plugin.BinaryPath, plugin.BinarySHA256,
		plugin.SignatureStatus, plugin.State, plugin.InstalledBy)
	var id int64
	if err := row.Scan(&id); err != nil {
		if err == sql.ErrNoRows {
			return nil, service.ErrPluginStateChanged
		}
		return nil, err
	}
	if err := replacePluginBindings(ctx, tx, id, bindings); err != nil {
		return nil, err
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return r.GetByID(ctx, id)
}

func (r *pluginRepository) GetArtifact(ctx context.Context, id int64) ([]byte, error) {
	var artifact []byte
	err := r.db.QueryRowContext(ctx, `SELECT artifact_data FROM sub2api_plugin_installations WHERE id = $1`, id).Scan(&artifact)
	return artifact, err
}

func (r *pluginRepository) Delete(ctx context.Context, id int64, expectedBinarySHA256 string) error {
	result, err := r.db.ExecContext(ctx, `
		DELETE FROM sub2api_plugin_installations p
		WHERE p.id = $1 AND p.binary_sha256 = $2 AND p.state NOT IN ('starting', 'enabled')
		  AND NOT EXISTS (SELECT 1 FROM sub2api_plugin_bindings b WHERE b.plugin_id = p.id AND b.enabled = TRUE)
	`, id, expectedBinarySHA256)
	if err != nil {
		return err
	}
	rows, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if rows != 1 {
		return service.ErrPluginStateChanged
	}
	return nil
}

func (r *pluginRepository) BeginEnable(ctx context.Context, id int64, binarySHA256, expectedState string) error {
	result, err := r.db.ExecContext(ctx, `
		UPDATE sub2api_plugin_installations
		SET state = 'starting', last_error = '', updated_at = NOW()
		WHERE id = $1 AND binary_sha256 = $2 AND state = $3 AND state <> 'starting'
	`, id, binarySHA256, expectedState)
	if err != nil {
		return err
	}
	rows, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if rows != 1 {
		return service.ErrPluginStateChanged
	}
	return nil
}

func (r *pluginRepository) MarkRuntimeHealthy(ctx context.Context, id int64, binarySHA256, configEncrypted string) error {
	result, err := r.db.ExecContext(ctx, `
		UPDATE sub2api_plugin_installations p
		SET state = 'enabled', last_error = '', enabled_at = COALESCE(enabled_at, NOW()), updated_at = NOW()
		WHERE p.id = $1 AND p.binary_sha256 = $2 AND p.config_encrypted = $3
		  AND EXISTS (SELECT 1 FROM sub2api_plugin_bindings b WHERE b.plugin_id = p.id AND b.enabled = TRUE)
	`, id, binarySHA256, configEncrypted)
	if err != nil {
		return err
	}
	rows, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if rows != 1 {
		return service.ErrPluginStateChanged
	}
	return nil
}

func (r *pluginRepository) UpdateState(ctx context.Context, id int64, state, lastError string, enabledAt *time.Time, expectedBinarySHA256, expectedState string) error {
	result, err := r.db.ExecContext(ctx, `
		UPDATE sub2api_plugin_installations
		SET state = $2, last_error = $3, enabled_at = $4, updated_at = NOW()
		WHERE id = $1 AND binary_sha256 = $5 AND state = $6
	`, id, state, lastError, enabledAt, expectedBinarySHA256, expectedState)
	if err != nil {
		return err
	}
	rows, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if rows != 1 {
		return service.ErrPluginStateChanged
	}
	return nil
}

func (r *pluginRepository) UpdateConfig(ctx context.Context, id int64, encrypted, expectedBinarySHA256 string) error {
	result, err := r.db.ExecContext(ctx, `
		UPDATE sub2api_plugin_installations
		SET config_encrypted = $2, updated_at = NOW()
		WHERE id = $1 AND binary_sha256 = $3
	`, id, encrypted, expectedBinarySHA256)
	if err != nil {
		return err
	}
	rows, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if rows != 1 {
		return service.ErrPluginStateChanged
	}
	return nil
}

func (r *pluginRepository) UpdateBindingsAndState(
	ctx context.Context,
	pluginID int64,
	bindings []service.PluginBinding,
	state string,
	lastError string,
	enabledAt *time.Time,
	expectedState string,
	expectedBinarySHA256 string,
) error {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	result, err := tx.ExecContext(ctx, `
		UPDATE sub2api_plugin_installations
		SET state = $2, last_error = $3, enabled_at = $4, updated_at = NOW()
		WHERE id = $1 AND ($5 = '' OR state = $5) AND binary_sha256 = $6
	`, pluginID, state, lastError, enabledAt, expectedState, expectedBinarySHA256)
	if err != nil {
		return err
	}
	rows, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if rows != 1 {
		return service.ErrPluginStateChanged
	}
	if err := replacePluginBindings(ctx, tx, pluginID, bindings); err != nil {
		return err
	}
	return tx.Commit()
}

type pluginBindingExecutor interface {
	ExecContext(ctx context.Context, query string, args ...any) (sql.Result, error)
}

func replacePluginBindings(ctx context.Context, executor pluginBindingExecutor, pluginID int64, bindings []service.PluginBinding) error {
	if _, err := executor.ExecContext(ctx, `DELETE FROM sub2api_plugin_bindings WHERE plugin_id = $1`, pluginID); err != nil {
		return err
	}
	for _, binding := range bindings {
		if _, err := executor.ExecContext(ctx, `
			INSERT INTO sub2api_plugin_bindings (
				plugin_id, capability, platform, account_type, enabled, rollout_percent, created_at, updated_at
			) VALUES ($1, $2, $3, $4, $5, $6, NOW(), NOW())
		`, pluginID, binding.Capability, binding.Platform, binding.AccountType, binding.Enabled, binding.RolloutPercent); err != nil {
			return err
		}
	}
	return nil
}

const pluginSelectSQL = `
	SELECT id, plugin_key, name, version, description, author, manifest,
	       artifact_path, install_path, binary_path, binary_sha256,
	       signature_status, state, config_encrypted, last_error,
	       installed_by, installed_at, enabled_at, updated_at
	FROM sub2api_plugin_installations`

type pluginScanner interface {
	Scan(dest ...any) error
}

func scanPlugin(scanner pluginScanner) (*service.PluginInstallation, error) {
	plugin := &service.PluginInstallation{}
	var manifestJSON []byte
	if err := scanner.Scan(
		&plugin.ID, &plugin.PluginKey, &plugin.Name, &plugin.Version, &plugin.Description,
		&plugin.Author, &manifestJSON, &plugin.ArtifactPath, &plugin.InstallPath,
		&plugin.BinaryPath, &plugin.BinarySHA256, &plugin.SignatureStatus, &plugin.State,
		&plugin.ConfigEncrypted, &plugin.LastError, &plugin.InstalledBy, &plugin.InstalledAt,
		&plugin.EnabledAt, &plugin.UpdatedAt,
	); err != nil {
		return nil, err
	}
	if err := json.Unmarshal(manifestJSON, &plugin.Manifest); err != nil {
		return nil, fmt.Errorf("解析插件清单: %w", err)
	}
	return plugin, nil
}

func (r *pluginRepository) listBindings(ctx context.Context, pluginID int64) ([]service.PluginBinding, error) {
	rows, err := r.db.QueryContext(ctx, `
		SELECT id, plugin_id, capability, platform, account_type, enabled,
		       rollout_percent, created_at, updated_at
		FROM sub2api_plugin_bindings WHERE plugin_id = $1 ORDER BY id
	`, pluginID)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	bindings := make([]service.PluginBinding, 0)
	for rows.Next() {
		var binding service.PluginBinding
		if err := rows.Scan(&binding.ID, &binding.PluginID, &binding.Capability, &binding.Platform,
			&binding.AccountType, &binding.Enabled, &binding.RolloutPercent,
			&binding.CreatedAt, &binding.UpdatedAt); err != nil {
			return nil, err
		}
		bindings = append(bindings, binding)
	}
	return bindings, rows.Err()
}

var _ service.PluginRepository = (*pluginRepository)(nil)
