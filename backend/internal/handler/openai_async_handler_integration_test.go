//go:build integration

package handler

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/openaiasync"
	"github.com/Wei-Shaw/sub2api/internal/server/middleware"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/Wei-Shaw/sub2api/internal/testutil/asyncdb"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

func newAsyncIntegration(t *testing.T) (*OpenAIAsyncHandler, *gin.Engine, *sql.DB) {
	t.Helper()
	db := asyncdb.Open(t)
	cfg := &config.Config{OpenAIAsync: config.OpenAIAsyncConfig{Enabled: true, MaxFileBytes: 1 << 20, UserStorageBytes: 10 << 20, UserTemporaryStorageBytes: 5 << 20, MaxActiveBatches: 5, MaxBatchRequests: 100, MaxLineBytes: 1 << 20, FileTTLSeconds: 3600, MaxOutputBytes: 1 << 20, MaxResponseBytes: 1 << 20, WorkerConcurrency: 1, RequestTimeoutSeconds: 10}}
	cfg.Gateway.MaxBodySize = 4 << 20
	h := NewOpenAIAsyncHandler(db, cfg, nil)
	r := gin.New()
	r.Use(func(c *gin.Context) {
		userID := int64(1)
		if c.GetHeader("X-Test-User") == "2" {
			userID = 2
		}
		groupID := int64(1)
		c.Set(string(middleware.ContextKeyAPIKey), &service.APIKey{ID: 1, UserID: userID, GroupID: &groupID})
		c.Next()
	})
	r.POST("/v1/files", h.Upload)
	r.GET("/v1/files", h.ListFiles)
	r.GET("/v1/files/:file_id", h.GetFile)
	r.GET("/v1/files/:file_id/content", h.FileContent)
	r.DELETE("/v1/files/:file_id", h.DeleteFile)
	r.POST("/v1/batches", h.CreateBatch)
	r.GET("/v1/batches", h.ListBatches)
	r.GET("/v1/batches/:batch_id", h.GetBatch)
	r.POST("/v1/batches/:batch_id/cancel", h.CancelBatch)
	return h, r, db
}

func asyncTestUpload(t *testing.T, r http.Handler, purpose, filename, body string) string {
	t.Helper()
	var buf bytes.Buffer
	w := multipart.NewWriter(&buf)
	require.NoError(t, w.WriteField("purpose", purpose))
	p, err := w.CreateFormFile("file", filename)
	require.NoError(t, err)
	_, err = io.WriteString(p, body)
	require.NoError(t, err)
	require.NoError(t, w.Close())
	req := httptest.NewRequest("POST", "/v1/files", &buf)
	req.Header.Set("Content-Type", w.FormDataContentType())
	out := httptest.NewRecorder()
	r.ServeHTTP(out, req)
	require.Equal(t, 200, out.Code, out.Body.String())
	return gjson.Get(out.Body.String(), "id").String()
}

func TestOpenAIAsyncHTTPFilesAndBatchWorker(t *testing.T) {
	h, r, _ := newAsyncIntegration(t)
	line := `{"custom_id":"req1","method":"POST","url":"/v1/responses","body":{"model":"test-model","input":"hi"}}`
	fileID := asyncTestUpload(t, r, "batch", "input.jsonl", line)
	for _, path := range []string{"/v1/files/" + fileID, "/v1/files/" + fileID + "/content"} {
		req := httptest.NewRequest("GET", path, nil)
		req.Header.Set("X-Test-User", "2")
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)
		require.Equal(t, 404, w.Code)
	}
	body := fmt.Sprintf(`{"input_file_id":%q,"endpoint":"/v1/responses","completion_window":"24h"}`, fileID)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest("POST", "/v1/batches", strings.NewReader(body)))
	require.Equal(t, 200, w.Code, w.Body.String())
	id := gjson.Get(w.Body.String(), "id").String()
	var seenModel string
	worker := openaiasync.NewWorker(h.store, func(ctx context.Context, j *openaiasync.Job, i *openaiasync.Item) openaiasync.Result {
		seenModel = gjson.GetBytes(i.Body, "model").String()
		return openaiasync.Result{ID: "batch_req_1", CustomID: i.CustomID, Response: &openaiasync.Response{StatusCode: 200, Body: json.RawMessage(`{"output_text":"ok"}`)}}
	}, 1, 1<<20, time.Second)
	worker.Start()
	defer worker.Stop()
	var b *openaiasync.Batch
	require.Eventually(t, func() bool {
		var err error
		b, err = h.store.GetBatch(context.Background(), 1, id)
		return err == nil && b.Status == "completed"
	}, 5*time.Second, 10*time.Millisecond)
	worker.Stop()
	require.Equal(t, "test-model", seenModel)
	require.NotNil(t, b.OutputFileID)
	w = httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest("GET", "/v1/files/"+*b.OutputFileID+"/content", nil))
	require.Equal(t, 200, w.Code)
	require.Contains(t, w.Body.String(), "req1")
	require.Contains(t, w.Body.String(), "output_text")
}

func TestOpenAIAsyncBatchOnlyUploadPurposes(t *testing.T) {
	_, router, _ := newAsyncIntegration(t)
	for _, purpose := range []string{"user_data", "vision", "batch_output"} {
		t.Run(purpose, func(t *testing.T) {
			var body bytes.Buffer
			multipartWriter := multipart.NewWriter(&body)
			require.NoError(t, multipartWriter.WriteField("purpose", purpose))
			file, err := multipartWriter.CreateFormFile("file", "input.jsonl")
			require.NoError(t, err)
			_, err = io.WriteString(file, "{}")
			require.NoError(t, err)
			require.NoError(t, multipartWriter.Close())
			request := httptest.NewRequest("POST", "/v1/files", &body)
			request.Header.Set("Content-Type", multipartWriter.FormDataContentType())
			response := httptest.NewRecorder()
			router.ServeHTTP(response, request)
			require.Equal(t, http.StatusBadRequest, response.Code)
		})
	}
}
