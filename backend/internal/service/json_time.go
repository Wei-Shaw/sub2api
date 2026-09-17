package service

import "time"

// isJSONTimeInRange rejects dates that PostgreSQL can store but time.Time cannot
// serialize in API responses. Nil represents an unset schedule or expiry.
func isJSONTimeInRange(value *time.Time) bool {
	return value == nil || (value.Year() >= 0 && value.Year() <= 9999)
}
