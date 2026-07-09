//go:build unit

package pluginhost

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"sort"
	"strings"
	"sync"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/pluginhost/sdk"
	"github.com/Wei-Shaw/sub2api/internal/pluginkit"

	"github.com/stretchr/testify/require"
)

// ============================================================
// memKVStore — KVStore 的内存替身（真实 DB 实现的行为由
// repository/plugin_kv_repo_test.go 覆盖）
// ============================================================

type memKVStore struct {
	mu   sync.Mutex
	rows map[pluginkit.ID]map[string]string
}

var _ KVStore = (*memKVStore)(nil)

func newMemKVStore() *memKVStore {
	return &memKVStore{rows: make(map[pluginkit.ID]map[string]string)}
}

func (s *memKVStore) Get(_ context.Context, pluginID pluginkit.ID, key string) (string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	value, ok := s.rows[pluginID][key]
	if !ok {
		return "", fmt.Errorf("%w: %s/%s", ErrKVNotFound, pluginID, key)
	}
	return value, nil
}

func (s *memKVStore) Set(_ context.Context, pluginID pluginkit.ID, key, value string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.rows[pluginID] == nil {
		s.rows[pluginID] = make(map[string]string)
	}
	s.rows[pluginID][key] = value
	return nil
}

func (s *memKVStore) Delete(_ context.Context, pluginID pluginkit.ID, key string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.rows[pluginID], key)
	return nil
}

func (s *memKVStore) Keys(_ context.Context, pluginID pluginkit.ID, prefix string) ([]string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	var keys []string
	for k := range s.rows[pluginID] {
		if prefix == "" || strings.HasPrefix(k, prefix) {
			keys = append(keys, k)
		}
	}
	sort.Strings(keys)
	return keys, nil
}

func (s *memKVStore) DeleteAll(_ context.Context, pluginID pluginkit.ID) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.rows, pluginID)
	return nil
}

// ============================================================
// 能力面测试夹具
// ============================================================

// capabilityFixture 起一个真实的能力 HTTP 服务并登记两个插件 token。
type capabilityFixture struct {
	server   *capabilityServer
	kv       *memKVStore
	installs *memInstallStore
	logBuf   *syncBuffer
	tokenA   string
	tokenB   string
}

type syncBuffer struct {
	mu  sync.Mutex
	buf bytes.Buffer
}

func (b *syncBuffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.Write(p)
}

func (b *syncBuffer) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.String()
}

func newCapabilityFixture(t *testing.T) *capabilityFixture {
	t.Helper()
	kv := newMemKVStore()
	installs := newMemInstallStore()
	logBuf := &syncBuffer{}
	server := newCapabilityServer(kv, installs, slog.New(slog.NewTextHandler(logBuf, nil)))
	require.NoError(t, server.start())
	t.Cleanup(func() { _ = server.close() })

	tokenA, err := newPluginToken()
	require.NoError(t, err)
	tokenB, err := newPluginToken()
	require.NoError(t, err)
	server.register(tokenA, "plugin.a")
	server.register(tokenB, "plugin.b")
	return &capabilityFixture{
		server: server, kv: kv, installs: installs, logBuf: logBuf,
		tokenA: tokenA, tokenB: tokenB,
	}
}

// doCapability 直接向能力服务发 HTTP 请求。
func (f *capabilityFixture) doCapability(t *testing.T, token, method, path, body string) *http.Response {
	t.Helper()
	req, err := http.NewRequest(method, f.server.baseURL+path, strings.NewReader(body))
	require.NoError(t, err)
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	t.Cleanup(func() { _ = resp.Body.Close() })
	return resp
}

// sdkClient 显式注入 token 构造 SDK 能力客户端（覆盖 SDK 与能力面的 HTTP
// 契约；stdin 握手取材路径由 supervisor 测试的真实子进程覆盖）。
func (f *capabilityFixture) sdkClient(t *testing.T, token string) *sdk.Client {
	t.Helper()
	return sdk.NewClientWithToken(f.server.baseURL, token)
}

// ============================================================
// 鉴权与隔离
// ============================================================

// TestCapabilityTokenAuth 未带 token / 错误 token 一律 401；正确 token 放行。
func TestCapabilityTokenAuth(t *testing.T) {
	f := newCapabilityFixture(t)

	for name, token := range map[string]string{
		"无token":  "",
		"伪造token": "deadbeef",
		"已知前缀":    f.tokenA[:32],
	} {
		resp := f.doCapability(t, token, http.MethodGet, "/v1/kv", "")
		require.Equal(t, http.StatusUnauthorized, resp.StatusCode, "case=%s", name)
	}

	// 撤销后的 token 立即失效（进程退出即失效的语义）
	f.server.unregister(f.tokenB)
	resp := f.doCapability(t, f.tokenB, http.MethodGet, "/v1/kv", "")
	require.Equal(t, http.StatusUnauthorized, resp.StatusCode)

	resp = f.doCapability(t, f.tokenA, http.MethodGet, "/v1/kv", "")
	require.Equal(t, http.StatusOK, resp.StatusCode)
}

// TestCapabilityKVNamespaceIsolation KV 按插件 ID 命名空间硬隔离：
// token 决定命名空间，插件永远读不到他人条目。
func TestCapabilityKVNamespaceIsolation(t *testing.T) {
	f := newCapabilityFixture(t)
	ctx := context.Background()

	clientA := f.sdkClient(t, f.tokenA)
	require.NoError(t, clientA.KVSet(ctx, "shared-key", "value-of-a"))

	clientB := f.sdkClient(t, f.tokenB)
	// B 看不到 A 的同名键
	_, found, err := clientB.KVGet(ctx, "shared-key")
	require.NoError(t, err)
	require.False(t, found, "命名空间必须隔离")
	keys, err := clientB.KVKeys(ctx, "")
	require.NoError(t, err)
	require.Empty(t, keys)

	// B 写同名键不影响 A
	require.NoError(t, clientB.KVSet(ctx, "shared-key", "value-of-b"))
	valueA, found, err := clientA.KVGet(ctx, "shared-key")
	require.NoError(t, err)
	require.True(t, found)
	require.Equal(t, "value-of-a", valueA)

	// 存储侧命名空间落位正确
	got, err := f.kv.Get(ctx, "plugin.b", "shared-key")
	require.NoError(t, err)
	require.Equal(t, "value-of-b", got)
}

// TestCapabilityKVRoundtrip KV 全操作经 SDK 客户端往返：set/get/keys/delete。
func TestCapabilityKVRoundtrip(t *testing.T) {
	f := newCapabilityFixture(t)
	ctx := context.Background()
	client := f.sdkClient(t, f.tokenA)

	_, found, err := client.KVGet(ctx, "missing")
	require.NoError(t, err)
	require.False(t, found)

	require.NoError(t, client.KVSet(ctx, "cfg:alpha", "1"))
	require.NoError(t, client.KVSet(ctx, "cfg:beta", "2"))
	require.NoError(t, client.KVSet(ctx, "other", "3"))

	keys, err := client.KVKeys(ctx, "cfg:")
	require.NoError(t, err)
	require.Equal(t, []string{"cfg:alpha", "cfg:beta"}, keys)

	require.NoError(t, client.KVDelete(ctx, "cfg:alpha"))
	_, found, err = client.KVGet(ctx, "cfg:alpha")
	require.NoError(t, err)
	require.False(t, found)
	require.NoError(t, client.KVDelete(ctx, "cfg:alpha"), "删除不存在的键应为 no-op")
}

// TestCapabilityKVLimits 键与值的边界校验：非法键 400、超限值 413。
func TestCapabilityKVLimits(t *testing.T) {
	f := newCapabilityFixture(t)

	resp := f.doCapability(t, f.tokenA, http.MethodPut, "/v1/kv/"+strings.Repeat("k", 257), "v")
	require.Equal(t, http.StatusBadRequest, resp.StatusCode, "超长键应拒绝")

	oversize := strings.Repeat("x", kvValueMaxBytes+1)
	resp = f.doCapability(t, f.tokenA, http.MethodPut, "/v1/kv/big", oversize)
	require.Equal(t, http.StatusRequestEntityTooLarge, resp.StatusCode)
}

// TestCapabilityLog Log 能力落宿主结构化日志并携带 plugin 命名空间字段。
func TestCapabilityLog(t *testing.T) {
	f := newCapabilityFixture(t)
	client := f.sdkClient(t, f.tokenA)
	require.NoError(t, client.Log(context.Background(), "warn", "something happened", map[string]any{"count": 3}))

	logged := f.logBuf.String()
	require.Contains(t, logged, "something happened")
	require.Contains(t, logged, "plugin=plugin.a")
	require.Contains(t, logged, "count=3")
	require.Contains(t, logged, "WARN")

	// 非法 level 拒绝
	resp := f.doCapability(t, f.tokenA, http.MethodPost, "/v1/log", `{"level":"fatal","message":"x"}`)
	require.Equal(t, http.StatusBadRequest, resp.StatusCode)
}

// TestCapabilityConfig Config 读：未配置归一为 {}，已配置返回原文。
func TestCapabilityConfig(t *testing.T) {
	f := newCapabilityFixture(t)
	ctx := context.Background()
	inst := &Installation{
		ID:      "plugin.a",
		Version: "1.0.0",
		Manifest: &Manifest{
			ID: "plugin.a", Name: "A", Version: "1.0.0", Protocol: ProtocolHTTP1,
			Frontend: &FrontendSpec{Entry: "webapp/plugin.js"},
		},
		InstallPath: t.TempDir(),
	}
	require.NoError(t, f.installs.Upsert(ctx, inst))

	client := f.sdkClient(t, f.tokenA)
	var cfg map[string]any
	require.NoError(t, client.Config(ctx, &cfg))
	require.Empty(t, cfg, "未配置应归一为空对象")

	require.NoError(t, f.installs.SetConfig(ctx, "plugin.a", []byte(`{"greeting":"hi"}`)))
	require.NoError(t, client.Config(ctx, &cfg))
	require.Equal(t, "hi", cfg["greeting"])

	// 未安装的插件（token 有效但登记不存在）→ 404
	clientB := f.sdkClient(t, f.tokenB)
	err := clientB.Config(ctx, &cfg)
	require.Error(t, err)
	require.Contains(t, err.Error(), "404")
}

// TestConfigOrEmptyObject 归一函数的边界。
func TestConfigOrEmptyObject(t *testing.T) {
	require.Equal(t, "{}", string(configOrEmptyObject(nil)))
	require.Equal(t, "{}", string(configOrEmptyObject([]byte("null"))))
	require.Equal(t, `{"a":1}`, string(configOrEmptyObject([]byte(`{"a":1}`))))
}

// TestMemKVStoreContract 内存替身与 KVStore 契约对齐（ErrKVNotFound 语义）。
func TestMemKVStoreContract(t *testing.T) {
	kv := newMemKVStore()
	ctx := context.Background()
	_, err := kv.Get(ctx, "p", "k")
	require.True(t, errors.Is(err, ErrKVNotFound))
	require.NoError(t, kv.Set(ctx, "p", "k", "v"))
	require.NoError(t, kv.DeleteAll(ctx, "p"))
	_, err = kv.Get(ctx, "p", "k")
	require.True(t, errors.Is(err, ErrKVNotFound))
}
