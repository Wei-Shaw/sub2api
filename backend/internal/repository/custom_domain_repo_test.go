package repository

import (
	"context"
	"database/sql"
	"fmt"
	"testing"

	dbent "github.com/Wei-Shaw/sub2api/ent"
	"github.com/Wei-Shaw/sub2api/ent/enttest"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/stretchr/testify/require"

	"entgo.io/ent/dialect"
	entsql "entgo.io/ent/dialect/sql"
	_ "modernc.org/sqlite"
)

func newCustomDomainRepoTestClient(t *testing.T) *dbent.Client {
	t.Helper()

	db, err := sql.Open("sqlite", fmt.Sprintf("file:%s?mode=memory&cache=shared&_fk=1", t.Name()))
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })
	db.SetMaxOpenConns(1)

	_, err = db.Exec("PRAGMA foreign_keys = ON")
	require.NoError(t, err)

	drv := entsql.OpenDB(dialect.SQLite, db)
	client := enttest.NewClient(t, enttest.WithOptions(dbent.Driver(drv)))
	t.Cleanup(func() { _ = client.Close() })
	return client
}

func TestCustomDomainRepositorySetAccessRollsBackOnGrantSyncFailure(t *testing.T) {
	ctx := context.Background()
	client := newCustomDomainRepoTestClient(t)
	repo := NewCustomDomainRepository(client)

	owner, err := client.User.Create().
		SetEmail("owner@example.com").
		SetPasswordHash("hash").
		SetRole(service.RoleUser).
		SetStatus(service.StatusActive).
		Save(ctx)
	require.NoError(t, err)

	domain, err := repo.Create(ctx, &service.CustomDomain{
		UserID:               owner.ID,
		AllUsers:             true,
		Domain:               "api.customer.example",
		Status:               service.CustomDomainStatusPendingDNS,
		VerificationToken:    "token",
		VerificationTXTName:  "_sub2api-verify.api.customer.example",
		VerificationTXTValue: "sub2api-domain-verification=token",
	})
	require.NoError(t, err)
	require.True(t, domain.AllUsers)

	_, err = repo.SetAccess(ctx, domain.ID, false, []int64{owner.ID, 999999})
	require.Error(t, err)

	got, err := repo.GetByID(ctx, domain.ID)
	require.NoError(t, err)
	require.True(t, got.AllUsers, "failed grant sync should not partially persist all_users=false")
	require.Empty(t, got.UserIDs)
}
