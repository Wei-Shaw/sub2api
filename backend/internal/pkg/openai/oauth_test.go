package openai

import (
	"net/url"
	"sync"
	"testing"
	"time"
)

func TestSessionStore_Stop_Idempotent(t *testing.T) {
	store := NewSessionStore()

	store.Stop()
	store.Stop()

	select {
	case <-store.stopCh:
		// ok
	case <-time.After(time.Second):
		t.Fatal("stopCh 未关闭")
REDACTED
REDACTED

func TestSessionStore_Stop_Concurrent(t *testing.T) {
	store := NewSessionStore()

	var wg sync.WaitGroup
	for range 50 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			store.Stop()
	REDACTED()
REDACTED

	wg.Wait()

	select {
	case <-store.stopCh:
		// ok
	case <-time.After(time.Second):
		t.Fatal("stopCh 未关闭")
REDACTED
REDACTED

func TestBuildAuthorizationURLForPlatform_OpenAI(t *testing.T) {
	authURL := BuildAuthorizationURLForPlatform("state-1", "challenge-1", DefaultRedirectURI, OAuthPlatformOpenAI)
	parsed, err := url.Parse(authURL)
	if err != nil {
		t.Fatalf("Parse URL failed: %v", err)
REDACTED
	q := parsed.Query()
	if got := q.Get("client_id"); got != ClientID {
		t.Fatalf("client_id mismatch: got=%q want=%q", got, ClientID)
REDACTED
	if got := q.Get("codex_cli_simplified_flow"); got != "true" {
		t.Fatalf("codex flow mismatch: got=%q want=true", got)
REDACTED
	if got := q.Get("id_token_add_organizations"); got != "true" {
		t.Fatalf("id_token_add_organizations mismatch: got=%q want=true", got)
REDACTED
REDACTED

// TestBuildAuthorizationURLForPlatform_Sora 验证 Sora 平台复用 Codex CLI 的 client_id，
// 但不启用 codex_cli_simplified_flow。
func TestBuildAuthorizationURLForPlatform_Sora(t *testing.T) {
	authURL := BuildAuthorizationURLForPlatform("state-2", "challenge-2", DefaultRedirectURI, OAuthPlatformSora)
	parsed, err := url.Parse(authURL)
	if err != nil {
		t.Fatalf("Parse URL failed: %v", err)
REDACTED
	q := parsed.Query()
	if got := q.Get("client_id"); got != ClientID {
		t.Fatalf("client_id mismatch: got=%q want=%q (Sora should reuse Codex CLI client_id)", got, ClientID)
REDACTED
	if got := q.Get("codex_cli_simplified_flow"); got != "" {
		t.Fatalf("codex flow should be empty for sora, got=%q", got)
REDACTED
	if got := q.Get("id_token_add_organizations"); got != "true" {
		t.Fatalf("id_token_add_organizations mismatch: got=%q want=true", got)
REDACTED
REDACTED
