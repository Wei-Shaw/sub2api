package service

import "strings"

const featureKeyCodexImageGenerationBridge = "codex_image_generation_bridge"

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
