//go:build unit

package service

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/usagestats"
	"github.com/stretchr/testify/require"
)

type threadPolicyRepo struct {
	SettingRepository
	value string
	err   error
	set   string
}

func (r *threadPolicyRepo) GetValue(context.Context, string) (string, error) { return r.value, r.err }

func (r *threadPolicyRepo) Set(_ context.Context, _, value string) error {
	r.set = value
	return nil
}

// The strict reader and the ordinary one, side by side: they agree on every
// policy that can be read, and differ only where the ordinary one answers
// with the default policy for a value it could not understand.
func TestMessageThreadStrictBetaPolicy(t *testing.T) {
	ctx := context.Background()
	rule := `{"beta_token":"some-beta","action":"block","scope":"oauth","error_message":"no"}`
	for name, tc := range map[string]struct {
		value     string
		err       error
		wantRules int // -1: the default policy
		fails     bool
	}{
		"never stored":        {err: ErrSettingNotFound, wantRules: -1},
		"stored empty":        {value: "", wantRules: -1},
		"stored as blanks":    {value: " \n", wantRules: -1, fails: false},
		"no rules":            {value: `{"rules":[]}`},
		"an empty object":     {value: `{}`},
		"rules null":          {value: `{"rules":null}`},
		"one rule":            {value: `{"rules":[` + rule + `]}`, wantRules: 1},
		"with a whitelist":    {value: `{"rules":[{"beta_token":"b","action":"pass","scope":"all","model_whitelist":["claude-*"],"fallback_action":"filter"}]}`, wantRules: 1},
		"an unknown member":   {value: `{"rules":[],"added_later":true}`},
		"store failure":       {err: errors.New("connection refused"), fails: true},
		"cut off":             {value: `{"rules":[` + rule[:20], fails: true},
		"not json":            {value: `rules: none`, fails: true},
		"rules of wrong type": {value: `{"rules":"all"}`, fails: true},
		"a list":              {value: `[]`, fails: true},
		"null":                {value: `null`, fails: true},
		"trailing data":       {value: `{"rules":[]} {"rules":[]}`, fails: true},
		"unknown action":      {value: `{"rules":[{"beta_token":"b","action":"deny","scope":"all"}]}`, fails: true},
		"unknown scope":       {value: `{"rules":[{"beta_token":"b","action":"block","scope":"everyone"}]}`, fails: true},
		"unknown fallback":    {value: `{"rules":[{"beta_token":"b","action":"pass","scope":"all","fallback_action":"deny"}]}`, fails: true},
		"no beta token":       {value: `{"rules":[{"beta_token":"","action":"block","scope":"all"}]}`, fails: true},
	} {
		settings := &SettingService{settingRepo: &threadPolicyRepo{value: tc.value, err: tc.err}}
		strict, err := settings.readBetaPolicyStrict(ctx)
		if tc.fails {
			require.Error(t, err, name)
			require.Nil(t, strict, name)

			// A thread request is not sent on it.
			thread := &MessageThread{}
			threadErr := threadTestError(t, (&GatewayService{settingService: settings}).loadMessageThreadPolicy(ctx, thread))
			require.Equal(t, http.StatusServiceUnavailable, threadErr.Status, name)
			require.Empty(t, threadErr.Code, name)
			require.False(t, thread.attempt.policyLoaded, name)
			continue
		}
		require.NoError(t, err, name)
		want := tc.wantRules
		if want < 0 {
			want = len(DefaultBetaPolicySettings().Rules)
		}
		require.Len(t, strict.Rules, want, name)
		ordinary, err := settings.GetBetaPolicySettings(ctx)
		require.NoError(t, err, name)
		if strings.TrimSpace(tc.value) != "" || tc.err != nil {
			require.Equal(t, ordinary, strict, name)
		}

		thread := &MessageThread{}
		require.NoError(t, (&GatewayService{settingService: settings}).loadMessageThreadPolicy(ctx, thread), name)
		require.True(t, thread.attempt.policyLoaded, name)
		require.Equal(t, strict, thread.attempt.policy, name)
	}

	// Whatever the settings API accepts and stores, the strict reader reads:
	// the two sets of rules are the same set.
	repo := &threadPolicyRepo{}
	settings := &SettingService{settingRepo: repo}
	accepted := DefaultBetaPolicySettings()
	for _, action := range []string{BetaPolicyActionPass, BetaPolicyActionFilter, BetaPolicyActionBlock} {
		for _, scope := range []string{BetaPolicyScopeAll, BetaPolicyScopeOAuth, BetaPolicyScopeAPIKey, BetaPolicyScopeBedrock} {
			accepted.Rules = append(accepted.Rules, BetaPolicyRule{BetaToken: "b-" + action + "-" + scope, Action: action, Scope: scope,
				ModelWhitelist: []string{"claude-*"}, FallbackAction: action, ErrorMessage: "m"})
		}
	}
	require.NoError(t, settings.SetBetaPolicySettings(ctx, accepted))
	repo.value = repo.set
	read, err := settings.readBetaPolicyStrict(ctx)
	require.NoError(t, err)
	require.Equal(t, accepted, read)
	for _, refused := range []BetaPolicyRule{
		{BetaToken: "b", Action: "deny", Scope: BetaPolicyScopeAll},
		{BetaToken: "b", Action: BetaPolicyActionBlock, Scope: "everyone"},
		{BetaToken: "", Action: BetaPolicyActionBlock, Scope: BetaPolicyScopeAll},
		{BetaToken: "b", Action: BetaPolicyActionPass, Scope: BetaPolicyScopeAll, FallbackAction: "deny"},
	} {
		require.Error(t, settings.SetBetaPolicySettings(ctx, &BetaPolicySettings{Rules: []BetaPolicyRule{refused}}))
		raw, _ := json.Marshal(BetaPolicySettings{Rules: []BetaPolicyRule{refused}})
		repo.value = string(raw)
		_, err := settings.readBetaPolicyStrict(ctx)
		require.Error(t, err)
	}

	// No settings service at all is not a failed read.
	thread := &MessageThread{}
	require.NoError(t, (&GatewayService{}).loadMessageThreadPolicy(ctx, thread))
	require.True(t, thread.attempt.policyLoaded)
}

type threadSessionStore struct {
	SessionLimitCache
	allowed bool
	err     error
	calls   int
}

func (s *threadSessionStore) RegisterSession(context.Context, int64, string, int, time.Duration) (bool, error) {
	s.calls++
	return s.allowed, s.err
}

// The four answers of a session limit stay four answers for a thread request,
// and the ordinary check keeps folding a failure into "room".
func TestMessageThreadSessionAdmission(t *testing.T) {
	ctx := context.Background()
	limited := &Account{ID: 7, Platform: PlatformAnthropic, Type: AccountTypeOAuth, Extra: map[string]any{"max_sessions": 2}}
	unlimited := &Account{ID: 8, Platform: PlatformAnthropic, Type: AccountTypeOAuth}
	apiKey := &Account{ID: 9, Platform: PlatformAnthropic, Type: AccountTypeAPIKey, Extra: map[string]any{"max_sessions": 2}}
	down := errors.New("redis: connection refused")

	for name, tc := range map[string]struct {
		account *Account
		session string
		store   *threadSessionStore
		want    limitAdmission
		asked   int
	}{
		"room":                     {limited, "s", &threadSessionStore{allowed: true}, limitAllowed, 1},
		"no room":                  {limited, "s", &threadSessionStore{}, limitDenied, 1},
		"store fails":              {limited, "s", &threadSessionStore{err: down}, limitUnavailable, 1},
		"store fails and says yes": {limited, "s", &threadSessionStore{allowed: true, err: down}, limitUnavailable, 1},
		"no store":                 {limited, "s", nil, limitUnavailable, 0},
		"no limit configured":      {unlimited, "s", &threadSessionStore{err: down}, limitNotConfigured, 0},
		"no limit, no store":       {unlimited, "s", nil, limitNotConfigured, 0},
		"an API key has no limit":  {apiKey, "s", &threadSessionStore{err: down}, limitNotConfigured, 0},
		"no session to count":      {limited, "", &threadSessionStore{err: down}, limitNotConfigured, 0},
		"no session, no store":     {limited, "", nil, limitNotConfigured, 0},
	} {
		svc := &GatewayService{}
		if tc.store != nil {
			svc.sessionLimitCache = tc.store
		}
		got, err := svc.admitThreadSession(ctx, tc.account, tc.session)
		require.Equal(t, tc.want, got, name)
		require.Equal(t, tc.want == limitUnavailable, err != nil, name)
		if tc.store != nil {
			require.Equal(t, tc.asked, tc.store.calls, name)
		}

		// The continue request's final registration.
		thread := &MessageThread{session: tc.session, parent: &MessageThreadBinding{}}
		admitErr := svc.registerThreadSessionForSend(ctx, thread, tc.account)
		switch tc.want {
		case limitDenied:
			require.Equal(t, "thread_not_found", threadTestError(t, admitErr).Code, name)
		case limitUnavailable:
			require.Equal(t, http.StatusServiceUnavailable, threadTestError(t, admitErr).Status, name)
			require.Empty(t, threadTestError(t, admitErr).Code, name)
		default:
			require.NoError(t, admitErr, name)
		}

		// Selection must not create or refresh a slot. Only the final
		// thread admission above can grant a send. Ordinary traffic retains
		// its eager check and its existing store-error behavior.
		if tc.store != nil {
			tc.store.calls = 0
		}
		require.True(t, svc.checkAndRegisterSession(thread.Context(ctx), tc.account, tc.session), name)
		if tc.store != nil {
			require.Zero(t, tc.store.calls, name)
		}
		require.Equal(t, tc.want != limitDenied, svc.checkAndRegisterSession(ctx, tc.account, tc.session), "%s: a request without a thread", name)
	}
}

// The stored form of a binding: every field on every record, one version.
func TestMessageThreadBindingRecord(t *testing.T) {
	binding := threadTestBinding()
	binding.ClientFixed = threadFixedFieldDigests([]byte(`{"tool_choice":{"type":"auto"}}`))
	raw, err := EncodeMessageThreadBinding(binding)
	require.NoError(t, err)
	var fields map[string]json.RawMessage
	require.NoError(t, json.Unmarshal(raw, &fields))
	require.Len(t, fields, len(messageThreadRecordFields))
	for _, name := range messageThreadRecordFields {
		require.Contains(t, fields, name)
	}
	require.NotContains(t, string(raw), "null")
	decoded, err := DecodeMessageThreadBinding(raw)
	require.NoError(t, err)
	require.Equal(t, binding, *decoded)

	// A binding with no fixed fields and nothing optional set still writes them.
	minimal := threadTestBinding()
	minimal.ClientFixed = nil
	raw, err = EncodeMessageThreadBinding(minimal)
	require.NoError(t, err)
	require.Contains(t, string(raw), `"cf":{}`)
	require.Contains(t, string(raw), `"mi":false`)
	require.Contains(t, string(raw), `"s":""`)
	decoded, err = DecodeMessageThreadBinding(raw)
	require.NoError(t, err)
	require.True(t, decoded.Valid())
	require.NotNil(t, decoded.ClientFixed)

	// What this version would not read back, it does not write.
	for name, damage := range map[string]func(*MessageThreadBinding){
		"no client beta digest": func(b *MessageThreadBinding) { b.ClientBeta = "" },
		"no identity":           func(b *MessageThreadBinding) { b.Identity = "" },
		"no context size":       func(b *MessageThreadBinding) { b.ContextTokens = 0 },
		"no thread beta":        func(b *MessageThreadBinding) { b.Beta = "other" },
	} {
		bad := threadTestBinding()
		damage(&bad)
		_, err := EncodeMessageThreadBinding(bad)
		require.Error(t, err, name)
	}

	raw, err = EncodeMessageThreadBinding(binding)
	require.NoError(t, err)
	require.NoError(t, json.Unmarshal(raw, &fields))
	rewrite := func(change func(map[string]json.RawMessage)) []byte {
		copied := map[string]json.RawMessage{}
		for name, value := range fields {
			copied[name] = value
		}
		change(copied)
		encoded, err := json.Marshal(copied)
		require.NoError(t, err)
		return encoded
	}
	for _, name := range messageThreadRecordFields {
		_, err := DecodeMessageThreadBinding(rewrite(func(f map[string]json.RawMessage) { delete(f, name) }))
		require.Error(t, err, "without %s", name)
		_, err = DecodeMessageThreadBinding(rewrite(func(f map[string]json.RawMessage) { f[name] = json.RawMessage("null") }))
		require.Error(t, err, "%s null", name)
		// One field less and one unknown field more is still not a record.
		_, err = DecodeMessageThreadBinding(rewrite(func(f map[string]json.RawMessage) { delete(f, name); f["zz"] = json.RawMessage("1") }))
		require.Error(t, err, "%s replaced", name)
	}
	for name, record := range map[string][]byte{
		"an unknown field": rewrite(func(f map[string]json.RawMessage) { f["zz"] = json.RawMessage("1") }),
		"another version":  rewrite(func(f map[string]json.RawMessage) { f["v"] = json.RawMessage("3") }),
		"a flag as text":   rewrite(func(f map[string]json.RawMessage) { f["ht"] = json.RawMessage(`"yes"`) }),
		"fields as a list": rewrite(func(f map[string]json.RawMessage) { f["cf"] = json.RawMessage(`[]`) }),
		"the old record":   []byte(`{"a":7,"i":"` + strings.Repeat("a", 64) + `","m":"m","b":"` + messageThreadsBeta + `","w":"` + strings.Repeat("b", 32) + `","cm":"m","t":5}`),
		"an empty object":  []byte(`{}`),
		"not an object":    []byte(`[]`),
		"not json":         []byte(`conflict?`),
		"two records":      append(append([]byte{}, raw...), raw...),
	} {
		_, err := DecodeMessageThreadBinding(record)
		require.Error(t, err, name)
	}
}

type threadRPMStore struct {
	RPMCache
	count int
	err   error
	reads int
}

func (s *threadRPMStore) GetRPM(context.Context, int64) (int, error) {
	s.reads++
	return s.count, s.err
}

type threadCostCache struct {
	SessionLimitCache
	cost   float64
	hit    bool
	err    error
	reads  int
	stored *float64
}

func (c *threadCostCache) GetWindowCost(context.Context, int64) (float64, bool, error) {
	c.reads++
	return c.cost, c.hit, c.err
}

func (c *threadCostCache) SetWindowCost(_ context.Context, _ int64, cost float64) error {
	c.stored = &cost
	return c.err
}

type threadCostState struct {
	UsageLogRepository
	costs map[int64]float64
	err   error
	reads int
}

func (r *threadCostState) GetAccountWindowStats(_ context.Context, id int64, _ time.Time) (*usagestats.AccountStats, error) {
	r.reads++
	return &usagestats.AccountStats{StandardCost: r.costs[id]}, r.err
}

// The two soft limits of a subscription account, read for one account. The
// prefetched values the scheduler works with are in the context of every
// call here and say "nothing used"; they are not what is read.
func TestMessageThreadRateAndCostAdmission(t *testing.T) {
	down := errors.New("connection refused")
	oauth := func(extra map[string]any) *Account {
		return &Account{ID: 7, Platform: PlatformAnthropic, Type: AccountTypeOAuth, Extra: extra}
	}
	painted := context.WithValue(context.Background(), rpmPrefetchContextKey, map[int64]int{7: 0})
	painted = context.WithValue(painted, windowCostPrefetchContextKey, map[int64]float64{7: 0})

	rpm := map[string]any{"base_rpm": 10, "rpm_sticky_buffer": 5}
	for name, tc := range map[string]struct {
		account         *Account
		store           *threadRPMStore
		fresh, reserved limitAdmission
		reads           int
	}{
		"not configured":             {oauth(nil), &threadRPMStore{err: down}, limitNotConfigured, limitNotConfigured, 0},
		"not configured, no store":   {oauth(nil), nil, limitNotConfigured, limitNotConfigured, 0},
		"an API key has no limit":    {&Account{ID: 7, Platform: PlatformAnthropic, Type: AccountTypeAPIKey, Extra: rpm}, &threadRPMStore{err: down}, limitNotConfigured, limitNotConfigured, 0},
		"no counter this minute":     {oauth(rpm), &threadRPMStore{}, limitAllowed, limitAllowed, 1},
		"below the limit":            {oauth(rpm), &threadRPMStore{count: 9}, limitAllowed, limitAllowed, 1},
		"at the limit":               {oauth(rpm), &threadRPMStore{count: 10}, limitDenied, limitAllowed, 1},
		"below the ceiling":          {oauth(rpm), &threadRPMStore{count: 14}, limitDenied, limitAllowed, 1},
		"at the ceiling":             {oauth(rpm), &threadRPMStore{count: 15}, limitDenied, limitDenied, 1},
		"the counter cannot be read": {oauth(rpm), &threadRPMStore{err: down}, limitUnavailable, limitUnavailable, 1},
		"an error with a number":     {oauth(rpm), &threadRPMStore{count: 3, err: down}, limitUnavailable, limitUnavailable, 1},
		"no store":                   {oauth(rpm), nil, limitUnavailable, limitUnavailable, 0},
	} {
		for _, reserved := range []bool{false, true} {
			svc := &GatewayService{}
			if tc.store != nil {
				tc.store.reads = 0
				svc.rpmCache = tc.store
			}
			got, err := svc.admitThreadRPM(painted, tc.account, reserved)
			want := tc.fresh
			if reserved {
				want = tc.reserved
			}
			require.Equal(t, want, got, "%s reserved=%t", name, reserved)
			require.Equal(t, want == limitUnavailable, err != nil, name)
			if tc.store != nil {
				require.Equal(t, tc.reads, tc.store.reads, name)
			}
			// The scheduler's own check is what it was: it trusts the prefetch.
			require.True(t, svc.isAccountSchedulableForRPM(painted, tc.account, reserved), name)
		}
	}

	cost := map[string]any{"window_cost_limit": 100.0, "window_cost_sticky_reserve": 10.0}
	for name, tc := range map[string]struct {
		account         *Account
		cache           *threadCostCache
		state           UsageLogRepository
		fresh, reserved limitAdmission
		stateReads      int
		cached          float64 // written back to the cache; -1: nothing
	}{
		"not configured":                   {oauth(nil), &threadCostCache{err: down}, &threadCostState{err: down}, limitNotConfigured, limitNotConfigured, 0, -1},
		"not configured, nothing behind":   {oauth(nil), nil, nil, limitNotConfigured, limitNotConfigured, 0, -1},
		"cached, below the limit":          {oauth(cost), &threadCostCache{cost: 99, hit: true}, &threadCostState{err: down}, limitAllowed, limitAllowed, 0, -1},
		"cached zero":                      {oauth(cost), &threadCostCache{hit: true}, nil, limitAllowed, limitAllowed, 0, -1},
		"cached, at the limit":             {oauth(cost), &threadCostCache{cost: 100, hit: true}, nil, limitDenied, limitAllowed, 0, -1},
		"cached, at the ceiling":           {oauth(cost), &threadCostCache{cost: 110, hit: true}, nil, limitDenied, limitDenied, 0, -1},
		"miss, the state answers":          {oauth(cost), &threadCostCache{}, &threadCostState{costs: map[int64]float64{7: 42}}, limitAllowed, limitAllowed, 1, 42},
		"miss, the state has no row":       {oauth(cost), &threadCostCache{}, &threadCostState{costs: map[int64]float64{}}, limitAllowed, limitAllowed, 1, 0},
		"miss, the state says ceiling":     {oauth(cost), &threadCostCache{}, &threadCostState{costs: map[int64]float64{7: 500}}, limitDenied, limitDenied, 1, 500},
		"miss, the state says kept zone":   {oauth(cost), &threadCostCache{}, &threadCostState{costs: map[int64]float64{7: 105}}, limitDenied, limitAllowed, 1, 105},
		"cache fails, the state answers":   {oauth(cost), &threadCostCache{err: down}, &threadCostState{costs: map[int64]float64{7: 42}}, limitAllowed, limitAllowed, 1, 42},
		"no cache, the state answers":      {oauth(cost), nil, &threadCostState{costs: map[int64]float64{7: 105}}, limitDenied, limitAllowed, 1, -1},
		"miss, the state fails":            {oauth(cost), &threadCostCache{}, &threadCostState{err: down}, limitUnavailable, limitUnavailable, 1, -1},
		"cache fails, the state fails":     {oauth(cost), &threadCostCache{err: down}, &threadCostState{err: down}, limitUnavailable, limitUnavailable, 1, -1},
		"a cached number with an error":    {oauth(cost), &threadCostCache{cost: 1, hit: true, err: down}, &threadCostState{err: down}, limitUnavailable, limitUnavailable, 1, -1},
		"miss, no usage repository at all": {oauth(cost), &threadCostCache{}, nil, limitUnavailable, limitUnavailable, 0, -1},
		"nothing behind the limit":         {oauth(cost), nil, nil, limitUnavailable, limitUnavailable, 0, -1},
	} {
		for _, reserved := range []bool{false, true} {
			svc := &GatewayService{usageLogRepo: tc.state}
			if tc.cache != nil {
				tc.cache.stored = nil
				svc.sessionLimitCache = tc.cache
			}
			state, _ := tc.state.(*threadCostState)
			if state != nil {
				state.reads = 0
			}
			got, err := svc.admitThreadWindowCost(painted, tc.account, reserved)
			want := tc.fresh
			if reserved {
				want = tc.reserved
			}
			require.Equal(t, want, got, "%s reserved=%t", name, reserved)
			require.Equal(t, want == limitUnavailable, err != nil, name)
			if state != nil {
				require.Equal(t, tc.stateReads, state.reads, name)
			}
			if tc.cache != nil {
				if tc.cached < 0 {
					require.Nil(t, tc.cache.stored, name)
				} else {
					require.NotNil(t, tc.cache.stored, name)
					require.Equal(t, tc.cached, *tc.cache.stored, name)
				}
			}
			require.True(t, svc.isAccountSchedulableForWindowCost(painted, tc.account, reserved), "%s: the scheduler trusts its prefetch", name)
		}
	}

	// The admission on the selected account: what each answer means for a
	// create request and for a continue request.
	binding := threadTestBinding()
	limited := oauth(rpm)
	limited.ID = binding.AccountID
	for name, tc := range map[string]struct {
		store              *threadRPMStore
		sticky             int64
		create, onContinue string // "", "pass over", "404", "503"
		fault, denied      bool
	}{
		"room":                              {&threadRPMStore{count: 1}, 0, "", "", false, false},
		"kept zone, someone else's session": {&threadRPMStore{count: 12}, 99, "pass over", "", false, true},
		"kept zone, this account's session": {&threadRPMStore{count: 12}, binding.AccountID, "", "", false, false},
		"ceiling":                           {&threadRPMStore{count: 15}, binding.AccountID, "pass over", "404", false, true},
		"unreadable":                        {&threadRPMStore{err: down}, binding.AccountID, "pass over", "503", true, false},
	} {
		svc := &GatewayService{rpmCache: tc.store}
		check := func(thread *MessageThread, want string) {
			t.Helper()
			thread.SetStickyAccount(tc.sticky)
			err := svc.admitMessageThreadLimits(painted, thread, limited)
			switch want {
			case "":
				require.NoError(t, err, name)
			case "pass over":
				require.True(t, IsMessageThreadAccountPassedOver(err), name)
				require.False(t, IsMessageThreadAccountIncapable(err), name)
			case "404":
				require.Equal(t, "thread_not_found", threadTestError(t, err).Code, name)
				require.False(t, IsMessageThreadAccountPassedOver(err), name)
			case "503":
				require.Equal(t, http.StatusServiceUnavailable, threadTestError(t, err).Status, name)
				require.Empty(t, threadTestError(t, err).Code, name)
				require.False(t, IsMessageThreadAccountPassedOver(err), name)
			}
		}
		created := &MessageThread{}
		check(created, tc.create)
		require.Equal(t, tc.fault, created.AdmissionFault(), name)
		require.Equal(t, tc.denied, created.LimitDenied(), name)
		continued := &MessageThread{parent: &binding, parentID: "msg_1"}
		check(continued, tc.onContinue)
		require.False(t, continued.AdmissionFault(), "%s: a continue request is answered at once", name)
	}
	require.NoError(t, (&GatewayService{}).admitMessageThreadLimits(painted, nil, limited))
}
