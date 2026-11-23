package service

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	coderws "github.com/coder/websocket"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

func testSignedWSCompanion(t *testing.T, validDigest bool) (*PluginManager, *pluginRuntime) {
	t.Helper()
	fixtureDir := os.Getenv("SUB2API_TEST_WS_COMPANION_DIR")
	if fixtureDir == "" {
		t.Skip("signed legacy/native .s2plugin archives and publisher.b64 are required")
	}
	root, err := os.MkdirTemp("", "wsc-")
	require.NoError(t, err)
	t.Cleanup(func() { _ = os.RemoveAll(root) })
	cfg := testPluginConfig(root, false)
	pub, err := os.ReadFile(filepath.Join(fixtureDir, "publisher.b64"))
	require.NoError(t, err)
	cfg.Plugins.TrustedPublishers["sp985-regression"] = strings.TrimSpace(string(pub))
	cfg.Plugins.OpenAIWSIdentityEnabled = true
	cfg.Plugins.OpenAIWSIdentityCanaryAccountID = 901
	cfg.Plugins.OpenAIWSIdentityCanaryAPIKeyID = 101
	cfg.Plugins.OpenAIWSIdentityCompanionPackage = filepath.Join(fixtureDir, "native.s2plugin")
	archive, err := os.ReadFile(cfg.Plugins.OpenAIWSIdentityCompanionPackage)
	require.NoError(t, err)
	digest := sha256.Sum256(archive)
	cfg.Plugins.OpenAIWSIdentityCompanionSHA256 = hex.EncodeToString(digest[:])
	if !validDigest {
		cfg.Plugins.OpenAIWSIdentityCompanionSHA256 = strings.Repeat("0", 64)
	}
	// A nil repository deliberately fails any attempt to access shared plugin data.
	m := NewPluginManager(nil, pluginTokenEncryptor{}, cfg, PluginHostInfo{Version: "0.2.24", BuildType: "release"}, nil)
	legacy, err := os.Open(filepath.Join(fixtureDir, "legacy.s2plugin"))
	require.NoError(t, err)
	t.Cleanup(func() { _ = legacy.Close() })
	installation, err := m.installer.Install(t.Context(), legacy, nil)
	require.NoError(t, err)
	require.Equal(t, "0.5.7", installation.Version)
	installation.ID = 42
	// The signed test archives contain a mock HMAC. Use the local UUID
	// mapping path because this fixture has no Gateway network.
	installation.ConfigEncrypted = `ENC:{"enabled":true,"session_map_enabled":false}`
	runtime, err := m.prepareRuntime(t.Context(), installation, true)
	require.NoError(t, err)
	t.Cleanup(runtime.kill)
	m.route.Store(&pluginRoute{pluginID: installation.ID, runtime: runtime, rolloutPercent: 100})
	return m, runtime
}

func assertLegacySignedHTTPForward(t *testing.T, m *PluginManager, expected *pluginRuntime) {
	t.Helper()
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		w.Header().Set("X-Fixture-Observed-UA", r.UserAgent())
		_, _ = io.WriteString(w, "data: {\"type\":\"response.completed\"}\n\n")
	}))
	t.Cleanup(upstream.Close)
	account := &Account{ID: 902, Platform: PlatformOpenAI, Type: AccountTypeOAuth, Concurrency: 1}
	route := m.openAIOAuthRoute(account)
	require.NotNil(t, route)
	require.Same(t, expected, route.runtime)
	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, upstream.URL+"/v1/responses", nil)
	require.NoError(t, err)
	req.Header.Set("User-Agent", "codex-tui/0.154.0 (Ubuntu 22.04.0; x86_64) xterm-256color")
	req.Header.Set("Originator", "codex-tui")
	resp, err := route.runtime.roundTrip(ctx, req, "", account)
	require.NoError(t, err)
	defer func() { _ = resp.Body.Close() }()
	require.Equal(t, http.StatusOK, resp.StatusCode)
	require.Contains(t, resp.Header.Get("X-Fixture-Observed-UA"), "ghostty/")
	body, err := io.ReadAll(resp.Body)
	require.NoError(t, err)
	require.Contains(t, string(body), "response.completed")
}

func TestSignedWSIdentityCompanionKeepsLegacyHTTPAndStopsOnConfigChange(t *testing.T) {
	m, legacy := testSignedWSCompanion(t, true)
	require.Nil(t, legacy.wsIdentity, "legacy archive must remain HTTP-only")
	require.NotNil(t, legacy.wsCompanion)
	companion := legacy.wsCompanion.runtime
	require.NotSame(t, legacy, companion)
	require.Equal(t, "0.5.8", companion.installation.Version)
	require.Equal(t, legacy.installation.PluginKey, companion.installation.PluginKey)
	require.Equal(t, int64(0), companion.installation.ID, "local companion was not registered")
	assertLegacySignedHTTPForward(t, m, legacy)

	fixture := newWSIdentityWireFixture(t)
	fixture.cfg.Plugins = m.cfg.Plugins
	fixture.svc.pluginManager = m
	client, done := fixture.startForKey(t, 101)
	requireWSIdentityCompletion(t, client, "resp_native_1")
	_ = waitWSIdentityFixtureValue(t, fixture.handshakes)
	first := waitWSIdentityFixtureValue(t, fixture.frames)
	require.Equal(t, gjson.False, gjson.GetBytes(first, "generate").Type)
	firstSession, err := uuid.Parse(gjson.GetBytes(first, "client_metadata.session_id").String())
	require.NoError(t, err)
	require.Equal(t, uuid.Version(7), firstSession.Version())
	require.NoError(t, client.Write(t.Context(), coderws.MessageText, []byte(wsIdentityContinuation)))
	requireWSIdentityCompletion(t, client, "resp_native_2")
	frame := waitWSIdentityFixtureValue(t, fixture.frames)
	require.Equal(t, "resp_native_1", gjson.GetBytes(frame, "previous_response_id").String())
	require.Equal(t, firstSession.String(), gjson.GetBytes(frame, "client_metadata.session_id").String())
	require.Empty(t, fixture.handshakes)
	require.NoError(t, legacy.validateAndApplyConfig(t.Context(), []byte(`{"enabled":true,"session_map_enabled":false,"sync_interval_seconds":120}`)))
	require.NoError(t, client.Write(t.Context(), coderws.MessageText, []byte(wsIdentityContinuation)))
	_, closeErr := readPassthroughLifecycleFrame(t, client, 3*time.Second)
	require.Equal(t, coderws.StatusInternalError, coderws.CloseStatus(closeErr))
	require.ErrorIs(t, waitWSIdentityFixtureValue(t, done), ErrOpenAIWSIdentityRewrite)
	stale, _ := legacy.nativeWSIdentity(true)
	require.Nil(t, stale, "new native connections must not reuse stale companion settings")
	require.Empty(t, fixture.frames)
	assertLegacySignedHTTPForward(t, m, legacy)

	legacy.kill()
	require.NoFileExists(t, companion.installation.BinaryPath)
	require.FileExists(t, legacy.installation.BinaryPath, "retiring companion preserves the legacy install")
}

func TestWSIdentityCompanionArchiveIsBoundToDigestAndSize(t *testing.T) {
	root := t.TempDir()
	file := filepath.Join(root, "archive")
	data := []byte("synthetic archive bytes")
	require.NoError(t, os.WriteFile(file, data, 0o600))
	hash := sha256.Sum256(data)
	digest := hex.EncodeToString(hash[:])
	got, err := readWSIdentityCompanionArchive(file, digest, int64(len(data)))
	require.NoError(t, err)
	require.Equal(t, data, got)
	_, err = readWSIdentityCompanionArchive(file, digest, int64(len(data)-1))
	require.ErrorContains(t, err, "size limit")
	_, err = readWSIdentityCompanionArchive(root, digest, 1024)
	require.ErrorContains(t, err, "regular file")
	_, err = readWSIdentityCompanionArchive(file, strings.Repeat("0", 64), 1024)
	require.ErrorContains(t, err, "SHA-256 does not match")
}

func TestSignedWSIdentityCompanionBadDigestLeavesLegacyHTTP(t *testing.T) {
	m, legacy := testSignedWSCompanion(t, false)
	require.Nil(t, legacy.wsCompanion)
	assertLegacySignedHTTPForward(t, m, legacy)
}
