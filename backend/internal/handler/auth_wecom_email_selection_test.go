package handler

import (
	"context"
	"database/sql"
	"testing"

	dbent "github.com/Wei-Shaw/sub2api/ent"
	"github.com/Wei-Shaw/sub2api/ent/enttest"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/stretchr/testify/require"

	"entgo.io/ent/dialect"
	entsql "entgo.io/ent/dialect/sql"
	_ "modernc.org/sqlite"
)

func TestSelectWeComRegistrationEmailPrefersEmailThenBizMail(t *testing.T) {
	client := newWeComOAuthEmailTestClient(t)
	ctx := context.Background()
	fallback := "fallback" + service.WeComConnectSyntheticEmailDomain

	claims := map[string]any{
		"wecom_email":    " User@One.EXAMPLE ",
		"wecom_biz_mail": "user@corp.example",
	}

	selected := selectWeComRegistrationEmail(ctx, client, fallback, claims)

	require.Equal(t, "user@one.example", selected)
	require.Equal(t, "email", claims["wecom_registration_email_source"])
	require.Equal(t, "user@one.example", claims["email"])

	claims = map[string]any{
		"wecom_email":    "Name <user@one.example>",
		"wecom_biz_mail": "User@Corp.EXAMPLE",
	}

	selected = selectWeComRegistrationEmail(ctx, client, fallback, claims)

	require.Equal(t, "user@corp.example", selected)
	require.Equal(t, "biz_mail", claims["wecom_registration_email_source"])
}

func TestSelectWeComRegistrationEmailKeepsOccupiedRealEmail(t *testing.T) {
	client := newWeComOAuthEmailTestClient(t)
	ctx := context.Background()
	fallback := "fallback" + service.WeComConnectSyntheticEmailDomain

	_, err := client.User.Create().
		SetEmail("occupied@example.com").
		SetPasswordHash("hash").
		SetRole(service.RoleUser).
		SetStatus(service.StatusActive).
		Save(ctx)
	require.NoError(t, err)

	claims := map[string]any{
		"wecom_email":    "occupied@example.com",
		"wecom_biz_mail": "also-occupied" + service.WeComConnectSyntheticEmailDomain,
	}

	selected := selectWeComRegistrationEmail(ctx, client, fallback, claims)

	require.Equal(t, "occupied@example.com", selected)
	require.Equal(t, "email", claims["wecom_registration_email_source"])
}

func newWeComOAuthEmailTestClient(t *testing.T) *dbent.Client {
	t.Helper()

	db, err := sql.Open("sqlite", "file:auth_wecom_oauth_email?mode=memory&cache=shared")
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })

	_, err = db.Exec("PRAGMA foreign_keys = ON")
	require.NoError(t, err)

	drv := entsql.OpenDB(dialect.SQLite, db)
	client := enttest.NewClient(t, enttest.WithOptions(dbent.Driver(drv)))
	t.Cleanup(func() { _ = client.Close() })
	return client
}
