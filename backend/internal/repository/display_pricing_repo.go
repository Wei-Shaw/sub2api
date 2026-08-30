package repository

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/shopspring/decimal"
)

type displayPricingRepository struct{ db *sql.DB }

func NewDisplayPricingRepository(db *sql.DB) service.DisplayPricingRepository {
	return &displayPricingRepository{db: db}
}

func (r *displayPricingRepository) GetSettings(ctx context.Context) (*service.DisplayPricingSettings, error) {
	out := &service.DisplayPricingSettings{}
	err := r.db.QueryRowContext(ctx, `SELECT global_multiplier, updated_at FROM display_pricing_settings WHERE id = 1`).Scan(&out.GlobalMultiplier, &out.UpdatedAt)
	if err != nil {
		return nil, fmt.Errorf("get display pricing settings: %w", err)
	}
	return out, nil
}

func (r *displayPricingRepository) UpdateSettings(ctx context.Context, settings *service.DisplayPricingSettings) error {
	return r.db.QueryRowContext(ctx, `
		INSERT INTO display_pricing_settings (id, global_multiplier, updated_at)
		VALUES (1, $1, NOW())
		ON CONFLICT (id) DO UPDATE SET global_multiplier = EXCLUDED.global_multiplier, updated_at = NOW()
		RETURNING updated_at`, settings.GlobalMultiplier).Scan(&settings.UpdatedAt)
}

func (r *displayPricingRepository) ListProviders(ctx context.Context) ([]service.DisplayPricingProvider, error) {
	rows, err := r.db.QueryContext(ctx, `SELECT provider, display_name, provider_note, per_request_note, image_note, currency, multiplier, logo_key, logo_url, sort_order, updated_at FROM display_pricing_providers ORDER BY sort_order, display_name`)
	if err != nil {
		return nil, fmt.Errorf("list display pricing providers: %w", err)
	}
	defer func() { _ = rows.Close() }()
	out := make([]service.DisplayPricingProvider, 0)
	for rows.Next() {
		var p service.DisplayPricingProvider
		var multiplier sql.NullFloat64
		if err := rows.Scan(&p.Provider, &p.DisplayName, &p.ProviderNote, &p.PerRequestNote, &p.ImageNote, &p.Currency, &multiplier, &p.LogoKey, &p.LogoURL, &p.SortOrder, &p.UpdatedAt); err != nil {
			return nil, err
		}
		p.Multiplier = nullFloatPtr(multiplier)
		out = append(out, p)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return out, nil
}

func (r *displayPricingRepository) CreateProvider(ctx context.Context, p *service.DisplayPricingProvider) error {
	err := r.db.QueryRowContext(ctx, `
		INSERT INTO display_pricing_providers (provider, display_name, provider_note, per_request_note, image_note, currency, multiplier, logo_key, logo_url, sort_order, updated_at)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,NOW())
		ON CONFLICT (provider) DO NOTHING
		RETURNING updated_at`, p.Provider, p.DisplayName, p.ProviderNote, p.PerRequestNote, p.ImageNote, p.Currency, p.Multiplier, p.LogoKey, p.LogoURL, p.SortOrder).Scan(&p.UpdatedAt)
	if err == sql.ErrNoRows {
		return service.ErrDisplayProviderExists
	}
	return err
}

func (r *displayPricingRepository) UpdateProvider(ctx context.Context, p *service.DisplayPricingProvider) error {
	err := r.db.QueryRowContext(ctx, `
		WITH updated_provider AS (
			UPDATE display_pricing_providers SET display_name=$2, provider_note=$3, per_request_note=$4, image_note=$5,
				currency=$6, multiplier=$7, logo_key=$8, logo_url=$9, sort_order=$10, updated_at=NOW()
			WHERE provider=$1 RETURNING updated_at
		), updated_models AS (
			UPDATE display_model_prices SET currency=$6, updated_at=NOW()
			WHERE provider=$1 AND currency<>$6 AND EXISTS (SELECT 1 FROM updated_provider)
		)
		SELECT updated_at FROM updated_provider`,
		p.Provider, p.DisplayName, p.ProviderNote, p.PerRequestNote, p.ImageNote, p.Currency, p.Multiplier, p.LogoKey, p.LogoURL, p.SortOrder).Scan(&p.UpdatedAt)
	if err == sql.ErrNoRows {
		return service.ErrDisplayProviderNotFound
	}
	return err
}

func (r *displayPricingRepository) DeleteProvider(ctx context.Context, provider string) (int64, error) {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return 0, err
	}
	defer func() { _ = tx.Rollback() }()

	// Lock the parent before counting so a concurrent child insert cannot make
	// deleted_models stale while the FK cascade is in progress.
	var lockedProvider string
	if err := tx.QueryRowContext(ctx, `SELECT provider FROM display_pricing_providers WHERE provider=$1 FOR UPDATE`, provider).Scan(&lockedProvider); err != nil {
		if err == sql.ErrNoRows {
			return 0, service.ErrDisplayProviderNotFound
		}
		return 0, err
	}
	var deletedModels int64
	if err := tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM display_model_prices WHERE provider=$1`, provider).Scan(&deletedModels); err != nil {
		return 0, err
	}
	if _, err := tx.ExecContext(ctx, `DELETE FROM display_pricing_providers WHERE provider=$1`, provider); err != nil {
		return 0, err
	}
	if err := tx.Commit(); err != nil {
		return 0, err
	}
	return deletedModels, nil
}

const displayModelSelect = `SELECT id, platform, model_name, provider, billing_mode, currency, enabled, sort_order,
	model_note,
	official_input_per_million, official_output_per_million, official_cache_write_per_million, official_cache_read_per_million,
	official_price_source, official_price_source_url, official_price_synced_at,
	model_multiplier, per_request_lte_256k, per_request_256k_512k_override, per_request_gt_512k_override,
	image_prices, created_at, updated_at FROM display_model_prices`

func (r *displayPricingRepository) ListModels(ctx context.Context) ([]service.DisplayModelPrice, error) {
	rows, err := r.db.QueryContext(ctx, displayModelSelect+` ORDER BY sort_order, lower(model_name), id`)
	if err != nil {
		return nil, fmt.Errorf("list display model prices: %w", err)
	}
	defer func() { _ = rows.Close() }()
	out := make([]service.DisplayModelPrice, 0)
	for rows.Next() {
		p, err := scanDisplayModel(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *p)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return out, nil
}

func (r *displayPricingRepository) GetModel(ctx context.Context, id int64) (*service.DisplayModelPrice, error) {
	p, err := scanDisplayModel(r.db.QueryRowContext(ctx, displayModelSelect+` WHERE id=$1`, id))
	if err == sql.ErrNoRows {
		return nil, service.ErrDisplayPriceNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("get display model price: %w", err)
	}
	return p, nil
}

func (r *displayPricingRepository) UpsertModel(ctx context.Context, p *service.DisplayModelPrice) error {
	imageJSON, err := json.Marshal(p.ImagePrices)
	if err != nil {
		return fmt.Errorf("marshal display image prices: %w", err)
	}
	return r.db.QueryRowContext(ctx, `
		INSERT INTO display_model_prices (
			platform, model_name, provider, billing_mode, currency, enabled, sort_order, model_note,
			official_input_per_million, official_output_per_million, official_cache_write_per_million, official_cache_read_per_million,
			official_price_source, official_price_source_url, official_price_synced_at,
			model_multiplier, per_request_lte_256k, per_request_256k_512k_override, per_request_gt_512k_override, image_prices,
			created_at, updated_at)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16,$17,$18,$19,$20,NOW(),NOW())
		ON CONFLICT (platform, model_name, billing_mode) DO UPDATE SET
			provider=EXCLUDED.provider, currency=EXCLUDED.currency, enabled=EXCLUDED.enabled, sort_order=EXCLUDED.sort_order,
			model_note=EXCLUDED.model_note,
			official_input_per_million=EXCLUDED.official_input_per_million,
			official_output_per_million=EXCLUDED.official_output_per_million,
			official_cache_write_per_million=EXCLUDED.official_cache_write_per_million,
			official_cache_read_per_million=EXCLUDED.official_cache_read_per_million,
			official_price_source=EXCLUDED.official_price_source,
			official_price_source_url=EXCLUDED.official_price_source_url,
			official_price_synced_at=EXCLUDED.official_price_synced_at,
			model_multiplier=EXCLUDED.model_multiplier, per_request_lte_256k=EXCLUDED.per_request_lte_256k,
			per_request_256k_512k_override=EXCLUDED.per_request_256k_512k_override,
			per_request_gt_512k_override=EXCLUDED.per_request_gt_512k_override,
			image_prices=EXCLUDED.image_prices, updated_at=NOW()
		RETURNING id, created_at, updated_at`, displayModelArgs(p, imageJSON)...).Scan(&p.ID, &p.CreatedAt, &p.UpdatedAt)
}

func (r *displayPricingRepository) UpdateModel(ctx context.Context, p *service.DisplayModelPrice) error {
	imageJSON, err := json.Marshal(p.ImagePrices)
	if err != nil {
		return fmt.Errorf("marshal display image prices: %w", err)
	}
	args := displayModelArgs(p, imageJSON)
	args = append(args, p.ID)
	err = r.db.QueryRowContext(ctx, `UPDATE display_model_prices SET
		platform=$1, model_name=$2, provider=$3, billing_mode=$4, currency=$5, enabled=$6, sort_order=$7, model_note=$8,
		official_input_per_million=$9, official_output_per_million=$10, official_cache_write_per_million=$11,
		official_cache_read_per_million=$12, official_price_source=$13, official_price_source_url=$14,
		official_price_synced_at=$15, model_multiplier=$16, per_request_lte_256k=$17,
		per_request_256k_512k_override=$18, per_request_gt_512k_override=$19, image_prices=$20, updated_at=NOW()
		WHERE id=$21 RETURNING created_at, updated_at`, args...).Scan(&p.CreatedAt, &p.UpdatedAt)
	if err == sql.ErrNoRows {
		return service.ErrDisplayPriceNotFound
	}
	return err
}

func (r *displayPricingRepository) DeleteModel(ctx context.Context, id int64) error {
	result, err := r.db.ExecContext(ctx, `DELETE FROM display_model_prices WHERE id=$1`, id)
	if err != nil {
		return err
	}
	rows, _ := result.RowsAffected()
	if rows == 0 {
		return service.ErrDisplayPriceNotFound
	}
	return nil
}

// ApplyOfficialPriceUpdates atomically updates only presentation-only official
// price columns and their provenance metadata. The predicates deliberately
// exclude per-request/image modes and all non-CNY rows. No channel, group, or
// billing table is reachable from this method.
func (r *displayPricingRepository) ApplyOfficialPriceUpdates(ctx context.Context, updates []service.OfficialPriceUpdate) error {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()

	for i := range updates {
		update := updates[i]
		var updatedID int64
		err := tx.QueryRowContext(ctx, `
			UPDATE display_model_prices SET
				official_input_per_million=$1,
				official_output_per_million=$2,
				official_cache_write_per_million=$3,
				official_cache_read_per_million=$4,
				official_price_source=$5,
				official_price_source_url=$6,
				official_price_synced_at=$7,
				updated_at=NOW()
			WHERE id=$8 AND billing_mode='token' AND currency='CNY' AND updated_at=$9
			RETURNING id`,
			decimalSQLArg(update.InputPerMillion), decimalSQLArg(update.OutputPerMillion),
			decimalSQLArg(update.CacheWritePerMillion), decimalSQLArg(update.CacheReadPerMillion),
			update.OfficialPriceSource, update.OfficialPriceSourceURL, update.OfficialPriceSyncedAt,
			update.ModelID, update.ExpectedUpdatedAt,
		).Scan(&updatedID)
		if err == sql.ErrNoRows {
			return service.ErrOfficialPriceApplyConflict
		}
		if err != nil {
			return fmt.Errorf("apply official display price for model %d: %w", update.ModelID, err)
		}
	}
	if err := tx.Commit(); err != nil {
		return err
	}
	return nil
}

func decimalSQLArg(value *decimal.Decimal) any {
	if value == nil {
		return nil
	}
	return value.Round(8).StringFixed(8)
}

func displayModelArgs(p *service.DisplayModelPrice, imageJSON []byte) []any {
	return []any{p.Platform, p.ModelName, p.Provider, p.BillingMode, p.Currency, p.Enabled, p.SortOrder,
		p.ModelNote,
		p.OfficialInputPerMillion, p.OfficialOutputPerMillion, p.OfficialCacheWritePerMillion, p.OfficialCacheReadPerMillion,
		p.OfficialPriceSource, p.OfficialPriceSourceURL, p.OfficialPriceSyncedAt,
		p.ModelMultiplier, p.PerRequestLTE256K, p.PerRequest256K512KOverride, p.PerRequestGT512KOverride, imageJSON}
}

type displayPriceRowScanner interface{ Scan(dest ...any) error }

func scanDisplayModel(row displayPriceRowScanner) (*service.DisplayModelPrice, error) {
	var p service.DisplayModelPrice
	var input, output, cacheWrite, cacheRead, multiplier sql.NullFloat64
	var base, mid, high sql.NullFloat64
	var syncedAt sql.NullTime
	var imageJSON []byte
	err := row.Scan(&p.ID, &p.Platform, &p.ModelName, &p.Provider, &p.BillingMode, &p.Currency, &p.Enabled, &p.SortOrder,
		&p.ModelNote,
		&input, &output, &cacheWrite, &cacheRead, &p.OfficialPriceSource, &p.OfficialPriceSourceURL, &syncedAt,
		&multiplier, &base, &mid, &high, &imageJSON, &p.CreatedAt, &p.UpdatedAt)
	if err != nil {
		return nil, err
	}
	p.OfficialInputPerMillion = nullFloatPtr(input)
	p.OfficialOutputPerMillion = nullFloatPtr(output)
	p.OfficialCacheWritePerMillion = nullFloatPtr(cacheWrite)
	p.OfficialCacheReadPerMillion = nullFloatPtr(cacheRead)
	if syncedAt.Valid {
		value := syncedAt.Time
		p.OfficialPriceSyncedAt = &value
	}
	p.ModelMultiplier = nullFloatPtr(multiplier)
	p.PerRequestLTE256K = nullFloatPtr(base)
	p.PerRequest256K512KOverride = nullFloatPtr(mid)
	p.PerRequestGT512KOverride = nullFloatPtr(high)
	if len(imageJSON) > 0 {
		if err := json.Unmarshal(imageJSON, &p.ImagePrices); err != nil {
			return nil, fmt.Errorf("unmarshal display image prices: %w", err)
		}
	}
	return &p, nil
}

func nullFloatPtr(v sql.NullFloat64) *float64 {
	if !v.Valid {
		return nil
	}
	out := v.Float64
	return &out
}
