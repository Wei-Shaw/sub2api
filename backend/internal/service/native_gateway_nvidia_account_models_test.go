//go:build unit

package service

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// Read through the normal account-model boundary with an encrypted API-key
// fixture. No provider network or OAuth catalogue is involved in this profile.
func TestGatewayNVIDIAAccountModelsPhysicalRouteAndFences(t *testing.T) {
	for _, change := range []string{"none", "profile", "origin", "model", "account_id", "birth", "submicrosecond_birth", "key_type", "owner", "epoch", "generation_after_read", "credentials_after_read"} {
		t.Run(change, func(t *testing.T) {
			account := &Account{
				ID:          17,
				CreatedAt:   time.Date(2026, 11, 10, 0, 0, 0, 0, time.UTC),
				Platform:    PlatformOpenAI,
				Type:        AccountTypeAPIKey,
				Status:      StatusActive,
				Credentials: map[string]any{"api_key": "nvidia-synthetic-credential", "base_url": GatewayNVIDIABuildBaseURL},
				Extra: map[string]any{
					GatewayGenerationExtraKey:              "12345678-1234-4234-8234-123456789abc",
					GatewayProfileExtraKey:                 GatewayNVIDIABuildChatProfile,
					GatewayModelExtraKey:                   GatewayNVIDIABuildAnchorModel,
					"openai_responses_mode":                "force_chat_completions",
					"openai_passthrough":                   false,
					"native_api_key_cancel_on_disconnect":  true,
					"openai_preserve_compatible_reasoning": true,
				},
			}
			nativeFixtureSealAccount(t, account)
			route, err := GatewayNativeDescriptor(account)
			require.NoError(t, err)
			scope, err := GatewayNativeCredentialScopeForAccount(account)
			require.NoError(t, err)
			input := GatewayAccountModelsInput{GatewayAccountLimitsInput: GatewayAccountLimitsInput{scope.Owner, scope.Account, GatewayNVIDIABuildChatProfile, 7}}
			switch change {
			case "profile":
				route.Profile = GatewayCodexOAuthResponsesProfile
			case "origin":
				route.BaseURL = "https://example.invalid/v1"
			case "model":
				route.Model = "vendor/unqualified-model"
			case "account_id":
				route.AccountID++
			case "birth":
				route.CreatedAt = route.CreatedAt.Add(time.Second)
			case "submicrosecond_birth":
				route.CreatedAt = route.CreatedAt.Add(time.Nanosecond)
			case "key_type":
				account.Type = AccountTypeOAuth
			case "owner":
				input.OwnerRef = "another-owner"
			case "epoch":
				input.ExpectedAuthorizationEpoch++
			}
			repository := &gatewayIdentityRepoFixture{row: account}
			// The public list is unreachable here, so the catalogue is the anchor alone.
			gateway := &OpenAIGatewayService{accountRepo: repository, gatewayLiveModels: NewGatewayLiveModels(roundTripFunc(func(*http.Request) (*http.Response, error) {
				return nil, errors.New("no provider in this test")
			}))}
			authorizations := 0
			reader, err := NewGatewayAccountModelsReader(nil, gateway, func(_ context.Context, in GatewayAccountLimitsInput) (GatewayNativeRoute, error) {
				authorizations++
				if in.OwnerRef != scope.Owner || in.AccountRef != scope.Account || in.ExpectedAuthorizationEpoch != 7 {
					return GatewayNativeRoute{}, ErrGatewayNativeIdentity
				}
				if authorizations == 2 {
					if change == "generation_after_read" {
						changed := route
						changed.Generation = "22345678-1234-4234-8234-123456789abc"
						return changed, nil
					}
					if change == "credentials_after_read" {
						fresh := *account
						fresh.Credentials = map[string]any{"api_key": "changed-credential", "base_url": GatewayNVIDIABuildBaseURL}
						repository.row = &fresh
					}
				}
				return route, nil
			})
			require.NoError(t, err)
			result := reader.Read(nativeFixtureContext(t, context.Background()), input)
			if change != "none" {
				require.Equal(t, GatewayAccountModelsResult{Status: "unavailable"}, result)
				return
			}
			require.Equal(t, 2, authorizations, "catalogue reads recheck logical authority")
			require.Equal(t, "snapshot", result.Status)
			require.Equal(t, "nvidia", result.Source)
			require.Equal(t, route.Generation, result.PhysicalGeneration)
			require.Equal(t, input.AccountRef, result.PrincipalVersion)
			require.Equal(t, []GatewayAccountModel{{ModelID: GatewayNVIDIABuildAnchorModel, Tools: GatewayAccountModelChoices{Allowed: []string{"function"}, Default: "function"}}}, *result.Models)
			first, err := time.Parse(time.RFC3339Nano, result.FetchedAt)
			require.NoError(t, err)
			last, err := time.Parse(time.RFC3339Nano, result.ExpiresAt)
			require.NoError(t, err)
			require.Equal(t, time.Minute, last.Sub(first))
			wire, err := json.Marshal(result)
			require.NoError(t, err)
			require.NotContains(t, string(wire), "nvidia-synthetic-credential")
			require.NotContains(t, string(wire), "gcn1.")
		})
	}
}
