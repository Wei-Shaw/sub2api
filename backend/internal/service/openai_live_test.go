package service

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/pkg/logger"
	"github.com/Wei-Shaw/sub2api/internal/pkg/tlsfingerprint"
	coderws "github.com/coder/websocket"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
	"go.uber.org/zap/zaptest/observer"
)

const liveTestSDPAnswer = "v=0\r\no=- 1 1 IN IP4 127.0.0.1\r\ns=-\r\nt=0 0\r\nm=audio 9 UDP/TLS/RTP/SAVPF 111\r\n"

type liveHTTPUpstreamStub struct {
	request  *http.Request
	body     []byte
	response *http.Response
}

type liveAttestationStub struct {
	header string
	err    error
}

func (s liveAttestationStub) Check(context.Context) error {
	return s.err
}

func (s liveAttestationStub) Generate(context.Context) (string, error) {
	return s.header, s.err
}

func (s *liveHTTPUpstreamStub) Do(
	request *http.Request,
	_ string,
	_ int64,
	_ int,
) (*http.Response, error) {
	s.request = request
	body, err := io.ReadAll(request.Body)
	if err != nil {
		return nil, err
	}
	s.body = body
	if s.response != nil {
		return s.response, nil
	}
	return &http.Response{
		StatusCode: http.StatusOK,
		Header: http.Header{
			"Location":     {"/backend-api/codex/call_test"},
			"Content-Type": {"application/sdp"},
		},
		Body: io.NopCloser(strings.NewReader(liveTestSDPAnswer)),
	}, nil
}

func (s *liveHTTPUpstreamStub) DoWithTLS(
	request *http.Request,
	proxyURL string,
	accountID int64,
	accountConcurrency int,
	_ *tlsfingerprint.Profile,
) (*http.Response, error) {
	return s.Do(request, proxyURL, accountID, accountConcurrency)
}

func TestLiveCapabilityOnlyAllowsOpenAIOAuth(t *testing.T) {
	require.True(t, (&Account{Platform: PlatformOpenAI, Type: AccountTypeOAuth}).SupportsOpenAIEndpointCapability(OpenAIEndpointCapabilityLive))
	require.False(t, (&Account{Platform: PlatformOpenAI, Type: AccountTypeAPIKey}).SupportsOpenAIEndpointCapability(OpenAIEndpointCapabilityLive))
	require.False(t, (&Account{Platform: PlatformGrok, Type: AccountTypeOAuth}).SupportsOpenAIEndpointCapability(OpenAIEndpointCapabilityLive))
	require.False(t, (&Account{
		Platform: PlatformOpenAI,
		Type:     AccountTypeOAuth,
		Credentials: map[string]any{
			openAIAuthModeCredentialKey: OpenAIAuthModePersonalAccessToken,
		},
	}).SupportsOpenAIEndpointCapability(OpenAIEndpointCapabilityLive))
	require.False(t, (&Account{
		Platform: PlatformOpenAI,
		Type:     AccountTypeOAuth,
		Credentials: map[string]any{
			openAIAuthModeCredentialKey: OpenAIAuthModeAgentIdentity,
		},
	}).SupportsOpenAIEndpointCapability(OpenAIEndpointCapabilityLive))
}

func TestValidateLiveCallRequestDoesNotRequireDelegation(t *testing.T) {
	request := &LiveCallRequest{
		SDP:     "v=0\r\n",
		Session: json.RawMessage(`{"model":"gpt-live-test","instructions":"hello"}`),
	}
	require.NoError(t, ValidateLiveCallRequest(request))
	require.NotContains(t, string(request.Session), "delegation")
}

func TestCreateUpstreamLiveCallPreservesSession(t *testing.T) {
	upstream := &liveHTTPUpstreamStub{}
	service := &OpenAIGatewayService{
		cfg:          &config.Config{},
		httpUpstream: upstream,
	}
	account := &Account{
		ID:          7,
		Platform:    PlatformOpenAI,
		Type:        AccountTypeOAuth,
		Concurrency: 2,
		Credentials: map[string]any{
			"access_token":       "test-access-token",
			"chatgpt_account_id": "acct_test",
		},
	}
	session := json.RawMessage(`{
		"model":"gpt-live-test",
		"delegation":{"type":"client"},
		"custom":{"keep":true}
	}`)

	created, err := service.createUpstreamLiveCall(context.Background(), account, &LiveCallRequest{
		SDP:     "v=offer\r\n",
		Session: session,
	}, &liveUpstreamAuth{mode: config.LiveUpstreamAuthModeDeviceCheck, attestation: `{"v":1,"s":0,"t":"v1.test"}`})
	require.NoError(t, err)
	require.Equal(t, "call_test", created.CallID)
	require.Equal(t, []byte(liveTestSDPAnswer), created.SDP)

	var forwarded struct {
		SDP     string          `json:"sdp"`
		Session json.RawMessage `json:"session"`
	}
	require.NoError(t, json.Unmarshal(upstream.body, &forwarded))
	require.Equal(t, "v=offer\r\n", forwarded.SDP)
	require.JSONEq(t, string(session), string(forwarded.Session))
	require.Equal(t, "Bearer test-access-token", upstream.request.Header.Get("Authorization"))
	require.Equal(t, "acct_test", upstream.request.Header.Get("Chatgpt-Account-Id"))
	require.Equal(t, "quicksilver=v2", upstream.request.Header.Get("OpenAI-Alpha"))
	require.Equal(t, `{"v":1,"s":0,"t":"v1.test"}`, upstream.request.Header.Get(liveAttestationHeader))
	require.NotEmpty(t, upstream.request.Header.Get("Session-Id"))
	require.NotEmpty(t, upstream.request.Header.Get("Thread-Id"))
	require.Empty(t, upstream.request.Header.Get("OpenAI-Beta"))
	require.Equal(t, HTTPUpstreamProfileOpenAI, HTTPUpstreamProfileFromContext(upstream.request.Context()))
	require.True(t, HTTPUpstreamRedirectsDisabled(upstream.request.Context()))
}

func liveOAuthTestAccount() *Account {
	return &Account{
		ID:       7,
		Platform: PlatformOpenAI,
		Type:     AccountTypeOAuth,
		Credentials: map[string]any{
			"access_token":       "test-access-token",
			"chatgpt_account_id": "acct_test",
		},
	}
}

func TestCodexOAuthLiveCreateAndSidebandShareIdentity(t *testing.T) {
	upstream := &liveHTTPUpstreamStub{}
	svc := &OpenAIGatewayService{
		cfg: &config.Config{Gateway: config.GatewayConfig{
			Live: config.GatewayLiveConfig{UpstreamAuthMode: config.LiveUpstreamAuthModeCodexOAuth},
		}},
		httpUpstream: upstream,
		// No DeviceCheck provider or cipher is available on this instance.
		liveAttestation: liveAttestationStub{err: errors.New("DeviceCheck is unavailable")},
	}
	auth, err := svc.prepareLiveUpstreamAuth(context.Background())
	require.NoError(t, err)
	require.Equal(t, config.LiveUpstreamAuthModeCodexOAuth, auth.mode)
	_, err = uuid.Parse(auth.sessionID)
	require.NoError(t, err)
	require.Empty(t, auth.attestation)
	require.Empty(t, auth.attestationCiphertext)

	account := liveOAuthTestAccount()
	session := json.RawMessage(`{"model":"gpt-live-1-codex","delegation":{"type":"client"},"custom":{"keep":true}}`)
	_, err = svc.createUpstreamLiveCall(context.Background(), account, &LiveCallRequest{
		SDP: "v=offer\r\n", Session: session,
	}, auth)
	require.NoError(t, err)
	require.Equal(t, chatGPTLiveCallsURL, upstream.request.URL.String())
	require.NotContains(t, upstream.request.Header, http.CanonicalHeaderKey(liveAttestationHeader))
	require.Equal(t, "Bearer test-access-token", upstream.request.Header.Get("Authorization"))
	require.Equal(t, "quicksilver=v2", upstream.request.Header.Get("OpenAI-Alpha"))
	var forwarded LiveCallRequest
	require.NoError(t, json.Unmarshal(upstream.body, &forwarded))
	require.JSONEq(t, string(session), string(forwarded.Session))

	// A global setting change must not change authentication for an existing call.
	svc.cfg.Gateway.Live.UpstreamAuthMode = config.LiveUpstreamAuthModeDeviceCheck
	record := &LiveCallRecord{UpstreamAuthMode: auth.mode, UpstreamSessionID: auth.sessionID}
	for range 2 {
		headers, err := svc.liveSidebandHeaders(context.Background(), account, record)
		require.NoError(t, err)
		require.NotContains(t, headers, http.CanonicalHeaderKey(liveAttestationHeader))
		for _, name := range []string{"Session-ID", "Thread-ID", "X-Session-ID"} {
			require.Equal(t, auth.sessionID, upstream.request.Header.Get(name))
			require.Equal(t, auth.sessionID, headers.Get(name))
		}
	}
}

func TestLiveAuthenticationDefaultsAndInvalidModesFailClosed(t *testing.T) {
	svc := &OpenAIGatewayService{}
	_, err := svc.prepareLiveUpstreamAuth(context.Background())
	var attestationErr *LiveAttestationUnavailableError
	require.ErrorAs(t, err, &attestationErr)

	svc.cfg = &config.Config{Gateway: config.GatewayConfig{
		Live: config.GatewayLiveConfig{UpstreamAuthMode: "unsupported-mode"},
	}}
	_, err = svc.prepareLiveUpstreamAuth(context.Background())
	require.ErrorContains(t, err, "upstream_auth_mode")

	svc.cfg.Gateway.Live.UpstreamAuthMode = config.LiveUpstreamAuthModeCodexOAuth
	// Old records keep requiring DeviceCheck even after opting in globally.
	_, err = svc.liveSidebandHeaders(context.Background(), liveOAuthTestAccount(), &LiveCallRecord{})
	require.ErrorAs(t, err, &attestationErr)
	_, err = svc.liveSidebandHeaders(context.Background(), nil, &LiveCallRecord{UpstreamAuthMode: "unsupported-mode"})
	require.ErrorContains(t, err, "upstream_auth_mode")
	_, err = svc.liveSidebandHeaders(context.Background(), liveOAuthTestAccount(), &LiveCallRecord{
		UpstreamAuthMode: config.LiveUpstreamAuthModeCodexOAuth,
	})
	require.ErrorContains(t, err, "session identity")
}

func TestCreateUpstreamLiveCallRejectsNonSDPSuccess(t *testing.T) {
	for _, tc := range []struct {
		name        string
		contentType string
		body        string
		location    string
	}{
		{name: "HTML SPA", contentType: "text/html", body: "<!doctype html><html></html>", location: "/backend-api/codex/call_test"},
		{name: "HTML labeled SDP", contentType: "application/sdp", body: "<!doctype html><html></html>", location: "/backend-api/codex/call_test"},
		{name: "HTML labeled plain text", contentType: "text/plain; charset=utf-8", body: "<!doctype html><html></html>", location: "/backend-api/codex/call_test"},
		{name: "JSON labeled plain text", contentType: "text/plain", body: `{"error":"upstream failed"}`, location: "/backend-api/codex/call_test"},
		{name: "incomplete SDP", contentType: "application/sdp", body: "v=0\r\n", location: "/backend-api/codex/call_test"},
		{name: "missing location", contentType: "application/sdp", body: liveTestSDPAnswer},
	} {
		t.Run(tc.name, func(t *testing.T) {
			upstream := &liveHTTPUpstreamStub{response: &http.Response{
				StatusCode: http.StatusOK,
				Header: http.Header{
					"Content-Type": {tc.contentType},
					"Location":     {tc.location},
				},
				Body: io.NopCloser(strings.NewReader(tc.body)),
			}}
			svc := &OpenAIGatewayService{cfg: &config.Config{}, httpUpstream: upstream}
			created, err := svc.createUpstreamLiveCall(context.Background(), liveOAuthTestAccount(), &LiveCallRequest{
				SDP: "v=offer\r\n", Session: json.RawMessage(`{"model":"gpt-live-1-codex"}`),
			}, &liveUpstreamAuth{mode: config.LiveUpstreamAuthModeCodexOAuth, sessionID: uuid.NewString()})
			require.Error(t, err)
			require.Nil(t, created)
		})
	}
}

func TestCreateUpstreamLiveCallAcceptsSDPWithNativeContentTypes(t *testing.T) {
	for _, contentType := range []string{"application/sdp", "text/plain", "text/plain; charset=utf-8"} {
		t.Run(contentType, func(t *testing.T) {
			upstream := &liveHTTPUpstreamStub{response: &http.Response{
				StatusCode: http.StatusCreated,
				Header: http.Header{
					"Content-Type": {contentType},
					"Location":     {"/backend-api/codex/realtime/calls/call_test"},
				},
				Body: io.NopCloser(strings.NewReader(liveTestSDPAnswer)),
			}}
			svc := &OpenAIGatewayService{cfg: &config.Config{}, httpUpstream: upstream}
			created, err := svc.createUpstreamLiveCall(context.Background(), liveOAuthTestAccount(), &LiveCallRequest{
				SDP: "v=offer\r\n", Session: json.RawMessage(`{"model":"gpt-live-1-codex"}`),
			}, &liveUpstreamAuth{mode: config.LiveUpstreamAuthModeCodexOAuth, sessionID: uuid.NewString()})
			require.NoError(t, err)
			require.Equal(t, "call_test", created.CallID)
			require.Equal(t, liveTestSDPAnswer, string(created.SDP))
		})
	}
}

func TestLiveDiagnosticLogsOmitSensitiveValues(t *testing.T) {
	core, logs := observer.New(zap.WarnLevel)
	ctx := logger.IntoContext(context.Background(), zap.New(core))
	record := &LiveCallRecord{
		AccountID:         7,
		CallID:            "private-call-id",
		UpstreamAuthMode:  "private-invalid-mode",
		UpstreamSessionID: "private-session-id",
	}
	logLiveSidebandFailure(ctx, record, "upstream_dial", http.StatusForbidden, coderws.CloseError{
		Code: coderws.StatusPolicyViolation, Reason: "private-token in websocket close reason",
	})
	logLiveCreateResponseFailure(ctx, record.AccountID, "response_location", errors.New("private-token"), &http.Response{
		StatusCode: http.StatusCreated,
		Header: http.Header{
			"Content-Type":  {"text/plain; charset=utf-8"},
			"Location":      {"https://chatgpt.com/private-call-id?token=private-token"},
			"Authorization": {"Bearer private-token"},
		},
	}, []byte("v=0\r\na=private-sdp-value\r\n"))
	entries := logs.All()
	require.Len(t, entries, 2)
	require.Equal(t, int64(http.StatusForbidden), entries[0].ContextMap()["upstream_status_code"])
	require.Equal(t, int64(coderws.StatusPolicyViolation), entries[0].ContextMap()["websocket_close_status"])
	require.Equal(t, "unknown", entries[0].ContextMap()["upstream_auth_mode"])
	require.Equal(t, "text/plain", entries[1].ContextMap()["upstream_media_type"])
	for _, entry := range entries {
		encoded, err := json.Marshal(entry.ContextMap())
		require.NoError(t, err)
		require.NotContains(t, string(encoded), "private-")
	}
}

func TestLiveExpectedSidebandEndLogsAtDebug(t *testing.T) {
	core, logs := observer.New(zap.DebugLevel)
	ctx := logger.IntoContext(context.Background(), zap.New(core))
	for _, err := range []error{
		context.Canceled,
		ErrLiveCallNotFound,
		ErrLiveControllerChanged,
		coderws.CloseError{Code: coderws.StatusNormalClosure},
		coderws.CloseError{Code: coderws.StatusGoingAway},
	} {
		logLiveSidebandFailure(ctx, &LiveCallRecord{}, "proxy_end", 0, err)
	}
	logLiveSidebandFailure(ctx, &LiveCallRecord{ExpiresAt: time.Now().Add(-time.Second)}, "proxy_end", 0, context.DeadlineExceeded)
	require.Len(t, logs.All(), 6)
	for _, entry := range logs.All() {
		require.Equal(t, zap.DebugLevel, entry.Level)
	}
}

func TestCreateUpstreamLiveCallRejectsAPIKeyAccounts(t *testing.T) {
	upstream := &liveHTTPUpstreamStub{}
	svc := &OpenAIGatewayService{cfg: &config.Config{}, httpUpstream: upstream}
	account := liveOAuthTestAccount()
	account.Type = AccountTypeAPIKey
	_, err := svc.createUpstreamLiveCall(context.Background(), account, &LiveCallRequest{
		SDP: "v=offer\r\n", Session: json.RawMessage(`{"model":"gpt-live-1-codex"}`),
	}, &liveUpstreamAuth{mode: config.LiveUpstreamAuthModeCodexOAuth, sessionID: uuid.NewString()})
	require.ErrorIs(t, err, ErrLiveUnavailable)
	require.Nil(t, upstream.request)
}

func TestLiveAttestationCipherRoundTripAndRejectsOtherInstanceKey(t *testing.T) {
	first := newLiveAttestationCipher(&config.Config{
		JWT: config.JWTConfig{Secret: "first-live-secret"},
	})
	second := newLiveAttestationCipher(&config.Config{
		JWT: config.JWTConfig{Secret: "second-live-secret"},
	})
	require.NotNil(t, first)
	require.NotNil(t, second)

	ciphertext, err := first.Encrypt(`{"v":1,"s":0,"t":"v1.opaque"}`)
	require.NoError(t, err)
	require.NotContains(t, ciphertext, "opaque")

	plaintext, err := first.Decrypt(ciphertext)
	require.NoError(t, err)
	require.Equal(t, `{"v":1,"s":0,"t":"v1.opaque"}`, plaintext)

	_, err = second.Decrypt(ciphertext)
	require.Error(t, err)
}

func TestPrepareLiveAttestationEncryptsHeaderAndReturnsExplicitProviderError(t *testing.T) {
	cipher := newLiveAttestationCipher(&config.Config{
		JWT: config.JWTConfig{Secret: "live-attestation-test-secret"},
	})
	service := &OpenAIGatewayService{
		liveAttestation:       liveAttestationStub{header: `{"v":1,"s":0,"t":"v1.test"}`},
		liveAttestationCipher: cipher,
	}
	header, ciphertext, err := service.prepareLiveAttestation(context.Background())
	require.NoError(t, err)
	require.Equal(t, `{"v":1,"s":0,"t":"v1.test"}`, header)
	require.NotContains(t, ciphertext, "v1.test")
	decrypted, err := cipher.Decrypt(ciphertext)
	require.NoError(t, err)
	require.Equal(t, header, decrypted)

	service.liveAttestation = liveAttestationStub{err: errors.New("macOS app missing")}
	_, _, err = service.prepareLiveAttestation(context.Background())
	var unavailable *LiveAttestationUnavailableError
	require.ErrorAs(t, err, &unavailable)
	require.Contains(t, unavailable.Error(), "macOS app missing")
}

func TestLiveMaxSessionDurationDefaultsAndOverrides(t *testing.T) {
	require.Equal(t, defaultLiveMaxSessionDuration, (&OpenAIGatewayService{}).liveMaxSessionDuration())
	require.Equal(
		t,
		90*time.Second,
		(&OpenAIGatewayService{cfg: &config.Config{
			Gateway: config.GatewayConfig{
				Live: config.GatewayLiveConfig{MaxSessionDurationSeconds: 90},
			},
		}}).liveMaxSessionDuration(),
	)
}

func TestLiveSidebandNormalCloseEndsCall(t *testing.T) {
	normalClose := coderws.CloseError{Code: coderws.StatusNormalClosure}
	require.ErrorIs(t, liveSidebandReadError(normalClose), ErrLiveCallNotFound)

	abnormalClose := coderws.CloseError{Code: coderws.StatusInternalError}
	require.Equal(t, abnormalClose, liveSidebandReadError(abnormalClose))
}

func TestLiveCreateFailoverUsesExistingOpenAIPolicy(t *testing.T) {
	service := &OpenAIGatewayService{}
	account := newOpenAIUpstreamErrorTestAccount()
	require.False(t, service.shouldFailoverLiveCreateError(account, &UpstreamFailoverError{
		StatusCode:   http.StatusBadRequest,
		ResponseBody: []byte(`{"error":{"message":"invalid session"}}`),
	}))
	require.True(t, service.shouldFailoverLiveCreateError(account, &UpstreamFailoverError{
		StatusCode: http.StatusForbidden,
	}))
	require.True(t, service.shouldFailoverLiveCreateError(account, &UpstreamFailoverError{
		StatusCode: http.StatusBadGateway,
	}))
	require.True(t, service.shouldFailoverLiveCreateError(account, errors.New("transport failed")))
}

func TestLiveCallIDFromLocation(t *testing.T) {
	callID, err := liveCallIDFromLocation("https://chatgpt.com/backend-api/codex/call_123?intent=quicksilver")
	require.NoError(t, err)
	require.Equal(t, "call_123", callID)

	callID, err = liveCallIDFromLocation("/backend-api/codex/call_456")
	require.NoError(t, err)
	require.Equal(t, "call_456", callID)
}

func TestRequestTypeLive(t *testing.T) {
	require.True(t, RequestTypeLive.IsValid())
	require.Equal(t, "live", RequestTypeLive.String())
	parsed, err := ParseUsageRequestType("live")
	require.NoError(t, err)
	require.Equal(t, RequestTypeLive, parsed)
}
