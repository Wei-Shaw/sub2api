//go:build unit

package pluginsdk

import (
	"context"
	"errors"
	"fmt"
	"testing"
)

// TestNilCredentialManager_GetOAuthToken verifies that the nil
// implementation returns a clear "not available" error and a nil
// token, matching the contract of nilEventsClient / nilHostClient.
func TestNilCredentialManager_GetOAuthToken(t *testing.T) {
	var c nilCredentialManager
	tok, err := c.GetOAuthToken(context.Background(), 42)
	if tok != nil {
		t.Errorf("nilCredentialManager.GetOAuthToken token = %v, want nil", tok)
	}
	if err == nil || !errorsContains(err, "CredentialManager not available") {
		t.Errorf("nilCredentialManager.GetOAuthToken err = %v, want 'not available' error", err)
	}
}

func TestNilCredentialManager_GetAPIKey(t *testing.T) {
	var c nilCredentialManager
	key, err := c.GetAPIKey(context.Background(), 42)
	if key != "" {
		t.Errorf("nilCredentialManager.GetAPIKey key = %q, want empty", key)
	}
	if err == nil || !errorsContains(err, "CredentialManager not available") {
		t.Errorf("nilCredentialManager.GetAPIKey err = %v, want 'not available' error", err)
	}
}

func TestNilCredentialManager_GetCustomToken(t *testing.T) {
	var c nilCredentialManager
	tok, err := c.GetCustomToken(context.Background(), 42, "bedrock-sigv4")
	if tok != nil {
		t.Errorf("nilCredentialManager.GetCustomToken token = %v, want nil", tok)
	}
	if err == nil || !errorsContains(err, "CredentialManager not available") {
		t.Errorf("nilCredentialManager.GetCustomToken err = %v, want 'not available' error", err)
	}
}

// TestNilCredentialManager_RegisterCustomAuth verifies that
// RegisterCustomAuth does not panic on the nil implementation.
func TestNilCredentialManager_RegisterCustomAuth(t *testing.T) {
	var c nilCredentialManager
	// Should not panic; no-op is acceptable for registration on a
	// nil manager since there is no meaningful action to take.
	c.RegisterCustomAuth("test", nil)
}

// TestNilCredentialManager_ImplementsInterface is a compile-time
// check that nilCredentialManager satisfies CredentialManager.
func TestNilCredentialManager_ImplementsInterface(t *testing.T) {
	var _ CredentialManager = nilCredentialManager{}
}

// TestSentinelErrors verifies that sentinel errors are distinct and
// usable with errors.Is.
func TestSentinelErrors(t *testing.T) {
	if errors.Is(ErrCredentialNotFound, ErrRefreshFailed) {
		t.Fatal("ErrCredentialNotFound and ErrRefreshFailed must be distinct")
	}
	wrapped := fmt.Errorf("outer: %w", ErrCredentialNotFound)
	if !errors.Is(wrapped, ErrCredentialNotFound) {
		t.Fatal("wrapped ErrCredentialNotFound must be unwrappable via errors.Is")
	}
	wrapped = fmt.Errorf("outer: %w", ErrRefreshFailed)
	if !errors.Is(wrapped, ErrRefreshFailed) {
		t.Fatal("wrapped ErrRefreshFailed must be unwrappable via errors.Is")
	}
}
