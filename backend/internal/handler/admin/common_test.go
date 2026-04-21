//go:build unit

package admin

import (
	"bytes"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"

	pkgerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
)

// newTestContext 构造一个带 URL param 和 JSON body 的 *gin.Context，用于测试 common.go
// 里不依赖路由/中间件的 helper。
func newTestContext(t *testing.T, params gin.Params, body string) *gin.Context {
	t.Helper()
	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Params = params
	if body != "" {
		req, err := http.NewRequest(http.MethodPost, "/test", bytes.NewBufferString(body))
		require.NoError(t, err)
		req.Header.Set("Content-Type", "application/json")
		c.Request = req
	}
	return c
}

func TestParseInt64Param(t *testing.T) {
	tests := []struct {
		name          string
		paramName     string
		paramValue    string
		invalidCode   string
		expectErr     bool
		expectVal     int64
		expectCode    string
		expectMdParam string
	}{
		{
			name:       "valid positive",
			paramName:  "id",
			paramValue: "42",
			expectErr:  false,
			expectVal:  42,
		},
		{
			name:       "valid zero (no range check in helper)",
			paramName:  "id",
			paramValue: "0",
			expectErr:  false,
			expectVal:  0,
		},
		{
			name:       "valid negative (no range check in helper)",
			paramName:  "ruleID",
			paramValue: "-3",
			expectErr:  false,
			expectVal:  -3,
		},
		{
			name:          "non-numeric returns structured error",
			paramName:     "id",
			paramValue:    "abc",
			invalidCode:   "QUOTA_INVALID_USER_ID",
			expectErr:     true,
			expectCode:    "QUOTA_INVALID_USER_ID",
			expectMdParam: "id",
		},
		{
			name:          "empty returns structured error",
			paramName:     "ruleID",
			paramValue:    "",
			invalidCode:   "QUOTA_INVALID_RULE_ID",
			expectErr:     true,
			expectCode:    "QUOTA_INVALID_RULE_ID",
			expectMdParam: "ruleID",
		},
		{
			name:          "overflow returns structured error",
			paramName:     "id",
			paramValue:    "999999999999999999999",
			invalidCode:   "QUOTA_INVALID_USER_ID",
			expectErr:     true,
			expectCode:    "QUOTA_INVALID_USER_ID",
			expectMdParam: "id",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			c := newTestContext(t, gin.Params{{Key: tc.paramName, Value: tc.paramValue}}, "")
			got, err := ParseInt64Param(c, tc.paramName, tc.invalidCode)
			if !tc.expectErr {
				require.NoError(t, err)
				require.Equal(t, tc.expectVal, got)
				return
			}
			require.Error(t, err)
			var appErr *pkgerrors.ApplicationError
			require.True(t, errors.As(err, &appErr), "error should be *ApplicationError, got %T", err)
			require.Equal(t, int32(http.StatusBadRequest), appErr.Code)
			require.Equal(t, tc.expectCode, appErr.Reason)
			require.Equal(t, tc.expectMdParam, appErr.Metadata["param"])
			require.Equal(t, tc.paramValue, appErr.Metadata["value"])
			require.NotEmpty(t, appErr.Metadata["reason"], "metadata.reason should contain underlying parse error")
			require.Contains(t, appErr.Message, tc.paramName)
		})
	}
}

func TestBindJSONOrError(t *testing.T) {
	type payload struct {
		Name  string `json:"name" binding:"required"`
		Count int    `json:"count"`
	}

	t.Run("valid body", func(t *testing.T) {
		c := newTestContext(t, nil, `{"name":"alice","count":3}`)
		var req payload
		err := BindJSONOrError(c, &req, "TEST_INVALID_REQUEST")
		require.NoError(t, err)
		require.Equal(t, "alice", req.Name)
		require.Equal(t, 3, req.Count)
	})

	t.Run("malformed JSON returns structured error", func(t *testing.T) {
		c := newTestContext(t, nil, `{"name":`)
		var req payload
		err := BindJSONOrError(c, &req, "TEST_INVALID_REQUEST")
		require.Error(t, err)
		var appErr *pkgerrors.ApplicationError
		require.True(t, errors.As(err, &appErr))
		require.Equal(t, int32(http.StatusBadRequest), appErr.Code)
		require.Equal(t, "TEST_INVALID_REQUEST", appErr.Reason)
		require.NotEmpty(t, appErr.Metadata["reason"])
	})

	t.Run("missing required field returns structured error", func(t *testing.T) {
		c := newTestContext(t, nil, `{"count":5}`)
		var req payload
		err := BindJSONOrError(c, &req, "QUOTA_INVALID_REQUEST")
		require.Error(t, err)
		var appErr *pkgerrors.ApplicationError
		require.True(t, errors.As(err, &appErr))
		require.Equal(t, "QUOTA_INVALID_REQUEST", appErr.Reason)
		require.True(t, strings.Contains(appErr.Metadata["reason"], "Name") ||
			strings.Contains(appErr.Metadata["reason"], "name"),
			"metadata.reason should reference the failing field; got %q", appErr.Metadata["reason"])
	})
}
