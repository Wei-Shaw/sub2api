//go:build unit

package service

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/pkg/meta"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

func metaTestAccount(credentials map[string]any, accountType string) *Account {
	return &Account{
		ID:          5001,
		Name:        "meta-test",
		Platform:    PlatformMeta,
		Type:        accountType,
		Credentials: credentials,
	}
}

type metaAccountRepoStub struct {
	mockAccountRepoForGemini
	account     *Account
	updateCalls int
}

func (m *metaAccountRepoStub) GetByID(ctx context.Context, id int64) (*Account, error) {
	return m.account, nil
}

func (m *metaAccountRepoStub) Update(ctx context.Context, account *Account) error {
	m.updateCalls++
	m.account = account
	return nil
}

type metaUpstreamStub struct {
	HTTPUpstream
	statusCode int
	body       string
	lastReq    *http.Request
	lastBody   []byte
	calls      int
}

func (u *metaUpstreamStub) Do(req *http.Request, proxyURL string, accountID int64, concurrency int) (*http.Response, error) {
	u.calls++
	u.lastReq = req
	if req.Body != nil {
		raw, _ := io.ReadAll(req.Body)
		u.lastBody = raw
	}
	status := u.statusCode
	if status == 0 {
		status = http.StatusOK
	}
	return &http.Response{
		StatusCode: status,
		Header:     http.Header{"Content-Type": []string{"application/json"}},
		Body:       io.NopCloser(strings.NewReader(u.body)),
	}, nil
}

func TestMetaPlatformProfileRoutesToResponses(t *testing.T) {
	profile := LookupProviderProfile(PlatformMeta)
	require.NotNil(t, profile)
	require.Equal(t, ProviderRoutingByModel, profile.Routing)
	require.Equal(t, DefaultMetaTestModel, profile.DefaultTestModel)
	require.Equal(t, DefaultMetaBaseURL, profile.DefaultBaseURL(AccountModePayG, APIProtocolResponses))
	require.Equal(t, DefaultMetaBaseURL, profile.DefaultBaseURL(AccountModePayG, APIProtocolChatCompletions))

	account := metaTestAccount(map[string]any{"api_key": "sk-muse", "base_url": DefaultMetaBaseURL}, AccountTypeAPIKey)
	require.True(t, account.IsMultiProtocolAPIKey())
	require.True(t, account.routesByModel())
	require.Equal(t, DefaultMetaBaseURL, account.GetOpenAIBaseURL())
	require.True(t, account.SupportsNativeCNResponses())

	require.Equal(t, APIProtocolResponses, resolveUpstreamProtocol(account, APIProtocolResponses, "muse-spark-1.3", nil))
	require.Equal(t, APIProtocolResponses, resolveUpstreamProtocol(account, APIProtocolChatCompletions, "muse-spark-1.3", nil))
	require.Equal(t, APIProtocolResponses, resolveUpstreamProtocol(account, APIProtocolAnthropic, "muse-wonder-1.0", nil))
}

func TestMetaAccountCredentialHelpers(t *testing.T) {
	t.Run("api key account", func(t *testing.T) {
		account := metaTestAccount(map[string]any{"api_key": "sk-muse", "base_url": "https://relay.example/v1"}, AccountTypeAPIKey)
		require.True(t, account.IsMeta())
		require.False(t, account.IsMetaOAuth())
		require.Equal(t, "sk-muse", account.GetMetaAPIKey())
		require.Equal(t, "sk-muse", account.GetOpenAIProtocolAPIKey())
		require.Equal(t, "https://relay.example/v1", account.GetMetaBaseURL())
		require.Empty(t, account.GetMetaDCAToken())
		require.False(t, account.NeedsMetaKeyMint())
	})

	t.Run("oauth account uses the minted key and keeps the dca token", func(t *testing.T) {
		account := metaTestAccount(map[string]any{
			"api_key":   "sk-muse",
			"dca_token": "dca:abc",
			"auth_kind": MetaAuthKindOAuth,
		}, AccountTypeOAuth)
		require.True(t, account.IsMetaOAuth())
		require.Equal(t, "sk-muse", account.GetMetaAPIKey())
		require.Equal(t, "sk-muse", account.GetOpenAIProtocolAPIKey())
		require.Equal(t, "dca:abc", account.GetMetaDCAToken())
		require.Equal(t, DefaultMetaBaseURL, account.GetMetaBaseURL())
		require.False(t, account.NeedsMetaKeyMint())
	})

	t.Run("oauth account without a minted key needs a mint", func(t *testing.T) {
		account := metaTestAccount(map[string]any{"dca_token": "dca:abc"}, AccountTypeOAuth)
		require.Empty(t, account.GetMetaAPIKey())
		require.True(t, account.NeedsMetaKeyMint())
		require.Equal(t, "dca:abc", account.GetMetaDCAToken())
	})

	t.Run("legacy dca access token is recognised", func(t *testing.T) {
		account := metaTestAccount(map[string]any{"access_token": "dca:legacy"}, AccountTypeOAuth)
		require.Equal(t, "dca:legacy", account.GetMetaDCAToken())
		require.Empty(t, account.GetMetaAPIKey())
	})

	t.Run("dca value in api_key is never used as an inference key", func(t *testing.T) {
		account := metaTestAccount(map[string]any{"api_key": "dca:oops"}, AccountTypeOAuth)
		require.Empty(t, account.GetMetaAPIKey())
		require.Empty(t, account.GetOpenAIProtocolAPIKey())
	})

	t.Run("invalid base url falls back to the official host", func(t *testing.T) {
		account := metaTestAccount(map[string]any{"base_url": "not a url"}, AccountTypeAPIKey)
		require.Equal(t, DefaultMetaBaseURL, account.GetMetaBaseURL())
	})
}

func TestHandleUpstreamErrorMetaThrottleUsesResetInstant(t *testing.T) {
	resetAt := time.Now().Add(42 * time.Minute).Truncate(time.Second)
	body := []byte(`{"error":{"code":"rate_limit_exceeded","message":"subscription quota exhausted","resets_at":` + strconv.FormatInt(resetAt.Unix(), 10) + `}}`)

	repo := &rateLimitAccountRepoStub{}
	service := NewRateLimitService(repo, nil, &config.Config{}, nil, nil)
	account := metaTestAccount(map[string]any{"api_key": "sk-muse"}, AccountTypeAPIKey)

	disable := service.HandleUpstreamError(context.Background(), account, http.StatusTooManyRequests, http.Header{}, body)

	require.False(t, disable)
	require.Equal(t, 1, repo.rateLimitedCalls)
	require.Equal(t, account.ID, repo.lastRateLimitedID)
	require.WithinDuration(t, resetAt, repo.lastRateLimitedAt, 2*time.Second)
	require.Equal(t, 0, repo.setErrorCalls)
}

func TestHandleUpstreamErrorMetaNotFoundWithoutResetGetsShortCooldown(t *testing.T) {
	repo := &rateLimitAccountRepoStub{}
	service := NewRateLimitService(repo, nil, &config.Config{}, nil, nil)
	account := metaTestAccount(map[string]any{"api_key": "sk-muse"}, AccountTypeAPIKey)

	disable := service.HandleUpstreamError(context.Background(), account, http.StatusNotFound, http.Header{}, []byte(`{"error":{"message":"model unavailable"}}`))

	require.False(t, disable)
	require.Equal(t, 1, repo.tempCalls)
	require.Equal(t, account.ID, repo.lastTempID)
	require.Contains(t, repo.lastTempReason, "meta_rate_limited")
	require.Equal(t, 0, repo.rateLimitedCalls)
}

func TestHandleUpstreamErrorMetaWithoutResetStaysOnDefaultPath(t *testing.T) {
	repo := &rateLimitAccountRepoStub{}
	service := NewRateLimitService(repo, nil, &config.Config{}, nil, nil)
	account := metaTestAccount(map[string]any{"api_key": "sk-muse"}, AccountTypeAPIKey)

	service.HandleUpstreamError(context.Background(), account, http.StatusTooManyRequests, http.Header{}, []byte(`{"error":{"message":"too many requests"}}`))

	require.Equal(t, 0, repo.tempCalls)
	require.Equal(t, 1, repo.rateLimitedCalls)
}

func TestMetaTokenProviderMintsAndPersists(t *testing.T) {
	repo := &metaAccountRepoStub{account: metaTestAccount(map[string]any{"dca_token": "dca:abc"}, AccountTypeOAuth)}
	upstream := &metaUpstreamStub{body: `{"api_key":"sk-minted","base_url":"https://api.meta.ai/v1","user_email":"muse@example.com","subs_tier_name":"Pro","is_subs_active":true}`}
	provider := NewMetaTokenProvider(repo, upstream)

	key, err := provider.GetAPIKey(context.Background(), repo.account)
	require.NoError(t, err)
	require.Equal(t, "sk-minted", key)
	require.Equal(t, 1, repo.updateCalls)
	require.Equal(t, "sk-minted", repo.account.GetMetaAPIKey())
	require.Equal(t, "dca:abc", repo.account.GetMetaDCAToken())
	require.Equal(t, "muse@example.com", repo.account.GetCredential("email"))
	require.Equal(t, "Bearer dca:abc", upstream.lastReq.Header.Get("Authorization"))
	require.Equal(t, "dca:abc", gjson.GetBytes(upstream.lastBody, "dca_token").String())
	require.Equal(t, "https://api.meta.ai/muse-code/key", upstream.lastReq.URL.String())
	require.NotNil(t, repo.account.GetCredential("last_refresh"))

	key, err = provider.GetAPIKey(context.Background(), repo.account)
	require.NoError(t, err)
	require.Equal(t, "sk-minted", key)
	require.Equal(t, 1, upstream.calls)
}

func TestMetaTokenProviderErrorsAndInvalidate(t *testing.T) {
	t.Run("no credential at all", func(t *testing.T) {
		provider := NewMetaTokenProvider(&metaAccountRepoStub{}, nil)
		_, err := provider.GetAPIKey(context.Background(), metaTestAccount(map[string]any{}, AccountTypeAPIKey))
		require.ErrorContains(t, err, "api_key or dca_token is required")
	})

	t.Run("mint failure", func(t *testing.T) {
		repo := &metaAccountRepoStub{account: metaTestAccount(map[string]any{"dca_token": "dca:abc"}, AccountTypeOAuth)}
		upstream := &metaUpstreamStub{statusCode: http.StatusUnauthorized, body: `{"error":"invalid_token"}`}
		provider := NewMetaTokenProvider(repo, upstream)
		_, err := provider.GetAPIKey(context.Background(), repo.account)
		require.ErrorContains(t, err, "HTTP 401")
		require.Equal(t, 0, repo.updateCalls)
	})

	t.Run("invalidate drops only the minted key", func(t *testing.T) {
		repo := &metaAccountRepoStub{account: metaTestAccount(map[string]any{
			"api_key":   "sk-muse",
			"dca_token": "dca:abc",
			"email":     "muse@example.com",
		}, AccountTypeOAuth)}
		provider := NewMetaTokenProvider(repo, &metaUpstreamStub{})
		require.NoError(t, provider.InvalidateAPIKey(context.Background(), repo.account))
		require.Equal(t, 1, repo.updateCalls)
		require.Empty(t, repo.account.GetMetaAPIKey())
		require.Equal(t, "dca:abc", repo.account.GetMetaDCAToken())
		require.Equal(t, "muse@example.com", repo.account.GetCredential("email"))
	})

	t.Run("invalidate is a no-op for pasted key accounts", func(t *testing.T) {
		repo := &metaAccountRepoStub{account: metaTestAccount(map[string]any{"api_key": "sk-muse"}, AccountTypeAPIKey)}
		provider := NewMetaTokenProvider(repo, &metaUpstreamStub{})
		require.NoError(t, provider.InvalidateAPIKey(context.Background(), repo.account))
		require.Equal(t, 0, repo.updateCalls)
		require.Equal(t, "sk-muse", repo.account.GetMetaAPIKey())
	})
}

type metaCredentialDoer struct {
	startBody   string
	tokenBodies []string
	mintBody    string
	mintStatus  int
	mintCalls   int
	lastToken   url.Values
}

func (d *metaCredentialDoer) Do(req *http.Request) (*http.Response, error) {
	switch {
	case strings.Contains(req.URL.Path, "authorization"):
		return jsonResponse(http.StatusOK, d.startBody), nil
	case strings.Contains(req.URL.Path, "token"):
		raw, _ := io.ReadAll(req.Body)
		form, err := url.ParseQuery(string(raw))
		if err == nil {
			d.lastToken = form
		}
		if len(d.tokenBodies) == 0 {
			return jsonResponse(http.StatusBadRequest, `{"error":"authorization_pending"}`), nil
		}
		body := d.tokenBodies[0]
		d.tokenBodies = d.tokenBodies[1:]
		status := http.StatusBadRequest
		if strings.Contains(body, "access_token") {
			status = http.StatusOK
		}
		return jsonResponse(status, body), nil
	case strings.Contains(req.URL.Path, "muse-code"):
		d.mintCalls++
		status := d.mintStatus
		if status == 0 {
			status = http.StatusOK
		}
		return jsonResponse(status, d.mintBody), nil
	default:
		return jsonResponse(http.StatusNotFound, `{"error":"unexpected path"}`), nil
	}
}

func jsonResponse(status int, body string) *http.Response {
	return &http.Response{
		StatusCode: status,
		Header:     http.Header{"Content-Type": []string{"application/json"}},
		Body:       io.NopCloser(bytes.NewReader([]byte(body))),
	}
}

func TestMetaOAuthServiceDeviceFlowLifecycle(t *testing.T) {
	doer := &metaCredentialDoer{
		startBody: `{"device_code":"dev-1","user_code":"ABCD-EFGH","verification_uri":"https://auth.meta.com/device","verification_uri_complete":"https://auth.meta.com/device?user_code=ABCD-EFGH","expires_in":900,"interval":5}`,
		tokenBodies: []string{
			`{"error":"authorization_pending"}`,
			`{"error":"slow_down"}`,
			`{"access_token":"dca:abc","token_type":"Bearer","expires_in":3600}`,
		},
		mintBody: `{"api_key":"sk-minted","base_url":"https://api.meta.ai/v1","user_email":"muse@example.com","subs_tier_name":"Pro","is_subs_active":true}`,
	}
	svc := NewMetaOAuthService(nil).WithTransport(doer)

	start, err := svc.StartDeviceFlow(context.Background(), nil)
	require.NoError(t, err)
	require.NotEmpty(t, start.DeviceCodeID)
	require.Equal(t, "ABCD-EFGH", start.UserCode)
	require.Equal(t, 900, start.ExpiresIn)
	require.Equal(t, 5, start.Interval)

	poll, err := svc.PollDeviceFlow(context.Background(), start.DeviceCodeID)
	require.NoError(t, err)
	require.Equal(t, meta.PollPending, poll.State)

	poll, err = svc.PollDeviceFlow(context.Background(), start.DeviceCodeID)
	require.NoError(t, err)
	require.Equal(t, meta.PollSlowDown, poll.State)
	require.Equal(t, 10, int(poll.Interval/time.Second))

	poll, err = svc.PollDeviceFlow(context.Background(), start.DeviceCodeID)
	require.NoError(t, err)
	require.Equal(t, meta.PollAuthorized, poll.State)
	require.Equal(t, "sk-minted", poll.MintedKey.APIKey)
	require.Equal(t, "dca:abc", poll.DCAToken)
	require.Equal(t, 1, doer.mintCalls)

	poll, err = svc.PollDeviceFlow(context.Background(), start.DeviceCodeID)
	require.NoError(t, err)
	require.Equal(t, meta.PollAuthorized, poll.State)
	require.Equal(t, "sk-minted", poll.MintedKey.APIKey)
	require.Equal(t, 1, doer.mintCalls)

	key, ok := svc.TakeAuthorizedKey(start.DeviceCodeID)
	require.True(t, ok)
	require.Equal(t, "sk-minted", key.APIKey)

	credentials := svc.BuildDeviceFlowCredentials(poll.MintedKey, poll.DCAToken)
	require.Equal(t, "sk-minted", credentials["api_key"])
	require.Equal(t, "dca:abc", credentials["dca_token"])
	require.Equal(t, MetaAuthKindOAuth, credentials["auth_kind"])
	require.Equal(t, "https://api.meta.ai/v1", credentials["base_url"])
	require.Equal(t, "muse@example.com", credentials["email"])
	require.Equal(t, true, credentials["is_subs_active"])

	apiKeyCredentials := svc.BuildAPIKeyCredentials("sk-pasted", "https://relay.example/v1")
	require.Equal(t, MetaAuthKindAPIKey, apiKeyCredentials["auth_kind"])
	require.Equal(t, "sk-pasted", apiKeyCredentials["api_key"])
	require.Equal(t, "https://relay.example/v1", apiKeyCredentials["base_url"])
	require.NotContains(t, apiKeyCredentials, "dca_token")
}

func TestMetaOAuthServiceDeviceFlowFailures(t *testing.T) {
	t.Run("denied", func(t *testing.T) {
		doer := &metaCredentialDoer{
			startBody:   `{"device_code":"dev-2","user_code":"CODE","expires_in":600,"interval":1}`,
			tokenBodies: []string{`{"error":"access_denied","error_description":"denied by user"}`},
		}
		svc := NewMetaOAuthService(nil).WithTransport(doer)
		start, err := svc.StartDeviceFlow(context.Background(), nil)
		require.NoError(t, err)
		poll, err := svc.PollDeviceFlow(context.Background(), start.DeviceCodeID)
		require.NoError(t, err)
		require.Equal(t, meta.PollDenied, poll.State)
		require.Equal(t, "denied by user", poll.Message)
		_, err = svc.PollDeviceFlow(context.Background(), start.DeviceCodeID)
		require.ErrorContains(t, err, "session not found")
	})

	t.Run("mint failure surfaces", func(t *testing.T) {
		doer := &metaCredentialDoer{
			startBody:   `{"device_code":"dev-3","user_code":"CODE","expires_in":600,"interval":1}`,
			tokenBodies: []string{`{"access_token":"dca:abc","token_type":"Bearer"}`},
			mintStatus:  http.StatusUnauthorized,
			mintBody:    `{"error":"invalid_token"}`,
		}
		svc := NewMetaOAuthService(nil).WithTransport(doer)
		start, err := svc.StartDeviceFlow(context.Background(), nil)
		require.NoError(t, err)
		_, err = svc.PollDeviceFlow(context.Background(), start.DeviceCodeID)
		require.ErrorContains(t, err, "META_OAUTH_MINT_FAILED")
	})

	t.Run("unknown session", func(t *testing.T) {
		svc := NewMetaOAuthService(nil)
		_, err := svc.PollDeviceFlow(context.Background(), "missing")
		require.ErrorContains(t, err, "session not found")
		_, err = svc.PollDeviceFlow(context.Background(), " ")
		require.ErrorContains(t, err, "device_code_id is required")
	})

	t.Run("expired session", func(t *testing.T) {
		svc := NewMetaOAuthService(nil)
		svc.sessions.Set("expired", &MetaDeviceSession{DeviceCode: "dev", ExpiresAt: time.Now().Add(-time.Minute)})
		_, err := svc.PollDeviceFlow(context.Background(), "expired")
		require.ErrorContains(t, err, "session not found")
	})
}

func TestMetaDeviceSessionStoreStaysBounded(t *testing.T) {
	store := NewMetaDeviceSessionStore()
	base := time.Now().Add(time.Minute)
	for i := 0; i < metaDeviceSessionMaxKeep; i++ {
		store.Set("session-"+strconv.Itoa(i), &MetaDeviceSession{
			DeviceCode: "dev",
			ExpiresAt:  base.Add(time.Duration(i) * time.Second),
		})
	}
	require.Len(t, store.sessions, metaDeviceSessionMaxKeep)

	store.Set("session-newest", &MetaDeviceSession{DeviceCode: "dev", ExpiresAt: base.Add(time.Hour)})
	require.Len(t, store.sessions, metaDeviceSessionMaxKeep)

	_, ok := store.Get("session-0")
	require.False(t, ok)
	newest, ok := store.Get("session-newest")
	require.True(t, ok)
	require.Equal(t, "dev", newest.DeviceCode)
}

func TestMetaOAuthServiceReleaseAccountCreation(t *testing.T) {
	svc := NewMetaOAuthService(nil)
	svc.sessions.Set("claimable", &MetaDeviceSession{
		DeviceCode:     "dev",
		ExpiresAt:      time.Now().Add(time.Minute),
		Authorized:     &meta.APIKey{APIKey: "sk-claim"},
		AccountCreated: false,
	})
	require.True(t, svc.ClaimAccountCreation("claimable"))
	require.False(t, svc.ClaimAccountCreation("claimable"))
	svc.ReleaseAccountCreation("claimable")
	require.True(t, svc.ClaimAccountCreation("claimable"))
}
