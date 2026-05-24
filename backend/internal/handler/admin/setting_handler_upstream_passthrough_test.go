package admin

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/pkg/response"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

// upstreamPassthroughRepoStub is a SettingRepository test double that supports
// both single-key Get/Set and the batched Get/SetMultiple paths used by the
// admin UpdateSettings handler.
type upstreamPassthroughRepoStub struct {
	mu     sync.Mutex
	values map[string]string
}

func newUpstreamPassthroughRepoStub() *upstreamPassthroughRepoStub {
	return &upstreamPassthroughRepoStub{values: map[string]string{}}
}

func (s *upstreamPassthroughRepoStub) Get(ctx context.Context, key string) (*service.Setting, error) {
	return nil, service.ErrSettingNotFound
}

func (s *upstreamPassthroughRepoStub) GetValue(ctx context.Context, key string) (string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if v, ok := s.values[key]; ok {
		return v, nil
	}
	return "", nil
}

func (s *upstreamPassthroughRepoStub) Set(ctx context.Context, key, value string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.values[key] = value
	return nil
}

func (s *upstreamPassthroughRepoStub) GetMultiple(ctx context.Context, keys []string) (map[string]string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make(map[string]string, len(keys))
	for _, key := range keys {
		if value, ok := s.values[key]; ok {
			out[key] = value
		}
	}
	return out, nil
}

func (s *upstreamPassthroughRepoStub) SetMultiple(ctx context.Context, settings map[string]string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	for key, value := range settings {
		s.values[key] = value
	}
	return nil
}

func (s *upstreamPassthroughRepoStub) GetAll(ctx context.Context) (map[string]string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make(map[string]string, len(s.values))
	for key, value := range s.values {
		out[key] = value
	}
	return out, nil
}

func (s *upstreamPassthroughRepoStub) Delete(ctx context.Context, key string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.values, key)
	return nil
}

func newUpstreamPassthroughTestHandler(t *testing.T) (*SettingHandler, *upstreamPassthroughRepoStub) {
	t.Helper()
	gin.SetMode(gin.TestMode)
	repo := newUpstreamPassthroughRepoStub()
	svc := service.NewSettingService(repo, &config.Config{Default: config.DefaultConfig{UserConcurrency: 5}})
	handler := NewSettingHandler(svc, nil, nil, nil, nil, nil, nil)
	return handler, repo
}

// TestSettingHandler_UpdateUpstreamPassthrough_RoundTrip exercises a PUT with a
// custom upstream passthrough config (defaults, global override, feature flag),
// then a subsequent GET, and asserts the GET response echoes the PUT body
// after normalization (unknown overrides dropped, profiles preserved).
func TestSettingHandler_UpdateUpstreamPassthrough_RoundTrip(t *testing.T) {
	handler, repo := newUpstreamPassthroughTestHandler(t)

	body := map[string]any{
		"upstream_passthrough_defaults": map[string]any{
			"relay": map[string]any{
				"profile": "protected",
				"overrides": map[string]any{
					"forward_client_headers": true,
					"unknown_key":            true, // should be silently dropped
				},
			},
			"official": map[string]any{
				"profile": "strict",
			},
			"reverse": map[string]any{
				"profile": "transparent",
			},
		},
		"upstream_passthrough_global_override": "force_protected",
		"upstream_policy_v1_enabled":           true,
	}
	rawBody, err := json.Marshal(body)
	require.NoError(t, err)

	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPut, "/api/v1/admin/settings", bytes.NewReader(rawBody))
	c.Request.Header.Set("Content-Type", "application/json")

	handler.UpdateSettings(c)
	require.Equal(t, http.StatusOK, rec.Code, "PUT body=%s resp=%s", string(rawBody), rec.Body.String())

	// Verify persisted setting keys directly.
	require.Equal(t, "force_protected", repo.values[service.SettingKeyUpstreamPassthroughGlobalOverride])
	require.Equal(t, "true", repo.values[service.SettingKeyUpstreamPolicyV1Enabled])

	persistedDefaults := repo.values[service.SettingKeyUpstreamPassthroughDefaults]
	require.NotEmpty(t, persistedDefaults)
	var parsed service.UpstreamPassthroughDefaults
	require.NoError(t, json.Unmarshal([]byte(persistedDefaults), &parsed))
	require.Equal(t, service.ProfileProtected, parsed.Relay.Profile)
	require.Equal(t, service.ProfileStrict, parsed.Official.Profile)
	require.Equal(t, service.ProfileTransparent, parsed.Reverse.Profile)
	// Known override preserved; unknown key dropped during normalization.
	require.True(t, parsed.Relay.Overrides[service.ToggleForwardClientHeaders])
	_, unknownPresent := parsed.Relay.Overrides["unknown_key"]
	require.False(t, unknownPresent)

	// Now exercise the GET path to confirm the response payload echoes back.
	getRec := httptest.NewRecorder()
	gc, _ := gin.CreateTestContext(getRec)
	gc.Request = httptest.NewRequest(http.MethodGet, "/api/v1/admin/settings", nil)
	handler.GetSettings(gc)
	require.Equal(t, http.StatusOK, getRec.Code, "GET resp=%s", getRec.Body.String())

	var resp response.Response
	require.NoError(t, json.Unmarshal(getRec.Body.Bytes(), &resp))
	data, ok := resp.Data.(map[string]any)
	require.True(t, ok)

	require.Equal(t, "force_protected", data["upstream_passthrough_global_override"])
	require.Equal(t, true, data["upstream_policy_v1_enabled"])

	defaults, ok := data["upstream_passthrough_defaults"].(map[string]any)
	require.True(t, ok, "missing upstream_passthrough_defaults in response")
	relay, ok := defaults["relay"].(map[string]any)
	require.True(t, ok)
	require.Equal(t, "protected", relay["profile"])
	overrides, ok := relay["overrides"].(map[string]any)
	require.True(t, ok)
	require.Equal(t, true, overrides[service.ToggleForwardClientHeaders])
	_, unknownEcho := overrides["unknown_key"]
	require.False(t, unknownEcho, "unknown override key must not survive normalization")

	official, ok := defaults["official"].(map[string]any)
	require.True(t, ok)
	require.Equal(t, "strict", official["profile"])

	reverse, ok := defaults["reverse"].(map[string]any)
	require.True(t, ok)
	require.Equal(t, "transparent", reverse["profile"])
}

// TestSettingHandler_UpdateUpstreamPassthrough_RejectsInvalidProfile ensures
// the handler surfaces SetUpstreamPassthroughDefaults validation errors as a
// 400 Bad Request and does not persist the invalid value.
func TestSettingHandler_UpdateUpstreamPassthrough_RejectsInvalidProfile(t *testing.T) {
	handler, repo := newUpstreamPassthroughTestHandler(t)

	body := map[string]any{
		"upstream_passthrough_defaults": map[string]any{
			"relay": map[string]any{
				"profile": "definitely-not-a-profile",
			},
			"official": map[string]any{"profile": "protected"},
			"reverse":  map[string]any{"profile": "strict"},
		},
	}
	rawBody, err := json.Marshal(body)
	require.NoError(t, err)

	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPut, "/api/v1/admin/settings", bytes.NewReader(rawBody))
	c.Request.Header.Set("Content-Type", "application/json")

	handler.UpdateSettings(c)
	require.Equal(t, http.StatusBadRequest, rec.Code, "expected 400, body=%s", rec.Body.String())
	require.Empty(t, repo.values[service.SettingKeyUpstreamPassthroughDefaults],
		"must not persist invalid upstream_passthrough_defaults")
}

// TestSettingHandler_UpdateUpstreamPassthrough_RejectsInvalidGlobalOverride
// verifies the global-override kill switch rejects unknown values.
func TestSettingHandler_UpdateUpstreamPassthrough_RejectsInvalidGlobalOverride(t *testing.T) {
	handler, repo := newUpstreamPassthroughTestHandler(t)

	body := map[string]any{
		"upstream_passthrough_global_override": "force_chaos",
	}
	rawBody, err := json.Marshal(body)
	require.NoError(t, err)

	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPut, "/api/v1/admin/settings", bytes.NewReader(rawBody))
	c.Request.Header.Set("Content-Type", "application/json")

	handler.UpdateSettings(c)
	require.Equal(t, http.StatusBadRequest, rec.Code, "expected 400, body=%s", rec.Body.String())
	require.Empty(t, repo.values[service.SettingKeyUpstreamPassthroughGlobalOverride],
		"must not persist invalid upstream_passthrough_global_override")
}
