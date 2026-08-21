package service

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func newSessionTaintTestContext(t *testing.T, apiKeyID int64, sessionID string) *gin.Context {
	t.Helper()
	gin.SetMode(gin.TestMode)
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", nil)
	if sessionID != "" {
		c.Request.Header.Set("session-id", sessionID)
	}
	if apiKeyID > 0 {
		c.Set("api_key", &APIKey{ID: apiKeyID})
	}
	return c
}

// TestRekeyCodexTaintID covers the deterministic id remapping: same input →
// same output, prefix inherited, UUIDv5-shaped suffix, legacy ids (no
// underscore or empty suffix) mapped to "" (caller deletes), account change →
// new value.
func TestRekeyCodexTaintID(t *testing.T) {
	a := rekeyCodexTaintID(101, "msg_0a1b2c3d-4e5f-6789-abcd-ef0123456789")
	b := rekeyCodexTaintID(101, "msg_0a1b2c3d-4e5f-6789-abcd-ef0123456789")
	require.Equal(t, a, b, "deterministic for same account + old id")
	require.True(t, strings.HasPrefix(a, "msg_"), "prefix inherited")
	suffix := strings.TrimPrefix(a, "msg_")
	require.Len(t, suffix, 36, "UUID-shaped suffix")
	require.Equal(t, "5", string(suffix[14]), "UUIDv5 version nibble")
	require.NotEqual(t, suffix, "0a1b2c3d-4e5f-6789-abcd-ef0123456789", "suffix must differ from old id")
}

// TestRekeyCodexTaintID_PrefixMatrix covers prefix inheritance across id
// families and the legacy-deletion rule.
func TestRekeyCodexTaintID_PrefixMatrix(t *testing.T) {
	for _, id := range []string{
		"msg_abc", "fc_abc", "fco_abc", "rs_abc", "cmp_abc", "lsh_abc", "tsc_abc",
	} {
		mapped := rekeyCodexTaintID(7, id)
		prefix := id[:strings.Index(id, "_")]
		require.True(t, strings.HasPrefix(mapped, prefix+"_"), "%s keeps prefix", id)
		require.NotEqual(t, mapped, id)
	}

	// Legacy ids without prefix_suffix shape are deleted (official client strips
	// them too before sending, client.rs:944-950).
	require.Empty(t, rekeyCodexTaintID(7, "legacy-uuid-no-underscore"))
	require.Empty(t, rekeyCodexTaintID(7, "msg_"), "empty suffix is not prefixed")
	require.Empty(t, rekeyCodexTaintID(7, "_abc"), "empty prefix is not prefixed")
	require.Empty(t, rekeyCodexTaintID(7, ""))
}

// TestRekeyCodexTaintID_AccountSensitivity verifies switching accounts changes
// every mapped id while call/output pairing survives (same function on both).
func TestRekeyCodexTaintID_AccountSensitivity(t *testing.T) {
	call := rekeyCodexTaintID(202, "fc_old-call")
	callOut := rekeyCodexTaintID(202, "fc_old-call") // output side uses the same call_id input
	require.Equal(t, call, callOut, "call_id pairing stable within one account")

	callNewAccount := rekeyCodexTaintID(303, "fc_old-call")
	require.NotEqual(t, call, callNewAccount, "account switch rekeys the id")
	require.True(t, strings.HasPrefix(callNewAccount, "fc_"))
}

// TestDeriveCodexTaintUUID covers the header/body session identifier derivation.
func TestDeriveCodexTaintUUID(t *testing.T) {
	v1 := deriveCodexTaintUUID("pck", 11, 22, "seed-1")
	require.Len(t, v1, 36, "UUID shape")
	require.Equal(t, v1, deriveCodexTaintUUID("pck", 11, 22, "seed-1"), "deterministic")
	require.NotEqual(t, v1, deriveCodexTaintUUID("pck", 11, 23, "seed-1"), "account-sensitive")
	require.NotEqual(t, v1, deriveCodexTaintUUID("pck", 12, 22, "seed-1"), "apiKey-sensitive")
	require.NotEqual(t, v1, deriveCodexTaintUUID("thread", 11, 22, "seed-1"), "domain-separated")
	require.Empty(t, deriveCodexTaintUUID("pck", 11, 22, ""), "empty seed → empty")
}

// TestOpenAICodexSessionTaintStateMachine covers the provenance table:
// first account recorded without taint, same account keeps it untainted,
// a different account marks it switched forever (sticky), and TTL expiry
// clears the record.
func TestOpenAICodexSessionTaintStateMachine(t *testing.T) {
	svc := &OpenAIGatewayService{}
	c := newSessionTaintTestContext(t, 7, "sess-taint")

	require.False(t, svc.isOpenAICodexSessionTainted(c), "unknown session is untainted")

	svc.noteOpenAICodexSessionAttempt(c, &Account{ID: 1})
	require.False(t, svc.isOpenAICodexSessionTainted(c), "single account never taints")

	svc.noteOpenAICodexSessionAttempt(c, &Account{ID: 1})
	require.False(t, svc.isOpenAICodexSessionTainted(c), "same account retry never taints")

	svc.noteOpenAICodexSessionAttempt(c, &Account{ID: 2})
	require.True(t, svc.isOpenAICodexSessionTainted(c), "account switch taints")

	// Sticky: switching back to the first account keeps the taint (rollout
	// still replays ids minted by account 2).
	svc.noteOpenAICodexSessionAttempt(c, &Account{ID: 1})
	require.True(t, svc.isOpenAICodexSessionTainted(c))

	// TTL expiry clears the record.
	seed := openAICodexTurnStateSeed(c)
	svc.openaiCodexSessionTaints.Store(seed, openAICodexSessionTaint{
		firstAccountID: 1,
		everSwitched:   true,
		expiresAt:      time.Now().Add(-time.Second),
	})
	require.False(t, svc.isOpenAICodexSessionTainted(c), "expired record is ignored and deleted")
	_, ok := svc.openaiCodexSessionTaints.Load(seed)
	require.False(t, ok)
}

// TestOpenAICodexSessionTaint_NoSessionSeed verifies requests without a client
// session header are not tracked (taint detection impossible → passthrough).
func TestOpenAICodexSessionTaint_NoSessionSeed(t *testing.T) {
	svc := &OpenAIGatewayService{}
	c := newSessionTaintTestContext(t, 7, "")
	svc.noteOpenAICodexSessionAttempt(c, &Account{ID: 1})
	svc.noteOpenAICodexSessionAttempt(c, &Account{ID: 2})
	require.False(t, svc.isOpenAICodexSessionTainted(c))
	require.False(t, svc.isOpenAICodexSessionTainted(nil), "nil context is safe")
}

// TestSweepOpenAICodexSessionTaints verifies the opportunistic sweep removes
// expired entries once the write counter hits its stride.
func TestSweepOpenAICodexSessionTaints(t *testing.T) {
	svc := &OpenAIGatewayService{}
	svc.openaiCodexSessionTaints.Store("expired", openAICodexSessionTaint{
		firstAccountID: 1,
		everSwitched:   true,
		expiresAt:      time.Now().Add(-time.Second),
	})
	svc.openaiCodexSessionTaints.Store("fresh", openAICodexSessionTaint{
		firstAccountID: 1,
		everSwitched:   true,
		expiresAt:      time.Now().Add(time.Hour),
	})
	// Sweep triggers when the internal counter (incremented inside the sweep
	// call itself) hits a multiple of 256; pre-drive it to 255.
	for i := uint64(0); i < 255; i++ {
		svc.openaiCodexSessionTaintWrites.Add(1)
	}
	svc.sweepOpenAICodexSessionTaints()
	_, ok := svc.openaiCodexSessionTaints.Load("expired")
	require.False(t, ok, "expired entry swept")
	_, ok = svc.openaiCodexSessionTaints.Load("fresh")
	require.True(t, ok, "fresh entry kept")
}

// TestOpenAICodexTaintSanitizeContextRoundTrip covers the gin-context marker
// used to carry "rekey with account N" from the handler failover loop into the
// service transform/header layers.
func TestOpenAICodexTaintSanitizeContextRoundTrip(t *testing.T) {
	c := newSessionTaintTestContext(t, 7, "sess-ctx")
	require.Zero(t, openAICodexTaintSanitizeAccountID(c), "default: disabled")

	setOpenAICodexTaintSanitize(c, 42)
	require.Equal(t, int64(42), openAICodexTaintSanitizeAccountID(c))

	// A later attempt against another account overrides the marker.
	setOpenAICodexTaintSanitize(c, 43)
	require.Equal(t, int64(43), openAICodexTaintSanitizeAccountID(c))

	require.Zero(t, openAICodexTaintSanitizeAccountID(nil), "nil context is safe")
}

// TestFilterCodexInput_TaintRekey covers taint-mode input filtering: prefixed
// item ids are rekeyed (deterministic, prefix inherited), reasoning keeps its
// id only when encrypted_content is present, call/output call_id pairing
// survives rekeying, legacy unprefixed ids are dropped, and item_reference is
// removed (the referenced id no longer exists after rekeying; the official
// client never emits item_reference).
func TestFilterCodexInput_TaintRekey(t *testing.T) {
	input := []any{
		map[string]any{"type": "message", "role": "user", "id": "msg_old1", "content": "hi"},
		map[string]any{"type": "reasoning", "id": "rs_old1", "encrypted_content": "ENC", "summary": []any{}},
		map[string]any{"type": "reasoning", "id": "rs_old2", "summary": []any{}},
		map[string]any{"type": "function_call", "id": "fc_old1", "call_id": "fc_call1", "name": "shell"},
		map[string]any{"type": "function_call_output", "call_id": "fc_call1", "output": "ok"},
		map[string]any{"type": "message", "role": "assistant", "id": "legacy-no-underscore"},
		map[string]any{"type": "item_reference", "id": "msg_old1"},
	}
	out := filterCodexInputWithOptions(input, codexInputFilterOptions{
		PreserveReferences: true,
		TaintAccountID:     9,
	})
	require.Len(t, out, 6, "item_reference dropped")

	msg := out[0].(map[string]any)
	require.Equal(t, rekeyCodexTaintID(9, "msg_old1"), msg["id"])
	require.NotEqual(t, "msg_old1", msg["id"])

	rsEnc := out[1].(map[string]any)
	require.Equal(t, rekeyCodexTaintID(9, "rs_old1"), rsEnc["id"], "reasoning with ciphertext keeps a rekeyed id")
	require.Equal(t, "ENC", rsEnc["encrypted_content"], "ciphertext verbatim")

	rsPlain := out[2].(map[string]any)
	_, hasID := rsPlain["id"]
	require.False(t, hasID, "reasoning without ciphertext keeps the historical strip behavior")

	fc := out[3].(map[string]any)
	require.Equal(t, rekeyCodexTaintID(9, "fc_old1"), fc["id"])
	rekeyedCall := rekeyCodexTaintID(9, "fc_call1")
	require.Equal(t, rekeyedCall, fc["call_id"])

	fco := out[4].(map[string]any)
	require.Equal(t, rekeyedCall, fco["call_id"], "call/output pairing survives rekeying")

	legacy := out[5].(map[string]any)
	_, hasID = legacy["id"]
	require.False(t, hasID, "legacy unprefixed id dropped")
}

// TestFilterCodexInput_TaintDisabledUnchanged pins the default behavior:
// without a taint account the filter behaves exactly as before (ids preserved
// under PreserveReferences, historical reasoning-id strip kept).
func TestFilterCodexInput_TaintDisabledUnchanged(t *testing.T) {
	input := []any{
		map[string]any{"type": "message", "role": "user", "id": "msg_old1", "content": "hi"},
		map[string]any{"type": "reasoning", "id": "rs_old1", "encrypted_content": "ENC", "summary": []any{}},
		map[string]any{"type": "function_call", "id": "fc_old1", "call_id": "fc_call1", "name": "shell"},
		map[string]any{"type": "function_call_output", "call_id": "fc_call1", "output": "ok"},
	}
	out := filterCodexInputWithOptions(input, codexInputFilterOptions{PreserveReferences: true})
	require.Equal(t, "msg_old1", out[0].(map[string]any)["id"])
	_, hasRS := out[1].(map[string]any)["id"]
	require.False(t, hasRS, "historical rs_* strip unaffected")
	require.Equal(t, "fc_old1", out[2].(map[string]any)["id"])
	require.Equal(t, "fc_call1", out[2].(map[string]any)["call_id"])
}

// TestApplyCodexOAuthTransform_TaintPromptCacheKey covers the top-level
// prompt_cache_key rewrite: under taint it becomes the account-aware derived
// UUID shared with the outbound session_id header; without taint it is
// untouched.
func TestApplyCodexOAuthTransform_TaintPromptCacheKey(t *testing.T) {
	body := map[string]any{
		"model":            "gpt-5",
		"prompt_cache_key": "client-key-1",
		"input": []any{
			map[string]any{"type": "message", "role": "user", "content": "hi"},
		},
	}
	res := applyCodexOAuthTransformWithOptions(body, codexOAuthTransformOptions{
		IsCodexCLI:              true,
		TaintAccountID:          9,
		TaintAPIKeyID:           7,
		SkipDefaultInstructions: true,
	})
	require.Equal(t, deriveCodexTaintUUID("pck", 7, 9, "client-key-1"), body["prompt_cache_key"])
	require.Equal(t, deriveCodexTaintUUID("pck", 7, 9, "client-key-1"), res.PromptCacheKey)

	plain := map[string]any{
		"model":            "gpt-5",
		"prompt_cache_key": "client-key-1",
		"input": []any{
			map[string]any{"type": "message", "role": "user", "content": "hi"},
		},
	}
	applyCodexOAuthTransformWithOptions(plain, codexOAuthTransformOptions{
		IsCodexCLI:              true,
		SkipDefaultInstructions: true,
	})
	require.Equal(t, "client-key-1", plain["prompt_cache_key"], "no taint → untouched")
}

// TestResolveOpenAICodexTaintInstallationID covers the installation-id
// derivation chain: account device_id → fingerprint seed → taint-derived UUID.
func TestResolveOpenAICodexTaintInstallationID(t *testing.T) {
	// Account with a real device_id wins.
	withDevice := &Account{ID: 42, Platform: PlatformOpenAI, Type: AccountTypeOAuth, Extra: map[string]any{"openai_device_id": "dev-real"}}
	require.Equal(t, "dev-real", resolveOpenAICodexTaintInstallationID(withDevice, 7, 42))

	// No device, no seed → deterministic taint derivation.
	bare := &Account{ID: 42, Extra: map[string]any{}}
	v1 := resolveOpenAICodexTaintInstallationID(bare, 7, 42)
	require.Len(t, v1, 36)
	require.Equal(t, v1, resolveOpenAICodexTaintInstallationID(bare, 7, 42), "deterministic")
	require.NotEqual(t, v1, resolveOpenAICodexTaintInstallationID(bare, 7, 43), "account-sensitive")

	// Nil account → taint derivation.
	require.Len(t, resolveOpenAICodexTaintInstallationID(nil, 7, 42), 36)
}

// TestApplyCodexTaintHeaders covers header-side taint rewriting: installation
// and window headers rederived, turn-metadata embedded ids rekeyed with the
// session_id==thread_id equality preserved, unrelated fields (git info) kept.
func TestApplyCodexTaintHeaders(t *testing.T) {
	c := newSessionTaintTestContext(t, 7, "sess-h")
	setOpenAICodexTaintSanitize(c, 42)
	h := http.Header{}
	h.Set("x-codex-installation-id", "11111111-2222-3333-4444-555555555555")
	h.Set("x-codex-window-id", "win-old")
	h.Set("x-codex-turn-metadata", `{"installation_id":"inst-old","session_id":"sess-old","thread_id":"sess-old","turn_id":"turn-old","window_id":"sess-old:0","git":{"remote_url":"x"}}`)

	require.NoError(t, applyCodexTaintHeaders(c, h, &Account{ID: 42}, 7))

	require.NotEqual(t, "11111111-2222-3333-4444-555555555555", h.Get("x-codex-installation-id"))
	require.NotEqual(t, "win-old", h.Get("x-codex-window-id"))

	var meta map[string]any
	require.NoError(t, json.Unmarshal([]byte(h.Get("x-codex-turn-metadata")), &meta))
	require.Equal(t, meta["session_id"], meta["thread_id"], "official session==thread equality preserved")
	require.NotEqual(t, "sess-old", meta["session_id"])
	require.NotEqual(t, "turn-old", meta["turn_id"])
	require.NotEqual(t, "inst-old", meta["installation_id"])
	require.NotEqual(t, "sess-old:0", meta["window_id"])
	require.Equal(t, "x", meta["git"].(map[string]any)["remote_url"], "unrelated fields kept verbatim")
}

// TestApplyCodexTaintHeaders_InvalidMetadata pins the fail-closed rule: an
// unparsable turn-metadata header must surface an error (caller rejects the
// request) instead of silently passing it through.
func TestApplyCodexTaintHeaders_InvalidMetadata(t *testing.T) {
	c := newSessionTaintTestContext(t, 7, "sess-h2")
	setOpenAICodexTaintSanitize(c, 42)
	h := http.Header{}
	h.Set("x-codex-turn-metadata", "not-json{")
	require.Error(t, applyCodexTaintHeaders(c, h, &Account{ID: 42}, 7))
}

// TestApplyCodexTaintHeaders_DisabledNoop verifies zero taint marker leaves
// headers untouched.
func TestApplyCodexTaintHeaders_DisabledNoop(t *testing.T) {
	c := newSessionTaintTestContext(t, 7, "sess-h3")
	h := http.Header{}
	h.Set("x-codex-window-id", "win-old")
	require.NoError(t, applyCodexTaintHeaders(c, h, &Account{ID: 42}, 7))
	require.Equal(t, "win-old", h.Get("x-codex-window-id"))
}

// TestApplyCodexTaintClientMetadata covers body client_metadata rewriting:
// top-level ids rederived, embedded turn-metadata JSON rekeyed, session_id ==
// thread_id equality kept, unrelated fields preserved, and fail-closed on an
// unparsable embedded JSON.
func TestApplyCodexTaintClientMetadata(t *testing.T) {
	c := newSessionTaintTestContext(t, 7, "sess-cm")
	setOpenAICodexTaintSanitize(c, 42)
	body := map[string]any{
		"client_metadata": map[string]any{
			"session_id":              "sess-old",
			"thread_id":               "sess-old",
			"turn_id":                 "turn-old",
			"x-codex-window-id":       "win-old",
			"x-codex-installation-id": "inst-old",
			"x-codex-turn-metadata":   `{"session_id":"sess-old","thread_id":"sess-old","cli_version":"0.146.0"}`,
		},
	}
	require.NoError(t, applyCodexTaintClientMetadata(c, body, &Account{ID: 42}, 7))

	cm := body["client_metadata"].(map[string]any)
	require.NotEqual(t, "sess-old", cm["session_id"])
	require.Equal(t, cm["session_id"], cm["thread_id"], "session==thread equality kept")
	require.NotEqual(t, "turn-old", cm["turn_id"])
	require.NotEqual(t, "win-old", cm["x-codex-window-id"])
	require.NotEqual(t, "inst-old", cm["x-codex-installation-id"])

	var meta map[string]any
	require.NoError(t, json.Unmarshal([]byte(cm["x-codex-turn-metadata"].(string)), &meta))
	require.Equal(t, cm["session_id"], meta["session_id"], "embedded metadata matches top-level session_id")
	require.Equal(t, "0.146.0", meta["cli_version"], "unrelated fields kept")

	// Fail-closed on unparsable embedded metadata.
	bad := map[string]any{
		"client_metadata": map[string]any{"x-codex-turn-metadata": "not-json{"},
	}
	require.Error(t, applyCodexTaintClientMetadata(c, bad, &Account{ID: 42}, 7))

	// Disabled marker → untouched.
	plain := map[string]any{
		"client_metadata": map[string]any{"session_id": "sess-old"},
	}
	require.NoError(t, applyCodexTaintClientMetadata(newSessionTaintTestContext(t, 7, "x"), plain, &Account{ID: 42}, 7))
	require.Equal(t, "sess-old", plain["client_metadata"].(map[string]any)["session_id"])
}

// TestTrackOpenAICodexSessionAttemptForTaint covers the handler-facing tracker:
// switch enabled → first attempt records without tainting, same-account retries
// stay clean, an account switch activates rekeying (marker = new account),
// stickiness re-activates on the next request's first attempt, and a disabled
// switch tracks nothing.
func TestTrackOpenAICodexSessionAttemptForTaint(t *testing.T) {
	enabled := func() *OpenAIGatewayService {
		return &OpenAIGatewayService{settingService: &SettingService{settingRepo: &fakeSettingRepo{vals: map[string]string{
			SettingKeyCodexSessionSwitchPurificationEnabled: "true",
		}}}}
	}

	svc := enabled()
	c := newSessionTaintTestContext(t, 7, "sess-track")

	// First attempt: records provenance, no rekey marker.
	require.False(t, svc.TrackOpenAICodexSessionAttemptForTaint(c, &Account{ID: 1}, 0))
	require.Zero(t, openAICodexTaintSanitizeAccountID(c))

	// Same-account retry: still clean.
	require.False(t, svc.TrackOpenAICodexSessionAttemptForTaint(c, &Account{ID: 1}, 1))
	require.Zero(t, openAICodexTaintSanitizeAccountID(c))

	// Account switch: rekey activated, marker = the new account.
	require.True(t, svc.TrackOpenAICodexSessionAttemptForTaint(c, &Account{ID: 2}, 1))
	require.Equal(t, int64(2), openAICodexTaintSanitizeAccountID(c))

	// Next request (fresh context), even back on account 1: sticky taint.
	c2 := newSessionTaintTestContext(t, 7, "sess-track")
	require.True(t, svc.TrackOpenAICodexSessionAttemptForTaint(c2, &Account{ID: 1}, 0))
	require.Equal(t, int64(1), openAICodexTaintSanitizeAccountID(c2))

	// Disabled switch: no tracking, no marker, even across accounts.
	svcOff := &OpenAIGatewayService{settingService: &SettingService{settingRepo: &fakeSettingRepo{vals: map[string]string{}}}}
	c3 := newSessionTaintTestContext(t, 7, "sess-off")
	require.False(t, svcOff.TrackOpenAICodexSessionAttemptForTaint(c3, &Account{ID: 1}, 0))
	require.False(t, svcOff.TrackOpenAICodexSessionAttemptForTaint(c3, &Account{ID: 2}, 1))
	require.Zero(t, openAICodexTaintSanitizeAccountID(c3))

	// Nil-safety.
	require.False(t, svc.TrackOpenAICodexSessionAttemptForTaint(nil, &Account{ID: 1}, 0))
}
