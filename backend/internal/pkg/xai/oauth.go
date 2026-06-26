package xai

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"net/url"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/util/logredact"
	"github.com/Wei-Shaw/sub2api/internal/util/urlvalidator"
)

const (
	OAuthIssuer         = "https://auth.x.ai"
	DiscoveryURL        = OAuthIssuer + "/.well-known/openid-configuration"
	DefaultAuthorizeURL = OAuthIssuer + "/oauth2/authorize"
	DefaultTokenURL     = OAuthIssuer + "/oauth2/token"
	DefaultBaseURL      = "https://api.x.ai/v1"
	DefaultCLIBaseURL   = "https://cli-chat-proxy.grok.com/v1"
	DefaultClientID     = "b1a00492-073a-47ea-816f-4c329264a828"
	DefaultScope        = "openid profile email offline_access grok-cli:access api:access"
	DefaultRedirectURI  = "http://127.0.0.1:56121/callback"
	SessionTTL          = 30 * time.Minute

	EnvAuthorizeURL               = "XAI_OAUTH_AUTHORIZE_URL"
	EnvTokenURL                   = "XAI_OAUTH_TOKEN_URL"
	EnvClientID                   = "XAI_OAUTH_CLIENT_ID"
	EnvScope                      = "XAI_OAUTH_SCOPE"
	EnvRedirectURI                = "XAI_OAUTH_REDIRECT_URI"
	EnvBaseURL                    = "XAI_BASE_URL"
	EnvAllowUnsafeURLOverrides    = "XAI_ALLOW_UNSAFE_URL_OVERRIDES"
	EnvUnsafeAllowHighConcurrency = "XAI_GROK_UNSAFE_ALLOW_CONCURRENCY_GT_ONE"
)

var (
	oauthEndpointAllowedHosts = []string{"x.ai", "*.x.ai"REDACTED
	baseURLAllowedHosts       = []string{"api.x.ai", "cli-chat-proxy.grok.com"REDACTED
)

// OAuthSession stores one PKCE OAuth flow.
type OAuthSession struct {
	State         string    `json:"state"`
	CodeVerifier  string    `json:"code_verifier"`
	CodeChallenge string    `json:"code_challenge"`
	ClientID      string    `json:"client_id,omitempty"`
	Scope         string    `json:"scope,omitempty"`
	ProxyURL      string    `json:"proxy_url,omitempty"`
	RedirectURI   string    `json:"redirect_uri"`
	CreatedAt     time.Time `json:"created_at"`
REDACTED

// SessionStore manages xAI OAuth sessions in memory.
type SessionStore struct {
	mu       sync.RWMutex
	sessions map[string]*OAuthSession
	stopOnce sync.Once
	stopCh   chan struct{REDACTED
REDACTED

func NewSessionStore() *SessionStore {
	store := &SessionStore{
		sessions: make(map[string]*OAuthSession),
		stopCh:   make(chan struct{REDACTED),
REDACTED
	go store.cleanup()
	return store
REDACTED

func (s *SessionStore) Set(sessionID string, session *OAuthSession) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.sessions[sessionID] = session
REDACTED

func (s *SessionStore) Get(sessionID string) (*OAuthSession, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	session, ok := s.sessions[sessionID]
	if !ok {
		return nil, false
REDACTED
	if time.Since(session.CreatedAt) > SessionTTL {
		return nil, false
REDACTED
	return session, true
REDACTED

func (s *SessionStore) Delete(sessionID string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.sessions, sessionID)
REDACTED

func (s *SessionStore) Stop() {
	s.stopOnce.Do(func() {
		close(s.stopCh)
REDACTED)
REDACTED

func (s *SessionStore) cleanup() {
	ticker := time.NewTicker(5 * time.Minute)
	defer ticker.Stop()
	for {
		select {
		case <-s.stopCh:
			return
		case <-ticker.C:
			s.mu.Lock()
			for id, session := range s.sessions {
				if time.Since(session.CreatedAt) > SessionTTL {
					delete(s.sessions, id)
			REDACTED
		REDACTED
			s.mu.Unlock()
	REDACTED
REDACTED
REDACTED

func EffectiveAuthorizeURL() string {
	return envOrDefault(EnvAuthorizeURL, DefaultAuthorizeURL)
REDACTED

func ValidatedAuthorizeURL() (string, error) {
	return ValidateOAuthEndpointURL(EffectiveAuthorizeURL())
REDACTED

func EffectiveTokenURL() string {
	return envOrDefault(EnvTokenURL, DefaultTokenURL)
REDACTED

func ValidatedTokenURL() (string, error) {
	return ValidateOAuthEndpointURL(EffectiveTokenURL())
REDACTED

func EffectiveClientID() string {
	return envOrDefault(EnvClientID, DefaultClientID)
REDACTED

func EffectiveScope() string {
	return envOrDefault(EnvScope, DefaultScope)
REDACTED

func EffectiveRedirectURI(override string) string {
	if trimmed := strings.TrimSpace(override); trimmed != "" {
		return trimmed
REDACTED
	return envOrDefault(EnvRedirectURI, DefaultRedirectURI)
REDACTED

func EffectiveBaseURL(override string) string {
	if trimmed := strings.TrimSpace(override); trimmed != "" {
		return strings.TrimRight(trimmed, "/")
REDACTED
	return strings.TrimRight(envOrDefault(EnvBaseURL, DefaultBaseURL), "/")
REDACTED

func ValidatedBaseURL(override string) (string, error) {
	return ValidateBaseURL(EffectiveBaseURL(override))
REDACTED

type RuntimeSanityCheck struct {
	Value     string `json:"value"`
	Valid     bool   `json:"valid"`
	Error     string `json:"error,omitempty"`
	IsDefault bool   `json:"is_default,omitempty"`
REDACTED

type RuntimeSanityReport struct {
	BaseURL               RuntimeSanityCheck `json:"base_url"`
	OAuthAuthorizeURL     RuntimeSanityCheck `json:"oauth_authorize_url"`
	OAuthTokenURL         RuntimeSanityCheck `json:"oauth_token_url"`
	OAuthRedirectURI      RuntimeSanityCheck `json:"oauth_redirect_uri"`
	UnsafeURLOverrides    bool               `json:"unsafe_url_overrides"`
	UnsafeHighConcurrency bool               `json:"unsafe_high_concurrency"`
	PublicGatewayScope    string             `json:"public_gateway_scope"`
	ProxyPolicy           string             `json:"proxy_policy"`
REDACTED

func RuntimeSanity() RuntimeSanityReport {
	return RuntimeSanityReport{
		BaseURL:               runtimeSanityCheck(EffectiveBaseURL(""), EnvBaseURL, ValidatedBaseURL),
		OAuthAuthorizeURL:     runtimeSanityCheck(EffectiveAuthorizeURL(), EnvAuthorizeURL, func(string) (string, error) { return ValidatedAuthorizeURL() REDACTED),
		OAuthTokenURL:         runtimeSanityCheck(EffectiveTokenURL(), EnvTokenURL, func(string) (string, error) { return ValidatedTokenURL() REDACTED),
		OAuthRedirectURI:      runtimeSanityCheck(EffectiveRedirectURI(""), EnvRedirectURI, validateRedirectURI),
		UnsafeURLOverrides:    AllowUnsafeURLOverrides(),
		UnsafeHighConcurrency: AllowUnsafeHighConcurrency(),
		PublicGatewayScope:    "responses_only",
		ProxyPolicy:           "account_proxy_optional; upstream URL allowlists enforced unless unsafe overrides are enabled",
REDACTED
REDACTED

func runtimeSanityCheck(value string, envKey string, validate func(string) (string, error)) RuntimeSanityCheck {
	normalized, err := validate(value)
	check := RuntimeSanityCheck{
		Value:     sanitizeRuntimeURLValue(normalized),
		Valid:     err == nil,
		IsDefault: strings.TrimSpace(os.Getenv(envKey)) == "",
REDACTED
	if err != nil {
		check.Value = sanitizeRuntimeURLValue(value)
		check.Error = sanitizeRuntimeError(err.Error(), value)
REDACTED
	return check
REDACTED

func validateRedirectURI(raw string) (string, error) {
	return urlvalidator.ValidateURLFormat(raw, true)
REDACTED

func sanitizeRuntimeURLValue(raw string) string {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return ""
REDACTED
	parsed, err := url.Parse(trimmed)
	if err != nil || parsed.Scheme == "" || parsed.Host == "" {
		return trimmed
REDACTED
	parsed.User = nil
	parsed.RawQuery = ""
	parsed.Fragment = ""
	return strings.TrimRight(parsed.String(), "/")
REDACTED

func sanitizeRuntimeError(rawErr string, rawValue string) string {
	redacted := logredact.RedactText(rawErr)
	trimmedValue := strings.TrimSpace(rawValue)
	if trimmedValue == "" {
		return redacted
REDACTED
	sanitizedValue := sanitizeRuntimeURLValue(trimmedValue)
	redacted = strings.ReplaceAll(redacted, trimmedValue, sanitizedValue)
	redacted = strings.ReplaceAll(redacted, logredact.RedactText(trimmedValue), sanitizedValue)
	return redacted
REDACTED

func ValidateOAuthEndpointURL(raw string) (string, error) {
	if AllowUnsafeURLOverrides() {
		return urlvalidator.ValidateURLFormat(raw, true)
REDACTED
	return urlvalidator.ValidateHTTPSURL(raw, urlvalidator.ValidationOptions{
		AllowedHosts:     oauthEndpointAllowedHosts,
		RequireAllowlist: true,
		AllowPrivate:     false,
REDACTED)
REDACTED

func ValidateBaseURL(raw string) (string, error) {
	if AllowUnsafeURLOverrides() {
		return urlvalidator.ValidateURLFormat(raw, true)
REDACTED
	normalized, err := urlvalidator.ValidateHTTPSURL(raw, urlvalidator.ValidationOptions{
		AllowedHosts:     baseURLAllowedHosts,
		RequireAllowlist: true,
		AllowPrivate:     false,
REDACTED)
	if err != nil {
		return "", err
REDACTED
	return normalizeKnownBaseURLPath(normalized)
REDACTED

func normalizeKnownBaseURLPath(raw string) (string, error) {
	parsed, err := url.Parse(raw)
	if err != nil || parsed.Scheme == "" || parsed.Host == "" {
		return "", fmt.Errorf("invalid url: %s", raw)
REDACTED
	path := strings.TrimRight(parsed.Path, "/")
	if path == "" {
		parsed.Path = "/v1"
		parsed.RawPath = ""
		return strings.TrimRight(parsed.String(), "/"), nil
REDACTED
	if path != "/v1" {
		return "", fmt.Errorf("base URL path must be /v1")
REDACTED
	parsed.Path = path
	parsed.RawPath = ""
	return strings.TrimRight(parsed.String(), "/"), nil
REDACTED

func AllowUnsafeURLOverrides() bool {
	return envBool(EnvAllowUnsafeURLOverrides)
REDACTED

func AllowUnsafeHighConcurrency() bool {
	return envBool(EnvUnsafeAllowHighConcurrency)
REDACTED

func envOrDefault(key, fallback string) string {
	if value := strings.TrimSpace(os.Getenv(key)); value != "" {
		return value
REDACTED
	return fallback
REDACTED

func envBool(key string) bool {
	switch strings.ToLower(strings.TrimSpace(os.Getenv(key))) {
	case "1", "true", "yes", "y", "on":
		return true
	default:
		return false
REDACTED
REDACTED

func GenerateRandomBytes(n int) ([]byte, error) {
	b := make([]byte, n)
	if _, err := rand.Read(b); err != nil {
		return nil, err
REDACTED
	return b, nil
REDACTED

func GenerateState() (string, error) {
	bytes, err := GenerateRandomBytes(32)
	if err != nil {
		return "", err
REDACTED
	return hex.EncodeToString(bytes), nil
REDACTED

func GenerateNonce() (string, error) {
	bytes, err := GenerateRandomBytes(16)
	if err != nil {
		return "", err
REDACTED
	return hex.EncodeToString(bytes), nil
REDACTED

func GenerateSessionID() (string, error) {
	bytes, err := GenerateRandomBytes(16)
	if err != nil {
		return "", err
REDACTED
	return hex.EncodeToString(bytes), nil
REDACTED

func GenerateCodeVerifier() (string, error) {
	bytes, err := GenerateRandomBytes(32)
	if err != nil {
		return "", err
REDACTED
	return base64URLEncode(bytes), nil
REDACTED

func GenerateCodeChallenge(verifier string) string {
	hash := sha256.Sum256([]byte(verifier))
	return base64URLEncode(hash[:])
REDACTED

func base64URLEncode(data []byte) string {
	return strings.TrimRight(base64.URLEncoding.EncodeToString(data), "=")
REDACTED

func BuildAuthorizationURL(state, codeChallenge, redirectURI, nonce string) (string, error) {
	redirectURI = EffectiveRedirectURI(redirectURI)
	authorizeURL, err := ValidatedAuthorizeURL()
	if err != nil {
		return "", fmt.Errorf("invalid authorize url: %w", err)
REDACTED

	params := url.Values{REDACTED
	params.Set("response_type", "code")
	params.Set("client_id", EffectiveClientID())
	params.Set("redirect_uri", redirectURI)
	params.Set("scope", EffectiveScope())
	params.Set("state", state)
	params.Set("nonce", nonce)
	params.Set("code_challenge", codeChallenge)
	params.Set("code_challenge_method", "S256")
	params.Set("plan", "generic")
	params.Set("referrer", "sub2api")

	return fmt.Sprintf("%s?%s", authorizeURL, params.Encode()), nil
REDACTED

// AuthorizationInput is a parsed manual OAuth callback input.
type AuthorizationInput struct {
	Code          string
	State         string
	RequiresState bool
REDACTED

// ParseAuthorizationInput accepts a full callback URL, query string, or bare code.
func ParseAuthorizationInput(raw string) AuthorizationInput {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return AuthorizationInput{REDACTED
REDACTED

	if parsed, err := url.Parse(trimmed); err == nil && parsed != nil {
		values := parsed.Query()
		if code := strings.TrimSpace(values.Get("code")); code != "" {
			return AuthorizationInput{
				Code:          code,
				State:         strings.TrimSpace(values.Get("state")),
				RequiresState: true,
		REDACTED
	REDACTED
REDACTED

	queryCandidate := strings.TrimPrefix(trimmed, "?")
	if strings.Contains(queryCandidate, "=") {
		if values, err := url.ParseQuery(queryCandidate); err == nil {
			if code := strings.TrimSpace(values.Get("code")); code != "" {
				return AuthorizationInput{
					Code:          code,
					State:         strings.TrimSpace(values.Get("state")),
					RequiresState: true,
			REDACTED
		REDACTED
	REDACTED
REDACTED

	return AuthorizationInput{Code: trimmedREDACTED
REDACTED

func BuildResponsesURL(baseURL string) (string, error) {
	validatedBaseURL, err := ValidatedBaseURL(baseURL)
	if err != nil {
		return "", fmt.Errorf("invalid base url: %w", err)
REDACTED
	return validatedBaseURL + "/responses", nil
REDACTED

func BuildChatCompletionsURL(baseURL string) (string, error) {
	validatedBaseURL, err := ValidatedBaseURL(baseURL)
	if err != nil {
		return "", fmt.Errorf("invalid base url: %w", err)
REDACTED
	return validatedBaseURL + "/chat/completions", nil
REDACTED

// TokenResponse represents xAI OAuth token responses.
type TokenResponse struct {
	AccessToken  string `json:"access_token"`
	RefreshToken string `json:"refresh_token,omitempty"`
	IDToken      string `json:"id_token,omitempty"`
	TokenType    string `json:"token_type,omitempty"`
	ExpiresIn    int64  `json:"expires_in,omitempty"`
	Scope        string `json:"scope,omitempty"`
REDACTED
