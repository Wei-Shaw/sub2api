package repository

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/url"
	"strings"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"
)

type TurnstileServiceSuite struct {
	suite.Suite
	ctx      context.Context
	verifier *turnstileVerifier
	received chan url.Values
}

func (s *TurnstileServiceSuite) SetupTest() {
	s.ctx = context.Background()
	s.received = make(chan url.Values, 1)
	verifier, ok := NewTurnstileVerifier().(*turnstileVerifier)
	require.True(s.T(), ok, "type assertion failed")
	s.verifier = verifier
}

func (s *TurnstileServiceSuite) setupTransport(handler http.HandlerFunc) {
	s.verifier.verifyURL = "http://in-process/turnstile"
	s.verifier.httpClient = &http.Client{
		Transport: newInProcessTransport(handler, nil),
	}
}

func (s *TurnstileServiceSuite) TestVerifyToken_SendsFormAndDecodesJSON() {
	s.setupTransport(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Capture form data in main goroutine context later
		body, _ := io.ReadAll(r.Body)
		values, _ := url.ParseQuery(string(body))
		s.received <- values

		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(service.TurnstileVerifyResponse{Success: true})
	}))

	resp, err := s.verifier.VerifyToken(s.ctx, "sk", "token", "1.1.1.1")
	require.NoError(s.T(), err, "VerifyToken")
	require.NotNil(s.T(), resp)
	require.True(s.T(), resp.Success, "expected success response")

	// Assert form fields in main goroutine
	select {
	case values := <-s.received:
		require.Equal(s.T(), "sk", values.Get("secret"))
		require.Equal(s.T(), "token", values.Get("response"))
		require.Equal(s.T(), "1.1.1.1", values.Get("remoteip"))
	default:
		require.Fail(s.T(), "expected server to receive request")
	}
}

func (s *TurnstileServiceSuite) TestVerifyToken_ContentType() {
	var contentType string
	s.setupTransport(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		contentType = r.Header.Get("Content-Type")
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(service.TurnstileVerifyResponse{Success: true})
	}))

	_, err := s.verifier.VerifyToken(s.ctx, "sk", "token", "1.1.1.1")
	require.NoError(s.T(), err)
	require.True(s.T(), strings.HasPrefix(contentType, "application/x-www-form-urlencoded"), "unexpected content-type: %s", contentType)
}

func (s *TurnstileServiceSuite) TestVerifyToken_EmptyRemoteIP_NotSent() {
	s.setupTransport(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		values, _ := url.ParseQuery(string(body))
		s.received <- values

		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(service.TurnstileVerifyResponse{Success: true})
	}))

	_, err := s.verifier.VerifyToken(s.ctx, "sk", "token", "")
	require.NoError(s.T(), err)

	select {
	case values := <-s.received:
		require.Equal(s.T(), "", values.Get("remoteip"), "remoteip should be empty or not sent")
	default:
		require.Fail(s.T(), "expected server to receive request")
	}
}

func (s *TurnstileServiceSuite) TestVerifyToken_RequestError() {
	s.verifier.verifyURL = "http://in-process/turnstile"
	s.verifier.httpClient = &http.Client{
		Transport: roundTripFunc(func(*http.Request) (*http.Response, error) {
			return nil, errors.New("dial failed")
		}),
	}

	_, err := s.verifier.VerifyToken(s.ctx, "sk", "token", "1.1.1.1")
	require.Error(s.T(), err, "expected error when server is closed")
}

func (s *TurnstileServiceSuite) TestVerifyToken_InvalidJSON() {
	s.setupTransport(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, "not-valid-json")
	}))

	_, err := s.verifier.VerifyToken(s.ctx, "sk", "token", "1.1.1.1")
	require.Error(s.T(), err, "expected error for invalid JSON response")
}

func (s *TurnstileServiceSuite) TestVerifyToken_SuccessFalse() {
	s.setupTransport(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(service.TurnstileVerifyResponse{
			Success:    false,
			ErrorCodes: []string{"invalid-input-response"},
		})
	}))

	resp, err := s.verifier.VerifyToken(s.ctx, "sk", "token", "1.1.1.1")
	require.NoError(s.T(), err, "VerifyToken should not error on success=false")
	require.NotNil(s.T(), resp)
	require.False(s.T(), resp.Success)
	require.Contains(s.T(), resp.ErrorCodes, "invalid-input-response")
}

func TestTurnstileServiceSuite(t *testing.T) {
	suite.Run(t, new(TurnstileServiceSuite))
}
