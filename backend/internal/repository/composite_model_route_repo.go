package repository

import (
	"context"

	dbent "github.com/Wei-Shaw/sub2api/ent"
	"github.com/Wei-Shaw/sub2api/ent/compositemodelroute"
	"github.com/Wei-Shaw/sub2api/ent/compositeroutescheme"
	"github.com/Wei-Shaw/sub2api/ent/group"
	"github.com/Wei-Shaw/sub2api/internal/service"
)

type compositeModelRouteRepository struct {
	client *dbent.Client
}

func NewCompositeModelRouteRepository(client *dbent.Client) service.CompositeModelRouteRepository {
	return &compositeModelRouteRepository{client: client}
}

func (r *compositeModelRouteRepository) ListByGroup(ctx context.Context, groupID int64, includeDisabled bool) ([]service.CompositeModelRoute, error) {
	row, err := clientFromContext(ctx, r.client).Group.Query().
		Where(group.IDEQ(groupID)).
		Only(ctx)
	if err != nil {
		return nil, translatePersistenceError(err, service.ErrGroupNotFound, nil)
	}
	if row.CompositeRouteSchemeID == nil || *row.CompositeRouteSchemeID <= 0 {
		return []service.CompositeModelRoute{}, nil
	}
	routes, err := r.ListByScheme(ctx, *row.CompositeRouteSchemeID, includeDisabled)
	if err != nil {
		return nil, err
	}
	for i := range routes {
		routes[i].GroupID = groupID
	}
	return routes, nil
}

func (r *compositeModelRouteRepository) ListByScheme(ctx context.Context, schemeID int64, includeDisabled bool) ([]service.CompositeModelRoute, error) {
	q := clientFromContext(ctx, r.client).CompositeModelRoute.Query().
		Where(compositemodelroute.SchemeIDEQ(schemeID)).
		Order(
			dbent.Asc(compositemodelroute.FieldPriority),
			dbent.Asc(compositemodelroute.FieldID),
		)
	if !includeDisabled {
		q = q.Where(compositemodelroute.EnabledEQ(true))
	}
	rows, err := q.All(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]service.CompositeModelRoute, 0, len(rows))
	for _, row := range rows {
		out = append(out, *compositeModelRouteEntityToService(row))
	}
	return out, nil
}

func (r *compositeModelRouteRepository) GetByID(ctx context.Context, id int64) (*service.CompositeModelRoute, error) {
	row, err := clientFromContext(ctx, r.client).CompositeModelRoute.Get(ctx, id)
	if err != nil {
		return nil, translatePersistenceError(err, service.ErrCompositeRouteNotFound, nil)
	}
	return compositeModelRouteEntityToService(row), nil
}

func (r *compositeModelRouteRepository) Create(ctx context.Context, route *service.CompositeModelRoute) error {
	if route == nil {
		return service.ErrCompositeRouteNotFound
	}
	created, err := clientFromContext(ctx, r.client).CompositeModelRoute.Create().
		SetSchemeID(route.SchemeID).
		SetPublicModel(route.PublicModel).
		SetMatchType(route.MatchType).
		SetTargetPlatform(route.TargetPlatform).
		SetUpstreamModel(route.UpstreamModel).
		SetEndpoint(route.Endpoint).
		SetPriority(route.Priority).
		SetEnabled(route.Enabled).
		SetNotes(route.Notes).
		Save(ctx)
	if err != nil {
		return translatePersistenceError(err, nil, service.ErrCompositeRouteExists)
	}
	mapped := compositeModelRouteEntityToService(created)
	mapped.GroupID = route.GroupID
	*route = *mapped
	return nil
}

func (r *compositeModelRouteRepository) Update(ctx context.Context, route *service.CompositeModelRoute) error {
	if route == nil {
		return service.ErrCompositeRouteNotFound
	}
	updated, err := clientFromContext(ctx, r.client).CompositeModelRoute.UpdateOneID(route.ID).
		SetPublicModel(route.PublicModel).
		SetMatchType(route.MatchType).
		SetTargetPlatform(route.TargetPlatform).
		SetUpstreamModel(route.UpstreamModel).
		SetEndpoint(route.Endpoint).
		SetPriority(route.Priority).
		SetEnabled(route.Enabled).
		SetNotes(route.Notes).
		Save(ctx)
	if err != nil {
		return translatePersistenceError(err, service.ErrCompositeRouteNotFound, service.ErrCompositeRouteExists)
	}
	mapped := compositeModelRouteEntityToService(updated)
	mapped.GroupID = route.GroupID
	*route = *mapped
	return nil
}

func (r *compositeModelRouteRepository) Delete(ctx context.Context, id int64) error {
	err := clientFromContext(ctx, r.client).CompositeModelRoute.DeleteOneID(id).Exec(ctx)
	return translatePersistenceError(err, service.ErrCompositeRouteNotFound, nil)
}

func (r *compositeModelRouteRepository) DeleteByScheme(ctx context.Context, schemeID int64) error {
	_, err := clientFromContext(ctx, r.client).CompositeModelRoute.Delete().
		Where(compositemodelroute.SchemeIDEQ(schemeID)).
		Exec(ctx)
	return err
}

func (r *compositeModelRouteRepository) ListSchemes(ctx context.Context) ([]service.CompositeRouteScheme, error) {
	rows, err := clientFromContext(ctx, r.client).CompositeRouteScheme.Query().
		Order(
			dbent.Asc(compositeroutescheme.FieldName),
			dbent.Asc(compositeroutescheme.FieldID),
		).
		All(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]service.CompositeRouteScheme, 0, len(rows))
	for _, row := range rows {
		scheme := compositeRouteSchemeEntityToService(row)
		routeCount, err := r.countRoutesByScheme(ctx, row.ID)
		if err != nil {
			return nil, err
		}
		groupCount, err := r.CountGroupsByScheme(ctx, row.ID)
		if err != nil {
			return nil, err
		}
		scheme.RouteCount = routeCount
		scheme.GroupCount = groupCount
		out = append(out, *scheme)
	}
	return out, nil
}

func (r *compositeModelRouteRepository) GetScheme(ctx context.Context, id int64) (*service.CompositeRouteScheme, error) {
	row, err := clientFromContext(ctx, r.client).CompositeRouteScheme.Get(ctx, id)
	if err != nil {
		return nil, translatePersistenceError(err, service.ErrCompositeRouteSchemeNotFound, nil)
	}
	scheme := compositeRouteSchemeEntityToService(row)
	routeCount, err := r.countRoutesByScheme(ctx, id)
	if err != nil {
		return nil, err
	}
	groupCount, err := r.CountGroupsByScheme(ctx, id)
	if err != nil {
		return nil, err
	}
	scheme.RouteCount = routeCount
	scheme.GroupCount = groupCount
	return scheme, nil
}

func (r *compositeModelRouteRepository) CreateScheme(ctx context.Context, scheme *service.CompositeRouteScheme) error {
	if scheme == nil {
		return service.ErrCompositeRouteSchemeNotFound
	}
	builder := clientFromContext(ctx, r.client).CompositeRouteScheme.Create().
		SetName(scheme.Name)
	if scheme.Description != "" {
		builder = builder.SetDescription(scheme.Description)
	}
	created, err := builder.Save(ctx)
	if err != nil {
		return translatePersistenceError(err, nil, service.ErrCompositeRouteSchemeExists)
	}
	*scheme = *compositeRouteSchemeEntityToService(created)
	return nil
}

func (r *compositeModelRouteRepository) UpdateScheme(ctx context.Context, scheme *service.CompositeRouteScheme) error {
	if scheme == nil {
		return service.ErrCompositeRouteSchemeNotFound
	}
	builder := clientFromContext(ctx, r.client).CompositeRouteScheme.UpdateOneID(scheme.ID).
		SetName(scheme.Name)
	if scheme.Description != "" {
		builder = builder.SetDescription(scheme.Description)
	} else {
		builder = builder.ClearDescription()
	}
	updated, err := builder.Save(ctx)
	if err != nil {
		return translatePersistenceError(err, service.ErrCompositeRouteSchemeNotFound, service.ErrCompositeRouteSchemeExists)
	}
	mapped := compositeRouteSchemeEntityToService(updated)
	mapped.RouteCount = scheme.RouteCount
	mapped.GroupCount = scheme.GroupCount
	*scheme = *mapped
	return nil
}

func (r *compositeModelRouteRepository) DeleteScheme(ctx context.Context, id int64) error {
	err := clientFromContext(ctx, r.client).CompositeRouteScheme.DeleteOneID(id).Exec(ctx)
	return translatePersistenceError(err, service.ErrCompositeRouteSchemeNotFound, nil)
}

func (r *compositeModelRouteRepository) CountGroupsByScheme(ctx context.Context, schemeID int64) (int, error) {
	count, err := clientFromContext(ctx, r.client).Group.Query().
		Where(group.CompositeRouteSchemeIDEQ(schemeID)).
		Count(ctx)
	return count, err
}

func (r *compositeModelRouteRepository) countRoutesByScheme(ctx context.Context, schemeID int64) (int, error) {
	return clientFromContext(ctx, r.client).CompositeModelRoute.Query().
		Where(compositemodelroute.SchemeIDEQ(schemeID)).
		Count(ctx)
}

func compositeModelRouteEntityToService(row *dbent.CompositeModelRoute) *service.CompositeModelRoute {
	if row == nil {
		return nil
	}
	return &service.CompositeModelRoute{
		ID:             row.ID,
		SchemeID:       row.SchemeID,
		PublicModel:    row.PublicModel,
		MatchType:      row.MatchType,
		TargetPlatform: row.TargetPlatform,
		UpstreamModel:  row.UpstreamModel,
		Endpoint:       row.Endpoint,
		Priority:       row.Priority,
		Enabled:        row.Enabled,
		Notes:          derefString(row.Notes),
		CreatedAt:      row.CreatedAt,
		UpdatedAt:      row.UpdatedAt,
	}
}

func compositeRouteSchemeEntityToService(row *dbent.CompositeRouteScheme) *service.CompositeRouteScheme {
	if row == nil {
		return nil
	}
	return &service.CompositeRouteScheme{
		ID:          row.ID,
		Name:        row.Name,
		Description: derefString(row.Description),
		CreatedAt:   row.CreatedAt,
		UpdatedAt:   row.UpdatedAt,
	}
}
