package service

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestChannel_IsWebSearchEmulationEnabled_Enabled(t *testing.T) {
	c := &Channel{
		FeaturesConfig: map[string]any{
			featureKeyWebSearchEmulation: map[string]any{"anthropic": trueREDACTED,
	REDACTED,
REDACTED
	require.True(t, c.IsWebSearchEmulationEnabled("anthropic"))
REDACTED

func TestChannel_IsWebSearchEmulationEnabled_DifferentPlatform(t *testing.T) {
	c := &Channel{
		FeaturesConfig: map[string]any{
			featureKeyWebSearchEmulation: map[string]any{"anthropic": trueREDACTED,
	REDACTED,
REDACTED
	require.False(t, c.IsWebSearchEmulationEnabled("openai"))
REDACTED

func TestChannel_IsWebSearchEmulationEnabled_Disabled(t *testing.T) {
	c := &Channel{
		FeaturesConfig: map[string]any{
			featureKeyWebSearchEmulation: map[string]any{"anthropic": falseREDACTED,
	REDACTED,
REDACTED
	require.False(t, c.IsWebSearchEmulationEnabled("anthropic"))
REDACTED

func TestChannel_IsWebSearchEmulationEnabled_NilFeaturesConfig(t *testing.T) {
	c := &Channel{FeaturesConfig: nilREDACTED
	require.False(t, c.IsWebSearchEmulationEnabled("anthropic"))
REDACTED

func TestChannel_IsWebSearchEmulationEnabled_NilChannel(t *testing.T) {
	var c *Channel
	require.False(t, c.IsWebSearchEmulationEnabled("anthropic"))
REDACTED

func TestChannel_IsWebSearchEmulationEnabled_WrongStructure(t *testing.T) {
	c := &Channel{
		FeaturesConfig: map[string]any{
			featureKeyWebSearchEmulation: true, // not a map
	REDACTED,
REDACTED
	require.False(t, c.IsWebSearchEmulationEnabled("anthropic"))
REDACTED

func TestChannel_IsWebSearchEmulationEnabled_PlatformValueNotBool(t *testing.T) {
	c := &Channel{
		FeaturesConfig: map[string]any{
			featureKeyWebSearchEmulation: map[string]any{"anthropic": "yes"REDACTED,
	REDACTED,
REDACTED
	require.False(t, c.IsWebSearchEmulationEnabled("anthropic"))
REDACTED
