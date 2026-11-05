package middleware

import (
	"bytes"
	"compress/gzip"
	"compress/zlib"
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/pkg/compresslevel"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func compressedResponseWithGzipLevel(t *testing.T, level int, calls *atomic.Int32) *httptest.ResponseRecorder {
	t.Helper()
	gin.SetMode(gin.TestMode)
	engine := gin.New()
	provider := compresslevel.ProviderFunc(func(context.Context) compresslevel.Settings {
		if calls != nil {
			calls.Add(1)
		}
		levels := compresslevel.Default()
		levels.Gzip = level
		return levels
	})
	engine.Use(ResponseCompress(provider))
	payload := strings.Repeat("response-compression-level-payload-", 4096)
	engine.GET("/payload", func(c *gin.Context) { c.String(http.StatusOK, payload) })
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/payload", nil)
	request.Header.Set("Accept-Encoding", "gzip")
	engine.ServeHTTP(recorder, request)
	require.Equal(t, http.StatusOK, recorder.Code)
	reader, err := gzip.NewReader(bytes.NewReader(recorder.Body.Bytes()))
	require.NoError(t, err)
	decoded, err := io.ReadAll(reader)
	require.NoError(t, err)
	require.NoError(t, reader.Close())
	require.Equal(t, payload, string(decoded))
	return recorder
}

func TestResponseCompressUsesConfiguredGzipLevel(t *testing.T) {
	var levelZeroCalls, levelNineCalls atomic.Int32
	levelZero := compressedResponseWithGzipLevel(t, 0, &levelZeroCalls)
	levelNine := compressedResponseWithGzipLevel(t, 9, &levelNineCalls)
	require.Greater(t, levelZero.Body.Len(), levelNine.Body.Len())
	require.Equal(t, int32(1), levelZeroCalls.Load())
	require.Equal(t, int32(1), levelNineCalls.Load())
}

func TestNewCompressorWithLevelsUsesDeflateLevel(t *testing.T) {
	payload := bytes.Repeat([]byte("response-deflate-level-payload-"), 4096)
	levels := compresslevel.Default()
	levels.Deflate = 0
	var fast bytes.Buffer
	compressor, err := newCompressorWithLevels(&fast, "deflate", levels)
	require.NoError(t, err)
	_, err = compressor.Write(payload)
	require.NoError(t, err)
	require.NoError(t, compressor.Close())

	levels.Deflate = 9
	var best bytes.Buffer
	compressor, err = newCompressorWithLevels(&best, "deflate", levels)
	require.NoError(t, err)
	_, err = compressor.Write(payload)
	require.NoError(t, err)
	require.NoError(t, compressor.Close())
	require.Greater(t, fast.Len(), best.Len())

	reader, err := zlib.NewReader(bytes.NewReader(best.Bytes()))
	require.NoError(t, err)
	decoded, err := io.ReadAll(reader)
	require.NoError(t, err)
	require.NoError(t, reader.Close())
	require.Equal(t, payload, decoded)
}

func TestResponseCompressSkipsLevelLookupWithoutCompression(t *testing.T) {
	gin.SetMode(gin.TestMode)
	engine := gin.New()
	var calls atomic.Int32
	provider := compresslevel.ProviderFunc(func(context.Context) compresslevel.Settings {
		calls.Add(1)
		return compresslevel.Default()
	})
	engine.Use(ResponseCompress(provider))
	engine.GET("/payload", func(c *gin.Context) { c.String(http.StatusOK, "plain") })
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/payload", nil)
	engine.ServeHTTP(recorder, request)
	require.Equal(t, http.StatusOK, recorder.Code)
	require.Zero(t, calls.Load())
}
