//go:build unit

package service

import (
	"context"
	"errors"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/pkg/logger"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
	"go.uber.org/zap/zaptest/observer"
)

type accountTestLoggingRepo struct {
	AccountRepository
	account *Account
}

func (r *accountTestLoggingRepo) GetByID(context.Context, int64) (*Account, error) {
	if r.account == nil {
		return nil, errors.New("missing")
	}
	return r.account, nil
}

func TestAccountTestFailureLogAttribution(t *testing.T) {
	gin.SetMode(gin.TestMode)
	for _, background := range []bool{false, true} {
		for _, missing := range []bool{false, true} {
			core, logs := observer.New(zap.WarnLevel)
			ctx := logger.IntoContext(context.Background(), zap.New(core).With(zap.String("request_id", "req-test")))
			repo := &accountTestLoggingRepo{}
			if !missing {
				repo.account = &Account{ID: 77, Platform: PlatformAnthropic, Type: AccountTypeAPIKey}
			}
			svc := &AccountTestService{accountRepo: repo}
			if background {
				result, err := svc.RunTestBackground(ctx, 77, "test-model")
				require.NoError(t, err)
				require.Equal(t, "failed", result.Status)
				require.NotEmpty(t, result.ErrorMessage)
			} else {
				rec := httptest.NewRecorder()
				c, _ := gin.CreateTestContext(rec)
				c.Request = httptest.NewRequest("POST", "/accounts/77/test", nil).WithContext(ctx)
				err := svc.TestAccountConnection(c, 77, "test-model", "private prompt", "")
				require.Error(t, err)
				require.Contains(t, rec.Body.String(), `"type":"error"`)
			}
			entries := logs.FilterMessage("account_test.failed").All()
			require.Len(t, entries, 1)
			fields := entries[0].ContextMap()
			require.Equal(t, int64(77), fields["account_id"])
			require.Equal(t, "req-test", fields["request_id"])
			require.Equal(t, "test-model", fields["requested_model"])
			require.NotEmpty(t, fields["test_id"])
			if background {
				require.Equal(t, "background", fields["test_source"])
			} else {
				require.Equal(t, "http", fields["test_source"])
			}
			if !missing {
				require.Equal(t, PlatformAnthropic, fields["platform"])
			}
			require.NotContains(t, fields, "prompt")
		}
	}
}

func TestAccountTestErrorLogRedaction(t *testing.T) {
	core, logs := observer.New(zap.WarnLevel)
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest("POST", "/test", nil).WithContext(logger.IntoContext(context.Background(), zap.New(core)))
	initAccountTestLogger(c, 1, "", "")
	message := `API returned 403: {"api_key":"secret-key","access_token":"secret-token","message":"denied"}`
	err := (&AccountTestService{}).sendErrorAndEnd(c, message)
	require.EqualError(t, err, message)
	entry := logs.FilterMessage("account_test.failed").All()[0]
	logged, ok := entry.ContextMap()["error"].(string)
	require.True(t, ok)
	require.NotContains(t, logged, "secret-key")
	require.NotContains(t, logged, "secret-token")
	require.Contains(t, logged, "denied")
	logAccountTestError(c, strings.Repeat("x", 10000))
	last, ok := logs.All()[1].ContextMap()["error"].(string)
	require.True(t, ok)
	require.Less(t, len(last), 2100)
}
