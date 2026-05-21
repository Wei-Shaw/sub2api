package service

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestChannel_IsBedrockCCCompatEnabled_Enabled(t *testing.T) {
	c := &Channel{
		FeaturesConfig: map[string]any{
			featureKeyBedrockCCCompat: true,
	REDACTED,
REDACTED
	require.True(t, c.IsBedrockCCCompatEnabled("bedrock"))
REDACTED

func TestChannel_IsBedrockCCCompatEnabled_AppliesToAllPlatforms(t *testing.T) {
	c := &Channel{
		FeaturesConfig: map[string]any{
			featureKeyBedrockCCCompat: true,
	REDACTED,
REDACTED
	require.True(t, c.IsBedrockCCCompatEnabled("anthropic"))
	require.True(t, c.IsBedrockCCCompatEnabled("openai"))
	require.True(t, c.IsBedrockCCCompatEnabled(""))
REDACTED

func TestChannel_IsBedrockCCCompatEnabled_Disabled(t *testing.T) {
	c := &Channel{
		FeaturesConfig: map[string]any{
			featureKeyBedrockCCCompat: false,
	REDACTED,
REDACTED
	require.False(t, c.IsBedrockCCCompatEnabled("bedrock"))
REDACTED

func TestChannel_IsBedrockCCCompatEnabled_NilFeaturesConfig(t *testing.T) {
	c := &Channel{FeaturesConfig: nilREDACTED
	require.False(t, c.IsBedrockCCCompatEnabled("bedrock"))
REDACTED

func TestChannel_IsBedrockCCCompatEnabled_NilChannel(t *testing.T) {
	var c *Channel
	require.False(t, c.IsBedrockCCCompatEnabled("bedrock"))
REDACTED

func TestChannel_IsBedrockCCCompatEnabled_WrongType(t *testing.T) {
	c := &Channel{
		FeaturesConfig: map[string]any{
			featureKeyBedrockCCCompat: "yes",
	REDACTED,
REDACTED
	require.False(t, c.IsBedrockCCCompatEnabled("bedrock"))
REDACTED

func TestChannel_IsBedrockCCCompatEnabled_OldMapFormat(t *testing.T) {
	c := &Channel{
		FeaturesConfig: map[string]any{
			featureKeyBedrockCCCompat: map[string]any{"bedrock": trueREDACTED,
	REDACTED,
REDACTED
	require.False(t, c.IsBedrockCCCompatEnabled("bedrock"))
REDACTED

func TestChannel_IsBedrockCCCompatEnabled_MissingKey(t *testing.T) {
	c := &Channel{
		FeaturesConfig: map[string]any{
			"other_feature": true,
	REDACTED,
REDACTED
	require.False(t, c.IsBedrockCCCompatEnabled("bedrock"))
REDACTED
