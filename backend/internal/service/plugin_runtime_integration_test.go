package service

import (
	"bytes"
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type pluginFailingRequestBody struct{}

func (pluginFailingRequestBody) Read([]byte) (int, error) {
	return 0, errors.New("测试请求体读取失败")
}
func (pluginFailingRequestBody) Close() error { return nil }

func TestPluginRuntimeIntegration(t *testing.T) {
	packagePath := os.Getenv("SUB2API_TEST_PLUGIN_PACKAGE")
	if packagePath == "" {
		t.Skip("未提供 SUB2API_TEST_PLUGIN_PACKAGE，跳过本地插件进程集成测试")
	}
	packageFile, err := os.Open(packagePath)
	require.NoError(t, err)
	defer func() { _ = packageFile.Close() }()

	root := t.TempDir()
	cfg := testPluginConfig(root, false)
	installer := NewPluginPackageInstaller(cfg, PluginHostInfo{Version: "0.1.179", BuildType: "release"})
	installation, err := installer.Install(context.Background(), packageFile, nil)
	require.NoError(t, err)
	assert.Equal(t, PluginSignatureTrusted, installation.SignatureStatus)
	for _, relative := range []string{
		"ui/index.html",
		"ui/assets/styles.css",
		"ui/assets/bridge-v1.js",
		"ui/assets/app.js",
	} {
		assert.FileExists(t, filepath.Join(installation.InstallPath, filepath.FromSlash(relative)))
	}

	runtime, err := startPluginRuntime(context.Background(), installation, 10*time.Second, filepath.Join(root, "runtime"))
	require.NoError(t, err)
	defer runtime.kill()
	require.NoError(t, runtime.validateAndApplyConfig(context.Background(), []byte(`{
		"request_timeout_seconds":30,
		"response_header_timeout_seconds":10,
		"idle_connection_timeout_seconds":30,
		"max_idle_connections":10,
		"max_idle_connections_per_host":5,
		"enable_http2":true,
		"tls_min_version":"1.2",
		"proxy_mode":"disabled",
		"extra_headers":{"X-Plugin-Test":"enabled"}
	}`)))

	upstream := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		body, readErr := io.ReadAll(request.Body)
		require.NoError(t, readErr)
		assert.Equal(t, "payload", string(body))
		assert.Equal(t, "enabled", request.Header.Get("X-Plugin-Test"))
		assert.Equal(t, int64(len("payload")), request.ContentLength)
		assert.Empty(t, request.TransferEncoding)
		writer.Header().Set("X-Upstream", "plugin-runtime")
		writer.WriteHeader(http.StatusCreated)
		_, _ = writer.Write([]byte("response-through-plugin"))
	}))
	defer upstream.Close()

	request, err := http.NewRequestWithContext(context.Background(), http.MethodPost, upstream.URL, bytes.NewBufferString("payload"))
	require.NoError(t, err)
	request.Header.Set("Content-Type", "application/json")
	account := &Account{ID: 42, Platform: PlatformOpenAI, Type: AccountTypeOAuth, Concurrency: 1}
	require.True(t, runtime.beginRequest())
	response, err := runtime.roundTrip(context.Background(), request, "", account)
	if err != nil {
		runtime.finishRequest()
	}
	require.NoError(t, err)
	body, err := io.ReadAll(response.Body)
	require.NoError(t, err)
	assert.Equal(t, http.StatusCreated, response.StatusCode)
	assert.Equal(t, "plugin-runtime", response.Header.Get("X-Upstream"))
	assert.Equal(t, "response-through-plugin", string(body))
	assert.Equal(t, int64(len("response-through-plugin")), response.ContentLength)
	require.NoError(t, response.Body.Close())

	failingRequest, err := http.NewRequestWithContext(context.Background(), http.MethodPost, upstream.URL, pluginFailingRequestBody{})
	require.NoError(t, err)
	failingRequest.ContentLength = -1
	require.True(t, runtime.beginRequest())
	started := time.Now()
	_, err = runtime.roundTrip(context.Background(), failingRequest, "", account)
	runtime.finishRequest()
	require.Error(t, err)
	assert.Less(t, time.Since(started), 2*time.Second)
}
