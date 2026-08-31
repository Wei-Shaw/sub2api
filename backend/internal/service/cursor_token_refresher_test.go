package service

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/cursor"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

type cursorCredRepo struct {
	AccountRepository
	account *Account
	updates []map[string]any
}

func (r *cursorCredRepo) GetByID(_ context.Context, id int64) (*Account, error) {
	if r.account == nil || r.account.ID != id {
		return nil, ErrAccountNotFound
	}
	return r.account, nil
}

func (r *cursorCredRepo) Update(_ context.Context, account *Account) error {
	r.account = account
	r.updates = append(r.updates, shallowCopyMap(account.Credentials))
	return nil
}

func (r *cursorCredRepo) UpdateCredentials(_ context.Context, id int64, credentials map[string]any) error {
	if r.account == nil || r.account.ID != id {
		return ErrAccountNotFound
	}
	r.account.Credentials = shallowCopyMap(credentials)
	r.updates = append(r.updates, shallowCopyMap(credentials))
	return nil
}

func TestCursorTokenRefresherNeedsRefreshFromJWT(t *testing.T) {
	refresher := NewCursorTokenRefresher()
	expired := fakeCursorJWT(time.Now().Add(-time.Minute))
	fresh := fakeCursorJWT(time.Now().Add(time.Hour))

	require.False(t, refresher.CanRefresh(&Account{Platform: PlatformCursor, Type: AccountTypeOAuth}))
	require.True(t, refresher.CanRefresh(&Account{
		Platform:    PlatformCursor,
		Type:        AccountTypeOAuth,
		Credentials: map[string]any{"refresh_token": "rt", "access_token": expired},
	}))

	require.True(t, refresher.NeedsRefresh(&Account{
		Platform:    PlatformCursor,
		Type:        AccountTypeOAuth,
		Credentials: map[string]any{"refresh_token": "rt", "access_token": expired},
	}, cursorTokenRefreshSkew))
	require.False(t, refresher.NeedsRefresh(&Account{
		Platform:    PlatformCursor,
		Type:        AccountTypeOAuth,
		Credentials: map[string]any{"refresh_token": "rt", "access_token": fresh},
	}, cursorTokenRefreshSkew))
}

func TestCursorTokenRefresherRefreshWritesExpiresAt(t *testing.T) {
	refresher := NewCursorTokenRefresher()
	refresher.refresh = func(context.Context, string) (*cursor.TokenRefreshResult, error) {
		return &cursor.TokenRefreshResult{
			AccessToken:  "new-access",
			RefreshToken: "new-refresh",
			ExpiresIn:    120,
		}, nil
	}

	account := &Account{
		ID:       9,
		Platform: PlatformCursor,
		Type:     AccountTypeOAuth,
		Credentials: map[string]any{
			"access_token":  "old-access",
			"refresh_token": "old-refresh",
			"machine_id":    "m",
		},
	}
	creds, err := refresher.Refresh(context.Background(), account)
	require.NoError(t, err)
	require.Equal(t, "new-access", creds["access_token"])
	require.Equal(t, "new-refresh", creds["refresh_token"])
	require.Equal(t, "m", creds["machine_id"])
	require.NotEmpty(t, creds["expires_at"])
}

func TestCursorGatewayRefreshesExpiredTokenBeforeUpstream(t *testing.T) {
	expired := fakeCursorJWT(time.Now().Add(-time.Minute))
	account := &Account{
		ID:       11,
		Platform: PlatformCursor,
		Type:     AccountTypeOAuth,
		Status:   StatusActive,
		Credentials: map[string]any{
			"access_token":  expired,
			"refresh_token": "rt",
		},
	}
	repo := &cursorCredRepo{account: account}
	svc := NewCursorGatewayService(repo, nil)
	svc.availableModels = func(context.Context, cursor.Credentials) ([]cursor.AvailableModel, error) {
		return nil, fmt.Errorf("catalog unused")
	}
	svc.refresher.refresh = func(context.Context, string) (*cursor.TokenRefreshResult, error) {
		return &cursor.TokenRefreshResult{AccessToken: "rotated", ExpiresIn: 3600}, nil
	}

	var seen []string
	svc.streamChat = func(_ context.Context, creds cursor.Credentials, _ []cursor.ChatMessage, _ string, _ int) (*http.Response, error) {
		seen = append(seen, creds.AccessToken)
		return &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(bytes.NewReader(nil))}, nil
	}

	resp, _, _, err := svc.startCursorChat(context.Background(), newCursorGinContext(), account, nil, "grok-4.6", cursor.RunOpts{})
	require.NoError(t, err)
	require.NotNil(t, resp)
	resp.Body.Close()
	require.Equal(t, []string{"rotated"}, seen)
	require.Equal(t, "rotated", account.GetCredential("access_token"))
	require.Len(t, repo.updates, 1)
}

func TestCursorGatewayRetriesAfterUnauthorized(t *testing.T) {
	fresh := fakeCursorJWT(time.Now().Add(time.Hour))
	account := &Account{
		ID:       12,
		Platform: PlatformCursor,
		Type:     AccountTypeOAuth,
		Status:   StatusActive,
		Credentials: map[string]any{
			"access_token":  fresh,
			"refresh_token": "rt",
		},
	}
	repo := &cursorCredRepo{account: account}
	svc := NewCursorGatewayService(repo, nil)
	svc.availableModels = func(context.Context, cursor.Credentials) ([]cursor.AvailableModel, error) {
		return nil, fmt.Errorf("catalog unused")
	}
	svc.refresher.refresh = func(context.Context, string) (*cursor.TokenRefreshResult, error) {
		return &cursor.TokenRefreshResult{AccessToken: "after-401", ExpiresIn: 3600}, nil
	}

	var seen []string
	svc.streamChat = func(_ context.Context, creds cursor.Credentials, _ []cursor.ChatMessage, _ string, _ int) (*http.Response, error) {
		seen = append(seen, creds.AccessToken)
		if len(seen) == 1 {
			return nil, fmt.Errorf("status 401: unauthorized")
		}
		return &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(bytes.NewReader(nil))}, nil
	}

	resp, _, _, err := svc.startCursorChat(context.Background(), newCursorGinContext(), account, nil, "grok-4.6", cursor.RunOpts{})
	require.NoError(t, err)
	resp.Body.Close()
	require.Equal(t, []string{fresh, "after-401"}, seen)
	require.Equal(t, "after-401", account.GetCredential("access_token"))
	require.Len(t, repo.updates, 1)
}

func TestIsCursorAuthError(t *testing.T) {
	require.True(t, isCursorAuthError(fmt.Errorf("status 401: nope")))
	require.True(t, isCursorAuthError(fmt.Errorf("Connect: unauthenticated")))
	require.False(t, isCursorAuthError(fmt.Errorf("status 429: rate limited")))
	require.False(t, isCursorAuthError(nil))
}

func TestCursorGatewayResolvesRunSlugFromLiveCatalog(t *testing.T) {
	account := cursorAccountWithFreshToken(13)
	svc := NewCursorGatewayService(nil, nil)
	var fetches int
	svc.availableModels = func(context.Context, cursor.Credentials) ([]cursor.AvailableModel, error) {
		fetches++
		return []cursor.AvailableModel{{
			Name:        "grok-4.6",
			LegacySlugs: []string{"cursor-grok-4.6-xhigh"},
		}}, nil
	}
	var seen []string
	svc.streamChat = func(_ context.Context, _ cursor.Credentials, _ []cursor.ChatMessage, model string, _ int) (*http.Response, error) {
		seen = append(seen, model)
		return &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(bytes.NewReader(nil))}, nil
	}

	resp, upstream, _, err := svc.startCursorChat(context.Background(), newCursorGinContext(), account, nil, "grok-4.6", cursor.RunOpts{})
	require.NoError(t, err)
	resp.Body.Close()
	require.Equal(t, "cursor-grok-4.6-xhigh", upstream)
	require.Equal(t, []string{"cursor-grok-4.6-xhigh"}, seen)

	resp, _, _, err = svc.startCursorChat(context.Background(), newCursorGinContext(), account, nil, "grok-4.6", cursor.RunOpts{})
	require.NoError(t, err)
	resp.Body.Close()
	require.Equal(t, 1, fetches)
}

func TestCursorGatewayFallsBackToSnapshotWhenCatalogFails(t *testing.T) {
	account := cursorAccountWithFreshToken(14)
	svc := NewCursorGatewayService(nil, nil)
	svc.availableModels = func(context.Context, cursor.Credentials) ([]cursor.AvailableModel, error) {
		return nil, fmt.Errorf("catalog down")
	}
	var seen string
	svc.streamChat = func(_ context.Context, _ cursor.Credentials, _ []cursor.ChatMessage, model string, _ int) (*http.Response, error) {
		seen = model
		return &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(bytes.NewReader(nil))}, nil
	}

	resp, upstream, _, err := svc.startCursorChat(context.Background(), newCursorGinContext(), account, nil, "grok-4.6", cursor.RunOpts{})
	require.NoError(t, err)
	resp.Body.Close()
	require.Equal(t, "cursor-grok-4.6-medium", upstream)
	require.Equal(t, "cursor-grok-4.6-medium", seen)
}

func cursorAccountWithFreshToken(id int64) *Account {
	return &Account{
		ID:       id,
		Platform: PlatformCursor,
		Type:     AccountTypeOAuth,
		Status:   StatusActive,
		Credentials: map[string]any{
			"access_token":  fakeCursorJWT(time.Now().Add(time.Hour)),
			"refresh_token": "rt",
		},
	}
}

func newCursorGinContext() *gin.Context {
	gin.SetMode(gin.TestMode)
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)
	return c
}

func fakeCursorJWT(exp time.Time) string {
	payload, _ := json.Marshal(map[string]any{"exp": exp.Unix()})
	return "eyJhbGciOiJub25lIn0." + base64.RawURLEncoding.EncodeToString(payload) + ".sig"
}
