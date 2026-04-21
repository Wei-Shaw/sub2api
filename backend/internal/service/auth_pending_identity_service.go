package service

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"strings"
	"time"

	dbent "github.com/Wei-Shaw/sub2api/ent"
	"github.com/Wei-Shaw/sub2api/ent/identityadoptiondecision"
	dbpredicate "github.com/Wei-Shaw/sub2api/ent/predicate"
	"github.com/Wei-Shaw/sub2api/ent/pendingauthsession"
	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"

	entsql "entgo.io/ent/dialect/sql"
)

var (
	ErrPendingAuthSessionNotFound = infraerrors.NotFound("PENDING_AUTH_SESSION_NOT_FOUND", "pending auth session not found")
	ErrPendingAuthSessionExpired  = infraerrors.Unauthorized("PENDING_AUTH_SESSION_EXPIRED", "pending auth session has expired")
	ErrPendingAuthSessionConsumed = infraerrors.Unauthorized("PENDING_AUTH_SESSION_CONSUMED", "pending auth session has already been used")
	ErrPendingAuthCodeInvalid     = infraerrors.Unauthorized("PENDING_AUTH_CODE_INVALID", "pending auth completion code is invalid")
	ErrPendingAuthCodeExpired     = infraerrors.Unauthorized("PENDING_AUTH_CODE_EXPIRED", "pending auth completion code has expired")
	ErrPendingAuthCodeConsumed    = infraerrors.Unauthorized("PENDING_AUTH_CODE_CONSUMED", "pending auth completion code has already been used")
	ErrPendingAuthBrowserMismatch = infraerrors.Unauthorized("PENDING_AUTH_BROWSER_MISMATCH", "pending auth completion code does not match this browser session")
)

const (
	defaultPendingAuthTTL           = 15 * time.Minute
	defaultPendingAuthCompletionTTL = 5 * time.Minute
)

type PendingAuthIdentityKey struct {
	ProviderType    string
	ProviderKey     string
	ProviderSubject string
REDACTED

type CreatePendingAuthSessionInput struct {
	SessionToken             string
	Intent                   string
	Identity                 PendingAuthIdentityKey
	TargetUserID             *int64
	RedirectTo               string
	ResolvedEmail            string
	RegistrationPasswordHash string
	BrowserSessionKey        string
	UpstreamIdentityClaims   map[string]any
	LocalFlowState           map[string]any
	ExpiresAt                time.Time
REDACTED

type IssuePendingAuthCompletionCodeInput struct {
	PendingAuthSessionID int64
	BrowserSessionKey    string
	TTL                  time.Duration
REDACTED

type IssuePendingAuthCompletionCodeResult struct {
	Code      string
	ExpiresAt time.Time
REDACTED

type PendingIdentityAdoptionDecisionInput struct {
	PendingAuthSessionID int64
	IdentityID           *int64
	AdoptDisplayName     bool
	AdoptAvatar          bool
REDACTED

type AuthPendingIdentityService struct {
	entClient *dbent.Client
REDACTED

func NewAuthPendingIdentityService(entClient *dbent.Client) *AuthPendingIdentityService {
	return &AuthPendingIdentityService{entClient: entClientREDACTED
REDACTED

func (s *AuthPendingIdentityService) CreatePendingSession(ctx context.Context, input CreatePendingAuthSessionInput) (*dbent.PendingAuthSession, error) {
	if s == nil || s.entClient == nil {
		return nil, fmt.Errorf("pending auth ent client is not configured")
REDACTED

	sessionToken := strings.TrimSpace(input.SessionToken)
	if sessionToken == "" {
		var err error
		sessionToken, err = randomOpaqueToken(24)
		if err != nil {
			return nil, err
	REDACTED
REDACTED

	expiresAt := input.ExpiresAt.UTC()
	if expiresAt.IsZero() {
		expiresAt = time.Now().UTC().Add(defaultPendingAuthTTL)
REDACTED

	create := s.entClient.PendingAuthSession.Create().
		SetSessionToken(sessionToken).
		SetIntent(strings.TrimSpace(input.Intent)).
		SetProviderType(strings.TrimSpace(input.Identity.ProviderType)).
		SetProviderKey(strings.TrimSpace(input.Identity.ProviderKey)).
		SetProviderSubject(strings.TrimSpace(input.Identity.ProviderSubject)).
		SetRedirectTo(strings.TrimSpace(input.RedirectTo)).
		SetResolvedEmail(strings.TrimSpace(input.ResolvedEmail)).
		SetRegistrationPasswordHash(strings.TrimSpace(input.RegistrationPasswordHash)).
		SetBrowserSessionKey(strings.TrimSpace(input.BrowserSessionKey)).
		SetUpstreamIdentityClaims(copyPendingMap(input.UpstreamIdentityClaims)).
		SetLocalFlowState(copyPendingMap(input.LocalFlowState)).
		SetExpiresAt(expiresAt)
	if input.TargetUserID != nil {
		create = create.SetTargetUserID(*input.TargetUserID)
REDACTED
	return create.Save(ctx)
REDACTED

func (s *AuthPendingIdentityService) IssueCompletionCode(ctx context.Context, input IssuePendingAuthCompletionCodeInput) (*IssuePendingAuthCompletionCodeResult, error) {
	if s == nil || s.entClient == nil {
		return nil, fmt.Errorf("pending auth ent client is not configured")
REDACTED

	session, err := s.entClient.PendingAuthSession.Get(ctx, input.PendingAuthSessionID)
	if err != nil {
		if dbent.IsNotFound(err) {
			return nil, ErrPendingAuthSessionNotFound
	REDACTED
		return nil, err
REDACTED

	code, err := randomOpaqueToken(24)
	if err != nil {
		return nil, err
REDACTED
	ttl := input.TTL
	if ttl <= 0 {
		ttl = defaultPendingAuthCompletionTTL
REDACTED
	expiresAt := time.Now().UTC().Add(ttl)

	update := s.entClient.PendingAuthSession.UpdateOneID(session.ID).
		SetCompletionCodeHash(hashPendingAuthCode(code)).
		SetCompletionCodeExpiresAt(expiresAt)
	if strings.TrimSpace(input.BrowserSessionKey) != "" {
		update = update.SetBrowserSessionKey(strings.TrimSpace(input.BrowserSessionKey))
REDACTED
	if _, err := update.Save(ctx); err != nil {
		return nil, err
REDACTED

	return &IssuePendingAuthCompletionCodeResult{
		Code:      code,
		ExpiresAt: expiresAt,
REDACTED, nil
REDACTED

func (s *AuthPendingIdentityService) ConsumeCompletionCode(ctx context.Context, rawCode, browserSessionKey string) (*dbent.PendingAuthSession, error) {
	if s == nil || s.entClient == nil {
		return nil, fmt.Errorf("pending auth ent client is not configured")
REDACTED

	codeHash := hashPendingAuthCode(strings.TrimSpace(rawCode))
	session, err := s.entClient.PendingAuthSession.Query().
		Where(pendingauthsession.CompletionCodeHashEQ(codeHash)).
		Only(ctx)
	if err != nil {
		if dbent.IsNotFound(err) {
			return nil, ErrPendingAuthCodeInvalid
	REDACTED
		return nil, err
REDACTED

	return s.consumeSession(ctx, session, browserSessionKey, ErrPendingAuthCodeExpired, ErrPendingAuthCodeConsumed)
REDACTED

func (s *AuthPendingIdentityService) ConsumeBrowserSession(ctx context.Context, sessionToken, browserSessionKey string) (*dbent.PendingAuthSession, error) {
	if s == nil || s.entClient == nil {
		return nil, fmt.Errorf("pending auth ent client is not configured")
REDACTED

	session, err := s.getBrowserSession(ctx, sessionToken)
	if err != nil {
		return nil, err
REDACTED

	return s.consumeSession(ctx, session, browserSessionKey, ErrPendingAuthSessionExpired, ErrPendingAuthSessionConsumed)
REDACTED

func (s *AuthPendingIdentityService) GetBrowserSession(ctx context.Context, sessionToken, browserSessionKey string) (*dbent.PendingAuthSession, error) {
	if s == nil || s.entClient == nil {
		return nil, fmt.Errorf("pending auth ent client is not configured")
REDACTED

	session, err := s.getBrowserSession(ctx, sessionToken)
	if err != nil {
		return nil, err
REDACTED
	if err := validatePendingSessionState(session, browserSessionKey, ErrPendingAuthSessionExpired, ErrPendingAuthSessionConsumed); err != nil {
		return nil, err
REDACTED
	return session, nil
REDACTED

func (s *AuthPendingIdentityService) getBrowserSession(ctx context.Context, sessionToken string) (*dbent.PendingAuthSession, error) {
	if s == nil || s.entClient == nil {
		return nil, fmt.Errorf("pending auth ent client is not configured")
REDACTED

	sessionToken = strings.TrimSpace(sessionToken)
	if sessionToken == "" {
		return nil, ErrPendingAuthSessionNotFound
REDACTED

	session, err := s.entClient.PendingAuthSession.Query().
		Where(pendingauthsession.SessionTokenEQ(sessionToken)).
		Only(ctx)
	if err != nil {
		if dbent.IsNotFound(err) {
			return nil, ErrPendingAuthSessionNotFound
	REDACTED
		return nil, err
REDACTED
	return session, nil
REDACTED

func (s *AuthPendingIdentityService) consumeSession(
	ctx context.Context,
	session *dbent.PendingAuthSession,
	browserSessionKey string,
	expiredErr error,
	consumedErr error,
) (*dbent.PendingAuthSession, error) {
	if err := validatePendingSessionState(session, browserSessionKey, expiredErr, consumedErr); err != nil {
		return nil, err
REDACTED

	now := time.Now().UTC()
	updated, err := s.entClient.PendingAuthSession.UpdateOneID(session.ID).
		SetConsumedAt(now).
		SetCompletionCodeHash("").
		ClearCompletionCodeExpiresAt().
		Save(ctx)
	if err != nil {
		return nil, err
REDACTED
	return updated, nil
REDACTED

func validatePendingSessionState(session *dbent.PendingAuthSession, browserSessionKey string, expiredErr error, consumedErr error) error {
	if session == nil {
		return ErrPendingAuthSessionNotFound
REDACTED

	now := time.Now().UTC()
	if session.ConsumedAt != nil {
		return consumedErr
REDACTED
	if !session.ExpiresAt.IsZero() && now.After(session.ExpiresAt) {
		return expiredErr
REDACTED
	if session.CompletionCodeExpiresAt != nil && now.After(*session.CompletionCodeExpiresAt) {
		return expiredErr
REDACTED
	if strings.TrimSpace(session.BrowserSessionKey) != "" && strings.TrimSpace(browserSessionKey) != strings.TrimSpace(session.BrowserSessionKey) {
		return ErrPendingAuthBrowserMismatch
REDACTED
	return nil
REDACTED

func (s *AuthPendingIdentityService) UpsertAdoptionDecision(ctx context.Context, input PendingIdentityAdoptionDecisionInput) (*dbent.IdentityAdoptionDecision, error) {
	if s == nil || s.entClient == nil {
		return nil, fmt.Errorf("pending auth ent client is not configured")
REDACTED

	if input.IdentityID != nil && *input.IdentityID > 0 {
		if _, err := s.entClient.IdentityAdoptionDecision.Update().
			Where(
				identityadoptiondecision.IdentityIDEQ(*input.IdentityID),
				dbpredicate.IdentityAdoptionDecision(func(s *entsql.Selector) {
					col := s.C(identityadoptiondecision.FieldPendingAuthSessionID)
					s.Where(entsql.Or(
						entsql.IsNull(col),
						entsql.NEQ(col, input.PendingAuthSessionID),
					))
			REDACTED),
			).
			ClearIdentityID().
			Save(ctx); err != nil {
			return nil, err
	REDACTED
REDACTED

	existing, err := s.entClient.IdentityAdoptionDecision.Query().
		Where(identityadoptiondecision.PendingAuthSessionIDEQ(input.PendingAuthSessionID)).
		Only(ctx)
	if err != nil && !dbent.IsNotFound(err) {
		return nil, err
REDACTED
	if existing == nil {
		create := s.entClient.IdentityAdoptionDecision.Create().
			SetPendingAuthSessionID(input.PendingAuthSessionID).
			SetAdoptDisplayName(input.AdoptDisplayName).
			SetAdoptAvatar(input.AdoptAvatar).
			SetDecidedAt(time.Now().UTC())
		if input.IdentityID != nil {
			create = create.SetIdentityID(*input.IdentityID)
	REDACTED
		return create.Save(ctx)
REDACTED

	update := s.entClient.IdentityAdoptionDecision.UpdateOneID(existing.ID).
		SetAdoptDisplayName(input.AdoptDisplayName).
		SetAdoptAvatar(input.AdoptAvatar)
	if input.IdentityID != nil {
		update = update.SetIdentityID(*input.IdentityID)
REDACTED
	return update.Save(ctx)
REDACTED

func copyPendingMap(in map[string]any) map[string]any {
	if len(in) == 0 {
		return map[string]any{REDACTED
REDACTED
	out := make(map[string]any, len(in))
	for k, v := range in {
		out[k] = v
REDACTED
	return out
REDACTED

func randomOpaqueToken(byteLen int) (string, error) {
	if byteLen <= 0 {
		byteLen = 16
REDACTED
	buf := make([]byte, byteLen)
	if _, err := rand.Read(buf); err != nil {
		return "", err
REDACTED
	return hex.EncodeToString(buf), nil
REDACTED

func hashPendingAuthCode(code string) string {
	sum := sha256.Sum256([]byte(code))
	return hex.EncodeToString(sum[:])
REDACTED
