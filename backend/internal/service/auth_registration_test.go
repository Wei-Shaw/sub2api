//go:build unit

package service

import (
	"context"
	"database/sql"
	"fmt"
	"testing"
	"time"

	dbent "github.com/Wei-Shaw/sub2api/ent"
	"github.com/Wei-Shaw/sub2api/ent/enttest"
	dbuser "github.com/Wei-Shaw/sub2api/ent/user"
	"github.com/stretchr/testify/require"

	"entgo.io/ent/dialect"
	entsql "entgo.io/ent/dialect/sql"
	_ "modernc.org/sqlite"
)

func newAuthRegistrationTestClient(t *testing.T) *dbent.Client {
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

func TestAuthServiceEnsureRegistrationIPAllowedLimitsActiveUsers(t *testing.T) {
	client := newAuthRegistrationTestClient(t)
	svc := &AuthService{}
	ctx := context.Background()
	registrationIP := "203.0.113.8"

	for i := 0; i < maxRegistrationsPerIP; i++ {
		_, err := client.User.Create().
			SetEmail(fmt.Sprintf("ip-limit-%d@example.com", i)).
			SetPasswordHash("hash").
			SetRole(RoleUser).
			SetStatus(StatusActive).
			SetRegistrationIP(registrationIP).
			Save(ctx)
		require.NoError(t, err)
	}

	err := svc.ensureRegistrationIPAllowed(ctx, client, registrationIP)
	require.ErrorIs(t, err, ErrRegistrationIPLimitExceeded)
}

func TestAuthServiceEnsureRegistrationIPAllowedIgnoresSoftDeletedUsers(t *testing.T) {
	client := newAuthRegistrationTestClient(t)
	svc := &AuthService{}
	ctx := context.Background()
	registrationIP := "203.0.113.9"

	deletedAt := time.Now().UTC()
	_, err := client.User.Create().
		SetEmail("ip-limit-deleted@example.com").
		SetPasswordHash("hash").
		SetRole(RoleUser).
		SetStatus(StatusActive).
		SetRegistrationIP(registrationIP).
		SetDeletedAt(deletedAt).
		Save(ctx)
	require.NoError(t, err)

	for i := 0; i < maxRegistrationsPerIP-1; i++ {
		_, err := client.User.Create().
			SetEmail(fmt.Sprintf("ip-limit-active-%d@example.com", i)).
			SetPasswordHash("hash").
			SetRole(RoleUser).
			SetStatus(StatusActive).
			SetRegistrationIP(registrationIP).
			Save(ctx)
		require.NoError(t, err)
	}

	err = svc.ensureRegistrationIPAllowed(ctx, client, registrationIP)
	require.NoError(t, err)

	activeCount, err := client.User.Query().
		Where(dbuser.RegistrationIPEQ(registrationIP), dbuser.DeletedAtIsNil()).
		Count(ctx)
	require.NoError(t, err)
	require.Equal(t, maxRegistrationsPerIP-1, activeCount)
}
