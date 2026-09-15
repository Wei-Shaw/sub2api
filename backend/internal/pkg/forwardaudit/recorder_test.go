package forwardaudit

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestRecorderGroupsJSONLByUserSessionAndDay(t *testing.T) {
	directory := t.TempDir()
	recorder := NewRecorder(Options{Enabled: true, Directory: directory, QueueSize: 16})
	t.Cleanup(recorder.Close)

	for _, record := range []auditRecord{
		{Timestamp: auditTestTime(2026, time.September, 14), UserID: 42, SessionID: "session-alpha", RequestID: "request-1", Stage: "client_request"},
		{Timestamp: auditTestTime(2026, time.September, 14), UserID: 42, SessionID: "session-alpha", RequestID: "request-2", Stage: "upstream_request"},
		{Timestamp: auditTestTime(2026, time.September, 15), UserID: 42, SessionID: "session-alpha", RequestID: "request-3", Stage: "client_request"},
		{Timestamp: auditTestTime(2026, time.September, 14), UserID: 42, SessionID: "session-beta", RequestID: "request-4", Stage: "client_request"},
		{Timestamp: auditTestTime(2026, time.September, 14), UserID: 84, SessionID: "session-alpha", RequestID: "request-5", Stage: "client_request"},
	} {
		require.True(t, recorder.enqueue(record))
	}

	recorder.Close()
	files := recorderAuditFiles(t, directory)
	require.Len(t, files, 4)

	alpha42DayOne := requireRecorderAuditPath(t, directory, files, 42, "session-alpha", "2026-09-14")
	requireRecorderAuditPath(t, directory, files, 42, "session-alpha", "2026-09-15")
	requireRecorderAuditPath(t, directory, files, 42, "session-beta", "2026-09-14")
	requireRecorderAuditPath(t, directory, files, 84, "session-alpha", "2026-09-14")

	lines := readRecorderAuditLines(t, alpha42DayOne)
	require.Len(t, lines, 2, "the same user/session/day must append to one JSONL file")
}

func TestRecorderHashesSessionPathAndPreventsTraversal(t *testing.T) {
	directory := t.TempDir()
	sessionID := "../../escape/../../../tmp/audit\nsecret"
	recorder := NewRecorder(Options{Enabled: true, Directory: directory, QueueSize: 4})
	t.Cleanup(recorder.Close)

	require.True(t, recorder.enqueue(auditRecord{
		Timestamp: auditTestTime(2026, time.September, 14),
		UserID:    9, SessionID: sessionID, RequestID: "request-traversal", Stage: "client_request",
	}))
	recorder.Close()

	files := recorderAuditFiles(t, directory)
	require.Len(t, files, 1)
	relative, err := filepath.Rel(directory, files[0])
	require.NoError(t, err)
	require.False(t, relative == ".." || strings.HasPrefix(relative, ".."+string(filepath.Separator)))
	require.NotContains(t, relative, sessionID)

	parts := strings.Split(filepath.ToSlash(relative), "/")
	require.Len(t, parts, 3)
	require.Equal(t, "user_9", parts[0])
	require.Equal(t, "2026-09-14.jsonl", parts[2])
	require.True(t, strings.HasPrefix(parts[1], "session_"))

	hashPart := strings.TrimPrefix(parts[1], "session_")
	digest := sha256.Sum256([]byte(sessionID))
	fullHash := hex.EncodeToString(digest[:])
	require.GreaterOrEqual(t, len(hashPart), 16)
	require.True(t, strings.HasPrefix(fullHash, hashPart))
	require.True(t, regexp.MustCompile(`^[0-9a-f]+$`).MatchString(hashPart))
}

func TestSanitizeHeadersIsCaseInsensitiveAndDoesNotModifyInput(t *testing.T) {
	headers := http.Header{
		"aUtHoRiZaTiOn":         []string{"Bearer client-secret"},
		"PROXY-authorization":   []string{"Basic proxy-secret"},
		"x-API-kEy":             []string{"upstream-key"},
		"X-gOoG-aPi-KeY":        []string{"google-key"},
		"api-KEY":               []string{"generic-key"},
		"COOKIE":                []string{"session=cookie-secret"},
		"set-cookie":            []string{"upstream=cookie-secret"},
		"X-auth-TOKEN":          []string{"auth-token"},
		"x-ACCESS-token":        []string{"access-token"},
		"X-amz-security-TOKEN":  []string{"aws-session-token"},
		"X-Client-Secret":       []string{"client-secret"},
		"Location":              []string{"https://client.example/callback?access_token=location-secret&visible=yes"},
		"Referer":               []string{"https://client.example/page?api_key=referer-secret&visible=yes"},
		"X-Visible-Multi-Value": []string{"first", "second"},
	}
	original := headers.Clone()

	redacted := sanitizeHeaders(headers)

	require.Equal(t, original, headers)
	for _, name := range []string{
		"Authorization", "Proxy-Authorization", "X-Api-Key", "X-Goog-Api-Key", "Api-Key",
		"Cookie", "Set-Cookie", "X-Auth-Token", "X-Access-Token", "X-Amz-Security-Token", "X-Client-Secret",
	} {
		require.Equal(t, []string{redactedValue}, redacted[name], "header %q must be redacted", name)
	}
	require.Equal(t, []string{"first", "second"}, redacted["X-Visible-Multi-Value"])
	require.NotContains(t, redacted.Get("Location"), "location-secret")
	require.Contains(t, redacted.Get("Location"), "visible=yes")
	require.NotContains(t, redacted.Get("Referer"), "referer-secret")
	require.Contains(t, redacted.Get("Referer"), "visible=yes")
	redacted["X-Visible-Multi-Value"][0] = "changed"
	require.Equal(t, "first", headers["X-Visible-Multi-Value"][0])
}

func TestRecorderQueueByteLimitDropsWithoutBlockingAndWarnsAsynchronously(t *testing.T) {
	appender := &blockingAuditWriter{started: make(chan struct{}), release: make(chan struct{})}
	warnStarted := make(chan struct{})
	warnRelease := make(chan struct{})
	recorder := NewRecorder(Options{
		Enabled:       true,
		Directory:     t.TempDir(),
		QueueSize:     8,
		MaxQueueBytes: 64,
		Warn: func(Warning) {
			select {
			case <-warnStarted:
			default:
				close(warnStarted)
			}
			<-warnRelease
		},
	})
	recorder.writeRecord = appender.write
	t.Cleanup(func() {
		appender.unblock()
		select {
		case <-warnRelease:
		default:
			close(warnRelease)
		}
		recorder.Close()
	})

	require.True(t, recorder.enqueue(auditRecord{Timestamp: time.Now(), UserID: 1, SessionID: "s", Stage: "one", rawBody: bytes.Repeat([]byte("a"), 32)}))
	select {
	case <-appender.started:
	case <-time.After(time.Second):
		t.Fatal("audit writer did not start")
	}
	require.True(t, recorder.enqueue(auditRecord{Timestamp: time.Now(), UserID: 1, SessionID: "s", Stage: "two", rawBody: bytes.Repeat([]byte("b"), 32)}))

	result := make(chan bool, 1)
	go func() {
		result <- recorder.enqueue(auditRecord{Timestamp: time.Now(), UserID: 1, SessionID: "s", Stage: "dropped", rawBody: bytes.Repeat([]byte("c"), 33)})
	}()
	select {
	case accepted := <-result:
		require.False(t, accepted)
	case <-time.After(250 * time.Millisecond):
		t.Fatal("enqueue blocked while the audit byte budget was full")
	}
	select {
	case <-warnStarted:
	case <-time.After(time.Second):
		t.Fatal("queue overflow did not trigger the asynchronous warning worker")
	}
	// The warning callback is deliberately blocked; enqueue must remain fast.
	start := time.Now()
	require.False(t, recorder.enqueue(auditRecord{Timestamp: time.Now(), UserID: 1, SessionID: "s", Stage: "dropped-again", rawBody: bytes.Repeat([]byte("d"), 33)}))
	require.Less(t, time.Since(start), 100*time.Millisecond)
}

func TestOversizedBodyIsForwardedUnchangedButAuditIsDropped(t *testing.T) {
	warnings := make(chan Warning, 4)
	recorder := NewRecorder(Options{
		Enabled:         true,
		Directory:       t.TempDir(),
		QueueSize:       16,
		MaxBodyBytes:    8,
		MaxCaptureBytes: 16,
		Warn: func(warning Warning) {
			warnings <- warning
		},
	})
	t.Cleanup(recorder.Close)

	original := []byte("0123456789")
	clientRequest, err := http.NewRequest(http.MethodPost, "https://sub2api.example/v1/responses", bytes.NewReader(original))
	require.NoError(t, err)
	clientRequest = AttachRequest(clientRequest, recorder, 42, 7, "session-large")
	forwarded, err := io.ReadAll(clientRequest.Body)
	require.NoError(t, err)
	require.Equal(t, original, forwarded)
	require.NoError(t, clientRequest.Body.Close())

	upstreamRequest, err := http.NewRequestWithContext(clientRequest.Context(), http.MethodPost, "https://api.openai.example/v1/responses", strings.NewReader(`{"ok":true}`))
	require.NoError(t, err)
	upstreamRequest = ActivateRequest(upstreamRequest)
	response, err := WrapTransport(roundTripFunc(func(request *http.Request) (*http.Response, error) {
		_, _ = io.Copy(io.Discard, request.Body)
		_ = request.Body.Close()
		return &http.Response{Status: "200 OK", StatusCode: http.StatusOK, Header: make(http.Header), Body: http.NoBody}, nil
	}), 99).RoundTrip(upstreamRequest)
	require.NoError(t, err)
	require.NoError(t, response.Body.Close())

	ReleaseRequest(clientRequest)
	recorder.Close()
	require.Empty(t, readAuditRecords(t, recorder.Directory()))
	require.Zero(t, recorder.captureBytes.Load())
	select {
	case warning := <-warnings:
		require.Contains(t, warning.Reason, "body")
	case <-time.After(time.Second):
		t.Fatal("oversized body did not emit an audit warning")
	}
}

func TestOversizedStreamingResponseIsForwardedUnchangedAndDropsOnlyResponseAudit(t *testing.T) {
	warnings := make(chan Warning, 4)
	recorder := NewRecorder(Options{
		Enabled: true, Directory: t.TempDir(), QueueSize: 16,
		MaxBodyBytes: 8, MaxCaptureBytes: 32,
		Warn: func(warning Warning) { warnings <- warning },
	})
	t.Cleanup(recorder.Close)

	clientRequest, err := http.NewRequest(http.MethodGet, "https://sub2api.example/v1/models", nil)
	require.NoError(t, err)
	clientRequest = AttachRequest(clientRequest, recorder, 42, 7, "session-stream")
	upstreamRequest, err := http.NewRequestWithContext(clientRequest.Context(), http.MethodGet, "https://api.openai.example/v1/models", nil)
	require.NoError(t, err)
	upstreamRequest = ActivateRequest(upstreamRequest)

	original := []byte("0123456789abcdef")
	response, err := WrapTransport(roundTripFunc(func(request *http.Request) (*http.Response, error) {
		return &http.Response{
			Status: "200 OK", StatusCode: http.StatusOK, Header: http.Header{"Content-Type": []string{"text/event-stream"}},
			Body: io.NopCloser(bytes.NewReader(original)), ContentLength: -1,
		}, nil
	}), 99).RoundTrip(upstreamRequest)
	require.NoError(t, err)
	forwarded, err := io.ReadAll(response.Body)
	require.NoError(t, err)
	require.Equal(t, original, forwarded)
	require.NoError(t, response.Body.Close())

	recorder.Close()
	records := readAuditRecords(t, recorder.Directory())
	require.Len(t, records, 2)
	for _, record := range records {
		require.NotEqual(t, "upstream_response", record["stage"])
	}
	require.Zero(t, recorder.captureBytes.Load())
	select {
	case warning := <-warnings:
		require.Equal(t, "body_too_large", warning.Reason)
		require.Equal(t, "upstream_response", warning.Stage)
	case <-time.After(time.Second):
		t.Fatal("oversized streaming response did not emit an audit warning")
	}
}

func TestRecorderGlobalCaptureBudgetDropsNewCaptureWithoutChangingForwardedBody(t *testing.T) {
	warnings := make(chan Warning, 4)
	recorder := NewRecorder(Options{
		Enabled: true, Directory: t.TempDir(), QueueSize: 4,
		MaxBodyBytes: 32, MaxCaptureBytes: 16,
		Warn: func(warning Warning) { warnings <- warning },
	})
	t.Cleanup(recorder.Close)

	first, err := http.NewRequest(http.MethodPost, "https://sub2api.example/v1/responses", strings.NewReader("first-payload"))
	require.NoError(t, err)
	first = AttachRequest(first, recorder, 42, 7, "first")
	firstBody, err := io.ReadAll(first.Body)
	require.NoError(t, err)
	require.Equal(t, "first-payload", string(firstBody))

	second, err := http.NewRequest(http.MethodPost, "https://sub2api.example/v1/responses", strings.NewReader("second-payload"))
	require.NoError(t, err)
	second = AttachRequest(second, recorder, 42, 7, "second")
	secondBody, err := io.ReadAll(second.Body)
	require.NoError(t, err)
	require.Equal(t, "second-payload", string(secondBody))

	ReleaseRequest(first)
	ReleaseRequest(second)
	require.Zero(t, recorder.captureBytes.Load())
	select {
	case warning := <-warnings:
		require.Equal(t, "capture_budget_exhausted", warning.Reason)
	case <-time.After(time.Second):
		t.Fatal("capture budget exhaustion did not emit an audit warning")
	}
}

func TestInactiveTraceReleaseReturnsGlobalCaptureBudget(t *testing.T) {
	recorder := NewRecorder(Options{
		Enabled: true, Directory: t.TempDir(), QueueSize: 4, MaxBodyBytes: 32, MaxCaptureBytes: 32,
	})
	t.Cleanup(recorder.Close)

	request, err := http.NewRequest(http.MethodPost, "https://sub2api.example/v1/responses", strings.NewReader("inactive-body"))
	require.NoError(t, err)
	request = AttachRequest(request, recorder, 42, 7, "session-inactive")
	forwarded, err := io.ReadAll(request.Body)
	require.NoError(t, err)
	require.Equal(t, "inactive-body", string(forwarded))
	require.Positive(t, recorder.captureBytes.Load())

	ReleaseRequest(request)
	require.Zero(t, recorder.captureBytes.Load())
}

func TestRecorderCleanupRemovesOnlyManagedArchivesOlderThanThirtyDays(t *testing.T) {
	directory := t.TempDir()
	oldFile := createRecorderArchive(t, directory, 42, strings.Repeat("a", 32), "2026-08-14")
	boundaryFile := createRecorderArchive(t, directory, 42, strings.Repeat("a", 32), "2026-08-15")
	currentFile := createRecorderArchive(t, directory, 42, strings.Repeat("a", 32), "2026-09-14")
	unmanagedFile := createRecorderArchive(t, directory, 42, "not-hex", "2026-08-01")
	recorder := &Recorder{directory: directory}

	require.NoError(t, recorder.cleanup(auditTestTime(2026, time.September, 14)))

	require.NoFileExists(t, oldFile)
	require.FileExists(t, boundaryFile)
	require.FileExists(t, currentFile)
	require.FileExists(t, unmanagedFile, "cleanup must ignore paths outside the exact managed layout")
}

func TestRecorderCanBeEnabledAndDisabledAtRuntimeThroughControlFile(t *testing.T) {
	directory := t.TempDir()
	recorder := NewRecorder(Options{Enabled: false, Directory: directory, QueueSize: 4})
	t.Cleanup(recorder.Close)

	require.False(t, recorder.Enabled())
	require.NoError(t, os.WriteFile(filepath.Join(directory, ".enabled"), []byte("true\n"), 0o600))
	recorder.applyControlFile()
	require.True(t, recorder.Enabled())
	require.True(t, recorder.enqueue(auditRecord{
		Timestamp: auditTestTime(2026, time.September, 14), UserID: 42, SessionID: "session-control", RequestID: "enabled", Stage: "client_request",
	}))

	require.NoError(t, os.WriteFile(filepath.Join(directory, ".enabled"), []byte("false\n"), 0o600))
	recorder.applyControlFile()
	require.False(t, recorder.Enabled())
	require.False(t, recorder.enqueue(auditRecord{
		Timestamp: auditTestTime(2026, time.September, 14), UserID: 42, SessionID: "session-control", RequestID: "disabled", Stage: "client_request",
	}))

	recorder.Close()
	files := recorderAuditFiles(t, directory)
	require.Len(t, files, 1)
	lines := readRecorderAuditLines(t, files[0])
	require.Len(t, lines, 1)
	require.Equal(t, "enabled", lines[0]["request_id"])
}

func TestRecorderWriteFailureWarnsAndDoesNotBlockProducer(t *testing.T) {
	warnings := make(chan Warning, 1)
	recorder := NewRecorder(Options{
		Enabled: true, Directory: t.TempDir(), QueueSize: 4,
		Warn: func(warning Warning) { warnings <- warning },
	})
	recorder.writeRecord = func(auditRecord) error { return errors.New("disk full") }
	t.Cleanup(recorder.Close)

	start := time.Now()
	require.True(t, recorder.enqueue(auditRecord{
		Timestamp: time.Now(), UserID: 42, SessionID: "session-write-error", RequestID: "request-write-error", Stage: "client_request",
	}))
	require.Less(t, time.Since(start), 100*time.Millisecond)
	select {
	case warning := <-warnings:
		require.Equal(t, "write_failed", warning.Reason)
		require.Equal(t, uint64(1), warning.Dropped)
		require.ErrorContains(t, warning.Err, "disk full")
	case <-time.After(time.Second):
		t.Fatal("write failure did not emit an asynchronous warning")
	}
}

type blockingAuditWriter struct {
	started chan struct{}
	release chan struct{}
	once    sync.Once
}

func (w *blockingAuditWriter) write(_ auditRecord) error {
	w.once.Do(func() { close(w.started) })
	<-w.release
	return nil
}

func (w *blockingAuditWriter) unblock() {
	w.once.Do(func() { close(w.started) })
	select {
	case <-w.release:
	default:
		close(w.release)
	}
}

func auditTestTime(year int, month time.Month, day int) time.Time {
	return time.Date(year, month, day, 12, 0, 0, 0, time.UTC)
}

func requireRecorderAuditPath(t *testing.T, directory string, files []string, userID int64, sessionID, date string) string {
	t.Helper()
	digest := sha256.Sum256([]byte(sessionID))
	fullHash := hex.EncodeToString(digest[:])
	for _, name := range files {
		relative, err := filepath.Rel(directory, name)
		require.NoError(t, err)
		parts := strings.Split(filepath.ToSlash(relative), "/")
		if len(parts) != 3 || parts[0] != fmt.Sprintf("user_%d", userID) || parts[2] != date+".jsonl" {
			continue
		}
		if !strings.HasPrefix(parts[1], "session_") {
			continue
		}
		hashPart := strings.TrimPrefix(parts[1], "session_")
		if len(hashPart) >= 16 && strings.HasPrefix(fullHash, hashPart) {
			return name
		}
	}
	t.Fatalf("missing audit file for user=%d session=%q date=%s in %#v", userID, sessionID, date, files)
	return ""
}

func recorderAuditFiles(t *testing.T, directory string) []string {
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

func readRecorderAuditLines(t *testing.T, path string) []map[string]any {
	t.Helper()
	content, err := os.ReadFile(path)
	require.NoError(t, err)
	var lines []map[string]any
	for _, raw := range strings.Split(strings.TrimSpace(string(content)), "\n") {
		if raw == "" {
			continue
		}
		var line map[string]any
		require.NoError(t, json.Unmarshal([]byte(raw), &line))
		lines = append(lines, line)
	}
	return lines
}

func createRecorderArchive(t *testing.T, directory string, userID int64, sessionHash, date string) string {
	t.Helper()
	dir := filepath.Join(directory, fmt.Sprintf("user_%d", userID), "session_"+sessionHash)
	require.NoError(t, os.MkdirAll(dir, 0o700))
	path := filepath.Join(dir, date+".jsonl")
	require.NoError(t, os.WriteFile(path, []byte("{}\n"), 0o600))
	return path
}
