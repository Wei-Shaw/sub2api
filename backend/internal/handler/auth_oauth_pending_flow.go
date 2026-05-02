package handler

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	dbent "github.com/Wei-Shaw/sub2api/ent"
	"github.com/Wei-Shaw/sub2api/ent/authidentity"
	"github.com/Wei-Shaw/sub2api/ent/authidentitychannel"
	"github.com/Wei-Shaw/sub2api/ent/identityadoptiondecision"
	"github.com/Wei-Shaw/sub2api/ent/predicate"
	dbuser "github.com/Wei-Shaw/sub2api/ent/user"
	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
	"github.com/Wei-Shaw/sub2api/internal/pkg/ip"
	"github.com/Wei-Shaw/sub2api/internal/pkg/oauth"
	"github.com/Wei-Shaw/sub2api/internal/pkg/response"
	"github.com/Wei-Shaw/sub2api/internal/service"

	entsql "entgo.io/ent/dialect/sql"
	"github.com/gin-gonic/gin"
)

const (
	oauthPendingBrowserCookiePath = "/api/v1/auth/oauth"
	oauthPendingBrowserCookieName = "oauth_pending_browser_session"
	oauthPendingSessionCookiePath = "/api/v1/auth/oauth"
	oauthPendingSessionCookieName = "oauth_pending_session"
	oauthPendingCookieMaxAgeSec   = 10 * 60
	oauthPendingChoiceStep        = "choose_account_action_required"

	oauthCompletionResponseKey = "completion_response"
)

var pendingOAuthCreateAccountPreCommitHook func(context.Context, *dbent.PendingAuthSession) error

type oauthPendingSessionPayload struct {
	Intent                 string
	Identity               service.PendingAuthIdentityKey
	TargetUserID           *int64
	ResolvedEmail          string
	RedirectTo             string
	BrowserSessionKey      string
	UpstreamIdentityClaims map[string]any
	CompletionResponse     map[string]any
}

type oauthAdoptionDecisionRequest struct {
	AdoptDisplayName *bool `json:"adopt_display_name,omitempty"`
	AdoptAvatar      *bool `json:"adopt_avatar,omitempty"`
}

type bindPendingOAuthLoginRequest struct {
	Email            string `json:"email" binding:"required,email"`
	Password         string `json:"password" binding:"required"`
	AdoptDisplayName *bool  `json:"adopt_display_name,omitempty"`
	AdoptAvatar      *bool  `json:"adopt_avatar,omitempty"`
}

type createPendingOAuthAccountRequest struct {
	Email            string `json:"email" binding:"required,email"`
	VerifyCode       string `json:"verify_code,omitempty"`
	Password         string `json:"password" binding:"required,min=6"`
	InvitationCode   string `json:"invitation_code,omitempty"`
	AdoptDisplayName *bool  `json:"adopt_display_name,omitempty"`
	AdoptAvatar      *bool  `json:"adopt_avatar,omitempty"`
}

type sendPendingOAuthVerifyCodeRequest struct {
	Email             string `json:"email" binding:"required,email"`
	TurnstileToken    string `json:"turnstile_token,omitempty"`
	PendingAuthToken  string `json:"pending_auth_token,omitempty"`
	PendingOAuthToken string `json:"pending_oauth_token,omitempty"`
}

func (r bindPendingOAuthLoginRequest) adoptionDecision() oauthAdoptionDecisionRequest {
	return oauthAdoptionDecisionRequest{
		AdoptDisplayName: r.AdoptDisplayName,
		AdoptAvatar:      r.AdoptAvatar,
	}
}

func (r createPendingOAuthAccountRequest) adoptionDecision() oauthAdoptionDecisionRequest {
	return oauthAdoptionDecisionRequest{
		AdoptDisplayName: r.AdoptDisplayName,
		AdoptAvatar:      r.AdoptAvatar,
	}
}

func (h *AuthHandler) pendingIdentityService() (*service.AuthPendingIdentityService, error) {
	if h == nil || h.authService == nil || h.authService.EntClient() == nil {
		return nil, infraerrors.ServiceUnavailable("PENDING_AUTH_NOT_READY", "pending auth service is not ready")
	}
	return service.NewAuthPendingIdentityService(h.authService.EntClient()), nil
}

func generateOAuthPendingBrowserSession() (string, error) {
	return oauth.GenerateState()
}

func setOAuthPendingBrowserCookie(c *gin.Context, sessionKey string, secure bool) {
	http.SetCookie(c.Writer, &http.Cookie{
		Name:     oauthPendingBrowserCookieName,
		Value:    encodeCookieValue(sessionKey),
		Path:     oauthPendingBrowserCookiePath,
		MaxAge:   oauthPendingCookieMaxAgeSec,
		HttpOnly: true,
		Secure:   secure,
		SameSite: http.SameSiteLaxMode,
	})
}

func clearOAuthPendingBrowserCookie(c *gin.Context, secure bool) {
	http.SetCookie(c.Writer, &http.Cookie{
		Name:     oauthPendingBrowserCookieName,
		Value:    "",
		Path:     oauthPendingBrowserCookiePath,
		MaxAge:   -1,
		HttpOnly: true,
		Secure:   secure,
		SameSite: http.SameSiteLaxMode,
	})
}

func readOAuthPendingBrowserCookie(c *gin.Context) (string, error) {
	return readCookieDecoded(c, oauthPendingBrowserCookieName)
}

func setOAuthPendingSessionCookie(c *gin.Context, sessionToken string, secure bool) {
	http.SetCookie(c.Writer, &http.Cookie{
		Name:     oauthPendingSessionCookieName,
		Value:    encodeCookieValue(sessionToken),
		Path:     oauthPendingSessionCookiePath,
		MaxAge:   oauthPendingCookieMaxAgeSec,
		HttpOnly: true,
		Secure:   secure,
		SameSite: http.SameSiteLaxMode,
	})
}

func clearOAuthPendingSessionCookie(c *gin.Context, secure bool) {
	http.SetCookie(c.Writer, &http.Cookie{
		Name:     oauthPendingSessionCookieName,
		Value:    "",
		Path:     oauthPendingSessionCookiePath,
		MaxAge:   -1,
		HttpOnly: true,
		Secure:   secure,
		SameSite: http.SameSiteLaxMode,
	})
}

func readOAuthPendingSessionCookie(c *gin.Context) (string, error) {
	return readCookieDecoded(c, oauthPendingSessionCookieName)
}

func redirectToFrontendCallback(c *gin.Context, frontendCallback string) {
	u, err := url.Parse(frontendCallback)
	if err != nil {
		c.Redirect(http.StatusFound, linuxDoOAuthDefaultRedirectTo)
		return
	}
	if u.Scheme != "" && !strings.EqualFold(u.Scheme, "http") && !strings.EqualFold(u.Scheme, "https") {
		c.Redirect(http.StatusFound, linuxDoOAuthDefaultRedirectTo)
		return
	}
	u.Fragment = ""
	c.Header("Cache-Control", "no-store")
	c.Header("Pragma", "no-cache")
	c.Redirect(http.StatusFound, u.String())
}

func (h *AuthHandler) createOAuthPendingSession(c *gin.Context, payload oauthPendingSessionPayload) error {
	svc, err := h.pendingIdentityService()
	if err != nil {
		return err
	}

	session, err := svc.CreatePendingSession(c.Request.Context(), service.CreatePendingAuthSessionInput{
		Intent:                 strings.TrimSpace(payload.Intent),
		Identity:               payload.Identity,
		TargetUserID:           payload.TargetUserID,
		ResolvedEmail:          strings.TrimSpace(payload.ResolvedEmail),
		RedirectTo:             strings.TrimSpace(payload.RedirectTo),
		BrowserSessionKey:      strings.TrimSpace(payload.BrowserSessionKey),
		UpstreamIdentityClaims: payload.UpstreamIdentityClaims,
		LocalFlowState: map[string]any{
			oauthCompletionResponseKey: payload.CompletionResponse,
		},
	})
	if err != nil {
		return infraerrors.InternalServer("PENDING_AUTH_SESSION_CREATE_FAILED", "failed to create pending auth session").WithCause(err)
	}

	setOAuthPendingSessionCookie(c, session.SessionToken, isRequestHTTPS(c))
	return nil
}

func readCompletionResponse(session map[string]any) (map[string]any, bool) {
	if len(session) == 0 {
		return nil, false
	}
	value, ok := session[oauthCompletionResponseKey]
	if !ok {
		return nil, false
	}
	result, ok := value.(map[string]any)
	if !ok {
		return nil, false
	}
	return result, true
}

func clonePendingMap(values map[string]any) map[string]any {
	if len(values) == 0 {
		return map[string]any{}
	}
	cloned := make(map[string]any, len(values))
	for key, value := range values {
		cloned[key] = value
	}
	return cloned
}

func mergePendingCompletionResponse(session *dbent.PendingAuthSession, overrides map[string]any) map[string]any {
	payload, _ := readCompletionResponse(session.LocalFlowState)
	merged := clonePendingMap(payload)
	if strings.TrimSpace(session.RedirectTo) != "" {
		if _, exists := merged["redirect"]; !exists {
			merged["redirect"] = session.RedirectTo
		}
	}
	for key, value := range overrides {
		if value == nil {
			delete(merged, key)
			continue
		}
		merged[key] = value
	}
	applySuggestedProfileToCompletionResponse(merged, session.UpstreamIdentityClaims)
	return merged
}

func pendingSessionStringValue(values map[string]any, key string) string {
	if len(values) == 0 {
		return ""
	}
	raw, ok := values[key]
	if !ok {
		return ""
	}
	value, ok := raw.(string)
	if !ok {
		return ""
	}
	return strings.TrimSpace(value)
}

func pendingSessionWantsInvitation(payload map[string]any) bool {
	return strings.EqualFold(strings.TrimSpace(pendingSessionStringValue(payload, "error")), "invitation_required")
}

func pendingOAuthCompletionCanIssueTokenPair(session *dbent.PendingAuthSession, payload map[string]any) bool {
	if session == nil {
		return false
	}
	if !strings.EqualFold(strings.TrimSpace(session.Intent), oauthIntentLogin) {
		return false
	}
	if session.TargetUserID == nil || *session.TargetUserID <= 0 {
		return false
	}
	if pendingSessionWantsInvitation(payload) {
		return false
	}
	return strings.TrimSpace(pendingSessionStringValue(payload, "step")) == ""
}

func ensurePendingOAuthCompleteRegistrationSession(session *dbent.PendingAuthSession) error {
	if session == nil {
		return infraerrors.BadRequest("PENDING_AUTH_SESSION_INVALID", "pending auth registration context is invalid")
	}
	if strings.TrimSpace(session.Intent) != oauthIntentLogin {
		return infraerrors.BadRequest("PENDING_AUTH_SESSION_INVALID", "pending auth registration context is invalid")
	}
	if session.TargetUserID != nil && *session.TargetUserID > 0 {
		return infraerrors.BadRequest("PENDING_AUTH_SESSION_INVALID", "pending auth registration context is invalid")
	}
	payload, _ := readCompletionResponse(session.LocalFlowState)
	if strings.EqualFold(strings.TrimSpace(pendingSessionStringValue(payload, "step")), "bind_login_required") {
		return infraerrors.BadRequest("PENDING_AUTH_SESSION_INVALID", "pending auth registration context is invalid")
	}
	return nil
}

func buildLegacyCompleteRegistrationPendingResponse(
	session *dbent.PendingAuthSession,
	forceEmailOnSignup bool,
	emailVerificationRequired bool,
) map[string]any {
	completionResponse := normalizePendingOAuthCompletionResponse(mergePendingCompletionResponse(session, map[string]any{
		"step":                   oauthPendingChoiceStep,
		"adoption_required":      true,
		"create_account_allowed": true,
		"force_email_on_signup":  forceEmailOnSignup,
	}))

	if email := strings.TrimSpace(session.ResolvedEmail); email != "" {
		if _, exists := completionResponse["email"]; !exists {
			completionResponse["email"] = email
		}
		if _, exists := completionResponse["resolved_email"]; !exists {
			completionResponse["resolved_email"] = email
		}
	}
	if _, exists := completionResponse["choice_reason"]; !exists {
		switch {
		case forceEmailOnSignup:
			completionResponse["choice_reason"] = "force_email_on_signup"
		case emailVerificationRequired:
			completionResponse["choice_reason"] = "email_verification_required"
		default:
			completionResponse["choice_reason"] = "third_party_signup"
		}
	}
	return completionResponse
}

func (h *AuthHandler) legacyCompleteRegistrationSessionStatus(
	c *gin.Context,
	session *dbent.PendingAuthSession,
) (*dbent.PendingAuthSession, bool, error) {
	if session == nil {
		return nil, false, infraerrors.BadRequest("PENDING_AUTH_SESSION_INVALID", "pending auth registration context is invalid")
	}

	payload := normalizePendingOAuthCompletionResponse(mergePendingCompletionResponse(session, nil))
	if step := pendingSessionStringValue(payload, "step"); step != "" {
		return session, true, nil
	}

	emailVerificationRequired := h != nil && h.authService != nil && h.authService.IsEmailVerifyEnabled(c.Request.Context())
	forceEmailOnSignup := h.isForceEmailOnThirdPartySignup(c.Request.Context())
	if !emailVerificationRequired && !forceEmailOnSignup {
		return session, false, nil
	}

	client := h.entClient()
	if client == nil {
		return nil, false, infraerrors.ServiceUnavailable("PENDING_AUTH_NOT_READY", "pending auth service is not ready")
	}

	updatedSession, err := updatePendingOAuthSessionProgress(
		c.Request.Context(),
		client,
		session,
		strings.TrimSpace(session.Intent),
		strings.TrimSpace(session.ResolvedEmail),
		nil,
		buildLegacyCompleteRegistrationPendingResponse(session, forceEmailOnSignup, emailVerificationRequired),
	)
	if err != nil {
		return nil, false, infraerrors.InternalServer("PENDING_AUTH_SESSION_UPDATE_FAILED", "failed to update pending oauth session").WithCause(err)
	}
	return updatedSession, true, nil
}

func (r oauthAdoptionDecisionRequest) hasDecision() bool {
	return r.AdoptDisplayName != nil || r.AdoptAvatar != nil
}

func bindOptionalOAuthAdoptionDecision(c *gin.Context) (oauthAdoptionDecisionRequest, error) {
	var req oauthAdoptionDecisionRequest
	if c == nil || c.Request == nil || c.Request.Body == nil {
		return req, nil
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		if errors.Is(err, io.EOF) {
			return req, nil
		}
		return req, err
	}
	return req, nil
}

func cloneOAuthMetadata(values map[string]any) map[string]any {
	if len(values) == 0 {
		return map[string]any{}
	}
	cloned := make(map[string]any, len(values))
	for key, value := range values {
		cloned[key] = value
	}
	return cloned
}

func mergeOAuthMetadata(base map[string]any, overlay map[string]any) map[string]any {
	merged := cloneOAuthMetadata(base)
	for key, value := range overlay {
		merged[key] = value
	}
	return merged
}

func normalizeAdoptedOAuthDisplayName(value string) string {
	value = strings.TrimSpace(value)
	if len([]rune(value)) > 100 {
		value = string([]rune(value)[:100])
	}
	return value
}

func (h *AuthHandler) entClient() *dbent.Client {
	if h == nil || h.authService == nil {
		return nil
	}
	return h.authService.EntClient()
}

func (h *AuthHandler) isForceEmailOnThirdPartySignup(ctx context.Context) bool {
	if h == nil || h.settingSvc == nil {
		return false
	}
	defaults, err := h.settingSvc.GetAuthSourceDefaultSettings(ctx)
	if err != nil || defaults == nil {
		return false
	}
	return defaults.ForceEmailOnThirdPartySignup
}

func (h *AuthHandler) findOAuthIdentityUser(ctx context.Context, identity service.PendingAuthIdentityKey) (*dbent.User, error) {
	client := h.entClient()
	if client == nil {
		return nil, infraerrors.ServiceUnavailable("PENDING_AUTH_NOT_READY", "pending auth service is not ready")
	}

	record, err := client.AuthIdentity.Query().
		Where(
			authidentity.ProviderTypeEQ(strings.TrimSpace(identity.ProviderType)),
			authidentity.ProviderKeyEQ(strings.TrimSpace(identity.ProviderKey)),
			authidentity.ProviderSubjectEQ(strings.TrimSpace(identity.ProviderSubject)),
		).
		Only(ctx)
	if err != nil {
		if dbent.IsNotFound(err) {
			return nil, nil
		}
		return nil, infraerrors.InternalServer("AUTH_IDENTITY_LOOKUP_FAILED", "failed to inspect auth identity ownership").WithCause(err)
	}
	return findActiveUserByID(ctx, client, record.UserID)
}

func (h *AuthHandler) BindLinuxDoOAuthLogin(c *gin.Context) { h.bindPendingOAuthLogin(c, "linuxdo") }
func (h *AuthHandler) BindOIDCOAuthLogin(c *gin.Context)    { h.bindPendingOAuthLogin(c, "oidc") }
func (h *AuthHandler) BindWeChatOAuthLogin(c *gin.Context)  { h.bindPendingOAuthLogin(c, "wechat") }
func (h *AuthHandler) BindPendingOAuthLogin(c *gin.Context) { h.bindPendingOAuthLogin(c, "") }

func (h *AuthHandler) CreateLinuxDoOAuthAccount(c *gin.Context) {
	h.createPendingOAuthAccount(c, "linuxdo")
}

func (h *AuthHandler) CreateOIDCOAuthAccount(c *gin.Context) { h.createPendingOAuthAccount(c, "oidc") }

func (h *AuthHandler) CreateWeChatOAuthAccount(c *gin.Context) {
	h.createPendingOAuthAccount(c, "wechat")
}

func (h *AuthHandler) CreatePendingOAuthAccount(c *gin.Context) {
	h.createPendingOAuthAccount(c, "")
}

// SendPendingOAuthVerifyCode sends a verification code for a browser-bound
// pending OAuth account-creation flow.
// POST /api/v1/auth/oauth/pending/send-verify-code
func (h *AuthHandler) SendPendingOAuthVerifyCode(c *gin.Context) {
	var req sendPendingOAuthVerifyCodeRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "Invalid request: "+err.Error())
		return
	}

	if err := h.authService.VerifyTurnstile(c.Request.Context(), req.TurnstileToken, ip.GetClientIP(c)); err != nil {
		response.ErrorFrom(c, err)
		return
	}

	_, session, _, err := readPendingOAuthBrowserSession(c, h)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	if err := ensurePendingOAuthCompleteRegistrationSession(session); err != nil {
		response.ErrorFrom(c, err)
		return
	}

	client := h.entClient()
	if client == nil {
		response.ErrorFrom(c, infraerrors.ServiceUnavailable("PENDING_AUTH_NOT_READY", "pending auth service is not ready"))
		return
	}

	email := strings.TrimSpace(strings.ToLower(req.Email))
	if existingUser, err := findUserByNormalizedEmail(c.Request.Context(), client, email); err == nil && existingUser != nil {
		session, err = h.transitionPendingOAuthAccountToChoiceState(c, client, session, existingUser, email)
		if err != nil {
			response.ErrorFrom(c, err)
			return
		}
		c.JSON(http.StatusOK, buildPendingOAuthSessionStatusPayload(session))
		return
	} else if err != nil && !errors.Is(err, service.ErrUserNotFound) {
		response.ErrorFrom(c, err)
		return
	}

	result, err := h.authService.SendPendingOAuthVerifyCode(c.Request.Context(), req.Email)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}

	response.Success(c, SendVerifyCodeResponse{
		Message:   "Verification code sent successfully",
		Countdown: result.Countdown,
	})
}

func (h *AuthHandler) upsertPendingOAuthAdoptionDecision(
	c *gin.Context,
	sessionID int64,
	req oauthAdoptionDecisionRequest,
) (*dbent.IdentityAdoptionDecision, error) {
	client := h.entClient()
	if client == nil {
		return nil, infraerrors.ServiceUnavailable("PENDING_AUTH_NOT_READY", "pending auth service is not ready")
	}

	existing, err := client.IdentityAdoptionDecision.Query().
		Where(identityadoptiondecision.PendingAuthSessionIDEQ(sessionID)).
		Only(c.Request.Context())
	if err != nil && !dbent.IsNotFound(err) {
		return nil, infraerrors.InternalServer("PENDING_AUTH_ADOPTION_LOAD_FAILED", "failed to load oauth profile adoption decision").WithCause(err)
	}
	if existing != nil && !req.hasDecision() {
		return existing, nil
	}
	if existing == nil && !req.hasDecision() {
		return nil, nil
	}

	input := service.PendingIdentityAdoptionDecisionInput{
		PendingAuthSessionID: sessionID,
	}
	if existing != nil {
		input.AdoptDisplayName = existing.AdoptDisplayName
		input.AdoptAvatar = existing.AdoptAvatar
		input.IdentityID = existing.IdentityID
	}
	if req.AdoptDisplayName != nil {
		input.AdoptDisplayName = *req.AdoptDisplayName
	}
	if req.AdoptAvatar != nil {
		input.AdoptAvatar = *req.AdoptAvatar
	}

	svc, err := h.pendingIdentityService()
	if err != nil {
		return nil, err
	}
	decision, err := svc.UpsertAdoptionDecision(c.Request.Context(), input)
	if err != nil {
		return nil, infraerrors.InternalServer("PENDING_AUTH_ADOPTION_SAVE_FAILED", "failed to save oauth profile adoption decision").WithCause(err)
	}
	return decision, nil
}

func (h *AuthHandler) ensurePendingOAuthAdoptionDecision(
	c *gin.Context,
	sessionID int64,
	req oauthAdoptionDecisionRequest,
) (*dbent.IdentityAdoptionDecision, error) {
	decision, err := h.upsertPendingOAuthAdoptionDecision(c, sessionID, req)
	if err != nil {
		return nil, err
	}
	if decision != nil {
		return decision, nil
	}

	svc, err := h.pendingIdentityService()
	if err != nil {
		return nil, err
	}
	decision, err = svc.UpsertAdoptionDecision(c.Request.Context(), service.PendingIdentityAdoptionDecisionInput{
		PendingAuthSessionID: sessionID,
	})
	if err != nil {
		return nil, infraerrors.InternalServer("PENDING_AUTH_ADOPTION_SAVE_FAILED", "failed to save oauth profile adoption decision").WithCause(err)
	}
	return decision, nil
}

func updatePendingOAuthSessionProgress(
	ctx context.Context,
	client *dbent.Client,
	session *dbent.PendingAuthSession,
	intent string,
	resolvedEmail string,
	targetUserID *int64,
	completionResponse map[string]any,
) (*dbent.PendingAuthSession, error) {
	if client == nil || session == nil {
		return nil, infraerrors.BadRequest("PENDING_AUTH_SESSION_INVALID", "pending auth session is invalid")
	}

	localFlowState := clonePendingMap(session.LocalFlowState)
	localFlowState[oauthCompletionResponseKey] = clonePendingMap(completionResponse)

	update := client.PendingAuthSession.UpdateOneID(session.ID).
		SetIntent(strings.TrimSpace(intent)).
		SetResolvedEmail(strings.TrimSpace(resolvedEmail)).
		SetLocalFlowState(localFlowState)
	if targetUserID != nil && *targetUserID > 0 {
		update = update.SetTargetUserID(*targetUserID)
	} else {
		update = update.ClearTargetUserID()
	}
	return update.Save(ctx)
}

func resolvePendingOAuthTargetUserID(ctx context.Context, client *dbent.Client, session *dbent.PendingAuthSession) (int64, error) {
	if session == nil {
		return 0, infraerrors.BadRequest("PENDING_AUTH_SESSION_INVALID", "pending auth session is invalid")
	}
	if session.TargetUserID != nil && *session.TargetUserID > 0 {
		return *session.TargetUserID, nil
	}
	email := strings.TrimSpace(session.ResolvedEmail)
	if email == "" {
		return 0, infraerrors.BadRequest("PENDING_AUTH_TARGET_USER_MISSING", "pending auth target user is missing")
	}

	userEntity, err := findUserByNormalizedEmail(ctx, client, email)
	if err != nil {
		if errors.Is(err, service.ErrUserNotFound) {
			return 0, infraerrors.InternalServer("PENDING_AUTH_TARGET_USER_NOT_FOUND", "pending auth target user was not found")
		}
		return 0, err
	}
	return userEntity.ID, nil
}

func userNormalizedEmailPredicate(email string) predicate.User {
	normalized := strings.ToLower(strings.TrimSpace(email))
	if normalized == "" {
		return dbuser.EmailEQ(email)
	}
	return predicate.User(func(s *entsql.Selector) {
		s.Where(entsql.P(func(b *entsql.Builder) {
			b.WriteString("LOWER(TRIM(").
				Ident(s.C(dbuser.FieldEmail)).
				WriteString(")) = ").
				Arg(normalized)
		}))
	})
}

func findUserByNormalizedEmail(ctx context.Context, client *dbent.Client, email string) (*dbent.User, error) {
	if client == nil {
		return nil, infraerrors.ServiceUnavailable("PENDING_AUTH_NOT_READY", "pending auth service is not ready")
	}

	matches, err := client.User.Query().
		Where(userNormalizedEmailPredicate(email)).
		Order(dbent.Asc(dbuser.FieldID)).
		All(ctx)
	if err != nil {
		return nil, err
	}
	if len(matches) == 0 {
		return nil, service.ErrUserNotFound
	}
	if len(matches) > 1 {
		return nil, infraerrors.Conflict("USER_EMAIL_CONFLICT", "normalized email matched multiple users")
	}
	return matches[0], nil
}

func ensurePendingOAuthRegistrationIdentityAvailable(ctx context.Context, client *dbent.Client, session *dbent.PendingAuthSession) error {
	if client == nil || session == nil {
		return infraerrors.BadRequest("PENDING_AUTH_SESSION_INVALID", "pending auth registration context is invalid")
	}

	identity, err := client.AuthIdentity.Query().
		Where(
			authidentity.ProviderTypeEQ(strings.TrimSpace(session.ProviderType)),
			authidentity.ProviderKeyEQ(strings.TrimSpace(session.ProviderKey)),
			authidentity.ProviderSubjectEQ(strings.TrimSpace(session.ProviderSubject)),
		).
		Only(ctx)
	if err != nil {
		if dbent.IsNotFound(err) {
			return nil
		}
		return err
	}
	if identity == nil || identity.UserID <= 0 {
		return nil
	}

	activeOwner, err := findActiveUserByID(ctx, client, identity.UserID)
	if err != nil {
		return err
	}
	if activeOwner != nil {
		return infraerrors.Conflict("AUTH_IDENTITY_OWNERSHIP_CONFLICT", "auth identity already belongs to another user")
	}
	return nil
}

func oauthIdentityIssuer(session *dbent.PendingAuthSession) *string {
	if session == nil {
		return nil
	}
	switch strings.TrimSpace(session.ProviderType) {
	case "oidc":
		issuer := strings.TrimSpace(session.ProviderKey)
		if issuer == "" {
			issuer = pendingSessionStringValue(session.UpstreamIdentityClaims, "issuer")
		}
		if issuer == "" {
			return nil
		}
		return &issuer
	default:
		issuer := pendingSessionStringValue(session.UpstreamIdentityClaims, "issuer")
		if issuer == "" {
			return nil
		}
		return &issuer
	}
}

func ensurePendingOAuthIdentityForUser(ctx context.Context, tx *dbent.Tx, session *dbent.PendingAuthSession, userID int64) (*dbent.AuthIdentity, error) {
	if session != nil && strings.EqualFold(strings.TrimSpace(session.ProviderType), "wechat") {
		return ensurePendingWeChatOAuthIdentityForUser(ctx, tx, session, userID)
	}

	client := tx.Client()
	identity, err := client.AuthIdentity.Query().
		Where(
			authidentity.ProviderTypeEQ(strings.TrimSpace(session.ProviderType)),
			authidentity.ProviderKeyEQ(strings.TrimSpace(session.ProviderKey)),
			authidentity.ProviderSubjectEQ(strings.TrimSpace(session.ProviderSubject)),
		).
		Only(ctx)
	if err != nil && !dbent.IsNotFound(err) {
		return nil, err
	}
	if identity != nil {
		if identity.UserID != userID {
			activeOwner, err := findActiveUserByID(ctx, client, identity.UserID)
			if err != nil {
				return nil, err
			}
			if activeOwner != nil {
				return nil, infraerrors.Conflict("AUTH_IDENTITY_OWNERSHIP_CONFLICT", "auth identity already belongs to another user")
			}
			return client.AuthIdentity.UpdateOneID(identity.ID).
				SetUserID(userID).
				Save(ctx)
		}
		return identity, nil
	}

	create := client.AuthIdentity.Create().
		SetUserID(userID).
		SetProviderType(strings.TrimSpace(session.ProviderType)).
		SetProviderKey(strings.TrimSpace(session.ProviderKey)).
		SetProviderSubject(strings.TrimSpace(session.ProviderSubject)).
		SetMetadata(cloneOAuthMetadata(session.UpstreamIdentityClaims))
	if issuer := oauthIdentityIssuer(session); issuer != nil {
		create = create.SetIssuer(strings.TrimSpace(*issuer))
	}
	return create.Save(ctx)
}

func ensurePendingWeChatOAuthIdentityForUser(ctx context.Context, tx *dbent.Tx, session *dbent.PendingAuthSession, userID int64) (*dbent.AuthIdentity, error) {
	client := tx.Client()
	providerType := strings.TrimSpace(session.ProviderType)
	providerKey := strings.TrimSpace(session.ProviderKey)
	providerSubject := strings.TrimSpace(session.ProviderSubject)
	providerKeys := wechatCompatibleProviderKeys(providerKey)
	channel := strings.TrimSpace(pendingSessionStringValue(session.UpstreamIdentityClaims, "channel"))
	channelAppID := strings.TrimSpace(pendingSessionStringValue(session.UpstreamIdentityClaims, "channel_app_id"))
	channelSubject := strings.TrimSpace(pendingSessionStringValue(session.UpstreamIdentityClaims, "channel_subject"))
	metadata := cloneOAuthMetadata(session.UpstreamIdentityClaims)

	identityRecords, err := client.AuthIdentity.Query().
		Where(
			authidentity.ProviderTypeEQ(providerType),
			authidentity.ProviderKeyIn(providerKeys...),
			authidentity.ProviderSubjectEQ(providerSubject),
		).
		All(ctx)
	if err != nil {
		return nil, err
	}
	identity, hasCanonicalKey, err := chooseWeChatIdentityForUser(ctx, client, identityRecords, userID, providerKey)
	if err != nil {
		return nil, err
	}

	var legacyOpenIDIdentity *dbent.AuthIdentity
	if channelSubject != "" && channelSubject != providerSubject {
		legacyOpenIDRecords, err := client.AuthIdentity.Query().
			Where(
				authidentity.ProviderTypeEQ(providerType),
				authidentity.ProviderKeyIn(providerKeys...),
				authidentity.ProviderSubjectEQ(channelSubject),
			).
			All(ctx)
		if err != nil {
			return nil, err
		}
		legacyOpenIDIdentity, _, err = chooseWeChatIdentityForUser(ctx, client, legacyOpenIDRecords, userID, providerKey)
		if err != nil {
			return nil, err
		}
	}

	switch {
	case identity != nil:
		update := client.AuthIdentity.UpdateOneID(identity.ID).
			SetMetadata(mergeOAuthMetadata(identity.Metadata, metadata))
		if identity.UserID != userID {
			update = update.SetUserID(userID)
		}
		if !strings.EqualFold(strings.TrimSpace(identity.ProviderKey), providerKey) && !hasCanonicalKey {
			update = update.SetProviderKey(providerKey)
		}
		if issuer := oauthIdentityIssuer(session); issuer != nil {
			update = update.SetIssuer(strings.TrimSpace(*issuer))
		}
		identity, err = update.Save(ctx)
		if err != nil {
			return nil, err
		}
	case legacyOpenIDIdentity != nil:
		update := client.AuthIdentity.UpdateOneID(legacyOpenIDIdentity.ID).
			SetProviderKey(providerKey).
			SetProviderSubject(providerSubject).
			SetMetadata(mergeOAuthMetadata(legacyOpenIDIdentity.Metadata, metadata))
		if issuer := oauthIdentityIssuer(session); issuer != nil {
			update = update.SetIssuer(strings.TrimSpace(*issuer))
		}
		identity, err = update.Save(ctx)
		if err != nil {
			return nil, err
		}
	default:
		create := client.AuthIdentity.Create().
			SetUserID(userID).
			SetProviderType(providerType).
			SetProviderKey(providerKey).
			SetProviderSubject(providerSubject).
			SetMetadata(metadata)
		if issuer := oauthIdentityIssuer(session); issuer != nil {
			create = create.SetIssuer(strings.TrimSpace(*issuer))
		}
		identity, err = create.Save(ctx)
		if err != nil {
			return nil, err
		}
	}

	if channel == "" || channelAppID == "" || channelSubject == "" {
		return identity, nil
	}

	channelRecords, err := client.AuthIdentityChannel.Query().
		Where(
			authidentitychannel.ProviderTypeEQ(providerType),
			authidentitychannel.ProviderKeyIn(providerKeys...),
			authidentitychannel.ChannelEQ(channel),
			authidentitychannel.ChannelAppIDEQ(channelAppID),
			authidentitychannel.ChannelSubjectEQ(channelSubject),
		).
		WithIdentity().
		All(ctx)
	if err != nil {
		return nil, err
	}
	channelRecord, hasCanonicalChannelKey, err := chooseWeChatChannelForUser(ctx, client, channelRecords, userID, providerKey)
	if err != nil {
		return nil, err
	}

	channelMetadata := mergeOAuthMetadata(channelRecordMetadata(channelRecord), metadata)
	if channelRecord == nil {
		if _, err := client.AuthIdentityChannel.Create().
			SetIdentityID(identity.ID).
			SetProviderType(providerType).
			SetProviderKey(providerKey).
			SetChannel(channel).
			SetChannelAppID(channelAppID).
			SetChannelSubject(channelSubject).
			SetMetadata(channelMetadata).
			Save(ctx); err != nil {
			return nil, err
		}
		return identity, nil
	}

	updateChannel := client.AuthIdentityChannel.UpdateOneID(channelRecord.ID).
		SetIdentityID(identity.ID).
		SetMetadata(channelMetadata)
	if !strings.EqualFold(strings.TrimSpace(channelRecord.ProviderKey), providerKey) && !hasCanonicalChannelKey {
		updateChannel = updateChannel.SetProviderKey(providerKey)
	}
	_, err = updateChannel.Save(ctx)
	if err != nil {
		return nil, err
	}
	return identity, nil
}

func chooseWeChatIdentityForUser(ctx context.Context, client *dbent.Client, records []*dbent.AuthIdentity, userID int64, preferredProviderKey string) (*dbent.AuthIdentity, bool, error) {
	var preferred *dbent.AuthIdentity
	var fallback *dbent.AuthIdentity
	hasCanonicalKey := false
	for _, record := range records {
		if record == nil {
			continue
		}
		if record.UserID != userID {
			activeOwner, err := findActiveUserByID(ctx, client, record.UserID)
			if err != nil {
				return nil, false, err
			}
			if activeOwner != nil {
				return nil, false, infraerrors.Conflict("AUTH_IDENTITY_OWNERSHIP_CONFLICT", "auth identity already belongs to another user")
			}
		}
		if strings.EqualFold(strings.TrimSpace(record.ProviderKey), preferredProviderKey) {
			hasCanonicalKey = true
			if preferred == nil {
				preferred = record
			}
			continue
		}
		if fallback == nil {
			fallback = record
		}
	}
	if preferred != nil {
		return preferred, hasCanonicalKey, nil
	}
	return fallback, hasCanonicalKey, nil
}

func chooseWeChatChannelForUser(ctx context.Context, client *dbent.Client, records []*dbent.AuthIdentityChannel, userID int64, preferredProviderKey string) (*dbent.AuthIdentityChannel, bool, error) {
	var preferred *dbent.AuthIdentityChannel
	var fallback *dbent.AuthIdentityChannel
	hasCanonicalKey := false
	for _, record := range records {
		if record == nil {
			continue
		}
		if record.Edges.Identity != nil && record.Edges.Identity.UserID != userID {
			activeOwner, err := findActiveUserByID(ctx, client, record.Edges.Identity.UserID)
			if err != nil {
				return nil, false, err
			}
			if activeOwner != nil {
				return nil, false, infraerrors.Conflict("AUTH_IDENTITY_CHANNEL_OWNERSHIP_CONFLICT", "auth identity channel already belongs to another user")
			}
		}
		if strings.EqualFold(strings.TrimSpace(record.ProviderKey), preferredProviderKey) {
			hasCanonicalKey = true
			if preferred == nil {
				preferred = record
			}
			continue
		}
		if fallback == nil {
			fallback = record
		}
	}
	if preferred != nil {
		return preferred, hasCanonicalKey, nil
	}
	return fallback, hasCanonicalKey, nil
}

func findActiveUserByID(ctx context.Context, client *dbent.Client, userID int64) (*dbent.User, error) {
	if client == nil || userID <= 0 {
		return nil, nil
	}
	userEntity, err := client.User.Get(ctx, userID)
	if err != nil {
		if dbent.IsNotFound(err) {
			return nil, nil
		}
		return nil, infraerrors.InternalServer("AUTH_IDENTITY_USER_LOOKUP_FAILED", "failed to load auth identity user").WithCause(err)
	}
	if !strings.EqualFold(strings.TrimSpace(userEntity.Status), service.StatusActive) {
		return nil, service.ErrUserNotActive
	}
	return userEntity, nil
}

func channelRecordMetadata(channel *dbent.AuthIdentityChannel) map[string]any {
	if channel == nil {
		return map[string]any{}
	}
	return cloneOAuthMetadata(channel.Metadata)
}

func shouldBindPendingOAuthIdentity(session *dbent.PendingAuthSession, decision *dbent.IdentityAdoptionDecision) bool {
	if session == nil || decision == nil {
		return false
	}
	switch strings.ToLower(strings.TrimSpace(session.Intent)) {
	case "bind_current_user", "login", "adopt_existing_user_by_email":
		return true
	default:
		return decision.AdoptDisplayName || decision.AdoptAvatar
	}
}

func shouldSkipAvatarAdoption(err error) bool {
	return errors.Is(err, service.ErrAvatarInvalid) ||
		errors.Is(err, service.ErrAvatarTooLarge) ||
		errors.Is(err, service.ErrAvatarNotImage)
}

func applyPendingOAuthBinding(
	ctx context.Context,
	client *dbent.Client,
	authService *service.AuthService,
	userService *service.UserService,
	session *dbent.PendingAuthSession,
	decision *dbent.IdentityAdoptionDecision,
	overrideUserID *int64,
	forceBind bool,
	applyFirstBindDefaults bool,
) error {
	if client == nil || session == nil {
		return nil
	}
	if !forceBind && !shouldBindPendingOAuthIdentity(session, decision) {
		return nil
	}

	if tx := dbent.TxFromContext(ctx); tx != nil {
		return applyPendingOAuthBindingTx(ctx, tx, authService, userService, session, decision, overrideUserID, forceBind, applyFirstBindDefaults)
	}

	tx, err := client.Tx(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()

	txCtx := dbent.NewTxContext(ctx, tx)
	if err := applyPendingOAuthBindingTx(txCtx, tx, authService, userService, session, decision, overrideUserID, forceBind, applyFirstBindDefaults); err != nil {
		return err
	}
	return tx.Commit()
}

func applyPendingOAuthBindingTx(
	ctx context.Context,
	tx *dbent.Tx,
	authService *service.AuthService,
	userService *service.UserService,
	session *dbent.PendingAuthSession,
	decision *dbent.IdentityAdoptionDecision,
	overrideUserID *int64,
	forceBind bool,
	applyFirstBindDefaults bool,
) error {
	if tx == nil || session == nil {
		return nil
	}
	if !forceBind && !shouldBindPendingOAuthIdentity(session, decision) {
		return nil
	}

	targetUserID := int64(0)
	if overrideUserID != nil && *overrideUserID > 0 {
		targetUserID = *overrideUserID
	} else {
		resolvedUserID, err := resolvePendingOAuthTargetUserID(ctx, tx.Client(), session)
		if err != nil {
			return err
		}
		targetUserID = resolvedUserID
	}

	adoptedDisplayName := ""
	if decision != nil && decision.AdoptDisplayName {
		adoptedDisplayName = normalizeAdoptedOAuthDisplayName(pendingSessionStringValue(session.UpstreamIdentityClaims, "suggested_display_name"))
	}
	adoptedAvatarURL := ""
	if decision != nil && decision.AdoptAvatar {
		adoptedAvatarURL = pendingSessionStringValue(session.UpstreamIdentityClaims, "suggested_avatar_url")
	}
	shouldAdoptAvatar := false
	if decision != nil && decision.AdoptAvatar && adoptedAvatarURL != "" {
		if err := service.ValidateUserAvatar(adoptedAvatarURL); err == nil {
			shouldAdoptAvatar = true
		} else if !shouldSkipAvatarAdoption(err) {
			return err
		}
	}

	if decision != nil && decision.AdoptDisplayName && adoptedDisplayName != "" {
		if err := tx.Client().User.UpdateOneID(targetUserID).
			SetUsername(adoptedDisplayName).
			Exec(ctx); err != nil {
			return err
		}
	}

	identity, err := ensurePendingOAuthIdentityForUser(ctx, tx, session, targetUserID)
	if err != nil {
		return err
	}

	metadata := cloneOAuthMetadata(identity.Metadata)
	for key, value := range session.UpstreamIdentityClaims {
		metadata[key] = value
	}
	if decision != nil && decision.AdoptDisplayName && adoptedDisplayName != "" {
		metadata["display_name"] = adoptedDisplayName
	}
	if shouldAdoptAvatar {
		metadata["avatar_url"] = adoptedAvatarURL
	}

	updateIdentity := tx.Client().AuthIdentity.UpdateOneID(identity.ID).SetMetadata(metadata)
	if issuer := oauthIdentityIssuer(session); issuer != nil {
		updateIdentity = updateIdentity.SetIssuer(strings.TrimSpace(*issuer))
	}
	if _, err := updateIdentity.Save(ctx); err != nil {
		return err
	}

	if decision != nil && (decision.IdentityID == nil || *decision.IdentityID != identity.ID) {
		if _, err := tx.Client().IdentityAdoptionDecision.Update().
			Where(
				identityadoptiondecision.IdentityIDEQ(identity.ID),
				identityadoptiondecision.IDNEQ(decision.ID),
			).
			ClearIdentityID().
			Save(ctx); err != nil {
			return err
		}
		if _, err := tx.Client().IdentityAdoptionDecision.UpdateOneID(decision.ID).
			SetIdentityID(identity.ID).
			Save(ctx); err != nil {
			return err
		}
	}

	if applyFirstBindDefaults && authService != nil {
		if err := authService.ApplyProviderDefaultSettingsOnFirstBind(ctx, targetUserID, session.ProviderType); err != nil {
			return err
		}
	}

	if shouldAdoptAvatar && userService != nil {
		if _, err := userService.SetAvatar(ctx, targetUserID, adoptedAvatarURL); err != nil {
			return err
		}
	}

	return nil
}

func consumePendingOAuthBrowserSessionTx(
	ctx context.Context,
	tx *dbent.Tx,
	session *dbent.PendingAuthSession,
) error {
	if tx == nil || session == nil {
		return service.ErrPendingAuthSessionNotFound
	}

	storedSession, err := tx.Client().PendingAuthSession.Get(ctx, session.ID)
	if err != nil {
		if dbent.IsNotFound(err) {
			return service.ErrPendingAuthSessionNotFound
		}
		return err
	}

	now := time.Now().UTC()
	if storedSession.ConsumedAt != nil {
		return service.ErrPendingAuthSessionConsumed
	}
	if !storedSession.ExpiresAt.IsZero() && now.After(storedSession.ExpiresAt) {
		return service.ErrPendingAuthSessionExpired
	}
	if strings.TrimSpace(storedSession.BrowserSessionKey) != "" &&
		strings.TrimSpace(storedSession.BrowserSessionKey) != strings.TrimSpace(session.BrowserSessionKey) {
		return service.ErrPendingAuthBrowserMismatch
	}

	if _, err := tx.Client().PendingAuthSession.UpdateOneID(storedSession.ID).
		SetConsumedAt(now).
		SetCompletionCodeHash("").
		ClearCompletionCodeExpiresAt().
		Save(ctx); err != nil {
		return err
	}

	return nil
}

func applyPendingOAuthAdoptionAndConsumeSession(
	ctx context.Context,
	client *dbent.Client,
	authService *service.AuthService,
	userService *service.UserService,
	session *dbent.PendingAuthSession,
	decision *dbent.IdentityAdoptionDecision,
	userID int64,
) error {
	if client == nil {
		return infraerrors.ServiceUnavailable("PENDING_AUTH_NOT_READY", "pending auth service is not ready")
	}
	if session == nil || userID <= 0 {
		return infraerrors.BadRequest("PENDING_AUTH_SESSION_INVALID", "pending auth registration context is invalid")
	}

	tx, err := client.Tx(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()

	txCtx := dbent.NewTxContext(ctx, tx)
	if err := applyPendingOAuthAdoption(txCtx, client, authService, userService, session, decision, &userID); err != nil {
		return err
	}
	if err := consumePendingOAuthBrowserSessionTx(txCtx, tx, session); err != nil {
		return err
	}
	return tx.Commit()
}

func applyPendingOAuthAdoption(
	ctx context.Context,
	client *dbent.Client,
	authService *service.AuthService,
	userService *service.UserService,
	session *dbent.PendingAuthSession,
	decision *dbent.IdentityAdoptionDecision,
	overrideUserID *int64,
) error {
	return applyPendingOAuthBinding(
		ctx,
		client,
		authService,
		userService,
		session,
		decision,
		overrideUserID,
		false,
		strings.EqualFold(strings.TrimSpace(session.Intent), "bind_current_user"),
	)
}

func applySuggestedProfileToCompletionResponse(payload map[string]any, upstream map[string]any) {
	if len(payload) == 0 || len(upstream) == 0 {
		return
	}

	displayName := pendingSessionStringValue(upstream, "suggested_display_name")
	avatarURL := pendingSessionStringValue(upstream, "suggested_avatar_url")

	if displayName != "" {
		if _, exists := payload["suggested_display_name"]; !exists {
			payload["suggested_display_name"] = displayName
		}
	}
	if avatarURL != "" {
		if _, exists := payload["suggested_avatar_url"]; !exists {
			payload["suggested_avatar_url"] = avatarURL
		}
	}
	if displayName != "" || avatarURL != "" {
		payload["adoption_required"] = true
	}
}

func pendingOAuthIdentityExistsForUser(
	ctx context.Context,
	client *dbent.Client,
	session *dbent.PendingAuthSession,
	userID int64,
) (bool, error) {
	if client == nil || session == nil || userID <= 0 {
		return false, nil
	}

	providerType := strings.TrimSpace(session.ProviderType)
	providerKey := strings.TrimSpace(session.ProviderKey)
	providerSubject := strings.TrimSpace(session.ProviderSubject)
	if providerType == "" || providerSubject == "" {
		return false, nil
	}

	query := client.AuthIdentity.Query().
		Where(
			authidentity.ProviderTypeEQ(providerType),
			authidentity.ProviderSubjectEQ(providerSubject),
			authidentity.UserIDEQ(userID),
		)
	if strings.EqualFold(providerType, "wechat") {
		query = query.Where(authidentity.ProviderKeyIn(wechatCompatibleProviderKeys(providerKey)...))
	} else if providerKey != "" {
		query = query.Where(authidentity.ProviderKeyEQ(providerKey))
	}

	count, err := query.Count(ctx)
	if err != nil {
		return false, infraerrors.InternalServer("AUTH_IDENTITY_LOOKUP_FAILED", "failed to inspect auth identity ownership").WithCause(err)
	}
	return count > 0, nil
}

func (h *AuthHandler) shouldSkipPendingOAuthAdoptionPrompt(
	ctx context.Context,
	session *dbent.PendingAuthSession,
	payload map[string]any,
) (bool, error) {
	if session == nil || len(payload) == 0 {
		return false, nil
	}
	if !pendingOAuthCompletionCanIssueTokenPair(session, payload) {
		return false, nil
	}
	if pendingSessionStringValue(session.UpstreamIdentityClaims, "suggested_display_name") == "" &&
		pendingSessionStringValue(session.UpstreamIdentityClaims, "suggested_avatar_url") == "" {
		return false, nil
	}

	return pendingOAuthIdentityExistsForUser(ctx, h.entClient(), session, *session.TargetUserID)
}

func readPendingOAuthBrowserSession(c *gin.Context, h *AuthHandler) (*service.AuthPendingIdentityService, *dbent.PendingAuthSession, func(), error) {
	secureCookie := isRequestHTTPS(c)
	clearCookies := func() {
		clearOAuthPendingSessionCookie(c, secureCookie)
		clearOAuthPendingBrowserCookie(c, secureCookie)
	}

	sessionToken, err := readOAuthPendingSessionCookie(c)
	if err != nil || strings.TrimSpace(sessionToken) == "" {
		clearCookies()
		return nil, nil, clearCookies, service.ErrPendingAuthSessionNotFound
	}
	browserSessionKey, err := readOAuthPendingBrowserCookie(c)
	if err != nil || strings.TrimSpace(browserSessionKey) == "" {
		clearCookies()
		return nil, nil, clearCookies, service.ErrPendingAuthBrowserMismatch
	}

	svc, err := h.pendingIdentityService()
	if err != nil {
		clearCookies()
		return nil, nil, clearCookies, err
	}

	session, err := svc.GetBrowserSession(c.Request.Context(), sessionToken, browserSessionKey)
	if err != nil {
		clearCookies()
		return nil, nil, clearCookies, err
	}

	return svc, session, clearCookies, nil
}

func (h *AuthHandler) consumePendingOAuthSessionOnLogout(c *gin.Context) {
	if c == nil || c.Request == nil {
		return
	}

	sessionToken, err := readOAuthPendingSessionCookie(c)
	if err != nil || strings.TrimSpace(sessionToken) == "" {
		return
	}
	browserSessionKey, err := readOAuthPendingBrowserCookie(c)
	if err != nil || strings.TrimSpace(browserSessionKey) == "" {
		return
	}

	svc, err := h.pendingIdentityService()
	if err != nil {
		return
	}
	_, _ = svc.ConsumeBrowserSession(c.Request.Context(), sessionToken, browserSessionKey)
}

func clearOAuthLogoutCookies(c *gin.Context) {
	secureCookie := isRequestHTTPS(c)

	clearOAuthPendingSessionCookie(c, secureCookie)
	clearOAuthPendingBrowserCookie(c, secureCookie)
	clearOAuthBindAccessTokenCookie(c, secureCookie)

	clearCookie(c, linuxDoOAuthStateCookieName, secureCookie)
	clearCookie(c, linuxDoOAuthVerifierCookie, secureCookie)
	clearCookie(c, linuxDoOAuthRedirectCookie, secureCookie)
	clearCookie(c, linuxDoOAuthIntentCookieName, secureCookie)
	clearCookie(c, linuxDoOAuthBindUserCookieName, secureCookie)

	oidcClearCookie(c, oidcOAuthStateCookieName, secureCookie)
	oidcClearCookie(c, oidcOAuthVerifierCookie, secureCookie)
	oidcClearCookie(c, oidcOAuthRedirectCookie, secureCookie)
	oidcClearCookie(c, oidcOAuthNonceCookie, secureCookie)
	oidcClearCookie(c, oidcOAuthIntentCookieName, secureCookie)
	oidcClearCookie(c, oidcOAuthBindUserCookieName, secureCookie)

	wechatClearCookie(c, wechatOAuthStateCookieName, secureCookie)
	wechatClearCookie(c, wechatOAuthRedirectCookieName, secureCookie)
	wechatClearCookie(c, wechatOAuthIntentCookieName, secureCookie)
	wechatClearCookie(c, wechatOAuthModeCookieName, secureCookie)
	wechatClearCookie(c, wechatOAuthBindUserCookieName, secureCookie)

	wechatPaymentClearCookie(c, wechatPaymentOAuthStateName, secureCookie)
	wechatPaymentClearCookie(c, wechatPaymentOAuthRedirect, secureCookie)
	wechatPaymentClearCookie(c, wechatPaymentOAuthContextName, secureCookie)
	wechatPaymentClearCookie(c, wechatPaymentOAuthScope, secureCookie)
}

func buildPendingOAuthSessionStatusPayload(session *dbent.PendingAuthSession) gin.H {
	completionResponse := normalizePendingOAuthCompletionResponse(mergePendingCompletionResponse(session, nil))
	payload := gin.H{
		"auth_result": "pending_session",
		"provider":    strings.TrimSpace(session.ProviderType),
		"intent":      strings.TrimSpace(session.Intent),
	}
	for key, value := range completionResponse {
		payload[key] = value
	}
	if email := strings.TrimSpace(session.ResolvedEmail); email != "" {
		payload["email"] = email
	}
	return payload
}

func normalizePendingOAuthCompletionResponse(payload map[string]any) map[string]any {
	normalized := clonePendingMap(payload)
	for _, key := range []string{"access_token", "refresh_token", "expires_in", "token_type"} {
		delete(normalized, key)
	}
	step := strings.ToLower(strings.TrimSpace(pendingSessionStringValue(normalized, "step")))
	switch step {
	case "choice", "choose_account_action", "choose_account", "choose", "email_required", "bind_login_required":
		normalized["step"] = oauthPendingChoiceStep
	}
	if strings.EqualFold(strings.TrimSpace(pendingSessionStringValue(normalized, "step")), oauthPendingChoiceStep) {
		normalized["adoption_required"] = true
	}
	if _, exists := normalized["adoption_required"]; !exists {
		if _, hasChoiceFields := normalized["email_binding_required"]; hasChoiceFields {
			normalized["adoption_required"] = true
		}
	}
	return normalized
}

func pendingOAuthChoiceCompletionResponse(session *dbent.PendingAuthSession, email string) map[string]any {
	response := mergePendingCompletionResponse(session, map[string]any{
		"step":                      oauthPendingChoiceStep,
		"adoption_required":         true,
		"force_email_on_signup":     true,
		"email_binding_required":    true,
		"existing_account_bindable": true,
	})
	if email = strings.TrimSpace(email); email != "" {
		response["email"] = email
		response["resolved_email"] = email
	}
	return response
}

func (h *AuthHandler) transitionPendingOAuthAccountToChoiceState(
	c *gin.Context,
	client *dbent.Client,
	session *dbent.PendingAuthSession,
	targetUser *dbent.User,
	email string,
) (*dbent.PendingAuthSession, error) {
	completionResponse := pendingOAuthChoiceCompletionResponse(session, email)
	var targetUserID *int64
	if targetUser != nil && targetUser.ID > 0 {
		targetUserID = &targetUser.ID
	}
	session, err := updatePendingOAuthSessionProgress(
		c.Request.Context(),
		client,
		session,
		strings.TrimSpace(session.Intent),
		email,
		targetUserID,
		completionResponse,
	)
	if err != nil {
		return nil, infraerrors.InternalServer("PENDING_AUTH_SESSION_UPDATE_FAILED", "failed to update pending oauth session").WithCause(err)
	}
	return session, nil
}

func writeOAuthTokenPairResponse(c *gin.Context, tokenPair *service.TokenPair) {
	c.JSON(http.StatusOK, gin.H{
		"access_token":  tokenPair.AccessToken,
		"refresh_token": tokenPair.RefreshToken,
		"expires_in":    tokenPair.ExpiresIn,
		"token_type":    "Bearer",
	})
}

func (h *AuthHandler) bindPendingOAuthLogin(c *gin.Context, provider string) {
	var req bindPendingOAuthLoginRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "Invalid request: "+err.Error())
		return
	}

	pendingSvc, session, clearCookies, err := readPendingOAuthBrowserSession(c, h)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	if strings.TrimSpace(provider) != "" && !strings.EqualFold(strings.TrimSpace(session.ProviderType), provider) {
		response.BadRequest(c, "Pending oauth session provider mismatch")
		return
	}

	user, err := h.authService.ValidatePasswordCredentials(c.Request.Context(), strings.TrimSpace(req.Email), req.Password)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	if session.TargetUserID != nil && *session.TargetUserID > 0 && user.ID != *session.TargetUserID {
		response.ErrorFrom(c, infraerrors.Conflict("PENDING_AUTH_TARGET_USER_MISMATCH", "pending oauth session must be completed by the targeted user"))
		return
	}
	if err := h.ensureBackendModeAllowsUser(c.Request.Context(), user); err != nil {
		response.ErrorFrom(c, err)
		return
	}

	decision, err := h.ensurePendingOAuthAdoptionDecision(c, session.ID, req.adoptionDecision())
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	if h.totpService != nil && h.settingSvc.IsTotpEnabled(c.Request.Context()) && user.TotpEnabled {
		tempToken, err := h.totpService.CreatePendingOAuthBindLoginSession(
			c.Request.Context(),
			user.ID,
			user.Email,
			session.SessionToken,
			session.BrowserSessionKey,
		)
		if err != nil {
			response.InternalError(c, "Failed to create 2FA session")
			return
		}
		response.Success(c, TotpLoginResponse{
			Requires2FA:     true,
			TempToken:       tempToken,
			UserEmailMasked: service.MaskEmail(user.Email),
		})
		return
	}
	if err := applyPendingOAuthBinding(c.Request.Context(), h.entClient(), h.authService, h.userService, session, decision, &user.ID, true, true); err != nil {
		respondPendingOAuthBindingApplyError(c, err)
		return
	}

	h.authService.RecordSuccessfulLogin(c.Request.Context(), user.ID)
	tokenPair, err := h.authService.GenerateTokenPair(c.Request.Context(), user, "")
	if err != nil {
		response.InternalError(c, "Failed to generate token pair")
		return
	}
	if _, err := pendingSvc.ConsumeBrowserSession(c.Request.Context(), session.SessionToken, session.BrowserSessionKey); err != nil {
		clearCookies()
		response.ErrorFrom(c, err)
		return
	}

	clearCookies()
	writeOAuthTokenPairResponse(c, tokenPair)
}

func respondPendingOAuthBindingApplyError(c *gin.Context, err error) {
	if code := infraerrors.Code(err); code >= http.StatusBadRequest && code < http.StatusInternalServerError {
		response.ErrorFrom(c, err)
		return
	}
	response.ErrorFrom(c, infraerrors.InternalServer("PENDING_AUTH_BIND_APPLY_FAILED", "failed to bind pending oauth identity").WithCause(err))
}

func (h *AuthHandler) createPendingOAuthAccount(c *gin.Context, provider string) {
	var req createPendingOAuthAccountRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "Invalid request: "+err.Error())
		return
	}

	_, session, clearCookies, err := readPendingOAuthBrowserSession(c, h)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	if err := ensurePendingOAuthCompleteRegistrationSession(session); err != nil {
		response.ErrorFrom(c, err)
		return
	}
	if strings.TrimSpace(provider) != "" && !strings.EqualFold(strings.TrimSpace(session.ProviderType), provider) {
		response.BadRequest(c, "Pending oauth session provider mismatch")
		return
	}

	client := h.entClient()
	if client == nil {
		response.ErrorFrom(c, infraerrors.ServiceUnavailable("PENDING_AUTH_NOT_READY", "pending auth service is not ready"))
		return
	}

	email := strings.TrimSpace(strings.ToLower(req.Email))
	existingUser, err := findUserByNormalizedEmail(c.Request.Context(), client, email)
	if err != nil {
		switch {
		case errors.Is(err, service.ErrUserNotFound):
			existingUser = nil
		case infraerrors.Code(err) >= http.StatusBadRequest && infraerrors.Code(err) < http.StatusInternalServerError:
			response.ErrorFrom(c, err)
			return
		default:
			response.ErrorFrom(c, infraerrors.ServiceUnavailable("SERVICE_UNAVAILABLE", "service temporarily unavailable"))
			return
		}
	}
	if existingUser != nil {
		session, err = h.transitionPendingOAuthAccountToChoiceState(c, client, session, existingUser, email)
		if err != nil {
			response.ErrorFrom(c, err)
			return
		}
		c.JSON(http.StatusOK, buildPendingOAuthSessionStatusPayload(session))
		return
	}
	if err := h.ensureBackendModeAllowsNewUserLogin(c.Request.Context()); err != nil {
		response.ErrorFrom(c, err)
		return
	}

	tokenPair, user, err := h.authService.RegisterOAuthEmailAccount(
		c.Request.Context(),
		email,
		req.Password,
		strings.TrimSpace(req.VerifyCode),
		strings.TrimSpace(req.InvitationCode),
		strings.TrimSpace(session.ProviderType),
	)
	if err != nil {
		if errors.Is(err, service.ErrEmailExists) {
			existingUser, lookupErr := findUserByNormalizedEmail(c.Request.Context(), client, email)
			if lookupErr != nil {
				response.ErrorFrom(c, lookupErr)
				return
			}
			session, err = h.transitionPendingOAuthAccountToChoiceState(c, client, session, existingUser, email)
			if err != nil {
				response.ErrorFrom(c, err)
				return
			}
			c.JSON(http.StatusOK, buildPendingOAuthSessionStatusPayload(session))
			return
		}
		response.ErrorFrom(c, err)
		return
	}

	rollbackCreatedUser := func(originalErr error) bool {
		if user == nil || user.ID <= 0 {
			return false
		}
		if rollbackErr := h.authService.RollbackOAuthEmailAccountCreation(
			c.Request.Context(),
			user.ID,
			strings.TrimSpace(req.InvitationCode),
		); rollbackErr != nil {
			response.ErrorFrom(c, infraerrors.InternalServer(
				"PENDING_AUTH_ACCOUNT_ROLLBACK_FAILED",
				"failed to rollback pending oauth account creation",
			).WithCause(fmt.Errorf("original error: %w; rollback error: %v", originalErr, rollbackErr)))
			return true
		}
		user = nil
		return false
	}

	decision, err := h.ensurePendingOAuthAdoptionDecision(c, session.ID, req.adoptionDecision())
	if err != nil {
		if rollbackCreatedUser(err) {
			return
		}
		response.ErrorFrom(c, err)
		return
	}

	tx, err := client.Tx(c.Request.Context())
	if err != nil {
		if rollbackCreatedUser(err) {
			return
		}
		response.ErrorFrom(c, infraerrors.InternalServer("PENDING_AUTH_BIND_APPLY_FAILED", "failed to bind pending oauth identity").WithCause(err))
		return
	}
	defer func() { _ = tx.Rollback() }()
	txCtx := dbent.NewTxContext(c.Request.Context(), tx)

	if err := applyPendingOAuthBinding(txCtx, client, h.authService, h.userService, session, decision, &user.ID, true, false); err != nil {
		_ = tx.Rollback()
		if rollbackCreatedUser(err) {
			return
		}
		respondPendingOAuthBindingApplyError(c, err)
		return
	}

	if err := h.authService.FinalizeOAuthEmailAccount(
		txCtx,
		user,
		strings.TrimSpace(req.InvitationCode),
		strings.TrimSpace(session.ProviderType),
	); err != nil {
		_ = tx.Rollback()
		if rollbackCreatedUser(err) {
			return
		}
		response.ErrorFrom(c, err)
		return
	}

	if err := consumePendingOAuthBrowserSessionTx(txCtx, tx, session); err != nil {
		_ = tx.Rollback()
		if rollbackCreatedUser(err) {
			return
		}
		clearCookies()
		response.ErrorFrom(c, err)
		return
	}

	if pendingOAuthCreateAccountPreCommitHook != nil {
		if err := pendingOAuthCreateAccountPreCommitHook(txCtx, session); err != nil {
			_ = tx.Rollback()
			if rollbackCreatedUser(err) {
				return
			}
			respondPendingOAuthBindingApplyError(c, err)
			return
		}
	}

	if err := tx.Commit(); err != nil {
		if rollbackCreatedUser(err) {
			return
		}
		response.ErrorFrom(c, infraerrors.InternalServer("PENDING_AUTH_BIND_APPLY_FAILED", "failed to bind pending oauth identity").WithCause(err))
		return
	}

	h.authService.RecordSuccessfulLogin(c.Request.Context(), user.ID)
	clearCookies()
	writeOAuthTokenPairResponse(c, tokenPair)
}

// ExchangePendingOAuthCompletion redeems a pending OAuth browser session into a frontend-safe payload.
// POST /api/v1/auth/oauth/pending/exchange
func (h *AuthHandler) ExchangePendingOAuthCompletion(c *gin.Context) {
	secureCookie := isRequestHTTPS(c)
	clearCookies := func() {
		clearOAuthPendingSessionCookie(c, secureCookie)
		clearOAuthPendingBrowserCookie(c, secureCookie)
	}
	adoptionDecision, err := bindOptionalOAuthAdoptionDecision(c)
	if err != nil {
		response.BadRequest(c, "Invalid request: "+err.Error())
		return
	}

	sessionToken, err := readOAuthPendingSessionCookie(c)
	if err != nil || strings.TrimSpace(sessionToken) == "" {
		clearCookies()
		response.ErrorFrom(c, service.ErrPendingAuthSessionNotFound)
		return
	}
	browserSessionKey, err := readOAuthPendingBrowserCookie(c)
	if err != nil || strings.TrimSpace(browserSessionKey) == "" {
		clearCookies()
		response.ErrorFrom(c, service.ErrPendingAuthBrowserMismatch)
		return
	}

	svc, err := h.pendingIdentityService()
	if err != nil {
		clearCookies()
		response.ErrorFrom(c, err)
		return
	}

	session, err := svc.GetBrowserSession(c.Request.Context(), sessionToken, browserSessionKey)
	if err != nil {
		clearCookies()
		response.ErrorFrom(c, err)
		return
	}

	payload, ok := readCompletionResponse(session.LocalFlowState)
	if !ok {
		clearCookies()
		response.ErrorFrom(c, infraerrors.InternalServer("PENDING_AUTH_COMPLETION_INVALID", "pending auth completion payload is invalid"))
		return
	}
	payload = normalizePendingOAuthCompletionResponse(payload)
	if strings.TrimSpace(session.RedirectTo) != "" {
		if _, exists := payload["redirect"]; !exists {
			payload["redirect"] = session.RedirectTo
		}
	}
	applySuggestedProfileToCompletionResponse(payload, session.UpstreamIdentityClaims)

	canIssueTokenPair := pendingOAuthCompletionCanIssueTokenPair(session, payload)
	var loginUser *service.User
	if canIssueTokenPair {
		loginUser, err = h.userService.GetByID(c.Request.Context(), *session.TargetUserID)
		if err != nil {
			clearCookies()
			response.ErrorFrom(c, err)
			return
		}
		if err := ensureLoginUserActive(loginUser); err != nil {
			clearCookies()
			response.ErrorFrom(c, err)
			return
		}
		if err := h.ensureBackendModeAllowsUser(c.Request.Context(), loginUser); err != nil {
			clearCookies()
			response.ErrorFrom(c, err)
			return
		}
	}
	skipAdoptionPrompt, err := h.shouldSkipPendingOAuthAdoptionPrompt(c.Request.Context(), session, payload)
	if err != nil {
		clearCookies()
		response.ErrorFrom(c, err)
		return
	}
	if skipAdoptionPrompt {
		delete(payload, "adoption_required")
	}

	if pendingSessionWantsInvitation(payload) {
		if adoptionDecision.hasDecision() {
			decision, err := h.upsertPendingOAuthAdoptionDecision(c, session.ID, adoptionDecision)
			if err != nil {
				response.ErrorFrom(c, err)
				return
			}
			_ = decision
		}
		response.Success(c, payload)
		return
	}
	if !adoptionDecision.hasDecision() {
		adoptionRequired, _ := payload["adoption_required"].(bool)
		if adoptionRequired {
			response.Success(c, payload)
			return
		}
	}

	decisionReq := adoptionDecision
	if !decisionReq.hasDecision() {
		adoptDisplayName := false
		adoptAvatar := false
		decisionReq = oauthAdoptionDecisionRequest{
			AdoptDisplayName: &adoptDisplayName,
			AdoptAvatar:      &adoptAvatar,
		}
	}

	decision, err := h.ensurePendingOAuthAdoptionDecision(c, session.ID, decisionReq)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	if err := applyPendingOAuthAdoption(c.Request.Context(), h.entClient(), h.authService, h.userService, session, decision, session.TargetUserID); err != nil {
		response.ErrorFrom(c, infraerrors.InternalServer("PENDING_AUTH_ADOPTION_APPLY_FAILED", "failed to apply oauth profile adoption").WithCause(err))
		return
	}

	if _, err := svc.ConsumeBrowserSession(c.Request.Context(), sessionToken, browserSessionKey); err != nil {
		clearCookies()
		response.ErrorFrom(c, err)
		return
	}

	if canIssueTokenPair {
		tokenPair, err := h.authService.GenerateTokenPair(c.Request.Context(), loginUser, "")
		if err != nil {
			clearCookies()
			response.InternalError(c, "Failed to generate token pair")
			return
		}
		h.authService.RecordSuccessfulLogin(c.Request.Context(), loginUser.ID)
		payload["access_token"] = tokenPair.AccessToken
		payload["refresh_token"] = tokenPair.RefreshToken
		payload["expires_in"] = tokenPair.ExpiresIn
		payload["token_type"] = "Bearer"
	}

	clearCookies()
	response.Success(c, payload)
}
