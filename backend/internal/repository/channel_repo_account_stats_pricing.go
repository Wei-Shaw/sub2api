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
	query := `SELECT id, channel_id, name, group_ids, account_ids, sort_order, created_at, updated_at
	 FROM channel_account_stats_pricing_rules WHERE channel_id = ANY($1) ORDER BY channel_id, sort_order, id`
	args := []any{pq.Array(channelIDs)}
	if IsMySQL() {
		query = `SELECT id, channel_id, name, group_ids, account_ids, sort_order, created_at, updated_at
	 FROM channel_account_stats_pricing_rules WHERE channel_id IN (` + Placeholders(len(channelIDs)) + `) ORDER BY channel_id, sort_order, id`
		args = make([]any, len(channelIDs))
		for i, id := range channelIDs {
			args[i] = id
		}
	}
	rows, err := r.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("batch load account stats pricing rules: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var allRules []service.AccountStatsPricingRule
	var ruleIDs []int64
	for rows.Next() {
		var rule service.AccountStatsPricingRule
		if IsMySQL() {
			// group_ids / account_ids 列在 MySQL 是 JSON（migrations/mysql/101）：
			// Scan 到 []byte 后用 encoding/json 反序列化，对应 PG 分支的 pq.Array 扫描。
			var groupIDs, accountIDs []byte
			if err := rows.Scan(
				&rule.ID, &rule.ChannelID, &rule.Name,
				&groupIDs, &accountIDs,
				&rule.SortOrder, &rule.CreatedAt, &rule.UpdatedAt,
			); err != nil {
				return nil, fmt.Errorf("scan account stats pricing rule: %w", err)
			}
			if len(groupIDs) > 0 {
				if err := json.Unmarshal(groupIDs, &rule.GroupIDs); err != nil {
					return nil, fmt.Errorf("decode account stats pricing rule group ids: %w", err)
				}
			}
			if len(accountIDs) > 0 {
				if err := json.Unmarshal(accountIDs, &rule.AccountIDs); err != nil {
					return nil, fmt.Errorf("decode account stats pricing rule account ids: %w", err)
				}
			}
			if rule.GroupIDs == nil {
				rule.GroupIDs = []int64{}
			}
			if rule.AccountIDs == nil {
				rule.AccountIDs = []int64{}
			}
		} else {
			if err := rows.Scan(
				&rule.ID, &rule.ChannelID, &rule.Name,
				pq.Array(&rule.GroupIDs), pq.Array(&rule.AccountIDs),
				&rule.SortOrder, &rule.CreatedAt, &rule.UpdatedAt,
			); err != nil {
				return nil, fmt.Errorf("scan account stats pricing rule: %w", err)
			}
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

	query := `SELECT id, rule_id, platform, models, billing_mode, input_price, output_price,
	        cache_write_price, cache_write_1h_price, cache_read_price, reasoning_effort_multipliers, image_output_price, per_request_price, created_at, updated_at
	 FROM channel_account_stats_model_pricing WHERE rule_id = ANY($1) ORDER BY rule_id, id`
	args := []any{pq.Array(ruleIDs)}
	if IsMySQL() {
		query = `SELECT id, rule_id, platform, models, billing_mode, input_price, output_price,
	        cache_write_price, cache_write_1h_price, cache_read_price, reasoning_effort_multipliers, image_output_price, per_request_price, created_at, updated_at
	 FROM channel_account_stats_model_pricing WHERE rule_id IN (` + Placeholders(len(ruleIDs)) + `) ORDER BY rule_id, id`
		args = make([]any, len(ruleIDs))
		for i, id := range ruleIDs {
			args[i] = id
		}
	}
	rows, err := r.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("batch load account stats model pricing: %w", err)
	}
	defer func() { _ = rows.Close() }()

	pricingMap := make(map[int64][]service.ChannelModelPricing, len(ruleIDs))
	for rows.Next() {
		var p service.ChannelModelPricing
		var ruleID int64
		var modelsJSON []byte
		var reasoningEffortMultipliersJSON []byte
		if err := rows.Scan(
			&p.ID, &ruleID, &p.Platform, &modelsJSON, &p.BillingMode,
			&p.InputPrice, &p.OutputPrice, &p.CacheWritePrice, &p.CacheWrite1hPrice, &p.CacheReadPrice,
			&reasoningEffortMultipliersJSON, &p.ImageOutputPrice, &p.PerRequestPrice, &p.CreatedAt, &p.UpdatedAt,
		); err != nil {
			return nil, fmt.Errorf("scan account stats model pricing: %w", err)
		}
		if err := json.Unmarshal(modelsJSON, &p.Models); err != nil {
			p.Models = []string{}
		}
		multipliers, err := unmarshalReasoningEffortMultipliers(reasoningEffortMultipliersJSON)
		if err != nil {
			return nil, err
		}
		p.ReasoningEffortMultipliers = multipliers
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
	deleteQuery := `DELETE FROM channel_account_stats_pricing_rules WHERE channel_id = $1`
	if IsMySQL() {
		deleteQuery = `DELETE FROM channel_account_stats_pricing_rules WHERE channel_id = ?`
	}
	// CASCADE 会自动删除关联的 model_pricing
	if _, err := tx.ExecContext(ctx, deleteQuery, channelID); err != nil {
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
	if IsMySQL() {
		if err := createAccountStatsPricingRuleRowMySQL(ctx, tx, rule); err != nil {
			return fmt.Errorf("insert account stats pricing rule: %w", err)
		}
	} else {
		err := tx.QueryRowContext(ctx,
			`INSERT INTO channel_account_stats_pricing_rules (channel_id, name, group_ids, account_ids, sort_order)
			 VALUES ($1, $2, $3, $4, $5) RETURNING id, created_at, updated_at`,
			rule.ChannelID, rule.Name, pq.Array(rule.GroupIDs), pq.Array(rule.AccountIDs), rule.SortOrder,
		).Scan(&rule.ID, &rule.CreatedAt, &rule.UpdatedAt)
		if err != nil {
			return fmt.Errorf("insert account stats pricing rule: %w", err)
		}
	}

	for j := range rule.Pricing {
		if err := createAccountStatsModelPricingTx(ctx, tx, rule.ID, &rule.Pricing[j]); err != nil {
			return err
		}
	}
	return nil
}

// createAccountStatsPricingRuleRowMySQL 是 createAccountStatsPricingRuleTx 内
// INSERT..RETURNING 的 MySQL 实现：group_ids / account_ids 列在 MySQL 是 JSON
// （migrations/mysql/101），写入侧 json.Marshal 成 JSON 字符串参数（nil 归一为 "[]"，
// 与 PG pq.Array(nil) 写 '{}' 的行为一致）；ExecContext + LastInsertId 取主键，
// created_at/updated_at 为数据库默认值，同事务回查补齐。
func createAccountStatsPricingRuleRowMySQL(ctx context.Context, tx *sql.Tx, rule *service.AccountStatsPricingRule) error {
	groupIDs := rule.GroupIDs
	if groupIDs == nil {
		groupIDs = []int64{}
	}
	accountIDs := rule.AccountIDs
	if accountIDs == nil {
		accountIDs = []int64{}
	}
	groupIDsJSON, err := json.Marshal(groupIDs)
	if err != nil {
		return err
	}
	accountIDsJSON, err := json.Marshal(accountIDs)
	if err != nil {
		return err
	}
	res, err := tx.ExecContext(ctx,
		`INSERT INTO channel_account_stats_pricing_rules (channel_id, name, group_ids, account_ids, sort_order)
		 VALUES (?, ?, ?, ?, ?)`,
		rule.ChannelID, rule.Name, string(groupIDsJSON), string(accountIDsJSON), rule.SortOrder,
	)
	if err != nil {
		return err
	}
	id, err := res.LastInsertId()
	if err != nil {
		return err
	}
	rule.ID = id
	return tx.QueryRowContext(ctx,
		`SELECT created_at, updated_at FROM channel_account_stats_pricing_rules WHERE id = ?`, id,
	).Scan(&rule.CreatedAt, &rule.UpdatedAt)
}

// createAccountStatsModelPricingTx 在事务中创建单条账号统计模型定价
func createAccountStatsModelPricingTx(ctx context.Context, tx *sql.Tx, ruleID int64, pricing *service.ChannelModelPricing) error {
	modelsJSON, err := json.Marshal(pricing.Models)
	if err != nil {
		return fmt.Errorf("marshal models: %w", err)
	}
	reasoningEffortMultipliersJSON, err := marshalReasoningEffortMultipliers(pricing.ReasoningEffortMultipliers)
	if err != nil {
		return err
	}
	billingMode := pricing.BillingMode
	if billingMode == "" {
		billingMode = service.BillingModeToken
	}
	platform := pricing.Platform
	if IsMySQL() {
		if err := createAccountStatsModelPricingRowMySQL(ctx, tx, ruleID, platform, pricing, modelsJSON, reasoningEffortMultipliersJSON, billingMode); err != nil {
			return fmt.Errorf("insert account stats model pricing: %w", err)
		}
	} else {
		err = tx.QueryRowContext(ctx,
			`INSERT INTO channel_account_stats_model_pricing (rule_id, platform, models, billing_mode, input_price, output_price, cache_write_price, cache_write_1h_price, cache_read_price, reasoning_effort_multipliers, image_output_price, per_request_price)
			 VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12) RETURNING id, created_at, updated_at`,
			ruleID, platform, modelsJSON, billingMode,
			pricing.InputPrice, pricing.OutputPrice, pricing.CacheWritePrice, pricing.CacheWrite1hPrice, pricing.CacheReadPrice,
			reasoningEffortMultipliersJSON, pricing.ImageOutputPrice, pricing.PerRequestPrice,
		).Scan(&pricing.ID, &pricing.CreatedAt, &pricing.UpdatedAt)
		if err != nil {
			return fmt.Errorf("insert account stats model pricing: %w", err)
		}
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
	if IsMySQL() {
		return createAccountStatsIntervalTxMySQL(ctx, tx, iv)
	}
	return tx.QueryRowContext(ctx,
		`INSERT INTO channel_account_stats_pricing_intervals
		 (pricing_id, min_tokens, max_tokens, tier_label, input_price, output_price, cache_write_price, cache_write_1h_price, cache_read_price, per_request_price, sort_order)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11) RETURNING id, created_at, updated_at`,
		iv.PricingID, iv.MinTokens, iv.MaxTokens, iv.TierLabel,
		iv.InputPrice, iv.OutputPrice, iv.CacheWritePrice, iv.CacheWrite1hPrice, iv.CacheReadPrice,
		iv.PerRequestPrice, iv.SortOrder,
	).Scan(&iv.ID, &iv.CreatedAt, &iv.UpdatedAt)
}

// createAccountStatsIntervalTxMySQL 是 createAccountStatsIntervalTx 的 MySQL 实现：
// ExecContext + LastInsertId 取主键 + 同事务回查 created_at/updated_at（数据库默认值）。
func createAccountStatsIntervalTxMySQL(ctx context.Context, tx *sql.Tx, iv *service.PricingInterval) error {
	res, err := tx.ExecContext(ctx,
		`INSERT INTO channel_account_stats_pricing_intervals
		 (pricing_id, min_tokens, max_tokens, tier_label, input_price, output_price, cache_write_price, cache_write_1h_price, cache_read_price, per_request_price, sort_order)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		iv.PricingID, iv.MinTokens, iv.MaxTokens, iv.TierLabel,
		iv.InputPrice, iv.OutputPrice, iv.CacheWritePrice, iv.CacheWrite1hPrice, iv.CacheReadPrice,
		iv.PerRequestPrice, iv.SortOrder,
	)
	if err != nil {
		return err
	}
	id, err := res.LastInsertId()
	if err != nil {
		return err
	}
	iv.ID = id
	return tx.QueryRowContext(ctx,
		`SELECT created_at, updated_at FROM channel_account_stats_pricing_intervals WHERE id = ?`, id,
	).Scan(&iv.CreatedAt, &iv.UpdatedAt)
}

// createAccountStatsModelPricingRowMySQL 是 createAccountStatsModelPricingTx 内
// INSERT..RETURNING 的 MySQL 实现：ExecContext + LastInsertId 取主键，
// created_at/updated_at 为数据库默认值，同事务回查补齐。
func createAccountStatsModelPricingRowMySQL(ctx context.Context, tx *sql.Tx, ruleID int64, platform string, pricing *service.ChannelModelPricing, modelsJSON []byte, reasoningEffortMultipliersJSON string, billingMode service.BillingMode) error {
	res, err := tx.ExecContext(ctx,
		`INSERT INTO channel_account_stats_model_pricing (rule_id, platform, models, billing_mode, input_price, output_price, cache_write_price, cache_write_1h_price, cache_read_price, reasoning_effort_multipliers, image_output_price, per_request_price)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		ruleID, platform, modelsJSON, billingMode,
		pricing.InputPrice, pricing.OutputPrice, pricing.CacheWritePrice, pricing.CacheWrite1hPrice, pricing.CacheReadPrice,
		reasoningEffortMultipliersJSON, pricing.ImageOutputPrice, pricing.PerRequestPrice,
	)
	if err != nil {
		return err
	}
	id, err := res.LastInsertId()
	if err != nil {
		return err
	}
	pricing.ID = id
	return tx.QueryRowContext(ctx,
		`SELECT created_at, updated_at FROM channel_account_stats_model_pricing WHERE id = ?`, id,
	).Scan(&pricing.CreatedAt, &pricing.UpdatedAt)
}

// batchLoadAccountStatsIntervals loads intervals for account stats pricing entries.
func (r *channelRepository) batchLoadAccountStatsIntervals(ctx context.Context, pricingIDs []int64) (map[int64][]service.PricingInterval, error) {
	if len(pricingIDs) == 0 {
		return nil, nil
	}
	query := `SELECT id, pricing_id, min_tokens, max_tokens, tier_label,
	        input_price, output_price, cache_write_price, cache_write_1h_price, cache_read_price,
	        per_request_price, sort_order, created_at, updated_at
	 FROM channel_account_stats_pricing_intervals
	 WHERE pricing_id = ANY($1) ORDER BY pricing_id, sort_order, id`
	args := []any{pq.Array(pricingIDs)}
	if IsMySQL() {
		query = `SELECT id, pricing_id, min_tokens, max_tokens, tier_label,
	        input_price, output_price, cache_write_price, cache_write_1h_price, cache_read_price,
	        per_request_price, sort_order, created_at, updated_at
	 FROM channel_account_stats_pricing_intervals
	 WHERE pricing_id IN (` + Placeholders(len(pricingIDs)) + `) ORDER BY pricing_id, sort_order, id`
		args = make([]any, len(pricingIDs))
		for i, id := range pricingIDs {
			args[i] = id
		}
	}
	rows, err := r.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("batch load account stats pricing intervals: %w", err)
	}
	defer func() { _ = rows.Close() }()

	result := make(map[int64][]service.PricingInterval)
	for rows.Next() {
		var iv service.PricingInterval
		if err := rows.Scan(
			&iv.ID, &iv.PricingID, &iv.MinTokens, &iv.MaxTokens, &iv.TierLabel,
			&iv.InputPrice, &iv.OutputPrice, &iv.CacheWritePrice, &iv.CacheWrite1hPrice, &iv.CacheReadPrice,
			&iv.PerRequestPrice, &iv.SortOrder, &iv.CreatedAt, &iv.UpdatedAt,
		); err != nil {
			return nil, fmt.Errorf("scan account stats pricing interval: %w", err)
		}
		result[iv.PricingID] = append(result[iv.PricingID], iv)
	}
	return result, rows.Err()
}
