//go:build unit

package service

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestGetWebSearchEmulationMode_Enabled(t *testing.T) {
	a := &Account{
		Platform: PlatformAnthropic,
		Type:     AccountTypeAPIKey,
		Extra:    map[string]any{featureKeyWebSearchEmulation: "enabled"REDACTED,
REDACTED
	require.Equal(t, WebSearchModeEnabled, a.GetWebSearchEmulationMode())
REDACTED

func TestGetWebSearchEmulationMode_Disabled(t *testing.T) {
	a := &Account{
		Platform: PlatformAnthropic,
		Type:     AccountTypeAPIKey,
		Extra:    map[string]any{featureKeyWebSearchEmulation: "disabled"REDACTED,
REDACTED
	require.Equal(t, WebSearchModeDisabled, a.GetWebSearchEmulationMode())
REDACTED

func TestGetWebSearchEmulationMode_Default(t *testing.T) {
	a := &Account{
		Platform: PlatformAnthropic,
		Type:     AccountTypeAPIKey,
		Extra:    map[string]any{featureKeyWebSearchEmulation: "default"REDACTED,
REDACTED
	require.Equal(t, WebSearchModeDefault, a.GetWebSearchEmulationMode())
REDACTED

func TestGetWebSearchEmulationMode_UnknownString(t *testing.T) {
	a := &Account{
		Platform: PlatformAnthropic,
		Type:     AccountTypeAPIKey,
		Extra:    map[string]any{featureKeyWebSearchEmulation: "unknown"REDACTED,
REDACTED
	require.Equal(t, WebSearchModeDefault, a.GetWebSearchEmulationMode())
REDACTED

func TestGetWebSearchEmulationMode_OldBoolTrue(t *testing.T) {
	a := &Account{
		Platform: PlatformAnthropic,
		Type:     AccountTypeAPIKey,
		Extra:    map[string]any{featureKeyWebSearchEmulation: trueREDACTED,
REDACTED
	// bool is not a string, type assertion fails → default
	require.Equal(t, WebSearchModeDefault, a.GetWebSearchEmulationMode())
REDACTED

func TestGetWebSearchEmulationMode_OldBoolFalse(t *testing.T) {
	a := &Account{
		Platform: PlatformAnthropic,
		Type:     AccountTypeAPIKey,
		Extra:    map[string]any{featureKeyWebSearchEmulation: falseREDACTED,
REDACTED
	require.Equal(t, WebSearchModeDefault, a.GetWebSearchEmulationMode())
REDACTED

func TestGetWebSearchEmulationMode_NilAccount(t *testing.T) {
	var a *Account
	require.Equal(t, WebSearchModeDefault, a.GetWebSearchEmulationMode())
REDACTED

func TestGetWebSearchEmulationMode_NilExtra(t *testing.T) {
	a := &Account{
		Platform: PlatformAnthropic,
		Type:     AccountTypeAPIKey,
		Extra:    nil,
REDACTED
	require.Equal(t, WebSearchModeDefault, a.GetWebSearchEmulationMode())
REDACTED

func TestGetWebSearchEmulationMode_MissingField(t *testing.T) {
	a := &Account{
		Platform: PlatformAnthropic,
		Type:     AccountTypeAPIKey,
		Extra:    map[string]any{REDACTED,
REDACTED
	require.Equal(t, WebSearchModeDefault, a.GetWebSearchEmulationMode())
REDACTED

func TestGetWebSearchEmulationMode_NonAnthropicPlatform(t *testing.T) {
	a := &Account{
		Platform: PlatformOpenAI,
		Type:     AccountTypeAPIKey,
		Extra:    map[string]any{featureKeyWebSearchEmulation: "enabled"REDACTED,
REDACTED
	require.Equal(t, WebSearchModeDefault, a.GetWebSearchEmulationMode())
REDACTED

func TestGetWebSearchEmulationMode_NonAPIKeyType(t *testing.T) {
	a := &Account{
		Platform: PlatformAnthropic,
		Type:     AccountTypeOAuth,
		Extra:    map[string]any{featureKeyWebSearchEmulation: "enabled"REDACTED,
REDACTED
	require.Equal(t, WebSearchModeDefault, a.GetWebSearchEmulationMode())
REDACTED
