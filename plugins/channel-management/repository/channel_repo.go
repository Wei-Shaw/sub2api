// Package repository implements the channel-management plugin's data access
// layer on top of the SDK-provided *sql.DB. The DB handle proxies queries
// through gRPC back to the core's connection pool, so SQL written here runs
// against the core's PostgreSQL instance transparently.
package repository

import (
	"context"
	"database/sql"
	"fmt"
	"strings"

	"github.com/Wei-Shaw/sub2api/plugins/channel-management/ent"
	"github.com/Wei-Shaw/sub2api/plugins/channel-management/internal/pagination"
	"github.com/Wei-Shaw/sub2api/plugins/channel-management/service"

	pluginsdk "github.com/Wei-Shaw/sub2api/plugin-sdk"
)

type channelRepository struct {
	db        *sql.DB
	entClient *ent.Client
}

// NewChannelRepository wires the channel repository on top of the SDK's DB
// handle. entClient may be nil when ent is not yet initialised (the POC
// GetByIDEnt method checks and returns an error in that case).
func NewChannelRepository(db *sql.DB, entClient *ent.Client) service.ChannelRepository {
	return &channelRepository{db: db, entClient: entClient}
}

// runInTx wraps fn in a transaction via the SDK helper; commits on success, rolls back on error.
func (r *channelRepository) runInTx(ctx context.Context, fn func(tx *sql.Tx) error) error {
	return pluginsdk.WithTx(ctx, r.db, fn)
}

func (r *channelRepository) Create(ctx context.Context, channel *service.Channel) error {
	return r.runInTx(ctx, func(tx *sql.Tx) error {
		modelMappingJSON, err := marshalModelMapping(channel.ModelMapping)
		if err != nil {
			return err
		}
		err = tx.QueryRowContext(ctx,
			`INSERT INTO channels (name, description, status, model_mapping, billing_model_source, restrict_models, features, apply_pricing_to_account_stats)
			 VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
			 RETURNING id, created_at, updated_at`,
			channel.Name, channel.Description, channel.Status, modelMappingJSON, channel.BillingModelSource, channel.RestrictModels, channel.Features, channel.ApplyPricingToAccountStats,
		).Scan(&channel.ID, &channel.CreatedAt, &channel.UpdatedAt)
		if err != nil {
			if isUniqueViolation(err) {
				return service.ErrChannelExists
			}
			return fmt.Errorf("insert channel: %w", err)
		}

		if len(channel.GroupIDs) > 0 {
			if err := setGroupIDsTx(ctx, tx, channel.ID, channel.GroupIDs); err != nil {
				return err
			}
		}
		if len(channel.ModelPricing) > 0 {
			if err := replaceModelPricingTx(ctx, tx, channel.ID, channel.ModelPricing); err != nil {
				return err
			}
		}
		if err := replaceAccountStatsPricingRulesTx(ctx, tx, channel.ID, channel.AccountStatsPricingRules); err != nil {
			return err
		}
		return nil
	})
}

func (r *channelRepository) GetByID(ctx context.Context, id int64) (*service.Channel, error) {
	ch := &service.Channel{}
	var modelMappingJSON []byte
	err := r.db.QueryRowContext(ctx,
		`SELECT id, name, description, status, model_mapping, billing_model_source, restrict_models, features, apply_pricing_to_account_stats, created_at, updated_at
		 FROM channels WHERE id = $1`, id,
	).Scan(&ch.ID, &ch.Name, &ch.Description, &ch.Status, &modelMappingJSON, &ch.BillingModelSource, &ch.RestrictModels, &ch.Features, &ch.ApplyPricingToAccountStats, &ch.CreatedAt, &ch.UpdatedAt)
	if err == sql.ErrNoRows {
		return nil, service.ErrChannelNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("get channel: %w", err)
	}
	ch.ModelMapping = unmarshalModelMapping(modelMappingJSON)

	groupIDs, err := r.GetGroupIDs(ctx, id)
	if err != nil {
		return nil, err
	}
	ch.GroupIDs = groupIDs

	pricing, err := r.ListModelPricing(ctx, id)
	if err != nil {
		return nil, err
	}
	ch.ModelPricing = pricing

	rules, err := r.loadAccountStatsPricingRules(ctx, id)
	if err != nil {
		return nil, err
	}
	ch.AccountStatsPricingRules = rules

	return ch, nil
}

func (r *channelRepository) Update(ctx context.Context, channel *service.Channel) error {
	return r.runInTx(ctx, func(tx *sql.Tx) error {
		modelMappingJSON, err := marshalModelMapping(channel.ModelMapping)
		if err != nil {
			return err
		}
		result, err := tx.ExecContext(ctx,
			`UPDATE channels SET name = $1, description = $2, status = $3, model_mapping = $4, billing_model_source = $5, restrict_models = $6, features = $7, apply_pricing_to_account_stats = $8, updated_at = NOW()
			 WHERE id = $9`,
			channel.Name, channel.Description, channel.Status, modelMappingJSON, channel.BillingModelSource, channel.RestrictModels, channel.Features, channel.ApplyPricingToAccountStats, channel.ID,
		)
		if err != nil {
			if isUniqueViolation(err) {
				return service.ErrChannelExists
			}
			return fmt.Errorf("update channel: %w", err)
		}
		rows, _ := result.RowsAffected()
		if rows == 0 {
			return service.ErrChannelNotFound
		}

		if channel.GroupIDs != nil {
			if err := setGroupIDsTx(ctx, tx, channel.ID, channel.GroupIDs); err != nil {
				return err
			}
		}
		if channel.ModelPricing != nil {
			if err := replaceModelPricingTx(ctx, tx, channel.ID, channel.ModelPricing); err != nil {
				return err
			}
		}
		if channel.AccountStatsPricingRules != nil {
			if err := replaceAccountStatsPricingRulesTx(ctx, tx, channel.ID, channel.AccountStatsPricingRules); err != nil {
				return err
			}
		}
		return nil
	})
}

func (r *channelRepository) Delete(ctx context.Context, id int64) error {
	result, err := r.db.ExecContext(ctx, `DELETE FROM channels WHERE id = $1`, id)
	if err != nil {
		return fmt.Errorf("delete channel: %w", err)
	}
	rows, _ := result.RowsAffected()
	if rows == 0 {
		return service.ErrChannelNotFound
	}
	return nil
}

func (r *channelRepository) List(ctx context.Context, params pagination.PaginationParams, status, search string) ([]service.Channel, *pagination.PaginationResult, error) {
	whereClause, args, argIdx := buildChannelListFilter(status, search)

	var total int64
	countQuery := fmt.Sprintf("SELECT COUNT(*) FROM channels c WHERE %s", whereClause)
	if err := r.db.QueryRowContext(ctx, countQuery, args...).Scan(&total); err != nil {
		return nil, nil, fmt.Errorf("count channels: %w", err)
	}

	pageSize := params.Limit()
	page := params.Page
	if page < 1 {
		page = 1
	}
	offset := (page - 1) * pageSize

	dataQuery := fmt.Sprintf(
		`SELECT c.id, c.name, c.description, c.status, c.model_mapping, c.billing_model_source, c.restrict_models, c.features, c.apply_pricing_to_account_stats, c.created_at, c.updated_at
		 FROM channels c WHERE %s ORDER BY %s LIMIT $%d OFFSET $%d`,
		whereClause, channelListOrderBy(params), argIdx, argIdx+1,
	)
	args = append(args, pageSize, offset)

	channels, channelIDs, err := r.scanChannelRows(ctx, dataQuery, args...)
	if err != nil {
		return nil, nil, err
	}
	if err := r.hydrateChannelChildren(ctx, channels, channelIDs); err != nil {
		return nil, nil, err
	}

	pages := 0
	if total > 0 {
		pages = int((total + int64(pageSize) - 1) / int64(pageSize))
	}
	return channels, &pagination.PaginationResult{
		Total:    total,
		Page:     page,
		PageSize: pageSize,
		Pages:    pages,
	}, nil
}

// buildChannelListFilter composes the dynamic WHERE clause for List.
func buildChannelListFilter(status, search string) (string, []any, int) {
	where := []string{"1=1"}
	args := []any{}
	argIdx := 1
	if status != "" {
		where = append(where, fmt.Sprintf("c.status = $%d", argIdx))
		args = append(args, status)
		argIdx++
	}
	if search != "" {
		where = append(where, fmt.Sprintf("(c.name ILIKE $%d OR c.description ILIKE $%d)", argIdx, argIdx))
		args = append(args, "%"+escapeLike(search)+"%")
		argIdx++
	}
	return strings.Join(where, " AND "), args, argIdx
}

// scanChannelRows is the single source of truth for "SELECT ... FROM channels
// -> []service.Channel".
func (r *channelRepository) scanChannelRows(ctx context.Context, query string, args ...any) ([]service.Channel, []int64, error) {
	rows, err := r.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, nil, fmt.Errorf("query channels: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var channels []service.Channel
	var channelIDs []int64
	for rows.Next() {
		var ch service.Channel
		var modelMappingJSON []byte
		if err := rows.Scan(&ch.ID, &ch.Name, &ch.Description, &ch.Status, &modelMappingJSON, &ch.BillingModelSource, &ch.RestrictModels, &ch.Features, &ch.ApplyPricingToAccountStats, &ch.CreatedAt, &ch.UpdatedAt); err != nil {
			return nil, nil, fmt.Errorf("scan channel: %w", err)
		}
		ch.ModelMapping = unmarshalModelMapping(modelMappingJSON)
		channels = append(channels, ch)
		channelIDs = append(channelIDs, ch.ID)
	}
	if err := rows.Err(); err != nil {
		return nil, nil, fmt.Errorf("iterate channels: %w", err)
	}
	return channels, channelIDs, nil
}

// hydrateChannelChildren attaches the three batch-loaded child collections
// (group IDs, model pricing, account-stats rules) onto each Channel.
func (r *channelRepository) hydrateChannelChildren(ctx context.Context, channels []service.Channel, channelIDs []int64) error {
	if len(channelIDs) == 0 {
		return nil
	}
	groupMap, err := r.batchLoadGroupIDs(ctx, channelIDs)
	if err != nil {
		return err
	}
	pricingMap, err := r.batchLoadModelPricing(ctx, channelIDs)
	if err != nil {
		return err
	}
	rulesMap, err := r.batchLoadAccountStatsPricingRules(ctx, channelIDs)
	if err != nil {
		return err
	}
	for i := range channels {
		channels[i].GroupIDs = groupMap[channels[i].ID]
		channels[i].ModelPricing = pricingMap[channels[i].ID]
		channels[i].AccountStatsPricingRules = rulesMap[channels[i].ID]
	}
	return nil
}

func channelListOrderBy(params pagination.PaginationParams) string {
	sortBy := strings.ToLower(strings.TrimSpace(params.SortBy))
	sortOrder := strings.ToUpper(params.NormalizedSortOrder(pagination.SortOrderAsc))

	var column string
	switch sortBy {
	case "":
		column = "c.id"
		sortOrder = "ASC"
	case "id":
		column = "c.id"
	case "name":
		column = "c.name"
	case "status":
		column = "c.status"
	case "created_at":
		column = "c.created_at"
	default:
		column = "c.id"
		sortOrder = "ASC"
	}
	return fmt.Sprintf("%s %s, c.id %s", column, sortOrder, sortOrder)
}

func (r *channelRepository) ListAll(ctx context.Context) ([]service.Channel, error) {
	channels, channelIDs, err := r.scanChannelRows(ctx,
		`SELECT id, name, description, status, model_mapping, billing_model_source, restrict_models, features, apply_pricing_to_account_stats, created_at, updated_at FROM channels ORDER BY id`,
	)
	if err != nil {
		return nil, err
	}
	if err := r.hydrateChannelChildren(ctx, channels, channelIDs); err != nil {
		return nil, err
	}
	return channels, nil
}
