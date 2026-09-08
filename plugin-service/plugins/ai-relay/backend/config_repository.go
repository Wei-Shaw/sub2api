package backend

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/Wei-Shaw/sub2api/plugin-service/internal/config"

	_ "github.com/lib/pq"
)

type SQLRouteRepository struct {
	db *sql.DB
}

func NewSQLRouteRepository(db *sql.DB) *SQLRouteRepository {
	return &SQLRouteRepository{db: db}
}

func NewRouteRepository(database config.DatabaseConfig) (RouteRepository, error) {
	if !database.Enabled {
		return NewMemoryRouteRepository(), nil
	}
	db, err := sql.Open("postgres", database.DSN())
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(4)
	db.SetMaxIdleConns(1)
	db.SetConnMaxLifetime(30 * time.Minute)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := db.PingContext(ctx); err != nil {
		_ = db.Close()
		return nil, err
	}
	if err := EnsureRouteSchema(ctx, db); err != nil {
		_ = db.Close()
		return nil, err
	}
	return NewSQLRouteRepository(db), nil
}

func EnsureRouteSchema(ctx context.Context, db *sql.DB) error {
	if db == nil {
		return errors.New("nil relay route database")
	}
	_, err := db.ExecContext(ctx, `CREATE TABLE IF NOT EXISTS plugin_ai_relay_routes (
		platform TEXT NOT NULL,
		slug TEXT NOT NULL,
		name TEXT NOT NULL DEFAULT '',
		base_url TEXT NOT NULL,
		created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
		updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
		PRIMARY KEY (platform, slug)
	)`)
	if err != nil {
		return err
	}
	if _, err := db.ExecContext(ctx, `ALTER TABLE plugin_ai_relay_routes ADD COLUMN IF NOT EXISTS name TEXT NOT NULL DEFAULT ''`); err != nil {
		return err
	}
	if _, err := db.ExecContext(ctx, `ALTER TABLE plugin_ai_relay_routes ADD COLUMN IF NOT EXISTS path_mappings JSONB NOT NULL DEFAULT '{}'::jsonb`); err != nil {
		return err
	}
	for _, column := range []string{"default_model", "model_map", "quality_map", "max_n", "enabled"} {
		if _, err := db.ExecContext(ctx, `ALTER TABLE plugin_ai_relay_routes DROP COLUMN IF EXISTS `+column); err != nil {
			return err
		}
	}
	return nil
}

func (r *SQLRouteRepository) Get(ctx context.Context, platform, slug string) (RouteConfig, bool, error) {
	row := r.db.QueryRowContext(ctx, routeSelectSQL+` WHERE platform = $1 AND slug = $2`, strings.ToLower(strings.TrimSpace(platform)), strings.ToLower(strings.TrimSpace(slug)))
	config, err := scanRouteConfig(row)
	if errors.Is(err, sql.ErrNoRows) {
		return RouteConfig{}, false, nil
	}
	if err != nil {
		return RouteConfig{}, false, err
	}
	return config, true, nil
}

func (r *SQLRouteRepository) List(ctx context.Context, filter RouteQuery) ([]RouteConfig, error) {
	query := routeSelectSQL
	args := []any{}
	conditions := []string{}
	if platform := strings.ToLower(strings.TrimSpace(filter.Platform)); platform != "" {
		args = append(args, platform)
		conditions = append(conditions, fmt.Sprintf("platform = $%d", len(args)))
	}
	if search := strings.TrimSpace(filter.Search); search != "" {
		args = append(args, "%"+search+"%")
		conditions = append(conditions, fmt.Sprintf("(name ILIKE $%d OR slug ILIKE $%d)", len(args), len(args)))
	}
	if len(conditions) > 0 {
		query += " WHERE " + strings.Join(conditions, " AND ")
	}
	query += ` ORDER BY platform, slug`
	rows, err := r.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	configs := []RouteConfig{}
	for rows.Next() {
		config, err := scanRouteConfig(rows)
		if err != nil {
			return nil, err
		}
		configs = append(configs, config)
	}
	return configs, rows.Err()
}

func (r *SQLRouteRepository) Upsert(ctx context.Context, config RouteConfig) (RouteConfig, error) {
	normalized, err := NormalizeRouteConfig(config)
	if err != nil {
		return RouteConfig{}, err
	}
	pathMappings, err := json.Marshal(normalized.PathMappings)
	if err != nil {
		return RouteConfig{}, err
	}
	row := r.db.QueryRowContext(ctx, `INSERT INTO plugin_ai_relay_routes (
		platform, slug, name, base_url, path_mappings
	) VALUES ($1, $2, $3, $4, $5::jsonb)
	ON CONFLICT (platform, slug) DO UPDATE SET
		name = EXCLUDED.name,
		base_url = EXCLUDED.base_url,
		path_mappings = EXCLUDED.path_mappings,
		updated_at = NOW()
	RETURNING platform, slug, name, base_url, path_mappings`,
		normalized.Platform,
		normalized.Slug,
		normalized.Name,
		normalized.BaseURL,
		string(pathMappings),
	)
	return scanRouteConfig(row)
}

func (r *SQLRouteRepository) Delete(ctx context.Context, platform, slug string) error {
	result, err := r.db.ExecContext(ctx, `DELETE FROM plugin_ai_relay_routes WHERE platform = $1 AND slug = $2`, strings.ToLower(strings.TrimSpace(platform)), strings.ToLower(strings.TrimSpace(slug)))
	if err != nil {
		return err
	}
	count, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if count == 0 {
		return ErrRouteNotFound
	}
	return nil
}

func (r *SQLRouteRepository) DeleteMany(ctx context.Context, routes []RouteReference) error {
	if len(routes) == 0 {
		return ErrInvalidRouteConfig
	}
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	seen := make(map[string]struct{}, len(routes))
	for _, route := range routes {
		platform := strings.ToLower(strings.TrimSpace(route.Platform))
		slug := strings.ToLower(strings.TrimSpace(route.Slug))
		key := routeKey(platform, slug)
		if _, ok := seen[key]; ok || !routeSlugPattern.MatchString(platform) || !routeSlugPattern.MatchString(slug) {
			return ErrInvalidRouteConfig
		}
		result, err := tx.ExecContext(ctx, `DELETE FROM plugin_ai_relay_routes WHERE platform = $1 AND slug = $2`, platform, slug)
		if err != nil {
			return err
		}
		count, err := result.RowsAffected()
		if err != nil {
			return err
		}
		if count == 0 {
			return ErrRouteNotFound
		}
		seen[key] = struct{}{}
	}
	return tx.Commit()
}

const routeSelectSQL = `SELECT platform, slug, name, base_url, path_mappings FROM plugin_ai_relay_routes`

type rowScanner interface {
	Scan(dest ...any) error
}

func scanRouteConfig(row rowScanner) (RouteConfig, error) {
	var config RouteConfig
	var pathMappings []byte
	if err := row.Scan(&config.Platform, &config.Slug, &config.Name, &config.BaseURL, &pathMappings); err != nil {
		return RouteConfig{}, err
	}
	config.PathMappings = map[string]string{}
	if len(pathMappings) > 0 {
		if err := json.Unmarshal(pathMappings, &config.PathMappings); err != nil {
			return RouteConfig{}, err
		}
	}
	config.PathMappings = copyPathMappings(config.PathMappings)
	return config, nil
}
