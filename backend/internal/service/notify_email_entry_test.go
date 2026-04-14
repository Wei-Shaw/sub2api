//go:build unit

package service

import (
	"testing"

	"github.com/stretchr/testify/require"
)

// ---------- ParseNotifyEmails ----------

func TestParseNotifyEmails_EmptyString(t *testing.T) {
	result := ParseNotifyEmails("")
	require.Nil(t, result)
REDACTED

func TestParseNotifyEmails_EmptyArray(t *testing.T) {
	result := ParseNotifyEmails("[]")
	require.Nil(t, result)
REDACTED

func TestParseNotifyEmails_Null(t *testing.T) {
	// "null" is valid JSON that unmarshals into a nil string slice.
	// The old-format branch then returns an empty (non-nil) slice.
	result := ParseNotifyEmails("null")
	require.Empty(t, result)
REDACTED

func TestParseNotifyEmails_WhitespaceOnly(t *testing.T) {
	result := ParseNotifyEmails("   ")
	require.Nil(t, result)
REDACTED

func TestParseNotifyEmails_OldFormat(t *testing.T) {
	raw := `["alice@example.com", "bob@example.com"]`
	result := ParseNotifyEmails(raw)
	require.Len(t, result, 2)

	require.Equal(t, "alice@example.com", result[0].Email)
	require.False(t, result[0].Verified, "old format emails should default to unverified")
	require.False(t, result[0].Disabled)

	require.Equal(t, "bob@example.com", result[1].Email)
	require.False(t, result[1].Verified)
	require.False(t, result[1].Disabled)
REDACTED

func TestParseNotifyEmails_OldFormat_SkipsEmptyEntries(t *testing.T) {
	raw := `["alice@example.com", "", "  ", "bob@example.com"]`
	result := ParseNotifyEmails(raw)
	require.Len(t, result, 2)
	require.Equal(t, "alice@example.com", result[0].Email)
	require.Equal(t, "bob@example.com", result[1].Email)
REDACTED

func TestParseNotifyEmails_NewFormat(t *testing.T) {
	raw := `[{"email":"alice@example.com","verified":true,"disabled":falseREDACTED,{"email":"bob@example.com","verified":false,"disabled":trueREDACTED]`
	result := ParseNotifyEmails(raw)
	require.Len(t, result, 2)

	require.Equal(t, "alice@example.com", result[0].Email)
	require.True(t, result[0].Verified)
	require.False(t, result[0].Disabled)

	require.Equal(t, "bob@example.com", result[1].Email)
	require.False(t, result[1].Verified)
	require.True(t, result[1].Disabled)
REDACTED

func TestParseNotifyEmails_NewFormat_SingleEntry(t *testing.T) {
	raw := `[{"email":"solo@example.com","verified":true,"disabled":falseREDACTED]`
	result := ParseNotifyEmails(raw)
	require.Len(t, result, 1)
	require.Equal(t, "solo@example.com", result[0].Email)
	require.True(t, result[0].Verified)
REDACTED

func TestParseNotifyEmails_InvalidJSON(t *testing.T) {
	result := ParseNotifyEmails(`{not valid json`)
	require.Nil(t, result)
REDACTED

func TestParseNotifyEmails_InvalidJSONObject(t *testing.T) {
	// A plain JSON object (not array) should return nil.
	result := ParseNotifyEmails(`{"email":"a@b.com"REDACTED`)
	require.Nil(t, result)
REDACTED

func TestParseNotifyEmails_WhitespacePadding(t *testing.T) {
	raw := `  ["padded@example.com"]  `
	result := ParseNotifyEmails(raw)
	require.Len(t, result, 1)
	require.Equal(t, "padded@example.com", result[0].Email)
REDACTED

// ---------- MarshalNotifyEmails ----------

func TestMarshalNotifyEmails_EmptySlice(t *testing.T) {
	result := MarshalNotifyEmails([]NotifyEmailEntry{REDACTED)
	require.Equal(t, "[]", result)
REDACTED

func TestMarshalNotifyEmails_NilSlice(t *testing.T) {
	result := MarshalNotifyEmails(nil)
	require.Equal(t, "[]", result)
REDACTED

func TestMarshalNotifyEmails_SingleEntry(t *testing.T) {
	entries := []NotifyEmailEntry{
		{Email: "test@example.com", Verified: true, Disabled: falseREDACTED,
REDACTED
	result := MarshalNotifyEmails(entries)
	require.Contains(t, result, `"email":"test@example.com"`)
	require.Contains(t, result, `"verified":true`)
	require.Contains(t, result, `"disabled":false`)

	// Round-trip: parsing the marshalled result should produce the original entries.
	parsed := ParseNotifyEmails(result)
	require.Len(t, parsed, 1)
	require.Equal(t, entries[0], parsed[0])
REDACTED

func TestMarshalNotifyEmails_MultipleEntries(t *testing.T) {
	entries := []NotifyEmailEntry{
		{Email: "a@example.com", Verified: true, Disabled: falseREDACTED,
		{Email: "b@example.com", Verified: false, Disabled: trueREDACTED,
REDACTED
	result := MarshalNotifyEmails(entries)

	// Round-trip verification.
	parsed := ParseNotifyEmails(result)
	require.Len(t, parsed, 2)
	require.Equal(t, entries[0], parsed[0])
	require.Equal(t, entries[1], parsed[1])
REDACTED

func TestMarshalNotifyEmails_RoundTrip_NewFormat(t *testing.T) {
	original := []NotifyEmailEntry{
		{Email: "x@example.com", Verified: true, Disabled: trueREDACTED,
		{Email: "y@example.com", Verified: false, Disabled: falseREDACTED,
REDACTED
	marshalled := MarshalNotifyEmails(original)
	parsed := ParseNotifyEmails(marshalled)
	require.Equal(t, original, parsed)
REDACTED

// ---------- isOldStringArrayFormat (indirectly via ParseNotifyEmails) ----------

func TestParseNotifyEmails_MixedOldFormatWithWhitespace(t *testing.T) {
	// Emails with leading/trailing whitespace in old format should be trimmed.
	raw := `["  alice@example.com  "]`
	result := ParseNotifyEmails(raw)
	require.Len(t, result, 1)
	require.Equal(t, "alice@example.com", result[0].Email)
REDACTED
