//go:build unit

package muse

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/url"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func testGatewayTarget() GatewayTarget {
	id := "11111111-2222-4333-8444-555555555555"
	return GatewayTarget{VMID: id, Name: id, URL: "wss://" + id + ".metaaivm.com/"}
}

func TestGatewayRejectsAlternateCredentialTargets(t *testing.T) {
	target := testGatewayTarget()
	for _, bad := range []string{"http://127.0.0.1/", "wss://evil-metaaivm.com/", target.URL + "?auth_token=secret", "wss://user:pass@" + target.VMID + ".metaaivm.com/", strings.Replace(target.URL, "/", "/ws", 1)} {
		t.Run(bad, func(t *testing.T) {
			copy := target
			copy.URL = bad
			client := GatewayClient{Do: func(*http.Request, Session) (*http.Response, error) {
				t.Fatal("must not send credentials")
				return nil, nil
			}}
			_, err := client.Token(context.Background(), Session{}, copy)
			require.ErrorIs(t, err, ErrSessionResponse)
		})
	}
	body, _ := json.Marshal(map[string]string{"status": "assigned", "vm_id": target.VMID, "vm_name": target.Name, "endpoint_url": target.URL})
	parsed, err := ParseGatewayTarget(body)
	require.NoError(t, err)
	require.Equal(t, target, *parsed)
	_, err = ParseGatewayTarget([]byte(`{"status":"waiting"}`))
	require.ErrorIs(t, err, ErrSessionResponse)
}

func TestGatewayTokenUsesObservedHeadersAndAccountProxy(t *testing.T) {
	session := Session{Document: map[string]any{"cookies": map[string]any{"hatch_sess": "synthetic-session", "hatch_gw": "synthetic-gateway", "hatch_native_auth_device": "synthetic-device"}}, ProxyURL: "http://configured-proxy:8080"}
	target := testGatewayTarget()
	client := GatewayClient{Do: func(req *http.Request, actual Session) (*http.Response, error) {
		require.Equal(t, session.ProxyURL, actual.ProxyURL)
		require.Equal(t, GatewayTokenEndpoint, req.URL.String())
		require.Equal(t, http.MethodPost, req.Method)
		require.Equal(t, "same-origin", req.Header.Get("Sec-Fetch-Site"))
		require.Equal(t, "cors", req.Header.Get("Sec-Fetch-Mode"))
		require.Equal(t, "empty", req.Header.Get("Sec-Fetch-Dest"))
		require.Equal(t, "https://muse.ai", req.Header.Get("Origin"))
		require.Len(t, req.Cookies(), 3)
		require.NotContains(t, req.Header.Get("Cookie"), "hatch_vml=")
		var body map[string]string
		require.NoError(t, json.NewDecoder(req.Body).Decode(&body))
		require.Equal(t, map[string]string{"vmAddress": target.URL, "vmName": target.Name}, body)
		_, bounded := req.Context().Deadline()
		require.True(t, bounded)
		return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(`{"token":"synthetic-token","notary_token":"synthetic-notary"}`))}, nil
	}}
	token, err := client.Token(context.Background(), session, target)
	require.NoError(t, err)
	endpoint, err := token.WebSocketURL(target, "aaaaaaaa-bbbb-4ccc-8ddd-eeeeeeeeeeee")
	require.NoError(t, err)
	u, err := url.Parse(endpoint)
	require.NoError(t, err)
	require.Equal(t, "hatch.metaaivm.com", u.Host)
	require.Equal(t, "/v1/noise", u.Path)
	require.Equal(t, target.VMID, u.Query().Get("vm_id"))
	require.Equal(t, "synthetic-token", u.Query().Get("auth_token"))
	require.Equal(t, "hatch-web", u.Query().Get("app_id"))
}

func TestGatewayRejectsDelegationRotationAndUnsafeErrors(t *testing.T) {
	session := Session{Document: map[string]any{"cookies": map[string]any{"hatch_sess": "synthetic-session", "hatch_gw": "synthetic-gateway", "hatch_native_auth_device": "synthetic-device"}}}
	for _, test := range []struct {
		name     string
		status   int
		body     string
		cookie   string
		err      error
		expected error
	}{
		{"expired", 401, "", "", nil, ErrSessionExpired},
		{"forbidden remains unverified", 403, `{"error":"Forbidden"}`, "", nil, ErrSessionResponse},
		{"redirect", 302, "", "", nil, ErrSessionResponse},
		{"secret transport error", 0, "", "", errors.New("https://secret-token.example/?auth_token=synthetic-secret"), ErrSessionResponse},
		{"invalid credential", 200, `{"token":"newline\nsecret"}`, "", nil, ErrSessionResponse},
		{"notary delegation", 200, `{"token":"ok","notary_token":"delegation.unqualified"}`, "", nil, ErrNoiseTrust},
		{"cookie rotation requires atomic renewal", 200, `{"token":"ok"}`, "hatch_gw=replacement; Path=/; Secure; HttpOnly", nil, ErrSessionResponse},
	} {
		t.Run(test.name, func(t *testing.T) {
			client := GatewayClient{Do: func(*http.Request, Session) (*http.Response, error) {
				if test.err != nil {
					return nil, test.err
				}
				h := http.Header{}
				if test.cookie != "" {
					h.Set("Set-Cookie", test.cookie)
				}
				return &http.Response{StatusCode: test.status, Header: h, Body: io.NopCloser(strings.NewReader(test.body))}, nil
			}}
			_, err := client.Token(context.Background(), session, testGatewayTarget())
			require.ErrorIs(t, err, test.expected)
			require.NotContains(t, err.Error(), "synthetic-secret")
		})
	}
}

func TestStandardNoisePeerBindsStaticKeyAndFreshNonce(t *testing.T) {
	peer := NoisePeer{StaticKey: []byte(strings.Repeat("k", 32)), ClientNonce: []byte(strings.Repeat("n", 32)), HandshakeHash: make([]byte, 32)}
	statement := noiseBytes(noiseBytes(nil, 1, peer.StaticKey), 2, peer.ClientNonce)
	peer.Attestation = noiseBytes(nil, 2, statement)
	require.False(t, noiseOwnerChallenge(peer.Attestation))
	require.NoError(t, VerifyStandardNoisePeer(context.Background(), peer))
	peer.ClientNonce = []byte(strings.Repeat("m", 32))
	require.ErrorIs(t, VerifyStandardNoisePeer(context.Background(), peer), ErrNoiseTrust)
	peer.ClientNonce = []byte(strings.Repeat("n", 32))
	peer.StaticKey = []byte(strings.Repeat("j", 32))
	require.ErrorIs(t, VerifyStandardNoisePeer(context.Background(), peer), ErrNoiseTrust)
	peer.Attestation = nil
	require.ErrorIs(t, VerifyStandardNoisePeer(context.Background(), peer), ErrNoiseTrust)
}
