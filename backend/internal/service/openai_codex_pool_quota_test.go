package service

import (
	"context"
	"fmt"
	"net/http"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

type codexPoolQuotaAccounts struct {
	AccountRepository
	accounts []Account
}

func (r *codexPoolQuotaAccounts) ListModelAvailabilityCandidates(_ context.Context, _ *int64, _ []string, _ bool) ([]Account, error) {
	return r.accounts, nil
}

func quotaTestAccount(id int64, observed, cooldown, reset time.Time, explicit bool) Account {
	return Account{
		ID: id, Platform: PlatformOpenAI, Status: StatusActive, Type: AccountTypeOAuth, Schedulable: true,
		RateLimitedAt: &observed, RateLimitResetAt: &cooldown,
		Extra: map[string]any{OpenAICodexQuotaCooldownExtraKey: map[string]any{
			"limited_at": observed.Format(time.RFC3339Nano), "cooldown_until": cooldown.Format(time.RFC3339Nano),
			"reset_at": reset.Format(time.RFC3339Nano), "explicit": explicit,
		}},
	}
}

func TestCodexPoolQuotaReset(t *testing.T) {
	now := time.Now().UTC().Truncate(time.Second)
	hour, twoHours, week := now.Add(time.Hour), now.Add(2*time.Hour), now.Add(7*24*time.Hour)
	first := quotaTestAccount(1, now, hour, week, true)
	second := quotaTestAccount(2, now, twoHours, twoHours, true)
	ordinary429 := quotaTestAccount(3, now, now.Add(time.Minute), now.Add(time.Minute), false)
	ordinary429.Extra = nil
	available := Account{ID: 4, Platform: PlatformOpenAI, Status: StatusActive, Type: AccountTypeAPIKey, Schedulable: true}
	expired := quotaTestAccount(5, now.Add(-time.Hour), now.Add(-time.Minute), now.Add(-time.Minute), true)
	disable := quotaTestAccount(6, now, hour, hour, true)
	disable.Schedulable = false
	otherBlock := quotaTestAccount(7, now, hour, hour, true)
	otherBlock.OverloadUntil = &twoHours
	mismatched := quotaTestAccount(8, now, hour, hour, true)
	mismatched.Credentials = map[string]any{"model_mapping": map[string]any{"different": "different"}}

	groupID := int64(5)
	tests := []struct {
		name     string
		accounts []Account
		failed   map[int64]struct{}
		want     time.Time
	}{
		{name: "earliest of effective deadlines", accounts: []Account{first, second}, want: twoHours},
		{name: "ordinary 429 is infinity", accounts: []Account{ordinary429, second}, want: twoHours},
		{name: "all ordinary 429", accounts: []Account{ordinary429}},
		{name: "remaining candidate after budget exhausted", accounts: []Account{second, available}},
		{name: "already attempted error without a reset", accounts: []Account{second, available}, failed: map[int64]struct{}{4: {}}, want: twoHours},
		{name: "expired cooldown makes account available", accounts: []Account{second, expired}},
		{name: "independent blockers and unsupported model", accounts: []Account{second, disable, otherBlock, mismatched}, want: twoHours},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			s := &OpenAIGatewayService{accountRepo: &codexPoolQuotaAccounts{accounts: tc.accounts}}
			got := s.EarliestCodexPoolQuotaReset(context.Background(), CodexPoolQuotaRequest{
				GroupID: &groupID, Model: "gpt-5.1", Transport: OpenAIUpstreamTransportAny, FailedAccounts: tc.failed,
			})
			require.True(t, got.Equal(tc.want), "got %v, want %v", got, tc.want)
		})
	}
}

func TestCodexPoolQuotaFiltersExplicitImageSize(t *testing.T) {
	now := time.Now().UTC().Truncate(time.Second)
	early, late := now.Add(time.Hour), now.Add(2*time.Hour)
	lowTier := quotaTestAccount(1, now, early, early, true)
	lowTier.Extra[AccountExtraOpenAIImageSizeTiers] = []any{"1K"}
	highTier := quotaTestAccount(2, now, late, late, true)
	highTier.Extra[AccountExtraOpenAIImageSizeTiers] = []any{"4K"}
	groupID := int64(5)
	req := CodexPoolQuotaRequest{GroupID: &groupID, Model: "gpt-5.1", Transport: OpenAIUpstreamTransportAny}
	for _, tc := range []struct {
		name      string
		size      string
		lowUsable bool
		want      time.Time
	}{
		{name: "4K ignores earlier 1K reset", size: "3840x2160", want: late},
		{name: "4K ignores available 1K account", size: "3840x2160", lowUsable: true, want: late},
		{name: "auto can use either tier", size: "auto", want: early},
		{name: "unspecified text request can use either tier", want: early},
	} {
		t.Run(tc.name, func(t *testing.T) {
			first := lowTier
			if tc.lowUsable {
				first.RateLimitedAt, first.RateLimitResetAt = nil, nil
			}
			s := &OpenAIGatewayService{accountRepo: &codexPoolQuotaAccounts{accounts: []Account{first, highTier}}}
			ctx := context.Background()
			if tc.size != "" {
				ctx = WithOpenAIImageSizeRouting(ctx, tc.size)
			}
			got := s.EarliestCodexPoolQuotaReset(ctx, req)
			require.True(t, got.Equal(tc.want), "got %v, want %v", got, tc.want)
		})
	}
}

func TestCodexPoolQuotaUsesPersistedCooldownForRuntimeBlock(t *testing.T) {
	now := time.Now().UTC().Truncate(time.Second)
	past, early, late, localDeadline := now.Add(-time.Minute), now.Add(time.Hour), now.Add(2*time.Hour), now.Add(3*time.Hour)
	groupID := int64(5)
	req := CodexPoolQuotaRequest{GroupID: &groupID, Model: "gpt-5.1", Transport: OpenAIUpstreamTransportAny}
	for _, tc := range []struct {
		name     string
		cooldown *time.Time
		want     time.Time
	}{
		{name: "persisted cooldown cleared"},
		{name: "persisted cooldown expired", cooldown: &past},
		{name: "active persisted cooldown keeps the longer runtime block", cooldown: &early, want: late},
	} {
		t.Run(tc.name, func(t *testing.T) {
			first := quotaTestAccount(1, now, early, early, true)
			first.RateLimitResetAt = tc.cooldown
			second := quotaTestAccount(2, now, late, late, true)
			s := &OpenAIGatewayService{accountRepo: &codexPoolQuotaAccounts{accounts: []Account{first, second}}}
			s.BlockAccountScheduling(&first, localDeadline, "quota cooldown")
			got := s.EarliestCodexPoolQuotaReset(context.Background(), req)
			require.True(t, got.Equal(tc.want), "got %v, want %v", got, tc.want)
			stored, ok := s.openaiAccountRuntimeBlockUntil.Load(first.ID)
			require.True(t, ok, "diagnosis is read-only")
			require.Equal(t, localDeadline, stored)
		})
	}
}

func TestCodexPoolQuotaUsageSnapshots(t *testing.T) {
	now := time.Now().UTC().Truncate(time.Second)
	short, long := now.Add(time.Hour), now.Add(24*time.Hour)
	groupID := int64(5)
	for _, tc := range []struct {
		name        string
		used5h      string
		used7d      string
		pause5h     float64
		pause7d     float64
		thresholds  map[string]int
		hasCooldown bool
		want        time.Time
	}{
		{name: "5h exhausted by snapshot alone", used5h: "100", used7d: "50", pause5h: 1, want: short},
		{name: "both windows exhausted by snapshots alone", used5h: "100", used7d: "100", pause5h: 1, pause7d: 1, want: long},
		{name: "7d exhaustion outlasts recorded 5h cooldown", used5h: "100", used7d: "100", hasCooldown: true, want: long},
		{name: "early 7d pause with 5h exhaustion", used5h: "100", used7d: "95", pause5h: 1, pause7d: 0.9},
		{name: "usage just below exhaustion", used5h: "99.99", used7d: "50", pause5h: 0.95},
		{name: "scheduling threshold uses percentages", used5h: "100", used7d: "50", thresholds: map[string]int{PlatformOpenAI: 90}, want: short},
		{name: "early 5h scheduling threshold with later 7d exhaustion", used5h: "95", used7d: "100", thresholds: map[string]int{PlatformOpenAI: 90}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			snapshot := ParseCodexRateLimitHeaders(http.Header{
				"X-Codex-Primary-Used-Percent": {tc.used5h}, "X-Codex-Primary-Window-Minutes": {"300"},
				"X-Codex-Primary-Reset-After-Seconds": {"3600"},
				"X-Codex-Secondary-Used-Percent":      {tc.used7d}, "X-Codex-Secondary-Window-Minutes": {"10080"},
				"X-Codex-Secondary-Reset-After-Seconds": {"86400"},
			})
			require.NotNil(t, snapshot)
			account := Account{ID: 1, Platform: PlatformOpenAI, Type: AccountTypeOAuth, Status: StatusActive, Schedulable: true}
			if tc.hasCooldown {
				account = quotaTestAccount(1, now, short, short, true)
			}
			extra := buildCodexUsageExtraUpdates(snapshot, now)
			if tc.hasCooldown {
				extra[OpenAICodexQuotaCooldownExtraKey] = account.Extra[OpenAICodexQuotaCooldownExtraKey]
			}
			extra["auto_pause_5h_threshold"] = tc.pause5h
			extra["auto_pause_7d_threshold"] = tc.pause7d
			account.Extra = extra
			s := &OpenAIGatewayService{accountRepo: &codexPoolQuotaAccounts{accounts: []Account{account}}}
			req := CodexPoolQuotaRequest{GroupID: &groupID, Model: "gpt-5.1", Transport: OpenAIUpstreamTransportAny}
			if tc.thresholds != nil {
				got, admissible := s.codexQuotaCandidateReset(context.Background(), &account, req, tc.thresholds, now)
				require.False(t, admissible)
				require.True(t, got.Equal(tc.want), "got %v, want %v", got, tc.want)
				return
			}
			got := s.EarliestCodexPoolQuotaReset(context.Background(), req)
			require.True(t, got.Equal(tc.want), "got %v, want %v", got, tc.want)
		})
	}
}

func TestCodexPoolQuotaProof(t *testing.T) {
	now := time.Now().UTC().Truncate(time.Second)
	short, long := now.Add(30*time.Minute), now.Add(time.Hour)
	explicit := []byte(fmt.Sprintf(`{"error":{"type":"usage_limit_reached","resets_at":%d}}`, short.Unix()))
	nested := []byte(fmt.Sprintf(`{"type":"response.failed","response":{"error":{"type":"usage_limit_reached","resets_at":%d}}}`, short.Unix()))
	ordinary := []byte(fmt.Sprintf(`{"error":{"type":"rate_limit_exceeded","resets_at":%d}}`, short.Unix()))
	account := quotaTestAccount(1, now, short, long, false)
	headers := http.Header{
		"X-Codex-Primary-Used-Percent": {"100"}, "X-Codex-Primary-Window-Minutes": {"300"}, "X-Codex-Primary-Reset-After-Seconds": {"1800"},
		"X-Codex-Secondary-Used-Percent": {"100"}, "X-Codex-Secondary-Window-Minutes": {"10080"}, "X-Codex-Secondary-Reset-After-Seconds": {"3600"},
	}

	for _, tc := range []struct {
		name    string
		body    []byte
		headers http.Header
		want    time.Time
	}{
		{"explicit", explicit, nil, short},
		{"SSE nested", nested, nil, short},
		{"ordinary 429 with exhausted headers", ordinary, headers, time.Time{}},
		{"both quota windows block", explicit, headers, long},
		{"header-only exhausted windows", nil, headers, long},
	} {
		t.Run(tc.name, func(t *testing.T) {
			reset, _ := codexQuota429Reset(&account, tc.headers, tc.body, now)
			require.True(t, reset.Equal(tc.want), "got %v, want %v", reset, tc.want)
		})
	}
	headers.Set("x-codex-credits-unlimited", "true")
	got, _ := codexQuota429Reset(&account, headers, nil, now)
	require.True(t, got.IsZero())
	got, _ = codexQuota429Reset(&account, nil, explicit, now)
	require.True(t, got.Equal(short))

	require.Equal(t, long, recordedCodexQuotaReset(&account, now))
	account.Extra[openaiQuotaCreditsKey] = map[string]any{"credits": map[string]any{"balance": "0.01"}}
	require.True(t, recordedCodexQuotaReset(&account, now).IsZero())
	delete(account.Extra, openaiQuotaCreditsKey)
	replacedGeneration := now.Add(time.Second)
	account.RateLimitedAt = &replacedGeneration
	require.True(t, recordedCodexQuotaReset(&account, now).IsZero())
	account.RateLimitResetAt = nil
	require.True(t, recordedCodexQuotaReset(&account, now).IsZero())
}

func TestCodexPoolQuotaIndependentWindowAndResetCredits(t *testing.T) {
	now := time.Now().UTC().Truncate(time.Second)
	reset := now.Add(time.Hour)
	account := quotaTestAccount(1, now, reset, reset, true)
	s := &OpenAIGatewayService{}
	req := CodexPoolQuotaRequest{Model: "gpt-5.1"}
	account.Extra[OpenAIAutoResetCreditStateExtraKey] = OpenAIAutoResetCreditState{
		Status: OpenAIAutoResetStatusResetting, CheckedAt: now.Format(time.RFC3339),
	}
	got, admissible := s.codexQuotaCandidateReset(context.Background(), &account, req, nil, now)
	require.False(t, admissible)
	require.True(t, got.IsZero(), "a reset in progress has no known completion time")
	account.Extra[OpenAIAutoResetCreditStateExtraKey] = OpenAIAutoResetCreditState{
		Status: OpenAIAutoResetStatusNoCredit, CheckedAt: now.Format(time.RFC3339),
	}
	got, admissible = s.codexQuotaCandidateReset(context.Background(), &account, req, nil, now)
	require.False(t, admissible)
	require.True(t, got.Equal(reset), "a quota window still has a natural reset when no reset credits remain")

	account.RateLimitResetAt = nil
	setAccountModelRateLimitSnapshot(&account, "gpt-5.1", reset, openAICodexSparkQuotaReason, now)
	got, admissible = s.codexQuotaCandidateReset(context.Background(), &account, req, nil, now)
	require.False(t, admissible)
	require.True(t, got.Equal(reset))
	req.Model = "gpt-5.2"
	got, admissible = s.codexQuotaCandidateReset(context.Background(), &account, req, nil, now)
	require.True(t, admissible)
	require.True(t, got.IsZero(), "a model-specific quota cannot affect another model")
}

func TestCodexPoolQuotaGuardianAffinityIsNotExclusive(t *testing.T) {
	now := time.Now()
	parent := quotaTestAccount(1, now, now.Add(time.Hour), now.Add(time.Hour), true)
	other := Account{ID: 2, Platform: PlatformOpenAI, Status: StatusActive, Type: AccountTypeOAuth, Schedulable: true}
	repo := &codexPoolQuotaAccounts{accounts: []Account{parent, other}}
	s := &OpenAIGatewayService{
		accountRepo: repo,
		cache:       &stubGatewayCache{sessionBindings: map[string]int64{"openai:guardian-parent-session": parent.ID}},
	}
	groupID := int64(5)
	ctx := context.WithValue(context.Background(), openAIGuardianParentAffinityContextKey{}, openAIGuardianParentAffinity{currentSessionHash: "guardian-parent-session"})
	require.Equal(t, parent.ID, s.resolveOpenAIGuardianParentAccountID(ctx, &groupID))
	require.True(t, s.EarliestCodexPoolQuotaReset(ctx, CodexPoolQuotaRequest{GroupID: &groupID, Model: "gpt-5.1", Transport: OpenAIUpstreamTransportAny}).IsZero())
}

func TestCodexPoolQuotaIgnoresQuarantinedProxies(t *testing.T) {
	now := time.Now()
	early := now.Add(time.Hour)
	late := now.Add(2 * time.Hour)
	proxyID := int64(42)
	quarantined := quotaTestAccount(1, now, early, early, true)
	quarantined.ProxyID = &proxyID
	availableViaQuarantinedProxy := Account{ID: 2, Platform: PlatformOpenAI, Status: StatusActive, Type: AccountTypeOAuth, Schedulable: true, ProxyID: &proxyID}
	eligible := quotaTestAccount(3, now, late, late, true)
	s := &OpenAIGatewayService{accountRepo: &codexPoolQuotaAccounts{accounts: []Account{quarantined, availableViaQuarantinedProxy, eligible}}}
	s.openaiProxyStreamCircuit = newOpenAIProxyStreamCircuit(openAIProxyStreamCircuitSettings{
		failureThreshold: 1, failureWindow: time.Minute, quarantineTTL: 10 * time.Minute, maxEntries: 16,
	})
	s.openaiProxyStreamCircuit.recordFailure(proxyID, now)
	groupID := int64(5)
	reset := s.EarliestCodexPoolQuotaReset(context.Background(), CodexPoolQuotaRequest{GroupID: &groupID, Model: "gpt-5.1", Transport: OpenAIUpstreamTransportAny})
	require.True(t, reset.Equal(late), "got %v, want %v", reset, late)
}
