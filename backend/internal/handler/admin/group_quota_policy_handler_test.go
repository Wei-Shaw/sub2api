package admin

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/config"
	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

type quotaPolicyHandlerStub struct {
	groupQuotaPolicyService
	calls             int
	groupID, revision int64
	err               error
}

func (s *quotaPolicyHandlerStub) Models(_ context.Context, id int64) (*service.GroupQuotaModels, error) {
	s.calls++
	s.groupID = id
	return &service.GroupQuotaModels{GroupID: id, Scope: "group"}, s.err
}

func (s *quotaPolicyHandlerStub) ValidateMapping(_ context.Context, id int64, req service.GroupQuotaMappingRequest) (*service.GroupQuotaMappingValidation, error) {
	s.calls++
	s.groupID = id
	return &service.GroupQuotaMappingValidation{GroupID: id, Source: req.Source, Target: req.Target, Scope: "group", Status: "untested"}, s.err
}

func TestGroupQuotaPolicyDiagnosticsRequestValidation(t *testing.T) {
	gin.SetMode(gin.TestMode)
	for _, tc := range []struct {
		name, id, body string
		status         int
		simple         bool
	}{
		{"valid", "7", `{"source":"source","target":"target","account_ids":[1],"force":true}`, 200, false},
		{"unknown field", "7", `{"source":"source","target":"target","account_ids":[1],"api_key":"private"}`, 400, false},
		{"multiple bodies", "7", `{} {}`, 400, false},
		{"bad group", "-1", `{}`, 400, false},
		{"malformed", "7", `{`, 400, false},
		{"simple mode", "7", `{}`, http.StatusForbidden, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			stub := &quotaPolicyHandlerStub{}
			h := &GroupHandler{quotaPolicyService: stub}
			if tc.simple {
				h.cfg = &config.Config{RunMode: config.RunModeSimple}
			}
			r := gin.New()
			r.POST("/groups/:id/quota-policy/validate-mapping", h.ValidateQuotaPolicyMapping)
			w := httptest.NewRecorder()
			r.ServeHTTP(w, httptest.NewRequest(http.MethodPost, "/groups/"+tc.id+"/quota-policy/validate-mapping", strings.NewReader(tc.body)))
			require.Equal(t, tc.status, w.Code, w.Body.String())
			if tc.status == 200 {
				require.Equal(t, 1, stub.calls)
				require.Equal(t, int64(7), stub.groupID)
			} else {
				require.Zero(t, stub.calls)
			}
		})
	}
	stub := &quotaPolicyHandlerStub{}
	h := &GroupHandler{quotaPolicyService: stub}
	r := gin.New()
	r.GET("/groups/:id/quota-policy/models", h.GetQuotaPolicyModels)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/groups/7/quota-policy/models", nil))
	require.Equal(t, 200, w.Code, w.Body.String())
	require.Equal(t, int64(7), stub.groupID)
}

func (s *quotaPolicyHandlerStub) Save(_ context.Context, id, revision int64, _ service.GroupQuotaPolicyConfig) (*service.GroupQuotaPolicyView, error) {
	s.calls++
	s.groupID, s.revision = id, revision
	return &service.GroupQuotaPolicyView{}, s.err
}

func TestGroupQuotaPolicySaveValidation(t *testing.T) {
	gin.SetMode(gin.TestMode)
	for _, tc := range []struct {
		name, id, body string
		status         int
	}{
		{"valid first save", "7", `{"revision":0,"config":{"rules":[]}}`, 200},
		{"missing revision", "7", `{"config":{"rules":[]}}`, 400},
		{"missing config", "7", `{"revision":0}`, 400},
		{"negative revision", "7", `{"revision":-1,"config":{"rules":[]}}`, 400},
		{"invalid group", "0", `{"revision":0,"config":{"rules":[]}}`, 400},
		{"unknown field", "7", `{"revision":0,"config":{"rules":[]},"credentials":{}}`, 400},
		{"multiple documents", "7", `{"revision":0,"config":{"rules":[]}} {}`, 400},
		{"oversize", "7", `{"revision":0,"config":{"rules":[]},"` + strings.Repeat("x", 128<<10) + `":0}`, 400},
	} {
		t.Run(tc.name, func(t *testing.T) {
			stub := &quotaPolicyHandlerStub{}
			h := &GroupHandler{quotaPolicyService: stub}
			r := gin.New()
			r.PUT("/groups/:id/quota-policy", h.SaveQuotaPolicy)
			w := httptest.NewRecorder()
			r.ServeHTTP(w, httptest.NewRequest(http.MethodPut, "/groups/"+tc.id+"/quota-policy", strings.NewReader(tc.body)))
			require.Equal(t, tc.status, w.Code, w.Body.String())
			if tc.status == 200 {
				require.Equal(t, 1, stub.calls)
				require.Equal(t, int64(7), stub.groupID)
				require.Zero(t, stub.revision)
			} else {
				require.Zero(t, stub.calls)
			}
		})
	}
}

func TestGroupQuotaPolicySaveConflict(t *testing.T) {
	gin.SetMode(gin.TestMode)
	stub := &quotaPolicyHandlerStub{err: infraerrors.New(http.StatusConflict, "QUOTA_POLICY_CONFLICT", "Reload the policy before saving")}
	h := &GroupHandler{quotaPolicyService: stub}
	r := gin.New()
	r.PUT("/groups/:id/quota-policy", h.SaveQuotaPolicy)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodPut, "/groups/7/quota-policy", strings.NewReader(`{"revision":4,"config":{"rules":[]}}`)))
	require.Equal(t, http.StatusConflict, w.Code)
	require.Contains(t, w.Body.String(), "QUOTA_POLICY_CONFLICT")
	require.Equal(t, int64(4), stub.revision)
}

func TestGroupQuotaPolicySimpleModeDoesNotMutate(t *testing.T) {
	gin.SetMode(gin.TestMode)
	stub := &quotaPolicyHandlerStub{}
	h := &GroupHandler{quotaPolicyService: stub, cfg: &config.Config{RunMode: config.RunModeSimple}}
	r := gin.New()
	r.PUT("/groups/:id/quota-policy", h.SaveQuotaPolicy)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodPut, "/groups/7/quota-policy", strings.NewReader(`{"revision":0,"config":{"rules":[]}}`)))
	require.GreaterOrEqual(t, w.Code, 400)
	require.Less(t, w.Code, 500)
	require.Zero(t, stub.calls)
}

func TestGroupQuotaPolicyUnavailable(t *testing.T) {
	gin.SetMode(gin.TestMode)
	h := &GroupHandler{quotaPolicyService: &quotaPolicyHandlerStub{}}
	h.SetQuotaPolicyService(nil)
	r := gin.New()
	r.GET("/groups/:id/quota-policy", h.GetQuotaPolicy)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/groups/7/quota-policy", nil))
	require.Equal(t, http.StatusServiceUnavailable, w.Code)
}
