package repository

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/lib/pq"
)

// --- 账号统计定价规则 ---

// batchLoadAccountStatsPricingRules 批量加载多个渠道的账号统计定价规则（含模型定价）
func (r *channelRepository) batchLoadAccountStatsPricingRules(ctx context.Context, channelIDs []int64) (map[int64][]service.AccountStatsPricingRule, error) {
	// 1. 查询规则
	rows, err := r.db.QueryContext(ctx,
		`SELECT id, channel_id, name, group_ids, account_ids, sort_order, created_at, updated_at
		 FROM channel_account_stats_pricing_rules WHERE channel_id = ANY($1) ORDER BY channel_id, sort_order, id`,
		pq.Array(channelIDs),
	)
	if err != nil {
		return nil, fmt.Errorf("batch load account stats pricing rules: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var allRules []service.AccountStatsPricingRule
	var ruleIDs []int64
	for rows.Next() {
		var rule service.AccountStatsPricingRule
		if err := rows.Scan(
			&rule.ID, &rule.ChannelID, &rule.Name,
			pq.Array(&rule.GroupIDs), pq.Array(&rule.AccountIDs),
			&rule.SortOrder, &rule.CreatedAt, &rule.UpdatedAt,
		); err != nil {
			return nil, fmt.Errorf("scan account stats pricing rule: %w", err)
		}
		ruleIDs = append(ruleIDs, rule.ID)
		allRules = append(allRules, rule)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate account stats pricing rules: %w", err)
	}

	// 2. 批量加载规则的模型定价
	pricingMap, err := r.batchLoadAccountStatsModelPricing(ctx, ruleIDs)
	if err != nil {
		return nil, err
	}

	// 3. 按 channelID 分组并关联定价
	result := make(map[int64][]service.AccountStatsPricingRule, len(channelIDs))
	for i := range allRules {
		allRules[i].Pricing = pricingMap[allRules[i].ID]
		result[allRules[i].ChannelID] = append(result[allRules[i].ChannelID], allRules[i])
	}

	return result, nil
}

// batchLoadAccountStatsModelPricing 批量加载规则的模型定价
func (r *channelRepository) batchLoadAccountStatsModelPricing(ctx context.Context, ruleIDs []int64) (map[int64][]service.ChannelModelPricing, error) {
	if len(ruleIDs) == 0 {
		return make(map[int64][]service.ChannelModelPricing), nil
	}

	rows, err := r.db.QueryContext(ctx,
		`SELECT id, rule_id, platform, models, billing_mode, input_price, output_price,
		        cache_write_price, cache_read_price, image_output_price, per_request_price, created_at, updated_at
		 FROM channel_account_stats_model_pricing WHERE rule_id = ANY($1) ORDER BY rule_id, id`,
		pq.Array(ruleIDs),
	)
	if err != nil {
		return nil, fmt.Errorf("batch load account stats model pricing: %w", err)
	}
	defer func() { _ = rows.Close() }()

	pricingMap := make(map[int64][]service.ChannelModelPricing, len(ruleIDs))
	for rows.Next() {
		var p service.ChannelModelPricing
		var ruleID int64
		var modelsJSON []byte
		if err := rows.Scan(
			&p.ID, &ruleID, &p.Platform, &modelsJSON, &p.BillingMode,
			&p.InputPrice, &p.OutputPrice, &p.CacheWritePrice, &p.CacheReadPrice,
			&p.ImageOutputPrice, &p.PerRequestPrice, &p.CreatedAt, &p.UpdatedAt,
		); err != nil {
			return nil, fmt.Errorf("scan account stats model pricing: %w", err)
		}
		if err := json.Unmarshal(modelsJSON, &p.Models); err != nil {
			p.Models = []string{}
		}
		pricingMap[ruleID] = append(pricingMap[ruleID], p)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate account stats model pricing: %w", err)
	}

	// Load intervals for all pricing entries.
	var allPricingIDs []int64
	for _, pricings := range pricingMap {
		for _, p := range pricings {
			allPricingIDs = append(allPricingIDs, p.ID)
		}
	}
	if len(allPricingIDs) > 0 {
		intervalsMap, err := r.batchLoadAccountStatsIntervals(ctx, allPricingIDs)
		if err != nil {
			return nil, err
		}
		for ruleID, pricings := range pricingMap {
			for i := range pricings {
				pricings[i].Intervals = intervalsMap[pricings[i].ID]
			}
			pricingMap[ruleID] = pricings
		}
	}

	return pricingMap, nil
}

// loadAccountStatsPricingRules 加载单个渠道的账号统计定价规则（供 GetByID 使用）
func (r *channelRepository) loadAccountStatsPricingRules(ctx context.Context, channelID int64) ([]service.AccountStatsPricingRule, error) {
	result, err := r.batchLoadAccountStatsPricingRules(ctx, []int64{channelID})
	if err != nil {
		return nil, err
	}
	return result[channelID], nil
}

// replaceAccountStatsPricingRulesTx 在事务中替换渠道的账号统计定价规则（删除旧的 + 插入新的）
func replaceAccountStatsPricingRulesTx(ctx context.Context, tx *sql.Tx, channelID int64, rules []service.AccountStatsPricingRule) error {
	// CASCADE 会自动删除关联的 model_pricing
	if _, err := tx.ExecContext(ctx,
		`DELETE FROM channel_account_stats_pricing_rules WHERE channel_id = $1`, channelID,
	); err != nil {
		return fmt.Errorf("delete old account stats pricing rules: %w", err)
	}

	for i := range rules {
		rules[i].ChannelID = channelID
		if err := createAccountStatsPricingRuleTx(ctx, tx, &rules[i]); err != nil {
			return fmt.Errorf("insert account stats pricing rule: %w", err)
		}
	}
	return nil
}

// createAccountStatsPricingRuleTx 在事务中创建单条账号统计定价规则及其模型定价
func createAccountStatsPricingRuleTx(ctx context.Context, tx *sql.Tx, rule *service.AccountStatsPricingRule) error {
	err := tx.QueryRowContext(ctx,
		`INSERT INTO channel_account_stats_pricing_rules (channel_id, name, group_ids, account_ids, sort_order)
		 VALUES ($1, $2, $3, $4, $5) RETURNING id, created_at, updated_at`,
		rule.ChannelID, rule.Name, pq.Array(rule.GroupIDs), pq.Array(rule.AccountIDs), rule.SortOrder,
	).Scan(&rule.ID, &rule.CreatedAt, &rule.UpdatedAt)
	if err != nil {
		return fmt.Errorf("insert account stats pricing rule: %w", err)
	}

	for j := range rule.Pricing {
		if err := createAccountStatsModelPricingTx(ctx, tx, rule.ID, &rule.Pricing[j]); err != nil {
			return err
		}
	}
	return nil
}

// createAccountStatsModelPricingTx 在事务中创建单条账号统计模型定价
func createAccountStatsModelPricingTx(ctx context.Context, tx *sql.Tx, ruleID int64, pricing *service.ChannelModelPricing) error {
	modelsJSON, err := json.Marshal(pricing.Models)
	if err != nil {
		return fmt.Errorf("marshal models: %w", err)
	}
	billingMode := pricing.BillingMode
	if billingMode == "" {
		billingMode = service.BillingModeToken
	}
	platform := pricing.Platform
	err = tx.QueryRowContext(ctx,
		`INSERT INTO channel_account_stats_model_pricing (rule_id, platform, models, billing_mode, input_price, output_price, cache_write_price, cache_read_price, image_output_price, per_request_price)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10) RETURNING id, created_at, updated_at`,
		ruleID, platform, modelsJSON, billingMode,
		pricing.InputPrice, pricing.OutputPrice, pricing.CacheWritePrice, pricing.CacheReadPrice,
		pricing.ImageOutputPrice, pricing.PerRequestPrice,
	).Scan(&pricing.ID, &pricing.CreatedAt, &pricing.UpdatedAt)
	if err != nil {
		return fmt.Errorf("insert account stats model pricing: %w", err)
	}
	// Persist intervals (mirrors channel_pricing_intervals logic).
	for i := range pricing.Intervals {
		iv := &pricing.Intervals[i]
		iv.PricingID = pricing.ID
		if err := createAccountStatsIntervalTx(ctx, tx, iv); err != nil {
			return err
		}
	}
	return nil
}

// createAccountStatsIntervalTx inserts a single interval for an account stats pricing entry.
func createAccountStatsIntervalTx(ctx context.Context, tx *sql.Tx, iv *service.PricingInterval) error {
	return tx.QueryRowContext(ctx,
		`INSERT INTO channel_account_stats_pricing_intervals
		 (pricing_id, min_tokens, max_tokens, tier_label, input_price, output_price, cache_write_price, cache_read_price, per_request_price, sort_order)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10) RETURNING id, created_at, updated_at`,
		iv.PricingID, iv.MinTokens, iv.MaxTokens, iv.TierLabel,
		iv.InputPrice, iv.OutputPrice, iv.CacheWritePrice, iv.CacheReadPrice,
		iv.PerRequestPrice, iv.SortOrder,
	).Scan(&iv.ID, &iv.CreatedAt, &iv.UpdatedAt)
}

// batchLoadAccountStatsIntervals loads intervals for account stats pricing entries.
func (r *channelRepository) batchLoadAccountStatsIntervals(ctx context.Context, pricingIDs []int64) (map[int64][]service.PricingInterval, error) {
	if len(pricingIDs) == 0 {
		return nil, nil
	}
	rows, err := r.db.QueryContext(ctx,
		`SELECT id, pricing_id, min_tokens, max_tokens, tier_label,
		        input_price, output_price, cache_write_price, cache_read_price,
		        per_request_price, sort_order, created_at, updated_at
		 FROM channel_account_stats_pricing_intervals
		 WHERE pricing_id = ANY($1) ORDER BY pricing_id, sort_order, id`,
		pq.Array(pricingIDs),
	)
	if err != nil {
		return nil, fmt.Errorf("batch load account stats pricing intervals: %w", err)
	}
	defer func() { _ = rows.Close() }()

	result := make(map[int64][]service.PricingInterval)
	for rows.Next() {
		var iv service.PricingInterval
		if err := rows.Scan(
			&iv.ID, &iv.PricingID, &iv.MinTokens, &iv.MaxTokens, &iv.TierLabel,
			&iv.InputPrice, &iv.OutputPrice, &iv.CacheWritePrice, &iv.CacheReadPrice,
			&iv.PerRequestPrice, &iv.SortOrder, &iv.CreatedAt, &iv.UpdatedAt,
		); err != nil {
			return nil, fmt.Errorf("scan account stats pricing interval: %w", err)
		}
		result[iv.PricingID] = append(result[iv.PricingID], iv)
	}
	return result, rows.Err()
}
