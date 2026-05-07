package service

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

const testImageID = "img_abcdefghijklmnopqrstuvwxyzABCDEF"

type fakeOpenCodePublicSettingsProvider struct {
	settings *PublicSettings
	err      error
}

func (f *fakeOpenCodePublicSettingsProvider) GetPublicSettings(ctx context.Context) (*PublicSettings, error) {
	return f.settings, f.err
}

func newTestStoreWithImage(t *testing.T, id string, format string, data []byte) *OpenAIGeneratedImageStore {
	t.Helper()
	store := newTestOpenAIGeneratedImageStore(t, fixedNow)
	_, err := store.saveDecodedForTest(id, format, data)
	require.NoError(t, err)
	return store
}

func mustJSONBytes(t *testing.T, value any) []byte {
	t.Helper()
	data, err := json.Marshal(value)
	require.NoError(t, err)
	return data
}

func resetOpenCodeUnavailableImageReportsForTest(t *testing.T) {
	t.Helper()
	old := openCodeUnavailableImageReports
	openCodeUnavailableImageReports = newOpenCodeUnavailableImageReportCache(256, time.Hour)
	t.Cleanup(func() { openCodeUnavailableImageReports = old })
}

func requireContainsOpenCodeGeneratedImageSpecificMarker(t *testing.T, text string) {
	t.Helper()
	matches := openCodeRehydrateSpecificMarkerPattern.FindAllStringSubmatch(text, -1)
	require.NotEmpty(t, matches, "message should contain a complete new image marker")
	for _, match := range matches {
		require.Len(t, match, 2)
		require.Regexp(t, `^img_[A-Za-z0-9_-]{32,}$`, match[1])
	}
}

func openCodeSpecificImageMarkerForTest(id string) string {
	return "[[sub2api-" + "generated-image:id=" + id + "]]"
}

func openCodeLegacyImageMarkerForTest(id string) string {
	return "sub2api" + "-image://" + id
}

func openCodeGeneratedImageDownloadPathForTest(id string) string {
	return "/sub2api/" + "generated-images/" + id + ".png"
}

func openCodeGeneratedImageDownloadURLForTest(baseURL string, id string) string {
	return strings.TrimRight(baseURL, "/") + openCodeGeneratedImageDownloadPathForTest(id)
}

const capturedStyleCompressedLegacyImageID = "img_oldoldoldoldoldoldoldoldoldoldoldold"

func openCodeImageRehydrateCapturedStyleBody(t *testing.T) []byte {
	t.Helper()
	return mustJSONBytes(t, map[string]any{
		"model": "gpt-5.5-Sys",
		"store": false,
		"reasoning": map[string]any{
			"effort":  "xhigh",
			"summary": "auto",
		},
		"include": []any{"reasoning.encrypted_content"},
		"input": []any{
			map[string]any{"role": "user", "content": []any{map[string]any{"type": "input_text", "text": "[Compressed conversation section]\nGenerated image: " + openCodeLegacyImageMarkerForTest(capturedStyleCompressedLegacyImageID)}}},
			openCodeSub2APIImageMessageForTest(testImageID, "Generated image saved by sub2api.\nImage reference: "+openCodeSpecificImageMarkerForTest(testImageID)),
			map[string]any{"role": "user", "content": []any{map[string]any{"type": "input_text", "text": "Continue"}}},
		},
	})
}

func openCodeTextMessageForTest(role string, text string) map[string]any {
	return map[string]any{"role": role, "content": []any{map[string]any{"type": "output_text", "text": text}}}
}

func openCodeSub2APIImageMessageForTest(id string, text string) map[string]any {
	msg := openCodeTextMessageForTest("assistant", text)
	msg["id"] = "msg_sub2api_" + id
	msg["type"] = "message"
	return msg
}

func requireOpenCodeImageToolPairAt(t *testing.T, input []any, index int) (map[string]any, map[string]any) {
	t.Helper()
	require.Less(t, index+1, len(input))
	call := input[index].(map[string]any)
	output := input[index+1].(map[string]any)
	require.Equal(t, "function_call", call["type"])
	require.Equal(t, openCodeGeneratedImageToolName, call["name"])
	require.Equal(t, "function_call_output", output["type"])
	require.Equal(t, call["call_id"], output["call_id"])
	return call, output
}

func openCodeImageToolIDsForTest(input []any) []string {
	ids := make([]string, 0)
	for _, item := range input {
		m, ok := item.(map[string]any)
		if !ok || m["type"] != "function_call" || m["name"] != openCodeGeneratedImageToolName {
			continue
		}
		ids = append(ids, imageIDFromOpenCodeImageCallID(m["call_id"]))
	}
	return ids
}

func requireExactSysDummyPairAt(t *testing.T, input []any, index int) {
	t.Helper()
	require.Less(t, index+1, len(input))
	call := input[index].(map[string]any)
	output := input[index+1].(map[string]any)
	require.Equal(t, "function_call", call["type"])
	require.Equal(t, sysDummyToolName, call["name"])
	require.Equal(t, sysDummyToolCallID, call["call_id"])
	require.Equal(t, "function_call_output", output["type"])
	require.Equal(t, sysDummyToolCallID, output["call_id"])
}

func TestRehydrateOpenCodeGeneratedImageMarkers_AddsSyntheticInputImage(t *testing.T) {
	store := newTestStoreWithImage(t, testImageID, "png", pngBytes)
	req := map[string]any{"input": []any{openCodeSub2APIImageMessageForTest(testImageID, "Generated image: "+openCodeLegacyImageMarkerForTest(testImageID))}}

	changed, err := rehydrateOpenCodeGeneratedImageMarkers(context.Background(), req, store, openCodeImageRehydrateOptions{MaxImages: 3})

	require.NoError(t, err)
	require.True(t, changed)
	input := req["input"].([]any)
	_, output := requireOpenCodeImageToolPairAt(t, input, 1)
	parts := output["output"].([]any)
	require.Equal(t, "input_image", parts[1].(map[string]any)["type"])
	require.Contains(t, parts[1].(map[string]any)["image_url"], "data:image/png;base64,")
}

func TestRehydrateOpenCodeGeneratedImageMarkers_IsIdempotent(t *testing.T) {
	store := newTestStoreWithImage(t, testImageID, "png", pngBytes)
	req := map[string]any{"input": openCodeSpecificImageMarkerForTest(testImageID)}

	changed, err := rehydrateOpenCodeGeneratedImageMarkers(context.Background(), req, store, openCodeImageRehydrateOptions{MaxImages: 3})
	require.NoError(t, err)
	require.True(t, changed)
	inputLen := len(req["input"].([]any))

	changed, err = rehydrateOpenCodeGeneratedImageMarkers(context.Background(), req, store, openCodeImageRehydrateOptions{MaxImages: 3})

	require.NoError(t, err)
	require.False(t, changed)
	require.Len(t, req["input"].([]any), inputLen)
	encoded, _ := json.Marshal(req)
	require.Equal(t, 1, strings.Count(string(encoded), `"type":"input_image"`))
}

func TestRehydrateOpenCodeGeneratedImageMarkers_ExpiredMarkerAddsUnavailableText(t *testing.T) {
	resetOpenCodeUnavailableImageReportsForTest(t)
	store := newTestStoreWithImage(t, testImageID, "png", pngBytes)
	store.now = func() time.Time { return fixedNow.Add(2 * time.Hour) }
	req := map[string]any{"input": openCodeSpecificImageMarkerForTest(testImageID)}

	changed, err := rehydrateOpenCodeGeneratedImageMarkers(context.Background(), req, store, openCodeImageRehydrateOptions{MaxImages: 3})

	require.NoError(t, err)
	require.True(t, changed)
	encoded, _ := json.Marshal(req)
	require.Contains(t, string(encoded), "no longer available")
	require.NotContains(t, string(encoded), "input_image")
}

func TestRehydrateOpenCodeGeneratedImageUnavailablePolicy(t *testing.T) {
	resetOpenCodeUnavailableImageReportsForTest(t)
	ctx := context.Background()
	store := newTestOpenAIGeneratedImageStore(t, fixedNow)
	id := testImageID
	history := map[string]any{"input": []any{
		openCodeSub2APIImageMessageForTest(id, "Generated image saved by sub2api.\nImage reference: "+openCodeSpecificImageMarkerForTest(id)),
	}}

	changed, err := rehydrateOpenCodeGeneratedImageMarkers(ctx, history, store, openCodeImageRehydrateOptions{MaxImages: 3})

	require.NoError(t, err)
	require.False(t, changed)

	current := map[string]any{"input": []any{
		map[string]any{"role": "user", "content": []any{map[string]any{"type": "input_text", "text": "restore " + openCodeSpecificImageMarkerForTest(id)}}},
	}}
	changed, err = rehydrateOpenCodeGeneratedImageMarkers(ctx, current, store, openCodeImageRehydrateOptions{MaxImages: 3})

	require.NoError(t, err)
	require.True(t, changed)
	input := current["input"].([]any)
	_, output := requireOpenCodeImageToolPairAt(t, input, 1)
	parts := output["output"].([]any)
	require.Len(t, parts, 1)
	require.Equal(t, "input_text", parts[0].(map[string]any)["type"])
	require.Contains(t, parts[0].(map[string]any)["text"], "no longer available")
	require.NotContains(t, string(mustJSONBytes(t, current)), "data:image")

	fresh := map[string]any{"input": []any{
		map[string]any{"role": "user", "content": []any{map[string]any{"type": "input_text", "text": "restore " + openCodeSpecificImageMarkerForTest(id)}}},
	}}
	changed, err = rehydrateOpenCodeGeneratedImageMarkers(ctx, fresh, store, openCodeImageRehydrateOptions{MaxImages: 3})

	require.NoError(t, err)
	require.False(t, changed)
}

func TestOpenCodeUnavailableImageReportCacheMarkTTLAndCapacity(t *testing.T) {
	now := fixedNow
	cache := newOpenCodeUnavailableImageReportCache(2, time.Hour)
	cache.now = func() time.Time { return now }

	require.True(t, cache.Mark("a"))
	require.False(t, cache.Mark("a"))
	now = now.Add(time.Hour + time.Second)
	require.True(t, cache.Mark("a"))
	require.True(t, cache.Mark("b"))
	require.True(t, cache.Mark("c"))
	require.True(t, cache.Mark("a"), "oldest key should have been evicted when capacity is exceeded")
}

func TestRehydrateOpenCodeGeneratedImageMarkers_DedupesByMostRecentAndCaps(t *testing.T) {
	ids := []string{
		"img_abcdefghijklmnopqrstuvwxyzABCDEF",
		"img_bcdefghijklmnopqrstuvwxyzABCDEFG",
		"img_cdefghijklmnopqrstuvwxyzABCDEFGH",
		"img_defghijklmnopqrstuvwxyzABCDEFGHI",
	}
	store := newTestOpenAIGeneratedImageStore(t, fixedNow)
	for _, id := range ids {
		_, err := store.saveDecodedForTest(id, "png", pngBytes)
		require.NoError(t, err)
	}
	req := map[string]any{"input": strings.Join([]string{openCodeSpecificImageMarkerForTest(ids[0]), openCodeSpecificImageMarkerForTest(ids[1]), openCodeSpecificImageMarkerForTest(ids[2]), openCodeSpecificImageMarkerForTest(ids[0]), openCodeSpecificImageMarkerForTest(ids[3])}, " ")}

	changed, err := rehydrateOpenCodeGeneratedImageMarkers(context.Background(), req, store, openCodeImageRehydrateOptions{MaxImages: 3})

	require.NoError(t, err)
	require.True(t, changed)
	encoded, _ := json.Marshal(req)
	require.Equal(t, 3, strings.Count(string(encoded), `"type":"input_image"`))
	input := req["input"].([]any)
	require.Equal(t, []string{ids[2], ids[0], ids[3]}, openCodeImageToolIDsForTest(input))
}

func TestRehydrateOpenCodeGeneratedImageMarkers_CapIsIdempotent(t *testing.T) {
	ids := []string{
		"img_abcdefghijklmnopqrstuvwxyzABCDEF",
		"img_bcdefghijklmnopqrstuvwxyzABCDEFG",
		"img_cdefghijklmnopqrstuvwxyzABCDEFGH",
		"img_defghijklmnopqrstuvwxyzABCDEFGHI",
	}
	store := newTestOpenAIGeneratedImageStore(t, fixedNow)
	for _, id := range ids {
		_, err := store.saveDecodedForTest(id, "png", pngBytes)
		require.NoError(t, err)
	}
	req := map[string]any{"input": strings.Join([]string{openCodeSpecificImageMarkerForTest(ids[0]), openCodeSpecificImageMarkerForTest(ids[1]), openCodeSpecificImageMarkerForTest(ids[2]), openCodeSpecificImageMarkerForTest(ids[3])}, " ")}

	changed, err := rehydrateOpenCodeGeneratedImageMarkers(context.Background(), req, store, openCodeImageRehydrateOptions{MaxImages: 3})
	require.NoError(t, err)
	require.True(t, changed)
	inputLen := len(req["input"].([]any))

	changed, err = rehydrateOpenCodeGeneratedImageMarkers(context.Background(), req, store, openCodeImageRehydrateOptions{MaxImages: 3})

	require.NoError(t, err)
	require.False(t, changed)
	require.Len(t, req["input"].([]any), inputLen)
	encoded, _ := json.Marshal(req)
	require.Equal(t, 3, strings.Count(string(encoded), `"type":"input_image"`))
	require.NotContains(t, string(encoded), `"call_id":"call_sub2api_image_`+ids[0]+`"`)
	input := req["input"].([]any)
	require.Equal(t, []string{ids[1], ids[2], ids[3]}, openCodeImageToolIDsForTest(input))
}

func TestRehydrateOpenCodeGeneratedImageMarkers_TooLargeSkipsInputImage(t *testing.T) {
	resetOpenCodeUnavailableImageReportsForTest(t)
	store := newTestStoreWithImage(t, testImageID, "png", pngBytes)
	req := map[string]any{"input": openCodeSpecificImageMarkerForTest(testImageID)}

	changed, err := rehydrateOpenCodeGeneratedImageMarkers(context.Background(), req, store, openCodeImageRehydrateOptions{MaxImages: 3, MaxRehydrateBytes: 4})

	require.NoError(t, err)
	require.True(t, changed)
	encoded, _ := json.Marshal(req)
	require.Contains(t, string(encoded), "no longer available")
	require.NotContains(t, string(encoded), "input_image")
}

func TestRehydrateOpenCodeGeneratedImageMarkers_MaxRehydrateBytesLimitsLoad(t *testing.T) {
	resetOpenCodeUnavailableImageReportsForTest(t)
	store := newTestStoreWithImage(t, testImageID, "png", pngBytes)
	rec, _, err := store.Load(context.Background(), testImageID)
	require.NoError(t, err)
	imagePath, err := safeOpenAIGeneratedImagePath(store.root, rec.Filename)
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(imagePath, append(append([]byte(nil), pngBytes...), []byte("extra-bytes")...), 0o600))
	req := map[string]any{"input": openCodeSpecificImageMarkerForTest(testImageID)}

	changed, err := rehydrateOpenCodeGeneratedImageMarkers(context.Background(), req, store, openCodeImageRehydrateOptions{MaxImages: 3, MaxRehydrateBytes: 4})

	require.NoError(t, err)
	require.True(t, changed)
	encoded, _ := json.Marshal(req)
	require.Contains(t, string(encoded), "no longer available")
	require.NotContains(t, string(encoded), "image bytes unavailable")
	require.NotContains(t, string(encoded), "input_image")
}

func TestRehydrateOpenCodeGeneratedImageMarkers_ScansDownloadPathsAndAbsoluteURLs(t *testing.T) {
	ids := []string{"img_abcdefghijklmnopqrstuvwxyzABCDEF", "img_bcdefghijklmnopqrstuvwxyzABCDEFG"}
	store := newTestOpenAIGeneratedImageStore(t, fixedNow)
	for _, id := range ids {
		_, err := store.saveDecodedForTest(id, "png", pngBytes)
		require.NoError(t, err)
	}
	req := map[string]any{"input": []any{openCodeSub2APIImageMessageForTest(ids[0], strings.Join([]string{
		"Generated image: " + openCodeLegacyImageMarkerForTest(ids[0]),
		"Download: " + openCodeGeneratedImageDownloadURLForTest("https://example.com", ids[1]),
	}, "\n"))}}

	changed, err := rehydrateOpenCodeGeneratedImageMarkers(context.Background(), req, store, openCodeImageRehydrateOptions{MaxImages: 3})

	require.NoError(t, err)
	require.True(t, changed)
	encoded, _ := json.Marshal(req)
	require.Equal(t, 2, strings.Count(string(encoded), `"type":"input_image"`))
}

func TestRehydrateOpenCodeGeneratedImageMarkersCapturedStyleSkipsCompressedLegacy(t *testing.T) {
	var req map[string]any
	require.NoError(t, json.Unmarshal(openCodeImageRehydrateCapturedStyleBody(t), &req))
	store := newTestStoreWithImage(t, testImageID, "png", pngBytes)

	changed, err := rehydrateOpenCodeGeneratedImageMarkers(context.Background(), req, store, openCodeImageRehydrateOptions{MaxImages: 3})

	require.NoError(t, err)
	require.True(t, changed)
	input := req["input"].([]any)
	call, output := requireOpenCodeImageToolPairAt(t, input, 2)
	callID := call["call_id"].(string)
	require.Contains(t, callID, testImageID)
	require.NotContains(t, callID, capturedStyleCompressedLegacyImageID)
	require.Equal(t, callID, output["call_id"])
	require.Equal(t, "Continue", gjson.GetBytes(mustJSONBytes(t, input[4]), "content.0.text").String())

	encoded := string(mustJSONBytes(t, req))
	require.Equal(t, 1, strings.Count(encoded, `"name":"`+openCodeGeneratedImageToolName+`"`))
	require.Contains(t, encoded, "call_sub2api_image_"+testImageID)
	require.NotContains(t, encoded, "call_sub2api_image_"+capturedStyleCompressedLegacyImageID)
}

func TestRehydrateOpenCodeGeneratedImageMarkersInsertsToolOutputNearMarker(t *testing.T) {
	ctx := context.Background()
	id := testImageID
	store := newTestStoreWithImage(t, id, "png", pngBytes)
	req := map[string]any{"input": []any{
		map[string]any{"role": "user", "content": []any{map[string]any{"type": "input_text", "text": "before"}}},
		openCodeSub2APIImageMessageForTest(id, "Generated image saved by sub2api.\nImage reference: "+openCodeSpecificImageMarkerForTest(id)),
		map[string]any{"role": "user", "content": []any{map[string]any{"type": "input_text", "text": "after"}}},
	}}

	changed, err := rehydrateOpenCodeGeneratedImageMarkers(ctx, req, store, openCodeImageRehydrateOptions{MaxImages: 3})

	require.NoError(t, err)
	require.True(t, changed)
	input := req["input"].([]any)
	require.Equal(t, "function_call", input[2].(map[string]any)["type"])
	require.Equal(t, openCodeGeneratedImageToolName, input[2].(map[string]any)["name"])
	require.Equal(t, "function_call_output", input[3].(map[string]any)["type"])
	require.Equal(t, input[2].(map[string]any)["call_id"], input[3].(map[string]any)["call_id"])
	output := input[3].(map[string]any)["output"].([]any)
	require.Equal(t, "input_text", output[0].(map[string]any)["type"])
	require.Equal(t, "input_image", output[1].(map[string]any)["type"])
	require.Contains(t, output[1].(map[string]any)["image_url"], "data:image/png;base64,")
	require.Equal(t, "after", gjson.GetBytes(mustJSONBytes(t, input[4]), "content.0.text").String())
}

func TestRehydrateOpenCodeGeneratedImageMarkersPreservesToolTailAndDisambiguatesCallID(t *testing.T) {
	ctx := context.Background()
	id := testImageID
	baseCallID := "call_sub2api_image_" + id
	for _, tc := range []struct {
		name       string
		occupied   []any
		wantCallID string
	}{
		{
			name: "base conflict uses first duplicate suffix",
			occupied: []any{
				map[string]any{"type": "function_call", "call_id": baseCallID, "name": "real_tool", "arguments": "{}"},
			},
			wantCallID: baseCallID + "_dup1",
		},
		{
			name: "base and first duplicate conflict uses second duplicate suffix",
			occupied: []any{
				map[string]any{"type": "function_call", "call_id": baseCallID, "name": "real_tool", "arguments": "{}"},
				map[string]any{"type": "function_call_output", "call_id": baseCallID + "_dup1", "output": "occupied by existing tool output"},
			},
			wantCallID: baseCallID + "_dup2",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			store := newTestStoreWithImage(t, id, "png", pngBytes)
			items := append([]any{}, tc.occupied...)
			items = append(items,
				openCodeSub2APIImageMessageForTest(id, "Generated image saved by sub2api.\nImage reference: "+openCodeSpecificImageMarkerForTest(id)),
				map[string]any{"type": "function_call_output", "call_id": "call_real_tail", "output": "real tail"},
			)
			req := map[string]any{"input": items}

			changed, err := rehydrateOpenCodeGeneratedImageMarkers(ctx, req, store, openCodeImageRehydrateOptions{MaxImages: 3})

			require.NoError(t, err)
			require.True(t, changed)
			input := req["input"].([]any)
			callIndex := len(tc.occupied) + 1
			call := input[callIndex].(map[string]any)
			out := input[callIndex+1].(map[string]any)
			require.Equal(t, openCodeGeneratedImageToolName, call["name"])
			require.Equal(t, tc.wantCallID, call["call_id"])
			require.Equal(t, call["call_id"], out["call_id"])
			require.Equal(t, "call_real_tail", input[len(input)-1].(map[string]any)["call_id"])
		})
	}
}

func TestRehydrateOpenCodeGeneratedImageMarkersSkipsSysDummyAndKeepsRecentThreeOrder(t *testing.T) {
	ctx := context.Background()
	ids := []string{
		"img_abcdefghijklmnopqrstuvwxyzABCDEF",
		"img_bcdefghijklmnopqrstuvwxyzABCDEFG",
		"img_cdefghijklmnopqrstuvwxyzABCDEFGH",
		"img_defghijklmnopqrstuvwxyzABCDEFGHI",
	}
	dummyOnlyID := "img_dummyonlydummyonlydummyonlydummyonly"
	store := newTestOpenAIGeneratedImageStore(t, fixedNow)
	for _, id := range ids {
		_, err := store.saveDecodedForTest(id, "png", pngBytes)
		require.NoError(t, err)
	}
	req := map[string]any{"input": []any{
		openCodeSub2APIImageMessageForTest(ids[0], "Generated image saved by sub2api.\nImage reference: "+openCodeSpecificImageMarkerForTest(ids[0])),
		openCodeSub2APIImageMessageForTest(ids[1], "Generated image saved by sub2api.\nImage reference: "+openCodeSpecificImageMarkerForTest(ids[1])),
		openCodeSub2APIImageMessageForTest(ids[2], "Generated image saved by sub2api.\nImage reference: "+openCodeSpecificImageMarkerForTest(ids[2])),
		openCodeSub2APIImageMessageForTest(ids[0], "Generated image saved by sub2api.\nImage reference: "+openCodeSpecificImageMarkerForTest(ids[0])),
		openCodeSub2APIImageMessageForTest(ids[3], "Generated image saved by sub2api.\nImage reference: "+openCodeSpecificImageMarkerForTest(ids[3])),
		map[string]any{"type": "function_call", "call_id": sysDummyToolCallID, "name": sysDummyToolName, "arguments": "{}"},
		map[string]any{"type": "function_call_output", "call_id": sysDummyToolCallID, "output": sysDummyToolOutput + " " + openCodeSpecificImageMarkerForTest(dummyOnlyID)},
	}}

	changed, err := rehydrateOpenCodeGeneratedImageMarkers(ctx, req, store, openCodeImageRehydrateOptions{MaxImages: 3})

	require.NoError(t, err)
	require.True(t, changed)
	input := req["input"].([]any)
	requireExactSysDummyPairAt(t, input, len(input)-2)
	encoded := string(mustJSONBytes(t, req))
	require.Equal(t, 3, strings.Count(encoded, `"name":"`+openCodeGeneratedImageToolName+`"`))
	require.NotContains(t, encoded, "call_sub2api_image_"+dummyOnlyID)
	var got []string
	for i := 0; i < len(input); i++ {
		item, ok := input[i].(map[string]any)
		if !ok || item["type"] != "function_call" || item["name"] != openCodeGeneratedImageToolName {
			continue
		}
		require.Less(t, i+1, len(input))
		output := input[i+1].(map[string]any)
		require.Equal(t, "function_call_output", output["type"])
		require.Equal(t, item["call_id"], output["call_id"])
		got = append(got, imageIDFromOpenCodeImageCallID(item["call_id"]))
	}
	require.Equal(t, []string{ids[2], ids[0], ids[3]}, got)
}

func TestRehydrateOpenCodeGeneratedImageMarkersIgnoresSourcesInsideExistingSysDummyTail(t *testing.T) {
	ctx := context.Background()
	id := testImageID
	store := newTestStoreWithImage(t, id, "png", pngBytes)
	_, ok := findSysDummyTail([]any{
		map[string]any{"type": " function_call ", "call_id": sysDummyToolCallID, "name": sysDummyToolName, "arguments": "{}"},
		map[string]any{"type": "function_call_output", "call_id": sysDummyToolCallID, "output": sysDummyToolOutput},
	})
	require.False(t, ok, "sys dummy tail matching must be exact, not whitespace-normalized")
	req := map[string]any{"input": []any{
		map[string]any{"role": "user", "content": []any{map[string]any{"type": "input_text", "text": "before dummy"}}},
		map[string]any{"type": "function_call", "call_id": sysDummyToolCallID, "name": sysDummyToolName, "arguments": "{}"},
		map[string]any{"type": "function_call_output", "call_id": sysDummyToolCallID, "output": sysDummyToolOutput + " " + openCodeSpecificImageMarkerForTest(id)},
	}}

	changed, err := rehydrateOpenCodeGeneratedImageMarkers(ctx, req, store, openCodeImageRehydrateOptions{MaxImages: 3})

	require.NoError(t, err)
	require.False(t, changed)
}

func TestScanOpenCodeGeneratedImageMarkersSkipsCompressedAndRepeatedText(t *testing.T) {
	id := "img_abcdefghijklmnopqrstuvwxyzABCDEF"
	specificMarker := openCodeSpecificImageMarkerForTest(id)
	input := []any{
		map[string]any{"role": "user", "content": []any{map[string]any{"type": "input_text", "text": "[Compressed conversation section]\nGenerated image: " + openCodeLegacyImageMarkerForTest(id)}}},
		map[string]any{"role": "assistant", "content": []any{map[string]any{"type": "output_text", "text": "I repeat " + specificMarker}}},
		openCodeSub2APIImageMessageForTest(id, "Generated image saved by sub2api.\nImage reference: "+specificMarker),
	}

	matches := scanOpenCodeGeneratedImageMarkerRefs(input)

	require.Len(t, matches, 1)
	require.Equal(t, id, matches[0].id)
	require.Equal(t, 2, matches[0].index)
	require.False(t, matches[0].legacy)
	require.Equal(t, 0, matches[0].seq)
	require.False(t, matches[0].explicit)
	require.False(t, matches[0].currentUser)
}

func TestScanOpenCodeGeneratedImageMarkersSourceMatrix(t *testing.T) {
	id := "img_abcdefghijklmnopqrstuvwxyzABCDEF"
	otherID := "img_bcdefghijklmnopqrstuvwxyzABCDEFG"
	specificMarker := openCodeSpecificImageMarkerForTest(id)
	otherSpecificMarker := openCodeSpecificImageMarkerForTest(otherID)
	legacyMarker := openCodeLegacyImageMarkerForTest(id)
	legacyDownloadLine := "I'll download from URL: " + openCodeGeneratedImageDownloadPathForTest(id)

	for _, tc := range []struct {
		name  string
		input []any
		want  []openCodeImageMarkerRef
	}{
		{
			name:  "reasoning item is skipped",
			input: []any{map[string]any{"type": "reasoning", "text": specificMarker}},
		},
		{
			name:  "ordinary user legacy marker is ignored",
			input: []any{map[string]any{"role": "user", "content": []any{map[string]any{"type": "input_text", "text": "please discuss " + legacyMarker}}}},
		},
		{
			name: "historical user specific marker is ignored",
			input: []any{
				map[string]any{"role": "user", "content": []any{map[string]any{"type": "input_text", "text": specificMarker}}},
				map[string]any{"role": "assistant", "content": []any{map[string]any{"type": "output_text", "text": "ordinary response"}}},
				map[string]any{"role": "user", "content": []any{map[string]any{"type": "input_text", "text": "latest user text"}}},
			},
		},
		{
			name:  "current user specific marker is explicit",
			input: []any{map[string]any{"role": "user", "content": []any{map[string]any{"type": "input_text", "text": specificMarker}}}},
			want:  []openCodeImageMarkerRef{{id: id, index: 0, seq: 0, explicit: true, currentUser: true}},
		},
		{
			name: "current user skips later blocked user item",
			input: []any{
				map[string]any{"role": "user", "content": []any{map[string]any{"type": "input_text", "text": specificMarker}}},
				map[string]any{"role": "user", "content": []any{map[string]any{"type": "input_text", "text": "[Compressed conversation section]\n" + otherSpecificMarker}}},
			},
			want: []openCodeImageMarkerRef{{id: id, index: 0, seq: 0, explicit: true, currentUser: true}},
		},
		{
			name:  "legacy download line is allowed in sub2api assistant image message",
			input: []any{openCodeSub2APIImageMessageForTest(id, "Generated image: "+legacyMarker+"\n"+legacyDownloadLine)},
			want: []openCodeImageMarkerRef{
				{id: id, index: 0, legacy: true, seq: 0},
				{id: id, index: 0, legacy: true, seq: 1},
			},
		},
		{
			name:  "legacy download line allows absolute URL path prefix",
			input: []any{openCodeSub2APIImageMessageForTest(id, "Generated image: "+legacyMarker+"\nDownload: "+openCodeGeneratedImageDownloadURLForTest("https://example.com/api/v1", id))},
			want: []openCodeImageMarkerRef{
				{id: id, index: 0, legacy: true, seq: 0},
				{id: id, index: 0, legacy: true, seq: 1},
			},
		},
		{
			name:  "ordinary assistant legacy repeat is ignored",
			input: []any{map[string]any{"role": "assistant", "content": []any{map[string]any{"type": "output_text", "text": "I repeat " + legacyMarker}}}},
		},
		{
			name: "sys dummy and synthetic image tool items are skipped",
			input: []any{
				map[string]any{"type": "function_call", "call_id": sysDummyToolCallID, "name": sysDummyToolName, "text": specificMarker},
				map[string]any{"type": "function_call_output", "call_id": sysDummyToolCallID, "output": sysDummyToolOutput, "text": specificMarker},
				map[string]any{"type": "function_call", "call_id": "call_sub2api_image_" + otherID, "name": "sub2api_generated_image", "text": otherSpecificMarker},
				map[string]any{"type": "function_call_output", "call_id": "call_sub2api_image_" + otherID, "text": otherSpecificMarker},
			},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			matches := scanOpenCodeGeneratedImageMarkerRefs(tc.input)
			if len(tc.want) == 0 {
				require.Empty(t, matches)
				return
			}
			require.Equal(t, tc.want, matches)
		})
	}
}

func TestRedactOpenCodeGeneratedImagesForOps_RemovesDataURLsAndResults(t *testing.T) {
	body := []byte(`{"input":[{"content":[{"type":"input_image","image_url":"data:image/png;base64,` + pngB64 + `"}]}],"output":[{"type":"image_generation_call","result":"` + pngB64 + `"}]}`)

	redacted := redactOpenCodeGeneratedImagesForOps(body)

	require.NotContains(t, string(redacted), "data:image")
	require.NotContains(t, string(redacted), pngB64)
	require.Contains(t, string(redacted), "[redacted-input-image]")
	require.Contains(t, string(redacted), "[redacted-image-result]")
}

func TestRedactOpenCodeGeneratedImagesForOpsRedactsFunctionOutputArrayImage(t *testing.T) {
	sample := "aGVsbG8="
	imageURL := "data:image/png;base64," + sample
	body := mustJSONBytes(t, map[string]any{
		"input": []any{
			map[string]any{
				"type":    "function_call_output",
				"call_id": "call_1",
				"output": []any{
					map[string]any{"type": "input_text", "text": "ok"},
					map[string]any{"type": "input_image", "image_url": imageURL},
				},
			},
		},
	})

	redacted := string(redactOpenCodeGeneratedImagesForOps(body))

	require.True(t, gjson.Valid(redacted))
	require.NotContains(t, redacted, "data:image")
	require.NotContains(t, redacted, sample)
	require.Contains(t, redacted, "[redacted-input-image]")
}

func TestRedactOpenCodeGeneratedImagesForOps_MalformedJSONFailClosed(t *testing.T) {
	body := []byte(`{"input":"data:image/png;base64,` + pngB64 + `","marker":"sub2api-image://` + testImageID + `","path":"/sub2api/generated-images/` + testImageID + `.png"`)

	redacted := redactOpenCodeGeneratedImagesForOps(body)

	require.NotContains(t, string(redacted), "data:image")
	require.NotContains(t, string(redacted), pngB64)
	require.NotContains(t, string(redacted), testImageID)
	require.Contains(t, string(redacted), "[redacted-input-image]")
	require.Contains(t, string(redacted), "/sub2api/generated-images/[redacted]")
	require.Contains(t, string(redacted), "sub2api-image://[redacted]")
}

func TestRedactOpenCodeGeneratedImagesForOps_MalformedJSONRedactsBareImageBase64Fields(t *testing.T) {
	body := []byte(`{"type":"image_generation_call","result":"` + pngB64 + `","partial_image_b64":"` + pngB64 + `"`)

	redacted := redactOpenCodeGeneratedImagesForOps(body)

	require.NotContains(t, string(redacted), pngB64)
	require.Contains(t, string(redacted), "[redacted-image-result]")
	require.Contains(t, string(redacted), "[redacted-partial-image]")
}

func TestRedactOpenCodeGeneratedImagesForOpsRedactsSpecificMarker(t *testing.T) {
	id := "img_" + "abcdefghijklmnopqrstuvwxyzABCDEF"
	specificMarker := "[[sub2api-generated-image:id=" + id + "]]"
	downloadURL := "https://example.com" + "/sub2api/" + "generated-images/" + id + ".png"
	body := mustJSONBytes(t, map[string]any{
		"input": []any{
			map[string]any{
				"content": []any{
					map[string]any{"type": "output_text", "text": "marker " + specificMarker + " url " + downloadURL},
				},
			},
		},
	})

	redacted := redactOpenCodeGeneratedImagesForOps(body)

	redactedText := string(redacted)
	require.False(t, strings.Contains(redactedText, id), "ops redaction should remove raw image id")
	require.True(t, strings.Contains(redactedText, "[[sub2api-generated-image:id=[redacted]]]"), "ops redaction should preserve redacted marker form")
	require.True(t, strings.Contains(redactedText, "[redacted-generated-image-url]"), "ops redaction should redact generated image URL")
}

func TestRedactedGatewayUpstreamBodyForLogRemovesGeneratedImagePayloads(t *testing.T) {
	sample := "aGVsbG8="
	body := []byte(`{"error":{"message":"invalid generated image data:image/png;base64,` + sample + `","partial_image_b64":"` + sample + `"}}`)

	redacted := redactedGatewayUpstreamBodyForLog(body, 4096)

	require.NotContains(t, redacted, "data:image")
	require.NotContains(t, redacted, sample)
	require.Contains(t, redacted, "[redacted-input-image]")
}

func TestRewriteOpenCodeImageGenerationOutput_ReplacesImageCallWithMessage(t *testing.T) {
	store := newTestOpenAIGeneratedImageStore(t, fixedNow)
	body := []byte(`{"id":"resp_1","output":[{"id":"ig_1","type":"image_generation_call","status":"completed","result":"` + pngB64 + `","output_format":"png"}],"usage":{"input_tokens":1,"output_tokens":2}}`)

	patched, changed, err := rewriteOpenCodeImageGenerationOutput(context.Background(), body, store, openCodeImageRewriteOptions{BaseURL: "https://example.com"})

	require.NoError(t, err)
	require.True(t, changed)
	require.Regexp(t, `^msg_sub2api_img_`, gjson.GetBytes(patched, "output.0.id").String())
	require.Equal(t, "message", gjson.GetBytes(patched, "output.0.type").String())
	require.Equal(t, "completed", gjson.GetBytes(patched, "output.0.status").String())
	require.Equal(t, "assistant", gjson.GetBytes(patched, "output.0.role").String())
	require.Equal(t, "output_text", gjson.GetBytes(patched, "output.0.content.0.type").String())
	require.Equal(t, int64(0), gjson.GetBytes(patched, "output.0.content.0.annotations.#").Int())
	requireContainsOpenCodeGeneratedImageSpecificMarker(t, gjson.GetBytes(patched, "output.0.content.0.text").String())
	require.Contains(t, gjson.GetBytes(patched, "output.0.content.0.text").String(), "https://example.com/sub2api/generated-images/")
	require.NotContains(t, string(patched), "image_generation_call")
	require.NotContains(t, string(patched), pngB64)
}

func TestRewriteOpenCodeImageGenerationOutput_DoesNotExposeContinuationToolCall(t *testing.T) {
	store := newTestOpenAIGeneratedImageStore(t, fixedNow)
	body := []byte(`{"id":"resp_1","output":[{"id":"ig_1","type":"image_generation_call","status":"completed","result":"` + pngB64 + `","output_format":"png"}],"usage":{"input_tokens":1,"output_tokens":2}}`)

	patched, changed, err := rewriteOpenCodeImageGenerationOutput(context.Background(), body, store, openCodeImageRewriteOptions{BaseURL: "https://example.com"})

	require.NoError(t, err)
	require.True(t, changed)
	require.Equal(t, int64(1), gjson.GetBytes(patched, "output.#").Int())
	require.Equal(t, "message", gjson.GetBytes(patched, "output.0.type").String())
	require.Contains(t, gjson.GetBytes(patched, "output.0.content.0.text").String(), "Temporary download URL: https://example.com/sub2api/generated-images/")
	require.NotContains(t, string(patched), `"type":"function_call"`)
	require.NotContains(t, string(patched), `"name":"bash"`)
	require.NotContains(t, string(patched), "Server download path")
	require.NotContains(t, string(patched), pngB64)
}

func TestResolveOpenCodeImageDownloadBaseURL_PrefersConfiguredFrontendURL(t *testing.T) {
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", nil)
	c.Request.Host = "attacker.example"
	cfg := &config.Config{}
	cfg.Server.FrontendURL = "https://sub2api.example/app/"

	require.Equal(t, "https://sub2api.example/app", resolveOpenCodeImageDownloadBaseURL(c, cfg))
}

func TestOpenAIGatewayResolveOpenCodeImageDownloadBaseURL_PrefersPublicAPIBaseURL(t *testing.T) {
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", nil)
	c.Request.Host = "attacker.example"
	cfg := &config.Config{}
	cfg.Server.FrontendURL = "https://frontend.example/app/"
	svc := &OpenAIGatewayService{
		cfg: cfg,
		publicSettingsProvider: &fakeOpenCodePublicSettingsProvider{
			settings: &PublicSettings{APIBaseURL: "https://api.example.com/v1/"},
		},
	}

	require.Equal(t, "https://api.example.com", svc.resolveOpenCodeImageDownloadBaseURL(context.Background(), c))
}

func TestOpenAIGatewayResolveOpenCodeImageDownloadBaseURL_FallsBackWhenPublicAPIBaseURLUnsafe(t *testing.T) {
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", nil)
	c.Request.Host = "attacker.example"
	cfg := &config.Config{}
	cfg.Server.FrontendURL = "https://frontend.example/app/"
	svc := &OpenAIGatewayService{
		cfg: cfg,
		publicSettingsProvider: &fakeOpenCodePublicSettingsProvider{
			settings: &PublicSettings{APIBaseURL: "javascript:alert(1)"},
		},
	}

	require.Equal(t, "https://frontend.example/app", svc.resolveOpenCodeImageDownloadBaseURL(context.Background(), c))
}

func TestResolveOpenCodeImageDownloadBaseURL_RejectsUntrustedHostFallback(t *testing.T) {
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", nil)
	c.Request.Host = "attacker.example"

	require.Equal(t, "", resolveOpenCodeImageDownloadBaseURL(c, &config.Config{}))
}

func TestResolveOpenCodeImageDownloadBaseURL_UsesTrustedForwardedHost(t *testing.T) {
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", nil)
	c.Request.RemoteAddr = "10.0.0.10:12345"
	c.Request.Header.Set("X-Forwarded-Proto", "https")
	c.Request.Header.Set("X-Forwarded-Host", " images.example, ignored.example ")
	cfg := &config.Config{}
	cfg.Server.TrustedProxies = []string{"10.0.0.0/24"}

	require.Equal(t, "https://images.example", resolveOpenCodeImageDownloadBaseURL(c, cfg))
}

func TestResolveOpenCodeImageDownloadBaseURL_RejectsUntrustedForwardedHost(t *testing.T) {
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", nil)
	c.Request.RemoteAddr = "10.0.1.10:12345"
	c.Request.Header.Set("X-Forwarded-Proto", "https")
	c.Request.Header.Set("X-Forwarded-Host", "images.example")
	cfg := &config.Config{}
	cfg.Server.TrustedProxies = []string{"10.0.0.0/24"}

	require.Equal(t, "", resolveOpenCodeImageDownloadBaseURL(c, cfg))
}

func TestResolveOpenCodeImageDownloadBaseURL_UsesTrustedRequestHostFallback(t *testing.T) {
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", nil)
	c.Request.RemoteAddr = "10.0.0.10:12345"
	c.Request.Host = "images.example"
	c.Request.Header.Set("X-Forwarded-Proto", "https")
	cfg := &config.Config{}
	cfg.Server.TrustedProxies = []string{"10.0.0.10"}

	require.Equal(t, "https://images.example", resolveOpenCodeImageDownloadBaseURL(c, cfg))
}

func TestResolveOpenCodeImageDownloadBaseURL_RejectsUnsafeFallbackInputs(t *testing.T) {
	for _, tc := range []struct {
		name  string
		proto string
		host  string
	}{
		{name: "bad proto", proto: "javascript", host: "images.example"},
		{name: "host with path", proto: "https", host: "images.example/path"},
		{name: "host with newline", proto: "https", host: "images.example\nattacker.example"},
		{name: "host with query", proto: "https", host: "images.example?x=1"},
		{name: "host with userinfo", proto: "https", host: "user@images.example"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			c, _ := gin.CreateTestContext(httptest.NewRecorder())
			c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", nil)
			c.Request.RemoteAddr = "10.0.0.10:12345"
			c.Request.Header.Set("X-Forwarded-Proto", tc.proto)
			c.Request.Header.Set("X-Forwarded-Host", tc.host)
			cfg := &config.Config{}
			cfg.Server.TrustedProxies = []string{"10.0.0.0/24"}

			require.Equal(t, "", resolveOpenCodeImageDownloadBaseURL(c, cfg))
		})
	}
}

func TestBuildOpenCodeGeneratedImageMessage_UsesSpecificMarkerWithoutDownloadURLWhenBaseURLEmpty(t *testing.T) {
	id := "img_abcdefghijklmnopqrstuvwxyzABCDEF"
	rec := OpenAIGeneratedImageRecord{
		ID:        id,
		Filename:  id + ".png",
		Format:    "png",
		MIME:      "image/png",
		ExpiresAt: fixedNow.Add(time.Hour),
	}

	msg := buildOpenCodeGeneratedImageMessage(rec, openCodeImageRewriteOptions{})
	content := msg["content"].([]any)[0].(map[string]any)["text"].(string)
	specificMarker := "[[sub2api-generated-image:id=" + id + "]]"
	legacyMarker := "sub2api" + "-image://" + id

	require.True(t, strings.Contains(content, "Generated image saved by sub2api."), "message should describe saved image")
	require.True(t, strings.Contains(content, specificMarker), "message should contain new image marker")
	require.False(t, strings.Contains(content, legacyMarker), "message should not contain legacy image marker")
	require.NotContains(t, content, "Server download path")
	require.NotContains(t, content, "/sub2api/generated-images/")
	require.NotContains(t, content, "Do not treat")
	require.NotContains(t, content, "If no Download URL")
	require.NotContains(t, content, "Download URL:")
}

func TestBuildOpenCodeGeneratedImageMessage_IncludesTemporaryDownloadURLWhenBaseURLSet(t *testing.T) {
	id := "img_abcdefghijklmnopqrstuvwxyzABCDEF"
	rec := OpenAIGeneratedImageRecord{
		ID:        id,
		Filename:  id + ".png",
		Format:    "png",
		MIME:      "image/png",
		ExpiresAt: fixedNow.Add(time.Hour),
	}

	msg := buildOpenCodeGeneratedImageMessage(rec, openCodeImageRewriteOptions{BaseURL: "https://example.com/"})
	content := msg["content"].([]any)[0].(map[string]any)["text"].(string)
	downloadURL := "https://example.com" + "/sub2api/" + "generated-images/" + id + ".png"

	require.True(t, strings.Contains(content, "Temporary download URL: "+downloadURL), "message should label temporary download URL")
	require.NotContains(t, content, "Server download path")
	require.NotContains(t, content, "Do not treat")
	require.NotContains(t, content, "If no Download URL")
	require.NotContains(t, content, "Download URL:")
	require.NotContains(t, content, "Download: https://example.com")
}

func TestOpenCodeGeneratedImageMessageUsesSpecificMarker(t *testing.T) {
	id := "img_" + "abcdefghijklmnopqrstuvwxyzABCDEF"
	rec := OpenAIGeneratedImageRecord{ID: id, Filename: id + ".png"}

	msg := buildOpenCodeGeneratedImageMessage(rec, openCodeImageRewriteOptions{BaseURL: "https://example.com"})
	text := gjson.GetBytes(mustJSONBytes(t, msg), "content.0.text").String()
	specificMarker := "[[sub2api-generated-image:id=" + id + "]]"
	legacyMarker := "sub2api" + "-image://" + id
	downloadURL := "https://example.com" + "/sub2api/" + "generated-images/" + id + ".png"

	require.True(t, strings.Contains(text, specificMarker), "message should contain new image marker")
	require.False(t, strings.Contains(text, legacyMarker), "message should not contain legacy image marker")
	require.True(t, strings.Contains(text, downloadURL), "message should contain download URL")
}

func TestOpenCodeImageServerContinuationOutput_InstructsAgentToDownloadImmediately(t *testing.T) {
	messageText := "Generated image saved by sub2api.\nImage reference: " + openCodeSpecificImageMarkerForTest(testImageID) + "\nTemporary download URL: " + openCodeGeneratedImageDownloadURLForTest("https://example.com", testImageID)

	output := openCodeImageServerContinuationOutput(messageText)

	require.Contains(t, output, messageText)
	require.Contains(t, output, "Synthetic image generation continuation inserted by sub2api")
	require.Contains(t, output, "Your next assistant output item must be a function_call, not a message")
	require.Contains(t, output, "Emit exactly one available download-capable tool call now")
	require.Contains(t, output, "Do not answer in plain text before the tool call")
	require.Contains(t, output, "Choose an available tool capable of fetching or downloading the Temporary download URL to a local temporary file")
	require.Contains(t, output, "Immediately use the available shell, command-line, or network-access tool")
	require.Contains(t, output, "download the image from the URL above")
	require.Contains(t, output, "Do not stop after image generation")
	require.Contains(t, output, "The saved image URL expires quickly")
	require.Contains(t, output, "short-lived bearer URL")
	require.Contains(t, output, "may expire within about 1 hour")
	require.Contains(t, output, "download before any other action")
	require.Contains(t, output, "Do not delay for any reason")
	require.Contains(t, output, "Do not ask the user whether to download")
	require.Contains(t, output, "Do not call question, notify, skill, or directory-listing tools before attempting the download")
	require.Contains(t, output, "Do not use ls or rtk ls as a preflight")
	require.Contains(t, output, "Invoke-WebRequest")
	require.Contains(t, output, "Join-Path $env:TEMP")
	require.Contains(t, output, "curl.exe")
	require.Contains(t, output, "save it to a temporary directory")
	require.Contains(t, output, "Only if no tool can access the URL")
	require.Contains(t, output, "generated image reference marker")
	require.NotContains(t, output, "bash")
	require.NotContains(t, output, "sub2api-image marker")
}

func TestRewriteOpenCodeImageGenerationOutput_ImageCallWithoutResultBecomesText(t *testing.T) {
	store := newTestOpenAIGeneratedImageStore(t, fixedNow)
	body := []byte(`{"id":"resp_1","output":[{"id":"ig_1","type":"image_generation_call","status":"completed"}],"usage":{"input_tokens":1,"output_tokens":2}}`)

	patched, changed, err := rewriteOpenCodeImageGenerationOutput(context.Background(), body, store, openCodeImageRewriteOptions{BaseURL: "https://example.com"})

	require.NoError(t, err)
	require.True(t, changed)
	require.Equal(t, "message", gjson.GetBytes(patched, "output.0.type").String())
	require.Contains(t, gjson.GetBytes(patched, "output.0.content.0.text").String(), "no image result")
	require.NotContains(t, string(patched), "image_generation_call")
}

func TestHandleNonStreamingResponse_NonOpenCodePreservesImageGenerationJSON(t *testing.T) {
	gin.SetMode(gin.TestMode)
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", nil)
	c.Request.Header.Set("User-Agent", "curl/8.0")
	svc := &OpenAIGatewayService{cfg: &config.Config{}}
	resp := &http.Response{
		StatusCode: http.StatusOK,
		Header:     http.Header{"Content-Type": []string{"application/json"}},
		Body:       io.NopCloser(strings.NewReader(`{"id":"resp_1","output":[{"id":"ig_1","type":"image_generation_call","status":"completed","result":"` + pngB64 + `","output_format":"png"}],"usage":{"input_tokens":1,"output_tokens":2}}`)),
	}

	_, err := svc.handleNonStreamingResponse(context.Background(), resp, c, &Account{Platform: PlatformOpenAI, Type: AccountTypeOAuth}, "gpt-5.5", "gpt-5.5")

	require.NoError(t, err)
	require.Equal(t, "image_generation_call", gjson.Get(rec.Body.String(), "output.0.type").String())
	require.Equal(t, pngB64, gjson.Get(rec.Body.String(), "output.0.result").String())
}

func TestHandleSSEToJSON_OpenCodeRewritesImageFromDoneWhenCompletedOutputEmpty(t *testing.T) {
	gin.SetMode(gin.TestMode)
	store := newTestOpenAIGeneratedImageStore(t, fixedNow)
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", nil)
	c.Request.Header.Set("User-Agent", "opencode/1.0")
	cfg := &config.Config{}
	cfg.Server.FrontendURL = "https://example.com"
	svc := &OpenAIGatewayService{cfg: cfg, generatedImageStore: store}
	resp := &http.Response{
		StatusCode: http.StatusOK,
		Header:     http.Header{"Content-Type": []string{"text/event-stream"}},
		Body: io.NopCloser(strings.NewReader(strings.Join([]string{
			"event: response.output_item.done",
			`data: {"type":"response.output_item.done","output_index":0,"item":{"id":"ig_1","type":"image_generation_call","status":"completed","result":"` + pngB64 + `","output_format":"png"}}`,
			"",
			"event: response.completed",
			`data: {"type":"response.completed","response":{"id":"resp_1","model":"gpt-5.5","output":[],"usage":{"input_tokens":1,"output_tokens":2}}}`,
			"",
		}, "\n"))),
	}

	_, err := svc.handleNonStreamingResponse(context.Background(), resp, c, &Account{Platform: PlatformOpenAI, Type: AccountTypeOAuth}, "gpt-5.5", "gpt-5.5")

	require.NoError(t, err)
	require.Equal(t, "message", gjson.Get(rec.Body.String(), "output.0.type").String())
	requireContainsOpenCodeGeneratedImageSpecificMarker(t, gjson.Get(rec.Body.String(), "output.0.content.0.text").String())
	require.Contains(t, rec.Body.String(), "https://example.com/sub2api/generated-images/")
	require.NotContains(t, rec.Body.String(), "image_generation_call")
	require.NotContains(t, rec.Body.String(), pngB64)
}

func TestHandleSSEToJSON_OpenCodeRewritesEventTypedImageDoneWithoutDataType(t *testing.T) {
	gin.SetMode(gin.TestMode)
	store := newTestOpenAIGeneratedImageStore(t, fixedNow)
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", nil)
	c.Request.Header.Set("User-Agent", "opencode/1.0")
	svc := &OpenAIGatewayService{cfg: &config.Config{}, generatedImageStore: store}
	resp := &http.Response{
		StatusCode: http.StatusOK,
		Header:     http.Header{"Content-Type": []string{"text/event-stream"}},
		Body: io.NopCloser(strings.NewReader(strings.Join([]string{
			"event: response.output_item.done",
			`data: {"output_index":0,"item":{"id":"ig_1","type":"image_generation_call","status":"completed","result":"` + pngB64 + `","output_format":"png"}}`,
			"",
			"event: response.completed",
			`data: {"type":"response.completed","response":{"id":"resp_1","model":"gpt-5.5","output":[],"usage":{"input_tokens":1,"output_tokens":2}}}`,
			"",
		}, "\n"))),
	}

	_, err := svc.handleNonStreamingResponse(context.Background(), resp, c, &Account{Platform: PlatformOpenAI, Type: AccountTypeOAuth}, "gpt-5.5", "gpt-5.5")

	require.NoError(t, err)
	require.Equal(t, "message", gjson.Get(rec.Body.String(), "output.0.type").String())
	requireContainsOpenCodeGeneratedImageSpecificMarker(t, gjson.Get(rec.Body.String(), "output.0.content.0.text").String())
	require.NotContains(t, rec.Body.String(), "image_generation_call")
	require.NotContains(t, rec.Body.String(), pngB64)
}

func TestHandleSSEToJSON_OpenCodeRewritesDataOnlyImageMarkerWhenCompletedOutputEmpty(t *testing.T) {
	for _, tc := range []struct {
		name    string
		account *Account
	}{
		{
			name:    "oauth canonical merge",
			account: &Account{Platform: PlatformOpenAI, Type: AccountTypeOAuth},
		},
		{
			name:    "api key reconstruction",
			account: &Account{Platform: PlatformOpenAI, Type: AccountTypeAPIKey},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			gin.SetMode(gin.TestMode)
			store := newTestOpenAIGeneratedImageStore(t, fixedNow)
			rec := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(rec)
			c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", nil)
			c.Request.Header.Set("User-Agent", "opencode/1.0")
			svc := &OpenAIGatewayService{cfg: &config.Config{}, generatedImageStore: store}
			resp := &http.Response{StatusCode: http.StatusOK, Header: http.Header{"Content-Type": []string{"text/event-stream"}}}
			body := []byte(strings.Join([]string{
				`data: {"output_index":0,"item":{"id":"ig_1","type":"image_generation_call","status":"completed","result":"` + pngB64 + `","output_format":"png"}}`,
				``,
				`event: response.completed`,
				`data: {"type":"response.completed","response":{"id":"resp_1","model":"gpt-5.5","output":[],"usage":{"input_tokens":1,"output_tokens":2}}}`,
				``,
			}, "\n"))

			_, err := svc.handleSSEToJSONForAccount(resp, c, body, tc.account, "gpt-5.5", "gpt-5.5")

			require.NoError(t, err)
			require.Equal(t, "message", gjson.Get(rec.Body.String(), "output.0.type").String())
			requireContainsOpenCodeGeneratedImageSpecificMarker(t, gjson.Get(rec.Body.String(), "output.0.content.0.text").String())
			require.NotContains(t, rec.Body.String(), "image_generation_call")
			require.NotContains(t, rec.Body.String(), pngB64)
		})
	}
}

func TestHandleSSEToJSON_OpenCodeResponseFailedUsesEventTypeWhenDataTypeMissing(t *testing.T) {
	gin.SetMode(gin.TestMode)
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", nil)
	c.Request.Header.Set("User-Agent", "opencode/1.0")
	svc := &OpenAIGatewayService{cfg: &config.Config{}}
	resp := &http.Response{StatusCode: http.StatusOK, Header: http.Header{"Content-Type": []string{"text/event-stream"}}}
	body := []byte(strings.Join([]string{
		`event: response.failed`,
		`data: {"response":{"error":{"message":"upstream rejected request"}}}`,
		``,
	}, "\n"))

	usage, err := svc.handleSSEToJSONForAccount(resp, c, body, &Account{Platform: PlatformOpenAI, Type: AccountTypeOAuth}, "gpt-5.5", "gpt-5.5")

	require.Nil(t, usage)
	require.Error(t, err)
	require.Equal(t, http.StatusBadGateway, rec.Code)
	require.Contains(t, rec.Body.String(), "upstream rejected request")
	require.NotContains(t, rec.Body.String(), "event: response.failed")
	require.Contains(t, rec.Header().Get("Content-Type"), "application/json")
}

func TestHandleSSEToJSON_OpenCodeRewritesOtherTerminalOutputImage(t *testing.T) {
	for _, eventType := range []string{"response.incomplete", "response.cancelled", "response.canceled"} {
		t.Run(eventType, func(t *testing.T) {
			gin.SetMode(gin.TestMode)
			store := newTestOpenAIGeneratedImageStore(t, fixedNow)
			rec := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(rec)
			c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", nil)
			c.Request.Header.Set("User-Agent", "opencode/1.0")
			svc := &OpenAIGatewayService{cfg: &config.Config{}, generatedImageStore: store}
			resp := &http.Response{
				StatusCode: http.StatusOK,
				Header:     http.Header{"Content-Type": []string{"text/event-stream"}},
				Body: io.NopCloser(strings.NewReader(strings.Join([]string{
					"event: " + eventType,
					`data: {"type":"` + eventType + `","response":{"id":"resp_1","model":"gpt-5.5","output":[{"id":"ig_1","type":"image_generation_call","status":"completed","result":"` + pngB64 + `","output_format":"png"}],"usage":{"input_tokens":1,"output_tokens":2}}}`,
					"",
				}, "\n"))),
			}

			_, err := svc.handleNonStreamingResponse(context.Background(), resp, c, &Account{Platform: PlatformOpenAI, Type: AccountTypeOAuth}, "gpt-5.5", "gpt-5.5")

			require.NoError(t, err)
			require.Equal(t, "message", gjson.Get(rec.Body.String(), "output.0.type").String())
			requireContainsOpenCodeGeneratedImageSpecificMarker(t, gjson.Get(rec.Body.String(), "output.0.content.0.text").String())
			require.NotContains(t, rec.Body.String(), "image_generation_call")
			require.NotContains(t, rec.Body.String(), pngB64)
		})
	}
}

func TestHandleSSEToJSON_OpenCodeRewritesImageWithoutResultFromDoneWhenCompletedOutputEmpty(t *testing.T) {
	gin.SetMode(gin.TestMode)
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", nil)
	c.Request.Header.Set("User-Agent", "opencode/1.0")
	svc := &OpenAIGatewayService{cfg: &config.Config{}}
	resp := &http.Response{
		StatusCode: http.StatusOK,
		Header:     http.Header{"Content-Type": []string{"text/event-stream"}},
		Body: io.NopCloser(strings.NewReader(strings.Join([]string{
			"event: response.output_item.done",
			`data: {"type":"response.output_item.done","output_index":0,"item":{"id":"ig_1","type":"image_generation_call","status":"completed"}}`,
			"",
			"event: response.completed",
			`data: {"type":"response.completed","response":{"id":"resp_1","model":"gpt-5.5","output":[],"usage":{"input_tokens":1,"output_tokens":2}}}`,
			"",
		}, "\n"))),
	}

	_, err := svc.handleNonStreamingResponse(context.Background(), resp, c, &Account{Platform: PlatformOpenAI, Type: AccountTypeAPIKey}, "gpt-5.5", "gpt-5.5")

	require.NoError(t, err)
	require.Equal(t, "message", gjson.Get(rec.Body.String(), "output.0.type").String())
	require.Contains(t, gjson.Get(rec.Body.String(), "output.0.content.0.text").String(), "no image result")
	require.NotContains(t, rec.Body.String(), "image_generation_call")
}

func TestHandleSSEToJSON_OpenCodePreservesOutputIndexOrderWhenReconstructingImageAndMessage(t *testing.T) {
	gin.SetMode(gin.TestMode)
	store := newTestOpenAIGeneratedImageStore(t, fixedNow)
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", nil)
	c.Request.Header.Set("User-Agent", "opencode/1.0")
	svc := &OpenAIGatewayService{cfg: &config.Config{}, generatedImageStore: store}
	resp := &http.Response{
		StatusCode: http.StatusOK,
		Header:     http.Header{"Content-Type": []string{"text/event-stream"}},
		Body: io.NopCloser(strings.NewReader(strings.Join([]string{
			"event: response.output_item.done",
			`data: {"type":"response.output_item.done","output_index":0,"item":{"id":"ig_1","type":"image_generation_call","status":"completed","result":"` + pngB64 + `","output_format":"png"}}`,
			"",
			"event: response.output_text.delta",
			`data: {"type":"response.output_text.delta","output_index":1,"content_index":0,"delta":"ordinary text"}`,
			"",
			"event: response.completed",
			`data: {"type":"response.completed","response":{"id":"resp_1","model":"gpt-5.5","output":[],"usage":{"input_tokens":1,"output_tokens":2}}}`,
			"",
		}, "\n"))),
	}

	_, err := svc.handleNonStreamingResponse(context.Background(), resp, c, &Account{Platform: PlatformOpenAI, Type: AccountTypeAPIKey}, "gpt-5.5", "gpt-5.5")

	require.NoError(t, err)
	require.Equal(t, "message", gjson.Get(rec.Body.String(), "output.0.type").String())
	requireContainsOpenCodeGeneratedImageSpecificMarker(t, gjson.Get(rec.Body.String(), "output.0.content.0.text").String())
	require.Equal(t, "message", gjson.Get(rec.Body.String(), "output.1.type").String())
	require.Equal(t, "ordinary text", gjson.Get(rec.Body.String(), "output.1.content.0.text").String())
	require.NotContains(t, rec.Body.String(), `"name":"bash"`)
	require.NotContains(t, rec.Body.String(), pngB64)
}

func TestHandleSSEToJSON_OpenCodePreservesDoneOnlyNonImageOutputWhenReconstructingImage(t *testing.T) {
	gin.SetMode(gin.TestMode)
	store := newTestOpenAIGeneratedImageStore(t, fixedNow)
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", nil)
	c.Request.Header.Set("User-Agent", "opencode/1.0")
	svc := &OpenAIGatewayService{cfg: &config.Config{}, generatedImageStore: store}
	resp := &http.Response{
		StatusCode: http.StatusOK,
		Header:     http.Header{"Content-Type": []string{"text/event-stream"}},
		Body: io.NopCloser(strings.NewReader(strings.Join([]string{
			"event: response.output_item.done",
			`data: {"type":"response.output_item.done","output_index":0,"item":{"id":"fc_1","type":"function_call","call_id":"call_1","name":"do_work","arguments":"{}","status":"completed"}}`,
			"",
			"event: response.output_item.done",
			`data: {"type":"response.output_item.done","output_index":1,"item":{"id":"ig_1","type":"image_generation_call","status":"completed","result":"` + pngB64 + `","output_format":"png"}}`,
			"",
			"event: response.completed",
			`data: {"type":"response.completed","response":{"id":"resp_1","model":"gpt-5.5","output":[],"usage":{"input_tokens":1,"output_tokens":2}}}`,
			"",
		}, "\n"))),
	}

	_, err := svc.handleNonStreamingResponse(context.Background(), resp, c, &Account{Platform: PlatformOpenAI, Type: AccountTypeAPIKey}, "gpt-5.5", "gpt-5.5")

	require.NoError(t, err)
	require.Equal(t, "function_call", gjson.Get(rec.Body.String(), "output.0.type").String())
	require.Equal(t, "call_1", gjson.Get(rec.Body.String(), "output.0.call_id").String())
	require.Equal(t, "message", gjson.Get(rec.Body.String(), "output.1.type").String())
	requireContainsOpenCodeGeneratedImageSpecificMarker(t, gjson.Get(rec.Body.String(), "output.1.content.0.text").String())
	require.False(t, gjson.Get(rec.Body.String(), "output.2").Exists())
	require.NotContains(t, rec.Body.String(), `"name":"bash"`)
	require.NotContains(t, rec.Body.String(), pngB64)
}

func TestHandleSSEToJSON_OpenCodeRejectsImageSSEWithoutTerminalResponse(t *testing.T) {
	gin.SetMode(gin.TestMode)
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", nil)
	c.Request.Header.Set("User-Agent", "opencode/1.0")
	svc := &OpenAIGatewayService{cfg: &config.Config{}}
	resp := &http.Response{
		StatusCode: http.StatusOK,
		Header:     http.Header{"Content-Type": []string{"text/event-stream"}},
		Body: io.NopCloser(strings.NewReader(strings.Join([]string{
			"event: response.image_generation_call.partial_image",
			`data: {"type":"response.image_generation_call.partial_image","output_index":0,"partial_image_index":0,"partial_image_b64":"` + pngB64 + `"}`,
			"",
			"event: response.output_item.done",
			`data: {"type":"response.output_item.done","output_index":0,"item":{"id":"ig_1","type":"image_generation_call","status":"completed","result":"` + pngB64 + `","output_format":"png"}}`,
			"",
		}, "\n"))),
	}

	usage, err := svc.handleNonStreamingResponse(context.Background(), resp, c, &Account{Platform: PlatformOpenAI, Type: AccountTypeAPIKey}, "gpt-5.5", "gpt-5.5")

	require.Nil(t, usage)
	require.Error(t, err)
	require.Equal(t, http.StatusBadGateway, rec.Code)
	require.NotContains(t, rec.Body.String(), "image_generation_call")
	require.NotContains(t, rec.Body.String(), pngB64)
	require.Contains(t, rec.Header().Get("Content-Type"), "application/json")
}
