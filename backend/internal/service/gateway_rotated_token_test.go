//go:build unit

package service

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

type rotatedClaudeTokenRepo struct {
	AccountRepository
	account       *Account
	err           error
	reads         int
	cooldownCalls int
	errorCalls    int
}

func (r *rotatedClaudeTokenRepo) GetByID(_ context.Context, id int64) (*Account, error) {
	r.reads++
	return r.account, r.err
}

func (r *rotatedClaudeTokenRepo) SetTempUnschedulable(context.Context, int64, time.Time, string) error {
	r.cooldownCalls++
	return nil
}

func (r *rotatedClaudeTokenRepo) SetError(context.Context, int64, string) error {
	r.errorCalls++
	return nil
}

type rotatedClaudeTokenBody struct {
	io.Reader
	closed bool
}

func (b *rotatedClaudeTokenBody) Close() error {
	b.closed = true
	return nil
}

func TestGatewayService_RotatedClaudeToken(t *testing.T) {
	gin.SetMode(gin.TestMode)
	for _, path := range []string{"messages", "chat/completions", "responses"} {
		for _, stream := range []bool{false, true} {
			for _, tc := range []struct {
				name           string
				snapshotToken  string
				storedToken    string
				lookupErr      error
				accountType    string
				inactive       bool
				unschedulable  bool
				cacheDeleteErr error
				rejectRetry    bool
				wantCalls      int
				wantCooldown   int
				wantError      int
			}{
				{name: "rotated", storedToken: "new-token", wantCalls: 2},
				{name: "cached_token_differs_from_snapshot", snapshotToken: "new-token", storedToken: "new-token", wantCalls: 2},
				{name: "unchanged", storedToken: "old-token", wantCalls: 1, wantCooldown: 1},
				{name: "missing", wantCalls: 1, wantCooldown: 1},
				{name: "lookup_failed", lookupErr: errors.New("database unavailable"), wantCalls: 1, wantCooldown: 1},
				{name: "retry_rejected", storedToken: "new-token", rejectRetry: true, wantCalls: 2, wantCooldown: 1},
				{name: "disabled_account", storedToken: "new-token", inactive: true, wantCalls: 1, wantCooldown: 1},
				{name: "unschedulable_account", storedToken: "new-token", unschedulable: true, wantCalls: 1, wantCooldown: 1},
				{name: "cache_delete_failed", storedToken: "new-token", cacheDeleteErr: errors.New("cache unavailable"), wantCalls: 2},
				{name: "setup_token", accountType: AccountTypeSetupToken, storedToken: "new-token", wantCalls: 1, wantError: 1},
				{name: "api_key", accountType: AccountTypeAPIKey, storedToken: "new-token", wantCalls: 1, wantError: 1},
			} {
				t.Run(fmt.Sprintf("%s/stream=%t/%s", path, stream, tc.name), func(t *testing.T) {
					snapshotToken := tc.snapshotToken
					if snapshotToken == "" {
						snapshotToken = "old-token"
					}
					account := &Account{
						ID: 71, Platform: PlatformAnthropic, Type: AccountTypeOAuth,
						Status: StatusActive, Schedulable: true, Concurrency: 1,
						Credentials: map[string]any{"access_token": snapshotToken, "refresh_token": "refresh-token", "api_key": "old-token"},
					}
					if tc.accountType != "" {
						account.Type = tc.accountType
					}
					latest := *account
					latest.Credentials = map[string]any{"access_token": tc.storedToken, "refresh_token": "new-refresh-token"}
					if tc.inactive {
						latest.Status = StatusError
					}
					if tc.unschedulable {
						latest.Schedulable = false
					}
					repo := &rotatedClaudeTokenRepo{account: account}
					cache := newClaudeTokenCacheStub()
					cache.deleteErr = tc.cacheDeleteErr
					require.NoError(t, cache.SetAccessToken(context.Background(), ClaudeTokenCacheKey(account), "old-token", time.Hour))
					cfg := &config.Config{Gateway: config.GatewayConfig{MaxLineSize: defaultMaxLineSize}}
					calls := 0
					var firstWireBody []byte
					var rejectedBodies []*rotatedClaudeTokenBody
					upstream := &systemOneHTTPUpstream{do: func(req *http.Request) (*http.Response, error) {
						calls++
						wireBody, err := io.ReadAll(req.Body)
						require.NoError(t, err)
						_ = req.Body.Close()
						if calls == 1 {
							if account.Type == AccountTypeAPIKey {
								require.Equal(t, "old-token", getHeaderRaw(req.Header, "x-api-key"))
							} else {
								require.Equal(t, "Bearer old-token", getHeaderRaw(req.Header, "authorization"))
							}
							firstWireBody = wireBody
							// The refresh completes only after the rejected request was sent.
							repo.account, repo.err = &latest, tc.lookupErr
						} else {
							require.Equal(t, 2, calls, "recover at most once, even if the token rotates again")
							require.Equal(t, "Bearer new-token", getHeaderRaw(req.Header, "authorization"))
							require.Equal(t, firstWireBody, wireBody, "replay the complete request")
							require.True(t, rejectedBodies[0].closed)
							latest.Credentials["access_token"] = "third-token"
						}
						if calls == 1 || tc.rejectRetry {
							body := &rotatedClaudeTokenBody{Reader: strings.NewReader(`{"type":"error","error":{"type":"authentication_error","message":"OAuth access token has been revoked"}}`)}
							rejectedBodies = append(rejectedBodies, body)
							return &http.Response{StatusCode: http.StatusUnauthorized, Header: make(http.Header), Body: body}, nil
						}
						payload := strings.Join([]string{
							"event: message_start",
							`data: {"type":"message_start","message":{"id":"msg_rotated","type":"message","role":"assistant","model":"claude-sonnet-4-6","content":[],"usage":{"input_tokens":11}}}`,
							"", "event: content_block_start",
							`data: {"type":"content_block_start","index":0,"content_block":{"type":"text","text":""}}`,
							"", "event: content_block_delta",
							`data: {"type":"content_block_delta","index":0,"delta":{"type":"text_delta","text":"hello"}}`,
							"", "event: content_block_stop",
							`data: {"type":"content_block_stop","index":0}`,
							"", "event: message_delta",
							`data: {"type":"message_delta","delta":{"stop_reason":"end_turn"},"usage":{"output_tokens":7}}`,
							"", "event: message_stop", `data: {"type":"message_stop"}`, "", "",
						}, "\n")
						contentType := "text/event-stream"
						if path == "messages" && !stream {
							payload = `{"id":"msg_rotated","type":"message","role":"assistant","model":"claude-sonnet-4-6","content":[{"type":"text","text":"hello"}],"stop_reason":"end_turn","usage":{"input_tokens":11,"output_tokens":7}}`
							contentType = "application/json"
						}
						return &http.Response{StatusCode: http.StatusOK, Header: http.Header{"Content-Type": []string{contentType}}, Body: io.NopCloser(strings.NewReader(payload))}, nil
					}}
					svc := &GatewayService{
						cfg: cfg, accountRepo: repo, httpUpstream: upstream,
						claudeTokenProvider: NewClaudeTokenProvider(repo, cache, nil),
						rateLimitService:    &RateLimitService{cfg: cfg, accountRepo: repo},
						deferredService:     &DeferredService{},
					}
					rec := httptest.NewRecorder()
					c, _ := gin.CreateTestContext(rec)
					c.Request = httptest.NewRequest(http.MethodPost, "/v1/"+path, nil)
					body := []byte(fmt.Sprintf(`{"model":"claude-sonnet-4-6","stream":%t,"max_tokens":32,"messages":[{"role":"user","content":"hello"}]}`, stream))
					var result *ForwardResult
					var err error
					switch path {
					case "messages":
						var parsed *ParsedRequest
						parsed, err = ParseGatewayRequest(NewRequestBodyRef(body), PlatformAnthropic)
						require.NoError(t, err)
						result, err = svc.Forward(context.Background(), c, account, parsed)
					case "chat/completions":
						result, err = svc.ForwardAsChatCompletions(context.Background(), c, account, body, nil)
					case "responses":
						body = []byte(fmt.Sprintf(`{"model":"claude-sonnet-4-6","stream":%t,"input":"hello"}`, stream))
						result, err = svc.ForwardAsResponses(context.Background(), c, account, body, nil)
					}
					if tc.wantCooldown == 0 && tc.wantError == 0 {
						require.NoError(t, err)
						require.NotNil(t, result)
						require.Equal(t, 11, result.Usage.InputTokens)
						require.Equal(t, 7, result.Usage.OutputTokens)
						require.Contains(t, rec.Body.String(), "hello")
						require.NotContains(t, rec.Body.String(), "revoked")
						cached, cacheErr := cache.GetAccessToken(context.Background(), ClaudeTokenCacheKey(account))
						require.NoError(t, cacheErr)
						if tc.cacheDeleteErr == nil {
							require.NotEqual(t, "old-token", cached)
						}
					} else {
						var failover *UpstreamFailoverError
						require.ErrorAs(t, err, &failover)
						require.Equal(t, http.StatusUnauthorized, failover.StatusCode)
						require.False(t, c.Writer.Written(), "leave the response available for account failover")
					}
					require.Equal(t, tc.wantCalls, calls)
					require.Equal(t, tc.wantCooldown, repo.cooldownCalls)
					require.Equal(t, tc.wantError, repo.errorCalls)
					require.Equal(t, snapshotToken, account.GetCredential("access_token"), "do not mutate the shared account snapshot")
					require.Equal(t, "new-refresh-token", latest.GetCredential("refresh_token"), "do not overwrite rotated credentials")
					for _, body := range rejectedBodies {
						require.True(t, body.closed)
					}
				})
			}
		}
	}
}
