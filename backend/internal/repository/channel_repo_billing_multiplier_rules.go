package repository

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/lib/pq"
)

// batchLoadBillingMultiplierRules loads channel billing multiplier rules for multiple channels.
func (r *channelRepository) batchLoadBillingMultiplierRules(ctx context.Context, channelIDs []int64) (map[int64][]service.ChannelBillingMultiplierRule, error) {
	rows, err := r.db.QueryContext(ctx,
		`SELECT id, channel_id, name, group_ids, account_ids, rate_multiplier, sort_order, created_at, updated_at
		 FROM channel_billing_multiplier_rules WHERE channel_id = ANY($1) ORDER BY channel_id, sort_order, id`,
		pq.Array(channelIDs),
	)
	if err != nil {
		return nil, fmt.Errorf("batch load channel billing multiplier rules: %w", err)
	}
	defer func() { _ = rows.Close() }()

	result := make(map[int64][]service.ChannelBillingMultiplierRule, len(channelIDs))
	for rows.Next() {
		var rule service.ChannelBillingMultiplierRule
		if err := rows.Scan(
			&rule.ID, &rule.ChannelID, &rule.Name,
			pq.Array(&rule.GroupIDs), pq.Array(&rule.AccountIDs),
			&rule.RateMultiplier, &rule.SortOrder, &rule.CreatedAt, &rule.UpdatedAt,
		); err != nil {
			return nil, fmt.Errorf("scan channel billing multiplier rule: %w", err)
		}
		result[rule.ChannelID] = append(result[rule.ChannelID], rule)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate channel billing multiplier rules: %w", err)
	}
	return result, nil
}

func (r *channelRepository) loadBillingMultiplierRules(ctx context.Context, channelID int64) ([]service.ChannelBillingMultiplierRule, error) {
	result, err := r.batchLoadBillingMultiplierRules(ctx, []int64{channelID})
	if err != nil {
		return nil, err
	}
	return result[channelID], nil
}

func replaceBillingMultiplierRulesTx(ctx context.Context, tx *sql.Tx, channelID int64, rules []service.ChannelBillingMultiplierRule) error {
	if _, err := tx.ExecContext(ctx,
		`DELETE FROM channel_billing_multiplier_rules WHERE channel_id = $1`, channelID,
	); err != nil {
		return fmt.Errorf("delete old channel billing multiplier rules: %w", err)
	}

	for i := range rules {
		rules[i].ChannelID = channelID
		if err := createBillingMultiplierRuleTx(ctx, tx, &rules[i]); err != nil {
			return fmt.Errorf("insert channel billing multiplier rule: %w", err)
		}
	}
	return nil
}

func createBillingMultiplierRuleTx(ctx context.Context, tx *sql.Tx, rule *service.ChannelBillingMultiplierRule) error {
	err := tx.QueryRowContext(ctx,
		`INSERT INTO channel_billing_multiplier_rules (channel_id, name, group_ids, account_ids, rate_multiplier, sort_order)
		 VALUES ($1, $2, $3, $4, $5, $6) RETURNING id, created_at, updated_at`,
		rule.ChannelID, rule.Name, pq.Array(rule.GroupIDs), pq.Array(rule.AccountIDs),
		rule.RateMultiplier, rule.SortOrder,
	).Scan(&rule.ID, &rule.CreatedAt, &rule.UpdatedAt)
	if err != nil {
		return fmt.Errorf("insert channel billing multiplier rule: %w", err)
	}
	return nil
}
