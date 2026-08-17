package service

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const (
	testCodexFingerprintSeedA = "11111111-1111-4111-8111-111111111111"
	testCodexFingerprintSeedB = "22222222-2222-4222-8222-222222222222"
)

func newTestOAuthAccount(id int64, extra map[string]any) *Account {
	if mode, _ := extra[codexFingerprintModeExtraKey].(string); mode == string(codexFingerprintDevice) || mode == string(codexFingerprintSession) || mode == string(codexFingerprintFull) {
		if _, ok := extra[codexFingerprintSeedExtraKey]; !ok {
			extra[codexFingerprintSeedExtraKey] = testCodexFingerprintSeedA
		}
	}
	return &Account{ID: id, Platform: PlatformOpenAI, Type: AccountTypeOAuth, Extra: extra}
}

func TestGetCodexFingerprintModeUsesDeviceOnlyContract(t *testing.T) {
	tests := []struct {
		name    string
		account *Account
		want    codexFingerprintMode
	}{
		{name: "nil", account: nil, want: codexFingerprintOff},
		{name: "missing", account: newTestOAuthAccount(1, nil), want: codexFingerprintOff},
		{name: "invalid", account: newTestOAuthAccount(1, map[string]any{codexFingerprintModeExtraKey: "invalid"}), want: codexFingerprintOff},
		{name: "off", account: newTestOAuthAccount(1, map[string]any{codexFingerprintModeExtraKey: " off "}), want: codexFingerprintOff},
		{name: "device", account: newTestOAuthAccount(1, map[string]any{codexFingerprintModeExtraKey: " device "}), want: codexFingerprintDevice},
		{name: "legacy session", account: newTestOAuthAccount(1, map[string]any{codexFingerprintModeExtraKey: "session"}), want: codexFingerprintDevice},
		{name: "legacy full", account: newTestOAuthAccount(1, map[string]any{codexFingerprintModeExtraKey: "full"}), want: codexFingerprintDevice},
		{name: "API key", account: &Account{Platform: PlatformOpenAI, Type: AccountTypeAPIKey, Extra: map[string]any{codexFingerprintModeExtraKey: "device"}}, want: codexFingerprintOff},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, tt.account.GetCodexFingerprintMode())
		})
	}
}

func TestNormalizeCodexFingerprintModeExtraCanonicalizesLegacyModes(t *testing.T) {
	for _, legacy := range []string{"session", "full"} {
		extra := map[string]any{codexFingerprintModeExtraKey: legacy, "keep": true}
		normalizeCodexFingerprintModeExtra(extra)
		assert.Equal(t, "device", extra[codexFingerprintModeExtraKey])
		assert.Equal(t, true, extra["keep"])
	}

	for _, current := range []string{"off", "device", "invalid"} {
		extra := map[string]any{codexFingerprintModeExtraKey: current}
		normalizeCodexFingerprintModeExtra(extra)
		assert.Equal(t, current, extra[codexFingerprintModeExtraKey])
	}
}

func TestInitializeCodexFingerprintSeedLifecycle(t *testing.T) {
	t.Run("enabled account gets random UUIDv4", func(t *testing.T) {
		account := newTestOAuthAccount(1, map[string]any{codexFingerprintModeExtraKey: "device"})
		delete(account.Extra, codexFingerprintSeedExtraKey)

		initializeCodexFingerprintSeed(account, false)

		parsed, err := uuid.Parse(account.getCodexFingerprintSeed())
		require.NoError(t, err)
		assert.Equal(t, uuid.Version(4), parsed.Version())
	})

	t.Run("trusted restore preserves valid seed and canonicalizes mode", func(t *testing.T) {
		account := newTestOAuthAccount(1, map[string]any{
			codexFingerprintModeExtraKey: "session",
			codexFingerprintSeedExtraKey: testCodexFingerprintSeedA,
		})

		initializeCodexFingerprintSeed(account, false)

		assert.Equal(t, "device", account.Extra[codexFingerprintModeExtraKey])
		assert.Equal(t, testCodexFingerprintSeedA, account.getCodexFingerprintSeed())
	})

	t.Run("ordinary creation and duplication rotate identity", func(t *testing.T) {
		account := newTestOAuthAccount(1, map[string]any{
			codexFingerprintModeExtraKey: "device",
			codexFingerprintSeedExtraKey: testCodexFingerprintSeedA,
		})

		initializeCodexFingerprintSeed(account, true)

		assert.NotEmpty(t, account.getCodexFingerprintSeed())
		assert.NotEqual(t, testCodexFingerprintSeedA, account.getCodexFingerprintSeed())
	})
}

func TestResolveCodexFingerprintIDsOwnsOnlyInstallation(t *testing.T) {
	accountA := newTestOAuthAccount(1, map[string]any{
		codexFingerprintModeExtraKey: "device",
		codexFingerprintSeedExtraKey: testCodexFingerprintSeedA,
	})
	accountB := newTestOAuthAccount(1, map[string]any{
		codexFingerprintModeExtraKey: "device",
		codexFingerprintSeedExtraKey: testCodexFingerprintSeedB,
	})

	idsA := resolveCodexFingerprintIDsFromRequest(accountA, http.Header{"Session-Id": {"client-a"}})
	idsB := resolveCodexFingerprintIDsFromRequest(accountB, http.Header{"Session-Id": {"client-a"}})

	require.NotNil(t, idsA)
	require.NotNil(t, idsB)
	assert.Equal(t, codexFingerprintDevice, idsA.mode)
	assert.Equal(t, testCodexFingerprintSeedA, idsA.installationID)
	assert.NotEqual(t, idsA.installationID, idsB.installationID)
	assert.Nil(t, resolveCodexFingerprintIDsFromRequest(newTestOAuthAccount(1, nil), nil))
}

func TestResolveCodexFingerprintIDsHonorsExplicitDeviceOverride(t *testing.T) {
	account := newTestOAuthAccount(1, map[string]any{
		codexFingerprintModeExtraKey: "device",
		codexFingerprintSeedExtraKey: testCodexFingerprintSeedA,
		"openai_device_id":           "explicit-installation",
	})

	ids := resolveCodexFingerprintIDsFromRequest(account, nil)

	require.NotNil(t, ids)
	assert.Equal(t, "explicit-installation", ids.installationID)
}

func TestApplyCodexFingerprintHeadersMatchesRegularRequestShape(t *testing.T) {
	ids := &codexFingerprintIDs{mode: codexFingerprintDevice, installationID: testCodexFingerprintSeedA}
	h := http.Header{}
	h.Set("x-codex-installation-id", "client-installation")
	h.Set("session-id", "client-session")
	h.Set("thread-id", "client-thread")
	h.Set("x-client-request-id", "client-thread")
	h.Set("x-codex-window-id", "client-window")
	h.Set("x-codex-turn-metadata", `{"installation_id":"client-installation","session_id":"client-session","thread_id":"client-thread","window_id":"client-window","sandbox":"seatbelt"}`)

	applyCodexFingerprintHeaders(h, ids)

	assert.Empty(t, h.Get("x-codex-installation-id"), "regular Codex requests carry installation in client_metadata")
	assert.Equal(t, "client-session", h.Get("session-id"))
	assert.Equal(t, "client-thread", h.Get("thread-id"))
	assert.Equal(t, "client-thread", h.Get("x-client-request-id"))
	assert.Equal(t, "client-window", h.Get("x-codex-window-id"))
	var metadata map[string]any
	require.NoError(t, json.Unmarshal([]byte(h.Get("x-codex-turn-metadata")), &metadata))
	assert.Equal(t, testCodexFingerprintSeedA, metadata["installation_id"])
	assert.Equal(t, "client-session", metadata["session_id"])
	assert.Equal(t, "client-thread", metadata["thread_id"])
	assert.Equal(t, "client-window", metadata["window_id"])
	assert.Equal(t, "seatbelt", metadata["sandbox"])
}

func TestApplyCodexFingerprintCompactHeadersMatchesLegacyCompactShape(t *testing.T) {
	ids := &codexFingerprintIDs{mode: codexFingerprintDevice, installationID: testCodexFingerprintSeedA}
	h := http.Header{}
	h.Set("session-id", "client-session")
	h.Set("thread-id", "client-thread")
	h.Set("x-codex-turn-metadata", `{"installation_id":"old","session_id":"client-session"}`)

	applyCodexFingerprintCompactHeaders(h, ids)

	assert.Equal(t, testCodexFingerprintSeedA, h.Get("x-codex-installation-id"))
	assert.Equal(t, "client-session", h.Get("session-id"))
	assert.Equal(t, "client-thread", h.Get("thread-id"))
	var metadata map[string]any
	require.NoError(t, json.Unmarshal([]byte(h.Get("x-codex-turn-metadata")), &metadata))
	assert.Equal(t, testCodexFingerprintSeedA, metadata["installation_id"])
	assert.Equal(t, "client-session", metadata["session_id"])
}

func TestApplyCodexFingerprintClientMetadataPreservesClientGraphAndCache(t *testing.T) {
	ids := &codexFingerprintIDs{mode: codexFingerprintDevice, installationID: testCodexFingerprintSeedA}
	body := map[string]any{
		"prompt_cache_key": "client-cache",
		"client_metadata": map[string]any{
			"x-codex-installation-id": "client-installation",
			"session_id":              "client-session",
			"thread_id":               "client-thread",
			"turn_id":                 "client-turn",
			"x-codex-window-id":       "client-window",
			"x-codex-turn-metadata":   `{"installation_id":"client-installation","session_id":"client-session","thread_id":"client-thread","turn_id":"client-turn","window_id":"client-window"}`,
		},
	}

	require.True(t, applyCodexFingerprintClientMetadata(body, ids))

	assert.Equal(t, "client-cache", body["prompt_cache_key"])
	metadata := body["client_metadata"].(map[string]any)
	assert.Equal(t, testCodexFingerprintSeedA, metadata["x-codex-installation-id"])
	assert.Equal(t, "client-session", metadata["session_id"])
	assert.Equal(t, "client-thread", metadata["thread_id"])
	assert.Equal(t, "client-turn", metadata["turn_id"])
	assert.Equal(t, "client-window", metadata["x-codex-window-id"])
	var embedded map[string]any
	require.NoError(t, json.Unmarshal([]byte(metadata["x-codex-turn-metadata"].(string)), &embedded))
	assert.Equal(t, testCodexFingerprintSeedA, embedded["installation_id"])
	assert.Equal(t, "client-session", embedded["session_id"])
	assert.Equal(t, "client-thread", embedded["thread_id"])
	assert.Equal(t, "client-turn", embedded["turn_id"])
	assert.Equal(t, "client-window", embedded["window_id"])
}

func TestApplyCodexFingerprintClientMetadataRawMatchesMapProjection(t *testing.T) {
	ids := &codexFingerprintIDs{mode: codexFingerprintDevice, installationID: testCodexFingerprintSeedA}
	for _, raw := range []string{
		`{"model":"gpt-5.6-sol","prompt_cache_key":"cache","input":[]}`,
		`{"model":"gpt-5.6-sol","prompt_cache_key":"cache","client_metadata":{"session_id":"session","trace":"keep"},"input":[]}`,
		`{"model":"gpt-5.6-sol","prompt_cache_key":"cache","client_metadata":"legacy","input":[]}`,
	} {
		var mapped map[string]any
		require.NoError(t, json.Unmarshal([]byte(raw), &mapped))
		require.True(t, applyCodexFingerprintClientMetadata(mapped, ids))

		patched, changed, err := applyCodexFingerprintClientMetadataRaw([]byte(raw), ids)
		require.NoError(t, err)
		require.True(t, changed)
		var rawMapped map[string]any
		require.NoError(t, json.Unmarshal(patched, &rawMapped))
		assert.Equal(t, mapped, rawMapped)
		assert.Equal(t, "cache", rawMapped["prompt_cache_key"])
	}
}

func TestApplyCodexFingerprintClientMetadataRawLeavesNonObjectRoot(t *testing.T) {
	ids := &codexFingerprintIDs{mode: codexFingerprintDevice, installationID: testCodexFingerprintSeedA}
	for _, raw := range []string{`[]`, `"value"`, `not-json`} {
		patched, changed, err := applyCodexFingerprintClientMetadataRaw([]byte(raw), ids)
		require.NoError(t, err)
		assert.False(t, changed)
		assert.Equal(t, raw, string(patched))
	}
}

func TestExtractClientSessionIDPrefersOfficialHyphenHeader(t *testing.T) {
	h := http.Header{}
	h.Set("session-id", "hyphen")
	h.Set("session_id", "underscore")
	assert.Equal(t, "hyphen", extractClientSessionID(h))
	h.Del("session-id")
	assert.Equal(t, "underscore", extractClientSessionID(h))
}

func TestStagedCodexFingerprintDoesNotLeakAcrossFailover(t *testing.T) {
	gin.SetMode(gin.TestMode)
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", nil)
	deviceAccount := newTestOAuthAccount(1, map[string]any{
		codexFingerprintModeExtraKey: "device",
		codexFingerprintSeedExtraKey: testCodexFingerprintSeedA,
	})
	offAccount := newTestOAuthAccount(2, map[string]any{codexFingerprintModeExtraKey: "off"})

	stageCodexFingerprintIDs(c, resolveCodexFingerprintIDsFromRequest(deviceAccount, nil))
	stageCodexFingerprintIDs(c, resolveCodexFingerprintIDsFromRequest(offAccount, nil))
	h := http.Header{"X-Codex-Installation-Id": {"client-installation"}}
	applyStagedCodexFingerprintHeaders(c, offAccount, h)

	assert.Equal(t, "client-installation", h.Get("x-codex-installation-id"))
}

func TestStagedCodexFingerprintChoosesProjectionByRequestPath(t *testing.T) {
	gin.SetMode(gin.TestMode)
	account := newTestOAuthAccount(1, map[string]any{
		codexFingerprintModeExtraKey: "device",
		codexFingerprintSeedExtraKey: testCodexFingerprintSeedA,
	})
	ids := resolveCodexFingerprintIDsFromRequest(account, nil)

	for _, tt := range []struct {
		name       string
		path       string
		wantHeader string
	}{
		{name: "regular", path: "/v1/responses", wantHeader: ""},
		{name: "legacy compact", path: "/v1/responses/compact", wantHeader: testCodexFingerprintSeedA},
	} {
		t.Run(tt.name, func(t *testing.T) {
			c, _ := gin.CreateTestContext(httptest.NewRecorder())
			c.Request = httptest.NewRequest(http.MethodPost, tt.path, nil)
			stageCodexFingerprintIDs(c, ids)
			h := http.Header{"X-Codex-Installation-Id": {"client-installation"}}

			applyStagedCodexFingerprintHeaders(c, account, h)

			assert.Equal(t, tt.wantHeader, h.Get("x-codex-installation-id"))
		})
	}
}

func TestBuildOpenAIWSHeadersUsesRegularCodexProjection(t *testing.T) {
	gin.SetMode(gin.TestMode)
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest(http.MethodGet, "/v1/responses", nil)
	c.Request.Header.Set("x-codex-installation-id", "client-installation")
	c.Request.Header.Set("x-codex-window-id", "client-window")
	c.Request.Header.Set("x-codex-turn-metadata", `{"installation_id":"client-installation","session_id":"client-session"}`)
	account := newTestOAuthAccount(1, map[string]any{
		codexFingerprintModeExtraKey: "device",
		codexFingerprintSeedExtraKey: testCodexFingerprintSeedA,
	})
	account.Credentials = map[string]any{
		"access_token":       "token",
		"chatgpt_account_id": "account",
	}

	headers, _, err := (&OpenAIGatewayService{}).buildOpenAIWSHeaders(
		context.Background(),
		c,
		account,
		"token",
		OpenAIWSProtocolDecision{Transport: OpenAIUpstreamTransportResponsesWebsocketV2},
		true,
		"",
		c.GetHeader("x-codex-turn-metadata"),
		"client-cache",
		"gpt-5.6-sol",
		"",
	)

	require.NoError(t, err)
	assert.Empty(t, headers.Get("x-codex-installation-id"))
	assert.Equal(t, "client-window", headers.Get("x-codex-window-id"))
	var metadata map[string]any
	require.NoError(t, json.Unmarshal([]byte(headers.Get("x-codex-turn-metadata")), &metadata))
	assert.Equal(t, testCodexFingerprintSeedA, metadata["installation_id"])
	assert.Equal(t, "client-session", metadata["session_id"])
}

func TestBuildUpstreamRequestPassthroughUsesRequestKindProjection(t *testing.T) {
	account := newTestOAuthAccount(1, map[string]any{
		"openai_oauth_passthrough":   true,
		codexFingerprintModeExtraKey: "device",
		codexFingerprintSeedExtraKey: testCodexFingerprintSeedA,
	})
	account.Credentials = map[string]any{"chatgpt_account_id": "account"}
	svc := &OpenAIGatewayService{}

	for _, tt := range []struct {
		name       string
		path       string
		wantHeader string
	}{
		{name: "regular", path: "/v1/responses", wantHeader: ""},
		{name: "legacy compact", path: "/v1/responses/compact", wantHeader: testCodexFingerprintSeedA},
	} {
		t.Run(tt.name, func(t *testing.T) {
			c, _ := gin.CreateTestContext(httptest.NewRecorder())
			c.Request = httptest.NewRequest(http.MethodPost, tt.path, nil)
			c.Request.Header.Set("user-agent", "codex_cli_rs/0.144.1")
			c.Request.Header.Set("originator", "codex_cli_rs")
			c.Request.Header.Set("session-id", "client-session")
			c.Request.Header.Set("thread-id", "client-thread")
			c.Request.Header.Set("x-client-request-id", "client-thread")
			c.Request.Header.Set("x-codex-window-id", "client-window")
			c.Request.Header.Set("x-codex-installation-id", "client-installation")
			c.Request.Header.Set("x-codex-turn-metadata", `{"installation_id":"client-installation","session_id":"client-session","thread_id":"client-thread","window_id":"client-window"}`)
			stageCodexFingerprintIDs(c, resolveCodexFingerprintIDsFromRequest(account, c.Request.Header))

			req, err := svc.buildUpstreamRequestOpenAIPassthrough(
				context.Background(),
				c,
				account,
				[]byte(`{"model":"gpt-5.6-sol","input":[],"stream":true}`),
				"token",
			)

			require.NoError(t, err)
			assert.Equal(t, tt.wantHeader, req.Header.Get("x-codex-installation-id"))
			assert.Equal(t, "client-session", req.Header.Get("session-id"))
			assert.Equal(t, "client-thread", req.Header.Get("thread-id"))
			assert.Equal(t, "client-thread", req.Header.Get("x-client-request-id"))
			assert.Equal(t, "client-window", req.Header.Get("x-codex-window-id"))
			var metadata map[string]any
			require.NoError(t, json.Unmarshal([]byte(req.Header.Get("x-codex-turn-metadata")), &metadata))
			assert.Equal(t, testCodexFingerprintSeedA, metadata["installation_id"])
			assert.Equal(t, "client-session", metadata["session_id"])
			assert.Equal(t, "client-thread", metadata["thread_id"])
			assert.Equal(t, "client-window", metadata["window_id"])
		})
	}
}
