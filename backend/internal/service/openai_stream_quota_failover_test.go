package service

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/stretchr/testify/require"
)

// Some Responses upstreams report exhausted credit over HTTP 200, without a
// response.failed event or a retryable phrase in the message.
const openAIStreamCreditExhaustedError = `{"code":"credit_balance_exhausted","message":"You have no credits remaining. Add credits to continue using the API at https://platform.openai.com/settings/organization/billing/.","param":null,"type":"insufficient_quota"}`

func TestOpenAIStreamQuotaExhaustionClassification(t *testing.T) {
	for _, tc := range []struct {
		name, payload string
	}{
		{"recorded_error", `{"error":` + openAIStreamCreditExhaustedError + `,"sequence_number":2,"type":"error"}`},
		{"response_failed", `{"type":"response.failed","response":{"error":` + openAIStreamCreditExhaustedError + `}}`},
		{"code_only", `{"type":"error","error":{"code":"credit_balance_exhausted","message":"billing rejected"}}`},
		{"quota_code", `{"type":"error","error":{"code":"insufficient_quota","type":"invalid_request_error"}}`},
		{"quota_type", `{"type":"error","error":{"type":"insufficient_quota","status":429}}`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			payload := []byte(tc.payload)
			message := extractOpenAISSEErrorMessage(payload)
			require.Equal(t, http.StatusPaymentRequired, openAIStreamFailureStatus(payload, message))
			require.True(t, openAIStreamErrorEventShouldFailover(payload, message))
			require.True(t, openAIStreamFailedEventShouldFailover(payload, message))
			require.False(t, openAIStreamFailedEventRetryableOnSameAccount(newNonStreamingFailoverAccount(), payload, message))
		})
	}
}

func TestOpenAIStreamQuotaExhaustionDoesNotTrustMessagesOrEchoedInput(t *testing.T) {
	for _, payload := range []string{
		`{"type":"error","error":{"code":"invalid_request_error","message":"Invalid input: credit_balance_exhausted insufficient_quota"}}`,
		`{"type":"error","error":{"message":"You have no credits remaining"}}`,
		`{"type":"error","error":{"code":"unknown_parameter"},"echo":{"error":{"code":"credit_balance_exhausted","type":"insufficient_quota"}}}`,
		`{"type":"error","error":{"code":"credit_balance_exhausted_invalid"}}`,
	} {
		t.Run(payload, func(t *testing.T) {
			repo := &openAIStream403AccountRepo{}
			svc := &OpenAIGatewayService{rateLimitService: &RateLimitService{accountRepo: repo}}
			account := &Account{ID: 1, Platform: PlatformOpenAI, Type: AccountTypeAPIKey}
			data := []byte(payload)
			message := extractOpenAISSEErrorMessage(data)
			require.False(t, openAIStreamErrorEventShouldFailover(data, message))
			status, disabled := svc.handleOpenAIStreamTerminalAccountSideEffects(nil, account, data, message, nil)
			require.NotEqual(t, http.StatusPaymentRequired, status)
			require.False(t, disabled)
			require.Zero(t, repo.setErrorCalls)
			require.False(t, svc.isOpenAIAccountRuntimeBlocked(account))
		})
	}
	data := []byte(`{"type":"error","error":{"code":"rate_limit_exceeded","type":"rate_limit_error"}}`)
	require.Equal(t, http.StatusTooManyRequests, openAIStreamFailureStatus(data, ""))
}

type openAIQuotaExhaustionAccountRepo struct {
	AccountRepository
	accountID int64
	message   string
}

func (r *openAIQuotaExhaustionAccountRepo) SetError(ctx context.Context, accountID int64, message string) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	r.accountID = accountID
	r.message = message
	return nil
}

func TestOpenAIStreamQuotaExhaustionDisablesAccountDespitePoolOrCustomPolicy(t *testing.T) {
	for _, tc := range []struct {
		name        string
		credentials map[string]any
	}{
		{"normal_account", nil},
		{"pool_account", map[string]any{"pool_mode": true}},
		{"custom_excludes_402", map[string]any{"custom_error_codes_enabled": true, "custom_error_codes": []any{float64(401)}}},
		{"custom_includes_402", map[string]any{"custom_error_codes_enabled": true, "custom_error_codes": []any{float64(402)}}},
		{"pool_retries_402", map[string]any{"pool_mode": true, "pool_mode_retry_status_codes": []any{float64(402)}}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			repo := &openAIQuotaExhaustionAccountRepo{}
			svc := &OpenAIGatewayService{rateLimitService: &RateLimitService{accountRepo: repo}}
			account := &Account{ID: 1, Platform: PlatformOpenAI, Type: AccountTypeAPIKey, Credentials: tc.credentials}
			payload := []byte(`{"type":"error","error":` + openAIStreamCreditExhaustedError + `}`)
			status, disabled := svc.handleOpenAIStreamTerminalAccountSideEffects(nil, account, payload, extractOpenAISSEErrorMessage(payload), nil)
			require.Equal(t, http.StatusPaymentRequired, status)
			require.True(t, disabled)
			require.Equal(t, account.ID, repo.accountID)
			require.Contains(t, repo.message, "quota exhausted")
			require.True(t, svc.isOpenAIAccountRuntimeBlocked(account))
			require.False(t, openAIStreamFailedEventRetryableOnSameAccount(account, payload, ""))
		})
	}
}

func TestOpenAIQuotaExhaustionDisablesAccountAfterRequestCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	repo := &openAIQuotaExhaustionAccountRepo{}
	svc := &OpenAIGatewayService{rateLimitService: &RateLimitService{accountRepo: repo}}
	account := &Account{ID: 1, Platform: PlatformOpenAI, Type: AccountTypeAPIKey}
	payload := []byte(`{"error":` + openAIStreamCreditExhaustedError + `}`)
	for _, status := range []int{http.StatusBadRequest, http.StatusPaymentRequired, http.StatusTooManyRequests, http.StatusBadGateway} {
		require.True(t, svc.handleOpenAIAccountUpstreamError(ctx, account, status, nil, payload))
		require.Equal(t, account.ID, repo.accountID)
		require.True(t, svc.isOpenAIAccountRuntimeBlocked(account))
	}
}

func TestOpenAIStreamQuotaExhaustionReplayBoundary(t *testing.T) {
	for _, passthrough := range []bool{false, true} {
		for _, event := range []string{"error", "response.failed"} {
			for _, afterOutput := range []bool{false, true} {
				t.Run(fmt.Sprintf("passthrough=%t/%s/after_output=%t", passthrough, event, afterOutput), func(t *testing.T) {
					c, rec := newNonStreamingFailoverContext(t)
					repo := &openAIStream403AccountRepo{}
					svc := &OpenAIGatewayService{cfg: &config.Config{}, toolCorrector: NewCodexToolCorrector(), rateLimitService: &RateLimitService{accountRepo: repo}}
					account := &Account{ID: 1, Platform: PlatformOpenAI, Type: AccountTypeAPIKey}
					payload := `{"type":"error","error":` + openAIStreamCreditExhaustedError + `}`
					if event == "response.failed" {
						payload = `{"type":"response.failed","response":{"status":"failed","error":` + openAIStreamCreditExhaustedError + `}}`
					}
					body := "event: " + event + "\ndata: " + payload + "\n\n"
					if afterOutput {
						body = "data: {\"type\":\"response.output_text.delta\",\"delta\":\"partial\"}\n\n" + body
					}
					resp := newNonStreamingSSEResponse()
					resp.Body = io.NopCloser(strings.NewReader(body))
					var err error
					if passthrough {
						_, err = svc.handleStreamingResponsePassthrough(c.Request.Context(), resp, c, account, time.Now(), "model", "model")
					} else {
						_, err = svc.handleStreamingResponse(c.Request.Context(), resp, c, account, time.Now(), "model", "model")
					}
					require.Error(t, err)
					var failoverErr *UpstreamFailoverError
					require.Equal(t, !afterOutput, errors.As(err, &failoverErr))
					if afterOutput {
						require.Contains(t, rec.Body.String(), "partial")
					} else {
						require.Equal(t, http.StatusPaymentRequired, failoverErr.StatusCode)
						require.False(t, failoverErr.RetryableOnSameAccount)
						require.False(t, c.Writer.Written())
						require.Empty(t, rec.Body.String())
					}
					require.Equal(t, 1, repo.setErrorCalls)
					require.True(t, svc.isOpenAIAccountRuntimeBlocked(account))
				})
			}
		}
	}
}

func TestOpenAINonStreamingSSEQuotaExhaustionFailsOver(t *testing.T) {
	for _, passthrough := range []bool{false, true} {
		for _, event := range []string{"error", "response.failed"} {
			t.Run(fmt.Sprintf("passthrough=%t/%s", passthrough, event), func(t *testing.T) {
				c, rec := newNonStreamingFailoverContext(t)
				repo := &openAIStream403AccountRepo{}
				svc := &OpenAIGatewayService{cfg: &config.Config{}, rateLimitService: &RateLimitService{accountRepo: repo}}
				account := &Account{ID: 1, Platform: PlatformOpenAI, Type: AccountTypeAPIKey}
				payload := `{"type":"` + event + `","error":` + openAIStreamCreditExhaustedError + `}`
				body := sseTerminalBody(event, payload)
				var err error
				if passthrough {
					_, err = svc.handlePassthroughSSEToJSON(newNonStreamingSSEResponse(), c, account, body, "model", "model")
				} else {
					_, err = svc.handleSSEToJSON(newNonStreamingSSEResponse(), c, account, body, "model", "model")
				}
				var failoverErr *UpstreamFailoverError
				require.ErrorAs(t, err, &failoverErr)
				require.Equal(t, http.StatusPaymentRequired, failoverErr.StatusCode)
				require.False(t, failoverErr.RetryableOnSameAccount)
				require.Empty(t, rec.Body.String())
				require.False(t, c.Writer.Written())
				require.Equal(t, 1, repo.setErrorCalls)
				require.True(t, svc.isOpenAIAccountRuntimeBlocked(account))
			})
		}
	}
}
