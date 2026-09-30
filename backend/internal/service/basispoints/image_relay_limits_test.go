package basispoints

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func relayBatch(t *testing.T, data []byte, count int) []byte {
	t.Helper()
	part := object{"type": "input_image", "image_url": "data:image/png;base64," + base64.StdEncoding.EncodeToString(data)}
	parts := make([]any, count)
	for i := range parts {
		parts[i] = part
	}
	raw, err := json.Marshal(object{"input": []any{object{"role": "user", "content": parts}}})
	require.NoError(t, err)
	return raw
}

func TestImageRelayConfigurableCountAndExpiry(t *testing.T) {
	r, err := newTestImageRelay(t, "https://images.example")
	require.NoError(t, err)
	raw := relayBatch(t, relayTestPNG(t), 21)
	_, err = r.Rewrite(raw, "scope")
	require.ErrorContains(t, err, "at most 20")
	require.Empty(t, r.entries)
	require.Zero(t, r.reservedBytes)
	limits := DefaultImageRelayLimits()
	limits.MaxImages = 100
	limits.TTLMinutes = 60
	require.NoError(t, r.Configure(r.baseURL, limits))
	out, err := r.Rewrite(raw, "scope")
	require.NoError(t, err)
	var source object
	require.NoError(t, decode(out, &source))
	input := mustTestValue[[]any](t, source["input"])
	item := mustTestValue[object](t, input[0])
	parts := mustTestValue[[]any](t, item["content"])
	require.Len(t, parts, 21, "conversion must not drop image items")
	part := mustTestValue[object](t, parts[0])
	url := mustTestValue[string](t, part["image_url"])
	token := strings.TrimPrefix(url, r.baseURL+ImageRelayPath)
	require.WithinDuration(t, time.Now().Add(time.Hour), r.entries[token].expires, time.Second)
	oldExpiry := r.entries[token].expires
	limits.TTLMinutes = 1
	require.NoError(t, r.Configure(r.baseURL, limits))
	require.Equal(t, oldExpiry, r.entries[token].expires, "saving must not expire existing links")
	_, err = r.Rewrite(raw, "scope")
	require.NoError(t, err)
	require.WithinDuration(t, time.Now().Add(time.Minute), r.entries[token].expires, time.Second)
	limits.MaxImages = 20
	require.NoError(t, r.Configure(r.baseURL, limits))
	_, err = r.Rewrite(raw, "scope")
	require.ErrorContains(t, err, "at most 20")
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, url, nil))
	require.Equal(t, http.StatusOK, rec.Code, "failed conversion must preserve previously issued links")
}

func TestImageRelayConfigurableSizesAndStorage(t *testing.T) {
	r, err := newTestImageRelay(t, "https://images.example")
	require.NoError(t, err)
	limits := DefaultImageRelayLimits()
	limits.MaxImageMiB = 1
	limits.MaxTotalMiB = 1
	limits.StorageMiB = 2
	require.NoError(t, r.Configure(r.baseURL, limits))
	// Valid PNG followed by padding tests decoded byte limits without image expansion.
	large := append(relayTestPNG(t), bytes.Repeat([]byte{0}, 1<<20)...)
	_, err = r.Rewrite(relayBatch(t, large, 1), "large")
	require.ErrorContains(t, err, "configured 1 MiB limit")
	small := append(relayTestPNG(t), bytes.Repeat([]byte{0}, 600<<10)...)
	_, err = r.Rewrite(relayBatch(t, small, 2), "total")
	require.ErrorContains(t, err, "configured 1 MiB request limit")
	require.Zero(t, r.bytes)
	require.Zero(t, r.reservedBytes)
	out, err := r.Rewrite(relayTestRequest(t, small), "first")
	require.NoError(t, err)
	_, err = r.Rewrite(relayBatch(t, small, 1), "second")
	require.NoError(t, err)
	limits.StorageMiB = 1
	require.NoError(t, r.Configure(r.baseURL, limits))
	_, err = r.Rewrite(relayBatch(t, small, 1), "third")
	require.ErrorIs(t, err, ErrImageRelayFull)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, relayTestURL(t, out), nil))
	require.Equal(t, http.StatusOK, rec.Code)
	limits.StorageMiB = 4
	limits.MaxImageMiB = 2
	limits.MaxTotalMiB = 2
	require.NoError(t, r.Configure(r.baseURL, limits))
	_, err = r.Rewrite(relayBatch(t, large, 1), "large")
	require.NoError(t, err)
	// Reducing the entry quota blocks admissions, not reads of existing files.
	limits.MaxImages = 1
	limits.StorageEntries = 1
	require.NoError(t, r.Configure(r.baseURL, limits))
	_, err = r.Rewrite(relayBatch(t, relayTestPNG(t), 1), "fourth")
	require.ErrorIs(t, err, ErrImageRelayFull)
	require.Zero(t, r.reservedBytes)
}

func TestImageRelayConfigureConcurrentConversions(t *testing.T) {
	r, err := newTestImageRelay(t, "https://images.example")
	require.NoError(t, err)
	raw := relayBatch(t, relayTestPNG(t), 1)
	var wg sync.WaitGroup
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < 20; j++ {
				limits := DefaultImageRelayLimits()
				limits.MaxImages = 20 + j
				limits.TTLMinutes = 1 + j
				if err := r.Configure("https://images.example", limits); err != nil {
					t.Error(err)
					return
				}
				if _, err := r.Rewrite(raw, "same"); err != nil {
					t.Error(err)
					return
				}
			}
		}()
	}
	wg.Wait()
	require.Zero(t, r.reservedBytes)
	require.Zero(t, r.reservedEntries)
	require.Len(t, r.entries, 1)
}

func TestImageRelayExpandedCapacityBounds(t *testing.T) {
	max := ImageRelayLimits{MaxImageMiB: MaxRelayImageMiB, MaxTotalMiB: MaxRelayRequestMiB, MaxImages: MaxRelayImages, StorageMiB: MaxRelayStorageMiB, StorageEntries: MaxRelayStorageEntries, TTLMinutes: MaxRelayTTLMinutes}
	require.NoError(t, max.Validate())
	for _, change := range []func(*ImageRelayLimits){
		func(l *ImageRelayLimits) { l.MaxImageMiB++ },
		func(l *ImageRelayLimits) { l.MaxTotalMiB++ },
		func(l *ImageRelayLimits) { l.MaxImages++ },
		func(l *ImageRelayLimits) { l.StorageMiB++ },
		func(l *ImageRelayLimits) { l.StorageEntries++ },
		func(l *ImageRelayLimits) { l.TTLMinutes++ },
	} {
		invalid := max
		change(&invalid)
		require.Error(t, invalid.Validate())
	}
	relay, err := newTestImageRelay(t, "https://images.example")
	require.NoError(t, err)
	require.NoError(t, relay.Configure(relay.baseURL, max))
	out, err := relay.Rewrite(relayTestRequest(t, relayTestPNG(t)), "expanded")
	require.NoError(t, err)
	rec := httptest.NewRecorder()
	relay.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, relayTestURL(t, out), nil))
	require.Equal(t, http.StatusOK, rec.Code)
	require.Len(t, relay.entries, 1, "raising a ceiling must not preallocate entries")
	require.Zero(t, relay.reservedBytes)
}
