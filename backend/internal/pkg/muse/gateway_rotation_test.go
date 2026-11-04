//go:build unit

package muse

import (
	"context"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestGatewayAtomicRotationHonorsCompleteCookieResponse(t *testing.T) {
	session := Session{Document: map[string]any{"cookies": map[string]any{"hatch_sess": "synthetic-session", "hatch_gw": "synthetic-gateway", "hatch_native_auth_device": "synthetic-device"}, "opaque": map[string]any{"preserve": true}}}
	expires := time.Now().Add(2 * time.Hour).UTC().Truncate(time.Second)
	client := GatewayClient{Do: func(*http.Request, Session) (*http.Response, error) {
		return &http.Response{StatusCode: 200, Header: http.Header{"Set-Cookie": {
			"hatch_gw=; Path=/; Max-Age=0; Secure; HttpOnly",
			(&http.Cookie{Name: "hatch_gw", Value: "rotated-gateway", Path: "/", Domain: ".muse.ai", Expires: expires, Secure: true, HttpOnly: true}).String(),
			"hatch_sess=untrusted; Domain=elsewhere.example; Path=/; Secure; HttpOnly",
		}}, Body: io.NopCloser(strings.NewReader(`{"token":"synthetic-token"}`))}, nil
	}}
	token, document, err := client.TokenAndRefresh(context.Background(), session, testGatewayTarget())
	require.NoError(t, err)
	require.Equal(t, "synthetic-token", token.Token)
	cookies, err := ParseCookieSession(document)
	require.NoError(t, err)
	require.Equal(t, "rotated-gateway", cookies.Cookies["hatch_gw"])
	require.Equal(t, expires.Unix(), cookies.Expires["hatch_gw"])
	require.Equal(t, "synthetic-session", cookies.Cookies["hatch_sess"])
	require.Equal(t, session.Document["opaque"], document["opaque"])
	original, err := ParseCookieSession(session.Document)
	require.NoError(t, err)
	require.Equal(t, "synthetic-gateway", original.Cookies["hatch_gw"])
	_, err = client.Token(context.Background(), session, testGatewayTarget())
	require.ErrorIs(t, err, ErrSessionResponse, "a caller unable to persist rotation must still reject it")
}

func TestGatewayAtomicRotationDoesNotInventExpiryOrIgnoreDeletion(t *testing.T) {
	session := Session{Document: map[string]any{"cookies": map[string]any{"hatch_sess": "synthetic-session", "hatch_gw": "synthetic-gateway", "hatch_native_auth_device": "synthetic-device"}}}
	for _, cookie := range []string{"hatch_gw=; Path=/; Max-Age=0; Secure; HttpOnly", "hatch_gw=rotation; Path=/; Secure; HttpOnly"} {
		client := GatewayClient{Do: func(*http.Request, Session) (*http.Response, error) {
			return &http.Response{StatusCode: 200, Header: http.Header{"Set-Cookie": {cookie}}, Body: io.NopCloser(strings.NewReader(`{"token":"synthetic-token"}`))}, nil
		}}
		_, document, err := client.TokenAndRefresh(context.Background(), session, testGatewayTarget())
		if strings.Contains(cookie, "Max-Age=0") {
			require.ErrorIs(t, err, ErrSessionExpired)
			continue
		}
		require.NoError(t, err)
		cookies, err := ParseCookieSession(document)
		require.NoError(t, err)
		_, known := cookies.Expires["hatch_gw"]
		require.False(t, known)
	}
}
