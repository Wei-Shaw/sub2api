package service

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

type memorySettingRepo struct {
	values map[string]string
}

func (r *memorySettingRepo) Get(ctx context.Context, key string) (*Setting, error) {
	if value, ok := r.values[key]; ok {
		return &Setting{Key: key, Value: value}, nil
	}
	return nil, ErrSettingNotFound
}

func (r *memorySettingRepo) GetValue(ctx context.Context, key string) (string, error) {
	if value, ok := r.values[key]; ok {
		return value, nil
	}
	return "", ErrSettingNotFound
}

func (r *memorySettingRepo) Set(ctx context.Context, key, value string) error {
	if r.values == nil {
		r.values = make(map[string]string)
	}
	r.values[key] = value
	return nil
}

func (r *memorySettingRepo) GetMultiple(ctx context.Context, keys []string) (map[string]string, error) {
	out := make(map[string]string, len(keys))
	for _, key := range keys {
		out[key] = r.values[key]
	}
	return out, nil
}

func (r *memorySettingRepo) SetMultiple(ctx context.Context, settings map[string]string) error {
	if r.values == nil {
		r.values = make(map[string]string)
	}
	for key, value := range settings {
		r.values[key] = value
	}
	return nil
}

func (r *memorySettingRepo) GetAll(ctx context.Context) (map[string]string, error) {
	out := make(map[string]string, len(r.values))
	for key, value := range r.values {
		out[key] = value
	}
	return out, nil
}

func (r *memorySettingRepo) Delete(ctx context.Context, key string) error {
	delete(r.values, key)
	return nil
}

func TestGeneratedImageURLForRequestUsesForwardedHost(t *testing.T) {
	gin.SetMode(gin.TestMode)
	req := httptest.NewRequest(http.MethodPost, "/v1/images/generations", nil)
	req.Host = "127.0.0.1"
	req.Header.Set("X-Forwarded-Proto", "https")
	req.Header.Set("X-Forwarded-Host", "sub2.70api.top")
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = req

	got := generatedImageURLForRequestWithBase(c, "img123", "")

	require.Equal(t, "https://sub2.70api.top/api/v1/generated-images/img123", got)
}

func TestGeneratedImageURLForRequestFallsBackToPublicBaseForInternalHost(t *testing.T) {
	gin.SetMode(gin.TestMode)
	req := httptest.NewRequest(http.MethodPost, "/v1/images/generations", nil)
	req.Host = "127.0.0.1"
	req.Header.Set("X-Forwarded-Proto", "http")
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = req

	got := generatedImageURLForRequestWithBase(c, "img123", "https://sub2.70api.top/v1")

	require.Equal(t, "https://sub2.70api.top/api/v1/generated-images/img123", got)
	require.False(t, strings.Contains(got, "127.0.0.1"))
}

func TestGeneratedImageURLForRequestParsesForwardedHeader(t *testing.T) {
	gin.SetMode(gin.TestMode)
	req := httptest.NewRequest(http.MethodPost, "/v1/images/generations", nil)
	req.Host = "127.0.0.1"
	req.Header.Set("Forwarded", `for=192.0.2.60;proto=https;host="sub2.70api.top"`)
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = req

	got := generatedImageURLForRequestWithBase(c, "img123", "")

	require.Equal(t, "https://sub2.70api.top/api/v1/generated-images/img123", got)
}
