package chatgptwebimage

import (
	"errors"
	"strings"
	"testing"
)

func TestErrorDiagnosticDoesNotExposeFreeText(t *testing.T) {
	e := &Error{Stage: "secret-stage", Kind: "secret-kind", StatusCode: 9999, ConversationID: "secret-conversation", cause: errors.New("secret signed URL")}
	d := e.Diagnostic()
	if d.Stage != "unknown" || d.Kind != "unknown" || d.StatusCode != 0 || d.Accepted {
		t.Fatalf("untrusted fields escaped: %#v", d)
	}
	if strings.Contains(e.Error(), "secret") {
		t.Fatal("provider error exposed private metadata")
	}
	if !errors.Is(e, e.cause) {
		t.Fatal("internal error chain was lost")
	}
}

func TestErrorDiagnosticPreservesProviderFailure(t *testing.T) {
	e := &Error{Stage: "bootstrap", Kind: "authentication_or_challenge", StatusCode: 403}
	d := e.Diagnostic()
	if d.Stage != "bootstrap" || d.Kind != "authentication_or_challenge" || d.StatusCode != 403 || d.Accepted {
		t.Fatalf("missing actionable diagnostic: %#v", d)
	}
	e.Stage, e.Kind, e.StatusCode, e.Accepted = "download_lookup", "transport", 0, true
	d = e.Diagnostic()
	if d.Stage != "download_lookup" || d.Kind != "transport" || d.StatusCode != 0 || !d.Accepted {
		t.Fatalf("lost uncertain generation outcome: %#v", d)
	}
}
