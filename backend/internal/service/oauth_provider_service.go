package service

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"database/sql"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"strings"
	"time"
)

const (
	oauthTransactionTTL = 10 * time.Minute
	oauthCodeTTL        = 60 * time.Second
	oauthAccessTTL      = 15 * time.Minute
	oauthRefreshTTL     = 30 * 24 * time.Hour
)

var (
	ErrOAuthInvalidClient       = errors.New("oauth invalid client")
	ErrOAuthInvalidGrant        = errors.New("oauth invalid grant")
	ErrOAuthInvalidRequest      = errors.New("oauth invalid request")
	ErrOAuthInvalidScope        = errors.New("oauth invalid scope")
	ErrOAuthRedirectMismatch    = errors.New("oauth redirect uri mismatch")
	ErrOAuthAuthorizationDenied = errors.New("oauth authorization denied")
	ErrOAuthTokenRevoked        = errors.New("oauth token revoked")
	// ErrOAuthInvalidClientInput marks admin client-management validation
	// failures; handlers surface these messages as 400 responses.
	ErrOAuthInvalidClientInput = errors.New("oauth invalid client input")
)

var oauthScopes = map[string]struct{}{
	"openid": {}, "profile": {}, "groups:read": {}, "keys:read": {}, "keys:create": {}, "keys:revoke": {},
}

type OAuthClient struct {
	ClientID      string   `json:"client_id"`
	ClientType    string   `json:"client_type"`
	Name          string   `json:"name"`
	Description   string   `json:"description"`
	LogoURL       string   `json:"logo_url"`
	RedirectURIs  []string `json:"redirect_uris"`
	AllowedScopes []string `json:"allowed_scopes"`
	PKCERequired  bool     `json:"pkce_required"`
	Status        string   `json:"status"`
}

type OAuthAuthorizationTransaction struct {
	ID                string   `json:"id"`
	ClientID          string   `json:"client_id"`
	ClientName        string   `json:"client_name"`
	ClientDescription string   `json:"client_description"`
	ClientLogoURL     string   `json:"client_logo_url"`
	RedirectURI       string   `json:"redirect_uri"`
	Scope             []string `json:"scope"`
	// State stays out of the JSON projection (the consent page never sees
	// it; the client already holds it), but the PKCE challenge is persisted
	// in the Redis record — dropping it would void every token exchange.
	State               string    `json:"-"`
	CodeChallenge       string    `json:"code_challenge"`
	CodeChallengeMethod string    `json:"code_challenge_method"`
	ExpiresAt           time.Time `json:"expires_at"`
}

type OAuthTokenInfo struct {
	TokenID  string
	UserID   int64
	ClientID string
	Scopes   map[string]struct{}
}

type OAuthTokenResponse struct {
	AccessToken  string `json:"access_token"`
	TokenType    string `json:"token_type"`
	ExpiresIn    int    `json:"expires_in"`
	RefreshToken string `json:"refresh_token,omitempty"`
	Scope        string `json:"scope"`
	// Issuer echo (RFC 9207), filled by the handler.
	Iss string `json:"iss,omitempty"`
}

// OAuthStoredAuthorizationCode 是已签发授权码的持久化形态；其线格式由存储层负责。
type OAuthStoredAuthorizationCode struct {
	CodeHash            string `json:"code_hash"`
	ClientID            string `json:"client_id"`
	UserID              int64  `json:"user_id"`
	RedirectURI         string `json:"redirect_uri"`
	Scope               string `json:"scope"`
	CodeChallenge       string `json:"code_challenge"`
	CodeChallengeMethod string `json:"code_challenge_method"`
}

// OAuthStoredToken 是已签发 access/refresh 令牌的持久化形态。
type OAuthStoredToken struct {
	TokenHash string `json:"token_hash"`
	FamilyID  string `json:"family_id"`
	ClientID  string `json:"client_id"`
	UserID    int64  `json:"user_id"`
	Scope     string `json:"scope"`
}

// OAuthProviderStore 抽象 OAuth Provider 的 redis 状态（授权事务、授权码、令牌），
// 由 repository 层实现；service 层不得直接触碰 redis。
type OAuthProviderStore interface {
	SaveTransaction(ctx context.Context, id string, tx OAuthAuthorizationTransaction, ttl time.Duration) error
	GetTransaction(ctx context.Context, id string) (OAuthAuthorizationTransaction, error)
	ConsumeTransaction(ctx context.Context, id string) (OAuthAuthorizationTransaction, error)
	SaveAuthorizationCode(ctx context.Context, code OAuthStoredAuthorizationCode, ttl time.Duration) error
	ConsumeAuthorizationCode(ctx context.Context, codeHash string) (OAuthStoredAuthorizationCode, error)
	ConsumeRefreshToken(ctx context.Context, tokenHash string) (OAuthStoredToken, error)
	SaveTokenPair(ctx context.Context, accessHash string, access OAuthStoredToken, accessTTL time.Duration, refreshHash string, refresh OAuthStoredToken, refreshTTL time.Duration) error
	GetAccessToken(ctx context.Context, tokenHash string) (OAuthStoredToken, error)
	DeleteTokens(ctx context.Context, accessHash, refreshHash string) error
	RevokeUserTokens(ctx context.Context, clientID string, userID int64) error
}

type OAuthProviderService struct {
	db    *sql.DB
	store OAuthProviderStore
}

func NewOAuthProviderService(db *sql.DB, store OAuthProviderStore) *OAuthProviderService {
	return &OAuthProviderService{db: db, store: store}
}

func (s *OAuthProviderService) client(ctx context.Context, clientID string) (OAuthClient, error) {
	if s == nil || s.db == nil {
		return OAuthClient{}, ErrOAuthInvalidClient
	}
	var client OAuthClient
	var redirectJSON, scopesJSON []byte
	var pkceRequired bool
	err := s.db.QueryRowContext(ctx, `
		SELECT client_id, client_type, name, description, logo_url, redirect_uris,
		       allowed_scopes, pkce_required, status
		FROM oauth_clients WHERE client_id = $1`, strings.TrimSpace(clientID)).Scan(
		&client.ClientID, &client.ClientType, &client.Name, &client.Description, &client.LogoURL,
		&redirectJSON, &scopesJSON, &pkceRequired, &client.Status,
	)
	if err != nil || client.Status != "active" {
		return OAuthClient{}, ErrOAuthInvalidClient
	}
	if err := json.Unmarshal(redirectJSON, &client.RedirectURIs); err != nil {
		return OAuthClient{}, ErrOAuthInvalidClient
	}
	if err := json.Unmarshal(scopesJSON, &client.AllowedScopes); err != nil {
		return OAuthClient{}, ErrOAuthInvalidClient
	}
	client.PKCERequired = pkceRequired
	return client, nil
}

func (s *OAuthProviderService) GetClient(ctx context.Context, clientID string) (OAuthClient, error) {
	return s.client(ctx, clientID)
}

// ==================== Admin client management ====================
// The provider currently only supports public clients (OAuth 2.1 + PKCE).
// The confidential type stays reserved until the token endpoint implements
// client-secret authentication; upsert therefore pins client_type='public'
// and pkce_required=TRUE.

// OAuthClientInput carries the admin-supplied fields for create/update.
type OAuthClientInput struct {
	Name          string
	Description   string
	LogoURL       string
	RedirectURIs  []string
	AllowedScopes []string
	Status        string
}

// OAuthClientListItem extends OAuthClient with audit timestamps for the admin UI.
type OAuthClientListItem struct {
	OAuthClient
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// ListClients returns every registered client, including disabled ones.
func (s *OAuthProviderService) ListClients(ctx context.Context) ([]OAuthClientListItem, error) {
	if s == nil || s.db == nil {
		return nil, errors.New("oauth database unavailable")
	}
	rows, err := s.db.QueryContext(ctx, `
		SELECT client_id, client_type, name, description, logo_url, redirect_uris,
		       allowed_scopes, pkce_required, status, created_at, updated_at
		FROM oauth_clients ORDER BY created_at`)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	out := make([]OAuthClientListItem, 0)
	for rows.Next() {
		var item OAuthClientListItem
		var redirectJSON, scopesJSON []byte
		var pkceRequired bool
		if err := rows.Scan(&item.ClientID, &item.ClientType, &item.Name, &item.Description, &item.LogoURL,
			&redirectJSON, &scopesJSON, &pkceRequired, &item.Status, &item.CreatedAt, &item.UpdatedAt); err != nil {
			return nil, err
		}
		if err := json.Unmarshal(redirectJSON, &item.RedirectURIs); err != nil {
			return nil, err
		}
		if err := json.Unmarshal(scopesJSON, &item.AllowedScopes); err != nil {
			return nil, err
		}
		item.PKCERequired = pkceRequired
		out = append(out, item)
	}
	return out, rows.Err()
}

// UpsertClient creates or updates one client. Validation failures return an
// error whose message is safe to surface to the admin UI.
func (s *OAuthProviderService) UpsertClient(ctx context.Context, clientID string, input OAuthClientInput) (OAuthClientListItem, error) {
	if s == nil || s.db == nil {
		return OAuthClientListItem{}, errors.New("oauth database unavailable")
	}
	client, err := normalizeOAuthClientInput(clientID, input)
	if err != nil {
		return OAuthClientListItem{}, err
	}
	var item OAuthClientListItem
	err = s.db.QueryRowContext(ctx, `
		INSERT INTO oauth_clients (client_id, client_type, name, description, logo_url, redirect_uris, allowed_scopes, pkce_required, status)
		VALUES ($1, 'public', $2, $3, $4, $5::jsonb, $6::jsonb, TRUE, $7)
		ON CONFLICT (client_id) DO UPDATE SET
			name = EXCLUDED.name,
			description = EXCLUDED.description,
			logo_url = EXCLUDED.logo_url,
			redirect_uris = EXCLUDED.redirect_uris,
			allowed_scopes = EXCLUDED.allowed_scopes,
			pkce_required = TRUE,
			status = EXCLUDED.status,
			updated_at = NOW()
		RETURNING created_at, updated_at`,
		client.ClientID, client.Name, client.Description, client.LogoURL,
		stringListJSON(client.RedirectURIs), stringListJSON(client.AllowedScopes), client.Status,
	).Scan(&item.CreatedAt, &item.UpdatedAt)
	if err != nil {
		return OAuthClientListItem{}, err
	}
	item.OAuthClient = client
	return item, nil
}

// DeleteClient removes one client; its grants (but not API keys) cascade away.
func (s *OAuthProviderService) DeleteClient(ctx context.Context, clientID string) error {
	if s == nil || s.db == nil {
		return errors.New("oauth database unavailable")
	}
	res, err := s.db.ExecContext(ctx, `DELETE FROM oauth_clients WHERE client_id = $1`, strings.TrimSpace(clientID))
	if err != nil {
		return err
	}
	if n, rowsErr := res.RowsAffected(); rowsErr == nil && n == 0 {
		return ErrOAuthInvalidClient
	}
	return nil
}

// normalizeOAuthClientInput validates and normalizes admin input. PKCE stays
// mandatory and the client type stays public.
func normalizeOAuthClientInput(clientID string, input OAuthClientInput) (OAuthClient, error) {
	clientID = strings.TrimSpace(clientID)
	if len(clientID) < 3 || len(clientID) > 160 {
		return OAuthClient{}, fmt.Errorf("%w: client_id must be 3-160 characters", ErrOAuthInvalidClientInput)
	}
	for _, r := range clientID {
		if (r < 'a' || r > 'z') && (r < 'A' || r > 'Z') && (r < '0' || r > '9') && r != '.' && r != '_' && r != '-' {
			return OAuthClient{}, fmt.Errorf("%w: client_id only allows letters, digits, dot, underscore and dash", ErrOAuthInvalidClientInput)
		}
	}
	name := strings.TrimSpace(input.Name)
	if name == "" || len(name) > 160 {
		return OAuthClient{}, fmt.Errorf("%w: name is required and must be at most 160 characters", ErrOAuthInvalidClientInput)
	}
	status := strings.TrimSpace(input.Status)
	if status == "" {
		status = "active"
	}
	if status != "active" && status != "disabled" {
		return OAuthClient{}, fmt.Errorf("%w: status must be active or disabled", ErrOAuthInvalidClientInput)
	}
	uris := make([]string, 0, len(input.RedirectURIs))
	seenURIs := make(map[string]struct{}, len(input.RedirectURIs))
	for _, raw := range input.RedirectURIs {
		uri := strings.TrimSpace(raw)
		if uri == "" {
			continue
		}
		parsed, parseErr := url.Parse(uri)
		if parseErr != nil || (parsed.Scheme != "http" && parsed.Scheme != "https") || parsed.Fragment != "" {
			return OAuthClient{}, fmt.Errorf("%w: invalid redirect_uri: %s", ErrOAuthInvalidClientInput, uri)
		}
		if _, dup := seenURIs[uri]; dup {
			continue
		}
		seenURIs[uri] = struct{}{}
		uris = append(uris, uri)
	}
	if len(uris) == 0 {
		return OAuthClient{}, fmt.Errorf("%w: at least one redirect_uri is required", ErrOAuthInvalidClientInput)
	}
	scopeList := make([]string, 0, len(input.AllowedScopes))
	seenScopes := make(map[string]struct{}, len(input.AllowedScopes))
	for _, scope := range input.AllowedScopes {
		scope = strings.TrimSpace(scope)
		if scope == "" {
			continue
		}
		if _, known := oauthScopes[scope]; !known {
			return OAuthClient{}, fmt.Errorf("%w: unknown scope: %s", ErrOAuthInvalidClientInput, scope)
		}
		if _, dup := seenScopes[scope]; dup {
			continue
		}
		seenScopes[scope] = struct{}{}
		scopeList = append(scopeList, scope)
	}
	if len(scopeList) == 0 {
		return OAuthClient{}, fmt.Errorf("%w: at least one allowed scope is required", ErrOAuthInvalidClientInput)
	}
	return OAuthClient{
		ClientID:      clientID,
		ClientType:    "public",
		Name:          name,
		Description:   strings.TrimSpace(input.Description),
		LogoURL:       strings.TrimSpace(input.LogoURL),
		RedirectURIs:  uris,
		AllowedScopes: scopeList,
		PKCERequired:  true,
		Status:        status,
	}, nil
}

func (s *OAuthProviderService) ValidateAuthorizationRequest(ctx context.Context, clientID, redirectURI string, scopes []string, challenge, method string) (OAuthClient, error) {
	client, err := s.client(ctx, clientID)
	if err != nil {
		return OAuthClient{}, err
	}
	if !validRedirectURI(client, redirectURI) {
		return OAuthClient{}, ErrOAuthRedirectMismatch
	}
	if client.PKCERequired && (strings.TrimSpace(challenge) == "" || !strings.EqualFold(strings.TrimSpace(method), "S256")) {
		return OAuthClient{}, ErrOAuthInvalidRequest
	}
	allowed := make(map[string]struct{}, len(client.AllowedScopes))
	for _, scope := range client.AllowedScopes {
		allowed[strings.TrimSpace(scope)] = struct{}{}
	}
	for _, scope := range scopes {
		scope = strings.TrimSpace(scope)
		if _, known := oauthScopes[scope]; !known {
			return OAuthClient{}, ErrOAuthInvalidScope
		}
		if _, ok := allowed[scope]; !ok {
			return OAuthClient{}, ErrOAuthInvalidScope
		}
	}
	return client, nil
}

// validRedirectURI enforces RFC 8252 §7.3 semantics for public clients:
// loopback IP literals (127.0.0.1 / [::1]) may vary the port per attempt, but
// the path is pinned to the paths the operator registered for this client.
// Everything else must match a registered URI byte-for-byte.
func validRedirectURI(client OAuthClient, raw string) bool {
	raw = strings.TrimSpace(raw)
	u, err := url.Parse(raw)
	if err != nil || u.Scheme == "" || u.Host == "" || u.Path == "" || u.Fragment != "" || u.User != nil {
		return false
	}
	registeredPaths := make(map[string]struct{}, len(client.RedirectURIs))
	for _, registered := range client.RedirectURIs {
		registered = strings.TrimSpace(registered)
		if registered == raw {
			return true
		}
		ru, rerr := url.Parse(registered)
		if rerr != nil || ru.Path == "" {
			continue
		}
		host := strings.ToLower(ru.Hostname())
		if (ru.Scheme == "http" || ru.Scheme == "https") && (host == "127.0.0.1" || host == "::1") {
			registeredPaths[ru.Path] = struct{}{}
		}
	}
	if client.ClientType != "public" || u.Scheme != "http" || u.Port() == "" {
		return false
	}
	host := strings.ToLower(u.Hostname())
	if host != "127.0.0.1" && host != "::1" {
		return false
	}
	_, allowed := registeredPaths[u.Path]
	return allowed
}

func (s *OAuthProviderService) CreateAuthorizationTransaction(ctx context.Context, tx OAuthAuthorizationTransaction) (OAuthAuthorizationTransaction, error) {
	if s == nil || s.store == nil {
		return OAuthAuthorizationTransaction{}, errors.New("oauth transaction store unavailable")
	}
	id, err := randomOAuthToken(24)
	if err != nil {
		return OAuthAuthorizationTransaction{}, err
	}
	tx.ID = id
	tx.ExpiresAt = time.Now().Add(oauthTransactionTTL)
	if err := s.store.SaveTransaction(ctx, id, tx, oauthTransactionTTL); err != nil {
		return OAuthAuthorizationTransaction{}, err
	}
	return tx, nil
}

func (s *OAuthProviderService) GetAuthorizationTransaction(ctx context.Context, id string) (OAuthAuthorizationTransaction, error) {
	if s == nil || s.store == nil {
		return OAuthAuthorizationTransaction{}, errors.New("oauth transaction store unavailable")
	}
	tx, err := s.store.GetTransaction(ctx, strings.TrimSpace(id))
	if err != nil {
		return OAuthAuthorizationTransaction{}, ErrOAuthInvalidGrant
	}
	if time.Now().After(tx.ExpiresAt) {
		return OAuthAuthorizationTransaction{}, ErrOAuthInvalidGrant
	}
	return tx, nil
}

func (s *OAuthProviderService) consumeAuthorizationTransaction(ctx context.Context, id string) (OAuthAuthorizationTransaction, error) {
	if s == nil || s.store == nil {
		return OAuthAuthorizationTransaction{}, errors.New("oauth transaction store unavailable")
	}
	tx, err := s.store.ConsumeTransaction(ctx, strings.TrimSpace(id))
	if err != nil {
		return OAuthAuthorizationTransaction{}, ErrOAuthInvalidGrant
	}
	if time.Now().After(tx.ExpiresAt) {
		return OAuthAuthorizationTransaction{}, ErrOAuthInvalidGrant
	}
	return tx, nil
}

func (s *OAuthProviderService) IssueAuthorizationCode(ctx context.Context, transactionID string, userID int64) (string, OAuthAuthorizationTransaction, error) {
	tx, err := s.consumeAuthorizationTransaction(ctx, transactionID)
	if err != nil {
		return "", OAuthAuthorizationTransaction{}, err
	}
	code, err := randomOAuthToken(32)
	if err != nil {
		return "", OAuthAuthorizationTransaction{}, err
	}
	if s.store == nil {
		return "", OAuthAuthorizationTransaction{}, errors.New("oauth token store unavailable")
	}
	if err := s.store.SaveAuthorizationCode(ctx, OAuthStoredAuthorizationCode{
		CodeHash: hashOAuthToken(code), ClientID: tx.ClientID, UserID: userID,
		RedirectURI: tx.RedirectURI, Scope: strings.Join(tx.Scope, " "),
		CodeChallenge: tx.CodeChallenge, CodeChallengeMethod: tx.CodeChallengeMethod,
	}, oauthCodeTTL); err != nil {
		return "", OAuthAuthorizationTransaction{}, err
	}
	return code, tx, nil
}

func (s *OAuthProviderService) ExchangeAuthorizationCode(ctx context.Context, clientID, code, redirectURI, verifier string) (OAuthTokenResponse, error) {
	if s == nil || s.store == nil || s.db == nil {
		return OAuthTokenResponse{}, ErrOAuthInvalidGrant
	}
	codeHash := hashOAuthToken(code)
	stored, err := s.store.ConsumeAuthorizationCode(ctx, codeHash)
	if err != nil {
		return OAuthTokenResponse{}, ErrOAuthInvalidGrant
	}
	if stored.ClientID != clientID || stored.RedirectURI != redirectURI {
		return OAuthTokenResponse{}, ErrOAuthInvalidGrant
	}
	if !verifyPKCE(verifier, stored.CodeChallenge, stored.CodeChallengeMethod) {
		return OAuthTokenResponse{}, ErrOAuthInvalidGrant
	}
	accessToken, err := randomOAuthToken(32)
	if err != nil {
		return OAuthTokenResponse{}, err
	}
	refreshToken, err := randomOAuthToken(48)
	if err != nil {
		return OAuthTokenResponse{}, err
	}
	familyID, err := randomOAuthToken(24)
	if err != nil {
		return OAuthTokenResponse{}, err
	}
	scope := stored.Scope
	if err := s.storeIssuedTokens(ctx, clientID, stored.UserID, scope, accessToken, refreshToken, familyID); err != nil {
		return OAuthTokenResponse{}, err
	}
	if _, err := s.db.ExecContext(ctx, `INSERT INTO oauth_grants (client_id, user_id, scopes) VALUES ($1,$2,$3::jsonb) ON CONFLICT (client_id, user_id) DO UPDATE SET scopes = EXCLUDED.scopes, approved_at = NOW(), revoked_at = NULL`, clientID, stored.UserID, scopesJSON(scope)); err != nil {
		return OAuthTokenResponse{}, err
	}
	return OAuthTokenResponse{AccessToken: accessToken, RefreshToken: refreshToken, TokenType: "Bearer", ExpiresIn: int(oauthAccessTTL.Seconds()), Scope: scope}, nil
}

func (s *OAuthProviderService) RefreshAccessToken(ctx context.Context, clientID, refreshToken string) (OAuthTokenResponse, error) {
	if s == nil || s.store == nil {
		return OAuthTokenResponse{}, ErrOAuthInvalidGrant
	}
	refreshHash := hashOAuthToken(refreshToken)
	stored, err := s.store.ConsumeRefreshToken(ctx, refreshHash)
	if err != nil {
		return OAuthTokenResponse{}, ErrOAuthInvalidGrant
	}
	if stored.ClientID != clientID {
		return OAuthTokenResponse{}, ErrOAuthInvalidGrant
	}
	newAccess, err := randomOAuthToken(32)
	if err != nil {
		return OAuthTokenResponse{}, err
	}
	newRefresh, err := randomOAuthToken(48)
	if err != nil {
		return OAuthTokenResponse{}, err
	}
	if err := s.storeIssuedTokens(ctx, clientID, stored.UserID, stored.Scope, newAccess, newRefresh, stored.FamilyID); err != nil {
		return OAuthTokenResponse{}, err
	}
	return OAuthTokenResponse{AccessToken: newAccess, RefreshToken: newRefresh, TokenType: "Bearer", ExpiresIn: int(oauthAccessTTL.Seconds()), Scope: stored.Scope}, nil
}

func (s *OAuthProviderService) storeIssuedTokens(ctx context.Context, clientID string, userID int64, scope, accessToken, refreshToken, familyID string) error {
	return s.store.SaveTokenPair(ctx,
		hashOAuthToken(accessToken),
		OAuthStoredToken{TokenHash: hashOAuthToken(accessToken), ClientID: clientID, UserID: userID, Scope: scope},
		oauthAccessTTL,
		hashOAuthToken(refreshToken),
		OAuthStoredToken{TokenHash: hashOAuthToken(refreshToken), FamilyID: familyID, ClientID: clientID, UserID: userID, Scope: scope},
		oauthRefreshTTL,
	)
}

func (s *OAuthProviderService) ValidateAccessToken(ctx context.Context, raw string) (OAuthTokenInfo, error) {
	if s == nil || s.store == nil || strings.TrimSpace(raw) == "" {
		return OAuthTokenInfo{}, ErrOAuthTokenRevoked
	}
	stored, err := s.store.GetAccessToken(ctx, hashOAuthToken(raw))
	if err != nil {
		return OAuthTokenInfo{}, ErrOAuthTokenRevoked
	}
	return OAuthTokenInfo{TokenID: stored.TokenHash, UserID: stored.UserID, ClientID: stored.ClientID, Scopes: scopeSet(stored.Scope)}, nil
}

func (s *OAuthProviderService) RevokeToken(ctx context.Context, raw string) error {
	if s == nil || s.store == nil || strings.TrimSpace(raw) == "" {
		return nil
	}
	hash := hashOAuthToken(raw)
	return s.store.DeleteTokens(ctx, hash, hash)
}

func (s *OAuthProviderService) GrantStatus(ctx context.Context, userID int64, clientID string) (bool, []string, error) {
	var raw []byte
	var revoked sql.NullTime
	err := s.db.QueryRowContext(ctx, `SELECT scopes, revoked_at FROM oauth_grants WHERE user_id = $1 AND client_id = $2`, userID, clientID).Scan(&raw, &revoked)
	if errors.Is(err, sql.ErrNoRows) {
		return false, nil, nil
	}
	if err != nil {
		return false, nil, err
	}
	var scopes []string
	if err := json.Unmarshal(raw, &scopes); err != nil {
		return false, nil, err
	}
	return !revoked.Valid, scopes, nil
}

// RevokeGrant revokes the user's authorization for one client and cascade-
// revokes every live token issued under it, so "断开连接" takes effect
// immediately instead of waiting for the access token to age out.
func (s *OAuthProviderService) RevokeGrant(ctx context.Context, userID int64, clientID string) error {
	if _, err := s.db.ExecContext(ctx, `UPDATE oauth_grants SET revoked_at = COALESCE(revoked_at, NOW()) WHERE user_id = $1 AND client_id = $2`, userID, clientID); err != nil {
		return err
	}
	if s.store == nil {
		return nil
	}
	return s.store.RevokeUserTokens(ctx, clientID, userID)
}

// OAuthGrantView is one connected-app row the settings page renders.
type OAuthGrantView struct {
	ClientID      string    `json:"client_id"`
	ClientName    string    `json:"client_name"`
	ClientLogoURL string    `json:"client_logo_url,omitempty"`
	Scopes        []string  `json:"scopes"`
	ApprovedAt    time.Time `json:"approved_at"`
}

// ListGrants returns the user's non-revoked authorizations with client display
// information, newest approval first.
func (s *OAuthProviderService) ListGrants(ctx context.Context, userID int64) ([]OAuthGrantView, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT g.client_id, COALESCE(NULLIF(c.name, ''), g.client_id), COALESCE(c.logo_url, ''), g.scopes, g.approved_at
		FROM oauth_grants g
		LEFT JOIN oauth_clients c ON c.client_id = g.client_id
		WHERE g.user_id = $1 AND g.revoked_at IS NULL
		ORDER BY g.approved_at DESC`, userID)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	grants := make([]OAuthGrantView, 0)
	for rows.Next() {
		var view OAuthGrantView
		var raw []byte
		if err := rows.Scan(&view.ClientID, &view.ClientName, &view.ClientLogoURL, &raw, &view.ApprovedAt); err != nil {
			return nil, err
		}
		if err := json.Unmarshal(raw, &view.Scopes); err != nil {
			view.Scopes = nil
		}
		grants = append(grants, view)
	}
	return grants, rows.Err()
}

// APIKeyOwnedByClient reports whether one key belongs to the user. The client
// argument is retained for API compatibility; the initial release does not
// distinguish panel-created and OAuth-created keys.
func (s *OAuthProviderService) APIKeyOwnedByClient(ctx context.Context, keyID, userID int64, clientID string) (bool, error) {
	if s == nil || s.db == nil {
		return false, errors.New("oauth database unavailable")
	}
	var exists bool
	err := s.db.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM api_keys WHERE id = $1 AND user_id = $2 AND deleted_at IS NULL)`, keyID, userID).Scan(&exists)
	return exists, err
}

// ListAPIKeyIDs lists all non-deleted keys belonging to the user. The client
// argument is retained for API compatibility; the initial release does not
// apply client-source filtering.
func (s *OAuthProviderService) ListAPIKeyIDs(ctx context.Context, userID int64, clientID, search, status string, groupID *int64, limit, offset int) ([]int64, int64, error) {
	if s == nil || s.db == nil {
		return nil, 0, errors.New("oauth database unavailable")
	}
	where := []string{"user_id = $1", "deleted_at IS NULL"}
	args := []any{userID}
	arg := 2
	if strings.TrimSpace(search) != "" {
		where = append(where, fmt.Sprintf("name ILIKE $%d", arg))
		args = append(args, "%"+strings.TrimSpace(search)+"%")
		arg++
	}
	if strings.TrimSpace(status) != "" {
		where = append(where, fmt.Sprintf("status = $%d", arg))
		args = append(args, strings.TrimSpace(status))
		arg++
	}
	if groupID != nil {
		where = append(where, fmt.Sprintf("group_id = $%d", arg))
		args = append(args, *groupID)
		arg++
	}
	clause := strings.Join(where, " AND ")
	var total int64
	if err := s.db.QueryRowContext(ctx, "SELECT COUNT(*) FROM api_keys WHERE "+clause, args...).Scan(&total); err != nil {
		return nil, 0, err
	}
	args = append(args, limit, offset)
	rows, err := s.db.QueryContext(ctx, fmt.Sprintf("SELECT id FROM api_keys WHERE %s ORDER BY created_at DESC LIMIT $%d OFFSET $%d", clause, arg, arg+1), args...)
	if err != nil {
		return nil, 0, err
	}
	defer func() { _ = rows.Close() }()
	ids := make([]int64, 0)
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			return nil, 0, err
		}
		ids = append(ids, id)
	}
	return ids, total, rows.Err()
}

func randomOAuthToken(bytes int) (string, error) {
	buf := make([]byte, bytes)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(buf), nil
}

func hashOAuthToken(raw string) string {
	sum := sha256.Sum256([]byte(raw))
	return hex.EncodeToString(sum[:])
}

func verifyPKCE(verifier, challenge, method string) bool {
	if !strings.EqualFold(strings.TrimSpace(method), "S256") || verifier == "" || challenge == "" {
		return false
	}
	sum := sha256.Sum256([]byte(verifier))
	return base64.RawURLEncoding.EncodeToString(sum[:]) == challenge
}

func scopesJSON(scope string) string {
	parts := strings.Fields(scope)
	encoded, _ := json.Marshal(parts)
	return string(encoded)
}

// stringListJSON marshals a string slice into a JSON array for JSONB columns.
func stringListJSON(items []string) string {
	if items == nil {
		items = []string{}
	}
	encoded, _ := json.Marshal(items)
	return string(encoded)
}

func scopeSet(scope string) map[string]struct{} {
	result := make(map[string]struct{})
	for _, item := range strings.Fields(scope) {
		result[item] = struct{}{}
	}
	return result
}

func scopeString(scopes []string) string {
	clean := make([]string, 0, len(scopes))
	seen := make(map[string]struct{}, len(scopes))
	for _, scope := range scopes {
		scope = strings.TrimSpace(scope)
		if scope == "" {
			continue
		}
		if _, ok := seen[scope]; ok {
			continue
		}
		seen[scope] = struct{}{}
		clean = append(clean, scope)
	}
	return strings.Join(clean, " ")
}
