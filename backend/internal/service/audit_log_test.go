package service

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestMaskAuditCredential(t *testing.T) {
	cases := []struct {
		name string
		in   string
		want string
REDACTED{
		{"empty", "", ""REDACTED,
		{"short", "abc", "****"REDACTED,
		{"boundary_14", "12345678901234", "****"REDACTED,
		{"long", "sk-ant-api03-abcdefghijklmnop1234", "sk-ant****1234"REDACTED,
REDACTED
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := MaskAuditCredential(tc.in)
			if got != tc.want {
				t.Fatalf("MaskAuditCredential(%q) = %q, want %q", tc.in, got, tc.want)
		REDACTED
			// 掩码结果绝不能包含原始凭证的中间部分。
			if len(tc.in) > 14 && strings.Contains(got, tc.in) {
				t.Fatalf("masked value leaks full credential: %q", got)
		REDACTED
	REDACTED)
REDACTED
REDACTED

func TestRedactAuditBody_JSONRedactsSecrets(t *testing.T) {
	raw := []byte(`{
		"name": "acc1",
		"base_url": "https://evil.example.com",
		"credentials": {"api_key": "sk-secret-123", "base_url": "https://evil.example.com"REDACTED,
		"new_password": "hunter2",
		"totp_code": "123456",
		"nested": [{"access_token": "tok_abc"REDACTED]
REDACTED`)
	out := RedactAuditBody(raw, "application/json")

	var parsed map[string]any
	if err := json.Unmarshal([]byte(out), &parsed); err != nil {
		t.Fatalf("output is not valid JSON: %v\n%s", err, out)
REDACTED

	// 敏感字段被擦除。
	for _, secret := range []string{"sk-secret-123", "hunter2", "123456", "tok_abc"REDACTED {
		if strings.Contains(out, secret) {
			t.Fatalf("redacted body still contains secret %q: %s", secret, out)
	REDACTED
REDACTED
	// 非敏感字段（base_url、name）保留以便追责。
	if !strings.Contains(out, "evil.example.com") {
		t.Fatalf("base_url should be preserved for accountability: %s", out)
REDACTED
	if !strings.Contains(out, "acc1") {
		t.Fatalf("name should be preserved: %s", out)
REDACTED
REDACTED

func TestRedactAuditBody_NonJSONOmitted(t *testing.T) {
	out := RedactAuditBody([]byte("username=admin&password=secret"), "application/x-www-form-urlencoded")
	if strings.Contains(out, "secret") {
		t.Fatalf("non-json body must not leak content: %s", out)
REDACTED
	if !strings.Contains(out, "omitted") {
		t.Fatalf("expected omission marker, got: %s", out)
REDACTED
REDACTED

func TestRedactAuditBody_Empty(t *testing.T) {
	if got := RedactAuditBody(nil, "application/json"); got != "" {
		t.Fatalf("expected empty for nil body, got %q", got)
REDACTED
REDACTED

func TestSessionBindingHash(t *testing.T) {
	a := &SessionBinding{IP: "1.2.3.4", UserAgent: "Mozilla/5.0"REDACTED
	b := &SessionBinding{IP: "1.2.3.4", UserAgent: "Mozilla/5.0"REDACTED
	if a.Hash() != b.Hash() {
		t.Fatalf("identical bindings must hash equal")
REDACTED
	if a.Hash() == "" {
		t.Fatalf("non-empty binding must produce non-empty hash")
REDACTED

	// IP 变化 → 哈希变化。
	c := &SessionBinding{IP: "5.6.7.8", UserAgent: "Mozilla/5.0"REDACTED
	if a.Hash() == c.Hash() {
		t.Fatalf("changing IP must change hash")
REDACTED
	// UA 变化 → 哈希变化。
	d := &SessionBinding{IP: "1.2.3.4", UserAgent: "curl/8.0"REDACTED
	if a.Hash() == d.Hash() {
		t.Fatalf("changing UA must change hash")
REDACTED

	// 空指纹 → 空哈希（旧 token 兼容）。
	empty := &SessionBinding{REDACTED
	if empty.Hash() != "" {
		t.Fatalf("empty binding must hash to empty string")
REDACTED
	var nilBinding *SessionBinding
	if nilBinding.Hash() != "" {
		t.Fatalf("nil binding must hash to empty string")
REDACTED
REDACTED

func TestParseAuditLogRetentionDays(t *testing.T) {
	cases := map[string]int{
		"":        defaultAuditLogRetentionDays,
		"abc":     defaultAuditLogRetentionDays,
		"90":      90,
		"0":       0,
		"-1":      0,
		"  30  ":  30,
REDACTED
	for in, want := range cases {
		if got := parseAuditLogRetentionDays(in); got != want {
			t.Fatalf("parseAuditLogRetentionDays(%q) = %d, want %d", in, got, want)
	REDACTED
REDACTED
REDACTED
