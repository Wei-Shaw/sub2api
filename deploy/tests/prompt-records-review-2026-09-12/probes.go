// Audit observations, loaded into securityaudit with Go's overlay mechanism.
// These assertions reproduce the current defects; passing does not mean that
// the corresponding security or reliability requirement has been satisfied.
package securityaudit

import (
	"context"
	"database/sql"
	"encoding/json"
	"net/http"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestAuditObservationCredentialsRetained(t *testing.T) {
	repo := &capturedRequestRepository{records: make(chan *PromptRecord, 1)}
	service := newPromptRecordService(repo, 1, 1, 1)
	service.persist(Request{RequestID: "audit-synthetic", Protocol: "responses", Body: []byte(`{"input":"hello"}`), Headers: http.Header{"Authorization": {"Bearer SYNTHETIC_AUDIT_CREDENTIAL"}, "Cookie": {"session=SYNTHETIC_COOKIE"}, "X-Api-Key": {"SYNTHETIC_API_KEY"}}})
	record := <-repo.records
	require.Contains(t, record.RequestHeaders, "SYNTHETIC_AUDIT_CREDENTIAL")
	require.Contains(t, record.RequestHeaders, "SYNTHETIC_COOKIE")
	require.Contains(t, record.RequestHeaders, "SYNTHETIC_API_KEY")
	require.Nil(t, record.ExpiresAt)
	t.Log("Observed: authentication headers are retained verbatim and ExpiresAt is nil.")
}

func TestAuditObservationAgentPresetWithPath(t *testing.T) {
	body, err := json.Marshal(map[string]any{"instructions": "You are Codex", "input": []any{map[string]any{"role": "user", "content": "# AGENTS.md instructions for D:\\sample\\project\n\n<INSTRUCTIONS>SYNTHETIC_AGENT_PRESET</INSTRUCTIONS>\nPlease explain the code."}}})
	require.NoError(t, err)
	prepared, err := preparePromptRecord(Request{Protocol: "responses", Body: body, recordingFilterPreset: true, recordingFilterAgent: true, recordingFilterSkills: true})
	require.NoError(t, err)
	require.Contains(t, string(prepared.StoredBody), "SYNTHETIC_AGENT_PRESET")
	t.Log("Observed: AGENTS.md instructions for <path> survives enabled preset filtering.")
}

func TestAuditObservationSSEScannerFailureUnmarked(t *testing.T) {
	body := "data: {\"type\":\"response.output_text.delta\",\"delta\":\"prefix\"}\n\n" +
		"data: {\"type\":\"metadata\",\"value\":\"" + strings.Repeat("x", 1024*1024) + "\"}\n\n" +
		"data: {\"type\":\"response.output_text.delta\",\"delta\":\"suffix\"}\n\n"
	result := ExtractResponseText([]byte(body), false)
	require.True(t, result.Recognized)
	require.Equal(t, "prefix", result.Text)
	require.False(t, result.Truncated)
	t.Log("Observed: an oversized SSE line drops the remaining output without a truncation marker.")
}

func TestAuditObservationTruncatedJSONResponseLost(t *testing.T) {
	body, err := json.Marshal(map[string]any{"output_text": strings.Repeat("x", 2*1024*1024)})
	require.NoError(t, err)
	result := ExtractResponseText(body[:2*1024*1024], true)
	require.False(t, result.Recognized)
	require.Empty(t, result.Text)
	t.Log("Observed: a successful JSON response exceeding the capture limit becomes unrecognized and is skipped.")
}

func TestAuditObservationResponseQueueHeadOfLineBlocking(t *testing.T) {
	repo := &blockingPromptRecordRepository{responseUpdated: make(chan PromptRecordKey, 2)}
	service := newPromptRecordService(repo, 4, 1, 1)
	_, first := newPromptRecordingRequestPair(Request{RequestID: "slow-first"})
	_, second := newPromptRecordingRequestPair(Request{RequestID: "ready-second"})
	second.recordingCorrelation.complete(PromptRecordKey{RequestID: "ready-second"}, true)
	service.RecordResponse(context.Background(), first, PromptResponse{Text: "first"})
	service.RecordResponse(context.Background(), second, PromptResponse{Text: "second"})
	select {
	case <-repo.responseUpdated:
		t.Fatal("observation no longer holds: ready response bypassed blocked predecessor")
	case <-time.After(150 * time.Millisecond):
	}
	first.recordingCorrelation.complete(PromptRecordKey{RequestID: "slow-first"}, false)
	select {
	case key := <-repo.responseUpdated:
		require.Equal(t, "ready-second", key.RequestID)
	case <-time.After(time.Second):
		t.Fatal("ready response did not resume after the blocked predecessor completed")
	}
	t.Log("Observed: an already persisted record cannot receive its response while the only response worker waits for another request.")
}

func TestAuditObservationLateRequestLosesResponse(t *testing.T) {
	repo := &blockingPromptRecordRepository{responseUpdated: make(chan PromptRecordKey, 1)}
	service := newPromptRecordService(repo, 1, 1, 1)
	_, reference := newPromptRecordingRequestPair(Request{RequestID: "late-record"})
	service.persistResponse(reference, PromptResponse{Text: "completed before insertion"})
	reference.recordingCorrelation.complete(PromptRecordKey{RequestID: "late-record"}, true)
	require.EqualValues(t, 1, service.QueueStats().PersistFailed)
	select {
	case <-repo.responseUpdated:
		t.Fatal("observation no longer holds: timed-out response was retried")
	default:
	}
	t.Log("Observed: after five seconds waiting for request persistence, the response is discarded even if the request subsequently succeeds.")
}

func TestAuditObservationShutdownDoesNotDrainRecords(t *testing.T) {
	repo := &blockingPromptRecordRepository{started: make(chan struct{}, 1), release: make(chan struct{})}
	recorder := newPromptRecordService(repo, 1, 1, 1)
	defer close(repo.release)
	recorder.RecordPrompt(context.Background(), Request{RequestID: "shutdown", Protocol: "responses", Body: []byte(`{"input":"hello"}`)})
	select {
	case <-repo.started:
	case <-time.After(time.Second):
		t.Fatal("worker did not start")
	}
	service := &PromptService{records: recorder}
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	require.NoError(t, service.Shutdown(ctx))
	require.EqualValues(t, 1, repo.inserted.Load())
	t.Log("Observed: Shutdown returns success while record insertion remains blocked.")
}

func TestAuditRecordRepositoryRoundTrip(t *testing.T) {
	dsn := os.Getenv("RECORD_AUDIT_SYNTHETIC_DSN")
	if dsn == "" {
		t.Skip("isolated synthetic database is not configured")
	}
	db, err := sql.Open("postgres", dsn)
	require.NoError(t, err)
	defer db.Close()
	repo := &PostgreSQLRepository{db: db}
	ctx := context.Background()
	record := &PromptRecord{RequestID: "audit-roundtrip", Stage: "http", PromptHash: strings.Repeat("a", 64), PromptText: "synthetic prompt", RequestBody: `{"input":"synthetic prompt"}`, RequestHeaders: `{"X-Test":["synthetic"]}`, CreatedAt: time.Now()}
	require.NoError(t, repo.InsertPromptRecord(ctx, record))
	require.NoError(t, repo.InsertPromptRecord(ctx, record))
	page, err := repo.ListPromptRecords(ctx, PromptRecordFilter{RequestID: record.RequestID}, 1, 20)
	require.NoError(t, err)
	require.EqualValues(t, 1, page.Total)
	require.Len(t, page.Items, 1)
	updated, err := repo.UpdatePromptRecordResponse(ctx, PromptRecordKey{RequestID: record.RequestID, Stage: "http", PromptHash: record.PromptHash}, PromptResponse{Text: "synthetic response", Length: 18, CapturedAt: time.Now()})
	require.NoError(t, err)
	require.True(t, updated)
	id := page.Items[0].ID
	detail, err := repo.GetPromptRecord(ctx, id)
	require.NoError(t, err)
	require.Equal(t, record.RequestBody, detail.RequestBody)
	require.Equal(t, record.RequestHeaders, detail.RequestHeaders)
	require.Equal(t, "synthetic response", detail.ResponseText)
	deleted, err := repo.DeletePromptRecords(ctx, []int64{id})
	require.NoError(t, err)
	require.EqualValues(t, 1, deleted)
	_, err = repo.GetPromptRecord(ctx, id)
	require.ErrorIs(t, err, ErrPromptRecordNotFound)
	t.Log("Verified: actual PostgreSQL insertion, deduplication, filtering, response correlation, detail lookup and deletion.")
}

func BenchmarkAuditPreparePromptRecord(b *testing.B) {
	for _, size := range []int{1024, 1024 * 1024, 8 * 1024 * 1024} {
		for _, skip := range []bool{false, true} {
			name := "enabled"
			if skip {
				name = "prompt_disabled"
			}
			body, _ := json.Marshal(map[string]any{"input": strings.Repeat("a", size)})
			req := Request{Protocol: "responses", Body: body, recordingSkipPrompt: skip}
			b.Run(name+"/"+fmtAuditSize(size), func(b *testing.B) {
				b.ReportAllocs()
				b.SetBytes(int64(len(body)))
				for i := 0; i < b.N; i++ {
					if _, err := preparePromptRecord(req); err != nil {
						b.Fatal(err)
					}
				}
			})
		}
	}
}

func BenchmarkAuditSaturatedQueueClone(b *testing.B) {
	body, _ := json.Marshal(map[string]any{"input": strings.Repeat("a", 1024*1024)})
	req := Request{Protocol: "responses", Body: body}
	repo := &blockingPromptRecordRepository{}
	service := newPromptRecordService(repo, 1, 1, 1)
	service.once.Do(func() {}) // Hold workers so both buffers remain saturated.
	service.queue <- promptRecordJob{}
	service.overflow <- promptRecordJob{}
	b.ReportAllocs()
	b.SetBytes(int64(len(body)))
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		service.RecordPrompt(context.Background(), req)
	}
}

func fmtAuditSize(size int) string {
	switch size {
	case 1024:
		return "1KiB"
	case 1024 * 1024:
		return "1MiB"
	default:
		return "8MiB"
	}
}
