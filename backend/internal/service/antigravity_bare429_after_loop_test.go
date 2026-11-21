//go:build unit

package service

import (
	"context"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

// 2026-11-19 17:03 UTC: one Codex request (/v1/responses, 2 MB) got the bare 429
// on #2, #7 and #6. The retry loop cooled #2 and logged bare_429_cooldown_skipped
// for #7 and #6, but the compat path then cooled #7 and #6 anyway when it turned
// the 429 into a failover, so the whole pool was down for a minute.
func compatBare429(t *testing.T, svc *AntigravityGatewayService, c *gin.Context, accountID int64) error {
	t.Helper()
	call := &antigravityCompatUpstreamCall{
		prefix:  "[test]",
		request: antigravityCompatRequest{originalModel: "gemini-4-argon-medium"},
	}
	resp := &http.Response{
		StatusCode: http.StatusTooManyRequests,
		Header:     http.Header{},
		Body:       io.NopCloser(strings.NewReader(bareAntigravity429Body)),
	}
	return svc.handleAntigravityCompatHTTPError(context.Background(), c, bare429Account(accountID), call, resp)
}

func cooledAccounts(repo *stubAntigravityAccountRepo) map[int64]bool {
	cooled := map[int64]bool{}
	for _, call := range repo.modelRateLimitCalls {
		cooled[call.accountID] = true
	}
	for _, call := range repo.rateCalls {
		cooled[call.accountID] = true
	}
	return cooled
}

func TestCompatHTTPError_Bare429_DoesNotCoolASecondAccount(t *testing.T) {
	repo := &stubAntigravityAccountRepo{}
	svc := &AntigravityGatewayService{accountRepo: repo}
	c := newBare429TestContext()

	// The retry loop already cooled #2 for this request.
	require.True(t, claimBare429Cooldown(c, 2))

	for _, id := range []int64{7, 6} {
		err := compatBare429(t, svc, c, id)
		var failover *UpstreamFailoverError
		require.ErrorAs(t, err, &failover, "the request still fails over from account %d", id)
	}
	require.Empty(t, cooledAccounts(repo), "accounts 7 and 6 must not be cooled by the request that cooled 2")
}

func TestCompatHTTPError_Bare429_FirstAccountIsStillCooled(t *testing.T) {
	repo := &stubAntigravityAccountRepo{}
	svc := &AntigravityGatewayService{accountRepo: repo}
	c := newBare429TestContext()

	require.Error(t, compatBare429(t, svc, c, 13))
	require.Equal(t, map[int64]bool{13: true}, cooledAccounts(repo))
}

func TestCompatHTTPError_429WithReason_CoolsEveryAccount(t *testing.T) {
	repo := &stubAntigravityAccountRepo{}
	svc := &AntigravityGatewayService{accountRepo: repo}
	c := newBare429TestContext()
	require.True(t, claimBare429Cooldown(c, 2))
	body := `{"error":{"code":429,"message":"Individual quota reached.","status":"RESOURCE_EXHAUSTED","details":[{"@type":"type.googleapis.com/google.rpc.ErrorInfo","reason":"QUOTA_EXHAUSTED","metadata":{"quotaResetDelay":"41m15s"}}]}}`
	call := &antigravityCompatUpstreamCall{prefix: "[test]", request: antigravityCompatRequest{originalModel: "gemini-4-argon-medium"}}
	resp := &http.Response{StatusCode: http.StatusTooManyRequests, Header: http.Header{}, Body: io.NopCloser(strings.NewReader(body))}
	require.Error(t, svc.handleAntigravityCompatHTTPError(context.Background(), c, bare429Account(7), call, resp))
	require.True(t, cooledAccounts(repo)[7])
}
