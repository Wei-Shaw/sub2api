//go:build unit

package service

import (
	"bytes"
	"context"
	"encoding/binary"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/domain"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

func TestAccountTestService_KiroUsesKiroUpstreamInsteadOfAnthropic(t *testing.T) {
	gin.SetMode(gin.TestMode)
	ctx, _ := newTestContext()

	account := &Account{
		ID:          1,
		Name:        "kiro-test",
		Platform:    PlatformKiro,
		Type:        AccountTypeOAuth,
		Concurrency: 1,
		Credentials: map[string]any{
			"access_token": "kiro-access-token",
			"profile_arn":  "arn:aws:codewhisperer:us-east-1:123456789012:profile/TESTSOCIAL",
		},
	}
	repo := &mockAccountRepoForGemini{accountsByID: map[int64]*Account{1: account}}
	upstream := &queuedHTTPUpstream{
		responses: []*http.Response{
			newJSONResponse(http.StatusUnauthorized, `{"type":"error","error":{"type":"authentication_error","message":"Invalid bearer token"}}`),
		},
	}
	svc := &AccountTestService{
		accountRepo:         repo,
		kiroTokenProvider:   NewKiroTokenProvider(nil, nil, nil),
		httpUpstream:        upstream,
		tlsFPProfileService: &TLSFingerprintProfileService{},
	}

	err := svc.TestAccountConnection(ctx, account.ID, "gpt-4o", "", AccountTestModeDefault)
	require.Error(t, err)
	require.Len(t, upstream.requests, 1)

	req := upstream.requests[0]
	require.Equal(t, "q.us-east-1.amazonaws.com", req.URL.Host)
	require.Equal(t, "/generateAssistantResponse", req.URL.Path)
	require.Equal(t, "Bearer kiro-access-token", req.Header.Get("Authorization"))
	require.Equal(t, "vibe", req.Header.Get("x-amzn-kiro-agent-mode"))
	require.Empty(t, req.Header.Get("anthropic-version"))
	require.NotContains(t, req.URL.Host, "api.anthropic.com")
}

func TestAccountTestService_Kiro429DoesNotFallbackToCodeWhispererEndpoint(t *testing.T) {
	gin.SetMode(gin.TestMode)
	ctx, _ := newTestContext()

	account := &Account{
		ID:          2,
		Name:        "kiro-fallback",
		Platform:    PlatformKiro,
		Type:        AccountTypeOAuth,
		Concurrency: 1,
		Credentials: map[string]any{
			"access_token": "kiro-access-token",
			"api_region":   "us-west-2",
			"region":       "us-west-2",
			"profile_arn":  "arn:aws:codewhisperer:us-west-2:123456789012:profile/TESTFALLBACK",
		},
	}
	repo := &mockAccountRepoForGemini{accountsByID: map[int64]*Account{2: account}}
	upstream := &queuedHTTPUpstream{
		responses: []*http.Response{
			newJSONResponse(http.StatusTooManyRequests, `{"message":"slow down"}`),
		},
	}
	svc := &AccountTestService{
		accountRepo:         repo,
		kiroTokenProvider:   NewKiroTokenProvider(nil, nil, nil),
		httpUpstream:        upstream,
		tlsFPProfileService: &TLSFingerprintProfileService{},
	}

	err := svc.TestAccountConnection(ctx, account.ID, "claude-sonnet-4-6", "", AccountTestModeDefault)
	require.Error(t, err)
	require.Len(t, upstream.requests, 1)

	require.Equal(t, "q.us-west-2.amazonaws.com", upstream.requests[0].URL.Host)
	require.Empty(t, upstream.requests[0].Header.Get("X-Amz-Target"))
	require.Contains(t, err.Error(), "API returned 429")
}

func TestAccountTestService_KiroIDCWithoutProfileArnOmitsProfileArnAndUsesDefaultRuntimeRegion(t *testing.T) {
	gin.SetMode(gin.TestMode)
	ctx, _ := newTestContext()

	account := &Account{
		ID:          5,
		Name:        "kiro-idc-default-region",
		Platform:    PlatformKiro,
		Type:        AccountTypeOAuth,
		Concurrency: 1,
		Credentials: map[string]any{
			"access_token": "kiro-access-token",
			"auth_method":  "idc",
			"provider":     "AWS",
			"region":       "ap-northeast-2",
			"start_url":    "https://d-example.awsapps.com/start",
		},
	}
	repo := &mockAccountRepoForGemini{accountsByID: map[int64]*Account{5: account}}
	upstream := &queuedHTTPUpstream{
		responses: []*http.Response{
			newJSONResponse(http.StatusUnauthorized, `{"type":"error","error":{"message":"Invalid bearer token"}}`),
		},
	}
	svc := &AccountTestService{
		accountRepo:         repo,
		kiroTokenProvider:   NewKiroTokenProvider(nil, nil, nil),
		httpUpstream:        upstream,
		tlsFPProfileService: &TLSFingerprintProfileService{},
	}

	err := svc.TestAccountConnection(ctx, account.ID, "claude-sonnet-4-6", "", AccountTestModeDefault)
	require.Error(t, err)
	require.Len(t, upstream.requests, 1)
	require.Equal(t, "q.us-east-1.amazonaws.com", upstream.requests[0].URL.Host)
	body, readErr := io.ReadAll(upstream.requests[0].Body)
	require.NoError(t, readErr)
	require.NotContains(t, string(body), `"profileArn":`)
}

func TestAccountTestService_KiroInvalidModelErrorPassthrough(t *testing.T) {
	gin.SetMode(gin.TestMode)
	ctx, _ := newTestContext()

	account := &Account{
		ID:          6,
		Name:        "kiro-invalid-model",
		Platform:    PlatformKiro,
		Type:        AccountTypeOAuth,
		Concurrency: 1,
		Credentials: map[string]any{
			"access_token": "kiro-access-token",
			"profile_arn":  "arn:aws:codewhisperer:us-west-2:123456789012:profile/TESTINVALIDMODEL",
		},
	}
	repo := &mockAccountRepoForGemini{accountsByID: map[int64]*Account{6: account}}
	upstream := &queuedHTTPUpstream{
		responses: []*http.Response{
			newJSONResponse(http.StatusBadRequest, `{"message":"Invalid model ID. Please select a different model to continue.","reason":"INVALID_MODEL_ID"}`),
		},
	}
	svc := &AccountTestService{
		accountRepo:         repo,
		kiroTokenProvider:   NewKiroTokenProvider(nil, nil, nil),
		httpUpstream:        upstream,
		tlsFPProfileService: &TLSFingerprintProfileService{},
	}

	err := svc.TestAccountConnection(ctx, account.ID, "claude-opus-4-6", "", AccountTestModeDefault)
	require.Error(t, err)
	require.Equal(t, `API returned 400: {"message":"Invalid model ID. Please select a different model to continue.","reason":"INVALID_MODEL_ID"}`, err.Error())
}

func TestAccountTestService_KiroInvalidModelDoesNotRefreshProfileArnOrRetry(t *testing.T) {
	gin.SetMode(gin.TestMode)
	ctx, _ := newTestContext()

	account := &Account{
		ID:          7,
		Name:        "kiro-invalid-model-refresh",
		Platform:    PlatformKiro,
		Type:        AccountTypeOAuth,
		Concurrency: 1,
		Credentials: map[string]any{
			"access_token": "kiro-access-token",
			"profile_arn":  "arn:aws:codewhisperer:us-east-1:123456789012:profile/STALE",
		},
	}
	repo := &mockAccountRepoForGemini{accountsByID: map[int64]*Account{7: account}}
	upstream := &queuedHTTPUpstream{
		responses: []*http.Response{
			newJSONResponse(http.StatusBadRequest, `{"message":"Invalid model ID. Please select a different model to continue.","reason":"INVALID_MODEL_ID"}`),
		},
	}
	svc := &AccountTestService{
		accountRepo:         repo,
		kiroTokenProvider:   NewKiroTokenProvider(nil, nil, nil),
		httpUpstream:        upstream,
		tlsFPProfileService: &TLSFingerprintProfileService{},
	}

	err := svc.TestAccountConnection(ctx, account.ID, "claude-opus-4-6", "", AccountTestModeDefault)
	require.Error(t, err)
	require.Contains(t, err.Error(), "API returned 400")
	require.Len(t, upstream.requests, 1)

	firstBody, readErr := io.ReadAll(upstream.requests[0].Body)
	require.NoError(t, readErr)
	require.Contains(t, string(firstBody), `"profileArn":"arn:aws:codewhisperer:us-east-1:123456789012:profile/STALE"`)
	require.Equal(t, "arn:aws:codewhisperer:us-east-1:123456789012:profile/STALE", account.GetCredential("profile_arn"))
}

func TestAccountTestService_KiroPreferredEndpointIsIgnored(t *testing.T) {
	gin.SetMode(gin.TestMode)
	ctx, _ := newTestContext()

	account := &Account{
		ID:          6,
		Name:        "kiro-preferred-endpoint",
		Platform:    PlatformKiro,
		Type:        AccountTypeOAuth,
		Concurrency: 1,
		Credentials: map[string]any{
			"access_token":       "kiro-access-token",
			"api_region":         "us-west-2",
			"profile_arn":        "arn:aws:codewhisperer:us-west-2:123456789012:profile/PREFERRED",
			"preferred_endpoint": "codewhisperer",
		},
	}
	repo := &mockAccountRepoForGemini{accountsByID: map[int64]*Account{6: account}}
	upstream := &queuedHTTPUpstream{
		responses: []*http.Response{
			newJSONResponse(http.StatusUnauthorized, `{"type":"error","error":{"message":"Invalid bearer token"}}`),
		},
	}
	svc := &AccountTestService{
		accountRepo:         repo,
		kiroTokenProvider:   NewKiroTokenProvider(nil, nil, nil),
		httpUpstream:        upstream,
		tlsFPProfileService: &TLSFingerprintProfileService{},
	}

	err := svc.TestAccountConnection(ctx, account.ID, "claude-sonnet-4-6", "", AccountTestModeDefault)
	require.Error(t, err)
	require.Len(t, upstream.requests, 1)
	require.Equal(t, "q.us-west-2.amazonaws.com", upstream.requests[0].URL.Host)
	require.Empty(t, upstream.requests[0].Header.Get("X-Amz-Target"))
}

func TestBuildKiroPayloadForAccount_KiroBuilderIDWithoutProfileArnOmitsProfileArn(t *testing.T) {
	account := &Account{
		ID:       3,
		Name:     "kiro-builder-id",
		Platform: PlatformKiro,
		Type:     AccountTypeOAuth,
		Credentials: map[string]any{
			"auth_method": "idc",
			"provider":    "BuilderId",
			"region":      "us-east-1",
			"client_id":   "builder-client-id",
		},
	}

	testPayload, err := createTestPayload("claude-sonnet-4-6")
	require.NoError(t, err)
	payloadBytes, err := json.Marshal(testPayload)
	require.NoError(t, err)

	buildResult, err := (&GatewayService{}).buildKiroPayloadForAccount(context.Background(), account, nil, payloadBytes, "claude-sonnet-4-6", "kiro-access-token", "claude-sonnet-4-6", nil)
	require.NoError(t, err)
	kiroPayload := buildResult.Payload
	require.NotContains(t, string(kiroPayload), `"profileArn":`)
}

func TestBuildKiroPayloadForAccount_KiroBuilderIDUsesCredentialProfileArn(t *testing.T) {
	account := &Account{
		ID:       33,
		Name:     "kiro-builder-id-cached",
		Platform: PlatformKiro,
		Type:     AccountTypeOAuth,
		Credentials: map[string]any{
			"auth_method": "builder-id",
			"provider":    "BuilderId",
			"region":      "us-east-1",
			"client_id":   "builder-client-id",
			"profile_arn": "arn:aws:codewhisperer:us-east-1:123456789012:profile/CACHED",
		},
	}

	testPayload, err := createTestPayload("claude-sonnet-4-6")
	require.NoError(t, err)
	payloadBytes, err := json.Marshal(testPayload)
	require.NoError(t, err)

	buildResult, err := (&GatewayService{}).buildKiroPayloadForAccount(context.Background(), account, nil, payloadBytes, "claude-sonnet-4-6", "kiro-access-token", "claude-sonnet-4-6", nil)
	require.NoError(t, err)
	kiroPayload := buildResult.Payload
	require.Contains(t, string(kiroPayload), `"profileArn":"arn:aws:codewhisperer:us-east-1:123456789012:profile/CACHED"`)
}

func TestForwardKiroMessagesStreamCapturesMeteringCredits(t *testing.T) {
	gin.SetMode(gin.TestMode)
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/messages", nil)

	account := &Account{
		ID:          21,
		Name:        "kiro-stream-credits",
		Platform:    PlatformKiro,
		Type:        AccountTypeOAuth,
		Status:      StatusActive,
		Schedulable: true,
		Concurrency: 1,
		Credentials: map[string]any{
			"access_token": "kiro-access-token",
			"profile_arn":  "arn:aws:codewhisperer:us-east-1:123456789012:profile/STREAMCREDITS",
		},
	}
	upstreamBody := bytes.NewBuffer(nil)
	_, _ = upstreamBody.Write(buildKiroEventStreamFrame(t, "assistantResponseEvent", map[string]any{
		"assistantResponseEvent": map[string]any{"content": "hello"},
	}))
	_, _ = upstreamBody.Write(buildKiroEventStreamFrame(t, "messageMetadataEvent", map[string]any{
		"messageMetadataEvent": map[string]any{
			"tokenUsage": map[string]any{
				"uncachedInputTokens": 7,
				"outputTokens":        3,
			},
		},
	}))
	_, _ = upstreamBody.Write(buildKiroEventStreamFrame(t, "meteringEvent", map[string]any{
		"meteringEvent": map[string]any{"usage": 0.17},
	}))
	upstream := &queuedHTTPUpstream{
		responses: []*http.Response{{
			StatusCode: http.StatusOK,
			Header:     http.Header{"Content-Type": []string{"application/vnd.amazon.eventstream"}},
			Body:       io.NopCloser(upstreamBody),
		}},
	}
	svc := &GatewayService{
		httpUpstream:        upstream,
		kiroCooldownStore:   &stubKiroCooldownStore{},
		tlsFPProfileService: &TLSFingerprintProfileService{},
		cfg: &config.Config{
			Gateway: config.GatewayConfig{
				StreamDataIntervalTimeout: 0,
				MaxLineSize:               defaultMaxLineSize,
			},
		},
		rateLimitService: &RateLimitService{},
	}
	requestBody := []byte(`{"model":"claude-sonnet-4-6","stream":true,"messages":[{"role":"user","content":"hi"}]}`)
	parsed, err := ParseGatewayRequest(NewRequestBodyRef(requestBody), domain.PlatformAnthropic)
	require.NoError(t, err)

	result, err := svc.forwardKiroMessages(context.Background(), c, account, parsed, time.Now())
	require.NoError(t, err)
	require.NotNil(t, result)
	require.True(t, result.Stream)
	require.InDelta(t, 0.17, result.Usage.KiroCredits, 0.000001)
	require.Equal(t, 3, result.Usage.OutputTokens)
	require.NotContains(t, rec.Body.String(), "_sub2api_kiro_credits")
}

func buildKiroEventStreamFrame(t *testing.T, eventType string, payload map[string]any) []byte {
	t.Helper()
	payloadBytes, err := json.Marshal(payload)
	require.NoError(t, err)

	headerName := []byte(":event-type")
	headerValue := []byte(eventType)
	headersLen := 1 + len(headerName) + 1 + 2 + len(headerValue)
	totalLen := 12 + headersLen + len(payloadBytes) + 4
	frame := make([]byte, totalLen)
	binary.BigEndian.PutUint32(frame[0:4], uint32(totalLen))
	binary.BigEndian.PutUint32(frame[4:8], uint32(headersLen))
	offset := 12
	frame[offset] = byte(len(headerName))
	offset++
	copy(frame[offset:], headerName)
	offset += len(headerName)
	frame[offset] = 7 // string header
	offset++
	binary.BigEndian.PutUint16(frame[offset:offset+2], uint16(len(headerValue)))
	offset += 2
	copy(frame[offset:], headerValue)
	offset += len(headerValue)
	copy(frame[offset:], payloadBytes)
	return frame
}

func TestBuildKiroPayloadForAccount_KiroEnterpriseIDCOmitsMissingProfileArn(t *testing.T) {
	account := &Account{
		ID:       4,
		Name:     "kiro-enterprise-idc",
		Platform: PlatformKiro,
		Type:     AccountTypeOAuth,
		Credentials: map[string]any{
			"auth_method": "idc",
			"provider":    "AWS",
			"region":      "us-east-1",
			"client_id":   "enterprise-client-id",
			"start_url":   "https://d-example.awsapps.com/start",
		},
	}

	testPayload, err := createTestPayload("claude-sonnet-4-6")
	require.NoError(t, err)
	payloadBytes, err := json.Marshal(testPayload)
	require.NoError(t, err)

	buildResult, err := (&GatewayService{}).buildKiroPayloadForAccount(context.Background(), account, nil, payloadBytes, "claude-sonnet-4-6", "kiro-access-token", "claude-sonnet-4-6", nil)
	require.NoError(t, err)
	kiroPayload := buildResult.Payload
	require.NotContains(t, string(kiroPayload), `"profileArn":`)
}

func TestBuildKiroPayloadForAccount_StableConversationIDByDefault(t *testing.T) {
	t.Setenv("SUB2API_KIRO_CONVERSATION_ID_MODE", "")
	account := &Account{
		ID:       44,
		Name:     "kiro-stable-conversation",
		Platform: PlatformKiro,
		Type:     AccountTypeOAuth,
		Credentials: map[string]any{
			"refresh_token": "stable-refresh",
			"profile_arn":   "arn:aws:codewhisperer:us-east-1:123456789012:profile/STABLE",
		},
	}
	body := []byte(`{"model":"claude-sonnet-4-5","system":"stable sys","messages":[{"role":"user","content":"hello"}]}`)
	ref := NewRequestBodyRef(body)
	parsed, err := ParseGatewayRequest(ref, "anthropic")
	require.NoError(t, err)
	parsed.Group = &Group{Platform: PlatformKiro}
	parsed.SessionContext = &SessionContext{APIKeyID: 9}

	first, err := (&GatewayService{}).buildKiroPayloadForAccount(context.Background(), account, parsed, body, "claude-sonnet-4.5", "kiro-access-token", "claude-sonnet-4.5", nil)
	require.NoError(t, err)
	second, err := (&GatewayService{}).buildKiroPayloadForAccount(context.Background(), account, parsed, body, "claude-sonnet-4.5", "rotated-token", "claude-sonnet-4.5", nil)
	require.NoError(t, err)

	firstID := gjson.GetBytes(first.Payload, "conversationState.conversationId").String()
	secondID := gjson.GetBytes(second.Payload, "conversationState.conversationId").String()
	require.NotEmpty(t, firstID)
	require.Equal(t, firstID, secondID)

	t.Setenv("SUB2API_KIRO_CONVERSATION_ID_MODE", "random")
	randomized, err := (&GatewayService{}).buildKiroPayloadForAccount(context.Background(), account, parsed, body, "claude-sonnet-4.5", "kiro-access-token", "claude-sonnet-4.5", nil)
	require.NoError(t, err)
	require.NotEqual(t, firstID, gjson.GetBytes(randomized.Payload, "conversationState.conversationId").String())
}
