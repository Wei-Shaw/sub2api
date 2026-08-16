//go:build unit

package handler

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/pquerna/otp/totp"
	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/require"

	"github.com/Wei-Shaw/sub2api/internal/repository"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
)

type failingConsumeTotpCache struct {
	service.TotpCache
}

func (failingConsumeTotpCache) ConsumeLoginSession(context.Context, string) (*service.TotpLoginSession, error) {
	return nil, errors.New("fake redis consume failure")
}

type login2FAResult struct {
	status int
	body   []byte
}

func newAtomicLogin2FATestHandler(t *testing.T, cache service.TotpCache) (*AuthHandler, string, string) {
	t.Helper()
	handler, client := newOAuthPendingFlowTestHandlerWithDependencies(t, oauthPendingFlowTestHandlerOptions{
		settingValues: map[string]string{service.SettingKeyTotpEnabled: "true"},
		totpCache:     cache,
		totpEncryptor: oauthPendingFlowTotpEncryptorStub{},
	})

	const secret = "JBSWY3DPEHPK3PXP"
	passwordHash, err := handler.authService.HashPassword("fake-local-password")
	require.NoError(t, err)
	user, err := client.User.Create().
		SetEmail("atomic-login@example.test").
		SetUsername("atomic-login-user").
		SetPasswordHash(passwordHash).
		SetRole(service.RoleUser).
		SetStatus(service.StatusActive).
		SetTotpEnabled(true).
		SetTotpSecretEncrypted(secret).
		SetTotpEnabledAt(time.Now().UTC()).
		Save(context.Background())
	require.NoError(t, err)

	tempToken, err := handler.totpService.CreateLoginSession(context.Background(), user.ID, user.Email)
	require.NoError(t, err)
	code, err := totp.GenerateCode(secret, time.Now().UTC())
	require.NoError(t, err)
	return handler, tempToken, code
}

func invokeLogin2FA(handler *AuthHandler, tempToken, code string) login2FAResult {
	body, _ := json.Marshal(Login2FARequest{TempToken: tempToken, TotpCode: code})
	recorder := httptest.NewRecorder()
	ginContext, _ := gin.CreateTestContext(recorder)
	request := httptest.NewRequest(http.MethodPost, "/api/v1/auth/login/2fa", bytes.NewReader(body))
	request.Header.Set("Content-Type", "application/json")
	ginContext.Request = request
	handler.Login2FA(ginContext)
	return login2FAResult{status: recorder.Code, body: recorder.Body.Bytes()}
}

func responseContainsTokenPair(t *testing.T, body []byte) bool {
	t.Helper()
	var envelope struct {
		Data map[string]any `json:"data"`
	}
	require.NoError(t, json.Unmarshal(body, &envelope))
	accessToken, accessOK := envelope.Data["access_token"].(string)
	refreshToken, refreshOK := envelope.Data["refresh_token"].(string)
	return accessOK && refreshOK && accessToken != "" && refreshToken != ""
}

func TestLogin2FASameTempTokenAndCode32ConcurrentRequestsIssueExactlyOneTokenPair(t *testing.T) {
	redisServer := miniredis.RunT(t)
	redisClient := redis.NewClient(&redis.Options{Addr: redisServer.Addr()})
	t.Cleanup(func() { require.NoError(t, redisClient.Close()) })
	cache := repository.NewTotpCache(redisClient)
	handler, tempToken, code := newAtomicLogin2FATestHandler(t, cache)

	const callers = 32
	start := make(chan struct{})
	results := make(chan login2FAResult, callers)
	var wg sync.WaitGroup
	for range callers {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			results <- invokeLogin2FA(handler, tempToken, code)
		}()
	}
	close(start)
	wg.Wait()
	close(results)

	tokenPairs := 0
	for result := range results {
		if responseContainsTokenPair(t, result.body) {
			tokenPairs++
			require.Equal(t, http.StatusOK, result.status)
		} else {
			require.NotEqual(t, http.StatusOK, result.status)
		}
	}
	require.Equal(t, 1, tokenPairs)

	remaining, err := cache.GetLoginSession(context.Background(), tempToken)
	require.NoError(t, err)
	require.Nil(t, remaining)
}

func TestLogin2FAConsumeStorageErrorIssuesNoTokenPair(t *testing.T) {
	redisServer := miniredis.RunT(t)
	redisClient := redis.NewClient(&redis.Options{Addr: redisServer.Addr()})
	t.Cleanup(func() { require.NoError(t, redisClient.Close()) })
	cache := failingConsumeTotpCache{TotpCache: repository.NewTotpCache(redisClient)}
	handler, tempToken, code := newAtomicLogin2FATestHandler(t, cache)

	result := invokeLogin2FA(handler, tempToken, code)
	require.Equal(t, http.StatusInternalServerError, result.status)
	require.False(t, responseContainsTokenPair(t, result.body))
}
