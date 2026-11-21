package service

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	pluginv1 "github.com/Wei-Shaw/sub2api/pkg/pluginapi/v1"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/sjson"
	"google.golang.org/grpc/status"
)

// 子进程复用当前测试二进制，测试不依赖外部插件包、Go 编译器或真实模型账号。
func init() {
	if os.Getenv(pluginv1.HandshakeConfig.MagicCookieKey) == pluginv1.HandshakeConfig.MagicCookieValue {
		pluginv1.Serve(&requestPatchProcessFixture{})
		os.Exit(0)
	}
}

type requestPatchProcessFixture struct {
	pluginv1.UnimplementedTransportPluginServer
	pluginv1.UnimplementedRequestPatchServer
}

func (*requestPatchProcessFixture) GetInfo(context.Context, *pluginv1.GetInfoRequest) (*pluginv1.GetInfoResponse, error) {
	return &pluginv1.GetInfoResponse{
		PluginId: filepath.Base(os.Args[0]), PluginVersion: "0.1.0",
		ProtocolVersion: pluginv1.ProtocolVersion, TransportApiVersion: pluginv1.TransportAPIVersion,
		Capabilities: []string{PluginCapabilityOpenAIOAuthOutbound, PluginCapabilityAnthropicOAuthPatch},
	}, nil
}

func (*requestPatchProcessFixture) Health(context.Context, *pluginv1.HealthRequest) (*pluginv1.HealthResponse, error) {
	return &pluginv1.HealthResponse{Healthy: true}, nil
}

func (*requestPatchProcessFixture) ValidateConfig(_ context.Context, request *pluginv1.ValidateConfigRequest) (*pluginv1.ValidateConfigResponse, error) {
	_, err := pluginPatchJSONObject(request.ConfigJson)
	return &pluginv1.ValidateConfigResponse{Valid: err == nil, NormalizedConfigJson: request.ConfigJson}, nil
}

func (*requestPatchProcessFixture) ApplyConfig(context.Context, *pluginv1.ApplyConfigRequest) (*pluginv1.ApplyConfigResponse, error) {
	return &pluginv1.ApplyConfigResponse{Applied: true}, nil
}

func (*requestPatchProcessFixture) PatchRequest(ctx context.Context, request *pluginv1.RequestPatchRequest) (*pluginv1.RequestPatchResponse, error) {
	if strings.HasSuffix(request.GetRequest().GetUrl(), "/deadline") {
		deadline, ok := ctx.Deadline()
		if !ok {
			return nil, errors.New("RPC 没有截止时间")
		}
		return &pluginv1.RequestPatchResponse{SetHeaders: map[string]*pluginv1.HeaderValues{
			"X-Fixture-Deadline-Ms": {Values: []string{strconv.FormatInt(time.Until(deadline).Milliseconds(), 10)}},
		}}, nil
	}
	if strings.HasSuffix(request.GetRequest().GetUrl(), "/wait") {
		<-ctx.Done()
		return nil, status.FromContextError(ctx.Err()).Err()
	}
	body, err := sjson.SetBytes(request.BodyJson, "metadata.plugin_fixture", true)
	if err != nil {
		return nil, err
	}
	return &pluginv1.RequestPatchResponse{
		BodyJson:      body,
		SetHeaders:    map[string]*pluginv1.HeaderValues{"x-request-patch": {Values: []string{"applied"}}},
		RemoveHeaders: []string{"X-Patch-Remove"},
	}, nil
}

func startRequestPatchFixture(t *testing.T, id int64, capabilities ...string) *pluginRuntime {
	t.Helper()
	executable, err := os.Executable()
	require.NoError(t, err)
	file, err := os.ReadFile(executable)
	require.NoError(t, err)
	key := "com.example.request-patch-" + strconv.FormatInt(id, 10)
	binary := filepath.Join(t.TempDir(), key)
	require.NoError(t, os.WriteFile(binary, file, 0o700))
	digest := sha256.Sum256(file)
	manifest := testPluginManifest(nil)
	manifest.ID = key
	manifest.Capabilities = nil
	installation := &PluginInstallation{
		ID: id, PluginKey: key, Version: "0.1.0", State: PluginStateEnabled,
		BinaryPath: binary, BinarySHA256: hex.EncodeToString(digest[:]),
	}
	for _, capability := range capabilities {
		platform, ok := pluginCapabilityPlatform(capability)
		require.True(t, ok)
		manifest.Capabilities = append(manifest.Capabilities, PluginCapability{ID: capability, Platform: platform, AccountType: AccountTypeOAuth})
		installation.Bindings = append(installation.Bindings, PluginBinding{PluginID: id, Capability: capability, Platform: platform, AccountType: AccountTypeOAuth, Enabled: true, RolloutPercent: 100})
	}
	installation.Manifest = manifest
	runtime, err := startPluginRuntime(context.Background(), installation, 10*time.Second, t.TempDir(), nil)
	require.NoError(t, err)
	t.Cleanup(runtime.kill)
	return runtime
}

func TestAnthropicRequestPatchProcessAndOriginalTransport(t *testing.T) {
	runtime := startRequestPatchFixture(t, 1, PluginCapabilityAnthropicOAuthPatch)
	manager := &PluginManager{runtimes: map[int64]*pluginRuntime{1: runtime}}
	manager.anthropicRoute.Store(&pluginRoute{pluginID: 1, runtime: runtime, rolloutPercent: 100})
	account := &Account{ID: 42, Platform: PlatformAnthropic, Type: AccountTypeOAuth}

	t.Run("补丁后的正文和请求头仍由原 HTTP 客户端发送", func(t *testing.T) {
		upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, request *http.Request) {
			body, err := io.ReadAll(request.Body)
			require.NoError(t, err)
			require.Equal(t, "Bearer test-secret", request.Header.Get("Authorization"))
			require.Equal(t, "applied", getHeaderRaw(request.Header, "x-request-patch"))
			require.Empty(t, request.Header.Get("X-Patch-Remove"))
			require.JSONEq(t, `{"model":"claude-test","stream":true,"metadata":{"plugin_fixture":true}}`, string(body))
			require.Equal(t, int64(len(body)), request.ContentLength)
			_, _ = w.Write([]byte("event: message_stop\ndata: {}\n\n"))
		}))
		defer upstream.Close()
		body := []byte(`{"model":"claude-test","stream":true}`)
		request, err := http.NewRequest(http.MethodPost, upstream.URL, bytes.NewReader(body))
		require.NoError(t, err)
		request.Header.Set("Authorization", "Bearer test-secret")
		request.Header.Set("X-Patch-Remove", "before")
		patched, sentBody, err := manager.PatchAnthropicOAuthRequest(context.Background(), request, body, account)
		require.NoError(t, err)
		replayed, err := patched.GetBody()
		require.NoError(t, err)
		defer func() { _ = replayed.Close() }()
		replayedBody, err := io.ReadAll(replayed)
		require.NoError(t, err)
		require.Equal(t, sentBody, replayedBody)
		response, err := upstream.Client().Do(patched)
		require.NoError(t, err)
		defer func() { _ = response.Body.Close() }()
		responseBody, err := io.ReadAll(response.Body)
		require.NoError(t, err)
		require.Equal(t, "event: message_stop\ndata: {}\n\n", string(responseBody))
	})

	t.Run("消息与计数构建器都在最终正文上应用补丁", func(t *testing.T) {
		gateway := &GatewayService{pluginManager: manager}
		body := []byte(`{"model":"claude-test","messages":[{"role":"user","content":"hello"}]}`)
		for _, countTokens := range []bool{false, true} {
			var request *http.Request
			var wireBody []byte
			var err error
			if countTokens {
				request, wireBody, err = gateway.buildCountTokensRequest(context.Background(), nil, account, body, "test-token", "oauth", "claude-test", false)
			} else {
				request, wireBody, err = gateway.buildUpstreamRequest(context.Background(), nil, account, body, "test-token", "oauth", "claude-test", false, false)
			}
			require.NoError(t, err)
			require.Equal(t, "applied", getHeaderRaw(request.Header, "x-request-patch"))
			sent, err := io.ReadAll(request.Body)
			require.NoError(t, err)
			require.Equal(t, wireBody, sent)
			require.Contains(t, string(wireBody), `"plugin_fixture":true`)
		}
	})

	t.Run("正文超过默认 gRPC 四 MiB 仍遵守显式上限", func(t *testing.T) {
		body, err := json.Marshal(map[string]any{"model": "claude-test", "input": strings.Repeat("x", 5<<20)})
		require.NoError(t, err)
		request, err := http.NewRequest(http.MethodPost, "https://api.anthropic.com/v1/messages", bytes.NewReader(body))
		require.NoError(t, err)
		patched, wireBody, err := manager.PatchAnthropicOAuthRequest(context.Background(), request, body, account)
		require.NoError(t, err)
		require.Greater(t, len(wireBody), 5<<20)
		require.NoError(t, patched.Body.Close())
	})

	t.Run("客户端取消传给插件且不发出上游请求", func(t *testing.T) {
		ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
		defer cancel()
		body := []byte(`{"model":"claude-test"}`)
		request, err := http.NewRequestWithContext(ctx, http.MethodPost, "https://api.anthropic.com/wait", bytes.NewReader(body))
		require.NoError(t, err)
		patched, _, err := manager.PatchAnthropicOAuthRequest(ctx, request, body, account)
		require.Nil(t, patched)
		require.ErrorIs(t, err, context.DeadlineExceeded)
		require.Zero(t, runtime.inFlight.Load())
	})

	t.Run("没有客户端期限时仍设置五秒 RPC 上限", func(t *testing.T) {
		body := []byte(`{"model":"claude-test"}`)
		request, err := http.NewRequest(http.MethodPost, "https://api.anthropic.com/deadline", bytes.NewReader(body))
		require.NoError(t, err)
		patched, _, err := manager.PatchAnthropicOAuthRequest(context.Background(), request, body, account)
		require.NoError(t, err)
		remaining, err := strconv.ParseInt(patched.Header.Get("X-Fixture-Deadline-Ms"), 10, 64)
		require.NoError(t, err)
		require.Positive(t, remaining)
		require.LessOrEqual(t, remaining, int64(5000))
	})
}

func TestAnthropicRequestPatchScopeAndFailClosed(t *testing.T) {
	manager := &PluginManager{}
	manager.anthropicRoute.Store(&pluginRoute{pluginID: 2, rolloutPercent: 100, unavailable: "测试不可用"})
	body := []byte(`{"model":"claude-test"}`)
	request := httptest.NewRequest(http.MethodPost, "https://api.anthropic.com/v1/messages", bytes.NewReader(body))
	for _, account := range []*Account{
		nil,
		{ID: 1, Platform: PlatformOpenAI, Type: AccountTypeOAuth},
		{ID: 1, Platform: PlatformAnthropic, Type: AccountTypeAPIKey},
		{ID: 1, Platform: PlatformAnthropic, Type: AccountTypeSetupToken},
		{ID: 1, Platform: PlatformAnthropic, Type: AccountTypeServiceAccount},
		{ID: 1, Platform: PlatformAnthropic, Type: AccountTypeBedrock},
	} {
		result, resultBody, err := manager.PatchAnthropicOAuthRequest(context.Background(), request, body, account)
		require.NoError(t, err)
		require.Same(t, request, result)
		require.Equal(t, body, resultBody)
	}
	account := &Account{ID: 1, Platform: PlatformAnthropic, Type: AccountTypeOAuth}
	_, _, err := manager.PatchAnthropicOAuthRequest(context.Background(), request, body, account)
	require.ErrorContains(t, err, "插件不可用")
	manager.anthropicRoute.Store(&pluginRoute{rolloutPercent: int(stablePluginBucket(account.ID))})
	result, _, err := manager.PatchAnthropicOAuthRequest(context.Background(), request, body, account)
	require.NoError(t, err)
	require.Same(t, request, result)
	manager.anthropicRoute.Store(nil)
	result, _, err = manager.PatchAnthropicOAuthRequest(context.Background(), request, body, account)
	require.NoError(t, err)
	require.Same(t, request, result)
}

func TestPluginPatchMetadataDoesNotExposeCredentials(t *testing.T) {
	body := []byte(`{"model":"claude-test"}`)
	request := httptest.NewRequest(http.MethodPost, "https://username:password@example.com/v1/messages?proxy=private&token=secret#fragment", bytes.NewReader(body))
	for _, key := range []string{"Authorization", "Cookie", "X-Api-Key", "Proxy-Authorization"} {
		request.Header.Set(key, "private")
	}
	request.Header.Set("User-Agent", "custom-client")
	metadata, err := pluginPatchRequestMetadata(request, body, &Account{ID: 1, Platform: PlatformAnthropic, Type: AccountTypeOAuth})
	require.NoError(t, err)
	require.Equal(t, "https://example.com/v1/messages", metadata.Url)
	require.Empty(t, metadata.ProxyUrl)
	require.Len(t, metadata.Headers, 1)
	require.Equal(t, []string{"custom-client"}, metadata.Headers["User-Agent"].Values)
}

func TestPluginRequestPatchRejectsUnsafeChangesAtomically(t *testing.T) {
	body := []byte(`{"model":"claude-test","stream":true}`)
	cases := map[string]*pluginv1.RequestPatchResponse{
		"缺少结果":       nil,
		"非对象正文":      {BodyJson: []byte(`[]`)},
		"重复模型字段":     {BodyJson: []byte(`{"model":"different","model":"claude-test","stream":true}`)},
		"正文异常不应用合法头": {BodyJson: []byte(`null`), SetHeaders: map[string]*pluginv1.HeaderValues{"X-Test": {Values: []string{"after"}}}},
		"空对象根":       {BodyJson: []byte(`null`)},
		"无效正文":       {BodyJson: []byte(`{`)},
		"改变模型":       {BodyJson: []byte(`{"model":"different","stream":true}`)},
		"移除流模式":      {BodyJson: []byte(`{"model":"claude-test"}`)},
		"改变流模式":      {BodyJson: []byte(`{"model":"claude-test","stream":false}`)},
		"认证头":        {SetHeaders: map[string]*pluginv1.HeaderValues{"Authorization": {Values: []string{"private"}}}},
		"移除 Cookie":  {RemoveHeaders: []string{"cookie"}},
		"连接控制":       {SetHeaders: map[string]*pluginv1.HeaderValues{"Transfer-Encoding": {Values: []string{"chunked"}}}},
		"长度":         {SetHeaders: map[string]*pluginv1.HeaderValues{"Content-Length": {Values: []string{"1"}}}},
		"换行注入":       {SetHeaders: map[string]*pluginv1.HeaderValues{"X-Test": {Values: []string{"bad\r\nInjected: x"}}}},
		"无效名称":       {SetHeaders: map[string]*pluginv1.HeaderValues{"bad name": {Values: []string{"x"}}}},
		"重复大小写":      {SetHeaders: map[string]*pluginv1.HeaderValues{"x-test": {Values: []string{"x"}}, "X-Test": {Values: []string{"y"}}}},
		"头过大":        {SetHeaders: map[string]*pluginv1.HeaderValues{"X-Test": {Values: []string{strings.Repeat("x", pluginRequestPatchMaxHeader)}}}},
		"头值过多":       {SetHeaders: map[string]*pluginv1.HeaderValues{"X-Test": {Values: make([]string, pluginRequestPatchMaxFields+1)}}},
	}
	for name, patch := range cases {
		t.Run(name, func(t *testing.T) {
			request := httptest.NewRequest(http.MethodPost, "https://api.anthropic.com/v1/messages", bytes.NewReader(body))
			request.Header.Set("X-Original", "preserved")
			result, _, err := applyPluginRequestPatch(request, body, patch)
			require.Error(t, err)
			require.Nil(t, result)
			require.Equal(t, http.Header{"X-Original": {"preserved"}}, request.Header)
			originalBody, err := io.ReadAll(request.Body)
			require.NoError(t, err)
			require.Equal(t, body, originalBody)
		})
	}
	t.Run("正文上限", func(t *testing.T) {
		_, err := pluginPatchJSONObject(make([]byte, pluginv1.RequestPatchMaxBodyBytes+1))
		require.ErrorContains(t, err, "64 MiB")
	})
}

func TestPluginRequestPatchClearsStaleBodyDigests(t *testing.T) {
	body := []byte(`{"model":"claude-test"}`)
	request := httptest.NewRequest(http.MethodPost, "https://api.anthropic.com/v1/messages", bytes.NewReader(body))
	for _, key := range []string{"Content-Length", "Content-MD5", "Digest", "Content-Digest", "Repr-Digest"} {
		request.Header.Set(key, "stale")
	}
	next, _, err := applyPluginRequestPatch(request, body, &pluginv1.RequestPatchResponse{BodyJson: []byte(`{"model":"claude-test","metadata":{}}`)})
	require.NoError(t, err)
	require.Empty(t, next.Header)
	_, _, err = applyPluginRequestPatch(request, body, &pluginv1.RequestPatchResponse{})
	require.NoError(t, err)
}

type pluginScopesRepository struct {
	PluginRepository
	installations map[int64]*PluginInstallation
}

func (r *pluginScopesRepository) List(ctx context.Context) ([]*PluginInstallation, error) {
	var result []*PluginInstallation
	for id := range r.installations {
		installation, _ := r.GetByID(ctx, id)
		result = append(result, installation)
	}
	return result, nil
}

func (r *pluginScopesRepository) GetByID(_ context.Context, id int64) (*PluginInstallation, error) {
	installation := r.installations[id]
	if installation == nil {
		return nil, errors.New("插件不存在")
	}
	copy := *installation
	copy.Bindings = append([]PluginBinding(nil), installation.Bindings...)
	return &copy, nil
}

func (r *pluginScopesRepository) UpdateBindingsAndState(_ context.Context, id int64, bindings []PluginBinding, state, lastError string, enabledAt *time.Time, _, _ string) error {
	installation := r.installations[id]
	installation.Bindings = append([]PluginBinding(nil), bindings...)
	installation.State, installation.LastError, installation.EnabledAt = state, lastError, enabledAt
	return nil
}

func TestPluginCapabilitiesCoexistAndReconcileIndependently(t *testing.T) {
	openai := startRequestPatchFixture(t, 1, PluginCapabilityOpenAIOAuthOutbound)
	anthropic := startRequestPatchFixture(t, 2, PluginCapabilityAnthropicOAuthPatch)
	repo := &pluginScopesRepository{installations: map[int64]*PluginInstallation{1: openai.installation, 2: anthropic.installation}}
	manager := NewPluginManager(repo, pluginTokenEncryptor{}, testPluginConfig(t.TempDir(), true), PluginHostInfo{Version: "0.1.179"}, nil)
	manager.publishRuntimeLocked(openai.installation, openai)
	manager.publishRuntimeLocked(anthropic.installation, anthropic)
	require.NoError(t, manager.reconcileOnce(context.Background()))
	require.Same(t, openai, manager.route.Load().runtime)
	require.Same(t, anthropic, manager.anthropicRoute.Load().runtime)
	_, err := manager.Enable(context.Background(), 2, false, 100)
	require.NoError(t, err)
	_, err = manager.Disable(context.Background(), 2)
	require.NoError(t, err)
	require.NoError(t, manager.reconcileOnce(context.Background()))
	require.Nil(t, manager.anthropicRoute.Load())
	require.Same(t, openai, manager.route.Load().runtime)
	require.False(t, openai.draining.Load())
	require.NoError(t, openai.checkHealth(context.Background()))
}

func TestPluginSharedRuntimeAndStaleFailure(t *testing.T) {
	installation := &PluginInstallation{ID: 1, Bindings: []PluginBinding{
		{Capability: PluginCapabilityOpenAIOAuthOutbound, Platform: PlatformOpenAI, AccountType: AccountTypeOAuth, Enabled: true, RolloutPercent: 40},
		{Capability: PluginCapabilityAnthropicOAuthPatch, Platform: PlatformAnthropic, AccountType: AccountTypeOAuth, Enabled: true, RolloutPercent: 75},
	}}
	runtime := &pluginRuntime{installation: installation, done: make(chan struct{})}
	manager := &PluginManager{runtimes: make(map[int64]*pluginRuntime)}
	manager.publishRuntimeLocked(installation, runtime)
	require.Len(t, manager.runtimes, 1)
	require.Equal(t, 40, manager.route.Load().rolloutPercent)
	require.Equal(t, 75, manager.anthropicRoute.Load().rolloutPercent)
	old := manager.route.Load()
	manager.route.Store(&pluginRoute{pluginID: 1, runtime: runtime, rolloutPercent: 40})
	require.NoError(t, manager.markRuntimeUnavailable(old, "旧请求异常"))
	require.Same(t, runtime, manager.route.Load().runtime)
	manager.detachCapabilityRoute(PluginCapabilityOpenAIOAuthOutbound)
	require.False(t, runtime.draining.Load())
	require.Len(t, manager.runtimes, 1)
	require.NoError(t, manager.markRuntimeUnavailable(manager.anthropicRoute.Load(), "当前进程异常"))
	require.Nil(t, manager.anthropicRoute.Load().runtime)
	require.Empty(t, manager.runtimes)
	manager.publishRuntimeLocked(installation, runtime)
	require.NoError(t, manager.markRuntimeUnavailable(manager.route.Load(), "共享进程异常"))
	require.Nil(t, manager.route.Load().runtime)
	require.Nil(t, manager.anthropicRoute.Load().runtime)
}

func TestPluginManifestAcceptsIndependentAnthropicCapability(t *testing.T) {
	manifest := testPluginManifest(nil)
	capability := PluginCapability{ID: PluginCapabilityAnthropicOAuthPatch, Platform: PlatformAnthropic, AccountType: AccountTypeOAuth}
	manifest.Capabilities = []PluginCapability{capability}
	require.NoError(t, manifest.Validate())
	require.True(t, pluginAccountScopeFromManifest(manifest).Empty())
	manifest.Capabilities = append(manifest.Capabilities, testPluginManifest(nil).Capabilities...)
	require.NoError(t, manifest.Validate())
	manifest.Capabilities = append(manifest.Capabilities, capability)
	require.Error(t, manifest.Validate())
	capability.Platform = PlatformOpenAI
	manifest.Capabilities = []PluginCapability{capability}
	require.Error(t, manifest.Validate())
}
