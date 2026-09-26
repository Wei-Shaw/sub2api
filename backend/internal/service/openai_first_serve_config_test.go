package service

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestFirstServeCustomTimings(t *testing.T) {
	now := time.Now()
	a := &Account{ID: 98704, Extra: map[string]any{"openai_first_serve": map[string]any{
		"rotate_seconds": 120, "ttl_minutes": 30, "ttft_seconds": 4, "max_switches": 2, "cooldown_seconds": 10,
	}}}
	s := newOpenAIFirstServeState(a, "route", now)
	s.observe(60000, now.Add(time.Minute))
	require.False(t, s.due(now.Add(time.Minute)), "latency never triggers rotation")
	require.Equal(t, now.Add(2*time.Minute), s.status.ExpiresAt)
	require.True(t, s.due(now.Add(2*time.Minute)))
	s.rotate(&Proxy{ID: 2}, now.Add(2*time.Minute))
	require.Equal(t, "route", s.status.ConnID)
	require.False(t, s.due(now.Add(239*time.Second)))
	require.True(t, s.due(now.Add(240*time.Second)))
}

func TestFirstServeConfigValidation(t *testing.T) {
	for name, raw := range map[string]any{
		"object": "bad", "null": nil,
		"rotation_zero":      map[string]any{"rotate_seconds": 0},
		"rotation_large":     map[string]any{"rotate_seconds": 86401},
		"rotation_fraction":  map[string]any{"rotate_seconds": 1.5},
		"zero":               map[string]any{"ttl_minutes": 0},
		"invalid_scope":      map[string]any{"reuse_scope": "global"},
		"negative":           map[string]any{"max_switches": -1},
		"negative_cooldown":  map[string]any{"cooldown_seconds": -1},
		"large":              map[string]any{"cooldown_seconds": 3601},
		"decimal":            map[string]any{"ttft_seconds": 1.5},
		"string":             map[string]any{"ttft_seconds": "15"},
		"field_null":         map[string]any{"ttft_seconds": nil},
		"typo":               map[string]any{"ttl_minute": 3},
		"empty_selected":     map[string]any{"proxy_mode": "selected", "proxy_ids": []int64{}},
		"duplicate_selected": map[string]any{"proxy_mode": "selected", "proxy_ids": []int64{1, 1}},
		"invalid_id":         map[string]any{"proxy_mode": "selected", "proxy_ids": []int64{0, 1}},
		"ambiguous_all":      map[string]any{"proxy_mode": "all", "proxy_ids": []int64{1, 2}},
	} {
		t.Run(name, func(t *testing.T) {
			a := &Account{Name: "account A", Extra: map[string]any{"openai_first_serve": raw}}
			_, err := a.firstServeConfig()
			require.ErrorContains(t, err, "account A")
		})
	}
	cfg, err := (&Account{}).firstServeConfig()
	require.NoError(t, err)
	require.Equal(t, defaultOpenAIFirstServeConfig(), cfg)
	changed := cfg
	changed.RotateSeconds = 3
	require.NotEqual(t, cfg.key(1), changed.key(1), "new settings isolate old connection state")
	require.NotEqual(t, cfg.key(1), cfg.key(2), "changing groups isolates old connections")
}

func TestFirstServeLegacyPolicyDoesNotAffectRotation(t *testing.T) {
	for _, extra := range []map[string]any{
		nil,
		{"openai_first_serve": map[string]any{"ttl_minutes": 12}},
		{"openai_first_serve": map[string]any{"cooldown_seconds": 60, "max_switches": 1, "ttft_seconds": 1}},
	} {
		now := time.Now()
		s := newOpenAIFirstServeState(&Account{Extra: extra}, "route", now)
		require.Equal(t, 240, s.status.Config.RotateSeconds)
		for range 6 {
			s.observe(60000, now)
			require.False(t, s.due(now))
			now = now.Add(240 * time.Second)
			require.True(t, s.due(now))
			s.rotate(nil, now)
			require.Equal(t, "route", s.status.ConnID)
		}
	}
}

func TestFirstServeConfigDefaultsToAccountSharing(t *testing.T) {
	for _, tc := range []struct {
		extra map[string]any
		want  string
	}{
		{want: "account"},
		{extra: map[string]any{"openai_first_serve": map[string]any{"ttl_minutes": 12}}, want: "account"},
		{extra: map[string]any{"openai_first_serve": map[string]any{"reuse_scope": "session"}}, want: "session"},
	} {
		cfg, err := (&Account{Extra: tc.extra}).firstServeConfig()
		require.NoError(t, err)
		require.Equal(t, tc.want, cfg.ReuseScope)
	}
}

type firstServeConfigRepo struct {
	proxyGroupServiceRepoStub
	groupID, previousID int64
	allowed             []int64
}

func (r *firstServeConfigRepo) SelectAvailableProxyExcluding(_ context.Context, groupID, previousID int64, allowed []int64) (*Proxy, error) {
	r.groupID, r.previousID, r.allowed = groupID, previousID, allowed
	return r.proxy, r.err
}

func (r *firstServeConfigRepo) GetByID(context.Context, int64) (*ProxyGroup, error) {
	return &ProxyGroup{ID: 10, ProxyIDs: []int64{1, 2, 3}}, nil
}

func TestFirstServeProxyAllowlist(t *testing.T) {
	defaultProxyGroupResolver.RLock()
	old := defaultProxyGroupResolver.resolver
	defaultProxyGroupResolver.RUnlock()
	defer SetDefaultProxyGroupResolver(old)
	repo := &firstServeConfigRepo{proxyGroupServiceRepoStub: proxyGroupServiceRepoStub{proxy: &Proxy{ID: 2, Host: "b", Port: 8080}}}
	NewProxyGroupService(repo)
	groupID := int64(10)
	a := &Account{Name: "account A", Platform: PlatformOpenAI, Type: AccountTypeAPIKey, ProxyGroupID: &groupID, Extra: map[string]any{
		"openai_apikey_responses_websockets_v2_mode": OpenAIWSIngressModeFirstServe,
		"openai_first_serve":                         map[string]any{"proxy_mode": "selected", "proxy_ids": []int64{2, 1}},
	}}
	require.NoError(t, validateOpenAIFirstServeProxies(context.Background(), a))
	for _, previous := range []int64{0, 1} {
		proxy, err := selectOpenAIFirstServeProxy(context.Background(), a, previous)
		require.NoError(t, err)
		require.Equal(t, int64(2), proxy.ID)
		require.Equal(t, groupID, repo.groupID)
		require.Equal(t, previous, repo.previousID)
		require.Equal(t, []int64{1, 2}, repo.allowed, "initial selection and rotation share the allowlist")
	}
	repo.proxy.ID = 3
	_, err := selectOpenAIFirstServeProxy(context.Background(), a, 0)
	require.ErrorIs(t, err, ErrProxyGroupNoProxy, "a resolver cannot escape the allowlist")
	a.Extra["openai_first_serve"] = map[string]any{"proxy_mode": "selected", "proxy_ids": []int64{2, 99}}
	require.ErrorContains(t, validateOpenAIFirstServeProxies(context.Background(), a), "#99")
	a.Extra["openai_first_serve"] = map[string]any{"proxy_mode": "all"}
	_, err = selectOpenAIFirstServeProxy(context.Background(), a, 0)
	require.NoError(t, err)
	require.Nil(t, repo.allowed, "all-mode uses NULL, not an empty SQL array that matches no proxies")
}

func TestFirstServeCreateDefaults(t *testing.T) {
	for _, kind := range []string{AccountTypeOAuth, AccountTypeSetupToken, AccountTypeAPIKey} {
		t.Run(kind, func(t *testing.T) {
			extra := map[string]any{"openai_passthrough": false}
			prepared := DefaultOpenAIFirstServeExtra(PlatformOpenAI, kind, extra)
			account := &Account{Platform: PlatformOpenAI, Type: kind, Extra: prepared}
			require.True(t, account.IsOpenAIFirstServe())
			cfg, err := account.firstServeConfig()
			require.NoError(t, err)
			require.Equal(t, 240, cfg.RotateSeconds)
			require.Equal(t, "account", cfg.ReuseScope)
			require.Equal(t, false, prepared["openai_passthrough"])
			require.Equal(t, map[string]any{"openai_passthrough": false}, extra, "caller map remains unchanged")
			if kind != AccountTypeAPIKey {
				require.Equal(t, "full", prepared[codexFingerprintModeExtraKey])
			}
		})
	}
	prepared := DefaultOpenAIFirstServeExtra(PlatformOpenAI, AccountTypeOAuth, map[string]any{
		"openai_oauth_responses_websockets_v2_mode": "off", "codex_fingerprint_mode": "off", "openai_passthrough": true,
	})
	require.Equal(t, "off", prepared["openai_oauth_responses_websockets_v2_mode"])
	require.Equal(t, "off", prepared["codex_fingerprint_mode"])
	require.Equal(t, true, prepared["openai_passthrough"])
	for _, key := range []string{"openai_oauth_responses_websockets_v2_enabled", "responses_websockets_v2_enabled", "openai_ws_enabled"} {
		for _, enabled := range []bool{false, true} {
			prepared := DefaultOpenAIFirstServeExtra(PlatformOpenAI, AccountTypeOAuth, map[string]any{key: enabled})
			account := &Account{Platform: PlatformOpenAI, Type: AccountTypeOAuth, Extra: prepared}
			require.False(t, account.IsOpenAIFirstServe(), "explicit legacy setting %s=%t must be preserved", key, enabled)
			require.NotContains(t, prepared, codexFingerprintModeExtraKey)
		}
	}
	require.Nil(t, DefaultOpenAIFirstServeExtra(PlatformAnthropic, AccountTypeOAuth, nil))
}

type firstServeDefaultResolver struct {
	firstServeTestResolver
	groups []ProxyGroup
}

func (r firstServeDefaultResolver) ListAll(context.Context) ([]ProxyGroup, error) {
	return r.groups, nil
}

func TestFirstServeCreateSelectsDefaultGroup(t *testing.T) {
	a := firstServeHTTPAccount(t)
	a.ProxyGroupID = nil
	SetDefaultProxyGroupResolver(firstServeDefaultResolver{groups: []ProxyGroup{
		{ID: 1, Status: ProxyGroupStatusInactive, AvailableMemberCount: 2},
		{ID: 2, Status: ProxyGroupStatusActive, AvailableMemberCount: 1},
		{ID: 3, Status: ProxyGroupStatusActive, AvailableMemberCount: 2, ProxyIDs: []int64{101, 102}},
	}})
	repo := &upstreamBillingProbeAccountRepo{}
	svc := &adminServiceImpl{accountRepo: repo}
	created, err := svc.CreateAccount(context.Background(), &CreateAccountInput{
		Name: "imported", Platform: PlatformOpenAI, Type: AccountTypeAPIKey, SkipDefaultGroupBind: true,
	})
	require.NoError(t, err)
	require.Equal(t, int64(3), *created.ProxyGroupID)
	require.True(t, created.IsOpenAIFirstServe())
	require.NotContains(t, created.Extra, "openai_passthrough")
	group := int64(8)
	a.ProxyGroupID = &group
	require.NoError(t, assignDefaultFirstServeProxyGroup(context.Background(), a))
	require.Equal(t, group, *a.ProxyGroupID, "explicit binding wins")
	fixed := int64(999)
	a.ProxyGroupID, a.ProxyID = nil, &fixed
	require.NoError(t, assignDefaultFirstServeProxyGroup(context.Background(), a))
	require.Nil(t, a.ProxyGroupID, "do not replace an explicit single proxy with an unrelated group")
	require.NoError(t, validateOpenAIFirstServe(a), "pending accounts can be saved")
	require.ErrorContains(t, validateOpenAIFirstServeRouting(a), a.Name)
}

func TestFirstServeEnableDefaultsToFullWithoutChangingPassthrough(t *testing.T) {
	a := &Account{Platform: PlatformOpenAI, Type: AccountTypeOAuth}
	extra := map[string]any{"openai_oauth_responses_websockets_v2_mode": OpenAIWSIngressModeFirstServe, "openai_passthrough": false}
	prepared := prepareCodexFingerprintExtraForUpdate(a, extra)
	require.Equal(t, "full", prepared[codexFingerprintModeExtraKey])
	require.Equal(t, false, prepared["openai_passthrough"])
	require.NotEmpty(t, prepared[codexFingerprintSeedExtraKey])
	extra[codexFingerprintModeExtraKey] = "off"
	prepared = prepareCodexFingerprintExtraForUpdate(a, extra)
	require.Equal(t, "off", prepared[codexFingerprintModeExtraKey], "explicit fingerprint choice wins")
	prepared = DefaultOpenAIFirstServeExtra(PlatformOpenAI, AccountTypeOAuth, map[string]any{"openai_oauth_responses_websockets_v2_enabled": false})
	require.False(t, (&Account{Platform: PlatformOpenAI, Type: AccountTypeOAuth, Extra: prepared}).IsOpenAIFirstServe())
	require.NotContains(t, prepared, codexFingerprintModeExtraKey)
}
