package handler

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
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
	oauthPromoCodeCookieName      = "oauth_promo_code"
	oauthPendingCookieMaxAgeSec   = 10 * 60
	oauthPendingChoiceStep        = "choose_account_action_required"

	oauthCompletionResponseKey = "completion_response"
	oauthPromoCodeStateKey     = "promo_code"
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
REDACTED

type oauthAdoptionDecisionRequest struct {
	AdoptDisplayName *bool `json:"adopt_display_name,omitempty"`
	AdoptAvatar      *bool `json:"adopt_avatar,omitempty"`
REDACTED

type bindPendingOAuthLoginRequest struct {
	Email            string `json:"email" binding:"required,email"`
	Password         string `json:"password" binding:"required"`
	AdoptDisplayName *bool  `json:"adopt_display_name,omitempty"`
	AdoptAvatar      *bool  `json:"adopt_avatar,omitempty"`
REDACTED

type createPendingOAuthAccountRequest struct {
	Email            string `json:"email" binding:"required,email"`
	VerifyCode       string `json:"verify_code,omitempty"`
	Password         string `json:"password" binding:"required,min=6"`
	InvitationCode   string `json:"invitation_code,omitempty"`
	AffCode          string `json:"aff_code,omitempty"`
	AdoptDisplayName *bool  `json:"adopt_display_name,omitempty"`
	AdoptAvatar      *bool  `json:"adopt_avatar,omitempty"`
REDACTED

type sendPendingOAuthVerifyCodeRequest struct {
	Email             string `json:"email" binding:"required,email"`
	TurnstileToken    string `json:"turnstile_token,omitempty"`
	PendingAuthToken  string `json:"pending_auth_token,omitempty"`
	PendingOAuthToken string `json:"pending_oauth_token,omitempty"`
REDACTED

func (r bindPendingOAuthLoginRequest) adoptionDecision() oauthAdoptionDecisionRequest {
	return oauthAdoptionDecisionRequest{
		AdoptDisplayName: r.AdoptDisplayName,
		AdoptAvatar:      r.AdoptAvatar,
REDACTED
REDACTED

func (r createPendingOAuthAccountRequest) adoptionDecision() oauthAdoptionDecisionRequest {
	return oauthAdoptionDecisionRequest{
		AdoptDisplayName: r.AdoptDisplayName,
		AdoptAvatar:      r.AdoptAvatar,
REDACTED
REDACTED

func (h *AuthHandler) pendingIdentityService() (*service.AuthPendingIdentityService, error) {
	if h == nil || h.authService == nil || h.authService.EntClient() == nil {
		return nil, infraerrors.ServiceUnavailable("PENDING_AUTH_NOT_READY", "pending auth service is not ready")
REDACTED
	return service.NewAuthPendingIdentityService(h.authService.EntClient()), nil
REDACTED

func generateOAuthPendingBrowserSession() (string, error) {
	return oauth.GenerateState()
REDACTED

func setOAuthPendingBrowserCookie(c *gin.Context, sessionKey string, secure bool) {
	http.SetCookie(c.Writer, &http.Cookie{
		Name:     oauthPendingBrowserCookieName,
		Value:    encodeCookieValue(sessionKey),
		Path:     oauthPendingBrowserCookiePath,
		MaxAge:   oauthPendingCookieMaxAgeSec,
		HttpOnly: true,
		Secure:   secure,
		SameSite: http.SameSiteLaxMode,
REDACTED)
REDACTED

func clearOAuthPendingBrowserCookie(c *gin.Context, secure bool) {
	http.SetCookie(c.Writer, &http.Cookie{
		Name:     oauthPendingBrowserCookieName,
		Value:    "",
		Path:     oauthPendingBrowserCookiePath,
		MaxAge:   -1,
		HttpOnly: true,
		Secure:   secure,
		SameSite: http.SameSiteLaxMode,
REDACTED)
REDACTED

func readOAuthPendingBrowserCookie(c *gin.Context) (string, error) {
	return readCookieDecoded(c, oauthPendingBrowserCookieName)
REDACTED

func setOAuthPendingSessionCookie(c *gin.Context, sessionToken string, secure bool) {
	http.SetCookie(c.Writer, &http.Cookie{
		Name:     oauthPendingSessionCookieName,
		Value:    encodeCookieValue(sessionToken),
		Path:     oauthPendingSessionCookiePath,
		MaxAge:   oauthPendingCookieMaxAgeSec,
		HttpOnly: true,
		Secure:   secure,
		SameSite: http.SameSiteLaxMode,
REDACTED)
REDACTED

func clearOAuthPendingSessionCookie(c *gin.Context, secure bool) {
	http.SetCookie(c.Writer, &http.Cookie{
		Name:     oauthPendingSessionCookieName,
		Value:    "",
		Path:     oauthPendingSessionCookiePath,
		MaxAge:   -1,
		HttpOnly: true,
		Secure:   secure,
		SameSite: http.SameSiteLaxMode,
REDACTED)
REDACTED

func readOAuthPendingSessionCookie(c *gin.Context) (string, error) {
	return readCookieDecoded(c, oauthPendingSessionCookieName)
REDACTED

func captureOAuthPromoCode(c *gin.Context, secure bool) {
	promoCode := strings.TrimSpace(c.Query("promo_code"))
	if promoCode == "" {
		clearOAuthPromoCodeCookie(c, secure)
		return
REDACTED
	http.SetCookie(c.Writer, &http.Cookie{
		Name:     oauthPromoCodeCookieName,
		Value:    encodeCookieValue(promoCode),
		Path:     oauthPendingBrowserCookiePath,
		MaxAge:   oauthPendingCookieMaxAgeSec,
		HttpOnly: true,
		Secure:   secure,
		SameSite: http.SameSiteLaxMode,
REDACTED)
REDACTED

func clearOAuthPromoCodeCookie(c *gin.Context, secure bool) {
	http.SetCookie(c.Writer, &http.Cookie{
		Name:     oauthPromoCodeCookieName,
		Value:    "",
		Path:     oauthPendingBrowserCookiePath,
		MaxAge:   -1,
		HttpOnly: true,
		Secure:   secure,
		SameSite: http.SameSiteLaxMode,
REDACTED)
REDACTED

func readOAuthPromoCode(c *gin.Context) string {
	if c == nil {
		return ""
REDACTED
	promoCode, err := readCookieDecoded(c, oauthPromoCodeCookieName)
	if err != nil {
		return ""
REDACTED
	return strings.TrimSpace(promoCode)
REDACTED

func pendingOAuthPromoCode(session *dbent.PendingAuthSession) string {
	if session == nil {
		return ""
REDACTED
	return pendingSessionStringValue(session.LocalFlowState, oauthPromoCodeStateKey)
REDACTED

func redirectToFrontendCallback(c *gin.Context, frontendCallback string) {
	u, err := url.Parse(frontendCallback)
	if err != nil {
		c.Redirect(http.StatusFound, linuxDoOAuthDefaultRedirectTo)
		return
REDACTED
	if u.Scheme != "" && !strings.EqualFold(u.Scheme, "http") && !strings.EqualFold(u.Scheme, "https") {
		c.Redirect(http.StatusFound, linuxDoOAuthDefaultRedirectTo)
		return
REDACTED
	u.Fragment = ""
	c.Header("Cache-Control", "no-store")
	c.Header("Pragma", "no-cache")
	c.Redirect(http.StatusFound, u.String())
REDACTED

func (h *AuthHandler) createOAuthPendingSession(c *gin.Context, payload oauthPendingSessionPayload) error {
	svc, err := h.pendingIdentityService()
	if err != nil {
		return err
REDACTED

	localFlowState := map[string]any{
		oauthCompletionResponseKey: payload.CompletionResponse,
REDACTED
	if promoCode := readOAuthPromoCode(c); promoCode != "" {
		localFlowState[oauthPromoCodeStateKey] = promoCode
REDACTED

	session, err := svc.CreatePendingSession(c.Request.Context(), service.CreatePendingAuthSessionInput{
		Intent:                 strings.TrimSpace(payload.Intent),
		Identity:               payload.Identity,
		TargetUserID:           payload.TargetUserID,
		ResolvedEmail:          strings.TrimSpace(payload.ResolvedEmail),
		RedirectTo:             strings.TrimSpace(payload.RedirectTo),
		BrowserSessionKey:      strings.TrimSpace(payload.BrowserSessionKey),
		UpstreamIdentityClaims: payload.UpstreamIdentityClaims,
		LocalFlowState:         localFlowState,
REDACTED)
	if err != nil {
		slog.Error("pending auth session create failed",
			"intent", strings.TrimSpace(payload.Intent),
			"provider_type", strings.TrimSpace(payload.Identity.ProviderType),
			"provider_key", strings.TrimSpace(payload.Identity.ProviderKey),
			"provider_subject_len", len(strings.TrimSpace(payload.Identity.ProviderSubject)),
			"resolved_email_len", len(strings.TrimSpace(payload.ResolvedEmail)),
			"has_target_user", payload.TargetUserID != nil,
			"error", err.Error())
		return infraerrors.InternalServer("PENDING_AUTH_SESSION_CREATE_FAILED", "failed to create pending auth session").WithCause(err)
REDACTED

	setOAuthPendingSessionCookie(c, session.SessionToken, isRequestHTTPS(c))
	return nil
REDACTED

func readCompletionResponse(session map[string]any) (map[string]any, bool) {
	if len(session) == 0 {
		return nil, false
REDACTED
	value, ok := session[oauthCompletionResponseKey]
	if !ok {
		return nil, false
REDACTED
	result, ok := value.(map[string]any)
	if !ok {
		return nil, false
REDACTED
	return result, true
REDACTED

func clonePendingMap(values map[string]any) map[string]any {
	if len(values) == 0 {
		return map[string]any{REDACTED
REDACTED
	cloned := make(map[string]any, len(values))
	for key, value := range values {
		cloned[key] = value
REDACTED
	return cloned
REDACTED

func mergePendingCompletionResponse(session *dbent.PendingAuthSession, overrides map[string]any) map[string]any {
	payload, _ := readCompletionResponse(session.LocalFlowState)
	merged := clonePendingMap(payload)
	if strings.TrimSpace(session.RedirectTo) != "" {
		if _, exists := merged["redirect"]; !exists {
			merged["redirect"] = session.RedirectTo
	REDACTED
REDACTED
	for key, value := range overrides {
		if value == nil {
			delete(merged, key)
			continue
	REDACTED
		merged[key] = value
REDACTED
	applySuggestedProfileToCompletionResponse(merged, session.UpstreamIdentityClaims)
	return merged
REDACTED

func pendingSessionStringValue(values map[string]any, key string) string {
	if len(values) == 0 {
		return ""
REDACTED
	raw, ok := values[key]
	if !ok {
		return ""
REDACTED
	value, ok := raw.(string)
	if !ok {
		return ""
REDACTED
	return strings.TrimSpace(value)
REDACTED

func pendingSessionWantsInvitation(payload map[string]any) bool {
	return strings.EqualFold(strings.TrimSpace(pendingSessionStringValue(payload, "error")), "invitation_required")
REDACTED

// pendingSessionRequiresEmailCompletion 判断 callback 写入的 completion payload 是否处于"补邮箱"状态。
// 钉钉跨组织/staff 邮箱缺失时进入此状态：前端跳到补邮箱页，exchange 不应走 adoption apply。
func pendingSessionRequiresEmailCompletion(payload map[string]any) bool {
	if v, ok := payload["requires_email_completion"].(bool); ok && v {
		return true
REDACTED
	return strings.EqualFold(strings.TrimSpace(pendingSessionStringValue(payload, "step")), "email_completion")
REDACTED

// pendingSessionRequiresBindLogin 判断 callback 写入的 completion payload 是否处于"必须绑定已有账户"状态。
// 钉钉 signupBlocked=true（注册关 + 钉钉企业豁免关）时进入此状态：前端渲染 bind_login 表单，
// exchange 不应消费 session，否则后续 /pending/bind-login 找不到 session。
func pendingSessionRequiresBindLogin(payload map[string]any) bool {
	return strings.EqualFold(strings.TrimSpace(pendingSessionStringValue(payload, "step")), "bind_login_required")
REDACTED

func pendingOAuthCompletionCanIssueTokenPair(session *dbent.PendingAuthSession, payload map[string]any) bool {
	if session == nil {
		return false
REDACTED
	if !strings.EqualFold(strings.TrimSpace(session.Intent), oauthIntentLogin) {
		return false
REDACTED
	if session.TargetUserID == nil || *session.TargetUserID <= 0 {
		return false
REDACTED
	if pendingSessionWantsInvitation(payload) {
		return false
REDACTED
	return strings.TrimSpace(pendingSessionStringValue(payload, "step")) == ""
REDACTED

func ensurePendingOAuthCompleteRegistrationSession(session *dbent.PendingAuthSession) error {
	if session == nil {
		return infraerrors.BadRequest("PENDING_AUTH_SESSION_INVALID", "pending auth registration context is invalid")
REDACTED
	if strings.TrimSpace(session.Intent) != oauthIntentLogin {
		return infraerrors.BadRequest("PENDING_AUTH_SESSION_INVALID", "pending auth registration context is invalid")
REDACTED
	if session.TargetUserID != nil && *session.TargetUserID > 0 {
		return infraerrors.BadRequest("PENDING_AUTH_SESSION_INVALID", "pending auth registration context is invalid")
REDACTED
	payload, _ := readCompletionResponse(session.LocalFlowState)
	if strings.EqualFold(strings.TrimSpace(pendingSessionStringValue(payload, "step")), "bind_login_required") {
		return infraerrors.BadRequest("PENDING_AUTH_SESSION_INVALID", "pending auth registration context is invalid")
REDACTED
	return nil
REDACTED

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
REDACTED))

	if email := strings.TrimSpace(session.ResolvedEmail); email != "" {
		if _, exists := completionResponse["email"]; !exists {
			completionResponse["email"] = email
	REDACTED
		if _, exists := completionResponse["resolved_email"]; !exists {
			completionResponse["resolved_email"] = email
	REDACTED
REDACTED
	if _, exists := completionResponse["choice_reason"]; !exists {
		switch {
		case forceEmailOnSignup:
			completionResponse["choice_reason"] = "force_email_on_signup"
		case emailVerificationRequired:
			completionResponse["choice_reason"] = "email_verification_required"
		default:
			completionResponse["choice_reason"] = "third_party_signup"
	REDACTED
REDACTED
	return completionResponse
REDACTED

func (h *AuthHandler) legacyCompleteRegistrationSessionStatus(
	c *gin.Context,
	session *dbent.PendingAuthSession,
) (*dbent.PendingAuthSession, bool, error) {
	if session == nil {
		return nil, false, infraerrors.BadRequest("PENDING_AUTH_SESSION_INVALID", "pending auth registration context is invalid")
REDACTED

	payload := normalizePendingOAuthCompletionResponse(mergePendingCompletionResponse(session, nil))
	if step := pendingSessionStringValue(payload, "step"); step != "" {
		return session, true, nil
REDACTED

	emailVerificationRequired := h != nil && h.authService != nil && h.authService.IsEmailVerifyEnabled(c.Request.Context())
	forceEmailOnSignup := h.isForceEmailOnThirdPartySignup(c.Request.Context())
	if !emailVerificationRequired && !forceEmailOnSignup {
		return session, false, nil
REDACTED

	client := h.entClient()
	if client == nil {
		return nil, false, infraerrors.ServiceUnavailable("PENDING_AUTH_NOT_READY", "pending auth service is not ready")
REDACTED

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
REDACTED
	return updatedSession, true, nil
REDACTED

func (r oauthAdoptionDecisionRequest) hasDecision() bool {
	return r.AdoptDisplayName != nil || r.AdoptAvatar != nil
REDACTED

func bindOptionalOAuthAdoptionDecision(c *gin.Context) (oauthAdoptionDecisionRequest, error) {
	var req oauthAdoptionDecisionRequest
	if c == nil || c.Request == nil || c.Request.Body == nil {
		return req, nil
REDACTED
	if err := c.ShouldBindJSON(&req); err != nil {
		if errors.Is(err, io.EOF) {
			return req, nil
	REDACTED
		return req, err
REDACTED
	return req, nil
REDACTED

func cloneOAuthMetadata(values map[string]any) map[string]any {
	if len(values) == 0 {
		return map[string]any{REDACTED
REDACTED
	cloned := make(map[string]any, len(values))
	for key, value := range values {
		cloned[key] = value
REDACTED
	return cloned
REDACTED

func mergeOAuthMetadata(base map[string]any, overlay map[string]any) map[string]any {
	merged := cloneOAuthMetadata(base)
	for key, value := range overlay {
		merged[key] = value
REDACTED
	return merged
REDACTED

func normalizeAdoptedOAuthDisplayName(value string) string {
	value = strings.TrimSpace(value)
	if len([]rune(value)) > 100 {
		value = string([]rune(value)[:100])
REDACTED
	return value
REDACTED

func (h *AuthHandler) entClient() *dbent.Client {
	if h == nil || h.authService == nil {
		return nil
REDACTED
	return h.authService.EntClient()
REDACTED

func (h *AuthHandler) isForceEmailOnThirdPartySignup(ctx context.Context) bool {
	if h == nil || h.settingSvc == nil {
		return false
REDACTED
	defaults, err := h.settingSvc.GetAuthSourceDefaultSettings(ctx)
	if err != nil || defaults == nil {
		return false
REDACTED
	return defaults.ForceEmailOnThirdPartySignup
REDACTED

func (h *AuthHandler) findOAuthIdentityUser(ctx context.Context, identity service.PendingAuthIdentityKey) (*dbent.User, error) {
	client := h.entClient()
	if client == nil {
		return nil, infraerrors.ServiceUnavailable("PENDING_AUTH_NOT_READY", "pending auth service is not ready")
REDACTED

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
	REDACTED
		return nil, infraerrors.InternalServer("AUTH_IDENTITY_LOOKUP_FAILED", "failed to inspect auth identity ownership").WithCause(err)
REDACTED
	return findActiveUserByID(ctx, client, record.UserID)
REDACTED

func (h *AuthHandler) BindLinuxDoOAuthLogin(c *gin.Context) { h.bindPendingOAuthLogin(c, "linuxdo") REDACTED
func (h *AuthHandler) BindOIDCOAuthLogin(c *gin.Context)    { h.bindPendingOAuthLogin(c, "oidc") REDACTED
func (h *AuthHandler) BindWeChatOAuthLogin(c *gin.Context)  { h.bindPendingOAuthLogin(c, "wechat") REDACTED
func (h *AuthHandler) BindPendingOAuthLogin(c *gin.Context) { h.bindPendingOAuthLogin(c, "") REDACTED

func (h *AuthHandler) CreateLinuxDoOAuthAccount(c *gin.Context) {
	h.createPendingOAuthAccount(c, "linuxdo")
REDACTED

func (h *AuthHandler) CreateOIDCOAuthAccount(c *gin.Context) { h.createPendingOAuthAccount(c, "oidc") REDACTED

func (h *AuthHandler) CreateWeChatOAuthAccount(c *gin.Context) {
	h.createPendingOAuthAccount(c, "wechat")
REDACTED

func (h *AuthHandler) CreatePendingOAuthAccount(c *gin.Context) {
	h.createPendingOAuthAccount(c, "")
REDACTED

// SendPendingOAuthVerifyCode sends a verification code for a browser-bound
// pending OAuth account-creation flow.
// POST /api/v1/auth/oauth/pending/send-verify-code
func (h *AuthHandler) SendPendingOAuthVerifyCode(c *gin.Context) {
	var req sendPendingOAuthVerifyCodeRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "Invalid request: "+err.Error())
		return
REDACTED

	if err := h.authService.VerifyTurnstile(c.Request.Context(), req.TurnstileToken, ip.GetClientIP(c)); err != nil {
		response.ErrorFrom(c, err)
		return
REDACTED

	_, session, _, err := readPendingOAuthBrowserSession(c, h)
	if err != nil {
		response.ErrorFrom(c, err)
		return
REDACTED
	if err := ensurePendingOAuthCompleteRegistrationSession(session); err != nil {
		response.ErrorFrom(c, err)
		return
REDACTED

	client := h.entClient()
	if client == nil {
		response.ErrorFrom(c, infraerrors.ServiceUnavailable("PENDING_AUTH_NOT_READY", "pending auth service is not ready"))
		return
REDACTED

	email := strings.TrimSpace(strings.ToLower(req.Email))
	if existingUser, err := findUserByNormalizedEmail(c.Request.Context(), client, email); err == nil && existingUser != nil {
		session, err = h.transitionPendingOAuthAccountToChoiceState(c, client, session, existingUser, email)
		if err != nil {
			response.ErrorFrom(c, err)
			return
	REDACTED
		c.JSON(http.StatusOK, buildPendingOAuthSessionStatusPayload(session))
		return
REDACTED else if err != nil && !errors.Is(err, service.ErrUserNotFound) {
		response.ErrorFrom(c, err)
		return
REDACTED

	result, err := h.authService.SendPendingOAuthVerifyCode(c.Request.Context(), req.Email, c.GetHeader("Accept-Language"))
	if err != nil {
		response.ErrorFrom(c, err)
		return
REDACTED

	response.Success(c, SendVerifyCodeResponse{
		Message:   "Verification code sent successfully",
		Countdown: result.Countdown,
REDACTED)
REDACTED

func (h *AuthHandler) upsertPendingOAuthAdoptionDecision(
	c *gin.Context,
	sessionID int64,
	req oauthAdoptionDecisionRequest,
) (*dbent.IdentityAdoptionDecision, error) {
	client := h.entClient()
	if client == nil {
		return nil, infraerrors.ServiceUnavailable("PENDING_AUTH_NOT_READY", "pending auth service is not ready")
REDACTED

	existing, err := client.IdentityAdoptionDecision.Query().
		Where(identityadoptiondecision.PendingAuthSessionIDEQ(sessionID)).
		Only(c.Request.Context())
	if err != nil && !dbent.IsNotFound(err) {
		return nil, infraerrors.InternalServer("PENDING_AUTH_ADOPTION_LOAD_FAILED", "failed to load oauth profile adoption decision").WithCause(err)
REDACTED
	if existing != nil && !req.hasDecision() {
		return existing, nil
REDACTED
	if existing == nil && !req.hasDecision() {
		return nil, nil
REDACTED

	input := service.PendingIdentityAdoptionDecisionInput{
		PendingAuthSessionID: sessionID,
REDACTED
	if existing != nil {
		input.AdoptDisplayName = existing.AdoptDisplayName
		input.AdoptAvatar = existing.AdoptAvatar
		input.IdentityID = existing.IdentityID
REDACTED
	if req.AdoptDisplayName != nil {
		input.AdoptDisplayName = *req.AdoptDisplayName
REDACTED
	if req.AdoptAvatar != nil {
		input.AdoptAvatar = *req.AdoptAvatar
REDACTED

	svc, err := h.pendingIdentityService()
	if err != nil {
		return nil, err
REDACTED
	decision, err := svc.UpsertAdoptionDecision(c.Request.Context(), input)
	if err != nil {
		return nil, infraerrors.InternalServer("PENDING_AUTH_ADOPTION_SAVE_FAILED", "failed to save oauth profile adoption decision").WithCause(err)
REDACTED
	return decision, nil
REDACTED

func (h *AuthHandler) ensurePendingOAuthAdoptionDecision(
	c *gin.Context,
	sessionID int64,
	req oauthAdoptionDecisionRequest,
) (*dbent.IdentityAdoptionDecision, error) {
	decision, err := h.upsertPendingOAuthAdoptionDecision(c, sessionID, req)
	if err != nil {
		return nil, err
REDACTED
	if decision != nil {
		return decision, nil
REDACTED

	svc, err := h.pendingIdentityService()
	if err != nil {
		return nil, err
REDACTED
	decision, err = svc.UpsertAdoptionDecision(c.Request.Context(), service.PendingIdentityAdoptionDecisionInput{
		PendingAuthSessionID: sessionID,
REDACTED)
	if err != nil {
		return nil, infraerrors.InternalServer("PENDING_AUTH_ADOPTION_SAVE_FAILED", "failed to save oauth profile adoption decision").WithCause(err)
REDACTED
	return decision, nil
REDACTED

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
REDACTED

	localFlowState := clonePendingMap(session.LocalFlowState)
	localFlowState[oauthCompletionResponseKey] = clonePendingMap(completionResponse)

	update := client.PendingAuthSession.UpdateOneID(session.ID).
		SetIntent(strings.TrimSpace(intent)).
		SetResolvedEmail(strings.TrimSpace(resolvedEmail)).
		SetLocalFlowState(localFlowState)
	if targetUserID != nil && *targetUserID > 0 {
		update = update.SetTargetUserID(*targetUserID)
REDACTED else {
		update = update.ClearTargetUserID()
REDACTED
	return update.Save(ctx)
REDACTED

func resolvePendingOAuthTargetUserID(ctx context.Context, client *dbent.Client, session *dbent.PendingAuthSession) (int64, error) {
	if session == nil {
		return 0, infraerrors.BadRequest("PENDING_AUTH_SESSION_INVALID", "pending auth session is invalid")
REDACTED
	if session.TargetUserID != nil && *session.TargetUserID > 0 {
		return *session.TargetUserID, nil
REDACTED
	email := strings.TrimSpace(session.ResolvedEmail)
	if email == "" {
		return 0, infraerrors.BadRequest("PENDING_AUTH_TARGET_USER_MISSING", "pending auth target user is missing")
REDACTED

	userEntity, err := findUserByNormalizedEmail(ctx, client, email)
	if err != nil {
		if errors.Is(err, service.ErrUserNotFound) {
			return 0, infraerrors.InternalServer("PENDING_AUTH_TARGET_USER_NOT_FOUND", "pending auth target user was not found")
	REDACTED
		return 0, err
REDACTED
	return userEntity.ID, nil
REDACTED

func userNormalizedEmailPredicate(email string) predicate.User {
	normalized := strings.ToLower(strings.TrimSpace(email))
	if normalized == "" {
		return dbuser.EmailEQ(email)
REDACTED
	return predicate.User(func(s *entsql.Selector) {
		s.Where(entsql.P(func(b *entsql.Builder) {
			b.WriteString("LOWER(TRIM(").
				Ident(s.C(dbuser.FieldEmail)).
				WriteString(")) = ").
				Arg(normalized)
	REDACTED))
REDACTED)
REDACTED

func findUserByNormalizedEmail(ctx context.Context, client *dbent.Client, email string) (*dbent.User, error) {
	if client == nil {
		return nil, infraerrors.ServiceUnavailable("PENDING_AUTH_NOT_READY", "pending auth service is not ready")
REDACTED

	matches, err := client.User.Query().
		Where(userNormalizedEmailPredicate(email)).
		Order(dbent.Asc(dbuser.FieldID)).
		All(ctx)
	if err != nil {
		return nil, err
REDACTED
	if len(matches) == 0 {
		return nil, service.ErrUserNotFound
REDACTED
	if len(matches) > 1 {
		return nil, infraerrors.Conflict("USER_EMAIL_CONFLICT", "normalized email matched multiple users")
REDACTED
	return matches[0], nil
REDACTED

func ensurePendingOAuthRegistrationIdentityAvailable(ctx context.Context, client *dbent.Client, session *dbent.PendingAuthSession) error {
	if client == nil || session == nil {
		return infraerrors.BadRequest("PENDING_AUTH_SESSION_INVALID", "pending auth registration context is invalid")
REDACTED

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
	REDACTED
		return err
REDACTED
	if identity == nil || identity.UserID <= 0 {
		return nil
REDACTED

	activeOwner, err := findActiveUserByID(ctx, client, identity.UserID)
	if err != nil {
		return err
REDACTED
	if activeOwner != nil {
		return infraerrors.Conflict("AUTH_IDENTITY_OWNERSHIP_CONFLICT", "auth identity already belongs to another user")
REDACTED
	return nil
REDACTED

func oauthIdentityIssuer(session *dbent.PendingAuthSession) *string {
	if session == nil {
		return nil
REDACTED
	switch strings.TrimSpace(session.ProviderType) {
	case "oidc":
		issuer := strings.TrimSpace(session.ProviderKey)
		if issuer == "" {
			issuer = pendingSessionStringValue(session.UpstreamIdentityClaims, "issuer")
	REDACTED
		if issuer == "" {
			return nil
	REDACTED
		return &issuer
	default:
		issuer := pendingSessionStringValue(session.UpstreamIdentityClaims, "issuer")
		if issuer == "" {
			return nil
	REDACTED
		return &issuer
REDACTED
REDACTED

func ensurePendingOAuthIdentityForUser(ctx context.Context, tx *dbent.Tx, session *dbent.PendingAuthSession, userID int64) (*dbent.AuthIdentity, error) {
	if session != nil && strings.EqualFold(strings.TrimSpace(session.ProviderType), "wechat") {
		return ensurePendingWeChatOAuthIdentityForUser(ctx, tx, session, userID)
REDACTED

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
REDACTED
	if identity != nil {
		if identity.UserID != userID {
			activeOwner, err := findActiveUserByID(ctx, client, identity.UserID)
			if err != nil {
				return nil, err
		REDACTED
			if activeOwner != nil {
				return nil, infraerrors.Conflict("AUTH_IDENTITY_OWNERSHIP_CONFLICT", "auth identity already belongs to another user")
		REDACTED
			return client.AuthIdentity.UpdateOneID(identity.ID).
				SetUserID(userID).
				Save(ctx)
	REDACTED
		return identity, nil
REDACTED

	create := client.AuthIdentity.Create().
		SetUserID(userID).
		SetProviderType(strings.TrimSpace(session.ProviderType)).
		SetProviderKey(strings.TrimSpace(session.ProviderKey)).
		SetProviderSubject(strings.TrimSpace(session.ProviderSubject)).
		SetMetadata(cloneOAuthMetadata(session.UpstreamIdentityClaims))
	if issuer := oauthIdentityIssuer(session); issuer != nil {
		create = create.SetIssuer(strings.TrimSpace(*issuer))
REDACTED
	return create.Save(ctx)
REDACTED

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
REDACTED
	identity, hasCanonicalKey, err := chooseWeChatIdentityForUser(ctx, client, identityRecords, userID, providerKey)
	if err != nil {
		return nil, err
REDACTED

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
	REDACTED
		legacyOpenIDIdentity, _, err = chooseWeChatIdentityForUser(ctx, client, legacyOpenIDRecords, userID, providerKey)
		if err != nil {
			return nil, err
	REDACTED
REDACTED

	switch {
	case identity != nil:
		update := client.AuthIdentity.UpdateOneID(identity.ID).
			SetMetadata(mergeOAuthMetadata(identity.Metadata, metadata))
		if identity.UserID != userID {
			update = update.SetUserID(userID)
	REDACTED
		if !strings.EqualFold(strings.TrimSpace(identity.ProviderKey), providerKey) && !hasCanonicalKey {
			update = update.SetProviderKey(providerKey)
	REDACTED
		if issuer := oauthIdentityIssuer(session); issuer != nil {
			update = update.SetIssuer(strings.TrimSpace(*issuer))
	REDACTED
		identity, err = update.Save(ctx)
		if err != nil {
			return nil, err
	REDACTED
	case legacyOpenIDIdentity != nil:
		update := client.AuthIdentity.UpdateOneID(legacyOpenIDIdentity.ID).
			SetProviderKey(providerKey).
			SetProviderSubject(providerSubject).
			SetMetadata(mergeOAuthMetadata(legacyOpenIDIdentity.Metadata, metadata))
		if issuer := oauthIdentityIssuer(session); issuer != nil {
			update = update.SetIssuer(strings.TrimSpace(*issuer))
	REDACTED
		identity, err = update.Save(ctx)
		if err != nil {
			return nil, err
	REDACTED
	default:
		create := client.AuthIdentity.Create().
			SetUserID(userID).
			SetProviderType(providerType).
			SetProviderKey(providerKey).
			SetProviderSubject(providerSubject).
			SetMetadata(metadata)
		if issuer := oauthIdentityIssuer(session); issuer != nil {
			create = create.SetIssuer(strings.TrimSpace(*issuer))
	REDACTED
		identity, err = create.Save(ctx)
		if err != nil {
			return nil, err
	REDACTED
REDACTED

	if channel == "" || channelAppID == "" || channelSubject == "" {
		return identity, nil
REDACTED

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
REDACTED
	channelRecord, hasCanonicalChannelKey, err := chooseWeChatChannelForUser(ctx, client, channelRecords, userID, providerKey)
	if err != nil {
		return nil, err
REDACTED

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
	REDACTED
		return identity, nil
REDACTED

	updateChannel := client.AuthIdentityChannel.UpdateOneID(channelRecord.ID).
		SetIdentityID(identity.ID).
		SetMetadata(channelMetadata)
	if !strings.EqualFold(strings.TrimSpace(channelRecord.ProviderKey), providerKey) && !hasCanonicalChannelKey {
		updateChannel = updateChannel.SetProviderKey(providerKey)
REDACTED
	_, err = updateChannel.Save(ctx)
	if err != nil {
		return nil, err
REDACTED
	return identity, nil
REDACTED

func chooseWeChatIdentityForUser(ctx context.Context, client *dbent.Client, records []*dbent.AuthIdentity, userID int64, preferredProviderKey string) (*dbent.AuthIdentity, bool, error) {
	var preferred *dbent.AuthIdentity
	var fallback *dbent.AuthIdentity
	hasCanonicalKey := false
	for _, record := range records {
		if record == nil {
			continue
	REDACTED
		if record.UserID != userID {
			activeOwner, err := findActiveUserByID(ctx, client, record.UserID)
			if err != nil {
				return nil, false, err
		REDACTED
			if activeOwner != nil {
				return nil, false, infraerrors.Conflict("AUTH_IDENTITY_OWNERSHIP_CONFLICT", "auth identity already belongs to another user")
		REDACTED
	REDACTED
		if strings.EqualFold(strings.TrimSpace(record.ProviderKey), preferredProviderKey) {
			hasCanonicalKey = true
			if preferred == nil {
				preferred = record
		REDACTED
			continue
	REDACTED
		if fallback == nil {
			fallback = record
	REDACTED
REDACTED
	if preferred != nil {
		return preferred, hasCanonicalKey, nil
REDACTED
	return fallback, hasCanonicalKey, nil
REDACTED

func chooseWeChatChannelForUser(ctx context.Context, client *dbent.Client, records []*dbent.AuthIdentityChannel, userID int64, preferredProviderKey string) (*dbent.AuthIdentityChannel, bool, error) {
	var preferred *dbent.AuthIdentityChannel
	var fallback *dbent.AuthIdentityChannel
	hasCanonicalKey := false
	for _, record := range records {
		if record == nil {
			continue
	REDACTED
		if record.Edges.Identity != nil && record.Edges.Identity.UserID != userID {
			activeOwner, err := findActiveUserByID(ctx, client, record.Edges.Identity.UserID)
			if err != nil {
				return nil, false, err
		REDACTED
			if activeOwner != nil {
				return nil, false, infraerrors.Conflict("AUTH_IDENTITY_CHANNEL_OWNERSHIP_CONFLICT", "auth identity channel already belongs to another user")
		REDACTED
	REDACTED
		if strings.EqualFold(strings.TrimSpace(record.ProviderKey), preferredProviderKey) {
			hasCanonicalKey = true
			if preferred == nil {
				preferred = record
		REDACTED
			continue
	REDACTED
		if fallback == nil {
			fallback = record
	REDACTED
REDACTED
	if preferred != nil {
		return preferred, hasCanonicalKey, nil
REDACTED
	return fallback, hasCanonicalKey, nil
REDACTED

func findActiveUserByID(ctx context.Context, client *dbent.Client, userID int64) (*dbent.User, error) {
	if client == nil || userID <= 0 {
		return nil, nil
REDACTED
	userEntity, err := client.User.Get(ctx, userID)
	if err != nil {
		if dbent.IsNotFound(err) {
			return nil, nil
	REDACTED
		return nil, infraerrors.InternalServer("AUTH_IDENTITY_USER_LOOKUP_FAILED", "failed to load auth identity user").WithCause(err)
REDACTED
	if !strings.EqualFold(strings.TrimSpace(userEntity.Status), service.StatusActive) {
		return nil, service.ErrUserNotActive
REDACTED
	return userEntity, nil
REDACTED

func channelRecordMetadata(channel *dbent.AuthIdentityChannel) map[string]any {
	if channel == nil {
		return map[string]any{REDACTED
REDACTED
	return cloneOAuthMetadata(channel.Metadata)
REDACTED

func shouldBindPendingOAuthIdentity(session *dbent.PendingAuthSession, decision *dbent.IdentityAdoptionDecision) bool {
	if session == nil || decision == nil {
		return false
REDACTED
	switch strings.ToLower(strings.TrimSpace(session.Intent)) {
	case "bind_current_user", "login", "adopt_existing_user_by_email":
		return true
	default:
		return decision.AdoptDisplayName || decision.AdoptAvatar
REDACTED
REDACTED

func shouldSkipAvatarAdoption(err error) bool {
	return errors.Is(err, service.ErrAvatarInvalid) ||
		errors.Is(err, service.ErrAvatarTooLarge) ||
		errors.Is(err, service.ErrAvatarNotImage)
REDACTED

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
REDACTED
	if !forceBind && !shouldBindPendingOAuthIdentity(session, decision) {
		return nil
REDACTED

	if tx := dbent.TxFromContext(ctx); tx != nil {
		return applyPendingOAuthBindingTx(ctx, tx, authService, userService, session, decision, overrideUserID, forceBind, applyFirstBindDefaults)
REDACTED

	tx, err := client.Tx(ctx)
	if err != nil {
		return err
REDACTED
	defer func() { _ = tx.Rollback() REDACTED()

	txCtx := dbent.NewTxContext(ctx, tx)
	if err := applyPendingOAuthBindingTx(txCtx, tx, authService, userService, session, decision, overrideUserID, forceBind, applyFirstBindDefaults); err != nil {
		return err
REDACTED
	return tx.Commit()
REDACTED

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
REDACTED
	if !forceBind && !shouldBindPendingOAuthIdentity(session, decision) {
		return nil
REDACTED

	targetUserID := int64(0)
	if overrideUserID != nil && *overrideUserID > 0 {
		targetUserID = *overrideUserID
REDACTED else {
		resolvedUserID, err := resolvePendingOAuthTargetUserID(ctx, tx.Client(), session)
		if err != nil {
			return err
	REDACTED
		targetUserID = resolvedUserID
REDACTED

	adoptedDisplayName := ""
	if decision != nil && decision.AdoptDisplayName {
		adoptedDisplayName = normalizeAdoptedOAuthDisplayName(pendingSessionStringValue(session.UpstreamIdentityClaims, "suggested_display_name"))
REDACTED
	adoptedAvatarURL := ""
	if decision != nil && decision.AdoptAvatar {
		adoptedAvatarURL = pendingSessionStringValue(session.UpstreamIdentityClaims, "suggested_avatar_url")
REDACTED
	shouldAdoptAvatar := false
	if decision != nil && decision.AdoptAvatar && adoptedAvatarURL != "" {
		if err := service.ValidateUserAvatar(adoptedAvatarURL); err == nil {
			shouldAdoptAvatar = true
	REDACTED else if !shouldSkipAvatarAdoption(err) {
			return err
	REDACTED
REDACTED

	if decision != nil && decision.AdoptDisplayName && adoptedDisplayName != "" {
		if err := tx.Client().User.UpdateOneID(targetUserID).
			SetUsername(adoptedDisplayName).
			Exec(ctx); err != nil {
			return err
	REDACTED
REDACTED

	identity, err := ensurePendingOAuthIdentityForUser(ctx, tx, session, targetUserID)
	if err != nil {
		return err
REDACTED

	metadata := cloneOAuthMetadata(identity.Metadata)
	for key, value := range session.UpstreamIdentityClaims {
		metadata[key] = value
REDACTED
	if decision != nil && decision.AdoptDisplayName && adoptedDisplayName != "" {
		metadata["display_name"] = adoptedDisplayName
REDACTED
	if shouldAdoptAvatar {
		metadata["avatar_url"] = adoptedAvatarURL
REDACTED

	updateIdentity := tx.Client().AuthIdentity.UpdateOneID(identity.ID).SetMetadata(metadata)
	if issuer := oauthIdentityIssuer(session); issuer != nil {
		updateIdentity = updateIdentity.SetIssuer(strings.TrimSpace(*issuer))
REDACTED
	if _, err := updateIdentity.Save(ctx); err != nil {
		return err
REDACTED

	if decision != nil && (decision.IdentityID == nil || *decision.IdentityID != identity.ID) {
		if _, err := tx.Client().IdentityAdoptionDecision.Update().
			Where(
				identityadoptiondecision.IdentityIDEQ(identity.ID),
				identityadoptiondecision.IDNEQ(decision.ID),
			).
			ClearIdentityID().
			Save(ctx); err != nil {
			return err
	REDACTED
		if _, err := tx.Client().IdentityAdoptionDecision.UpdateOneID(decision.ID).
			SetIdentityID(identity.ID).
			Save(ctx); err != nil {
			return err
	REDACTED
REDACTED

	if applyFirstBindDefaults && authService != nil {
		if err := authService.ApplyProviderDefaultSettingsOnFirstBind(ctx, targetUserID, session.ProviderType); err != nil {
			return err
	REDACTED
REDACTED

	if shouldAdoptAvatar && userService != nil {
		if _, err := userService.SetAvatar(ctx, targetUserID, adoptedAvatarURL); err != nil {
			return err
	REDACTED
REDACTED

	return nil
REDACTED

func consumePendingOAuthBrowserSessionTx(
	ctx context.Context,
	tx *dbent.Tx,
	session *dbent.PendingAuthSession,
) error {
	if tx == nil || session == nil {
		return service.ErrPendingAuthSessionNotFound
REDACTED

	storedSession, err := tx.Client().PendingAuthSession.Get(ctx, session.ID)
	if err != nil {
		if dbent.IsNotFound(err) {
			return service.ErrPendingAuthSessionNotFound
	REDACTED
		return err
REDACTED

	now := time.Now().UTC()
	if storedSession.ConsumedAt != nil {
		return service.ErrPendingAuthSessionConsumed
REDACTED
	if !storedSession.ExpiresAt.IsZero() && now.After(storedSession.ExpiresAt) {
		return service.ErrPendingAuthSessionExpired
REDACTED
	if strings.TrimSpace(storedSession.BrowserSessionKey) != "" &&
		strings.TrimSpace(storedSession.BrowserSessionKey) != strings.TrimSpace(session.BrowserSessionKey) {
		return service.ErrPendingAuthBrowserMismatch
REDACTED

	if _, err := tx.Client().PendingAuthSession.UpdateOneID(storedSession.ID).
		SetConsumedAt(now).
		SetCompletionCodeHash("").
		ClearCompletionCodeExpiresAt().
		Save(ctx); err != nil {
		return err
REDACTED

	return nil
REDACTED

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
REDACTED
	if session == nil || userID <= 0 {
		return infraerrors.BadRequest("PENDING_AUTH_SESSION_INVALID", "pending auth registration context is invalid")
REDACTED

	tx, err := client.Tx(ctx)
	if err != nil {
		return err
REDACTED
	defer func() { _ = tx.Rollback() REDACTED()

	txCtx := dbent.NewTxContext(ctx, tx)
	if err := applyPendingOAuthAdoption(txCtx, client, authService, userService, session, decision, &userID); err != nil {
		return err
REDACTED
	if err := consumePendingOAuthBrowserSessionTx(txCtx, tx, session); err != nil {
		return err
REDACTED
	return tx.Commit()
REDACTED

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
REDACTED

func applySuggestedProfileToCompletionResponse(payload map[string]any, upstream map[string]any) {
	if len(payload) == 0 || len(upstream) == 0 {
		return
REDACTED

	displayName := pendingSessionStringValue(upstream, "suggested_display_name")
	avatarURL := pendingSessionStringValue(upstream, "suggested_avatar_url")

	if displayName != "" {
		if _, exists := payload["suggested_display_name"]; !exists {
			payload["suggested_display_name"] = displayName
	REDACTED
REDACTED
	if avatarURL != "" {
		if _, exists := payload["suggested_avatar_url"]; !exists {
			payload["suggested_avatar_url"] = avatarURL
	REDACTED
REDACTED
	if displayName != "" || avatarURL != "" {
		payload["adoption_required"] = true
REDACTED
REDACTED

func pendingOAuthIdentityExistsForUser(
	ctx context.Context,
	client *dbent.Client,
	session *dbent.PendingAuthSession,
	userID int64,
) (bool, error) {
	if client == nil || session == nil || userID <= 0 {
		return false, nil
REDACTED

	providerType := strings.TrimSpace(session.ProviderType)
	providerKey := strings.TrimSpace(session.ProviderKey)
	providerSubject := strings.TrimSpace(session.ProviderSubject)
	if providerType == "" || providerSubject == "" {
		return false, nil
REDACTED

	query := client.AuthIdentity.Query().
		Where(
			authidentity.ProviderTypeEQ(providerType),
			authidentity.ProviderSubjectEQ(providerSubject),
			authidentity.UserIDEQ(userID),
		)
	if strings.EqualFold(providerType, "wechat") {
		query = query.Where(authidentity.ProviderKeyIn(wechatCompatibleProviderKeys(providerKey)...))
REDACTED else if providerKey != "" {
		query = query.Where(authidentity.ProviderKeyEQ(providerKey))
REDACTED

	count, err := query.Count(ctx)
	if err != nil {
		return false, infraerrors.InternalServer("AUTH_IDENTITY_LOOKUP_FAILED", "failed to inspect auth identity ownership").WithCause(err)
REDACTED
	return count > 0, nil
REDACTED

func (h *AuthHandler) shouldSkipPendingOAuthAdoptionPrompt(
	ctx context.Context,
	session *dbent.PendingAuthSession,
	payload map[string]any,
) (bool, error) {
	if session == nil || len(payload) == 0 {
		return false, nil
REDACTED
	if !pendingOAuthCompletionCanIssueTokenPair(session, payload) {
		return false, nil
REDACTED
	if pendingSessionStringValue(session.UpstreamIdentityClaims, "suggested_display_name") == "" &&
		pendingSessionStringValue(session.UpstreamIdentityClaims, "suggested_avatar_url") == "" {
		return false, nil
REDACTED

	return pendingOAuthIdentityExistsForUser(ctx, h.entClient(), session, *session.TargetUserID)
REDACTED

func readPendingOAuthBrowserSession(c *gin.Context, h *AuthHandler) (*service.AuthPendingIdentityService, *dbent.PendingAuthSession, func(), error) {
	secureCookie := isRequestHTTPS(c)
	clearCookies := func() {
		clearOAuthPendingSessionCookie(c, secureCookie)
		clearOAuthPendingBrowserCookie(c, secureCookie)
REDACTED

	sessionToken, err := readOAuthPendingSessionCookie(c)
	if err != nil || strings.TrimSpace(sessionToken) == "" {
		clearCookies()
		return nil, nil, clearCookies, service.ErrPendingAuthSessionNotFound
REDACTED
	browserSessionKey, err := readOAuthPendingBrowserCookie(c)
	if err != nil || strings.TrimSpace(browserSessionKey) == "" {
		clearCookies()
		return nil, nil, clearCookies, service.ErrPendingAuthBrowserMismatch
REDACTED

	svc, err := h.pendingIdentityService()
	if err != nil {
		clearCookies()
		return nil, nil, clearCookies, err
REDACTED

	session, err := svc.GetBrowserSession(c.Request.Context(), sessionToken, browserSessionKey)
	if err != nil {
		clearCookies()
		return nil, nil, clearCookies, err
REDACTED

	return svc, session, clearCookies, nil
REDACTED

func (h *AuthHandler) consumePendingOAuthSessionOnLogout(c *gin.Context) {
	if c == nil || c.Request == nil {
		return
REDACTED

	sessionToken, err := readOAuthPendingSessionCookie(c)
	if err != nil || strings.TrimSpace(sessionToken) == "" {
		return
REDACTED
	browserSessionKey, err := readOAuthPendingBrowserCookie(c)
	if err != nil || strings.TrimSpace(browserSessionKey) == "" {
		return
REDACTED

	svc, err := h.pendingIdentityService()
	if err != nil {
		return
REDACTED
	_, _ = svc.ConsumeBrowserSession(c.Request.Context(), sessionToken, browserSessionKey)
REDACTED

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
REDACTED

func buildPendingOAuthSessionStatusPayload(session *dbent.PendingAuthSession) gin.H {
	completionResponse := normalizePendingOAuthCompletionResponse(mergePendingCompletionResponse(session, nil))
	payload := gin.H{
		"auth_result": "pending_session",
		"provider":    strings.TrimSpace(session.ProviderType),
		"intent":      strings.TrimSpace(session.Intent),
REDACTED
	for key, value := range completionResponse {
		payload[key] = value
REDACTED
	if email := strings.TrimSpace(session.ResolvedEmail); email != "" {
		payload["email"] = email
REDACTED
	return payload
REDACTED

func normalizePendingOAuthCompletionResponse(payload map[string]any) map[string]any {
	normalized := clonePendingMap(payload)
	for _, key := range []string{"access_token", "refresh_token", "expires_in", "token_type"REDACTED {
		delete(normalized, key)
REDACTED
	step := strings.ToLower(strings.TrimSpace(pendingSessionStringValue(normalized, "step")))
	// 把多种 choice 别名归一为 oauthPendingChoiceStep；bind_login_required 是独立终态
	// （前端渲染 needsBindLogin 而非 needsChooser），故不能并入归一化列表。
	switch step {
	case "choice", "choose_account_action", "choose_account", "choose", "email_required":
		normalized["step"] = oauthPendingChoiceStep
REDACTED
	if strings.EqualFold(strings.TrimSpace(pendingSessionStringValue(normalized, "step")), oauthPendingChoiceStep) {
		normalized["adoption_required"] = true
REDACTED
	if _, exists := normalized["adoption_required"]; !exists {
		if _, hasChoiceFields := normalized["email_binding_required"]; hasChoiceFields {
			normalized["adoption_required"] = true
	REDACTED
REDACTED
	return normalized
REDACTED

func pendingOAuthChoiceCompletionResponse(session *dbent.PendingAuthSession, email string) map[string]any {
	response := mergePendingCompletionResponse(session, map[string]any{
		"step":                      oauthPendingChoiceStep,
		"adoption_required":         true,
		"force_email_on_signup":     true,
		"email_binding_required":    true,
		"existing_account_bindable": true,
REDACTED)
	if email = strings.TrimSpace(email); email != "" {
		response["email"] = email
		response["resolved_email"] = email
REDACTED
	return response
REDACTED

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
REDACTED
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
REDACTED
	return session, nil
REDACTED

func writeOAuthTokenPairResponse(c *gin.Context, tokenPair *service.TokenPair) {
	c.JSON(http.StatusOK, gin.H{
		"access_token":  tokenPair.AccessToken,
		"refresh_token": tokenPair.RefreshToken,
		"expires_in":    tokenPair.ExpiresIn,
		"token_type":    "Bearer",
REDACTED)
REDACTED

func (h *AuthHandler) bindPendingOAuthLogin(c *gin.Context, provider string) {
	var req bindPendingOAuthLoginRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "Invalid request: "+err.Error())
		return
REDACTED

	pendingSvc, session, clearCookies, err := readPendingOAuthBrowserSession(c, h)
	if err != nil {
		response.ErrorFrom(c, err)
		return
REDACTED
	if strings.TrimSpace(provider) != "" && !strings.EqualFold(strings.TrimSpace(session.ProviderType), provider) {
		response.BadRequest(c, "Pending oauth session provider mismatch")
		return
REDACTED

	user, err := h.authService.ValidatePasswordCredentials(c.Request.Context(), strings.TrimSpace(req.Email), req.Password)
	if err != nil {
		response.ErrorFrom(c, err)
		return
REDACTED
	if session.TargetUserID != nil && *session.TargetUserID > 0 && user.ID != *session.TargetUserID {
		response.ErrorFrom(c, infraerrors.Conflict("PENDING_AUTH_TARGET_USER_MISMATCH", "pending oauth session must be completed by the targeted user"))
		return
REDACTED
	if err := h.ensureBackendModeAllowsUser(c.Request.Context(), user); err != nil {
		response.ErrorFrom(c, err)
		return
REDACTED

	decision, err := h.ensurePendingOAuthAdoptionDecision(c, session.ID, req.adoptionDecision())
	if err != nil {
		response.ErrorFrom(c, err)
		return
REDACTED
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
	REDACTED
		response.Success(c, TotpLoginResponse{
			Requires2FA:     true,
			TempToken:       tempToken,
			UserEmailMasked: service.MaskEmail(user.Email),
	REDACTED)
		return
REDACTED
	if err := applyPendingOAuthBinding(c.Request.Context(), h.entClient(), h.authService, h.userService, session, decision, &user.ID, true, true); err != nil {
		respondPendingOAuthBindingApplyError(c, err)
		return
REDACTED

	h.authService.RecordSuccessfulLogin(c.Request.Context(), user.ID)
	// bindPendingOAuthLogin = 绑定已有账户登录，不动 users.username（用户已有自己的名字）
	h.maybeSyncDingTalkAfterLogin(c.Request.Context(), session, user.ID)
	tokenPair, err := h.authService.GenerateTokenPair(c.Request.Context(), user, "")
	if err != nil {
		response.InternalError(c, "Failed to generate token pair")
		return
REDACTED
	if _, err := pendingSvc.ConsumeBrowserSession(c.Request.Context(), session.SessionToken, session.BrowserSessionKey); err != nil {
		clearCookies()
		response.ErrorFrom(c, err)
		return
REDACTED

	clearCookies()
	writeOAuthTokenPairResponse(c, tokenPair)
REDACTED

func respondPendingOAuthBindingApplyError(c *gin.Context, err error) {
	if code := infraerrors.Code(err); code >= http.StatusBadRequest && code < http.StatusInternalServerError {
		response.ErrorFrom(c, err)
		return
REDACTED
	response.ErrorFrom(c, infraerrors.InternalServer("PENDING_AUTH_BIND_APPLY_FAILED", "failed to bind pending oauth identity").WithCause(err))
REDACTED

func (h *AuthHandler) createPendingOAuthAccount(c *gin.Context, provider string) {
	var req createPendingOAuthAccountRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "Invalid request: "+err.Error())
		return
REDACTED

	_, session, clearCookies, err := readPendingOAuthBrowserSession(c, h)
	if err != nil {
		response.ErrorFrom(c, err)
		return
REDACTED
	if err := ensurePendingOAuthCompleteRegistrationSession(session); err != nil {
		response.ErrorFrom(c, err)
		return
REDACTED
	if strings.TrimSpace(provider) != "" && !strings.EqualFold(strings.TrimSpace(session.ProviderType), provider) {
		response.BadRequest(c, "Pending oauth session provider mismatch")
		return
REDACTED

	client := h.entClient()
	if client == nil {
		response.ErrorFrom(c, infraerrors.ServiceUnavailable("PENDING_AUTH_NOT_READY", "pending auth service is not ready"))
		return
REDACTED

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
	REDACTED
REDACTED
	if existingUser != nil {
		session, err = h.transitionPendingOAuthAccountToChoiceState(c, client, session, existingUser, email)
		if err != nil {
			response.ErrorFrom(c, err)
			return
	REDACTED
		c.JSON(http.StatusOK, buildPendingOAuthSessionStatusPayload(session))
		return
REDACTED
	if err := h.ensureBackendModeAllowsNewUserLogin(c.Request.Context()); err != nil {
		response.ErrorFrom(c, err)
		return
REDACTED

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
		REDACTED
			session, err = h.transitionPendingOAuthAccountToChoiceState(c, client, session, existingUser, email)
			if err != nil {
				response.ErrorFrom(c, err)
				return
		REDACTED
			c.JSON(http.StatusOK, buildPendingOAuthSessionStatusPayload(session))
			return
	REDACTED
		response.ErrorFrom(c, err)
		return
REDACTED

	rollbackCreatedUser := func(originalErr error) bool {
		if user == nil || user.ID <= 0 {
			return false
	REDACTED
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
	REDACTED
		user = nil
		return false
REDACTED

	decision, err := h.ensurePendingOAuthAdoptionDecision(c, session.ID, req.adoptionDecision())
	if err != nil {
		if rollbackCreatedUser(err) {
			return
	REDACTED
		response.ErrorFrom(c, err)
		return
REDACTED

	tx, err := client.Tx(c.Request.Context())
	if err != nil {
		if rollbackCreatedUser(err) {
			return
	REDACTED
		response.ErrorFrom(c, infraerrors.InternalServer("PENDING_AUTH_BIND_APPLY_FAILED", "failed to bind pending oauth identity").WithCause(err))
		return
REDACTED
	defer func() { _ = tx.Rollback() REDACTED()
	txCtx := dbent.NewTxContext(c.Request.Context(), tx)

	if err := applyPendingOAuthBinding(txCtx, client, h.authService, h.userService, session, decision, &user.ID, true, false); err != nil {
		_ = tx.Rollback()
		if rollbackCreatedUser(err) {
			return
	REDACTED
		respondPendingOAuthBindingApplyError(c, err)
		return
REDACTED

	if err := h.authService.FinalizeOAuthEmailAccount(
		txCtx,
		user,
		strings.TrimSpace(req.InvitationCode),
		strings.TrimSpace(session.ProviderType),
		strings.TrimSpace(req.AffCode),
	); err != nil {
		_ = tx.Rollback()
		if rollbackCreatedUser(err) {
			return
	REDACTED
		response.ErrorFrom(c, err)
		return
REDACTED

	if err := consumePendingOAuthBrowserSessionTx(txCtx, tx, session); err != nil {
		_ = tx.Rollback()
		if rollbackCreatedUser(err) {
			return
	REDACTED
		clearCookies()
		response.ErrorFrom(c, err)
		return
REDACTED

	if pendingOAuthCreateAccountPreCommitHook != nil {
		if err := pendingOAuthCreateAccountPreCommitHook(txCtx, session); err != nil {
			_ = tx.Rollback()
			if rollbackCreatedUser(err) {
				return
		REDACTED
			respondPendingOAuthBindingApplyError(c, err)
			return
	REDACTED
REDACTED

	if err := tx.Commit(); err != nil {
		if rollbackCreatedUser(err) {
			return
	REDACTED
		response.ErrorFrom(c, infraerrors.InternalServer("PENDING_AUTH_BIND_APPLY_FAILED", "failed to bind pending oauth identity").WithCause(err))
		return
REDACTED

	h.authService.ApplyOAuthSignupPromoCode(c.Request.Context(), user.ID, pendingOAuthPromoCode(session))
	h.authService.RecordSuccessfulLogin(c.Request.Context(), user.ID)
	// createPendingOAuthAccount = 注册新账户，需要把钉钉昵称同步到 users.username 作为初始值
	h.maybeSyncDingTalkAfterRegistration(c.Request.Context(), session, user.ID)
	clearCookies()
	writeOAuthTokenPairResponse(c, tokenPair)
REDACTED

// ExchangePendingOAuthCompletion redeems a pending OAuth browser session into a frontend-safe payload.
// POST /api/v1/auth/oauth/pending/exchange
func (h *AuthHandler) ExchangePendingOAuthCompletion(c *gin.Context) {
	secureCookie := isRequestHTTPS(c)
	clearCookies := func() {
		clearOAuthPendingSessionCookie(c, secureCookie)
		clearOAuthPendingBrowserCookie(c, secureCookie)
REDACTED
	adoptionDecision, err := bindOptionalOAuthAdoptionDecision(c)
	if err != nil {
		response.BadRequest(c, "Invalid request: "+err.Error())
		return
REDACTED

	sessionToken, err := readOAuthPendingSessionCookie(c)
	if err != nil || strings.TrimSpace(sessionToken) == "" {
		clearCookies()
		response.ErrorFrom(c, service.ErrPendingAuthSessionNotFound)
		return
REDACTED
	browserSessionKey, err := readOAuthPendingBrowserCookie(c)
	if err != nil || strings.TrimSpace(browserSessionKey) == "" {
		clearCookies()
		response.ErrorFrom(c, service.ErrPendingAuthBrowserMismatch)
		return
REDACTED

	svc, err := h.pendingIdentityService()
	if err != nil {
		clearCookies()
		response.ErrorFrom(c, err)
		return
REDACTED

	session, err := svc.GetBrowserSession(c.Request.Context(), sessionToken, browserSessionKey)
	if err != nil {
		clearCookies()
		response.ErrorFrom(c, err)
		return
REDACTED

	payload, ok := readCompletionResponse(session.LocalFlowState)
	if !ok {
		clearCookies()
		response.ErrorFrom(c, infraerrors.InternalServer("PENDING_AUTH_COMPLETION_INVALID", "pending auth completion payload is invalid"))
		return
REDACTED
	payload = normalizePendingOAuthCompletionResponse(payload)
	if strings.TrimSpace(session.RedirectTo) != "" {
		if _, exists := payload["redirect"]; !exists {
			payload["redirect"] = session.RedirectTo
	REDACTED
REDACTED
	applySuggestedProfileToCompletionResponse(payload, session.UpstreamIdentityClaims)

	canIssueTokenPair := pendingOAuthCompletionCanIssueTokenPair(session, payload)
	var loginUser *service.User
	if canIssueTokenPair {
		loginUser, err = h.userService.GetByID(c.Request.Context(), *session.TargetUserID)
		if err != nil {
			clearCookies()
			response.ErrorFrom(c, err)
			return
	REDACTED
		if err := ensureLoginUserActive(loginUser); err != nil {
			clearCookies()
			response.ErrorFrom(c, err)
			return
	REDACTED
		if err := h.ensureBackendModeAllowsUser(c.Request.Context(), loginUser); err != nil {
			clearCookies()
			response.ErrorFrom(c, err)
			return
	REDACTED
REDACTED
	skipAdoptionPrompt, err := h.shouldSkipPendingOAuthAdoptionPrompt(c.Request.Context(), session, payload)
	if err != nil {
		clearCookies()
		response.ErrorFrom(c, err)
		return
REDACTED
	if skipAdoptionPrompt {
		delete(payload, "adoption_required")
REDACTED

	if pendingSessionWantsInvitation(payload) {
		if adoptionDecision.hasDecision() {
			decision, err := h.upsertPendingOAuthAdoptionDecision(c, session.ID, adoptionDecision)
			if err != nil {
				response.ErrorFrom(c, err)
				return
		REDACTED
			_ = decision
	REDACTED
		response.Success(c, payload)
		return
REDACTED
	if pendingSessionRequiresEmailCompletion(payload) {
		response.Success(c, payload)
		return
REDACTED
	if pendingSessionRequiresBindLogin(payload) {
		response.Success(c, payload)
		return
REDACTED
	if !adoptionDecision.hasDecision() {
		adoptionRequired, _ := payload["adoption_required"].(bool)
		if adoptionRequired {
			response.Success(c, payload)
			return
	REDACTED
REDACTED

	decisionReq := adoptionDecision
	if !decisionReq.hasDecision() {
		adoptDisplayName := false
		adoptAvatar := false
		decisionReq = oauthAdoptionDecisionRequest{
			AdoptDisplayName: &adoptDisplayName,
			AdoptAvatar:      &adoptAvatar,
	REDACTED
REDACTED

	decision, err := h.ensurePendingOAuthAdoptionDecision(c, session.ID, decisionReq)
	if err != nil {
		response.ErrorFrom(c, err)
		return
REDACTED
	if err := applyPendingOAuthAdoption(c.Request.Context(), h.entClient(), h.authService, h.userService, session, decision, session.TargetUserID); err != nil {
		response.ErrorFrom(c, infraerrors.InternalServer("PENDING_AUTH_ADOPTION_APPLY_FAILED", "failed to apply oauth profile adoption").WithCause(err))
		return
REDACTED

	if _, err := svc.ConsumeBrowserSession(c.Request.Context(), sessionToken, browserSessionKey); err != nil {
		clearCookies()
		response.ErrorFrom(c, err)
		return
REDACTED

	if canIssueTokenPair {
		tokenPair, err := h.authService.GenerateTokenPair(c.Request.Context(), loginUser, "")
		if err != nil {
			clearCookies()
			response.InternalError(c, "Failed to generate token pair")
			return
	REDACTED
		h.authService.RecordSuccessfulLogin(c.Request.Context(), loginUser.ID)
		payload["access_token"] = tokenPair.AccessToken
		payload["refresh_token"] = tokenPair.RefreshToken
		payload["expires_in"] = tokenPair.ExpiresIn
		payload["token_type"] = "Bearer"
REDACTED

	clearCookies()
	response.Success(c, payload)
REDACTED
