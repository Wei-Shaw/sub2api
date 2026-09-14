//go:build unit

package service

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/adobe"
	"github.com/stretchr/testify/require"
)

type adobeGatewayCredsRepo struct {
	AccountRepository
	updated map[string]any
}

func (r *adobeGatewayCredsRepo) UpdateCredentials(_ context.Context, _ int64, credentials map[string]any) error {
	r.updated = credentials
	return nil
}

func TestGetOAuthTokenRefreshesExpiredAdobeToken(t *testing.T) {
	newToken := adobeTestToken(t, map[string]any{
		"user_id": "u1", "exp": time.Now().Add(24 * time.Hour).Unix(),
	})
	refresher := newAdobeTestRefresher(t, func(*adobe.Request, int) (*adobe.Response, error) {
		body, _ := json.Marshal(map[string]any{"access_token": newToken, "expires_in": 86400})
		return &adobe.Response{StatusCode: 200, Headers: map[string]string{}, Body: body}, nil
	})
	repo := &adobeGatewayCredsRepo{}
	svc := &GatewayService{accountRepo: repo, adobeTokenRefresher: refresher}

	account := &Account{
		ID: 11, Platform: PlatformAdobe, Type: AccountTypeOAuth,
		Credentials: map[string]any{
			"cookie":       "aux_sid=abc",
			"access_token": adobeTestToken(t, map[string]any{"exp": time.Now().Add(-time.Hour).Unix()}),
		},
	}

	token, kind, err := svc.getOAuthToken(context.Background(), account)
	require.NoError(t, err)
	require.Equal(t, "oauth", kind)
	require.Equal(t, newToken, token)
	require.Equal(t, newToken, repo.updated["access_token"])
	require.Equal(t, newToken, account.GetCredential("access_token"))
}

func TestGetOAuthTokenSkipsFreshAdobeToken(t *testing.T) {
	fresh := adobeTestToken(t, map[string]any{"exp": time.Now().Add(24 * time.Hour).Unix()})
	refresher := newAdobeTestRefresher(t, func(*adobe.Request, int) (*adobe.Response, error) {
		t.Fatal("fresh token must not hit IMS")
		return nil, nil
	})
	svc := &GatewayService{adobeTokenRefresher: refresher}

	token, kind, err := svc.getOAuthToken(context.Background(), &Account{
		ID: 12, Platform: PlatformAdobe, Type: AccountTypeOAuth,
		Credentials: map[string]any{"cookie": "aux_sid=abc", "access_token": fresh},
	})
	require.NoError(t, err)
	require.Equal(t, "oauth", kind)
	require.Equal(t, fresh, token)
}

func TestGetOAuthTokenReturnsExpiredAdobeTokenWithoutCookie(t *testing.T) {
	stale := adobeTestToken(t, map[string]any{"exp": time.Now().Add(-time.Hour).Unix()})
	refresher := newAdobeTestRefresher(t, func(*adobe.Request, int) (*adobe.Response, error) {
		t.Fatal("no cookie means refresh must not be attempted")
		return nil, nil
	})
	svc := &GatewayService{adobeTokenRefresher: refresher}

	token, kind, err := svc.getOAuthToken(context.Background(), &Account{
		ID: 13, Platform: PlatformAdobe, Type: AccountTypeOAuth,
		Credentials: map[string]any{"access_token": stale},
	})
	require.NoError(t, err)
	require.Equal(t, "oauth", kind)
	require.Equal(t, stale, token)
}
