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

func TestCustomDomainRepositoryVerificationCannotOverwriteDisable(t *testing.T) {
	ctx := context.Background()
	client := newCustomDomainRepoTestClient(t)
	repo := NewCustomDomainRepository(client)
	owner, err := client.User.Create().SetEmail("disabled@example.com").SetPasswordHash("hash").Save(ctx)
	require.NoError(t, err)
	created, err := repo.Create(ctx, &service.CustomDomain{
		UserID: owner.ID, Domain: "disabled.example.com", Status: service.CustomDomainStatusPendingDNS,
		VerificationToken: "token", VerificationTXTName: "_sub2api-verify.disabled.example.com", VerificationTXTValue: "token",
	})
	require.NoError(t, err)
	stale := *created
	disabled := *created
	disabled.Status = service.CustomDomainStatusDisabled
	reason := "operator hold"
	disabled.DisabledReason = &reason
	_, err = repo.Update(ctx, &disabled)
	require.NoError(t, err)
	stale.Status = service.CustomDomainStatusActive
	_, err = repo.UpdateVerification(ctx, &stale)
	require.ErrorIs(t, err, service.ErrCustomDomainInactive)
	require.ErrorIs(t, repo.DeleteIfNotDisabled(ctx, created.ID), service.ErrCustomDomainInactive)
	stored, err := repo.GetByID(ctx, created.ID)
	require.NoError(t, err)
	require.Equal(t, service.CustomDomainStatusDisabled, stored.Status)
	require.Equal(t, reason, *stored.DisabledReason)
	require.NoError(t, repo.Delete(ctx, created.ID), "administrator may explicitly release a disabled domain")
}

func TestCustomDomainRepositoryVerificationPreservesConcurrentAccessUpdate(t *testing.T) {
	ctx := context.Background()
	client := newCustomDomainRepoTestClient(t)
	repo := NewCustomDomainRepository(client)
	owner, err := client.User.Create().SetEmail("access@example.com").SetPasswordHash("hash").Save(ctx)
	require.NoError(t, err)
	domain, err := repo.Create(ctx, &service.CustomDomain{
		UserID: owner.ID, Domain: "access.example.com", Status: service.CustomDomainStatusPendingDNS,
		VerificationToken: "token", VerificationTXTName: "_sub2api-verify.access.example.com", VerificationTXTValue: "token",
	})
	require.NoError(t, err)
	_, err = repo.SetAccess(ctx, domain.ID, true, nil)
	require.NoError(t, err)
	domain.Status = service.CustomDomainStatusActive
	verified, err := repo.UpdateVerification(ctx, domain)
	require.NoError(t, err)
	require.True(t, verified.AllUsers)
	require.Equal(t, service.CustomDomainStatusActive, verified.Status)
}
