package service

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

func TestFirstServeFingerprintLifetime(t *testing.T) {
	a := firstServeHTTPAccount(t)
	a.Type = AccountTypeOAuth
	a.Extra = map[string]any{
		"openai_oauth_responses_websockets_v2_mode": OpenAIWSIngressModeFirstServe,
		"openai_first_serve":                        map[string]any{"rotate_seconds": 120},
		codexFingerprintModeExtraKey:                "full", codexFingerprintSeedExtraKey: testCodexFingerprintSeed,
		"openai_device_id": "saved-device", "openai_passthrough": false,
	}
	svc := &OpenAIGatewayService{}
	body := []byte(`{"previous_response_id":"resp_own","input":"hello","client_metadata":{"installation_id":"old","x-codex-turn-metadata":"{\"installation_id\":\"old\"}"}}`)
	ctx, selected, first, err := svc.prepareFirstServeHTTP(context.Background(), firstServeHTTPContext(1, "one"), a, body)
	require.NoError(t, err)
	defer first.finish(nil, nil)
	require.NotEmpty(t, first.fingerprint)
	require.NotEqual(t, "saved-device", first.fingerprint)
	require.Equal(t, first.started.Add(120*time.Second), first.entry.state.status.ExpiresAt)

	// Exercise the normal convergence pass followed by final routing rewriting.
	wire := func(ctx context.Context, account *Account) (string, string) {
		t.Helper()
		ids := resolveCodexFingerprintIDsFromRequest(account, nil)
		payload, _, err := applyCodexFingerprintClientMetadataRaw(body, ids)
		require.NoError(t, err)
		req, err := http.NewRequestWithContext(ctx, http.MethodPost, "https://example.com/v1/responses", bytes.NewReader(payload))
		require.NoError(t, err)
		req.Header.Set("x-codex-turn-metadata", `{"installation_id":"old"}`)
		applyCodexFingerprintHeaders(req.Header, ids)
		require.NoError(t, applyFirstServeHTTPRequest(req, account))
		forwarded, err := io.ReadAll(req.Body)
		require.NoError(t, err)
		_ = req.Body.Close()
		fingerprint := req.Header.Get("x-codex-installation-id")
		require.Equal(t, fingerprint, gjson.Get(req.Header.Get("x-codex-turn-metadata"), "installation_id").String())
		require.Equal(t, fingerprint, gjson.GetBytes(forwarded, "client_metadata.x-codex-installation-id").String())
		require.Equal(t, fingerprint, gjson.GetBytes(forwarded, "client_metadata.installation_id").String())
		require.Equal(t, fingerprint, gjson.Get(gjson.GetBytes(forwarded, "client_metadata.x-codex-turn-metadata").String(), "installation_id").String())
		require.Equal(t, "resp_own", gjson.GetBytes(forwarded, "previous_response_id").String())
		require.Equal(t, "hello", gjson.GetBytes(forwarded, "input").String())
		require.Equal(t, first.id, gjson.GetBytes(forwarded, "prompt_cache_key").String())
		return fingerprint, req.Header.Get("session_id")
	}
	fingerprint, routeID := wire(ctx, selected)
	require.Equal(t, first.fingerprint, fingerprint)
	expires := first.entry.state.status.ExpiresAt
	for i := range 3 {
		nextCtx, nextAccount, next, err := svc.prepareFirstServeHTTP(context.Background(), firstServeHTTPContext(int64(i+2), "other"), a, body)
		require.NoError(t, err)
		fp, id := wire(nextCtx, nextAccount)
		require.Equal(t, fingerprint, fp, "all sessions reuse the current device")
		require.Equal(t, routeID, id)
		require.Equal(t, expires, next.entry.state.status.ExpiresAt, "reuse must not extend the lifetime")
		next.finish(nil, nil)
	}
	first.entry.state.status.ExpiresAt = time.Now().Add(-time.Second)
	nextCtx, nextAccount, next, err := svc.prepareFirstServeHTTP(context.Background(), firstServeHTTPContext(5, "next"), a, body)
	require.NoError(t, err)
	defer next.finish(nil, nil)
	newFingerprint, newRoute := wire(nextCtx, nextAccount)
	require.NotEqual(t, fingerprint, newFingerprint)
	require.Equal(t, routeID, newRoute)
	require.NotEqual(t, selected.Proxy.ID, nextAccount.Proxy.ID)
	require.Equal(t, next.started.Add(120*time.Second), next.entry.state.status.ExpiresAt)
	oldFingerprint, oldRoute := wire(ctx, selected)
	require.Equal(t, fingerprint, oldFingerprint, "in-flight requests and retries keep their captured device")
	require.Equal(t, routeID, oldRoute)

	// The resolver has no alternative after B: retain IP and fingerprint together.
	next.entry.state.status.ExpiresAt = time.Now().Add(-time.Second)
	_, unchangedAccount, unchanged, err := svc.prepareFirstServeHTTP(context.Background(), firstServeHTTPContext(6, "failed-rotation"), a, body)
	require.NoError(t, err)
	require.Equal(t, newFingerprint, unchanged.fingerprint)
	require.Equal(t, nextAccount.Proxy.ID, unchangedAccount.Proxy.ID)
	unchanged.finish(nil, nil)
	require.Equal(t, testCodexFingerprintSeed, a.Extra[codexFingerprintSeedExtraKey])
	require.Equal(t, "saved-device", a.Extra["openai_device_id"])
	require.Equal(t, false, a.Extra["openai_passthrough"])
}

func TestFirstServeFingerprintConcurrentRotation(t *testing.T) {
	a := firstServeHTTPAccount(t)
	a.Type = AccountTypeOAuth
	a.Extra = map[string]any{"openai_oauth_responses_websockets_v2_mode": OpenAIWSIngressModeFirstServe, codexFingerprintModeExtraKey: "full"}
	svc := &OpenAIGatewayService{}
	_, _, old, err := svc.prepareFirstServeHTTP(context.Background(), firstServeHTTPContext(1, "old"), a, nil)
	require.NoError(t, err)
	old.finish(nil, nil)
	old.entry.state.status.ExpiresAt = time.Now().Add(-time.Second)
	type snapshot struct {
		fingerprint, id string
		proxy           int64
		err             error
	}
	results := make(chan snapshot, 16)
	var wg sync.WaitGroup
	for i := range 16 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, selected, lease, err := svc.prepareFirstServeHTTP(context.Background(), firstServeHTTPContext(int64(i+2), "parallel"), a, nil)
			if err != nil {
				results <- snapshot{err: err}
				return
			}
			results <- snapshot{fingerprint: lease.fingerprint, id: lease.id, proxy: selected.Proxy.ID}
			lease.finish(nil, nil)
		}()
	}
	wg.Wait()
	close(results)
	identities := make(map[string]bool)
	for result := range results {
		require.NoError(t, result.err)
		require.NotEmpty(t, result.fingerprint)
		require.NotEqual(t, old.fingerprint, result.fingerprint)
		require.Equal(t, old.id, result.id)
		require.Equal(t, int64(102), result.proxy)
		identities[result.fingerprint] = true
	}
	require.Len(t, identities, 1, "concurrent requests must rotate IP and device only once")
	require.Equal(t, 1, old.entry.state.status.Rotations)

	a.Extra[codexFingerprintModeExtraKey] = "off"
	_, unchanged, off, err := svc.prepareFirstServeHTTP(context.Background(), firstServeHTTPContext(1, "off"), a, nil)
	require.NoError(t, err)
	defer off.finish(nil, nil)
	require.Empty(t, off.fingerprint)
	require.NotContains(t, unchanged.Extra, codexFingerprintSeedExtraKey)
	headers := http.Header{"X-Codex-Installation-Id": {"client-device"}}
	applyFirstServeHeaders(headers, off)
	require.Equal(t, "client-device", headers.Get("x-codex-installation-id"))
}
