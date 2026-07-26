package repository

import (
	"context"
	"database/sql"
	"testing"
	"time"

	dbent "github.com/Wei-Shaw/sub2api/ent"
	"github.com/Wei-Shaw/sub2api/ent/enttest"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/stretchr/testify/require"

	"entgo.io/ent/dialect"
	entsql "entgo.io/ent/dialect/sql"
	_ "modernc.org/sqlite"
)

func newCustomDomainRepo(t *testing.T) (*customDomainRepository, *dbent.Client) {
	t.Helper()
	db, err := sql.Open("sqlite", "file:custom_domain?mode=memory&cache=shared")
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })
	_, err = db.Exec("PRAGMA foreign_keys = ON")
	require.NoError(t, err)

	drv := entsql.OpenDB(dialect.SQLite, db)
	client := enttest.NewClient(t, enttest.WithOptions(dbent.Driver(drv)))
	t.Cleanup(func() { _ = client.Close() })
	repo, ok := NewCustomDomainRepository(client).(*customDomainRepository)
	require.True(t, ok)
	return repo, client
}

func createCustomDomainTestUser(t *testing.T, client *dbent.Client, email string) int64 {
	t.Helper()
	row, err := client.User.Create().
		SetEmail(email).
		SetPasswordHash("test-password-hash").
		SetStatus(service.StatusActive).
		SetRole(service.RoleUser).
		Save(context.Background())
	require.NoError(t, err)
	return row.ID
}

func TestCustomDomainRepositoryCoreContract(t *testing.T) {
	repo, client := newCustomDomainRepo(t)
	ctx := context.Background()
	ownerID := createCustomDomainTestUser(t, client, "domain-owner@example.com")
	allowedID := createCustomDomainTestUser(t, client, "domain-allowed@example.com")
	deniedID := createCustomDomainTestUser(t, client, "domain-denied@example.com")

	domain := &service.CustomDomain{
		UserID:               ownerID,
		Domain:               "api.example.com",
		Status:               service.CustomDomainStatusPendingDNS,
		VerificationToken:    "verification-token",
		VerificationTXTName:  "_sub2api.api.example.com",
		VerificationTXTValue: "sub2api-domain-verification=verification-token",
		AuthorizedUserIDs:    []int64{allowedID},
	}
	domain, err := repo.Create(ctx, domain)
	require.NoError(t, err)
	require.NotZero(t, domain.ID)

	byName, err := repo.GetByDomain(ctx, "API.EXAMPLE.COM")
	require.NoError(t, err)
	require.Equal(t, domain.ID, byName.ID)
	require.Equal(t, []int64{allowedID}, byName.AuthorizedUserIDs)
	require.NotNil(t, byName.User)
	require.Equal(t, "domain-owner@example.com", byName.User.Email)
	require.Len(t, byName.AuthorizedUsers, 1)
	require.Equal(t, allowedID, byName.AuthorizedUsers[0].ID)
	require.Equal(t, "domain-allowed@example.com", byName.AuthorizedUsers[0].Email)
	require.Nil(t, byName.DeletedAt)

	for _, tc := range []struct {
		userID int64
		want   bool
	}{{ownerID, true}, {allowedID, true}, {deniedID, false}} {
		rows, err := repo.ListByUserID(ctx, tc.userID)
		require.NoError(t, err)
		require.Equal(t, tc.want, len(rows) == 1)
	}

	_, err = repo.SetAccess(ctx, domain.ID, true, nil)
	require.NoError(t, err)
	rows, err := repo.ListByUserID(ctx, deniedID)
	require.NoError(t, err)
	require.Len(t, rows, 1)

	require.NoError(t, repo.Delete(ctx, domain.ID))
	_, err = repo.GetByID(ctx, domain.ID)
	require.ErrorIs(t, err, service.ErrCustomDomainNotFound)
}

func TestCustomDomainRepositorySetAccessRollsBackAtomically(t *testing.T) {
	repo, client := newCustomDomainRepo(t)
	ctx := context.Background()
	ownerID := createCustomDomainTestUser(t, client, "atomic-owner@example.com")
	allowedID := createCustomDomainTestUser(t, client, "atomic-allowed@example.com")
	domain := &service.CustomDomain{
		UserID:               ownerID,
		Domain:               "atomic.example.com",
		Status:               service.CustomDomainStatusActive,
		VerificationToken:    "atomic-token",
		VerificationTXTName:  "_sub2api.atomic.example.com",
		VerificationTXTValue: "sub2api-domain-verification=atomic-token",
		AuthorizedUserIDs:    []int64{allowedID},
	}
	domain, err := repo.Create(ctx, domain)
	require.NoError(t, err)

	_, err = repo.SetAccess(ctx, domain.ID, false, []int64{999999})
	require.Error(t, err)

	stored, err := repo.GetByID(ctx, domain.ID)
	require.NoError(t, err)
	require.False(t, stored.AllUsers)
	require.Equal(t, []int64{allowedID}, stored.AuthorizedUserIDs)
}

func TestCustomDomainRepositorySetAccessReturnsReloadedDomain(t *testing.T) {
	repo, client := newCustomDomainRepo(t)
	ctx := context.Background()
	ownerID := createCustomDomainTestUser(t, client, "set-access-owner@example.com")
	allowedID := createCustomDomainTestUser(t, client, "set-access-allowed@example.com")
	domain, err := repo.Create(ctx, &service.CustomDomain{
		UserID:               ownerID,
		Domain:               "set-access.example.com",
		Status:               service.CustomDomainStatusActive,
		VerificationToken:    "set-access-token",
		VerificationTXTName:  "_sub2api.set-access.example.com",
		VerificationTXTValue: "sub2api-domain-verification=set-access-token",
	})
	require.NoError(t, err)

	updated, err := repo.SetAccess(ctx, domain.ID, false, []int64{allowedID})
	require.NoError(t, err)
	require.False(t, updated.AllUsers)
	require.Equal(t, []int64{allowedID}, updated.AuthorizedUserIDs)
	require.Len(t, updated.AuthorizedUsers, 1)
	require.Equal(t, allowedID, updated.AuthorizedUsers[0].ID)
}

func TestCustomDomainRepositoryUpdatePreservesAccessMembership(t *testing.T) {
	repo, client := newCustomDomainRepo(t)
	ctx := context.Background()
	ownerID := createCustomDomainTestUser(t, client, "update-owner@example.com")
	allowedID := createCustomDomainTestUser(t, client, "update-allowed@example.com")
	replacementID := createCustomDomainTestUser(t, client, "update-replacement@example.com")
	domain, err := repo.Create(ctx, &service.CustomDomain{
		UserID:               ownerID,
		Domain:               "update.example.com",
		Status:               service.CustomDomainStatusPendingDNS,
		VerificationToken:    "update-token",
		VerificationTXTName:  "_sub2api.update.example.com",
		VerificationTXTValue: "sub2api-domain-verification=update-token",
		AuthorizedUserIDs:    []int64{allowedID},
	})
	require.NoError(t, err)

	domain.Domain = "replacement.example.com"
	domain.Status = service.CustomDomainStatusActive
	domain.AuthorizedUserIDs = []int64{replacementID}
	updated, err := repo.Update(ctx, domain)
	require.NoError(t, err)
	require.Equal(t, domain.ID, updated.ID)
	require.NotNil(t, updated.User)
	require.Len(t, updated.AuthorizedUsers, 1)
	updated, err = repo.Update(ctx, nil)
	require.NoError(t, err)
	require.Nil(t, updated)

	stored, err := repo.GetByID(ctx, domain.ID)
	require.NoError(t, err)
	require.Equal(t, "update.example.com", stored.Domain)
	require.Equal(t, service.CustomDomainStatusActive, stored.Status)
	require.Equal(t, []int64{allowedID}, stored.AuthorizedUserIDs)
}

func TestCustomDomainRepositoryUpdateClearsNillableLifecycleState(t *testing.T) {
	repo, client := newCustomDomainRepo(t)
	ctx := context.Background()
	ownerID := createCustomDomainTestUser(t, client, "update-clear-owner@example.com")
	now := time.Now().UTC()
	cnameTarget := "origin.example.com"
	lastError := "dns lookup failed"
	disabledReason := "manual review"
	domain, err := repo.Create(ctx, &service.CustomDomain{
		UserID:               ownerID,
		Domain:               "update-clear.example.com",
		Status:               service.CustomDomainStatusDisabled,
		VerificationToken:    "update-clear-token",
		VerificationTXTName:  "_sub2api.update-clear.example.com",
		VerificationTXTValue: "sub2api-domain-verification=update-clear-token",
	})
	require.NoError(t, err)

	_, err = client.CustomDomain.UpdateOneID(domain.ID).
		SetCnameTarget(cnameTarget).
		SetVerifiedAt(now).
		SetLastCheckedAt(now).
		SetLastError(lastError).
		SetDisabledAt(now).
		SetDisabledReason(disabledReason).
		Save(ctx)
	require.NoError(t, err)

	domain, err = repo.GetByID(ctx, domain.ID)
	require.NoError(t, err)
	domain.CNAMETarget = nil
	domain.VerifiedAt = nil
	domain.LastCheckedAt = nil
	domain.LastError = nil
	domain.DisabledAt = nil
	domain.DisabledReason = nil
	updated, err := repo.Update(ctx, domain)
	require.NoError(t, err)
	require.Equal(t, domain.ID, updated.ID)

	stored, err := repo.GetByID(ctx, domain.ID)
	require.NoError(t, err)
	require.Nil(t, stored.CNAMETarget)
	require.Nil(t, stored.VerifiedAt)
	require.Nil(t, stored.LastCheckedAt)
	require.Nil(t, stored.LastError)
	require.Nil(t, stored.DisabledAt)
	require.Nil(t, stored.DisabledReason)
}

func TestCustomDomainRepositoryCreateRollsBackAtomically(t *testing.T) {
	repo, client := newCustomDomainRepo(t)
	ctx := context.Background()
	ownerID := createCustomDomainTestUser(t, client, "create-atomic-owner@example.com")
	domain := &service.CustomDomain{
		UserID:               ownerID,
		Domain:               "create-atomic.example.com",
		Status:               service.CustomDomainStatusPendingDNS,
		VerificationToken:    "create-atomic-token",
		VerificationTXTName:  "_sub2api.create-atomic.example.com",
		VerificationTXTValue: "sub2api-domain-verification=create-atomic-token",
		AuthorizedUserIDs:    []int64{999999},
	}

	_, err := repo.Create(ctx, domain)
	require.Error(t, err)

	_, err = repo.GetByDomain(ctx, domain.Domain)
	require.ErrorIs(t, err, service.ErrCustomDomainNotFound)
}

func TestCustomDomainRepositoryCreateIgnoresLifecycleState(t *testing.T) {
	repo, client := newCustomDomainRepo(t)
	ctx := context.Background()
	ownerID := createCustomDomainTestUser(t, client, "create-lifecycle-owner@example.com")
	now := time.Now().UTC()
	lastError := "stale error"
	disabledReason := "stale reason"
	domain := &service.CustomDomain{
		UserID:               ownerID,
		Domain:               "create-lifecycle.example.com",
		Status:               service.CustomDomainStatusActive,
		VerificationToken:    "create-lifecycle-token",
		VerificationTXTName:  "_sub2api.create-lifecycle.example.com",
		VerificationTXTValue: "sub2api-domain-verification=create-lifecycle-token",
		VerifiedAt:           &now,
		LastCheckedAt:        &now,
		LastError:            &lastError,
		DisabledAt:           &now,
		DisabledReason:       &disabledReason,
	}

	created, err := repo.Create(ctx, domain)
	require.NoError(t, err)
	require.Nil(t, created.VerifiedAt)
	require.Nil(t, created.LastCheckedAt)
	require.Nil(t, created.LastError)
	require.Nil(t, created.DisabledAt)
	require.Nil(t, created.DisabledReason)
}

func TestCustomDomainRepositoryListAllIgnoresAmbientTransaction(t *testing.T) {
	repo, client := newCustomDomainRepo(t)
	ctx := context.Background()
	ownerID := createCustomDomainTestUser(t, client, "list-all-owner@example.com")
	_, err := repo.Create(ctx, &service.CustomDomain{
		UserID:               ownerID,
		Domain:               "list-all.example.com",
		Status:               service.CustomDomainStatusActive,
		VerificationToken:    "list-all-token",
		VerificationTXTName:  "_sub2api.list-all.example.com",
		VerificationTXTValue: "sub2api-domain-verification=list-all-token",
	})
	require.NoError(t, err)

	tx, err := client.Tx(ctx)
	require.NoError(t, err)
	require.NoError(t, tx.Rollback())
	txCtx := dbent.NewTxContext(ctx, tx)

	rows, err := repo.ListAll(txCtx, service.CustomDomainListFilters{UserID: ownerID})
	require.NoError(t, err)
	require.Len(t, rows, 1)
}
