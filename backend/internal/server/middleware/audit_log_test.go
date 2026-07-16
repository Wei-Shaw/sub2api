package middleware

import "testing"

func TestDeriveAuditAction(t *testing.T) {
	cases := []struct {
		method string
		path   string
		want   string
REDACTED{
		{"PUT", "/api/v1/admin/accounts/:id", "admin.accounts.update"REDACTED,
		{"POST", "/api/v1/admin/accounts", "admin.accounts.create"REDACTED,
		{"DELETE", "/api/v1/admin/backups/:id", "admin.backups.delete"REDACTED,
		{"GET", "/api/v1/admin/users/:id/api-keys", "admin.users.api_keys.read"REDACTED,
		{"POST", "/api/v1/admin/redeem-codes/batch", "admin.redeem_codes.batch.create"REDACTED,
REDACTED
	for _, tc := range cases {
		if got := deriveAuditAction(tc.method, tc.path); got != tc.want {
			t.Fatalf("deriveAuditAction(%q, %q) = %q, want %q", tc.method, tc.path, got, tc.want)
	REDACTED
REDACTED
REDACTED
