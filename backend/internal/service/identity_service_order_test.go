package service

import (
	"context"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

type identityCacheStub struct {
	maskedSessionID string
REDACTED

func (s *identityCacheStub) GetFingerprint(_ context.Context, _ int64) (*Fingerprint, error) {
	return nil, nil
REDACTED
func (s *identityCacheStub) SetFingerprint(_ context.Context, _ int64, _ *Fingerprint) error {
	return nil
REDACTED
func (s *identityCacheStub) GetMaskedSessionID(_ context.Context, _ int64) (string, error) {
	return s.maskedSessionID, nil
REDACTED
func (s *identityCacheStub) SetMaskedSessionID(_ context.Context, _ int64, sessionID string) error {
	s.maskedSessionID = sessionID
	return nil
REDACTED

func TestIdentityService_RewriteUserID_PreservesTopLevelFieldOrder(t *testing.T) {
	cache := &identityCacheStub{REDACTED
	svc := NewIdentityService(cache)

	originalUserID := FormatMetadataUserID(
		"d61f76d0730d2b920763648949bad5c79742155c27037fc77ac3f9805cb90169",
		"",
		"7578cf37-aaca-46e4-a45c-71285d9dbb83",
		"2.1.78",
	)
	body := []byte(`{"alpha":1,"messages":[],"metadata":{"user_id":` + strconvQuote(originalUserID) + `REDACTED,"max_tokens":64000,"thinking":{"type":"adaptive"REDACTED,"output_config":{"effort":"high"REDACTED,"stream":trueREDACTED`)

	result, err := svc.RewriteUserID(body, 123, "acc-uuid", "client-xyz", "claude-cli/2.1.78 (external, cli)")
REDACTED
	resultStr := string(result)

	assertJSONTokenOrder(t, resultStr, `"alpha"`, `"messages"`, `"metadata"`, `"max_tokens"`, `"thinking"`, `"output_config"`, `"stream"`)
	require.NotContains(t, resultStr, originalUserID)
	require.Contains(t, resultStr, `"metadata":{"user_id":"`)
REDACTED

func TestIdentityService_RewriteUserIDWithMasking_PreservesTopLevelFieldOrder(t *testing.T) {
	cache := &identityCacheStub{maskedSessionID: "11111111-2222-4333-8444-555555555555"REDACTED
	svc := NewIdentityService(cache)

	originalUserID := FormatMetadataUserID(
		"d61f76d0730d2b920763648949bad5c79742155c27037fc77ac3f9805cb90169",
		"",
		"7578cf37-aaca-46e4-a45c-71285d9dbb83",
		"2.1.78",
	)
	body := []byte(`{"alpha":1,"messages":[],"metadata":{"user_id":` + strconvQuote(originalUserID) + `REDACTED,"max_tokens":64000,"thinking":{"type":"adaptive"REDACTED,"output_config":{"effort":"high"REDACTED,"stream":trueREDACTED`)

	account := &Account{
		ID:       123,
		Platform: PlatformAnthropic,
		Type:     AccountTypeOAuth,
		Extra: map[string]any{
			"session_id_masking_enabled": true,
	REDACTED,
REDACTED

	result, err := svc.RewriteUserIDWithMasking(context.Background(), body, account, "acc-uuid", "client-xyz", "claude-cli/2.1.78 (external, cli)")
REDACTED
	resultStr := string(result)

	assertJSONTokenOrder(t, resultStr, `"alpha"`, `"messages"`, `"metadata"`, `"max_tokens"`, `"thinking"`, `"output_config"`, `"stream"`)
	require.Contains(t, resultStr, cache.maskedSessionID)
	require.True(t, strings.Contains(resultStr, `"metadata":{"user_id":"`))
REDACTED

func strconvQuote(v string) string {
	return `"` + strings.ReplaceAll(strings.ReplaceAll(v, `\`, `\\`), `"`, `\"`) + `"`
REDACTED
