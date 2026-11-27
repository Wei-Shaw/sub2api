//go:build unit

package handler

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/pkg/tlsfingerprint"
	middleware "github.com/Wei-Shaw/sub2api/internal/server/middleware"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

type stickyWritebackContextKey struct{}

type stickyWritebackCall struct {
	err         error
	groupID     int64
	account     int64
	session     string
	ttl         time.Duration
	value       any
	deadline    time.Time
	replaceable []int64
}

type stickyWritebackCache struct {
	service.GatewayCache
	accountID int64
	// readErr is returned by the first lookup only, then cleared: the binding
	// lookup at request start fails while later reads see the key again.
	readErr  error
	finished bool
	calls    []stickyWritebackCall
	lookups  []int64
}

func (s *stickyWritebackCache) GetSessionAccountID(context.Context, int64, string) (int64, error) {
	s.lookups = append(s.lookups, s.accountID)
	if s.readErr != nil {
		err := s.readErr
		s.readErr = nil
		return 0, err
	}
	return s.accountID, nil
}

func (s *stickyWritebackCache) record(ctx context.Context, groupID int64, session string, account int64, ttl time.Duration, replaceable []int64) {
	deadline, _ := ctx.Deadline()
	s.calls = append(s.calls, stickyWritebackCall{
		err: ctx.Err(), groupID: groupID, account: account, session: session, ttl: ttl,
		value: ctx.Value(stickyWritebackContextKey{}), deadline: deadline,
		replaceable: append([]int64(nil), replaceable...),
	})
}

func (s *stickyWritebackCache) SetSessionAccountID(ctx context.Context, groupID int64, session string, account int64, ttl time.Duration) error {
	if s.finished {
		s.record(ctx, groupID, session, account, ttl, nil)
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	s.accountID = account
	return nil
}

// CompareAndBindSessionAccountID mirrors the Redis compare-and-bind script: a
// key already holding the account only gets its TTL refreshed, a missing key or
// one holding an account this request failed over from is overwritten, anything
// else was rebound by another request and is left alone.
func (s *stickyWritebackCache) CompareAndBindSessionAccountID(ctx context.Context, groupID int64, session string, account int64, replaceable []int64, ttl time.Duration) (bool, error) {
	if err := ctx.Err(); err != nil {
		return false, err
	}
	if s.accountID == account {
		if s.finished {
			s.record(ctx, groupID, session, account, ttl, replaceable)
		}
		return true, nil
	}
	if s.accountID == 0 || slices.Contains(replaceable, s.accountID) {
		if s.finished {
			s.record(ctx, groupID, session, account, ttl, replaceable)
		}
		s.accountID = account
		return true, nil
	}
	return false, nil
}

func (s *stickyWritebackCache) RefreshSessionTTL(context.Context, int64, string, time.Duration) error {
	return nil
}

func (s *stickyWritebackCache) DeleteSessionAccountID(context.Context, int64, string) error {
	s.accountID = 0
	return nil
}

type stickyWritebackBody struct {
	io.Reader
	finish func()
}

func (b *stickyWritebackBody) Close() error {
	b.finish()
	return nil
}

// stickyWritebackUpstream answers per account with a freshly built response, so
// a failover row can fail on the first account and serve on the second.
type stickyWritebackUpstream struct {
	service.HTTPUpstream
	responses map[int64]func() *http.Response
	seen      []int64
}

func (u *stickyWritebackUpstream) DoWithTLS(_ *http.Request, _ string, accountID int64, _ int, _ *tlsfingerprint.Profile) (*http.Response, error) {
	u.seen = append(u.seen, accountID)
	build, ok := u.responses[accountID]
	if !ok {
		return nil, fmt.Errorf("no upstream response for account %d", accountID)
	}
	return build(), nil
}

func TestGatewayMessagesStickyWritebackAfterForward(t *testing.T) {
	gin.SetMode(gin.TestMode)
	for _, stream := range []bool{false, true} {
		name := "buffered"
		if stream {
			name = "streaming"
		}
		for _, tt := range []struct {
			name   string
			bound  int64
			cancel bool
			fail   bool
			// failover makes account 9401 fail with a 500 and account 9499 serve,
			// mirroring a request that switches accounts mid-flight.
			failover bool
			// profitGate installs the group profit gate, which makes the request
			// write the admitted account eagerly and skip a different binding.
			profitGate bool
			// readErr fails the binding lookup at request start.
			readErr bool
			during  func(*stickyWritebackCache)
			// rebindOnServe runs during on the serving forward instead of the
			// failing one.
			rebindOnServe bool
			wantSeen      []int64
			// wantWrites counts the terminal writes; reads and admission-time
			// writes made before the forward are not counted.
			wantWrites       int
			wantFinal        int64
			checkFinal       bool
			wantSecondLookup int64
		}{
			{name: "new binding", wantWrites: 1},
			{name: "new binding after disconnect", cancel: true, wantWrites: 1},
			{name: "refresh binding", bound: 9401, wantWrites: 1},
			{name: "refresh after disconnect", bound: 9401, cancel: true, wantWrites: 1},
			{name: "different binding", bound: 9499, cancel: true},
			{name: "forward failed", cancel: true, fail: true},
			{
				name: "rebound during forward", bound: 9401, checkFinal: true, wantFinal: 9499, wantSecondLookup: 9499,
				during: func(c *stickyWritebackCache) { c.accountID = 9499 },
			},
			{
				name: "bound by another request during forward", checkFinal: true, wantFinal: 9499, wantSecondLookup: 9499,
				during: func(c *stickyWritebackCache) { c.accountID = 9499 },
			},
			{
				name: "deleted during forward", bound: 9401, wantWrites: 1, checkFinal: true, wantFinal: 9401,
				during: func(c *stickyWritebackCache) { c.accountID = 0 },
			},
			{
				name: "rebound away and back during forward", bound: 9401, wantWrites: 1, checkFinal: true, wantFinal: 9401,
				during: func(c *stickyWritebackCache) { c.accountID = 9499; c.accountID = 9401 },
			},
			{
				// K1: the key starts missing, the request writes the admitted
				// account A, A fails over to B. The key must end on B instead of
				// keeping the account that just failed.
				name: "profit gate missing binding failover", failover: true, profitGate: true,
				wantSeen: []int64{9401, 9499}, wantWrites: 1, checkFinal: true, wantFinal: 9499, wantSecondLookup: 9499,
			},
			{
				name: "profit gate failed start lookup failover", failover: true, profitGate: true, readErr: true,
				wantSeen: []int64{9401, 9499}, wantWrites: 1, checkFinal: true, wantFinal: 9499, wantSecondLookup: 9499,
			},
			{
				name: "profit gate bound to the failed account", bound: 9401, failover: true, profitGate: true,
				wantSeen: []int64{9401, 9499}, wantWrites: 0, checkFinal: true, wantFinal: 9401, wantSecondLookup: 9401,
			},
			{
				// Another request rebinds to a third account while the failover
				// already serves B: the third account was never tried here, so the
				// terminal write leaves it alone and writes nothing.
				name: "profit gate rebound by another request during failover", failover: true, profitGate: true, rebindOnServe: true,
				wantSeen: []int64{9401, 9499}, checkFinal: true, wantFinal: 9455,
				during: func(c *stickyWritebackCache) { c.accountID = 9455 },
			},
			{
				// Without a profit gate the scheduler binds before every forward,
				// so a failed account found in the key was bound by another request
				// and is kept.
				name: "rebound to the failed account during failover", failover: true, rebindOnServe: true,
				wantSeen: []int64{9401, 9499}, checkFinal: true, wantFinal: 9401,
				during: func(c *stickyWritebackCache) { c.accountID = 9401 },
			},
		} {
			t.Run(name+"/"+tt.name, func(t *testing.T) {
				groupID := int64(9400)
				group := &service.Group{
					ID: groupID, Hydrated: true, Platform: service.PlatformAnthropic, Status: service.StatusActive,
					RateMultiplier: 1, SubscriptionType: service.SubscriptionTypeStandard,
					ProfitControlEnabled: tt.profitGate, ProfitMinMargin: 0.5,
				}
				newAccount := func(id int64, priority int, schedulable bool) *service.Account {
					rate := 0.1
					return &service.Account{
						ID: id, Platform: service.PlatformAnthropic, Type: service.AccountTypeAPIKey,
						Status: service.StatusActive, Schedulable: schedulable, Concurrency: 1, Priority: priority,
						RateMultiplier: &rate,
						Credentials:    map[string]any{"api_key": "test-key"},
						Extra:          map[string]any{"anthropic_passthrough": true},
						AccountGroups:  []service.AccountGroup{{AccountID: id, GroupID: groupID}},
					}
				}
				account := newAccount(9401, 0, true)
				other := newAccount(9499, 1, tt.failover)
				// third is only bound by the other request; it only has to resolve
				// as a known account.
				third := newAccount(9455, 2, true)
				servedID := account.ID
				if tt.failover {
					servedID = other.ID
				}
				wantCalls := 1
				if tt.failover {
					wantCalls = 2
				}
				cache := &stickyWritebackCache{accountID: tt.bound}
				if tt.readErr {
					cache.readErr = fmt.Errorf("sticky cache read failed")
				}
				ctx := context.WithValue(context.Background(), stickyWritebackContextKey{}, "request-scoped value")
				ctx, cancel := context.WithTimeout(ctx, 4*time.Second)
				t.Cleanup(cancel)
				responseBody := `{"id":"msg_1","type":"message","role":"assistant","model":"claude-sonnet-4.5","content":[{"type":"text","text":"ok"}],"stop_reason":"end_turn","usage":{"input_tokens":1,"output_tokens":1}}`
				contentType := "application/json"
				if stream {
					responseBody = compatStreamUsagePartialStream + compatStreamUsageCompletion
					contentType = "text/event-stream"
				}
				status := http.StatusOK
				if tt.fail {
					status = http.StatusBadRequest
					responseBody = `{"type":"error","error":{"type":"invalid_request_error","message":"invalid input"}}`
				}
				duringRan := false
				runDuring := func() {
					if duringRan || tt.during == nil {
						return
					}
					duringRan = true
					tt.during(cache)
				}
				finish := func() {
					// Forward closes its upstream body on return. Bound the concurrent
					// rebind and cancel precisely after that work, before the handler's
					// terminal sticky write.
					cache.finished = true
					runDuring()
					if tt.cancel {
						cancel()
					}
				}
				respond := func(status int, contentType, body string, bodyFinish func()) func() *http.Response {
					return func() *http.Response {
						return &http.Response{
							StatusCode: status,
							Header:     http.Header{"Content-Type": []string{contentType}},
							Body:       &stickyWritebackBody{Reader: strings.NewReader(body), finish: bodyFinish},
						}
					}
				}
				responses := map[int64]func() *http.Response{}
				if tt.failover {
					// By default the failing attempt's body closes before the next
					// account is selected, so a rebind there sits inside the
					// failover; rebindOnServe moves it into the serving forward.
					failingFinish := runDuring
					if tt.rebindOnServe {
						failingFinish = func() {}
					}
					responses[account.ID] = respond(http.StatusInternalServerError, "application/json", `{"type":"error","error":{"type":"api_error","message":"upstream boom"}}`, failingFinish)
					responses[other.ID] = respond(status, contentType, responseBody, finish)
				} else {
					responses[account.ID] = respond(status, contentType, responseBody, finish)
				}
				upstream := &stickyWritebackUpstream{responses: responses}
				cfg := &config.Config{RunMode: config.RunModeSimple}
				snapshot := service.NewSchedulerSnapshotService(&fakeSchedulerCache{accounts: []*service.Account{account, other, third}}, nil, nil, nil, nil)
				gw := service.NewGatewayService(
					nil, &fakeGroupRepo{group: group}, nil, nil, nil, nil, nil, cache, cfg,
					snapshot, nil, service.NewBillingService(cfg, nil), nil, nil, nil, upstream,
					&service.DeferredService{}, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil,
				)
				billingCache := service.NewBillingCacheService(nil, nil, nil, nil, nil, nil, cfg, nil)
				t.Cleanup(billingCache.Stop)
				h := &GatewayHandler{
					gatewayService: gw, billingCacheService: billingCache, cfg: cfg, maxAccountSwitches: 1,
					concurrencyHelper: NewConcurrencyHelper(service.NewConcurrencyService(&fakeConcurrencyCache{}), SSEPingFormatClaude, 0),
				}
				requestBody := `{"model":"claude-sonnet-4.5","max_tokens":16,"stream":false,"messages":[{"role":"user","content":"sticky writeback test"}]}`
				if stream {
					requestBody = strings.Replace(requestBody, `"stream":false`, `"stream":true`, 1)
				}
				rec := httptest.NewRecorder()
				c, _ := gin.CreateTestContext(rec)
				c.Request = httptest.NewRequest(http.MethodPost, "/v1/messages", strings.NewReader(requestBody)).WithContext(ctx)
				c.Request.Header.Set("Content-Type", "application/json")
				key := &service.APIKey{ID: 9402, UserID: 9403, GroupID: &groupID, Group: group, User: &service.User{ID: 9403, Balance: 100}}
				c.Set(string(middleware.ContextKeyAPIKey), key)
				c.Set(string(middleware.ContextKeyUser), middleware.AuthSubject{UserID: 9403})

				h.Messages(c)

				require.Equal(t, wantCalls, len(upstream.seen))
				if tt.wantSeen != nil {
					require.Equal(t, tt.wantSeen, upstream.seen)
				} else {
					require.Equal(t, servedID, upstream.seen[len(upstream.seen)-1])
				}
				require.True(t, cache.finished)
				require.Equal(t, status, rec.Code)
				if tt.cancel {
					require.ErrorIs(t, ctx.Err(), context.Canceled)
				}
				require.Len(t, cache.calls, tt.wantWrites)
				for _, call := range cache.calls {
					require.NoError(t, call.err, "a completed upstream request must retain its terminal sticky write")
					require.Equal(t, groupID, call.groupID)
					require.Equal(t, servedID, call.account)
					require.NotEmpty(t, call.session)
					require.Positive(t, call.ttl)
					require.Equal(t, "request-scoped value", call.value)
					require.False(t, call.deadline.IsZero(), "the detached cache write must have a deadline")
					require.WithinDuration(t, time.Now(), call.deadline, 5*time.Second)
				}
				if tt.checkFinal {
					require.Equal(t, tt.wantFinal, cache.accountID, "the terminal write must only cover values this request may have written")
				}
				if tt.wantSecondLookup != 0 {
					nextLookup := len(cache.lookups)
					rec2 := httptest.NewRecorder()
					c2, _ := gin.CreateTestContext(rec2)
					c2.Request = httptest.NewRequest(http.MethodPost, "/v1/messages", strings.NewReader(requestBody)).WithContext(context.WithValue(context.Background(), stickyWritebackContextKey{}, "request-scoped value"))
					c2.Request.Header.Set("Content-Type", "application/json")
					c2.Set(string(middleware.ContextKeyAPIKey), key)
					c2.Set(string(middleware.ContextKeyUser), middleware.AuthSubject{UserID: 9403})

					h.Messages(c2)

					require.Equal(t, status, rec2.Code)
					require.Equal(t, tt.wantSecondLookup, cache.lookups[nextLookup], "a later request must observe the terminal binding")
				}
			})
		}
	}
}
