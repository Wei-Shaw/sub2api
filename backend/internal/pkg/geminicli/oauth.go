package geminicli

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"net/url"
	"strings"
	"sync"
	"time"
)

type OAuthConfig struct {
	ClientID     string
	ClientSecret string
	Scopes       string
REDACTED

type OAuthSession struct {
	State        string    `json:"state"`
	CodeVerifier string    `json:"code_verifier"`
	ProxyURL     string    `json:"proxy_url,omitempty"`
	RedirectURI  string    `json:"redirect_uri"`
	CreatedAt    time.Time `json:"created_at"`
REDACTED

type SessionStore struct {
	mu       sync.RWMutex
	sessions map[string]*OAuthSession
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
	select {
	case <-s.stopCh:
		return
	default:
		close(s.stopCh)
REDACTED
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

func GenerateRandomBytes(n int) ([]byte, error) {
	b := make([]byte, n)
	_, err := rand.Read(b)
	if err != nil {
		return nil, err
REDACTED
	return b, nil
REDACTED

func GenerateState() (string, error) {
	bytes, err := GenerateRandomBytes(32)
	if err != nil {
		return "", err
REDACTED
	return base64URLEncode(bytes), nil
REDACTED

func GenerateSessionID() (string, error) {
	bytes, err := GenerateRandomBytes(16)
	if err != nil {
		return "", err
REDACTED
	return hex.EncodeToString(bytes), nil
REDACTED

// GenerateCodeVerifier returns an RFC 7636 compatible code verifier (43+ chars).
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

func BuildAuthorizationURL(cfg OAuthConfig, state, codeChallenge, redirectURI string) (string, error) {
	if strings.TrimSpace(cfg.ClientID) == "" {
		return "", fmt.Errorf("gemini oauth client_id is empty")
REDACTED
	redirectURI = strings.TrimSpace(redirectURI)
	if redirectURI == "" {
		return "", fmt.Errorf("redirect_uri is required")
REDACTED

	scopes := strings.TrimSpace(cfg.Scopes)
	if scopes == "" {
		scopes = DefaultScopes
REDACTED

	params := url.Values{REDACTED
	params.Set("response_type", "code")
	params.Set("client_id", cfg.ClientID)
	params.Set("redirect_uri", redirectURI)
	params.Set("scope", scopes)
	params.Set("state", state)
	params.Set("code_challenge", codeChallenge)
	params.Set("code_challenge_method", "S256")
	params.Set("access_type", "offline")
	params.Set("prompt", "consent")
	params.Set("include_granted_scopes", "true")

	return fmt.Sprintf("%s?%s", AuthorizeURL, params.Encode()), nil
REDACTED
