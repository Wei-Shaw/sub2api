//go:build unit

package service

import (
	"testing"

	"github.com/stretchr/testify/require"
)

// ============ ParseMetadataUserID Tests ============

func TestParseMetadataUserID_LegacyFormat_WithoutAccountUUID(t *testing.T) {
	raw := "user_a1b2c3d4e5f6a1b2c3d4e5f6a1b2c3d4e5f6a1b2c3d4e5f6a1b2c3d4e5f6a1b2_account__session_123e4567-e89b-12d3-a456-426614174000"
	parsed := ParseMetadataUserID(raw)
	require.NotNil(t, parsed)
	require.Equal(t, "a1b2c3d4e5f6a1b2c3d4e5f6a1b2c3d4e5f6a1b2c3d4e5f6a1b2c3d4e5f6a1b2", parsed.DeviceID)
	require.Equal(t, "", parsed.AccountUUID)
	require.Equal(t, "123e4567-e89b-12d3-a456-426614174000", parsed.SessionID)
	require.False(t, parsed.IsNewFormat)
}

func TestParseMetadataUserID_LegacyFormat_WithAccountUUID(t *testing.T) {
	raw := "user_a1b2c3d4e5f6a1b2c3d4e5f6a1b2c3d4e5f6a1b2c3d4e5f6a1b2c3d4e5f6a1b2_account_550e8400-e29b-41d4-a716-446655440000_session_123e4567-e89b-12d3-a456-426614174000"
	parsed := ParseMetadataUserID(raw)
	require.NotNil(t, parsed)
	require.Equal(t, "a1b2c3d4e5f6a1b2c3d4e5f6a1b2c3d4e5f6a1b2c3d4e5f6a1b2c3d4e5f6a1b2", parsed.DeviceID)
	require.Equal(t, "550e8400-e29b-41d4-a716-446655440000", parsed.AccountUUID)
	require.Equal(t, "123e4567-e89b-12d3-a456-426614174000", parsed.SessionID)
	require.False(t, parsed.IsNewFormat)
}

func TestParseMetadataUserID_JSONFormat_WithoutAccountUUID(t *testing.T) {
	raw := `{"device_id":"d61f76d0aabbccdd00112233445566778899aabbccddeeff0011223344556677","account_uuid":"","session_id":"c72554f2-1234-5678-abcd-123456789abc"}`
	parsed := ParseMetadataUserID(raw)
	require.NotNil(t, parsed)
	require.Equal(t, "d61f76d0aabbccdd00112233445566778899aabbccddeeff0011223344556677", parsed.DeviceID)
	require.Equal(t, "", parsed.AccountUUID)
	require.Equal(t, "c72554f2-1234-5678-abcd-123456789abc", parsed.SessionID)
	require.True(t, parsed.IsNewFormat)
}

func TestParseMetadataUserID_JSONFormat_WithAccountUUID(t *testing.T) {
	raw := `{"device_id":"d61f76d0aabbccdd00112233445566778899aabbccddeeff0011223344556677","account_uuid":"550e8400-e29b-41d4-a716-446655440000","session_id":"c72554f2-1234-5678-abcd-123456789abc"}`
	parsed := ParseMetadataUserID(raw)
	require.NotNil(t, parsed)
	require.Equal(t, "d61f76d0aabbccdd00112233445566778899aabbccddeeff0011223344556677", parsed.DeviceID)
	require.Equal(t, "550e8400-e29b-41d4-a716-446655440000", parsed.AccountUUID)
	require.Equal(t, "c72554f2-1234-5678-abcd-123456789abc", parsed.SessionID)
	require.True(t, parsed.IsNewFormat)
}

func TestParseMetadataUserID_InvalidInputs(t *testing.T) {
	tests := []struct {
		name string
		raw  string
	}{
		{"empty string", ""},
		{"whitespace only", "   "},
		{"random text", "not-a-valid-user-id"},
		{"partial legacy format", "session_123e4567-e89b-12d3-a456-426614174000"},
		{"invalid JSON", `{"device_id":}`},
		{"JSON missing device_id", `{"account_uuid":"","session_id":"c72554f2-1234-5678-abcd-123456789abc"}`},
		{"JSON missing session_id", `{"device_id":"d61f76d0aabbccdd00112233445566778899aabbccddeeff0011223344556677","account_uuid":""}`},
		{"JSON empty device_id", `{"device_id":"","account_uuid":"","session_id":"c72554f2-1234-5678-abcd-123456789abc"}`},
		{"JSON empty session_id", `{"device_id":"d61f76d0aabbccdd00112233445566778899aabbccddeeff0011223344556677","account_uuid":"","session_id":""}`},
		{"legacy format short hex", "user_a1b2c3d4_account__session_123e4567-e89b-12d3-a456-426614174000"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			require.Nil(t, ParseMetadataUserID(tt.raw), "should return nil for: %s", tt.raw)
		})
	}
}

func TestParseMetadataUserID_HexCaseInsensitive(t *testing.T) {
	// Legacy format should accept both upper and lower case hex
	rawUpper := "user_A1B2C3D4E5F6A1B2C3D4E5F6A1B2C3D4E5F6A1B2C3D4E5F6A1B2C3D4E5F6A1B2_account__session_123e4567-e89b-12d3-a456-426614174000"
	parsed := ParseMetadataUserID(rawUpper)
	require.NotNil(t, parsed, "legacy format should accept uppercase hex")
	require.Equal(t, "A1B2C3D4E5F6A1B2C3D4E5F6A1B2C3D4E5F6A1B2C3D4E5F6A1B2C3D4E5F6A1B2", parsed.DeviceID)
}

// ============ FormatMetadataUserID Tests ============

func TestFormatMetadataUserID_LegacyVersion(t *testing.T) {
	result := FormatMetadataUserID("deadbeef"+"00112233445566778899aabbccddeeff0011223344556677", "acc-uuid", "sess-uuid", "2.1.77")
	require.Equal(t, "user_deadbeef00112233445566778899aabbccddeeff0011223344556677_account_acc-uuid_session_sess-uuid", result)
}

func TestFormatMetadataUserID_NewVersion(t *testing.T) {
	result := FormatMetadataUserID("deadbeef"+"00112233445566778899aabbccddeeff0011223344556677", "acc-uuid", "sess-uuid", "2.1.78")
	require.Equal(t, `{"device_id":"deadbeef00112233445566778899aabbccddeeff0011223344556677","account_uuid":"acc-uuid","session_id":"sess-uuid"}`, result)
}

func TestFormatMetadataUserID_EmptyVersion_Legacy(t *testing.T) {
	result := FormatMetadataUserID("deadbeef"+"00112233445566778899aabbccddeeff0011223344556677", "", "sess-uuid", "")
	require.Equal(t, "user_deadbeef00112233445566778899aabbccddeeff0011223344556677_account__session_sess-uuid", result)
}

func TestFormatMetadataUserID_EmptyAccountUUID(t *testing.T) {
	// Legacy format with empty account UUID → double underscore
	result := FormatMetadataUserID("deadbeef"+"00112233445566778899aabbccddeeff0011223344556677", "", "sess-uuid", "2.1.22")
	require.Contains(t, result, "_account__session_")

	// New format with empty account UUID → empty string in JSON
	result = FormatMetadataUserID("deadbeef"+"00112233445566778899aabbccddeeff0011223344556677", "", "sess-uuid", "2.1.78")
	require.Contains(t, result, `"account_uuid":""`)
}

// ============ IsNewMetadataFormatVersion Tests ============

func TestIsNewMetadataFormatVersion(t *testing.T) {
	tests := []struct {
		version string
		want    bool
	}{
		{"", false},
		{"2.1.77", false},
		{"2.1.78", true},
		{"2.1.79", true},
		{"2.2.0", true},
		{"3.0.0", true},
		{"2.0.100", false},
		{"1.9.99", false},
	}
	for _, tt := range tests {
		t.Run(tt.version, func(t *testing.T) {
			require.Equal(t, tt.want, IsNewMetadataFormatVersion(tt.version))
		})
	}
}

// ============ Round-trip Tests ============

func TestParseFormat_RoundTrip_Legacy(t *testing.T) {
	deviceID := "a1b2c3d4e5f6a1b2c3d4e5f6a1b2c3d4e5f6a1b2c3d4e5f6a1b2c3d4e5f6a1b2"
	accountUUID := "550e8400-e29b-41d4-a716-446655440000"
	sessionID := "123e4567-e89b-12d3-a456-426614174000"

	formatted := FormatMetadataUserID(deviceID, accountUUID, sessionID, "2.1.22")
	parsed := ParseMetadataUserID(formatted)
	require.NotNil(t, parsed)
	require.Equal(t, deviceID, parsed.DeviceID)
	require.Equal(t, accountUUID, parsed.AccountUUID)
	require.Equal(t, sessionID, parsed.SessionID)
	require.False(t, parsed.IsNewFormat)
}

func TestParseFormat_RoundTrip_JSON(t *testing.T) {
	deviceID := "a1b2c3d4e5f6a1b2c3d4e5f6a1b2c3d4e5f6a1b2c3d4e5f6a1b2c3d4e5f6a1b2"
	accountUUID := "550e8400-e29b-41d4-a716-446655440000"
	sessionID := "123e4567-e89b-12d3-a456-426614174000"

	formatted := FormatMetadataUserID(deviceID, accountUUID, sessionID, "2.1.78")
	parsed := ParseMetadataUserID(formatted)
	require.NotNil(t, parsed)
	require.Equal(t, deviceID, parsed.DeviceID)
	require.Equal(t, accountUUID, parsed.AccountUUID)
	require.Equal(t, sessionID, parsed.SessionID)
	require.True(t, parsed.IsNewFormat)
}

func TestParseFormat_RoundTrip_EmptyAccountUUID(t *testing.T) {
	deviceID := "a1b2c3d4e5f6a1b2c3d4e5f6a1b2c3d4e5f6a1b2c3d4e5f6a1b2c3d4e5f6a1b2"
	sessionID := "123e4567-e89b-12d3-a456-426614174000"

	// Legacy round-trip with empty account UUID
	formatted := FormatMetadataUserID(deviceID, "", sessionID, "2.1.22")
	parsed := ParseMetadataUserID(formatted)
	require.NotNil(t, parsed)
	require.Equal(t, deviceID, parsed.DeviceID)
	require.Equal(t, "", parsed.AccountUUID)
	require.Equal(t, sessionID, parsed.SessionID)

	// JSON round-trip with empty account UUID
	formatted = FormatMetadataUserID(deviceID, "", sessionID, "2.1.78")
	parsed = ParseMetadataUserID(formatted)
	require.NotNil(t, parsed)
	require.Equal(t, deviceID, parsed.DeviceID)
	require.Equal(t, "", parsed.AccountUUID)
	require.Equal(t, sessionID, parsed.SessionID)
}
