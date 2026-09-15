package forwardaudit

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime/debug"
	"strings"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/ctxkey"
	"github.com/stretchr/testify/require"
)

func TestRecorderCapturesClientRequestUpstreamRequestAndRawResponse(t *testing.T) {
	recorder := NewRecorder(Options{
		Enabled:   true,
		Directory: t.TempDir(),
		QueueSize: 16,
	})
	t.Cleanup(recorder.Close)

	clientBody := []byte(`{"model":"client-model","prompt_cache_key":"body-session"}`)
	clientRequest, err := http.NewRequest(http.MethodPost, "https://sub2api.example/v1/responses?visible=yes", bytes.NewReader(clientBody))
	require.NoError(t, err)
	clientRequest.Header.Set("Authorization", "Bearer client-secret")
	clientRequest.Header.Set("Cookie", "session=client-cookie")
	clientRequest.Header.Set("X-Visible-Client", "visible-client-value")
	clientRequest = clientRequest.WithContext(context.WithValue(clientRequest.Context(), ctxkey.RequestID, "request-1"))
	clientRequest = AttachRequest(clientRequest, recorder, 42, 7, "session-alpha")

	consumedClientBody, err := io.ReadAll(clientRequest.Body)
	require.NoError(t, err)
	require.Equal(t, clientBody, consumedClientBody, "auditing must not change the client request body")
	require.NoError(t, clientRequest.Body.Close())

	upstreamBody := []byte(`{"model":"normalized-model","input":"hello"}`)
	upstreamRequest, err := http.NewRequestWithContext(
		clientRequest.Context(),
		http.MethodPost,
		"https://upstream.example/v1/responses?api_key=upstream-query-secret&visible=yes",
		bytes.NewReader(upstreamBody),
	)
	require.NoError(t, err)
	upstreamRequest.Header.Set("Authorization", "Bearer upstream-secret")
	upstreamRequest.Header.Set("X-API-Key", "upstream-api-key")
	upstreamRequest.Header.Set("X-Codex-Fingerprint", "converged-fingerprint")
	upstreamRequest = ActivateRequest(upstreamRequest)

	rawResponseBody := []byte("raw-upstream-response")
	base := roundTripFunc(func(request *http.Request) (*http.Response, error) {
		actualBody, readErr := io.ReadAll(request.Body)
		require.NoError(t, readErr)
		require.Equal(t, upstreamBody, actualBody, "auditing must not change the upstream request body")
		require.NoError(t, request.Body.Close())
		return &http.Response{
			Status:     "202 Accepted",
			StatusCode: http.StatusAccepted,
			Header: http.Header{
				"Content-Type":       []string{"application/json"},
				"Set-Cookie":         []string{"upstream_session=secret"},
				"X-Visible-Upstream": []string{"visible-upstream-value"},
			},
			Body: io.NopCloser(bytes.NewReader(rawResponseBody)),
		}, nil
	})

	response, err := WrapTransport(base, 99).RoundTrip(upstreamRequest)
	require.NoError(t, err)
	require.Equal(t, http.StatusAccepted, response.StatusCode)
	actualResponseBody, err := io.ReadAll(response.Body)
	require.NoError(t, err)
	require.Equal(t, rawResponseBody, actualResponseBody, "auditing must not change the upstream response body")
	require.NoError(t, response.Body.Close())

	recorder.Close()
	records := readAuditRecords(t, recorder.Directory())
	require.Len(t, records, 3)

	byStage := make(map[string]map[string]any, len(records))
	for _, record := range records {
		stage, _ := record["stage"].(string)
		byStage[stage] = record
		require.Equal(t, float64(42), record["user_id"])
		require.Equal(t, float64(7), record["api_key_id"])
		require.Equal(t, "session-alpha", record["session_id"])
		require.Equal(t, "request-1", record["request_id"])
	}

	clientRecord := byStage["client_request"]
	require.NotNil(t, clientRecord)
	require.Equal(t, string(clientBody), auditBodyContent(t, clientRecord))
	require.Equal(t, "[REDACTED]", auditHeaderValue(t, clientRecord, "Authorization"))
	require.Equal(t, "[REDACTED]", auditHeaderValue(t, clientRecord, "Cookie"))
	require.Equal(t, "visible-client-value", auditHeaderValue(t, clientRecord, "X-Visible-Client"))

	upstreamRecord := byStage["upstream_request"]
	require.NotNil(t, upstreamRecord)
	require.Equal(t, float64(99), upstreamRecord["account_id"])
	require.Equal(t, string(upstreamBody), auditBodyContent(t, upstreamRecord))
	require.Equal(t, "[REDACTED]", auditHeaderValue(t, upstreamRecord, "Authorization"))
	require.Equal(t, "[REDACTED]", auditHeaderValue(t, upstreamRecord, "X-Api-Key"))
	require.Equal(t, "converged-fingerprint", auditHeaderValue(t, upstreamRecord, "X-Codex-Fingerprint"))
	upstreamURL, _ := upstreamRecord["url"].(string)
	require.NotContains(t, upstreamURL, "upstream-query-secret")
	require.Contains(t, upstreamURL, "%5BREDACTED%5D")

	responseRecord := byStage["upstream_response"]
	require.NotNil(t, responseRecord)
	require.Equal(t, float64(http.StatusAccepted), responseRecord["status_code"])
	require.Equal(t, string(rawResponseBody), auditBodyContent(t, responseRecord))
	require.Equal(t, "[REDACTED]", auditHeaderValue(t, responseRecord, "Set-Cookie"))
	require.Equal(t, "visible-upstream-value", auditHeaderValue(t, responseRecord, "X-Visible-Upstream"))

	files := auditFiles(t, recorder.Directory())
	require.Len(t, files, 1)
	require.Contains(t, files[0], filepath.Join("user_42", "session_"))
	require.False(t, strings.Contains(files[0], "session-alpha"), "raw session IDs must not be used as path components")
}

func TestTransportErrorRedactsCredentials(t *testing.T) {
	recorder := NewRecorder(Options{Enabled: true, Directory: t.TempDir(), QueueSize: 16})
	t.Cleanup(recorder.Close)

	clientRequest, err := http.NewRequest(http.MethodPost, "https://sub2api.example/v1/responses", strings.NewReader(`{"model":"gpt-5"}`))
	require.NoError(t, err)
	clientRequest = AttachRequest(clientRequest, recorder, 42, 7, "session-error")
	_, err = io.Copy(io.Discard, clientRequest.Body)
	require.NoError(t, err)
	require.NoError(t, clientRequest.Body.Close())

	upstreamRequest, err := http.NewRequestWithContext(clientRequest.Context(), http.MethodPost, "https://api.openai.example/v1/responses", strings.NewReader(`{"model":"gpt-5.1"}`))
	require.NoError(t, err)
	upstreamRequest = ActivateRequest(upstreamRequest)
	_, err = WrapTransport(roundTripFunc(func(request *http.Request) (*http.Response, error) {
		_, _ = io.Copy(io.Discard, request.Body)
		_ = request.Body.Close()
		return nil, errors.New(`Post "https://api.openai.example/v1/responses?api_key=query-secret": Authorization: Bearer bearer-secret`)
	}), 99).RoundTrip(upstreamRequest)
	require.Error(t, err)

	recorder.Close()
	records := readAuditRecords(t, recorder.Directory())
	upstreamRecord := map[string]any(nil)
	for _, record := range records {
		if record["stage"] == "upstream_request" {
			upstreamRecord = record
			break
		}
	}
	require.NotNil(t, upstreamRecord)
	transportError, _ := upstreamRecord["transport_error"].(string)
	require.NotContains(t, transportError, "query-secret")
	require.NotContains(t, transportError, "bearer-secret")
	require.Contains(t, transportError, redactedValue)
}

func TestSanitizeURLStringMalformedURLDoesNotRecurse(t *testing.T) {
	const childEnv = "FORWARD_AUDIT_SANITIZE_MALFORMED_URL_CHILD"
	const malformedURL = "https://api.openai.example/%zz?api_key=query-secret"

	if os.Getenv(childEnv) == "1" {
		debug.SetMaxStack(256 << 10)
		_, _ = os.Stdout.WriteString(sanitizeURLString(malformedURL))
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, os.Args[0], "-test.run=^TestSanitizeURLStringMalformedURLDoesNotRecurse$")
	cmd.Env = append(os.Environ(), childEnv+"=1")
	output, err := cmd.CombinedOutput()
	require.NoErrorf(t, err, "malformed URL sanitization must terminate: %s", output)
	require.NoError(t, ctx.Err())
	require.NotContains(t, string(output), "query-secret")
	require.Contains(t, string(output), redactedValue)
}

func TestSessionResolutionFallsBackFromExplicitHeaderToBodyThenRequestID(t *testing.T) {
	for _, test := range []struct {
		name            string
		explicitSession string
		body            string
		requestID       string
		wantSession     string
		wantSource      string
	}{
		{
			name: "explicit header", explicitSession: "header-session", body: `{"prompt_cache_key":"body-session"}`,
			requestID: "request-header", wantSession: "header-session", wantSource: "header",
		},
		{
			name: "body prompt cache key", body: `{"prompt_cache_key":"body-session"}`,
			requestID: "request-body", wantSession: "body-session", wantSource: "body.prompt_cache_key",
		},
		{
			name: "request id fallback", body: `{"model":"gpt-5"}`,
			requestID: "request-fallback", wantSession: "request-fallback", wantSource: "request_id",
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			recorder := NewRecorder(Options{Enabled: true, Directory: t.TempDir(), QueueSize: 4})
			t.Cleanup(recorder.Close)
			request, err := http.NewRequest(http.MethodPost, "https://sub2api.example/v1/responses", strings.NewReader(test.body))
			require.NoError(t, err)
			request = request.WithContext(context.WithValue(request.Context(), ctxkey.RequestID, test.requestID))
			request = AttachRequest(request, recorder, 42, 7, test.explicitSession)
			_, err = io.Copy(io.Discard, request.Body)
			require.NoError(t, err)

			trace := traceFromContext(request.Context())
			require.NotNil(t, trace)
			sessionID, source := trace.resolveSession()
			require.Equal(t, test.wantSession, sessionID)
			require.Equal(t, test.wantSource, source)
			ReleaseRequest(request)
		})
	}
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(request *http.Request) (*http.Response, error) {
	return f(request)
}

func readAuditRecords(t *testing.T, directory string) []map[string]any {
	t.Helper()
	files := auditFiles(t, directory)
	var records []map[string]any
	for _, name := range files {
		content, err := os.ReadFile(name)
		require.NoError(t, err)
		for _, line := range bytes.Split(bytes.TrimSpace(content), []byte("\n")) {
			if len(line) == 0 {
				continue
			}
			var record map[string]any
			require.NoError(t, json.Unmarshal(line, &record))
			records = append(records, record)
		}
	}
	return records
}

func auditFiles(t *testing.T, directory string) []string {
	t.Helper()
	var files []string
	require.NoError(t, filepath.WalkDir(directory, func(path string, entry os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if !entry.IsDir() && strings.HasSuffix(entry.Name(), ".jsonl") {
			files = append(files, path)
		}
		return nil
	}))
	return files
}

func auditBodyContent(t *testing.T, record map[string]any) string {
	t.Helper()
	body, ok := record["body"].(map[string]any)
	require.True(t, ok)
	content, ok := body["content"].(string)
	require.True(t, ok)
	return content
}

func auditHeaderValue(t *testing.T, record map[string]any, name string) string {
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
