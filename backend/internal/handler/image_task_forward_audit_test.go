package handler

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/forwardaudit"
	pkghttputil "github.com/Wei-Shaw/sub2api/internal/pkg/httputil"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

// The async worker starts after the submit handler (and its audit cleanup) returns.
func TestNewAsyncImageContextPreservesForwardAuditClientBodyAfterSubmitReturns(t *testing.T) {
	gin.SetMode(gin.TestMode)
	directory := t.TempDir()
	auditRecorder := forwardaudit.NewRecorder(forwardaudit.Options{
		Enabled:   true,
		Directory: directory,
		QueueSize: 16,
	})
	t.Cleanup(auditRecorder.Close)

	clientBody := []byte(`{"model":"gpt-image-1","prompt":"cat","prompt_cache_key":"async-image-session"}`)
	clientRequest := httptest.NewRequest(http.MethodPost, "/v1/images/generations/async", bytes.NewReader(clientBody))
	clientRequest.Header.Set("Content-Type", "application/json")
	clientRequest.Header.Set("X-Request-ID", "async-image-request")
	clientRequest = forwardaudit.AttachRequest(clientRequest, auditRecorder, 42, 7, "")

	responseRecorder := httptest.NewRecorder()
	clientContext, _ := gin.CreateTestContext(responseRecorder)
	clientContext.Request = clientRequest

	readBody, err := pkghttputil.ReadRequestBodyWithPrealloc(clientContext.Request)
	require.NoError(t, err)
	require.Equal(t, clientBody, readBody)

	taskContext, _, cancel := newAsyncImageContext(clientContext, readBody, time.Minute)
	forwardaudit.ReleaseRequest(clientContext.Request)

	upstreamBody := []byte(`{"model":"gpt-image-1","prompt":"cat","quality":"high"}`)
	upstreamRequest := httptest.NewRequest(http.MethodPost, "https://api.openai.com/v1/images/generations", bytes.NewReader(upstreamBody))
	upstreamRequest = upstreamRequest.WithContext(taskContext.Request.Context())
	upstreamRequest = forwardaudit.ActivateRequest(upstreamRequest)
	response, err := forwardaudit.WrapTransport(imageTaskAuditRoundTripFunc(func(request *http.Request) (*http.Response, error) {
		forwardedBody, readErr := io.ReadAll(request.Body)
		require.NoError(t, readErr)
		require.Equal(t, upstreamBody, forwardedBody)
		require.NoError(t, request.Body.Close())
		return &http.Response{
			Status:     "200 OK",
			StatusCode: http.StatusOK,
			Header:     http.Header{"Content-Type": []string{"application/json"}},
			Body:       io.NopCloser(bytes.NewReader([]byte(`{"created":123,"data":[]}`))),
		}, nil
	}), 99).RoundTrip(upstreamRequest)
	require.NoError(t, err)
	_, err = io.Copy(io.Discard, response.Body)
	require.NoError(t, err)
	require.NoError(t, response.Body.Close())
	cancel()

	auditRecorder.Close()
	records := readImageTaskAuditRecords(t, directory)
	require.Len(t, records, 3)

	recordsByStage := make(map[string]map[string]any, len(records))
	for _, record := range records {
		stage, _ := record["stage"].(string)
		if stage != "" {
			recordsByStage[stage] = record
		}
	}
	require.Contains(t, recordsByStage, "upstream_request")
	require.Contains(t, recordsByStage, "upstream_response")
	clientRecord := recordsByStage["client_request"]
	require.NotNil(t, clientRecord)
	require.Equal(t, "async-image-session", clientRecord["session_id"])
	require.Equal(t, "body.prompt_cache_key", clientRecord["session_source"])
	body, ok := clientRecord["body"].(map[string]any)
	require.True(t, ok)
	require.Equal(t, string(clientBody), body["content"])
}

type imageTaskAuditRoundTripFunc func(*http.Request) (*http.Response, error)

func (f imageTaskAuditRoundTripFunc) RoundTrip(request *http.Request) (*http.Response, error) {
	return f(request)
}

func readImageTaskAuditRecords(t *testing.T, directory string) []map[string]any {
	t.Helper()
	var records []map[string]any
	require.NoError(t, filepath.WalkDir(directory, func(path string, entry os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() || filepath.Ext(entry.Name()) != ".jsonl" {
			return nil
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		for _, line := range bytes.Split(bytes.TrimSpace(data), []byte("\n")) {
			if len(line) == 0 {
				continue
			}
			var record map[string]any
			if err := json.Unmarshal(line, &record); err != nil {
				return err
			}
			records = append(records, record)
		}
		return nil
	}))
	return records
}
