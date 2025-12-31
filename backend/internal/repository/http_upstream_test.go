package repository

import (
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"
)

type HTTPUpstreamSuite struct {
	suite.Suite
	cfg *config.Config
REDACTED

func (s *HTTPUpstreamSuite) SetupTest() {
	s.cfg = &config.Config{REDACTED
REDACTED

func (s *HTTPUpstreamSuite) TestDefaultResponseHeaderTimeout() {
	up := NewHTTPUpstream(s.cfg)
	svc, ok := up.(*httpUpstreamService)
	require.True(s.T(), ok, "expected *httpUpstreamService")
	transport, ok := svc.defaultClient.Transport.(*http.Transport)
	require.True(s.T(), ok, "expected *http.Transport")
	require.Equal(s.T(), 300*time.Second, transport.ResponseHeaderTimeout, "ResponseHeaderTimeout mismatch")
REDACTED

func (s *HTTPUpstreamSuite) TestCustomResponseHeaderTimeout() {
	s.cfg.Gateway = config.GatewayConfig{ResponseHeaderTimeout: 7REDACTED
	up := NewHTTPUpstream(s.cfg)
	svc, ok := up.(*httpUpstreamService)
	require.True(s.T(), ok, "expected *httpUpstreamService")
	transport, ok := svc.defaultClient.Transport.(*http.Transport)
	require.True(s.T(), ok, "expected *http.Transport")
	require.Equal(s.T(), 7*time.Second, transport.ResponseHeaderTimeout, "ResponseHeaderTimeout mismatch")
REDACTED

func (s *HTTPUpstreamSuite) TestGetOrCreateClient_InvalidURLFallsBackToDefault() {
	s.cfg.Gateway = config.GatewayConfig{ResponseHeaderTimeout: 5REDACTED
	up := NewHTTPUpstream(s.cfg)
	svc, ok := up.(*httpUpstreamService)
	require.True(s.T(), ok, "expected *httpUpstreamService")

	got := svc.getOrCreateClient("://bad-proxy-url")
	require.Equal(s.T(), svc.defaultClient, got, "expected defaultClient fallback")
REDACTED

func (s *HTTPUpstreamSuite) TestDo_WithoutProxy_GoesDirect() {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.WriteString(w, "direct")
REDACTED))
	s.T().Cleanup(upstream.Close)

	up := NewHTTPUpstream(s.cfg)

	req, err := http.NewRequest(http.MethodGet, upstream.URL+"/x", nil)
	require.NoError(s.T(), err, "NewRequest")
	resp, err := up.Do(req, "")
	require.NoError(s.T(), err, "Do")
	defer func() { _ = resp.Body.Close() REDACTED()
	b, _ := io.ReadAll(resp.Body)
	require.Equal(s.T(), "direct", string(b), "unexpected body")
REDACTED

func (s *HTTPUpstreamSuite) TestDo_WithHTTPProxy_UsesProxy() {
	seen := make(chan string, 1)
	proxySrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		seen <- r.RequestURI
		_, _ = io.WriteString(w, "proxied")
REDACTED))
	s.T().Cleanup(proxySrv.Close)

	s.cfg.Gateway = config.GatewayConfig{ResponseHeaderTimeout: 1REDACTED
	up := NewHTTPUpstream(s.cfg)

	req, err := http.NewRequest(http.MethodGet, "http://example.com/test", nil)
	require.NoError(s.T(), err, "NewRequest")
	resp, err := up.Do(req, proxySrv.URL)
	require.NoError(s.T(), err, "Do")
	defer func() { _ = resp.Body.Close() REDACTED()
	b, _ := io.ReadAll(resp.Body)
	require.Equal(s.T(), "proxied", string(b), "unexpected body")

	select {
	case uri := <-seen:
		require.Equal(s.T(), "http://example.com/test", uri, "expected absolute-form request URI")
	default:
		require.Fail(s.T(), "expected proxy to receive request")
REDACTED
REDACTED

func (s *HTTPUpstreamSuite) TestDo_EmptyProxy_UsesDirect() {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.WriteString(w, "direct-empty")
REDACTED))
	s.T().Cleanup(upstream.Close)

	up := NewHTTPUpstream(s.cfg)
	req, err := http.NewRequest(http.MethodGet, upstream.URL+"/y", nil)
	require.NoError(s.T(), err, "NewRequest")
	resp, err := up.Do(req, "")
	require.NoError(s.T(), err, "Do with empty proxy")
	defer func() { _ = resp.Body.Close() REDACTED()
	b, _ := io.ReadAll(resp.Body)
	require.Equal(s.T(), "direct-empty", string(b))
REDACTED

func TestHTTPUpstreamSuite(t *testing.T) {
	suite.Run(t, new(HTTPUpstreamSuite))
REDACTED
