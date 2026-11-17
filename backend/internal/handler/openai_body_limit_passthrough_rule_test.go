package handler

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/model"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

// errorPassthroughRepoStub is a minimal service.ErrorPassthroughRepository for
// handler-level rule tests. Rules are returned as-is; the mutating methods are
// never exercised here.
type errorPassthroughRepoStub struct {
	rules []*model.ErrorPassthroughRule
}

func (s *errorPassthroughRepoStub) List(context.Context) ([]*model.ErrorPassthroughRule, error) {
	return s.rules, nil
}

func (s *errorPassthroughRepoStub) GetByID(context.Context, int64) (*model.ErrorPassthroughRule, error) {
	return nil, nil
}

func (s *errorPassthroughRepoStub) Create(_ context.Context, rule *model.ErrorPassthroughRule) (*model.ErrorPassthroughRule, error) {
	return rule, nil
}

func (s *errorPassthroughRepoStub) Update(_ context.Context, rule *model.ErrorPassthroughRule) (*model.ErrorPassthroughRule, error) {
	return rule, nil
}

func (s *errorPassthroughRepoStub) Delete(context.Context, int64) error { return nil }

func newBodyLimitPassthroughHandler(t *testing.T, rule *model.ErrorPassthroughRule) *OpenAIGatewayHandler {
	t.Helper()
	repo := &errorPassthroughRepoStub{}
	if rule != nil {
		repo.rules = []*model.ErrorPassthroughRule{rule}
	}
	return &OpenAIGatewayHandler{
		errorPassthroughService: service.NewErrorPassthroughService(repo, nil),
	}
}

func newBodyLimitPassthroughRule(responseCode int, customMessage string) *model.ErrorPassthroughRule {
	msg := customMessage
	return &model.ErrorPassthroughRule{
		ID:              1,
		Name:            "body limit -> client error",
		Enabled:         true,
		Priority:        1,
		ErrorCodes:      []int{http.StatusRequestEntityTooLarge},
		MatchMode:       model.MatchModeAny,
		Platforms:       []string{model.PlatformOpenAI},
		PassthroughCode: false,
		ResponseCode:    &responseCode,
		PassthroughBody: false,
		CustomMessage:   &msg,
	}
}

// A configured 413 rule must win over the built-in body-limit mapping: the
// built-in branch returns 413 before the generic passthrough lookup runs, so a
// rule could otherwise never match on this path.
func TestOpenAIBodyLimitFailoverExhausted_PassthroughRuleOverridesBuiltInMapping(t *testing.T) {
	gin.SetMode(gin.TestMode)
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", bytes.NewReader(nil))

	h := newBodyLimitPassthroughHandler(t, newBodyLimitPassthroughRule(http.StatusBadRequest, "Request body exceeds the upstream limit"))

	h.handleFailoverExhausted(c, bodyLimitFailoverTestError(), false)

	require.Equal(t, http.StatusBadRequest, rec.Code)
	var envelope map[string]any
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &envelope))
	errBody, ok := envelope["error"].(map[string]any)
	require.True(t, ok)
	require.Equal(t, "upstream_error", errBody["type"])
	require.Equal(t, "Request body exceeds the upstream limit", errBody["message"])
	require.NotContains(t, rec.Body.String(), "must-not-leak")
}

// The override must also apply once the SSE stream has already started.
func TestOpenAIBodyLimitFailoverExhausted_PassthroughRuleAppliesToStartedStream(t *testing.T) {
	gin.SetMode(gin.TestMode)
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", bytes.NewReader(nil))

	h := newBodyLimitPassthroughHandler(t, newBodyLimitPassthroughRule(http.StatusBadRequest, "Request body exceeds the upstream limit"))

	h.handleFailoverExhausted(c, bodyLimitFailoverTestError(), true)

	body := rec.Body.String()
	require.Contains(t, body, `"message":"Request body exceeds the upstream limit"`)
	require.NotContains(t, body, "must-not-leak")
}

// Without a rule the built-in 413 mapping stays in place.
func TestOpenAIBodyLimitFailoverExhausted_NoRuleKeepsBuiltIn413(t *testing.T) {
	gin.SetMode(gin.TestMode)
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", bytes.NewReader(nil))

	h := newBodyLimitPassthroughHandler(t, nil)

	h.handleFailoverExhausted(c, bodyLimitFailoverTestError(), false)

	require.Equal(t, http.StatusRequestEntityTooLarge, rec.Code)
	require.Contains(t, rec.Body.String(), service.OpenAIRequestBodyTooLargeClientMessage)
}

// A rule scoped to a different upstream status must not affect the 413 path.
func TestOpenAIBodyLimitFailoverExhausted_NonMatchingRuleKeepsBuiltIn413(t *testing.T) {
	gin.SetMode(gin.TestMode)
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", bytes.NewReader(nil))

	rule := newBodyLimitPassthroughRule(http.StatusBadRequest, "Request body exceeds the upstream limit")
	rule.ErrorCodes = []int{http.StatusServiceUnavailable}
	h := newBodyLimitPassthroughHandler(t, rule)

	h.handleFailoverExhausted(c, bodyLimitFailoverTestError(), false)

	require.Equal(t, http.StatusRequestEntityTooLarge, rec.Code)
	require.Contains(t, rec.Body.String(), service.OpenAIRequestBodyTooLargeClientMessage)
}

// skip_monitoring on a matching rule must still be honored on this path.
func TestOpenAIBodyLimitFailoverExhausted_PassthroughRuleSkipMonitoring(t *testing.T) {
	gin.SetMode(gin.TestMode)
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", bytes.NewReader(nil))

	rule := newBodyLimitPassthroughRule(http.StatusBadRequest, "Request body exceeds the upstream limit")
	rule.SkipMonitoring = true
	h := newBodyLimitPassthroughHandler(t, rule)

	h.handleFailoverExhausted(c, bodyLimitFailoverTestError(), false)

	value, exists := c.Get(service.OpsSkipPassthroughKey)
	require.True(t, exists)
	require.Equal(t, true, value)
}
