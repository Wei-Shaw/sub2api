//go:build unit

package antigravity

import (
	"crypto/sha256"
	"encoding/hex"
	"regexp"
	"strconv"
	"strings"
	"testing"
)

var officialRequestIDPattern = regexp.MustCompile(`^agent/([0-9a-f]{16})/(\d+)/([0-9a-f]{8})/(\d+)$`)

func TestGenerateOfficialRequestIDFormat(t *testing.T) {
	reqID := GenerateOfficialRequestID("my-conversation-id", 1)
	matches := officialRequestIDPattern.FindStringSubmatch(reqID)
	if len(matches) != 5 {
		t.Fatalf("expected 5-segment official request ID format, got %q", reqID)
	}

	h := sha256.Sum256([]byte("my-conversation-id"))
	expectedConv16 := hex.EncodeToString(h[:])[:16]
	if matches[1] != expectedConv16 {
		t.Fatalf("expected conv16 %q, got %q", expectedConv16, matches[1])
	}

	stepVal, err := strconv.Atoi(matches[4])
	if err != nil || stepVal != 1 {
		t.Fatalf("expected step 1, got %v", matches[4])
	}
}

func TestGenerateOfficialRequestIDEmptyConversation(t *testing.T) {
	reqID := GenerateOfficialRequestID("", 0)
	if !officialRequestIDPattern.MatchString(reqID) {
		t.Fatalf("expected valid format with empty conversation ID, got %q", reqID)
	}
	parts := strings.Split(reqID, "/")
	if len(parts) != 5 {
		t.Fatalf("expected 5 parts, got %d", len(parts))
	}
	if len(parts[1]) != 16 {
		t.Fatalf("expected 16 hex chars for random convID, got %q", parts[1])
	}
}
