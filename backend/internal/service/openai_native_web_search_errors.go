package service

import (
	"errors"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/tidwall/gjson"
)

func (s *OpenAIGatewayService) readOpenAIResponseBody(resp *http.Response, c *gin.Context, account *Account) ([]byte, error) {
	if account == nil || !account.IsOpenAI() {
		return ReadUpstreamResponseBody(resp.Body, s.cfg, c, openAITooLargeError)
	}
	body, err := readUpstreamResponseBodyLimitedWithPartial(resp.Body, resolveUpstreamResponseReadLimit(s.cfg))
	if errors.Is(err, ErrUpstreamResponseBodyTooLarge) {
		setOpsUpstreamError(c, http.StatusBadGateway, "upstream response too large", "")
		openAITooLargeError(c)
	}
	return body, err
}

func (s *OpenAIGatewayService) nativeSearchPartialUsage(body []byte) *OpenAIUsage {
	if usage, ok := extractOpenAIUsageFromJSONBytes(body); ok && !bodyHasSSEFraming(body) {
		return &usage
	}
	return s.parseSSEUsageFromBody(string(body))
}

func (s *OpenAIGatewayService) nativeSearchFailedResponseError(resp *http.Response, c *gin.Context, account *Account, body []byte) error {
	if gjson.GetBytes(body, "status").String() != "failed" || countOpenAINativeWebSearchCalls(account, body) == 0 {
		return nil
	}
	markOpenAICyberPolicyEvent(c, body, resp.StatusCode, s.nativeSearchPartialUsage(body))
	message := extractOpenAISSEErrorMessage(body)
	if message == "" {
		message = "Upstream response failed"
	}
	return s.writeOpenAINonStreamingProtocolError(resp, c, message)
}
