//go:build unit

package handler

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	middleware2 "github.com/Wei-Shaw/sub2api/internal/server/middleware"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

type monthlyAdvanceHandlerRepo struct {
	service.UserSubscriptionRepository
	called bool
}

func (r *monthlyAdvanceHandlerRepo) GetByIDForUpdate(context.Context, int64) (*service.UserSubscription, error) {
	r.called = true
	return nil, service.ErrSubscriptionNotFound
}

func TestMonthlyAdvanceHandlerConfirmationValidation(t *testing.T) {
	gin.SetMode(gin.TestMode)
	const month = `"monthly_window_start":"2026-09-01T12:00:00Z"`
	const expiry = `"expires_at":"2026-10-31T12:00:00Z"`
	for _, tc := range []struct {
		name       string
		body       string
		wantStatus int
		wantCall   bool
	}{
		{"missing weekly anchor", `{` + month + `,` + expiry + `}`, http.StatusBadRequest, false},
		{"invalid weekly anchor", `{` + month + `,` + expiry + `,"weekly_window_start":"bad"}`, http.StatusBadRequest, false},
		{"missing monthly anchor", `{` + expiry + `,"weekly_window_start":null}`, http.StatusBadRequest, false},
		{"missing expiry", `{` + month + `,"weekly_window_start":null}`, http.StatusBadRequest, false},
		{"confirmed null anchor", `{` + month + `,` + expiry + `,"weekly_window_start":null}`, http.StatusNotFound, true},
		{"confirmed date anchor", `{` + month + `,` + expiry + `,"weekly_window_start":"2026-09-29T12:00:00Z"}`, http.StatusNotFound, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			repo := &monthlyAdvanceHandlerRepo{}
			svc := service.NewSubscriptionService(nil, repo, nil, nil, nil)
			t.Cleanup(svc.Stop)
			h := NewSubscriptionHandler(svc)
			w := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(w)
			c.Request = httptest.NewRequest(http.MethodPost, "/subscriptions/10/advance-month", strings.NewReader(tc.body))
			c.Request.Header.Set("Content-Type", "application/json")
			c.Params = gin.Params{{Key: "id", Value: "10"}}
			c.Set(string(middleware2.ContextKeyUser), middleware2.AuthSubject{UserID: 20})
			h.AdvanceMonth(c)
			require.Equal(t, tc.wantStatus, w.Code, w.Body.String())
			require.Equal(t, tc.wantCall, repo.called)
		})
	}
}

func TestMonthlyAdvanceHandlerRequiresAuthenticationAndValidID(t *testing.T) {
	gin.SetMode(gin.TestMode)
	for _, handler := range []func(*SubscriptionHandler, *gin.Context){(*SubscriptionHandler).PreviewAdvanceMonth, (*SubscriptionHandler).AdvanceMonth} {
		for _, authorized := range []bool{false, true} {
			w := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(w)
			c.Request = httptest.NewRequest(http.MethodPost, "/subscriptions/invalid/advance-month", nil)
			c.Params = gin.Params{{Key: "id", Value: "invalid"}}
			if authorized {
				c.Set(string(middleware2.ContextKeyUser), middleware2.AuthSubject{UserID: 20})
			}
			handler(&SubscriptionHandler{}, c)
			want := http.StatusUnauthorized
			if authorized {
				want = http.StatusBadRequest
			}
			require.Equal(t, want, w.Code)
		}
	}
}
