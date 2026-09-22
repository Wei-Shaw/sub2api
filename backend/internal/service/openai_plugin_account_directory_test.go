package service

import (
	"context"
	"encoding/json"
	"reflect"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// pluginDirRepoStub embeds AccountRepository so it satisfies the full interface;
// only ListByPlatform is implemented (the sole method the directory listing uses).
// Any other call panics, which keeps the test honest about the surface it touches.
type pluginDirRepoStub struct {
	AccountRepository
	byPlatform map[string][]Account
}

func (r *pluginDirRepoStub) ListByPlatform(_ context.Context, platform string) ([]Account, error) {
	return r.byPlatform[platform], nil
}

func TestListPluginAccounts_ScopeAndSchedulable(t *testing.T) {
	future := time.Now().Add(time.Hour)
	parentID := int64(1)
	openai := []Account{
		// schedulable active oauth. Credentials hold secrets (stripped); Extra is
		// released.
		{ID: 1, Platform: PlatformOpenAI, Type: AccountTypeOAuth, Status: StatusActive, Schedulable: true,
			Name:        "primary",
			Credentials: map[string]any{"access_token": "SECRET-TOKEN", "refresh_token": "SECRET-REFRESH"},
			Extra:       map[string]any{"existing_key": "ek-value", "openai_compact_mode": "auto"}},
		// active but temp-unschedulable (paused) — status stays active, must still be
		// returned, but not schedulable
		{ID: 2, Platform: PlatformOpenAI, Type: AccountTypeOAuth, Status: StatusActive, Schedulable: true,
			TempUnschedulableUntil: &future, TempUnschedulableReason: "429 from upstream"},
		// active but rate-limited (429) — status stays active, returned, not schedulable
		{ID: 3, Platform: PlatformOpenAI, Type: AccountTypeOAuth, Status: StatusActive, Schedulable: true,
			RateLimitResetAt: &future},
		// shadow oauth — excluded entirely
		{ID: 4, Platform: PlatformOpenAI, Type: AccountTypeOAuth, Status: StatusActive, Schedulable: true,
			ParentAccountID: &parentID},
		// apikey type — out of (openai, oauth) scope, excluded
		{ID: 5, Platform: PlatformOpenAI, Type: AccountTypeAPIKey, Status: StatusActive, Schedulable: true},
	}
	svc := &OpenAIGatewayService{accountRepo: &pluginDirRepoStub{byPlatform: map[string][]Account{PlatformOpenAI: openai}}}
	scope := newPluginAccountScope(pluginAccountScopeEntry{Platform: PlatformOpenAI, AccountType: AccountTypeOAuth})

	infos, err := svc.ListPluginAccounts(context.Background(), scope, "", "")
	require.NoError(t, err)

	got := map[int64]PluginAccountInfo{}
	for _, info := range infos {
		got[info.ID] = info
	}
	// 1,2,3 in scope; 4 shadow and 5 apikey excluded.
	require.Len(t, infos, 3)
	assert.Contains(t, got, int64(1))
	assert.Contains(t, got, int64(2))
	assert.Contains(t, got, int64(3))
	assert.NotContains(t, got, int64(4), "shadow account must be excluded")
	assert.NotContains(t, got, int64(5), "out-of-scope account type must be excluded")

	// Host-authoritative schedulable decision.
	assert.True(t, got[1].Schedulable, "active oauth is schedulable")
	assert.False(t, got[2].Schedulable, "temp-unschedulable account is not schedulable")
	assert.False(t, got[3].Schedulable, "rate-limited account is not schedulable")

	// Metadata carries readable info (incl. Extra) but NEVER the raw Credentials blob.
	meta := string(got[1].MetadataJSON)
	assert.NotContains(t, meta, "SECRET-TOKEN", "credentials must never appear in metadata")
	assert.NotContains(t, meta, "SECRET-REFRESH", "credentials must never appear in metadata")
	assert.Contains(t, meta, "ek-value", "Extra is intentionally released")
	assert.Contains(t, meta, "openai_compact_mode", "Extra is intentionally released")
	assert.Contains(t, meta, "primary", "readable name must be present in metadata")
	assert.Contains(t, string(got[2].MetadataJSON), "429 from upstream", "pause reason must be readable in metadata")
}

// TestAccountReadableSnapshot_DenylistTripwire fails whenever a new EXPORTED field
// is added to Account without being classified as either safe-to-expose or
// stripped by accountReadableSnapshotJSON. Because the snapshot is a denylist, a
// newly added secret-bearing field would otherwise silently ship to plugins. When
// this test fails: add the field to `stripped` (and zero it in
// accountReadableSnapshotJSON) if it can hold secrets/heavy data, otherwise add it
// to `safeToExpose`.
func TestAccountReadableSnapshot_DenylistTripwire(t *testing.T) {
	// Fields the snapshot intentionally strips. Credentials = long-lived secret
	// (refresh_token) not handed out by ResolveOutboundIdentity. Groups/AccountGroups
	// = relational graphs with back-references that would cycle under encoding/json.
	stripped := map[string]struct{}{
		"Credentials": {}, "Groups": {}, "AccountGroups": {},
	}
	// Fields intentionally exposed as readable metadata (incl. Extra and Proxy —
	// the proxy password is already handed out via ResolveOutboundIdentity's URL).
	safeToExpose := map[string]struct{}{
		"ID": {}, "Name": {}, "Notes": {}, "Platform": {}, "Type": {}, "Extra": {},
		"Proxy": {}, "ProxyID": {}, "ProxyFallbackOriginID": {}, "ProxyFallbackOriginName": {},
		"Concurrency": {}, "Priority": {}, "RateMultiplier": {}, "GroupRateMultiplier": {}, "LoadFactor": {},
		"Status": {}, "ErrorMessage": {}, "LastUsedAt": {}, "ExpiresAt": {},
		"AutoPauseOnExpired": {}, "CreatedAt": {}, "UpdatedAt": {}, "Schedulable": {},
		"RateLimitedAt": {}, "RateLimitResetAt": {}, "OverloadUntil": {},
		"TempUnschedulableUntil": {}, "TempUnschedulableReason": {},
		"SessionWindowStart": {}, "SessionWindowEnd": {}, "SessionWindowStatus": {},
		"ParentAccountID": {}, "QuotaDimension": {}, "GroupIDs": {},
	}
	tp := reflect.TypeOf(Account{})
	for i := 0; i < tp.NumField(); i++ {
		f := tp.Field(i)
		if f.PkgPath != "" {
			continue // unexported: never marshaled by encoding/json
		}
		_, isStripped := stripped[f.Name]
		_, isSafe := safeToExpose[f.Name]
		if !isStripped && !isSafe {
			t.Fatalf("Account.%s is a new exported field not classified for the plugin snapshot: "+
				"add it to accountReadableSnapshotJSON's denylist (if it holds secrets or is a "+
				"cyclic/heavy relation) or to safeToExpose (if it is non-secret readable metadata)", f.Name)
		}
	}

	// The raw Credentials blob must never serialize; Extra and the proxy ARE released.
	acct := &Account{
		ID: 1, Platform: PlatformOpenAI, Type: AccountTypeOAuth, Status: StatusActive,
		Credentials: map[string]any{"access_token": "AT", "refresh_token": "LEAK-REFRESH"},
		Extra:       map[string]any{"opaque": "extra-released"},
		Proxy:       &Proxy{Host: "host", Port: 1, Username: "user", Password: "pw-released"},
	}
	snap := accountReadableSnapshotJSON(acct)
	require.NotNil(t, snap)
	var m map[string]any
	require.NoError(t, json.Unmarshal(snap, &m))
	assert.NotContains(t, string(snap), "LEAK-REFRESH", "raw Credentials must never appear in metadata")
	assert.Contains(t, string(snap), "extra-released", "Extra is intentionally released")
	assert.Contains(t, string(snap), "pw-released", "proxy is intentionally released (already exposed via 打票)")

	// Cycle safety: a populated Groups/AccountGroups back-reference cycle must NOT
	// crash json.Marshal (encoding/json does not detect cycles). Stripping them
	// guarantees the snapshot still returns valid JSON instead of stack-overflowing.
	g := &Group{ID: 7, Name: "g7"}
	ag := AccountGroup{GroupID: 7, Group: g, Account: acct}
	g.AccountGroups = []AccountGroup{ag} // g -> ag -> g  (and ag -> acct -> ...)
	acct.Groups = []*Group{g}
	acct.AccountGroups = []AccountGroup{ag}
	cyc := accountReadableSnapshotJSON(acct)
	require.NotNil(t, cyc, "snapshot must survive a cyclic Groups/AccountGroups graph")
	require.NoError(t, json.Unmarshal(cyc, &m))

	// Shallow-copy safety: the source account must be untouched.
	assert.NotNil(t, acct.Credentials, "snapshot must not mutate the source account")
	assert.NotNil(t, acct.Proxy, "snapshot must not mutate the source account")
	assert.Len(t, acct.Groups, 1, "snapshot must not mutate the source account's Groups")
}

func TestListPluginAccounts_ExcludesNonActiveDefenseInDepth(t *testing.T) {
	// Even if a repo were to return a non-active account, the directory must not
	// expose it (defense-in-depth beyond ListByPlatform's DB filter).
	svc := &OpenAIGatewayService{accountRepo: &pluginDirRepoStub{byPlatform: map[string][]Account{
		PlatformOpenAI: {
			{ID: 1, Platform: PlatformOpenAI, Type: AccountTypeOAuth, Status: StatusActive, Schedulable: true},
			{ID: 2, Platform: PlatformOpenAI, Type: AccountTypeOAuth, Status: StatusDisabled},
			{ID: 3, Platform: PlatformOpenAI, Type: AccountTypeOAuth, Status: StatusError},
		},
	}}}
	scope := newPluginAccountScope(pluginAccountScopeEntry{Platform: PlatformOpenAI, AccountType: AccountTypeOAuth})
	infos, err := svc.ListPluginAccounts(context.Background(), scope, "", "")
	require.NoError(t, err)
	require.Len(t, infos, 1)
	assert.Equal(t, int64(1), infos[0].ID)
}

func TestListPluginAccounts_EmptyScopeReturnsNothing(t *testing.T) {
	svc := &OpenAIGatewayService{accountRepo: &pluginDirRepoStub{byPlatform: map[string][]Account{
		PlatformOpenAI: {{ID: 1, Platform: PlatformOpenAI, Type: AccountTypeOAuth, Status: StatusActive, Schedulable: true}},
	}}}
	infos, err := svc.ListPluginAccounts(context.Background(), PluginAccountScope{}, "", "")
	require.NoError(t, err)
	assert.Empty(t, infos, "an empty scope must never enumerate accounts")
}
