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
}

func (s *PricingServiceSuite) SetupTest() {
	s.ctx = context.Background()
	client, ok := NewPricingRemoteClient().(*pricingRemoteClient)
	require.True(s.T(), ok, "type assertion failed")
	s.client = client
}

func (s *PricingServiceSuite) TearDownTest() {
	if s.srv != nil {
		s.srv.Close()
		s.srv = nil
	}
}

func (s *PricingServiceSuite) setupServer(handler http.HandlerFunc) {
	s.srv = httptest.NewServer(handler)
}

func (s *PricingServiceSuite) TestFetchPricingJSON_Success() {
	s.setupServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/ok" {
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{"ok":true}`))
			return
		}
		w.WriteHeader(http.StatusInternalServerError)
	}))

	body, err := s.client.FetchPricingJSON(s.ctx, s.srv.URL+"/ok")
	require.NoError(s.T(), err, "FetchPricingJSON")
	require.Equal(s.T(), `{"ok":true}`, string(body), "body mismatch")
}

func (s *PricingServiceSuite) TestFetchPricingJSON_NonOKStatus() {
	s.setupServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))

	_, err := s.client.FetchPricingJSON(s.ctx, s.srv.URL+"/err")
	require.Error(s.T(), err, "expected error for non-200 status")
}

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
		}
	}))

	hash, err := s.client.FetchHashText(s.ctx, s.srv.URL+"/hashfile")
	require.NoError(s.T(), err, "FetchHashText")
	require.Equal(s.T(), "abc123", hash, "hash mismatch")

	hash2, err := s.client.FetchHashText(s.ctx, s.srv.URL+"/hashonly")
	require.NoError(s.T(), err, "FetchHashText")
	require.Equal(s.T(), "def456", hash2, "hash mismatch")
}

func (s *PricingServiceSuite) TestFetchHashText_NonOKStatus() {
	s.setupServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))

	_, err := s.client.FetchHashText(s.ctx, s.srv.URL+"/nope")
	require.Error(s.T(), err, "expected error for non-200 status")
}

func (s *PricingServiceSuite) TestFetchPricingJSON_InvalidURL() {
	_, err := s.client.FetchPricingJSON(s.ctx, "://invalid-url")
	require.Error(s.T(), err, "expected error for invalid URL")
}

func (s *PricingServiceSuite) TestFetchHashText_EmptyBody() {
	s.setupServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		// empty body
	}))

	hash, err := s.client.FetchHashText(s.ctx, s.srv.URL+"/empty")
	require.NoError(s.T(), err, "FetchHashText empty body should not error")
	require.Equal(s.T(), "", hash, "expected empty hash")
}

func (s *PricingServiceSuite) TestFetchHashText_WhitespaceOnly() {
	s.setupServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("   \n"))
	}))

	hash, err := s.client.FetchHashText(s.ctx, s.srv.URL+"/ws")
	require.NoError(s.T(), err, "FetchHashText whitespace body should not error")
	require.Equal(s.T(), "", hash, "expected empty hash after trimming")
}

func (s *PricingServiceSuite) TestFetchPricingJSON_ContextCancel() {
	started := make(chan struct{})
	block := make(chan struct{})
	s.setupServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		close(started)
		<-block
	}))

	ctx, cancel := context.WithCancel(s.ctx)

	done := make(chan error, 1)
	go func() {
		_, err := s.client.FetchPricingJSON(ctx, s.srv.URL+"/block")
		done <- err
	}()

	<-started
	cancel()
	close(block)

	err := <-done
	require.Error(s.T(), err)
}

func TestPricingServiceSuite(t *testing.T) {
	suite.Run(t, new(PricingServiceSuite))
}
