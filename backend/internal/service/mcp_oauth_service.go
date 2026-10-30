package service

import (
	"context"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"net/url"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
)

const (
	MCPScopeUser           = "user"
	MCPScopeAdmin          = "admin"
	MCPOAuthScope          = "mcp"
	MCPOAuthGrantLimit     = 20
	MCPOAuthAccessLifetime = 10 * time.Minute
	MCPOAuthCodeLifetime   = 5 * time.Minute
	MCPOAuthGrantLifetime  = 30 * 24 * time.Hour
	// last_used_at is advisory; writing it once per interval keeps requests read-only.
	MCPOAuthTouchInterval = time.Minute

	mcpOAuthClientIDPrefix = "mcp_client_"
	// Registration bounds keep issued IDs well below this limit.
	mcpOAuthClientIDMaxLength = 2048
)

var (
	ErrMCPOAuthInvalidToken  = infraerrors.Unauthorized("INVALID_MCP_TOKEN", "MCP access token is invalid or expired")
	ErrMCPOAuthNotFound      = errors.New("MCP OAuth record not found")
	ErrMCPOAuthGrantLimit    = infraerrors.Conflict("MCP_GRANT_LIMIT", "Revoke an existing MCP authorization before authorizing another client (maximum 20 active authorizations)")
	ErrMCPOAuthGrantNotFound = infraerrors.NotFound("MCP_GRANT_NOT_FOUND", "MCP authorization not found")
)

type MCPOAuthError struct {
	Code        string `json:"error"`
	Description string `json:"error_description"`
}

func (e *MCPOAuthError) Error() string { return e.Code + ": " + e.Description }

func mcpOAuthError(code, description string) error {
	return &MCPOAuthError{Code: code, Description: description}
}

type MCPOAuthRegisterInput struct {
	ClientName              string   `json:"client_name"`
	TokenEndpointAuthMethod string   `json:"token_endpoint_auth_method"`
	RedirectURIs            []string `json:"redirect_uris"`
	GrantTypes              []string `json:"grant_types"`
	ResponseTypes           []string `json:"response_types"`
}

type MCPOAuthClient struct {
	ClientID                string   `json:"client_id"`
	ClientName              string   `json:"client_name"`
	TokenEndpointAuthMethod string   `json:"token_endpoint_auth_method"`
	RedirectURIs            []string `json:"redirect_uris"`
	GrantTypes              []string `json:"grant_types"`
	ResponseTypes           []string `json:"response_types"`
	ClientIDIssuedAt        int64    `json:"client_id_issued_at"`
}

type MCPOAuthAuthorizeInput struct {
	ResponseType        string `json:"response_type"`
	ClientID            string `json:"client_id"`
	RedirectURI         string `json:"redirect_uri"`
	Scope               string `json:"scope"`
	State               string `json:"state"`
	CodeChallenge       string `json:"code_challenge"`
	CodeChallengeMethod string `json:"code_challenge_method"`
	Resource            string `json:"resource"`
}

type MCPOAuthConsentInput struct {
	Authorization MCPOAuthAuthorizeInput `json:"authorization"`
	Approved      bool                   `json:"approved"`
	Scope         string                 `json:"scope"`
	ReadOnly      *bool                  `json:"read_only"`
}

type MCPOAuthConsent struct {
	ClientID    string `json:"client_id"`
	ClientName  string `json:"client_name"`
	RedirectURI string `json:"redirect_uri"`
	Resource    string `json:"resource"`
	Scope       string `json:"scope"`
}

type MCPOAuthGrant struct {
	ID           string     `json:"id"`
	ClientID     string     `json:"client_id"`
	ClientName   string     `json:"client_name"`
	Scope        string     `json:"scope"`
	ReadOnly     bool       `json:"read_only"`
	CreatedAt    time.Time  `json:"created_at"`
	ExpiresAt    time.Time  `json:"expires_at"`
	LastUsedAt   *time.Time `json:"last_used_at"`
	RevokedAt    *time.Time `json:"revoked_at"`
	Invalidated  bool       `json:"invalidated"`
	UserID       int64      `json:"-"`
	TokenVersion int64      `json:"-"`
	Resource     string     `json:"-"`
}

type MCPPrincipal struct {
	GrantID     string
	UserID      int64
	Role        string
	Scope       string
	ReadOnly    bool
	Concurrency int
	Email       string
	Resource    string
}

type MCPOAuthTokenInput struct {
	GrantType    string `json:"grant_type"`
	ClientID     string `json:"client_id"`
	Code         string `json:"code"`
	RedirectURI  string `json:"redirect_uri"`
	CodeVerifier string `json:"code_verifier"`
	RefreshToken string `json:"refresh_token"`
	Resource     string `json:"resource"`
	Scope        string `json:"scope"`
}

type MCPOAuthTokenResponse struct {
	AccessToken  string `json:"access_token"`
	RefreshToken string `json:"refresh_token,omitempty"`
	TokenType    string `json:"token_type"`
	ExpiresIn    int64  `json:"expires_in"`
	Scope        string `json:"scope"`
}

// Authorization codes and credentials contain digests only; bearer secrets are
// returned directly to the client and never reach persistence.
type MCPOAuthCode struct {
	Hash, GrantID, ClientID, RedirectURI, Resource, CodeChallenge string
	ExpiresAt                                                     time.Time
	ConsumedAt                                                    *time.Time
}

type MCPOAuthCredential struct {
	Hash, GrantID, Kind string
	ExpiresAt           time.Time
	UsedAt              *time.Time
}

type MCPOAuthRepository interface {
	// ClientIDKey returns the persistent key that signs stateless client IDs.
	ClientIDKey(context.Context) ([]byte, error)
	CreateAuthorization(context.Context, *MCPOAuthGrant, *MCPOAuthCode, int) error
	GetCode(context.Context, string) (*MCPOAuthCode, error)
	GetCredential(context.Context, string) (*MCPOAuthCredential, error)
	GetGrant(context.Context, string) (*MCPOAuthGrant, error)
	// ExchangeCode and RotateRefresh atomically consume and issue credentials.
	// Replays revoke the entire grant and COMMIT that revocation before erroring.
	ExchangeCode(context.Context, string, string, time.Time, *MCPOAuthCredential, *MCPOAuthCredential) error
	RotateRefresh(context.Context, string, string, time.Time, *MCPOAuthCredential, *MCPOAuthCredential) error
	TouchGrant(context.Context, string, time.Time) error
	ListGrants(context.Context, int64) ([]*MCPOAuthGrant, error)
	RevokeGrant(context.Context, int64, string, time.Time) error
	RevokeUserGrants(context.Context, int64, time.Time) error
}

type MCPOAuthService struct {
	repo  MCPOAuthRepository
	users UserRepository
}

func NewMCPOAuthService(repo MCPOAuthRepository, users UserRepository) *MCPOAuthService {
	return &MCPOAuthService{repo: repo, users: users}
}

func (s *MCPOAuthService) Register(ctx context.Context, input MCPOAuthRegisterInput) (*MCPOAuthClient, error) {
	input.ClientName = strings.TrimSpace(input.ClientName)
	if input.ClientName == "" {
		input.ClientName = "MCP client"
	}
	if !validMCPClientName(input.ClientName) {
		return nil, mcpOAuthError("invalid_client_metadata", "client_name must contain at most 100 printable characters")
	}
	if input.TokenEndpointAuthMethod != "" && input.TokenEndpointAuthMethod != "none" {
		return nil, mcpOAuthError("invalid_client_metadata", "Only public clients using token_endpoint_auth_method=none are supported")
	}
	if len(input.RedirectURIs) < 1 || len(input.RedirectURIs) > 10 {
		return nil, mcpOAuthError("invalid_redirect_uri", "Register between 1 and 10 redirect URIs")
	}
	redirects := make([]string, 0, len(input.RedirectURIs))
	for _, uri := range input.RedirectURIs {
		if !validMCPRedirectURI(uri) {
			return nil, mcpOAuthError("invalid_redirect_uri", "Redirect URIs must use HTTPS or HTTP loopback, without fragments, credentials, or wildcards")
		}
		if !containsMCPString(redirects, uri) {
			redirects = append(redirects, uri)
		}
	}
	grants := input.GrantTypes
	if len(grants) == 0 {
		grants = []string{"authorization_code", "refresh_token"}
	}
	if len(grants) > 2 || !containsMCPString(grants, "authorization_code") {
		return nil, mcpOAuthError("invalid_client_metadata", "authorization_code is required")
	}
	for _, grant := range grants {
		if grant != "authorization_code" && grant != "refresh_token" {
			return nil, mcpOAuthError("invalid_client_metadata", "Unsupported grant type")
		}
	}
	if len(input.ResponseTypes) != 0 && (len(input.ResponseTypes) != 1 || input.ResponseTypes[0] != "code") {
		return nil, mcpOAuthError("invalid_client_metadata", "Only the code response type is supported")
	}
	nonce := make([]byte, 16)
	if _, err := rand.Read(nonce); err != nil {
		return nil, err
	}
	claims := mcpOAuthClientClaims{Name: input.ClientName, GrantTypes: append([]string(nil), grants...)}
	for _, uri := range redirects {
		claims.RedirectDigests = append(claims.RedirectDigests, mcpOAuthRedirectDigest(uri))
	}
	id, err := s.encodeClientID(ctx, base64.RawURLEncoding.EncodeToString(nonce), &claims)
	if err != nil {
		return nil, err
	}
	return &MCPOAuthClient{
		ClientID: id, ClientName: input.ClientName, RedirectURIs: redirects,
		GrantTypes: claims.GrantTypes, ResponseTypes: []string{"code"},
		TokenEndpointAuthMethod: "none", ClientIDIssuedAt: time.Now().UTC().Unix(),
	}, nil
}

// Registration is unauthenticated, so it is never persisted: the client ID is
// the signed metadata itself. Nothing is stored until a user approves a client,
// leaving no storage or shared capacity for anonymous registrations to exhaust.
// Redirect URIs are embedded as digests to keep IDs short enough for URLs.
//
// Format: mcp_client_<nonce>.<base64url claims>.<HMAC of nonce and claims>.
// The random nonce leads, so truncated IDs still tell clients apart.
type mcpOAuthClientClaims struct {
	Name            string   `json:"n"`
	RedirectDigests []string `json:"r"`
	GrantTypes      []string `json:"g"`
}

func (c *mcpOAuthClientClaims) allowsRedirect(uri string) bool {
	return containsMCPString(c.RedirectDigests, mcpOAuthRedirectDigest(uri))
}

func mcpOAuthRedirectDigest(uri string) string {
	sum := sha256.Sum256([]byte(uri))
	return base64.RawURLEncoding.EncodeToString(sum[:])
}

func (s *MCPOAuthService) encodeClientID(ctx context.Context, nonce string, claims *mcpOAuthClientClaims) (string, error) {
	raw, err := json.Marshal(claims)
	if err != nil {
		return "", err
	}
	signed := nonce + "." + base64.RawURLEncoding.EncodeToString(raw)
	signature, err := s.signClientID(ctx, signed)
	if err != nil {
		return "", err
	}
	return mcpOAuthClientIDPrefix + signed + "." + signature, nil
}

// client verifies a client ID and returns its metadata. IDs that were not
// issued by this server yield ErrMCPOAuthNotFound.
func (s *MCPOAuthService) client(ctx context.Context, id string) (*mcpOAuthClientClaims, error) {
	if len(id) > mcpOAuthClientIDMaxLength || !strings.HasPrefix(id, mcpOAuthClientIDPrefix) {
		return nil, ErrMCPOAuthNotFound
	}
	signed := id[len(mcpOAuthClientIDPrefix):]
	dot := strings.LastIndexByte(signed, '.')
	if dot < 0 {
		return nil, ErrMCPOAuthNotFound
	}
	signed, signature := signed[:dot], signed[dot+1:]
	expected, err := s.signClientID(ctx, signed)
	if err != nil {
		return nil, err
	}
	if !hmac.Equal([]byte(signature), []byte(expected)) {
		return nil, ErrMCPOAuthNotFound
	}
	_, payload, ok := strings.Cut(signed, ".")
	if !ok {
		return nil, ErrMCPOAuthNotFound
	}
	raw, err := base64.RawURLEncoding.DecodeString(payload)
	if err != nil {
		return nil, ErrMCPOAuthNotFound
	}
	var claims mcpOAuthClientClaims
	if err := json.Unmarshal(raw, &claims); err != nil {
		return nil, ErrMCPOAuthNotFound
	}
	return &claims, nil
}

func (s *MCPOAuthService) signClientID(ctx context.Context, payload string) (string, error) {
	key, err := s.repo.ClientIDKey(ctx)
	if err != nil {
		return "", err
	}
	mac := hmac.New(sha256.New, key)
	_, _ = mac.Write([]byte(payload))
	return base64.RawURLEncoding.EncodeToString(mac.Sum(nil)), nil
}

func validMCPClientName(name string) bool {
	if !utf8.ValidString(name) || utf8.RuneCountInString(name) > 100 {
		return false
	}
	for _, r := range name {
		// Bidi overrides, zero-width and control characters can make the consent
		// page show a self-reported name that looks like a different client.
		if unicode.In(r, unicode.Cc, unicode.Cf, unicode.Co, unicode.Zl, unicode.Zp) {
			return false
		}
	}
	return true
}

func validMCPRedirectURI(raw string) bool {
	if len(raw) > 2048 || strings.ContainsAny(raw, "*#") {
		return false
	}
	u, err := url.Parse(raw)
	if err != nil || u.Hostname() == "" || u.User != nil || u.Opaque != "" || u.Fragment != "" {
		return false
	}
	if u.Scheme == "https" {
		return true
	}
	if u.Scheme != "http" {
		return false
	}
	switch strings.ToLower(u.Hostname()) {
	case "localhost", "127.0.0.1", "::1":
		return true
	default:
		return false
	}
}

func containsMCPString(values []string, want string) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
}

func validMCPOAuthScope(scope string) bool {
	fields := strings.Fields(scope)
	return len(fields) == 0 || (len(fields) == 1 && fields[0] == MCPOAuthScope)
}

func (s *MCPOAuthService) validateAuthorization(ctx context.Context, userID int64, input MCPOAuthAuthorizeInput, resource string) (*mcpOAuthClientClaims, *User, error) {
	client, err := s.client(ctx, input.ClientID)
	if errors.Is(err, ErrMCPOAuthNotFound) {
		return nil, nil, mcpOAuthError("invalid_client", "Unknown client")
	}
	if err != nil {
		return nil, nil, err
	}
	if !client.allowsRedirect(input.RedirectURI) {
		return nil, nil, mcpOAuthError("invalid_request", "redirect_uri must exactly match a registered URI")
	}
	if input.ResponseType != "code" {
		return nil, nil, mcpOAuthError("unsupported_response_type", "Only response_type=code is supported")
	}
	if !validMCPOAuthScope(input.Scope) {
		return nil, nil, mcpOAuthError("invalid_scope", "Only the mcp OAuth scope is supported")
	}
	if resource == "" || input.Resource != resource {
		return nil, nil, mcpOAuthError("invalid_target", "resource must identify this MCP endpoint")
	}
	challenge, decodeErr := base64.RawURLEncoding.DecodeString(input.CodeChallenge)
	if input.CodeChallengeMethod != "S256" || decodeErr != nil || len(challenge) != sha256.Size || base64.RawURLEncoding.EncodeToString(challenge) != input.CodeChallenge {
		return nil, nil, mcpOAuthError("invalid_request", "A valid S256 PKCE code challenge is required")
	}
	if len(input.State) > 2048 {
		return nil, nil, mcpOAuthError("invalid_request", "state is too long")
	}
	user, err := s.users.GetByID(ctx, userID)
	if errors.Is(err, ErrUserNotFound) || (err == nil && (user == nil || !user.IsActive() || user.DeletedAt != nil)) {
		return nil, nil, mcpOAuthError("access_denied", "User account is not active")
	}
	if err != nil {
		return nil, nil, err
	}
	return client, user, nil
}

func (s *MCPOAuthService) Preview(ctx context.Context, userID int64, input MCPOAuthAuthorizeInput, resource string) (*MCPOAuthConsent, error) {
	client, _, err := s.validateAuthorization(ctx, userID, input, resource)
	if err != nil {
		return nil, err
	}
	return &MCPOAuthConsent{ClientID: input.ClientID, ClientName: client.Name, RedirectURI: input.RedirectURI, Resource: resource, Scope: MCPOAuthScope}, nil
}

func (s *MCPOAuthService) Decide(ctx context.Context, userID int64, input MCPOAuthConsentInput, resource string) (string, error) {
	client, user, err := s.validateAuthorization(ctx, userID, input.Authorization, resource)
	if err != nil {
		return "", err
	}
	redirect, _ := url.Parse(input.Authorization.RedirectURI)
	query := redirect.Query()
	// Registered URI queries are retained; OAuth response fields are authoritative.
	query.Del("code")
	query.Del("error")
	query.Del("error_description")
	query.Del("state")
	if input.Authorization.State != "" {
		query.Set("state", input.Authorization.State)
	}
	if !input.Approved {
		query.Set("error", "access_denied")
		redirect.RawQuery = query.Encode()
		return redirect.String(), nil
	}
	if input.Scope == "" {
		input.Scope = MCPScopeUser
	}
	if input.Scope != MCPScopeUser && input.Scope != MCPScopeAdmin {
		return "", mcpOAuthError("invalid_scope", "Management permission must be user or admin")
	}
	if input.Scope == MCPScopeAdmin && !user.IsAdmin() {
		return "", mcpOAuthError("access_denied", "Only administrators can authorize admin access")
	}
	id, err := mcpOAuthSecret("")
	if err != nil {
		return "", err
	}
	rawCode, err := mcpOAuthSecret("mcp_code_")
	if err != nil {
		return "", err
	}
	now := time.Now().UTC()
	grant := &MCPOAuthGrant{ID: id, ClientID: input.Authorization.ClientID, ClientName: client.Name,
		UserID: user.ID, TokenVersion: resolvedTokenVersion(user), Resource: resource,
		Scope: input.Scope, ReadOnly: true, CreatedAt: now, ExpiresAt: now.Add(MCPOAuthGrantLifetime)}
	if input.ReadOnly != nil {
		grant.ReadOnly = *input.ReadOnly
	}
	code := &MCPOAuthCode{Hash: hashMCPOAuthSecret(rawCode), GrantID: grant.ID, ClientID: grant.ClientID,
		RedirectURI: input.Authorization.RedirectURI, Resource: resource, CodeChallenge: input.Authorization.CodeChallenge,
		ExpiresAt: now.Add(MCPOAuthCodeLifetime)}
	if err := s.repo.CreateAuthorization(ctx, grant, code, MCPOAuthGrantLimit); err != nil {
		return "", err
	}
	query.Set("code", rawCode)
	redirect.RawQuery = query.Encode()
	return redirect.String(), nil
}

func (s *MCPOAuthService) Token(ctx context.Context, input MCPOAuthTokenInput, resource string) (*MCPOAuthTokenResponse, error) {
	client, err := s.client(ctx, input.ClientID)
	if errors.Is(err, ErrMCPOAuthNotFound) {
		return nil, mcpOAuthError("invalid_client", "Unknown client")
	}
	if err != nil {
		return nil, err
	}
	if !validMCPOAuthScope(input.Scope) {
		return nil, mcpOAuthError("invalid_scope", "Only the mcp OAuth scope is supported")
	}
	if input.GrantType != "authorization_code" && input.GrantType != "refresh_token" {
		return nil, mcpOAuthError("unsupported_grant_type", "Unsupported grant type")
	}
	if !containsMCPString(client.GrantTypes, input.GrantType) {
		return nil, mcpOAuthError("unauthorized_client", "Client is not registered for this grant type")
	}
	var grantID, credentialHash string
	if input.GrantType == "authorization_code" {
		if !validMCPOAuthSecret(input.Code, "mcp_code_") || !validMCPCodeVerifier(input.CodeVerifier) {
			return nil, mcpOAuthError("invalid_grant", "Invalid authorization code or verifier")
		}
		credentialHash = hashMCPOAuthSecret(input.Code)
		code, err := s.repo.GetCode(ctx, credentialHash)
		if errors.Is(err, ErrMCPOAuthNotFound) || (err == nil && code == nil) {
			return nil, mcpOAuthError("invalid_grant", "Invalid authorization code")
		}
		if err != nil {
			return nil, err
		}
		sum := sha256.Sum256([]byte(input.CodeVerifier))
		challenge := base64.RawURLEncoding.EncodeToString(sum[:])
		if code.ClientID != input.ClientID || code.RedirectURI != input.RedirectURI || resource == "" || input.Resource != resource || code.Resource != resource || subtle.ConstantTimeCompare([]byte(challenge), []byte(code.CodeChallenge)) != 1 {
			return nil, mcpOAuthError("invalid_grant", "Authorization code binding or PKCE verification failed")
		}
		grantID = code.GrantID
	} else {
		if !validMCPOAuthSecret(input.RefreshToken, "mcp_rt_") {
			return nil, mcpOAuthError("invalid_grant", "Invalid refresh token")
		}
		credentialHash = hashMCPOAuthSecret(input.RefreshToken)
		credential, err := s.repo.GetCredential(ctx, credentialHash)
		if errors.Is(err, ErrMCPOAuthNotFound) || (err == nil && (credential == nil || credential.Kind != "refresh")) {
			return nil, mcpOAuthError("invalid_grant", "Invalid refresh token")
		}
		if err != nil {
			return nil, err
		}
		grantID = credential.GrantID
	}
	grant, err := s.repo.GetGrant(ctx, grantID)
	if errors.Is(err, ErrMCPOAuthNotFound) || (err == nil && grant == nil) {
		return nil, mcpOAuthError("invalid_grant", "Authorization is no longer valid")
	}
	if err != nil {
		return nil, err
	}
	if grant.ClientID != input.ClientID || resource == "" || grant.Resource != resource || (input.Resource != "" && input.Resource != grant.Resource) {
		return nil, mcpOAuthError("invalid_grant", "Token is not bound to this client and resource")
	}
	if _, err := s.validateGrant(ctx, grant); err != nil {
		if errors.Is(err, ErrMCPOAuthInvalidToken) {
			return nil, mcpOAuthError("invalid_grant", "Authorization is no longer valid")
		}
		return nil, err
	}
	now := time.Now().UTC()
	result, access, refresh, err := newMCPOAuthTokens(grant, now, containsMCPString(client.GrantTypes, "refresh_token"))
	if err != nil {
		return nil, err
	}
	if input.GrantType == "authorization_code" {
		err = s.repo.ExchangeCode(ctx, credentialHash, grant.ID, now, access, refresh)
	} else {
		err = s.repo.RotateRefresh(ctx, credentialHash, grant.ID, now, access, refresh)
	}
	if errors.Is(err, ErrMCPOAuthNotFound) {
		return nil, mcpOAuthError("invalid_grant", "Code or refresh token was expired, revoked, or already used")
	}
	if err != nil {
		return nil, err
	}
	return result, nil
}

func newMCPOAuthTokens(grant *MCPOAuthGrant, now time.Time, includeRefresh bool) (*MCPOAuthTokenResponse, *MCPOAuthCredential, *MCPOAuthCredential, error) {
	access, err := mcpOAuthSecret("mcp_at_")
	if err != nil {
		return nil, nil, nil, err
	}
	expiresAt := now.Add(MCPOAuthAccessLifetime)
	if grant.ExpiresAt.Before(expiresAt) {
		expiresAt = grant.ExpiresAt
	}
	result := &MCPOAuthTokenResponse{AccessToken: access, TokenType: "Bearer", ExpiresIn: int64(expiresAt.Sub(now) / time.Second), Scope: MCPOAuthScope}
	accessRecord := &MCPOAuthCredential{Hash: hashMCPOAuthSecret(access), GrantID: grant.ID, Kind: "access", ExpiresAt: expiresAt}
	var refreshRecord *MCPOAuthCredential
	if includeRefresh {
		refresh, err := mcpOAuthSecret("mcp_rt_")
		if err != nil {
			return nil, nil, nil, err
		}
		result.RefreshToken = refresh
		refreshRecord = &MCPOAuthCredential{Hash: hashMCPOAuthSecret(refresh), GrantID: grant.ID, Kind: "refresh", ExpiresAt: grant.ExpiresAt}
	}
	return result, accessRecord, refreshRecord, nil
}

func validMCPCodeVerifier(value string) bool {
	if len(value) < 43 || len(value) > 128 {
		return false
	}
	for _, ch := range value {
		if (ch < 'a' || ch > 'z') && (ch < 'A' || ch > 'Z') && (ch < '0' || ch > '9') && ch != '-' && ch != '.' && ch != '_' && ch != '~' {
			return false
		}
	}
	return true
}

func (s *MCPOAuthService) validateGrant(ctx context.Context, grant *MCPOAuthGrant) (*User, error) {
	if grant == nil || grant.RevokedAt != nil || !time.Now().Before(grant.ExpiresAt) || (grant.Scope != MCPScopeUser && grant.Scope != MCPScopeAdmin) {
		return nil, ErrMCPOAuthInvalidToken
	}
	user, err := s.users.GetByID(ctx, grant.UserID)
	if errors.Is(err, ErrUserNotFound) {
		return nil, ErrMCPOAuthInvalidToken
	}
	if err != nil {
		return nil, err
	}
	if mcpGrantInvalidated(grant, user) {
		return nil, ErrMCPOAuthInvalidToken
	}
	return user, nil
}

func mcpGrantInvalidated(grant *MCPOAuthGrant, user *User) bool {
	return user == nil || !user.IsActive() || user.DeletedAt != nil || grant.TokenVersion != resolvedTokenVersion(user) || (grant.Scope == MCPScopeAdmin && !user.IsAdmin())
}

func (s *MCPOAuthService) Authenticate(ctx context.Context, raw string) (*MCPPrincipal, error) {
	if !validMCPOAuthSecret(raw, "mcp_at_") {
		return nil, ErrMCPOAuthInvalidToken
	}
	credential, err := s.repo.GetCredential(ctx, hashMCPOAuthSecret(raw))
	if errors.Is(err, ErrMCPOAuthNotFound) {
		return nil, ErrMCPOAuthInvalidToken
	}
	if err != nil {
		return nil, err
	}
	now := time.Now().UTC()
	if credential == nil || credential.Kind != "access" || !now.Before(credential.ExpiresAt) {
		return nil, ErrMCPOAuthInvalidToken
	}
	grant, err := s.repo.GetGrant(ctx, credential.GrantID)
	if errors.Is(err, ErrMCPOAuthNotFound) {
		return nil, ErrMCPOAuthInvalidToken
	}
	if err != nil {
		return nil, err
	}
	user, err := s.validateGrant(ctx, grant)
	if err != nil {
		return nil, err
	}
	if grant.LastUsedAt == nil || !now.Before(grant.LastUsedAt.Add(MCPOAuthTouchInterval)) {
		if err := s.repo.TouchGrant(ctx, grant.ID, now); err != nil {
			if errors.Is(err, ErrMCPOAuthNotFound) {
				return nil, ErrMCPOAuthInvalidToken
			}
			return nil, err
		}
	}
	return &MCPPrincipal{GrantID: grant.ID, UserID: user.ID, Role: user.Role, Scope: grant.Scope,
		ReadOnly: grant.ReadOnly, Concurrency: user.Concurrency, Email: user.Email, Resource: grant.Resource}, nil
}

func (s *MCPOAuthService) ListGrants(ctx context.Context, userID int64) ([]*MCPOAuthGrant, error) {
	user, err := s.users.GetByID(ctx, userID)
	if err != nil {
		return nil, err
	}
	grants, err := s.repo.ListGrants(ctx, userID)
	if err != nil {
		return nil, err
	}
	for _, grant := range grants {
		grant.Invalidated = mcpGrantInvalidated(grant, user)
	}
	return grants, nil
}

func (s *MCPOAuthService) RevokeGrant(ctx context.Context, userID int64, id string) error {
	err := s.repo.RevokeGrant(ctx, userID, id, time.Now().UTC())
	if errors.Is(err, ErrMCPOAuthNotFound) {
		return ErrMCPOAuthGrantNotFound
	}
	return err
}

// RevokeAllGrants persistently revokes every MCP authorization of the user,
// including refresh families, e.g. when all browser sessions are revoked.
func (s *MCPOAuthService) RevokeAllGrants(ctx context.Context, userID int64) error {
	return s.repo.RevokeUserGrants(ctx, userID, time.Now().UTC())
}

func (s *MCPOAuthService) RevokeToken(ctx context.Context, clientID, raw string) error {
	if !validMCPOAuthSecret(raw, "mcp_at_") && !validMCPOAuthSecret(raw, "mcp_rt_") {
		return nil
	}
	credential, err := s.repo.GetCredential(ctx, hashMCPOAuthSecret(raw))
	if errors.Is(err, ErrMCPOAuthNotFound) || (err == nil && credential == nil) {
		return nil
	}
	if err != nil {
		return err
	}
	grant, err := s.repo.GetGrant(ctx, credential.GrantID)
	if errors.Is(err, ErrMCPOAuthNotFound) || (err == nil && (grant == nil || grant.ClientID != clientID)) {
		return nil
	}
	if err != nil {
		return err
	}
	err = s.repo.RevokeGrant(ctx, grant.UserID, grant.ID, time.Now().UTC())
	if errors.Is(err, ErrMCPOAuthNotFound) {
		return nil
	}
	return err
}

func mcpOAuthSecret(prefix string) (string, error) {
	raw := make([]byte, 32)
	if _, err := rand.Read(raw); err != nil {
		return "", err
	}
	return prefix + base64.RawURLEncoding.EncodeToString(raw), nil
}

func validMCPOAuthSecret(raw, prefix string) bool {
	if len(raw) != len(prefix)+43 || !strings.HasPrefix(raw, prefix) {
		return false
	}
	decoded, err := base64.RawURLEncoding.DecodeString(raw[len(prefix):])
	return err == nil && len(decoded) == 32 && base64.RawURLEncoding.EncodeToString(decoded) == raw[len(prefix):]
}

func hashMCPOAuthSecret(raw string) string {
	sum := sha256.Sum256([]byte(raw))
	return hex.EncodeToString(sum[:])
}
