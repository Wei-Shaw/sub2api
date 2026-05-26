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
}

func TestParseNotifyEmails_EmptyArray(t *testing.T) {
	result := ParseNotifyEmails("[]")
	require.Nil(t, result)
}

func TestParseNotifyEmails_Null(t *testing.T) {
	// "null" is valid JSON that unmarshals into a nil string slice.
	// The old-format branch then returns an empty (non-nil) slice.
	result := ParseNotifyEmails("null")
	require.Empty(t, result)
}

func TestParseNotifyEmails_WhitespaceOnly(t *testing.T) {
	result := ParseNotifyEmails("   ")
	require.Nil(t, result)
}

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
}

func TestParseNotifyEmails_OldFormat_SkipsEmptyEntries(t *testing.T) {
	raw := `["alice@example.com", "", "  ", "bob@example.com"]`
	result := ParseNotifyEmails(raw)
	require.Len(t, result, 2)
	require.Equal(t, "alice@example.com", result[0].Email)
	require.Equal(t, "bob@example.com", result[1].Email)
}

func TestParseNotifyEmails_NewFormat(t *testing.T) {
	raw := `[{"email":"alice@example.com","verified":true,"disabled":false},{"email":"bob@example.com","verified":false,"disabled":true}]`
	result := ParseNotifyEmails(raw)
	require.Len(t, result, 2)

	require.Equal(t, "alice@example.com", result[0].Email)
	require.True(t, result[0].Verified)
	require.False(t, result[0].Disabled)

	require.Equal(t, "bob@example.com", result[1].Email)
	require.False(t, result[1].Verified)
	require.True(t, result[1].Disabled)
}

func TestParseNotifyEmails_NewFormat_SingleEntry(t *testing.T) {
	raw := `[{"email":"solo@example.com","verified":true,"disabled":false}]`
	result := ParseNotifyEmails(raw)
	require.Len(t, result, 1)
	require.Equal(t, "solo@example.com", result[0].Email)
	require.True(t, result[0].Verified)
}

func TestParseNotifyEmails_InvalidJSON(t *testing.T) {
	result := ParseNotifyEmails(`{not valid json`)
	require.Nil(t, result)
}

func TestParseNotifyEmails_InvalidJSONObject(t *testing.T) {
	// A plain JSON object (not array) should return nil.
	result := ParseNotifyEmails(`{"email":"a@b.com"}`)
	require.Nil(t, result)
}

func TestParseNotifyEmails_WhitespacePadding(t *testing.T) {
	raw := `  ["padded@example.com"]  `
	result := ParseNotifyEmails(raw)
	require.Len(t, result, 1)
	require.Equal(t, "padded@example.com", result[0].Email)
}

// ---------- MarshalNotifyEmails ----------

func TestMarshalNotifyEmails_EmptySlice(t *testing.T) {
	result := MarshalNotifyEmails([]NotifyEmailEntry{})
	require.Equal(t, "[]", result)
}

func TestMarshalNotifyEmails_NilSlice(t *testing.T) {
	result := MarshalNotifyEmails(nil)
	require.Equal(t, "[]", result)
}

func TestMarshalNotifyEmails_SingleEntry(t *testing.T) {
	entries := []NotifyEmailEntry{
		{Email: "test@example.com", Verified: true, Disabled: false},
	}
	result := MarshalNotifyEmails(entries)
	require.Contains(t, result, `"email":"test@example.com"`)
	require.Contains(t, result, `"verified":true`)
	require.Contains(t, result, `"disabled":false`)

	// Round-trip: parsing the marshalled result should produce the original entries.
	parsed := ParseNotifyEmails(result)
	require.Len(t, parsed, 1)
	require.Equal(t, entries[0], parsed[0])
}

func TestMarshalNotifyEmails_MultipleEntries(t *testing.T) {
	entries := []NotifyEmailEntry{
		{Email: "a@example.com", Verified: true, Disabled: false},
		{Email: "b@example.com", Verified: false, Disabled: true},
	}
	result := MarshalNotifyEmails(entries)

	// Round-trip verification.
	parsed := ParseNotifyEmails(result)
	require.Len(t, parsed, 2)
	require.Equal(t, entries[0], parsed[0])
	require.Equal(t, entries[1], parsed[1])
}

func TestMarshalNotifyEmails_RoundTrip_NewFormat(t *testing.T) {
	original := []NotifyEmailEntry{
		{Email: "x@example.com", Verified: true, Disabled: true},
		{Email: "y@example.com", Verified: false, Disabled: false},
	}
	marshalled := MarshalNotifyEmails(original)
	parsed := ParseNotifyEmails(marshalled)
	require.Equal(t, original, parsed)
}

// ---------- isOldStringArrayFormat (indirectly via ParseNotifyEmails) ----------

func TestParseNotifyEmails_MixedOldFormatWithWhitespace(t *testing.T) {
	// Emails with leading/trailing whitespace in old format should be trimmed.
	raw := `["  alice@example.com  "]`
	result := ParseNotifyEmails(raw)
	require.Len(t, result, 1)
	require.Equal(t, "alice@example.com", result[0].Email)
}
