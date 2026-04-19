//go:build unit

package signature

import (
	"context"
	"io"
	"strings"
	"testing"
)

func TestHarvester_SSE_ExtractsContentBlockStartSignatures(t *testing.T) {
	pool := newFakePool()
	h := NewHarvester(pool, 10)

	// Two thinking content_block_start events + one unrelated event.
	body := strings.Join([]string{
		`event: message_start`,
		`data: {"type":"message_start","message":{}}`,
		``,
		`event: content_block_start`,
		`data: {"type":"content_block_start","index":0,"content_block":{"type":"thinking","thinking":"","signature":"SIG-A"}}`,
		``,
		`event: content_block_start`,
		`data: {"type":"content_block_start","index":1,"content_block":{"type":"thinking","thinking":"","signature":"SIG-B"}}`,
		``,
		`event: content_block_delta`,
		`data: {"type":"content_block_delta","delta":{"type":"thinking_delta","thinking":"hi"}}`,
		``,
	}, "\n") + "\n"

	rc := h.Wrap(context.Background(), io.NopCloser(strings.NewReader(body)), HarvestOptions{
		Bucket:    "oauth",
		Streaming: true,
	})
	if _, err := io.ReadAll(rc); err != nil {
		t.Fatalf("read: %v", err)
	}
	_ = rc.Close()

	got := pool.entries["oauth"]
	if len(got) != 2 {
		t.Fatalf("expected 2 signatures harvested, got %d: %v", len(got), got)
	}
	// Newest-first semantics: fakePool prepends on Add, so the last-emitted is index 0.
	if got[0] != "SIG-B" || got[1] != "SIG-A" {
		t.Errorf("ordering: got %v, want [SIG-B, SIG-A]", got)
	}
}

func TestHarvester_SSE_DeduplicatesWithinSameResponse(t *testing.T) {
	pool := newFakePool()
	h := NewHarvester(pool, 10)
	body := strings.Repeat(
		"event: content_block_start\ndata: {\"type\":\"content_block_start\",\"content_block\":{\"type\":\"thinking\",\"signature\":\"DUP\"}}\n\n",
		5,
	)
	rc := h.Wrap(context.Background(), io.NopCloser(strings.NewReader(body)), HarvestOptions{
		Bucket:    "oauth",
		Streaming: true,
	})
	_, _ = io.ReadAll(rc)
	_ = rc.Close()

	if got := pool.entries["oauth"]; len(got) != 1 {
		t.Errorf("expected dedup to 1 entry within single response, got %d: %v", len(got), got)
	}
}

func TestHarvester_SkipCallbackBlocksIngestion(t *testing.T) {
	pool := newFakePool()
	h := NewHarvester(pool, 10)
	body := `data: {"type":"content_block_start","content_block":{"type":"thinking","signature":"NO"}}` + "\n\n"
	rc := h.Wrap(context.Background(), io.NopCloser(strings.NewReader(body)), HarvestOptions{
		Bucket:    "oauth",
		Streaming: true,
		Skip:      func() bool { return true },
	})
	_, _ = io.ReadAll(rc)
	_ = rc.Close()
	if len(pool.entries["oauth"]) != 0 {
		t.Errorf("skip callback must prevent harvesting, got %v", pool.entries["oauth"])
	}
}

func TestHarvester_NonStreaming_ParsesOnClose(t *testing.T) {
	pool := newFakePool()
	h := NewHarvester(pool, 10)
	body := `{"id":"msg_1","content":[
		{"type":"thinking","thinking":"x","signature":"NS-A"},
		{"type":"text","text":"hi"},
		{"type":"thinking","thinking":"y","signature":"NS-B"}
	]}`
	rc := h.Wrap(context.Background(), io.NopCloser(strings.NewReader(body)), HarvestOptions{
		Bucket:    "oauth",
		Streaming: false,
	})
	_, _ = io.ReadAll(rc)
	// Nothing should be harvested before Close on non-streaming path.
	if len(pool.entries["oauth"]) != 0 {
		t.Errorf("non-streaming must not ingest before Close; got %v", pool.entries["oauth"])
	}
	_ = rc.Close()
	if got := pool.entries["oauth"]; len(got) != 2 {
		t.Fatalf("expected 2 entries after Close, got %d: %v", len(got), got)
	}
}

func TestHarvester_EmptyBucketDisablesWrap(t *testing.T) {
	pool := newFakePool()
	h := NewHarvester(pool, 10)
	inner := io.NopCloser(strings.NewReader("irrelevant"))
	out := h.Wrap(context.Background(), inner, HarvestOptions{Bucket: ""})
	if out != inner {
		t.Errorf("empty bucket must return the original body unmodified")
	}
}

func TestHarvester_ZeroCapacityDisablesWrap(t *testing.T) {
	pool := newFakePool()
	h := NewHarvester(pool, 0)
	inner := io.NopCloser(strings.NewReader("irrelevant"))
	out := h.Wrap(context.Background(), inner, HarvestOptions{Bucket: "oauth"})
	if out != inner {
		t.Errorf("zero capacity must return the original body unmodified")
	}
}
