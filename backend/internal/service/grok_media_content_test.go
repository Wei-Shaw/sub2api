//go:build unit

package service

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/pkg/tlsfingerprint"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

type grokMediaContentUpstreamStub struct {
	request  *http.Request
	response *http.Response
REDACTED

func (s *grokMediaContentUpstreamStub) Do(req *http.Request, _ string, _ int64, _ int) (*http.Response, error) {
	s.request = req
	return s.response, nil
REDACTED

func (s *grokMediaContentUpstreamStub) DoWithTLS(req *http.Request, proxyURL string, accountID int64, accountConcurrency int, _ *tlsfingerprint.Profile) (*http.Response, error) {
	return s.Do(req, proxyURL, accountID, accountConcurrency)
REDACTED

func grokMediaContentTestAccount() *Account {
REDACTED
		ID:       9,
		Platform: PlatformGrok,
		Type:     AccountTypeAPIKey,
REDACTED
			"api_key":  "upstream-key",
			"base_url": "https://relay.example/v1",
	REDACTED,
REDACTED
REDACTED

func grokMediaContentTestContext(method, target string, headers map[string]string) (*gin.Context, *httptest.ResponseRecorder) {
	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(method, target, nil)
	for name, value := range headers {
		c.Request.Header.Set(name, value)
REDACTED
	return c, recorder
REDACTED

func TestForwardGrokMediaContentUsesUpstreamCredentialAndStreamsRange(t *testing.T) {
	upstream := &grokMediaContentUpstreamStub{
		response: &http.Response{
			StatusCode: http.StatusPartialContent,
			Header: http.Header{
				"Content-Type":   []string{"video/mp4"REDACTED,
				"Content-Length": []string{"13"REDACTED,
				"Content-Range":  []string{"bytes 0-12/100"REDACTED,
				"Accept-Ranges":  []string{"bytes"REDACTED,
				"Content-Disposition": []string{
					`attachment; filename="task-1.mp4"`,
			REDACTED,
		REDACTED,
			Body: io.NopCloser(strings.NewReader("video-payload")),
	REDACTED,
REDACTED
	svc := &OpenAIGatewayService{cfg: &config.Config{REDACTED, httpUpstream: upstreamREDACTED
	c, recorder := grokMediaContentTestContext(http.MethodGet, "https://api.example/v1/videos/task-1/content", map[string]string{
		"Range": "bytes=0-12",
REDACTED)

	result, err := svc.ForwardGrokMedia(
		context.Background(), c, grokMediaContentTestAccount(),
		GrokMediaEndpointVideoContent, "task-1", nil, "",
	)

REDACTED
	require.NotNil(t, result)
	require.Equal(t, http.StatusPartialContent, recorder.Code)
	require.Equal(t, "video-payload", recorder.Body.String())
	require.Equal(t, "https://relay.example/v1/videos/task-1/content", upstream.request.URL.String())
	require.Equal(t, "Bearer upstream-key", upstream.request.Header.Get("Authorization"))
	require.Equal(t, "bytes=0-12", upstream.request.Header.Get("Range"))
	require.Equal(t, "*/*", upstream.request.Header.Get("Accept"))
	require.Equal(t, "video/mp4", recorder.Header().Get("Content-Type"))
	require.Equal(t, "13", recorder.Header().Get("Content-Length"))
	require.Equal(t, "bytes 0-12/100", recorder.Header().Get("Content-Range"))
	require.Equal(t, "bytes", recorder.Header().Get("Accept-Ranges"))
	require.Equal(t, `attachment; filename="task-1.mp4"`, recorder.Header().Get("Content-Disposition"))
	require.True(t, IsResponseCommitted(c))
REDACTED

func TestForwardGrokMediaContentStreamsFullResponseWithSafeDefaults(t *testing.T) {
	upstream := &grokMediaContentUpstreamStub{
		response: &http.Response{
			StatusCode:    http.StatusOK,
			Header:        http.Header{"Set-Cookie": []string{"secret=upstream"REDACTED, "X-Upstream-Secret": []string{"hidden"REDACTEDREDACTED,
			Body:          io.NopCloser(strings.NewReader("full-video")),
			ContentLength: -1,
	REDACTED,
REDACTED
	svc := &OpenAIGatewayService{cfg: &config.Config{REDACTED, httpUpstream: upstreamREDACTED
	c, recorder := grokMediaContentTestContext(http.MethodGet, "https://api.example/v1/videos/task-1/content", nil)

	_, err := svc.ForwardGrokMedia(
		context.Background(), c, grokMediaContentTestAccount(),
		GrokMediaEndpointVideoContent, "task-1", nil, "",
	)

REDACTED
	require.Equal(t, http.StatusOK, recorder.Code)
	require.Equal(t, "full-video", recorder.Body.String())
	require.Empty(t, upstream.request.Header.Get("Range"))
	require.Equal(t, "application/octet-stream", recorder.Header().Get("Content-Type"))
	require.Empty(t, recorder.Header().Get("Content-Length"))
	require.Empty(t, recorder.Header().Get("Set-Cookie"))
	require.Empty(t, recorder.Header().Get("X-Upstream-Secret"))
	require.True(t, IsResponseCommitted(c))
REDACTED

func TestForwardGrokMediaContentPreservesRangeNotSatisfiable(t *testing.T) {
	upstream := &grokMediaContentUpstreamStub{
		response: &http.Response{
			StatusCode: http.StatusRequestedRangeNotSatisfiable,
			Header: http.Header{
				"Content-Type":   []string{"text/plain"REDACTED,
				"Content-Length": []string{"11"REDACTED,
				"Content-Range":  []string{"bytes */100"REDACTED,
				"Accept-Ranges":  []string{"bytes"REDACTED,
		REDACTED,
			Body: io.NopCloser(strings.NewReader("bad-range!!")),
	REDACTED,
REDACTED
	svc := &OpenAIGatewayService{cfg: &config.Config{REDACTED, httpUpstream: upstreamREDACTED
	c, recorder := grokMediaContentTestContext(http.MethodGet, "https://api.example/v1/videos/task-1/content", map[string]string{
		"Range": "bytes=500-600",
REDACTED)

	_, err := svc.ForwardGrokMedia(
		context.Background(), c, grokMediaContentTestAccount(),
		GrokMediaEndpointVideoContent, "task-1", nil, "",
	)

REDACTED
	require.Equal(t, http.StatusRequestedRangeNotSatisfiable, recorder.Code)
	require.Equal(t, "bad-range!!", recorder.Body.String())
	require.Equal(t, "bytes=500-600", upstream.request.Header.Get("Range"))
	require.Equal(t, "bytes */100", recorder.Header().Get("Content-Range"))
	require.Equal(t, "bytes", recorder.Header().Get("Accept-Ranges"))
	require.True(t, IsResponseCommitted(c))
REDACTED

func TestForwardGrokVideoStatusRewritesOnlyProtectedContentURL(t *testing.T) {
	statusBody := `{"id":"task-1","status":"completed","url":"https://relay.example/v1/videos/task-1/content","download_url":"/v1/videos/task-1/content","video_url":"https://vidgen.x.ai/task-1.mp4","counter":9007199254740993REDACTED`
	upstream := &grokMediaContentUpstreamStub{
		response: &http.Response{
			StatusCode: http.StatusOK,
			Header:     http.Header{"Content-Type": []string{"application/json"REDACTEDREDACTED,
			Body:       io.NopCloser(strings.NewReader(statusBody)),
	REDACTED,
REDACTED
	svc := &OpenAIGatewayService{cfg: &config.Config{REDACTED, httpUpstream: upstreamREDACTED
	c, recorder := grokMediaContentTestContext(http.MethodGet, "https://api.example/v1/videos/task-1", map[string]string{
		"X-Forwarded-Host":  "malicious.invalid",
		"X-Forwarded-Proto": "https",
REDACTED)

	_, err := svc.ForwardGrokMedia(
		context.Background(), c, grokMediaContentTestAccount(),
		GrokMediaEndpointVideoStatus, "task-1", nil, "",
	)

REDACTED
	require.Equal(t, http.StatusOK, recorder.Code)
	require.Equal(t, "/v1/videos/task-1/content", gjson.Get(recorder.Body.String(), "url").String())
	require.Equal(t, "/v1/videos/task-1/content", gjson.Get(recorder.Body.String(), "download_url").String())
	require.Equal(t, "https://vidgen.x.ai/task-1.mp4", gjson.Get(recorder.Body.String(), "video_url").String())
	require.Equal(t, "9007199254740993", gjson.Get(recorder.Body.String(), "counter").String())
	require.NotContains(t, recorder.Body.String(), "malicious.invalid")
REDACTED

func TestRewriteGrokMediaVideoContentURLsPreservesOtherIDsAndHandlesNestedEscapedID(t *testing.T) {
	body := []byte(`{"nested":[{"url":"https://relay.example/v1/videos/task%2Fone/content"REDACTED,{"url":"https://relay.example/v1/videos/task-two/content"REDACTED]REDACTED`)

	rewritten := rewriteGrokMediaVideoContentURLs(body, "task/one", "/v1/videos/task%2Fone/content")

	require.Equal(t, "/v1/videos/task%2Fone/content", gjson.GetBytes(rewritten, "nested.0.url").String())
	require.Equal(t, "https://relay.example/v1/videos/task-two/content", gjson.GetBytes(rewritten, "nested.1.url").String())
REDACTED

func TestRewriteGrokMediaVideoContentURLsRewritesSignedVideoURL(t *testing.T) {
	body := []byte(`{"status":"done","video":{"url":"https://vidgen.x.ai/signed-token/xai-video-request-1.mp4","duration":8REDACTEDREDACTED`)

	rewritten := rewriteGrokMediaVideoContentURLs(body, "request-1", "/v1/videos/request-1/content")

	require.Equal(t, "/v1/videos/request-1/content", gjson.GetBytes(rewritten, "video.url").String())
	require.Equal(t, "8", gjson.GetBytes(rewritten, "video.duration").String())
	require.Equal(t, "done", gjson.GetBytes(rewritten, "status").String())
REDACTED
