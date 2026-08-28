package repository

import (
	"context"
	"database/sql"
	"fmt"
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
	// db enables multi-statement transactions (SetGroupMembers). Nil in pure mocks.
	db *sql.DB
}

func NewProxyGroupRepository(client *dbent.Client, sqlDB *sql.DB) service.ProxyGroupRepository {
	return newProxyGroupRepositoryWithSQL(client, sqlDB)
}

func newProxyGroupRepositoryWithSQL(client *dbent.Client, sqlq sqlExecutor) *proxyGroupRepository {
	r := &proxyGroupRepository{client: client, sql: sqlq}
	if db, ok := sqlq.(*sql.DB); ok {
		r.db = db
	}
	return r
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
	if err := r.writeGroupHealthThresholds(ctx, group.ID, group.HealthFailThreshold, group.HealthSuccessThreshold); err != nil {
		return err
	}
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
	g := proxyGroupEntityToService(m)
	r.attachGroupHealthThresholds(ctx, g)
	return g, nil
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
	if err := r.writeGroupHealthThresholds(ctx, group.ID, group.HealthFailThreshold, group.HealthSuccessThreshold); err != nil {
		return err
	}
	return nil
}

// CreateWithMembers persists group metadata, health thresholds and membership
// as one transaction. It is intentionally exposed as an optional repository
// capability so lightweight service tests can continue using simple stubs.
func (r *proxyGroupRepository) CreateWithMembers(ctx context.Context, group *service.ProxyGroup, proxyIDs []int64) error {
	if r == nil || r.db == nil || group == nil {
		return fmt.Errorf("atomic proxy group repository is unavailable")
	}
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()

	var description any
	if group.Description != "" {
		description = group.Description
	}
	err = tx.QueryRowContext(ctx, `
		INSERT INTO proxy_groups (
			name, description, strategy, sticky_by_account, status,
			health_fail_threshold, health_success_threshold, created_at, updated_at
		) VALUES ($1,$2,$3,$4,$5,$6,$7,NOW(),NOW())
		RETURNING id, created_at, updated_at`,
		group.Name, description, group.Strategy, group.StickyByAccount, group.Status,
		nullableInt(group.HealthFailThreshold), nullableInt(group.HealthSuccessThreshold),
	).Scan(&group.ID, &group.CreatedAt, &group.UpdatedAt)
	if err != nil {
		return err
	}
	if err := replaceProxyGroupMembersTx(ctx, tx, group.ID, proxyIDs); err != nil {
		return err
	}
	return tx.Commit()
}

// UpdateWithMembers updates metadata and, when proxyIDs is non-nil, replaces
// membership in the same transaction.
func (r *proxyGroupRepository) UpdateWithMembers(ctx context.Context, group *service.ProxyGroup, proxyIDs *[]int64) error {
	if r == nil || r.db == nil || group == nil {
		return fmt.Errorf("atomic proxy group repository is unavailable")
	}
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()

	var description any
	if group.Description != "" {
		description = group.Description
	}
	err = tx.QueryRowContext(ctx, `
		UPDATE proxy_groups
		SET name=$2, description=$3, strategy=$4, sticky_by_account=$5,
			status=$6, health_fail_threshold=$7, health_success_threshold=$8,
			updated_at=NOW()
		WHERE id=$1 AND deleted_at IS NULL
		RETURNING created_at, updated_at`,
		group.ID, group.Name, description, group.Strategy, group.StickyByAccount,
		group.Status, nullableInt(group.HealthFailThreshold), nullableInt(group.HealthSuccessThreshold),
	).Scan(&group.CreatedAt, &group.UpdatedAt)
	if err == sql.ErrNoRows {
		return service.ErrProxyGroupNotFound
	}
	if err != nil {
		return err
	}
	if proxyIDs != nil {
		if err := replaceProxyGroupMembersTx(ctx, tx, group.ID, *proxyIDs); err != nil {
			return err
		}
	}
	return tx.Commit()
}

// DeleteIfUnused serializes against account/proxy membership writes, rechecks
// references and soft-deletes only when the group is still unused.
func (r *proxyGroupRepository) DeleteIfUnused(ctx context.Context, groupID int64) error {
	if r == nil || r.db == nil || groupID <= 0 {
		return service.ErrProxyGroupNotFound
	}
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	// Group deletion is rare. A short table lock closes the logical-FK race with
	// concurrent account/group membership updates that cannot be expressed as a
	// PostgreSQL partial foreign key against deleted_at.
	if _, err := tx.ExecContext(ctx, `LOCK TABLE accounts, proxies IN SHARE ROW EXCLUSIVE MODE`); err != nil {
		return err
	}
	var exists bool
	if err := tx.QueryRowContext(ctx, `SELECT EXISTS(
		SELECT 1 FROM proxy_groups WHERE id=$1 AND deleted_at IS NULL
	)`, groupID).Scan(&exists); err != nil {
		return err
	}
	if !exists {
		return service.ErrProxyGroupNotFound
	}
	var inUse bool
	if err := tx.QueryRowContext(ctx, `SELECT EXISTS(
		SELECT 1 FROM proxies WHERE group_id=$1 AND deleted_at IS NULL
		UNION ALL
		SELECT 1 FROM accounts WHERE proxy_group_id=$1 AND deleted_at IS NULL
	)`, groupID).Scan(&inUse); err != nil {
		return err
	}
	if inUse {
		return service.ErrProxyGroupInUse
	}
	if _, err := tx.ExecContext(ctx, `
		UPDATE proxy_groups SET deleted_at=NOW(), updated_at=NOW()
		WHERE id=$1 AND deleted_at IS NULL`, groupID); err != nil {
		return err
	}
	return tx.Commit()
}

func nullableInt(v *int) any {
	if v == nil || *v <= 0 {
		return nil
	}
	return *v
}

func replaceProxyGroupMembersTx(ctx context.Context, tx *sql.Tx, groupID int64, proxyIDs []int64) error {
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
	if _, err := tx.ExecContext(ctx, `
		UPDATE proxies SET group_id=NULL, updated_at=NOW()
		WHERE group_id=$1 AND deleted_at IS NULL`, groupID); err != nil {
		return err
	}
	if len(uniq) == 0 {
		return nil
	}
	_, err := tx.ExecContext(ctx, `
		UPDATE proxies SET group_id=$1, updated_at=NOW()
		WHERE id=ANY($2) AND deleted_at IS NULL`, groupID, pq.Array(uniq))
	return err
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
	r.attachGroupHealthThresholdsBatch(ctx, out)
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
	r.attachGroupHealthThresholdsBatch(ctx, out)
	return out, nil
}

func (r *proxyGroupRepository) writeGroupHealthThresholds(ctx context.Context, groupID int64, fail, success *int) error {
	if r == nil || r.sql == nil || groupID <= 0 {
		return nil
	}
	var failV, succV any
	if fail != nil && *fail > 0 {
		failV = *fail
	}
	if success != nil && *success > 0 {
		succV = *success
	}
	_, err := r.sql.ExecContext(ctx, `
		UPDATE proxy_groups
		SET health_fail_threshold = $2,
		    health_success_threshold = $3,
		    updated_at = NOW()
		WHERE id = $1 AND deleted_at IS NULL`,
		groupID, failV, succV,
	)
	return err
}

func (r *proxyGroupRepository) attachGroupHealthThresholds(ctx context.Context, g *service.ProxyGroup) {
	if g == nil || r == nil || r.sql == nil {
		return
	}
	var fail, succ sql.NullInt64
	err := scanSingleRow(ctx, r.sql, `
		SELECT health_fail_threshold, health_success_threshold
		FROM proxy_groups WHERE id = $1 AND deleted_at IS NULL`,
		[]any{g.ID}, &fail, &succ)
	if err != nil {
		return
	}
	if fail.Valid && fail.Int64 > 0 {
		v := int(fail.Int64)
		g.HealthFailThreshold = &v
	}
	if succ.Valid && succ.Int64 > 0 {
		v := int(succ.Int64)
		g.HealthSuccessThreshold = &v
	}
}

func (r *proxyGroupRepository) attachGroupHealthThresholdsBatch(ctx context.Context, groups []service.ProxyGroup) {
	if len(groups) == 0 || r == nil || r.sql == nil {
		return
	}
	ids := make([]int64, 0, len(groups))
	index := make(map[int64]int, len(groups))
	for i := range groups {
		ids = append(ids, groups[i].ID)
		index[groups[i].ID] = i
	}
	rows, err := r.sql.QueryContext(ctx, `
		SELECT id, health_fail_threshold, health_success_threshold
		FROM proxy_groups WHERE id = ANY($1) AND deleted_at IS NULL`, pq.Array(ids),
	)
	if err != nil {
		return
	}
	defer func() { _ = rows.Close() }()
	for rows.Next() {
		var id int64
		var fail, succ sql.NullInt64
		if err := rows.Scan(&id, &fail, &succ); err != nil {
			continue
		}
		i, ok := index[id]
		if !ok {
			continue
		}
		if fail.Valid && fail.Int64 > 0 {
			v := int(fail.Int64)
			groups[i].HealthFailThreshold = &v
		}
		if succ.Valid && succ.Int64 > 0 {
			v := int(succ.Int64)
			groups[i].HealthSuccessThreshold = &v
		}
	}
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
// Clear + assign run in one transaction when *sql.DB is available so a failed
// second step cannot leave the group empty.
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

	apply := func(ex sqlExecutor) error {
		if _, err := ex.ExecContext(ctx,
			`UPDATE proxies SET group_id = NULL, updated_at = NOW() WHERE group_id = $1 AND deleted_at IS NULL`,
			groupID,
		); err != nil {
			return err
		}
		if len(uniq) == 0 {
			return nil
		}
		if _, err := ex.ExecContext(ctx,
			`UPDATE proxies SET group_id = $1, updated_at = NOW() WHERE id = ANY($2) AND deleted_at IS NULL`,
			groupID, pq.Array(uniq),
		); err != nil {
			return err
		}
		return nil
	}

	if r.db != nil {
		tx, err := r.db.BeginTx(ctx, nil)
		if err != nil {
			return err
		}
		defer func() { _ = tx.Rollback() }()
		if err := apply(tx); err != nil {
			return err
		}
		return tx.Commit()
	}
	return apply(r.sql)
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
