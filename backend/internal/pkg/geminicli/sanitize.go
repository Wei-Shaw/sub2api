package geminicli

import "strings"

const maxLogBodyLen = 2048

func SanitizeBodyForLogs(body string) string {
	body = truncateBase64InMessage(body)
	if len(body) > maxLogBodyLen {
		body = body[:maxLogBodyLen] + "...[truncated]"
REDACTED
	return body
REDACTED

func truncateBase64InMessage(message string) string {
	const maxBase64Length = 50

	result := message
	offset := 0
	for {
		idx := strings.Index(result[offset:], ";base64,")
		if idx == -1 {
			break
	REDACTED
		actualIdx := offset + idx
		start := actualIdx + len(";base64,")

		end := start
		for end < len(result) && isBase64Char(result[end]) {
			end++
	REDACTED

		if end-start > maxBase64Length {
			result = result[:start+maxBase64Length] + "...[truncated]" + result[end:]
			offset = start + maxBase64Length + len("...[truncated]")
			continue
	REDACTED
		offset = end
REDACTED

	return result
REDACTED

func isBase64Char(c byte) bool {
	return (c >= 'A' && c <= 'Z') || (c >= 'a' && c <= 'z') || (c >= '0' && c <= '9') || c == '+' || c == '/' || c == '='
REDACTED
