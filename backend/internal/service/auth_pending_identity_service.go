package service

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"hash/fnv"
	"sort"
	"strings"
	"sync"
	"time"

	"entgo.io/ent/dialect"
	dbent "github.com/Wei-Shaw/sub2api/ent"
	"github.com/Wei-Shaw/sub2api/ent/identityadoptiondecision"
	"github.com/Wei-Shaw/sub2api/ent/pendingauthsession"
	dbpredicate "github.com/Wei-Shaw/sub2api/ent/predicate"
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

var authPendingIdentityScopedKeyLocks = newAuthPendingIdentityScopedKeyLockRegistry()

type authPendingIdentityScopedKeyLockRegistry struct {
	mu    sync.Mutex
	locks map[string]*authPendingIdentityScopedKeyLockEntry
REDACTED

type authPendingIdentityScopedKeyLockEntry struct {
	mu   sync.Mutex
	refs int
REDACTED

func newAuthPendingIdentityScopedKeyLockRegistry() *authPendingIdentityScopedKeyLockRegistry {
	return &authPendingIdentityScopedKeyLockRegistry{
		locks: make(map[string]*authPendingIdentityScopedKeyLockEntry),
REDACTED
REDACTED

func (r *authPendingIdentityScopedKeyLockRegistry) lock(keys ...string) func() {
	normalized := normalizeAuthPendingIdentityLockKeys(keys...)
	if len(normalized) == 0 {
		return func() {REDACTED
REDACTED

	entries := make([]*authPendingIdentityScopedKeyLockEntry, 0, len(normalized))
	r.mu.Lock()
	for _, key := range normalized {
		entry := r.locks[key]
		if entry == nil {
			entry = &authPendingIdentityScopedKeyLockEntry{REDACTED
			r.locks[key] = entry
	REDACTED
		entry.refs++
		entries = append(entries, entry)
REDACTED
	r.mu.Unlock()

	for _, entry := range entries {
		entry.mu.Lock()
REDACTED

	return func() {
		for i := len(entries) - 1; i >= 0; i-- {
			entries[i].mu.Unlock()
	REDACTED

		r.mu.Lock()
		defer r.mu.Unlock()
		for idx, key := range normalized {
			entry := entries[idx]
			entry.refs--
			if entry.refs == 0 {
				delete(r.locks, key)
		REDACTED
	REDACTED
REDACTED
REDACTED

func normalizeAuthPendingIdentityLockKeys(keys ...string) []string {
	if len(keys) == 0 {
		return nil
REDACTED

	deduped := make(map[string]struct{REDACTED, len(keys))
	for _, key := range keys {
		trimmed := strings.TrimSpace(key)
		if trimmed == "" {
			continue
	REDACTED
		deduped[trimmed] = struct{REDACTED{REDACTED
REDACTED
	if len(deduped) == 0 {
		return nil
REDACTED

	normalized := make([]string, 0, len(deduped))
	for key := range deduped {
		normalized = append(normalized, key)
REDACTED
	sort.Strings(normalized)
	return normalized
REDACTED

func authPendingIdentityAdvisoryLockHash(key string) int64 {
	hasher := fnv.New64a()
	_, _ = hasher.Write([]byte(key))
	return int64(hasher.Sum64())
REDACTED

func lockAuthPendingIdentityKeys(ctx context.Context, client *dbent.Client, keys ...string) (func(), error) {
	release := authPendingIdentityScopedKeyLocks.lock(keys...)
	normalized := normalizeAuthPendingIdentityLockKeys(keys...)
	if len(normalized) == 0 || client == nil || client.Driver().Dialect() != dialect.Postgres {
		return release, nil
REDACTED

	for _, key := range normalized {
		var rows entsql.Rows
		if err := client.Driver().Query(ctx, "SELECT pg_advisory_xact_lock($1)", []any{authPendingIdentityAdvisoryLockHash(key)REDACTED, &rows); err != nil {
			release()
			return nil, err
	REDACTED
		_ = rows.Close()
REDACTED

	return release, nil
REDACTED

func pendingIdentityAdoptionLockKeys(pendingAuthSessionID int64, identityID *int64) []string {
	keys := []string{fmt.Sprintf("pending-auth-adoption:pending:%d", pendingAuthSessionID)REDACTED
	if identityID != nil && *identityID > 0 {
		keys = append(keys, fmt.Sprintf("pending-auth-adoption:identity:%d", *identityID))
REDACTED
	return keys
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

	sanitizedLocalFlowState := sanitizePendingAuthLocalFlowState(session.LocalFlowState)
	now := time.Now().UTC()
	update := s.entClient.PendingAuthSession.UpdateOneID(session.ID).
		Where(
			pendingauthsession.ConsumedAtIsNil(),
			pendingauthsession.ExpiresAtGTE(now),
			pendingauthsession.Or(
				pendingauthsession.CompletionCodeExpiresAtIsNil(),
				pendingauthsession.CompletionCodeExpiresAtGTE(now),
			),
		).
		SetConsumedAt(now).
		SetLocalFlowState(sanitizedLocalFlowState).
		SetCompletionCodeHash("").
		ClearCompletionCodeExpiresAt()
	if expectedBrowserSessionKey := strings.TrimSpace(session.BrowserSessionKey); expectedBrowserSessionKey != "" {
		update = update.Where(pendingauthsession.BrowserSessionKeyEQ(expectedBrowserSessionKey))
REDACTED
	updated, err := update.Save(ctx)
	if err == nil {
		return updated, nil
REDACTED
	if !dbent.IsNotFound(err) {
		return nil, err
REDACTED

	current, currentErr := s.entClient.PendingAuthSession.Get(ctx, session.ID)
	if currentErr != nil {
		if dbent.IsNotFound(currentErr) {
			return nil, ErrPendingAuthSessionNotFound
	REDACTED
		return nil, currentErr
REDACTED
	if err := validatePendingSessionState(current, browserSessionKey, expiredErr, consumedErr); err != nil {
		return nil, err
REDACTED
	return nil, consumedErr
REDACTED

func sanitizePendingAuthLocalFlowState(localFlowState map[string]any) map[string]any {
	sanitized := copyPendingMap(localFlowState)
	if len(sanitized) == 0 {
		return sanitized
REDACTED

	rawCompletion, ok := sanitized["completion_response"]
	if !ok {
		return sanitized
REDACTED
	completion, ok := rawCompletion.(map[string]any)
	if !ok {
		return sanitized
REDACTED

	cleanedCompletion := copyPendingMap(completion)
	for _, key := range []string{"access_token", "refresh_token", "expires_in", "token_type"REDACTED {
		delete(cleanedCompletion, key)
REDACTED
	sanitized["completion_response"] = cleanedCompletion
	return sanitized
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

	tx, err := s.entClient.Tx(ctx)
	if err != nil && !errors.Is(err, dbent.ErrTxStarted) {
		return nil, err
REDACTED

	client := s.entClient
	txCtx := ctx
	if err == nil {
		defer func() { _ = tx.Rollback() REDACTED()
		client = tx.Client()
		txCtx = dbent.NewTxContext(ctx, tx)
REDACTED else if existingTx := dbent.TxFromContext(ctx); existingTx != nil {
		client = existingTx.Client()
REDACTED

	releaseLocks, err := lockAuthPendingIdentityKeys(txCtx, client, pendingIdentityAdoptionLockKeys(input.PendingAuthSessionID, input.IdentityID)...)
	if err != nil {
		return nil, err
REDACTED
	defer releaseLocks()

	if input.IdentityID != nil && *input.IdentityID > 0 {
		if _, err := client.IdentityAdoptionDecision.Update().
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
			Save(txCtx); err != nil {
			return nil, err
	REDACTED
REDACTED

	create := client.IdentityAdoptionDecision.Create().
		SetPendingAuthSessionID(input.PendingAuthSessionID).
		SetAdoptDisplayName(input.AdoptDisplayName).
		SetAdoptAvatar(input.AdoptAvatar).
		SetDecidedAt(time.Now().UTC())
	if input.IdentityID != nil && *input.IdentityID > 0 {
		create = create.SetIdentityID(*input.IdentityID)
REDACTED

	decisionID, err := create.
		OnConflictColumns(identityadoptiondecision.FieldPendingAuthSessionID).
		UpdateNewValues().
		ID(txCtx)
	if err != nil {
		return nil, err
REDACTED

	decision, err := client.IdentityAdoptionDecision.Get(txCtx, decisionID)
	if err != nil {
		return nil, err
REDACTED

	if tx != nil {
		if err := tx.Commit(); err != nil {
			return nil, err
	REDACTED
REDACTED

	return decision, nil
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
