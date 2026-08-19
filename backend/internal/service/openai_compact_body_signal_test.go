//go:build unit

package service

import (
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
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
	require.True(t, HasCompactionTriggerInInput(body))
REDACTED

func TestHasCompactionTriggerInInput_NoTrigger(t *testing.T) {
	body := []byte(`{
		"model":"gpt-5.5",
		"input":[
			{"type":"message","role":"user","content":"hello"REDACTED
		]
REDACTED`)
	require.False(t, HasCompactionTriggerInInput(body))
REDACTED

func TestHasCompactionTriggerInInput_EmptyInput(t *testing.T) {
	body := []byte(`{"model":"gpt-5.5","input":[]REDACTED`)
	require.False(t, HasCompactionTriggerInInput(body))
REDACTED

func TestHasCompactionTriggerInInput_NoInputField(t *testing.T) {
	body := []byte(`{"model":"gpt-5.5"REDACTED`)
	require.False(t, HasCompactionTriggerInInput(body))
REDACTED

func TestHasCompactionTriggerInInput_EmptyBody(t *testing.T) {
	require.False(t, HasCompactionTriggerInInput(nil))
	require.False(t, HasCompactionTriggerInInput([]byte{REDACTED))
REDACTED

func TestHasCompactionTriggerInInput_StringInput(t *testing.T) {
	body := []byte(`{"model":"gpt-5.5","input":"compaction_trigger"REDACTED`)
	require.False(t, HasCompactionTriggerInInput(body))
REDACTED

func TestHasCompactionTriggerInInput_CompactTriggerOnly(t *testing.T) {
	body := []byte(`{"model":"gpt-5.5","input":[{"type":"compaction_trigger"REDACTED]REDACTED`)
	require.True(t, HasCompactionTriggerInInput(body))
REDACTED

func TestNormalizeCompactionTriggerInputOrder_MovesAndCollapsesTriggers(t *testing.T) {
	body := []byte(`{"model":"gpt-5.5","input":[{"type":"compaction_trigger"REDACTED,{"type":"message","role":"user","content":"tail"REDACTED,{"type":"compaction_trigger"REDACTED]REDACTED`)
	normalized, changed, err := NormalizeCompactionTriggerInputOrder(body)
REDACTED
	require.True(t, changed)
	items := gjson.GetBytes(normalized, "input").Array()
	require.Len(t, items, 2)
	require.Equal(t, "message", items[0].Get("type").String())
	require.Equal(t, "compaction_trigger", items[1].Get("type").String())
REDACTED

func TestNormalizeCompactionTriggerInputOrder_AlreadyFinalPreservesBytes(t *testing.T) {
	body := []byte(`{"input":[{"type":"message"REDACTED,{"type":"compaction_trigger"REDACTED]REDACTED`)
	normalized, changed, err := NormalizeCompactionTriggerInputOrder(body)
REDACTED
	require.False(t, changed)
	require.Equal(t, string(body), string(normalized))
REDACTED
