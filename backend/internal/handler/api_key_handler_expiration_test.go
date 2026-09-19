//go:build unit

package handler

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	middleware2 "github.com/Wei-Shaw/sub2api/internal/server/middleware"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

type expirationAPIKeyRepoStub struct {
	service.APIKeyRepository
	created *service.APIKey
}

func (r *expirationAPIKeyRepoStub) Create(_ context.Context, key *service.APIKey) error {
	r.created = key
	key.ID = 1
	return nil
}

type expirationUserRepoStub struct {
	service.UserRepository
}

func (*expirationUserRepoStub) GetByID(_ context.Context, id int64) (*service.User, error) {
	return &service.User{ID: id}, nil
}

func TestAPIKeyHandlerCreateExpiration(t *testing.T) {
	gin.SetMode(gin.TestMode)

	tests := []struct {
		name       string
		body       string
		wantExpiry string
		wantDays   int
		wantBad    bool
	}{
		{
			name:       "exact UTC timestamp with milliseconds",
			body:       `{"name":"test","expires_at":"2030-09-19T05:00:15.123Z"}`,
			wantExpiry: "2030-09-19T05:00:15.123Z",
		},
		{
			name:       "positive timezone offset",
			body:       `{"name":"test","expires_at":"2030-09-19T13:00:00+08:00"}`,
			wantExpiry: "2030-09-19T05:00:00Z",
		},
		{
			name:       "timezone conversion crosses midnight",
			body:       `{"name":"test","expires_at":"2030-09-20T00:15:00+08:00"}`,
			wantExpiry: "2030-09-19T16:15:00Z",
		},
		{
			name:       "negative timezone offset",
			body:       `{"name":"test","expires_at":"2030-09-19T23:30:00-07:00"}`,
			wantExpiry: "2030-09-20T06:30:00Z",
		},
		{
			name:       "exact timestamp takes precedence over legacy days",
			body:       `{"name":"test","expires_at":"2030-09-19T05:00:00Z","expires_in_days":7}`,
			wantExpiry: "2030-09-19T05:00:00Z",
		},
		{name: "omitted expiry", body: `{"name":"test"}`},
		{name: "null expiry", body: `{"name":"test","expires_at":null}`},
		{name: "legacy days", body: `{"name":"test","expires_in_days":7}`, wantDays: 7},
		{name: "null timestamp with legacy days", body: `{"name":"test","expires_at":null,"expires_in_days":1}`, wantDays: 1},
		{name: "invalid timestamp", body: `{"name":"test","expires_at":"invalid"}`, wantBad: true},
		{name: "missing timezone", body: `{"name":"test","expires_at":"2030-09-19T13:00:00"}`, wantBad: true},
		{name: "invalid calendar date", body: `{"name":"test","expires_at":"2030-02-30T13:00:00Z"}`, wantBad: true},
		{name: "empty timestamp", body: `{"name":"test","expires_at":""}`, wantBad: true},
		{name: "invalid legacy days", body: `{"name":"test","expires_in_days":0}`, wantBad: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repo := &expirationAPIKeyRepoStub{}
			svc := service.NewAPIKeyService(repo, &expirationUserRepoStub{}, nil, nil, nil, nil, &config.Config{})
			h := NewAPIKeyHandler(svc)
			recorder := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(recorder)
			c.Request = httptest.NewRequest(http.MethodPost, "/api/v1/keys", strings.NewReader(tt.body))
			c.Request.Header.Set("Content-Type", "application/json")
			c.Set(string(middleware2.ContextKeyUser), middleware2.AuthSubject{UserID: 42})

			before := time.Now()
			h.Create(c)
			after := time.Now()

			if tt.wantBad {
				require.Equal(t, http.StatusBadRequest, recorder.Code, recorder.Body.String())
				require.Nil(t, repo.created, "invalid expiry must not create a key")
				return
			}
			require.Equal(t, http.StatusOK, recorder.Code, recorder.Body.String())
			require.NotNil(t, repo.created)
			var response struct {
				Data struct {
					ExpiresAt *time.Time `json:"expires_at"`
				} `json:"data"`
			}
			require.NoError(t, json.Unmarshal(recorder.Body.Bytes(), &response))

			if tt.wantExpiry == "" && tt.wantDays == 0 {
				require.Nil(t, repo.created.ExpiresAt)
				require.Nil(t, response.Data.ExpiresAt)
				return
			}
			require.NotNil(t, repo.created.ExpiresAt)
			require.NotNil(t, response.Data.ExpiresAt)
			require.True(t, repo.created.ExpiresAt.Equal(*response.Data.ExpiresAt))
			if tt.wantDays > 0 {
				require.False(t, repo.created.ExpiresAt.Before(before.AddDate(0, 0, tt.wantDays)))
				require.False(t, repo.created.ExpiresAt.After(after.AddDate(0, 0, tt.wantDays)))
			} else {
				want, err := time.Parse(time.RFC3339Nano, tt.wantExpiry)
				require.NoError(t, err)
				require.True(t, want.Equal(*repo.created.ExpiresAt), "want %s, got %s", want, repo.created.ExpiresAt)
			}
		})
	}
}
