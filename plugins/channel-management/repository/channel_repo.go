// Package repository implements the channel-management plugin's data access
// layer on top of the SDK-provided *sql.DB. The DB handle proxies queries
// through gRPC back to the core's connection pool, so SQL written here runs
// against the core's PostgreSQL instance transparently.
package repository

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/Wei-Shaw/sub2api/plugins/channel-management/internal/pagination"
	"github.com/Wei-Shaw/sub2api/plugins/channel-management/service"

	"github.com/lib/pq"
)

type channelRepository struct {
	db *sql.DB
}

// NewChannelRepository wires the channel repository on top of the SDK's DB
// handle.
func NewChannelRepository(db *sql.DB) service.ChannelRepository {
	return &channelRepository{db: db}
}

// runInTx wraps fn in a transaction; commits on success, rolls back on error.
func (r *channelRepository) runInTx(ctx context.Context, fn func(tx *sql.Tx) error) error {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin tx: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	if err := fn(tx); err != nil {
		return err
	}
	return tx.Commit()
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
		// AccountStatsPricingRules: nil means "leave untouched"; non-nil
		// (including empty slice) means "replace with this set" — same
		// semantics as ModelPricing above. Since the service layer always
		// passes the post-input snapshot, the channel.AccountStatsPricingRules
		// reflects the user-intended state.
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

	whereClause := strings.Join(where, " AND ")

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

	rows, err := r.db.QueryContext(ctx, dataQuery, args...)
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

	if len(channelIDs) > 0 {
		groupMap, err := r.batchLoadGroupIDs(ctx, channelIDs)
		if err != nil {
			return nil, nil, err
		}
		pricingMap, err := r.batchLoadModelPricing(ctx, channelIDs)
		if err != nil {
			return nil, nil, err
		}
		rulesMap, err := r.batchLoadAccountStatsPricingRules(ctx, channelIDs)
		if err != nil {
			return nil, nil, err
		}
		for i := range channels {
			channels[i].GroupIDs = groupMap[channels[i].ID]
			channels[i].ModelPricing = pricingMap[channels[i].ID]
			channels[i].AccountStatsPricingRules = rulesMap[channels[i].ID]
		}
	}

	pages := 0
	if total > 0 {
		pages = int((total + int64(pageSize) - 1) / int64(pageSize))
	}

	paginationResult := &pagination.PaginationResult{
		Total:    total,
		Page:     page,
		PageSize: pageSize,
		Pages:    pages,
	}
	return channels, paginationResult, nil
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
	rows, err := r.db.QueryContext(ctx,
		`SELECT id, name, description, status, model_mapping, billing_model_source, restrict_models, features, apply_pricing_to_account_stats, created_at, updated_at FROM channels ORDER BY id`,
	)
	if err != nil {
		return nil, fmt.Errorf("query all channels: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var channels []service.Channel
	var channelIDs []int64
	for rows.Next() {
		var ch service.Channel
		var modelMappingJSON []byte
		if err := rows.Scan(&ch.ID, &ch.Name, &ch.Description, &ch.Status, &modelMappingJSON, &ch.BillingModelSource, &ch.RestrictModels, &ch.Features, &ch.ApplyPricingToAccountStats, &ch.CreatedAt, &ch.UpdatedAt); err != nil {
			return nil, fmt.Errorf("scan channel: %w", err)
		}
		ch.ModelMapping = unmarshalModelMapping(modelMappingJSON)
		channels = append(channels, ch)
		channelIDs = append(channelIDs, ch.ID)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate channels: %w", err)
	}

	if len(channelIDs) == 0 {
		return channels, nil
	}

	groupMap, err := r.batchLoadGroupIDs(ctx, channelIDs)
	if err != nil {
		return nil, err
	}
	pricingMap, err := r.batchLoadModelPricing(ctx, channelIDs)
	if err != nil {
		return nil, err
	}
	rulesMap, err := r.batchLoadAccountStatsPricingRules(ctx, channelIDs)
	if err != nil {
		return nil, err
	}

	for i := range channels {
		channels[i].GroupIDs = groupMap[channels[i].ID]
		channels[i].ModelPricing = pricingMap[channels[i].ID]
		channels[i].AccountStatsPricingRules = rulesMap[channels[i].ID]
	}
	return channels, nil
}

// batchLoadGroupIDs loads group IDs for many channels in one round trip.
func (r *channelRepository) batchLoadGroupIDs(ctx context.Context, channelIDs []int64) (map[int64][]int64, error) {
	rows, err := r.db.QueryContext(ctx,
		`SELECT channel_id, group_id FROM channel_groups
		 WHERE channel_id = ANY($1) ORDER BY channel_id, group_id`,
		pq.Array(channelIDs),
	)
	if err != nil {
		return nil, fmt.Errorf("batch load group ids: %w", err)
	}
	defer func() { _ = rows.Close() }()

	groupMap := make(map[int64][]int64, len(channelIDs))
	for rows.Next() {
		var channelID, groupID int64
		if err := rows.Scan(&channelID, &groupID); err != nil {
			return nil, fmt.Errorf("scan group id: %w", err)
		}
		groupMap[channelID] = append(groupMap[channelID], groupID)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate group ids: %w", err)
	}
	return groupMap, nil
}

func (r *channelRepository) ExistsByName(ctx context.Context, name string) (bool, error) {
	var exists bool
	err := r.db.QueryRowContext(ctx,
		`SELECT EXISTS(SELECT 1 FROM channels WHERE name = $1)`, name,
	).Scan(&exists)
	return exists, err
}

func (r *channelRepository) ExistsByNameExcluding(ctx context.Context, name string, excludeID int64) (bool, error) {
	var exists bool
	err := r.db.QueryRowContext(ctx,
		`SELECT EXISTS(SELECT 1 FROM channels WHERE name = $1 AND id != $2)`, name, excludeID,
	).Scan(&exists)
	return exists, err
}

func (r *channelRepository) GetGroupIDs(ctx context.Context, channelID int64) ([]int64, error) {
	rows, err := r.db.QueryContext(ctx,
		`SELECT group_id FROM channel_groups WHERE channel_id = $1 ORDER BY group_id`, channelID,
	)
	if err != nil {
		return nil, fmt.Errorf("get group ids: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var ids []int64
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			return nil, fmt.Errorf("scan group id: %w", err)
		}
		ids = append(ids, id)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate group ids: %w", err)
	}
	return ids, nil
}

func (r *channelRepository) SetGroupIDs(ctx context.Context, channelID int64, groupIDs []int64) error {
	return setGroupIDsTx(ctx, r.db, channelID, groupIDs)
}

func (r *channelRepository) GetChannelIDByGroupID(ctx context.Context, groupID int64) (int64, error) {
	var channelID int64
	err := r.db.QueryRowContext(ctx,
		`SELECT channel_id FROM channel_groups WHERE group_id = $1`, groupID,
	).Scan(&channelID)
	if err == sql.ErrNoRows {
		return 0, nil
	}
	return channelID, err
}

func (r *channelRepository) GetGroupsInOtherChannels(ctx context.Context, channelID int64, groupIDs []int64) ([]int64, error) {
	if len(groupIDs) == 0 {
		return nil, nil
	}
	rows, err := r.db.QueryContext(ctx,
		`SELECT group_id FROM channel_groups WHERE group_id = ANY($1) AND channel_id != $2`,
		pq.Array(groupIDs), channelID,
	)
	if err != nil {
		return nil, fmt.Errorf("get groups in other channels: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var conflicting []int64
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			return nil, fmt.Errorf("scan conflicting group id: %w", err)
		}
		conflicting = append(conflicting, id)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate conflicting group ids: %w", err)
	}
	return conflicting, nil
}

// marshalModelMapping serialises {"platform": {"src": "dst"}, ...} to JSON.
// Empty / nil maps become "{}".
func marshalModelMapping(m map[string]map[string]string) ([]byte, error) {
	if len(m) == 0 {
		return []byte("{}"), nil
	}
	data, err := json.Marshal(m)
	if err != nil {
		return nil, fmt.Errorf("marshal model_mapping: %w", err)
	}
	return data, nil
}

// unmarshalModelMapping parses JSON back into the nested mapping. Returns nil
// on any error so the caller never has to inspect malformed cached rows.
func unmarshalModelMapping(data []byte) map[string]map[string]string {
	if len(data) == 0 {
		return nil
	}
	var m map[string]map[string]string
	if err := json.Unmarshal(data, &m); err != nil {
		return nil
	}
	return m
}

// GetGroupPlatforms returns a map[group_id]platform for the given IDs.
func (r *channelRepository) GetGroupPlatforms(ctx context.Context, groupIDs []int64) (map[int64]string, error) {
	if len(groupIDs) == 0 {
		return make(map[int64]string), nil
	}
	rows, err := r.db.QueryContext(ctx,
		`SELECT id, platform FROM groups WHERE id = ANY($1)`,
		pq.Array(groupIDs),
	)
	if err != nil {
		return nil, fmt.Errorf("get group platforms: %w", err)
	}
	defer rows.Close() //nolint:errcheck

	result := make(map[int64]string, len(groupIDs))
	for rows.Next() {
		var id int64
		var platform string
		if err := rows.Scan(&id, &platform); err != nil {
			return nil, fmt.Errorf("scan group platform: %w", err)
		}
		result[id] = platform
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate group platforms: %w", err)
	}
	return result, nil
}
