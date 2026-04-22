//go:build unit

package service

import (
	"context"
	"database/sql"
	"testing"
	"time"

	dbent "github.com/Wei-Shaw/sub2api/ent"
	"github.com/Wei-Shaw/sub2api/ent/enttest"
	"github.com/Wei-Shaw/sub2api/ent/identityadoptiondecision"
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

func TestAuthPendingIdentityService_UpsertAdoptionDecision_ReassignsExistingIdentityReference(t *testing.T) {
	svc, client := newAuthPendingIdentityServiceTestClient(t)
	ctx := context.Background()

	user, err := client.User.Create().
		SetEmail("adoption-reassign@example.com").
		SetPasswordHash("hash").
		SetRole(RoleUser).
		SetStatus(StatusActive).
		Save(ctx)
REDACTED

	identity, err := client.AuthIdentity.Create().
		SetUserID(user.ID).
		SetProviderType("wechat").
		SetProviderKey("wechat-open").
		SetProviderSubject("union-reassign").
		SetMetadata(map[string]any{REDACTED).
		Save(ctx)
REDACTED

	firstSession, err := svc.CreatePendingSession(ctx, CreatePendingAuthSessionInput{
		Intent: "bind_current_user",
		Identity: PendingAuthIdentityKey{
			ProviderType:    "wechat",
			ProviderKey:     "wechat-open",
			ProviderSubject: "union-reassign",
	REDACTED,
REDACTED)
REDACTED

	firstDecision, err := svc.UpsertAdoptionDecision(ctx, PendingIdentityAdoptionDecisionInput{
		PendingAuthSessionID: firstSession.ID,
		IdentityID:           &identity.ID,
		AdoptDisplayName:     true,
		AdoptAvatar:          false,
REDACTED)
REDACTED
	require.NotNil(t, firstDecision.IdentityID)
	require.Equal(t, identity.ID, *firstDecision.IdentityID)

	secondSession, err := svc.CreatePendingSession(ctx, CreatePendingAuthSessionInput{
		Intent: "bind_current_user",
		Identity: PendingAuthIdentityKey{
			ProviderType:    "wechat",
			ProviderKey:     "wechat-open",
			ProviderSubject: "union-reassign",
	REDACTED,
REDACTED)
REDACTED

	secondDecision, err := svc.UpsertAdoptionDecision(ctx, PendingIdentityAdoptionDecisionInput{
		PendingAuthSessionID: secondSession.ID,
		IdentityID:           &identity.ID,
		AdoptDisplayName:     false,
		AdoptAvatar:          true,
REDACTED)
REDACTED
	require.NotNil(t, secondDecision.IdentityID)
	require.Equal(t, identity.ID, *secondDecision.IdentityID)

	reloadedFirst, err := client.IdentityAdoptionDecision.Get(ctx, firstDecision.ID)
REDACTED
	require.Nil(t, reloadedFirst.IdentityID)
REDACTED

func TestAuthPendingIdentityService_UpsertAdoptionDecision_ClearsLegacyNullSessionReference(t *testing.T) {
	t.Skip("legacy NULL pending_auth_session_id rows only exist in production PostgreSQL history; sqlite unit schema rejects NULL")

	svc, client := newAuthPendingIdentityServiceTestClient(t)
	ctx := context.Background()

	user, err := client.User.Create().
		SetEmail("legacy-null-session@example.com").
		SetPasswordHash("hash").
		SetRole(RoleUser).
		SetStatus(StatusActive).
		Save(ctx)
REDACTED

	identity, err := client.AuthIdentity.Create().
		SetUserID(user.ID).
		SetProviderType("wechat").
		SetProviderKey("wechat-main").
		SetProviderSubject("legacy-null-session").
		SetMetadata(map[string]any{REDACTED).
		Save(ctx)
REDACTED

	_, err = client.ExecContext(
		ctx,
		`INSERT INTO identity_adoption_decisions
			(identity_id, adopt_display_name, adopt_avatar, decided_at, created_at, updated_at, pending_auth_session_id)
		VALUES (?, ?, ?, ?, ?, ?, NULL)`,
		identity.ID,
		true,
		false,
		time.Now().UTC(),
		time.Now().UTC(),
		time.Now().UTC(),
	)
REDACTED
	legacyDecision, err := client.IdentityAdoptionDecision.Query().
		Where(identityadoptiondecision.IdentityIDEQ(identity.ID)).
		Only(ctx)
REDACTED
	require.NotNil(t, legacyDecision.IdentityID)

	session, err := svc.CreatePendingSession(ctx, CreatePendingAuthSessionInput{
		Intent: "bind_current_user",
		Identity: PendingAuthIdentityKey{
			ProviderType:    "wechat",
			ProviderKey:     "wechat-main",
			ProviderSubject: "legacy-null-session",
	REDACTED,
REDACTED)
REDACTED

	decision, err := svc.UpsertAdoptionDecision(ctx, PendingIdentityAdoptionDecisionInput{
		PendingAuthSessionID: session.ID,
		IdentityID:           &identity.ID,
		AdoptDisplayName:     false,
		AdoptAvatar:          true,
REDACTED)
REDACTED
	require.NotNil(t, decision.IdentityID)
	require.Equal(t, identity.ID, *decision.IdentityID)

	reloadedLegacy, err := client.IdentityAdoptionDecision.Get(ctx, legacyDecision.ID)
REDACTED
	require.Nil(t, reloadedLegacy.IdentityID)
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

func TestAuthPendingIdentityService_ConsumeBrowserSessionRejectsStaleLoadedSessionReplay(t *testing.T) {
	svc, _ := newAuthPendingIdentityServiceTestClient(t)
	ctx := context.Background()

	session, err := svc.CreatePendingSession(ctx, CreatePendingAuthSessionInput{
		Intent: "login",
		Identity: PendingAuthIdentityKey{
			ProviderType:    "linuxdo",
			ProviderKey:     "linuxdo",
			ProviderSubject: "stale-replay-subject",
	REDACTED,
		BrowserSessionKey: "browser-session",
REDACTED)
REDACTED

	loaded, err := svc.getBrowserSession(ctx, session.SessionToken)
REDACTED

	consumed, err := svc.consumeSession(ctx, loaded, "browser-session", ErrPendingAuthSessionExpired, ErrPendingAuthSessionConsumed)
REDACTED
	require.NotNil(t, consumed.ConsumedAt)

	_, err = svc.consumeSession(ctx, loaded, "browser-session", ErrPendingAuthSessionExpired, ErrPendingAuthSessionConsumed)
	require.ErrorIs(t, err, ErrPendingAuthSessionConsumed)
REDACTED

func TestAuthPendingIdentityService_ConsumeBrowserSessionScrubsLegacyCompletionTokens(t *testing.T) {
	svc, client := newAuthPendingIdentityServiceTestClient(t)
	ctx := context.Background()

	session, err := svc.CreatePendingSession(ctx, CreatePendingAuthSessionInput{
		Intent: "login",
		Identity: PendingAuthIdentityKey{
			ProviderType:    "linuxdo",
			ProviderKey:     "linuxdo",
			ProviderSubject: "legacy-token-subject",
	REDACTED,
		BrowserSessionKey: "browser-session",
		LocalFlowState: map[string]any{
			"completion_response": map[string]any{
				"access_token":  "legacy-access-token",
				"refresh_token": "legacy-refresh-token",
				"expires_in":    float64(3600),
				"token_type":    "Bearer",
				"redirect":      "/dashboard",
		REDACTED,
	REDACTED,
REDACTED)
REDACTED

	consumed, err := svc.ConsumeBrowserSession(ctx, session.SessionToken, "browser-session")
REDACTED
	require.NotNil(t, consumed.ConsumedAt)

	stored, err := client.PendingAuthSession.Get(ctx, session.ID)
REDACTED

	completion, ok := stored.LocalFlowState["completion_response"].(map[string]any)
	require.True(t, ok)
	require.NotContains(t, completion, "access_token")
	require.NotContains(t, completion, "refresh_token")
	require.NotContains(t, completion, "expires_in")
	require.NotContains(t, completion, "token_type")
	require.Equal(t, "/dashboard", completion["redirect"])
REDACTED
