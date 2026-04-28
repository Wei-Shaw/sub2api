package handler

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

var generatedImageHandlerPNG = []byte("\x89PNG\r\n\x1a\nimage-bytes")

func TestGeneratedImageHandlerGetServesStoredImage(t *testing.T) {
	gin.SetMode(gin.TestMode)
	store := service.NewOpenAIGeneratedImageStore(t.TempDir())
	rec, err := store.SaveBase64(context.Background(), service.OpenAIGeneratedImageSaveInput{
		Base64:       base64.StdEncoding.EncodeToString(generatedImageHandlerPNG),
		OutputFormat: "png",
	})
	require.NoError(t, err)

	router := generatedImageHandlerTestRouter(NewGeneratedImageHandler(store))
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/sub2api/generated-images/"+rec.Filename, nil)

	router.ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code)
	require.Equal(t, generatedImageHandlerPNG, w.Body.Bytes())
	require.Equal(t, "image/png", w.Header().Get("Content-Type"))
	require.Equal(t, `inline; filename="`+rec.Filename+`"`, w.Header().Get("Content-Disposition"))
	require.Equal(t, "nosniff", w.Header().Get("X-Content-Type-Options"))
	require.NotEmpty(t, w.Header().Get("Cache-Control"))
}

func TestGeneratedImageHandlerGetRejectsInvalidFilename(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := generatedImageHandlerTestRouter(NewGeneratedImageHandler(service.NewOpenAIGeneratedImageStore(t.TempDir())))
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/sub2api/generated-images/not-an-image.txt", nil)

	router.ServeHTTP(w, req)

	require.Equal(t, http.StatusBadRequest, w.Code)
}

func TestGeneratedImageHandlerGetReturnsNotFoundForMissingAndExpired(t *testing.T) {
	gin.SetMode(gin.TestMode)

	t.Run("missing", func(t *testing.T) {
		router := generatedImageHandlerTestRouter(NewGeneratedImageHandler(service.NewOpenAIGeneratedImageStore(t.TempDir())))
		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/sub2api/generated-images/img_abcdefghijklmnopqrstuvwxyzABCDEF.png", nil)

		router.ServeHTTP(w, req)

		require.Equal(t, http.StatusNotFound, w.Code)
	})

	t.Run("expired", func(t *testing.T) {
		root := t.TempDir()
		store := service.NewOpenAIGeneratedImageStore(root)
		filename := "img_abcdefghijklmnopqrstuvwxyzABCDEF.png"
		id := "img_abcdefghijklmnopqrstuvwxyzABCDEF"
		require.NoError(t, os.WriteFile(filepath.Join(root, filename), generatedImageHandlerPNG, 0o600))
		meta, err := json.Marshal(service.OpenAIGeneratedImageRecord{
			ID:        id,
			Filename:  filename,
			Format:    "png",
			MIME:      "image/png",
			CreatedAt: time.Now().Add(-2 * time.Hour).UTC(),
			ExpiresAt: time.Now().Add(-time.Hour).UTC(),
		})
		require.NoError(t, err)
		require.NoError(t, os.WriteFile(filepath.Join(root, id+".json"), meta, 0o600))

		router := generatedImageHandlerTestRouter(NewGeneratedImageHandler(store))
		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/sub2api/generated-images/"+filename, nil)

		router.ServeHTTP(w, req)

		require.Equal(t, http.StatusNotFound, w.Code)
	})
}

func TestGeneratedImageHandlerGetReturnsInternalServerErrorForUnexpectedStoreError(t *testing.T) {
	gin.SetMode(gin.TestMode)
	root := t.TempDir()
	filename := "img_abcdefghijklmnopqrstuvwxyzABCDEF.png"
	id := "img_abcdefghijklmnopqrstuvwxyzABCDEF"
	require.NoError(t, os.Mkdir(filepath.Join(root, filename), 0o700))
	meta, err := json.Marshal(service.OpenAIGeneratedImageRecord{
		ID:        id,
		Filename:  filename,
		Format:    "png",
		MIME:      "image/png",
		CreatedAt: time.Now().UTC(),
		ExpiresAt: time.Now().Add(time.Hour).UTC(),
	})
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(filepath.Join(root, id+".json"), meta, 0o600))
	router := generatedImageHandlerTestRouter(NewGeneratedImageHandler(service.NewOpenAIGeneratedImageStore(root)))
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/sub2api/generated-images/"+filename, nil)

	router.ServeHTTP(w, req)

	require.Equal(t, http.StatusInternalServerError, w.Code)
}

func TestGeneratedImageHandlerGetReturnsPayloadTooLargeForOversizedRehydrate(t *testing.T) {
	gin.SetMode(gin.TestMode)
	root := t.TempDir()
	filename := "img_abcdefghijklmnopqrstuvwxyzABCDEF.png"
	id := "img_abcdefghijklmnopqrstuvwxyzABCDEF"
	hash := sha256.Sum256(generatedImageHandlerPNG)
	require.NoError(t, os.WriteFile(filepath.Join(root, filename), generatedImageHandlerPNG, 0o600))
	meta, err := json.Marshal(service.OpenAIGeneratedImageRecord{
		ID:           id,
		Filename:     filename,
		Format:       "png",
		MIME:         "image/png",
		SHA256:       hex.EncodeToString(hash[:]),
		DecodedBytes: 21 << 20,
		CreatedAt:    time.Now().UTC(),
		ExpiresAt:    time.Now().Add(time.Hour).UTC(),
	})
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(filepath.Join(root, id+".json"), meta, 0o600))
	router := generatedImageHandlerTestRouter(NewGeneratedImageHandler(service.NewOpenAIGeneratedImageStore(root)))
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/sub2api/generated-images/"+filename, nil)

	router.ServeHTTP(w, req)

	require.Equal(t, http.StatusRequestEntityTooLarge, w.Code)
}

func generatedImageHandlerTestRouter(h *GeneratedImageHandler) *gin.Engine {
	router := gin.New()
	router.GET("/sub2api/generated-images/:filename", h.Get)
	return router
}
