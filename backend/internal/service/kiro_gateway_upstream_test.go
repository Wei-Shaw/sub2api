//go:build unit

package service

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/pkg/kiro"
	"github.com/stretchr/testify/require"
)

func TestKiroCallEndpointSendsFingerprintHeaders(t *testing.T) {
	var gotUA, gotOptout, gotAuth, gotTokenType string
	var gotBody []byte

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotUA = r.Header.Get("User-Agent")
		gotOptout = r.Header.Get("x-amzn-codewhisperer-optout")
		gotAuth = r.Header.Get("Authorization")
		gotTokenType = r.Header.Get("tokentype")
		gotBody, _ = io.ReadAll(r.Body)
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	svc := &KiroGatewayService{}
	account := &Account{ID: 1, Platform: PlatformKiro, Credentials: map[string]any{
		"auth_method":  "social",
		"access_token": "at_1",
		"machine_id":   "stable-machine",
	}}
	ep := kiro.Endpoint{URL: srv.URL, Origin: "AI_EDITOR", Name: "test"}

	resp, err := svc.callEndpoint(context.Background(), account, ep, []byte(`{"a":1}`))
	require.NoError(t, err)
	defer func() { _ = resp.Body.Close() }()

	require.Equal(t, http.StatusOK, resp.StatusCode)
	require.Contains(t, gotUA, "stable-machine")
	require.Equal(t, "true", gotOptout)
	require.Equal(t, "Bearer at_1", gotAuth)
	require.Empty(t, gotTokenType)
	require.JSONEq(t, `{"a":1}`, string(gotBody))
}

func TestKiroCallEndpointAPIKeyAccountSendsTokenType(t *testing.T) {
	var gotTokenType, gotAuth string

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotTokenType = r.Header.Get("tokentype")
		gotAuth = r.Header.Get("Authorization")
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	svc := &KiroGatewayService{}
	account := &Account{ID: 2, Platform: PlatformKiro, Credentials: map[string]any{
		"auth_method": "api_key",
		"api_key":     "kiro_ak_9",
	}}
	ep := kiro.Endpoint{URL: srv.URL, Origin: "KIRO_CLI", Name: "cli"}

	resp, err := svc.callEndpoint(context.Background(), account, ep, []byte(`{}`))
	require.NoError(t, err)
	defer func() { _ = resp.Body.Close() }()

	require.Equal(t, "API_KEY", gotTokenType)
	require.Equal(t, "Bearer kiro_ak_9", gotAuth)
}

// TestKiroCallEndpointGeneratesAndPersistsMachineID 覆盖首次调用时的指纹固化。
func TestKiroCallEndpointGeneratesAndPersistsMachineID(t *testing.T) {
	var seenUA []string

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		seenUA = append(seenUA, r.Header.Get("User-Agent"))
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	svc := &KiroGatewayService{}
	account := &Account{ID: 3, Platform: PlatformKiro, Credentials: map[string]any{
		"auth_method":  "social",
		"access_token": "at",
	}}
	ep := kiro.Endpoint{URL: srv.URL, Origin: "AI_EDITOR", Name: "test"}

	for i := 0; i < 2; i++ {
		resp, err := svc.callEndpoint(context.Background(), account, ep, []byte(`{}`))
		require.NoError(t, err)
		_ = resp.Body.Close()
	}

	require.Len(t, seenUA, 2)
	require.Equal(t, seenUA[0], seenUA[1], "同一账号两次请求的指纹必须一致")
	require.NotEmpty(t, account.Credentials["machine_id"], "生成的指纹必须写回 credentials 供调用方落库")
}
