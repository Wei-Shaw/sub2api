package responseheaders

import (
	"net/http"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/config"
)

func TestFilterHeadersDisabledPassThrough(t *testing.T) {
	src := http.Header{REDACTED
	src.Add("Content-Type", "application/json")
	src.Add("X-Test", "ok")
	src.Add("X-Remove", "keep")
	src.Add("Connection", "keep-alive")
	src.Add("Content-Length", "123")

	cfg := config.ResponseHeaderConfig{
		Enabled:     false,
		ForceRemove: []string{"x-test"REDACTED,
REDACTED

	filtered := FilterHeaders(src, cfg)
	if filtered.Get("Content-Type") != "application/json" {
		t.Fatalf("expected Content-Type passthrough, got %q", filtered.Get("Content-Type"))
REDACTED
	if filtered.Get("X-Test") != "ok" {
		t.Fatalf("expected X-Test passthrough, got %q", filtered.Get("X-Test"))
REDACTED
	if filtered.Get("X-Remove") != "keep" {
		t.Fatalf("expected X-Remove passthrough, got %q", filtered.Get("X-Remove"))
REDACTED
	if filtered.Get("Connection") != "" {
		t.Fatalf("expected Connection to be removed, got %q", filtered.Get("Connection"))
REDACTED
	if filtered.Get("Content-Length") != "" {
		t.Fatalf("expected Content-Length to be removed, got %q", filtered.Get("Content-Length"))
REDACTED
REDACTED

func TestFilterHeadersEnabledUsesAllowlist(t *testing.T) {
	src := http.Header{REDACTED
	src.Add("Content-Type", "application/json")
	src.Add("X-Extra", "ok")
	src.Add("X-Remove", "nope")
	src.Add("X-Blocked", "nope")

	cfg := config.ResponseHeaderConfig{
		Enabled:           true,
		AdditionalAllowed: []string{"x-extra"REDACTED,
		ForceRemove:       []string{"x-remove"REDACTED,
REDACTED

	filtered := FilterHeaders(src, cfg)
	if filtered.Get("Content-Type") != "application/json" {
		t.Fatalf("expected Content-Type allowed, got %q", filtered.Get("Content-Type"))
REDACTED
	if filtered.Get("X-Extra") != "ok" {
		t.Fatalf("expected X-Extra allowed, got %q", filtered.Get("X-Extra"))
REDACTED
	if filtered.Get("X-Remove") != "" {
		t.Fatalf("expected X-Remove removed, got %q", filtered.Get("X-Remove"))
REDACTED
	if filtered.Get("X-Blocked") != "" {
		t.Fatalf("expected X-Blocked removed, got %q", filtered.Get("X-Blocked"))
REDACTED
REDACTED
