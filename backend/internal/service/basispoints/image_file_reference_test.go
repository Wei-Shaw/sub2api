package basispoints

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestMessageFileImagesOnlySendNativeFields(t *testing.T) {
	for _, detail := range []any{nil, "auto", "low", "high", "original"} {
		for _, kind := range []string{"message", "agent_message"} {
			t.Run(kind+"/"+text(detail), func(t *testing.T) {
				part := object{"type": "input_image", "file_id": "file-existing", "detail": detail, "client_metadata": "private-client-metadata"}
				source := testSource()
				source["input"] = []any{object{"type": kind, "role": "user", "author": "/root/worker", "recipient": "/root", "content": []any{
					object{"type": "input_text", "text": "before"}, part, object{"type": "input_text", "text": "after"},
				}}}
				wire, _ := mustPrepare(t, source, "file-reference", nil)
				items := mustTestValue[[]any](t, wire["input"])
				message := mustTestValue[object](t, items[len(items)-1])
				parts := mustTestValue[[]any](t, message["content"])
				require.Equal(t, object{"type": "input_image", "file_id": "file-existing"}, parts[len(parts)-2])
				require.Equal(t, "before", mustTestValue[object](t, parts[len(parts)-3])["text"])
				require.Equal(t, "after", mustTestValue[object](t, parts[len(parts)-1])["text"])
				require.Equal(t, "private-client-metadata", part["client_metadata"], "do not mutate caller-owned history")
			})
		}
	}
}

func TestNativeImageFileReferencesAfterUploadAndReplay(t *testing.T) {
	url, _ := nativeTestURL(t)
	raw := nativeTestRequest(t, url, url)
	cache := new(AttachmentCache)
	uploads := 0
	for range 2 {
		plan, err := PrepareNativeImages(raw)
		require.NoError(t, err)
		_, bridge, err := plan.PrepareWithCatalog("same-scope", nil, nil)
		require.NoError(t, err)
		body, err := plan.Upload(context.Background(), cache, "same-scope", func(context.Context, InlineAttachment) (string, error) {
			uploads++
			return "file-uploaded", nil
		})
		require.NoError(t, err)
		wire, _, err := bridge.Reprepare(body)
		require.NoError(t, err)
		var decoded object
		require.NoError(t, decode(wire, &decoded))
		items := mustTestValue[[]any](t, decoded["input"])
		parts := mustTestValue[[]any](t, mustTestValue[object](t, items[len(items)-1])["content"])
		require.Equal(t, []any{
			object{"type": "input_image", "file_id": "file-uploaded"},
			object{"type": "input_image", "file_id": "file-uploaded"},
		}, parts)
	}
	require.Equal(t, 1, uploads, "normalization must preserve scoped attachment reuse")
}

func TestMessageFileImagesValidateBeforeNormalizing(t *testing.T) {
	for name, part := range map[string]object{
		"invalid ID":       {"type": "input_image", "file_id": "private-invalid-id", "detail": "auto"},
		"mixed references": {"type": "input_image", "file_id": "file-existing", "image_url": "https://example.com/private-image", "detail": "auto"},
		"invalid detail":   {"type": "input_image", "file_id": "file-existing", "detail": "private-invalid-detail"},
	} {
		t.Run(name, func(t *testing.T) {
			source := testSource()
			source["input"] = []any{object{"role": "user", "content": []any{part}}}
			raw, err := json.Marshal(source)
			require.NoError(t, err)
			_, _, err = Prepare(raw, "", nil)
			require.Error(t, err)
			require.NotContains(t, err.Error(), "private-")
		})
	}
}
