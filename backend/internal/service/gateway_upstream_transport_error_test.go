//go:build unit

package service

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
)

type transportTempUnschedRepoStub struct {
	AccountRepository
	calls      int
	lastID     int64
	lastUntil  time.Time
	lastReason string
}

func (r *transportTempUnschedRepoStub) SetTempUnschedulable(_ context.Context, id int64, until time.Time, reason string) error {
	r.calls++
	r.lastID = id
	r.lastUntil = until
	r.lastReason = reason
	return nil
}

func newTransportErrorTestGin(t *testing.T) *gin.Context {
	t.Helper()
	gin.SetMode(gin.TestMode)
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/messages", nil)
	return c
}

// TestHandleUpstreamTransportError_TransientFailsOverWithoutEviction pins the
// contract for transient transport blips (EOF / connection reset): the request
// fails over to another account, the current account stays schedulable, and
// nothing is written to the response (the handler owns it).
func TestHandleUpstreamTransportError_TransientFailsOverWithoutEviction(t *testing.T) {
	repo := &transportTempUnschedRepoStub{}
	s := &GatewayService{accountRepo: repo}
	c := newTransportErrorTestGin(t)
	account := &Account{ID: 149, Name: "acc", Platform: PlatformAnthropic}

	err := s.handleUpstreamTransportError(context.Background(), c, account,
		errors.New(`Post "http://upstream/v1/messages?beta=true": EOF`), OpsUpstreamErrorEvent{})

	var failoverErr *UpstreamFailoverError
	if !errors.As(err, &failoverErr) {
		t.Fatalf("expected *UpstreamFailoverError, got %T: %v", err, err)
	}
	if failoverErr.StatusCode != http.StatusBadGateway {
		t.Fatalf("StatusCode = %d, want 502", failoverErr.StatusCode)
	}
	if string(failoverErr.ResponseBody) != string(gatewayTransportFailoverBody) {
		t.Fatalf("ResponseBody = %s, want legacy 502 body", failoverErr.ResponseBody)
	}
	if !failoverErr.ShouldRetryNextAccount() {
		t.Fatal("transient transport error must allow retrying the next account")
	}
	if repo.calls != 0 {
		t.Fatalf("SetTempUnschedulable called %d times for a transient error, want 0", repo.calls)
	}
	if c.Writer.Written() {
		t.Fatal("handler owns the response; service must not write on transport failover")
	}
}

// TestHandleUpstreamTransportError_AttributesEventTimeProxy pins that the
// shared helper stamps proxy attribution from the same account snapshot the
// transport used, so every Anthropic/Bedrock caller inherits it.
func TestHandleUpstreamTransportError_AttributesEventTimeProxy(t *testing.T) {
	proxyID := int64(10060)
	tests := []struct {
		name     string
		account  *Account
		wantID   *int64
		wantName string
	}{
		{
			name:     "managed proxy",
			account:  &Account{ID: 149, Name: "acc", Platform: PlatformAnthropic, ProxyID: &proxyID, Proxy: &Proxy{ID: proxyID, Name: "wldsg82-ipv6-10060"}},
			wantID:   &proxyID,
			wantName: "wldsg82-ipv6-10060",
		},
		{
			name:     "direct",
			account:  &Account{ID: 149, Name: "acc", Platform: PlatformAnthropic},
			wantName: opsProxyNameDirect,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := &GatewayService{accountRepo: &transportTempUnschedRepoStub{}}
			c := newTransportErrorTestGin(t)
			_ = s.handleUpstreamTransportError(context.Background(), c, tt.account,
				errors.New("EOF"), OpsUpstreamErrorEvent{UpstreamURL: "https://api.anthropic.com/v1/messages", Passthrough: true})
			raw, ok := c.Get(OpsUpstreamErrorsKey)
			if !ok {
				t.Fatal("no upstream events recorded")
			}
			events := raw.([]*OpsUpstreamErrorEvent)
			if len(events) != 1 {
				t.Fatalf("events = %d, want 1", len(events))
			}
			ev := events[0]
			if tt.wantID == nil {
				if ev.ProxyID != nil {
					t.Fatalf("proxy_id = %d, want null", *ev.ProxyID)
				}
			} else if ev.ProxyID == nil || *ev.ProxyID != *tt.wantID {
				t.Fatalf("proxy_id = %v, want %d", ev.ProxyID, *tt.wantID)
			}
			if ev.ProxyName != tt.wantName {
				t.Fatalf("proxy_name = %q, want %q", ev.ProxyName, tt.wantName)
			}
			if !ev.Passthrough || ev.UpstreamURL == "" || ev.Kind != "request_error" {
				t.Fatalf("caller-supplied fields lost: %+v", ev)
			}
		})
	}
}

// TestHandleUpstreamTransportError_PersistentEvictsAccount pins the contract
// for durable faults (dead endpoint / DNS / proxy credentials): fail over AND
// temporarily unschedule the account for the transport cooldown.
func TestHandleUpstreamTransportError_PersistentEvictsAccount(t *testing.T) {
	repo := &transportTempUnschedRepoStub{}
	s := &GatewayService{accountRepo: repo}
	c := newTransportErrorTestGin(t)
	account := &Account{ID: 149, Name: "acc", Platform: PlatformAnthropic}

	before := time.Now()
	err := s.handleUpstreamTransportError(context.Background(), c, account,
		errors.New(`dial tcp 1.2.3.4:443: connect: connection refused`), OpsUpstreamErrorEvent{})

	var failoverErr *UpstreamFailoverError
	if !errors.As(err, &failoverErr) {
		t.Fatalf("expected *UpstreamFailoverError, got %T: %v", err, err)
	}
	if repo.calls != 1 {
		t.Fatalf("SetTempUnschedulable called %d times for a persistent error, want 1", repo.calls)
	}
	if repo.lastID != account.ID {
		t.Fatalf("unscheduled account = %d, want %d", repo.lastID, account.ID)
	}
	wantUntil := before.Add(gatewayTransportErrorTempUnschedDuration)
	if repo.lastUntil.Before(wantUntil.Add(-time.Minute)) || repo.lastUntil.After(wantUntil.Add(time.Minute)) {
		t.Fatalf("until = %v, want ~%v", repo.lastUntil, wantUntil)
	}
	if !strings.HasPrefix(repo.lastReason, "upstream transport error (proxy/network): ") {
		t.Fatalf("reason = %q, want transport-error prefix", repo.lastReason)
	}
}

// TestHandleUpstreamTransportError_ClientCanceledNoFailover pins that a
// canceled client neither fails over nor evicts: the upstream never had a
// chance to exhibit a fault.
func TestHandleUpstreamTransportError_ClientCanceledNoFailover(t *testing.T) {
	repo := &transportTempUnschedRepoStub{}
	s := &GatewayService{accountRepo: repo}
	c := newTransportErrorTestGin(t)
	account := &Account{ID: 149, Name: "acc", Platform: PlatformAnthropic}

	inErr := context.Canceled
	err := s.handleUpstreamTransportError(context.Background(), c, account, inErr, OpsUpstreamErrorEvent{})

	var failoverErr *UpstreamFailoverError
	if errors.As(err, &failoverErr) {
		t.Fatal("canceled client must not fail over to another account")
	}
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("err = %v, want context.Canceled passthrough", err)
	}
	if repo.calls != 0 {
		t.Fatalf("SetTempUnschedulable called %d times on client cancel, want 0", repo.calls)
	}
}

// TestHandleUpstreamTransportError_UpstreamDeadlineStillFailsOver pins that an
// upstream-side timeout (request context still alive) is treated as a
// transient fault: fail over, no eviction.
func TestHandleUpstreamTransportError_UpstreamDeadlineStillFailsOver(t *testing.T) {
	repo := &transportTempUnschedRepoStub{}
	s := &GatewayService{accountRepo: repo}
	c := newTransportErrorTestGin(t)
	account := &Account{ID: 149, Name: "acc", Platform: PlatformAnthropic}

	err := s.handleUpstreamTransportError(context.Background(), c, account,
		context.DeadlineExceeded, OpsUpstreamErrorEvent{})

	var failoverErr *UpstreamFailoverError
	if !errors.As(err, &failoverErr) {
		t.Fatalf("upstream deadline with live request context must fail over, got %T: %v", err, err)
	}
	if repo.calls != 0 {
		t.Fatalf("SetTempUnschedulable called %d times for upstream deadline, want 0", repo.calls)
	}
}
