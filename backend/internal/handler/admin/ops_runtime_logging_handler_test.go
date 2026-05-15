package admin

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/pkg/logger"
	"github.com/Wei-Shaw/sub2api/internal/server/middleware"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
)

type testSettingRepo struct {
	values map[string]string
}

func newTestSettingRepo() *testSettingRepo {
	return &testSettingRepo{values: map[string]string{}}
}

func (s *testSettingRepo) Get(ctx context.Context, key string) (*service.Setting, error) {
	v, err := s.GetValue(ctx, key)
	if err != nil {
		return nil, err
	}
	return &service.Setting{Key: key, Value: v}, nil
}
func (s *testSettingRepo) GetValue(ctx context.Context, key string) (string, error) {
	v, ok := s.values[key]
	if !ok {
		return "", service.ErrSettingNotFound
	}
	return v, nil
}
func (s *testSettingRepo) Set(ctx context.Context, key, value string) error {
	s.values[key] = value
	return nil
}
func (s *testSettingRepo) GetMultiple(ctx context.Context, keys []string) (map[string]string, error) {
	out := make(map[string]string, len(keys))
	for _, k := range keys {
		if v, ok := s.values[k]; ok {
			out[k] = v
		}
	}
	return out, nil
}
func (s *testSettingRepo) SetMultiple(ctx context.Context, settings map[string]string) error {
	for k, v := range settings {
		s.values[k] = v
	}
	return nil
}
func (s *testSettingRepo) GetAll(ctx context.Context) (map[string]string, error) {
	out := make(map[string]string, len(s.values))
	for k, v := range s.values {
		out[k] = v
	}
	return out, nil
}
func (s *testSettingRepo) Delete(ctx context.Context, key string) error {
	delete(s.values, key)
	return nil
}

func newOpsRuntimeRouter(handler *OpsHandler, withUser bool) *gin.Engine {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	if withUser {
		r.Use(func(c *gin.Context) {
			c.Set(string(middleware.ContextKeyUser), middleware.AuthSubject{UserID: 7})
			c.Next()
		})
	}
	r.GET("/runtime/logging", handler.GetRuntimeLogConfig)
	r.PUT("/runtime/logging", handler.UpdateRuntimeLogConfig)
	r.POST("/runtime/logging/reset", handler.ResetRuntimeLogConfig)
	return r
}

func newRuntimeOpsService(t *testing.T) *service.OpsService {
	t.Helper()
	if err := logger.Init(logger.InitOptions{
		Level:       "info",
		Format:      "json",
		ServiceName: "sub2api",
		Environment: "test",
		Output: logger.OutputOptions{
			ToStdout: false,
			ToFile:   false,
		},
	}); err != nil {
		t.Fatalf("init logger: %v", err)
	}

	settingRepo := newTestSettingRepo()
	cfg := &config.Config{
		Ops: config.OpsConfig{Enabled: true},
		Log: config.LogConfig{
			Level:           "info",
			Caller:          true,
			StacktraceLevel: "error",
			Sampling: config.LogSamplingConfig{
				Enabled:    false,
				Initial:    100,
				Thereafter: 100,
			},
		},
	}
	return service.NewOpsService(nil, settingRepo, cfg, nil, nil, nil, nil, nil, nil, nil, nil)
}

func TestOpsRuntimeLoggingHandler_GetConfig(t *testing.T) {
	h := NewOpsHandler(newRuntimeOpsService(t), nil)
	r := newOpsRuntimeRouter(h, false)

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/runtime/logging", nil)
	r.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("status=%d, want 200", w.Code)
	}
}

func TestOpsRuntimeLoggingHandler_UpdateUnauthorized(t *testing.T) {
	h := NewOpsHandler(newRuntimeOpsService(t), nil)
	r := newOpsRuntimeRouter(h, false)

	body := `{"level":"debug","enable_sampling":false,"sampling_initial":100,"sampling_thereafter":100,"caller":true,"stacktrace_level":"error","retention_days":30}`
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPut, "/runtime/logging", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(w, req)
	if w.Code != http.StatusUnauthorized {
		t.Fatalf("status=%d, want 401", w.Code)
	}
}

func TestOpsRuntimeLoggingHandler_UpdateAndResetSuccess(t *testing.T) {
	h := NewOpsHandler(newRuntimeOpsService(t), nil)
	r := newOpsRuntimeRouter(h, true)

	payload := map[string]any{
		"level":               "debug",
		"enable_sampling":     false,
		"sampling_initial":    100,
		"sampling_thereafter": 100,
		"caller":              true,
		"stacktrace_level":    "error",
		"retention_days":      30,
	}
	raw, _ := json.Marshal(payload)

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPut, "/runtime/logging", bytes.NewReader(raw))
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("update status=%d, want 200, body=%s", w.Code, w.Body.String())
	}

	w = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodPost, "/runtime/logging/reset", nil)
	r.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("reset status=%d, want 200, body=%s", w.Code, w.Body.String())
	}
}
