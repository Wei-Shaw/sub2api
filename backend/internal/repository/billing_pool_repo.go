package repository

import (
	"context"
	"fmt"
	"strings"
	"time"

	dbent "github.com/Wei-Shaw/sub2api/ent"
	"github.com/Wei-Shaw/sub2api/ent/billingpool"
	"github.com/Wei-Shaw/sub2api/ent/billingpoolgroup"
	"github.com/Wei-Shaw/sub2api/internal/pkg/pagination"
	"github.com/Wei-Shaw/sub2api/internal/service"

	entsql "entgo.io/ent/dialect/sql"
)

type billingPoolRepository struct {
	client *dbent.Client
}

func NewBillingPoolRepository(client *dbent.Client) service.BillingPoolRepository {
	return &billingPoolRepository{client: client}
}

func (r *billingPoolRepository) activeQuery() *dbent.BillingPoolQuery {
	return r.client.BillingPool.Query().Where(billingpool.DeletedAtIsNil())
}

func (r *billingPoolRepository) withMembers(q *dbent.BillingPoolQuery) *dbent.BillingPoolQuery {
	return q.WithMembers(func(mq *dbent.BillingPoolGroupQuery) {
		mq.Where(billingpoolgroup.DeletedAtIsNil())
		mq.Order(dbent.Asc(billingpoolgroup.FieldChainOrder), dbent.Asc(billingpoolgroup.FieldGroupID))
		mq.WithGroup()
	})
}

func (r *billingPoolRepository) withWriteTx(ctx context.Context, fn func(txCtx context.Context, client *dbent.Client) error) error {
	if tx := dbent.TxFromContext(ctx); tx != nil {
		return fn(ctx, tx.Client())
	}

	tx, err := r.client.Tx(ctx)
	if err != nil {
		return err
	}
	txCtx := dbent.NewTxContext(ctx, tx)
	if err := fn(txCtx, tx.Client()); err != nil {
		if rbErr := tx.Rollback(); rbErr != nil {
			return fmt.Errorf("%w: rollback billing pool tx: %v", err, rbErr)
		}
		return err
	}
	return tx.Commit()
}

func (r *billingPoolRepository) Create(ctx context.Context, pool *service.BillingPool) error {
	if pool == nil {
		return fmt.Errorf("billing pool is required")
	}
	return r.withWriteTx(ctx, func(txCtx context.Context, client *dbent.Client) error {
		created, err := client.BillingPool.Create().
			SetName(pool.Name).
			SetCode(pool.Code).
			SetDescription(pool.Description).
			SetStatus(pool.Status).
			SetPlatformScope(pool.PlatformScope).
			SetAllowUserReorder(pool.AllowUserReorder).
			SetRequirePrimarySubscription(pool.RequirePrimarySubscription).
			SetAllowBalanceFallback(pool.AllowBalanceFallback).
			Save(txCtx)
		if err != nil {
			return translatePersistenceError(err, nil, service.ErrBillingPoolExists)
		}
		pool.ID = created.ID
		pool.CreatedAt = created.CreatedAt
		pool.UpdatedAt = created.UpdatedAt
		return r.replaceMembersWithClient(txCtx, client, pool.ID, pool.Members, false)
	})
}

func (r *billingPoolRepository) GetByID(ctx context.Context, id int64) (*service.BillingPool, error) {
	m, err := r.withMembers(r.activeQuery()).Where(billingpool.IDEQ(id)).Only(ctx)
	if err != nil {
		return nil, translatePersistenceError(err, service.ErrBillingPoolNotFound, nil)
	}
	return billingPoolEntityToService(m), nil
}

func (r *billingPoolRepository) GetByCode(ctx context.Context, code string) (*service.BillingPool, error) {
	m, err := r.withMembers(r.activeQuery()).Where(billingpool.CodeEQ(strings.TrimSpace(code))).Only(ctx)
	if err != nil {
		return nil, translatePersistenceError(err, service.ErrBillingPoolNotFound, nil)
	}
	return billingPoolEntityToService(m), nil
}

func (r *billingPoolRepository) Update(ctx context.Context, pool *service.BillingPool) error {
	if pool == nil {
		return fmt.Errorf("billing pool is required")
	}
	client := clientFromContext(ctx, r.client)
	updated, err := client.BillingPool.UpdateOneID(pool.ID).
		SetName(pool.Name).
		SetCode(pool.Code).
		SetDescription(pool.Description).
		SetStatus(pool.Status).
		SetPlatformScope(pool.PlatformScope).
		SetAllowUserReorder(pool.AllowUserReorder).
		SetRequirePrimarySubscription(pool.RequirePrimarySubscription).
		SetAllowBalanceFallback(pool.AllowBalanceFallback).
		Save(ctx)
	if err != nil {
		return translatePersistenceError(err, service.ErrBillingPoolNotFound, service.ErrBillingPoolExists)
	}
	pool.CreatedAt = updated.CreatedAt
	pool.UpdatedAt = updated.UpdatedAt
	return nil
}

func (r *billingPoolRepository) Delete(ctx context.Context, id int64) error {
	return r.withWriteTx(ctx, func(txCtx context.Context, client *dbent.Client) error {
		now := time.Now()
		if _, err := client.BillingPoolGroup.Update().
			Where(billingpoolgroup.BillingPoolIDEQ(id), billingpoolgroup.DeletedAtIsNil()).
			SetDeletedAt(now).
			SetUpdatedAt(now).
			Save(txCtx); err != nil {
			return err
		}
		affected, err := client.BillingPool.Update().
			Where(billingpool.IDEQ(id), billingpool.DeletedAtIsNil()).
			SetDeletedAt(now).
			SetUpdatedAt(now).
			Save(txCtx)
		if err != nil {
			return err
		}
		if affected == 0 {
			return service.ErrBillingPoolNotFound
		}
		return nil
	})
}

func (r *billingPoolRepository) List(ctx context.Context, params pagination.PaginationParams, filters service.BillingPoolListFilters) ([]service.BillingPool, *pagination.PaginationResult, error) {
	q := r.activeQuery()
	if status := strings.TrimSpace(filters.Status); status != "" {
		q = q.Where(billingpool.StatusEQ(status))
	}
	if platformScope := strings.TrimSpace(filters.PlatformScope); platformScope != "" {
		q = q.Where(billingpool.PlatformScopeEQ(platformScope))
	}
	if search := strings.TrimSpace(filters.Search); search != "" {
		q = q.Where(billingpool.Or(
			billingpool.NameContainsFold(search),
			billingpool.CodeContainsFold(search),
			billingpool.DescriptionContainsFold(search),
		))
	}

	total, err := q.Clone().Count(ctx)
	if err != nil {
		return nil, nil, err
	}

	items, err := r.withMembers(q).
		Offset(params.Offset()).
		Limit(params.Limit()).
		Order(billingPoolListOrder(params)...).
		All(ctx)
	if err != nil {
		return nil, nil, err
	}

	out := make([]service.BillingPool, 0, len(items))
	for i := range items {
		mapped := billingPoolEntityToService(items[i])
		if mapped != nil {
			out = append(out, *mapped)
		}
	}
	return out, paginationResultFromTotal(int64(total), params), nil
}

func (r *billingPoolRepository) ListLookup(ctx context.Context) ([]service.BillingPoolLookup, error) {
	items, err := r.activeQuery().
		Order(billingpool.ByName(entsql.OrderAsc()), billingpool.ByID(entsql.OrderAsc())).
		All(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]service.BillingPoolLookup, 0, len(items))
	for i := range items {
		out = append(out, service.BillingPoolLookup{
			ID:            items[i].ID,
			Name:          items[i].Name,
			Status:        items[i].Status,
			PlatformScope: items[i].PlatformScope,
		})
	}
	return out, nil
}

func (r *billingPoolRepository) GetByGroupID(ctx context.Context, groupID int64) (*service.BillingPool, error) {
	m, err := r.withMembers(r.activeQuery()).
		Where(billingpool.HasMembersWith(
			billingpoolgroup.GroupIDEQ(groupID),
			billingpoolgroup.DeletedAtIsNil(),
		)).
		First(ctx)
	if err != nil {
		return nil, translatePersistenceError(err, service.ErrBillingPoolNotFound, nil)
	}
	return billingPoolEntityToService(m), nil
}

func (r *billingPoolRepository) ReplaceMembers(ctx context.Context, poolID int64, members []service.BillingPoolMember) error {
	return r.withWriteTx(ctx, func(txCtx context.Context, client *dbent.Client) error {
		return r.replaceMembersWithClient(txCtx, client, poolID, members, true)
	})
}

func (r *billingPoolRepository) replaceMembersWithClient(ctx context.Context, client *dbent.Client, poolID int64, members []service.BillingPoolMember, requirePool bool) error {
	if requirePool {
		exists, err := client.BillingPool.Query().
			Where(billingpool.IDEQ(poolID), billingpool.DeletedAtIsNil()).
			Exist(ctx)
		if err != nil {
			return err
		}
		if !exists {
			return service.ErrBillingPoolNotFound
		}
	}

	now := time.Now()
	if _, err := client.BillingPoolGroup.Update().
		Where(billingpoolgroup.BillingPoolIDEQ(poolID), billingpoolgroup.DeletedAtIsNil()).
		SetDeletedAt(now).
		SetUpdatedAt(now).
		Save(ctx); err != nil {
		return err
	}
	for i := range members {
		member := members[i]
		builder := client.BillingPoolGroup.Create().
			SetBillingPoolID(poolID).
			SetGroupID(member.GroupID).
			SetChainOrder(member.ChainOrder).
			SetCanBePrimary(member.CanBePrimary).
			SetCanBeFallback(member.CanBeFallback)
		if !member.CreatedAt.IsZero() {
			builder.SetCreatedAt(member.CreatedAt)
		}
		if !member.UpdatedAt.IsZero() {
			builder.SetUpdatedAt(member.UpdatedAt)
		}
		if _, err := builder.Save(ctx); err != nil {
			return translatePersistenceError(err, nil, service.ErrBillingPoolGroupNotAllowed)
		}
	}
	return nil
}

func billingPoolListOrder(params pagination.PaginationParams) []billingpool.OrderOption {
	sortBy := strings.ToLower(strings.TrimSpace(params.SortBy))
	sortOrder := params.NormalizedSortOrder(pagination.SortOrderDesc)

	updatedAtOrders := func() []billingpool.OrderOption {
		if sortOrder == pagination.SortOrderAsc {
			return []billingpool.OrderOption{billingpool.ByUpdatedAt(entsql.OrderAsc(), entsql.OrderNullsLast()), billingpool.ByID(entsql.OrderAsc())}
		}
		return []billingpool.OrderOption{billingpool.ByUpdatedAt(entsql.OrderDesc(), entsql.OrderNullsLast()), billingpool.ByID(entsql.OrderDesc())}
	}

	switch sortBy {
	case "id":
		if sortOrder == pagination.SortOrderAsc {
			return []billingpool.OrderOption{billingpool.ByID(entsql.OrderAsc())}
		}
		return []billingpool.OrderOption{billingpool.ByID(entsql.OrderDesc())}
	case "name":
		if sortOrder == pagination.SortOrderAsc {
			return []billingpool.OrderOption{billingpool.ByName(entsql.OrderAsc()), billingpool.ByID(entsql.OrderAsc())}
		}
		return []billingpool.OrderOption{billingpool.ByName(entsql.OrderDesc()), billingpool.ByID(entsql.OrderDesc())}
	case "code":
		if sortOrder == pagination.SortOrderAsc {
			return []billingpool.OrderOption{billingpool.ByCode(entsql.OrderAsc()), billingpool.ByID(entsql.OrderAsc())}
		}
		return []billingpool.OrderOption{billingpool.ByCode(entsql.OrderDesc()), billingpool.ByID(entsql.OrderDesc())}
	case "status":
		if sortOrder == pagination.SortOrderAsc {
			return []billingpool.OrderOption{billingpool.ByStatus(entsql.OrderAsc()), billingpool.ByID(entsql.OrderAsc())}
		}
		return []billingpool.OrderOption{billingpool.ByStatus(entsql.OrderDesc()), billingpool.ByID(entsql.OrderDesc())}
	case "platform_scope":
		if sortOrder == pagination.SortOrderAsc {
			return []billingpool.OrderOption{billingpool.ByPlatformScope(entsql.OrderAsc()), billingpool.ByID(entsql.OrderAsc())}
		}
		return []billingpool.OrderOption{billingpool.ByPlatformScope(entsql.OrderDesc()), billingpool.ByID(entsql.OrderDesc())}
	case "created_at":
		if sortOrder == pagination.SortOrderAsc {
			return []billingpool.OrderOption{billingpool.ByCreatedAt(entsql.OrderAsc()), billingpool.ByID(entsql.OrderAsc())}
		}
		return []billingpool.OrderOption{billingpool.ByCreatedAt(entsql.OrderDesc()), billingpool.ByID(entsql.OrderDesc())}
	case "updated_at", "":
		return updatedAtOrders()
	default:
		return updatedAtOrders()
	}
}

func billingPoolEntityToService(m *dbent.BillingPool) *service.BillingPool {
	if m == nil {
		return nil
	}
	out := &service.BillingPool{
		ID:                         m.ID,
		Name:                       m.Name,
		Code:                       m.Code,
		Description:                derefString(m.Description),
		Status:                     m.Status,
		PlatformScope:              m.PlatformScope,
		AllowUserReorder:           m.AllowUserReorder,
		RequirePrimarySubscription: m.RequirePrimarySubscription,
		AllowBalanceFallback:       m.AllowBalanceFallback,
		CreatedAt:                  m.CreatedAt,
		UpdatedAt:                  m.UpdatedAt,
	}
	if len(m.Edges.Members) > 0 {
		out.Members = make([]service.BillingPoolMember, 0, len(m.Edges.Members))
		for i := range m.Edges.Members {
			out.Members = append(out.Members, billingPoolMemberEntityToService(m.Edges.Members[i]))
		}
	}
	return out
}

func billingPoolMemberEntityToService(m *dbent.BillingPoolGroup) service.BillingPoolMember {
	out := service.BillingPoolMember{
		ID:            m.ID,
		BillingPoolID: m.BillingPoolID,
		GroupID:       m.GroupID,
		ChainOrder:    m.ChainOrder,
		CanBePrimary:  m.CanBePrimary,
		CanBeFallback: m.CanBeFallback,
		CreatedAt:     m.CreatedAt,
		UpdatedAt:     m.UpdatedAt,
	}
	if m.Edges.Group != nil {
		out.Group = groupEntityToService(m.Edges.Group)
	}
	return out
}
