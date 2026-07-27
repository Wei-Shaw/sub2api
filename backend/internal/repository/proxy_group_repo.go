package repository

import (
	"context"
	"database/sql"
	"strings"

	dbent "github.com/Wei-Shaw/sub2api/ent"
	"github.com/Wei-Shaw/sub2api/ent/proxygroup"
	"github.com/Wei-Shaw/sub2api/internal/pkg/pagination"
	"github.com/Wei-Shaw/sub2api/internal/service"

	entsql "entgo.io/ent/dialect/sql"
	"github.com/lib/pq"
)

type proxyGroupRepository struct {
	client *dbent.Client
	sql    sqlExecutor
}

func NewProxyGroupRepository(client *dbent.Client, sqlDB *sql.DB) service.ProxyGroupRepository {
	return newProxyGroupRepositoryWithSQL(client, sqlDB)
}

func newProxyGroupRepositoryWithSQL(client *dbent.Client, sqlq sqlExecutor) *proxyGroupRepository {
	return &proxyGroupRepository{client: client, sql: sqlq}
}

func (r *proxyGroupRepository) Create(ctx context.Context, group *service.ProxyGroup) error {
	if group == nil {
		return nil
	}
	builder := r.client.ProxyGroup.Create().
		SetName(group.Name).
		SetStrategy(group.Strategy).
		SetStickyByAccount(group.StickyByAccount).
		SetStatus(group.Status)
	if group.Description != "" {
		builder.SetDescription(group.Description)
	}
	if group.Strategy == "" {
		builder.SetStrategy(service.ProxyGroupStrategyRoundRobin)
	}
	if group.Status == "" {
		builder.SetStatus(service.StatusActive)
	}
	created, err := builder.Save(ctx)
	if err != nil {
		return err
	}
	applyProxyGroupEntityToService(group, created)
	return nil
}

func (r *proxyGroupRepository) GetByID(ctx context.Context, id int64) (*service.ProxyGroup, error) {
	m, err := r.client.ProxyGroup.Get(ctx, id)
	if err != nil {
		if dbent.IsNotFound(err) {
			return nil, service.ErrProxyGroupNotFound
		}
		return nil, err
	}
	return proxyGroupEntityToService(m), nil
}

func (r *proxyGroupRepository) Update(ctx context.Context, group *service.ProxyGroup) error {
	if group == nil {
		return nil
	}
	builder := r.client.ProxyGroup.UpdateOneID(group.ID).
		SetName(group.Name).
		SetStrategy(group.Strategy).
		SetStickyByAccount(group.StickyByAccount).
		SetStatus(group.Status)
	if group.Description == "" {
		builder.ClearDescription()
	} else {
		builder.SetDescription(group.Description)
	}
	updated, err := builder.Save(ctx)
	if err != nil {
		if dbent.IsNotFound(err) {
			return service.ErrProxyGroupNotFound
		}
		return err
	}
	applyProxyGroupEntityToService(group, updated)
	return nil
}

func (r *proxyGroupRepository) Delete(ctx context.Context, id int64) error {
	err := r.client.ProxyGroup.DeleteOneID(id).Exec(ctx)
	if err != nil {
		if dbent.IsNotFound(err) {
			return service.ErrProxyGroupNotFound
		}
		return err
	}
	return nil
}

func (r *proxyGroupRepository) List(ctx context.Context, params pagination.PaginationParams) ([]service.ProxyGroup, *pagination.PaginationResult, error) {
	query := r.client.ProxyGroup.Query()
	total, err := query.Clone().Count(ctx)
	if err != nil {
		return nil, nil, err
	}
	itemsQuery := query.
		Offset(params.Offset()).
		Limit(params.Limit())
	for _, order := range proxyGroupListOrder(params) {
		itemsQuery = itemsQuery.Order(order)
	}
	items, err := itemsQuery.All(ctx)
	if err != nil {
		return nil, nil, err
	}
	out := make([]service.ProxyGroup, 0, len(items))
	for i := range items {
		if g := proxyGroupEntityToService(items[i]); g != nil {
			out = append(out, *g)
		}
	}
	return out, paginationResultFromTotal(int64(total), params), nil
}

func (r *proxyGroupRepository) ListActive(ctx context.Context) ([]service.ProxyGroup, error) {
	items, err := r.client.ProxyGroup.Query().
		Where(proxygroup.StatusEQ(service.StatusActive)).
		Order(dbent.Asc(proxygroup.FieldName)).
		Order(dbent.Asc(proxygroup.FieldID)).
		All(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]service.ProxyGroup, 0, len(items))
	for i := range items {
		if g := proxyGroupEntityToService(items[i]); g != nil {
			out = append(out, *g)
		}
	}
	return out, nil
}

func (r *proxyGroupRepository) CountProxiesByGroupID(ctx context.Context, groupID int64) (int64, error) {
	var c int64
	err := scanSingleRow(ctx, r.sql,
		`SELECT COUNT(*) FROM proxies WHERE group_id=$1 AND deleted_at IS NULL`,
		[]any{groupID}, &c)
	return c, err
}

func (r *proxyGroupRepository) CountAccountsByGroupID(ctx context.Context, groupID int64) (int64, error) {
	var c int64
	err := scanSingleRow(ctx, r.sql,
		`SELECT COUNT(*) FROM accounts WHERE proxy_group_id=$1 AND deleted_at IS NULL`,
		[]any{groupID}, &c)
	return c, err
}

// SetGroupMembers 全量替换组成员：先清空该组既有成员，再把 proxyIDs 写入 group_id。
func (r *proxyGroupRepository) SetGroupMembers(ctx context.Context, groupID int64, proxyIDs []int64) error {
	if groupID <= 0 {
		return service.ErrProxyGroupNotFound
	}
	// 去重并过滤非法 id
	uniq := make([]int64, 0, len(proxyIDs))
	seen := make(map[int64]struct{}, len(proxyIDs))
	for _, id := range proxyIDs {
		if id <= 0 {
			continue
		}
		if _, ok := seen[id]; ok {
			continue
		}
		seen[id] = struct{}{}
		uniq = append(uniq, id)
	}

	if _, err := r.sql.ExecContext(ctx,
		`UPDATE proxies SET group_id = NULL, updated_at = NOW() WHERE group_id = $1 AND deleted_at IS NULL`,
		groupID,
	); err != nil {
		return err
	}
	if len(uniq) == 0 {
		return nil
	}
	if _, err := r.sql.ExecContext(ctx,
		`UPDATE proxies SET group_id = $1, updated_at = NOW() WHERE id = ANY($2) AND deleted_at IS NULL`,
		groupID, pq.Array(uniq),
	); err != nil {
		return err
	}
	return nil
}

func proxyGroupListOrder(params pagination.PaginationParams) []func(*entsql.Selector) {
	sortBy := strings.ToLower(strings.TrimSpace(params.SortBy))
	sortOrder := params.NormalizedSortOrder(pagination.SortOrderDesc)
	var field string
	switch sortBy {
	case "name":
		field = proxygroup.FieldName
	case "status":
		field = proxygroup.FieldStatus
	case "strategy":
		field = proxygroup.FieldStrategy
	case "created_at":
		field = proxygroup.FieldCreatedAt
	default:
		field = proxygroup.FieldID
	}
	if sortOrder == pagination.SortOrderAsc {
		return []func(*entsql.Selector){dbent.Asc(field), dbent.Asc(proxygroup.FieldID)}
	}
	return []func(*entsql.Selector){dbent.Desc(field), dbent.Desc(proxygroup.FieldID)}
}

func proxyGroupEntityToService(m *dbent.ProxyGroup) *service.ProxyGroup {
	if m == nil {
		return nil
	}
	out := &service.ProxyGroup{
		ID:              m.ID,
		Name:            m.Name,
		Strategy:        m.Strategy,
		StickyByAccount: m.StickyByAccount,
		Status:          m.Status,
		CreatedAt:       m.CreatedAt,
		UpdatedAt:       m.UpdatedAt,
	}
	if m.Description != nil {
		out.Description = *m.Description
	}
	return out
}

func applyProxyGroupEntityToService(dst *service.ProxyGroup, src *dbent.ProxyGroup) {
	if dst == nil || src == nil {
		return
	}
	dst.ID = src.ID
	dst.CreatedAt = src.CreatedAt
	dst.UpdatedAt = src.UpdatedAt
	dst.Name = src.Name
	dst.Strategy = src.Strategy
	dst.StickyByAccount = src.StickyByAccount
	dst.Status = src.Status
	if src.Description != nil {
		dst.Description = *src.Description
	} else {
		dst.Description = ""
	}
}
