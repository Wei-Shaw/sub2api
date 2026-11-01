package repository

import (
	"context"
	"time"

	dbent "github.com/Wei-Shaw/sub2api/ent"
	"github.com/Wei-Shaw/sub2api/ent/group"
	"github.com/Wei-Shaw/sub2api/ent/predicate"
	"github.com/Wei-Shaw/sub2api/internal/pkg/adminaccess"
	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
	"github.com/Wei-Shaw/sub2api/internal/pkg/pagination"
	"github.com/Wei-Shaw/sub2api/internal/service"
)

func (r *groupRepository) ScopedAdminUpdateGroupModels(ctx context.Context, id int64, cfg service.GroupModelAllowlist) (*service.Group, error) {
	p := adminaccess.FromContext(ctx)
	if p == nil {
		return nil, infraerrors.Forbidden("SCOPED_GROUP_MODELS_FORBIDDEN", "缺少受限身份")
	}
	tx, err := r.client.Tx(ctx)
	if err != nil {
		return nil, err
	}
	defer func() { _ = tx.Rollback() }()
	txctx := dbent.NewTxContext(ctx, tx)
	row, err := tx.Client().Group.Query().Where(group.IDEQ(id), group.DeletedAtIsNil()).
		Select(group.FieldID, group.FieldName, group.FieldPlatform, group.FieldModelAllowlist).ForUpdate().Only(txctx)
	if err != nil {
		return nil, translatePersistenceError(err, service.ErrGroupNotFound, nil)
	}
	if !p.Policy.AllowsGroupConfig("groups.models.write", row.ID, row.Platform) {
		return nil, infraerrors.NotFound("SCOPED_GROUP_NOT_FOUND", "分组不存在")
	}
	// 只写这一个业务字段，避免完整分组 Update 覆盖并发变化或修改费率等敏感配置。
	if _, err = tx.Client().Group.Update().Where(group.IDEQ(id), group.DeletedAtIsNil()).SetModelAllowlist(service.DomainGroupModelAllowlist(cfg)).SetUpdatedAt(time.Now()).Save(txctx); err != nil {
		return nil, err
	}
	if err = enqueueSchedulerOutbox(txctx, tx.Client(), service.SchedulerOutboxEventGroupChanged, nil, &id, nil); err != nil {
		return nil, err
	}
	if err = tx.Commit(); err != nil {
		return nil, err
	}
	return &service.Group{ID: row.ID, Name: row.Name, Platform: row.Platform, ModelAllowlist: cfg}, nil
}

// ScopedAdminListGroupModels 在数据库内过滤范围，分页和总数均只包含已授权分组。
func (r *groupRepository) ScopedAdminListGroupModels(ctx context.Context, params pagination.PaginationParams, all bool, platform, status, search string) ([]service.Group, int64, error) {
	p := adminaccess.FromContext(ctx)
	if p == nil {
		return nil, 0, infraerrors.Forbidden("SCOPED_GROUP_MODELS_FORBIDDEN", "缺少受限身份")
	}
	alternatives := []predicate.Group{}
	for _, rule := range p.Policy.RulesFor("groups.models.read") {
		terms := []predicate.Group{group.IDGT(0)}
		if !rule.AllGroups {
			if len(rule.GroupIDs) == 0 {
				continue
			}
			terms = append(terms, group.IDIn(rule.GroupIDs...))
		}
		if len(rule.Platforms) > 0 {
			terms = append(terms, group.PlatformIn(rule.Platforms...))
		}
		alternatives = append(alternatives, group.And(terms...))
	}
	if len(alternatives) == 0 {
		return []service.Group{}, 0, nil
	}
	q := r.client.Group.Query().Where(group.DeletedAtIsNil(), group.Or(alternatives...))
	if platform != "" {
		q.Where(group.PlatformEQ(platform))
	}
	if status != "" {
		q.Where(group.StatusEQ(status))
	}
	if search != "" {
		q.Where(group.NameContainsFold(search))
	}
	total, err := q.Clone().Count(ctx)
	if err != nil {
		return nil, 0, err
	}
	q.Select(group.FieldID, group.FieldName, group.FieldPlatform, group.FieldModelAllowlist).Order(group.ByID())
	if !all {
		q.Offset(params.Offset()).Limit(params.Limit())
	}
	rows, err := q.All(ctx)
	if err != nil {
		return nil, 0, err
	}
	result := make([]service.Group, 0, len(rows))
	for _, row := range rows {
		result = append(result, service.Group{ID: row.ID, Name: row.Name, Platform: row.Platform, ModelAllowlist: service.GroupModelAllowlistFromDomain(row.ModelAllowlist)})
	}
	return result, int64(total), nil
}
