package service

import "github.com/tidwall/gjson"

// GJSON/SJSON select the first duplicate key, while encoding/json keeps the
// last. Remove ambiguous session fields before setting the isolated identity,
// so the later v1internal wrapper cannot restore a caller-controlled value.
// Preserve every other field's order and raw value, including nested content.
func removeAmbiguousGeminiSessionFields(body []byte) []byte {
	root := gjson.ParseBytes(body)
	count, ambiguous := 0, false
	root.ForEach(func(key, _ gjson.Result) bool {
		if key.Str == "sessionId" || key.Str == "session_id" {
			count++
			ambiguous = count > 1 || key.Raw != `"`+key.Str+`"`
		}
		return !ambiguous
	})
	if !ambiguous {
		return body
	}

	// Rebuild once instead of repeatedly deleting keys from an attacker-sized
	// object. Repeated SJSON deletions would make duplicate-heavy input quadratic.
	cleaned := make([]byte, 0, len(body))
	cleaned = append(cleaned, '{')
	first := true
	root.ForEach(func(key, value gjson.Result) bool {
		if key.Str == "sessionId" || key.Str == "session_id" {
			return true
		}
		if !first {
			cleaned = append(cleaned, ',')
		}
		first = false
		cleaned = append(cleaned, key.Raw...)
		cleaned = append(cleaned, ':')
		cleaned = append(cleaned, value.Raw...)
		return true
	})
	return append(cleaned, '}')
}
