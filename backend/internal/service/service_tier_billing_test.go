package service

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestResolveBillingServiceTier(t *testing.T) {
	tests := []struct {
		name       string
		requested  string
		observed   string
		billing    string
		downgraded bool
REDACTED{
		{name: "openai priority served as default", requested: "priority", observed: "default", billing: "default", downgraded: trueREDACTED,
		{name: "anthropic fast served as standard", requested: "fast", observed: "standard", billing: "standard", downgraded: trueREDACTED,
		{name: "priority honoured", requested: "priority", observed: "priority", billing: "priority"REDACTED,
		{name: "no declaration keeps request", requested: "priority", observed: "", billing: "priority"REDACTED,
		{name: "no request no declaration", requested: "", observed: "", billing: ""REDACTED,
		{name: "response never raises the tier", requested: "", observed: "priority", billing: ""REDACTED,
		{name: "flex never raised to default", requested: "flex", observed: "default", billing: "flex"REDACTED,
		{name: "default echoed for untiered request", requested: "", observed: "default", billing: ""REDACTED,
		{name: "unknown response tier ignored", requested: "priority", observed: "turbo", billing: "priority"REDACTED,
		{name: "case and whitespace normalised", requested: " Priority ", observed: "DEFAULT", billing: "default", downgraded: trueREDACTED,
REDACTED
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ResolveBillingServiceTier(tt.requested, tt.observed)
			require.Equal(t, tt.billing, got.Billing)
			require.Equal(t, tt.downgraded, got.Downgraded)
	REDACTED)
REDACTED
REDACTED

func TestApplyServiceTierBillingResolutionOnlyRewritesDowngrades(t *testing.T) {
	t.Run("openai downgrade rewrites tier", func(t *testing.T) {
		requested := "priority"
		result := &OpenAIForwardResult{ServiceTier: &requested, UpstreamResponseServiceTier: "default"REDACTED
		resolution := ApplyOpenAIServiceTierBillingResolution(result)
		require.True(t, resolution.Downgraded)
		require.NotNil(t, result.ServiceTier)
		require.Equal(t, "default", *result.ServiceTier)
REDACTED)

	t.Run("openai honoured tier keeps pointer", func(t *testing.T) {
		requested := "priority"
		result := &OpenAIForwardResult{ServiceTier: &requested, UpstreamResponseServiceTier: "priority"REDACTED
		require.False(t, ApplyOpenAIServiceTierBillingResolution(result).Downgraded)
		require.Same(t, &requested, result.ServiceTier)
REDACTED)

	t.Run("openai untiered request stays nil", func(t *testing.T) {
		result := &OpenAIForwardResult{UpstreamResponseServiceTier: "priority"REDACTED
		require.False(t, ApplyOpenAIServiceTierBillingResolution(result).Downgraded)
		require.Nil(t, result.ServiceTier)
REDACTED)

	t.Run("anthropic standard speed rewrites fast", func(t *testing.T) {
		requested := "fast"
		result := &ForwardResult{ServiceTier: &requested, UpstreamResponseServiceTier: "standard"REDACTED
		require.True(t, ApplyForwardServiceTierBillingResolution(result).Downgraded)
		require.Equal(t, "standard", *result.ServiceTier)
REDACTED)

	t.Run("nil results are ignored", func(t *testing.T) {
		require.False(t, ApplyOpenAIServiceTierBillingResolution(nil).Downgraded)
		require.False(t, ApplyForwardServiceTierBillingResolution(nil).Downgraded)
REDACTED)
REDACTED
