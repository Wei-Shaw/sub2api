//go:build unit

package admin

import (
	"bytes"
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

type metaHandlerCredentialDoer struct {
	startBody   string
	tokenBodies []string
	mintBody    string
}

func (d *metaHandlerCredentialDoer) Do(req *http.Request) (*http.Response, error) {
	status := http.StatusOK
	body := d.startBody
	switch {
	case strings.Contains(req.URL.Path, "authorization"):
		body = d.startBody
	case strings.Contains(req.URL.Path, "token"):
		if len(d.tokenBodies) == 0 {
			body = `{"error":"authorization_pending"}`
		} else {
			body = d.tokenBodies[0]
			d.tokenBodies = d.tokenBodies[1:]
		}
		if !strings.Contains(body, "access_token") {
			status = http.StatusBadRequest
		}
	case strings.Contains(req.URL.Path, "muse-code"):
		body = d.mintBody
	}
	return &http.Response{
		StatusCode: status,
		Header:     http.Header{"Content-Type": []string{"application/json"}},
		Body:       io.NopCloser(bytes.NewReader([]byte(body))),
	}, nil
}

type metaAdminServiceStub struct {
	service.AdminService
	created     *service.CreateAccountInput
	createCalls int
	createErr   error
}

func (s *metaAdminServiceStub) CreateAccount(ctx context.Context, input *service.CreateAccountInput) (*service.Account, error) {
	s.createCalls++
	if s.createErr != nil {
		return nil, s.createErr
	}
	s.created = input
	return &service.Account{
		ID:          77,
		Name:        input.Name,
		Platform:    input.Platform,
		Type:        input.Type,
		Credentials: input.Credentials,
	}, nil
}

func newMetaHandlerTestRouter(t *testing.T, doer *metaHandlerCredentialDoer, adminService service.AdminService) *gin.Engine {
	t.Helper()
	gin.SetMode(gin.TestMode)
	oauthService := service.NewMetaOAuthService(nil).WithTransport(doer)
	handler := NewMetaOAuthHandler(oauthService, service.NewMetaTokenProvider(nil, nil), adminService)
	router := gin.New()
	router.POST("/api/v1/admin/meta/oauth/device/start", handler.StartDeviceFlow)
	router.POST("/api/v1/admin/meta/oauth/device/poll", handler.PollDeviceFlow)
	return router
}

func metaHandlerDoer() *metaHandlerCredentialDoer {
	return &metaHandlerCredentialDoer{
		startBody: `{"device_code":"dev-handler","user_code":"WXYZ-1234","verification_uri":"https://auth.meta.com/device","verification_uri_complete":"https://auth.meta.com/device?user_code=WXYZ-1234","expires_in":600,"interval":3}`,
		mintBody:  `{"api_key":"sk-handler","base_url":"https://api.meta.ai/v1","user_email":"muse@example.com","subs_tier_name":"Pro","is_subs_active":true}`,
	}
}

func TestMetaOAuthHandlerStartDeviceFlow(t *testing.T) {
	router := newMetaHandlerTestRouter(t, metaHandlerDoer(), &metaAdminServiceStub{})

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/admin/meta/oauth/device/start", strings.NewReader(`{}`))
	req.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(rec, req)

	require.Equal(t, http.StatusOK, rec.Code)
	body := rec.Body.String()
	require.Contains(t, body, `"user_code":"WXYZ-1234"`)
	require.Contains(t, body, `"device_code_id"`)
	require.Contains(t, body, `"verification_uri_complete":"https://auth.meta.com/device?user_code=WXYZ-1234"`)
	require.Contains(t, body, `"interval":3`)
}

func TestMetaOAuthHandlerPollCreatesAccount(t *testing.T) {
	doer := metaHandlerDoer()
	doer.tokenBodies = []string{
		`{"error":"authorization_pending"}`,
		`{"access_token":"dca:handler","token_type":"Bearer","expires_in":3600}`,
	}
	adminService := &metaAdminServiceStub{}
	router := newMetaHandlerTestRouter(t, doer, adminService)

	startRec := httptest.NewRecorder()
	startReq := httptest.NewRequest(http.MethodPost, "/api/v1/admin/meta/oauth/device/start", strings.NewReader(`{}`))
	startReq.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(startRec, startReq)
	require.Equal(t, http.StatusOK, startRec.Code)
	deviceCodeID := gjson.Get(startRec.Body.String(), "data.device_code_id").String()
	require.NotEmpty(t, deviceCodeID)

	pendingRec := httptest.NewRecorder()
	pendingReq := httptest.NewRequest(http.MethodPost, "/api/v1/admin/meta/oauth/device/poll", strings.NewReader(`{"device_code_id":"`+deviceCodeID+`"}`))
	pendingReq.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(pendingRec, pendingReq)
	require.Equal(t, http.StatusOK, pendingRec.Code)
	require.Contains(t, pendingRec.Body.String(), `"state":"pending"`)
	require.Nil(t, adminService.created)

	authorizedRec := httptest.NewRecorder()
	authorizedReq := httptest.NewRequest(http.MethodPost, "/api/v1/admin/meta/oauth/device/poll", strings.NewReader(`{"device_code_id":"`+deviceCodeID+`","name":"Muse prod","concurrency":3}`))
	authorizedReq.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(authorizedRec, authorizedReq)
	require.Equal(t, http.StatusOK, authorizedRec.Code)
	require.Contains(t, authorizedRec.Body.String(), `"state":"authorized"`)

	require.NotNil(t, adminService.created)
	require.Equal(t, service.PlatformMeta, adminService.created.Platform)
	require.Equal(t, service.AccountTypeOAuth, adminService.created.Type)
	require.Equal(t, "Muse prod", adminService.created.Name)
	require.Equal(t, 3, adminService.created.Concurrency)
	require.Equal(t, "sk-handler", adminService.created.Credentials["api_key"])
	require.Equal(t, "dca:handler", adminService.created.Credentials["dca_token"])
	require.Equal(t, service.MetaAuthKindOAuth, adminService.created.Credentials["auth_kind"])
	require.Contains(t, authorizedRec.Body.String(), `"has_api_key":true`)
	require.NotContains(t, authorizedRec.Body.String(), "sk-handler", "the stored api key never leaves the admin API")
}

func TestMetaOAuthHandlerPollCreatesOnlyOneAccount(t *testing.T) {
	doer := metaHandlerDoer()
	doer.tokenBodies = []string{`{"access_token":"dca:handler","token_type":"Bearer","expires_in":3600}`}
	adminService := &metaAdminServiceStub{}
	router := newMetaHandlerTestRouter(t, doer, adminService)

	startRec := httptest.NewRecorder()
	startReq := httptest.NewRequest(http.MethodPost, "/api/v1/admin/meta/oauth/device/start", strings.NewReader(`{}`))
	startReq.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(startRec, startReq)
	deviceCodeID := gjson.Get(startRec.Body.String(), "data.device_code_id").String()
	require.NotEmpty(t, deviceCodeID)

	for attempt := 0; attempt < 2; attempt++ {
		rec := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPost, "/api/v1/admin/meta/oauth/device/poll",
			strings.NewReader(`{"device_code_id":"`+deviceCodeID+`"}`))
		req.Header.Set("Content-Type", "application/json")
		router.ServeHTTP(rec, req)
		require.Equal(t, http.StatusOK, rec.Code)
		require.Contains(t, rec.Body.String(), `"state":"authorized"`)
	}
	require.NotNil(t, adminService.created)
	require.Equal(t, 1, adminService.createCalls, "a replayed poll must not create a second account")
}

func TestMetaOAuthHandlerPollReleasesClaimWhenCreateFails(t *testing.T) {
	doer := metaHandlerDoer()
	doer.tokenBodies = []string{`{"access_token":"dca:handler","token_type":"Bearer","expires_in":3600}`}
	adminService := &metaAdminServiceStub{createErr: errors.New("create failed")}
	router := newMetaHandlerTestRouter(t, doer, adminService)

	startRec := httptest.NewRecorder()
	startReq := httptest.NewRequest(http.MethodPost, "/api/v1/admin/meta/oauth/device/start", strings.NewReader(`{}`))
	startReq.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(startRec, startReq)
	require.Equal(t, http.StatusOK, startRec.Code)
	deviceCodeID := gjson.Get(startRec.Body.String(), "data.device_code_id").String()
	require.NotEmpty(t, deviceCodeID)

	failedRec := httptest.NewRecorder()
	failedReq := httptest.NewRequest(http.MethodPost, "/api/v1/admin/meta/oauth/device/poll",
		strings.NewReader(`{"device_code_id":"`+deviceCodeID+`"}`))
	failedReq.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(failedRec, failedReq)
	require.NotEqual(t, http.StatusOK, failedRec.Code)
	require.Equal(t, 1, adminService.createCalls)
	require.Nil(t, adminService.created)

	adminService.createErr = nil
	retryRec := httptest.NewRecorder()
	retryReq := httptest.NewRequest(http.MethodPost, "/api/v1/admin/meta/oauth/device/poll",
		strings.NewReader(`{"device_code_id":"`+deviceCodeID+`"}`))
	retryReq.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(retryRec, retryReq)
	require.Equal(t, http.StatusOK, retryRec.Code)
	require.Contains(t, retryRec.Body.String(), `"state":"authorized"`)
	require.NotNil(t, adminService.created)
	require.Equal(t, 2, adminService.createCalls)
}

func TestMetaOAuthHandlerPollRejectsUnknownSession(t *testing.T) {
	router := newMetaHandlerTestRouter(t, metaHandlerDoer(), &metaAdminServiceStub{})
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/admin/meta/oauth/device/poll", strings.NewReader(`{"device_code_id":"missing"}`))
	req.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(rec, req)
	require.Equal(t, http.StatusBadRequest, rec.Code)
}
