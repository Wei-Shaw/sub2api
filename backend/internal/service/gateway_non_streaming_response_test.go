package service

import (
	"bytes"
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

type nonJSONTempUnschedAccountRepo struct {
	AccountRepository
	tempUnschedCalls int
	tempReason       string
REDACTED

func (r *nonJSONTempUnschedAccountRepo) SetTempUnschedulable(_ context.Context, _ int64, _ time.Time, reason string) error {
	r.tempUnschedCalls++
	r.tempReason = reason
	return nil
REDACTED

func TestHandleNonStreamingResponse_NonJSON2xxTriggersFailover(t *testing.T) {
	gin.SetMode(gin.TestMode)
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/messages", nil)

	body := []byte("(upstream request failed)")
	resp := &http.Response{
		StatusCode: http.StatusOK,
		Header: http.Header{
			"Content-Type": []string{"text/plain"REDACTED,
			"X-Request-Id": []string{"rid-invalid-json"REDACTED,
	REDACTED,
		Body: io.NopCloser(bytes.NewReader(body)),
REDACTED
	svc := &GatewayService{
		cfg:              &config.Config{REDACTED,
		rateLimitService: &RateLimitService{REDACTED,
REDACTED

	usage, err := svc.handleNonStreamingResponse(context.Background(), resp, c, &Account{ID: 1REDACTED, "claude-sonnet-4-6", "claude-sonnet-4-6")

	require.Nil(t, usage)
	var failoverErr *UpstreamFailoverError
	require.True(t, errors.As(err, &failoverErr))
	require.Equal(t, http.StatusBadGateway, failoverErr.StatusCode)
	require.Equal(t, body, failoverErr.ResponseBody)
	require.Equal(t, "rid-invalid-json", failoverErr.ResponseHeaders.Get("x-request-id"))
	require.False(t, c.Writer.Written(), "invalid upstream response must not be committed before failover")
REDACTED

func TestHandleNonStreamingResponse_ValidJSONUnchanged(t *testing.T) {
	gin.SetMode(gin.TestMode)
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/messages", nil)

	body := []byte(`{"id":"msg_1","type":"message","usage":{"input_tokens":12,"output_tokens":7REDACTEDREDACTED`)
	resp := &http.Response{
		StatusCode: http.StatusOK,
		Header:     http.Header{"Content-Type": []string{"application/json"REDACTEDREDACTED,
		Body:       io.NopCloser(bytes.NewReader(body)),
REDACTED
	svc := &GatewayService{
		cfg:              &config.Config{REDACTED,
		rateLimitService: &RateLimitService{REDACTED,
REDACTED

	usage, err := svc.handleNonStreamingResponse(context.Background(), resp, c, &Account{ID: 1REDACTED, "claude-sonnet-4-6", "claude-sonnet-4-6")

REDACTED
	require.NotNil(t, usage)
	require.Equal(t, 12, usage.InputTokens)
	require.Equal(t, 7, usage.OutputTokens)
	require.JSONEq(t, string(body), rec.Body.String())
REDACTED

func TestHandleNonStreamingResponseAnthropicAPIKeyPassthrough_NonJSON2xxTriggersFailover(t *testing.T) {
	gin.SetMode(gin.TestMode)
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/messages", nil)

	body := []byte("(upstream request failed)")
	resp := &http.Response{
		StatusCode: http.StatusOK,
		Header:     http.Header{"Content-Type": []string{"text/plain"REDACTEDREDACTED,
		Body:       io.NopCloser(bytes.NewReader(body)),
REDACTED
	svc := &GatewayService{cfg: &config.Config{REDACTEDREDACTED

	usage, err := svc.handleNonStreamingResponseAnthropicAPIKeyPassthrough(context.Background(), resp, c, &Account{ID: 2REDACTED)

	require.Nil(t, usage)
	var failoverErr *UpstreamFailoverError
	require.True(t, errors.As(err, &failoverErr))
	require.Equal(t, http.StatusBadGateway, failoverErr.StatusCode)
	require.Equal(t, body, failoverErr.ResponseBody)
	require.False(t, c.Writer.Written(), "invalid passthrough response must not be committed before failover")
REDACTED

func TestHandleNonStreamingResponseAnthropicAPIKeyPassthrough_ValidJSONUnchanged(t *testing.T) {
	gin.SetMode(gin.TestMode)
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/messages", nil)

	body := []byte(`{"id":"msg_1","type":"message","usage":{"input_tokens":5,"output_tokens":3REDACTEDREDACTED`)
	resp := &http.Response{
		StatusCode: http.StatusOK,
		Header:     http.Header{"Content-Type": []string{"application/json"REDACTEDREDACTED,
		Body:       io.NopCloser(bytes.NewReader(body)),
REDACTED
	svc := &GatewayService{cfg: &config.Config{REDACTEDREDACTED

	usage, err := svc.handleNonStreamingResponseAnthropicAPIKeyPassthrough(context.Background(), resp, c, &Account{ID: 2REDACTED)

REDACTED
	require.NotNil(t, usage)
	require.Equal(t, 5, usage.InputTokens)
	require.Equal(t, 3, usage.OutputTokens)
	require.JSONEq(t, string(body), rec.Body.String())
REDACTED

func TestHandleNonStreamingResponse_NonJSON2xxMatchesTempUnschedulableRule(t *testing.T) {
	gin.SetMode(gin.TestMode)
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/messages", nil)

	repo := &nonJSONTempUnschedAccountRepo{REDACTED
	rateLimitService := NewRateLimitService(repo, nil, &config.Config{REDACTED, nil, nil)
	svc := &GatewayService{
		cfg:              &config.Config{REDACTED,
		rateLimitService: rateLimitService,
REDACTED
	account := &Account{
		ID:       3,
		Platform: PlatformAnthropic,
		Type:     AccountTypeAPIKey,
REDACTED
			"temp_unschedulable_enabled": true,
			"temp_unschedulable_rules": []any{
				map[string]any{
					"error_code":       float64(http.StatusBadGateway),
					"keywords":         []any{"upstream request failed"REDACTED,
					"duration_minutes": float64(10),
			REDACTED,
		REDACTED,
	REDACTED,
REDACTED
	body := []byte("(upstream request failed)")
	resp := &http.Response{
		StatusCode: http.StatusOK,
		Header:     http.Header{REDACTED,
		Body:       io.NopCloser(bytes.NewReader(body)),
REDACTED

	_, err := svc.handleNonStreamingResponse(context.Background(), resp, c, account, "claude-sonnet-4-6", "claude-sonnet-4-6")

	var failoverErr *UpstreamFailoverError
	require.True(t, errors.As(err, &failoverErr))
	require.Equal(t, http.StatusBadGateway, failoverErr.StatusCode)
	require.Equal(t, body, failoverErr.ResponseBody)
	require.Equal(t, 1, repo.tempUnschedCalls)
	require.Contains(t, repo.tempReason, `"status_code":502`)
	require.Contains(t, repo.tempReason, `"matched_keyword":"upstream request failed"`)
REDACTED
