//go:build unit

package service

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"testing"
	"testing/synctest"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/stretchr/testify/require"
)

func TestOpenAIWSHTTPBridgeTransportFailure_LegacyPolicyAllTurns(t *testing.T) {
	for _, tc := range openAITransportLegacyPolicyCases() {
		for _, accountType := range []string{AccountTypeAPIKey, AccountTypeOAuth} {
			for _, turn := range []int{1, 2} {
				t.Run(fmt.Sprintf("%s/%s/turn=%d", tc.name, accountType, turn), func(t *testing.T) {
					repo := &openaiTransportAccountRepoStub{}
					upstream := &httpUpstreamRecorder{err: tc.cause}
					svc := &OpenAIGatewayService{
						cfg: &config.Config{}, httpUpstream: upstream, accountRepo: repo,
					}
					account := &Account{ID: 720, Platform: PlatformOpenAI, Type: accountType, Concurrency: 1}
					c, rec := newOpenAITransportErrTestContext()
					payload := []byte(`{"type":"response.create","model":"gpt-5","input":"hi"}`)
					writes := 0
					before := time.Now()
					_, err := svc.proxyOpenAIWSHTTPBridgeTurn(
						context.Background(), c, account, "test-token", payload, len(payload),
						"gpt-5", "", "", "", "", turn,
						func([]byte) error { writes++; return nil },
					)
					after := time.Now()
					var failover *UpstreamFailoverError
					require.ErrorAs(t, err, &failover)
					require.Equal(t, http.StatusBadGateway, failover.StatusCode)
					require.True(t, failover.ShouldRetryNextAccount())
					require.False(t, failover.RetryableOnSameAccount)
					require.Zero(t, writes)
					require.Zero(t, rec.Body.Len())
					require.Len(t, upstream.requests, 1)
					require.Equal(t, tc.block, svc.isOpenAIAccountRuntimeBlocked(account))
					if !tc.block {
						require.Empty(t, repo.tempUnschedCalls)
						return
					}
					require.Len(t, repo.tempUnschedCalls, 1)
					until := repo.tempUnschedCalls[0].until
					require.False(t, until.Before(before.Add(10*time.Minute)))
					require.False(t, until.After(after.Add(10*time.Minute)))
				})
			}
		}
	}
}

type openAIWSBridgeEndedSessionUpstream struct {
	*httpUpstreamRecorder
	onRequest func(*http.Request)
}

func (u *openAIWSBridgeEndedSessionUpstream) Do(req *http.Request, proxyURL string, accountID int64, concurrency int) (*http.Response, error) {
	u.onRequest(req)
	return u.httpUpstreamRecorder.Do(req, proxyURL, accountID, concurrency)
}

func TestOpenAIWSHTTPBridgeTransportFailure_EndedSessionDoesNotFailoverOrCooldown(t *testing.T) {
	for _, accountType := range []string{AccountTypeAPIKey, AccountTypeOAuth} {
		for _, turn := range []int{1, 2} {
			for _, ending := range []string{"cancel", "preempt", "preempt_before_cancel", "deadline"} {
				t.Run(fmt.Sprintf("%s/turn=%d/%s", accountType, turn, ending), func(t *testing.T) {
					synctest.Test(t, func(t *testing.T) {
						ctx, cancel := context.WithCancelCause(context.Background())
						defer cancel(nil)
						wantErr := error(context.Canceled)
						endSession := func() { cancel(nil) }
						switch ending {
						case "preempt":
							wantErr = errOpenAIWSSessionPreempted
							endSession = func() { cancel(errOpenAIWSSessionPreempted) }
						case "preempt_before_cancel":
							state := &openAIWSSessionPreemptState{}
							ctx = context.WithValue(ctx, openAIWSSessionPreemptContextKey{}, state)
							wantErr = errOpenAIWSSessionPreempted
							endSession = func() { state.preempted.Store(true) }
						case "deadline":
							var stop context.CancelFunc
							ctx, stop = context.WithTimeout(ctx, time.Second)
							defer stop()
							wantErr = context.DeadlineExceeded
							endSession = func() { <-ctx.Done() }
						}
						repo := &openaiTransportAccountRepoStub{}
						upstream := &openAIWSBridgeEndedSessionUpstream{
							httpUpstreamRecorder: &httpUpstreamRecorder{err: errors.New("proxy authentication required")},
							onRequest: func(req *http.Request) {
								require.NoError(t, ctx.Err(), "WS session must be active when the request starts")
								require.False(t, isOpenAIWSSessionPreempted(ctx))
								require.NoError(t, req.Context().Err())
								endSession()
								require.NoError(t, req.Context().Err(), "detached upstream must remain active after the WS session ends")
								if ending == "preempt" || ending == "preempt_before_cancel" {
									require.True(t, isOpenAIWSSessionPreempted(ctx))
								} else {
									require.ErrorIs(t, ctx.Err(), wantErr)
								}
							},
						}
						svc := &OpenAIGatewayService{cfg: &config.Config{}, httpUpstream: upstream, accountRepo: repo}
						account := &Account{ID: 721, Platform: PlatformOpenAI, Type: accountType, Concurrency: 1}
						c, rec := newOpenAITransportErrTestContext()
						payload := []byte(`{"type":"response.create","model":"gpt-5","input":"hi"}`)
						writes := 0
						_, err := svc.proxyOpenAIWSHTTPBridgeTurn(
							ctx, c, account, "test-token", payload, len(payload), "gpt-5", "", "", "", "", turn,
							func([]byte) error { writes++; return nil },
						)
						var failover *UpstreamFailoverError
						require.False(t, errors.As(err, &failover), "ended WS session must not request account failover")
						require.ErrorIs(t, err, wantErr)
						require.Zero(t, writes)
						require.Zero(t, rec.Body.Len())
						require.Len(t, upstream.requests, 1)
						require.False(t, svc.isOpenAIAccountRuntimeBlocked(account))
						require.Empty(t, repo.tempUnschedCalls)
					})
				})
			}
		}
	}
}
