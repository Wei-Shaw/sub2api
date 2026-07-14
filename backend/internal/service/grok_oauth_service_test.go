//go:build unit

package service

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/xai"
	"github.com/stretchr/testify/require"
)

type grokOAuthClientStub struct {
	deviceResponse  *xai.DeviceCodeResponse
	pollResponse    *xai.DevicePollResult
	refreshResponse *xai.TokenResponse
	ssoResponse     *xai.TokenResponse
	exchangeCalls   int
	deviceCalls     int
	pollCalls       int
}

func (s *grokOAuthClientStub) RequestDeviceCode(context.Context, string, string, string) (*xai.DeviceCodeResponse, error) {
	s.deviceCalls++
	if s.deviceResponse != nil {
		return s.deviceResponse, nil
	}
	return &xai.DeviceCodeResponse{
		DeviceCode:              "device-code",
		UserCode:                "ABCD-EFGH",
		VerificationURI:         "https://auth.x.ai/oauth2/device",
		VerificationURIComplete: "https://auth.x.ai/oauth2/device?user_code=ABCD-EFGH",
		ExpiresIn:               600,
		Interval:                5,
	}, nil
}

func (s *grokOAuthClientStub) PollDeviceToken(context.Context, string, string, string) (*xai.DevicePollResult, error) {
	s.pollCalls++
	if s.pollResponse != nil {
		return s.pollResponse, nil
	}
	return &xai.DevicePollResult{Status: xai.DevicePollPending}, nil
}

func (s *grokOAuthClientStub) ExchangeCode(context.Context, string, string, string, string, string) (*xai.TokenResponse, error) {
	s.exchangeCalls++
	return &xai.TokenResponse{}, nil
}

func (s *grokOAuthClientStub) RefreshToken(context.Context, string, string, string) (*xai.TokenResponse, error) {
	return s.refreshResponse, nil
}

func (s *grokOAuthClientStub) ConvertSSOToBuild(context.Context, string, string) (*xai.TokenResponse, error) {
	return s.ssoResponse, nil
}

func TestGrokOAuthServiceRefreshTokenPreservesOriginalRefreshTokenWhenNotRotated(t *testing.T) {
	svc := NewGrokOAuthService(nil, &grokOAuthClientStub{
		refreshResponse: &xai.TokenResponse{
			AccessToken: "new-access-token",
			TokenType:   "Bearer",
			ExpiresIn:   3600,
		},
	})
	defer svc.Stop()

	info, err := svc.RefreshToken(context.Background(), "original-refresh-token", "", "client-id")
	require.NoError(t, err)
	require.Equal(t, "new-access-token", info.AccessToken)
	require.Equal(t, "original-refresh-token", info.RefreshToken)
	require.Equal(t, "client-id", info.ClientID)
}

func TestGrokOAuthServiceGenerateAuthURLUsesDeviceFlow(t *testing.T) {
	client := &grokOAuthClientStub{}
	svc := NewGrokOAuthService(nil, client)
	defer svc.Stop()

	auth, err := svc.GenerateAuthURL(context.Background(), nil, "")
	require.NoError(t, err)
	require.Equal(t, 1, client.deviceCalls)
	require.Equal(t, xai.OAuthFlowDevice, auth.Flow)
	require.Equal(t, "ABCD-EFGH", auth.UserCode)
	require.Contains(t, auth.AuthURL, "user_code=ABCD-EFGH")
	require.NotEmpty(t, auth.SessionID)
	require.NotEmpty(t, auth.State)
}

func TestGrokOAuthServicePollDeviceLoginAuthorizesAndConsumesSession(t *testing.T) {
	client := &grokOAuthClientStub{
		pollResponse: &xai.DevicePollResult{
			Status: xai.DevicePollAuthorized,
			Token: &xai.TokenResponse{
				AccessToken:  "access-token",
				RefreshToken: "refresh-token",
				TokenType:    "Bearer",
				ExpiresIn:    3600,
			},
		},
	}
	svc := NewGrokOAuthService(nil, client)
	defer svc.Stop()

	auth, err := svc.GenerateAuthURL(context.Background(), nil, "")
	require.NoError(t, err)

	poll, err := svc.PollDeviceLogin(context.Background(), &GrokDevicePollInput{SessionID: auth.SessionID})
	require.NoError(t, err)
	require.Equal(t, string(xai.DevicePollAuthorized), poll.Status)
	require.NotNil(t, poll.Token)
	require.Equal(t, "access-token", poll.Token.AccessToken)
	require.Equal(t, "refresh-token", poll.Token.RefreshToken)
	require.Equal(t, 1, client.pollCalls)

	_, err = svc.PollDeviceLogin(context.Background(), &GrokDevicePollInput{SessionID: auth.SessionID})
	require.Error(t, err)
	require.Contains(t, err.Error(), "GROK_OAUTH_SESSION_NOT_FOUND")
}

func TestGrokOAuthServiceExchangeCodeCompletesDeviceSession(t *testing.T) {
	client := &grokOAuthClientStub{
		pollResponse: &xai.DevicePollResult{
			Status: xai.DevicePollAuthorized,
			Token: &xai.TokenResponse{
				AccessToken:  "access-token",
				RefreshToken: "refresh-token",
				ExpiresIn:    3600,
			},
		},
	}
	svc := NewGrokOAuthService(nil, client)
	defer svc.Stop()

	auth, err := svc.GenerateAuthURL(context.Background(), nil, "")
	require.NoError(t, err)

	info, err := svc.ExchangeCode(context.Background(), &GrokExchangeCodeInput{
		SessionID: auth.SessionID,
	})
	require.NoError(t, err)
	require.Equal(t, "access-token", info.AccessToken)
	require.Equal(t, "refresh-token", info.RefreshToken)
	require.Zero(t, client.exchangeCalls)
	require.Equal(t, 1, client.pollCalls)
}

func TestGrokOAuthServiceExchangeCodePendingDeviceAuthorization(t *testing.T) {
	client := &grokOAuthClientStub{
		pollResponse: &xai.DevicePollResult{Status: xai.DevicePollPending},
	}
	svc := NewGrokOAuthService(nil, client)
	defer svc.Stop()

	auth, err := svc.GenerateAuthURL(context.Background(), nil, "")
	require.NoError(t, err)

	_, err = svc.ExchangeCode(context.Background(), &GrokExchangeCodeInput{SessionID: auth.SessionID})
	require.Error(t, err)
	require.Contains(t, err.Error(), "GROK_OAUTH_AUTHORIZATION_PENDING")
}

func TestGrokOAuthServiceBuildAccountCredentialsDefaultsToSubscriptionProxy(t *testing.T) {
	svc := NewGrokOAuthService(nil, &grokOAuthClientStub{})
	defer svc.Stop()

	credentials := svc.BuildAccountCredentials(&GrokTokenInfo{
		AccessToken: "access-token",
		ExpiresAt:   time.Now().Add(time.Hour).Unix(),
	})

	require.Equal(t, xai.DefaultCLIBaseURL, credentials["base_url"])
}

func TestGrokOAuthServiceConvertFromSSOExtractsBuildClaims(t *testing.T) {
	svc := NewGrokOAuthService(nil, &grokOAuthClientStub{
		ssoResponse: &xai.TokenResponse{
			AccessToken:  makeGrokOAuthJWT(map[string]any{"sub": "user-sub", "team_id": "team-1"}),
			RefreshToken: "refresh-token",
			IDToken:      makeGrokOAuthJWT(map[string]any{"email": "user@example.com"}),
			ExpiresIn:    3600,
		},
	})
	defer svc.Stop()

	info, err := svc.ConvertFromSSO(context.Background(), "sso-token", nil)
	require.NoError(t, err)
	require.Equal(t, "user@example.com", info.Email)
	require.Equal(t, "user-sub", info.Subject)
	require.Equal(t, "team-1", info.TeamID)

	credentials := svc.BuildAccountCredentials(info)
	require.Equal(t, "user@example.com", credentials["email"])
	require.Equal(t, "user-sub", credentials["sub"])
	require.Equal(t, "team-1", credentials["team_id"])
}

func makeGrokOAuthJWT(claims map[string]any) string {
	payload, _ := json.Marshal(claims)
	return "header." + base64.RawURLEncoding.EncodeToString(payload) + ".signature"
}
