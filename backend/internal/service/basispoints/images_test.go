package basispoints

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"
)

func TestHTTPSImagesPreserveURLsAndText(t *testing.T) {
	for _, detail := range []string{"", "auto", "low", "high", "original"} {
		image := object{"type": "input_image", "image_url": "https://images.example/photo.png?signature=unchanged%2Fvalue&expires=123"}
		if detail != "" {
			image["detail"] = detail
		}
		item := object{"type": "message", "role": "user", "content": []any{object{"type": "input_text", "text": "Describe this image."}, image}}
		source := testSource()
		source["input"] = []any{item}
		wire, _ := mustPrepare(t, source, "scope", nil)
		items := mustTestValue[[]any](t, wire["input"])
		if !reflect.DeepEqual(items[len(items)-1], item) {
			t.Fatal("image URL, detail or neighboring text was changed")
		}
	}
}

func TestUnsupportedImageFormsReturnActionableErrors(t *testing.T) {
	for name, image := range map[string]object{
		"base64":          {"image_url": "data:image/png;base64,PRIVATE_IMAGE_BYTES"},
		"http":            {"image_url": "http://images.example/photo.png"},
		"relative":        {"image_url": "/photo.png"},
		"local file":      {"image_url": "file:///private/photo.png"},
		"missing host":    {"image_url": "https:///photo.png"},
		"credentials":     {"image_url": "https://private-secret:password@images.example/photo.png"},
		"URL object":      {"image_url": object{"url": "https://images.example/photo.png"}},
		"invalid file ID": {"file_id": "invalid-private"},
		"mixed file ID":   {"image_url": "https://images.example/photo.png", "file_id": "file-private"},
		"invalid detail":  {"image_url": "https://images.example/photo.png", "detail": "invalid"},
	} {
		t.Run(name, func(t *testing.T) {
			image["type"] = "input_image"
			source := testSource()
			source["input"] = []any{object{"role": "user", "content": []any{image}}}
			raw, _ := json.Marshal(source)
			_, _, err := Prepare(raw, "", nil)
			if err == nil {
				t.Fatal("unsupported image silently forwarded")
			}
			if strings.Contains(err.Error(), "PRIVATE_IMAGE_BYTES") || strings.Contains(err.Error(), "private-secret") {
				t.Fatal("image data or credentials leaked into the error")
			}
			if name == "base64" && (!strings.Contains(err.Error(), "HTTPS image URL") || !strings.Contains(err.Error(), "disable Basispoints")) {
				t.Fatalf("base64 rejection lacks a remedy: %v", err)
			}
		})
	}
}
