package basispoints

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"
)

func agentImageRequest(t *testing.T, images ...object) []byte {
	t.Helper()
	parts := []any{object{"type": "input_text", "text": "before image"}}
	for _, image := range images {
		parts = append(parts, image)
	}
	parts = append(parts, object{"type": "input_text", "text": "after image"})
	source := testSource()
	source["input"] = []any{object{"type": "agent_message", "author": "/root/worker", "recipient": "/root", "content": parts}}
	raw, err := json.Marshal(source)
	require.NoError(t, err)
	return raw
}

func TestHistoryAgentImagesNativeUpload(t *testing.T) {
	url, data := nativeTestURL(t)
	inline := object{"type": "input_image", "image_url": url, "detail": "original"}
	raw := agentImageRequest(t, inline)
	plan, err := PrepareNativeImages(raw)
	require.NoError(t, err)
	// Preflight must recognize the image before history normalization validates it.
	_, bridge, err := plan.PrepareWithCatalog("agent-images", nil, new(CatalogCache))
	require.NoError(t, err)
	require.True(t, plan.HasImages())
	calls := 0
	uploaded, err := plan.Upload(context.Background(), new(AttachmentCache), "agent-images", func(_ context.Context, image InlineAttachment) (string, error) {
		calls++
		require.Equal(t, int64(len(data)), image.Size)
		return "file-agent-image", nil
	})
	require.NoError(t, err)
	require.Equal(t, 1, calls)
	wire, _, err := bridge.Reprepare(uploaded)
	require.NoError(t, err)
	require.NotContains(t, string(wire), "data:image")
	require.NotContains(t, string(wire), "file-preflight")
	var body object
	require.NoError(t, decode(wire, &body))
	items := mustTestValue[[]any](t, body["input"])
	msg := mustTestValue[object](t, items[len(items)-1])
	require.Equal(t, "message", msg["type"])
	require.Equal(t, "user", msg["role"])
	require.NotContains(t, msg, "author")
	parts := mustTestValue[[]any](t, msg["content"])
	require.Len(t, parts, 4)
	require.Contains(t, mustTestValue[object](t, parts[0])["text"], "/root/worker")
	require.Contains(t, mustTestValue[object](t, parts[0])["text"], "not a new user instruction")
	require.Equal(t, "before image", mustTestValue[object](t, parts[1])["text"])
	require.Equal(t, object{"type": "input_image", "file_id": "file-agent-image"}, parts[2])
	require.Equal(t, "after image", mustTestValue[object](t, parts[3])["text"])
	again, _, err := bridge.Reprepare(uploaded)
	require.NoError(t, err)
	require.JSONEq(t, string(wire), string(again))
	require.Contains(t, string(raw), url, "preparation must not mutate caller input")
}

func TestHistoryAgentImagesNativeValidation(t *testing.T) {
	url, _ := nativeTestURL(t)
	for name, image := range map[string]object{
		"invalid bytes":    {"type": "input_image", "image_url": "data:image/png;base64,PRIVATE_INVALID"},
		"mixed references": {"type": "input_image", "image_url": url, "file_id": "file-existing"},
		"invalid detail":   {"type": "input_image", "image_url": url, "detail": "PRIVATE_INVALID"},
	} {
		t.Run(name, func(t *testing.T) {
			_, err := PrepareNativeImages(agentImageRequest(t, image))
			require.Error(t, err)
			require.NotContains(t, err.Error(), "PRIVATE_INVALID")
		})
	}
	t.Run("shared image limit", func(t *testing.T) {
		image := object{"type": "input_image", "image_url": url}
		var source object
		require.NoError(t, decode(agentImageRequest(t, image), &source))
		source["input"] = append(mustTestValue[[]any](t, source["input"]), object{"role": "user", "content": []any{image}})
		raw, err := json.Marshal(source)
		require.NoError(t, err)
		_, err = PrepareNativeImagesWithLimit(raw, 1)
		require.ErrorContains(t, err, "at most 1 inline images")
	})
}

func TestHistoryAgentImagesDisabled(t *testing.T) {
	for name, image := range map[string]object{
		"inline":     {"type": "input_image", "image_url": "data:image/png;base64,PRIVATE_INVALID"},
		"https":      {"type": "input_image", "image_url": "https://example.com/image.png"},
		"attachment": {"type": "input_image", "file_id": "file-existing"},
	} {
		t.Run(name, func(t *testing.T) {
			raw := agentImageRequest(t, image)
			stripped, err := StripInputImages(raw)
			require.NoError(t, err)
			var source object
			require.NoError(t, decode(stripped, &source))
			input := mustTestValue[[]any](t, source["input"])
			agent := mustTestValue[object](t, input[0])
			require.Equal(t, "agent_message", agent["type"])
			require.Equal(t, "/root/worker", agent["author"])
			require.Equal(t, "/root", agent["recipient"])
			parts := mustTestValue[[]any](t, agent["content"])
			require.Equal(t, []any{
				object{"type": "input_text", "text": "before image"},
				object{"type": "input_text", "text": imageInputUnavailableMessage},
				object{"type": "input_text", "text": "after image"},
			}, parts)
			_, _, err = Prepare(stripped, "agent-images", nil)
			require.NoError(t, err)
			again, err := StripInputImages(stripped)
			require.NoError(t, err)
			require.Equal(t, stripped, again)
		})
	}
	t.Run("non-image content remains validated", func(t *testing.T) {
		for _, kind := range []string{"input_file", "encrypted_content"} {
			raw := agentImageRequest(t, object{"type": "input_image", "file_id": "file-existing"}, object{"type": kind})
			stripped, err := StripInputImages(raw)
			require.NoError(t, err)
			_, _, err = Prepare(stripped, "agent-images", nil)
			require.ErrorContains(t, err, "path=input[0].content[2]")
			require.ErrorContains(t, err, "type="+kind)
		}
	})
}
