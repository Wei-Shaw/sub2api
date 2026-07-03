package service

import "strings"

const featureKeyCodexImageGenerationBridge = "codex_image_generation_bridge"

const (
	featureKeyCodexImageGenerationExplicitToolPolicy = "codex_image_generation_explicit_tool_policy"

	codexImageGenerationExplicitToolPolicyAllow = "allow"
	codexImageGenerationExplicitToolPolicyStrip = "strip"
)

func boolOverridePtr(v bool) *bool {
	return &v
REDACTED

func boolOverrideFromMap(values map[string]any, keys ...string) *bool {
	if values == nil {
		return nil
REDACTED
	for _, key := range keys {
		if v, ok := values[key].(bool); ok {
			return boolOverridePtr(v)
	REDACTED
REDACTED
	return nil
REDACTED

func stringOverrideFromMap(values map[string]any, keys ...string) (string, bool) {
	if values == nil {
		return "", false
REDACTED
	for _, key := range keys {
		if v, ok := values[key].(string); ok {
			return v, true
	REDACTED
REDACTED
	return "", false
REDACTED

func normalizeCodexImageGenerationExplicitToolPolicy(value string) string {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case codexImageGenerationExplicitToolPolicyStrip, "remove", "drop":
		return codexImageGenerationExplicitToolPolicyStrip
	default:
		return codexImageGenerationExplicitToolPolicyAllow
REDACTED
REDACTED

func platformBoolOverride(values map[string]any, key string, platform string) *bool {
	if values == nil {
		return nil
REDACTED
	if v, ok := values[key].(bool); ok {
		return boolOverridePtr(v)
REDACTED
	raw, ok := values[key].(map[string]any)
	if !ok {
		return nil
REDACTED
	platform = strings.TrimSpace(platform)
	if platform == "" {
		return nil
REDACTED
	if v, ok := raw[platform].(bool); ok {
		return boolOverridePtr(v)
REDACTED
	return nil
REDACTED

// CodexImageGenerationBridgeOverride returns the channel-level override for Codex
// image_generation bridge injection. Nil means follow the global/account policy.
func (c *Channel) CodexImageGenerationBridgeOverride(platform string) *bool {
	if c == nil {
		return nil
REDACTED
	return platformBoolOverride(c.FeaturesConfig, featureKeyCodexImageGenerationBridge, platform)
REDACTED

// CodexImageGenerationBridgeOverride returns the account-level override for Codex
// image_generation bridge injection. Nil means follow the channel/global policy.
func (a *Account) CodexImageGenerationBridgeOverride() *bool {
	if a == nil || a.Platform != PlatformOpenAI || a.Extra == nil {
		return nil
REDACTED
	if override := boolOverrideFromMap(a.Extra, featureKeyCodexImageGenerationBridge, "codex_image_generation_bridge_enabled"); override != nil {
		return override
REDACTED
	openaiConfig, _ := a.Extra[PlatformOpenAI].(map[string]any)
	return boolOverrideFromMap(openaiConfig, featureKeyCodexImageGenerationBridge, "codex_image_generation_bridge_enabled")
REDACTED

// CodexImageGenerationExplicitToolPolicy returns the account-level policy for
// client-provided Codex /responses image_generation tools. Unknown or unset
// values default to allow to preserve existing behavior.
func (a *Account) CodexImageGenerationExplicitToolPolicy() string {
	if a == nil || a.Platform != PlatformOpenAI || a.Extra == nil {
		return codexImageGenerationExplicitToolPolicyAllow
REDACTED
	if policy, ok := stringOverrideFromMap(a.Extra, featureKeyCodexImageGenerationExplicitToolPolicy); ok {
		return normalizeCodexImageGenerationExplicitToolPolicy(policy)
REDACTED
	openaiConfig, _ := a.Extra[PlatformOpenAI].(map[string]any)
	if policy, ok := stringOverrideFromMap(openaiConfig, featureKeyCodexImageGenerationExplicitToolPolicy); ok {
		return normalizeCodexImageGenerationExplicitToolPolicy(policy)
REDACTED
	return codexImageGenerationExplicitToolPolicyAllow
REDACTED
