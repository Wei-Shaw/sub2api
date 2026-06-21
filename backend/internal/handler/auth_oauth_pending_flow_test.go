package handler

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	dbent "github.com/Wei-Shaw/sub2api/ent"
	"github.com/Wei-Shaw/sub2api/ent/authidentity"
	"github.com/Wei-Shaw/sub2api/ent/enttest"
	"github.com/Wei-Shaw/sub2api/ent/identityadoptiondecision"
	"github.com/Wei-Shaw/sub2api/ent/pendingauthsession"
	"github.com/Wei-Shaw/sub2api/ent/redeemcode"
	dbuser "github.com/Wei-Shaw/sub2api/ent/user"
	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/pkg/pagination"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/pquerna/otp/totp"
	"github.com/stretchr/testify/require"

	"entgo.io/ent/dialect"
	entsql "entgo.io/ent/dialect/sql"
	_ "modernc.org/sqlite"
)

func TestApplySuggestedProfileToCompletionResponse(t *testing.T) {
	payload := map[string]any{
		"access_token": "token",
REDACTED
	upstream := map[string]any{
		"suggested_display_name": "Alice",
		"suggested_avatar_url":   "https://cdn.example/avatar.png",
REDACTED

	applySuggestedProfileToCompletionResponse(payload, upstream)

	require.Equal(t, "Alice", payload["suggested_display_name"])
	require.Equal(t, "https://cdn.example/avatar.png", payload["suggested_avatar_url"])
	require.Equal(t, true, payload["adoption_required"])
REDACTED

func TestApplySuggestedProfileToCompletionResponseKeepsExistingPayloadValues(t *testing.T) {
	payload := map[string]any{
		"suggested_display_name": "Existing",
		"adoption_required":      false,
REDACTED
	upstream := map[string]any{
		"suggested_display_name": "Alice",
		"suggested_avatar_url":   "https://cdn.example/avatar.png",
REDACTED

	applySuggestedProfileToCompletionResponse(payload, upstream)

	require.Equal(t, "Existing", payload["suggested_display_name"])
	require.Equal(t, "https://cdn.example/avatar.png", payload["suggested_avatar_url"])
	require.Equal(t, true, payload["adoption_required"])
REDACTED

func TestSetOAuthPendingSessionCookieUsesProviderCompletionPathPrefix(t *testing.T) {
	recorder := httptest.NewRecorder()
	ginCtx, _ := gin.CreateTestContext(recorder)
	ginCtx.Request = httptest.NewRequest(http.MethodGet, "/api/v1/auth/oauth/oidc/callback", nil)

	setOAuthPendingSessionCookie(ginCtx, "pending-session-token", false)

	cookie := findCookie(recorder.Result().Cookies(), oauthPendingSessionCookieName)
	require.NotNil(t, cookie)
	require.Equal(t, "/api/v1/auth/oauth", cookie.Path)
REDACTED

func TestExchangePendingOAuthCompletionPreviewThenFinalizeAppliesAdoptionDecision(t *testing.T) {
	handler, client := newOAuthPendingFlowTestHandler(t, false)
	ctx := context.Background()

	userEntity, err := client.User.Create().
		SetEmail("linuxdo-123@linuxdo-connect.invalid").
		SetUsername("legacy-name").
		SetPasswordHash("hash").
		SetRole(service.RoleUser).
		SetStatus(service.StatusActive).
		Save(ctx)
REDACTED

	session, err := client.PendingAuthSession.Create().
		SetSessionToken("pending-session-token").
		SetIntent("login").
		SetProviderType("linuxdo").
		SetProviderKey("linuxdo").
		SetProviderSubject("123").
		SetTargetUserID(userEntity.ID).
		SetResolvedEmail(userEntity.Email).
		SetBrowserSessionKey("browser-session-key").
		SetUpstreamIdentityClaims(map[string]any{
			"username":               "linuxdo_user",
			"suggested_display_name": "Alice Example",
			"suggested_avatar_url":   "https://cdn.example/alice.png",
	REDACTED).
		SetLocalFlowState(map[string]any{
			oauthCompletionResponseKey: map[string]any{
				"access_token": "access-token",
				"redirect":     "/dashboard",
		REDACTED,
	REDACTED).
		SetExpiresAt(time.Now().UTC().Add(10 * time.Minute)).
		Save(ctx)
REDACTED

	previewRecorder := httptest.NewRecorder()
	previewCtx, _ := gin.CreateTestContext(previewRecorder)
	previewReq := httptest.NewRequest(http.MethodPost, "/api/v1/auth/oauth/pending/exchange", nil)
	previewReq.AddCookie(&http.Cookie{Name: oauthPendingSessionCookieName, Value: encodeCookieValue(session.SessionToken)REDACTED)
	previewReq.AddCookie(&http.Cookie{Name: oauthPendingBrowserCookieName, Value: encodeCookieValue("browser-session-key")REDACTED)
	previewCtx.Request = previewReq

	handler.ExchangePendingOAuthCompletion(previewCtx)

	require.Equal(t, http.StatusOK, previewRecorder.Code)
	previewData := decodeJSONResponseData(t, previewRecorder)
	require.Equal(t, "Alice Example", previewData["suggested_display_name"])
	require.Equal(t, "https://cdn.example/alice.png", previewData["suggested_avatar_url"])
	require.Equal(t, true, previewData["adoption_required"])

	storedUser, err := client.User.Get(ctx, userEntity.ID)
REDACTED
	require.Equal(t, "legacy-name", storedUser.Username)

	previewSession, err := client.PendingAuthSession.Query().
		Where(pendingauthsession.IDEQ(session.ID)).
		Only(ctx)
REDACTED
	require.Nil(t, previewSession.ConsumedAt)

	body := bytes.NewBufferString(`{"adopt_display_name":true,"adopt_avatar":trueREDACTED`)
	finalizeRecorder := httptest.NewRecorder()
	finalizeCtx, _ := gin.CreateTestContext(finalizeRecorder)
	finalizeReq := httptest.NewRequest(http.MethodPost, "/api/v1/auth/oauth/pending/exchange", body)
	finalizeReq.Header.Set("Content-Type", "application/json")
	finalizeReq.AddCookie(&http.Cookie{Name: oauthPendingSessionCookieName, Value: encodeCookieValue(session.SessionToken)REDACTED)
	finalizeReq.AddCookie(&http.Cookie{Name: oauthPendingBrowserCookieName, Value: encodeCookieValue("browser-session-key")REDACTED)
	finalizeCtx.Request = finalizeReq

	handler.ExchangePendingOAuthCompletion(finalizeCtx)

	require.Equal(t, http.StatusOK, finalizeRecorder.Code)

	storedUser, err = client.User.Get(ctx, userEntity.ID)
REDACTED
	require.Equal(t, "Alice Example", storedUser.Username)

	identity, err := client.AuthIdentity.Query().
		Where(
			authidentity.ProviderTypeEQ("linuxdo"),
			authidentity.ProviderKeyEQ("linuxdo"),
			authidentity.ProviderSubjectEQ("123"),
		).
		Only(ctx)
REDACTED
	require.Equal(t, userEntity.ID, identity.UserID)
	require.Equal(t, "Alice Example", identity.Metadata["display_name"])
	require.Equal(t, "https://cdn.example/alice.png", identity.Metadata["avatar_url"])

	avatar := loadUserAvatarRecord(t, client, userEntity.ID)
	require.NotNil(t, avatar)
	require.Equal(t, "remote_url", avatar.StorageProvider)
	require.Equal(t, "https://cdn.example/alice.png", avatar.URL)

	decision, err := client.IdentityAdoptionDecision.Query().
		Where(identityadoptiondecision.PendingAuthSessionIDEQ(session.ID)).
		Only(ctx)
REDACTED
	require.NotNil(t, decision.IdentityID)
	require.Equal(t, identity.ID, *decision.IdentityID)
	require.True(t, decision.AdoptDisplayName)
	require.True(t, decision.AdoptAvatar)

	consumed, err := client.PendingAuthSession.Query().
		Where(pendingauthsession.IDEQ(session.ID)).
		Only(ctx)
REDACTED
	require.NotNil(t, consumed.ConsumedAt)
REDACTED

func TestExchangePendingOAuthCompletionSkipsInvalidAvatarAdoptionWithoutBlockingCompletion(t *testing.T) {
	handler, client := newOAuthPendingFlowTestHandler(t, false)
	ctx := context.Background()

	userEntity, err := client.User.Create().
		SetEmail("invalid-avatar@example.com").
		SetUsername("legacy-name").
		SetPasswordHash("hash").
		SetRole(service.RoleUser).
		SetStatus(service.StatusActive).
		Save(ctx)
REDACTED

	session, err := client.PendingAuthSession.Create().
		SetSessionToken("pending-invalid-avatar-token").
		SetIntent("login").
		SetProviderType("linuxdo").
		SetProviderKey("linuxdo").
		SetProviderSubject("invalid-avatar-123").
		SetTargetUserID(userEntity.ID).
		SetResolvedEmail(userEntity.Email).
		SetBrowserSessionKey("browser-invalid-avatar-key").
		SetUpstreamIdentityClaims(map[string]any{
			"username":               "linuxdo_user",
			"suggested_display_name": "Alice Example",
			"suggested_avatar_url":   "/avatars/alice.png",
	REDACTED).
		SetLocalFlowState(map[string]any{
			oauthCompletionResponseKey: map[string]any{
				"access_token": "access-token",
				"redirect":     "/dashboard",
		REDACTED,
	REDACTED).
		SetExpiresAt(time.Now().UTC().Add(10 * time.Minute)).
		Save(ctx)
REDACTED

	body := bytes.NewBufferString(`{"adopt_display_name":true,"adopt_avatar":trueREDACTED`)
	recorder := httptest.NewRecorder()
	ginCtx, _ := gin.CreateTestContext(recorder)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/oauth/pending/exchange", body)
	req.Header.Set("Content-Type", "application/json")
	req.AddCookie(&http.Cookie{Name: oauthPendingSessionCookieName, Value: encodeCookieValue(session.SessionToken)REDACTED)
	req.AddCookie(&http.Cookie{Name: oauthPendingBrowserCookieName, Value: encodeCookieValue("browser-invalid-avatar-key")REDACTED)
	ginCtx.Request = req

	handler.ExchangePendingOAuthCompletion(ginCtx)

	require.Equal(t, http.StatusOK, recorder.Code)

	identity, err := client.AuthIdentity.Query().
		Where(
			authidentity.ProviderTypeEQ("linuxdo"),
			authidentity.ProviderKeyEQ("linuxdo"),
			authidentity.ProviderSubjectEQ("invalid-avatar-123"),
		).
		Only(ctx)
REDACTED
	require.Equal(t, "Alice Example", identity.Metadata["display_name"])
	_, hasAdoptedAvatar := identity.Metadata["avatar_url"]
	require.False(t, hasAdoptedAvatar)

	avatar := loadUserAvatarRecord(t, client, userEntity.ID)
	require.Nil(t, avatar)

	consumed, err := client.PendingAuthSession.Query().
		Where(pendingauthsession.IDEQ(session.ID)).
		Only(ctx)
REDACTED
	require.NotNil(t, consumed.ConsumedAt)
REDACTED

func TestExchangePendingOAuthCompletionBindCurrentUserPreviewThenFinalizeBindsIdentityWithoutAdoption(t *testing.T) {
	handler, client := newOAuthPendingFlowTestHandler(t, false)
	ctx := context.Background()

	userEntity, err := client.User.Create().
		SetEmail("bind-target@example.com").
		SetUsername("legacy-name").
		SetPasswordHash("hash").
		SetRole(service.RoleUser).
		SetStatus(service.StatusActive).
		Save(ctx)
REDACTED

	session, err := client.PendingAuthSession.Create().
		SetSessionToken("bind-pending-session-token").
		SetIntent("bind_current_user").
		SetProviderType("linuxdo").
		SetProviderKey("linuxdo").
		SetProviderSubject("bind-123").
		SetTargetUserID(userEntity.ID).
		SetResolvedEmail(userEntity.Email).
		SetBrowserSessionKey("bind-browser-session-key").
		SetUpstreamIdentityClaims(map[string]any{
			"username":               "linuxdo_user",
			"suggested_display_name": "Bound Example",
			"suggested_avatar_url":   "https://cdn.example/bound.png",
	REDACTED).
		SetLocalFlowState(map[string]any{
			oauthCompletionResponseKey: map[string]any{
				"access_token": "access-token",
				"redirect":     "/settings/profile",
		REDACTED,
	REDACTED).
		SetExpiresAt(time.Now().UTC().Add(10 * time.Minute)).
		Save(ctx)
REDACTED

	previewRecorder := httptest.NewRecorder()
	previewCtx, _ := gin.CreateTestContext(previewRecorder)
	previewReq := httptest.NewRequest(http.MethodPost, "/api/v1/auth/oauth/pending/exchange", nil)
	previewReq.AddCookie(&http.Cookie{Name: oauthPendingSessionCookieName, Value: encodeCookieValue(session.SessionToken)REDACTED)
	previewReq.AddCookie(&http.Cookie{Name: oauthPendingBrowserCookieName, Value: encodeCookieValue("bind-browser-session-key")REDACTED)
	previewCtx.Request = previewReq

	handler.ExchangePendingOAuthCompletion(previewCtx)

	require.Equal(t, http.StatusOK, previewRecorder.Code)
	previewData := decodeJSONResponseData(t, previewRecorder)
	require.Equal(t, "Bound Example", previewData["suggested_display_name"])
	require.Equal(t, "https://cdn.example/bound.png", previewData["suggested_avatar_url"])
	require.Equal(t, true, previewData["adoption_required"])

	identityCount, err := client.AuthIdentity.Query().
		Where(
			authidentity.ProviderTypeEQ("linuxdo"),
			authidentity.ProviderKeyEQ("linuxdo"),
			authidentity.ProviderSubjectEQ("bind-123"),
		).
		Count(ctx)
REDACTED
	require.Zero(t, identityCount)

	previewSession, err := client.PendingAuthSession.Query().
		Where(pendingauthsession.IDEQ(session.ID)).
		Only(ctx)
REDACTED
	require.Nil(t, previewSession.ConsumedAt)

	body := bytes.NewBufferString(`{"adopt_display_name":false,"adopt_avatar":falseREDACTED`)
	finalizeRecorder := httptest.NewRecorder()
	finalizeCtx, _ := gin.CreateTestContext(finalizeRecorder)
	finalizeReq := httptest.NewRequest(http.MethodPost, "/api/v1/auth/oauth/pending/exchange", body)
	finalizeReq.Header.Set("Content-Type", "application/json")
	finalizeReq.AddCookie(&http.Cookie{Name: oauthPendingSessionCookieName, Value: encodeCookieValue(session.SessionToken)REDACTED)
	finalizeReq.AddCookie(&http.Cookie{Name: oauthPendingBrowserCookieName, Value: encodeCookieValue("bind-browser-session-key")REDACTED)
	finalizeCtx.Request = finalizeReq

	handler.ExchangePendingOAuthCompletion(finalizeCtx)

	require.Equal(t, http.StatusOK, finalizeRecorder.Code)

	storedUser, err := client.User.Get(ctx, userEntity.ID)
REDACTED
	require.Equal(t, "legacy-name", storedUser.Username)

	identity, err := client.AuthIdentity.Query().
		Where(
			authidentity.ProviderTypeEQ("linuxdo"),
			authidentity.ProviderKeyEQ("linuxdo"),
			authidentity.ProviderSubjectEQ("bind-123"),
		).
		Only(ctx)
REDACTED
	require.Equal(t, userEntity.ID, identity.UserID)
	require.Equal(t, "Bound Example", identity.Metadata["suggested_display_name"])
	require.Equal(t, "https://cdn.example/bound.png", identity.Metadata["suggested_avatar_url"])
	_, hasDisplayName := identity.Metadata["display_name"]
	require.False(t, hasDisplayName)
	_, hasAvatarURL := identity.Metadata["avatar_url"]
	require.False(t, hasAvatarURL)

	decision, err := client.IdentityAdoptionDecision.Query().
		Where(identityadoptiondecision.PendingAuthSessionIDEQ(session.ID)).
		Only(ctx)
REDACTED
	require.NotNil(t, decision.IdentityID)
	require.Equal(t, identity.ID, *decision.IdentityID)
	require.False(t, decision.AdoptDisplayName)
	require.False(t, decision.AdoptAvatar)

	consumed, err := client.PendingAuthSession.Query().
		Where(pendingauthsession.IDEQ(session.ID)).
		Only(ctx)
REDACTED
	require.NotNil(t, consumed.ConsumedAt)
REDACTED

func TestExchangePendingOAuthCompletionBindCurrentUserOwnershipConflict(t *testing.T) {
	handler, client := newOAuthPendingFlowTestHandler(t, false)
	ctx := context.Background()

	targetUser, err := client.User.Create().
		SetEmail("bind-conflict-target@example.com").
		SetUsername("target-user").
		SetPasswordHash("hash").
		SetRole(service.RoleUser).
		SetStatus(service.StatusActive).
		Save(ctx)
REDACTED

	ownerUser, err := client.User.Create().
		SetEmail("bind-conflict-owner@example.com").
		SetUsername("owner-user").
		SetPasswordHash("hash").
		SetRole(service.RoleUser).
		SetStatus(service.StatusActive).
		Save(ctx)
REDACTED

	existingIdentity, err := client.AuthIdentity.Create().
		SetUserID(ownerUser.ID).
		SetProviderType("linuxdo").
		SetProviderKey("linuxdo").
		SetProviderSubject("conflict-123").
		SetMetadata(map[string]any{"username": "owner-user"REDACTED).
		Save(ctx)
REDACTED

	session, err := client.PendingAuthSession.Create().
		SetSessionToken("bind-conflict-session-token").
		SetIntent("bind_current_user").
		SetProviderType("linuxdo").
		SetProviderKey("linuxdo").
		SetProviderSubject("conflict-123").
		SetTargetUserID(targetUser.ID).
		SetResolvedEmail(targetUser.Email).
		SetBrowserSessionKey("bind-conflict-browser-session-key").
		SetUpstreamIdentityClaims(map[string]any{
			"suggested_display_name": "Conflict Example",
			"suggested_avatar_url":   "https://cdn.example/conflict.png",
	REDACTED).
		SetLocalFlowState(map[string]any{
			oauthCompletionResponseKey: map[string]any{
				"access_token": "access-token",
		REDACTED,
	REDACTED).
		SetExpiresAt(time.Now().UTC().Add(10 * time.Minute)).
		Save(ctx)
REDACTED

	body := bytes.NewBufferString(`{"adopt_display_name":false,"adopt_avatar":falseREDACTED`)
	recorder := httptest.NewRecorder()
	ginCtx, _ := gin.CreateTestContext(recorder)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/oauth/pending/exchange", body)
	req.Header.Set("Content-Type", "application/json")
	req.AddCookie(&http.Cookie{Name: oauthPendingSessionCookieName, Value: encodeCookieValue(session.SessionToken)REDACTED)
	req.AddCookie(&http.Cookie{Name: oauthPendingBrowserCookieName, Value: encodeCookieValue("bind-conflict-browser-session-key")REDACTED)
	ginCtx.Request = req

	handler.ExchangePendingOAuthCompletion(ginCtx)

	require.Equal(t, http.StatusInternalServerError, recorder.Code)
	payload := decodeJSONBody(t, recorder)
	require.Equal(t, "PENDING_AUTH_ADOPTION_APPLY_FAILED", payload["reason"])

	identity, err := client.AuthIdentity.Get(ctx, existingIdentity.ID)
REDACTED
	require.Equal(t, ownerUser.ID, identity.UserID)

	decision, err := client.IdentityAdoptionDecision.Query().
		Where(identityadoptiondecision.PendingAuthSessionIDEQ(session.ID)).
		Only(ctx)
REDACTED
	require.Nil(t, decision.IdentityID)
	require.False(t, decision.AdoptDisplayName)
	require.False(t, decision.AdoptAvatar)

	storedSession, err := client.PendingAuthSession.Query().
		Where(pendingauthsession.IDEQ(session.ID)).
		Only(ctx)
REDACTED
	require.Nil(t, storedSession.ConsumedAt)
REDACTED

func TestExchangePendingOAuthCompletionLoginFalseFalseBindsIdentityWithoutAdoption(t *testing.T) {
	handler, client := newOAuthPendingFlowTestHandler(t, false)
	ctx := context.Background()

	userEntity, err := client.User.Create().
		SetEmail("login-false@example.com").
		SetUsername("legacy-name").
		SetPasswordHash("hash").
		SetRole(service.RoleUser).
		SetStatus(service.StatusActive).
		Save(ctx)
REDACTED

	session, err := client.PendingAuthSession.Create().
		SetSessionToken("login-false-session-token").
		SetIntent("login").
		SetProviderType("linuxdo").
		SetProviderKey("linuxdo").
		SetProviderSubject("login-false-123").
		SetTargetUserID(userEntity.ID).
		SetResolvedEmail(userEntity.Email).
		SetBrowserSessionKey("login-false-browser-session-key").
		SetUpstreamIdentityClaims(map[string]any{
			"suggested_display_name": "Login Example",
			"suggested_avatar_url":   "https://cdn.example/login.png",
	REDACTED).
		SetLocalFlowState(map[string]any{
			oauthCompletionResponseKey: map[string]any{
				"access_token": "access-token",
		REDACTED,
	REDACTED).
		SetExpiresAt(time.Now().UTC().Add(10 * time.Minute)).
		Save(ctx)
REDACTED

	body := bytes.NewBufferString(`{"adopt_display_name":false,"adopt_avatar":falseREDACTED`)
	recorder := httptest.NewRecorder()
	ginCtx, _ := gin.CreateTestContext(recorder)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/oauth/pending/exchange", body)
	req.Header.Set("Content-Type", "application/json")
	req.AddCookie(&http.Cookie{Name: oauthPendingSessionCookieName, Value: encodeCookieValue(session.SessionToken)REDACTED)
	req.AddCookie(&http.Cookie{Name: oauthPendingBrowserCookieName, Value: encodeCookieValue("login-false-browser-session-key")REDACTED)
	ginCtx.Request = req

	handler.ExchangePendingOAuthCompletion(ginCtx)

	require.Equal(t, http.StatusOK, recorder.Code)

	identity, err := client.AuthIdentity.Query().
		Where(
			authidentity.ProviderTypeEQ("linuxdo"),
			authidentity.ProviderKeyEQ("linuxdo"),
			authidentity.ProviderSubjectEQ("login-false-123"),
		).
		Only(ctx)
REDACTED
	require.Equal(t, userEntity.ID, identity.UserID)

	decision, err := client.IdentityAdoptionDecision.Query().
		Where(identityadoptiondecision.PendingAuthSessionIDEQ(session.ID)).
		Only(ctx)
REDACTED
	require.NotNil(t, decision.IdentityID)
	require.Equal(t, identity.ID, *decision.IdentityID)
	require.False(t, decision.AdoptDisplayName)
	require.False(t, decision.AdoptAvatar)

	storedSession, err := client.PendingAuthSession.Query().
		Where(pendingauthsession.IDEQ(session.ID)).
		Only(ctx)
REDACTED
	require.NotNil(t, storedSession.ConsumedAt)
REDACTED

func TestExchangePendingOAuthCompletionLoginReassignsExistingDecisionIdentityReference(t *testing.T) {
	handler, client := newOAuthPendingFlowTestHandler(t, false)
	ctx := context.Background()

	userEntity, err := client.User.Create().
		SetEmail("login-reassign@example.com").
		SetUsername("legacy-name").
		SetPasswordHash("hash").
		SetRole(service.RoleUser).
		SetStatus(service.StatusActive).
		Save(ctx)
REDACTED

	existingIdentity, err := client.AuthIdentity.Create().
		SetUserID(userEntity.ID).
		SetProviderType("linuxdo").
		SetProviderKey("linuxdo").
		SetProviderSubject("login-reassign-123").
		SetMetadata(map[string]any{REDACTED).
		Save(ctx)
REDACTED

	previousSession, err := client.PendingAuthSession.Create().
		SetSessionToken("login-reassign-previous-session-token").
		SetIntent("login").
		SetProviderType("linuxdo").
		SetProviderKey("linuxdo").
		SetProviderSubject("login-reassign-123").
		SetTargetUserID(userEntity.ID).
		SetResolvedEmail(userEntity.Email).
		SetBrowserSessionKey("login-reassign-previous-browser-session-key").
		SetLocalFlowState(map[string]any{
			oauthCompletionResponseKey: map[string]any{
				"access_token": "previous-access-token",
		REDACTED,
	REDACTED).
		SetExpiresAt(time.Now().UTC().Add(10 * time.Minute)).
		Save(ctx)
REDACTED

	previousDecision, err := client.IdentityAdoptionDecision.Create().
		SetPendingAuthSessionID(previousSession.ID).
		SetIdentityID(existingIdentity.ID).
		SetAdoptDisplayName(true).
		SetAdoptAvatar(true).
		Save(ctx)
REDACTED

	session, err := client.PendingAuthSession.Create().
		SetSessionToken("login-reassign-session-token").
		SetIntent("login").
		SetProviderType("linuxdo").
		SetProviderKey("linuxdo").
		SetProviderSubject("login-reassign-123").
		SetTargetUserID(userEntity.ID).
		SetResolvedEmail(userEntity.Email).
		SetBrowserSessionKey("login-reassign-browser-session-key").
		SetUpstreamIdentityClaims(map[string]any{
			"suggested_display_name": "Login Reassign",
			"suggested_avatar_url":   "https://cdn.example/login-reassign.png",
	REDACTED).
		SetLocalFlowState(map[string]any{
			oauthCompletionResponseKey: map[string]any{
				"access_token": "access-token",
		REDACTED,
	REDACTED).
		SetExpiresAt(time.Now().UTC().Add(10 * time.Minute)).
		Save(ctx)
REDACTED

	_, err = client.IdentityAdoptionDecision.Create().
		SetPendingAuthSessionID(session.ID).
		SetAdoptDisplayName(false).
		SetAdoptAvatar(false).
		Save(ctx)
REDACTED

	body := bytes.NewBufferString(`{"adopt_display_name":false,"adopt_avatar":falseREDACTED`)
	recorder := httptest.NewRecorder()
	ginCtx, _ := gin.CreateTestContext(recorder)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/oauth/pending/exchange", body)
	req.Header.Set("Content-Type", "application/json")
	req.AddCookie(&http.Cookie{Name: oauthPendingSessionCookieName, Value: encodeCookieValue(session.SessionToken)REDACTED)
	req.AddCookie(&http.Cookie{Name: oauthPendingBrowserCookieName, Value: encodeCookieValue("login-reassign-browser-session-key")REDACTED)
	ginCtx.Request = req

	handler.ExchangePendingOAuthCompletion(ginCtx)

	require.Equal(t, http.StatusOK, recorder.Code)

	reloadedPrevious, err := client.IdentityAdoptionDecision.Get(ctx, previousDecision.ID)
REDACTED
	require.Nil(t, reloadedPrevious.IdentityID)

	currentDecision, err := client.IdentityAdoptionDecision.Query().
		Where(identityadoptiondecision.PendingAuthSessionIDEQ(session.ID)).
		Only(ctx)
REDACTED
	require.NotNil(t, currentDecision.IdentityID)
	require.Equal(t, existingIdentity.ID, *currentDecision.IdentityID)

	storedSession, err := client.PendingAuthSession.Query().
		Where(pendingauthsession.IDEQ(session.ID)).
		Only(ctx)
REDACTED
	require.NotNil(t, storedSession.ConsumedAt)
REDACTED

func TestExchangePendingOAuthCompletionLoginWithoutDecisionStillBindsIdentity(t *testing.T) {
	handler, client := newOAuthPendingFlowTestHandler(t, false)
	ctx := context.Background()

	userEntity, err := client.User.Create().
		SetEmail("login-nodecision@example.com").
		SetUsername("legacy-name").
		SetPasswordHash("hash").
		SetRole(service.RoleUser).
		SetStatus(service.StatusActive).
		Save(ctx)
REDACTED

	session, err := client.PendingAuthSession.Create().
		SetSessionToken("login-nodecision-session-token").
		SetIntent("login").
		SetProviderType("linuxdo").
		SetProviderKey("linuxdo").
		SetProviderSubject("login-nodecision-123").
		SetTargetUserID(userEntity.ID).
		SetResolvedEmail(userEntity.Email).
		SetBrowserSessionKey("login-nodecision-browser-session-key").
		SetUpstreamIdentityClaims(map[string]any{
			"username": "login-nodecision-user",
	REDACTED).
		SetLocalFlowState(map[string]any{
			oauthCompletionResponseKey: map[string]any{
				"access_token": "access-token",
		REDACTED,
	REDACTED).
		SetExpiresAt(time.Now().UTC().Add(10 * time.Minute)).
		Save(ctx)
REDACTED

	recorder := httptest.NewRecorder()
	ginCtx, _ := gin.CreateTestContext(recorder)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/oauth/pending/exchange", nil)
	req.AddCookie(&http.Cookie{Name: oauthPendingSessionCookieName, Value: encodeCookieValue(session.SessionToken)REDACTED)
	req.AddCookie(&http.Cookie{Name: oauthPendingBrowserCookieName, Value: encodeCookieValue("login-nodecision-browser-session-key")REDACTED)
	ginCtx.Request = req

	handler.ExchangePendingOAuthCompletion(ginCtx)

	require.Equal(t, http.StatusOK, recorder.Code)

	identity, err := client.AuthIdentity.Query().
		Where(
			authidentity.ProviderTypeEQ("linuxdo"),
			authidentity.ProviderKeyEQ("linuxdo"),
			authidentity.ProviderSubjectEQ("login-nodecision-123"),
		).
		Only(ctx)
REDACTED
	require.Equal(t, userEntity.ID, identity.UserID)

	storedSession, err := client.PendingAuthSession.Query().
		Where(pendingauthsession.IDEQ(session.ID)).
		Only(ctx)
REDACTED
	require.NotNil(t, storedSession.ConsumedAt)
REDACTED

func TestExchangePendingOAuthCompletionExistingLoginWithSuggestedProfileSkipsAdoptionPrompt(t *testing.T) {
	handler, client := newOAuthPendingFlowTestHandler(t, false)
	ctx := context.Background()

	userEntity, err := client.User.Create().
		SetEmail("existing-login@example.com").
		SetUsername("existing-login-user").
		SetPasswordHash("hash").
		SetRole(service.RoleUser).
		SetStatus(service.StatusActive).
		Save(ctx)
REDACTED

	_, err = client.AuthIdentity.Create().
		SetUserID(userEntity.ID).
		SetProviderType("linuxdo").
		SetProviderKey("linuxdo").
		SetProviderSubject("existing-login-123").
		SetMetadata(map[string]any{
			"username": "existing-login-user",
	REDACTED).
		Save(ctx)
REDACTED

	session, err := client.PendingAuthSession.Create().
		SetSessionToken("existing-login-session-token").
		SetIntent("login").
		SetProviderType("linuxdo").
		SetProviderKey("linuxdo").
		SetProviderSubject("existing-login-123").
		SetTargetUserID(userEntity.ID).
		SetResolvedEmail(userEntity.Email).
		SetBrowserSessionKey("existing-login-browser-session-key").
		SetUpstreamIdentityClaims(map[string]any{
			"suggested_display_name": "Existing Login Example",
			"suggested_avatar_url":   "https://cdn.example/existing-login.png",
	REDACTED).
		SetLocalFlowState(map[string]any{
			oauthCompletionResponseKey: map[string]any{
				"access_token":  "legacy-access-token",
				"refresh_token": "legacy-refresh-token",
				"expires_in":    float64(3600),
				"token_type":    "Bearer",
				"redirect":      "/dashboard",
		REDACTED,
	REDACTED).
		SetExpiresAt(time.Now().UTC().Add(10 * time.Minute)).
		Save(ctx)
REDACTED

	recorder := httptest.NewRecorder()
	ginCtx, _ := gin.CreateTestContext(recorder)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/oauth/pending/exchange", nil)
	req.AddCookie(&http.Cookie{Name: oauthPendingSessionCookieName, Value: encodeCookieValue(session.SessionToken)REDACTED)
	req.AddCookie(&http.Cookie{Name: oauthPendingBrowserCookieName, Value: encodeCookieValue("existing-login-browser-session-key")REDACTED)
	ginCtx.Request = req

	handler.ExchangePendingOAuthCompletion(ginCtx)

	require.Equal(t, http.StatusOK, recorder.Code)

	payload := decodeJSONResponseData(t, recorder)
	require.NotEmpty(t, payload["access_token"])
	require.NotEmpty(t, payload["refresh_token"])
	require.NotEqual(t, "legacy-access-token", payload["access_token"])
	require.NotEqual(t, "legacy-refresh-token", payload["refresh_token"])
	require.Equal(t, "/dashboard", payload["redirect"])
	require.Equal(t, "Existing Login Example", payload["suggested_display_name"])
	require.Equal(t, "https://cdn.example/existing-login.png", payload["suggested_avatar_url"])
	require.NotContains(t, payload, "adoption_required")

	accessToken, ok := payload["access_token"].(string)
	require.True(t, ok)
	claims, err := handler.authService.ValidateToken(accessToken)
REDACTED
	reloadedUser, err := handler.userService.GetByID(ctx, userEntity.ID)
REDACTED
	require.Equal(t, reloadedUser.TokenVersion, claims.TokenVersion)

	decisionCount, err := client.IdentityAdoptionDecision.Query().
		Where(identityadoptiondecision.PendingAuthSessionIDEQ(session.ID)).
		Count(ctx)
REDACTED
	require.Equal(t, 1, decisionCount)

	storedSession, err := client.PendingAuthSession.Get(ctx, session.ID)
REDACTED
	require.NotNil(t, storedSession.ConsumedAt)

	completion, ok := storedSession.LocalFlowState[oauthCompletionResponseKey].(map[string]any)
	require.True(t, ok)
	require.NotContains(t, completion, "access_token")
	require.NotContains(t, completion, "refresh_token")
	require.NotContains(t, completion, "expires_in")
	require.NotContains(t, completion, "token_type")
	require.Equal(t, "/dashboard", completion["redirect"])
REDACTED

func TestExchangePendingOAuthCompletionBlocksBackendModeBeforeReturningTokenPayload(t *testing.T) {
	handler, client := newOAuthPendingFlowTestHandlerWithDependencies(t, oauthPendingFlowTestHandlerOptions{
		settingValues: map[string]string{
			service.SettingKeyBackendModeEnabled: "true",
	REDACTED,
REDACTED)
	ctx := context.Background()

	userEntity, err := client.User.Create().
		SetEmail("blocked@example.com").
		SetUsername("blocked-user").
		SetPasswordHash("hash").
		SetRole(service.RoleUser).
		SetStatus(service.StatusActive).
		Save(ctx)
REDACTED

	session, err := client.PendingAuthSession.Create().
		SetSessionToken("blocked-backend-mode-session-token").
		SetIntent("login").
		SetProviderType("linuxdo").
		SetProviderKey("linuxdo").
		SetProviderSubject("blocked-subject-123").
		SetTargetUserID(userEntity.ID).
		SetResolvedEmail(userEntity.Email).
		SetBrowserSessionKey("blocked-backend-mode-browser-session-key").
		SetLocalFlowState(map[string]any{
			oauthCompletionResponseKey: map[string]any{
				"access_token":  "access-token",
				"refresh_token": "refresh-token",
				"expires_in":    float64(3600),
				"token_type":    "Bearer",
		REDACTED,
	REDACTED).
		SetExpiresAt(time.Now().UTC().Add(10 * time.Minute)).
		Save(ctx)
REDACTED

	recorder := httptest.NewRecorder()
	ginCtx, _ := gin.CreateTestContext(recorder)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/oauth/pending/exchange", nil)
	req.AddCookie(&http.Cookie{Name: oauthPendingSessionCookieName, Value: encodeCookieValue(session.SessionToken)REDACTED)
	req.AddCookie(&http.Cookie{Name: oauthPendingBrowserCookieName, Value: encodeCookieValue("blocked-backend-mode-browser-session-key")REDACTED)
	ginCtx.Request = req

	handler.ExchangePendingOAuthCompletion(ginCtx)

	require.Equal(t, http.StatusForbidden, recorder.Code)

	storedSession, err := client.PendingAuthSession.Get(ctx, session.ID)
REDACTED
	require.Nil(t, storedSession.ConsumedAt)
REDACTED

func TestExchangePendingOAuthCompletionRejectsDisabledTargetUser(t *testing.T) {
	handler, client := newOAuthPendingFlowTestHandler(t, false)
	ctx := context.Background()

	userEntity, err := client.User.Create().
		SetEmail("disabled-linked@example.com").
		SetUsername("disabled-linked-user").
		SetPasswordHash("hash").
		SetRole(service.RoleUser).
		SetStatus(service.StatusDisabled).
		Save(ctx)
REDACTED

	session, err := client.PendingAuthSession.Create().
		SetSessionToken("disabled-linked-session-token").
		SetIntent("login").
		SetProviderType("linuxdo").
		SetProviderKey("linuxdo").
		SetProviderSubject("disabled-linked-subject").
		SetTargetUserID(userEntity.ID).
		SetResolvedEmail(userEntity.Email).
		SetBrowserSessionKey("disabled-linked-browser-session-key").
		SetUpstreamIdentityClaims(map[string]any{
			"suggested_display_name": "Disabled Linked User",
	REDACTED).
		SetLocalFlowState(map[string]any{
			oauthCompletionResponseKey: map[string]any{
				"redirect": "/dashboard",
		REDACTED,
	REDACTED).
		SetExpiresAt(time.Now().UTC().Add(10 * time.Minute)).
		Save(ctx)
REDACTED

	recorder := httptest.NewRecorder()
	ginCtx, _ := gin.CreateTestContext(recorder)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/oauth/pending/exchange", nil)
	req.AddCookie(&http.Cookie{Name: oauthPendingSessionCookieName, Value: encodeCookieValue(session.SessionToken)REDACTED)
	req.AddCookie(&http.Cookie{Name: oauthPendingBrowserCookieName, Value: encodeCookieValue("disabled-linked-browser-session-key")REDACTED)
	ginCtx.Request = req

	handler.ExchangePendingOAuthCompletion(ginCtx)

	require.Equal(t, http.StatusForbidden, recorder.Code)

	storedSession, err := client.PendingAuthSession.Get(ctx, session.ID)
REDACTED
	require.Nil(t, storedSession.ConsumedAt)
REDACTED

func TestNormalizePendingOAuthCompletionResponseScrubsLegacyTokenPayload(t *testing.T) {
	payload := normalizePendingOAuthCompletionResponse(map[string]any{
		"access_token":  "legacy-access-token",
		"refresh_token": "legacy-refresh-token",
		"expires_in":    float64(3600),
		"token_type":    "Bearer",
		"redirect":      "/dashboard",
REDACTED)

	require.NotContains(t, payload, "access_token")
	require.NotContains(t, payload, "refresh_token")
	require.NotContains(t, payload, "expires_in")
	require.NotContains(t, payload, "token_type")
	require.Equal(t, "/dashboard", payload["redirect"])
REDACTED

func TestExchangePendingOAuthCompletionInvitationRequiredFalseFalsePersistsDecisionWithoutBinding(t *testing.T) {
	handler, client := newOAuthPendingFlowTestHandler(t, true)
	ctx := context.Background()

	session, err := client.PendingAuthSession.Create().
		SetSessionToken("invitation-required-session-token").
		SetIntent("login").
		SetProviderType("linuxdo").
		SetProviderKey("linuxdo").
		SetProviderSubject("invitation-123").
		SetBrowserSessionKey("invitation-required-browser-session-key").
		SetUpstreamIdentityClaims(map[string]any{
			"suggested_display_name": "Invite Example",
			"suggested_avatar_url":   "https://cdn.example/invite.png",
	REDACTED).
		SetLocalFlowState(map[string]any{
			oauthCompletionResponseKey: map[string]any{
				"error": "invitation_required",
		REDACTED,
	REDACTED).
		SetExpiresAt(time.Now().UTC().Add(10 * time.Minute)).
		Save(ctx)
REDACTED

	body := bytes.NewBufferString(`{"adopt_display_name":false,"adopt_avatar":falseREDACTED`)
	recorder := httptest.NewRecorder()
	ginCtx, _ := gin.CreateTestContext(recorder)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/oauth/pending/exchange", body)
	req.Header.Set("Content-Type", "application/json")
	req.AddCookie(&http.Cookie{Name: oauthPendingSessionCookieName, Value: encodeCookieValue(session.SessionToken)REDACTED)
	req.AddCookie(&http.Cookie{Name: oauthPendingBrowserCookieName, Value: encodeCookieValue("invitation-required-browser-session-key")REDACTED)
	ginCtx.Request = req

	handler.ExchangePendingOAuthCompletion(ginCtx)

	require.Equal(t, http.StatusOK, recorder.Code)
	data := decodeJSONResponseData(t, recorder)
	require.Equal(t, "invitation_required", data["error"])
	require.Equal(t, true, data["adoption_required"])

	identityCount, err := client.AuthIdentity.Query().
		Where(
			authidentity.ProviderTypeEQ("linuxdo"),
			authidentity.ProviderKeyEQ("linuxdo"),
			authidentity.ProviderSubjectEQ("invitation-123"),
		).
		Count(ctx)
REDACTED
	require.Zero(t, identityCount)

	decision, err := client.IdentityAdoptionDecision.Query().
		Where(identityadoptiondecision.PendingAuthSessionIDEQ(session.ID)).
		Only(ctx)
REDACTED
	require.Nil(t, decision.IdentityID)
	require.False(t, decision.AdoptDisplayName)
	require.False(t, decision.AdoptAvatar)

	storedSession, err := client.PendingAuthSession.Query().
		Where(pendingauthsession.IDEQ(session.ID)).
		Only(ctx)
REDACTED
	require.Nil(t, storedSession.ConsumedAt)
REDACTED

func TestCreateOIDCOAuthAccountCreatesUserBindsIdentityAndConsumesSession(t *testing.T) {
	handler, client := newOAuthPendingFlowTestHandlerWithEmailVerification(t, false, "fresh@example.com", "246810")
	ctx := context.Background()

	session, err := client.PendingAuthSession.Create().
		SetSessionToken("create-account-session-token").
		SetIntent("login").
		SetProviderType("oidc").
		SetProviderKey("https://issuer.example").
		SetProviderSubject("oidc-create-123").
		SetBrowserSessionKey("create-account-browser-session-key").
		SetUpstreamIdentityClaims(map[string]any{
			"username":               "oidc_user",
			"suggested_display_name": "Fresh OIDC User",
			"suggested_avatar_url":   "https://cdn.example/fresh.png",
	REDACTED).
		SetRedirectTo("/profile").
		SetExpiresAt(time.Now().UTC().Add(10 * time.Minute)).
		Save(ctx)
REDACTED

	body := bytes.NewBufferString(`{"email":"fresh@example.com","verify_code":"246810","password":"secret-123","adopt_display_name":false,"adopt_avatar":falseREDACTED`)
	recorder := httptest.NewRecorder()
	ginCtx, _ := gin.CreateTestContext(recorder)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/oauth/oidc/create-account", body)
	req.Header.Set("Content-Type", "application/json")
	req.AddCookie(&http.Cookie{Name: oauthPendingSessionCookieName, Value: encodeCookieValue(session.SessionToken)REDACTED)
	req.AddCookie(&http.Cookie{Name: oauthPendingBrowserCookieName, Value: encodeCookieValue("create-account-browser-session-key")REDACTED)
	ginCtx.Request = req

	handler.CreateOIDCOAuthAccount(ginCtx)

	require.Equal(t, http.StatusOK, recorder.Code)

	var payload map[string]any
	require.NoError(t, json.Unmarshal(recorder.Body.Bytes(), &payload))
	require.NotEmpty(t, payload["access_token"])
	require.NotEmpty(t, payload["refresh_token"])
	require.Equal(t, "Bearer", payload["token_type"])

	createdUser, err := client.User.Query().Where(dbuser.EmailEQ("fresh@example.com")).Only(ctx)
REDACTED
	require.Equal(t, service.StatusActive, createdUser.Status)

	identity, err := client.AuthIdentity.Query().
		Where(
			authidentity.ProviderTypeEQ("oidc"),
			authidentity.ProviderKeyEQ("https://issuer.example"),
			authidentity.ProviderSubjectEQ("oidc-create-123"),
		).
		Only(ctx)
REDACTED
	require.Equal(t, createdUser.ID, identity.UserID)

	storedSession, err := client.PendingAuthSession.Get(ctx, session.ID)
REDACTED
	require.NotNil(t, storedSession.ConsumedAt)
REDACTED

func TestCreateOIDCOAuthAccountAppliesPromoCodeFromPendingSession(t *testing.T) {
	promoRepo := newOAuthPendingFlowPromoRepoStub("WELCOME2024", 25)
	emailCache := &oauthPendingFlowEmailCacheStub{
		verificationCodes: map[string]*service.VerificationCodeData{
			"promo@example.com": {
				Code:      "246810",
				CreatedAt: time.Now().UTC(),
				ExpiresAt: time.Now().UTC().Add(15 * time.Minute),
		REDACTED,
	REDACTED,
REDACTED
	handler, client := newOAuthPendingFlowTestHandlerWithDependencies(t, oauthPendingFlowTestHandlerOptions{
		emailVerifyEnabled: true,
		emailCache:         emailCache,
		promoRepo:          promoRepo,
		settingValues: map[string]string{
			service.SettingKeyPromoCodeEnabled: "true",
	REDACTED,
REDACTED)
	ctx := context.Background()

	session, err := client.PendingAuthSession.Create().
		SetSessionToken("promo-create-account-session-token").
		SetIntent(oauthIntentLogin).
		SetProviderType("oidc").
		SetProviderKey("https://issuer.example").
		SetProviderSubject("oidc-promo-123").
		SetBrowserSessionKey("promo-create-account-browser-key").
		SetUpstreamIdentityClaims(map[string]any{"username": "promo_user"REDACTED).
		SetLocalFlowState(map[string]any{oauthPromoCodeStateKey: "WELCOME2024"REDACTED).
		SetExpiresAt(time.Now().UTC().Add(10 * time.Minute)).
		Save(ctx)
REDACTED

	body := bytes.NewBufferString(`{"email":"promo@example.com","verify_code":"246810","password":"secret-123"REDACTED`)
	recorder := httptest.NewRecorder()
	ginCtx, _ := gin.CreateTestContext(recorder)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/oauth/oidc/create-account", body)
	req.Header.Set("Content-Type", "application/json")
	req.AddCookie(&http.Cookie{Name: oauthPendingSessionCookieName, Value: encodeCookieValue(session.SessionToken)REDACTED)
	req.AddCookie(&http.Cookie{Name: oauthPendingBrowserCookieName, Value: encodeCookieValue(session.BrowserSessionKey)REDACTED)
	ginCtx.Request = req

	handler.CreateOIDCOAuthAccount(ginCtx)

	require.Equal(t, http.StatusOK, recorder.Code)
	require.Equal(t, []string{"WELCOME2024"REDACTED, promoRepo.applyCalls)
	createdUser, err := client.User.Query().Where(dbuser.EmailEQ("promo@example.com")).Only(ctx)
REDACTED
	require.Equal(t, 25.0, createdUser.Balance)
	require.Len(t, promoRepo.usages, 1)
	require.Equal(t, createdUser.ID, promoRepo.usages[0].UserID)
REDACTED

func TestCreateOIDCOAuthAccountWithoutPromoCodeDoesNotApplyPromo(t *testing.T) {
	promoRepo := newOAuthPendingFlowPromoRepoStub("WELCOME2024", 25)
	emailCache := &oauthPendingFlowEmailCacheStub{
		verificationCodes: map[string]*service.VerificationCodeData{
			"no-promo@example.com": {
				Code:      "246810",
				CreatedAt: time.Now().UTC(),
				ExpiresAt: time.Now().UTC().Add(15 * time.Minute),
		REDACTED,
	REDACTED,
REDACTED
	handler, client := newOAuthPendingFlowTestHandlerWithDependencies(t, oauthPendingFlowTestHandlerOptions{
		emailVerifyEnabled: true,
		emailCache:         emailCache,
		promoRepo:          promoRepo,
		settingValues: map[string]string{
			service.SettingKeyPromoCodeEnabled: "true",
	REDACTED,
REDACTED)
	ctx := context.Background()

	session, err := client.PendingAuthSession.Create().
		SetSessionToken("no-promo-create-account-session-token").
		SetIntent(oauthIntentLogin).
		SetProviderType("oidc").
		SetProviderKey("https://issuer.example").
		SetProviderSubject("oidc-no-promo-123").
		SetBrowserSessionKey("no-promo-create-account-browser-key").
		SetUpstreamIdentityClaims(map[string]any{"username": "no_promo_user"REDACTED).
		SetExpiresAt(time.Now().UTC().Add(10 * time.Minute)).
		Save(ctx)
REDACTED

	body := bytes.NewBufferString(`{"email":"no-promo@example.com","verify_code":"246810","password":"secret-123"REDACTED`)
	recorder := httptest.NewRecorder()
	ginCtx, _ := gin.CreateTestContext(recorder)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/oauth/oidc/create-account", body)
	req.Header.Set("Content-Type", "application/json")
	req.AddCookie(&http.Cookie{Name: oauthPendingSessionCookieName, Value: encodeCookieValue(session.SessionToken)REDACTED)
	req.AddCookie(&http.Cookie{Name: oauthPendingBrowserCookieName, Value: encodeCookieValue(session.BrowserSessionKey)REDACTED)
	ginCtx.Request = req

	handler.CreateOIDCOAuthAccount(ginCtx)

	require.Equal(t, http.StatusOK, recorder.Code)
	require.Empty(t, promoRepo.applyCalls)
	createdUser, err := client.User.Query().Where(dbuser.EmailEQ("no-promo@example.com")).Only(ctx)
REDACTED
	require.Zero(t, createdUser.Balance)
REDACTED

func TestCreateOIDCOAuthAccountDoesNotApplyPromoWhenDisabled(t *testing.T) {
	promoRepo := newOAuthPendingFlowPromoRepoStub("WELCOME2024", 25)
	emailCache := &oauthPendingFlowEmailCacheStub{
		verificationCodes: map[string]*service.VerificationCodeData{
			"promo-disabled@example.com": {
				Code:      "246810",
				CreatedAt: time.Now().UTC(),
				ExpiresAt: time.Now().UTC().Add(15 * time.Minute),
		REDACTED,
	REDACTED,
REDACTED
	handler, client := newOAuthPendingFlowTestHandlerWithDependencies(t, oauthPendingFlowTestHandlerOptions{
		emailVerifyEnabled: true,
		emailCache:         emailCache,
		promoRepo:          promoRepo,
		settingValues: map[string]string{
			service.SettingKeyPromoCodeEnabled: "false",
	REDACTED,
REDACTED)
	ctx := context.Background()

	session, err := client.PendingAuthSession.Create().
		SetSessionToken("promo-disabled-session-token").
		SetIntent(oauthIntentLogin).
		SetProviderType("oidc").
		SetProviderKey("https://issuer.example").
		SetProviderSubject("oidc-promo-disabled-123").
		SetBrowserSessionKey("promo-disabled-browser-key").
		SetUpstreamIdentityClaims(map[string]any{"username": "promo_disabled_user"REDACTED).
		SetLocalFlowState(map[string]any{oauthPromoCodeStateKey: "WELCOME2024"REDACTED).
		SetExpiresAt(time.Now().UTC().Add(10 * time.Minute)).
		Save(ctx)
REDACTED

	body := bytes.NewBufferString(`{"email":"promo-disabled@example.com","verify_code":"246810","password":"secret-123"REDACTED`)
	recorder := httptest.NewRecorder()
	ginCtx, _ := gin.CreateTestContext(recorder)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/oauth/oidc/create-account", body)
	req.Header.Set("Content-Type", "application/json")
	req.AddCookie(&http.Cookie{Name: oauthPendingSessionCookieName, Value: encodeCookieValue(session.SessionToken)REDACTED)
	req.AddCookie(&http.Cookie{Name: oauthPendingBrowserCookieName, Value: encodeCookieValue(session.BrowserSessionKey)REDACTED)
	ginCtx.Request = req

	handler.CreateOIDCOAuthAccount(ginCtx)

	require.Equal(t, http.StatusOK, recorder.Code)
	require.Empty(t, promoRepo.applyCalls)
	createdUser, err := client.User.Query().Where(dbuser.EmailEQ("promo-disabled@example.com")).Only(ctx)
REDACTED
	require.Zero(t, createdUser.Balance)
REDACTED

func TestOAuthExistingUserLoginDoesNotApplyPromoCode(t *testing.T) {
	promoRepo := newOAuthPendingFlowPromoRepoStub("WELCOME2024", 25)
	handler, client := newOAuthPendingFlowTestHandlerWithDependencies(t, oauthPendingFlowTestHandlerOptions{
		promoRepo: promoRepo,
		settingValues: map[string]string{
			service.SettingKeyPromoCodeEnabled: "true",
	REDACTED,
REDACTED)
	ctx := context.Background()

	existingUser, err := client.User.Create().
		SetEmail("existing-promo@example.com").
		SetUsername("existing").
		SetPasswordHash("hash").
		SetRole(service.RoleUser).
		SetStatus(service.StatusActive).
		SetBalance(7).
		Save(ctx)
REDACTED

	_, loggedInUser, err := handler.authService.LoginOrRegisterOAuthWithTokenPairAndPromoCode(
		ctx,
		existingUser.Email,
		existingUser.Username,
		"",
		"",
		"WELCOME2024",
		"oidc",
	)

REDACTED
	require.Equal(t, existingUser.ID, loggedInUser.ID)
	require.Empty(t, promoRepo.applyCalls)
	reloadedUser, err := client.User.Get(ctx, existingUser.ID)
REDACTED
	require.Equal(t, 7.0, reloadedUser.Balance)
REDACTED

func TestCreateOIDCOAuthAccountExistingEmailReturnsChoicePendingSessionState(t *testing.T) {
	handler, client := newOAuthPendingFlowTestHandlerWithEmailVerification(t, false, "owner@example.com", "135790")
	ctx := context.Background()

	existingUser, err := client.User.Create().
		SetEmail("owner@example.com").
		SetUsername("owner-user").
		SetPasswordHash("hash").
		SetRole(service.RoleUser).
		SetStatus(service.StatusActive).
		Save(ctx)
REDACTED

	session, err := client.PendingAuthSession.Create().
		SetSessionToken("existing-email-session-token").
		SetIntent("login").
		SetProviderType("oidc").
		SetProviderKey("https://issuer.example").
		SetProviderSubject("oidc-existing-123").
		SetBrowserSessionKey("existing-email-browser-session-key").
		SetUpstreamIdentityClaims(map[string]any{
			"username":               "oidc_user",
			"suggested_display_name": "Existing OIDC User",
			"suggested_avatar_url":   "https://cdn.example/existing.png",
	REDACTED).
		SetRedirectTo("/dashboard").
		SetExpiresAt(time.Now().UTC().Add(10 * time.Minute)).
		Save(ctx)
REDACTED

	body := bytes.NewBufferString(`{"email":"owner@example.com","verify_code":"135790","password":"secret-123"REDACTED`)
	recorder := httptest.NewRecorder()
	ginCtx, _ := gin.CreateTestContext(recorder)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/oauth/oidc/create-account", body)
	req.Header.Set("Content-Type", "application/json")
	req.AddCookie(&http.Cookie{Name: oauthPendingSessionCookieName, Value: encodeCookieValue(session.SessionToken)REDACTED)
	req.AddCookie(&http.Cookie{Name: oauthPendingBrowserCookieName, Value: encodeCookieValue("existing-email-browser-session-key")REDACTED)
	ginCtx.Request = req

	handler.CreateOIDCOAuthAccount(ginCtx)

	require.Equal(t, http.StatusOK, recorder.Code)

	var payload map[string]any
	require.NoError(t, json.Unmarshal(recorder.Body.Bytes(), &payload))
	require.Equal(t, "pending_session", payload["auth_result"])
	require.Equal(t, oauthIntentLogin, payload["intent"])
	require.Equal(t, "oidc", payload["provider"])
	require.Equal(t, "/dashboard", payload["redirect"])
	require.Equal(t, true, payload["adoption_required"])
	require.Equal(t, oauthPendingChoiceStep, payload["step"])
	require.Equal(t, "owner@example.com", payload["email"])
	require.Equal(t, "Existing OIDC User", payload["suggested_display_name"])
	require.Equal(t, "https://cdn.example/existing.png", payload["suggested_avatar_url"])

	storedSession, err := client.PendingAuthSession.Get(ctx, session.ID)
REDACTED
	require.Equal(t, oauthIntentLogin, storedSession.Intent)
	require.NotNil(t, storedSession.TargetUserID)
	require.Equal(t, existingUser.ID, *storedSession.TargetUserID)
	require.Equal(t, "owner@example.com", storedSession.ResolvedEmail)
	require.Nil(t, storedSession.ConsumedAt)

	identityCount, err := client.AuthIdentity.Query().
		Where(
			authidentity.ProviderTypeEQ("oidc"),
			authidentity.ProviderKeyEQ("https://issuer.example"),
			authidentity.ProviderSubjectEQ("oidc-existing-123"),
		).
		Count(ctx)
REDACTED
	require.Zero(t, identityCount)
REDACTED

func TestCreateOIDCOAuthAccountExistingEmailNormalizesLegacySpacingAndCase(t *testing.T) {
	handler, client := newOAuthPendingFlowTestHandlerWithEmailVerification(t, false, "owner@example.com", "135790")
	ctx := context.Background()

	existingUser, err := client.User.Create().
		SetEmail(" Owner@Example.com ").
		SetUsername("owner-user").
		SetPasswordHash("hash").
		SetRole(service.RoleUser).
		SetStatus(service.StatusActive).
		Save(ctx)
REDACTED

	session, err := client.PendingAuthSession.Create().
		SetSessionToken("existing-email-normalized-session-token").
		SetIntent("login").
		SetProviderType("oidc").
		SetProviderKey("https://issuer.example").
		SetProviderSubject("oidc-existing-normalized-123").
		SetBrowserSessionKey("existing-email-normalized-browser-session-key").
		SetUpstreamIdentityClaims(map[string]any{
			"username":               "oidc_user",
			"suggested_display_name": "Existing OIDC User",
			"suggested_avatar_url":   "https://cdn.example/existing.png",
	REDACTED).
		SetRedirectTo("/dashboard").
		SetExpiresAt(time.Now().UTC().Add(10 * time.Minute)).
		Save(ctx)
REDACTED

	body := bytes.NewBufferString(`{"email":"owner@example.com","verify_code":"135790","password":"secret-123"REDACTED`)
	recorder := httptest.NewRecorder()
	ginCtx, _ := gin.CreateTestContext(recorder)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/oauth/oidc/create-account", body)
	req.Header.Set("Content-Type", "application/json")
	req.AddCookie(&http.Cookie{Name: oauthPendingSessionCookieName, Value: encodeCookieValue(session.SessionToken)REDACTED)
	req.AddCookie(&http.Cookie{Name: oauthPendingBrowserCookieName, Value: encodeCookieValue("existing-email-normalized-browser-session-key")REDACTED)
	ginCtx.Request = req

	handler.CreateOIDCOAuthAccount(ginCtx)

	require.Equal(t, http.StatusOK, recorder.Code)

	var payload map[string]any
	require.NoError(t, json.Unmarshal(recorder.Body.Bytes(), &payload))
	require.Equal(t, oauthIntentLogin, payload["intent"])
	require.Equal(t, oauthPendingChoiceStep, payload["step"])

	storedSession, err := client.PendingAuthSession.Get(ctx, session.ID)
REDACTED
	require.NotNil(t, storedSession.TargetUserID)
	require.Equal(t, existingUser.ID, *storedSession.TargetUserID)
	require.Equal(t, "owner@example.com", storedSession.ResolvedEmail)
REDACTED

func TestCreateOIDCOAuthAccountRejectsEmailOutsideRegistrationSuffixWhitelist(t *testing.T) {
	handler, client := newOAuthPendingFlowTestHandlerWithDependencies(t, oauthPendingFlowTestHandlerOptions{
		emailVerifyEnabled: true,
		emailCache: &oauthPendingFlowEmailCacheStub{
			verificationCodes: map[string]*service.VerificationCodeData{
				"foo@gmail.com": {
					Code:      "135790",
					CreatedAt: time.Now().UTC(),
					ExpiresAt: time.Now().UTC().Add(15 * time.Minute),
			REDACTED,
		REDACTED,
	REDACTED,
		settingValues: map[string]string{
			service.SettingKeyRegistrationEmailSuffixWhitelist: `["@qq.com"]`,
	REDACTED,
REDACTED)
	ctx := context.Background()

	session, err := client.PendingAuthSession.Create().
		SetSessionToken("suffix-whitelist-session-token").
		SetIntent("login").
		SetProviderType("oidc").
		SetProviderKey("https://issuer.example").
		SetProviderSubject("oidc-suffix-whitelist-123").
		SetBrowserSessionKey("suffix-whitelist-browser-session-key").
		SetUpstreamIdentityClaims(map[string]any{
			"username": "oidc_user",
	REDACTED).
		SetExpiresAt(time.Now().UTC().Add(10 * time.Minute)).
		Save(ctx)
REDACTED

	body := bytes.NewBufferString(`{"email":"foo@gmail.com","verify_code":"135790","password":"secret-123"REDACTED`)
	recorder := httptest.NewRecorder()
	ginCtx, _ := gin.CreateTestContext(recorder)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/oauth/oidc/create-account", body)
	req.Header.Set("Content-Type", "application/json")
	req.AddCookie(&http.Cookie{Name: oauthPendingSessionCookieName, Value: encodeCookieValue(session.SessionToken)REDACTED)
	req.AddCookie(&http.Cookie{Name: oauthPendingBrowserCookieName, Value: encodeCookieValue("suffix-whitelist-browser-session-key")REDACTED)
	ginCtx.Request = req

	handler.CreateOIDCOAuthAccount(ginCtx)

	require.Equal(t, http.StatusBadRequest, recorder.Code)
	payload := decodeJSONBody(t, recorder)
	require.Equal(t, "EMAIL_SUFFIX_NOT_ALLOWED", payload["reason"])

	count, err := client.User.Query().Where(dbuser.EmailEQ("foo@gmail.com")).Count(ctx)
REDACTED
	require.Zero(t, count)
REDACTED

func TestSendPendingOAuthVerifyCodeExistingEmailReturnsBindLoginState(t *testing.T) {
	handler, client := newOAuthPendingFlowTestHandlerWithEmailVerification(t, false, "owner@example.com", "135790")
	ctx := context.Background()

	existingUser, err := client.User.Create().
		SetEmail("owner@example.com").
		SetUsername("owner-user").
		SetPasswordHash("hash").
		SetRole(service.RoleUser).
		SetStatus(service.StatusActive).
		Save(ctx)
REDACTED

	session, err := client.PendingAuthSession.Create().
		SetSessionToken("existing-email-send-code-session-token").
		SetIntent("login").
		SetProviderType("oidc").
		SetProviderKey("https://issuer.example").
		SetProviderSubject("oidc-existing-send-code-123").
		SetBrowserSessionKey("existing-email-send-code-browser-session-key").
		SetLocalFlowState(map[string]any{
			oauthCompletionResponseKey: map[string]any{
				"step": "email_required",
		REDACTED,
	REDACTED).
		SetRedirectTo("/dashboard").
		SetExpiresAt(time.Now().UTC().Add(10 * time.Minute)).
		Save(ctx)
REDACTED

	body := bytes.NewBufferString(`{"email":"owner@example.com"REDACTED`)
	recorder := httptest.NewRecorder()
	ginCtx, _ := gin.CreateTestContext(recorder)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/oauth/pending/send-verify-code", body)
	req.Header.Set("Content-Type", "application/json")
	req.AddCookie(&http.Cookie{Name: oauthPendingSessionCookieName, Value: encodeCookieValue(session.SessionToken)REDACTED)
	req.AddCookie(&http.Cookie{Name: oauthPendingBrowserCookieName, Value: encodeCookieValue("existing-email-send-code-browser-session-key")REDACTED)
	ginCtx.Request = req

	handler.SendPendingOAuthVerifyCode(ginCtx)

	require.Equal(t, http.StatusOK, recorder.Code)

	var payload map[string]any
	require.NoError(t, json.Unmarshal(recorder.Body.Bytes(), &payload))
	require.Equal(t, "pending_session", payload["auth_result"])
	require.Equal(t, oauthPendingChoiceStep, payload["step"])
	require.Equal(t, "owner@example.com", payload["email"])

	storedSession, err := client.PendingAuthSession.Get(ctx, session.ID)
REDACTED
	require.Equal(t, oauthIntentLogin, storedSession.Intent)
	require.NotNil(t, storedSession.TargetUserID)
	require.Equal(t, existingUser.ID, *storedSession.TargetUserID)
	require.Equal(t, "owner@example.com", storedSession.ResolvedEmail)
REDACTED

func TestCreateOIDCOAuthAccountBlocksBackendModeBeforeCreatingUser(t *testing.T) {
	handler, client := newOAuthPendingFlowTestHandlerWithDependencies(t, oauthPendingFlowTestHandlerOptions{
		emailVerifyEnabled: true,
		emailCache: &oauthPendingFlowEmailCacheStub{
			verificationCodes: map[string]*service.VerificationCodeData{
				"fresh@example.com": {
					Code:      "246810",
					CreatedAt: time.Now().UTC(),
					ExpiresAt: time.Now().UTC().Add(15 * time.Minute),
			REDACTED,
		REDACTED,
	REDACTED,
		settingValues: map[string]string{
			service.SettingKeyBackendModeEnabled: "true",
	REDACTED,
REDACTED)
	ctx := context.Background()

	session, err := client.PendingAuthSession.Create().
		SetSessionToken("create-account-backend-mode-session-token").
		SetIntent("login").
		SetProviderType("oidc").
		SetProviderKey("https://issuer.example").
		SetProviderSubject("oidc-create-backend-mode-123").
		SetBrowserSessionKey("create-account-backend-mode-browser-session-key").
		SetUpstreamIdentityClaims(map[string]any{
			"username": "oidc_user",
	REDACTED).
		SetExpiresAt(time.Now().UTC().Add(10 * time.Minute)).
		Save(ctx)
REDACTED

	body := bytes.NewBufferString(`{"email":"fresh@example.com","verify_code":"246810","password":"secret-123"REDACTED`)
	recorder := httptest.NewRecorder()
	ginCtx, _ := gin.CreateTestContext(recorder)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/oauth/oidc/create-account", body)
	req.Header.Set("Content-Type", "application/json")
	req.AddCookie(&http.Cookie{Name: oauthPendingSessionCookieName, Value: encodeCookieValue(session.SessionToken)REDACTED)
	req.AddCookie(&http.Cookie{Name: oauthPendingBrowserCookieName, Value: encodeCookieValue("create-account-backend-mode-browser-session-key")REDACTED)
	ginCtx.Request = req

	handler.CreateOIDCOAuthAccount(ginCtx)

	require.Equal(t, http.StatusForbidden, recorder.Code)

	userCount, err := client.User.Query().Where(dbuser.EmailEQ("fresh@example.com")).Count(ctx)
REDACTED
	require.Zero(t, userCount)

	storedSession, err := client.PendingAuthSession.Get(ctx, session.ID)
REDACTED
	require.Nil(t, storedSession.ConsumedAt)
REDACTED

func TestLogoutClearsPendingOAuthAndBindCookies(t *testing.T) {
	handler, _ := newOAuthPendingFlowTestHandler(t, false)

	recorder := httptest.NewRecorder()
	ginCtx, _ := gin.CreateTestContext(recorder)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/logout", bytes.NewBufferString(`{REDACTED`))
	req.Header.Set("Content-Type", "application/json")
	req.AddCookie(&http.Cookie{Name: oauthPendingSessionCookieName, Value: encodeCookieValue("pending-session-token")REDACTED)
	req.AddCookie(&http.Cookie{Name: oauthPendingBrowserCookieName, Value: encodeCookieValue("pending-browser-key")REDACTED)
	req.AddCookie(&http.Cookie{Name: oauthBindAccessTokenCookieName, Value: "bind-token"REDACTED)
	ginCtx.Request = req

	handler.Logout(ginCtx)

	require.Equal(t, http.StatusOK, recorder.Code)
	require.Equal(t, -1, findCookie(recorder.Result().Cookies(), oauthPendingSessionCookieName).MaxAge)
	require.Equal(t, -1, findCookie(recorder.Result().Cookies(), oauthPendingBrowserCookieName).MaxAge)
	require.Equal(t, -1, findCookie(recorder.Result().Cookies(), oauthBindAccessTokenCookieName).MaxAge)
REDACTED

func TestCreateOIDCOAuthAccountRollsBackCreatedUserWhenBindingFails(t *testing.T) {
	handler, client := newOAuthPendingFlowTestHandlerWithEmailVerification(t, true, "fresh@example.com", "246810")
	ctx := context.Background()

	conflictOwner, err := client.User.Create().
		SetEmail("owner@example.com").
		SetUsername("owner-user").
		SetPasswordHash("hash").
		SetRole(service.RoleUser).
		SetStatus(service.StatusActive).
		Save(ctx)
REDACTED

	_, err = client.AuthIdentity.Create().
		SetUserID(conflictOwner.ID).
		SetProviderType("oidc").
		SetProviderKey("https://issuer.example").
		SetProviderSubject("oidc-conflict-123").
		SetMetadata(map[string]any{
			"username": "owner-user",
	REDACTED).
		Save(ctx)
REDACTED

	invitation, err := client.RedeemCode.Create().
		SetCode("INVITE123").
		SetType(service.RedeemTypeInvitation).
		SetStatus(service.StatusUnused).
		SetValue(0).
		Save(ctx)
REDACTED

	session, err := client.PendingAuthSession.Create().
		SetSessionToken("create-account-conflict-session-token").
		SetIntent("login").
		SetProviderType("oidc").
		SetProviderKey("https://issuer.example").
		SetProviderSubject("oidc-conflict-123").
		SetBrowserSessionKey("create-account-conflict-browser-session-key").
		SetUpstreamIdentityClaims(map[string]any{
			"username": "oidc_user",
	REDACTED).
		SetRedirectTo("/profile").
		SetExpiresAt(time.Now().UTC().Add(10 * time.Minute)).
		Save(ctx)
REDACTED

	body := bytes.NewBufferString(`{"email":"fresh@example.com","verify_code":"246810","password":"secret-123","invitation_code":"INVITE123"REDACTED`)
	recorder := httptest.NewRecorder()
	ginCtx, _ := gin.CreateTestContext(recorder)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/oauth/oidc/create-account", body)
	req.Header.Set("Content-Type", "application/json")
	req.AddCookie(&http.Cookie{Name: oauthPendingSessionCookieName, Value: encodeCookieValue(session.SessionToken)REDACTED)
	req.AddCookie(&http.Cookie{Name: oauthPendingBrowserCookieName, Value: encodeCookieValue("create-account-conflict-browser-session-key")REDACTED)
	ginCtx.Request = req

	handler.CreateOIDCOAuthAccount(ginCtx)

	require.Equal(t, http.StatusConflict, recorder.Code)

	userCount, err := client.User.Query().Where(dbuser.EmailEQ("fresh@example.com")).Count(ctx)
REDACTED
	require.Zero(t, userCount)

	storedInvitation, err := client.RedeemCode.Get(ctx, invitation.ID)
REDACTED
	require.Equal(t, service.StatusUnused, storedInvitation.Status)
	require.Nil(t, storedInvitation.UsedBy)
	require.Nil(t, storedInvitation.UsedAt)

	storedSession, err := client.PendingAuthSession.Get(ctx, session.ID)
REDACTED
	require.Nil(t, storedSession.ConsumedAt)
REDACTED

func TestCreateOIDCOAuthAccountRollsBackPostBindFailureBeforeIdentityCanCommit(t *testing.T) {
	handler, client := newOAuthPendingFlowTestHandlerWithDependencies(t, oauthPendingFlowTestHandlerOptions{
		emailVerifyEnabled: true,
		emailCache: &oauthPendingFlowEmailCacheStub{
			verificationCodes: map[string]*service.VerificationCodeData{
				"fresh@example.com": {
					Code:      "246810",
					CreatedAt: time.Now().UTC(),
					ExpiresAt: time.Now().UTC().Add(15 * time.Minute),
			REDACTED,
		REDACTED,
	REDACTED,
		userRepoOptions: oauthPendingFlowUserRepoOptions{
			rejectDeleteWhileAuthIdentityExists: true,
	REDACTED,
REDACTED)
	ctx := context.Background()

	session, err := client.PendingAuthSession.Create().
		SetSessionToken("create-account-finalize-failure-session-token").
		SetIntent("login").
		SetProviderType("oidc").
		SetProviderKey("https://issuer.example").
		SetProviderSubject("oidc-finalize-failure-123").
		SetBrowserSessionKey("create-account-finalize-failure-browser-session-key").
		SetUpstreamIdentityClaims(map[string]any{
			"username": "oidc_user",
	REDACTED).
		SetRedirectTo("/profile").
		SetExpiresAt(time.Now().UTC().Add(10 * time.Minute)).
		Save(ctx)
REDACTED

	pendingOAuthCreateAccountPreCommitHook = func(context.Context, *dbent.PendingAuthSession) error {
		return errors.New("forced post-bind failure")
REDACTED
	t.Cleanup(func() {
		pendingOAuthCreateAccountPreCommitHook = nil
REDACTED)

	body := bytes.NewBufferString(`{"email":"fresh@example.com","verify_code":"246810","password":"secret-123"REDACTED`)
	recorder := httptest.NewRecorder()
	ginCtx, _ := gin.CreateTestContext(recorder)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/oauth/oidc/create-account", body)
	req.Header.Set("Content-Type", "application/json")
	req.AddCookie(&http.Cookie{Name: oauthPendingSessionCookieName, Value: encodeCookieValue(session.SessionToken)REDACTED)
	req.AddCookie(&http.Cookie{Name: oauthPendingBrowserCookieName, Value: encodeCookieValue("create-account-finalize-failure-browser-session-key")REDACTED)
	ginCtx.Request = req

	handler.CreateOIDCOAuthAccount(ginCtx)

	require.Equal(t, http.StatusInternalServerError, recorder.Code)

	userCount, err := client.User.Query().Where(dbuser.EmailEQ("fresh@example.com")).Count(ctx)
REDACTED
	require.Zero(t, userCount)

	identityCount, err := client.AuthIdentity.Query().
		Where(
			authidentity.ProviderTypeEQ("oidc"),
			authidentity.ProviderKeyEQ("https://issuer.example"),
			authidentity.ProviderSubjectEQ("oidc-finalize-failure-123"),
		).
		Count(ctx)
REDACTED
	require.Zero(t, identityCount)

	storedSession, err := client.PendingAuthSession.Get(ctx, session.ID)
REDACTED
	require.Nil(t, storedSession.ConsumedAt)
REDACTED

func TestBindOIDCOAuthLoginBindsExistingUserAndConsumesSession(t *testing.T) {
	handler, client := newOAuthPendingFlowTestHandler(t, false)
	ctx := context.Background()

	passwordHash, err := handler.authService.HashPassword("secret-123")
REDACTED

	existingUser, err := client.User.Create().
		SetEmail("owner@example.com").
		SetUsername("owner-user").
		SetPasswordHash(passwordHash).
		SetRole(service.RoleUser).
		SetStatus(service.StatusActive).
		Save(ctx)
REDACTED

	session, err := client.PendingAuthSession.Create().
		SetSessionToken("bind-login-session-token").
		SetIntent("adopt_existing_user_by_email").
		SetProviderType("oidc").
		SetProviderKey("https://issuer.example").
		SetProviderSubject("oidc-bind-123").
		SetTargetUserID(existingUser.ID).
		SetResolvedEmail(existingUser.Email).
		SetBrowserSessionKey("bind-login-browser-session-key").
		SetUpstreamIdentityClaims(map[string]any{
			"username":               "oidc_user",
			"suggested_display_name": "Bound OIDC User",
			"suggested_avatar_url":   "https://cdn.example/bound.png",
	REDACTED).
		SetRedirectTo("/profile").
		SetExpiresAt(time.Now().UTC().Add(10 * time.Minute)).
		Save(ctx)
REDACTED

	body := bytes.NewBufferString(`{"email":"owner@example.com","password":"secret-123","adopt_display_name":false,"adopt_avatar":falseREDACTED`)
	recorder := httptest.NewRecorder()
	ginCtx, _ := gin.CreateTestContext(recorder)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/oauth/oidc/bind-login", body)
	req.Header.Set("Content-Type", "application/json")
	req.AddCookie(&http.Cookie{Name: oauthPendingSessionCookieName, Value: encodeCookieValue(session.SessionToken)REDACTED)
	req.AddCookie(&http.Cookie{Name: oauthPendingBrowserCookieName, Value: encodeCookieValue("bind-login-browser-session-key")REDACTED)
	ginCtx.Request = req

	handler.BindOIDCOAuthLogin(ginCtx)

	require.Equal(t, http.StatusOK, recorder.Code)

	var payload map[string]any
	require.NoError(t, json.Unmarshal(recorder.Body.Bytes(), &payload))
	require.NotEmpty(t, payload["access_token"])
	require.NotEmpty(t, payload["refresh_token"])
	require.Equal(t, "Bearer", payload["token_type"])

	identity, err := client.AuthIdentity.Query().
		Where(
			authidentity.ProviderTypeEQ("oidc"),
			authidentity.ProviderKeyEQ("https://issuer.example"),
			authidentity.ProviderSubjectEQ("oidc-bind-123"),
		).
		Only(ctx)
REDACTED
	require.Equal(t, existingUser.ID, identity.UserID)

	storedSession, err := client.PendingAuthSession.Get(ctx, session.ID)
REDACTED
	require.NotNil(t, storedSession.ConsumedAt)
REDACTED

func TestBindOIDCOAuthLoginBlocksBackendModeBeforeTokenIssue(t *testing.T) {
	handler, client := newOAuthPendingFlowTestHandlerWithDependencies(t, oauthPendingFlowTestHandlerOptions{
		settingValues: map[string]string{
			service.SettingKeyBackendModeEnabled: "true",
	REDACTED,
REDACTED)
	ctx := context.Background()

	passwordHash, err := handler.authService.HashPassword("secret-123")
REDACTED

	existingUser, err := client.User.Create().
		SetEmail("owner@example.com").
		SetUsername("owner-user").
		SetPasswordHash(passwordHash).
		SetRole(service.RoleUser).
		SetStatus(service.StatusActive).
		Save(ctx)
REDACTED

	session, err := client.PendingAuthSession.Create().
		SetSessionToken("bind-login-backend-mode-session-token").
		SetIntent("adopt_existing_user_by_email").
		SetProviderType("oidc").
		SetProviderKey("https://issuer.example").
		SetProviderSubject("oidc-bind-backend-mode-123").
		SetTargetUserID(existingUser.ID).
		SetResolvedEmail(existingUser.Email).
		SetBrowserSessionKey("bind-login-backend-mode-browser-session-key").
		SetUpstreamIdentityClaims(map[string]any{
			"username": "oidc_user",
	REDACTED).
		SetExpiresAt(time.Now().UTC().Add(10 * time.Minute)).
		Save(ctx)
REDACTED

	body := bytes.NewBufferString(`{"email":"owner@example.com","password":"secret-123"REDACTED`)
	recorder := httptest.NewRecorder()
	ginCtx, _ := gin.CreateTestContext(recorder)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/oauth/oidc/bind-login", body)
	req.Header.Set("Content-Type", "application/json")
	req.AddCookie(&http.Cookie{Name: oauthPendingSessionCookieName, Value: encodeCookieValue(session.SessionToken)REDACTED)
	req.AddCookie(&http.Cookie{Name: oauthPendingBrowserCookieName, Value: encodeCookieValue("bind-login-backend-mode-browser-session-key")REDACTED)
	ginCtx.Request = req

	handler.BindOIDCOAuthLogin(ginCtx)

	require.Equal(t, http.StatusForbidden, recorder.Code)

	identityCount, err := client.AuthIdentity.Query().
		Where(
			authidentity.ProviderTypeEQ("oidc"),
			authidentity.ProviderKeyEQ("https://issuer.example"),
			authidentity.ProviderSubjectEQ("oidc-bind-backend-mode-123"),
		).
		Count(ctx)
REDACTED
	require.Zero(t, identityCount)

	storedSession, err := client.PendingAuthSession.Get(ctx, session.ID)
REDACTED
	require.Nil(t, storedSession.ConsumedAt)
REDACTED

func TestBindOIDCOAuthLoginRejectsInvalidPasswordWithoutConsumingSession(t *testing.T) {
	handler, client := newOAuthPendingFlowTestHandler(t, false)
	ctx := context.Background()

	passwordHash, err := handler.authService.HashPassword("secret-123")
REDACTED

	existingUser, err := client.User.Create().
		SetEmail("owner@example.com").
		SetUsername("owner-user").
		SetPasswordHash(passwordHash).
		SetRole(service.RoleUser).
		SetStatus(service.StatusActive).
		Save(ctx)
REDACTED

	session, err := client.PendingAuthSession.Create().
		SetSessionToken("bind-login-invalid-password-session-token").
		SetIntent("adopt_existing_user_by_email").
		SetProviderType("oidc").
		SetProviderKey("https://issuer.example").
		SetProviderSubject("oidc-bind-invalid-123").
		SetTargetUserID(existingUser.ID).
		SetResolvedEmail(existingUser.Email).
		SetBrowserSessionKey("bind-login-invalid-password-browser-session-key").
		SetUpstreamIdentityClaims(map[string]any{
			"username":               "oidc_user",
			"suggested_display_name": "Bound OIDC User",
			"suggested_avatar_url":   "https://cdn.example/bound.png",
	REDACTED).
		SetExpiresAt(time.Now().UTC().Add(10 * time.Minute)).
		Save(ctx)
REDACTED

	body := bytes.NewBufferString(`{"email":"owner@example.com","password":"wrong-password"REDACTED`)
	recorder := httptest.NewRecorder()
	ginCtx, _ := gin.CreateTestContext(recorder)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/oauth/oidc/bind-login", body)
	req.Header.Set("Content-Type", "application/json")
	req.AddCookie(&http.Cookie{Name: oauthPendingSessionCookieName, Value: encodeCookieValue(session.SessionToken)REDACTED)
	req.AddCookie(&http.Cookie{Name: oauthPendingBrowserCookieName, Value: encodeCookieValue("bind-login-invalid-password-browser-session-key")REDACTED)
	ginCtx.Request = req

	handler.BindOIDCOAuthLogin(ginCtx)

	require.Equal(t, http.StatusUnauthorized, recorder.Code)
	payload := decodeJSONBody(t, recorder)
	require.Equal(t, "INVALID_CREDENTIALS", payload["reason"])

	identityCount, err := client.AuthIdentity.Query().
		Where(
			authidentity.ProviderTypeEQ("oidc"),
			authidentity.ProviderKeyEQ("https://issuer.example"),
			authidentity.ProviderSubjectEQ("oidc-bind-invalid-123"),
		).
		Count(ctx)
REDACTED
	require.Zero(t, identityCount)

	storedSession, err := client.PendingAuthSession.Get(ctx, session.ID)
REDACTED
	require.Nil(t, storedSession.ConsumedAt)
REDACTED

func TestBindOIDCOAuthLoginReclaimsIdentityOwnedBySoftDeletedUser(t *testing.T) {
	handler, client := newOAuthPendingFlowTestHandler(t, false)
	ctx := context.Background()

	oldOwnerHash, err := handler.authService.HashPassword("old-secret")
REDACTED
	oldOwner, err := client.User.Create().
		SetEmail("old-owner@example.com").
		SetUsername("old-owner").
		SetPasswordHash(oldOwnerHash).
		SetRole(service.RoleUser).
		SetStatus(service.StatusActive).
		Save(ctx)
REDACTED

	identity, err := client.AuthIdentity.Create().
		SetUserID(oldOwner.ID).
		SetProviderType("oidc").
		SetProviderKey("https://issuer.example").
		SetProviderSubject("oidc-bind-soft-deleted-123").
		SetMetadata(map[string]any{"username": "old-owner"REDACTED).
		Save(ctx)
REDACTED

	_, err = client.User.Delete().Where(dbuser.IDEQ(oldOwner.ID)).Exec(ctx)
REDACTED

	newOwnerHash, err := handler.authService.HashPassword("secret-123")
REDACTED
	newOwner, err := client.User.Create().
		SetEmail("owner@example.com").
		SetUsername("owner-user").
		SetPasswordHash(newOwnerHash).
		SetRole(service.RoleUser).
		SetStatus(service.StatusActive).
		Save(ctx)
REDACTED

	session, err := client.PendingAuthSession.Create().
		SetSessionToken("bind-login-soft-deleted-owner-session-token").
		SetIntent("adopt_existing_user_by_email").
		SetProviderType("oidc").
		SetProviderKey("https://issuer.example").
		SetProviderSubject("oidc-bind-soft-deleted-123").
		SetTargetUserID(newOwner.ID).
		SetResolvedEmail(newOwner.Email).
		SetBrowserSessionKey("bind-login-soft-deleted-owner-browser-session-key").
		SetUpstreamIdentityClaims(map[string]any{
			"username":               "oidc_user",
			"suggested_display_name": "Recovered OIDC User",
	REDACTED).
		SetRedirectTo("/profile").
		SetExpiresAt(time.Now().UTC().Add(10 * time.Minute)).
		Save(ctx)
REDACTED

	body := bytes.NewBufferString(`{"email":"owner@example.com","password":"secret-123","adopt_display_name":false,"adopt_avatar":falseREDACTED`)
	recorder := httptest.NewRecorder()
	ginCtx, _ := gin.CreateTestContext(recorder)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/oauth/oidc/bind-login", body)
	req.Header.Set("Content-Type", "application/json")
	req.AddCookie(&http.Cookie{Name: oauthPendingSessionCookieName, Value: encodeCookieValue(session.SessionToken)REDACTED)
	req.AddCookie(&http.Cookie{Name: oauthPendingBrowserCookieName, Value: encodeCookieValue("bind-login-soft-deleted-owner-browser-session-key")REDACTED)
	ginCtx.Request = req

	handler.BindOIDCOAuthLogin(ginCtx)

	require.Equal(t, http.StatusOK, recorder.Code)

	identity, err = client.AuthIdentity.Get(ctx, identity.ID)
REDACTED
	require.Equal(t, newOwner.ID, identity.UserID)
REDACTED

func TestBindOIDCOAuthLoginAppliesFirstBindGrantOnce(t *testing.T) {
	defaultSubAssigner := &oauthPendingFlowDefaultSubAssignerStub{REDACTED
	handler, client := newOAuthPendingFlowTestHandlerWithDependencies(t, oauthPendingFlowTestHandlerOptions{
		settingValues: map[string]string{
			service.SettingKeyAuthSourceDefaultOIDCBalance:          "12.5",
			service.SettingKeyAuthSourceDefaultOIDCConcurrency:      "3",
			service.SettingKeyAuthSourceDefaultOIDCSubscriptions:    `[{"group_id":101,"validity_days":30REDACTED]`,
			service.SettingKeyAuthSourceDefaultOIDCGrantOnFirstBind: "true",
	REDACTED,
		defaultSubAssigner: defaultSubAssigner,
REDACTED)
	ctx := context.Background()

	passwordHash, err := handler.authService.HashPassword("secret-123")
REDACTED

	existingUser, err := client.User.Create().
		SetEmail("owner@example.com").
		SetUsername("owner-user").
		SetPasswordHash(passwordHash).
		SetBalance(5).
		SetConcurrency(2).
		SetRole(service.RoleUser).
		SetStatus(service.StatusActive).
		Save(ctx)
REDACTED

	firstSession, err := client.PendingAuthSession.Create().
		SetSessionToken("first-bind-session-token").
		SetIntent("adopt_existing_user_by_email").
		SetProviderType("oidc").
		SetProviderKey("https://issuer.example").
		SetProviderSubject("oidc-bind-first-123").
		SetTargetUserID(existingUser.ID).
		SetResolvedEmail(existingUser.Email).
		SetBrowserSessionKey("first-bind-browser-session-key").
		SetUpstreamIdentityClaims(map[string]any{
			"suggested_display_name": "Bound OIDC User",
			"suggested_avatar_url":   "https://cdn.example/bound.png",
	REDACTED).
		SetRedirectTo("/profile").
		SetExpiresAt(time.Now().UTC().Add(10 * time.Minute)).
		Save(ctx)
REDACTED

	firstBody := bytes.NewBufferString(`{"email":"owner@example.com","password":"secret-123","adopt_display_name":false,"adopt_avatar":falseREDACTED`)
	firstRecorder := httptest.NewRecorder()
	firstGinCtx, _ := gin.CreateTestContext(firstRecorder)
	firstReq := httptest.NewRequest(http.MethodPost, "/api/v1/auth/oauth/oidc/bind-login", firstBody)
	firstReq.Header.Set("Content-Type", "application/json")
	firstReq.AddCookie(&http.Cookie{Name: oauthPendingSessionCookieName, Value: encodeCookieValue(firstSession.SessionToken)REDACTED)
	firstReq.AddCookie(&http.Cookie{Name: oauthPendingBrowserCookieName, Value: encodeCookieValue("first-bind-browser-session-key")REDACTED)
	firstGinCtx.Request = firstReq

	handler.BindOIDCOAuthLogin(firstGinCtx)

	require.Equal(t, http.StatusOK, firstRecorder.Code)

	storedUser, err := client.User.Get(ctx, existingUser.ID)
REDACTED
	require.Equal(t, 17.5, storedUser.Balance)
	require.Equal(t, 5, storedUser.Concurrency)
	require.Zero(t, storedUser.TotalRecharged)
	require.Len(t, defaultSubAssigner.calls, 1)
	require.Equal(t, int64(existingUser.ID), defaultSubAssigner.calls[0].UserID)
	require.Equal(t, int64(101), defaultSubAssigner.calls[0].GroupID)
	require.Equal(t, 30, defaultSubAssigner.calls[0].ValidityDays)
	require.Equal(t, 1, countProviderGrantRecords(t, client, existingUser.ID, "oidc", "first_bind"))

	secondSession, err := client.PendingAuthSession.Create().
		SetSessionToken("second-bind-session-token").
		SetIntent("adopt_existing_user_by_email").
		SetProviderType("oidc").
		SetProviderKey("https://issuer.example").
		SetProviderSubject("oidc-bind-second-456").
		SetTargetUserID(existingUser.ID).
		SetResolvedEmail(existingUser.Email).
		SetBrowserSessionKey("second-bind-browser-session-key").
		SetUpstreamIdentityClaims(map[string]any{
			"suggested_display_name": "Second OIDC User",
			"suggested_avatar_url":   "https://cdn.example/second.png",
	REDACTED).
		SetRedirectTo("/profile").
		SetExpiresAt(time.Now().UTC().Add(10 * time.Minute)).
		Save(ctx)
REDACTED

	secondBody := bytes.NewBufferString(`{"email":"owner@example.com","password":"secret-123","adopt_display_name":false,"adopt_avatar":falseREDACTED`)
	secondRecorder := httptest.NewRecorder()
	secondGinCtx, _ := gin.CreateTestContext(secondRecorder)
	secondReq := httptest.NewRequest(http.MethodPost, "/api/v1/auth/oauth/oidc/bind-login", secondBody)
	secondReq.Header.Set("Content-Type", "application/json")
	secondReq.AddCookie(&http.Cookie{Name: oauthPendingSessionCookieName, Value: encodeCookieValue(secondSession.SessionToken)REDACTED)
	secondReq.AddCookie(&http.Cookie{Name: oauthPendingBrowserCookieName, Value: encodeCookieValue("second-bind-browser-session-key")REDACTED)
	secondGinCtx.Request = secondReq

	handler.BindOIDCOAuthLogin(secondGinCtx)

	require.Equal(t, http.StatusOK, secondRecorder.Code)

	storedUser, err = client.User.Get(ctx, existingUser.ID)
REDACTED
	require.Equal(t, 17.5, storedUser.Balance)
	require.Equal(t, 5, storedUser.Concurrency)
	require.Zero(t, storedUser.TotalRecharged)
	require.Len(t, defaultSubAssigner.calls, 1)
	require.Equal(t, 1, countProviderGrantRecords(t, client, existingUser.ID, "oidc", "first_bind"))
REDACTED

func TestResolvePendingOAuthTargetUserIDNormalizesLegacySpacingAndCase(t *testing.T) {
	handler, client := newOAuthPendingFlowTestHandler(t, false)
	_ = handler
	ctx := context.Background()

	existingUser, err := client.User.Create().
		SetEmail(" Owner@Example.com ").
		SetUsername("owner-user").
		SetPasswordHash("hash").
		SetRole(service.RoleUser).
		SetStatus(service.StatusActive).
		Save(ctx)
REDACTED

	session, err := client.PendingAuthSession.Create().
		SetSessionToken("resolve-target-session-token").
		SetIntent("login").
		SetProviderType("oidc").
		SetProviderKey("https://issuer.example").
		SetProviderSubject("oidc-target-123").
		SetResolvedEmail("owner@example.com").
		SetBrowserSessionKey("resolve-target-browser-session-key").
		SetExpiresAt(time.Now().UTC().Add(10 * time.Minute)).
		Save(ctx)
REDACTED

	resolvedUserID, err := resolvePendingOAuthTargetUserID(ctx, client, session)
REDACTED
	require.Equal(t, existingUser.ID, resolvedUserID)
REDACTED

func TestBindOIDCOAuthLoginReturns2FAChallengeWhenUserHasTotp(t *testing.T) {
	totpCache := &oauthPendingFlowTotpCacheStub{REDACTED
	handler, client := newOAuthPendingFlowTestHandlerWithDependencies(t, oauthPendingFlowTestHandlerOptions{
		settingValues: map[string]string{
			service.SettingKeyTotpEnabled: "true",
	REDACTED,
		totpCache:     totpCache,
		totpEncryptor: oauthPendingFlowTotpEncryptorStub{REDACTED,
REDACTED)
	ctx := context.Background()

	passwordHash, err := handler.authService.HashPassword("secret-123")
REDACTED
	totpEnabledAt := time.Now().UTC().Add(-time.Hour)
	secret := "JBSWY3DPEHPK3PXP"

	existingUser, err := client.User.Create().
		SetEmail("owner@example.com").
		SetUsername("owner-user").
		SetPasswordHash(passwordHash).
		SetRole(service.RoleUser).
		SetStatus(service.StatusActive).
		SetTotpEnabled(true).
		SetTotpSecretEncrypted(secret).
		SetTotpEnabledAt(totpEnabledAt).
		Save(ctx)
REDACTED

	session, err := client.PendingAuthSession.Create().
		SetSessionToken("bind-login-2fa-session-token").
		SetIntent("adopt_existing_user_by_email").
		SetProviderType("oidc").
		SetProviderKey("https://issuer.example").
		SetProviderSubject("oidc-bind-2fa-123").
		SetTargetUserID(existingUser.ID).
		SetResolvedEmail(existingUser.Email).
		SetBrowserSessionKey("bind-login-2fa-browser-session-key").
		SetUpstreamIdentityClaims(map[string]any{
			"suggested_display_name": "Bound OIDC User",
			"suggested_avatar_url":   "https://cdn.example/bound.png",
	REDACTED).
		SetRedirectTo("/profile").
		SetExpiresAt(time.Now().UTC().Add(10 * time.Minute)).
		Save(ctx)
REDACTED

	body := bytes.NewBufferString(`{"email":"owner@example.com","password":"secret-123","adopt_display_name":false,"adopt_avatar":falseREDACTED`)
	recorder := httptest.NewRecorder()
	ginCtx, _ := gin.CreateTestContext(recorder)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/oauth/oidc/bind-login", body)
	req.Header.Set("Content-Type", "application/json")
	req.AddCookie(&http.Cookie{Name: oauthPendingSessionCookieName, Value: encodeCookieValue(session.SessionToken)REDACTED)
	req.AddCookie(&http.Cookie{Name: oauthPendingBrowserCookieName, Value: encodeCookieValue("bind-login-2fa-browser-session-key")REDACTED)
	ginCtx.Request = req

	handler.BindOIDCOAuthLogin(ginCtx)

	require.Equal(t, http.StatusOK, recorder.Code)
	data := decodeJSONResponseData(t, recorder)
	require.Equal(t, true, data["requires_2fa"])
	require.Equal(t, "o***r@example.com", data["user_email_masked"])
	tempToken, ok := data["temp_token"].(string)
	require.True(t, ok)
	require.NotEmpty(t, tempToken)

	loginSession, err := totpCache.GetLoginSession(ctx, tempToken)
REDACTED
	require.NotNil(t, loginSession)
	require.NotNil(t, loginSession.PendingOAuthBind)
	require.Equal(t, session.SessionToken, loginSession.PendingOAuthBind.PendingSessionToken)
	require.Equal(t, session.BrowserSessionKey, loginSession.PendingOAuthBind.BrowserSessionKey)

	identityCount, err := client.AuthIdentity.Query().
		Where(
			authidentity.ProviderTypeEQ("oidc"),
			authidentity.ProviderKeyEQ("https://issuer.example"),
			authidentity.ProviderSubjectEQ("oidc-bind-2fa-123"),
		).
		Count(ctx)
REDACTED
	require.Zero(t, identityCount)

	storedSession, err := client.PendingAuthSession.Get(ctx, session.ID)
REDACTED
	require.Nil(t, storedSession.ConsumedAt)
REDACTED

func TestLogin2FACompletesPendingOAuthBindAndConsumesSession(t *testing.T) {
	totpCache := &oauthPendingFlowTotpCacheStub{REDACTED
	defaultSubAssigner := &oauthPendingFlowDefaultSubAssignerStub{REDACTED
	handler, client := newOAuthPendingFlowTestHandlerWithDependencies(t, oauthPendingFlowTestHandlerOptions{
		settingValues: map[string]string{
			service.SettingKeyTotpEnabled:                           "true",
			service.SettingKeyAuthSourceDefaultOIDCBalance:          "8",
			service.SettingKeyAuthSourceDefaultOIDCConcurrency:      "2",
			service.SettingKeyAuthSourceDefaultOIDCGrantOnFirstBind: "true",
	REDACTED,
		defaultSubAssigner: defaultSubAssigner,
		totpCache:          totpCache,
		totpEncryptor:      oauthPendingFlowTotpEncryptorStub{REDACTED,
REDACTED)
	ctx := context.Background()

	passwordHash, err := handler.authService.HashPassword("secret-123")
REDACTED
	totpEnabledAt := time.Now().UTC().Add(-time.Hour)
	secret := "JBSWY3DPEHPK3PXP"

	existingUser, err := client.User.Create().
		SetEmail("owner@example.com").
		SetUsername("owner-user").
		SetPasswordHash(passwordHash).
		SetBalance(1.5).
		SetConcurrency(4).
		SetRole(service.RoleUser).
		SetStatus(service.StatusActive).
		SetTotpEnabled(true).
		SetTotpSecretEncrypted(secret).
		SetTotpEnabledAt(totpEnabledAt).
		Save(ctx)
REDACTED

	session, err := client.PendingAuthSession.Create().
		SetSessionToken("login-2fa-pending-session-token").
		SetIntent("adopt_existing_user_by_email").
		SetProviderType("oidc").
		SetProviderKey("https://issuer.example").
		SetProviderSubject("oidc-login-2fa-123").
		SetTargetUserID(existingUser.ID).
		SetResolvedEmail(existingUser.Email).
		SetBrowserSessionKey("login-2fa-browser-session-key").
		SetUpstreamIdentityClaims(map[string]any{
			"suggested_display_name": "Bound OIDC User",
			"suggested_avatar_url":   "https://cdn.example/bound.png",
	REDACTED).
		SetRedirectTo("/profile").
		SetExpiresAt(time.Now().UTC().Add(10 * time.Minute)).
		Save(ctx)
REDACTED

	_, err = client.IdentityAdoptionDecision.Create().
		SetPendingAuthSessionID(session.ID).
		SetAdoptDisplayName(false).
		SetAdoptAvatar(false).
		Save(ctx)
REDACTED

	tempToken, err := handler.totpService.CreatePendingOAuthBindLoginSession(
		ctx,
		existingUser.ID,
		existingUser.Email,
		session.SessionToken,
		session.BrowserSessionKey,
	)
REDACTED

	code, err := totp.GenerateCode(secret, time.Now().UTC())
REDACTED

	body := bytes.NewBufferString(`{"temp_token":"` + tempToken + `","totp_code":"` + code + `"REDACTED`)
	recorder := httptest.NewRecorder()
	ginCtx, _ := gin.CreateTestContext(recorder)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/login/2fa", body)
	req.Header.Set("Content-Type", "application/json")
	req.AddCookie(&http.Cookie{Name: oauthPendingSessionCookieName, Value: encodeCookieValue(session.SessionToken)REDACTED)
	req.AddCookie(&http.Cookie{Name: oauthPendingBrowserCookieName, Value: encodeCookieValue(session.BrowserSessionKey)REDACTED)
	ginCtx.Request = req

	handler.Login2FA(ginCtx)

	require.Equal(t, http.StatusOK, recorder.Code)
	payload := decodeJSONResponseData(t, recorder)
	require.NotEmpty(t, payload["access_token"])
	require.NotEmpty(t, payload["refresh_token"])
	accessToken, ok := payload["access_token"].(string)
	require.True(t, ok)
	claims, err := handler.authService.ValidateToken(accessToken)
REDACTED
	reloadedUser, err := handler.userService.GetByID(ctx, existingUser.ID)
REDACTED
	require.Equal(t, reloadedUser.TokenVersion, claims.TokenVersion)

	identity, err := client.AuthIdentity.Query().
		Where(
			authidentity.ProviderTypeEQ("oidc"),
			authidentity.ProviderKeyEQ("https://issuer.example"),
			authidentity.ProviderSubjectEQ("oidc-login-2fa-123"),
		).
		Only(ctx)
REDACTED
	require.Equal(t, existingUser.ID, identity.UserID)

	storedSession, err := client.PendingAuthSession.Get(ctx, session.ID)
REDACTED
	require.NotNil(t, storedSession.ConsumedAt)

	loginSession, err := totpCache.GetLoginSession(ctx, tempToken)
REDACTED
	require.Nil(t, loginSession)

	storedUser, err := client.User.Get(ctx, existingUser.ID)
REDACTED
	require.Equal(t, 9.5, storedUser.Balance)
	require.Equal(t, 6, storedUser.Concurrency)
	require.Equal(t, 1, countProviderGrantRecords(t, client, existingUser.ID, "oidc", "first_bind"))
	require.Empty(t, defaultSubAssigner.calls)
REDACTED

func newOAuthPendingFlowTestHandler(t *testing.T, invitationEnabled bool) (*AuthHandler, *dbent.Client) {
REDACTED

	return newOAuthPendingFlowTestHandlerWithOptions(t, invitationEnabled, false, nil)
REDACTED

func newOAuthPendingFlowTestHandlerWithEmailVerification(
	t *testing.T,
	invitationEnabled bool,
	email string,
	code string,
) (*AuthHandler, *dbent.Client) {
REDACTED

	cache := &oauthPendingFlowEmailCacheStub{
		verificationCodes: map[string]*service.VerificationCodeData{
			email: {
				Code:      code,
				Attempts:  0,
				CreatedAt: time.Now().UTC(),
				ExpiresAt: time.Now().UTC().Add(15 * time.Minute),
		REDACTED,
	REDACTED,
REDACTED
	return newOAuthPendingFlowTestHandlerWithOptions(t, invitationEnabled, true, cache)
REDACTED

func newOAuthPendingFlowTestHandlerWithOptions(
	t *testing.T,
	invitationEnabled bool,
	emailVerifyEnabled bool,
	emailCache service.EmailCache,
) (*AuthHandler, *dbent.Client) {
	return newOAuthPendingFlowTestHandlerWithDependencies(t, oauthPendingFlowTestHandlerOptions{
		invitationEnabled:  invitationEnabled,
		emailVerifyEnabled: emailVerifyEnabled,
		emailCache:         emailCache,
REDACTED)
REDACTED

type oauthPendingFlowTestHandlerOptions struct {
	invitationEnabled  bool
	emailVerifyEnabled bool
	emailCache         service.EmailCache
	settingValues      map[string]string
	promoRepo          service.PromoCodeRepository
	defaultSubAssigner service.DefaultSubscriptionAssigner
	affiliateService   *service.AffiliateService
	affiliateFactory   func(*dbent.Client, *service.SettingService) *service.AffiliateService
	totpCache          service.TotpCache
	totpEncryptor      service.SecretEncryptor
	userRepoOptions    oauthPendingFlowUserRepoOptions
REDACTED

func newOAuthPendingFlowTestHandlerWithDependencies(
	t *testing.T,
	options oauthPendingFlowTestHandlerOptions,
) (*AuthHandler, *dbent.Client) {
REDACTED

	db, err := sql.Open("sqlite", "file:auth_oauth_pending_flow_handler?mode=memory&cache=shared")
REDACTED
	t.Cleanup(func() { _ = db.Close() REDACTED)

	_, err = db.Exec("PRAGMA foreign_keys = ON")
REDACTED
	_, err = db.Exec(`
CREATE TABLE IF NOT EXISTS user_provider_default_grants (
	id INTEGER PRIMARY KEY AUTOINCREMENT,
	user_id INTEGER NOT NULL,
	provider_type TEXT NOT NULL,
	grant_reason TEXT NOT NULL DEFAULT 'first_bind',
	created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
	UNIQUE(user_id, provider_type, grant_reason)
)`)
REDACTED
	_, err = db.Exec(`
CREATE TABLE IF NOT EXISTS user_avatars (
	user_id INTEGER PRIMARY KEY,
	storage_provider TEXT NOT NULL,
	storage_key TEXT NOT NULL DEFAULT '',
	url TEXT NOT NULL,
	content_type TEXT NOT NULL DEFAULT '',
	byte_size INTEGER NOT NULL DEFAULT 0,
	sha256 TEXT NOT NULL DEFAULT '',
	updated_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
)`)
REDACTED
	_, err = db.Exec(`
CREATE TABLE IF NOT EXISTS user_affiliates (
	user_id INTEGER PRIMARY KEY,
	aff_code TEXT NOT NULL UNIQUE,
	aff_code_custom BOOLEAN NOT NULL DEFAULT false,
	aff_rebate_rate_percent REAL NULL,
	inviter_id INTEGER NULL,
	aff_count INTEGER NOT NULL DEFAULT 0,
	aff_quota REAL NOT NULL DEFAULT 0,
	aff_frozen_quota REAL NOT NULL DEFAULT 0,
	aff_history_quota REAL NOT NULL DEFAULT 0,
	created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
	updated_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
)`)
REDACTED

	drv := entsql.OpenDB(dialect.SQLite, db)
	client := enttest.NewClient(t, enttest.WithOptions(dbent.Driver(drv)))

	cfg := &config.Config{
		JWT: config.JWTConfig{
			Secret:                   "test-secret",
			ExpireHour:               1,
			AccessTokenExpireMinutes: 60,
			RefreshTokenExpireDays:   7,
	REDACTED,
		Default: config.DefaultConfig{
			UserBalance:     0,
			UserConcurrency: 1,
	REDACTED,
REDACTED
	settingValues := map[string]string{
		service.SettingKeyRegistrationEnabled:              "true",
		service.SettingKeyInvitationCodeEnabled:            boolSettingValue(options.invitationEnabled),
		service.SettingKeyEmailVerifyEnabled:               boolSettingValue(options.emailVerifyEnabled),
		service.SettingKeyRegistrationEmailSuffixWhitelist: "[]",
REDACTED
	for key, value := range options.settingValues {
		settingValues[key] = value
REDACTED
	settingSvc := service.NewSettingService(&oauthPendingFlowSettingRepoStub{values: settingValuesREDACTED, cfg)
	affiliateService := options.affiliateService
	if affiliateService == nil && options.affiliateFactory != nil {
		affiliateService = options.affiliateFactory(client, settingSvc)
REDACTED
	userRepo := &oauthPendingFlowUserRepo{
		client:  client,
		options: options.userRepoOptions,
REDACTED
	redeemRepo := &oauthPendingFlowRedeemCodeRepo{client: clientREDACTED
	var promoService *service.PromoService
	if options.promoRepo != nil {
		promoService = service.NewPromoService(options.promoRepo, userRepo, nil, client, nil)
REDACTED
	var emailService *service.EmailService
	if options.emailCache != nil {
		emailService = service.NewEmailService(&oauthPendingFlowSettingRepoStub{
			values: map[string]string{
				service.SettingKeyEmailVerifyEnabled: boolSettingValue(options.emailVerifyEnabled),
		REDACTED,
	REDACTED, options.emailCache)
REDACTED
	authSvc := service.NewAuthService(
		client,
		userRepo,
		redeemRepo,
		&oauthPendingFlowRefreshTokenCacheStub{REDACTED,
		cfg,
		settingSvc,
		emailService,
		nil,
		nil,
		promoService,
		options.defaultSubAssigner,
		affiliateService,
		nil,
	)
	userSvc := service.NewUserService(userRepo, nil, nil, nil)
	var totpSvc *service.TotpService
	if options.totpCache != nil || options.totpEncryptor != nil {
		totpCache := options.totpCache
		if totpCache == nil {
			totpCache = &oauthPendingFlowTotpCacheStub{REDACTED
	REDACTED
		totpEncryptor := options.totpEncryptor
		if totpEncryptor == nil {
			totpEncryptor = oauthPendingFlowTotpEncryptorStub{REDACTED
	REDACTED
		totpSvc = service.NewTotpService(userRepo, totpEncryptor, totpCache, settingSvc, nil, nil)
REDACTED

	return &AuthHandler{
		authService:  authSvc,
		userService:  userSvc,
		settingSvc:   settingSvc,
		promoService: promoService,
		totpService:  totpSvc,
REDACTED, client
REDACTED

func boolSettingValue(v bool) string {
	if v {
		return "true"
REDACTED
	return "false"
REDACTED

func boolPtr(v bool) *bool {
	return &v
REDACTED

type oauthPendingFlowSettingRepoStub struct {
	values map[string]string
REDACTED

type oauthPendingFlowPromoRepoStub struct {
	promo      *service.PromoCode
	applyCalls []string
	usages     []service.PromoCodeUsage
REDACTED

func newOAuthPendingFlowPromoRepoStub(code string, bonusAmount float64) *oauthPendingFlowPromoRepoStub {
	return &oauthPendingFlowPromoRepoStub{
		promo: &service.PromoCode{
			ID:          1,
			Code:        code,
			BonusAmount: bonusAmount,
			Status:      service.PromoCodeStatusActive,
	REDACTED,
REDACTED
REDACTED

func (r *oauthPendingFlowPromoRepoStub) Create(context.Context, *service.PromoCode) error {
	panic("unexpected Create call")
REDACTED

func (r *oauthPendingFlowPromoRepoStub) GetByID(context.Context, int64) (*service.PromoCode, error) {
	panic("unexpected GetByID call")
REDACTED

func (r *oauthPendingFlowPromoRepoStub) GetByCode(_ context.Context, code string) (*service.PromoCode, error) {
	if r.promo == nil || !strings.EqualFold(strings.TrimSpace(code), r.promo.Code) {
		return nil, service.ErrPromoCodeNotFound
REDACTED
	clone := *r.promo
	return &clone, nil
REDACTED

func (r *oauthPendingFlowPromoRepoStub) GetByCodeForUpdate(ctx context.Context, code string) (*service.PromoCode, error) {
	promoCode, err := r.GetByCode(ctx, code)
	if err != nil {
		return nil, err
REDACTED
	r.applyCalls = append(r.applyCalls, strings.TrimSpace(code))
	return promoCode, nil
REDACTED

func (r *oauthPendingFlowPromoRepoStub) Update(context.Context, *service.PromoCode) error {
	panic("unexpected Update call")
REDACTED

func (r *oauthPendingFlowPromoRepoStub) Delete(context.Context, int64) error {
	panic("unexpected Delete call")
REDACTED

func (r *oauthPendingFlowPromoRepoStub) List(context.Context, pagination.PaginationParams) ([]service.PromoCode, *pagination.PaginationResult, error) {
	panic("unexpected List call")
REDACTED

func (r *oauthPendingFlowPromoRepoStub) ListWithFilters(context.Context, pagination.PaginationParams, string, string) ([]service.PromoCode, *pagination.PaginationResult, error) {
	panic("unexpected ListWithFilters call")
REDACTED

func (r *oauthPendingFlowPromoRepoStub) CreateUsage(_ context.Context, usage *service.PromoCodeUsage) error {
	r.usages = append(r.usages, *usage)
	return nil
REDACTED

func (r *oauthPendingFlowPromoRepoStub) GetUsageByPromoCodeAndUser(context.Context, int64, int64) (*service.PromoCodeUsage, error) {
	return nil, nil
REDACTED

func (r *oauthPendingFlowPromoRepoStub) ListUsagesByPromoCode(context.Context, int64, pagination.PaginationParams) ([]service.PromoCodeUsage, *pagination.PaginationResult, error) {
	panic("unexpected ListUsagesByPromoCode call")
REDACTED

func (r *oauthPendingFlowPromoRepoStub) IncrementUsedCount(context.Context, int64) error {
	if r.promo != nil {
		r.promo.UsedCount++
REDACTED
	return nil
REDACTED

func (s *oauthPendingFlowSettingRepoStub) Get(context.Context, string) (*service.Setting, error) {
	return nil, service.ErrSettingNotFound
REDACTED

func (s *oauthPendingFlowSettingRepoStub) GetValue(_ context.Context, key string) (string, error) {
	value, ok := s.values[key]
	if !ok {
		return "", service.ErrSettingNotFound
REDACTED
	return value, nil
REDACTED

func (s *oauthPendingFlowSettingRepoStub) Set(context.Context, string, string) error {
	return nil
REDACTED

func (s *oauthPendingFlowSettingRepoStub) GetMultiple(_ context.Context, keys []string) (map[string]string, error) {
	result := make(map[string]string, len(keys))
	for _, key := range keys {
		if value, ok := s.values[key]; ok {
			result[key] = value
	REDACTED
REDACTED
	return result, nil
REDACTED

func (s *oauthPendingFlowSettingRepoStub) SetMultiple(context.Context, map[string]string) error {
	return nil
REDACTED

func (s *oauthPendingFlowSettingRepoStub) GetAll(context.Context) (map[string]string, error) {
	result := make(map[string]string, len(s.values))
	for key, value := range s.values {
		result[key] = value
REDACTED
	return result, nil
REDACTED

func (s *oauthPendingFlowSettingRepoStub) Delete(context.Context, string) error {
	return nil
REDACTED

type oauthPendingFlowRefreshTokenCacheStub struct{REDACTED

type oauthPendingFlowEmailCacheStub struct {
	verificationCodes map[string]*service.VerificationCodeData
REDACTED

func (s *oauthPendingFlowEmailCacheStub) GetVerificationCode(_ context.Context, email string) (*service.VerificationCodeData, error) {
	if s == nil || s.verificationCodes == nil {
		return nil, nil
REDACTED
	return s.verificationCodes[email], nil
REDACTED

func (s *oauthPendingFlowEmailCacheStub) SetVerificationCode(_ context.Context, email string, data *service.VerificationCodeData, _ time.Duration) error {
	if s.verificationCodes == nil {
		s.verificationCodes = map[string]*service.VerificationCodeData{REDACTED
REDACTED
	s.verificationCodes[email] = data
	return nil
REDACTED

func (s *oauthPendingFlowEmailCacheStub) DeleteVerificationCode(_ context.Context, email string) error {
	delete(s.verificationCodes, email)
	return nil
REDACTED

func (s *oauthPendingFlowEmailCacheStub) GetNotifyVerifyCode(context.Context, string) (*service.VerificationCodeData, error) {
	return nil, nil
REDACTED

func (s *oauthPendingFlowEmailCacheStub) SetNotifyVerifyCode(context.Context, string, *service.VerificationCodeData, time.Duration) error {
	return nil
REDACTED

func (s *oauthPendingFlowEmailCacheStub) DeleteNotifyVerifyCode(context.Context, string) error {
	return nil
REDACTED

func (s *oauthPendingFlowEmailCacheStub) GetPasswordResetToken(context.Context, string) (*service.PasswordResetTokenData, error) {
	return nil, nil
REDACTED

func (s *oauthPendingFlowEmailCacheStub) SetPasswordResetToken(context.Context, string, *service.PasswordResetTokenData, time.Duration) error {
	return nil
REDACTED

func (s *oauthPendingFlowEmailCacheStub) DeletePasswordResetToken(context.Context, string) error {
	return nil
REDACTED

func (s *oauthPendingFlowEmailCacheStub) IsPasswordResetEmailInCooldown(context.Context, string) bool {
	return false
REDACTED

func (s *oauthPendingFlowEmailCacheStub) SetPasswordResetEmailCooldown(context.Context, string, time.Duration) error {
	return nil
REDACTED

func (s *oauthPendingFlowEmailCacheStub) IncrNotifyCodeUserRate(context.Context, int64, time.Duration) (int64, error) {
	return 0, nil
REDACTED

func (s *oauthPendingFlowEmailCacheStub) GetNotifyCodeUserRate(context.Context, int64) (int64, error) {
	return 0, nil
REDACTED

func (s *oauthPendingFlowRefreshTokenCacheStub) StoreRefreshToken(context.Context, string, *service.RefreshTokenData, time.Duration) error {
	return nil
REDACTED

func (s *oauthPendingFlowRefreshTokenCacheStub) GetRefreshToken(context.Context, string) (*service.RefreshTokenData, error) {
	return nil, service.ErrRefreshTokenNotFound
REDACTED

func (s *oauthPendingFlowRefreshTokenCacheStub) DeleteRefreshToken(context.Context, string) error {
	return nil
REDACTED

func (s *oauthPendingFlowRefreshTokenCacheStub) DeleteUserRefreshTokens(context.Context, int64) error {
	return nil
REDACTED

func (s *oauthPendingFlowRefreshTokenCacheStub) DeleteTokenFamily(context.Context, string) error {
	return nil
REDACTED

func (s *oauthPendingFlowRefreshTokenCacheStub) AddToUserTokenSet(context.Context, int64, string, time.Duration) error {
	return nil
REDACTED

func (s *oauthPendingFlowRefreshTokenCacheStub) AddToFamilyTokenSet(context.Context, string, string, time.Duration) error {
	return nil
REDACTED

func (s *oauthPendingFlowRefreshTokenCacheStub) GetUserTokenHashes(context.Context, int64) ([]string, error) {
	return nil, nil
REDACTED

func (s *oauthPendingFlowRefreshTokenCacheStub) GetFamilyTokenHashes(context.Context, string) ([]string, error) {
	return nil, nil
REDACTED

func (s *oauthPendingFlowRefreshTokenCacheStub) IsTokenInFamily(context.Context, string, string) (bool, error) {
	return false, nil
REDACTED

type oauthPendingFlowRedeemCodeRepo struct {
	client *dbent.Client
REDACTED

func (r *oauthPendingFlowRedeemCodeRepo) Create(context.Context, *service.RedeemCode) error {
	panic("unexpected Create call")
REDACTED

func (r *oauthPendingFlowRedeemCodeRepo) CreateBatch(context.Context, []service.RedeemCode) error {
	panic("unexpected CreateBatch call")
REDACTED

func (r *oauthPendingFlowRedeemCodeRepo) GetByID(context.Context, int64) (*service.RedeemCode, error) {
	panic("unexpected GetByID call")
REDACTED

func (r *oauthPendingFlowRedeemCodeRepo) GetByCode(ctx context.Context, code string) (*service.RedeemCode, error) {
	entity, err := r.client.RedeemCode.Query().Where(redeemcode.CodeEQ(code)).Only(ctx)
	if err != nil {
		if dbent.IsNotFound(err) {
			return nil, service.ErrRedeemCodeNotFound
	REDACTED
		return nil, err
REDACTED
	notes := ""
	if entity.Notes != nil {
		notes = *entity.Notes
REDACTED
	return &service.RedeemCode{
		ID:           entity.ID,
		Code:         entity.Code,
		Type:         entity.Type,
		Value:        entity.Value,
		Status:       entity.Status,
		UsedBy:       entity.UsedBy,
		UsedAt:       entity.UsedAt,
		Notes:        notes,
		CreatedAt:    entity.CreatedAt,
		GroupID:      entity.GroupID,
		ValidityDays: entity.ValidityDays,
REDACTED, nil
REDACTED

func (r *oauthPendingFlowRedeemCodeRepo) Update(ctx context.Context, code *service.RedeemCode) error {
	if code == nil {
		return nil
REDACTED
	update := r.client.RedeemCode.UpdateOneID(code.ID).
		SetCode(code.Code).
		SetType(code.Type).
		SetValue(code.Value).
		SetStatus(code.Status).
		SetNotes(code.Notes).
		SetValidityDays(code.ValidityDays)
	if code.UsedBy != nil {
		update = update.SetUsedBy(*code.UsedBy)
REDACTED else {
		update = update.ClearUsedBy()
REDACTED
	if code.UsedAt != nil {
		update = update.SetUsedAt(*code.UsedAt)
REDACTED else {
		update = update.ClearUsedAt()
REDACTED
	if code.GroupID != nil {
		update = update.SetGroupID(*code.GroupID)
REDACTED else {
		update = update.ClearGroupID()
REDACTED
	_, err := update.Save(ctx)
	return err
REDACTED

func (r *oauthPendingFlowRedeemCodeRepo) BatchUpdate(context.Context, []int64, service.RedeemCodeBatchUpdateFields) (int64, error) {
	panic("unexpected BatchUpdate call")
REDACTED

func (r *oauthPendingFlowRedeemCodeRepo) Delete(context.Context, int64) error {
	panic("unexpected Delete call")
REDACTED

func (r *oauthPendingFlowRedeemCodeRepo) Use(ctx context.Context, id, userID int64) error {
	affected, err := r.client.RedeemCode.Update().
		Where(redeemcode.IDEQ(id), redeemcode.StatusEQ(service.StatusUnused)).
		SetStatus(service.StatusUsed).
		SetUsedBy(userID).
		SetUsedAt(time.Now().UTC()).
		Save(ctx)
	if err != nil {
		return err
REDACTED
	if affected == 0 {
		return service.ErrRedeemCodeUsed
REDACTED
	return nil
REDACTED

func (r *oauthPendingFlowRedeemCodeRepo) List(context.Context, pagination.PaginationParams) ([]service.RedeemCode, *pagination.PaginationResult, error) {
	panic("unexpected List call")
REDACTED

func (r *oauthPendingFlowRedeemCodeRepo) ListWithFilters(context.Context, pagination.PaginationParams, string, string, string) ([]service.RedeemCode, *pagination.PaginationResult, error) {
	panic("unexpected ListWithFilters call")
REDACTED

func (r *oauthPendingFlowRedeemCodeRepo) ListByUser(context.Context, int64, int) ([]service.RedeemCode, error) {
	panic("unexpected ListByUser call")
REDACTED

func (r *oauthPendingFlowRedeemCodeRepo) ListByUserPaginated(context.Context, int64, pagination.PaginationParams, string) ([]service.RedeemCode, *pagination.PaginationResult, error) {
	panic("unexpected ListByUserPaginated call")
REDACTED

func (r *oauthPendingFlowRedeemCodeRepo) SumPositiveBalanceByUser(context.Context, int64) (float64, error) {
	panic("unexpected SumPositiveBalanceByUser call")
REDACTED

func decodeJSONResponseData(t *testing.T, recorder *httptest.ResponseRecorder) map[string]any {
REDACTED

	var envelope struct {
		Data map[string]any `json:"data"`
REDACTED
	require.NoError(t, json.Unmarshal(recorder.Body.Bytes(), &envelope))
	return envelope.Data
REDACTED

func decodeJSONBody(t *testing.T, recorder *httptest.ResponseRecorder) map[string]any {
REDACTED

	var payload map[string]any
	require.NoError(t, json.Unmarshal(recorder.Body.Bytes(), &payload))
	return payload
REDACTED

type oauthPendingFlowAvatarRecord struct {
	StorageProvider string
	URL             string
REDACTED

func loadUserAvatarRecord(t *testing.T, client *dbent.Client, userID int64) *oauthPendingFlowAvatarRecord {
REDACTED

	var rows entsql.Rows
	err := client.Driver().Query(
		context.Background(),
		`SELECT storage_provider, url FROM user_avatars WHERE user_id = ?`,
		[]any{userIDREDACTED,
		&rows,
	)
REDACTED
	defer func() { _ = rows.Close() REDACTED()

	if !rows.Next() {
		require.NoError(t, rows.Err())
		return nil
REDACTED

	var record oauthPendingFlowAvatarRecord
	require.NoError(t, rows.Scan(&record.StorageProvider, &record.URL))
	require.NoError(t, rows.Err())
	return &record
REDACTED

func countProviderGrantRecords(
	t *testing.T,
	client *dbent.Client,
	userID int64,
	providerType string,
	grantReason string,
) int {
REDACTED

	var rows entsql.Rows
	err := client.Driver().Query(
		context.Background(),
		`SELECT COUNT(*) FROM user_provider_default_grants WHERE user_id = ? AND provider_type = ? AND grant_reason = ?`,
		[]any{userID, providerType, grantReasonREDACTED,
		&rows,
	)
REDACTED
	defer func() { _ = rows.Close() REDACTED()

	require.True(t, rows.Next())
	var count int
	require.NoError(t, rows.Scan(&count))
	require.False(t, rows.Next())
	return count
REDACTED

type oauthPendingFlowUserRepo struct {
	client  *dbent.Client
	options oauthPendingFlowUserRepoOptions
REDACTED

type oauthPendingFlowUserRepoOptions struct {
	rejectDeleteWhileAuthIdentityExists bool
REDACTED

func (r *oauthPendingFlowUserRepo) Create(ctx context.Context, user *service.User) error {
	entity, err := r.client.User.Create().
		SetEmail(user.Email).
		SetUsername(user.Username).
		SetNotes(user.Notes).
		SetPasswordHash(user.PasswordHash).
		SetRole(user.Role).
		SetBalance(user.Balance).
		SetConcurrency(user.Concurrency).
		SetStatus(user.Status).
		SetNillableTotpSecretEncrypted(user.TotpSecretEncrypted).
		SetTotpEnabled(user.TotpEnabled).
		SetNillableTotpEnabledAt(user.TotpEnabledAt).
		SetTotalRecharged(user.TotalRecharged).
		SetSignupSource(user.SignupSource).
		SetNillableLastLoginAt(user.LastLoginAt).
		SetNillableLastActiveAt(user.LastActiveAt).
		Save(ctx)
	if err != nil {
		return err
REDACTED
	user.ID = entity.ID
	user.CreatedAt = entity.CreatedAt
	user.UpdatedAt = entity.UpdatedAt
	return nil
REDACTED

func (r *oauthPendingFlowUserRepo) GetByID(ctx context.Context, id int64) (*service.User, error) {
	entity, err := r.client.User.Get(ctx, id)
	if err != nil {
		if dbent.IsNotFound(err) {
			return nil, service.ErrUserNotFound
	REDACTED
		return nil, err
REDACTED
	return oauthPendingFlowServiceUser(entity), nil
REDACTED

func (r *oauthPendingFlowUserRepo) GetByEmail(ctx context.Context, email string) (*service.User, error) {
	entity, err := r.client.User.Query().Where(dbuser.EmailEQ(email)).Only(ctx)
	if err != nil {
		if dbent.IsNotFound(err) {
			return nil, service.ErrUserNotFound
	REDACTED
		return nil, err
REDACTED
	return oauthPendingFlowServiceUser(entity), nil
REDACTED

func (r *oauthPendingFlowUserRepo) GetFirstAdmin(context.Context) (*service.User, error) {
	panic("unexpected GetFirstAdmin call")
REDACTED

func (r *oauthPendingFlowUserRepo) Update(ctx context.Context, user *service.User) error {
	entity, err := r.client.User.UpdateOneID(user.ID).
		SetEmail(user.Email).
		SetUsername(user.Username).
		SetNotes(user.Notes).
		SetPasswordHash(user.PasswordHash).
		SetRole(user.Role).
		SetBalance(user.Balance).
		SetConcurrency(user.Concurrency).
		SetStatus(user.Status).
		SetNillableTotpSecretEncrypted(user.TotpSecretEncrypted).
		SetTotpEnabled(user.TotpEnabled).
		SetNillableTotpEnabledAt(user.TotpEnabledAt).
		SetTotalRecharged(user.TotalRecharged).
		SetSignupSource(user.SignupSource).
		SetNillableLastLoginAt(user.LastLoginAt).
		SetNillableLastActiveAt(user.LastActiveAt).
		Save(ctx)
	if err != nil {
		return err
REDACTED
	user.UpdatedAt = entity.UpdatedAt
	return nil
REDACTED

func (r *oauthPendingFlowUserRepo) UpdateUserLastActiveAt(ctx context.Context, userID int64, activeAt time.Time) error {
	return r.client.User.UpdateOneID(userID).SetLastActiveAt(activeAt).Exec(ctx)
REDACTED

func (r *oauthPendingFlowUserRepo) Delete(ctx context.Context, id int64) error {
	if r.options.rejectDeleteWhileAuthIdentityExists {
		count, err := r.client.AuthIdentity.Query().Where(authidentity.UserIDEQ(id)).Count(ctx)
		if err != nil {
			return err
	REDACTED
		if count > 0 {
			return errors.New("cannot delete user while auth identities still exist")
	REDACTED
REDACTED
	return r.client.User.DeleteOneID(id).Exec(ctx)
REDACTED

func (r *oauthPendingFlowUserRepo) GetUserAvatar(ctx context.Context, userID int64) (*service.UserAvatar, error) {
	driver := r.client.Driver()
	if tx := dbent.TxFromContext(ctx); tx != nil {
		driver = tx.Client().Driver()
REDACTED

	var rows entsql.Rows
	if err := driver.Query(
		ctx,
		`SELECT storage_provider, storage_key, url, content_type, byte_size, sha256 FROM user_avatars WHERE user_id = ?`,
		[]any{userIDREDACTED,
		&rows,
	); err != nil {
		return nil, err
REDACTED
	defer func() { _ = rows.Close() REDACTED()

	if !rows.Next() {
		return nil, rows.Err()
REDACTED

	var avatar service.UserAvatar
	if err := rows.Scan(
		&avatar.StorageProvider,
		&avatar.StorageKey,
		&avatar.URL,
		&avatar.ContentType,
		&avatar.ByteSize,
		&avatar.SHA256,
	); err != nil {
		return nil, err
REDACTED
	if err := rows.Err(); err != nil {
		return nil, err
REDACTED
	return &avatar, nil
REDACTED

func (r *oauthPendingFlowUserRepo) UpsertUserAvatar(ctx context.Context, userID int64, input service.UpsertUserAvatarInput) (*service.UserAvatar, error) {
	driver := r.client.Driver()
	if tx := dbent.TxFromContext(ctx); tx != nil {
		driver = tx.Client().Driver()
REDACTED

	var result entsql.Result
	if err := driver.Exec(
		ctx,
		`INSERT INTO user_avatars (user_id, storage_provider, storage_key, url, content_type, byte_size, sha256, updated_at)
VALUES (?, ?, ?, ?, ?, ?, ?, CURRENT_TIMESTAMP)
ON CONFLICT(user_id) DO UPDATE SET
	storage_provider = excluded.storage_provider,
	storage_key = excluded.storage_key,
	url = excluded.url,
	content_type = excluded.content_type,
	byte_size = excluded.byte_size,
	sha256 = excluded.sha256,
	updated_at = CURRENT_TIMESTAMP`,
		[]any{
			userID,
			input.StorageProvider,
			input.StorageKey,
			input.URL,
			input.ContentType,
			input.ByteSize,
			input.SHA256,
	REDACTED,
		&result,
	); err != nil {
		return nil, err
REDACTED

	return &service.UserAvatar{
		StorageProvider: input.StorageProvider,
		StorageKey:      input.StorageKey,
		URL:             input.URL,
		ContentType:     input.ContentType,
		ByteSize:        input.ByteSize,
		SHA256:          input.SHA256,
REDACTED, nil
REDACTED

func (r *oauthPendingFlowUserRepo) DeleteUserAvatar(ctx context.Context, userID int64) error {
	driver := r.client.Driver()
	if tx := dbent.TxFromContext(ctx); tx != nil {
		driver = tx.Client().Driver()
REDACTED

	var result entsql.Result
	return driver.Exec(ctx, `DELETE FROM user_avatars WHERE user_id = ?`, []any{userIDREDACTED, &result)
REDACTED

func (r *oauthPendingFlowUserRepo) List(context.Context, pagination.PaginationParams) ([]service.User, *pagination.PaginationResult, error) {
	panic("unexpected List call")
REDACTED

func (r *oauthPendingFlowUserRepo) ListWithFilters(context.Context, pagination.PaginationParams, service.UserListFilters) ([]service.User, *pagination.PaginationResult, error) {
	panic("unexpected ListWithFilters call")
REDACTED

func (r *oauthPendingFlowUserRepo) UpdateBalance(ctx context.Context, userID int64, amount float64) error {
	client := r.client
	if tx := dbent.TxFromContext(ctx); tx != nil {
		client = tx.Client()
REDACTED
	return client.User.UpdateOneID(userID).AddBalance(amount).Exec(ctx)
REDACTED

func (r *oauthPendingFlowUserRepo) DeductBalance(context.Context, int64, float64) error {
	panic("unexpected DeductBalance call")
REDACTED

func (r *oauthPendingFlowUserRepo) UpdateConcurrency(context.Context, int64, int) error {
	panic("unexpected UpdateConcurrency call")
REDACTED

func (r *oauthPendingFlowUserRepo) BatchSetConcurrency(context.Context, []int64, int) (int, error) {
	panic("unexpected BatchSetConcurrency call")
REDACTED

func (r *oauthPendingFlowUserRepo) BatchAddConcurrency(context.Context, []int64, int) (int, error) {
	panic("unexpected BatchAddConcurrency call")
REDACTED

func (r *oauthPendingFlowUserRepo) GetLatestUsedAtByUserIDs(context.Context, []int64) (map[int64]*time.Time, error) {
	return map[int64]*time.Time{REDACTED, nil
REDACTED

func (r *oauthPendingFlowUserRepo) GetLatestUsedAtByUserID(context.Context, int64) (*time.Time, error) {
	return nil, nil
REDACTED

func (r *oauthPendingFlowUserRepo) ExistsByEmail(ctx context.Context, email string) (bool, error) {
	count, err := r.client.User.Query().Where(dbuser.EmailEQ(email)).Count(ctx)
	return count > 0, err
REDACTED

func (r *oauthPendingFlowUserRepo) RemoveGroupFromAllowedGroups(context.Context, int64) (int64, error) {
	panic("unexpected RemoveGroupFromAllowedGroups call")
REDACTED

func (r *oauthPendingFlowUserRepo) AddGroupToAllowedGroups(context.Context, int64, int64) error {
	panic("unexpected AddGroupToAllowedGroups call")
REDACTED

func (r *oauthPendingFlowUserRepo) RemoveGroupFromUserAllowedGroups(context.Context, int64, int64) error {
	panic("unexpected RemoveGroupFromUserAllowedGroups call")
REDACTED

func (r *oauthPendingFlowUserRepo) ListUserAuthIdentities(ctx context.Context, userID int64) ([]service.UserAuthIdentityRecord, error) {
	identities, err := r.client.AuthIdentity.Query().
		Where(authidentity.UserIDEQ(userID)).
		All(ctx)
	if err != nil {
		return nil, err
REDACTED

	records := make([]service.UserAuthIdentityRecord, 0, len(identities))
	for _, identity := range identities {
		if identity == nil {
			continue
	REDACTED
		records = append(records, service.UserAuthIdentityRecord{
			ProviderType:    identity.ProviderType,
			ProviderKey:     identity.ProviderKey,
			ProviderSubject: identity.ProviderSubject,
			VerifiedAt:      identity.VerifiedAt,
			Issuer:          identity.Issuer,
			Metadata:        identity.Metadata,
			CreatedAt:       identity.CreatedAt,
			UpdatedAt:       identity.UpdatedAt,
	REDACTED)
REDACTED
	return records, nil
REDACTED

func (r *oauthPendingFlowUserRepo) UnbindUserAuthProvider(context.Context, int64, string) error {
	panic("unexpected UnbindUserAuthProvider call")
REDACTED

func (r *oauthPendingFlowUserRepo) UpdateTotpSecret(ctx context.Context, userID int64, encryptedSecret *string) error {
	update := r.client.User.UpdateOneID(userID)
	if encryptedSecret == nil {
		update = update.ClearTotpSecretEncrypted()
REDACTED else {
		update = update.SetTotpSecretEncrypted(*encryptedSecret)
REDACTED
	return update.Exec(ctx)
REDACTED

func (r *oauthPendingFlowUserRepo) EnableTotp(ctx context.Context, userID int64) error {
	return r.client.User.UpdateOneID(userID).
		SetTotpEnabled(true).
		SetTotpEnabledAt(time.Now().UTC()).
		Exec(ctx)
REDACTED

func (r *oauthPendingFlowUserRepo) DisableTotp(ctx context.Context, userID int64) error {
	return r.client.User.UpdateOneID(userID).
		SetTotpEnabled(false).
		ClearTotpSecretEncrypted().
		ClearTotpEnabledAt().
		Exec(ctx)
REDACTED

func (r *oauthPendingFlowUserRepo) GetByIDIncludeDeleted(ctx context.Context, id int64) (*service.User, error) {
	return r.GetByID(ctx, id)
REDACTED

func oauthPendingFlowServiceUser(entity *dbent.User) *service.User {
	if entity == nil {
		return nil
REDACTED
	return &service.User{
		ID:                  entity.ID,
		Email:               entity.Email,
		Username:            entity.Username,
		Notes:               entity.Notes,
		PasswordHash:        entity.PasswordHash,
		Role:                entity.Role,
		Balance:             entity.Balance,
		Concurrency:         entity.Concurrency,
		Status:              entity.Status,
		SignupSource:        entity.SignupSource,
		LastLoginAt:         entity.LastLoginAt,
		LastActiveAt:        entity.LastActiveAt,
		TotpSecretEncrypted: entity.TotpSecretEncrypted,
		TotpEnabled:         entity.TotpEnabled,
		TotpEnabledAt:       entity.TotpEnabledAt,
		TotalRecharged:      entity.TotalRecharged,
		CreatedAt:           entity.CreatedAt,
		UpdatedAt:           entity.UpdatedAt,
REDACTED
REDACTED

type oauthPendingFlowDefaultSubAssignerStub struct {
	calls []service.AssignSubscriptionInput
REDACTED

func (s *oauthPendingFlowDefaultSubAssignerStub) AssignOrExtendSubscription(
	_ context.Context,
	input *service.AssignSubscriptionInput,
) (*service.UserSubscription, bool, error) {
	if input != nil {
		s.calls = append(s.calls, *input)
REDACTED
	return nil, false, nil
REDACTED

type oauthPendingFlowTotpCacheStub struct {
	setupSessions  map[int64]*service.TotpSetupSession
	loginSessions  map[string]*service.TotpLoginSession
	verifyAttempts map[int64]int
REDACTED

func (s *oauthPendingFlowTotpCacheStub) GetSetupSession(_ context.Context, userID int64) (*service.TotpSetupSession, error) {
	if s == nil || s.setupSessions == nil {
		return nil, nil
REDACTED
	return s.setupSessions[userID], nil
REDACTED

func (s *oauthPendingFlowTotpCacheStub) SetSetupSession(_ context.Context, userID int64, session *service.TotpSetupSession, _ time.Duration) error {
	if s.setupSessions == nil {
		s.setupSessions = map[int64]*service.TotpSetupSession{REDACTED
REDACTED
	s.setupSessions[userID] = session
	return nil
REDACTED

func (s *oauthPendingFlowTotpCacheStub) DeleteSetupSession(_ context.Context, userID int64) error {
	delete(s.setupSessions, userID)
	return nil
REDACTED

func (s *oauthPendingFlowTotpCacheStub) GetLoginSession(_ context.Context, tempToken string) (*service.TotpLoginSession, error) {
	if s == nil || s.loginSessions == nil {
		return nil, nil
REDACTED
	return s.loginSessions[tempToken], nil
REDACTED

func (s *oauthPendingFlowTotpCacheStub) SetLoginSession(_ context.Context, tempToken string, session *service.TotpLoginSession, _ time.Duration) error {
	if s.loginSessions == nil {
		s.loginSessions = map[string]*service.TotpLoginSession{REDACTED
REDACTED
	s.loginSessions[tempToken] = session
	return nil
REDACTED

func (s *oauthPendingFlowTotpCacheStub) DeleteLoginSession(_ context.Context, tempToken string) error {
	delete(s.loginSessions, tempToken)
	return nil
REDACTED

func (s *oauthPendingFlowTotpCacheStub) IncrementVerifyAttempts(_ context.Context, userID int64) (int, error) {
	if s.verifyAttempts == nil {
		s.verifyAttempts = map[int64]int{REDACTED
REDACTED
	s.verifyAttempts[userID]++
	return s.verifyAttempts[userID], nil
REDACTED

func (s *oauthPendingFlowTotpCacheStub) GetVerifyAttempts(_ context.Context, userID int64) (int, error) {
	if s == nil || s.verifyAttempts == nil {
		return 0, nil
REDACTED
	return s.verifyAttempts[userID], nil
REDACTED

func (s *oauthPendingFlowTotpCacheStub) ClearVerifyAttempts(_ context.Context, userID int64) error {
	delete(s.verifyAttempts, userID)
	return nil
REDACTED

type oauthPendingFlowTotpEncryptorStub struct{REDACTED

func (oauthPendingFlowTotpEncryptorStub) Encrypt(plaintext string) (string, error) {
	return plaintext, nil
REDACTED

func (oauthPendingFlowTotpEncryptorStub) Decrypt(ciphertext string) (string, error) {
	return ciphertext, nil
REDACTED
