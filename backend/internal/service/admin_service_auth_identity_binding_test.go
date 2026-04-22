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
REDACTED

	db, err := sql.Open("sqlite", "file:admin_service_auth_identity_binding?mode=memory&cache=shared&_fk=1")
REDACTED
	t.Cleanup(func() { _ = db.Close() REDACTED)

	_, err = db.Exec("PRAGMA foreign_keys = ON")
REDACTED

	drv := entsql.OpenDB(dialect.SQLite, db)
	client := enttest.NewClient(t, enttest.WithOptions(dbent.Driver(drv)))
	t.Cleanup(func() { _ = client.Close() REDACTED)
	return client
REDACTED

func TestAdminServiceBindUserAuthIdentityCreatesCanonicalAndChannelBinding(t *testing.T) {
	client := newAdminServiceAuthIdentityBindingTestClient(t)
	ctx := context.Background()

	user, err := client.User.Create().
		SetEmail("bind-target@example.com").
		SetPasswordHash("hash").
		SetRole(RoleUser).
		SetStatus(StatusActive).
		Save(ctx)
REDACTED

	svc := &adminServiceImpl{
		userRepo:  &userRepoStub{user: &User{ID: user.ID, Email: user.Email, Status: StatusActiveREDACTEDREDACTED,
		entClient: client,
REDACTED

	result, err := svc.BindUserAuthIdentity(ctx, user.ID, AdminBindAuthIdentityInput{
		ProviderType:    "wechat",
		ProviderKey:     "wechat-main",
		ProviderSubject: "union-123",
		Metadata:        map[string]any{"source": "admin-repair"REDACTED,
		Channel: &AdminBindAuthIdentityChannelInput{
			Channel:        "open",
			ChannelAppID:   "wx-open",
			ChannelSubject: "openid-123",
			Metadata:       map[string]any{"scene": "migration"REDACTED,
	REDACTED,
REDACTED)
REDACTED
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
REDACTED
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
REDACTED
	require.Equal(t, identity.ID, channel.IdentityID)
	require.Equal(t, "migration", channel.Metadata["scene"])
REDACTED

func TestAdminServiceBindUserAuthIdentityRejectsOtherOwner(t *testing.T) {
	client := newAdminServiceAuthIdentityBindingTestClient(t)
	ctx := context.Background()

	owner, err := client.User.Create().
		SetEmail("owner@example.com").
		SetPasswordHash("hash").
		SetRole(RoleUser).
		SetStatus(StatusActive).
		Save(ctx)
REDACTED

	target, err := client.User.Create().
		SetEmail("target@example.com").
		SetPasswordHash("hash").
		SetRole(RoleUser).
		SetStatus(StatusActive).
		Save(ctx)
REDACTED

	_, err = client.AuthIdentity.Create().
		SetUserID(owner.ID).
		SetProviderType("oidc").
		SetProviderKey("https://issuer.example").
		SetProviderSubject("subject-1").
		Save(ctx)
REDACTED

	svc := &adminServiceImpl{
		userRepo:  &userRepoStub{user: &User{ID: target.ID, Email: target.Email, Status: StatusActiveREDACTEDREDACTED,
		entClient: client,
REDACTED

	_, err = svc.BindUserAuthIdentity(ctx, target.ID, AdminBindAuthIdentityInput{
		ProviderType:    "oidc",
		ProviderKey:     "https://issuer.example",
		ProviderSubject: "subject-1",
REDACTED)
REDACTED
	require.Equal(t, "AUTH_IDENTITY_OWNERSHIP_CONFLICT", infraerrors.Reason(err))
REDACTED

func TestAdminServiceBindUserAuthIdentityIsIdempotentForSameUser(t *testing.T) {
	client := newAdminServiceAuthIdentityBindingTestClient(t)
	ctx := context.Background()

	user, err := client.User.Create().
		SetEmail("same-user@example.com").
		SetPasswordHash("hash").
		SetRole(RoleUser).
		SetStatus(StatusActive).
		Save(ctx)
REDACTED

	svc := &adminServiceImpl{
		userRepo:  &userRepoStub{user: &User{ID: user.ID, Email: user.Email, Status: StatusActiveREDACTEDREDACTED,
		entClient: client,
REDACTED

	first, err := svc.BindUserAuthIdentity(ctx, user.ID, AdminBindAuthIdentityInput{
		ProviderType:    "oidc",
		ProviderKey:     "https://issuer.example",
		ProviderSubject: "subject-2",
		Metadata:        map[string]any{"source": "first"REDACTED,
REDACTED)
REDACTED

	second, err := svc.BindUserAuthIdentity(ctx, user.ID, AdminBindAuthIdentityInput{
		ProviderType:    "oidc",
		ProviderKey:     "https://issuer.example",
		ProviderSubject: "subject-2",
		Metadata:        map[string]any{"source": "second"REDACTED,
REDACTED)
REDACTED
	require.Equal(t, first.UserID, second.UserID)
	require.Equal(t, "second", second.Metadata["source"])

	identities, err := client.AuthIdentity.Query().
		Where(
			authidentity.ProviderTypeEQ("oidc"),
			authidentity.ProviderKeyEQ("https://issuer.example"),
			authidentity.ProviderSubjectEQ("subject-2"),
		).
		All(ctx)
REDACTED
	require.Len(t, identities, 1)
	require.Equal(t, "second", identities[0].Metadata["source"])
REDACTED

func TestAdminServiceBindUserAuthIdentityReusesLegacyWeChatAliasRecords(t *testing.T) {
	client := newAdminServiceAuthIdentityBindingTestClient(t)
	ctx := context.Background()

	user, err := client.User.Create().
		SetEmail("wechat-alias@example.com").
		SetPasswordHash("hash").
		SetRole(RoleUser).
		SetStatus(StatusActive).
		Save(ctx)
REDACTED

	legacyIdentity, err := client.AuthIdentity.Create().
		SetUserID(user.ID).
		SetProviderType("wechat").
		SetProviderKey("wechat").
		SetProviderSubject("union-legacy-123").
		SetMetadata(map[string]any{"source": "legacy"REDACTED).
		Save(ctx)
REDACTED

	legacyChannel, err := client.AuthIdentityChannel.Create().
		SetIdentityID(legacyIdentity.ID).
		SetProviderType("wechat").
		SetProviderKey("wechat").
		SetChannel("open").
		SetChannelAppID("wx-open").
		SetChannelSubject("openid-legacy-123").
		SetMetadata(map[string]any{"scene": "legacy"REDACTED).
		Save(ctx)
REDACTED

	svc := &adminServiceImpl{
		userRepo:  &userRepoStub{user: &User{ID: user.ID, Email: user.Email, Status: StatusActiveREDACTEDREDACTED,
		entClient: client,
REDACTED

	result, err := svc.BindUserAuthIdentity(ctx, user.ID, AdminBindAuthIdentityInput{
		ProviderType:    "wechat",
		ProviderKey:     "wechat-main",
		ProviderSubject: "union-legacy-123",
		Metadata:        map[string]any{"source": "admin-repair"REDACTED,
		Channel: &AdminBindAuthIdentityChannelInput{
			Channel:        "open",
			ChannelAppID:   "wx-open",
			ChannelSubject: "openid-legacy-123",
			Metadata:       map[string]any{"scene": "admin-repair"REDACTED,
	REDACTED,
REDACTED)
REDACTED
	require.NotNil(t, result)
	require.Equal(t, "wechat-main", result.ProviderKey)
	require.NotNil(t, result.Channel)
	require.Equal(t, "open", result.Channel.Channel)

	identity, err := client.AuthIdentity.Get(ctx, legacyIdentity.ID)
REDACTED
	require.Equal(t, "wechat-main", identity.ProviderKey)
	require.Equal(t, "admin-repair", identity.Metadata["source"])

	channel, err := client.AuthIdentityChannel.Get(ctx, legacyChannel.ID)
REDACTED
	require.Equal(t, "wechat-main", channel.ProviderKey)
	require.Equal(t, legacyIdentity.ID, channel.IdentityID)
	require.Equal(t, "admin-repair", channel.Metadata["scene"])

	identityCount, err := client.AuthIdentity.Query().
		Where(
			authidentity.ProviderTypeEQ("wechat"),
			authidentity.ProviderSubjectEQ("union-legacy-123"),
		).
		Count(ctx)
REDACTED
	require.Equal(t, 1, identityCount)

	channelCount, err := client.AuthIdentityChannel.Query().
		Where(
			authidentitychannel.ProviderTypeEQ("wechat"),
			authidentitychannel.ChannelEQ("open"),
			authidentitychannel.ChannelAppIDEQ("wx-open"),
			authidentitychannel.ChannelSubjectEQ("openid-legacy-123"),
		).
		Count(ctx)
REDACTED
	require.Equal(t, 1, channelCount)
REDACTED

func TestAdminServiceBindUserAuthIdentityRejectsInvalidProviderType(t *testing.T) {
	client := newAdminServiceAuthIdentityBindingTestClient(t)
	ctx := context.Background()

	user, err := client.User.Create().
		SetEmail("invalid-provider@example.com").
		SetPasswordHash("hash").
		SetRole(RoleUser).
		SetStatus(StatusActive).
		Save(ctx)
REDACTED

	svc := &adminServiceImpl{
		userRepo:  &userRepoStub{user: &User{ID: user.ID, Email: user.Email, Status: StatusActiveREDACTEDREDACTED,
		entClient: client,
REDACTED

	_, err = svc.BindUserAuthIdentity(ctx, user.ID, AdminBindAuthIdentityInput{
		ProviderType:    "github",
		ProviderKey:     "github-main",
		ProviderSubject: "subject-3",
REDACTED)
REDACTED
	require.Equal(t, "INVALID_INPUT", infraerrors.Reason(err))
REDACTED
