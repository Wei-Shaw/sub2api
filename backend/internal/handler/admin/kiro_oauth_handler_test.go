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
	out      *service.KiroTokenInfo
	err      error
	gotRT    string
	gotProxy *int64
}

func (s *stubKiroSvc) ValidateSocialRefreshToken(_ context.Context, rt string, proxyID *int64) (*service.KiroTokenInfo, error) {
	s.gotRT = rt
	s.gotProxy = proxyID
	return s.out, s.err
}

func newKiroHandlerForTest(svc kiroOAuthSvc) (*gin.Engine, *KiroOAuthHandler) {
	gin.SetMode(gin.TestMode)
	h := &KiroOAuthHandler{svc: svc}
	r := gin.New()
	r.POST("/validate", h.ValidateSocialRefreshToken)
	return r, h
}

func TestKiroOAuthHandler_ValidateSocialRefreshToken_OK(t *testing.T) {
	stub := &stubKiroSvc{out: &service.KiroTokenInfo{
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
	stub := &stubKiroSvc{err: errors.New("upstream 400")}
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
