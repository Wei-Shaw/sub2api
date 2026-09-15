package repository

import (
	"bytes"
	"compress/gzip"
	"encoding/base64"
	"encoding/json"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/pkg/forwardaudit"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/stretchr/testify/require"
)

func TestHTTPUpstreamAuditCapturesFinalRequestAndRawCompressedResponse(t *testing.T) {
	recorder := forwardaudit.NewRecorder(forwardaudit.Options{
		Enabled:   true,
		Directory: t.TempDir(),
		QueueSize: 16,
	})
	t.Cleanup(recorder.Close)

	inbound, err := http.NewRequest(http.MethodPost, "https://sub2api.example/v1/responses", strings.NewReader(`{"model":"before-normalization"}`))
	require.NoError(t, err)
	inbound = forwardaudit.AttachRequest(inbound, recorder, 51, 8, "session-final-boundary")
	_, err = io.Copy(io.Discard, inbound.Body)
	require.NoError(t, err)
	require.NoError(t, inbound.Body.Close())

	finalBody := []byte(`{"model":"after-normalization","input":"hello"}`)
	upstreamRequest, err := http.NewRequestWithContext(
		inbound.Context(),
		http.MethodPost,
		"https://upstream.example/v1/responses",
		bytes.NewReader(finalBody),
	)
	require.NoError(t, err)
	upstreamRequest.Header.Set("Authorization", "Bearer upstream-secret")
	upstreamRequest.Header.Set("X-Fingerprint-After", "normalized-fingerprint")
	upstreamRequest = forwardaudit.ActivateRequest(upstreamRequest)

	rawPayload := []byte(`{"id":"upstream-response","output":"ok"}`)
	var compressed bytes.Buffer
	zw := gzip.NewWriter(&compressed)
	_, err = zw.Write(rawPayload)
	require.NoError(t, err)
	require.NoError(t, zw.Close())
	rawCompressed := append([]byte(nil), compressed.Bytes()...)

	upstream := NewHTTPUpstream(nil)
	svc, ok := upstream.(*httpUpstreamService)
	require.True(t, ok)

	const accountID int64 = 902
	isolation := svc.getIsolationMode()
	profile := service.HTTPUpstreamProfileDefault
	proxyKey := directProxyKey
	protocolMode := svc.resolveProtocolMode(profile, proxyKey, nil)
	settings := svc.applyProfilePoolSettings(svc.resolvePoolSettings(isolation, 1), profile)
	cacheKey := buildCacheKey(isolation, proxyKey, accountID, protocolMode)

	svc.clients[cacheKey] = &upstreamClientEntry{
		client: &http.Client{Transport: roundTripFunc(func(request *http.Request) (*http.Response, error) {
			actualBody, readErr := io.ReadAll(request.Body)
			require.NoError(t, readErr)
			require.Equal(t, finalBody, actualBody, "audit wrapping must not change the final upstream request")
			return &http.Response{
				Status:        "200 OK",
				StatusCode:    http.StatusOK,
				ContentLength: int64(len(rawCompressed)),
				Header: http.Header{
					"Content-Encoding":    []string{"gzip"},
					"Content-Length":      []string{"123"},
					"Content-Type":        []string{"application/json"},
					"X-Upstream-Original": []string{"kept-for-audit"},
				},
				Body:    io.NopCloser(bytes.NewReader(rawCompressed)),
				Request: request,
			}, nil
		})},
		proxyKey:     proxyKey,
		poolKey:      buildPoolKey(settings, protocolMode),
		protocolMode: protocolMode,
	}

	response, err := svc.Do(upstreamRequest, "", accountID, 1)
	require.NoError(t, err)
	callerBody, err := io.ReadAll(response.Body)
	require.NoError(t, err)
	require.Equal(t, rawPayload, callerBody, "existing callers must continue to receive the decompressed response")
	require.Empty(t, response.Header.Get("Content-Encoding"))
	require.NoError(t, response.Body.Close())

	recorder.Close()
	records := readHTTPUpstreamAuditRecords(t, recorder.Directory())

	upstreamRequestRecord := findHTTPUpstreamAuditStage(t, records, "upstream_request")
	require.Equal(t, float64(accountID), upstreamRequestRecord["account_id"])
	require.Equal(t, finalBody, auditRecordBodyBytes(t, upstreamRequestRecord))
	require.Equal(t, "[REDACTED]", auditRecordHeader(t, upstreamRequestRecord, "Authorization"))
	require.Equal(t, "normalized-fingerprint", auditRecordHeader(t, upstreamRequestRecord, "X-Fingerprint-After"))

	upstreamResponseRecord := findHTTPUpstreamAuditStage(t, records, "upstream_response")
	require.Equal(t, "gzip", auditRecordHeader(t, upstreamResponseRecord, "Content-Encoding"), "audit must retain the original upstream headers")
	require.Equal(t, "kept-for-audit", auditRecordHeader(t, upstreamResponseRecord, "X-Upstream-Original"))
	require.Equal(t, rawCompressed, auditRecordBodyBytes(t, upstreamResponseRecord), "audit must capture bytes before response decompression")
}

func readHTTPUpstreamAuditRecords(t *testing.T, directory string) []map[string]any {
	t.Helper()
	var records []map[string]any
	require.NoError(t, filepath.WalkDir(directory, func(path string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
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

func findHTTPUpstreamAuditStage(t *testing.T, records []map[string]any, stage string) map[string]any {
	t.Helper()
	for _, record := range records {
		if record["stage"] == stage {
			return record
		}
	}
	require.FailNow(t, "missing audit stage", "stage=%s records=%#v", stage, records)
	return nil
}

func auditRecordHeader(t *testing.T, record map[string]any, name string) string {
	t.Helper()
	headers, ok := record["headers"].(map[string]any)
	require.True(t, ok)
	values, ok := headers[name].([]any)
	require.True(t, ok, "missing header %s in %#v", name, headers)
	require.NotEmpty(t, values)
	value, ok := values[0].(string)
	require.True(t, ok)
	return value
}

func auditRecordBodyBytes(t *testing.T, record map[string]any) []byte {
	t.Helper()
	body, ok := record["body"].(map[string]any)
	require.True(t, ok)
	content, ok := body["content"].(string)
	require.True(t, ok)
	if body["encoding"] == "base64" {
		decoded, err := base64.StdEncoding.DecodeString(content)
		require.NoError(t, err)
		return decoded
	}
	return []byte(content)
}
