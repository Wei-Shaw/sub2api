//go:build unit

package service

import (
	"context"
	"net/http"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/model"
	"github.com/stretchr/testify/require"
)

const relayGenericFailureBody = `{"error":{"type":"upstream_error","message":"Upstream request failed（上游请求失败，请稍后重试）"}}`
const relayForbiddenMessage = "Upstream access forbidden, please contact administrator（上游拒绝访问，请联系管理员）"
const relayForbiddenPayload = `{"type":"response.failed","response":{"id":"resp_test","status":"failed","error":{"code":"upstream_error","message":"` + relayForbiddenMessage + `"}}}`

func relayPoolTestAccount() *Account {
	return &Account{ID: 1, Platform: PlatformOpenAI, Type: AccountTypeAPIKey, Credentials: map[string]any{"pool_mode": true}}
}

func TestRelayGeneric400GetsOneReplayWithoutCommitting(t *testing.T) {
	for _, compat := range []bool{false, true} {
		c, rec := newOpenAIUpstreamErrorTestContext(t)
		svc := &OpenAIGatewayService{cfg: &config.Config{}}
		resp := newOpenAIUpstreamErrorResponse(http.StatusBadRequest, relayGenericFailureBody)
		var err error
		if compat {
			_, err = svc.handleChatCompletionsErrorResponse(resp, c, relayPoolTestAccount())
		} else {
			_, err = svc.handleErrorResponse(context.Background(), resp, c, relayPoolTestAccount(), nil)
		}
		var failoverErr *UpstreamFailoverError
		require.ErrorAs(t, err, &failoverErr)
		require.True(t, failoverErr.RetryableOnSameAccount)
		require.Equal(t, 1, failoverErr.SameAccountRetryMax)
		require.False(t, c.Writer.Written())
		require.Empty(t, rec.Body.String())
	}
}

func TestRelayGeneric400PreservesOtherClientErrors(t *testing.T) {
	for _, body := range []string{
		`{"error":{"type":"invalid_request_error","message":"Upstream request failed"}}`,
		`{"error":{"type":"upstream_error","message":"Upstream rejected the request（请检查请求参数）"}}`,
		`{"error":{"type":"upstream_error","message":"Upstream request failed because invalid schema"}}`,
		`{"error":{"type":"upstream_error","code":"invalid_request","message":"Upstream request failed"}}`,
	} {
		c, rec := newOpenAIUpstreamErrorTestContext(t)
		_, err := (&OpenAIGatewayService{cfg: &config.Config{}}).handleErrorResponse(context.Background(), newOpenAIUpstreamErrorResponse(http.StatusBadRequest, body), c, relayPoolTestAccount(), nil)
		var failoverErr *UpstreamFailoverError
		require.NotErrorAs(t, err, &failoverErr)
		require.Equal(t, http.StatusBadRequest, rec.Code)
	}
	c, rec := newOpenAIUpstreamErrorTestContext(t)
	_, err := (&OpenAIGatewayService{cfg: &config.Config{}}).handleErrorResponse(context.Background(), newOpenAIUpstreamErrorResponse(http.StatusBadRequest, relayGenericFailureBody), c, newOpenAIUpstreamErrorTestAccount(), nil)
	var failoverErr *UpstreamFailoverError
	require.NotErrorAs(t, err, &failoverErr)
	require.Equal(t, http.StatusBadRequest, rec.Code)
}

func TestRelayGeneric400PreservesPassthroughRuleAndDisabledAccount(t *testing.T) {
	c, rec := newOpenAIUpstreamErrorTestContext(t)
	rules := &ErrorPassthroughService{}
	rules.setLocalCache([]*model.ErrorPassthroughRule{newNonFailoverPassthroughRule(http.StatusBadRequest, "Upstream request failed", http.StatusTeapot, "operator response")})
	BindErrorPassthroughService(c, rules)
	svc := &OpenAIGatewayService{cfg: &config.Config{}}
	_, err := svc.handleErrorResponse(context.Background(), newOpenAIUpstreamErrorResponse(http.StatusBadRequest, relayGenericFailureBody), c, relayPoolTestAccount(), nil)
	var failoverErr *UpstreamFailoverError
	require.NotErrorAs(t, err, &failoverErr)
	require.Equal(t, http.StatusTeapot, rec.Code)
	require.Contains(t, rec.Body.String(), "operator response")
	failoverErr = svc.newOpenAIAccountFailoverError(relayPoolTestAccount(), http.StatusBadRequest, nil, []byte(relayGenericFailureBody), "Upstream request failed", true, false)
	require.False(t, failoverErr.RetryableOnSameAccount)
}

func TestRelayForbiddenFailedEventFailsOverWithoutCredentialPenalty(t *testing.T) {
	c, rec := newOpenAIUpstreamErrorTestContext(t)
	resp := newOpenAIUpstreamErrorResponse(http.StatusOK, "event: response.failed\ndata: "+relayForbiddenPayload+"\n\n")
	result, err := (&OpenAIGatewayService{}).handleChatBufferedStreamingResponse(resp, c, relayPoolTestAccount(), "model", "model", "model", time.Now())
	require.Nil(t, result)
	var failoverErr *UpstreamFailoverError
	require.ErrorAs(t, err, &failoverErr)
	require.True(t, failoverErr.RetryableOnSameAccount)
	require.False(t, openAIStream403AccountFailure([]byte(relayForbiddenPayload), relayForbiddenMessage))
	_, _, eligible := classifyOpenAIAPIKeyHealthFailure(err)
	require.False(t, eligible)
	require.False(t, c.Writer.Written())
	require.Empty(t, rec.Body.String())
}

func TestRelayForbiddenStillPreservesPermissionFailuresAndPassthrough(t *testing.T) {
	permission := []byte(`{"type":"response.failed","response":{"error":{"type":"permission_error","code":"forbidden","message":"You are not allowed to use this tool"}}}`)
	require.False(t, openAIStreamFailedEventShouldFailover(permission, "You are not allowed to use this tool"))
	permission = []byte(`{"type":"response.failed","response":{"error":{"type":"permission_error","code":"upstream_error","message":"` + relayForbiddenMessage + `"}}}`)
	require.False(t, openAIStreamFailedEventShouldFailover(permission, relayForbiddenMessage))
	c, rec := newOpenAIUpstreamErrorTestContext(t)
	rules := &ErrorPassthroughService{}
	rules.setLocalCache([]*model.ErrorPassthroughRule{newNonFailoverPassthroughRule(http.StatusForbidden, "Upstream access forbidden", http.StatusTeapot, "operator response")})
	BindErrorPassthroughService(c, rules)
	_, err := (&OpenAIGatewayService{}).handleChatBufferedStreamingResponse(newOpenAIUpstreamErrorResponse(http.StatusOK, "data: "+relayForbiddenPayload+"\n\n"), c, relayPoolTestAccount(), "model", "model", "model", time.Now())
	var failoverErr *UpstreamFailoverError
	require.NotErrorAs(t, err, &failoverErr)
	require.Equal(t, http.StatusTeapot, rec.Code)
	require.Contains(t, rec.Body.String(), "operator response")
}

func TestRelayForbiddenDoesNotReplayAfterStreamOutput(t *testing.T) {
	c, rec := newOpenAIUpstreamErrorTestContext(t)
	stream := "data: {\"type\":\"response.created\",\"response\":{\"id\":\"resp_test\",\"status\":\"in_progress\",\"model\":\"model\"}}\n\n" +
		"data: {\"type\":\"response.output_text.delta\",\"delta\":\"already sent\",\"output_index\":0,\"content_index\":0}\n\n" +
		"data: " + relayForbiddenPayload + "\n\n"
	_, err := (&OpenAIGatewayService{}).handleChatStreamingResponse(newOpenAIUpstreamErrorResponse(http.StatusOK, stream), c, relayPoolTestAccount(), "model", "model", "model", time.Now(), 0)
	var failoverErr *UpstreamFailoverError
	require.NotErrorAs(t, err, &failoverErr)
	require.Contains(t, rec.Body.String(), "already sent")
	require.True(t, c.Writer.Written())
}
