package repository

import (
	"context"
	"testing"
	"time"

	"entgo.io/ent/dialect"
	entsql "entgo.io/ent/dialect/sql"
	"github.com/DATA-DOG/go-sqlmock"
	dbent "github.com/Wei-Shaw/sub2api/ent"
	"github.com/Wei-Shaw/sub2api/ent/compositemodelroute"
	_ "github.com/Wei-Shaw/sub2api/ent/runtime"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/stretchr/testify/require"
)

func TestCompositeRouteDestinationScanAndClone(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	client := dbent.NewClient(dbent.Driver(entsql.OpenDB(dialect.Postgres, db)))
	t.Cleanup(func() { _ = client.Close() })
	now := time.Now()
	rows := sqlmock.NewRows(compositemodelroute.Columns).
		AddRow(int64(1), now, now, nil, int64(7), int64(20), nil, "group-alias", "exact", service.PlatformOpenAI, "gpt-5", "any", 100, true, nil).
		AddRow(int64(2), now, now, nil, int64(7), nil, int64(31), "pin-alias", "exact", service.PlatformOpenAI, "gpt-5", "any", 100, true, nil).
		AddRow(int64(3), now, now, nil, int64(7), nil, nil, "legacy-alias", "exact", service.PlatformOpenAI, "gpt-5", "any", 100, true, nil)
	mock.ExpectQuery(`SELECT .*target_group_id.*target_account_id.* FROM "composite_model_routes"`).WithArgs(int64(7)).WillReturnRows(rows)
	routes, err := NewCompositeModelRouteRepository(client).ListByGroup(context.Background(), 7, false)
	require.NoError(t, err)
	require.Len(t, routes, 3)
	require.Equal(t, int64(20), *routes[0].TargetGroupID)
	require.Nil(t, routes[0].TargetAccountID)
	require.Equal(t, int64(31), *routes[1].TargetAccountID)
	require.Nil(t, routes[1].TargetGroupID)
	require.Nil(t, routes[2].TargetGroupID)
	require.Nil(t, routes[2].TargetAccountID)
	id := int64(31)
	entity := &dbent.CompositeModelRoute{TargetAccountID: &id}
	converted := compositeModelRouteEntityToService(entity)
	*converted.TargetAccountID = 99
	require.Equal(t, int64(31), *entity.TargetAccountID, "entity pointer must not escape repository")
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestCompositeRouteDestinationCreate(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	client := dbent.NewClient(dbent.Driver(entsql.OpenDB(dialect.Postgres, db)))
	t.Cleanup(func() { _ = client.Close() })
	pin := int64(31)
	route := &service.CompositeModelRoute{GroupID: 7, PublicModel: "alias", MatchType: "exact", TargetPlatform: service.PlatformOpenAI, TargetAccountID: &pin, UpstreamModel: "gpt-5", Endpoint: "any", Priority: 100, Enabled: true}
	mock.ExpectQuery(`INSERT INTO "composite_model_routes" .*"target_account_id".* RETURNING "id"`).WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow(int64(9)))
	require.NoError(t, NewCompositeModelRouteRepository(client).Create(context.Background(), route))
	require.Equal(t, int64(9), route.ID)
	require.Equal(t, pin, *route.TargetAccountID)
	require.Nil(t, route.TargetGroupID)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestCompositeRouteDestinationUpdateClearsRefs(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	client := dbent.NewClient(dbent.Driver(entsql.OpenDB(dialect.Postgres, db)))
	t.Cleanup(func() { _ = client.Close() })
	now := time.Now()
	route := &service.CompositeModelRoute{ID: 9, GroupID: 7, PublicModel: "alias", MatchType: "exact", TargetPlatform: service.PlatformOpenAI, UpstreamModel: "gpt-5", Endpoint: "any", Priority: 100, Enabled: true}
	mock.ExpectBegin()
	mock.ExpectExec(`UPDATE "composite_model_routes" SET .*"target_group_id" = NULL.*"target_account_id" = NULL`).WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectQuery(`SELECT .* FROM "composite_model_routes"`).WillReturnRows(sqlmock.NewRows(compositemodelroute.Columns).AddRow(int64(9), now, now, nil, int64(7), nil, nil, "alias", "exact", service.PlatformOpenAI, "gpt-5", "any", 100, true, nil))
	mock.ExpectCommit()
	require.NoError(t, NewCompositeModelRouteRepository(client).Update(context.Background(), route))
	require.Nil(t, route.TargetGroupID)
	require.Nil(t, route.TargetAccountID)
	require.NoError(t, mock.ExpectationsWereMet())
}
