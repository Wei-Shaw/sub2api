//go:build unit

package service

import (
	"context"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/pkg/websearch"
	"github.com/stretchr/testify/require"
)

// --- validateWebSearchConfig ---

func TestValidateWebSearchConfig_Nil(t *testing.T) {
	require.NoError(t, validateWebSearchConfig(nil))
REDACTED

func TestValidateWebSearchConfig_Valid(t *testing.T) {
	cfg := &WebSearchEmulationConfig{
		Enabled: true,
		Providers: []WebSearchProviderConfig{
			{Type: "brave", QuotaLimit: int64Ptr(1000)REDACTED,
			{Type: "tavily", QuotaLimit: int64Ptr(500)REDACTED,
	REDACTED,
REDACTED
	require.NoError(t, validateWebSearchConfig(cfg))
REDACTED

func TestValidateWebSearchConfig_TooManyProviders(t *testing.T) {
	cfg := &WebSearchEmulationConfig{Providers: make([]WebSearchProviderConfig, 11)REDACTED
	for i := range cfg.Providers {
		cfg.Providers[i] = WebSearchProviderConfig{Type: "brave"REDACTED
REDACTED
	err := validateWebSearchConfig(cfg)
	require.ErrorContains(t, err, "too many providers")
REDACTED

func TestValidateWebSearchConfig_InvalidType(t *testing.T) {
	cfg := &WebSearchEmulationConfig{
		Providers: []WebSearchProviderConfig{{Type: "bing"REDACTEDREDACTED,
REDACTED
	require.ErrorContains(t, validateWebSearchConfig(cfg), "invalid type")
REDACTED

func TestValidateWebSearchConfig_NegativeQuotaLimit(t *testing.T) {
	cfg := &WebSearchEmulationConfig{
		Providers: []WebSearchProviderConfig{{Type: "brave", QuotaLimit: int64Ptr(-1)REDACTEDREDACTED,
REDACTED
	require.ErrorContains(t, validateWebSearchConfig(cfg), "quota_limit must be > 0 or null")
REDACTED

func TestValidateWebSearchConfig_DuplicateType(t *testing.T) {
	cfg := &WebSearchEmulationConfig{
		Providers: []WebSearchProviderConfig{
			{Type: "brave"REDACTED,
			{Type: "brave"REDACTED,
	REDACTED,
REDACTED
	require.ErrorContains(t, validateWebSearchConfig(cfg), "duplicate type")
REDACTED

func TestValidateWebSearchConfig_NilQuotaLimit(t *testing.T) {
	cfg := &WebSearchEmulationConfig{
		Providers: []WebSearchProviderConfig{{Type: "brave", QuotaLimit: nilREDACTEDREDACTED,
REDACTED
	require.NoError(t, validateWebSearchConfig(cfg))
REDACTED

// --- parseWebSearchConfigJSON ---

func TestParseWebSearchConfigJSON_ValidJSON(t *testing.T) {
	raw := `{"enabled":true,"providers":[{"type":"brave","api_key":"sk-xxx"REDACTED]REDACTED`
	cfg := parseWebSearchConfigJSON(raw)
	require.True(t, cfg.Enabled)
	require.Len(t, cfg.Providers, 1)
	require.Equal(t, "brave", cfg.Providers[0].Type)
REDACTED

func TestParseWebSearchConfigJSON_EmptyString(t *testing.T) {
	cfg := parseWebSearchConfigJSON("")
	require.False(t, cfg.Enabled)
	require.Empty(t, cfg.Providers)
REDACTED

func TestParseWebSearchConfigJSON_InvalidJSON(t *testing.T) {
	cfg := parseWebSearchConfigJSON("not{json")
	require.False(t, cfg.Enabled)
	require.Empty(t, cfg.Providers)
REDACTED

func TestParseWebSearchConfigJSON_BackwardCompatibility(t *testing.T) {
	// Old config with priority and quota_refresh_interval should parse without error
	raw := `{"enabled":true,"providers":[{"type":"brave","priority":1,"quota_refresh_interval":"monthly","quota_limit":1000REDACTED]REDACTED`
	cfg := parseWebSearchConfigJSON(raw)
	require.True(t, cfg.Enabled)
	require.Len(t, cfg.Providers, 1)
	require.Equal(t, int64(1000), *cfg.Providers[0].QuotaLimit)
REDACTED

// --- SanitizeWebSearchConfig ---

func TestSanitizeWebSearchConfig_MaskAPIKey(t *testing.T) {
	cfg := &WebSearchEmulationConfig{
		Enabled: true,
		Providers: []WebSearchProviderConfig{
			{Type: "brave", APIKey: "sk-secret-xxx"REDACTED,
	REDACTED,
REDACTED
	out := SanitizeWebSearchConfig(context.Background(), cfg)
	require.Equal(t, "", out.Providers[0].APIKey)
	require.True(t, out.Providers[0].APIKeyConfigured)
REDACTED

func TestSanitizeWebSearchConfig_NoAPIKey(t *testing.T) {
	cfg := &WebSearchEmulationConfig{
		Providers: []WebSearchProviderConfig{{Type: "brave", APIKey: ""REDACTEDREDACTED,
REDACTED
	out := SanitizeWebSearchConfig(context.Background(), cfg)
	require.Equal(t, "", out.Providers[0].APIKey)
	require.False(t, out.Providers[0].APIKeyConfigured)
REDACTED

func TestSanitizeWebSearchConfig_Nil(t *testing.T) {
	require.Nil(t, SanitizeWebSearchConfig(context.Background(), nil))
REDACTED

func TestSanitizeWebSearchConfig_PreservesOtherFields(t *testing.T) {
	cfg := &WebSearchEmulationConfig{
		Enabled: true,
		Providers: []WebSearchProviderConfig{
			{Type: "brave", APIKey: "secret", QuotaLimit: int64Ptr(1000)REDACTED,
	REDACTED,
REDACTED
	out := SanitizeWebSearchConfig(context.Background(), cfg)
	require.True(t, out.Enabled)
	require.Equal(t, int64(1000), *out.Providers[0].QuotaLimit)
REDACTED

func TestSanitizeWebSearchConfig_DoesNotMutateOriginal(t *testing.T) {
	cfg := &WebSearchEmulationConfig{
		Providers: []WebSearchProviderConfig{{Type: "brave", APIKey: "secret"REDACTEDREDACTED,
REDACTED
	_ = SanitizeWebSearchConfig(context.Background(), cfg)
	require.Equal(t, "secret", cfg.Providers[0].APIKey)
REDACTED

// --- PopulateWebSearchUsage ---

func TestPopulateWebSearchUsage_NilInput(t *testing.T) {
	require.Nil(t, PopulateWebSearchUsage(context.Background(), nil))
REDACTED

func TestPopulateWebSearchUsage_NoManager_QuotaUsedZero(t *testing.T) {
	// Ensure no global manager is set
	SetWebSearchManager(nil)
	defer SetWebSearchManager(nil)

	cfg := &WebSearchEmulationConfig{
		Enabled: true,
		Providers: []WebSearchProviderConfig{
			{Type: "brave", APIKey: "sk-key", QuotaLimit: int64Ptr(1000)REDACTED,
	REDACTED,
REDACTED
	out := PopulateWebSearchUsage(context.Background(), cfg)
	require.NotNil(t, out)
	require.Len(t, out.Providers, 1)
	require.Equal(t, int64(0), out.Providers[0].QuotaUsed)
REDACTED

func TestPopulateWebSearchUsage_APIKeyConfigured_True(t *testing.T) {
	SetWebSearchManager(nil)
	defer SetWebSearchManager(nil)

	cfg := &WebSearchEmulationConfig{
		Providers: []WebSearchProviderConfig{
			{Type: "brave", APIKey: "sk-key"REDACTED,
	REDACTED,
REDACTED
	out := PopulateWebSearchUsage(context.Background(), cfg)
	require.True(t, out.Providers[0].APIKeyConfigured)
REDACTED

func TestPopulateWebSearchUsage_APIKeyConfigured_False(t *testing.T) {
	SetWebSearchManager(nil)
	defer SetWebSearchManager(nil)

	cfg := &WebSearchEmulationConfig{
		Providers: []WebSearchProviderConfig{
			{Type: "brave", APIKey: ""REDACTED,
	REDACTED,
REDACTED
	out := PopulateWebSearchUsage(context.Background(), cfg)
	require.False(t, out.Providers[0].APIKeyConfigured)
REDACTED

func TestPopulateWebSearchUsage_NilQuotaLimit(t *testing.T) {
	SetWebSearchManager(nil)
	defer SetWebSearchManager(nil)

	cfg := &WebSearchEmulationConfig{
		Providers: []WebSearchProviderConfig{
			{Type: "brave", APIKey: "sk-key", QuotaLimit: nilREDACTED,
	REDACTED,
REDACTED
	out := PopulateWebSearchUsage(context.Background(), cfg)
	require.Nil(t, out.Providers[0].QuotaLimit)
REDACTED

func TestPopulateWebSearchUsage_NonNilQuotaLimit(t *testing.T) {
	SetWebSearchManager(nil)
	defer SetWebSearchManager(nil)

	cfg := &WebSearchEmulationConfig{
		Providers: []WebSearchProviderConfig{
			{Type: "brave", APIKey: "sk-key", QuotaLimit: int64Ptr(500)REDACTED,
	REDACTED,
REDACTED
	out := PopulateWebSearchUsage(context.Background(), cfg)
	require.NotNil(t, out.Providers[0].QuotaLimit)
	require.Equal(t, int64(500), *out.Providers[0].QuotaLimit)
REDACTED

func TestPopulateWebSearchUsage_WithManager_NilRedis(t *testing.T) {
	// Manager with nil Redis returns 0 usage without error
	mgr := websearch.NewManager([]websearch.ProviderConfig{
		{Type: "brave", APIKey: "k"REDACTED,
REDACTED, nil)
	SetWebSearchManager(mgr)
	defer SetWebSearchManager(nil)

	cfg := &WebSearchEmulationConfig{
		Providers: []WebSearchProviderConfig{
			{Type: "brave", APIKey: "sk-key", QuotaLimit: int64Ptr(1000)REDACTED,
	REDACTED,
REDACTED
	out := PopulateWebSearchUsage(context.Background(), cfg)
	require.Equal(t, int64(0), out.Providers[0].QuotaUsed)
	require.True(t, out.Providers[0].APIKeyConfigured)
REDACTED

func TestPopulateWebSearchUsage_DoesNotMutateOriginal(t *testing.T) {
	SetWebSearchManager(nil)
	defer SetWebSearchManager(nil)

	cfg := &WebSearchEmulationConfig{
		Providers: []WebSearchProviderConfig{
			{Type: "brave", APIKey: "secret", QuotaLimit: int64Ptr(100)REDACTED,
	REDACTED,
REDACTED
	_ = PopulateWebSearchUsage(context.Background(), cfg)
	// Original should be unchanged
	require.Equal(t, "secret", cfg.Providers[0].APIKey)
	require.Equal(t, int64(0), cfg.Providers[0].QuotaUsed)
REDACTED

// --- ResetWebSearchUsage ---

func TestResetWebSearchUsage_NilManager(t *testing.T) {
	SetWebSearchManager(nil)
	defer SetWebSearchManager(nil)

	err := ResetWebSearchUsage(context.Background(), "brave")
REDACTED
	require.Contains(t, err.Error(), "not initialized")
REDACTED
