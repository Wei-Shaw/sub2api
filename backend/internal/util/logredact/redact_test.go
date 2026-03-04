package logredact

import (
	"strings"
	"testing"
)

func TestRedactText_JSONLike(t *testing.T) {
	in := `{"access_token":"REDACTED","refresh_token":"1//0gDUMMY","other":"ok"REDACTED`
	out := RedactText(in)
	if out == in {
		t.Fatalf("expected redaction, got unchanged")
REDACTED
	if want := `"access_token":"***"`; !strings.Contains(out, want) {
		t.Fatalf("expected %q in %q", want, out)
REDACTED
	if want := `"refresh_token":"***"`; !strings.Contains(out, want) {
		t.Fatalf("expected %q in %q", want, out)
REDACTED
REDACTED

func TestRedactText_QueryLike(t *testing.T) {
	in := "access_token=REDACTED refresh_token=1//0gDUMMY"
	out := RedactText(in)
	if strings.Contains(out, "ya29") || strings.Contains(out, "1//0") {
		t.Fatalf("expected tokens redacted, got %q", out)
REDACTED
REDACTED

func TestRedactText_GOCSPX(t *testing.T) {
	in := "client_secret=GOCSPX-your-client-secret"
	out := RedactText(in)
	if strings.Contains(out, "your-client-secret") {
		t.Fatalf("expected secret redacted, got %q", out)
REDACTED
	if !strings.Contains(out, "client_secret=***") {
		t.Fatalf("expected key redacted, got %q", out)
REDACTED
REDACTED

func TestRedactText_ExtraKeyCacheUsesNormalizedSortedKey(t *testing.T) {
	clearExtraTextPatternCache()

	out1 := RedactText("custom_secret=abc", "Custom_Secret", " custom_secret ")
	out2 := RedactText("custom_secret=xyz", "custom_secret")
	if !strings.Contains(out1, "custom_secret=***") {
		t.Fatalf("expected custom key redacted in first call, got %q", out1)
REDACTED
	if !strings.Contains(out2, "custom_secret=***") {
		t.Fatalf("expected custom key redacted in second call, got %q", out2)
REDACTED

	if got := countExtraTextPatternCacheEntries(); got != 1 {
		t.Fatalf("expected 1 cached pattern set, got %d", got)
REDACTED
REDACTED

func TestRedactText_DefaultPathDoesNotUseExtraCache(t *testing.T) {
	clearExtraTextPatternCache()

	out := RedactText("access_token=abc")
	if !strings.Contains(out, "access_token=***") {
		t.Fatalf("expected default key redacted, got %q", out)
REDACTED
	if got := countExtraTextPatternCacheEntries(); got != 0 {
		t.Fatalf("expected extra cache to remain empty, got %d", got)
REDACTED
REDACTED

func clearExtraTextPatternCache() {
	extraTextPatternCache.Range(func(key, value any) bool {
		extraTextPatternCache.Delete(key)
		return true
REDACTED)
REDACTED

func countExtraTextPatternCacheEntries() int {
	count := 0
	extraTextPatternCache.Range(func(key, value any) bool {
		count++
		return true
REDACTED)
	return count
REDACTED
