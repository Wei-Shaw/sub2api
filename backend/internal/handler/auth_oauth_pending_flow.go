package handler

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/url"
	"strings"

	dbent "github.com/Wei-Shaw/sub2api/ent"
	"github.com/Wei-Shaw/sub2api/ent/authidentity"
	"github.com/Wei-Shaw/sub2api/ent/authidentitychannel"
	"github.com/Wei-Shaw/sub2api/ent/identityadoptiondecision"
	dbuser "github.com/Wei-Shaw/sub2api/ent/user"
	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
	"github.com/Wei-Shaw/sub2api/internal/pkg/oauth"
	"github.com/Wei-Shaw/sub2api/internal/pkg/response"
	"github.com/Wei-Shaw/sub2api/internal/service"

	"github.com/gin-gonic/gin"
)

const (
	oauthPendingBrowserCookiePath = "/api/v1/auth/oauth"
	oauthPendingBrowserCookieName = "oauth_pending_browser_session"
	oauthPendingSessionCookiePath = "/api/v1/auth/oauth/pending"
	oauthPendingSessionCookieName = "oauth_pending_session"
	oauthPendingCookieMaxAgeSec   = 10 * 60

	oauthCompletionResponseKey = "completion_response"
)

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
	AdoptDisplayName *bool  `json:"adopt_display_name,omitempty"`
	AdoptAvatar      *bool  `json:"adopt_avatar,omitempty"`
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
	REDACTED,
REDACTED)
	if err != nil {
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

func (r oauthAdoptionDecisionRequest) hasDecision() bool {
	return r.AdoptDisplayName != nil || r.AdoptAvatar != nil
REDACTED

func (r oauthAdoptionDecisionRequest) toServiceInput(sessionID int64) service.PendingIdentityAdoptionDecisionInput {
	input := service.PendingIdentityAdoptionDecisionInput{
		PendingAuthSessionID: sessionID,
REDACTED
	if r.AdoptDisplayName != nil {
		input.AdoptDisplayName = *r.AdoptDisplayName
REDACTED
	if r.AdoptAvatar != nil {
		input.AdoptAvatar = *r.AdoptAvatar
REDACTED
	return input
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

func persistPendingOAuthAdoptionDecision(
	c *gin.Context,
	svc *service.AuthPendingIdentityService,
	sessionID int64,
	req oauthAdoptionDecisionRequest,
) error {
	if !req.hasDecision() {
		return nil
REDACTED
	if svc == nil {
		return infraerrors.ServiceUnavailable("PENDING_AUTH_NOT_READY", "pending auth service is not ready")
REDACTED
	if _, err := svc.UpsertAdoptionDecision(c.Request.Context(), req.toServiceInput(sessionID)); err != nil {
		return infraerrors.InternalServer("PENDING_AUTH_ADOPTION_SAVE_FAILED", "failed to save oauth profile adoption decision").WithCause(err)
REDACTED
	return nil
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

	userEntity, err := client.User.Get(ctx, record.UserID)
	if err != nil {
		if dbent.IsNotFound(err) {
			return nil, nil
	REDACTED
		return nil, infraerrors.InternalServer("AUTH_IDENTITY_USER_LOOKUP_FAILED", "failed to load auth identity user").WithCause(err)
REDACTED
	return userEntity, nil
REDACTED

func (h *AuthHandler) createOAuthEmailRequiredPendingSession(
	c *gin.Context,
	identity service.PendingAuthIdentityKey,
	redirectTo string,
	browserSessionKey string,
	upstreamClaims map[string]any,
) error {
	return h.createOAuthPendingSession(c, oauthPendingSessionPayload{
		Intent:                 oauthIntentLogin,
		Identity:               identity,
		RedirectTo:             redirectTo,
		BrowserSessionKey:      browserSessionKey,
		UpstreamIdentityClaims: upstreamClaims,
		CompletionResponse: map[string]any{
			"redirect":                  redirectTo,
			"step":                      "email_required",
			"force_email_on_signup":     true,
			"email_binding_required":    true,
			"existing_account_bindable": true,
	REDACTED,
REDACTED)
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

	userEntity, err := client.User.Query().
		Where(dbuser.EmailEQ(email)).
		Only(ctx)
	if err != nil {
		if dbent.IsNotFound(err) {
			return 0, infraerrors.InternalServer("PENDING_AUTH_TARGET_USER_NOT_FOUND", "pending auth target user was not found")
	REDACTED
		return 0, err
REDACTED
	return userEntity.ID, nil
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
			return nil, infraerrors.Conflict("AUTH_IDENTITY_OWNERSHIP_CONFLICT", "auth identity already belongs to another user")
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
	identity, hasCanonicalKey, err := chooseWeChatIdentityForUser(identityRecords, userID, providerKey)
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
		legacyOpenIDIdentity, _, err = chooseWeChatIdentityForUser(legacyOpenIDRecords, userID, providerKey)
		if err != nil {
			return nil, err
	REDACTED
REDACTED

	switch {
	case identity != nil:
		update := client.AuthIdentity.UpdateOneID(identity.ID).
			SetMetadata(mergeOAuthMetadata(identity.Metadata, metadata))
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
	channelRecord, hasCanonicalChannelKey, err := chooseWeChatChannelForUser(channelRecords, userID, providerKey)
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

func chooseWeChatIdentityForUser(records []*dbent.AuthIdentity, userID int64, preferredProviderKey string) (*dbent.AuthIdentity, bool, error) {
	var preferred *dbent.AuthIdentity
	var fallback *dbent.AuthIdentity
	hasCanonicalKey := false
	for _, record := range records {
		if record == nil {
			continue
	REDACTED
		if record.UserID != userID {
			return nil, false, infraerrors.Conflict("AUTH_IDENTITY_OWNERSHIP_CONFLICT", "auth identity already belongs to another user")
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

func chooseWeChatChannelForUser(records []*dbent.AuthIdentityChannel, userID int64, preferredProviderKey string) (*dbent.AuthIdentityChannel, bool, error) {
	var preferred *dbent.AuthIdentityChannel
	var fallback *dbent.AuthIdentityChannel
	hasCanonicalKey := false
	for _, record := range records {
		if record == nil {
			continue
	REDACTED
		if record.Edges.Identity != nil && record.Edges.Identity.UserID != userID {
			return nil, false, infraerrors.Conflict("AUTH_IDENTITY_CHANNEL_OWNERSHIP_CONFLICT", "auth identity channel already belongs to another user")
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

func applyPendingOAuthBinding(
	ctx context.Context,
	client *dbent.Client,
	authService *service.AuthService,
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

	targetUserID := int64(0)
	if overrideUserID != nil && *overrideUserID > 0 {
		targetUserID = *overrideUserID
REDACTED else {
		resolvedUserID, err := resolvePendingOAuthTargetUserID(ctx, client, session)
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

	tx, err := client.Tx(ctx)
	if err != nil {
		return err
REDACTED
	defer func() { _ = tx.Rollback() REDACTED()
	txCtx := dbent.NewTxContext(ctx, tx)

	if decision != nil && decision.AdoptDisplayName && adoptedDisplayName != "" {
		if err := tx.Client().User.UpdateOneID(targetUserID).
			SetUsername(adoptedDisplayName).
			Exec(txCtx); err != nil {
			return err
	REDACTED
REDACTED

	identity, err := ensurePendingOAuthIdentityForUser(txCtx, tx, session, targetUserID)
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
	if decision != nil && decision.AdoptAvatar && adoptedAvatarURL != "" {
		metadata["avatar_url"] = adoptedAvatarURL
REDACTED

	updateIdentity := tx.Client().AuthIdentity.UpdateOneID(identity.ID).SetMetadata(metadata)
	if issuer := oauthIdentityIssuer(session); issuer != nil {
		updateIdentity = updateIdentity.SetIssuer(strings.TrimSpace(*issuer))
REDACTED
	if _, err := updateIdentity.Save(txCtx); err != nil {
		return err
REDACTED

	if decision != nil && (decision.IdentityID == nil || *decision.IdentityID != identity.ID) {
		if _, err := tx.Client().IdentityAdoptionDecision.UpdateOneID(decision.ID).
			SetIdentityID(identity.ID).
			Save(txCtx); err != nil {
			return err
	REDACTED
REDACTED

	if applyFirstBindDefaults && authService != nil {
		if err := authService.ApplyProviderDefaultSettingsOnFirstBind(txCtx, targetUserID, session.ProviderType); err != nil {
			return err
	REDACTED
REDACTED

	return tx.Commit()
REDACTED

func applyPendingOAuthAdoption(
	ctx context.Context,
	client *dbent.Client,
	authService *service.AuthService,
	session *dbent.PendingAuthSession,
	decision *dbent.IdentityAdoptionDecision,
	overrideUserID *int64,
) error {
	return applyPendingOAuthBinding(
		ctx,
		client,
		authService,
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

func buildPendingOAuthSessionStatusPayload(session *dbent.PendingAuthSession) gin.H {
	payload := gin.H{
		"auth_result": "pending_session",
		"provider":    strings.TrimSpace(session.ProviderType),
		"intent":      strings.TrimSpace(session.Intent),
REDACTED
	for key, value := range mergePendingCompletionResponse(session, nil) {
		payload[key] = value
REDACTED
	if email := strings.TrimSpace(session.ResolvedEmail); email != "" {
		payload["email"] = email
REDACTED
	return payload
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
	if err := applyPendingOAuthBinding(c.Request.Context(), h.entClient(), h.authService, session, decision, &user.ID, true, true); err != nil {
		response.ErrorFrom(c, infraerrors.InternalServer("PENDING_AUTH_BIND_APPLY_FAILED", "failed to bind pending oauth identity").WithCause(err))
		return
REDACTED

	h.authService.RecordSuccessfulLogin(c.Request.Context(), user.ID)
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

func (h *AuthHandler) createPendingOAuthAccount(c *gin.Context, provider string) {
	var req createPendingOAuthAccountRequest
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

	client := h.entClient()
	if client == nil {
		response.ErrorFrom(c, infraerrors.ServiceUnavailable("PENDING_AUTH_NOT_READY", "pending auth service is not ready"))
		return
REDACTED

	email := strings.TrimSpace(strings.ToLower(req.Email))
	existingUser, err := client.User.Query().Where(dbuser.EmailEQ(email)).Only(c.Request.Context())
	if err != nil && !dbent.IsNotFound(err) {
		response.ErrorFrom(c, infraerrors.ServiceUnavailable("SERVICE_UNAVAILABLE", "service temporarily unavailable"))
		return
REDACTED
	if existingUser != nil {
		completionResponse := mergePendingCompletionResponse(session, map[string]any{
			"step":  "bind_login_required",
			"email": email,
	REDACTED)
		session, err = updatePendingOAuthSessionProgress(
			c.Request.Context(),
			client,
			session,
			"adopt_existing_user_by_email",
			email,
			&existingUser.ID,
			completionResponse,
		)
		if err != nil {
			response.ErrorFrom(c, infraerrors.InternalServer("PENDING_AUTH_SESSION_UPDATE_FAILED", "failed to update pending oauth session").WithCause(err))
			return
	REDACTED

		if _, err := h.ensurePendingOAuthAdoptionDecision(c, session.ID, req.adoptionDecision()); err != nil {
			response.ErrorFrom(c, err)
			return
	REDACTED

		c.JSON(http.StatusOK, buildPendingOAuthSessionStatusPayload(session))
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
		response.ErrorFrom(c, err)
		return
REDACTED

	decision, err := h.ensurePendingOAuthAdoptionDecision(c, session.ID, req.adoptionDecision())
	if err != nil {
		response.ErrorFrom(c, err)
		return
REDACTED
	if err := applyPendingOAuthBinding(c.Request.Context(), client, h.authService, session, decision, &user.ID, true, false); err != nil {
		response.ErrorFrom(c, infraerrors.InternalServer("PENDING_AUTH_BIND_APPLY_FAILED", "failed to bind pending oauth identity").WithCause(err))
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
	if strings.TrimSpace(session.RedirectTo) != "" {
		if _, exists := payload["redirect"]; !exists {
			payload["redirect"] = session.RedirectTo
	REDACTED
REDACTED
	applySuggestedProfileToCompletionResponse(payload, session.UpstreamIdentityClaims)

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
	if !adoptionDecision.hasDecision() {
		response.Success(c, payload)
		return
REDACTED
	decision, err := h.upsertPendingOAuthAdoptionDecision(c, session.ID, adoptionDecision)
	if err != nil {
		response.ErrorFrom(c, err)
		return
REDACTED
	if err := applyPendingOAuthAdoption(c.Request.Context(), h.entClient(), h.authService, session, decision, session.TargetUserID); err != nil {
		response.ErrorFrom(c, infraerrors.InternalServer("PENDING_AUTH_ADOPTION_APPLY_FAILED", "failed to apply oauth profile adoption").WithCause(err))
		return
REDACTED

	if _, err := svc.ConsumeBrowserSession(c.Request.Context(), sessionToken, browserSessionKey); err != nil {
		clearCookies()
		response.ErrorFrom(c, err)
		return
REDACTED

	clearCookies()
	response.Success(c, payload)
REDACTED
