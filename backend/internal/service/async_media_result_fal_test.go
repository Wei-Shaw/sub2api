//go:build unit

package service

import (
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/pkg/fal"
)

func TestExtractFalImageMetadataPreservesOutputFields(t *testing.T) {
	metadata := extractFalImageMetadata(&fal.Response{Images: []fal.Image{{
		URL: "https://cdn.example.test/fal-result.webp", ContentType: "image/webp",
		FileName: "fal-result.webp", FileSize: 2048, Width: 1024, Height: 768,
	}}})
	if len(metadata) != 1 {
		t.Fatalf("metadata length = %d, want 1", len(metadata))
	}
	got := metadata[0]
	if got.ContentType != "image/webp" || got.FileName != "fal-result.webp" || got.FileSize != 2048 || got.Width != 1024 || got.Height != 768 {
		t.Fatalf("unexpected metadata: %#v", got)
	}
}
