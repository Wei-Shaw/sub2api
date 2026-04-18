//go:build unit

package service

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

// TestRewriteUserID_EquivalenceAfterRefactor verifies the helper extraction
// (extractRewriteTarget + spliceUserID) preserves the early-return behavior
// shared by RewriteUserID and RewriteUserIDWithMasking. Each case is asserted
// against both methods (the masking variant is exercised via its sessionHash=""
// fallback, which delegates to RewriteUserID — keeping us honest that both
// paths use identical guards).
func TestRewriteUserID_EquivalenceAfterRefactor(t *testing.T) {
	const (
		accountUUID    = "11111111-2222-3333-4444-555555555555"
		clientID       = "abcdef0123456789abcdef0123456789abcdef0123456789abcdef0123456789"
		fingerprintUA  = "claude-cli/2.1.112 (external, sdk-cli)"
		legacyValidUID = "user_abcdef0123456789abcdef0123456789abcdef0123456789abcdef0123456789_account_99999999-8888-7777-6666-555555555555_session_aaaaaaaa-bbbb-cccc-dddd-eeeeeeeeeeee"
	)

	cases := []struct {
		name     string
		body     []byte
		expectEq bool // true = body should be returned unchanged
	}{
		{
			name:     "empty body",
			body:     nil,
			expectEq: true,
		},
		{
			name:     "metadata is null",
			body:     []byte(`{"metadata":null,"foo":"bar"}`),
			expectEq: true,
		},
		{
			name:     "metadata is array, not object",
			body:     []byte(`{"metadata":[1,2,3]}`),
			expectEq: true,
		},
		{
			name:     "user_id missing from metadata",
			body:     []byte(`{"metadata":{"other":"value"}}`),
			expectEq: true,
		},
		{
			name:     "valid legacy user_id rewritten",
			body:     []byte(`{"metadata":{"user_id":"` + legacyValidUID + `"}}`),
			expectEq: false,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			svc := NewIdentityService(&identityCacheStub{})

			// Path 1: direct RewriteUserID
			gotDirect, err := svc.RewriteUserID(tc.body, 42, accountUUID, clientID, fingerprintUA)
			require.NoError(t, err)

			// Path 2: RewriteUserIDWithMasking with empty sessionHash → falls
			// back to RewriteUserID (same guard chain).
			account := &Account{ID: 42}
			gotMasking, err := svc.RewriteUserIDWithMasking(context.Background(), tc.body, account, accountUUID, clientID, fingerprintUA)
			require.NoError(t, err)

			if tc.expectEq {
				// Both methods must return body unchanged (same slice / same bytes).
				require.Equal(t, string(tc.body), string(gotDirect), "RewriteUserID should return body unchanged")
				require.Equal(t, string(tc.body), string(gotMasking), "RewriteUserIDWithMasking fallback should return body unchanged")
				return
			}

			// Rewrite path: metadata.user_id must change and remain a parseable
			// metadata user id with the expected device id + account uuid.
			for _, got := range [][]byte{gotDirect, gotMasking} {
				newUID := gjson.GetBytes(got, "metadata.user_id").String()
				require.NotEqual(t, legacyValidUID, newUID, "user_id must be rewritten")

				parsed := ParseMetadataUserID(newUID)
				require.NotNil(t, parsed, "rewritten user_id must be parseable")
				require.Equal(t, clientID, parsed.DeviceID)
				require.Equal(t, accountUUID, parsed.AccountUUID)
				require.NotEmpty(t, parsed.SessionID)
			}
		})
	}
}

// TestRewriteUserIDWithMasking_StickyPath_EarlyReturns covers the sticky
// session path (sessionHash != ""), asserting that the same early-return
// guards apply: empty body, non-object metadata, missing user_id, and
// unparseable user_id all return the body unchanged byte-for-byte.
//
// This complements TestRewriteUserID_EquivalenceAfterRefactor, which only
// exercises the sessionHash="" fallback (delegates to RewriteUserID).
func TestRewriteUserIDWithMasking_StickyPath_EarlyReturns(t *testing.T) {
	cases := []struct {
		name string
		body []byte
	}{
		{"empty body", nil},
		{"metadata=null", []byte(`{"metadata":null}`)},
		{"metadata=array", []byte(`{"metadata":[1,2,3]}`)},
		{"missing user_id", []byte(`{"metadata":{}}`)},
		{"unparseable user_id", []byte(`{"metadata":{"user_id":"not-a-valid-uid"}}`)},
	}

	cache := &identityCacheStub{}
	svc := NewIdentityService(cache)
	account := &Account{ID: 42, Platform: PlatformAnthropic, Type: AccountTypeOAuth}
	ctx := WithSessionHash(context.Background(), "test-session-hash-123")

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			result, err := svc.RewriteUserIDWithMasking(ctx, tc.body, account, "acc-uuid", "client-id", "claude-cli/2.1.112 (external, sdk-cli)")
			require.NoError(t, err)
			require.Equal(t, string(tc.body), string(result))
		})
	}
}

// TestRewriteUserIDWithMasking_StickyPath_CacheHitReusesUUID asserts that
// when sessionHash is non-empty, two consecutive rewrites with the same
// hash produce byte-identical output (the second call hits the cached
// sticky session UUID populated by the first call).
//
// This is the positive equivalence counterpart to the early-return cases
// above and to the StickyPerSessionHash test in identity_service_order_test.go.
func TestRewriteUserIDWithMasking_StickyPath_CacheHitReusesUUID(t *testing.T) {
	cache := &identityCacheStub{}
	svc := NewIdentityService(cache)
	account := &Account{ID: 42, Platform: PlatformAnthropic, Type: AccountTypeOAuth}
	ctx := WithSessionHash(context.Background(), "stable-session-h")

	// Use a real legacy-format user_id that ParseMetadataUserID accepts.
	const legacyValidUID = "user_abcdef0123456789abcdef0123456789abcdef0123456789abcdef0123456789_account_99999999-8888-7777-6666-555555555555_session_aaaaaaaa-bbbb-cccc-dddd-eeeeeeeeeeee"
	body := []byte(`{"metadata":{"user_id":"` + legacyValidUID + `"}}`)

	r1, err := svc.RewriteUserIDWithMasking(ctx, body, account, "acc-uuid", "client-id", "claude-cli/2.1.112 (external, sdk-cli)")
	require.NoError(t, err)

	r2, err := svc.RewriteUserIDWithMasking(ctx, body, account, "acc-uuid", "client-id", "claude-cli/2.1.112 (external, sdk-cli)")
	require.NoError(t, err)

	require.Equal(t, string(r1), string(r2), "sticky cache hit must yield byte-identical rewrite")

	// Sanity: a different sessionHash must produce different output.
	ctxOther := WithSessionHash(context.Background(), "different-session-h")
	rOther, err := svc.RewriteUserIDWithMasking(ctxOther, body, account, "acc-uuid", "client-id", "claude-cli/2.1.112 (external, sdk-cli)")
	require.NoError(t, err)
	require.NotEqual(t, string(r1), string(rOther), "different sessionHash must yield different sticky session id")
}
