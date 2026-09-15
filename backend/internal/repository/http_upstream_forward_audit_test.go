package repository

import (
	"bytes"
	"compress/gzip"
	"context"
	"encoding/base64"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/pkg/ctxkey"
	"github.com/Wei-Shaw/sub2api/internal/pkg/forwardaudit"
	"github.com/stretchr/testify/require"
)

func TestHTTPUpstreamAuditsRawResponseBeforeDecompression(t *testing.T) {
	plainResponse := []byte(`{"result":"from-upstream"}`)
	var compressedResponse bytes.Buffer
	gzipWriter := gzip.NewWriter(&compressedResponse)
	_, err := gzipWriter.Write(plainResponse)
	require.NoError(t, err)
	require.NoError(t, gzipWriter.Close())
	rawResponse := append([]byte(nil), compressedResponse.Bytes()...)

	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		body, readErr := io.ReadAll(request.Body)
		require.NoError(t, readErr)
		require.JSONEq(t, `{"model":"after-convergence"}`, string(body))
		require.Equal(t, "after-convergence", request.Header.Get("X-Fingerprint-State"))
		writer.Header().Set("Content-Type", "application/json")
		writer.Header().Set("Content-Encoding", "gzip")
		writer.WriteHeader(http.StatusOK)
		_, _ = writer.Write(rawResponse)
	}))
	t.Cleanup(server.Close)

	recorder := forwardaudit.NewRecorder(forwardaudit.Options{
		Enabled:   true,
		Directory: t.TempDir(),
		QueueSize: 16,
	})
	t.Cleanup(recorder.Close)

	clientBody := []byte(`{"model":"before-convergence"}`)
	clientRequest, err := http.NewRequest(http.MethodPost, "https://sub2api.example/v1/responses", bytes.NewReader(clientBody))
	require.NoError(t, err)
	clientRequest = clientRequest.WithContext(context.WithValue(clientRequest.Context(), ctxkey.RequestID, "request-raw-response"))
	clientRequest = forwardaudit.AttachRequest(clientRequest, recorder, 55, 66, "session-raw-response")
	consumed, err := io.ReadAll(clientRequest.Body)
	require.NoError(t, err)
	require.Equal(t, clientBody, consumed)
	require.NoError(t, clientRequest.Body.Close())

	upstreamRequest, err := http.NewRequestWithContext(
		clientRequest.Context(),
		http.MethodPost,
		server.URL,
		strings.NewReader(`{"model":"after-convergence"}`),
	)
	require.NoError(t, err)
	upstreamRequest.Header.Set("Content-Type", "application/json")
	upstreamRequest.Header.Set("Accept-Encoding", "gzip")
	upstreamRequest.Header.Set("X-Fingerprint-State", "after-convergence")
	upstreamRequest = forwardaudit.ActivateRequest(upstreamRequest)

	upstream := NewHTTPUpstream(nil)
	response, err := upstream.Do(upstreamRequest, "", 77, 1)
	require.NoError(t, err)
	require.Empty(t, response.Header.Get("Content-Encoding"), "existing response decompression behavior must remain unchanged")
	actualResponse, err := io.ReadAll(response.Body)
	require.NoError(t, err)
	require.Equal(t, plainResponse, actualResponse)
	require.NoError(t, response.Body.Close())

	recorder.Close()
	records := readForwardAuditRecords(t, recorder.Directory())
	require.Len(t, records, 3)
	responseRecord := recordByStage(t, records, "upstream_response")
	require.Equal(t, float64(http.StatusOK), responseRecord["status_code"])
	require.Equal(t, "gzip", forwardAuditHeaderValue(t, responseRecord, "Content-Encoding"))
	body, ok := responseRecord["body"].(map[string]any)
	require.True(t, ok)
	require.Equal(t, "base64", body["encoding"])
	encoded, ok := body["content"].(string)
	require.True(t, ok)
	decoded, err := base64.StdEncoding.DecodeString(encoded)
	require.NoError(t, err)
	require.Equal(t, rawResponse, decoded, "audit must retain the raw compressed upstream bytes")
}

func readForwardAuditRecords(t *testing.T, directory string) []map[string]any {
	t.Helper()
	var records []map[string]any
	require.NoError(t, filepath.WalkDir(directory, func(path string, entry os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".jsonl") {
			return nil
		}
		content, readErr := os.ReadFile(path)
		if readErr != nil {
			return readErr
		}
		for _, line := range bytes.Split(bytes.TrimSpace(content), []byte("\n")) {
			if len(line) == 0 {
				continue
			}
			var record map[string]any
			if unmarshalErr := json.Unmarshal(line, &record); unmarshalErr != nil {
				return unmarshalErr
			}
			records = append(records, record)
		}
		return nil
	}))
	return records
}

func recordByStage(t *testing.T, records []map[string]any, stage string) map[string]any {
	t.Helper()
	for _, record := range records {
		if record["stage"] == stage {
			return record
		}
	}
	t.Fatalf("missing stage %q in %#v", stage, records)
	return nil
}

func forwardAuditHeaderValue(t *testing.T, record map[string]any, name string) string {
	t.Helper()
	headers, ok := record["headers"].(map[string]any)
	require.True(t, ok)
	values, ok := headers[name].([]any)
	require.True(t, ok)
	require.NotEmpty(t, values)
	value, ok := values[0].(string)
	require.True(t, ok)
	return value
}
