package anthropicfp

import (
	"bytes"
	"strings"
	"testing"
)

func TestNormalizeText_ASCIIHyphenIsIdentity(t *testing.T) {
	in := "Today's date is 2026-07-01."
	out, hits := NormalizeText(in)
	if out != in {
		t.Fatalf("canonical form should be returned identity, got %q", out)
REDACTED
	if len(hits) != 0 {
		t.Fatalf("no hits expected on canonical input, got %d", len(hits))
REDACTED
REDACTED

func TestNormalizeText_SlashSeparatorASCIIApostrophe(t *testing.T) {
	in := "Today's date is 2026/07/01."
	out, hits := NormalizeText(in)
	want := "Today's date is 2026-07-01."
	if out != want {
		t.Fatalf("want %q, got %q", want, out)
REDACTED
	if len(hits) != 1 || hits[0].ApostropheVariant != "ascii" || hits[0].DateSeparator != "/" {
		t.Fatalf("unexpected hit: %+v", hits)
REDACTED
REDACTED

func TestNormalizeText_U2019Apostrophe(t *testing.T) {
	in := "Today’s date is 2026-07-01."
	out, hits := NormalizeText(in)
	want := "Today's date is 2026-07-01."
	if out != want {
		t.Fatalf("want %q, got %q", want, out)
REDACTED
	if len(hits) != 1 || hits[0].ApostropheVariant != "u2019" || hits[0].DateSeparator != "-" {
		t.Fatalf("unexpected hit: %+v", hits)
REDACTED
REDACTED

func TestNormalizeText_U02BCApostropheWithSlash(t *testing.T) {
	in := "Todayʼs date is 2026/07/01."
	out, hits := NormalizeText(in)
	want := "Today's date is 2026-07-01."
	if out != want {
		t.Fatalf("want %q, got %q", want, out)
REDACTED
	if len(hits) != 1 || hits[0].ApostropheVariant != "u02bc" || hits[0].DateSeparator != "/" {
		t.Fatalf("unexpected hit: %+v", hits)
REDACTED
REDACTED

func TestNormalizeText_U02B9Apostrophe(t *testing.T) {
	in := "Todayʹs date is 2026/07/01."
	out, hits := NormalizeText(in)
	want := "Today's date is 2026-07-01."
	if out != want {
		t.Fatalf("want %q, got %q", want, out)
REDACTED
	if len(hits) != 1 || hits[0].ApostropheVariant != "u02b9" || hits[0].DateSeparator != "/" {
		t.Fatalf("unexpected hit: %+v", hits)
REDACTED
REDACTED

func TestNormalizeText_MixedSeparatorsNoMatch(t *testing.T) {
	// backreference \3 forces the two separators to agree
	in := "Today's date is 2026-07/01."
	out, hits := NormalizeText(in)
	if out != in {
		t.Fatalf("mixed-separator input should not be matched, got %q", out)
REDACTED
	if len(hits) != 0 {
		t.Fatalf("expected no hits, got %d", len(hits))
REDACTED
REDACTED

func TestNormalizeText_NegativeLookalike(t *testing.T) {
	cases := []string{
		"Today is a great day.",
		"His date is 2026-07-01.",
		"Yesterday's date was 2026-06-30.",
		"'s date is 2026-07-01.",
REDACTED
	for _, c := range cases {
		out, hits := NormalizeText(c)
		if out != c {
			t.Fatalf("input %q should not be modified, got %q", c, out)
	REDACTED
		if len(hits) != 0 {
			t.Fatalf("input %q should produce no hits", c)
	REDACTED
REDACTED
REDACTED

func TestNormalizeText_Idempotent(t *testing.T) {
	in := "Today’s date is 2026/07/01."
	out1, _ := NormalizeText(in)
	out2, hits2 := NormalizeText(out1)
	if out1 != out2 {
		t.Fatalf("normalization not idempotent: %q vs %q", out1, out2)
REDACTED
	if len(hits2) != 0 {
		t.Fatalf("second pass should produce no hits, got %d", len(hits2))
REDACTED
REDACTED

func TestNormalizeText_MultipleOccurrences(t *testing.T) {
	in := "First line.\nToday’s date is 2026/07/01.\nMore text.\nTodayʼs date is 2026-07-01.\nEnd."
	out, hits := NormalizeText(in)
	if strings.Count(out, "Today's date is") != 2 {
		t.Fatalf("expected two canonicalized sentences, got: %q", out)
REDACTED
	if strings.Contains(out, "’") || strings.Contains(out, "ʼ") || strings.Contains(out, "2026/07/01") {
		t.Fatalf("fingerprint characters must be gone, got: %q", out)
REDACTED
	if len(hits) != 2 {
		t.Fatalf("expected 2 hits, got %d", len(hits))
REDACTED
REDACTED

func TestNormalizeDateline_SystemString(t *testing.T) {
	body := []byte(`{"system":"You are helpful.\nToday’s date is 2026/07/01.\nBe brief.","messages":[]REDACTED`)
	out, hits, changed := NormalizeDateline(body)
	if !changed {
		t.Fatalf("expected changed=true")
REDACTED
	if len(hits) != 1 {
		t.Fatalf("expected 1 hit, got %d", len(hits))
REDACTED
	if !bytes.Contains(out, []byte("Today's date is 2026-07-01.")) {
		t.Fatalf("output missing canonical dateline: %s", string(out))
REDACTED
	if bytes.Contains(out, []byte("2026/07/01")) {
		t.Fatalf("output should not contain slash date: %s", string(out))
REDACTED
REDACTED

func TestNormalizeDateline_SystemBlocksArray(t *testing.T) {
	body := []byte(`{"system":[{"type":"text","text":"You are helpful."REDACTED,{"type":"text","text":"Todayʼs date is 2026/07/01."REDACTED],"messages":[]REDACTED`)
	out, hits, changed := NormalizeDateline(body)
	if !changed {
		t.Fatalf("expected changed=true")
REDACTED
	if len(hits) != 1 || hits[0].ApostropheVariant != "u02bc" {
		t.Fatalf("unexpected hits: %+v", hits)
REDACTED
	if !bytes.Contains(out, []byte("Today's date is 2026-07-01.")) {
		t.Fatalf("output missing canonical dateline: %s", string(out))
REDACTED
REDACTED

func TestNormalizeDateline_MessagesContentStringInSystemReminder(t *testing.T) {
	body := []byte(`{"messages":[{"role":"user","content":"<system-reminder>\n# currentDate\nToday’s date is 2026/07/01.\n</system-reminder>\nHello, please help."REDACTED]REDACTED`)
	out, hits, changed := NormalizeDateline(body)
	if !changed {
		t.Fatalf("expected changed=true")
REDACTED
	if len(hits) != 1 {
		t.Fatalf("expected 1 hit, got %d", len(hits))
REDACTED
	if !bytes.Contains(out, []byte("Today's date is 2026-07-01.")) {
		t.Fatalf("canonical dateline missing: %s", string(out))
REDACTED
REDACTED

func TestNormalizeDateline_MessagesContentBlocksInSystemReminder(t *testing.T) {
	body := []byte(`{"messages":[{"role":"user","content":[{"type":"text","text":"<system-reminder>\nToday’s date is 2026/07/01.\n</system-reminder>"REDACTED,{"type":"text","text":"do X"REDACTED]REDACTED]REDACTED`)
	out, hits, changed := NormalizeDateline(body)
	if !changed {
		t.Fatalf("expected changed=true")
REDACTED
	if len(hits) != 1 {
		t.Fatalf("expected 1 hit, got %d", len(hits))
REDACTED
	if !bytes.Contains(out, []byte("Today's date is 2026-07-01.")) {
		t.Fatalf("canonical dateline missing: %s", string(out))
REDACTED
REDACTED

func TestNormalizeDateline_LeavesOutOfScopeUntouched(t *testing.T) {
	// User prose outside <system-reminder> that mentions today's date must
	// not be modified. tool_use.input / tool_result.content are never scanned.
	body := []byte(`{"messages":[` +
		`{"role":"user","content":"Today’s date is 2026/07/01. Please help."REDACTED,` +
		`{"role":"assistant","content":[{"type":"tool_use","id":"x","name":"y","input":{"note":"Today’s date is 2026/07/01."REDACTEDREDACTED]REDACTED,` +
		`{"role":"user","content":[{"type":"tool_result","tool_use_id":"x","content":"log: Today’s date is 2026/07/01."REDACTED]REDACTED` +
		`]REDACTED`)
	out, hits, changed := NormalizeDateline(body)
	if changed {
		t.Fatalf("expected changed=false, hits=%v out=%s", hits, string(out))
REDACTED
	if !bytes.Equal(out, body) {
		t.Fatalf("output should equal input byte-for-byte")
REDACTED
REDACTED

func TestNormalizeDateline_Idempotent(t *testing.T) {
	body := []byte(`{"messages":[{"role":"user","content":"<system-reminder>\nToday’s date is 2026/07/01.\n</system-reminder>"REDACTED]REDACTED`)
	first, _, changed1 := NormalizeDateline(body)
	if !changed1 {
		t.Fatalf("expected first pass to change body")
REDACTED
	second, _, changed2 := NormalizeDateline(first)
	if changed2 {
		t.Fatalf("second pass should not report changes")
REDACTED
	if !bytes.Equal(first, second) {
		t.Fatalf("second pass diverged: %s vs %s", string(first), string(second))
REDACTED
REDACTED

func TestNormalizeDateline_EmptyBody(t *testing.T) {
	out, hits, changed := NormalizeDateline(nil)
	if changed || out != nil || hits != nil {
		t.Fatalf("empty body should be no-op")
REDACTED
REDACTED

func TestNormalizeDateline_NoDateline(t *testing.T) {
	body := []byte(`{"messages":[{"role":"user","content":"hello"REDACTED],"system":"just a system prompt"REDACTED`)
	out, hits, changed := NormalizeDateline(body)
	if changed || len(hits) != 0 {
		t.Fatalf("expected no changes; changed=%v hits=%v", changed, hits)
REDACTED
	if &out[0] != &body[0] {
		// Identity is a bonus but not strict; verify content equality at minimum
		if !bytes.Equal(out, body) {
			t.Fatalf("output must byte-match input when no changes needed")
	REDACTED
REDACTED
REDACTED

func TestNormalizeDateline_MultipleSystemReminderBlocksInSameText(t *testing.T) {
	body := []byte(`{"messages":[{"role":"user","content":"<system-reminder>\nToday’s date is 2026/07/01.\n</system-reminder>\nsome prose\n<system-reminder>\nAlso Todayʼs date is 2026/07/01.\n</system-reminder>"REDACTED]REDACTED`)
	out, hits, changed := NormalizeDateline(body)
	if !changed {
		t.Fatalf("expected changed=true")
REDACTED
	if len(hits) != 2 {
		t.Fatalf("expected 2 hits, got %d", len(hits))
REDACTED
	if bytes.Contains(out, []byte("2026/07/01")) {
		t.Fatalf("slash separator must be gone: %s", string(out))
REDACTED
REDACTED
