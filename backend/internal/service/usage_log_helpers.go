package service

import "strings"

func optionalTrimmedStringPtr(raw string) *string {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return nil
REDACTED
	return &trimmed
REDACTED

// optionalNonEqualStringPtr returns a pointer to value if it is non-empty and
// differs from compare; otherwise nil. Usage logging passes the requested
// model as compare so a channel mapping still records its effective upstream.
func optionalNonEqualStringPtr(value, compare string) *string {
	if value == "" || value == compare {
		return nil
REDACTED
	return &value
REDACTED

func forwardResultBillingModel(requestedModel, upstreamModel string) string {
	if trimmed := strings.TrimSpace(requestedModel); trimmed != "" {
		return trimmed
REDACTED
	return strings.TrimSpace(upstreamModel)
REDACTED

func optionalInt64Ptr(v int64) *int64 {
	if v == 0 {
		return nil
REDACTED
	return &v
REDACTED
