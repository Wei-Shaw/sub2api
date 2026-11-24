//go:build unit

package service

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/antigravity"
	"github.com/Wei-Shaw/sub2api/internal/pkg/tlsfingerprint"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

// fixed429Upstream answers every call with one 429 body.
type fixed429Upstream struct {
	body  string
	calls int
}

func (u *fixed429Upstream) Do(req *http.Request, proxyURL string, accountID int64, accountConcurrency int) (*http.Response, error) {
	u.calls++
	return &http.Response{
		StatusCode: http.StatusTooManyRequests,
		Header:     http.Header{},
		Body:       io.NopCloser(strings.NewReader(u.body)),
	}, nil
}

func (u *fixed429Upstream) DoWithTLS(req *http.Request, proxyURL string, accountID int64, accountConcurrency int, profile *tlsfingerprint.Profile) (*http.Response, error) {
	return u.Do(req, proxyURL, accountID, accountConcurrency)
}

// The body Google sent on 2026-11-16 for one 1.8 MB request on every account it
// reached, while the same accounts served other requests: no reason, no delay.
const bareAntigravity429Body = `{"error":{"code":429,"message":"Resource has been exhausted (e.g. check quota).","status":"RESOURCE_EXHAUSTED"}}`

func runBare429Loop(t *testing.T, c *gin.Context, account *Account, body string) (cooled bool) {
	t.Helper()
	svc := &AntigravityGatewayService{}
	result, err := svc.antigravityRetryLoop(antigravityRetryLoopParams{
		prefix:         "[test]",
		ctx:            context.Background(),
		account:        account,
		accessToken:    "token",
		action:         "streamGenerateContent",
		body:           []byte(`{"input":"test"}`),
		c:              c,
		httpUpstream:   &fixed429Upstream{body: body},
		requestedModel: "gemini-4-argon-high",
		handleError: func(ctx context.Context, prefix string, account *Account, statusCode int, headers http.Header, body []byte, requestedModel string, groupID int64, sessionHash string, isStickySession bool) *handleModelRateLimitResult {
			cooled = true
			return nil
		},
	})
	require.NoError(t, err)
	require.NotNil(t, result)
	require.NotNil(t, result.resp)
	_ = result.resp.Body.Close()
	require.Equal(t, http.StatusTooManyRequests, result.resp.StatusCode)
	return cooled
}

func setupBare429Test(t *testing.T) {
	t.Helper()
	t.Setenv(antigravityForwardBaseURLEnv, "")
	oldBaseURLs := append([]string(nil), antigravity.BaseURLs...)
	oldAvailability := antigravity.DefaultURLAvailability
	t.Cleanup(func() {
		antigravity.BaseURLs = oldBaseURLs
		antigravity.DefaultURLAvailability = oldAvailability
	})
	antigravity.BaseURLs = []string{"https://ag-1.test"}
	antigravity.DefaultURLAvailability = antigravity.NewURLAvailability(time.Minute)
}

func newBare429TestContext() *gin.Context {
	gin.SetMode(gin.TestMode)
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", nil)
	return c
}

func bare429Account(id int64) *Account {
	return &Account{ID: id, Name: "acc", Platform: PlatformAntigravity, Schedulable: true, Status: StatusActive, Concurrency: 1}
}

// One request that gets a bare 429 on every account it reaches must not cool
// them all: it cools the first, and the rest are only skipped for this request.
func TestAntigravityRetryLoop_Bare429_CoolsOnlyTheFirstAccountPerRequest(t *testing.T) {
	setupBare429Test(t)
	c := newBare429TestContext()

	require.True(t, runBare429Loop(t, c, bare429Account(13), bareAntigravity429Body), "first account is cooled")
	require.False(t, runBare429Loop(t, c, bare429Account(6), bareAntigravity429Body), "same request on a second account: not cooled")
	require.False(t, runBare429Loop(t, c, bare429Account(2), bareAntigravity429Body), "same request on a third account: not cooled")
	require.True(t, runBare429Loop(t, c, bare429Account(13), bareAntigravity429Body), "the account it already cooled may be cooled again")
}

// Another request that reaches the second account first still cools it, so an
// account that really is throttled for everyone is still taken out.
func TestAntigravityRetryLoop_Bare429_OtherRequestStillCools(t *testing.T) {
	setupBare429Test(t)

	require.True(t, runBare429Loop(t, newBare429TestContext(), bare429Account(13), bareAntigravity429Body))
	require.True(t, runBare429Loop(t, newBare429TestContext(), bare429Account(6), bareAntigravity429Body))
}

// A 429 that says when it resets is Google's statement about the account and is
// applied on every account, as before.
func TestAntigravityRetryLoop_429WithResetDelay_CoolsEveryAccount(t *testing.T) {
	setupBare429Test(t)
	c := newBare429TestContext()
	body := `{"error":{"code":429,"message":"You have exhausted your capacity on this model. Your quota will reset after 2h.","status":"RESOURCE_EXHAUSTED","details":[{"@type":"type.googleapis.com/google.rpc.ErrorInfo","reason":"QUOTA_EXHAUSTED","metadata":{"quotaResetDelay":"2h0m0s"}}]}}`

	require.True(t, runBare429Loop(t, c, bare429Account(13), body))
	require.True(t, runBare429Loop(t, c, bare429Account(6), body))
}
