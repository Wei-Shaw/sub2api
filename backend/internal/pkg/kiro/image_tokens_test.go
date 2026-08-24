//go:build unit

package kiro

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/binary"
	"image"
	"image/color"
	"image/gif"
	"image/jpeg"
	"image/png"
	"net/http"
	"net/http/httptest"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestImageTokensForDimensions(t *testing.T) {
	require.Equal(t, 54, imageTokensForDimensions(200, 200))
	require.Equal(t, 1334, imageTokensForDimensions(1000, 1000))
	require.Equal(t, 1533, imageTokensForDimensions(2000, 1000))
	require.Equal(t, 1600, imageTokensForDimensions(0, 100))
}

func TestEstimateImageTokensSupportedFormats(t *testing.T) {
	tests := []struct {
		name      string
		mediaType string
		data      []byte
	}{
		{name: "png", mediaType: "image/png", data: encodeImageForTokenTest(t, "png", 200, 200)},
		{name: "jpeg", mediaType: "image/jpeg", data: encodeImageForTokenTest(t, "jpeg", 200, 200)},
		{name: "gif", mediaType: "image/gif", data: encodeImageForTokenTest(t, "gif", 200, 200)},
		{name: "webp", mediaType: "image/webp", data: webpConfigForTokenTest(200, 200)},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			encoded := base64.StdEncoding.EncodeToString(tt.data)
			require.Equal(t, 54, EstimateImageTokens(context.Background(), tt.mediaType, encoded))
			require.Equal(t, 54, EstimateImageTokens(context.Background(), tt.mediaType, "data:"+tt.mediaType+";base64,"+encoded))
		})
	}
}

func TestEstimateImageTokensUsesDimensionsNotEncodedLength(t *testing.T) {
	flat := image.NewRGBA(image.Rect(0, 0, 512, 512))
	var flatPNG bytes.Buffer
	require.NoError(t, png.Encode(&flatPNG, flat))

	noisy := image.NewRGBA(image.Rect(0, 0, 512, 512))
	for y := 0; y < 512; y++ {
		for x := 0; x < 512; x++ {
			noisy.SetRGBA(x, y, color.RGBA{R: uint8(x), G: uint8(y), B: uint8(x ^ y), A: 255})
		}
	}
	var noisyPNG bytes.Buffer
	require.NoError(t, png.Encode(&noisyPNG, noisy))
	require.Greater(t, noisyPNG.Len(), flatPNG.Len())

	flatTokens := EstimateImageTokens(context.Background(), "image/png", base64.StdEncoding.EncodeToString(flatPNG.Bytes()))
	noisyTokens := EstimateImageTokens(context.Background(), "image/png", base64.StdEncoding.EncodeToString(noisyPNG.Bytes()))
	require.Equal(t, 350, flatTokens)
	require.Equal(t, flatTokens, noisyTokens)
}

func TestEstimateImageTokensRemoteURLCachesSuccess(t *testing.T) {
	resetImageTokenEstimateStateForTest()
	var requests atomic.Int32
	pngBody := encodeImageForTokenTest(t, "png", 200, 200)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		requests.Add(1)
		w.Header().Set("Content-Type", "image/png")
		_, _ = w.Write(pngBody)
	}))
	defer server.Close()

	const workers = 8
	var wg sync.WaitGroup
	results := make(chan int, workers)
	for range workers {
		wg.Add(1)
		go func() {
			defer wg.Done()
			results <- EstimateImageTokens(context.Background(), "", server.URL+"/image.png")
		}()
	}
	wg.Wait()
	close(results)
	for tokens := range results {
		require.Equal(t, 54, tokens)
	}
	require.Equal(t, int32(1), requests.Load())
}

func TestEstimateImageTokensRemoteFailuresUseCachedFallback(t *testing.T) {
	resetImageTokenEstimateStateForTest()
	var requests atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests.Add(1)
		switch r.URL.Path {
		case "/oversized":
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write(bytes.Repeat([]byte{'x'}, kiroRemoteImageMaxBytes+1))
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer server.Close()

	require.Equal(t, 1600, EstimateImageTokens(context.Background(), "", server.URL+"/missing"))
	require.Equal(t, 1600, EstimateImageTokens(context.Background(), "", server.URL+"/missing"))
	require.Equal(t, int32(1), requests.Load())
	require.Equal(t, 1600, EstimateImageTokens(context.Background(), "", server.URL+"/oversized"))
	require.Equal(t, int32(2), requests.Load())
}

func TestEstimateImageTokensRemoteRespectsContext(t *testing.T) {
	resetImageTokenEstimateStateForTest()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		<-r.Context().Done()
	}))
	defer server.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()
	require.Equal(t, 1600, EstimateImageTokens(ctx, "", server.URL+"/slow"))
}

func TestEstimateImageTokensMalformedDataUsesFallback(t *testing.T) {
	require.Equal(t, 1600, EstimateImageTokens(context.Background(), "image/png", "not-base64"))
	require.Equal(t, 1600, EstimateImageTokens(context.Background(), "", ""))
}

func TestImageTokenCacheIsBounded(t *testing.T) {
	resetImageTokenEstimateStateForTest()
	now := time.Now()
	for i := 0; i <= kiroImageTokenCacheMaxItems; i++ {
		storeImageTokenCache(string(rune(i+1)), i+1, time.Minute, now.Add(time.Duration(i)*time.Nanosecond))
	}
	imageTokenEstimates.Lock()
	defer imageTokenEstimates.Unlock()
	require.Len(t, imageTokenEstimates.entries, kiroImageTokenCacheMaxItems)
	_, hasOldest := imageTokenEstimates.entries[string(rune(1))]
	require.False(t, hasOldest)
}

func encodeImageForTokenTest(t *testing.T, format string, width, height int) []byte {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, width, height))
	var buf bytes.Buffer
	var err error
	switch format {
	case "png":
		err = png.Encode(&buf, img)
	case "jpeg":
		err = jpeg.Encode(&buf, img, nil)
	case "gif":
		err = gif.Encode(&buf, img, nil)
	default:
		t.Fatalf("unsupported test image format %q", format)
	}
	require.NoError(t, err)
	return buf.Bytes()
}

func webpConfigForTokenTest(width, height int) []byte {
	data := make([]byte, 30)
	copy(data[0:4], "RIFF")
	binary.LittleEndian.PutUint32(data[4:8], uint32(len(data)-8))
	copy(data[8:12], "WEBP")
	copy(data[12:16], "VP8X")
	binary.LittleEndian.PutUint32(data[16:20], 10)
	writeUint24LE(data[24:27], width-1)
	writeUint24LE(data[27:30], height-1)
	return data
}

func writeUint24LE(dst []byte, value int) {
	dst[0] = byte(value)
	dst[1] = byte(value >> 8)
	dst[2] = byte(value >> 16)
}

func resetImageTokenEstimateStateForTest() {
	imageTokenEstimates.Lock()
	imageTokenEstimates.entries = make(map[string]imageTokenCacheEntry)
	imageTokenEstimates.Unlock()
}
