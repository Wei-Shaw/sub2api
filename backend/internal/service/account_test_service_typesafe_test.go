package service

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/pkg/typesafe"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func newTypeSafeAccountTestFixture(t *testing.T, status int, responseBody string) (*AccountTestService, *systemOnePolicyAccountRepo, *[]*http.Request, *gin.Context, *httptest.ResponseRecorder) {
	t.Helper()
	account := &Account{ID: 31, Platform: PlatformTypeSafe, Type: AccountTypeAPIKey, Credentials: map[string]any{"api_key": "ts-secret"}}
	repo := &systemOnePolicyAccountRepo{account: account}
	var requests []*http.Request
	upstream := &systemOneHTTPUpstream{do: func(req *http.Request) (*http.Response, error) {
		requests = append(requests, req)
		return &http.Response{StatusCode: status, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(responseBody))}, nil
	}}
	svc := &AccountTestService{
		accountRepo:  repo,
		httpUpstream: upstream,
		cfg:          &config.Config{},
	}
	gin.SetMode(gin.TestMode)
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPost, "/api/v1/admin/accounts/31/test", nil)
	return svc, repo, &requests, c, rec
}

func TestTypeSafeAccountTestSendsNativeSystemOneRequest(t *testing.T) {
	svc, repo, requests, c, rec := newTypeSafeAccountTestFixture(t, http.StatusOK, `{"model":"jev-1.13.0","answers":{"connection_test":{"type":"noul"}},"usage":{"input_tokens":9}}`)

	// The Claude default model must be ignored: TypeSafe only serves jev-latest.
	require.NoError(t, svc.TestAccountConnection(c, 31, "claude-sonnet-4-5", "", AccountTestModeDefault))

	require.Len(t, *requests, 1)
	req := (*requests)[0]
	require.Equal(t, typesafe.DefaultBaseURL+typesafe.SystemOnePath, req.URL.String())
	require.Equal(t, "Bearer ts-secret", req.Header.Get("Authorization"))
	require.Empty(t, req.Header.Get("x-api-key"))
	body, err := io.ReadAll(req.Body)
	require.NoError(t, err)
	model, err := typesafe.ValidateSystemOneRequest(body)
	require.NoError(t, err)
	require.Equal(t, typesafe.JevLatestModel, model)

	output := rec.Body.String()
	require.Contains(t, output, `"type":"test_start"`)
	require.Contains(t, output, `"model":"jev-latest"`)
	require.Contains(t, output, `"type":"test_complete"`)
	require.Contains(t, output, "jev-1.13.0")
	require.Zero(t, repo.errorCalls)
}

func TestTypeSafeAccountTestMarksRejectedKey(t *testing.T) {
	for _, status := range []int{http.StatusUnauthorized, http.StatusForbidden} {
		t.Run(http.StatusText(status), func(t *testing.T) {
			svc, repo, _, c, rec := newTypeSafeAccountTestFixture(t, status, `{"detail":"invalid key"}`)
			require.Error(t, svc.TestAccountConnection(c, 31, "", "", AccountTestModeDefault))
			require.Equal(t, 1, repo.errorCalls)
			responseText, errMsg := parseTestSSEOutput(rec.Body.String())
			require.Empty(t, responseText)
			require.Contains(t, errMsg, "API returned")
		})
	}
}

func TestTypeSafeAccountTestTransientFailureKeepsAccountState(t *testing.T) {
	svc, repo, _, c, _ := newTypeSafeAccountTestFixture(t, http.StatusServiceUnavailable, `{"detail":"busy"}`)
	require.Error(t, svc.TestAccountConnection(c, 31, "", "", AccountTestModeDefault))
	require.Zero(t, repo.errorCalls)
}

func TestTypeSafeAccountTestUsesPromptAsState(t *testing.T) {
	svc, _, requests, c, _ := newTypeSafeAccountTestFixture(t, http.StatusOK, `{"answers":{}}`)
	require.NoError(t, svc.TestAccountConnection(c, 31, "", "custom state", AccountTestModeDefault))
	body, err := io.ReadAll((*requests)[0].Body)
	require.NoError(t, err)
	var payload struct {
		State string `json:"state"`
	}
	require.NoError(t, json.Unmarshal(body, &payload))
	require.Equal(t, "custom state", payload.State)
}
