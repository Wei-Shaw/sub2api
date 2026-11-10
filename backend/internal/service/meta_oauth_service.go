package service

import (
	"context"
	"errors"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
	"github.com/Wei-Shaw/sub2api/internal/pkg/meta"
)

const (
	metaDeviceSessionTTL     = 20 * time.Minute
	metaDeviceSessionMaxKeep = 4096
)

type MetaDeviceSessionState = meta.PollState

type MetaDeviceSession struct {
	DeviceCode              string
	UserCode                string
	VerificationURI         string
	VerificationURIComplete string
	Interval                time.Duration
	ExpiresAt               time.Time
	ProxyURL                string
	Authorized              *meta.APIKey
	DCAToken                string
	AccountCreated          bool
}

type MetaDeviceStart struct {
	DeviceCodeID            string `json:"device_code_id"`
	UserCode                string `json:"user_code"`
	VerificationURI         string `json:"verification_uri"`
	VerificationURIComplete string `json:"verification_uri_complete"`
	ExpiresIn               int    `json:"expires_in"`
	Interval                int    `json:"interval"`
}

type MetaDevicePoll struct {
	State     MetaDeviceSessionState `json:"state"`
	Interval  time.Duration          `json:"interval"`
	Message   string                 `json:"message,omitempty"`
	MintedKey *meta.APIKey           `json:"-"`
	DCAToken  string                 `json:"-"`
}

type MetaDeviceSessionStore struct {
	mu       sync.Mutex
	sessions map[string]*MetaDeviceSession
}

func NewMetaDeviceSessionStore() *MetaDeviceSessionStore {
	return &MetaDeviceSessionStore{sessions: make(map[string]*MetaDeviceSession)}
}

func (s *MetaDeviceSessionStore) Set(id string, session *MetaDeviceSession) {
	if s == nil || session == nil {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if len(s.sessions) >= metaDeviceSessionMaxKeep {
		s.pruneLocked(time.Now())
	}
	s.sessions[id] = session
}

func (s *MetaDeviceSessionStore) Get(id string) (*MetaDeviceSession, bool) {
	if s == nil {
		return nil, false
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	session, ok := s.sessions[id]
	if !ok {
		return nil, false
	}
	if time.Now().After(session.ExpiresAt) {
		delete(s.sessions, id)
		return nil, false
	}
	return session, true
}

func (s *MetaDeviceSessionStore) Delete(id string) {
	if s == nil {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.sessions, id)
}

func (s *MetaDeviceSessionStore) pruneLocked(now time.Time) {
	for id, session := range s.sessions {
		if session == nil || now.After(session.ExpiresAt) {
			delete(s.sessions, id)
		}
	}
}

type MetaOAuthService struct {
	sessions        *MetaDeviceSessionStore
	proxyRepo       ProxyRepository
	httpClient      meta.Doer
	customTransport bool
	now             func() time.Time
}

func NewMetaOAuthService(proxyRepo ProxyRepository) *MetaOAuthService {
	return &MetaOAuthService{
		sessions:   NewMetaDeviceSessionStore(),
		proxyRepo:  proxyRepo,
		httpClient: &http.Client{Timeout: 30 * time.Second},
		now:        time.Now,
	}
}

func (s *MetaOAuthService) WithTransport(doer meta.Doer) *MetaOAuthService {
	if s != nil && doer != nil {
		s.httpClient = doer
		s.customTransport = true
	}
	return s
}

func (s *MetaOAuthService) clientFor(proxyURL string) *meta.Client {
	if s == nil {
		return meta.NewClient(nil)
	}
	if s.httpClient == nil || !s.customTransport || strings.TrimSpace(proxyURL) == "" {
		return meta.NewClient(s.httpClient)
	}
	parsed, err := url.Parse(strings.TrimSpace(proxyURL))
	if err != nil || parsed.Host == "" {
		return meta.NewClient(s.httpClient)
	}
	return meta.NewClient(&http.Client{
		Timeout:   30 * time.Second,
		Transport: &http.Transport{Proxy: http.ProxyURL(parsed)},
	})
}

func (s *MetaOAuthService) StartDeviceFlow(ctx context.Context, proxyID *int64) (*MetaDeviceStart, error) {
	if s == nil {
		return nil, infraerrors.New(http.StatusServiceUnavailable, "META_OAUTH_NOT_AVAILABLE", "meta oauth service is not available")
	}
	proxyURL, err := s.proxyURL(ctx, proxyID)
	if err != nil {
		return nil, err
	}
	client := s.clientFor(proxyURL)
	code, err := client.StartDeviceFlow(ctx)
	if err != nil {
		return nil, infraerrors.Newf(http.StatusBadGateway, "META_OAUTH_DEVICE_START_FAILED", "%v", err)
	}
	sessionID := meta.NewSessionID()
	expiresIn := code.ExpiresIn
	if expiresIn <= 0 {
		expiresIn = int(metaDeviceSessionTTL / time.Second)
	}
	interval := time.Duration(code.Interval) * time.Second
	if interval <= 0 {
		interval = meta.DefaultPollInterval
	}
	verificationURI := strings.TrimSpace(code.VerificationURI)
	verificationURIComplete := strings.TrimSpace(code.VerificationURIComplete)
	s.sessions.Set(sessionID, &MetaDeviceSession{
		DeviceCode:              code.DeviceCode,
		UserCode:                code.UserCode,
		VerificationURI:         verificationURI,
		VerificationURIComplete: verificationURIComplete,
		Interval:                interval,
		ExpiresAt:               s.now().Add(time.Duration(expiresIn) * time.Second),
		ProxyURL:                proxyURL,
	})
	return &MetaDeviceStart{
		DeviceCodeID:            sessionID,
		UserCode:                code.UserCode,
		VerificationURI:         verificationURI,
		VerificationURIComplete: verificationURIComplete,
		ExpiresIn:               expiresIn,
		Interval:                int(interval / time.Second),
	}, nil
}

func (s *MetaOAuthService) PollDeviceFlow(ctx context.Context, sessionID string) (*MetaDevicePoll, error) {
	if s == nil {
		return nil, infraerrors.New(http.StatusServiceUnavailable, "META_OAUTH_NOT_AVAILABLE", "meta oauth service is not available")
	}
	sessionID = strings.TrimSpace(sessionID)
	if sessionID == "" {
		return nil, infraerrors.New(http.StatusBadRequest, "META_OAUTH_SESSION_REQUIRED", "device_code_id is required")
	}
	session, ok := s.sessions.Get(sessionID)
	if !ok {
		return nil, infraerrors.New(http.StatusBadRequest, "META_OAUTH_SESSION_NOT_FOUND", "device flow session not found or expired")
	}
	if session.Authorized != nil {
		return &MetaDevicePoll{State: meta.PollAuthorized, Interval: session.Interval, MintedKey: session.Authorized, DCAToken: session.DCAToken}, nil
	}
	client := s.clientFor(session.ProxyURL)
	result, err := client.PollDeviceCode(ctx, session.DeviceCode, session.Interval)
	if err != nil {
		return nil, infraerrors.Newf(http.StatusBadGateway, "META_OAUTH_DEVICE_POLL_FAILED", "%v", err)
	}
	if result.Interval > 0 {
		session.Interval = result.Interval
	}
	switch result.State {
	case meta.PollPending, meta.PollSlowDown:
		return &MetaDevicePoll{State: result.State, Interval: session.Interval}, nil
	case meta.PollDenied, meta.PollExpired:
		s.sessions.Delete(sessionID)
		return &MetaDevicePoll{State: result.State, Interval: session.Interval, Message: result.Message}, nil
	case meta.PollAuthorized:
		minted, err := client.MintAPIKey(ctx, result.Token.AccessToken)
		if err != nil {
			return nil, infraerrors.Newf(http.StatusBadGateway, "META_OAUTH_MINT_FAILED", "%v", err)
		}
		session.Authorized = minted
		session.DCAToken = result.Token.AccessToken
		session.DeviceCode = ""
		return &MetaDevicePoll{State: meta.PollAuthorized, Interval: session.Interval, MintedKey: minted, DCAToken: session.DCAToken}, nil
	default:
		return nil, infraerrors.Newf(http.StatusBadGateway, "META_OAUTH_DEVICE_POLL_FAILED", "unexpected poll state %q", result.State)
	}
}

func (s *MetaOAuthService) MintAPIKey(ctx context.Context, proxyURL, dcaToken string) (*meta.APIKey, error) {
	if s == nil {
		return nil, infraerrors.New(http.StatusServiceUnavailable, "META_OAUTH_NOT_AVAILABLE", "meta oauth service is not available")
	}
	minted, err := s.clientFor(proxyURL).MintAPIKey(ctx, dcaToken)
	if err != nil {
		return nil, infraerrors.Newf(http.StatusBadGateway, "META_OAUTH_MINT_FAILED", "%v", err)
	}
	return minted, nil
}

func (s *MetaOAuthService) BuildDeviceFlowCredentials(minted *meta.APIKey, dcaToken string) map[string]any {
	return buildMetaCredentials(minted, dcaToken)
}

func (s *MetaOAuthService) BuildAPIKeyCredentials(apiKey, baseURL string) map[string]any {
	return buildMetaCredentials(&meta.APIKey{APIKey: apiKey, BaseURL: baseURL}, "")
}

func (s *MetaOAuthService) TakeAuthorizedKey(sessionID string) (*meta.APIKey, bool) {
	session, ok := s.sessions.Get(strings.TrimSpace(sessionID))
	if !ok || session.Authorized == nil {
		return nil, false
	}
	return session.Authorized, true
}

func (s *MetaOAuthService) ClaimAccountCreation(sessionID string) bool {
	session, ok := s.sessions.Get(strings.TrimSpace(sessionID))
	if !ok || session.Authorized == nil || session.AccountCreated {
		return false
	}
	session.AccountCreated = true
	return true
}

func (s *MetaOAuthService) proxyURL(ctx context.Context, proxyID *int64) (string, error) {
	if proxyID == nil {
		return "", nil
	}
	if s.proxyRepo == nil {
		return "", infraerrors.New(http.StatusBadRequest, "META_OAUTH_PROXY_NOT_AVAILABLE", "proxy repository is not available")
	}
	proxy, err := s.proxyRepo.GetByID(ctx, *proxyID)
	if err != nil {
		if errors.Is(err, ErrProxyNotFound) {
			return "", infraerrors.New(http.StatusBadRequest, "META_OAUTH_PROXY_NOT_FOUND", "configured proxy was not found")
		}
		return "", infraerrors.New(http.StatusServiceUnavailable, "META_OAUTH_PROXY_LOOKUP_FAILED", "proxy lookup is temporarily unavailable")
	}
	if proxy == nil {
		return "", infraerrors.New(http.StatusBadRequest, "META_OAUTH_PROXY_NOT_FOUND", "configured proxy was not found")
	}
	return proxy.URL(), nil
}

func buildMetaCredentials(minted *meta.APIKey, dcaToken string) map[string]any {
	credentials := map[string]any{
		"base_url": meta.DefaultBaseURL,
	}
	apiKey := ""
	if minted != nil {
		apiKey = strings.TrimSpace(minted.APIKey)
		if baseURL := strings.TrimSpace(minted.BaseURL); baseURL != "" {
			credentials["base_url"] = strings.TrimRight(baseURL, "/")
		}
		if email := strings.TrimSpace(minted.UserEmail); email != "" {
			credentials["email"] = email
		}
		if name := strings.TrimSpace(minted.UserFullName); name != "" {
			credentials["name"] = name
		}
		if tier := strings.TrimSpace(minted.SubsTierName); tier != "" {
			credentials["subs_tier_name"] = tier
		}
		if tierID := strings.TrimSpace(minted.SubsTierID); tierID != "" {
			credentials["subs_tier_id"] = tierID
		}
		credentials["is_subs_active"] = minted.IsSubsActive
		credentials["has_payment_method"] = minted.HasPaymentMethod
	}
	if apiKey != "" {
		credentials["api_key"] = apiKey
	}
	if token := strings.TrimSpace(dcaToken); token != "" {
		credentials["dca_token"] = token
		credentials["auth_kind"] = MetaAuthKindOAuth
	} else {
		credentials["auth_kind"] = MetaAuthKindAPIKey
	}
	if minted != nil && minted.APIKey != "" {
		credentials["last_refresh"] = time.Now().UTC().Format(time.RFC3339)
	}
	return credentials
}
