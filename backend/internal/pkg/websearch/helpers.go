package websearch

const (
	maxResponseSize   = 1 << 20 // 1 MB
	errorBodyTruncLen = 200
)

// truncateBody returns a truncated string of body for error messages.
func truncateBody(body []byte) string {
	if len(body) <= errorBodyTruncLen {
		return string(body)
	}
	return string(body[:errorBodyTruncLen]) + "...(truncated)"
}

// truncateRunes cuts s to at most n Unicode characters.
func truncateRunes(s string, n int) string {
	runes := []rune(s)
	if len(runes) <= n {
		return s
	}
	return string(runes[:n])
}
