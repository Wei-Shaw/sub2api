package service

import "strings"

func optionalTrimmedStringPtr(raw string) *string {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return nil
REDACTED
	return &trimmed
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
