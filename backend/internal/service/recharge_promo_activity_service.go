package service

import (
	"context"
	"fmt"
	"strings"

	dbent "github.com/Wei-Shaw/sub2api/ent"
	"github.com/Wei-Shaw/sub2api/ent/rechargepromoactivity"
	"github.com/Wei-Shaw/sub2api/internal/domain"
	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
)

// RechargePromoActivityService 是充值赠送活动表的访问层（CRUD 列表语义）。
//
// 设计要点：
//   - 活动表（recharge_promo_activities）是充值赠送配置的 single source of truth，
//     不再走 system_settings.RECHARGE_PROMO；每条记录对应管理员列表 UI 的一行；
//   - 全表至多一行 enabled=TRUE（DB partial unique 兜底；SetEnabled 在事务内
//     先关旧再启新）；
//   - 当前生效活动 = WHERE enabled = TRUE LIMIT 1；
//   - service.RechargePromo.Version = "{id}:{updated_at_unix}"，编辑同一活动后
//     version 会变化，前端红点能重新点亮。
type RechargePromoActivityService struct {
	entClient *dbent.Client
}

// NewRechargePromoActivityService 构造活动表 service。
func NewRechargePromoActivityService(entClient *dbent.Client) *RechargePromoActivityService {
	return &RechargePromoActivityService{entClient: entClient}
}

// CreateActivityInput 是创建活动的输入。
type CreateActivityInput struct {
	Name     string
	Promo    *RechargePromo // tiers / valid_from / valid_until / enabled
	Operator string
	Note     string
}

// UpdateActivityInput 是部分更新活动的输入；指针字段为 nil 表示不修改。
type UpdateActivityInput struct {
	Name     *string
	Promo    *RechargePromo // 整体替换：tiers / valid_from / valid_until / enabled
	Operator string
	Note     *string
}

// Create 创建一条新的活动记录。
//
// 当 input.Promo.Enabled = true 时，会先在事务内将其他 enabled=TRUE 的行改为 false，
// 再插入本行；保证 partial unique 约束不会失败。
func (s *RechargePromoActivityService) Create(
	ctx context.Context,
	in CreateActivityInput,
) (*dbent.RechargePromoActivity, error) {
	if s == nil || s.entClient == nil {
		return nil, fmt.Errorf("recharge promo activity service not initialized")
	}
	if in.Promo == nil {
		return nil, infraerrors.BadRequest("INVALID_RECHARGE_PROMO", "promo payload is required")
	}
	name := strings.TrimSpace(in.Name)
	if name == "" {
		return nil, infraerrors.BadRequest("INVALID_RECHARGE_PROMO", "name is required")
	}
	if err := validateRechargePromo(in.Promo); err != nil {
		return nil, err
	}
	op := strings.TrimSpace(in.Operator)
	if op == "" {
		op = "system"
	}
	dt := tiersToDomain(in.Promo.Tiers)

	tx, err := s.entClient.Tx(ctx)
	if err != nil {
		return nil, fmt.Errorf("begin tx: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	if in.Promo.Enabled {
		if _, err := tx.RechargePromoActivity.Update().
			Where(rechargepromoactivity.EnabledEQ(true)).
			SetEnabled(false).
			Save(ctx); err != nil {
			return nil, fmt.Errorf("disable previous enabled activities: %w", err)
		}
	}

	builder := tx.RechargePromoActivity.Create().
		SetName(name).
		SetEnabled(in.Promo.Enabled).
		SetTiers(dt).
		SetOperator(op)
	if in.Promo.ValidFrom != nil {
		builder = builder.SetValidFrom(*in.Promo.ValidFrom)
	}
	if in.Promo.ValidUntil != nil {
		builder = builder.SetValidUntil(*in.Promo.ValidUntil)
	}
	if note := strings.TrimSpace(in.Note); note != "" {
		builder = builder.SetNote(note)
	}
	row, err := builder.Save(ctx)
	if err != nil {
		return nil, fmt.Errorf("save recharge promo activity: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("commit recharge promo create tx: %w", err)
	}
	return row, nil
}

// Update 部分更新活动记录。
// 如果 in.Promo != nil 且其 Enabled=true，会在同一事务内先关掉其他 enabled=TRUE 行。
func (s *RechargePromoActivityService) Update(
	ctx context.Context,
	id int64,
	in UpdateActivityInput,
) (*dbent.RechargePromoActivity, error) {
	if s == nil || s.entClient == nil {
		return nil, fmt.Errorf("recharge promo activity service not initialized")
	}
	if id <= 0 {
		return nil, infraerrors.BadRequest("INVALID_RECHARGE_PROMO", "invalid activity id")
	}
	if in.Promo != nil {
		if err := validateRechargePromo(in.Promo); err != nil {
			return nil, err
		}
	}
	if in.Name != nil {
		if strings.TrimSpace(*in.Name) == "" {
			return nil, infraerrors.BadRequest("INVALID_RECHARGE_PROMO", "name must not be empty")
		}
	}

	tx, err := s.entClient.Tx(ctx)
	if err != nil {
		return nil, fmt.Errorf("begin tx: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	// 校验存在性
	if _, err := tx.RechargePromoActivity.Get(ctx, id); err != nil {
		if dbent.IsNotFound(err) {
			return nil, infraerrors.NotFound("RECHARGE_PROMO_NOT_FOUND", "recharge promo activity not found")
		}
		return nil, fmt.Errorf("get activity %d: %w", id, err)
	}

	// 若本次更新要把活动设为 enabled=true，先关其他 enabled=true 的行
	if in.Promo != nil && in.Promo.Enabled {
		if _, err := tx.RechargePromoActivity.Update().
			Where(
				rechargepromoactivity.EnabledEQ(true),
				rechargepromoactivity.IDNEQ(id),
			).
			SetEnabled(false).
			Save(ctx); err != nil {
			return nil, fmt.Errorf("disable other enabled activities: %w", err)
		}
	}

	upd := tx.RechargePromoActivity.UpdateOneID(id)
	if in.Name != nil {
		upd = upd.SetName(strings.TrimSpace(*in.Name))
	}
	if in.Promo != nil {
		upd = upd.SetEnabled(in.Promo.Enabled).
			SetTiers(tiersToDomain(in.Promo.Tiers))
		if in.Promo.ValidFrom != nil {
			upd = upd.SetValidFrom(*in.Promo.ValidFrom)
		} else {
			upd = upd.ClearValidFrom()
		}
		if in.Promo.ValidUntil != nil {
			upd = upd.SetValidUntil(*in.Promo.ValidUntil)
		} else {
			upd = upd.ClearValidUntil()
		}
	}
	if op := strings.TrimSpace(in.Operator); op != "" {
		upd = upd.SetOperator(op)
	}
	if in.Note != nil {
		if note := strings.TrimSpace(*in.Note); note != "" {
			upd = upd.SetNote(note)
		} else {
			upd = upd.ClearNote()
		}
	}
	row, err := upd.Save(ctx)
	if err != nil {
		return nil, fmt.Errorf("update activity %d: %w", id, err)
	}
	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("commit update tx: %w", err)
	}
	return row, nil
}

// SetEnabled 单独切换活动启用状态。开启时事务内先关其他启用行。
func (s *RechargePromoActivityService) SetEnabled(
	ctx context.Context,
	id int64,
	enabled bool,
) (*dbent.RechargePromoActivity, error) {
	if s == nil || s.entClient == nil {
		return nil, fmt.Errorf("recharge promo activity service not initialized")
	}
	tx, err := s.entClient.Tx(ctx)
	if err != nil {
		return nil, fmt.Errorf("begin tx: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	row, err := tx.RechargePromoActivity.Get(ctx, id)
	if err != nil {
		if dbent.IsNotFound(err) {
			return nil, infraerrors.NotFound("RECHARGE_PROMO_NOT_FOUND", "recharge promo activity not found")
		}
		return nil, fmt.Errorf("get activity %d: %w", id, err)
	}
	if row.Enabled == enabled {
		// 幂等返回，避免无谓更新 updated_at。
		_ = tx.Commit()
		return row, nil
	}
	if enabled {
		// 把表上其他 enabled=true 的行先关掉。
		if _, err := tx.RechargePromoActivity.Update().
			Where(
				rechargepromoactivity.EnabledEQ(true),
				rechargepromoactivity.IDNEQ(id),
			).
			SetEnabled(false).
			Save(ctx); err != nil {
			return nil, fmt.Errorf("disable other enabled activities: %w", err)
		}
		// 启用前还需对 promo 内容做一次校验（避免开启一个非法活动）：
		p := &RechargePromo{
			Enabled:    true,
			ValidFrom:  row.ValidFrom,
			ValidUntil: row.ValidUntil,
			Tiers:      tiersFromDomain(row.Tiers),
		}
		if err := validateRechargePromo(p); err != nil {
			return nil, err
		}
	}
	row, err = tx.RechargePromoActivity.UpdateOneID(id).
		SetEnabled(enabled).
		Save(ctx)
	if err != nil {
		return nil, fmt.Errorf("set enabled %v on activity %d: %w", enabled, id, err)
	}
	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("commit toggle tx: %w", err)
	}
	return row, nil
}

// Delete 硬删除一条活动记录。orders.activity_id 是无外键的弱引用，历史订单不受影响。
func (s *RechargePromoActivityService) Delete(ctx context.Context, id int64) error {
	if s == nil || s.entClient == nil {
		return fmt.Errorf("recharge promo activity service not initialized")
	}
	if err := s.entClient.RechargePromoActivity.DeleteOneID(id).Exec(ctx); err != nil {
		if dbent.IsNotFound(err) {
			return infraerrors.NotFound("RECHARGE_PROMO_NOT_FOUND", "recharge promo activity not found")
		}
		return fmt.Errorf("delete activity %d: %w", id, err)
	}
	return nil
}

// GetCurrent 返回当前生效活动（enabled=TRUE，至多一行）。
// 当无任何 enabled 活动时返回 (nil, nil)，调用方将 RechargePromo 视作未启用。
func (s *RechargePromoActivityService) GetCurrent(ctx context.Context) (*dbent.RechargePromoActivity, error) {
	if s == nil || s.entClient == nil {
		return nil, nil
	}
	row, err := s.entClient.RechargePromoActivity.Query().
		Where(rechargepromoactivity.EnabledEQ(true)).
		First(ctx)
	if err != nil {
		if dbent.IsNotFound(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("query current recharge promo activity: %w", err)
	}
	return row, nil
}

// GetByID 用于 fulfillment / admin 详情查询。Not Found 时返回 (nil, nil)。
func (s *RechargePromoActivityService) GetByID(ctx context.Context, id int64) (*dbent.RechargePromoActivity, error) {
	if s == nil || s.entClient == nil {
		return nil, nil
	}
	row, err := s.entClient.RechargePromoActivity.Get(ctx, id)
	if err != nil {
		if dbent.IsNotFound(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("get recharge promo activity %d: %w", id, err)
	}
	return row, nil
}

// List 返回 created_at 倒序分页结果，供 admin 列表 UI 使用。
func (s *RechargePromoActivityService) List(
	ctx context.Context,
	page, pageSize int,
) ([]*dbent.RechargePromoActivity, int, error) {
	if s == nil || s.entClient == nil {
		return nil, 0, nil
	}
	if page < 1 {
		page = 1
	}
	if pageSize < 1 {
		pageSize = 20
	}
	if pageSize > 200 {
		pageSize = 200
	}
	q := s.entClient.RechargePromoActivity.Query()
	total, err := q.Clone().Count(ctx)
	if err != nil {
		return nil, 0, fmt.Errorf("count recharge promo activities: %w", err)
	}
	rows, err := q.
		Order(dbent.Desc(rechargepromoactivity.FieldCreatedAt)).
		Offset((page - 1) * pageSize).
		Limit(pageSize).
		All(ctx)
	if err != nil {
		return nil, 0, fmt.Errorf("list recharge promo activities: %w", err)
	}
	return rows, total, nil
}

// ActivityToPromo 把 ent 实体映射成 service 层的 *RechargePromo。
//
// Version 形如 "{id}:{updated_at_unix}"，编辑同一活动后 version 会变化，
// 前端红点能重新点亮。入参 nil 时返回 nil。
func ActivityToPromo(row *dbent.RechargePromoActivity) *RechargePromo {
	if row == nil {
		return nil
	}
	return &RechargePromo{
		Enabled:    row.Enabled,
		ValidFrom:  row.ValidFrom,
		ValidUntil: row.ValidUntil,
		Tiers:      tiersFromDomain(row.Tiers),
		Version:    fmt.Sprintf("%d:%d", row.ID, row.UpdatedAt.Unix()),
		ActivityID: row.ID,
	}
}

func tiersToDomain(in []RechargePromoTier) []domain.RechargePromoTier {
	out := make([]domain.RechargePromoTier, 0, len(in))
	for _, t := range in {
		out = append(out, domain.RechargePromoTier{
			MinAmount: t.MinAmount,
			BonusRate: t.BonusRate,
		})
	}
	return out
}

func tiersFromDomain(in []domain.RechargePromoTier) []RechargePromoTier {
	out := make([]RechargePromoTier, 0, len(in))
	for _, t := range in {
		out = append(out, RechargePromoTier{
			MinAmount: t.MinAmount,
			BonusRate: t.BonusRate,
		})
	}
	return out
}
