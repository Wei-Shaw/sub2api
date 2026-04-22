//go:build unit

package service

import (
	"context"
	"database/sql"
	"testing"

	dbent "github.com/Wei-Shaw/sub2api/ent"
	"github.com/Wei-Shaw/sub2api/ent/authidentity"
	"github.com/Wei-Shaw/sub2api/ent/authidentitychannel"
	"github.com/Wei-Shaw/sub2api/ent/enttest"
	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
	"github.com/stretchr/testify/require"

	"entgo.io/ent/dialect"
	entsql "entgo.io/ent/dialect/sql"
	_ "modernc.org/sqlite"
)

func newAdminServiceAuthIdentityBindingTestClient(t *testing.T) *dbent.Client {
	t.Helper()

	db, err := sql.Open("sqlite", "file:admin_service_auth_identity_binding?mode=memory&cache=shared&_fk=1")
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })

	_, err = db.Exec("PRAGMA foreign_keys = ON")
	require.NoError(t, err)

	drv := entsql.OpenDB(dialect.SQLite, db)
	client := enttest.NewClient(t, enttest.WithOptions(dbent.Driver(drv)))
	t.Cleanup(func() { _ = client.Close() })
	return client
}

func TestAdminServiceBindUserAuthIdentityCreatesCanonicalAndChannelBinding(t *testing.T) {
	client := newAdminServiceAuthIdentityBindingTestClient(t)
	ctx := context.Background()

	user, err := client.User.Create().
		SetEmail("bind-target@example.com").
		SetPasswordHash("hash").
		SetRole(RoleUser).
		SetStatus(StatusActive).
		Save(ctx)
	require.NoError(t, err)

	svc := &adminServiceImpl{
		userRepo:  &userRepoStub{user: &User{ID: user.ID, Email: user.Email, Status: StatusActive}},
		entClient: client,
	}

	result, err := svc.BindUserAuthIdentity(ctx, user.ID, AdminBindAuthIdentityInput{
		ProviderType:    "wechat",
		ProviderKey:     "wechat-main",
		ProviderSubject: "union-123",
		Metadata:        map[string]any{"source": "admin-repair"},
		Channel: &AdminBindAuthIdentityChannelInput{
			Channel:        "open",
			ChannelAppID:   "wx-open",
			ChannelSubject: "openid-123",
			Metadata:       map[string]any{"scene": "migration"},
		},
	})
	require.NoError(t, err)
	require.NotNil(t, result)
	require.Equal(t, user.ID, result.UserID)
	require.Equal(t, "wechat", result.ProviderType)
	require.Equal(t, "wechat-main", result.ProviderKey)
	require.NotNil(t, result.VerifiedAt)
	require.NotNil(t, result.Channel)
	require.Equal(t, "open", result.Channel.Channel)

	identity, err := client.AuthIdentity.Query().
		Where(
			authidentity.ProviderTypeEQ("wechat"),
			authidentity.ProviderKeyEQ("wechat-main"),
			authidentity.ProviderSubjectEQ("union-123"),
		).
		Only(ctx)
	require.NoError(t, err)
	require.Equal(t, user.ID, identity.UserID)
	require.Equal(t, "admin-repair", identity.Metadata["source"])
	require.NotNil(t, identity.VerifiedAt)

	channel, err := client.AuthIdentityChannel.Query().
		Where(
			authidentitychannel.ProviderTypeEQ("wechat"),
			authidentitychannel.ProviderKeyEQ("wechat-main"),
			authidentitychannel.ChannelEQ("open"),
			authidentitychannel.ChannelAppIDEQ("wx-open"),
			authidentitychannel.ChannelSubjectEQ("openid-123"),
		).
		Only(ctx)
	require.NoError(t, err)
	require.Equal(t, identity.ID, channel.IdentityID)
	require.Equal(t, "migration", channel.Metadata["scene"])
}

func TestAdminServiceBindUserAuthIdentityRejectsOtherOwner(t *testing.T) {
	client := newAdminServiceAuthIdentityBindingTestClient(t)
	ctx := context.Background()

	owner, err := client.User.Create().
		SetEmail("owner@example.com").
		SetPasswordHash("hash").
		SetRole(RoleUser).
		SetStatus(StatusActive).
		Save(ctx)
	require.NoError(t, err)

	target, err := client.User.Create().
		SetEmail("target@example.com").
		SetPasswordHash("hash").
		SetRole(RoleUser).
		SetStatus(StatusActive).
		Save(ctx)
	require.NoError(t, err)

	_, err = client.AuthIdentity.Create().
		SetUserID(owner.ID).
		SetProviderType("oidc").
		SetProviderKey("https://issuer.example").
		SetProviderSubject("subject-1").
		Save(ctx)
	require.NoError(t, err)

	svc := &adminServiceImpl{
		userRepo:  &userRepoStub{user: &User{ID: target.ID, Email: target.Email, Status: StatusActive}},
		entClient: client,
	}

	_, err = svc.BindUserAuthIdentity(ctx, target.ID, AdminBindAuthIdentityInput{
		ProviderType:    "oidc",
		ProviderKey:     "https://issuer.example",
		ProviderSubject: "subject-1",
	})
	require.Error(t, err)
	require.Equal(t, "AUTH_IDENTITY_OWNERSHIP_CONFLICT", infraerrors.Reason(err))
}

func TestAdminServiceBindUserAuthIdentityIsIdempotentForSameUser(t *testing.T) {
	client := newAdminServiceAuthIdentityBindingTestClient(t)
	ctx := context.Background()

	user, err := client.User.Create().
		SetEmail("same-user@example.com").
		SetPasswordHash("hash").
		SetRole(RoleUser).
		SetStatus(StatusActive).
		Save(ctx)
	require.NoError(t, err)

	svc := &adminServiceImpl{
		userRepo:  &userRepoStub{user: &User{ID: user.ID, Email: user.Email, Status: StatusActive}},
		entClient: client,
	}

	first, err := svc.BindUserAuthIdentity(ctx, user.ID, AdminBindAuthIdentityInput{
		ProviderType:    "oidc",
		ProviderKey:     "https://issuer.example",
		ProviderSubject: "subject-2",
		Metadata:        map[string]any{"source": "first"},
	})
	require.NoError(t, err)

	second, err := svc.BindUserAuthIdentity(ctx, user.ID, AdminBindAuthIdentityInput{
		ProviderType:    "oidc",
		ProviderKey:     "https://issuer.example",
		ProviderSubject: "subject-2",
		Metadata:        map[string]any{"source": "second"},
	})
	require.NoError(t, err)
	require.Equal(t, first.UserID, second.UserID)
	require.Equal(t, "second", second.Metadata["source"])

	identities, err := client.AuthIdentity.Query().
		Where(
			authidentity.ProviderTypeEQ("oidc"),
			authidentity.ProviderKeyEQ("https://issuer.example"),
			authidentity.ProviderSubjectEQ("subject-2"),
		).
		All(ctx)
	require.NoError(t, err)
	require.Len(t, identities, 1)
	require.Equal(t, "second", identities[0].Metadata["source"])
}

func TestAdminServiceBindUserAuthIdentityReusesLegacyWeChatAliasRecords(t *testing.T) {
	client := newAdminServiceAuthIdentityBindingTestClient(t)
	ctx := context.Background()

	user, err := client.User.Create().
		SetEmail("wechat-alias@example.com").
		SetPasswordHash("hash").
		SetRole(RoleUser).
		SetStatus(StatusActive).
		Save(ctx)
	require.NoError(t, err)

	legacyIdentity, err := client.AuthIdentity.Create().
		SetUserID(user.ID).
		SetProviderType("wechat").
		SetProviderKey("wechat").
		SetProviderSubject("union-legacy-123").
		SetMetadata(map[string]any{"source": "legacy"}).
		Save(ctx)
	require.NoError(t, err)

	legacyChannel, err := client.AuthIdentityChannel.Create().
		SetIdentityID(legacyIdentity.ID).
		SetProviderType("wechat").
		SetProviderKey("wechat").
		SetChannel("open").
		SetChannelAppID("wx-open").
		SetChannelSubject("openid-legacy-123").
		SetMetadata(map[string]any{"scene": "legacy"}).
		Save(ctx)
	require.NoError(t, err)

	svc := &adminServiceImpl{
		userRepo:  &userRepoStub{user: &User{ID: user.ID, Email: user.Email, Status: StatusActive}},
		entClient: client,
	}

	result, err := svc.BindUserAuthIdentity(ctx, user.ID, AdminBindAuthIdentityInput{
		ProviderType:    "wechat",
		ProviderKey:     "wechat-main",
		ProviderSubject: "union-legacy-123",
		Metadata:        map[string]any{"source": "admin-repair"},
		Channel: &AdminBindAuthIdentityChannelInput{
			Channel:        "open",
			ChannelAppID:   "wx-open",
			ChannelSubject: "openid-legacy-123",
			Metadata:       map[string]any{"scene": "admin-repair"},
		},
	})
	require.NoError(t, err)
	require.NotNil(t, result)
	require.Equal(t, "wechat-main", result.ProviderKey)
	require.NotNil(t, result.Channel)
	require.Equal(t, "open", result.Channel.Channel)

	identity, err := client.AuthIdentity.Get(ctx, legacyIdentity.ID)
	require.NoError(t, err)
	require.Equal(t, "wechat-main", identity.ProviderKey)
	require.Equal(t, "admin-repair", identity.Metadata["source"])

	channel, err := client.AuthIdentityChannel.Get(ctx, legacyChannel.ID)
	require.NoError(t, err)
	require.Equal(t, "wechat-main", channel.ProviderKey)
	require.Equal(t, legacyIdentity.ID, channel.IdentityID)
	require.Equal(t, "admin-repair", channel.Metadata["scene"])

	identityCount, err := client.AuthIdentity.Query().
		Where(
			authidentity.ProviderTypeEQ("wechat"),
			authidentity.ProviderSubjectEQ("union-legacy-123"),
		).
		Count(ctx)
	require.NoError(t, err)
	require.Equal(t, 1, identityCount)

	channelCount, err := client.AuthIdentityChannel.Query().
		Where(
			authidentitychannel.ProviderTypeEQ("wechat"),
			authidentitychannel.ChannelEQ("open"),
			authidentitychannel.ChannelAppIDEQ("wx-open"),
			authidentitychannel.ChannelSubjectEQ("openid-legacy-123"),
		).
		Count(ctx)
	require.NoError(t, err)
	require.Equal(t, 1, channelCount)
}

func TestAdminServiceBindUserAuthIdentityRejectsInvalidProviderType(t *testing.T) {
	client := newAdminServiceAuthIdentityBindingTestClient(t)
	ctx := context.Background()

	user, err := client.User.Create().
		SetEmail("invalid-provider@example.com").
		SetPasswordHash("hash").
		SetRole(RoleUser).
		SetStatus(StatusActive).
		Save(ctx)
	require.NoError(t, err)

	svc := &adminServiceImpl{
		userRepo:  &userRepoStub{user: &User{ID: user.ID, Email: user.Email, Status: StatusActive}},
		entClient: client,
	}

	_, err = svc.BindUserAuthIdentity(ctx, user.ID, AdminBindAuthIdentityInput{
		ProviderType:    "github",
		ProviderKey:     "github-main",
		ProviderSubject: "subject-3",
	})
	require.Error(t, err)
	require.Equal(t, "INVALID_INPUT", infraerrors.Reason(err))
}
