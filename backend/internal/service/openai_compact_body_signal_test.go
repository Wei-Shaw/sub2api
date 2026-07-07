//go:build unit

package service

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestHasCompactionTriggerInInput_DetectsCompactSignal(t *testing.T) {
	body := []byte(`{
		"model":"gpt-5.5",
		"stream":true,
		"input":[
			{"type":"message","role":"user","content":"hello"REDACTED,
			{"type":"compaction_trigger"REDACTED
		]
REDACTED`)
	require.True(t, hasCompactionTriggerInInput(body))
REDACTED

func TestHasCompactionTriggerInInput_NoTrigger(t *testing.T) {
	body := []byte(`{
		"model":"gpt-5.5",
		"input":[
			{"type":"message","role":"user","content":"hello"REDACTED
		]
REDACTED`)
	require.False(t, hasCompactionTriggerInInput(body))
REDACTED

func TestHasCompactionTriggerInInput_EmptyInput(t *testing.T) {
	body := []byte(`{"model":"gpt-5.5","input":[]REDACTED`)
	require.False(t, hasCompactionTriggerInInput(body))
REDACTED

func TestHasCompactionTriggerInInput_NoInputField(t *testing.T) {
	body := []byte(`{"model":"gpt-5.5"REDACTED`)
	require.False(t, hasCompactionTriggerInInput(body))
REDACTED

func TestHasCompactionTriggerInInput_EmptyBody(t *testing.T) {
	require.False(t, hasCompactionTriggerInInput(nil))
	require.False(t, hasCompactionTriggerInInput([]byte{REDACTED))
REDACTED

func TestHasCompactionTriggerInInput_StringInput(t *testing.T) {
	body := []byte(`{"model":"gpt-5.5","input":"compaction_trigger"REDACTED`)
	require.False(t, hasCompactionTriggerInInput(body))
REDACTED

func TestHasCompactionTriggerInInput_CompactTriggerOnly(t *testing.T) {
	body := []byte(`{"model":"gpt-5.5","input":[{"type":"compaction_trigger"REDACTED]REDACTED`)
	require.True(t, hasCompactionTriggerInInput(body))
REDACTED
