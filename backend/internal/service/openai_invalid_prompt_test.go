package service

import (
	"context"
	"errors"
	"net/http"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

// issue #8551: OpenAI 用 code=invalid_prompt + type=invalid_request_error 表示 prompt
// 被 usage policy 拦截。它和 cyber_policy 一样是请求内容被策略拒绝，必须进入风控
// 链路（透传、不 failover、不打账号），而不是落到普通 400 参数错误分支。
const openAIInvalidPromptPolicyBody = `{"error":{` +
	`"code":"invalid_prompt",` +
	`"message":"Invalid prompt: your prompt was flagged as potentially violating our usage policy. Please try again with a different prompt: https://platform.openai.com/docs/guides/reasoning#advice-on-prompting",` +
	`"param":null,` +
	`"type":"invalid_request_error"},` +
	`"sequence_number":3,"type":"error"}`

func TestHandleErrorResponse_InvalidPromptPolicyBlockIsForwardedAndMarked(t *testing.T) {
	c, rec := newOpenAIUpstreamErrorTestContext(t)
	svc := &OpenAIGatewayService{cfg: &config.Config{}}

	_, err := svc.handleErrorResponse(
		context.Background(),
		newOpenAIUpstreamErrorResponse(http.StatusBadRequest, openAIInvalidPromptPolicyBody),
		c, newOpenAIUpstreamErrorTestAccount(), nil,
	)

	require.Error(t, err)
	var failoverErr *UpstreamFailoverError
	require.False(t, errors.As(err, &failoverErr), "prompt policy block must not fail over")
	require.Equal(t, http.StatusBadRequest, rec.Code)
	require.Equal(t, "invalid_prompt", gjson.Get(rec.Body.String(), "error.code").String())
	require.Equal(t, "invalid_request_error", gjson.Get(rec.Body.String(), "error.type").String())

	mark := GetOpsCyberPolicy(c)
	require.NotNil(t, mark, "prompt policy block must enter the risk-control pipeline")
	require.Equal(t, "invalid_prompt", mark.Code)
	require.Equal(t, http.StatusBadRequest, mark.UpstreamStatus)
}

func TestHandleErrorResponse_InvalidRequestParamErrorDoesNotMarkPolicyBlock(t *testing.T) {
	c, rec := newOpenAIUpstreamErrorTestContext(t)
	svc := &OpenAIGatewayService{cfg: &config.Config{}}

	_, err := svc.handleErrorResponse(
		context.Background(),
		newOpenAIUpstreamErrorResponse(http.StatusBadRequest, openAIInvalidFunctionParametersBody),
		c, newOpenAIUpstreamErrorTestAccount(), nil,
	)

	require.Error(t, err)
	require.Equal(t, http.StatusBadRequest, rec.Code)
	require.Nil(t, GetOpsCyberPolicy(c), "ordinary 400 parameter errors must stay out of risk control")
}

func TestHandleCompatErrorResponse_InvalidPromptPolicyBlockIsMarked(t *testing.T) {
	c, rec := newOpenAIUpstreamErrorTestContext(t)
	svc := &OpenAIGatewayService{cfg: &config.Config{}}

	_, err := svc.handleCompatErrorResponse(
		newOpenAIUpstreamErrorResponse(http.StatusBadRequest, openAIInvalidPromptPolicyBody),
		c, newOpenAIUpstreamErrorTestAccount(), writeChatCompletionsError,
	)

	require.Error(t, err)
	var failoverErr *UpstreamFailoverError
	require.False(t, errors.As(err, &failoverErr))
	require.Equal(t, http.StatusBadRequest, rec.Code)
	require.Equal(t, "invalid_request_error", gjson.Get(rec.Body.String(), "error.type").String())
	require.Contains(t, gjson.Get(rec.Body.String(), "error.message").String(), "usage policy")

	mark := GetOpsCyberPolicy(c)
	require.NotNil(t, mark)
	require.Equal(t, "invalid_prompt", mark.Code)
}

func TestShouldFailoverOpenAIUpstreamResponse_InvalidPromptPolicyBlockNeverFailsOver(t *testing.T) {
	svc := &OpenAIGatewayService{}
	body := []byte(openAIInvalidPromptPolicyBody)

	require.False(t, svc.shouldFailoverOpenAIUpstreamResponse(
		newOpenAIUpstreamErrorTestAccount(), http.StatusBadRequest, "", body,
	))
	require.False(t, svc.shouldFailoverOpenAIUpstreamResponse(
		newOpenAIUpstreamErrorTestAccount(), http.StatusBadGateway, "wrapped upstream failure", body,
	))
}

func TestOpenAIStreamTerminals_InvalidPromptPolicyBlockNeverFailsOver(t *testing.T) {
	payload := []byte(openAIInvalidPromptPolicyBody)
	message := extractOpenAISSEErrorMessage(payload)

	require.False(t, openAIStreamErrorEventShouldFailover(payload, message))
	require.False(t, openAIStreamFailedEventShouldFailover(payload, message))
}
