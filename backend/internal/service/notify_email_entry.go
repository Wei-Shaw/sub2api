package service

import (
	"encoding/json"
	"strings"
)

// NotifyEmailEntry represents a notification email with enable/disable and verification state.
// Email="" is a placeholder for the "primary email" (user's registration email or first admin email).
type NotifyEmailEntry struct {
	Email    string `json:"email"`
	Disabled bool   `json:"disabled"`
	Verified bool   `json:"verified"`
}

// parseNotifyEmails parses a JSON string into []NotifyEmailEntry.
// It auto-detects the format:
//   - Old format ["email1","email2"] → converted to [{email, disabled:false, verified:true}, ...]
//   - New format [{email,disabled,verified}, ...] → parsed directly
//
// Returns nil on empty/invalid input.
func ParseNotifyEmails(raw string) []NotifyEmailEntry {
	raw = strings.TrimSpace(raw)
	if raw == "" || raw == "[]" {
		return nil
	}

	// Try parsing as new format first (array of objects)
	var entries []NotifyEmailEntry
	if err := json.Unmarshal([]byte(raw), &entries); err == nil && len(entries) > 0 {
		// Verify it's actually the new format by checking the first element
		// json.Unmarshal into []NotifyEmailEntry succeeds even for ["string"]
		// because it tries to fit "string" into NotifyEmailEntry and gets zero values.
		// We need to detect old format explicitly.
		if !isOldStringArrayFormat(raw) {
			return entries
		}
	}

	// Try parsing as old format (array of strings)
	var emails []string
	if err := json.Unmarshal([]byte(raw), &emails); err == nil {
		result := make([]NotifyEmailEntry, 0, len(emails))
		for _, e := range emails {
			e = strings.TrimSpace(e)
			if e != "" {
				result = append(result, NotifyEmailEntry{
					Email:    e,
					Disabled: false,
					Verified: false, // Old format emails default to unverified
				})
			}
		}
		return result
	}

	return nil
}

// isOldStringArrayFormat checks if the JSON is a string array like ["email1","email2"].
func isOldStringArrayFormat(raw string) bool {
	var arr []json.RawMessage
	if err := json.Unmarshal([]byte(raw), &arr); err != nil || len(arr) == 0 {
		return false
	}
	// Check if first element starts with a quote (string) vs { (object)
	first := strings.TrimSpace(string(arr[0]))
	return len(first) > 0 && first[0] == '"'
}

// marshalNotifyEmails serializes []NotifyEmailEntry to JSON string.
func MarshalNotifyEmails(entries []NotifyEmailEntry) string {
	if len(entries) == 0 {
		return "[]"
	}
	data, err := json.Marshal(entries)
	if err != nil {
		return "[]"
	}
	return string(data)
}

// filterEnabledEmails returns only non-disabled email addresses from entries.
// Empty email placeholders are skipped (caller should resolve them separately).
func FilterEnabledEmails(entries []NotifyEmailEntry) []string {
	var result []string
	for _, e := range entries {
		if e.Disabled {
			continue
		}
		email := strings.TrimSpace(e.Email)
		if email != "" {
			result = append(result, email)
		}
	}
	return result
}

// isPrimaryDisabled checks if the primary email placeholder (email="") exists and is disabled.
func IsPrimaryDisabled(entries []NotifyEmailEntry) bool {
	for _, e := range entries {
		if e.Email == "" {
			return e.Disabled
		}
	}
	return false // No primary placeholder = not disabled
}
