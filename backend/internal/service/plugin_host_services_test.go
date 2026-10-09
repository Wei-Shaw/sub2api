package service

import (
	"context"
	"math"
	"net/http"
	"sort"
	"strings"
	"sync"
	"testing"
	"time"

	pluginv1 "github.com/Wei-Shaw/sub2api/pkg/pluginapi/v1"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// fakePluginKVStore 是内存版 PluginKVStore，按 (pluginKey, namespace, key) 三元组隔离，
// 用于确定性地验证宿主服务端的校验与命名空间隔离逻辑。
type fakePluginKVStore struct {
	mu     sync.Mutex
	values map[string][]byte
	ttls   map[string]time.Duration
}

func newFakePluginKVStore() *fakePluginKVStore {
	return &fakePluginKVStore{values: map[string][]byte{}, ttls: map[string]time.Duration{}}
}

func (f *fakePluginKVStore) compose(pluginKey, namespace, key string) string {
	return pluginKey + "\x00" + namespace + "\x00" + key
}

func (f *fakePluginKVStore) Get(_ context.Context, pluginKey, namespace, key string) ([]byte, bool, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	value, ok := f.values[f.compose(pluginKey, namespace, key)]
	if !ok {
		return nil, false, nil
	}
	return append([]byte(nil), value...), true, nil
}

func (f *fakePluginKVStore) Set(_ context.Context, pluginKey, namespace, key string, value []byte, ttl time.Duration) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	composed := f.compose(pluginKey, namespace, key)
	f.values[composed] = append([]byte(nil), value...)
	f.ttls[composed] = ttl
	return nil
}

func (f *fakePluginKVStore) Delete(_ context.Context, pluginKey, namespace, key string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	composed := f.compose(pluginKey, namespace, key)
	delete(f.values, composed)
	delete(f.ttls, composed)
	return nil
}

func (f *fakePluginKVStore) List(_ context.Context, pluginKey, namespace, keyPrefix string, limit int) ([]string, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	prefix := f.compose(pluginKey, namespace, keyPrefix)
	scope := f.compose(pluginKey, namespace, "")
	var keys []string
	for composed := range f.values {
		if strings.HasPrefix(composed, prefix) {
			keys = append(keys, strings.TrimPrefix(composed, scope))
		}
	}
	sort.Strings(keys)
	if limit > 0 && len(keys) > limit {
		keys = keys[:limit]
	}
	return keys, nil
}

func TestPluginHostServiceServer_SetGetDeleteRoundtrip(t *testing.T) {
	store := newFakePluginKVStore()
	server := newPluginHostServiceServer("local.example.plugin", store, nil, PluginAccountScope{})
	ctx := context.Background()

	_, err := server.KVSet(ctx, &pluginv1.KVSetRequest{Namespace: "state", Key: "alpha", Value: []byte("v1"), TtlSeconds: 60})
	require.NoError(t, err)

	got, err := server.KVGet(ctx, &pluginv1.KVGetRequest{Namespace: "state", Key: "alpha"})
	require.NoError(t, err)
	assert.True(t, got.Found)
	assert.Equal(t, []byte("v1"), got.Value)
	assert.Equal(t, time.Minute, store.ttls[store.compose("local.example.plugin", "state", "alpha")])

	_, err = server.KVDelete(ctx, &pluginv1.KVDeleteRequest{Namespace: "state", Key: "alpha"})
	require.NoError(t, err)

	got, err = server.KVGet(ctx, &pluginv1.KVGetRequest{Namespace: "state", Key: "alpha"})
	require.NoError(t, err)
	assert.False(t, got.Found)
}

func TestPluginHostServiceServer_GetNotFound(t *testing.T) {
	server := newPluginHostServiceServer("local.example.plugin", newFakePluginKVStore(), nil, PluginAccountScope{})
	got, err := server.KVGet(context.Background(), &pluginv1.KVGetRequest{Namespace: "state", Key: "missing"})
	require.NoError(t, err)
	assert.False(t, got.Found)
	assert.Nil(t, got.Value)
}

func TestPluginHostServiceServer_NamespaceIsolationAcrossPlugins(t *testing.T) {
	store := newFakePluginKVStore()
	first := newPluginHostServiceServer("local.plugin.one", store, nil, PluginAccountScope{})
	second := newPluginHostServiceServer("local.plugin.two", store, nil, PluginAccountScope{})
	ctx := context.Background()

	_, err := first.KVSet(ctx, &pluginv1.KVSetRequest{Namespace: "state", Key: "shared", Value: []byte("first")})
	require.NoError(t, err)

	// 另一个插件即便使用完全相同的 namespace/key 也读不到，因为 pluginKey 由宿主注入。
	got, err := second.KVGet(ctx, &pluginv1.KVGetRequest{Namespace: "state", Key: "shared"})
	require.NoError(t, err)
	assert.False(t, got.Found)

	got, err = first.KVGet(ctx, &pluginv1.KVGetRequest{Namespace: "state", Key: "shared"})
	require.NoError(t, err)
	assert.True(t, got.Found)
	assert.Equal(t, []byte("first"), got.Value)
}

func TestPluginHostServiceServer_ListPrefixAndLimit(t *testing.T) {
	store := newFakePluginKVStore()
	server := newPluginHostServiceServer("local.example.plugin", store, nil, PluginAccountScope{})
	ctx := context.Background()
	for _, key := range []string{"account-1", "account-2", "account-3", "other-1"} {
		_, err := server.KVSet(ctx, &pluginv1.KVSetRequest{Namespace: "state", Key: key, Value: []byte("x")})
		require.NoError(t, err)
	}

	resp, err := server.KVList(ctx, &pluginv1.KVListRequest{Namespace: "state", KeyPrefix: "account-"})
	require.NoError(t, err)
	assert.ElementsMatch(t, []string{"account-1", "account-2", "account-3"}, resp.Keys)

	resp, err = server.KVList(ctx, &pluginv1.KVListRequest{Namespace: "state", KeyPrefix: "account-", Limit: 2})
	require.NoError(t, err)
	assert.Len(t, resp.Keys, 2)
}

func TestPluginHostServiceServer_Validation(t *testing.T) {
	server := newPluginHostServiceServer("local.example.plugin", newFakePluginKVStore(), nil, PluginAccountScope{})
	ctx := context.Background()
	oversized := make([]byte, pluginKVMaxValueBytes+1)

	cases := []struct {
		name string
		call func() error
	}{
		{"empty namespace", func() error {
			_, err := server.KVGet(ctx, &pluginv1.KVGetRequest{Namespace: "", Key: "k"})
			return err
		}},
		{"namespace with colon", func() error {
			_, err := server.KVGet(ctx, &pluginv1.KVGetRequest{Namespace: "a:b", Key: "k"})
			return err
		}},
		{"key with glob", func() error {
			_, err := server.KVGet(ctx, &pluginv1.KVGetRequest{Namespace: "state", Key: "a*"})
			return err
		}},
		{"empty key", func() error {
			_, err := server.KVGet(ctx, &pluginv1.KVGetRequest{Namespace: "state", Key: ""})
			return err
		}},
		{"oversized value", func() error {
			_, err := server.KVSet(ctx, &pluginv1.KVSetRequest{Namespace: "state", Key: "k", Value: oversized})
			return err
		}},
		{"negative ttl", func() error {
			_, err := server.KVSet(ctx, &pluginv1.KVSetRequest{Namespace: "state", Key: "k", Value: []byte("v"), TtlSeconds: -1})
			return err
		}},
		{"ttl over max", func() error {
			_, err := server.KVSet(ctx, &pluginv1.KVSetRequest{Namespace: "state", Key: "k", Value: []byte("v"), TtlSeconds: int64(pluginKVMaxTTL/time.Second) + 1})
			return err
		}},
		{"ttl overflow", func() error {
			// 溢出用例：seconds*time.Second 会回绕成负值，必须仍判为超限而非放行。
			_, err := server.KVSet(ctx, &pluginv1.KVSetRequest{Namespace: "state", Key: "k", Value: []byte("v"), TtlSeconds: math.MaxInt64})
			return err
		}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := tc.call()
			require.Error(t, err)
			assert.Equal(t, codes.InvalidArgument, status.Code(err))
		})
	}
}

func TestPluginHostServiceServer_UnavailableWhenNoStore(t *testing.T) {
	server := newPluginHostServiceServer("local.example.plugin", nil, nil, PluginAccountScope{})
	_, err := server.KVGet(context.Background(), &pluginv1.KVGetRequest{Namespace: "state", Key: "k"})
	require.Error(t, err)
	assert.Equal(t, codes.Unavailable, status.Code(err))

	// pluginKey 为空同样视为不可用，避免键前缀塌缩导致的跨插件泄漏。
	blank := newPluginHostServiceServer("", newFakePluginKVStore(), nil, PluginAccountScope{})
	_, err = blank.KVGet(context.Background(), &pluginv1.KVGetRequest{Namespace: "state", Key: "k"})
	require.Error(t, err)
	assert.Equal(t, codes.Unavailable, status.Code(err))

	// pluginKey 含 ':' 会破坏内部键结构，必须判定为不可用而非放行。
	tainted := newPluginHostServiceServer("bad:key", newFakePluginKVStore(), nil, PluginAccountScope{})
	_, err = tainted.KVGet(context.Background(), &pluginv1.KVGetRequest{Namespace: "state", Key: "k"})
	require.Error(t, err)
	assert.Equal(t, codes.Unavailable, status.Code(err))
}

// pluginKey 的长度上限必须覆盖清单 id 的 maxLength（160），否则长 id 的合法插件会被
// 静默剥夺宿主服务能力。
func TestPluginHostServiceServer_PluginKeyLengthBoundary(t *testing.T) {
	ctx := context.Background()
	maxLen := newPluginHostServiceServer(strings.Repeat("a", pluginKVMaxPluginKeyLen), newFakePluginKVStore(), nil, PluginAccountScope{})
	_, err := maxLen.KVSet(ctx, &pluginv1.KVSetRequest{Namespace: "state", Key: "k", Value: []byte("v")})
	require.NoError(t, err)

	tooLong := newPluginHostServiceServer(strings.Repeat("a", pluginKVMaxPluginKeyLen+1), newFakePluginKVStore(), nil, PluginAccountScope{})
	_, err = tooLong.KVSet(ctx, &pluginv1.KVSetRequest{Namespace: "state", Key: "k", Value: []byte("v")})
	require.Error(t, err)
	assert.Equal(t, codes.Unavailable, status.Code(err))
}

type fakeAccountDirectory struct {
	infos     []PluginAccountInfo
	identity  *PluginOutboundIdentity
	lastReq   int64
	lastScope PluginAccountScope
}

func (f *fakeAccountDirectory) ListPluginAccounts(_ context.Context, scope PluginAccountScope, _, _ string) ([]PluginAccountInfo, error) {
	f.lastScope = scope
	return f.infos, nil
}

func (f *fakeAccountDirectory) ResolvePluginOutboundIdentity(_ context.Context, scope PluginAccountScope, accountID int64) (*PluginOutboundIdentity, error) {
	f.lastScope = scope
	f.lastReq = accountID
	if f.identity == nil || f.identity.AccountID != accountID {
		return nil, nil
	}
	return f.identity, nil
}

func TestPluginHostServiceServer_AccountDirectory(t *testing.T) {
	ctx := context.Background()
	dir := &fakeAccountDirectory{
		infos: []PluginAccountInfo{
			{ID: 3, Platform: "openai", AccountType: "oauth", Status: "active", Schedulable: false, MetadataJSON: []byte(`{"ID":3}`)},
			{ID: 7, Platform: "openai", AccountType: "oauth", Status: "active", Schedulable: true, MetadataJSON: []byte(`{"ID":7}`)},
		},
		identity: &PluginOutboundIdentity{
			AccountID: 7, Platform: "openai", AccountType: "oauth", ProxyURL: "http://p:1",
			Token: "tok", Headers: http.Header{"Originator": {"codex-tui"}},
		},
	}
	scope := newPluginAccountScope(pluginAccountScopeEntry{Platform: "openai", AccountType: "oauth"})
	server := newPluginHostServiceServer("local.example.plugin", newFakePluginKVStore(), dir, scope)

	list, err := server.ListAccounts(ctx, &pluginv1.ListAccountsRequest{Platform: "openai", AccountType: "oauth"})
	require.NoError(t, err)
	assert.Equal(t, []int64{3, 7}, list.AccountIds)
	require.Len(t, list.Accounts, 2)
	assert.False(t, list.Accounts[0].Schedulable)
	assert.True(t, list.Accounts[1].Schedulable)
	assert.Equal(t, []byte(`{"ID":7}`), list.Accounts[1].MetadataJson)
	assert.Equal(t, scope, dir.lastScope, "server must pass its scope to the directory")

	resolved, err := server.ResolveOutboundIdentity(ctx, &pluginv1.ResolveOutboundIdentityRequest{AccountId: 7})
	require.NoError(t, err)
	require.True(t, resolved.Found)
	assert.Equal(t, "tok", resolved.Token)
	assert.Equal(t, "http://p:1", resolved.ProxyUrl)
	assert.Equal(t, "codex-tui", resolved.Headers["Originator"].Values[0])

	// unknown account → found=false, no error
	miss, err := server.ResolveOutboundIdentity(ctx, &pluginv1.ResolveOutboundIdentityRequest{AccountId: 99})
	require.NoError(t, err)
	assert.False(t, miss.Found)

	// invalid account id → InvalidArgument
	_, err = server.ResolveOutboundIdentity(ctx, &pluginv1.ResolveOutboundIdentityRequest{AccountId: 0})
	assert.Equal(t, codes.InvalidArgument, status.Code(err))
}

// 无目录时账号目录 RPC 必须返回 Unavailable（KV 仍可用），保证未授权插件拿不到凭据。
func TestPluginHostServiceServer_DirectoryUnavailableWithoutDirectory(t *testing.T) {
	server := newPluginHostServiceServer("local.example.plugin", newFakePluginKVStore(), nil, PluginAccountScope{})
	_, err := server.ListAccounts(context.Background(), &pluginv1.ListAccountsRequest{})
	assert.Equal(t, codes.Unavailable, status.Code(err))
	_, err = server.ResolveOutboundIdentity(context.Background(), &pluginv1.ResolveOutboundIdentityRequest{AccountId: 1})
	assert.Equal(t, codes.Unavailable, status.Code(err))
	// KV 仍可用
	_, kvErr := server.KVGet(context.Background(), &pluginv1.KVGetRequest{Namespace: "state", Key: "k"})
	require.NoError(t, kvErr)
}

func TestPluginAccountScopeFromManifest(t *testing.T) {
	// A declared OpenAI OAuth capability grants exactly the (openai, oauth) scope.
	match := PluginManifest{Capabilities: []PluginCapability{{ID: PluginCapabilityOpenAIOAuthOutbound, Platform: PlatformOpenAI, AccountType: AccountTypeOAuth}}}
	scope := pluginAccountScopeFromManifest(match)
	if scope.Empty() {
		t.Fatal("declared OpenAI OAuth capability must grant a non-empty scope")
	}
	if !scope.Contains(PlatformOpenAI, AccountTypeOAuth) {
		t.Fatal("scope must contain (openai, oauth)")
	}
	if scope.Contains("anthropic", "oauth") {
		t.Fatal("scope must not leak to other platforms")
	}

	// The granted scope is pinned to the capability id, so a manifest cannot widen
	// it by declaring a different platform/account_type on a known capability.
	spoof := PluginManifest{Capabilities: []PluginCapability{{ID: PluginCapabilityOpenAIOAuthOutbound, Platform: "anthropic", AccountType: "apikey"}}}
	spoofScope := pluginAccountScopeFromManifest(spoof)
	if spoofScope.Contains("anthropic", "apikey") {
		t.Fatal("granted scope must be pinned to the capability id, not the declared platform/type")
	}
	if !spoofScope.Contains(PlatformOpenAI, AccountTypeOAuth) {
		t.Fatal("known capability must still grant its pinned (openai, oauth) scope")
	}

	// An unknown capability grants nothing.
	unknown := PluginManifest{Capabilities: []PluginCapability{{ID: "some.other.capability", Platform: "x", AccountType: "y"}}}
	if !pluginAccountScopeFromManifest(unknown).Empty() {
		t.Fatal("unknown capability must grant an empty scope")
	}
	if !pluginAccountScopeFromManifest(PluginManifest{}).Empty() {
		t.Fatal("no capability must grant an empty scope")
	}
}

// buildHostServices 只对声明了 OpenAI OAuth 能力的插件注入账号目录。
func TestBuildHostServicesGatesDirectoryByCapability(t *testing.T) {
	dir := &fakeAccountDirectory{}
	m := &PluginManager{kvStore: newFakePluginKVStore(), accountDirectory: dir}

	authorized := &PluginInstallation{PluginKey: "p.authorized", Manifest: PluginManifest{
		Capabilities: []PluginCapability{{ID: PluginCapabilityOpenAIOAuthOutbound, Platform: PlatformOpenAI, AccountType: AccountTypeOAuth}},
	}}
	srv, ok := m.buildHostServices(authorized).(*pluginHostServiceServer)
	require.True(t, ok)
	require.NotNil(t, srv.directory)

	unauthorized := &PluginInstallation{PluginKey: "p.other", Manifest: PluginManifest{
		Capabilities: []PluginCapability{{ID: "some.other.capability", Platform: "x", AccountType: "y"}},
	}}
	srv2, ok := m.buildHostServices(unauthorized).(*pluginHostServiceServer)
	require.True(t, ok)
	require.Nil(t, srv2.directory, "unauthorized plugin must not receive the account directory")
	require.NotNil(t, srv2.store, "but KV remains available")
}
