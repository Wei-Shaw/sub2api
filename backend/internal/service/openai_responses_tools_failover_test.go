//go:build unit

package service

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestResponsesSolToolsCapabilityFailover(t *testing.T) {
	gin.SetMode(gin.TestMode)
	for _, tools := range []struct{ name, fields string }{
		{"function", `"tools":[{"type":"function","name":"lookup","parameters":{"type":"object"}}]`},
		{"custom", `"tools":[{"type":"custom","name":"exec"}]`},
		{"namespace", `"tools":[{"type":"namespace","name":"shell","tools":[{"type":"function","name":"pwd","parameters":{"type":"object"}}]}]`},
		{"additional", `"additional_tools":[{"type":"function","name":"lookup","parameters":{"type":"object"}}]`},
		{"native", `"tools":[{"type":"computer_use_preview","display_width":1024,"display_height":768,"environment":"browser"}]`},
	} {
		for _, stream := range []bool{false, true} {
			t.Run(fmt.Sprintf("%s/stream=%t", tools.name, stream), func(t *testing.T) {
				body := fmt.Sprintf(`{"model":"public-alias","input":"hello","stream":%t,%s}`, stream, tools.fields)
				c, rec := newSolToolsFallbackContext(body)
				account := forceChatResponsesFallbackAccount()
				account.Credentials["model_mapping"] = map[string]any{"public-alias": "gpt-6.1-sol"}
				upstream := &httpUpstreamRecorder{}
				svc := &OpenAIGatewayService{
					cfg: rawChatCompletionsTestConfig(), accountRepo: &accountRepoStub{}, httpUpstream: upstream,
				}

				result, err := svc.forwardResponsesViaRawChatCompletions(context.Background(), c, account, []byte(body), time.Now())

				require.Nil(t, result)
				var failover *UpstreamFailoverError
				require.ErrorAs(t, err, &failover)
				require.True(t, failover.ShouldRetryNextAccount())
				require.False(t, failover.RetryableOnSameAccount)
				require.False(t, failover.ShouldReportAccountScheduleFailure())
				_, _, healthFailure := classifyOpenAIAPIKeyHealthFailure(failover)
				require.False(t, healthFailure)
				require.False(t, c.Writer.Written(), "another account must still be able to answer")
				require.Empty(t, rec.Body.String())
				require.Empty(t, upstream.requests, "the incompatible account must never be called")
				require.Equal(t, "gpt-6.1-sol", account.Credentials["model_mapping"].(map[string]any)["public-alias"])
			})
		}
	}
}

func TestResponsesSolInvalidReasoningDoesNotFailover(t *testing.T) {
	body := `{"model":"gpt-6.1-sol","input":"hello","reasoning":{"effort":"none"},"tools":[{"type":"function","name":"lookup","parameters":{"type":"object"}}]}`
	c, rec := newSolToolsFallbackContext(body)
	svc := &OpenAIGatewayService{cfg: rawChatCompletionsTestConfig(), accountRepo: &accountRepoStub{}}

	_, err := svc.forwardResponsesViaRawChatCompletions(context.Background(), c, forceChatResponsesFallbackAccount(), []byte(body), time.Now())

	var failover *UpstreamFailoverError
	require.Error(t, err)
	require.False(t, errors.As(err, &failover))
	require.Equal(t, http.StatusBadRequest, rec.Code)
}

func newSolToolsFallbackContext(body string) (*gin.Context, *httptest.ResponseRecorder) {
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", strings.NewReader(body))
	return c, rec
}
