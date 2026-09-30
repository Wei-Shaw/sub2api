package basispoints

import (
	"bytes"
	"encoding/base64"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"hash/crc32"
	"image"
	"image/gif"
	"image/jpeg"
	"image/png"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func newTestImageRelay(t *testing.T, origin string) (*ImageRelay, error) {
	t.Helper()
	r, err := NewImageRelay(origin, t.TempDir())
	if err == nil {
		t.Cleanup(func() { require.NoError(t, r.Close()) })
	}
	return r, err
}

func decodeTestRelayImage(t *testing.T, raw string) ([]byte, string, error) {
	t.Helper()
	r, err := newTestImageRelay(t, "https://images.example")
	if err != nil {
		return nil, "", err
	}
	img, _, err := r.storeImage(raw, "test")
	if err != nil {
		return nil, "", err
	}
	data, err := os.ReadFile(img.path)
	return data, img.contentType, err
}

func relayTestPNG(t *testing.T) []byte {
	t.Helper()
	var out bytes.Buffer
	require.NoError(t, png.Encode(&out, image.NewRGBA(image.Rect(0, 0, 2, 3))))
	return out.Bytes()
}

func relayTestRequest(t *testing.T, data []byte) []byte {
	t.Helper()
	raw, err := json.Marshal(object{"model": "gpt-6-astra", "input": []any{object{"role": "user", "content": []any{
		object{"type": "input_text", "text": "Inspect this image"},
		object{"type": "input_image", "image_url": "data:image/png;base64," + base64.StdEncoding.EncodeToString(data), "detail": "high"},
	}}}})
	require.NoError(t, err)
	return raw
}

func relayTestURL(t *testing.T, raw []byte) string {
	t.Helper()
	var source object
	require.NoError(t, decode(raw, &source))
	input := mustTestValue[[]any](t, source["input"])
	item := mustTestValue[object](t, input[0])
	parts := mustTestValue[[]any](t, item["content"])
	return text(mustTestValue[object](t, parts[1])["image_url"])
}

func TestImageRelayRoundTripAndScope(t *testing.T) {
	r, err := newTestImageRelay(t, "https://images.example/")
	require.NoError(t, err)
	data := relayTestPNG(t)
	raw := relayTestRequest(t, data)
	out, err := r.Rewrite(raw, "account:1/key:2/thread:a")
	require.NoError(t, err)
	url := relayTestURL(t, out)
	require.True(t, strings.HasPrefix(url, "https://images.example"+ImageRelayPath))
	require.NotContains(t, string(out), "data:image")
	require.Contains(t, string(raw), "data:image", "caller input must not be mutated")
	_, _, err = Prepare(out, "scope", nil)
	require.NoError(t, err, "rewritten request must satisfy the actual bridge")
	for _, method := range []string{http.MethodGet, http.MethodHead} {
		w := httptest.NewRecorder()
		r.ServeHTTP(w, httptest.NewRequest(method, url, nil))
		require.Equal(t, 200, w.Code)
		require.Equal(t, "image/png", w.Header().Get("Content-Type"))
		require.Equal(t, "private, no-store", w.Header().Get("Cache-Control"))
		require.Equal(t, "nosniff", w.Header().Get("X-Content-Type-Options"))
		if method == http.MethodGet {
			require.Equal(t, data, w.Body.Bytes())
		} else {
			require.Empty(t, w.Body.Bytes())
		}
	}
	repeated, err := r.Rewrite(raw, "account:1/key:2/thread:a")
	require.NoError(t, err)
	require.Equal(t, out, repeated)
	require.Len(t, r.entries, 1)
	require.EqualValues(t, len(data), r.bytes)
	other, err := r.Rewrite(raw, "account:1/key:3/thread:a")
	require.NoError(t, err)
	require.NotEqual(t, url, relayTestURL(t, other), "API key scopes must not share capabilities")
}

func TestImageRelayPreservesOriginalDetail(t *testing.T) {
	r, err := newTestImageRelay(t, "https://images.example")
	require.NoError(t, err)
	data := relayTestPNG(t)
	var source object
	require.NoError(t, decode(relayTestRequest(t, data), &source))
	item := mustTestValue[object](t, mustTestValue[[]any](t, source["input"])[0])
	part := mustTestValue[object](t, mustTestValue[[]any](t, item["content"])[1])
	part["detail"] = "original"
	for _, toolResult := range []bool{false, true} {
		if toolResult {
			source["input"] = []any{
				object{"type": "function_call", "name": "view_image", "call_id": "call_image", "arguments": `{}`},
				object{"type": "function_call_output", "call_id": "call_image", "output": []any{part}},
			}
		}
		raw, err := json.Marshal(source)
		require.NoError(t, err)
		rewritten, err := r.Rewrite(raw, "scope")
		require.NoError(t, err)
		wire, _, err := Prepare(rewritten, "scope", nil)
		require.NoError(t, err)
		var prepared object
		require.NoError(t, decode(wire, &prepared))
		items := mustTestValue[[]any](t, prepared["input"])
		last := mustTestValue[object](t, items[len(items)-1])
		field, index := "content", 1
		if toolResult {
			index = 2 // Tool images follow their result in a labeled user message.
		}
		image := mustTestValue[object](t, mustTestValue[[]any](t, last[field])[index])
		require.Equal(t, "original", image["detail"])
		w := httptest.NewRecorder()
		r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, text(image["image_url"]), nil))
		require.Equal(t, http.StatusOK, w.Code)
		require.Equal(t, data, w.Body.Bytes())
	}
}

func TestImageRelayToolResultsAndUntouchedFields(t *testing.T) {
	r, err := newTestImageRelay(t, "https://images.example")
	require.NoError(t, err)
	dataURL := "data:image/png;base64," + base64.StdEncoding.EncodeToString(relayTestPNG(t))
	for _, kind := range []string{"function_call_output", "custom_tool_call_output"} {
		raw := []byte(fmt.Sprintf(`{"model":"gpt-6-astra","metadata":{"id":9007199254740993},"input":[{"type":"function_call","name":"view_image","call_id":"call_image","arguments":"{}"},{"type":%q,"call_id":"call_image","output":[{"type":"input_image","image_url":%q},{"type":"input_image","image_url":"https://cdn.example/image.png?sig=a%%2Fb","detail":"low"},{"type":"input_text","text":%q}]}]}`, kind, dataURL, dataURL))
		out, err := r.Rewrite(raw, "tool")
		require.NoError(t, err)
		require.Contains(t, string(out), "9007199254740993")
		require.Contains(t, string(out), "https://cdn.example/image.png?sig=a%2Fb")
		var source object
		require.NoError(t, decode(out, &source))
		input := mustTestValue[[]any](t, source["input"])
		item := mustTestValue[object](t, input[1])
		parts := mustTestValue[[]any](t, item["output"])
		require.Equal(t, dataURL, mustTestValue[object](t, parts[2])["text"])
		wire, _, err := Prepare(out, "tool", nil)
		require.NoError(t, err)
		require.Contains(t, string(wire), "https://images.example"+ImageRelayPath)
	}
}

func TestImageRelayDisabledAndHTTPSPassthrough(t *testing.T) {
	var disabled *ImageRelay
	raw := relayTestRequest(t, relayTestPNG(t))
	out, err := disabled.Rewrite(raw, "scope")
	require.NoError(t, err)
	require.Equal(t, raw, out)
	_, _, err = Prepare(out, "scope", nil)
	require.ErrorContains(t, err, "image support is disabled")
	r, err := newTestImageRelay(t, "https://images.example")
	require.NoError(t, err)
	raw = []byte(`{ "model":"gpt-6-astra", "input":[{"role":"user","content":[{"type":"input_image","image_url":"https://cdn.example/image.png?sig=a%2Fb"}]}] }`)
	out, err = r.Rewrite(raw, "scope")
	require.NoError(t, err)
	require.Equal(t, raw, out)
	require.Empty(t, r.entries)
}

func TestImageRelayRejectsInvalidOrigins(t *testing.T) {
	for _, origin := range []string{"", "http://images.example", "https:///image", "https://user:secret@images.example", "https://images.example/path", "https://images.example?x=y", "https://images.example?", "https://images.example#", "https://images.example/#"} {
		_, err := newTestImageRelay(t, origin)
		require.Error(t, err, origin)
		require.NotContains(t, err.Error(), "secret")
	}
}

func TestImageRelaySupportedFormats(t *testing.T) {
	img := image.NewRGBA(image.Rect(0, 0, 2, 3))
	var jpg, gifBytes bytes.Buffer
	require.NoError(t, jpeg.Encode(&jpg, img, nil))
	require.NoError(t, gif.Encode(&gifBytes, img, nil))
	webp, err := base64.StdEncoding.DecodeString("UklGRiIAAABXRUJQVlA4IBYAAAAwAQCdASoBAAEADsD+JaQAA3AAAAAA")
	require.NoError(t, err)
	for mimeType, data := range map[string][]byte{"image/png": relayTestPNG(t), "image/jpeg": jpg.Bytes(), "image/gif": gifBytes.Bytes(), "image/webp": webp} {
		decoded, contentType, err := decodeTestRelayImage(t, "data:"+mimeType+";base64,"+base64.StdEncoding.EncodeToString(data))
		require.NoError(t, err, mimeType)
		require.Equal(t, mimeType, contentType)
		require.Equal(t, data, decoded)
	}
}

func TestImageRelayRejectsUnsafeOrOversizedData(t *testing.T) {
	pngData := relayTestPNG(t)
	bigDimensions := bytes.Clone(pngData)
	binary.BigEndian.PutUint32(bigDimensions[16:20], 9000)
	binary.BigEndian.PutUint32(bigDimensions[20:24], 9000)
	binary.BigEndian.PutUint32(bigDimensions[29:33], crc32.ChecksumIEEE(bigDimensions[12:29]))
	for _, raw := range []string{
		"data:image/png,no-base64-marker",
		"data:image/png;base64,",
		"data:image/png;base64,PRIVATE_INVALID_PAYLOAD",
		"data:image/png;base64;foo=bar,AAAA",
		"data:image/svg+xml;base64,PHN2Zy8+",
		"data:text/html;base64,PGh0bWw+",
		"data:image/png;base64," + base64.StdEncoding.EncodeToString([]byte("not an image")),
		"data:image/jpeg;base64," + base64.StdEncoding.EncodeToString(pngData),
		"data:image/png;base64," + base64.StdEncoding.EncodeToString(bigDimensions),
		"data:image/png;base64," + strings.Repeat("A", base64.StdEncoding.EncodedLen(imageRelayMaxImageBytes)+1),
	} {
		_, _, err := decodeTestRelayImage(t, raw)
		require.Error(t, err)
		require.NotContains(t, err.Error(), "PRIVATE_INVALID_PAYLOAD")
	}
}

func TestImageRelayBatchValidationAndLimitsAreAtomic(t *testing.T) {
	r, err := newTestImageRelay(t, "https://images.example")
	require.NoError(t, err)
	var source object
	require.NoError(t, decode(relayTestRequest(t, relayTestPNG(t)), &source))
	input := mustTestValue[[]any](t, source["input"])
	item := mustTestValue[object](t, input[0])
	part := mustTestValue[object](t, mustTestValue[[]any](t, item["content"])[1])
	for _, invalid := range []object{
		{"type": "input_image", "image_url": "data:image/png;base64,INVALID"},
		{"type": "input_image", "image_url": part["image_url"], "file_id": "file-private"},
		{"type": "input_image", "image_url": part["image_url"], "detail": "invalid"},
	} {
		item["content"] = []any{part, invalid}
		raw, err := json.Marshal(source)
		require.NoError(t, err)
		_, err = r.Rewrite(raw, "scope")
		require.Error(t, err)
		require.Empty(t, r.entries)
	}
	parts := make([]any, imageRelayMaxRequestImages+1)
	for i := range parts {
		parts[i] = part
	}
	item["content"] = parts
	raw, err := json.Marshal(source)
	require.NoError(t, err)
	_, err = r.Rewrite(raw, "scope")
	require.ErrorContains(t, err, "at most 20")
	require.Empty(t, r.entries)
	limited, err := newTestImageRelay(t, "https://images.example")
	require.NoError(t, err)
	limits := DefaultImageRelayLimits()
	limits.MaxImages = 1
	require.NoError(t, limited.Configure("https://images.example", limits))
	_, err = limited.Rewrite(relayTestRequest(t, relayTestPNG(t)), "scope")
	require.NoError(t, err)
	entriesBeforeLimit := len(limited.entries)
	parts = make([]any, 2)
	for i := range parts {
		parts[i] = part
	}
	item["content"] = parts
	raw, err = json.Marshal(source)
	require.NoError(t, err)
	_, err = limited.Rewrite(raw, "scope")
	require.ErrorContains(t, err, "at most 1")
	require.Len(t, limited.entries, entriesBeforeLimit)
	r.bytes = imageRelayMaxBytes
	_, err = r.Rewrite(relayTestRequest(t, relayTestPNG(t)), "scope")
	require.ErrorIs(t, err, ErrImageRelayFull)
	require.Empty(t, r.entries)
	r.bytes = 0
	for i := 0; i < imageRelayMaxEntries; i++ {
		r.entries[fmt.Sprint(i)] = &relayImage{expires: time.Now().Add(time.Hour)}
	}
	_, err = r.Rewrite(relayTestRequest(t, relayTestPNG(t)), "scope")
	require.ErrorIs(t, err, ErrImageRelayFull)
	require.Len(t, r.entries, imageRelayMaxEntries)
}

func TestImageRelayAggregateRequestByteLimit(t *testing.T) {
	r, err := newTestImageRelay(t, "https://images.example")
	require.NoError(t, err)
	// Valid PNG metadata with padding exercises the decoded byte accounting
	// without requiring a decompression bomb or a large pixel allocation.
	data := make([]byte, imageRelayMaxRequestBytes/2+1)
	copy(data, relayTestPNG(t))
	var source object
	require.NoError(t, decode(relayTestRequest(t, data), &source))
	input := mustTestValue[[]any](t, source["input"])
	item := mustTestValue[object](t, input[0])
	parts := mustTestValue[[]any](t, item["content"])
	item["content"] = []any{parts[1], parts[1]}
	raw, err := json.Marshal(source)
	require.NoError(t, err)
	_, err = r.Rewrite(raw, "scope")
	require.ErrorContains(t, err, "32 MiB")
	require.Empty(t, r.entries)
}

func TestImageRelayExpiryAndMissingTokens(t *testing.T) {
	r, err := newTestImageRelay(t, "https://images.example")
	require.NoError(t, err)
	raw := relayTestRequest(t, relayTestPNG(t))
	out, err := r.Rewrite(raw, "scope")
	require.NoError(t, err)
	url := relayTestURL(t, out)
	token := strings.TrimPrefix(url, r.baseURL+ImageRelayPath)
	old := r.entries[token]
	old.expires = time.Now().Add(-time.Second)
	r.expire(token, old)
	require.Empty(t, r.entries)
	require.Zero(t, r.bytes)
	for _, path := range []string{url, r.baseURL + ImageRelayPath, r.baseURL + ImageRelayPath + strings.Repeat("a", 43), r.baseURL + ImageRelayPath + "../secret"} {
		w := httptest.NewRecorder()
		r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, path, nil))
		require.Equal(t, 404, w.Code)
	}
	_, err = r.Rewrite(raw, "scope")
	require.NoError(t, err)
	r.expire(token, old) // A stale timer must not remove or reschedule a new entry.
	require.Len(t, r.entries, 1)
	r.entries[token].expires = time.Now().Add(-time.Second)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, url, nil))
	require.Equal(t, 404, w.Code, "expiry must be enforced even before a timer runs")
	require.Zero(t, r.bytes)
	w = httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodPost, url, nil))
	require.Equal(t, 405, w.Code)
}

func TestImageRelayConcurrentReuse(t *testing.T) {
	r, err := newTestImageRelay(t, "https://images.example")
	require.NoError(t, err)
	raw := relayTestRequest(t, relayTestPNG(t))
	var wg sync.WaitGroup
	for i := 0; i < 32; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			out, err := r.Rewrite(raw, "scope")
			if err != nil {
				t.Error(err)
				return
			}
			w := httptest.NewRecorder()
			r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, relayTestURL(t, out), nil))
			if w.Code != 200 {
				t.Errorf("GET returned %d", w.Code)
			}
		}()
	}
	wg.Wait()
	require.Len(t, r.entries, 1)
}

func TestImageRelayConcurrentOriginUpdatesPreserveImages(t *testing.T) {
	r, err := newTestImageRelay(t, "https://images.example")
	require.NoError(t, err)
	raw := relayTestRequest(t, relayTestPNG(t))
	initial, err := r.Rewrite(raw, "scope")
	require.NoError(t, err)
	initialURL := relayTestURL(t, initial)
	require.Error(t, r.SetPublicOrigin("http://invalid.example"))
	unchanged, err := r.Rewrite(raw, "scope")
	require.NoError(t, err)
	require.Equal(t, initialURL, relayTestURL(t, unchanged))
	var wg sync.WaitGroup
	for i := 0; i < 32; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			if err := r.SetPublicOrigin(fmt.Sprintf("https://images-%d.example", i)); err != nil {
				t.Error(err)
				return
			}
			out, err := r.Rewrite(raw, "scope")
			if err != nil {
				t.Error(err)
				return
			}
			for _, imageURL := range []string{initialURL, relayTestURL(t, out)} {
				w := httptest.NewRecorder()
				r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, imageURL, nil))
				if w.Code != http.StatusOK {
					t.Errorf("GET returned %d", w.Code)
				}
			}
		}(i)
	}
	wg.Wait()
	require.Len(t, r.entries, 1)
}
