package basispoints

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"hash/crc32"
	"image"
	"image/png"
	"io"
	"mime"
	"mime/multipart"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func nativeTestURL(t *testing.T) (string, []byte) {
	t.Helper()
	var data bytes.Buffer
	require.NoError(t, png.Encode(&data, image.NewRGBA(image.Rect(0, 0, 2, 3))))
	return "data:image/png;base64," + base64.StdEncoding.EncodeToString(data.Bytes()), data.Bytes()
}
func nativeTestRequest(t *testing.T, urls ...string) []byte {
	t.Helper()
	var parts []any
	for _, url := range urls {
		parts = append(parts, object{"type": "input_image", "image_url": url, "detail": "original"})
	}
	raw, err := json.Marshal(object{"model": "gpt-6-astra", "input": []any{object{"role": "user", "content": parts}}, "metadata": object{"large": json.Number("9007199254740993")}})
	require.NoError(t, err)
	return raw
}

func TestNativeImagesMultipartAndScopedReuse(t *testing.T) {
	url, data := nativeTestURL(t)
	raw := nativeTestRequest(t, url, url)
	cache := &AttachmentCache{}
	calls := 0
	upload := func(_ context.Context, img InlineAttachment) (string, error) {
		calls++
		reader, contentType, size, err := img.Multipart()
		require.NoError(t, err)
		body, err := io.ReadAll(reader)
		require.NoError(t, err)
		require.Equal(t, int64(len(body)), size)
		_, params, err := mime.ParseMediaType(contentType)
		require.NoError(t, err)
		mr := multipart.NewReader(bytes.NewReader(body), params["boundary"])
		part, err := mr.NextPart()
		require.NoError(t, err)
		require.Equal(t, "file", part.FormName())
		require.Equal(t, "image.png", part.FileName())
		require.Equal(t, "image/png", part.Header.Get("Content-Type"))
		decoded, err := io.ReadAll(part)
		require.NoError(t, err)
		require.Equal(t, data, decoded)
		_, err = mr.NextPart()
		require.ErrorIs(t, err, io.EOF)
		return fmt.Sprintf("file-upload%d", calls), nil
	}
	for _, scope := range []string{"account1/key1/thread1/token1", "account1/key1/thread1/token1", "account2/key1/thread1/token1", "account1/key2/thread1/token1", "account1/key1/thread2/token1", "account1/key1/thread1/token2", "", ""} {
		plan, err := PrepareNativeImages(raw)
		require.NoError(t, err)
		preflight, err := plan.Body()
		require.NoError(t, err)
		_, _, err = Prepare(preflight, scope, nil)
		require.NoError(t, err)
		out, err := plan.Upload(context.Background(), cache, scope, upload)
		require.NoError(t, err)
		require.NotContains(t, string(out), "data:image")
		require.Contains(t, string(out), "9007199254740993")
		require.Contains(t, string(out), "original")
		require.NotContains(t, string(out), "file-preflight")
		_, _, err = Prepare(out, scope, nil)
		require.NoError(t, err)
	}
	require.Equal(t, 7, calls, "same request duplicates upload once; only identical trusted scope reuses across requests")
}

func TestNativeImagesValidateAllBeforeUpload(t *testing.T) {
	url, _ := nativeTestURL(t)
	for name, urls := range map[string][]string{
		"bad second image": {url, "data:image/png;base64,PRIVATE_INVALID"},
		"mime mismatch":    {strings.Replace(url, "image/png", "image/jpeg", 1)},
		"bad image bytes":  {"data:image/png;base64," + base64.StdEncoding.EncodeToString([]byte("not an image"))},
	} {
		t.Run(name, func(t *testing.T) {
			_, err := PrepareNativeImages(nativeTestRequest(t, urls...))
			require.Error(t, err)
			require.NotContains(t, err.Error(), "PRIVATE_INVALID")
		})
	}
	// 20 occurrences remain legal; invalid detail and mixed references fail locally.
	_, err := PrepareNativeImages(nativeTestRequest(t, strings.Fields(strings.Repeat(url+" ", 20))...))
	require.NoError(t, err)
	_, err = PrepareNativeImages(nativeTestRequest(t, strings.Fields(strings.Repeat(url+" ", 21))...))
	require.EqualError(t, err, "local gateway is configured for at most 20 inline images per request")
	_, err = PrepareNativeImagesWithLimit(nativeTestRequest(t, url, url), 1)
	require.EqualError(t, err, "local gateway is configured for at most 1 inline images per request")
	raw := nativeTestRequest(t, url)
	_, err = PrepareNativeImages(bytes.ReplaceAll(raw, []byte("original"), []byte("invalid")))
	require.Error(t, err)
	_, err = PrepareNativeImages(append(raw, []byte(" {}")...))
	require.Error(t, err)
}

func TestNativeImagesOnlyTraverseTypedContent(t *testing.T) {
	url, _ := nativeTestURL(t)
	source := object{"model": "gpt-6-astra", "input": []any{
		object{"type": "function_call", "arguments": object{"content": []any{object{"type": "input_image", "image_url": url}}}, "content": []any{object{"type": "input_image", "image_url": url}}},
		object{"type": "function_call_output", "output": []any{object{"type": "input_image", "image_url": url}}},
		object{"type": "custom_tool_call_output", "output": []any{object{"type": "input_image", "image_url": url}}},
	}}
	raw, err := json.Marshal(source)
	require.NoError(t, err)
	plan, err := PrepareNativeImages(raw)
	require.NoError(t, err)
	require.Empty(t, plan.parts)
	out, err := plan.Upload(context.Background(), &AttachmentCache{}, "", func(context.Context, InlineAttachment) (string, error) {
		t.Fatal("tool screenshots must not upload")
		return "", nil
	})
	require.NoError(t, err)
	var decoded object
	require.NoError(t, json.Unmarshal(out, &decoded))
	items := mustTestValue[[]any](t, decoded["input"])
	require.Equal(t, mustTestValue[[]any](t, source["input"])[0], items[0])
}

func TestAttachmentCacheExpiryCapacityAndFailure(t *testing.T) {
	now := time.Now()
	cache := &AttachmentCache{now: func() time.Time { return now }}
	upload := func(context.Context, InlineAttachment) (string, error) { return "file-cached", nil }
	for i := 0; i < 513; i++ {
		_, err := cache.get(context.Background(), fmt.Sprint(i), InlineAttachment{}, upload)
		require.NoError(t, err)
		now = now.Add(time.Millisecond)
	}
	require.Len(t, cache.entries, 512)
	calls := 0
	count := func(ctx context.Context, img InlineAttachment) (string, error) { calls++; return upload(ctx, img) }
	_, err := cache.get(context.Background(), "0", InlineAttachment{}, count)
	require.NoError(t, err)
	require.Equal(t, 1, calls, "oldest metadata evicted")
	now = now.Add(29 * time.Minute)
	_, err = cache.get(context.Background(), "0", InlineAttachment{}, count)
	require.NoError(t, err)
	require.Equal(t, 1, calls)
	now = now.Add(time.Minute)
	_, err = cache.get(context.Background(), "0", InlineAttachment{}, count)
	require.NoError(t, err)
	require.Equal(t, 2, calls, "reads must not renew attachment lifetime")
	for i := 0; i < 2; i++ {
		_, err = cache.get(context.Background(), "failure", InlineAttachment{}, func(context.Context, InlineAttachment) (string, error) { return "", errors.New("failed") })
		require.Error(t, err)
	}
	require.Len(t, cache.flights, 0)
	require.Zero(t, cache.active)
	_, err = cache.get(context.Background(), "invalid ID", InlineAttachment{}, func(context.Context, InlineAttachment) (string, error) { return "https://invalid", nil })
	require.Error(t, err)
}

func TestAttachmentCacheCoalescingCancellationAndAdmission(t *testing.T) {
	cache := &AttachmentCache{}
	var calls atomic.Int32
	started := make(chan struct{})
	release := make(chan struct{})
	upload := func(context.Context, InlineAttachment) (string, error) {
		if calls.Add(1) == 1 {
			close(started)
		}
		<-release
		return "file-shared", nil
	}
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		id, err := cache.get(context.Background(), "same", InlineAttachment{}, upload)
		require.NoError(t, err)
		require.Equal(t, "file-shared", id)
	}()
	<-started
	ctx, cancel := context.WithCancel(context.Background())
	waiter := make(chan error, 1)
	go func() { _, err := cache.get(ctx, "same", InlineAttachment{}, upload); waiter <- err }()
	cancel()
	require.ErrorIs(t, <-waiter, context.Canceled)
	for i := 0; i < 16; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			id, err := cache.get(context.Background(), "same", InlineAttachment{}, upload)
			require.NoError(t, err)
			require.Equal(t, "file-shared", id)
		}()
	}
	close(release)
	wg.Wait()
	require.Equal(t, int32(1), calls.Load())
	cache.mu.Lock()
	cache.active = 32
	cache.mu.Unlock()
	_, err := cache.get(context.Background(), "new", InlineAttachment{}, upload)
	require.ErrorIs(t, err, ErrAttachmentBusy)
	// Cached requests do not consume an upload slot.
	_, err = cache.get(context.Background(), "same", InlineAttachment{}, upload)
	require.NoError(t, err)
}

func TestNativeImagesByteAndPixelLimits(t *testing.T) {
	_, data := nativeTestURL(t)
	// A valid PNG header with excessive dimensions must fail without allocating a bitmap.
	large := append([]byte(nil), data...)
	binary.BigEndian.PutUint32(large[16:20], 1<<20)
	binary.BigEndian.PutUint32(large[20:24], 128)
	binary.BigEndian.PutUint32(large[29:33], crc32.ChecksumIEEE(large[12:29]))
	_, err := PrepareNativeImages(nativeTestRequest(t, "data:image/png;base64,"+base64.StdEncoding.EncodeToString(large)))
	require.ErrorContains(t, err, "64 megapixel")
	t.Run("single image", func(t *testing.T) {
		url := "data:image/png;base64," + strings.Repeat("A", base64.StdEncoding.EncodedLen(imageRelayMaxImageBytes)+4)
		_, err := PrepareNativeImages(nativeTestRequest(t, url))
		require.ErrorContains(t, err, "20 MiB")
	})
	t.Run("duplicate occurrences count toward request bytes", func(t *testing.T) {
		padded := make([]byte, 17<<20)
		copy(padded, data)
		url := "data:image/png;base64," + base64.StdEncoding.EncodeToString(padded)
		_, err := PrepareNativeImages(nativeTestRequest(t, url, url))
		require.ErrorContains(t, err, "32 MiB")
	})
}

func TestAttachmentCacheBoundsActualUploads(t *testing.T) {
	cache := &AttachmentCache{}
	started := make(chan struct{}, 32)
	release := make(chan struct{})
	upload := func(context.Context, InlineAttachment) (string, error) {
		started <- struct{}{}
		<-release
		return "file-bounded", nil
	}
	var wg sync.WaitGroup
	for i := 0; i < 32; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, err := cache.get(context.Background(), "", InlineAttachment{}, upload)
			require.NoError(t, err)
		}()
	}
	for i := 0; i < 32; i++ {
		<-started
	}
	_, err := cache.get(context.Background(), "new scope", InlineAttachment{}, upload)
	require.ErrorIs(t, err, ErrAttachmentBusy)
	close(release)
	wg.Wait()
	require.Zero(t, cache.active)
	require.Empty(t, cache.entries)
	require.Empty(t, cache.flights)
}

func TestNativeToolImageValidationAndScope(t *testing.T) {
	url, _ := nativeTestURL(t)
	source := object{"model": "gpt-6-astra", "input": []any{
		object{"type": "function_call", "name": "view_image", "call_id": "call_image", "arguments": "{}"},
		object{"type": "function_call_output", "call_id": "call_image", "output": []any{object{"type": "input_image", "image_url": url}}},
	}}
	raw, err := json.Marshal(source)
	require.NoError(t, err)
	plan, err := PrepareNativeImages(raw)
	require.NoError(t, err)
	require.False(t, plan.HasImages())
	wire, bridge, err := plan.PrepareWithCatalog("scope", nil, new(CatalogCache))
	require.NoError(t, err)
	require.Contains(t, string(wire), url)
	require.Contains(t, string(wire), `"detail":"auto"`)
	_, _, err = Prepare(raw, "scope", nil)
	require.ErrorContains(t, err, "image support is disabled")
	for _, change := range []func(object){
		func(p object) { p["image_url"] = "data:image/png;base64,PRIVATE_INVALID" },
		func(p object) { p["detail"] = "invalid" },
		func(p object) { p["file_id"] = "file-mixed" },
	} {
		var modified object
		require.NoError(t, json.Unmarshal(raw, &modified))
		items := mustTestValue[[]any](t, modified["input"])
		item := mustTestValue[object](t, items[1])
		parts := mustTestValue[[]any](t, item["output"])
		part := mustTestValue[object](t, parts[0])
		change(part)
		invalid, err := json.Marshal(modified)
		require.NoError(t, err)
		_, err = PrepareNativeImages(invalid)
		require.Error(t, err)
		require.NotContains(t, err.Error(), "PRIVATE_INVALID")
		_, _, err = bridge.Reprepare(invalid)
		require.Error(t, err, "reprepare must not admit new/unvalidated screenshots or invalid metadata")
	}
	// The native grant is restricted to tool output, even for an identical URL.
	messageRaw := nativeTestRequest(t, url)
	_, _, err = bridge.Reprepare(messageRaw)
	require.Error(t, err)
	source["input"] = append(mustTestValue[[]any](t, source["input"]), object{"role": "user", "content": []any{object{"type": "input_image", "image_url": url}}})
	mixed, err := json.Marshal(source)
	require.NoError(t, err)
	_, err = PrepareNativeImagesWithLimit(mixed, 1)
	require.EqualError(t, err, "local gateway is configured for at most 1 inline images per request")
}
