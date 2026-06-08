package repository

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"

	"github.com/Wei-Shaw/sub2api/plugins/channel-management/service"
	"github.com/lib/pq"
)

// --- 账号统计定价规则 ---
//
// 这些 helper 与 channel_repo_pricing.go 风格对齐：
//   * batchLoadAccountStatsPricingRules / loadAccountStatsPricingRules 供
//     GetByID / List / ListAll 在 N+1 与 1 行两种场景下复用；
//   * replaceAccountStatsPricingRulesTx + createAccountStatsPricingRuleTx
//     /createAccountStatsModelPricingTx 给 Create / Update 在事务里走"全量替换"
//     的语义（与 ModelPricing 的 replaceModelPricingTx 一致）；
//   * 价格列存为 NUMERIC(20,12)，与 channel_model_pricing 的精度一致。

// batchLoadAccountStatsPricingRules 批量加载多个渠道的账号统计定价规则（含规则下的模型定价）
func (r *channelRepository) batchLoadAccountStatsPricingRules(ctx context.Context, channelIDs []int64) (map[int64][]service.AccountStatsPricingRule, error) {
	if len(channelIDs) == 0 {
		return make(map[int64][]service.AccountStatsPricingRule), nil
	}

	rows, err := r.db.QueryContext(ctx,
		`SELECT id, channel_id, name, group_ids, account_ids, sort_order, created_at, updated_at
		 FROM channel_account_stats_pricing_rules WHERE channel_id = ANY($1)
		 ORDER BY channel_id, sort_order, id`,
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

	pricingMap, err := r.batchLoadAccountStatsModelPricing(ctx, ruleIDs)
	if err != nil {
		return nil, err
	}

	result := make(map[int64][]service.AccountStatsPricingRule, len(channelIDs))
	for i := range allRules {
		allRules[i].Pricing = pricingMap[allRules[i].ID]
		result[allRules[i].ChannelID] = append(result[allRules[i].ChannelID], allRules[i])
	}
	return result, nil
}

// batchLoadAccountStatsModelPricing 批量加载规则的模型定价行
func (r *channelRepository) batchLoadAccountStatsModelPricing(ctx context.Context, ruleIDs []int64) (map[int64][]service.ChannelModelPricing, error) {
	if len(ruleIDs) == 0 {
		return make(map[int64][]service.ChannelModelPricing), nil
	}

	rows, err := r.db.QueryContext(ctx,
		`SELECT id, rule_id, platform, models, billing_mode,
		        input_price, output_price, cache_write_price, cache_read_price,
		        image_output_price, per_request_price, created_at, updated_at
		 FROM channel_account_stats_model_pricing WHERE rule_id = ANY($1)
		 ORDER BY rule_id, id`,
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
		var billingMode string
		if err := rows.Scan(
			&p.ID, &ruleID, &p.Platform, &modelsJSON, &billingMode,
			&p.InputPrice, &p.OutputPrice, &p.CacheWritePrice, &p.CacheReadPrice,
			&p.ImageOutputPrice, &p.PerRequestPrice, &p.CreatedAt, &p.UpdatedAt,
		); err != nil {
			return nil, fmt.Errorf("scan account stats model pricing: %w", err)
		}
		p.BillingMode = service.BillingMode(billingMode)
		if err := json.Unmarshal(modelsJSON, &p.Models); err != nil {
			p.Models = []string{}
		}
		pricingMap[ruleID] = append(pricingMap[ruleID], p)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate account stats model pricing: %w", err)
	}
	return pricingMap, nil
}

// loadAccountStatsPricingRules 加载单个渠道的账号统计定价规则（GetByID 路径）
func (r *channelRepository) loadAccountStatsPricingRules(ctx context.Context, channelID int64) ([]service.AccountStatsPricingRule, error) {
	result, err := r.batchLoadAccountStatsPricingRules(ctx, []int64{channelID})
	if err != nil {
		return nil, err
	}
	return result[channelID], nil
}

// replaceAccountStatsPricingRulesTx 在事务中替换渠道的账号统计定价规则
// (删除旧的 + 插入新的)。CASCADE 自动级联删除 model_pricing。
func replaceAccountStatsPricingRulesTx(ctx context.Context, tx *sql.Tx, channelID int64, rules []service.AccountStatsPricingRule) error {
	if _, err := tx.ExecContext(ctx,
		`DELETE FROM channel_account_stats_pricing_rules WHERE channel_id = $1`, channelID,
	); err != nil {
		return fmt.Errorf("delete old account stats pricing rules: %w", err)
	}

	for i := range rules {
		rules[i].ChannelID = channelID
		if rules[i].SortOrder == 0 {
			rules[i].SortOrder = i
		}
		if err := createAccountStatsPricingRuleTx(ctx, tx, &rules[i]); err != nil {
			return fmt.Errorf("insert account stats pricing rule: %w", err)
		}
	}
	return nil
}

// createAccountStatsPricingRuleTx 在事务中创建单条规则及其模型定价行
func createAccountStatsPricingRuleTx(ctx context.Context, tx *sql.Tx, rule *service.AccountStatsPricingRule) error {
	groupIDs := rule.GroupIDs
	if groupIDs == nil {
		groupIDs = []int64{}
	}
	accountIDs := rule.AccountIDs
	if accountIDs == nil {
		accountIDs = []int64{}
	}
	err := tx.QueryRowContext(ctx,
		`INSERT INTO channel_account_stats_pricing_rules
		 (channel_id, name, group_ids, account_ids, sort_order)
		 VALUES ($1, $2, $3, $4, $5)
		 RETURNING id, created_at, updated_at`,
		rule.ChannelID, rule.Name, pq.Array(groupIDs), pq.Array(accountIDs), rule.SortOrder,
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

// createAccountStatsModelPricingTx 在事务中插入规则下的一条模型定价行
func createAccountStatsModelPricingTx(ctx context.Context, tx *sql.Tx, ruleID int64, pricing *service.ChannelModelPricing) error {
	models := pricing.Models
	if models == nil {
		models = []string{}
	}
	modelsJSON, err := json.Marshal(models)
	if err != nil {
		return fmt.Errorf("marshal account stats pricing models: %w", err)
	}
	billingMode := pricing.BillingMode
	if billingMode == "" {
		billingMode = service.BillingModeToken
	}
	err = tx.QueryRowContext(ctx,
		`INSERT INTO channel_account_stats_model_pricing
		 (rule_id, platform, models, billing_mode,
		  input_price, output_price, cache_write_price, cache_read_price,
		  image_output_price, per_request_price)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)
		 RETURNING id, created_at, updated_at`,
		ruleID, pricing.Platform, modelsJSON, string(billingMode),
		pricing.InputPrice, pricing.OutputPrice, pricing.CacheWritePrice, pricing.CacheReadPrice,
		pricing.ImageOutputPrice, pricing.PerRequestPrice,
	).Scan(&pricing.ID, &pricing.CreatedAt, &pricing.UpdatedAt)
	if err != nil {
		return fmt.Errorf("insert account stats model pricing: %w", err)
	}
	return nil
}
