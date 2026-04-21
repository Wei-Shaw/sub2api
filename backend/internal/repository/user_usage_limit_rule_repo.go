package repository

import (
	"context"

	dbent "github.com/Wei-Shaw/sub2api/ent"
	"github.com/Wei-Shaw/sub2api/ent/user"
	"github.com/Wei-Shaw/sub2api/ent/userusagelimitrule"
	"github.com/Wei-Shaw/sub2api/internal/service"
)

// userUsageLimitRuleRepository 用户配额规则仓储实现。
//
// 职责：规则行的 CRUD；业务校验（group_ids 重叠、订阅分组禁止、正数校验等）由
// quota_service 负责。
type userUsageLimitRuleRepository struct {
	client *dbent.Client
}

// NewUserUsageLimitRuleRepository 构造仓储实例
func NewUserUsageLimitRuleRepository(client *dbent.Client) service.UserUsageLimitRuleRepository {
	return &userUsageLimitRuleRepository{client: client}
}

func (r *userUsageLimitRuleRepository) ListByUser(ctx context.Context, userID int64) ([]*service.QuotaRule, error) {
	rows, err := r.client.UserUsageLimitRule.Query().
		Where(userusagelimitrule.UserIDEQ(userID)).
		Order(userusagelimitrule.ByID()).
		All(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]*service.QuotaRule, 0, len(rows))
	for _, row := range rows {
		out = append(out, ruleEntityToService(row))
	}
	return out, nil
}

func (r *userUsageLimitRuleRepository) GetByIDForUser(ctx context.Context, userID, ruleID int64) (*service.QuotaRule, error) {
	row, err := r.client.UserUsageLimitRule.Query().
		Where(
			userusagelimitrule.IDEQ(ruleID),
			userusagelimitrule.UserIDEQ(userID),
		).
		Only(ctx)
	if err != nil {
		if dbent.IsNotFound(err) {
			return nil, service.ErrQuotaRuleNotFound
		}
		return nil, err
	}
	return ruleEntityToService(row), nil
}

func (r *userUsageLimitRuleRepository) Create(ctx context.Context, userID int64, req service.CreateRuleRequest) (*service.QuotaRule, error) {
	period := req.Period
	if period == "" {
		period = service.QuotaPeriodDaily
	}
	row, err := r.client.UserUsageLimitRule.Create().
		SetUserID(userID).
		SetGroupIds(req.GroupIDs).
		SetDailyLimitUsd(req.DailyLimitUSD).
		SetPeriod(period).
		Save(ctx)
	if err != nil {
		return nil, err
	}
	return ruleEntityToService(row), nil
}

func (r *userUsageLimitRuleRepository) Update(ctx context.Context, userID, ruleID int64, req service.UpdateRuleRequest) (*service.QuotaRule, error) {
	existing, err := r.GetByIDForUser(ctx, userID, ruleID)
	if err != nil {
		return nil, err
	}
	update := r.client.UserUsageLimitRule.UpdateOneID(ruleID)
	if req.GroupIDs != nil {
		update = update.SetGroupIds(*req.GroupIDs)
	}
	if req.DailyLimitUSD != nil {
		update = update.SetDailyLimitUsd(*req.DailyLimitUSD)
	}
	row, err := update.Save(ctx)
	if err != nil {
		if dbent.IsNotFound(err) {
			return nil, service.ErrQuotaRuleNotFound
		}
		return nil, err
	}
	_ = existing // 预留给审计日志；目前仅用于 NotFound 前置校验
	return ruleEntityToService(row), nil
}

// ReplaceAll 单事务内清空指定用户所有规则并批量插入新规则。
//
// 校验由 service 层完成，repo 只负责事务边界：
//  1. SELECT users.id FOR UPDATE（行锁，串行化同一用户的规则变更，防止竞态覆盖）
//  2. DELETE FROM user_usage_limit_rule WHERE user_id = ?
//  3. 批量 INSERT（CreateBulk 单条 SQL）
//  4. 任何一步失败 → Rollback；全部成功 → Commit
//
// 事务 DELETE + INSERT 序列必须在同一函数内，以保证失败时 defer tx.Rollback() 能
// 统一回滚。拆出子函数会让事务边界跨函数，不符合"事务是持久化细节，在 repo 层一次性
// 落地"的原则。
func (r *userUsageLimitRuleRepository) ReplaceAll(ctx context.Context, userID int64, rules []service.CreateRuleRequest) ([]*service.QuotaRule, error) {
	tx, err := r.client.Tx(ctx)
	if err != nil {
		return nil, err
	}
	defer func() { _ = tx.Rollback() }()

	// 1. 对 users 行加 FOR UPDATE 锁，串行化同一用户的规则变更。
	exists, err := tx.User.Query().
		Where(user.IDEQ(userID)).
		ForUpdate().
		Exist(ctx)
	if err != nil {
		return nil, err
	}
	if !exists {
		return nil, service.ErrUserNotFound
	}

	// 2. 清空旧规则。
	if _, err := tx.UserUsageLimitRule.Delete().
		Where(userusagelimitrule.UserIDEQ(userID)).
		Exec(ctx); err != nil {
		return nil, err
	}

	// 3. 批量 INSERT（一次 SQL 写入）。
	builders := make([]*dbent.UserUsageLimitRuleCreate, 0, len(rules))
	for _, req := range rules {
		period := req.Period
		if period == "" {
			period = service.QuotaPeriodDaily
		}
		builders = append(builders, tx.UserUsageLimitRule.Create().
			SetUserID(userID).
			SetGroupIds(req.GroupIDs).
			SetDailyLimitUsd(req.DailyLimitUSD).
			SetPeriod(period),
		)
	}
	inserted := make([]*service.QuotaRule, 0, len(rules))
	if len(builders) > 0 {
		rows, err := tx.UserUsageLimitRule.CreateBulk(builders...).Save(ctx)
		if err != nil {
			return nil, err
		}
		for _, row := range rows {
			inserted = append(inserted, ruleEntityToService(row))
		}
	}

	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return inserted, nil
}

func (r *userUsageLimitRuleRepository) Delete(ctx context.Context, userID, ruleID int64) error {
	affected, err := r.client.UserUsageLimitRule.Delete().
		Where(
			userusagelimitrule.IDEQ(ruleID),
			userusagelimitrule.UserIDEQ(userID),
		).
		Exec(ctx)
	if err != nil {
		return err
	}
	if affected == 0 {
		return service.ErrQuotaRuleNotFound
	}
	return nil
}

// ruleEntityToService 转换 ent 行到 service DTO
func ruleEntityToService(row *dbent.UserUsageLimitRule) *service.QuotaRule {
	if row == nil {
		return nil
	}
	groupIDs := append([]int64(nil), row.GroupIds...)
	return &service.QuotaRule{
		ID:            row.ID,
		UserID:        row.UserID,
		GroupIDs:      groupIDs,
		DailyLimitUSD: row.DailyLimitUsd,
		Period:        row.Period,
		CreatedAt:     row.CreatedAt,
		UpdatedAt:     row.UpdatedAt,
	}
}
