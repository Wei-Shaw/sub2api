package reqcompress

import (
	"bytes"
	"compress/zlib"
	"context"
	"io"
	"net/http"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/compresslevel"
	"github.com/andybalholm/brotli"
	"github.com/stretchr/testify/require"
)

func TestEncodeBodyWithLevelsUsesDeflateLevel(t *testing.T) {
	body := bytes.Repeat([]byte("compressible-request-body-"), 4096)
	levels := compresslevel.Default()
	levels.Deflate = 0
	uncompressed, err := encodeBodyWithLevels(AlgorithmDeflate, body, levels)
	require.NoError(t, err)
	levels.Deflate = 9
	compressed, err := encodeBodyWithLevels(AlgorithmDeflate, body, levels)
	require.NoError(t, err)
	require.Greater(t, len(uncompressed), len(compressed))

	reader, err := zlib.NewReader(bytes.NewReader(compressed))
	require.NoError(t, err)
	decoded, err := io.ReadAll(reader)
	require.NoError(t, err)
	require.NoError(t, reader.Close())
	require.Equal(t, body, decoded)
}

func TestEncodeBodyWithLevelsUsesBrotliLevel(t *testing.T) {
	body := bytes.Repeat([]byte("compressible-brotli-request-body-"), 4096)
	levels := compresslevel.Default()
	levels.Brotli = 0
	fast, err := encodeBodyWithLevels(AlgorithmBrotli, body, levels)
	require.NoError(t, err)
	levels.Brotli = 11
	best, err := encodeBodyWithLevels(AlgorithmBrotli, body, levels)
	require.NoError(t, err)
	require.Greater(t, len(fast), len(best))
	decoded, err := io.ReadAll(brotli.NewReader(bytes.NewReader(best)))
	require.NoError(t, err)
	require.Equal(t, body, decoded)
}

func TestSharedZstdEncoderReusesFourActualModes(t *testing.T) {
	fastest, err := sharedZstdEncoder(1)()
	require.NoError(t, err)
	fastestAgain, err := sharedZstdEncoder(2)()
	require.NoError(t, err)
	require.Same(t, fastest, fastestAgain)

	defaultMode, err := sharedZstdEncoder(3)()
	require.NoError(t, err)
	require.NotSame(t, fastest, defaultMode)
	better, err := sharedZstdEncoder(6)()
	require.NoError(t, err)
	betterAgain, err := sharedZstdEncoder(9)()
	require.NoError(t, err)
	require.Same(t, better, betterAgain)
	best, err := sharedZstdEncoder(19)()
	require.NoError(t, err)
	require.NotSame(t, better, best)
}

func TestRegistryResolvesOneLevelSnapshotPerRequest(t *testing.T) {
	registry := NewRegistry()
	levels := compresslevel.Default()
	levels.Brotli = 0
	var calls atomic.Int32
	registry.SetLevelProvider(compresslevel.ProviderFunc(func(context.Context) compresslevel.Settings {
		calls.Add(1)
		return levels
	}))
	const accountID int64 = 991
	registry.Sync(accountID, true, time.Time{})
	req := newTestRequest(t, testBody())
	resp, err := registry.Execute(accountID, req, func(attempt *http.Request) (*http.Response, error) {
		require.Equal(t, "br", attempt.Header.Get("Content-Encoding"))
		require.Equal(t, testBody(), decodeRequest(t, attempt))
		return okResponse("ok"), nil
	})
	require.NoError(t, err)
	require.Equal(t, http.StatusOK, resp.StatusCode)
	require.Equal(t, int32(1), calls.Load())
}
