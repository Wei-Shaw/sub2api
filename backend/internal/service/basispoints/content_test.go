package basispoints

import (
	"errors"
	"strings"
	"testing"
)

func TestEncryptedContentValidationExposesSafePath(t *testing.T) {
	b := &Bridge{}
	err := b.validateHistoryContent([]any{object{
		"type":    "encrypted_content",
		"content": "opaque-secret-payload",
	}}, 103, "content")
	var validationErr *ContentValidationError
	if !errors.As(err, &validationErr) {
		t.Fatalf("expected ContentValidationError, got %T: %v", err, err)
	}
	if validationErr.Path != "input[103].content[0]" || validationErr.ContentType != "encrypted_content" {
		t.Fatalf("unexpected location: %+v", validationErr)
	}
	if strings.Contains(err.Error(), "opaque-secret-payload") {
		t.Fatal("validation error leaked encrypted payload")
	}
	if got := err.Error(); got != "basispoints cannot forward encrypted_content message parts; refresh the model catalog and start a new conversation without a multi-agent v2 override, or resend the original plaintext (path=input[103].content[0]; type=encrypted_content)" {
		t.Fatalf("unexpected compatibility message: %s", got)
	}
}
