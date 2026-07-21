//go:build unit

package service

import (
	"context"
	"net/http"
	"testing"

	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
	"github.com/Wei-Shaw/sub2api/internal/pkg/kimi"
	"github.com/stretchr/testify/require"
)

type kimiOAuthClientStub struct {
	deviceAuthResponse *kimi.DeviceAuthResponse
	deviceAuthErr      error
	pollResponse       *kimi.TokenResponse
	pollErr            error
	refreshResponse    *kimi.TokenResponse
	refreshErr         error

	deviceAuthCalls     int
	pollCalls           int
	refreshCalls        int
	lastDeviceAuthDevID string
	lastPollDeviceCode  string
	lastPollProxyURL    string
	lastPollDeviceID    string
	lastRefreshToken    string
	lastRefreshDeviceID string
}

func (s *kimiOAuthClientStub) DeviceAuthorization(_ context.Context, _, deviceID string) (*kimi.DeviceAuthResponse, error) {
	s.deviceAuthCalls++
	s.lastDeviceAuthDevID = deviceID
	return s.deviceAuthResponse, s.deviceAuthErr
}

func (s *kimiOAuthClientStub) PollDeviceToken(_ context.Context, deviceCode, proxyURL, deviceID string) (*kimi.TokenResponse, error) {
	s.pollCalls++
	s.lastPollDeviceCode = deviceCode
	s.lastPollProxyURL = proxyURL
	s.lastPollDeviceID = deviceID
	return s.pollResponse, s.pollErr
}

func (s *kimiOAuthClientStub) RefreshToken(_ context.Context, refreshToken, _, deviceID string) (*kimi.TokenResponse, error) {
	s.refreshCalls++
	s.lastRefreshToken = refreshToken
	s.lastRefreshDeviceID = deviceID
	return s.refreshResponse, s.refreshErr
}

// startKimiDeviceSession 发起一次设备码登录并返回 service 与 sessionID。
func startKimiDeviceSession(t *testing.T, client *kimiOAuthClientStub) (*KimiOAuthService, string) {
	t.Helper()
	if client.deviceAuthResponse == nil {
		client.deviceAuthResponse = &kimi.DeviceAuthResponse{
			DeviceCode:              "device-code-123",
			UserCode:                "ABCD-EFGH",
			VerificationURI:         "https://kimi.com/device",
			VerificationURIComplete: "https://kimi.com/device?code=ABCD-EFGH",
			Interval:                3,
			ExpiresIn:               1800,
		}
	}
	svc := NewKimiOAuthService(nil, client)
	t.Cleanup(svc.Stop)
	result, err := svc.StartDeviceAuth(context.Background(), nil)
	require.NoError(t, err)
	require.NotEmpty(t, result.SessionID)
	return svc, result.SessionID
}

func TestKimiOAuthServiceStartDeviceAuthReturnsSessionAndGeneratesDeviceID(t *testing.T) {
	client := &kimiOAuthClientStub{}
	svc, sessionID := startKimiDeviceSession(t, client)

	require.Equal(t, 1, client.deviceAuthCalls)
	require.Len(t, client.lastDeviceAuthDevID, 32, "device_id 应为 32 位 hex")

	result, err := svc.GetSessionToken(sessionID)
	require.Error(t, err, "未完成轮询前 GetSessionToken 应报 pending")
	require.Nil(t, result)
	require.Equal(t, "KIMI_OAUTH_AUTHORIZATION_PENDING", infraerrors.Reason(err))

	// 会话已保存：轮询时透传 device_code 与同一 device_id
	client.pollErr = &kimi.OAuthError{StatusCode: http.StatusBadRequest, Code: kimi.OAuthErrorAuthorizationPending}
	_, err = svc.PollDeviceToken(context.Background(), sessionID, nil)
	require.Error(t, err)
	require.Equal(t, "device-code-123", client.lastPollDeviceCode)
	require.Equal(t, client.lastDeviceAuthDevID, client.lastPollDeviceID)
}

func TestKimiOAuthServicePollDeviceTokenPendingKeepsSession(t *testing.T) {
	client := &kimiOAuthClientStub{
		pollErr: &kimi.OAuthError{StatusCode: http.StatusBadRequest, Code: kimi.OAuthErrorAuthorizationPending},
	}
	svc, sessionID := startKimiDeviceSession(t, client)

	_, err := svc.PollDeviceToken(context.Background(), sessionID, nil)
	require.Error(t, err)
	require.Equal(t, "KIMI_OAUTH_AUTHORIZATION_PENDING", infraerrors.Reason(err))
	require.Equal(t, "3", infraerrors.FromError(err).Metadata["interval"])

	// pending 不销毁会话：授权完成后可继续轮询成功
	client.pollErr = nil
	client.pollResponse = &kimi.TokenResponse{AccessToken: "access-token", RefreshToken: "refresh-token", ExpiresIn: 900}
	info, err := svc.PollDeviceToken(context.Background(), sessionID, nil)
	require.NoError(t, err)
	require.Equal(t, "access-token", info.AccessToken)
	require.Equal(t, 2, client.pollCalls)
}

func TestKimiOAuthServicePollDeviceTokenSlowDownSuggestsLongerInterval(t *testing.T) {
	client := &kimiOAuthClientStub{
		pollErr: &kimi.OAuthError{StatusCode: http.StatusBadRequest, Code: kimi.OAuthErrorSlowDown},
	}
	svc, sessionID := startKimiDeviceSession(t, client)

	_, err := svc.PollDeviceToken(context.Background(), sessionID, nil)
	require.Error(t, err)
	require.Equal(t, "KIMI_OAUTH_SLOW_DOWN", infraerrors.Reason(err))
	// 建议间隔 = 会话 interval(3) + SlowDownExtraSeconds(5)
	require.Equal(t, "8", infraerrors.FromError(err).Metadata["suggested_interval"])
}

func TestKimiOAuthServicePollDeviceTokenExpiredDeletesSession(t *testing.T) {
	client := &kimiOAuthClientStub{
		pollErr: &kimi.OAuthError{StatusCode: http.StatusBadRequest, Code: kimi.OAuthErrorExpiredToken},
	}
	svc, sessionID := startKimiDeviceSession(t, client)

	_, err := svc.PollDeviceToken(context.Background(), sessionID, nil)
	require.Error(t, err)
	require.Equal(t, "KIMI_OAUTH_DEVICE_CODE_EXPIRED", infraerrors.Reason(err))

	// 设备码过期后会话作废，再次轮询直接报会话不存在且不再调用上游
	_, err = svc.PollDeviceToken(context.Background(), sessionID, nil)
	require.Error(t, err)
	require.Equal(t, "KIMI_OAUTH_SESSION_NOT_FOUND", infraerrors.Reason(err))
	require.Equal(t, 1, client.pollCalls)
}

func TestKimiOAuthServicePollDeviceTokenAccessDeniedDeletesSession(t *testing.T) {
	client := &kimiOAuthClientStub{
		pollErr: &kimi.OAuthError{StatusCode: http.StatusBadRequest, Code: kimi.OAuthErrorAccessDenied},
	}
	svc, sessionID := startKimiDeviceSession(t, client)

	_, err := svc.PollDeviceToken(context.Background(), sessionID, nil)
	require.Error(t, err)
	require.Equal(t, "KIMI_OAUTH_ACCESS_DENIED", infraerrors.Reason(err))

	_, err = svc.PollDeviceToken(context.Background(), sessionID, nil)
	require.Error(t, err)
	require.Equal(t, "KIMI_OAUTH_SESSION_NOT_FOUND", infraerrors.Reason(err))
	require.Equal(t, 1, client.pollCalls)
}

func TestKimiOAuthServicePollDeviceTokenSuccessStoresSessionToken(t *testing.T) {
	client := &kimiOAuthClientStub{
		pollResponse: &kimi.TokenResponse{
			AccessToken:  "access-token",
			RefreshToken: "refresh-token",
			TokenType:    "Bearer",
			ExpiresIn:    900,
		},
	}
	svc, sessionID := startKimiDeviceSession(t, client)

	info, err := svc.PollDeviceToken(context.Background(), sessionID, nil)
	require.NoError(t, err)
	require.Equal(t, "access-token", info.AccessToken)
	require.Equal(t, "refresh-token", info.RefreshToken)
	require.Equal(t, "Bearer", info.TokenType)
	require.Equal(t, int64(900), info.ExpiresIn)
	require.Equal(t, client.lastDeviceAuthDevID, info.DeviceID)
	require.Equal(t, kimi.DefaultClientID, info.ClientID)
	require.Greater(t, info.ExpiresAt, int64(0))

	// 幂等：已成功会话再次轮询直接返回暂存 token，不再调用上游
	again, err := svc.PollDeviceToken(context.Background(), sessionID, nil)
	require.NoError(t, err)
	require.Equal(t, "access-token", again.AccessToken)
	require.Equal(t, 1, client.pollCalls)

	// GetSessionToken 供 create-from-oauth 一步建号读取
	stored, err := svc.GetSessionToken(sessionID)
	require.NoError(t, err)
	require.Equal(t, "access-token", stored.AccessToken)
	require.Equal(t, "refresh-token", stored.RefreshToken)

	// 建号完成后删除会话
	svc.DeleteSession(sessionID)
	_, err = svc.GetSessionToken(sessionID)
	require.Error(t, err)
	require.Equal(t, "KIMI_OAUTH_SESSION_NOT_FOUND", infraerrors.Reason(err))
}

func TestKimiOAuthServiceRefreshTokenWritesBackRotatedRefreshToken(t *testing.T) {
	client := &kimiOAuthClientStub{
		refreshResponse: &kimi.TokenResponse{
			AccessToken:  "new-access-token",
			RefreshToken: "rotated-refresh-token",
			TokenType:    "Bearer",
			ExpiresIn:    900,
		},
	}
	svc := NewKimiOAuthService(nil, client)
	t.Cleanup(svc.Stop)

	info, err := svc.RefreshToken(context.Background(), "old-refresh-token", "", "device-xyz")
	require.NoError(t, err)
	require.Equal(t, "new-access-token", info.AccessToken)
	require.Equal(t, "rotated-refresh-token", info.RefreshToken, "refresh_token 轮换时新值必须写回")
	require.Equal(t, "device-xyz", info.DeviceID)
	require.Equal(t, "old-refresh-token", client.lastRefreshToken)
	require.Equal(t, "device-xyz", client.lastRefreshDeviceID)
}

func TestKimiOAuthServiceRefreshTokenPreservesOriginalWhenNotRotated(t *testing.T) {
	client := &kimiOAuthClientStub{
		refreshResponse: &kimi.TokenResponse{
			AccessToken: "new-access-token",
			TokenType:   "Bearer",
			ExpiresIn:   900,
		},
	}
	svc := NewKimiOAuthService(nil, client)
	t.Cleanup(svc.Stop)

	info, err := svc.RefreshToken(context.Background(), "original-refresh-token", "", "")
	require.NoError(t, err)
	require.Equal(t, "new-access-token", info.AccessToken)
	require.Equal(t, "original-refresh-token", info.RefreshToken, "响应未携带新 refresh_token 时保留旧值")
}

func TestKimiOAuthServiceRefreshTokenUnauthorizedMapsTokenInvalid(t *testing.T) {
	tests := []struct {
		name string
		err  error
	}{
		{name: "401", err: &kimi.OAuthError{StatusCode: http.StatusUnauthorized, Code: "unauthorized"}},
		{name: "403", err: &kimi.OAuthError{StatusCode: http.StatusForbidden, Code: "forbidden"}},
		{name: "invalid_grant", err: &kimi.OAuthError{StatusCode: http.StatusBadRequest, Code: kimi.OAuthErrorInvalidGrant}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			svc := NewKimiOAuthService(nil, &kimiOAuthClientStub{refreshErr: tt.err})
			t.Cleanup(svc.Stop)

			_, err := svc.RefreshToken(context.Background(), "refresh-token", "", "")
			require.Error(t, err)
			require.Equal(t, "KIMI_OAUTH_TOKEN_INVALID", infraerrors.Reason(err))
		})
	}
}

func TestKimiOAuthServiceRefreshTokenRequiresRefreshToken(t *testing.T) {
	svc := NewKimiOAuthService(nil, &kimiOAuthClientStub{})
	t.Cleanup(svc.Stop)

	_, err := svc.RefreshToken(context.Background(), "  ", "", "")
	require.Error(t, err)
	require.Equal(t, "KIMI_OAUTH_NO_REFRESH_TOKEN", infraerrors.Reason(err))
}

func TestKimiOAuthServiceBuildAccountCredentials(t *testing.T) {
	svc := NewKimiOAuthService(nil, &kimiOAuthClientStub{})
	t.Cleanup(svc.Stop)

	creds := svc.BuildAccountCredentials(&KimiTokenInfo{
		AccessToken:  "access-token",
		RefreshToken: "refresh-token",
		TokenType:    "Bearer",
		ExpiresAt:    1893456000,
		ClientID:     "client-id",
		DeviceID:     "device-xyz",
	})
	require.Equal(t, "access-token", creds["access_token"])
	require.Equal(t, "refresh-token", creds["refresh_token"])
	require.Equal(t, "Bearer", creds["token_type"])
	require.Equal(t, "client-id", creds["client_id"])
	require.Equal(t, "device-xyz", creds["device_id"])
	require.Equal(t, kimi.DefaultBaseURL, creds["base_url"])
	require.NotEmpty(t, creds["expires_at"])

	require.Nil(t, svc.BuildAccountCredentials(nil))
}
