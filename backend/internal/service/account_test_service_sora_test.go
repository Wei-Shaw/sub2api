package service

import (
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

type queuedHTTPUpstream struct {
	responses []*http.Response
	requests  []*http.Request
	tlsFlags  []bool
REDACTED

func (u *queuedHTTPUpstream) Do(_ *http.Request, _ string, _ int64, _ int) (*http.Response, error) {
	return nil, fmt.Errorf("unexpected Do call")
REDACTED

func (u *queuedHTTPUpstream) DoWithTLS(req *http.Request, _ string, _ int64, _ int, enableTLSFingerprint bool) (*http.Response, error) {
	u.requests = append(u.requests, req)
	u.tlsFlags = append(u.tlsFlags, enableTLSFingerprint)
	if len(u.responses) == 0 {
		return nil, fmt.Errorf("no mocked response")
REDACTED
	resp := u.responses[0]
	u.responses = u.responses[1:]
	return resp, nil
REDACTED

func newJSONResponse(status int, body string) *http.Response {
	return &http.Response{
		StatusCode: status,
		Header:     make(http.Header),
		Body:       io.NopCloser(strings.NewReader(body)),
REDACTED
REDACTED

func newJSONResponseWithHeader(status int, body, key, value string) *http.Response {
	resp := newJSONResponse(status, body)
	resp.Header.Set(key, value)
	return resp
REDACTED

func newSoraTestContext() (*gin.Context, *httptest.ResponseRecorder) {
	gin.SetMode(gin.TestMode)
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPost, "/api/v1/admin/accounts/1/test", nil)
	return c, rec
REDACTED

func TestAccountTestService_testSoraAccountConnection_WithSubscription(t *testing.T) {
	upstream := &queuedHTTPUpstream{
		responses: []*http.Response{
			newJSONResponse(http.StatusOK, `{"email":"demo@example.com"REDACTED`),
			newJSONResponse(http.StatusOK, `{"data":[{"plan":{"id":"chatgpt_plus","title":"ChatGPT Plus"REDACTED,"end_ts":"2026-12-31T00:00:00Z"REDACTED]REDACTED`),
	REDACTED,
REDACTED
	svc := &AccountTestService{
		httpUpstream: upstream,
		cfg: &config.Config{
			Gateway: config.GatewayConfig{
				TLSFingerprint: config.TLSFingerprintConfig{
					Enabled: true,
			REDACTED,
		REDACTED,
			Sora: config.SoraConfig{
				Client: config.SoraClientConfig{
					DisableTLSFingerprint: false,
			REDACTED,
		REDACTED,
	REDACTED,
REDACTED
	account := &Account{
		ID:          1,
		Platform:    PlatformSora,
		Type:        AccountTypeOAuth,
		Concurrency: 1,
REDACTED
			"access_token": "test_token",
	REDACTED,
REDACTED

	c, rec := newSoraTestContext()
	err := svc.testSoraAccountConnection(c, account)

REDACTED
	require.Len(t, upstream.requests, 2)
	require.Equal(t, soraMeAPIURL, upstream.requests[0].URL.String())
	require.Equal(t, soraBillingAPIURL, upstream.requests[1].URL.String())
	require.Equal(t, "Bearer test_token", upstream.requests[0].Header.Get("Authorization"))
	require.Equal(t, "Bearer test_token", upstream.requests[1].Header.Get("Authorization"))
	require.Equal(t, []bool{true, trueREDACTED, upstream.tlsFlags)

	body := rec.Body.String()
	require.Contains(t, body, `"type":"test_start"`)
	require.Contains(t, body, "Sora connection OK - Email: demo@example.com")
	require.Contains(t, body, "Subscription: ChatGPT Plus | chatgpt_plus | end=2026-12-31T00:00:00Z")
	require.Contains(t, body, `"type":"test_complete","success":true`)
REDACTED

func TestAccountTestService_testSoraAccountConnection_SubscriptionFailedStillSuccess(t *testing.T) {
	upstream := &queuedHTTPUpstream{
		responses: []*http.Response{
			newJSONResponse(http.StatusOK, `{"name":"demo-user"REDACTED`),
			newJSONResponse(http.StatusForbidden, `{"error":{"message":"forbidden"REDACTEDREDACTED`),
	REDACTED,
REDACTED
	svc := &AccountTestService{httpUpstream: upstreamREDACTED
	account := &Account{
		ID:          1,
		Platform:    PlatformSora,
		Type:        AccountTypeOAuth,
		Concurrency: 1,
REDACTED
			"access_token": "test_token",
	REDACTED,
REDACTED

	c, rec := newSoraTestContext()
	err := svc.testSoraAccountConnection(c, account)

REDACTED
	require.Len(t, upstream.requests, 2)
	body := rec.Body.String()
	require.Contains(t, body, "Sora connection OK - User: demo-user")
	require.Contains(t, body, "Subscription check returned 403")
	require.Contains(t, body, `"type":"test_complete","success":true`)
REDACTED

func TestAccountTestService_testSoraAccountConnection_CloudflareChallenge(t *testing.T) {
	upstream := &queuedHTTPUpstream{
		responses: []*http.Response{
			newJSONResponseWithHeader(http.StatusForbidden, `<!DOCTYPE html><html><head><title>Just a moment...</title></head><body><script>window._cf_chl_opt={REDACTED;</script><noscript>Enable JavaScript and cookies to continue</noscript></body></html>`, "cf-ray", "9cff2d62d83bb98d"),
	REDACTED,
REDACTED
	svc := &AccountTestService{httpUpstream: upstreamREDACTED
	account := &Account{
		ID:          1,
		Platform:    PlatformSora,
		Type:        AccountTypeOAuth,
		Concurrency: 1,
REDACTED
			"access_token": "test_token",
	REDACTED,
REDACTED

	c, rec := newSoraTestContext()
	err := svc.testSoraAccountConnection(c, account)

REDACTED
	require.Contains(t, err.Error(), "Cloudflare challenge")
	require.Contains(t, err.Error(), "cf-ray: 9cff2d62d83bb98d")
	body := rec.Body.String()
	require.Contains(t, body, `"type":"error"`)
	require.Contains(t, body, "Cloudflare challenge")
	require.Contains(t, body, "cf-ray: 9cff2d62d83bb98d")
REDACTED

func TestAccountTestService_testSoraAccountConnection_SubscriptionCloudflareChallengeWithRay(t *testing.T) {
	upstream := &queuedHTTPUpstream{
		responses: []*http.Response{
			newJSONResponse(http.StatusOK, `{"name":"demo-user"REDACTED`),
			newJSONResponse(http.StatusForbidden, `<!DOCTYPE html><html><head><title>Just a moment...</title></head><body><script>window._cf_chl_opt={cRay: '9cff2d62d83bb98d'REDACTED;</script><noscript>Enable JavaScript and cookies to continue</noscript></body></html>`),
	REDACTED,
REDACTED
	svc := &AccountTestService{httpUpstream: upstreamREDACTED
	account := &Account{
		ID:          1,
		Platform:    PlatformSora,
		Type:        AccountTypeOAuth,
		Concurrency: 1,
REDACTED
			"access_token": "test_token",
	REDACTED,
REDACTED

	c, rec := newSoraTestContext()
	err := svc.testSoraAccountConnection(c, account)

REDACTED
	body := rec.Body.String()
	require.Contains(t, body, "Subscription check blocked by Cloudflare challenge (HTTP 403)")
	require.Contains(t, body, "cf-ray: 9cff2d62d83bb98d")
	require.Contains(t, body, `"type":"test_complete","success":true`)
REDACTED
