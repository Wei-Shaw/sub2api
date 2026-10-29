package service

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// fakeOAuthStore 以内存映射实现 OAuthProviderStore，行为对齐 redis 语义
// （GetDel 原子取删、miss 返回错误），使 service 测试摆脱 redis 依赖。
type fakeOAuthStore struct {
	mu   sync.Mutex
	data map[string][]byte
}

var errFakeNotFound = errors.New("fake oauth store: entry not found")

func newFakeOAuthStore() *fakeOAuthStore {
	return &fakeOAuthStore{data: make(map[string][]byte)}
}

func (f *fakeOAuthStore) save(key string, payload []byte) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.data[key] = payload
}

func (f *fakeOAuthStore) get(key string) ([]byte, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	v, ok := f.data[key]
	if !ok {
		return nil, errFakeNotFound
	}
	return v, nil
}

func (f *fakeOAuthStore) getDel(key string) ([]byte, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	v, ok := f.data[key]
	if !ok {
		return nil, errFakeNotFound
	}
	delete(f.data, key)
	return v, nil
}

func (f *fakeOAuthStore) del(keys ...string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	for _, k := range keys {
		delete(f.data, k)
	}
}

// fakeTransactionDocument 复刻存储层的事务文档格式：State 在域结构体的
// JSON 投影之外，随文档一并持久化。
type fakeTransactionDocument struct {
	OAuthAuthorizationTransaction
	State string `json:"state"`
}

func (f *fakeOAuthStore) SaveTransaction(ctx context.Context, id string, tx OAuthAuthorizationTransaction, ttl time.Duration) error {
	payload, err := json.Marshal(fakeTransactionDocument{OAuthAuthorizationTransaction: tx, State: tx.State})
	if err != nil {
		return err
	}
	f.save("oauth:transaction:"+id, payload)
	return nil
}

func (f *fakeOAuthStore) GetTransaction(ctx context.Context, id string) (OAuthAuthorizationTransaction, error) {
	raw, err := f.get("oauth:transaction:" + id)
	if err != nil {
		return OAuthAuthorizationTransaction{}, err
	}
	var doc fakeTransactionDocument
	if err := json.Unmarshal(raw, &doc); err != nil {
		return OAuthAuthorizationTransaction{}, err
	}
	doc.OAuthAuthorizationTransaction.State = doc.State
	return doc.OAuthAuthorizationTransaction, nil
}

func (f *fakeOAuthStore) ConsumeTransaction(ctx context.Context, id string) (OAuthAuthorizationTransaction, error) {
	raw, err := f.getDel("oauth:transaction:" + id)
	if err != nil {
		return OAuthAuthorizationTransaction{}, err
	}
	var doc fakeTransactionDocument
	if err := json.Unmarshal(raw, &doc); err != nil {
		return OAuthAuthorizationTransaction{}, err
	}
	doc.OAuthAuthorizationTransaction.State = doc.State
	return doc.OAuthAuthorizationTransaction, nil
}

func (f *fakeOAuthStore) SaveAuthorizationCode(ctx context.Context, code OAuthStoredAuthorizationCode, ttl time.Duration) error {
	payload, err := json.Marshal(code)
	if err != nil {
		return err
	}
	f.save("oauth:code:"+code.CodeHash, payload)
	return nil
}

func (f *fakeOAuthStore) ConsumeAuthorizationCode(ctx context.Context, codeHash string) (OAuthStoredAuthorizationCode, error) {
	raw, err := f.getDel("oauth:code:" + codeHash)
	if err != nil {
		return OAuthStoredAuthorizationCode{}, err
	}
	var stored OAuthStoredAuthorizationCode
	if err := json.Unmarshal(raw, &stored); err != nil {
		return OAuthStoredAuthorizationCode{}, err
	}
	return stored, nil
}

func (f *fakeOAuthStore) ConsumeRefreshToken(ctx context.Context, tokenHash string) (OAuthStoredToken, error) {
	raw, err := f.getDel("oauth:refresh:" + tokenHash)
	if err != nil {
		return OAuthStoredToken{}, err
	}
	var stored OAuthStoredToken
	if err := json.Unmarshal(raw, &stored); err != nil {
		return OAuthStoredToken{}, err
	}
	return stored, nil
}

func (f *fakeOAuthStore) SaveTokenPair(ctx context.Context, accessHash string, access OAuthStoredToken, accessTTL time.Duration, refreshHash string, refresh OAuthStoredToken, refreshTTL time.Duration) error {
	accessPayload, err := json.Marshal(access)
	if err != nil {
		return err
	}
	refreshPayload, err := json.Marshal(refresh)
	if err != nil {
		return err
	}
	f.save("oauth:access:"+accessHash, accessPayload)
	f.save("oauth:refresh:"+refreshHash, refreshPayload)
	return nil
}

func (f *fakeOAuthStore) GetAccessToken(ctx context.Context, tokenHash string) (OAuthStoredToken, error) {
	raw, err := f.get("oauth:access:" + tokenHash)
	if err != nil {
		return OAuthStoredToken{}, err
	}
	var stored OAuthStoredToken
	if err := json.Unmarshal(raw, &stored); err != nil {
		return OAuthStoredToken{}, err
	}
	return stored, nil
}

func (f *fakeOAuthStore) DeleteTokens(ctx context.Context, accessHash, refreshHash string) error {
	f.del("oauth:access:"+accessHash, "oauth:refresh:"+refreshHash)
	return nil
}

func (f *fakeOAuthStore) RevokeUserTokens(ctx context.Context, clientID string, userID int64) error {
	for _, prefix := range []string{"oauth:access:", "oauth:refresh:"} {
		f.mu.Lock()
		var doomed []string
		for key, raw := range f.data {
			if !strings.HasPrefix(key, prefix) {
				continue
			}
			var token OAuthStoredToken
			if json.Unmarshal(raw, &token) == nil && token.UserID == userID && token.ClientID == clientID {
				doomed = append(doomed, key)
			}
		}
		for _, key := range doomed {
			delete(f.data, key)
		}
		f.mu.Unlock()
	}
	return nil
}

// Regression: the authorization transaction must survive its store round
// trip with the PKCE challenge intact — a dropped challenge voids every
// token exchange (caught live, now pinned here).
func TestOAuthAuthorizationTransactionRoundTripKeepsPKCE(t *testing.T) {
	svc := NewOAuthProviderService(nil, newFakeOAuthStore())
	original := OAuthAuthorizationTransaction{
		ClientID:            "dsh-sub2api-sync",
		ClientName:          "DSH Sub2API Sync",
		RedirectURI:         "http://127.0.0.1:19387/plugins/dsh-sub2api-sync/oauth/callback",
		Scope:               []string{"openid", "profile"},
		State:               "st-1",
		CodeChallenge:       "challenge-value",
		CodeChallengeMethod: "S256",
	}
	created, err := svc.CreateAuthorizationTransaction(context.Background(), original)
	require.NoError(t, err)
	loaded, err := svc.GetAuthorizationTransaction(context.Background(), created.ID)
	require.NoError(t, err)
	require.Equal(t, "challenge-value", loaded.CodeChallenge)
	require.Equal(t, "S256", loaded.CodeChallengeMethod)
	require.Equal(t, "st-1", loaded.State)
	require.Equal(t, original.RedirectURI, loaded.RedirectURI)
	require.ElementsMatch(t, original.Scope, loaded.Scope)
}

// 事务是一次性凭据：consume 之后必须消失（GetDel 语义）。
func TestOAuthAuthorizationTransactionIsSingleUse(t *testing.T) {
	svc := NewOAuthProviderService(nil, newFakeOAuthStore())
	created, err := svc.CreateAuthorizationTransaction(context.Background(), OAuthAuthorizationTransaction{
		ClientID: "dsh-sub2api-sync",
		State:    "st-2",
	})
	require.NoError(t, err)
	_, err = svc.consumeAuthorizationTransaction(context.Background(), created.ID)
	require.NoError(t, err)
	_, err = svc.consumeAuthorizationTransaction(context.Background(), created.ID)
	require.ErrorIs(t, err, ErrOAuthInvalidGrant)
}

func TestOAuthRedirectURIAllowsOnlyLoopbackDynamicPorts(t *testing.T) {
	client := OAuthClient{
		ClientType:   "public",
		RedirectURIs: []string{"http://127.0.0.1/oauth/callback", "http://[::1]/oauth/callback"},
	}
	require.True(t, validRedirectURI(client, "http://127.0.0.1:19387/oauth/callback"))
	require.True(t, validRedirectURI(client, "http://[::1]:41231/oauth/callback"))
	require.False(t, validRedirectURI(client, "http://0.0.0.0:19387/oauth/callback"))
	require.False(t, validRedirectURI(client, "http://127.0.0.1:19387/other"))
	require.False(t, validRedirectURI(client, "https://127.0.0.1:19387/oauth/callback"))
}

// The DSH harness callback rides the plugin route path; the loopback
// port-variable exception must honor the client's *registered* paths.
func TestOAuthRedirectURIPinsLoopbackPathToRegistration(t *testing.T) {
	client := OAuthClient{
		ClientType: "public",
		RedirectURIs: []string{
			"http://127.0.0.1/plugins/dsh-sub2api-sync/oauth/callback",
			"http://[::1]/plugins/dsh-sub2api-sync/oauth/callback",
		},
	}
	require.True(t, validRedirectURI(client, "http://127.0.0.1:19387/plugins/dsh-sub2api-sync/oauth/callback"))
	require.True(t, validRedirectURI(client, "http://[::1]:5123/plugins/dsh-sub2api-sync/oauth/callback"))
	// Unregistered path — even loopback — is refused.
	require.False(t, validRedirectURI(client, "http://127.0.0.1:19387/oauth/callback"))
	// Exact match still works without a port.
	require.True(t, validRedirectURI(client, "http://127.0.0.1/plugins/dsh-sub2api-sync/oauth/callback"))
	// Confidential clients get no loopback exception at all.
	confidential := OAuthClient{
		ClientType:   "confidential",
		RedirectURIs: []string{"http://127.0.0.1/plugins/dsh-sub2api-sync/oauth/callback"},
	}
	require.False(t, validRedirectURI(confidential, "http://127.0.0.1:19387/plugins/dsh-sub2api-sync/oauth/callback"))
}

func TestOAuthPKCEVerifierUsesS256(t *testing.T) {
	verifier := "dsh-test-code-verifier"
	sum := sha256.Sum256([]byte(verifier))
	challenge := base64.RawURLEncoding.EncodeToString(sum[:])
	require.True(t, verifyPKCE(verifier, challenge, "S256"))
	require.False(t, verifyPKCE("wrong", challenge, "S256"))
	require.False(t, verifyPKCE(verifier, challenge, "plain"))
}

func TestOAuthScopeStringDeduplicatesValues(t *testing.T) {
	require.Equal(t, "openid profile groups:read", scopeString([]string{"openid", "profile", "openid", "groups:read"}))
}

func TestOAuthClientListItemMarshalsSnakeCase(t *testing.T) {
	// 管理端前端按蛇形命名读取字段；缺 json tag 会序列化成帕斯卡命名，
	// 导致 redirect_uris.join 抛错、整张管理卡片渲染失败（回归保护）。
	item := OAuthClientListItem{
		OAuthClient: OAuthClient{
			ClientID:      "dsh-sub2api-sync",
			ClientType:    "public",
			Name:          "DSH Sub2API Sync",
			RedirectURIs:  []string{"http://127.0.0.1/oauth/callback"},
			AllowedScopes: []string{"openid", "profile"},
			PKCERequired:  true,
			Status:        "active",
		},
	}
	payload, err := json.Marshal(item)
	require.NoError(t, err)

	var decoded map[string]any
	require.NoError(t, json.Unmarshal(payload, &decoded))
	for _, key := range []string{"client_id", "client_type", "name", "description", "logo_url", "redirect_uris", "allowed_scopes", "pkce_required", "status", "created_at", "updated_at"} {
		require.Contains(t, decoded, key)
	}
	require.NotContains(t, decoded, "ClientID")
	require.NotContains(t, decoded, "RedirectURIs")
}
