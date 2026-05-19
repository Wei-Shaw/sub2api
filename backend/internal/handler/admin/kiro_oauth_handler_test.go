package admin

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"

	"github.com/Wei-Shaw/sub2api/internal/service"
)

type stubKiroSvc struct {
	socialOut   *service.KiroTokenInfo
	socialErr   error
	gotRT       string
	gotProxy    *int64
	idcStartOut *service.KiroIdCAuthURLResult
	idcStartErr error
	idcCmpltOut *service.KiroTokenInfo
	idcCmpltErr error
	bidStartOut *service.KiroBuilderIDLoginResult
	bidStartErr error
	bidPollOut  *service.KiroBuilderIDPollResult
	bidPollErr  error
}

func (s *stubKiroSvc) ValidateSocialRefreshToken(_ context.Context, rt string, proxyID *int64) (*service.KiroTokenInfo, error) {
	s.gotRT = rt
	s.gotProxy = proxyID
	return s.socialOut, s.socialErr
}

func (s *stubKiroSvc) StartIdCLogin(_ context.Context, _, _ string, _ *int64) (*service.KiroIdCAuthURLResult, error) {
	return s.idcStartOut, s.idcStartErr
}

func (s *stubKiroSvc) CompleteIdCLogin(_ context.Context, _, _ string) (*service.KiroTokenInfo, error) {
	return s.idcCmpltOut, s.idcCmpltErr
}

func (s *stubKiroSvc) StartBuilderIDLogin(_ context.Context, _ string, _ *int64) (*service.KiroBuilderIDLoginResult, error) {
	return s.bidStartOut, s.bidStartErr
}

func (s *stubKiroSvc) PollBuilderIDLogin(_ context.Context, _ string) (*service.KiroBuilderIDPollResult, error) {
	return s.bidPollOut, s.bidPollErr
}

func newKiroHandlerForTest(svc kiroOAuthSvc) (*gin.Engine, *KiroOAuthHandler) {
	gin.SetMode(gin.TestMode)
	h := &KiroOAuthHandler{svc: svc}
	r := gin.New()
	r.POST("/validate", h.ValidateSocialRefreshToken)
	r.POST("/idc/start", h.StartIdCLogin)
	r.POST("/idc/complete", h.CompleteIdCLogin)
	r.POST("/builderid/start", h.StartBuilderIDLogin)
	r.POST("/builderid/poll", h.PollBuilderIDLogin)
	return r, h
}

func TestKiroOAuthHandler_ValidateSocialRefreshToken_OK(t *testing.T) {
	stub := &stubKiroSvc{socialOut: &service.KiroTokenInfo{
		AccessToken:  "at",
		RefreshToken: "rt2",
		ExpiresAt:    9999,
		AuthMethod:   "social",
		Email:        "u@e.com",
	}}
	r, _ := newKiroHandlerForTest(stub)

	pid := int64(7)
	body, _ := json.Marshal(map[string]any{"refresh_token": "rt-1", "proxy_id": pid})
	req := httptest.NewRequest(http.MethodPost, "/validate", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status %d body %s", w.Code, w.Body.String())
	}
	var env struct {
		Code int                    `json:"code"`
		Data *service.KiroTokenInfo `json:"data"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &env); err != nil {
		t.Fatalf("decode: %v body=%s", err, w.Body.String())
	}
	if env.Code != 0 || env.Data == nil || env.Data.AccessToken != "at" || env.Data.Email != "u@e.com" {
		t.Fatalf("unexpected envelope: %+v", env)
	}
	if stub.gotRT != "rt-1" {
		t.Fatalf("service got rt=%q", stub.gotRT)
	}
	if stub.gotProxy == nil || *stub.gotProxy != 7 {
		t.Fatalf("service got proxy=%v", stub.gotProxy)
	}
}

func TestKiroOAuthHandler_ValidateSocialRefreshToken_MissingBody(t *testing.T) {
	stub := &stubKiroSvc{}
	r, _ := newKiroHandlerForTest(stub)

	req := httptest.NewRequest(http.MethodPost, "/validate", bytes.NewReader([]byte(`{}`)))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d body %s", w.Code, w.Body.String())
	}
	if stub.gotRT != "" {
		t.Fatalf("service should not have been called when refresh_token missing")
	}
}

func TestKiroOAuthHandler_ValidateSocialRefreshToken_ServiceError(t *testing.T) {
	stub := &stubKiroSvc{socialErr: errors.New("upstream 400")}
	r, _ := newKiroHandlerForTest(stub)

	body, _ := json.Marshal(map[string]any{"refresh_token": "rt"})
	req := httptest.NewRequest(http.MethodPost, "/validate", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code == http.StatusOK {
		t.Fatalf("expected non-200, got %d body %s", w.Code, w.Body.String())
	}
}

func TestKiroOAuthHandler_StartIdCLogin_OK(t *testing.T) {
	stub := &stubKiroSvc{idcStartOut: &service.KiroIdCAuthURLResult{
		AuthURL:   "https://oidc.example/authorize?x=1",
		SessionID: "sid-1",
		ExpiresIn: 600,
	}}
	r, _ := newKiroHandlerForTest(stub)

	body, _ := json.Marshal(map[string]any{"start_url": "https://start", "region": "us-east-1"})
	req := httptest.NewRequest(http.MethodPost, "/idc/start", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status %d body %s", w.Code, w.Body.String())
	}
}

func TestKiroOAuthHandler_StartIdCLogin_MissingStartURL(t *testing.T) {
	r, _ := newKiroHandlerForTest(&stubKiroSvc{})
	req := httptest.NewRequest(http.MethodPost, "/idc/start", bytes.NewReader([]byte(`{}`)))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d body %s", w.Code, w.Body.String())
	}
}

func TestKiroOAuthHandler_CompleteIdCLogin_OK(t *testing.T) {
	stub := &stubKiroSvc{idcCmpltOut: &service.KiroTokenInfo{AccessToken: "at", AuthMethod: "idc"}}
	r, _ := newKiroHandlerForTest(stub)

	body, _ := json.Marshal(map[string]any{
		"session_id":   "sid-1",
		"callback_url": "http://127.0.0.1/oauth/callback?code=c&state=s",
	})
	req := httptest.NewRequest(http.MethodPost, "/idc/complete", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("status %d body %s", w.Code, w.Body.String())
	}
}

func TestKiroOAuthHandler_StartBuilderIDLogin_AllowsEmptyBody(t *testing.T) {
	stub := &stubKiroSvc{bidStartOut: &service.KiroBuilderIDLoginResult{
		SessionID: "bid-1", UserCode: "ABCD", VerificationURI: "https://x", Interval: 5,
	}}
	r, _ := newKiroHandlerForTest(stub)

	// Empty body — region defaults
	req := httptest.NewRequest(http.MethodPost, "/builderid/start", bytes.NewReader([]byte(`{}`)))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("status %d body %s", w.Code, w.Body.String())
	}
}

func TestKiroOAuthHandler_PollBuilderIDLogin_OK(t *testing.T) {
	stub := &stubKiroSvc{bidPollOut: &service.KiroBuilderIDPollResult{Status: "pending"}}
	r, _ := newKiroHandlerForTest(stub)

	body, _ := json.Marshal(map[string]any{"session_id": "bid-1"})
	req := httptest.NewRequest(http.MethodPost, "/builderid/poll", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("status %d body %s", w.Code, w.Body.String())
	}
	var env struct {
		Code int                              `json:"code"`
		Data *service.KiroBuilderIDPollResult `json:"data"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &env); err != nil {
		t.Fatal(err)
	}
	if env.Data == nil || env.Data.Status != "pending" {
		t.Fatalf("unexpected: %+v", env)
	}
}

func TestKiroOAuthHandler_PollBuilderIDLogin_MissingSessionID(t *testing.T) {
	r, _ := newKiroHandlerForTest(&stubKiroSvc{})
	req := httptest.NewRequest(http.MethodPost, "/builderid/poll", bytes.NewReader([]byte(`{}`)))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d body %s", w.Code, w.Body.String())
	}
}
