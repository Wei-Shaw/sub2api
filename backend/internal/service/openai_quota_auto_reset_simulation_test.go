//go:build simulation

package service

import (
	"context"
	"encoding/json"
	"net/http"
	"net/url"
	"os"
	"strings"
	"testing"

	"github.com/imroc/req/v3"
	"github.com/stretchr/testify/require"
)

// TestOpenAIResetExpiryPostmanSimulation drives the expiry evaluator and actual
// quota HTTP client together. The fixture URL is opt-in and contains synthetic cards only.
func TestOpenAIResetExpiryPostmanSimulation(t *testing.T) {
	base := os.Getenv("TEST_OPENAI_RESET_EXPIRY_SIMULATION_URL")
	if base == "" {
		t.Skip("set TEST_OPENAI_RESET_EXPIRY_SIMULATION_URL to the local Postman simulation")
	}
	target, err := url.Parse(base)
	require.NoError(t, err)
	require.Contains(t, []string{"127.0.0.1", "localhost"}, target.Hostname(), "this fixture must never target production")

	type observedRequest struct {
		Account         string `json:"account"`
		Action          string `json:"action"`
		CreditID        string `json:"credit_id"`
		RedeemRequestID string `json:"redeem_request_id"`
	}
	readRequests := func(t *testing.T, account string) []observedRequest {
		t.Helper()
		response, err := http.Get(strings.TrimRight(base, "/") + "/_requests")
		require.NoError(t, err)
		defer response.Body.Close()
		require.Equal(t, http.StatusOK, response.StatusCode)
		var rows []observedRequest
		require.NoError(t, json.NewDecoder(response.Body).Decode(&rows))
		var consumed []observedRequest
		usage, credits := 0, 0
		for _, row := range rows {
			if row.Account != account {
				continue
			}
			switch row.Action {
			case "consume":
				consumed = append(consumed, row)
			case "usage":
				usage++
			case "credits":
				credits++
			}
		}
		require.Positive(t, usage, "the real client must reach the fixture's usage endpoint")
		require.Positive(t, credits, "the real client must query official card details before consumption")
		return consumed
	}

	for index, scenario := range []string{"success", "noop", "retry", "unknown"} {
		t.Run(scenario, func(t *testing.T) {
			identity := "fixture-" + scenario
			account := &Account{
				ID: int64(650001 + index), Platform: PlatformOpenAI, Type: AccountTypeOAuth,
				Status: "disabled", Schedulable: false,
				Credentials: map[string]any{"chatgpt_account_id": identity},
				Extra: map[string]any{
					OpenAIAutoResetCreditEnabledExtraKey:       false,
					OpenAIAutoResetCreditExpiryEnabledExtraKey: true,
					OpenAIAutoResetCreditExpiryHoursExtraKey:   1.0,
				},
			}
			repo := &autoResetTestAccountRepo{account: account}
			cache := &stubQuotaTokenCache{tokens: map[string]string{OpenAITokenCacheKey(account): "synthetic-token"}}
			factory := func(_ string) (*req.Client, error) {
				client := req.C().WrapRoundTripFunc(func(rt req.RoundTripper) req.RoundTripFunc {
					return func(request *req.Request) (*req.Response, error) {
						request.URL.Scheme = target.Scheme
						request.URL.Host = target.Host
						request.URL.Path = strings.TrimRight(target.Path, "/") + request.URL.Path
						return rt.RoundTrip(request)
					}
				})
				return client, nil
			}
			quota := NewOpenAIQuotaService(repo, nil, NewOpenAITokenProvider(repo, cache, nil), factory, nil)
			config := DefaultIdempotencyConfig()
			config.ObserveOnly = false
			config.FailedRetryBackoff = 0
			coordinator := NewIdempotencyCoordinator(newInMemoryIdempotencyRepo(), config)
			service := NewOpenAIQuotaAutoResetService(repo, quota, autoResetTestRecoverer{}, coordinator, nil, nil, nil)
			firstErr := service.evaluateAccount(context.Background(), account.ID)
			if scenario == "retry" || scenario == "unknown" {
				require.Error(t, firstErr)
			} else {
				require.NoError(t, firstErr)
			}

			// Recreate the evaluator to verify that deduplication survives process-local state loss.
			restarted := NewOpenAIQuotaAutoResetService(repo, quota, autoResetTestRecoverer{}, coordinator, nil, nil, nil)
			secondErr := restarted.evaluateAccount(context.Background(), account.ID)
			require.NoError(t, secondErr)
			rows := readRequests(t, identity)
			if scenario == "retry" || scenario == "unknown" {
				require.Len(t, rows, 2)
				require.Equal(t, rows[0].CreditID, rows[1].CreditID)
				require.Equal(t, rows[0].RedeemRequestID, rows[1].RedeemRequestID, "an uncertain response must preserve the original redemption identity")
			} else {
				require.Len(t, rows, 1, "rescans and restarts must not create a new redemption")
			}
			require.Equal(t, "credit-"+identity, rows[0].CreditID)
			require.NotEmpty(t, rows[0].RedeemRequestID)
			state := repo.stateForTest()
			switch scenario {
			case "noop":
				require.Equal(t, OpenAIAutoResetStatusNotConsumed, state.Status)
				require.Equal(t, 1, state.AvailableCount)
			default:
				require.Equal(t, OpenAIAutoResetStatusSuccess, state.Status)
				require.Zero(t, state.AvailableCount)
			}
		})
	}
}
