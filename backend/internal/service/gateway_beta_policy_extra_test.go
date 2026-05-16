//go:build unit

package service

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestBetaPolicyScopeMatches_All(t *testing.T) {
	assert.True(t, betaPolicyScopeMatches("all", true, false))
	assert.True(t, betaPolicyScopeMatches("all", false, false))
	assert.True(t, betaPolicyScopeMatches("all", false, true))
}

func TestBetaPolicyScopeMatches_OAuth(t *testing.T) {
	assert.True(t, betaPolicyScopeMatches("oauth", true, false))
	assert.False(t, betaPolicyScopeMatches("oauth", false, false))
	assert.False(t, betaPolicyScopeMatches("oauth", false, true))
}

func TestBetaPolicyScopeMatches_APIKey(t *testing.T) {
	assert.True(t, betaPolicyScopeMatches("apikey", false, false))
	assert.False(t, betaPolicyScopeMatches("apikey", true, false))
	assert.False(t, betaPolicyScopeMatches("apikey", false, true))
}

func TestBetaPolicyScopeMatches_Bedrock(t *testing.T) {
	assert.True(t, betaPolicyScopeMatches("bedrock", false, true))
	assert.False(t, betaPolicyScopeMatches("bedrock", true, false))
	assert.False(t, betaPolicyScopeMatches("bedrock", false, false))
}

func TestBetaPolicyScopeMatches_Unknown_FailOpen(t *testing.T) {
	// Unknown scope defaults to match-all (fail-open)
	assert.True(t, betaPolicyScopeMatches("unknown", true, true))
	assert.True(t, betaPolicyScopeMatches("", true, false))
}

func TestMatchModelWhitelist_ExactMatch(t *testing.T) {
	assert.True(t, matchModelWhitelist("claude-sonnet-4-5", []string{"claude-sonnet-4-5"}))
	assert.False(t, matchModelWhitelist("claude-opus-4", []string{"claude-sonnet-4-5"}))
}

func TestMatchModelWhitelist_EmptyWhitelist(t *testing.T) {
	assert.False(t, matchModelWhitelist("claude-sonnet-4-5", nil))
	assert.False(t, matchModelWhitelist("claude-sonnet-4-5", []string{}))
}

func TestMatchModelWhitelist_WildcardPrefix(t *testing.T) {
	assert.True(t, matchModelWhitelist("claude-sonnet-4-5-20250514", []string{"claude-sonnet-4-5*"}))
	assert.False(t, matchModelWhitelist("claude-opus-4", []string{"claude-sonnet*"}))
}

func TestResolveRuleAction_NoWhitelist(t *testing.T) {
	rule := BetaPolicyRule{Action: "block", ErrorMessage: "blocked"}
	action, msg := resolveRuleAction(rule, "any-model")
	assert.Equal(t, "block", action)
	assert.Equal(t, "blocked", msg)
}

func TestResolveRuleAction_ModelInWhitelist(t *testing.T) {
	rule := BetaPolicyRule{
		Action:        "block",
		ErrorMessage:  "blocked",
		ModelWhitelist: []string{"claude-sonnet-4-5"},
	}
	action, _ := resolveRuleAction(rule, "claude-sonnet-4-5")
	assert.Equal(t, "block", action)
}

func TestResolveRuleAction_ModelNotInWhitelist(t *testing.T) {
	rule := BetaPolicyRule{
		Action:            "block",
		ErrorMessage:      "blocked",
		ModelWhitelist:    []string{"claude-sonnet-4-5"},
		FallbackAction:    "filter",
	}
	action, _ := resolveRuleAction(rule, "claude-opus-4")
	assert.Equal(t, "filter", action)
}

func TestFilterBetaTokens_RemovesFiltered(t *testing.T) {
	tokens := []string{"token-a", "token-b", "token-c"}
	filterSet := map[string]struct{}{"token-b": {}}
	result := filterBetaTokens(tokens, filterSet)
	assert.Equal(t, []string{"token-a", "token-c"}, result)
}

func TestFilterBetaTokens_EmptyFilterSet(t *testing.T) {
	tokens := []string{"token-a", "token-b"}
	result := filterBetaTokens(tokens, nil)
	assert.Equal(t, []string{"token-a", "token-b"}, result)
}

func TestFilterBetaTokens_AllFiltered(t *testing.T) {
	tokens := []string{"token-a", "token-b"}
	filterSet := map[string]struct{}{"token-a": {}, "token-b": {}}
	result := filterBetaTokens(tokens, filterSet)
	assert.Empty(t, result)
}

func TestRequestNeedsBetaFeatures_WithTools(t *testing.T) {
	body := []byte(`{"model":"claude-sonnet-4-5","tools":[{"name":"get_weather"}]}`)
	assert.True(t, requestNeedsBetaFeatures(body))
}

func TestRequestNeedsBetaFeatures_WithThinking(t *testing.T) {
	body := []byte(`{"model":"claude-sonnet-4-5","thinking":{"type":"enabled"}}`)
	assert.True(t, requestNeedsBetaFeatures(body))
}

func TestRequestNeedsBetaFeatures_AdaptiveThinking(t *testing.T) {
	body := []byte(`{"model":"claude-sonnet-4-5","thinking":{"type":"adaptive"}}`)
	assert.True(t, requestNeedsBetaFeatures(body))
}

func TestRequestNeedsBetaFeatures_NoFeatures(t *testing.T) {
	body := []byte(`{"model":"claude-sonnet-4-5","messages":[]}`)
	assert.False(t, requestNeedsBetaFeatures(body))
}

func TestRequestNeedsBetaFeatures_EmptyTools(t *testing.T) {
	body := []byte(`{"model":"claude-sonnet-4-5","tools":[]}`)
	assert.False(t, requestNeedsBetaFeatures(body))
}

func TestRequestNeedsBetaFeatures_DisabledThinking(t *testing.T) {
	body := []byte(`{"model":"claude-sonnet-4-5","thinking":{"type":"disabled"}}`)
	assert.False(t, requestNeedsBetaFeatures(body))
}
