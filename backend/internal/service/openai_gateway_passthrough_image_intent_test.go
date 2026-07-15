package service

import (
	"context"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

func TestOpenAIGatewayService_APIKeyPassthrough_ImageIntentPreservesGateAndBilling(t *testing.T) {
	gin.SetMode(gin.TestMode)
	body := []byte(`{"model":"gpt-5.4","stream":false,"tools":[{"type":"image_generation","model":"gpt-image-2","size":"2048x1152"REDACTED],"input":"draw"REDACTED`)

	t.Run("disabled group rejects before upstream", func(t *testing.T) {
		upstream := &httpUpstreamRecorder{REDACTED
		svc := newOpenAIImageGenerationControlTestService(upstream)
		c, recorder := newOpenAIImageGenerationControlTestContext(false, "curl/8.0")
		account := newOpenAIImageGenerationControlTestAccount()
		account.Extra = map[string]any{"openai_passthrough": trueREDACTED

		result, err := svc.Forward(context.Background(), c, account, body)

	REDACTED
		require.Nil(t, result)
		require.Equal(t, http.StatusForbidden, recorder.Code)
		require.Equal(t, "permission_error", gjson.GetBytes(recorder.Body.Bytes(), "error.type").String())
		require.Nil(t, upstream.lastReq)
REDACTED)

	t.Run("allowed group keeps image billing", func(t *testing.T) {
		upstream := &httpUpstreamRecorder{resp: &http.Response{
			StatusCode: http.StatusOK,
			Header:     http.Header{"Content-Type": []string{"application/json"REDACTEDREDACTED,
			Body: io.NopCloser(strings.NewReader(
				`{"output":[{"id":"ig_1","type":"image_generation_call","result":"final-image","size":"2048x1152"REDACTED],"usage":{"input_tokens":1,"output_tokens":2REDACTEDREDACTED`,
			)),
	REDACTEDREDACTED
		svc := newOpenAIImageGenerationControlTestService(upstream)
		c, _ := newOpenAIImageGenerationControlTestContext(true, "curl/8.0")
		account := newOpenAIImageGenerationControlTestAccount()
		account.Extra = map[string]any{"openai_passthrough": trueREDACTED

		result, err := svc.Forward(context.Background(), c, account, body)

	REDACTED
		require.NotNil(t, result)
		require.NotNil(t, upstream.lastReq)
		require.Equal(t, body, upstream.lastBody)
		require.Equal(t, 1, result.ImageCount)
		require.Equal(t, "gpt-image-2", result.BillingModel)
		require.Equal(t, "2K", result.ImageSize)
		require.Equal(t, "2048x1152", result.ImageInputSize)
REDACTED)
REDACTED
