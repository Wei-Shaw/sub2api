//go:build unit

package service

import (
	"context"
	"database/sql"
	"testing"
	"time"

	dbent "github.com/Wei-Shaw/sub2api/ent"
	"github.com/Wei-Shaw/sub2api/ent/enttest"
	"github.com/stretchr/testify/require"

	"entgo.io/ent/dialect"
	entsql "entgo.io/ent/dialect/sql"
	_ "modernc.org/sqlite"
)

func newAuthPendingIdentityServiceTestClient(t *testing.T) (*AuthPendingIdentityService, *dbent.Client) {
REDACTED

	db, err := sql.Open("sqlite", "file:auth_pending_identity_service?mode=memory&cache=shared")
REDACTED
	t.Cleanup(func() { _ = db.Close() REDACTED)

	_, err = db.Exec("PRAGMA foreign_keys = ON")
REDACTED

	drv := entsql.OpenDB(dialect.SQLite, db)
	client := enttest.NewClient(t, enttest.WithOptions(dbent.Driver(drv)))
	t.Cleanup(func() { _ = client.Close() REDACTED)

	return NewAuthPendingIdentityService(client), client
REDACTED

func TestAuthPendingIdentityService_CreatePendingSessionStoresSeparatedState(t *testing.T) {
	svc, client := newAuthPendingIdentityServiceTestClient(t)
	ctx := context.Background()

	targetUser, err := client.User.Create().
		SetEmail("pending-target@example.com").
		SetPasswordHash("hash").
		SetRole(RoleUser).
		SetStatus(StatusActive).
		Save(ctx)
REDACTED

	session, err := svc.CreatePendingSession(ctx, CreatePendingAuthSessionInput{
		Intent: "bind_current_user",
		Identity: PendingAuthIdentityKey{
			ProviderType:    "wechat",
			ProviderKey:     "wechat-open",
			ProviderSubject: "union-123",
	REDACTED,
		TargetUserID:           &targetUser.ID,
		RedirectTo:             "/profile",
		ResolvedEmail:          "user@example.com",
		BrowserSessionKey:      "browser-1",
		UpstreamIdentityClaims: map[string]any{"nickname": "wx-user", "avatar_url": "https://cdn.example/avatar.png"REDACTED,
		LocalFlowState:         map[string]any{"step": "email_required"REDACTED,
REDACTED)
REDACTED
	require.NotEmpty(t, session.SessionToken)
	require.Equal(t, "bind_current_user", session.Intent)
	require.Equal(t, "wechat", session.ProviderType)
	require.NotNil(t, session.TargetUserID)
	require.Equal(t, targetUser.ID, *session.TargetUserID)
	require.Equal(t, "wx-user", session.UpstreamIdentityClaims["nickname"])
	require.Equal(t, "email_required", session.LocalFlowState["step"])
REDACTED

func TestAuthPendingIdentityService_CompletionCodeIsBrowserBoundAndOneTime(t *testing.T) {
	svc, _ := newAuthPendingIdentityServiceTestClient(t)
	ctx := context.Background()

	session, err := svc.CreatePendingSession(ctx, CreatePendingAuthSessionInput{
		Intent: "login",
		Identity: PendingAuthIdentityKey{
			ProviderType:    "linuxdo",
			ProviderKey:     "linuxdo-main",
			ProviderSubject: "subject-1",
	REDACTED,
		BrowserSessionKey:      "browser-expected",
		UpstreamIdentityClaims: map[string]any{"nickname": "linux-user"REDACTED,
		LocalFlowState:         map[string]any{"step": "pending"REDACTED,
REDACTED)
REDACTED

	issued, err := svc.IssueCompletionCode(ctx, IssuePendingAuthCompletionCodeInput{
		PendingAuthSessionID: session.ID,
		BrowserSessionKey:    "browser-expected",
REDACTED)
REDACTED
	require.NotEmpty(t, issued.Code)

	_, err = svc.ConsumeCompletionCode(ctx, issued.Code, "browser-other")
	require.ErrorIs(t, err, ErrPendingAuthBrowserMismatch)

	consumed, err := svc.ConsumeCompletionCode(ctx, issued.Code, "browser-expected")
REDACTED
	require.NotNil(t, consumed.ConsumedAt)
	require.Empty(t, consumed.CompletionCodeHash)
	require.Nil(t, consumed.CompletionCodeExpiresAt)

	_, err = svc.ConsumeCompletionCode(ctx, issued.Code, "browser-expected")
	require.ErrorIs(t, err, ErrPendingAuthCodeInvalid)
REDACTED

func TestAuthPendingIdentityService_CompletionCodeExpires(t *testing.T) {
	svc, client := newAuthPendingIdentityServiceTestClient(t)
	ctx := context.Background()

	session, err := svc.CreatePendingSession(ctx, CreatePendingAuthSessionInput{
		Intent: "login",
		Identity: PendingAuthIdentityKey{
			ProviderType:    "oidc",
			ProviderKey:     "https://issuer.example",
			ProviderSubject: "subject-1",
	REDACTED,
		BrowserSessionKey: "browser-expired",
REDACTED)
REDACTED

	issued, err := svc.IssueCompletionCode(ctx, IssuePendingAuthCompletionCodeInput{
		PendingAuthSessionID: session.ID,
		BrowserSessionKey:    "browser-expired",
		TTL:                  time.Second,
REDACTED)
REDACTED

	_, err = client.PendingAuthSession.UpdateOneID(session.ID).
		SetCompletionCodeExpiresAt(time.Now().UTC().Add(-time.Minute)).
		Save(ctx)
REDACTED

	_, err = svc.ConsumeCompletionCode(ctx, issued.Code, "browser-expired")
	require.ErrorIs(t, err, ErrPendingAuthCodeExpired)
REDACTED

func TestAuthPendingIdentityService_UpsertAdoptionDecision(t *testing.T) {
	svc, client := newAuthPendingIdentityServiceTestClient(t)
	ctx := context.Background()

	user, err := client.User.Create().
		SetEmail("adoption@example.com").
		SetPasswordHash("hash").
		SetRole(RoleUser).
		SetStatus(StatusActive).
		Save(ctx)
REDACTED

	identity, err := client.AuthIdentity.Create().
		SetUserID(user.ID).
		SetProviderType("wechat").
		SetProviderKey("wechat-open").
		SetProviderSubject("union-adoption").
		SetMetadata(map[string]any{REDACTED).
		Save(ctx)
REDACTED

	session, err := svc.CreatePendingSession(ctx, CreatePendingAuthSessionInput{
		Intent: "bind_current_user",
		Identity: PendingAuthIdentityKey{
			ProviderType:    "wechat",
			ProviderKey:     "wechat-open",
			ProviderSubject: "union-adoption",
	REDACTED,
REDACTED)
REDACTED

	first, err := svc.UpsertAdoptionDecision(ctx, PendingIdentityAdoptionDecisionInput{
		PendingAuthSessionID: session.ID,
		AdoptDisplayName:     true,
		AdoptAvatar:          false,
REDACTED)
REDACTED
	require.True(t, first.AdoptDisplayName)
	require.False(t, first.AdoptAvatar)
	require.Nil(t, first.IdentityID)

	second, err := svc.UpsertAdoptionDecision(ctx, PendingIdentityAdoptionDecisionInput{
		PendingAuthSessionID: session.ID,
		IdentityID:           &identity.ID,
		AdoptDisplayName:     true,
		AdoptAvatar:          true,
REDACTED)
REDACTED
	require.Equal(t, first.ID, second.ID)
	require.NotNil(t, second.IdentityID)
	require.Equal(t, identity.ID, *second.IdentityID)
	require.True(t, second.AdoptAvatar)
REDACTED

func TestAuthPendingIdentityService_ConsumeBrowserSession(t *testing.T) {
	svc, _ := newAuthPendingIdentityServiceTestClient(t)
	ctx := context.Background()

	session, err := svc.CreatePendingSession(ctx, CreatePendingAuthSessionInput{
		Intent: "login",
		Identity: PendingAuthIdentityKey{
			ProviderType:    "linuxdo",
			ProviderKey:     "linuxdo",
			ProviderSubject: "subject-session-token",
	REDACTED,
		BrowserSessionKey: "browser-session",
		LocalFlowState: map[string]any{
			"completion_response": map[string]any{
				"access_token": "token",
		REDACTED,
	REDACTED,
REDACTED)
REDACTED

	_, err = svc.ConsumeBrowserSession(ctx, session.SessionToken, "browser-other")
	require.ErrorIs(t, err, ErrPendingAuthBrowserMismatch)

	consumed, err := svc.ConsumeBrowserSession(ctx, session.SessionToken, "browser-session")
REDACTED
	require.NotNil(t, consumed.ConsumedAt)

	_, err = svc.ConsumeBrowserSession(ctx, session.SessionToken, "browser-session")
	require.ErrorIs(t, err, ErrPendingAuthSessionConsumed)
REDACTED
