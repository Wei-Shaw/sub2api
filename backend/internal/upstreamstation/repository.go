package upstreamstation

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"time"
)

var ErrNotFound = errors.New("upstream station resource not found")

type Repository struct {
	db *sql.DB
}

func NewRepository(db *sql.DB) *Repository {
	return &Repository{db: db}
}

func (r *Repository) CreateStation(ctx context.Context, station *Station) (*Station, error) {
	if station.RechargeMultiplier <= 0 {
		station.RechargeMultiplier = 1
	}
	if station.RechargeSource == "" {
		station.RechargeSource = RechargeSourceManual
	}
	err := r.db.QueryRowContext(ctx, `
		INSERT INTO upstream_stations (
			name, site_type, base_url, credential_mode, credential_cipher,
			recharge_multiplier, recharge_source, enabled, auto_sync
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
		RETURNING id, created_at, updated_at
	`, station.Name, station.SiteType, station.BaseURL, station.CredentialMode, station.CredentialCipher,
		station.RechargeMultiplier, station.RechargeSource, station.Enabled, station.AutoSync,
	).Scan(&station.ID, &station.CreatedAt, &station.UpdatedAt)
	if err != nil {
		return nil, fmt.Errorf("create upstream station: %w", err)
	}
	station.CredentialConfigured = station.CredentialCipher != ""
	return station, nil
}

func (r *Repository) GetStation(ctx context.Context, id int64) (*Station, error) {
	row := r.db.QueryRowContext(ctx, stationSelect+` WHERE id = $1`, id)
	station, err := scanStation(row)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	return station, err
}

func (r *Repository) ListStations(ctx context.Context) ([]Station, error) {
	rows, err := r.db.QueryContext(ctx, stationSelect+` ORDER BY name ASC, id ASC`)
	if err != nil {
		return nil, fmt.Errorf("list upstream stations: %w", err)
	}
	defer func() { _ = rows.Close() }()
	list := make([]Station, 0)
	for rows.Next() {
		station, scanErr := scanStation(rows)
		if scanErr != nil {
			return nil, scanErr
		}
		list = append(list, *station)
	}
	return list, rows.Err()
}

func (r *Repository) UpdateStation(ctx context.Context, station *Station) error {
	if station.RechargeMultiplier <= 0 {
		station.RechargeMultiplier = 1
	}
	result, err := r.db.ExecContext(ctx, `
		UPDATE upstream_stations SET
			name = $2, site_type = $3, base_url = $4, credential_mode = $5,
			credential_cipher = CASE WHEN $6 = '' THEN credential_cipher ELSE $6 END,
			recharge_multiplier = $7, recharge_source = $8, enabled = $9,
			auto_sync = $10, updated_at = NOW()
		WHERE id = $1
	`, station.ID, station.Name, station.SiteType, station.BaseURL, station.CredentialMode,
		station.CredentialCipher, station.RechargeMultiplier, station.RechargeSource, station.Enabled, station.AutoSync)
	if err != nil {
		return fmt.Errorf("update upstream station: %w", err)
	}
	if count, _ := result.RowsAffected(); count == 0 {
		return ErrNotFound
	}
	return nil
}

func (r *Repository) DeleteStation(ctx context.Context, id int64) error {
	result, err := r.db.ExecContext(ctx, `DELETE FROM upstream_stations WHERE id = $1`, id)
	if err != nil {
		return fmt.Errorf("delete upstream station: %w", err)
	}
	if count, _ := result.RowsAffected(); count == 0 {
		return ErrNotFound
	}
	return nil
}

func (r *Repository) UpdateStationObservation(ctx context.Context, id int64, balance *float64, rechargeMultiplier float64, health, lastError string, tested, synced bool) error {
	now := time.Now()
	var testAt, syncAt *time.Time
	if tested {
		testAt = &now
	}
	if synced {
		syncAt = &now
	}
	_, err := r.db.ExecContext(ctx, `
		UPDATE upstream_stations SET balance = COALESCE($2, balance),
			recharge_multiplier = CASE WHEN $3 > 0 THEN $3 ELSE recharge_multiplier END,
			health_status = $4, last_error = $5,
			last_test_at = COALESCE($6, last_test_at), last_sync_at = COALESCE($7, last_sync_at),
			updated_at = NOW() WHERE id = $1
	`, id, balance, rechargeMultiplier, health, lastError, testAt, syncAt)
	return err
}

func (r *Repository) UpsertRoute(ctx context.Context, route *Route) (*Route, error) {
	models, err := json.Marshal(route.Models)
	if err != nil {
		return nil, fmt.Errorf("encode upstream route models: %w", err)
	}
	if route.RechargeMultiplier <= 0 {
		route.RechargeMultiplier = 1
	}
	route.EffectiveRate = EffectiveRate(route.GroupRate, route.RechargeMultiplier)
	err = r.db.QueryRowContext(ctx, `
		INSERT INTO upstream_routes (
			station_id, remote_group_key, remote_group_name, platform, models,
			group_rate, recharge_multiplier, effective_rate, fixed_route,
			remote_api_key_id, api_key_cipher, managed_account_id, schedulable,
			health_status, last_error, last_sync_at
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, NOW())
		ON CONFLICT (station_id, remote_group_key, platform) DO UPDATE SET
			remote_group_name = EXCLUDED.remote_group_name,
			models = EXCLUDED.models,
			group_rate = EXCLUDED.group_rate,
			recharge_multiplier = EXCLUDED.recharge_multiplier,
			effective_rate = EXCLUDED.effective_rate,
			fixed_route = EXCLUDED.fixed_route,
			remote_api_key_id = CASE WHEN EXCLUDED.remote_api_key_id = '' THEN upstream_routes.remote_api_key_id ELSE EXCLUDED.remote_api_key_id END,
			api_key_cipher = CASE WHEN EXCLUDED.api_key_cipher = '' THEN upstream_routes.api_key_cipher ELSE EXCLUDED.api_key_cipher END,
			managed_account_id = COALESCE(EXCLUDED.managed_account_id, upstream_routes.managed_account_id),
			schedulable = EXCLUDED.schedulable,
			health_status = EXCLUDED.health_status,
			last_error = EXCLUDED.last_error,
			last_sync_at = NOW(), updated_at = NOW()
		RETURNING id, created_at, updated_at, last_sync_at
	`, route.StationID, route.RemoteGroupKey, route.RemoteGroupName, route.Platform, models,
		route.GroupRate, route.RechargeMultiplier, route.EffectiveRate, route.FixedRoute,
		route.RemoteAPIKeyID, route.APIKeyCipher, route.ManagedAccountID, route.Schedulable,
		route.HealthStatus, route.LastError,
	).Scan(&route.ID, &route.CreatedAt, &route.UpdatedAt, &route.LastSyncAt)
	if err != nil {
		return nil, fmt.Errorf("upsert upstream route: %w", err)
	}
	return route, nil
}

func (r *Repository) GetRoute(ctx context.Context, id int64) (*Route, error) {
	route, err := scanRoute(r.db.QueryRowContext(ctx, routeSelect+` WHERE id = $1`, id))
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	return route, err
}

func (r *Repository) ListRoutes(ctx context.Context, stationID int64) ([]Route, error) {
	rows, err := r.db.QueryContext(ctx, routeSelect+` WHERE station_id = $1 ORDER BY effective_rate ASC, id ASC`, stationID)
	if err != nil {
		return nil, fmt.Errorf("list upstream routes: %w", err)
	}
	defer func() { _ = rows.Close() }()
	list := make([]Route, 0)
	for rows.Next() {
		route, scanErr := scanRoute(rows)
		if scanErr != nil {
			return nil, scanErr
		}
		list = append(list, *route)
	}
	return list, rows.Err()
}

func (r *Repository) SetRouteSchedulable(ctx context.Context, id int64, schedulable bool) error {
	result, err := r.db.ExecContext(ctx, `UPDATE upstream_routes SET schedulable = $2, updated_at = NOW() WHERE id = $1`, id, schedulable)
	if err != nil {
		return err
	}
	if count, _ := result.RowsAffected(); count == 0 {
		return ErrNotFound
	}
	return nil
}

func (r *Repository) UpdateRouteManagedAccount(ctx context.Context, id, accountID int64) error {
	_, err := r.db.ExecContext(ctx, `UPDATE upstream_routes SET managed_account_id = $2, updated_at = NOW() WHERE id = $1`, id, accountID)
	return err
}

func (r *Repository) AppendRateSnapshot(ctx context.Context, snapshot RateSnapshot) error {
	_, err := r.db.ExecContext(ctx, `
		INSERT INTO upstream_rate_snapshots (route_id, group_rate, recharge_multiplier, effective_rate, sampled_at)
		VALUES ($1, $2, $3, $4, COALESCE($5, NOW()))
	`, snapshot.RouteID, snapshot.GroupRate, snapshot.RechargeMultiplier, snapshot.EffectiveRate, nullableTime(snapshot.SampledAt))
	return err
}

func (r *Repository) AppendSyncLog(ctx context.Context, item SyncLog) error {
	_, err := r.db.ExecContext(ctx, `
		INSERT INTO upstream_sync_logs (station_id, action, success, message, detail, created_at)
		VALUES ($1, $2, $3, $4, $5, COALESCE($6, NOW()))
	`, item.StationID, item.Action, item.Success, item.Message, item.Detail, nullableTime(item.CreatedAt))
	return err
}

func (r *Repository) ListSyncLogs(ctx context.Context, stationID int64, limit int) ([]SyncLog, error) {
	if limit <= 0 || limit > 200 {
		limit = 50
	}
	rows, err := r.db.QueryContext(ctx, `
		SELECT id, station_id, action, success, message, detail, created_at
		FROM upstream_sync_logs WHERE station_id = $1 ORDER BY created_at DESC LIMIT $2
	`, stationID, limit)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	list := make([]SyncLog, 0)
	for rows.Next() {
		var item SyncLog
		if err := rows.Scan(&item.ID, &item.StationID, &item.Action, &item.Success, &item.Message, &item.Detail, &item.CreatedAt); err != nil {
			return nil, err
		}
		list = append(list, item)
	}
	return list, rows.Err()
}

const stationSelect = `SELECT id, name, site_type, base_url, credential_mode, credential_cipher,
	recharge_multiplier, recharge_source, balance, enabled, auto_sync, health_status,
	last_error, last_sync_at, last_test_at, created_at, updated_at FROM upstream_stations`

type scanner interface {
	Scan(dest ...any) error
}

func scanStation(row scanner) (*Station, error) {
	var station Station
	var balance sql.NullFloat64
	var syncAt, testAt sql.NullTime
	if err := row.Scan(&station.ID, &station.Name, &station.SiteType, &station.BaseURL,
		&station.CredentialMode, &station.CredentialCipher, &station.RechargeMultiplier,
		&station.RechargeSource, &balance, &station.Enabled, &station.AutoSync,
		&station.HealthStatus, &station.LastError, &syncAt, &testAt,
		&station.CreatedAt, &station.UpdatedAt); err != nil {
		return nil, err
	}
	station.CredentialConfigured = station.CredentialCipher != ""
	if balance.Valid {
		station.Balance = &balance.Float64
	}
	if syncAt.Valid {
		station.LastSyncAt = &syncAt.Time
	}
	if testAt.Valid {
		station.LastTestAt = &testAt.Time
	}
	return &station, nil
}

const routeSelect = `SELECT id, station_id, remote_group_key, remote_group_name, platform, models,
	group_rate, recharge_multiplier, effective_rate, fixed_route, remote_api_key_id,
	api_key_cipher, managed_account_id, schedulable, health_status, last_error,
	last_test_at, last_sync_at, created_at, updated_at FROM upstream_routes`

func scanRoute(row scanner) (*Route, error) {
	var route Route
	var models []byte
	var accountID sql.NullInt64
	var testAt, syncAt sql.NullTime
	if err := row.Scan(&route.ID, &route.StationID, &route.RemoteGroupKey,
		&route.RemoteGroupName, &route.Platform, &models, &route.GroupRate,
		&route.RechargeMultiplier, &route.EffectiveRate, &route.FixedRoute,
		&route.RemoteAPIKeyID, &route.APIKeyCipher, &accountID, &route.Schedulable,
		&route.HealthStatus, &route.LastError, &testAt, &syncAt,
		&route.CreatedAt, &route.UpdatedAt); err != nil {
		return nil, err
	}
	if len(models) > 0 {
		if err := json.Unmarshal(models, &route.Models); err != nil {
			return nil, fmt.Errorf("decode upstream route models: %w", err)
		}
	}
	if route.Models == nil {
		route.Models = []string{}
	}
	if accountID.Valid {
		route.ManagedAccountID = &accountID.Int64
	}
	if testAt.Valid {
		route.LastTestAt = &testAt.Time
	}
	if syncAt.Valid {
		route.LastSyncAt = &syncAt.Time
	}
	return &route, nil
}

func nullableTime(value time.Time) any {
	if value.IsZero() {
		return nil
	}
	return value
}
