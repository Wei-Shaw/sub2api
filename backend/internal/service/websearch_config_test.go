package service

import (
	"testing"

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
			{Type: "brave", Priority: 1, QuotaLimit: 1000, QuotaRefreshInterval: "monthly"REDACTED,
			{Type: "tavily", Priority: 2, QuotaLimit: 500, QuotaRefreshInterval: "daily"REDACTED,
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

func TestValidateWebSearchConfig_InvalidQuotaInterval(t *testing.T) {
	cfg := &WebSearchEmulationConfig{
		Providers: []WebSearchProviderConfig{{Type: "brave", QuotaRefreshInterval: "hourly"REDACTEDREDACTED,
REDACTED
	require.ErrorContains(t, validateWebSearchConfig(cfg), "invalid quota_refresh_interval")
REDACTED

func TestValidateWebSearchConfig_NegativeQuotaLimit(t *testing.T) {
	cfg := &WebSearchEmulationConfig{
		Providers: []WebSearchProviderConfig{{Type: "brave", QuotaLimit: -1REDACTEDREDACTED,
REDACTED
	require.ErrorContains(t, validateWebSearchConfig(cfg), "quota_limit must be >= 0")
REDACTED

func TestValidateWebSearchConfig_DuplicateType(t *testing.T) {
	cfg := &WebSearchEmulationConfig{
		Providers: []WebSearchProviderConfig{
			{Type: "brave", Priority: 1REDACTED,
			{Type: "brave", Priority: 2REDACTED,
	REDACTED,
REDACTED
	require.ErrorContains(t, validateWebSearchConfig(cfg), "duplicate type")
REDACTED

func TestValidateWebSearchConfig_EmptyQuotaInterval(t *testing.T) {
	cfg := &WebSearchEmulationConfig{
		Providers: []WebSearchProviderConfig{{Type: "brave", QuotaRefreshInterval: ""REDACTEDREDACTED,
REDACTED
	require.NoError(t, validateWebSearchConfig(cfg))
REDACTED

func TestValidateWebSearchConfig_ZeroQuotaLimit(t *testing.T) {
	cfg := &WebSearchEmulationConfig{
		Providers: []WebSearchProviderConfig{{Type: "brave", QuotaLimit: 0REDACTEDREDACTED,
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

// --- SanitizeWebSearchConfig ---

func TestSanitizeWebSearchConfig_MaskAPIKey(t *testing.T) {
	cfg := &WebSearchEmulationConfig{
		Enabled: true,
		Providers: []WebSearchProviderConfig{
			{Type: "brave", APIKey: "sk-secret-xxx"REDACTED,
	REDACTED,
REDACTED
	out := SanitizeWebSearchConfig(cfg)
	require.Equal(t, "", out.Providers[0].APIKey)
	require.True(t, out.Providers[0].APIKeyConfigured)
REDACTED

func TestSanitizeWebSearchConfig_NoAPIKey(t *testing.T) {
	cfg := &WebSearchEmulationConfig{
		Providers: []WebSearchProviderConfig{{Type: "brave", APIKey: ""REDACTEDREDACTED,
REDACTED
	out := SanitizeWebSearchConfig(cfg)
	require.Equal(t, "", out.Providers[0].APIKey)
	require.False(t, out.Providers[0].APIKeyConfigured)
REDACTED

func TestSanitizeWebSearchConfig_Nil(t *testing.T) {
	require.Nil(t, SanitizeWebSearchConfig(nil))
REDACTED

func TestSanitizeWebSearchConfig_PreservesOtherFields(t *testing.T) {
	cfg := &WebSearchEmulationConfig{
		Enabled: true,
		Providers: []WebSearchProviderConfig{
			{Type: "brave", APIKey: "secret", Priority: 10, QuotaLimit: 1000REDACTED,
	REDACTED,
REDACTED
	out := SanitizeWebSearchConfig(cfg)
	require.True(t, out.Enabled)
	require.Equal(t, 10, out.Providers[0].Priority)
	require.Equal(t, int64(1000), out.Providers[0].QuotaLimit)
REDACTED

func TestSanitizeWebSearchConfig_DoesNotMutateOriginal(t *testing.T) {
	cfg := &WebSearchEmulationConfig{
		Providers: []WebSearchProviderConfig{{Type: "brave", APIKey: "secret"REDACTEDREDACTED,
REDACTED
	_ = SanitizeWebSearchConfig(cfg)
	require.Equal(t, "secret", cfg.Providers[0].APIKey)
REDACTED
