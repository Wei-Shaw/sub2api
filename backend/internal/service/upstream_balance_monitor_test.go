package service

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"sort"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

type upstreamBalanceTestEncryptor struct{}

func (upstreamBalanceTestEncryptor) Encrypt(v string) (string, error) { return "enc:" + v, nil }
func (upstreamBalanceTestEncryptor) Decrypt(v string) (string, error) {
	if len(v) < 4 || v[:4] != "enc:" {
		return "", errors.New("bad ciphertext")
	}
	return v[4:], nil
}

type upstreamBalanceMemoryRepo struct {
	mu   sync.Mutex
	next int64
	rows map[int64]*UpstreamBalanceMonitor
}

func newUpstreamBalanceMemoryRepo() *upstreamBalanceMemoryRepo {
	return &upstreamBalanceMemoryRepo{next: 1, rows: map[int64]*UpstreamBalanceMonitor{}}
}
func cloneUpstreamBalance(m *UpstreamBalanceMonitor) *UpstreamBalanceMonitor {
	c := *m
	if m.SnapshotData != nil {
		c.SnapshotData = map[string]any{}
		for k, v := range m.SnapshotData {
			c.SnapshotData[k] = v
		}
	}
	return &c
}
func (r *upstreamBalanceMemoryRepo) Create(_ context.Context, m *UpstreamBalanceMonitor) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	m.ID = r.next
	r.next++
	m.CreatedAt = time.Now()
	m.UpdatedAt = m.CreatedAt
	r.rows[m.ID] = cloneUpstreamBalance(m)
	return nil
}
func (r *upstreamBalanceMemoryRepo) GetByID(_ context.Context, id int64) (*UpstreamBalanceMonitor, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	m := r.rows[id]
	if m == nil {
		return nil, ErrUpstreamBalanceMonitorNotFound
	}
	return cloneUpstreamBalance(m), nil
}
func (r *upstreamBalanceMemoryRepo) Update(_ context.Context, m *UpstreamBalanceMonitor) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.rows[m.ID] == nil {
		return ErrUpstreamBalanceMonitorNotFound
	}
	r.rows[m.ID] = cloneUpstreamBalance(m)
	return nil
}
func (r *upstreamBalanceMemoryRepo) Delete(_ context.Context, id int64) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.rows[id] == nil {
		return ErrUpstreamBalanceMonitorNotFound
	}
	delete(r.rows, id)
	return nil
}
func (r *upstreamBalanceMemoryRepo) List(_ context.Context) ([]*UpstreamBalanceMonitor, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := make([]*UpstreamBalanceMonitor, 0, len(r.rows))
	for _, m := range r.rows {
		out = append(out, cloneUpstreamBalance(m))
	}
	sort.Slice(out, func(i, j int) bool { return out[i].DisplayOrder < out[j].DisplayOrder })
	return out, nil
}
func (r *upstreamBalanceMemoryRepo) ListDue(_ context.Context, now time.Time, limit int) ([]*UpstreamBalanceMonitor, error) {
	all, _ := r.List(context.Background())
	out := all[:0]
	for _, m := range all {
		if m.Enabled && (m.NextProbeAt == nil || !m.NextProbeAt.After(now)) {
			out = append(out, m)
		}
	}
	if len(out) > limit {
		out = out[:limit]
	}
	return out, nil
}
func (r *upstreamBalanceMemoryRepo) UpdateProbeResult(ctx context.Context, m *UpstreamBalanceMonitor) error {
	return r.Update(ctx, m)
}

func newUpstreamBalanceTestService(repo UpstreamBalanceMonitorRepository, server *httptest.Server) *UpstreamBalanceMonitorService {
	s := NewUpstreamBalanceMonitorService(repo, upstreamBalanceTestEncryptor{})
	s.client = server.Client()
	s.validateHost = func(string) error { return nil }
	s.validateInput = func(in *UpstreamBalanceMonitorInput, requireKey bool) error {
		if requireKey && in.CredentialMode == UpstreamCredentialToken && in.Type == UpstreamBalanceTypeSub2API && in.APIKey == "" {
			return errors.New("access token required")
		}
		if requireKey && in.CredentialMode == UpstreamCredentialToken && in.Type == UpstreamBalanceTypeNewAPI && (in.Cookie == "" || in.UserID == "") {
			return errors.New("cookie and user id required")
		}
		return nil
	}
	return s
}

func TestUpstreamBalanceMonitorCreateProbesSub2APIAndMasksKey(t *testing.T) {
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/v1/auth/me":
			require.Equal(t, "Bearer access-token-123456789", r.Header.Get("Authorization"))
			_, _ = w.Write([]byte(`{"code":0,"message":"success","data":{"username":"owner","balance":12.5}}`))
		case "/api/v1/groups/available":
			_, _ = w.Write([]byte(`{"code":0,"data":[{"id":1,"name":"default","rate_multiplier":1.2}]}`))
		case "/api/v1/groups/rates":
			_, _ = w.Write([]byte(`{"code":0,"data":{"1":1.5}}`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()
	svc := newUpstreamBalanceTestService(newUpstreamBalanceMemoryRepo(), server)
	item, err := svc.Create(context.Background(), UpstreamBalanceMonitorInput{Name: "primary", Type: "sub2api", BaseURL: server.URL, CredentialMode: UpstreamCredentialToken, APIKey: "access-token-123456789", Enabled: true, ProbeIntervalMinutes: 30, LowBalanceThresholdUSD: 10})
	require.NoError(t, err)
	require.Equal(t, "ok", item.LastProbeStatus)
	require.Equal(t, 12.5, item.BalanceDisplay["quota_remaining_usd"])
	require.Equal(t, "owner", item.BalanceDisplay["username"])
	require.Equal(t, "acce****6789", item.APIKeyMasked)
	require.Empty(t, item.APIKey)
}

func TestUpstreamBalanceMonitorNewAPIDisplayAndFailurePreservesSnapshot(t *testing.T) {
	fail := false
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if fail {
			http.Error(w, "down", http.StatusBadGateway)
			return
		}
		if r.URL.Path == "/api/status" {
			_, _ = w.Write([]byte(`{"success":true,"data":{"quota_per_unit":1000000}}`))
			return
		}
		if r.URL.Path == "/api/user/self/groups" {
			_, _ = w.Write([]byte(`{"success":true,"data":{"default":{"ratio":1.2,"desc":"Default"}}}`))
			return
		}
		require.Equal(t, "/api/user/self", r.URL.Path)
		require.Equal(t, "session=abcdef123456", r.Header.Get("Cookie"))
		require.Equal(t, "42", r.Header.Get("New-Api-User"))
		_, _ = w.Write([]byte(`{"success":true,"data":{"quota":1000000,"used_quota":250000,"request_count":12,"group":"default"}}`))
	}))
	defer server.Close()
	repo := newUpstreamBalanceMemoryRepo()
	svc := newUpstreamBalanceTestService(repo, server)
	item, err := svc.Create(context.Background(), UpstreamBalanceMonitorInput{Name: "backup", Type: "newapi", BaseURL: server.URL, CredentialMode: UpstreamCredentialToken, Cookie: "session=abcdef123456", UserID: "42", Enabled: true, ProbeIntervalMinutes: 5, LowBalanceThresholdUSD: 2})
	require.NoError(t, err)
	require.Equal(t, 1.0, item.BalanceDisplay["quota_remaining_usd"])
	require.Equal(t, 0.25, item.BalanceDisplay["used_quota_usd"])
	fail = true
	failed, err := svc.Probe(context.Background(), item.ID)
	require.Error(t, err)
	require.Equal(t, "failed", failed.LastProbeStatus)
	require.Equal(t, 1.0, failed.BalanceDisplay["quota_remaining_usd"])
	require.Equal(t, 1, repo.rows[item.ID].FailureCount)
	require.NotNil(t, repo.rows[item.ID].NextProbeAt)
}

func TestUpstreamBalanceMonitorPasswordLogin(t *testing.T) {
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/v1/auth/login":
			require.Equal(t, http.MethodPost, r.Method)
			_, _ = w.Write([]byte(`{"code":0,"data":{"access_token":"fresh-token"}}`))
		case "/api/v1/auth/me":
			require.Equal(t, "Bearer fresh-token", r.Header.Get("Authorization"))
			_, _ = w.Write([]byte(`{"code":0,"data":{"email":"owner@example.com","balance":8.75}}`))
		case "/api/v1/groups/available":
			_, _ = w.Write([]byte(`{"code":0,"data":[{"id":1,"name":"default","rate_multiplier":1}]}`))
		case "/api/v1/groups/rates":
			_, _ = w.Write([]byte(`{"code":0,"data":{}}`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()
	svc := newUpstreamBalanceTestService(newUpstreamBalanceMemoryRepo(), server)
	item, err := svc.Create(context.Background(), UpstreamBalanceMonitorInput{Name: "password", Type: "sub2api", BaseURL: server.URL, CredentialMode: UpstreamCredentialPassword, Username: "owner@example.com", Password: "secret", Enabled: true, ProbeIntervalMinutes: 30})
	require.NoError(t, err)
	require.Equal(t, 8.75, item.BalanceDisplay["quota_remaining_usd"])
	require.Equal(t, UpstreamCredentialPassword, item.CredentialMode)
	require.Equal(t, "owner@example.com", item.Username)
}

func TestUpstreamBalanceMonitorNewAPIPasswordLogin(t *testing.T) {
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/user/login":
			http.SetCookie(w, &http.Cookie{Name: "session", Value: "logged-in"})
			_, _ = w.Write([]byte(`{"success":true,"data":{"id":42}}`))
		case "/api/status":
			_, _ = w.Write([]byte(`{"success":true,"data":{"quota_per_unit":500000}}`))
		case "/api/user/self":
			require.Equal(t, "session=logged-in", r.Header.Get("Cookie"))
			require.Equal(t, "42", r.Header.Get("New-Api-User"))
			_, _ = w.Write([]byte(`{"success":true,"data":{"quota":750000,"used_quota":250000}}`))
		case "/api/user/self/groups":
			_, _ = w.Write([]byte(`{"success":true,"data":{"default":{"ratio":1}}}`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()
	svc := newUpstreamBalanceTestService(newUpstreamBalanceMemoryRepo(), server)
	item, err := svc.Create(context.Background(), UpstreamBalanceMonitorInput{Name: "newapi-password", Type: "newapi", BaseURL: server.URL, CredentialMode: UpstreamCredentialPassword, Username: "owner", Password: "secret", Enabled: true, ProbeIntervalMinutes: 30})
	require.NoError(t, err)
	require.Equal(t, 1.5, item.BalanceDisplay["quota_remaining_usd"])
}

func TestValidateUpstreamBalanceInputRejectsPrivateAndInvalidInterval(t *testing.T) {
	in := UpstreamBalanceMonitorInput{Name: "x", Type: "newapi", BaseURL: "https://127.0.0.1", APIKey: "x", ProbeIntervalMinutes: 30}
	require.ErrorContains(t, validateUpstreamBalanceInput(&in, true), "host is not allowed")
	in.BaseURL = "https://example.com"
	in.ProbeIntervalMinutes = 2
	require.ErrorContains(t, validateUpstreamBalanceInput(&in, true), "between 5 and 1440")
}

func TestUpstreamBalanceMonitorRedirectValidation(t *testing.T) {
	svc := NewUpstreamBalanceMonitorService(newUpstreamBalanceMemoryRepo(), upstreamBalanceTestEncryptor{})
	svc.validateHost = func(host string) error {
		if host == "internal.example" {
			return errors.New("private address")
		}
		return nil
	}

	httpsReq := &http.Request{URL: mustParseURL(t, "https://public.example/billing")}
	require.NoError(t, svc.checkRedirect(httpsReq, nil))
	require.ErrorContains(t, svc.checkRedirect(&http.Request{URL: mustParseURL(t, "http://public.example/billing")}, nil), "must use https")
	require.ErrorContains(t, svc.checkRedirect(&http.Request{URL: mustParseURL(t, "https://internal.example/billing")}, nil), "unsafe upstream redirect host")
	require.ErrorContains(t, svc.checkRedirect(httpsReq, make([]*http.Request, 10)), "too many")
}

func mustParseURL(t *testing.T, raw string) *url.URL {
	t.Helper()
	u, err := url.Parse(raw)
	require.NoError(t, err)
	return u
}
