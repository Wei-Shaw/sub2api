package basispoints

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func toolImageSource(kind string, output any) object {
	source := testSource()
	source["tools"] = []any{object{"type": kind, "name": "view_image"}}
	historyKind := kind
	if kind == "custom" {
		historyKind = "custom_tool"
	}
	call := object{"type": historyKind + "_call", "call_id": "call_image", "name": "view_image"}
	if kind == "function" {
		call["arguments"] = "{}"
	} else {
		call["input"] = "image.png"
	}
	source["input"] = []any{call, object{"type": historyKind + "_call_output", "id": "ctco_image", "call_id": "call_image", "output": output}}
	return source
}

func TestToolImagesPreserveContentOrderAndAssociation(t *testing.T) {
	for _, kind := range []string{"function", "custom"} {
		for _, reference := range []string{"file_id", "image_url"} {
			for _, detail := range []string{"", "auto", "low", "high", "original"} {
				for _, mixed := range []bool{false, true} {
					t.Run(fmt.Sprintf("%s/%s/%s/mixed=%v", kind, reference, detail, mixed), func(t *testing.T) {
						first := object{"type": "input_image", reference: "file-first"}
						second := object{"type": "input_image", reference: "file-second"}
						if reference == "image_url" {
							first[reference] = "https://example.com/a.png?signature=a%2Fb"
							second[reference] = "https://example.com/b.png?signature=c%2Fd"
						}
						if detail != "" {
							first["detail"], second["detail"] = detail, detail
						}
						parts := []any{first, second}
						if mixed {
							parts = []any{object{"type": "input_text", "text": "before"}, first, object{"type": "input_text", "text": "between"}, second, object{"type": "input_text", "text": "after"}}
						}
						source := toolImageSource(kind, parts)
						cache := new(ReplayCache)
						body, _ := mustPrepare(t, source, "scope", cache)
						items := mustTestValue[[]any](t, body["input"])
						call := mustTestValue[object](t, items[len(items)-3])
						result := mustTestValue[object](t, items[len(items)-2])
						msg := mustTestValue[object](t, items[len(items)-1])
						require.Equal(t, "function_call", call["type"])
						require.Equal(t, "function_call_output", result["type"])
						require.Equal(t, call["call_id"], result["call_id"])
						require.True(t, strings.HasPrefix(mustTestValue[string](t, result["id"]), "fc_"))
						require.Equal(t, "user", msg["role"])
						content := mustTestValue[[]any](t, msg["content"])
						require.Len(t, content, 5)
						require.Contains(t, mustTestValue[object](t, content[0])["text"], "tool output")
						if reference == "file_id" {
							require.Equal(t, object{"type": "input_image", "file_id": "file-first"}, content[2])
							require.Equal(t, object{"type": "input_image", "file_id": "file-second"}, content[4])
						} else {
							require.Equal(t, first, content[2])
							require.Equal(t, second, content[4])
						}
						output := mustTestValue[[]any](t, result["output"])
						require.Len(t, output, len(parts))
						imageIndex := 0
						for i, part := range parts {
							if mustTestValue[object](t, part)["type"] != "input_image" {
								require.Equal(t, part, output[i])
								continue
							}
							label := mustTestValue[string](t, mustTestValue[object](t, content[1+imageIndex*2])["text"])
							require.Contains(t, label, fmt.Sprintf("image %d", imageIndex+1))
							require.Contains(t, label, "call_image")
							require.Equal(t, "input_text", mustTestValue[object](t, output[i])["type"])
							require.Contains(t, mustTestValue[object](t, output[i])["text"], label)
							imageIndex++
						}
						require.Nil(t, separateToolImages(result), "normalization must be idempotent")
						retry, _ := mustPrepare(t, source, "scope", cache)
						require.Equal(t, body, retry, "retries must not accumulate messages")
						source["input"] = mustTestValue[[]any](t, source["input"])[1:]
						replayed, _ := mustPrepare(t, source, "scope", cache)
						require.Equal(t, body["input"], replayed["input"], "cached native call must remain unchanged")
					})
				}
			}
		}
	}
}

func TestToolImagesMultipleResultsStayAdjacent(t *testing.T) {
	source := toolImageSource("function", nil)
	source["input"] = []any{
		object{"type": "function_call", "call_id": "call_a", "name": "view_image", "arguments": "{}"},
		object{"type": "function_call", "call_id": "call_b", "name": "view_image", "arguments": "{}"},
		object{"type": "function_call_output", "call_id": "call_b", "output": []any{object{"type": "input_image", "file_id": "file-b"}}},
		object{"type": "function_call_output", "call_id": "call_a", "output": []any{object{"type": "input_image", "file_id": "file-a"}}},
		message("user", "Continue after both results."),
	}
	body, _ := mustPrepare(t, source, "", nil)
	items := mustTestValue[[]any](t, body["input"])
	tail := items[len(items)-7:]
	for i, id := range []string{"b", "a"} {
		result := mustTestValue[object](t, tail[2+i*2])
		msg := mustTestValue[object](t, tail[3+i*2])
		require.Equal(t, "call_"+id, result["call_id"])
		parts := mustTestValue[[]any](t, msg["content"])
		require.Contains(t, mustTestValue[object](t, parts[1])["text"], "call_"+id)
		require.Equal(t, "file-"+id, mustTestValue[object](t, parts[2])["file_id"])
	}
	require.Equal(t, message("user", "Continue after both results."), tail[6])
}

func TestToolImagesLeaveTextOnlyOutputUnchanged(t *testing.T) {
	for _, output := range []any{"plain text", []any{}, []any{object{"type": "input_text", "text": "literal input_image text"}}} {
		body, _ := mustPrepare(t, toolImageSource("function", output), "", nil)
		items := mustTestValue[[]any](t, body["input"])
		result := mustTestValue[object](t, items[len(items)-1])
		require.Equal(t, "function_call_output", result["type"])
		require.Equal(t, output, result["output"])
	}
}

func TestToolImagesValidateBeforeMoving(t *testing.T) {
	for _, image := range []object{
		{"type": "input_image", "file_id": "PRIVATE_INVALID"},
		{"type": "input_image", "file_id": "file-valid", "detail": "PRIVATE_INVALID"},
		{"type": "input_image", "file_id": "file-valid", "image_url": "https://example.com/image"},
		{"type": "input_image", "image_url": "http://example.com/PRIVATE_INVALID"},
	} {
		raw, err := json.Marshal(toolImageSource("function", []any{image}))
		require.NoError(t, err)
		_, _, err = Prepare(raw, "", nil)
		require.ErrorContains(t, err, "path=input[1].output[0]")
		require.NotContains(t, err.Error(), "PRIVATE_INVALID")
	}
}

func TestToolImagesPreserveValidatedInlineScreenshots(t *testing.T) {
	url, _ := nativeTestURL(t)
	for _, kind := range []string{"function", "custom"} {
		for _, mixed := range []bool{false, true} {
			t.Run(fmt.Sprintf("%s/mixed=%v", kind, mixed), func(t *testing.T) {
				inline := object{"type": "input_image", "image_url": url, "detail": "original"}
				parts := []any{inline}
				if mixed {
					parts = []any{object{"type": "input_image", "file_id": "file-existing"}, inline, object{"type": "input_image", "image_url": "https://example.com/screenshot.png"}, object{"type": "input_text", "text": "after"}}
				}
				raw, err := json.Marshal(toolImageSource(kind, parts))
				require.NoError(t, err)
				plan, err := PrepareNativeImages(raw)
				require.NoError(t, err)
				require.False(t, plan.HasImages(), "inline tool screenshots must not be uploaded")
				wire, bridge, err := plan.PrepareWithCatalog("scope", nil, new(CatalogCache))
				require.NoError(t, err)
				var body object
				require.NoError(t, decode(wire, &body))
				items := mustTestValue[[]any](t, body["input"])
				resultIndex := len(items) - 1
				if mixed {
					resultIndex--
				}
				result := mustTestValue[object](t, items[resultIndex])
				require.Equal(t, "function_call_output", result["type"])
				output := mustTestValue[[]any](t, result["output"])
				if mixed {
					require.Len(t, output, 4)
					require.Equal(t, inline, output[1])
					require.Equal(t, parts[3], output[3])
					msg := mustTestValue[object](t, items[resultIndex+1])
					content := mustTestValue[[]any](t, msg["content"])
					require.Len(t, content, 5)
					require.Equal(t, parts[0], content[2])
					require.Equal(t, parts[2], content[4])
				} else {
					require.Equal(t, parts, output)
				}
				again, _, err := bridge.Reprepare(raw)
				require.NoError(t, err)
				require.JSONEq(t, string(wire), string(again))
			})
		}
	}
}

func TestToolImagesNativeUploadReprepare(t *testing.T) {
	url, expectedData := nativeTestURL(t)
	for _, kind := range []string{"function", "custom"} {
		source := toolImageSource(kind, []any{object{"type": "input_text", "text": "screenshot"}, object{"type": "input_image", "file_id": "file-existing", "detail": "original"}})
		source["input"] = append([]any{object{"role": "user", "content": []any{object{"type": "input_image", "image_url": url}}}}, mustTestValue[[]any](t, source["input"])...)
		raw, err := json.Marshal(source)
		require.NoError(t, err)
		plan, err := PrepareNativeImages(raw)
		require.NoError(t, err)
		_, bridge, err := plan.PrepareWithCatalog("scope", new(ReplayCache), new(CatalogCache))
		require.NoError(t, err)
		calls := 0
		uploaded, err := plan.Upload(context.Background(), new(AttachmentCache), "scope", func(_ context.Context, image InlineAttachment) (string, error) {
			calls++
			require.Equal(t, int64(len(expectedData)), image.Size)
			return "file-uploaded", nil
		})
		require.NoError(t, err)
		require.Equal(t, 1, calls)
		body, _, err := bridge.Reprepare(uploaded)
		require.NoError(t, err)
		require.NotContains(t, string(body), "file-preflight")
		require.NotContains(t, string(body), "data:image")
		var wire object
		require.NoError(t, decode(body, &wire))
		items := mustTestValue[[]any](t, wire["input"])
		result := mustTestValue[object](t, items[len(items)-2])
		require.NotContains(t, fmt.Sprint(result["output"]), "file-existing")
		require.Contains(t, string(body), "file-uploaded", "the user image must still be uploaded")
		content := mustTestValue[[]any](t, mustTestValue[object](t, items[len(items)-1])["content"])
		require.Equal(t, object{"type": "input_image", "file_id": "file-existing"}, content[2])
		again, _, err := bridge.Reprepare(uploaded)
		require.NoError(t, err)
		require.JSONEq(t, string(body), string(again))
	}
}
