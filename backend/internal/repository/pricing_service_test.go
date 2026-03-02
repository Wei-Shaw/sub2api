package repository

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"
)

type PricingServiceSuite struct {
	suite.Suite
	ctx    context.Context
	srv    *httptest.Server
	client *pricingRemoteClient
REDACTED

func (s *PricingServiceSuite) SetupTest() {
	s.ctx = context.Background()
	client, ok := NewPricingRemoteClient("", false).(*pricingRemoteClient)
	require.True(s.T(), ok, "type assertion failed")
	s.client = client
REDACTED

func (s *PricingServiceSuite) TearDownTest() {
	if s.srv != nil {
		s.srv.Close()
		s.srv = nil
REDACTED
REDACTED

func (s *PricingServiceSuite) setupServer(handler http.HandlerFunc) {
	s.srv = newLocalTestServer(s.T(), handler)
REDACTED

func (s *PricingServiceSuite) TestFetchPricingJSON_Success() {
	s.setupServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/ok" {
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{"ok":trueREDACTED`))
			return
	REDACTED
		w.WriteHeader(http.StatusInternalServerError)
REDACTED))

	body, err := s.client.FetchPricingJSON(s.ctx, s.srv.URL+"/ok")
	require.NoError(s.T(), err, "FetchPricingJSON")
	require.Equal(s.T(), `{"ok":trueREDACTED`, string(body), "body mismatch")
REDACTED

func (s *PricingServiceSuite) TestFetchPricingJSON_NonOKStatus() {
	s.setupServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
REDACTED))

	_, err := s.client.FetchPricingJSON(s.ctx, s.srv.URL+"/err")
	require.Error(s.T(), err, "expected error for non-200 status")
REDACTED

func (s *PricingServiceSuite) TestFetchHashText_ParsesFields() {
	s.setupServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/hashfile":
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte("abc123  model_prices.json\n"))
		case "/hashonly":
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte("def456\n"))
		default:
			w.WriteHeader(http.StatusNotFound)
	REDACTED
REDACTED))

	hash, err := s.client.FetchHashText(s.ctx, s.srv.URL+"/hashfile")
	require.NoError(s.T(), err, "FetchHashText")
	require.Equal(s.T(), "abc123", hash, "hash mismatch")

	hash2, err := s.client.FetchHashText(s.ctx, s.srv.URL+"/hashonly")
	require.NoError(s.T(), err, "FetchHashText")
	require.Equal(s.T(), "def456", hash2, "hash mismatch")
REDACTED

func (s *PricingServiceSuite) TestFetchHashText_NonOKStatus() {
	s.setupServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
REDACTED))

	_, err := s.client.FetchHashText(s.ctx, s.srv.URL+"/nope")
	require.Error(s.T(), err, "expected error for non-200 status")
REDACTED

func (s *PricingServiceSuite) TestFetchPricingJSON_InvalidURL() {
	_, err := s.client.FetchPricingJSON(s.ctx, "://invalid-url")
	require.Error(s.T(), err, "expected error for invalid URL")
REDACTED

func (s *PricingServiceSuite) TestFetchHashText_EmptyBody() {
	s.setupServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		// empty body
REDACTED))

	hash, err := s.client.FetchHashText(s.ctx, s.srv.URL+"/empty")
	require.NoError(s.T(), err, "FetchHashText empty body should not error")
	require.Equal(s.T(), "", hash, "expected empty hash")
REDACTED

func (s *PricingServiceSuite) TestFetchHashText_WhitespaceOnly() {
	s.setupServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("   \n"))
REDACTED))

	hash, err := s.client.FetchHashText(s.ctx, s.srv.URL+"/ws")
	require.NoError(s.T(), err, "FetchHashText whitespace body should not error")
	require.Equal(s.T(), "", hash, "expected empty hash after trimming")
REDACTED

func (s *PricingServiceSuite) TestFetchPricingJSON_ContextCancel() {
	started := make(chan struct{REDACTED)
	s.setupServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		close(started)
		<-r.Context().Done()
REDACTED))

	ctx, cancel := context.WithCancel(s.ctx)

	done := make(chan error, 1)
	go func() {
		_, err := s.client.FetchPricingJSON(ctx, s.srv.URL+"/block")
		done <- err
REDACTED()

	<-started
	cancel()

	err := <-done
	require.Error(s.T(), err)
REDACTED

func TestNewPricingRemoteClient_InvalidProxy_NoFallback(t *testing.T) {
	client := NewPricingRemoteClient("://bad", false)
	_, ok := client.(*pricingRemoteClientError)
	require.True(t, ok, "should return error client when proxy is invalid and fallback disabled")

	_, err := client.FetchPricingJSON(context.Background(), "http://example.com")
REDACTED
	require.Contains(t, err.Error(), "proxy client init failed")
REDACTED

func TestNewPricingRemoteClient_InvalidProxy_WithFallback(t *testing.T) {
	client := NewPricingRemoteClient("://bad", true)
	_, ok := client.(*pricingRemoteClient)
	require.True(t, ok, "should fallback to direct client when allowed")
REDACTED

func TestPricingServiceSuite(t *testing.T) {
	suite.Run(t, new(PricingServiceSuite))
REDACTED
