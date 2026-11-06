//go:build unit

package handler

import (
	"context"
	"encoding/json"
	"errors"
	"github.com/Wei-Shaw/sub2api/internal/pkg/adminaccess"
	"math"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
)

type quotaSettingsStub struct {
	config service.UpstreamQuotaSettings
	err    error
}

func (s *quotaSettingsStub) GetUpstreamQuotaSettings(context.Context) (*service.UpstreamQuotaSettings, error) {
	return &s.config, s.err
}
func (s *quotaSettingsStub) SetUpstreamQuotaSettings(_ context.Context, c *service.UpstreamQuotaSettings) error {
	s.config = *c
	return nil
}

type quotaAccountsStub struct {
	service.AccountRepository
	items []*service.Account
	err   error
	mu    sync.Mutex
	calls int
}

func (s *quotaAccountsStub) GetByIDs(_ context.Context, ids []int64) ([]*service.Account, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.calls++
	if s.err != nil {
		return nil, s.err
	}
	var out []*service.Account
	for _, a := range s.items {
		for _, id := range ids {
			if a.ID == id {
				out = append(out, a)
			}
		}
	}
	return out, nil
}

type quotaUsageStub struct {
	mu    sync.Mutex
	calls int
	fail  bool
}

func (s *quotaUsageStub) GetUsageForAccount(_ context.Context, a *service.Account, _ ...bool) (*service.UsageInfo, error) {
	s.mu.Lock()
	s.calls++
	s.mu.Unlock()
	if s.fail {
		return nil, errors.New("secret-token provider failure")
	}
	reset := time.Now().Add(time.Hour)
	return &service.UsageInfo{FiveHour: &service.UsageProgress{Utilization: 20, ResetsAt: &reset}, ValidationURL: "https://private.invalid"}, nil
}

type quotaConcurrencyStub struct{}

func (*quotaConcurrencyStub) GetAccountConcurrencyBatch(context.Context, []int64) (map[int64]int, error) {
	return map[int64]int{1: 2}, nil
}
func quotaFixture() (*UpstreamQuotaHandler, *quotaSettingsStub, *quotaAccountsStub, *quotaUsageStub) {
	s := &quotaSettingsStub{config: service.UpstreamQuotaSettings{Enabled: true, AccountIDs: []int64{1, 2}}}
	a := &quotaAccountsStub{items: []*service.Account{{ID: 1, Name: "private@example.com", Platform: "openai", Status: "active", Schedulable: true, Concurrency: 5, Credentials: map[string]any{"access_token": "secret-token"}}, {ID: 2, Platform: "openai", Status: "disabled", Schedulable: false}, {ID: 3, Name: "not-shared", Platform: "openai", Status: "active", Schedulable: true}}}
	u := &quotaUsageStub{}
	return newUpstreamQuotaHandler(s, a, u, &quotaConcurrencyStub{}), s, a, u
}
func quotaRequest(h *UpstreamQuotaHandler, query string) *httptest.ResponseRecorder {
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest("GET", "/upstream-quota"+query, nil)
	h.Snapshot(c)
	return w
}
func TestUpstreamQuotaDisabledFailsClosed(t *testing.T) {
	h, s, a, u := quotaFixture()
	s.config.Enabled = false
	w := quotaRequest(h, "")
	if w.Code != 404 || a.calls != 0 || u.calls != 0 {
		t.Fatalf("disabled code=%d accounts=%d usage=%d", w.Code, a.calls, u.calls)
	}
	s.config.Enabled = true
	s.err = errors.New("settings unavailable")
	if quotaRequest(h, "").Code == 200 {
		t.Fatal("settings error allowed access")
	}
}
func TestUpstreamQuotaAllowlistRedactionAndDisabled(t *testing.T) {
	h, _, _, u := quotaFixture()
	w := quotaRequest(h, "?platform=openai")
	if w.Code != 200 {
		t.Fatal(w.Body.String())
	}
	body := w.Body.String()
	for _, secret := range []string{"private@example.com", "secret-token", "credentials", "ValidationURL", "private.invalid", "not-shared"} {
		if strings.Contains(body, secret) {
			t.Fatalf("leaked %q", secret)
		}
	}
	var x struct {
		Data struct {
			Accounts []struct {
				ID      int64
				Status  string
				Windows []any
			}
		}
	}
	if err := json.Unmarshal(w.Body.Bytes(), &x); err != nil {
		t.Fatal(err)
	}
	if len(x.Data.Accounts) != 2 || u.calls != 1 {
		t.Fatalf("allowlist/probes: %s calls=%d", body, u.calls)
	}
	if x.Data.Accounts[1].Status != "disabled" || len(x.Data.Accounts[1].Windows) != 0 {
		t.Fatal("disabled quota exposed")
	}
}
func TestUpstreamQuotaSharedCacheAndRevocation(t *testing.T) {
	h, s, _, u := quotaFixture()
	var wg sync.WaitGroup
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if quotaRequest(h, "").Code != 200 {
				t.Error("request failed")
			}
		}()
	}
	wg.Wait()
	if u.calls != 1 {
		t.Fatalf("shared cache made %d calls", u.calls)
	}
	s.config.AccountIDs = []int64{2}
	w := quotaRequest(h, "")
	if strings.Contains(w.Body.String(), `"id":1,`) {
		t.Fatal("removed account remained cached")
	}
	s.config.Enabled = false
	if quotaRequest(h, "").Code != 404 {
		t.Fatal("cached data bypassed disable")
	}
}
func TestUpstreamQuotaFailureDoesNotInventBalance(t *testing.T) {
	h, _, _, u := quotaFixture()
	u.fail = true
	w := quotaRequest(h, "")
	if w.Code != 200 || !strings.Contains(w.Body.String(), `"quota_state":"unknown"`) || strings.Contains(w.Body.String(), "secret-token") {
		t.Fatal(w.Body.String())
	}
}
func TestUpstreamQuotaRejectsUnknownPlatform(t *testing.T) {
	h, _, a, _ := quotaFixture()
	if quotaRequest(h, "?platform=arbitrary").Code != 400 || a.calls != 0 {
		t.Fatal("invalid platform queried")
	}
}

func TestUpstreamQuotaSanitizesProviderData(t *testing.T) {
	reset := time.Now().Add(time.Hour)
	for _, u := range []*service.UsageInfo{
		{FiveHour: &service.UsageProgress{Utilization: math.NaN()}},
		{FiveHour: &service.UsageProgress{Utilization: math.Inf(1)}},
		{FiveHour: &service.UsageProgress{Utilization: -1}},
		{FiveHour: &service.UsageProgress{Utilization: 0}, Error: "secret"},
		{FiveHour: &service.UsageProgress{Utilization: 0}, IsForbidden: true},
	} {
		if got := sanitizeSharedQuota("openai", u); len(got.windows) != 0 {
			t.Fatal("invalid/error quota became known")
		}
	}
	u := &service.UsageInfo{FiveHour: &service.UsageProgress{Utilization: 20, ResetsAt: &reset}, SevenDay: &service.UsageProgress{Utilization: 70}, AntigravityQuota: map[string]*service.AntigravityModelQuota{
		"gemini-pro":          {Utilization: 10, ResetTime: reset.Format(time.RFC3339)},
		"gemini-pro-thinking": {Utilization: 10, ResetTime: reset.Format(time.RFC3339)},
		"gemini-flash":        {Utilization: 80},
		"private@example.com": {Utilization: 50},
	}}
	got := sanitizeSharedQuota("antigravity", u)
	if len(got.windows) != 2 {
		t.Fatalf("distinct model snapshots collapsed or duplicate leaked: %#v", got.windows)
	}
	for _, w := range got.windows {
		if w.Key != "gemini_model" {
			t.Fatal("unverified window or provider key leaked")
		}
	}
	g := sanitizeSharedQuota("gemini", &service.UsageInfo{GeminiProMinute: &service.UsageProgress{Utilization: 20}, GeminiProDaily: &service.UsageProgress{Utilization: 50}})
	if len(g.windows) != 2 || g.windows[0].Key != "gemini_pro_minute" {
		t.Fatal("Gemini minute/day order incorrect")
	}
}

func TestUpstreamQuotaRestrictedAdminCannotChangeSharing(t *testing.T) {
	h, s, _, _ := quotaFixture()
	for _, method := range []string{"GET", "PUT"} {
		w := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(w)
		c.Request = httptest.NewRequest(method, "/admin/upstream-quota/settings", strings.NewReader(`{"enabled":false,"account_ids":[]}`))
		c.Request = c.Request.WithContext(adminaccess.WithPrincipal(c.Request.Context(), &adminaccess.Principal{}))
		if method == "GET" {
			h.AdminSettings(c)
		} else {
			h.SaveSettings(c)
		}
		if w.Code != 403 || !s.config.Enabled {
			t.Fatal("restricted key accessed sharing configuration")
		}
	}
}

func TestUpstreamQuotaAdminCanDisableWithoutAccountStorage(t *testing.T) {
	h, s, a, _ := quotaFixture()
	a.err = errors.New("account database unavailable")
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest("PUT", "/admin/upstream-quota/settings", strings.NewReader(`{"enabled":false,"account_ids":[1,2]}`))
	c.Request.Header.Set("Content-Type", "application/json")
	h.SaveSettings(c)
	if w.Code != 200 || s.config.Enabled || a.calls != 0 {
		t.Fatalf("cannot disable sharing: %d %s", w.Code, w.Body.String())
	}
}
