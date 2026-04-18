package service

import (
	"context"
	"fmt"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

type identityCacheStub struct {
	maskedSessionID    string
	stickySessionUUIDs map[string]string
}

func (s *identityCacheStub) GetFingerprint(_ context.Context, _ int64) (*Fingerprint, error) {
	return nil, nil
}
func (s *identityCacheStub) SetFingerprint(_ context.Context, _ int64, _ *Fingerprint) error {
	return nil
}
func (s *identityCacheStub) GetMaskedSessionID(_ context.Context, _ int64) (string, error) {
	return s.maskedSessionID, nil
}
func (s *identityCacheStub) SetMaskedSessionID(_ context.Context, _ int64, sessionID string) error {
	s.maskedSessionID = sessionID
	return nil
}
func (s *identityCacheStub) GetStickySessionUUID(_ context.Context, accountID int64, sessionHash string) (string, error) {
	if s.stickySessionUUIDs == nil {
		return "", nil
	}
	return s.stickySessionUUIDs[stickyStubKey(accountID, sessionHash)], nil
}
func (s *identityCacheStub) SetStickySessionUUID(_ context.Context, accountID int64, sessionHash string, sessionUUID string) error {
	if s.stickySessionUUIDs == nil {
		s.stickySessionUUIDs = map[string]string{}
	}
	s.stickySessionUUIDs[stickyStubKey(accountID, sessionHash)] = sessionUUID
	return nil
}

func stickyStubKey(accountID int64, sessionHash string) string {
	return fmt.Sprintf("%d:%s", accountID, sessionHash)
}

func TestIdentityService_RewriteUserID_PreservesTopLevelFieldOrder(t *testing.T) {
	cache := &identityCacheStub{}
	svc := NewIdentityService(cache)

	originalUserID := FormatMetadataUserID(
		"d61f76d0730d2b920763648949bad5c79742155c27037fc77ac3f9805cb90169",
		"",
		"7578cf37-aaca-46e4-a45c-71285d9dbb83",
		"2.1.78",
	)
	body := []byte(`{"alpha":1,"messages":[],"metadata":{"user_id":` + strconvQuote(originalUserID) + `},"max_tokens":64000,"thinking":{"type":"adaptive"},"output_config":{"effort":"high"},"stream":true}`)

	result, err := svc.RewriteUserID(body, 123, "acc-uuid", "client-xyz", "claude-cli/2.1.78 (external, cli)")
	require.NoError(t, err)
	resultStr := string(result)

	assertJSONTokenOrder(t, resultStr, `"alpha"`, `"messages"`, `"metadata"`, `"max_tokens"`, `"thinking"`, `"output_config"`, `"stream"`)
	require.NotContains(t, resultStr, originalUserID)
	require.Contains(t, resultStr, `"metadata":{"user_id":"`)
}

func TestIdentityService_RewriteUserIDWithMasking_PreservesTopLevelFieldOrder(t *testing.T) {
	// 2026-04-15: session_id masking was removed for anti-fingerprinting
	// reasons — it forced every concurrent request on one account to share
	// the same 15-minute-cached session UUID, which is trivially detectable
	// as non-human. RewriteUserIDWithMasking is now a thin wrapper that
	// delegates to RewriteUserID (which mints a fresh random UUID per call).
	//
	// This test now verifies that:
	//   1) The wrapper still preserves top-level JSON field order.
	//   2) The cached masked session ID is **NOT** applied — even though the
	//      cache contains one and the account has masking enabled, the
	//      output session_id is a fresh UUID and does not equal the cached
	//      value.
	cache := &identityCacheStub{maskedSessionID: "11111111-2222-4333-8444-555555555555"}
	svc := NewIdentityService(cache)

	originalUserID := FormatMetadataUserID(
		"d61f76d0730d2b920763648949bad5c79742155c27037fc77ac3f9805cb90169",
		"",
		"7578cf37-aaca-46e4-a45c-71285d9dbb83",
		"2.1.78",
	)
	body := []byte(`{"alpha":1,"messages":[],"metadata":{"user_id":` + strconvQuote(originalUserID) + `},"max_tokens":64000,"thinking":{"type":"adaptive"},"output_config":{"effort":"high"},"stream":true}`)

	account := &Account{
		ID:       123,
		Platform: PlatformAnthropic,
		Type:     AccountTypeOAuth,
		Extra: map[string]any{
			"session_id_masking_enabled": true,
		},
	}

	result, err := svc.RewriteUserIDWithMasking(context.Background(), body, account, "acc-uuid", "client-xyz", "claude-cli/2.1.78 (external, cli)")
	require.NoError(t, err)
	resultStr := string(result)

	assertJSONTokenOrder(t, resultStr, `"alpha"`, `"messages"`, `"metadata"`, `"max_tokens"`, `"thinking"`, `"output_config"`, `"stream"`)
	require.NotContains(t, resultStr, "11111111-2222-4333-8444-555555555555",
		"masked session id must no longer leak through")
	require.NotContains(t, resultStr, originalUserID,
		"original session id must still be rewritten")
	require.True(t, strings.Contains(resultStr, `"metadata":{"user_id":"`))

	// Sanity: two consecutive calls on the same account must produce
	// different session ids (proving randomness, not determinism).
	result2, err := svc.RewriteUserIDWithMasking(context.Background(), body, account, "acc-uuid", "client-xyz", "claude-cli/2.1.78 (external, cli)")
	require.NoError(t, err)
	require.NotEqual(t, resultStr, string(result2), "session id must differ across calls")
}

func TestIdentityService_RewriteUserIDWithMasking_StickyPerSessionHash(t *testing.T) {
	// With a session hash carried in ctx, two consecutive rewrites must
	// produce the SAME session id (a real CLI holds one session_id across
	// the full conversation). Different session hashes → different UUIDs.
	cache := &identityCacheStub{}
	svc := NewIdentityService(cache)

	originalUserID := FormatMetadataUserID(
		"d61f76d0730d2b920763648949bad5c79742155c27037fc77ac3f9805cb90169",
		"",
		"7578cf37-aaca-46e4-a45c-71285d9dbb83",
		"2.1.78",
	)
	body := []byte(`{"alpha":1,"messages":[],"metadata":{"user_id":` + strconvQuote(originalUserID) + `},"max_tokens":64000,"stream":true}`)

	account := &Account{ID: 42, Platform: PlatformAnthropic, Type: AccountTypeOAuth}

	ctxA := WithSessionHash(context.Background(), "conversation-A")
	first, err := svc.RewriteUserIDWithMasking(ctxA, body, account, "acc-uuid", "client-xyz", "claude-cli/2.1.78 (external, cli)")
	require.NoError(t, err)
	second, err := svc.RewriteUserIDWithMasking(ctxA, body, account, "acc-uuid", "client-xyz", "claude-cli/2.1.78 (external, cli)")
	require.NoError(t, err)
	require.Equal(t, string(first), string(second), "same session hash must yield sticky session id")

	ctxB := WithSessionHash(context.Background(), "conversation-B")
	other, err := svc.RewriteUserIDWithMasking(ctxB, body, account, "acc-uuid", "client-xyz", "claude-cli/2.1.78 (external, cli)")
	require.NoError(t, err)
	require.NotEqual(t, string(first), string(other), "different session hashes must yield different session ids")
}

func strconvQuote(v string) string {
	return `"` + strings.ReplaceAll(strings.ReplaceAll(v, `\`, `\\`), `"`, `\"`) + `"`
}
