//go:build unit

package service

import (
	"context"
	"encoding/base64"
	"net/http"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/pkg/xai"
	"github.com/stretchr/testify/require"
)

func TestInspectAccountTokenRiskFromAccessTokenBFS(t *testing.T) {
	account := &Account{
		Platform: PlatformGrok,
		Type:     AccountTypeOAuth,
		Credentials: map[string]any{
			"access_token": grokRiskTestJWT(`{"bfs":2,"sub":"flagged","email":"a@x.ai"}`),
			"email":        "a@x.ai",
		},
	}
	report := InspectAccountTokenRisk(account)
	require.Equal(t, xai.GrokRiskFlagged, report.Verdict)
	require.Equal(t, xai.GrokRiskKindJWT, report.Kind)
	require.True(t, report.HasBFS)
}

func TestInspectAccountTokenRiskCleanToken(t *testing.T) {
	account := &Account{
		Platform: PlatformGrok,
		Type:     AccountTypeOAuth,
		Credentials: map[string]any{
			"access_token": grokRiskTestJWT(`{"sub":"clean","bot_flag_source":0}`),
		},
	}
	report := InspectAccountTokenRisk(account)
	require.Equal(t, xai.GrokRiskClean, report.Verdict)
	require.False(t, report.HasBFS)
}

func TestInspectAccountTokenRiskPersistedBFS(t *testing.T) {
	account := &Account{
		Platform: PlatformGrok,
		Type:     AccountTypeOAuth,
		Credentials: map[string]any{
			"access_token": "opaque",
			"bfs":          true,
			"bfs_value":    float64(2),
		},
	}
	report := InspectAccountTokenRisk(account)
	require.Equal(t, xai.GrokRiskFlagged, report.Verdict)
	require.True(t, report.HasBFS)
}

func TestMergeGrokRiskReportsPrefersLiveFlag(t *testing.T) {
	source := int64(2)
	live := xai.GrokAccountState{
		Found:          true,
		BotFlagSource:  &source,
		BotFlagDetails: "eapi_ip_bot_farm=1,policy=allow",
		Policy:         "allow",
		StatusCode:     http.StatusOK,
	}
	jwt := GrokRiskReport{Verdict: xai.GrokRiskClean, Source: "jwt"}
	merged := MergeGrokRiskReports(live, jwt)
	require.Equal(t, xai.GrokRiskFlagged, merged.Verdict)
	require.Equal(t, xai.GrokRiskKindIP, merged.Kind)
	require.Contains(t, merged.Source, "grok.com")
}

func TestGrokOAuthServiceInspectSSOAccountStateUsesInspector(t *testing.T) {
	source := int64(1)
	svc := NewGrokOAuthService(nil, &grokOAuthClientStub{})
	defer svc.Stop()
	svc.WithSSOStateInspector(func(context.Context, string, string) xai.GrokAccountState {
		return xai.GrokAccountState{Found: true, BotFlagSource: &source, Denied: true, Policy: "deny"}
	})
	state := svc.InspectSSOAccountState(context.Background(), "sso-token", nil)
	require.Equal(t, xai.GrokRiskFlagged, xai.ClassifyGrokAccountState(state))
}

func TestBuildAccountCredentialsStoresBFS(t *testing.T) {
	svc := NewGrokOAuthService(nil, &grokOAuthClientStub{})
	defer svc.Stop()
	creds := svc.BuildAccountCredentials(&GrokTokenInfo{
		AccessToken: "token",
		HasBFS:      true,
		BFS:         float64(2),
	})
	require.Equal(t, true, creds["bfs"])
	require.EqualValues(t, 2, creds["bfs_value"])
}

func TestGrokMediaGenerationEligibilityRejectsBFSToken(t *testing.T) {
	weeklyUsagePercent := 12.5
	account := &Account{
		Platform: PlatformGrok,
		Type:     AccountTypeOAuth,
		Credentials: map[string]any{
			"access_token": grokRiskTestJWT(`{"bfs":2,"sub":"flagged"}`),
		},
		Extra: map[string]any{grokBillingExtraKey: &xai.BillingSummary{
			PeriodType:       "weekly",
			UsagePercent:     &weeklyUsagePercent,
			StatusCode:       http.StatusOK,
			WeeklyStatusCode: http.StatusOK,
		}},
	}
	ok, reason := account.GrokMediaGenerationEligibility()
	require.False(t, ok)
	require.Equal(t, "token_bot_flagged", reason)
}

func grokRiskTestJWT(payloadJSON string) string {
	header := base64.RawURLEncoding.EncodeToString([]byte(`{"alg":"none","typ":"JWT"}`))
	payload := base64.RawURLEncoding.EncodeToString([]byte(payloadJSON))
	return header + "." + payload + ".sig"
}
