package main

import "testing"

func TestHistoricalIngressRejectReason(t *testing.T) {
	tests := []struct {
		name   string
		item   candidate
		reason string
		match  bool
REDACTED{
		{name: "standard invalid key", item: candidate{body: `{"code":"INVALID_API_KEY","message":"Invalid API key"REDACTED`REDACTED, reason: "invalid_key", match: trueREDACTED,
		{name: "google missing key", item: candidate{body: `{"error":{"code":401,"message":"API key is required","status":"UNAUTHENTICATED"REDACTEDREDACTED`REDACTED, reason: "missing_key", match: trueREDACTED,
		{name: "google group deleted", item: candidate{body: `{"error":{"code":403,"message":"API Key 所属分组已删除","status":"PERMISSION_DENIED"REDACTEDREDACTED`REDACTED, reason: "group_deleted", match: trueREDACTED,
		{name: "ip acl", item: candidate{body: `{"code":"ACCESS_DENIED","message":"Access denied. Your IP is 192.0.2.1"REDACTED`REDACTED, reason: "ip_acl_denied", match: trueREDACTED,
		{name: "user not found remains", item: candidate{body: `{"code":"USER_NOT_FOUND","message":"User associated with API key not found"REDACTED`REDACTED, match: falseREDACTED,
		{name: "quota remains", item: candidate{body: `{"code":"API_KEY_QUOTA_EXHAUSTED","message":"quota"REDACTED`REDACTED, match: falseREDACTED,
		{name: "database failure remains", item: candidate{statusCode: 500, message: "Failed to validate API key", body: `{"code":"INTERNAL_ERROR","message":"Failed to validate API key"REDACTED`REDACTED, match: falseREDACTED,
REDACTED

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			reason, ok := historicalIngressRejectReason(tt.item)
			if ok != tt.match || reason != tt.reason {
				t.Fatalf("got (%q, %v), want (%q, %v)", reason, ok, tt.reason, tt.match)
		REDACTED
	REDACTED)
REDACTED
REDACTED
